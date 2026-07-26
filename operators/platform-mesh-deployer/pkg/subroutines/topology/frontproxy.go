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

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	operatorv1alpha1 "github.com/kcp-dev/kcp-operator/sdk/apis/operator/v1alpha1"
)

func (s *Subroutine) reconcileFrontProxy(ctx context.Context, pm *pmdeployerv1alpha1.PlatformMesh) error {
	frontProxy := pm.Spec.Topology.FrontProxy
	rootRef, err := s.rootShardRef(pm)
	if err != nil {
		return err
	}

	engaged := s.registry.ClustersFor(pm.Name, components.FrontProxy)
	desired := map[string]struct{}{}
	for _, cl := range engaged {
		// "<frontProxy>-<clusterID>": the admin CR name teardown matches back to a cluster.
		name := frontProxy.Name + "-" + cl.ClusterID
		spec, err := s.buildFrontProxySpec(pm, frontProxy, cl.ClusterID, rootRef)
		if err != nil {
			return err
		}
		fp := &operatorv1alpha1.FrontProxy{ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: pm.Namespace}}
		if err := s.apply(ctx, pm, fp, func() {
			fp.Labels = labels(pm.Name, components.FrontProxy, cl.ClusterID)
			fp.Spec = spec
		}); err != nil {
			return err
		}
		desired[name] = struct{}{}
	}
	return s.teardown(ctx, pm, components.FrontProxy, &operatorv1alpha1.FrontProxyList{}, desired)
}

func (s *Subroutine) buildFrontProxySpec(pm *pmdeployerv1alpha1.PlatformMesh, frontProxy pmdeployerv1alpha1.FrontProxy, clusterID, rootRef string) (operatorv1alpha1.FrontProxySpec, error) {
	name := frontProxy.Name + "-" + clusterID
	celCtx := celtemplate.Context{
		PlatformMesh: pm.Name,
		Component:    components.FrontProxy,
		Cluster:      clusterID,
	}

	var spec operatorv1alpha1.FrontProxySpec
	if frontProxy.Template != nil {
		data, err := json.Marshal(frontProxy.Template)
		if err != nil {
			return spec, err
		}
		if err := json.Unmarshal(data, &spec); err != nil {
			return spec, err
		}
	}

	spec.RootShard.Reference = &corev1.LocalObjectReference{Name: rootRef}

	host, err := celtemplate.Eval(frontProxy.Exposure.HostnameTemplate, celCtx)
	if err != nil {
		return spec, fmt.Errorf("front proxy %q hostname: %w", name, err)
	}
	spec.External.Hostname = host
	spec.External.Port = uint32(frontProxy.Exposure.Port)

	return spec, nil
}
