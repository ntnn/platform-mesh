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

package topology_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	pmdeployerv1alpha1 "go.platform-mesh.io/apis/deployer/v1alpha1"
	"go.platform-mesh.io/platform-mesh-deployer/pkg/clusters"
	"go.platform-mesh.io/platform-mesh-deployer/pkg/components"
	"go.platform-mesh.io/platform-mesh-deployer/pkg/names"
	"go.platform-mesh.io/platform-mesh-deployer/pkg/subroutines/topology"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	ctrlruntimeclient "sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/multicluster-runtime/pkg/multicluster"

	operatorv1alpha1 "github.com/kcp-dev/kcp-operator/sdk/apis/operator/v1alpha1"
)

func scheme(t *testing.T) *runtime.Scheme {
	t.Helper()
	s := runtime.NewScheme()
	require.NoError(t, clientgoscheme.AddToScheme(s))
	require.NoError(t, pmdeployerv1alpha1.AddToScheme(s))
	require.NoError(t, operatorv1alpha1.AddToScheme(s))
	return s
}

func platformMesh() *pmdeployerv1alpha1.PlatformMesh {
	return &pmdeployerv1alpha1.PlatformMesh{
		ObjectMeta: metav1.ObjectMeta{Name: "customer-a", Namespace: "pm"},
		Spec: pmdeployerv1alpha1.PlatformMeshSpec{
			Topology: pmdeployerv1alpha1.Topology{
				RootShard: pmdeployerv1alpha1.RootShard{
					Name:        "root",
					TemplateRef: &pmdeployerv1alpha1.TemplateReference{Name: "root"},
					Exposure: pmdeployerv1alpha1.Exposure{
						HostnameTemplate: `"kcp." + platformMesh + ".example.com"`,
						Port:             6443,
					},
				},
				FrontProxy: pmdeployerv1alpha1.FrontProxy{
					Name: "fp",
					Exposure: pmdeployerv1alpha1.Exposure{
						HostnameTemplate: `"fp." + platformMesh + ".example.com"`,
						Port:             6443,
					},
				},
			},
		},
	}
}

func rootShardTemplate() *pmdeployerv1alpha1.RootShardTemplate {
	return &pmdeployerv1alpha1.RootShardTemplate{
		ObjectMeta: metav1.ObjectMeta{Name: "root", Namespace: "pm"},
		Spec: operatorv1alpha1.RootShardTemplateSpec{
			CommonShardSpecTemplate: operatorv1alpha1.CommonShardSpecTemplate{
				Etcd: &operatorv1alpha1.EtcdConfig{
					Endpoints: []string{`"https://etcd-" + platformMesh + ".pm:2379"`},
					Prefix:    `"/" + platformMesh + "/" + cluster`,
				},
			},
		},
	}
}

func shardTemplate() *pmdeployerv1alpha1.ShardTemplate {
	return &pmdeployerv1alpha1.ShardTemplate{
		ObjectMeta: metav1.ObjectMeta{Name: "eu", Namespace: "pm"},
		Spec: operatorv1alpha1.ShardTemplateSpec{
			CommonShardSpecTemplate: operatorv1alpha1.CommonShardSpecTemplate{
				Etcd: &operatorv1alpha1.EtcdConfig{
					Endpoints: []string{`"https://etcd-" + platformMesh + ".pm:2379"`},
					Prefix:    `"/" + platformMesh + "/" + shardGroup + "/" + cluster`,
				},
			},
		},
	}
}

func engage(t *testing.T, r *clusters.Registry, name string) {
	t.Helper()
	require.NoError(t, r.Engage(context.Background(), multicluster.ClusterName(name), nil))
}

