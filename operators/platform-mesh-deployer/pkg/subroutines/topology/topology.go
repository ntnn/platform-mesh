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

	pmdeployv1alpha1 "go.platform-mesh.io/apis/deploy/v1alpha1"
	"go.platform-mesh.io/platform-mesh-deployer/pkg/celtemplate"
	"go.platform-mesh.io/platform-mesh-deployer/pkg/clusters"
	"go.platform-mesh.io/platform-mesh-deployer/pkg/components"
	"go.platform-mesh.io/subroutines"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	ctrlruntimeclient "sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"

	operatorv1alpha1 "github.com/kcp-dev/kcp-operator/sdk/apis/operator/v1alpha1"
)

const Name = "TopologySubroutine"

type Subroutine struct {
	client   ctrlruntimeclient.Client
	registry *clusters.Registry
}

func New(client ctrlruntimeclient.Client, registry *clusters.Registry) *Subroutine {
	return &Subroutine{client: client, registry: registry}
}

func (s *Subroutine) GetName() string { return Name }

func (s *Subroutine) Process(ctx context.Context, obj ctrlruntimeclient.Object) (subroutines.Result, error) {
	pm := obj.(*pmdeployv1alpha1.PlatformMesh)
	if err := s.reconcileRootShard(ctx, pm); err != nil {
		return subroutines.Result{}, err
	}
	if err := s.reconcileShards(ctx, pm); err != nil {
		return subroutines.Result{}, err
	}
	if err := s.reconcileFrontProxy(ctx, pm); err != nil {
		return subroutines.Result{}, err
	}
	if err := s.reconcileCacheServer(ctx, pm); err != nil {
		return subroutines.Result{}, err
	}
	if err := s.reconcileVirtualWorkspaces(ctx, pm); err != nil {
		return subroutines.Result{}, err
	}
	return subroutines.OK(), nil
}

// resolveTemplate converts the referenced template CR's spec into out.
// A nil ref leaves out at its zero value.
func (s *Subroutine) resolveTemplate(ctx context.Context, pm *pmdeployv1alpha1.PlatformMesh, ref *pmdeployv1alpha1.TemplateReference, tpl ctrlruntimeclient.Object, spec func() any, out any) error {
	if ref == nil {
		return nil
	}
	namespace := ref.Namespace
	if namespace == "" {
		namespace = pm.Namespace
	}
	key := ctrlruntimeclient.ObjectKey{Namespace: namespace, Name: ref.Name}
	if err := s.client.Get(ctx, key, tpl); err != nil {
		return fmt.Errorf("template %s: %w", key, err)
	}
	// The template spec mirrors the target spec with every field optional.
	data, err := json.Marshal(spec())
	if err != nil {
		return err
	}
	return json.Unmarshal(data, out)
}

// resolveEtcd expands the CEL expressions in the owned etcd endpoints and prefix.
func resolveEtcd(etcd *operatorv1alpha1.EtcdConfig, celCtx celtemplate.Context, what string) error {
	for i, endpoint := range etcd.Endpoints {
		resolved, err := celtemplate.Eval(endpoint, celCtx)
		if err != nil {
			return fmt.Errorf("%s etcd endpoint: %w", what, err)
		}
		etcd.Endpoints[i] = resolved
	}
	if etcd.Prefix != "" {
		resolved, err := celtemplate.Eval(etcd.Prefix, celCtx)
		if err != nil {
			return fmt.Errorf("%s etcd prefix: %w", what, err)
		}
		etcd.Prefix = resolved
	}
	return nil
}

func labels(platformMesh, component, clusterID string) map[string]string {
	return map[string]string{
		components.LabelPlatformMesh: platformMesh,
		components.LabelComponent:    component,
		components.LabelCluster:      clusterID,
	}
}

func (s *Subroutine) apply(ctx context.Context, pm *pmdeployv1alpha1.PlatformMesh, obj ctrlruntimeclient.Object, mutate func()) error {
	_, err := controllerutil.CreateOrUpdate(ctx, s.client, obj, func() error {
		mutate()
		return controllerutil.SetControllerReference(pm, obj, s.client.Scheme())
	})
	return err
}

// teardown deletes admin CRs of the deleted component identified by their name.
func (s *Subroutine) teardown(ctx context.Context, pm *pmdeployv1alpha1.PlatformMesh, component string, list ctrlruntimeclient.ObjectList, desired map[string]struct{}) error {
	if err := s.client.List(ctx, list,
		ctrlruntimeclient.InNamespace(pm.Namespace),
		ctrlruntimeclient.MatchingLabels{components.LabelPlatformMesh: pm.Name, components.LabelComponent: component},
	); err != nil {
		return err
	}
	items, err := meta.ExtractList(list)
	if err != nil {
		return err
	}
	for _, item := range items {
		obj := item.(ctrlruntimeclient.Object)
		if _, ok := desired[obj.GetName()]; ok {
			continue
		}
		if err := s.client.Delete(ctx, obj); err != nil && !apierrors.IsNotFound(err) {
			return err
		}
	}
	return nil
}
