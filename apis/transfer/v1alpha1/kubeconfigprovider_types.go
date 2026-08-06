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

// +kubebuilder:rbac:groups=transfer.platform-mesh.io,resources=kubeconfigproviders,verbs=get;list;watch
// +kubebuilder:rbac:groups=transfer.platform-mesh.io,resources=kubeconfigproviders/status,verbs=get;update;patch
// +kubebuilder:rbac:groups="",resources=secrets,verbs=get;list;watch

// KubeconfigProviderSpec defines the desired state of KubeconfigProvider.
type KubeconfigProviderSpec struct {
	// SecretRef names the Secret holding the kubeconfig of the cluster.
	// +kubebuilder:validation:Required
	SecretRef corev1.SecretReference `json:"secretRef"`

	// Key is the Secret data key holding the kubeconfig.
	// +optional
	// +kubebuilder:default=kubeconfig
	// +kubebuilder:validation:MinLength=1
	Key string `json:"key,omitempty"`
}

// Condition types for KubeconfigProvider.
const (
	// KubeconfigProviderConditionAvailable indicates whether the referenced Secret yielded a reachable cluster.
	KubeconfigProviderConditionAvailable = "Available"
)

// KubeconfigProviderStatus defines the observed state of KubeconfigProvider.
type KubeconfigProviderStatus struct {
	// conditions represent the current state of the KubeconfigProvider resource.
	//
	// Condition types used by the transfer operator:
	// - "Available": the referenced Secret yielded a reachable cluster
	// +listType=map
	// +listMapKey=type
	// +optional
	Conditions []metav1.Condition `json:"conditions,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:resource:scope=Cluster
// +kubebuilder:subresource:status
// +kubebuilder:printcolumn:name="Secret",type=string,JSONPath=`.spec.secretRef.name`
// +kubebuilder:printcolumn:name="Available",type=string,JSONPath=`.status.conditions[?(@.type=="Available")].status`
// +kubebuilder:printcolumn:name="Age",type=date,JSONPath=`.metadata.creationTimestamp`

// KubeconfigProvider engages a remote cluster from kubeconfig Secret.
// The object's name is the provider name that a Transfer refers to.
type KubeconfigProvider struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   KubeconfigProviderSpec   `json:"spec,omitempty"`
	Status KubeconfigProviderStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// KubeconfigProviderList contains a list of KubeconfigProvider.
type KubeconfigProviderList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []KubeconfigProvider `json:"items"`
}