func TestReconcileRootShard(t *testing.T) {
	pm := platformMesh()
	cl := fake.NewClientBuilder().WithScheme(scheme(t)).WithObjects(pm, rootShardTemplate(), shardTemplate()).Build()
	reg := clusters.NewRegistry()
	engage(t, reg, "rootshard#customer-a--east")
	engage(t, reg, "frontproxy#customer-a--fp")

	sub := topology.New(cl, reg)
	_, err := sub.Process(t.Context(), pm)
	require.NoError(t, err)

	rs := &operatorv1alpha1.RootShard{}
	require.NoError(t, cl.Get(t.Context(), ctrlruntimeclient.ObjectKey{Namespace: "pm", Name: names.RootShard("customer-a", "root", "east")}, rs))
	require.Len(t, rs.Spec.Etcd.Endpoints, 1)
	assert.Equal(t, "https://etcd-customer-a.pm:2379", rs.Spec.Etcd.Endpoints[0])
	assert.Equal(t, "/customer-a/east", rs.Spec.Etcd.Prefix)
	assert.Equal(t, "fp.customer-a.example.com", rs.Spec.External.Hostname)
	assert.Equal(t, uint32(6443), rs.Spec.External.Port)
	assert.Equal(t, "https://kcp.customer-a.example.com:6443", rs.Spec.ShardBaseURL)
	assert.Equal(t, "customer-a", rs.Labels[topology.LabelPlatformMesh])
	assert.Equal(t, components.RootShard, rs.Labels[topology.LabelComponent])
	assert.Equal(t, "east", rs.Labels[topology.LabelCluster])
	require.Len(t, rs.OwnerReferences, 1)
	assert.Equal(t, "customer-a", rs.OwnerReferences[0].Name)
}

func TestReconcileRootShardRejectsSecondCluster(t *testing.T) {
	pm := platformMesh()
	cl := fake.NewClientBuilder().WithScheme(scheme(t)).WithObjects(pm, rootShardTemplate(), shardTemplate()).Build()
	reg := clusters.NewRegistry()
	engage(t, reg, "rootshard#customer-a--east")
	engage(t, reg, "rootshard#customer-a--west")

	sub := topology.New(cl, reg)
	_, err := sub.Process(t.Context(), pm)
	require.Error(t, err)
}

func TestReconcileRootShardTeardownStale(t *testing.T) {
	pm := platformMesh()
	stale := &operatorv1alpha1.RootShard{
		ObjectMeta: metav1.ObjectMeta{
			Name:      names.RootShard("customer-a", "root", "west"),
			Namespace: "pm",
			Labels: map[string]string{
				topology.LabelPlatformMesh: "customer-a",
				topology.LabelComponent:    components.RootShard,
				topology.LabelCluster:      "west",
			},
		},
	}
	cl := fake.NewClientBuilder().WithScheme(scheme(t)).WithObjects(pm, stale, rootShardTemplate(), shardTemplate()).Build()
	reg := clusters.NewRegistry()
	engage(t, reg, "rootshard#customer-a--east")
	engage(t, reg, "frontproxy#customer-a--fp")

	sub := topology.New(cl, reg)
	_, err := sub.Process(t.Context(), pm)
	require.NoError(t, err)

	require.NoError(t, cl.Get(t.Context(), ctrlruntimeclient.ObjectKey{Namespace: "pm", Name: names.RootShard("customer-a", "root", "east")}, &operatorv1alpha1.RootShard{}))
	err = cl.Get(t.Context(), ctrlruntimeclient.ObjectKey{Namespace: "pm", Name: names.RootShard("customer-a", "root", "west")}, &operatorv1alpha1.RootShard{})
	assert.True(t, apierrors.IsNotFound(err), "expected stale RootShard torn down, got %v", err)
}

