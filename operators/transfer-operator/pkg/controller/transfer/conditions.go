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

package transfer

import (
	pmtransferv1alpha1 "go.platform-mesh.io/apis/transfer/v1alpha1"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func (r *reconciler) setConditionReady(status bool, reason, message string) {
	metaStatus := metav1.ConditionTrue
	if !status {
		metaStatus = metav1.ConditionFalse
	}

	meta.SetStatusCondition(
		&r.transfer.Status.Conditions,
		metav1.Condition{
			Type:               pmtransferv1alpha1.TransferConditionReady,
			ObservedGeneration: r.transfer.Generation,
			Status:             metaStatus,
			Reason:             reason,
			Message:            message,
		},
	)
}

func (r *reconciler) setConditionResourcesResolved(status bool, reason, message string) {
	metaStatus := metav1.ConditionTrue
	if !status {
		metaStatus = metav1.ConditionFalse
	}

	meta.SetStatusCondition(
		&r.transfer.Status.Conditions,
		metav1.Condition{
			Type:               pmtransferv1alpha1.TransferConditionResourcesResolved,
			ObservedGeneration: r.transfer.Generation,
			Status:             metaStatus,
			Reason:             reason,
			Message:            message,
		},
	)
	if !status {
		r.setConditionReady(false, "ResourcesNotResolved", "resources are not resolved")
	}
}

func (r *reconciler) setConditionProviderReady(status bool, reason, message string) {
	metaStatus := metav1.ConditionTrue
	if !status {
		metaStatus = metav1.ConditionFalse
	}

	meta.SetStatusCondition(
		&r.transfer.Status.Conditions,
		metav1.Condition{
			Type:               pmtransferv1alpha1.TransferConditionProviderReady,
			ObservedGeneration: r.transfer.Generation,
			Status:             metaStatus,
			Reason:             reason,
			Message:            message,
		},
	)
	if !status {
		r.setConditionReady(false, "ProviderNotReady", "provider is not ready")
	}
}
