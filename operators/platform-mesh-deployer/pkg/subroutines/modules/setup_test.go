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

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	pmdeployerv1alpha1 "go.platform-mesh.io/apis/deployer/v1alpha1"
	"go.platform-mesh.io/platform-mesh-deployer/pkg/clusters"

	ctrlruntimeclient "sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

// A module declaring workspaces must get a ModuleSetup that keeps each
// workspace's content with that workspace: flattening it would apply a child's
// manifests into the parent.
func TestProcessWritesModuleSetupPerWorkspace(t *testing.T) {
	mod := testModule()
	mod.Spec.Workspaces = []pmdeployerv1alpha1.ModuleWorkspace{
		{Name: "", Content: []pmdeployerv1alpha1.ResourceRef{{Name: "apiexports"}}},
		{Name: "validation", Content: []pmdeployerv1alpha1.ResourceRef{{Name: "validation-schemas"}}},
	}

	workload := fake.NewClientBuilder().WithScheme(scheme(t)).Build()
	reg := clusters.NewRegistry()
	engage(t, reg, "shards-default#customer-a--s1", workload)

	local := fake.NewClientBuilder().WithScheme(scheme(t)).
		WithObjects(platformMesh(true), mod).Build()
	sub := newSubroutineWithClient(t, local, reg)

	res, err := sub.Process(t.Context(), mod)
	require.NoError(t, err)
	assert.False(t, res.IsContinue(), "deploy waits until the setup is ready")

	setup := &pmdeployerv1alpha1.ModuleSetup{}
	require.NoError(t, local.Get(t.Context(),
		ctrlruntimeclient.ObjectKey{Namespace: "pm", Name: "acme"}, setup))

	require.Len(t, setup.Spec.Workspaces, 2)
	byPath := map[string][]string{}
	for _, ws := range setup.Spec.Workspaces {
		for _, c := range ws.Content {
			byPath[ws.Path] = append(byPath[ws.Path], c.Name)
		}
	}
	assert.Equal(t, map[string][]string{
		"root:modules:acme":            {"apiexports"},
		"root:modules:acme:validation": {"validation-schemas"},
	}, byPath)
}

// Without workspaces there is no kcp side, so no handshake object is written.
func TestProcessWithoutWorkspacesWritesNoSetup(t *testing.T) {
	mod := testModule()

	workload := fake.NewClientBuilder().WithScheme(scheme(t)).Build()
	reg := clusters.NewRegistry()
	engage(t, reg, "shards-default#customer-a--s1", workload)

	local := fake.NewClientBuilder().WithScheme(scheme(t)).
		WithObjects(platformMesh(true), mod).Build()
	sub := newSubroutineWithClient(t, local, reg)

	_, err := sub.Process(t.Context(), mod)
	require.NoError(t, err)

	err = local.Get(t.Context(),
		ctrlruntimeclient.ObjectKey{Namespace: "pm", Name: "acme"}, &pmdeployerv1alpha1.ModuleSetup{})
	assert.Error(t, err)
}