func TestReconcileShard(t *testing.T) {
	pm := platformMesh()
	pm.Spec.Topology.ShardGroups = []pmdeployerv1alpha1.ShardGroup{{
		Name:           "eu",
		TemplateRef:    &pmdeployerv1alpha1.TemplateReference{Name: "eu"},
		CacheServerRef: "cache",
		Exposure: &pmdeployerv1alpha1.Exposure{
			HostnameTemplate: `component + "." + cluster + ".sslip.io"`,
			Port:             31443,
		},
	}}
	pm.Spec.Topology.CacheServer = &pmdeployerv1alpha1.CacheServer{Name: "cache"}
	cl := fake.NewClientBuilder().WithScheme(scheme(t)).WithObjects(pm, rootShardTemplate(), shardTemplate()).Build()
	reg := clusters.NewRegistry()
	engage(t, reg, "rootshard#customer-a--east")
	engage(t, reg, "frontproxy#customer-a--fp")
	engage(t, reg, "shards-eu#customer-a--west")
	engage(t, reg, "cacheserver#customer-a--cache1")

	sub := topology.New(cl, reg)
	_, err := sub.Process(t.Context(), pm)
	require.NoError(t, err)

	sh := &operatorv1alpha1.Shard{}
	require.NoError(t, cl.Get(t.Context(), ctrlruntimeclient.ObjectKey{Namespace: "pm", Name: names.Shard("customer-a", "eu", "west")}, sh))
	assert.Equal(t, []string{"https://etcd-customer-a.pm:2379"}, sh.Spec.Etcd.Endpoints)
	assert.Equal(t, "/customer-a/eu/west", sh.Spec.Etcd.Prefix)
	require.NotNil(t, sh.Spec.RootShard.Reference)
	assert.Equal(t, names.RootShard("customer-a", "root", "east"), sh.Spec.RootShard.Reference.Name)
	assert.Equal(t, "https://shards-eu.west.sslip.io:31443", sh.Spec.ShardBaseURL)
	require.NotNil(t, sh.Spec.Cache)
	require.NotNil(t, sh.Spec.Cache.Reference)
	assert.Equal(t, names.CacheServer("customer-a", "cache", "cache1"), sh.Spec.Cache.Reference.Name)
	assert.Equal(t, components.Shard("eu"), sh.Labels[topology.LabelComponent])
	assert.Equal(t, "west", sh.Labels[topology.LabelCluster])
}

func TestReconcileFrontProxy(t *testing.T) {
	pm := platformMesh()
	pm.Spec.Topology.FrontProxy = pmdeployerv1alpha1.FrontProxy{
		Name: "fp",
		Exposure: pmdeployerv1alpha1.Exposure{
			HostnameTemplate: `"api." + platformMesh + ".example.com"`,
			Port:             443,
		},
	}
	cl := fake.NewClientBuilder().WithScheme(scheme(t)).WithObjects(pm, rootShardTemplate(), shardTemplate()).Build()
	reg := clusters.NewRegistry()
	engage(t, reg, "rootshard#customer-a--east")
	engage(t, reg, "frontproxy#customer-a--west")

	sub := topology.New(cl, reg)
	_, err := sub.Process(t.Context(), pm)
	require.NoError(t, err)

	fp := &operatorv1alpha1.FrontProxy{}
	require.NoError(t, cl.Get(t.Context(), ctrlruntimeclient.ObjectKey{Namespace: "pm", Name: names.FrontProxy("customer-a", "fp", "west")}, fp))
	require.NotNil(t, fp.Spec.RootShard.Reference)
	assert.Equal(t, names.RootShard("customer-a", "root", "east"), fp.Spec.RootShard.Reference.Name)
	assert.Equal(t, "api.customer-a.example.com", fp.Spec.External.Hostname)
	assert.Equal(t, uint32(443), fp.Spec.External.Port)
	assert.Equal(t, components.FrontProxy, fp.Labels[topology.LabelComponent])
	assert.Equal(t, "west", fp.Labels[topology.LabelCluster])
}

