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

package modules_test

import (
	"testing"

	"github.com/stretchr/testify/require"

	pmdeployv1alpha1 "go.platform-mesh.io/apis/deploy/v1alpha1"
	"go.platform-mesh.io/platform-mesh-deployer/pkg/clusters"

	corev1 "k8s.io/api/core/v1"
	ctrlruntimeclient "sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

// These are cross-field rules the CRD cannot express, so they are rejected
// before anything is created rather than failing halfway through a deploy.
func TestProcessRejectsUnsatisfiableReferences(t *testing.T) {
	tests := []struct {
		name    string
		mutate  func(*pmdeployv1alpha1.Module)
		wantErr string
	}{
		{
			name: "kubeconfig references an undeclared workspace",
			mutate: func(m *pmdeployv1alpha1.Module) {
				m.Spec.Workspaces = []pmdeployv1alpha1.ModuleWorkspace{{Name: ""}}
				m.Spec.Kubeconfigs = []pmdeployv1alpha1.ModuleKubeconfig{{
					Name: "kcp", Target: pmdeployv1alpha1.KubeconfigTargetFrontProxy, Workspace: "nope",
				}}
			},
			wantErr: "which the module does not declare",
		},
		{
			name: "component references an undeclared kubeconfig",
			mutate: func(m *pmdeployv1alpha1.Module) {
				m.Spec.Components[0].Kubeconfigs = []string{"missing"}
			},
			wantErr: "references kubeconfig",
		},
		{
			name: "duplicate dependency",
			mutate: func(m *pmdeployv1alpha1.Module) {
				m.Spec.DependsOn = []corev1.LocalObjectReference{{Name: "base"}, {Name: "base"}}
			},
			wantErr: "more than once",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mod := testModule()
			tt.mutate(mod)

			sub := newSubroutine(t, []ctrlruntimeclient.Object{platformMesh(true), mod}, clusters.NewRegistry())
			_, err := sub.Process(t.Context(), mod)
			require.ErrorContains(t, err, tt.wantErr)
		})
	}
}

// A kubeconfig scoped to a declared workspace is accepted.
func TestProcessAcceptsDeclaredReferences(t *testing.T) {
	mod := testModule()
	mod.Spec.Workspaces = []pmdeployv1alpha1.ModuleWorkspace{{Name: ""}, {Name: "validation"}}
	mod.Spec.Kubeconfigs = []pmdeployv1alpha1.ModuleKubeconfig{
		{Name: "kcp", Target: pmdeployv1alpha1.KubeconfigTargetFrontProxy},
		{Name: "val", Target: pmdeployv1alpha1.KubeconfigTargetFrontProxy, Workspace: "validation"},
	}
	mod.Spec.Components[0].Kubeconfigs = []string{"kcp", "val"}

	sub := newSubroutine(t, []ctrlruntimeclient.Object{platformMesh(true), mod}, clusters.NewRegistry())
	_, err := sub.Process(t.Context(), mod)
	require.NoError(t, err)
}

// A pre-topology module has to deploy before kcp exists, since the topology
// waits for it; only post-topology modules wait for the topology.
func TestProcessPreTopologyDoesNotWaitForTopology(t *testing.T) {
	mod := testModule()
	mod.Spec.Stage = pmdeployv1alpha1.StagePreTopology

	workload := fake.NewClientBuilder().WithScheme(scheme(t)).Build()
	reg := clusters.NewRegistry()
	engage(t, reg, "shards-default#customer-a--s1", workload)

	// platformMesh(false) has Ready=False, i.e. no topology yet.
	sub := newSubroutine(t, []ctrlruntimeclient.Object{platformMesh(false), mod}, reg)
	res, err := sub.Process(t.Context(), mod)
	require.NoError(t, err)
	require.True(t, res.IsContinue(), "a pre-topology module must not wait for kcp")

	require.NoError(t, workload.Get(t.Context(),
		ctrlruntimeclient.ObjectKey{Namespace: "acme-system", Name: "acme-agent"}, &corev1.Service{}))
}
