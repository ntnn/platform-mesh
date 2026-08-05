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

package modules

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	pmdeployv1alpha1 "go.platform-mesh.io/apis/deploy/v1alpha1"
	"go.platform-mesh.io/platform-mesh-deployer/pkg/celtemplate"
	"go.platform-mesh.io/platform-mesh-deployer/pkg/clusters"
	"go.platform-mesh.io/platform-mesh-deployer/pkg/module"
	"go.platform-mesh.io/platform-mesh-deployer/pkg/names"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/multicluster-runtime/pkg/multicluster"

	operatorv1alpha1 "github.com/kcp-dev/kcp-operator/sdk/apis/operator/v1alpha1"
)

func internalScheme(t *testing.T) *runtime.Scheme {
	t.Helper()
	s := runtime.NewScheme()
	require.NoError(t, clientgoscheme.AddToScheme(s))
	require.NoError(t, pmdeployv1alpha1.AddToScheme(s))
	require.NoError(t, operatorv1alpha1.AddToScheme(s))
	return s
}

func internalState(t *testing.T, engaged ...string) *state {
	t.Helper()
	reg := clusters.NewRegistry()
	for _, n := range engaged {
		require.NoError(t, reg.Engage(context.Background(), multicluster.ClusterName(n), nil))
	}
	return &state{
		platformMesh: &pmdeployv1alpha1.PlatformMesh{
			ObjectMeta: metav1.ObjectMeta{Name: "customer-a", Namespace: "pm"},
			Spec: pmdeployv1alpha1.PlatformMeshSpec{
				Topology: pmdeployv1alpha1.Topology{
					RootShard:  pmdeployv1alpha1.RootShard{Name: "root"},
					FrontProxy: pmdeployv1alpha1.FrontProxy{Name: "fp"},
				},
			},
		},
	}
}

func internalSubroutine(t *testing.T, engaged ...string) (*Subroutine, *state) {
	t.Helper()
	st := internalState(t, engaged...)
	reg := clusters.NewRegistry()
	for _, n := range engaged {
		require.NoError(t, reg.Engage(context.Background(), multicluster.ClusterName(n), nil))
	}
	s := internalScheme(t)
	return New(fake.NewClientBuilder().WithScheme(s).Build(), reg, nil), st
}

func instance(component string, placement pmdeployv1alpha1.Placement, clusterID, shardGroup string) module.Instance {
	return module.Instance{
		Component: pmdeployv1alpha1.ModuleComponent{
			Name: component, Placement: placement, Namespace: "acme-system",
		},
		Cluster:    clusters.Cluster{ClusterID: clusterID},
		ShardGroup: shardGroup,
	}
}

// A kubeconfig is minted against the topology object of its target, named
// "<spec name>-<cluster ID>".
func TestKubeconfigTarget(t *testing.T) {
	sub, st := internalSubroutine(t, "rootshard#customer-a--east", "frontproxy#customer-a--fp1")

	front, err := sub.kubeconfigTarget(st,
		pmdeployv1alpha1.ModuleKubeconfig{Name: "kcp", Target: pmdeployv1alpha1.KubeconfigTargetFrontProxy},
		instance("app", pmdeployv1alpha1.PlacementRootShard, "east", ""))
	require.NoError(t, err)
	require.NotNil(t, front.FrontProxyRef)
	assert.Equal(t, names.FrontProxy("customer-a", "fp", "fp1"), front.FrontProxyRef.Name)

	root, err := sub.kubeconfigTarget(st,
		pmdeployv1alpha1.ModuleKubeconfig{Name: "kcp", Target: pmdeployv1alpha1.KubeconfigTargetRootShard},
		instance("app", pmdeployv1alpha1.PlacementRootShard, "east", ""))
	require.NoError(t, err)
	require.NotNil(t, root.RootShardRef)
	assert.Equal(t, names.RootShard("customer-a", "root", "east"), root.RootShardRef.Name)

	shard, err := sub.kubeconfigTarget(st,
		pmdeployv1alpha1.ModuleKubeconfig{Name: "kcp", Target: pmdeployv1alpha1.KubeconfigTargetShard},
		instance("agent", pmdeployv1alpha1.PlacementPerShard, "s1", "default"))
	require.NoError(t, err)
	require.NotNil(t, shard.ShardRef)
	assert.Equal(t, names.Shard("customer-a", "default", "s1"), shard.ShardRef.Name)
}

