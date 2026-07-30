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

package ready_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	pmdeployerv1alpha1 "go.platform-mesh.io/apis/deployer/v1alpha1"
	"go.platform-mesh.io/platform-mesh-deployer/pkg/subroutines/ready"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestProcessRecordsResolvedVersion(t *testing.T) {
	pm := &pmdeployerv1alpha1.PlatformMesh{
		ObjectMeta: metav1.ObjectMeta{Name: "customer-a", Namespace: "pm"},
		Spec:       pmdeployerv1alpha1.PlatformMeshSpec{Version: "1.2.3"},
	}

	sub := ready.New()
	assert.Equal(t, ready.Name, sub.GetName())

	res, err := sub.Process(t.Context(), pm)
	require.NoError(t, err)
	assert.True(t, res.IsContinue())
	assert.Equal(t, "1.2.3", pm.Status.ResolvedVersion)
}