func TestReconcileCacheServer(t *testing.T) {
	pm := platformMesh()
	pm.Spec.Topology.CacheServer = &pmdeployerv1alpha1.CacheServer{
		Name:        "global",
		TemplateRef: &pmdeployerv1alpha1.TemplateReference{Name: "global"},
	}
	cacheTemplate := &pmdeployerv1alpha1.CacheServerTemplate{
		ObjectMeta: metav1.ObjectMeta{Name: "global", Namespace: "pm"},
		Spec: operatorv1alpha1.CacheServerTemplateSpec{
			Etcd: &operatorv1alpha1.EtcdConfig{
				Endpoints: []string{`"https://cache-etcd-" + platformMesh + ".pm:2379"`},
				Prefix:    `"/" + platformMesh + "/cache"`,
			},
		},
	}
	cl := fake.NewClientBuilder().WithScheme(scheme(t)).WithObjects(pm, rootShardTemplate(), shardTemplate(), cacheTemplate).Build()
	reg := clusters.NewRegistry()
	engage(t, reg, "rootshard#customer-a--east")
	engage(t, reg, "frontproxy#customer-a--fp")
	engage(t, reg, "cacheserver#customer-a--west")

	sub := topology.New(cl, reg)
	_, err := sub.Process(t.Context(), pm)
	require.NoError(t, err)

	cs := &operatorv1alpha1.CacheServer{}
	require.NoError(t, cl.Get(t.Context(), ctrlruntimeclient.ObjectKey{Namespace: "pm", Name: names.CacheServer("customer-a", "global", "west")}, cs))
	require.NotNil(t, cs.Spec.Etcd)
	assert.Equal(t, []string{"https://cache-etcd-customer-a.pm:2379"}, cs.Spec.Etcd.Endpoints)
	assert.Equal(t, "/customer-a/cache", cs.Spec.Etcd.Prefix)
	assert.Equal(t, components.CacheServer, cs.Labels[topology.LabelComponent])
	assert.Equal(t, "west", cs.Labels[topology.LabelCluster])
}

func TestReconcileVirtualWorkspace(t *testing.T) {
	pm := platformMesh()
	pm.Spec.Topology.RootShard.VirtualWorkspaces = pmdeployerv1alpha1.VirtualWorkspaceSpec{
		Mode: pmdeployerv1alpha1.VirtualWorkspaceModeStandalone,
		Exposure: pmdeployerv1alpha1.Exposure{
			HostnameTemplate: `"vw." + platformMesh + ".example.com"`,
			Port:             443,
		},
	}
	cl := fake.NewClientBuilder().WithScheme(scheme(t)).WithObjects(pm, rootShardTemplate(), shardTemplate()).Build()
	reg := clusters.NewRegistry()
	engage(t, reg, "rootshard#customer-a--east")
	engage(t, reg, "frontproxy#customer-a--fp")

	sub := topology.New(cl, reg)
	_, err := sub.Process(t.Context(), pm)
	require.NoError(t, err)

	vw := &operatorv1alpha1.VirtualWorkspace{}
	require.NoError(t, cl.Get(t.Context(), ctrlruntimeclient.ObjectKey{Namespace: "pm", Name: names.VirtualWorkspace("customer-a", "root", "east")}, vw))
	require.NotNil(t, vw.Spec.Target.RootShardRef)
	assert.Equal(t, names.RootShard("customer-a", "root", "east"), vw.Spec.Target.RootShardRef.Name)
	assert.Equal(t, "vw.customer-a.example.com", vw.Spec.External.Hostname)
	assert.Equal(t, uint32(443), vw.Spec.External.Port)
	assert.Equal(t, components.VirtualWorkspace, vw.Labels[topology.LabelComponent])
	assert.Equal(t, "east", vw.Labels[topology.LabelCluster])
}

