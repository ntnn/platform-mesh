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

package topology

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"strconv"

	pmdeployerv1alpha1 "go.platform-mesh.io/apis/deployer/v1alpha1"
	"go.platform-mesh.io/platform-mesh-deployer/pkg/celtemplate"
	"go.platform-mesh.io/platform-mesh-deployer/pkg/components"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	operatorv1alpha1 "github.com/kcp-dev/kcp-operator/sdk/apis/operator/v1alpha1"
)

func (s *Subroutine) reconcileRootShard(ctx context.Context, pm *pmdeployerv1alpha1.PlatformMesh) error {
	group := pm.Spec.Topology.RootShard

	engaged := s.registry.ClustersFor(pm.Name, components.RootShard)
	if len(engaged) > 1 {
		return fmt.Errorf("root shard must be a single cluster, got %d engaged", len(engaged))
	}
	if len(engaged) == 0 {
		return fmt.Errorf("no root shard cluster engaged")
	}
	clusterID := engaged[0].ClusterID
	name := group.Name + "-" + clusterID

	spec, err := s.buildRootShardSpec(pm, group, clusterID)
	if err != nil {
		return err
	}
	rs := &operatorv1alpha1.RootShard{ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: pm.Namespace}}
	if err := s.apply(ctx, pm, rs, func() {
		rs.Labels = labels(pm.Name, components.RootShard, clusterID)
		rs.Spec = spec
	}); err != nil {
		return err
	}

	return s.teardown(ctx, pm, components.RootShard, &operatorv1alpha1.RootShardList{}, map[string]struct{}{name: {}})
}

// rootShardRef is the name of the single root shard admin CR that shards reference.
func (s *Subroutine) rootShardRef(pm *pmdeployerv1alpha1.PlatformMesh) (string, error) {
	engaged := s.registry.ClustersFor(pm.Name, components.RootShard)
	if len(engaged) != 1 {
		return "", fmt.Errorf("root shard not ready")
	}
	return pm.Spec.Topology.RootShard.Name + "-" + engaged[0].ClusterID, nil
}

// frontProxyExternal returns the front-proxy's hostname and port.
func (s *Subroutine) frontProxyExternal(pm *pmdeployerv1alpha1.PlatformMesh) (string, uint32, error) {
	fp := pm.Spec.Topology.FrontProxy
	engaged := s.registry.ClustersFor(pm.Name, components.FrontProxy)
	if len(engaged) == 0 {
		return "", 0, fmt.Errorf("front proxy not ready")
	}
	host, err := celtemplate.Eval(fp.Exposure.HostnameTemplate, celtemplate.Context{
		PlatformMesh: pm.Name,
		Component:    components.FrontProxy,
		Cluster:      engaged[0].ClusterID,
	})
	if err != nil {
		return "", 0, fmt.Errorf("front proxy hostname: %w", err)
	}
	return host, uint32(fp.Exposure.Port), nil
}

func (s *Subroutine) buildRootShardSpec(pm *pmdeployerv1alpha1.PlatformMesh, group pmdeployerv1alpha1.RootShard, clusterID string) (operatorv1alpha1.RootShardSpec, error) {
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
			return spec, err
		}
		if err := json.Unmarshal(data, &spec); err != nil {
			return spec, err
		}
	}

	if err := resolveEtcd(&spec.Etcd, celCtx, "root shard "+name); err != nil {
		return spec, err
	}

	fpHost, fpPort, err := s.frontProxyExternal(pm)
	if err != nil {
		return spec, fmt.Errorf("root shard %q: %w", name, err)
	}
	spec.External.Hostname = fpHost
	spec.External.Port = fpPort

	host, err := celtemplate.Eval(group.Exposure.HostnameTemplate, celCtx)
	if err != nil {
		return spec, fmt.Errorf("root shard %q hostname: %w", name, err)
	}
	spec.ShardBaseURL = "https://" + net.JoinHostPort(host, strconv.Itoa(int(group.Exposure.Port)))

	if group.CacheServerRef != "" {
		spec.Cache.Reference = &corev1.LocalObjectReference{Name: group.CacheServerRef}
	}
	return spec, nil
}
