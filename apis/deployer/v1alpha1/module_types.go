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
	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// ModuleSpec defines the desired state of a Module deployed on top of a PlatformMesh installation.
type ModuleSpec struct {
	// PlatformMeshRef references the PlatformMesh installation the module is
	// deployed to.
	PlatformMeshRef corev1.LocalObjectReference `json:"platformMeshRef"`

	// OCM overrides the OCM repository the module component is resolved from.
	// Defaults to the repository of the referenced PlatformMesh.
	// +optional
	OCM *OCMRepository `json:"ocm,omitempty"`

	// Component is the name of the module's OCM component, e.g. github.com/platform-mesh/account-operator.
	// +kubebuilder:validation:MinLength=1
	Component string `json:"component"`

	// Version is the pinned version of the module component.
	// +kubebuilder:validation:MinLength=1
	Version string `json:"version"`

	// Values holds module-specific configuration.
	// +optional
	Values *apiextensionsv1.JSON `json:"values,omitempty"`
}

// ModuleStatus defines the observed state of a Module.
type ModuleStatus struct {
	// +optional
	Conditions []metav1.Condition `json:"conditions,omitempty"`

	// +optional
	ObservedGeneration int64 `json:"observedGeneration,omitempty"`

	// +optional
	NextReconcileTime metav1.Time `json:"nextReconcileTime,omitempty"`

	// ResolvedDigest is the digest of the resolved component version.
	// +optional
	ResolvedDigest string `json:"resolvedDigest,omitempty"`
}

// Module is the schema for a module deployed on top of a PlatformMesh installation.
// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:printcolumn:name="Component",type=string,JSONPath=`.spec.component`
// +kubebuilder:printcolumn:name="Version",type=string,JSONPath=`.spec.version`
// +kubebuilder:printcolumn:name="Age",type=date,JSONPath=`.metadata.creationTimestamp`
type Module struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   ModuleSpec   `json:"spec,omitempty"`
	Status ModuleStatus `json:"status,omitempty"`
}

// ModuleList contains a list of Module.
// +kubebuilder:object:root=true
type ModuleList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []Module `json:"items"`
}

func (m *Module) GetObservedGeneration() int64       { return m.Status.ObservedGeneration }
func (m *Module) SetObservedGeneration(g int64)      { m.Status.ObservedGeneration = g }
func (m *Module) GetNextReconcileTime() metav1.Time  { return m.Status.NextReconcileTime }
func (m *Module) SetNextReconcileTime(t metav1.Time) { m.Status.NextReconcileTime = t }
func (m *Module) GetConditions() []metav1.Condition  { return m.Status.Conditions }
func (m *Module) SetConditions(c []metav1.Condition) { m.Status.Conditions = c }