func TestReconcileVirtualWorkspaceEmbeddedSkipped(t *testing.T) {
	pm := platformMesh() // root shard VirtualWorkspaces defaults to embedded (mode unset)
	cl := fake.NewClientBuilder().WithScheme(scheme(t)).WithObjects(pm, rootShardTemplate(), shardTemplate()).Build()
	reg := clusters.NewRegistry()
	engage(t, reg, "rootshard#customer-a--east")
	engage(t, reg, "frontproxy#customer-a--fp")

	sub := topology.New(cl, reg)
	_, err := sub.Process(t.Context(), pm)
	require.NoError(t, err)

	list := &operatorv1alpha1.VirtualWorkspaceList{}
	require.NoError(t, cl.List(t.Context(), list))
	assert.Empty(t, list.Items)
}

func TestReconcileNamesAreUniquePerPlatformMesh(t *testing.T) {
	a := platformMesh()
	b := platformMesh()
	b.Name = "customer-b"

	cl := fake.NewClientBuilder().WithScheme(scheme(t)).WithObjects(a, b, rootShardTemplate(), shardTemplate()).Build()
	reg := clusters.NewRegistry()
	for _, pm := range []string{"customer-a", "customer-b"} {
		engage(t, reg, multiclusterName("rootshard", pm, "east"))
		engage(t, reg, multiclusterName("frontproxy", pm, "east"))
	}

	sub := topology.New(cl, reg)
	for _, pm := range []*pmdeployerv1alpha1.PlatformMesh{a, b} {
		_, err := sub.Process(t.Context(), pm)
		require.NoError(t, err)
	}

	list := &operatorv1alpha1.RootShardList{}
	require.NoError(t, cl.List(t.Context(), list, ctrlruntimeclient.InNamespace("pm")))
	require.Len(t, list.Items, 2, "both installations must keep their own root shard")

	// Each root shard belongs to exactly one PlatformMesh and points at its own front proxy.
	byName := map[string]string{}
	for _, rs := range list.Items {
		byName[rs.Name] = rs.Labels[topology.LabelPlatformMesh]
	}
	assert.Equal(t, map[string]string{
		names.RootShard("customer-a", "root", "east"): "customer-a",
		names.RootShard("customer-b", "root", "east"): "customer-b",
	}, byName)

	fps := &operatorv1alpha1.FrontProxyList{}
	require.NoError(t, cl.List(t.Context(), fps, ctrlruntimeclient.InNamespace("pm")))
	require.Len(t, fps.Items, 2)
	for _, fp := range fps.Items {
		pm := fp.Labels[topology.LabelPlatformMesh]
		assert.Equal(t, names.RootShard(pm, "root", "east"), fp.Spec.RootShard.Reference.Name)
	}
}

func multiclusterName(component, platformMesh, clusterID string) string {
	return component + "#" + platformMesh + "--" + clusterID
}

func TestReconcileCacheServerRef(t *testing.T) {
	shardGroup := func(ref string) []pmdeployerv1alpha1.ShardGroup {
		return []pmdeployerv1alpha1.ShardGroup{{
			Name:           "eu",
			CacheServerRef: ref,
		}}
	}

	for name, tc := range map[string]struct {
		cacheServer *pmdeployerv1alpha1.CacheServer
		engaged     bool
		ref         string
		wantErr     string
	}{
		"undefined cache server": {
			ref:     "cache",
			wantErr: `cacheServerRef "cache" set but no cache server defined`,
		},
		"ref does not match": {
			cacheServer: &pmdeployerv1alpha1.CacheServer{Name: "global"},
			engaged:     true,
			ref:         "cache",
			wantErr:     `cacheServerRef "cache" does not match cache server "global"`,
		},
		"not engaged yet": {
			cacheServer: &pmdeployerv1alpha1.CacheServer{Name: "cache"},
			ref:         "cache",
			wantErr:     `cache server "cache" not ready`,
		},
	} {
		t.Run(name, func(t *testing.T) {
			pm := platformMesh()
			pm.Spec.Topology.ShardGroups = shardGroup(tc.ref)
			pm.Spec.Topology.CacheServer = tc.cacheServer

			cl := fake.NewClientBuilder().WithScheme(scheme(t)).WithObjects(pm, rootShardTemplate(), shardTemplate()).Build()
			reg := clusters.NewRegistry()
			engage(t, reg, "rootshard#customer-a--east")
			engage(t, reg, "frontproxy#customer-a--fp")
			engage(t, reg, "shards-eu#customer-a--west")
			if tc.engaged {
				engage(t, reg, "cacheserver#customer-a--cache1")
			}

			_, err := topology.New(cl, reg).Process(t.Context(), pm)
			require.ErrorContains(t, err, tc.wantErr)
		})
	}
}

