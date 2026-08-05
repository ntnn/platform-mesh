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
)

// ResourceRef references a resource of an OCM component version.
type ResourceRef struct {
	// Name is the name of the resource within the component version.
	// +kubebuilder:validation:MinLength=1
	Name string `json:"name"`

	// Version is the version of the resource.
	// +optional
	Version string `json:"version,omitempty"`

	// Digest is the digest of the resource.
	// +optional
	Digest string `json:"digest,omitempty"`
}

// ModuleSetupSpec defines the kcp-side setup work for a module.
// The deployer writes the spec, the module-provisioner running against kcp performs the setup and reports back via the status.
type ModuleSetupSpec struct {
	// PlatformMeshRef references the PlatformMesh installation.
	PlatformMeshRef corev1.LocalObjectReference `json:"platformMeshRef"`

	// ModuleRef references the Module this setup belongs to.
	ModuleRef corev1.LocalObjectReference `json:"moduleRef"`

	// ComponentDigest is the digest of the resolved module component
	// version.
	// +optional
	ComponentDigest string `json:"componentDigest,omitempty"`

	// Workspaces are the kcp workspaces the module is set up in, each with
	// the content to apply inside it.
	// +optional
	// +listType=map
	// +listMapKey=path
	Workspaces []ModuleSetupWorkspace `json:"workspaces,omitempty"`

	// KubeconfigRefs reference secrets containing kubeconfigs for the target
	// kcp workspaces.
	// +optional
	KubeconfigRefs []corev1.LocalObjectReference `json:"kubeconfigRefs,omitempty"`
}

// ModuleSetupWorkspace is one kcp workspace and the content that belongs in it.
type ModuleSetupWorkspace struct {
	// Path is the absolute kcp workspace path.
	// +kubebuilder:validation:MinLength=1
	Path string `json:"path"`

	// Content references the component version resources holding the
	// manifests applied inside this workspace.
	// +optional
	Content []ResourceRef `json:"content,omitempty"`
}

// ModuleSetupStatus defines the observed state of a ModuleSetup. It is
// written by the module-provisioner.
type ModuleSetupStatus struct {
	// +optional
	Conditions []metav1.Condition `json:"conditions,omitempty"`

	// +optional
	ObservedGeneration int64 `json:"observedGeneration,omitempty"`

	// +optional
	NextReconcileTime metav1.Time `json:"nextReconcileTime,omitempty"`

	// Endpoints are the endpoints exposed by the module setup.
	// +optional
	Endpoints map[string]string `json:"endpoints,omitempty"`
}

// ModuleSetup is the schema for the kcp-side setup work of a module.
// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:printcolumn:name="Module",type=string,JSONPath=`.spec.moduleRef.name`
// +kubebuilder:printcolumn:name="Age",type=date,JSONPath=`.metadata.creationTimestamp`
type ModuleSetup struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   ModuleSetupSpec   `json:"spec,omitempty"`
	Status ModuleSetupStatus `json:"status,omitempty"`
}

// ModuleSetupList contains a list of ModuleSetup.
// +kubebuilder:object:root=true
type ModuleSetupList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []ModuleSetup `json:"items"`
}

func (ms *ModuleSetup) GetObservedGeneration() int64       { return ms.Status.ObservedGeneration }
func (ms *ModuleSetup) SetObservedGeneration(g int64)      { ms.Status.ObservedGeneration = g }
func (ms *ModuleSetup) GetNextReconcileTime() metav1.Time  { return ms.Status.NextReconcileTime }
func (ms *ModuleSetup) SetNextReconcileTime(t metav1.Time) { ms.Status.NextReconcileTime = t }
func (ms *ModuleSetup) GetConditions() []metav1.Condition  { return ms.Status.Conditions }
func (ms *ModuleSetup) SetConditions(c []metav1.Condition) { ms.Status.Conditions = c }