func TestKubeconfigTargetErrors(t *testing.T) {
	tests := []struct {
		name    string
		engaged []string
		kc      pmdeployv1alpha1.ModuleKubeconfig
		inst    module.Instance
		wantErr string
	}{
		{
			name:    "shard target on a component that is not per shard",
			engaged: []string{"rootshard#customer-a--east"},
			kc:      pmdeployv1alpha1.ModuleKubeconfig{Name: "kcp", Target: pmdeployv1alpha1.KubeconfigTargetShard},
			inst:    instance("app", pmdeployv1alpha1.PlacementRootShard, "east", ""),
			wantErr: "not placed per shard",
		},
		{
			name:    "no front proxy engaged",
			kc:      pmdeployv1alpha1.ModuleKubeconfig{Name: "kcp", Target: pmdeployv1alpha1.KubeconfigTargetFrontProxy},
			inst:    instance("app", pmdeployv1alpha1.PlacementRootShard, "east", ""),
			wantErr: "no frontproxy cluster engaged",
		},
		{
			name:    "several front proxies",
			engaged: []string{"frontproxy#customer-a--a", "frontproxy#customer-a--b"},
			kc:      pmdeployv1alpha1.ModuleKubeconfig{Name: "kcp", Target: pmdeployv1alpha1.KubeconfigTargetFrontProxy},
			inst:    instance("app", pmdeployv1alpha1.PlacementRootShard, "east", ""),
			wantErr: "not supported yet",
		},
		{
			name:    "unknown target",
			kc:      pmdeployv1alpha1.ModuleKubeconfig{Name: "kcp", Target: pmdeployv1alpha1.KubeconfigTarget("nowhere")},
			inst:    instance("app", pmdeployv1alpha1.PlacementRootShard, "east", ""),
			wantErr: "unknown target",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			sub, st := internalSubroutine(t, tt.engaged...)
			_, err := sub.kubeconfigTarget(st, tt.kc, tt.inst)
			require.ErrorContains(t, err, tt.wantErr)
		})
	}
}

// The front proxy dials the backend by its in-cluster names, so the serving
// certificate has to cover all of them.
func TestServiceDNSNames(t *testing.T) {
	assert.Equal(t, []string{
		"acme-vw",
		"acme-vw.acme-system",
		"acme-vw.acme-system.svc",
		"acme-vw.acme-system.svc.cluster.local",
	}, serviceDNSNames("acme-vw", "acme-system"))
}

func TestRootShardIssuer(t *testing.T) {
	sub, st := internalSubroutine(t, "rootshard#customer-a--east")
	issuer, err := sub.rootShardIssuer(st)
	require.NoError(t, err)
	assert.Equal(t, names.RootShard("customer-a", "root", "east")+"-server-ca", issuer)

	sub, st = internalSubroutine(t)
	_, err = sub.rootShardIssuer(st)
	require.ErrorContains(t, err, "exactly one root shard")
}

// The requestheader CA is a kcp-operator secret named after the root shard, so
// waiting for it is ordinary progress rather than a failure.
func TestRequestHeaderCAPending(t *testing.T) {
	sub, st := internalSubroutine(t, "rootshard#customer-a--east")
	st.resolved = &module.Resolved{Module: &pmdeployv1alpha1.Module{
		ObjectMeta: metav1.ObjectMeta{Name: "acme", Namespace: "pm"},
	}}
	inst := instance("vw", pmdeployv1alpha1.PlacementPerFrontProxy, "fp1", "")

	err := sub.ensureRequestHeaderCA(context.Background(), st, inst)
	require.ErrorIs(t, err, errRequestHeaderCAPending)
	assert.Contains(t, err.Error(), names.RootShard("customer-a", "root", "east")+"-requestheader-client-ca")
}

// The mapping is templated per instance, so the backend is only known after
// interpolation.
func TestResolveMapping(t *testing.T) {
	inst := instance("vw", pmdeployv1alpha1.PlacementPerFrontProxy, "fp1", "")
	inst.Component.Mapping = &pmdeployv1alpha1.Mapping{
		Path:    "/services/acme/",
		Service: `${module + "-" + component}`,
		Port:    8443,
	}
	celCtx := celtemplate.Context{Module: "acme", Component: "vw"}

	got, err := resolveMapping(inst, celCtx)
	require.NoError(t, err)
	assert.Equal(t, "/services/acme/", got.Path)
	assert.Equal(t, "https://acme-vw.acme-system.svc:8443", got.Backend)
}

func TestResolveMappingRejectsBadTemplates(t *testing.T) {
	tests := []struct{ name, service, path string }{
		{name: "bad service", service: "${nope}", path: "/services/acme/"},
		{name: "bad path", service: "svc", path: "${nope}"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			inst := instance("vw", pmdeployv1alpha1.PlacementPerFrontProxy, "fp1", "")
			inst.Component.Mapping = &pmdeployv1alpha1.Mapping{
				Path: tt.path, Service: tt.service, Port: 8443,
			}
			_, err := resolveMapping(inst, celtemplate.Context{})
			require.Error(t, err)
		})
	}
}

func TestToAnySlice(t *testing.T) {
	assert.Equal(t, []any{"a", "b"}, toAnySlice([]string{"a", "b"}))
	assert.Empty(t, toAnySlice(nil))
}

func TestGetName(t *testing.T) {
	sub, _ := internalSubroutine(t)
	assert.Equal(t, Name, sub.GetName())
}
