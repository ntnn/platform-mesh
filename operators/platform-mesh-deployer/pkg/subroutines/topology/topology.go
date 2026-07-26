/*
Copyright The Platform Mesh Authors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

// Package topology creates resources for orchestrated operators from the deployer APIs.
package topology

import (
	"context"
	"encoding/json"
	"fmt"

	pmdeployerv1alpha1 "go.platform-mesh.io/apis/deployer/v1alpha1"
	"go.platform-mesh.io/platform-mesh-deployer/pkg/celtemplate"
	"go.platform-mesh.io/platform-mesh-deployer/pkg/clusters"
	"go.platform-mesh.io/platform-mesh-deployer/pkg/components"
	"go.platform-mesh.io/subroutines"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	ctrlruntimeclient "sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"

	operatorv1alpha1 "github.com/kcp-dev/kcp-operator/sdk/apis/operator/v1alpha1"
)

const Name = "TopologySubroutine"

const (
	LabelPlatformMesh = "deployer.platform-mesh.io/platform-mesh"
	LabelComponent    = "deployer.platform-mesh.io/component"
	LabelCluster      = "deployer.platform-mesh.io/cluster"
)

type Subroutine struct {
	client   ctrlruntimeclient.Client
	registry *clusters.Registry
}

func New(client ctrlruntimeclient.Client, registry *clusters.Registry) *Subroutine {
	return &Subroutine{client: client, registry: registry}
}

func (s *Subroutine) GetName() string { return Name }

func (s *Subroutine) Process(ctx context.Context, obj ctrlruntimeclient.Object) (subroutines.Result, error) {
	pm := obj.(*pmdeployerv1alpha1.PlatformMesh)
	if err := s.reconcileRootShard(ctx, pm); err != nil {
		return subroutines.Result{}, err
	}
	return subroutines.OK(), nil
}

func (s *Subroutine) reconcileRootShard(ctx context.Context, pm *pmdeployerv1alpha1.PlatformMesh) error {
	group := pm.Spec.Topology.RootShard

	engaged := s.registry.ClustersFor(pm.Name, components.RootShard)
	if len(engaged) > 1 {
		return fmt.Errorf("root shard must be a single cluster, got %d engaged", len(engaged))
	}
	if len(engaged) == 0 {
		return fmt.Errorf("no root shard cluster engaged")
	}
	cl := engaged[0]

	rs, err := s.buildRootShard(pm, group, cl.ClusterID)
	if err != nil {
		return err
	}
	if err := s.apply(ctx, pm, rs); err != nil {
		return err
	}

	return s.teardownRootShards(ctx, pm, map[string]struct{}{cl.ClusterID: {}})
}

func (s *Subroutine) buildRootShard(pm *pmdeployerv1alpha1.PlatformMesh, group pmdeployerv1alpha1.RootShard, clusterID string) (*operatorv1alpha1.RootShard, error) {
	name := group.Name + "-" + clusterID
	celCtx := celtemplate.Context{
		PlatformMesh: pm.Name,
		Component:    components.RootShard,
		ShardGroup:   group.Name,
		Cluster:      clusterID,
	}

	var spec operatorv1alpha1.RootShardSpec
	if group.Template != nil {
		data, err := json.Marshal(group.Template)
		if err != nil {
			return nil, err
		}
		if err := json.Unmarshal(data, &spec); err != nil {
			return nil, err
		}
	}

	for i, endpoint := range spec.Etcd.Endpoints {
		resolved, err := celtemplate.Eval(endpoint, celCtx)
		if err != nil {
			return nil, fmt.Errorf("root shard %q etcd endpoint: %w", name, err)
		}
		spec.Etcd.Endpoints[i] = resolved
	}
	if spec.Etcd.Prefix != "" {
		resolved, err := celtemplate.Eval(spec.Etcd.Prefix, celCtx)
		if err != nil {
			return nil, fmt.Errorf("root shard %q etcd prefix: %w", name, err)
		}
		spec.Etcd.Prefix = resolved
	}

	host, err := celtemplate.Eval(group.Exposure.HostnameTemplate, celCtx)
	if err != nil {
		return nil, fmt.Errorf("root shard %q hostname: %w", name, err)
	}
	spec.External.Hostname = host
	spec.External.Port = uint32(group.Exposure.Port)

	if group.CacheServerRef != "" {
		spec.Cache.Reference = &corev1.LocalObjectReference{Name: group.CacheServerRef}
	}

	return &operatorv1alpha1.RootShard{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: pm.Namespace,
			Labels: map[string]string{
				LabelPlatformMesh: pm.Name,
				LabelComponent:    components.RootShard,
				LabelCluster:      clusterID,
			},
		},
		Spec: spec,
	}, nil
}

func (s *Subroutine) apply(ctx context.Context, pm *pmdeployerv1alpha1.PlatformMesh, desired *operatorv1alpha1.RootShard) error {
	obj := &operatorv1alpha1.RootShard{ObjectMeta: metav1.ObjectMeta{Name: desired.Name, Namespace: desired.Namespace}}
	_, err := controllerutil.CreateOrUpdate(ctx, s.client, obj, func() error {
		obj.Labels = desired.Labels
		obj.Spec = desired.Spec
		return controllerutil.SetControllerReference(pm, obj, s.client.Scheme())
	})
	return err
}

func (s *Subroutine) teardownRootShards(ctx context.Context, pm *pmdeployerv1alpha1.PlatformMesh, engaged map[string]struct{}) error {
	list := &operatorv1alpha1.RootShardList{}
	if err := s.client.List(ctx, list,
		ctrlruntimeclient.InNamespace(pm.Namespace),
		ctrlruntimeclient.MatchingLabels{LabelPlatformMesh: pm.Name, LabelComponent: components.RootShard},
	); err != nil {
		return err
	}
	for i := range list.Items {
		if _, ok := engaged[list.Items[i].Labels[LabelCluster]]; ok {
			continue
		}
		if err := s.client.Delete(ctx, &list.Items[i]); err != nil && !apierrors.IsNotFound(err) {
			return err
		}
	}
	return nil
}