func TestReconcileTemplateRef(t *testing.T) {
	t.Run("nil ref renders a zero spec", func(t *testing.T) {
		pm := platformMesh()
		pm.Spec.Topology.RootShard.TemplateRef = nil

		cl := fake.NewClientBuilder().WithScheme(scheme(t)).WithObjects(pm).Build()
		reg := clusters.NewRegistry()
		engage(t, reg, "rootshard#customer-a--east")
		engage(t, reg, "frontproxy#customer-a--fp")

		_, err := topology.New(cl, reg).Process(t.Context(), pm)
		require.NoError(t, err)

		rs := &operatorv1alpha1.RootShard{}
		require.NoError(t, cl.Get(t.Context(),
			ctrlruntimeclient.ObjectKey{Namespace: "pm", Name: names.RootShard("customer-a", "root", "east")}, rs))
		assert.Nil(t, rs.Spec.Etcd.Endpoints)
	})

	t.Run("dangling ref errors", func(t *testing.T) {
		pm := platformMesh()
		pm.Spec.Topology.RootShard.TemplateRef = &pmdeployerv1alpha1.TemplateReference{Name: "gone"}

		cl := fake.NewClientBuilder().WithScheme(scheme(t)).WithObjects(pm).Build()
		reg := clusters.NewRegistry()
		engage(t, reg, "rootshard#customer-a--east")
		engage(t, reg, "frontproxy#customer-a--fp")

		_, err := topology.New(cl, reg).Process(t.Context(), pm)
		require.ErrorContains(t, err, "template pm/gone")
	})

	t.Run("shared across namespaces", func(t *testing.T) {
		shared := rootShardTemplate()
		shared.Namespace = "shared"

		a := platformMesh()
		a.Spec.Topology.RootShard.TemplateRef = &pmdeployerv1alpha1.TemplateReference{Name: "root", Namespace: "shared"}
		b := platformMesh()
		b.Name, b.Namespace = "customer-b", "pm-b"
		b.Spec.Topology.RootShard.TemplateRef = &pmdeployerv1alpha1.TemplateReference{Name: "root", Namespace: "shared"}

		cl := fake.NewClientBuilder().WithScheme(scheme(t)).WithObjects(a, b, shared).Build()
		reg := clusters.NewRegistry()
		for _, pm := range []string{"customer-a", "customer-b"} {
			engage(t, reg, multiclusterName("rootshard", pm, "east"))
			engage(t, reg, multiclusterName("frontproxy", pm, "east"))
		}

		sub := topology.New(cl, reg)
		for _, pm := range []*pmdeployerv1alpha1.PlatformMesh{a, b} {
			_, err := sub.Process(t.Context(), pm)
			require.NoError(t, err)
		}

		// Both installations picked up the shared template, each with its own
		// CEL context.
		for _, tc := range []struct{ pm, namespace, prefix string }{
			{"customer-a", "pm", "/customer-a/east"},
			{"customer-b", "pm-b", "/customer-b/east"},
		} {
			rs := &operatorv1alpha1.RootShard{}
			require.NoError(t, cl.Get(t.Context(), ctrlruntimeclient.ObjectKey{
				Namespace: tc.namespace, Name: names.RootShard(tc.pm, "root", "east"),
			}, rs))
			assert.Equal(t, tc.prefix, rs.Spec.Etcd.Prefix)
		}
	})
}
