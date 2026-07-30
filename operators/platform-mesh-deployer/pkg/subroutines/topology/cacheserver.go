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

	pmdeployerv1alpha1 "go.platform-mesh.io/apis/deployer/v1alpha1"
	"go.platform-mesh.io/platform-mesh-deployer/pkg/celtemplate"
	"go.platform-mesh.io/platform-mesh-deployer/pkg/components"
	"go.platform-mesh.io/platform-mesh-deployer/pkg/names"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	operatorv1alpha1 "github.com/kcp-dev/kcp-operator/sdk/apis/operator/v1alpha1"
)

func (s *Subroutine) reconcileCacheServer(ctx context.Context, pm *pmdeployerv1alpha1.PlatformMesh) error {
	cacheServer := pm.Spec.Topology.CacheServer

	desired := map[string]struct{}{}
	if cacheServer != nil {
		engaged := s.registry.ClustersFor(pm.Name, components.CacheServer)
		for _, cl := range engaged {
			name := names.CacheServer(pm.Name, cacheServer.Name, cl.ClusterID)
			spec, err := s.buildCacheServerSpec(pm, *cacheServer, cl.ClusterID)
			if err != nil {
				return err
			}
			cs := &operatorv1alpha1.CacheServer{ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: pm.Namespace}}
			if err := s.apply(ctx, pm, cs, func() {
				cs.Labels = labels(pm.Name, components.CacheServer, cl.ClusterID)
				cs.Spec = spec
			}); err != nil {
				return err
			}
			desired[name] = struct{}{}
		}
	}
	return s.teardown(ctx, pm, components.CacheServer, &operatorv1alpha1.CacheServerList{}, desired)
}

// cacheServerRef is the name of the CacheServer admin CR a shard references.
// Federating several cache servers is not supported in v1alpha1, so exactly
// one must be engaged.
func (s *Subroutine) cacheServerRef(pm *pmdeployerv1alpha1.PlatformMesh, ref string) (string, error) {
	cacheServer := pm.Spec.Topology.CacheServer
	if cacheServer == nil {
		return "", fmt.Errorf("cacheServerRef %q set but no cache server defined", ref)
	}
	if cacheServer.Name != ref {
		return "", fmt.Errorf("cacheServerRef %q does not match cache server %q", ref, cacheServer.Name)
	}
	engaged := s.registry.ClustersFor(pm.Name, components.CacheServer)
	if len(engaged) != 1 {
		return "", fmt.Errorf("cache server %q not ready", ref)
	}
	return names.CacheServer(pm.Name, cacheServer.Name, engaged[0].ClusterID), nil
}

func (s *Subroutine) buildCacheServerSpec(pm *pmdeployerv1alpha1.PlatformMesh, cacheServer pmdeployerv1alpha1.CacheServer, clusterID string) (operatorv1alpha1.CacheServerSpec, error) {
	name := names.CacheServer(pm.Name, cacheServer.Name, clusterID)
	celCtx := celtemplate.Context{
		PlatformMesh: pm.Name,
		Component:    components.CacheServer,
		Cluster:      clusterID,
	}

	var spec operatorv1alpha1.CacheServerSpec
	if cacheServer.Template != nil {
		data, err := json.Marshal(cacheServer.Template)
		if err != nil {
			return spec, err
		}
		if err := json.Unmarshal(data, &spec); err != nil {
			return spec, err
		}
	}

	if spec.Etcd != nil {
		if err := resolveEtcd(spec.Etcd, celCtx, "cache server "+name); err != nil {
			return spec, err
		}
	}
	return spec, nil
}
