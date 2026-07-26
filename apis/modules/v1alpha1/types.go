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

// Package v1alpha1 contains API Schema definitions for the modules v1alpha1 API group.
package v1alpha1

// GroupVersion identifies the module-descriptor schema.
const (
	GroupName  = "modules.platform-mesh.io"
	Version    = "v1alpha1"
	APIVersion = GroupName + "/" + Version
	Kind       = "ModuleDescriptor"
)

// Placement selects where a component is deployed relative to the topology.
type Placement string

const (
	PlacementGlobal        Placement = "global"
	PlacementPerShard      Placement = "per-shard"
	PlacementPerFrontProxy Placement = "per-front-proxy"
)

// KubeconfigTarget selects which topology endpoint a minted kubeconfig points at.
type KubeconfigTarget string

const (
	KubeconfigTargetVirtualWorkspace KubeconfigTarget = "virtual-workspace"
	KubeconfigTargetFrontProxy       KubeconfigTarget = "front-proxy"
	KubeconfigTargetShard            KubeconfigTarget = "shard"
)

// ModuleDescriptor is the parsed module descriptor.
type ModuleDescriptor struct {
	APIVersion string      `json:"apiVersion"`
	Kind       string      `json:"kind"`
	Components []Component `json:"components,omitempty"`
	// Workspaces the module needs provisioned, each with the kcp content
	// (APIResourceSchemas, APIExports, ...) applied inside it by the
	// module-provisioner, never the deployer.
	Workspaces []Workspace `json:"workspaces,omitempty"`
}

// Workspace is a kcp workspace the module needs, relative to the module's own
// provisioned workspace (e.g. root:modules:<module>). An empty Name means the
// module workspace itself; a non-empty Name is a direct child under it and must
// not be nested.
type Workspace struct {
	Name    string        `json:"name,omitempty"`
	Content []ResourceRef `json:"content,omitempty"`
}

// Component is a single deployable unit within the descriptor.
type Component struct {
	Name        string       `json:"name"`
	Placement   Placement    `json:"placement"`
	Payload     Payload      `json:"payload"`
	Kubeconfigs []Kubeconfig `json:"kubeconfigs,omitempty"`
}

// Payload references the CV resource (helm chart or raw manifests) the deployer applies.
type Payload struct {
	Resource string `json:"resource"`
}

// Kubeconfig requests a minted kubeconfig for a topology endpoint.
type Kubeconfig struct {
	Name   string           `json:"name"`
	Target KubeconfigTarget `json:"target"`
}

// ResourceRef names a CV resource.
type ResourceRef struct {
	Resource string `json:"resource"`
}
