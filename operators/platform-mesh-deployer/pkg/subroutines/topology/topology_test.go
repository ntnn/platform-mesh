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
					Name: "root",
					Template: &operatorv1alpha1.RootShardTemplateSpec{
						CommonShardSpecTemplate: operatorv1alpha1.CommonShardSpecTemplate{
							Etcd: &operatorv1alpha1.EtcdConfig{
								Endpoints: []string{`"https://etcd-" + platformMesh + ".pm:2379"`},
								Prefix:    `"/" + platformMesh + "/" + cluster`,
							},
						},
					},
					Exposure: pmdeployerv1alpha1.Exposure{
						HostnameTemplate: `"kcp." + platformMesh + ".example.com"`,
						Port:             6443,
					},
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
	cl := fake.NewClientBuilder().WithScheme(scheme(t)).WithObjects(pm).Build()
	reg := clusters.NewRegistry()
	engage(t, reg, "rootshard#customer-a--east")

	sub := topology.New(cl, reg)
	_, err := sub.Process(t.Context(), pm)
	require.NoError(t, err)

	rs := &operatorv1alpha1.RootShard{}
	require.NoError(t, cl.Get(t.Context(), ctrlruntimeclient.ObjectKey{Namespace: "pm", Name: "root-east"}, rs))
	require.Len(t, rs.Spec.Etcd.Endpoints, 1)
	assert.Equal(t, "https://etcd-customer-a.pm:2379", rs.Spec.Etcd.Endpoints[0])
	assert.Equal(t, "/customer-a/east", rs.Spec.Etcd.Prefix)
	assert.Equal(t, "kcp.customer-a.example.com", rs.Spec.External.Hostname)
	assert.Equal(t, uint32(6443), rs.Spec.External.Port)
	assert.Equal(t, "customer-a", rs.Labels[topology.LabelPlatformMesh])
	assert.Equal(t, components.RootShard, rs.Labels[topology.LabelComponent])
	assert.Equal(t, "east", rs.Labels[topology.LabelCluster])
	require.Len(t, rs.OwnerReferences, 1)
	assert.Equal(t, "customer-a", rs.OwnerReferences[0].Name)
}

func TestReconcileRootShardRejectsSecondCluster(t *testing.T) {
	pm := platformMesh()
	cl := fake.NewClientBuilder().WithScheme(scheme(t)).WithObjects(pm).Build()
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
			Name:      "root-west",
			Namespace: "pm",
			Labels: map[string]string{
				topology.LabelPlatformMesh: "customer-a",
				topology.LabelComponent:    components.RootShard,
				topology.LabelCluster:      "west",
			},
		},
	}
	cl := fake.NewClientBuilder().WithScheme(scheme(t)).WithObjects(pm, stale).Build()
	reg := clusters.NewRegistry()
	engage(t, reg, "rootshard#customer-a--east")

	sub := topology.New(cl, reg)
	_, err := sub.Process(t.Context(), pm)
	require.NoError(t, err)

	require.NoError(t, cl.Get(t.Context(), ctrlruntimeclient.ObjectKey{Namespace: "pm", Name: "root-east"}, &operatorv1alpha1.RootShard{}))
	err = cl.Get(t.Context(), ctrlruntimeclient.ObjectKey{Namespace: "pm", Name: "root-west"}, &operatorv1alpha1.RootShard{})
	assert.True(t, apierrors.IsNotFound(err), "expected stale RootShard torn down, got %v", err)
}
