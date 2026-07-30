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

package module_test

import (
	"testing"

	"github.com/stretchr/testify/assert"

	pmdeployerv1alpha1 "go.platform-mesh.io/apis/deployer/v1alpha1"
	"go.platform-mesh.io/platform-mesh-deployer/pkg/module"
)

// An empty name is the module's own workspace; anything else is a direct child.
func TestWorkspacePath(t *testing.T) {
	assert.Equal(t, "root:modules:acme", module.WorkspacePath("acme", ""))
	assert.Equal(t, "root:modules:acme:validation", module.WorkspacePath("acme", "validation"))
}

// Derived names carry no cluster ID: a cluster holds at most one instance of a
// component, so they cannot collide there.
func TestDerivedNames(t *testing.T) {
	assert.Equal(t, "acme-kcp", module.KubeconfigSecretName("acme", "kcp"))
	assert.Equal(t, "acme-kcp-s1", module.KubeconfigName("acme", "kcp", "s1"))
	assert.Equal(t, "acme-app-serving", module.ServingCertSecretName("acme", "app"))
	assert.Equal(t, "acme-app-serving-s1", module.ServingCertName("acme", "app", "s1"))
	assert.Equal(t, "acme-app", module.ConfigMapName("acme", "app"))
}

// A component only gets the kubeconfigs it references, in the order it lists
// them, and unknown names are ignored rather than failing the render.
func TestKubeconfigsOfComponent(t *testing.T) {
	mod := moduleWith(component("app", pmdeployerv1alpha1.PlacementRootShard))
	mod.Spec.Kubeconfigs = []pmdeployerv1alpha1.ModuleKubeconfig{
		{Name: "kcp", Target: pmdeployerv1alpha1.KubeconfigTargetFrontProxy},
		{Name: "shardadmin", Target: pmdeployerv1alpha1.KubeconfigTargetShard},
	}
	resolved := &module.Resolved{Module: mod}

	got := resolved.Kubeconfigs(pmdeployerv1alpha1.ModuleComponent{
		Kubeconfigs: []string{"shardadmin", "unknown", "kcp"},
	})
	names := make([]string, 0, len(got))
	for _, kc := range got {
		names = append(names, kc.Name)
	}
	assert.Equal(t, []string{"shardadmin", "kcp"}, names)
}

// The module's own workspace is separate so templating does not need an empty
// map key for it.
func TestWorkspacePaths(t *testing.T) {
	mod := moduleWith(component("app", pmdeployerv1alpha1.PlacementRootShard))
	mod.Spec.Workspaces = []pmdeployerv1alpha1.ModuleWorkspace{
		{Name: ""},
		{Name: "validation"},
	}
	resolved := &module.Resolved{Module: mod}

	own, children := resolved.WorkspacePaths()
	assert.Equal(t, "root:modules:acme", own)
	assert.Equal(t, map[string]string{"validation": "root:modules:acme:validation"}, children)
}
