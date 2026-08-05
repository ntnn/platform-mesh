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

package provisioner

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	pmdeployv1alpha1 "go.platform-mesh.io/apis/deploy/v1alpha1"
	"go.platform-mesh.io/platform-mesh-deployer/pkg/clusters"
	"go.platform-mesh.io/platform-mesh-deployer/pkg/kcp"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	ctrlruntimeclient "sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/multicluster-runtime/pkg/multicluster"

	operatorv1alpha1 "github.com/kcp-dev/kcp-operator/sdk/apis/operator/v1alpha1"
	kcpcorev1alpha1 "github.com/kcp-dev/kcp/sdk/apis/core/v1alpha1"
	kcptenancyv1alpha1 "github.com/kcp-dev/kcp/sdk/apis/tenancy/v1alpha1"
)

func testScheme(t *testing.T) *runtime.Scheme {
	t.Helper()
	s := runtime.NewScheme()
	require.NoError(t, clientgoscheme.AddToScheme(s))
	require.NoError(t, pmdeployv1alpha1.AddToScheme(s))
	require.NoError(t, operatorv1alpha1.AddToScheme(s))
	require.NoError(t, kcptenancyv1alpha1.AddToScheme(s))
	require.NoError(t, kcpcorev1alpha1.AddToScheme(s))
	return s
}

func platformMesh(rootStructureDone bool) *pmdeployv1alpha1.PlatformMesh {
	pm := &pmdeployv1alpha1.PlatformMesh{
		ObjectMeta: metav1.ObjectMeta{Name: "customer-a", Namespace: "pm"},
		Spec: pmdeployv1alpha1.PlatformMeshSpec{
			Topology: pmdeployv1alpha1.Topology{
				FrontProxy: pmdeployv1alpha1.FrontProxy{Name: "fp"},
			},
		},
	}
	status := metav1.ConditionFalse
	if rootStructureDone {
		status = metav1.ConditionTrue
	}
	meta.SetStatusCondition(&pm.Status.Conditions, metav1.Condition{
		Type: "RootStructureProvisioned", Status: status, Reason: "R", Message: "root",
	})
	return pm
}

func moduleSetup() *pmdeployv1alpha1.ModuleSetup {
	return &pmdeployv1alpha1.ModuleSetup{
		ObjectMeta: metav1.ObjectMeta{Name: "acme", Namespace: "pm"},
		Spec: pmdeployv1alpha1.ModuleSetupSpec{
			PlatformMeshRef: corev1.LocalObjectReference{Name: "customer-a"},
			ModuleRef:       corev1.LocalObjectReference{Name: "acme"},
			Workspaces: []pmdeployv1alpha1.ModuleSetupWorkspace{
				{Path: "root:modules:acme"},
			},
		},
	}
}

func access(t *testing.T, cl ctrlruntimeclient.Client, s *runtime.Scheme) *kcp.Access {
	t.Helper()
	reg := clusters.NewRegistry()
	require.NoError(t, reg.Engage(context.Background(),
		multicluster.ClusterName("frontproxy#customer-a--fp1"), nil))
	return kcp.New(cl, reg, s, nil)
}

// The module's workspaces live below the root structure, so nothing is created
// before that exists.
func TestProcessWaitsForRootStructure(t *testing.T) {
	setup := moduleSetup()
	s := testScheme(t)
	cl := fake.NewClientBuilder().WithScheme(s).WithObjects(platformMesh(false), setup).Build()

	sub := New(cl, access(t, cl, s), nil)
	assert.Equal(t, Name, sub.GetName())

	res, err := sub.Process(t.Context(), setup)
	require.NoError(t, err)
	assert.False(t, res.IsContinue())

	cond := meta.FindStatusCondition(setup.Status.Conditions, ConditionReady)
	require.NotNil(t, cond)
	assert.Equal(t, "WaitingForRootStructure", cond.Reason)
}

func TestProcessWaitsForKubeconfig(t *testing.T) {
	setup := moduleSetup()
	s := testScheme(t)
	cl := fake.NewClientBuilder().WithScheme(s).WithObjects(platformMesh(true), setup).Build()

	res, err := New(cl, access(t, cl, s), nil).Process(t.Context(), setup)
	require.NoError(t, err)
	assert.False(t, res.IsContinue())

	cond := meta.FindStatusCondition(setup.Status.Conditions, ConditionReady)
	require.NotNil(t, cond)
	assert.Equal(t, "WaitingForKubeconfig", cond.Reason)
}

func TestProcessRequiresPlatformMesh(t *testing.T) {
	setup := moduleSetup()
	s := testScheme(t)
	cl := fake.NewClientBuilder().WithScheme(s).WithObjects(setup).Build()

	_, err := New(cl, access(t, cl, s), nil).Process(t.Context(), setup)
	require.ErrorContains(t, err, "getting PlatformMesh")
}

func TestFinalizers(t *testing.T) {
	s := testScheme(t)
	cl := fake.NewClientBuilder().WithScheme(s).Build()
	assert.Equal(t, []string{Finalizer}, New(cl, access(t, cl, s), nil).Finalizers(moduleSetup()))
}

// A deleted PlatformMesh takes its kcp with it, so there is nothing to clean up.
func TestFinalizeWithoutPlatformMesh(t *testing.T) {
	setup := moduleSetup()
	s := testScheme(t)
	cl := fake.NewClientBuilder().WithScheme(s).WithObjects(setup).Build()

	res, err := New(cl, access(t, cl, s), nil).Finalize(t.Context(), setup)
	require.NoError(t, err)
	assert.True(t, res.IsContinue())
}

// Without a reachable kcp the workspaces cannot be removed, and they are gone
// with kcp anyway.
func TestFinalizeWithoutKcp(t *testing.T) {
	setup := moduleSetup()
	s := testScheme(t)
	cl := fake.NewClientBuilder().WithScheme(s).WithObjects(platformMesh(true), setup).Build()

	res, err := New(cl, access(t, cl, s), nil).Finalize(t.Context(), setup)
	require.NoError(t, err)
	assert.True(t, res.IsContinue())
}

func TestDecode(t *testing.T) {
	objs, err := decode([]byte("apiVersion: v1\nkind: ConfigMap\nmetadata:\n  name: a\n---\n---\napiVersion: v1\nkind: Secret\nmetadata:\n  name: b\n"))
	require.NoError(t, err)
	require.Len(t, objs, 2, "empty documents are skipped")
	assert.Equal(t, "ConfigMap", objs[0].GetKind())
	assert.Equal(t, "Secret", objs[1].GetKind())
}

func TestDecodeInvalid(t *testing.T) {
	_, err := decode([]byte("\tnot: [valid"))
	require.Error(t, err)
}
