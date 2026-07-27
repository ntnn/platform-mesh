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

// Package exposure reconciles the ingress routes.
package exposure

import (
	"context"
	"fmt"

	pmdeployerv1alpha1 "go.platform-mesh.io/apis/deployer/v1alpha1"
	"go.platform-mesh.io/platform-mesh-deployer/pkg/celtemplate"
	"go.platform-mesh.io/platform-mesh-deployer/pkg/clusters"
	"go.platform-mesh.io/platform-mesh-deployer/pkg/components"
	"go.platform-mesh.io/subroutines"

	ctrlruntimeclient "sigs.k8s.io/controller-runtime/pkg/client"
)

const Name = "ExposureSubroutine"

// shardServicePort is the kcp-operator service port for root shards and shards.
// The front proxy service port equals its external port instead.
const shardServicePort = 6443

// routeParams is the stack-agnostic input a renderer turns into its own route object.
type routeParams struct {
	pmName    string
	namespace string
	name      string
	host      string // SNI hostname
	service   string // backend Service name
	port      int32  // backend Service port
	component string
	clusterID string
}

// stackRenderer reconciles the route objects of one ingress stack. It is built
// once per stack with its typed configuration already resolved.
type stackRenderer interface {
	ensure(ctx context.Context, workload ctrlruntimeclient.Client, p routeParams) error
	// teardown removes this stack type's routes of the PlatformMesh not in desired.
	teardown(ctx context.Context, workload ctrlruntimeclient.Client, pmName, namespace string, desired map[string]struct{}) error
}

// newRenderer builds the renderer for one ingress stack, or nil for an
// unsupported type. Each supported type reads its own typed field.
func newRenderer(stack pmdeployerv1alpha1.IngressStack) (stackRenderer, error) {
	switch stack.Type {
	case pmdeployerv1alpha1.IngressTypeGatewayAPI:
		return newGatewayAPIRenderer(stack.GatewayAPI)
	default:
		return nil, nil
	}
}

type Subroutine struct {
	registry *clusters.Registry
}

func New(registry *clusters.Registry) *Subroutine { return &Subroutine{registry: registry} }

func (s *Subroutine) GetName() string { return Name }

// endpoint is one exposed kcp component reachable through the ingress stacks.
type endpoint struct {
	component  string
	shardGroup string
	name       string // admin CR base name
	svcSuffix  string
	// backendPort is the Service port; 0 means use the exposure port (front proxy).
	backendPort int32
	exposure    pmdeployerv1alpha1.Exposure
}

func (s *Subroutine) Process(ctx context.Context, obj ctrlruntimeclient.Object) (subroutines.Result, error) {
	pm := obj.(*pmdeployerv1alpha1.PlatformMesh)

	byName, byType, err := renderers(pm)
	if err != nil {
		return subroutines.Result{}, err
	}
	if len(byName) == 0 {
		return subroutines.OK(), nil
	}

	desired := map[string]struct{}{}
	clients := map[string]ctrlruntimeclient.Client{}

	for _, ep := range endpoints(pm) {
		for _, cl := range s.registry.ClustersFor(pm.Name, ep.component) {
			celCtx := celtemplate.Context{
				PlatformMesh: pm.Name,
				Component:    ep.component,
				ShardGroup:   ep.shardGroup,
				Cluster:      cl.ClusterID,
			}
			host, err := celtemplate.Eval(ep.exposure.HostnameTemplate, celCtx)
			if err != nil {
				return subroutines.Result{}, fmt.Errorf("%s %q hostname: %w", ep.component, ep.name, err)
			}
			adminName := ep.name + "-" + cl.ClusterID
			port := ep.backendPort
			if port == 0 {
				port = ep.exposure.Port
			}

			refs := ep.exposure.IngressRefs
			if len(refs) == 0 {
				refs = keys(byName)
			}
			workload := cl.Cluster.GetClient()
			clients[cl.Name.String()] = workload
			for _, ref := range refs {
				rend, ok := byName[ref]
				if !ok {
					continue
				}
				name := adminName + "-" + ref
				if err := rend.ensure(ctx, workload, routeParams{
					pmName:    pm.Name,
					namespace: pm.Namespace,
					name:      name,
					host:      host,
					service:   adminName + ep.svcSuffix,
					port:      port,
					component: ep.component,
					clusterID: cl.ClusterID,
				}); err != nil {
					return subroutines.Result{}, err
				}
				desired[name] = struct{}{}
			}
		}
	}

	for _, workload := range clients {
		for _, rend := range byType {
			if err := rend.teardown(ctx, workload, pm.Name, pm.Namespace, desired); err != nil {
				return subroutines.Result{}, err
			}
		}
	}
	return subroutines.OK(), nil
}

// endpoints lists the exposed components of a PlatformMesh.
func endpoints(pm *pmdeployerv1alpha1.PlatformMesh) []endpoint {
	t := pm.Spec.Topology
	eps := []endpoint{{
		component:   components.RootShard,
		name:        t.RootShard.Name,
		svcSuffix:   "-kcp",
		backendPort: shardServicePort,
		exposure:    t.RootShard.Exposure,
	}, {
		component:   components.FrontProxy,
		name:        t.FrontProxy.Name,
		svcSuffix:   "-front-proxy",
		backendPort: 0, // front proxy service port == external port
		exposure:    t.FrontProxy.Exposure,
	}}
	for i := range t.ShardGroups {
		g := t.ShardGroups[i]
		if g.Exposure == nil {
			continue
		}
		eps = append(eps, endpoint{
			component:   components.Shard(g.Name),
			shardGroup:  g.Name,
			name:        g.Name,
			svcSuffix:   "-shard-kcp",
			backendPort: shardServicePort,
			exposure:    *g.Exposure,
		})
	}
	return eps
}

// renderers builds a renderer per ingress stack, keyed by stack name for lookup
// and by type for teardown.
func renderers(pm *pmdeployerv1alpha1.PlatformMesh) (map[string]stackRenderer, map[pmdeployerv1alpha1.IngressType]stackRenderer, error) {
	byName := map[string]stackRenderer{}
	byType := map[pmdeployerv1alpha1.IngressType]stackRenderer{}
	for i := range pm.Spec.Ingress {
		stack := pm.Spec.Ingress[i]
		rend, err := newRenderer(stack)
		if err != nil {
			return nil, nil, fmt.Errorf("ingress stack %q: %w", stack.Name, err)
		}
		if rend == nil {
			continue
		}
		byName[stack.Name] = rend
		byType[stack.Type] = rend
	}
	return byName, byType, nil
}

func keys(m map[string]stackRenderer) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	return out
}
