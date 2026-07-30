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
	"testing"

	"github.com/stretchr/testify/assert"

	pmdeployerv1alpha1 "go.platform-mesh.io/apis/deployer/v1alpha1"
)

// Children must be deleted before their parents, otherwise removing the parent
// strands them.
func TestDeepestFirst(t *testing.T) {
	got := deepestFirst([]pmdeployerv1alpha1.ModuleSetupWorkspace{
		{Path: "root:modules:acme"},
		{Path: "root:modules:acme:validation:deep"},
		{Path: "root:modules:acme:validation"},
		{Path: "root:modules:other"},
	})
	assert.Equal(t, []string{
		"root:modules:acme:validation:deep",
		"root:modules:acme:validation",
		"root:modules:acme",
		"root:modules:other",
	}, got)
}

func TestDeepestFirstEmpty(t *testing.T) {
	assert.Empty(t, deepestFirst(nil))
}

// A module's payload addresses its workspaces through the published endpoints,
// so the module workspace itself gets a stable name and children keep theirs.
func TestWorkspaceEndpoints(t *testing.T) {
	got := workspaceEndpoints("https://fp.example.com:6443", []pmdeployerv1alpha1.ModuleSetupWorkspace{
		{Path: "root:modules:acme"},
		{Path: "root:modules:acme:validation"},
	})
	assert.Equal(t, map[string]string{
		"workspace":  "https://fp.example.com:6443/clusters/root:modules:acme",
		"validation": "https://fp.example.com:6443/clusters/root:modules:acme:validation",
	}, got)
}

func TestWorkspaceEndpointsEmpty(t *testing.T) {
	assert.Nil(t, workspaceEndpoints("https://fp.example.com:6443", nil))
}
