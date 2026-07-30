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

package rootstructure_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	pmdeployerv1alpha1 "go.platform-mesh.io/apis/deployer/v1alpha1"
	"go.platform-mesh.io/platform-mesh-deployer/pkg/clusters"
	"go.platform-mesh.io/platform-mesh-deployer/pkg/kcp"
	"go.platform-mesh.io/platform-mesh-deployer/pkg/subroutines/rootstructure"

	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/multicluster-runtime/pkg/multicluster"

	operatorv1alpha1 "github.com/kcp-dev/kcp-operator/sdk/apis/operator/v1alpha1"
	kcpcorev1alpha1 "github.com/kcp-dev/kcp/sdk/apis/core/v1alpha1"
	kcptenancyv1alpha1 "github.com/kcp-dev/kcp/sdk/apis/tenancy/v1alpha1"
)

func scheme(t *testing.T) *runtime.Scheme {
	t.Helper()
	s := runtime.NewScheme()
	require.NoError(t, clientgoscheme.AddToScheme(s))
	require.NoError(t, pmdeployerv1alpha1.AddToScheme(s))
	require.NoError(t, operatorv1alpha1.AddToScheme(s))
	require.NoError(t, kcptenancyv1alpha1.AddToScheme(s))
	require.NoError(t, kcpcorev1alpha1.AddToScheme(s))
	return s
}

// platformMesh returns a PlatformMesh whose topology subroutine has or has not
// finished, which is what gates the kcp side.
func platformMesh(topologyDone bool) *pmdeployerv1alpha1.PlatformMesh {
	pm := &pmdeployerv1alpha1.PlatformMesh{
		ObjectMeta: metav1.ObjectMeta{Name: "customer-a", Namespace: "pm"},
		Spec: pmdeployerv1alpha1.PlatformMeshSpec{
			Topology: pmdeployerv1alpha1.Topology{
				FrontProxy: pmdeployerv1alpha1.FrontProxy{Name: "fp"},
			},
		},
	}
	status := metav1.ConditionFalse
	if topologyDone {
		status = metav1.ConditionTrue
	}
	meta.SetStatusCondition(&pm.Status.Conditions, metav1.Condition{
		Type: "TopologySubroutine", Status: status, Reason: "T", Message: "topology",
	})
	return pm
}

func registry(t *testing.T) *clusters.Registry {
	t.Helper()
	r := clusters.NewRegistry()
	require.NoError(t, r.Engage(context.Background(),
		multicluster.ClusterName("frontproxy#customer-a--fp1"), nil))
	return r
}

// kcp only exists once the topology is up, so nothing is attempted before then.
func TestProcessWaitsForTopology(t *testing.T) {
	pm := platformMesh(false)
	s := scheme(t)
	cl := fake.NewClientBuilder().WithScheme(s).WithObjects(pm).Build()

	sub := rootstructure.New(kcp.New(cl, registry(t), s, nil))
	assert.Equal(t, rootstructure.Name, sub.GetName())

	res, err := sub.Process(t.Context(), pm)
	require.NoError(t, err)
	assert.False(t, res.IsContinue())

	cond := meta.FindStatusCondition(pm.Status.Conditions, rootstructure.ConditionProvisioned)
	require.NotNil(t, cond)
	assert.Equal(t, "WaitingForTopology", cond.Reason)
}

// With the topology up the admin kubeconfig is minted, and provisioning waits
// for kcp-operator to write the secret.
func TestProcessWaitsForKubeconfig(t *testing.T) {
	pm := platformMesh(true)
	s := scheme(t)
	cl := fake.NewClientBuilder().WithScheme(s).WithObjects(pm).Build()

	res, err := rootstructure.New(kcp.New(cl, registry(t), s, nil)).Process(t.Context(), pm)
	require.NoError(t, err)
	assert.False(t, res.IsContinue())

	cond := meta.FindStatusCondition(pm.Status.Conditions, rootstructure.ConditionProvisioned)
	require.NotNil(t, cond)
	assert.Equal(t, "WaitingForKubeconfig", cond.Reason)
}
