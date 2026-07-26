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

package v1alpha1_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	pmmodulesv1alpha1 "go.platform-mesh.io/apis/modules/v1alpha1"
)

func TestParse(t *testing.T) {
	const valid = `
apiVersion: modules.platform-mesh.io/v1alpha1
kind: ModuleDescriptor
components:
  - name: account-operator
    placement: global
    payload:
      resource: chart
    kubeconfigs:
      - name: admin
        target: virtual-workspace
workspaces:
  - content:
      - resource: apibindings
  - name: validation
    content:
      - resource: validation-schemas
`

	tests := []struct {
		name    string
		in      string
		wantErr string
		assert  func(t *testing.T, md *pmmodulesv1alpha1.ModuleDescriptor)
	}{
		{
			name: "valid",
			in:   valid,
			assert: func(t *testing.T, md *pmmodulesv1alpha1.ModuleDescriptor) {
				require.Len(t, md.Components, 1)
				assert.Equal(t, "account-operator", md.Components[0].Name)
				assert.Equal(t, pmmodulesv1alpha1.PlacementGlobal, md.Components[0].Placement)
				assert.Equal(t, "chart", md.Components[0].Payload.Resource)
				require.Len(t, md.Components[0].Kubeconfigs, 1)
				assert.Equal(t, pmmodulesv1alpha1.KubeconfigTargetVirtualWorkspace, md.Components[0].Kubeconfigs[0].Target)
				require.Len(t, md.Workspaces, 2)
				assert.Empty(t, md.Workspaces[0].Name)
				require.Len(t, md.Workspaces[0].Content, 1)
				assert.Equal(t, "apibindings", md.Workspaces[0].Content[0].Resource)
				assert.Equal(t, "validation", md.Workspaces[1].Name)
				assert.Equal(t, "validation-schemas", md.Workspaces[1].Content[0].Resource)
			},
		},
		{
			name: "unknown field rejected",
			in: `
apiVersion: modules.platform-mesh.io/v1alpha1
kind: ModuleDescriptor
bogus: true
`,
			wantErr: "decoding module descriptor",
		},
		{
			name: "missing apiVersion",
			in: `
kind: ModuleDescriptor
`,
			wantErr: "unsupported apiVersion",
		},
		{
			name: "wrong apiVersion",
			in: `
apiVersion: modules.platform-mesh.io/v1beta1
kind: ModuleDescriptor
`,
			wantErr: "unsupported apiVersion",
		},
		{
			name: "wrong kind",
			in: `
apiVersion: modules.platform-mesh.io/v1alpha1
kind: Module
`,
			wantErr: "unsupported kind",
		},
		{
			name: "bad placement",
			in: `
apiVersion: modules.platform-mesh.io/v1alpha1
kind: ModuleDescriptor
components:
  - name: x
    placement: everywhere
    payload:
      resource: chart
`,
			wantErr: "invalid placement",
		},
		{
			name: "bad kubeconfig target",
			in: `
apiVersion: modules.platform-mesh.io/v1alpha1
kind: ModuleDescriptor
components:
  - name: x
    placement: global
    payload:
      resource: chart
    kubeconfigs:
      - name: admin
        target: nowhere
`,
			wantErr: "invalid target",
		},
		{
			name: "nested workspace name",
			in: `
apiVersion: modules.platform-mesh.io/v1alpha1
kind: ModuleDescriptor
workspaces:
  - name: validation:deep
`,
			wantErr: "must not be nested",
		},
		{
			name: "duplicate workspace name",
			in: `
apiVersion: modules.platform-mesh.io/v1alpha1
kind: ModuleDescriptor
workspaces:
  - name: validation
  - name: validation
`,
			wantErr: "duplicate workspace",
		},
		{
			name: "duplicate empty workspace",
			in: `
apiVersion: modules.platform-mesh.io/v1alpha1
kind: ModuleDescriptor
workspaces:
  - content:
      - resource: a
  - content:
      - resource: b
`,
			wantErr: "duplicate workspace",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			md, err := pmmodulesv1alpha1.Parse([]byte(tt.in))
			if tt.wantErr != "" {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.wantErr)
				return
			}
			require.NoError(t, err)
			if tt.assert != nil {
				tt.assert(t, md)
			}
		})
	}
}
