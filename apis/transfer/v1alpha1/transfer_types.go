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
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

// +kubebuilder:rbac:groups=transfer.platform-mesh.io,resources=transfers,verbs=get;list;watch
// +kubebuilder:rbac:groups=transfer.platform-mesh.io,resources=transfers/status,verbs=get;update;patch

// Direction is the direction of a transport.
// +kubebuilder:validation:Enum=Push;Pull
type Direction string

const (
	// DirectionPush copies the spec from the control plane to the remote cluster and reflects the remote status back.
	DirectionPush Direction = "Push"

	// DirectionPull copies the spec from the remote cluster to the control plane and reflects the local status back.
	DirectionPull Direction = "Pull"
)

// TransferSpec defines the desired state of Transfer.
type TransferSpec struct {
	// ProviderRef names any Provider.
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:XValidation:rule="size(self.name) > 0",message="providerRef.name must not be empty"
	ProviderRef corev1.LocalObjectReference `json:"providerRef"`

	// Direction decides which side owns the spec.
	// +kubebuilder:validation:Required
	Direction Direction `json:"direction"`

	// Resources selects the objects to transfer.
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:MinItems=1
	Resources []ResourceSelector `json:"resources"`
}

// ResourceSelector narrows the objects to transfer.
// +kubebuilder:validation:XValidation:rule="!(has(self.resource) && size(self.resource) > 0 && has(self.kind) && size(self.kind) > 0)",message="resource and kind are mutually exclusive"
type ResourceSelector struct {
	// Group is the API group to transfer. The empty string selects the core group.
	Group string `json:"group"`

	// Version, when empty, resolves to the storage version of every kind in Group.
	// +optional
	Version string `json:"version,omitempty"`

	// Resource is the plural resource name. Mutually exclusive with Kind.
	// +optional
	Resource string `json:"resource,omitempty"`

	// Kind is the kind name. Mutually exclusive with Resource.
	// +optional
	Kind string `json:"kind,omitempty"`

	// Namespaces restricts the transfer to these namespaces.
	// Empty selects all of them, including cluster-scoped resources.
	// Must be empty for cluster-scoped resources.
	// +optional
	Namespaces []string `json:"namespaces,omitempty"`

	// Names restricts the transfer to explicitly named objects.
	// It composes with any level of narrowing above, so a group and
	// a name transfers every object of that name in the group.
	// +optional
	Names []string `json:"names,omitempty"`

	// Selector restricts the transfer to objects carrying these labels.
	// +optional
	Selector *metav1.LabelSelector `json:"selector,omitempty"`
}

// GroupVersionKind returns the selector's kind as a schema.GroupVersionKind.
func (s ResourceSelector) GroupVersionKind() schema.GroupVersionKind {
	return schema.GroupVersionKind{Group: s.Group, Version: s.Version, Kind: s.Kind}
}

// Condition types for Transfer.
const (
	// TransferConditionResourcesResolved indicates whether every ResourceSelector resolved to at least one kind served by the clusters it applies to.
	TransferConditionResourcesResolved = "ResourcesResolved"

	// TransferConditionProviderReady indicates whether the referenced KubeconfigProvider exists in this workspace and has engaged its clusters.
	TransferConditionProviderReady = "ProviderReady"

	// TransferConditionReady indicates whether objects are being transferred.
	TransferConditionReady = "Ready"
)

// Reasons for TransferConditionReady.
const (
	// TransferReasonConflict is set when a target object already exists and was not written by this Transfer. Nothing is overwritten.
	TransferReasonConflict = "Conflict"
)

// TransferStatus defines the observed state of Transfer.
//
// Like KubeconfigProviderStatus it reports nothing per cluster, so its size is
// independent of how many clusters the referenced provider engages.
type TransferStatus struct {
	// ResolvedResources are the concrete kinds the selectors resolved to.
	// +optional
	ResolvedResources []metav1.GroupVersionKind `json:"resolvedResources,omitempty"`

	// conditions represent the current state of the Transfer resource.
	//
	// Condition types used by the transfer operator:
	// - "ResourcesResolved": every selector resolved to at least one kind
	// - "ProviderReady": the referenced KubeconfigProvider is usable
	// - "Ready": objects are being transferred
	// +listType=map
	// +listMapKey=type
	// +optional
	Conditions []metav1.Condition `json:"conditions,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:resource:scope=Cluster
// +kubebuilder:subresource:status
// +kubebuilder:printcolumn:name="Direction",type=string,JSONPath=`.spec.direction`
// +kubebuilder:printcolumn:name="Provider",type=string,JSONPath=`.spec.providerRef.name`
// +kubebuilder:printcolumn:name="Ready",type=string,JSONPath=`.status.conditions[?(@.type=="Ready")].status`
// +kubebuilder:printcolumn:name="Age",type=date,JSONPath=`.metadata.creationTimestamp`

// Transfer mirrors objects between this control plane and a target.
type Transfer struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   TransferSpec   `json:"spec,omitempty"`
	Status TransferStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// TransferList contains a list of Transfer.
type TransferList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []Transfer `json:"items"`
}
