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

package v1alpha1

import (
	"encoding/json"
	"testing"

	"k8s.io/apimachinery/pkg/api/equality"
)

func fuzzRoundTrip[T any](t *testing.T, data []byte, obj *T, obj2 *T) {
	t.Helper()
	if err := json.Unmarshal(data, obj); err != nil {
		return
	}
	roundtripped, err := json.Marshal(obj)
	if err != nil {
		t.Fatalf("failed to marshal: %v", err)
	}
	if err := json.Unmarshal(roundtripped, obj2); err != nil {
		t.Fatalf("failed to unmarshal roundtripped data: %v", err)
	}
	if !equality.Semantic.DeepEqual(obj, obj2) {
		t.Errorf("roundtrip mismatch for %T", obj)
	}
}

func FuzzPlatformMeshRoundTrip(f *testing.F) {
	f.Add([]byte(`{"apiVersion":"deployer.platform-mesh.io/v1alpha1","kind":"PlatformMesh","metadata":{"name":"pm"},"spec":{"version":"1.0.0","ocm":{"url":"ghcr.io/platform-mesh","component":"github.com/platform-mesh/platform-mesh"},"topology":{"rootShard":{"name":"root","template":{"replicas":2,"extraArgs":["--v=4"]},"virtualWorkspaces":{"mode":"Embedded","template":{"replicas":1},"exposure":{"hostnameTemplate":"vw.{{ .Cluster }}.example.com","port":443}}},"frontProxies":[{"name":"public","template":{"replicas":3},"exposure":{"hostnameTemplate":"kcp.example.com","port":443}}],"cacheServers":[{"name":"global","template":{"replicas":1}}]},"ingress":[{"name":"default","type":"gatewayapi"}]}}`))
	f.Add([]byte(`{"spec":{"version":"1.0.0","ocm":{"url":"ghcr.io/platform-mesh"},"topology":{"rootShard":{"name":"root","virtualWorkspaces":{"exposure":{"hostnameTemplate":"vw.example.com","port":6443}}}}}}`))
	f.Add([]byte(`{}`))
	f.Fuzz(func(t *testing.T, data []byte) {
		fuzzRoundTrip(t, data, &PlatformMesh{}, &PlatformMesh{})
	})
}

func FuzzModuleRoundTrip(f *testing.F) {
	f.Add([]byte(`{"apiVersion":"deployer.platform-mesh.io/v1alpha1","kind":"Module","metadata":{"name":"account-operator"},"spec":{"platformMeshRef":{"name":"pm"},"component":"github.com/platform-mesh/account-operator","version":"0.1.0","values":{"replicas":2}}}`))
	f.Add([]byte(`{"spec":{"platformMeshRef":{"name":"pm"},"component":"github.com/platform-mesh/account-operator","version":"0.1.0","ocm":{"url":"ghcr.io/other","secretRef":{"name":"creds"}}}}`))
	f.Add([]byte(`{}`))
	f.Fuzz(func(t *testing.T, data []byte) {
		fuzzRoundTrip(t, data, &Module{}, &Module{})
	})
}

func FuzzModuleSetupRoundTrip(f *testing.F) {
	f.Add([]byte(`{"apiVersion":"deployer.platform-mesh.io/v1alpha1","kind":"ModuleSetup","metadata":{"name":"account-operator-setup"},"spec":{"platformMeshRef":{"name":"pm"},"moduleRef":{"name":"account-operator"},"componentDigest":"sha256:abc","kcpContent":[{"name":"kcp-manifests","version":"0.1.0"}],"workspaces":["root:orgs"],"kubeconfigRefs":[{"name":"ws-kubeconfig"}]},"status":{"endpoints":{"api":"https://kcp.example.com"}}}`))
	f.Add([]byte(`{"spec":{"platformMeshRef":{"name":"pm"},"moduleRef":{"name":"account-operator"}}}`))
	f.Add([]byte(`{}`))
	f.Fuzz(func(t *testing.T, data []byte) {
		fuzzRoundTrip(t, data, &ModuleSetup{}, &ModuleSetup{})
	})
}
