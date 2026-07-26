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

// Package controller holds the deployer's reconcilers.
package controller

import (
	"context"

	pmdeployerv1alpha1 "go.platform-mesh.io/apis/deployer/v1alpha1"
	"go.platform-mesh.io/platform-mesh-deployer/pkg/subroutines/ready"
	"go.platform-mesh.io/subroutines"
	"go.platform-mesh.io/subroutines/conditions"
	"go.platform-mesh.io/subroutines/lifecycle"

	"k8s.io/utils/ptr"
	ctrl "sigs.k8s.io/controller-runtime"
	ctrlruntimeclient "sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	mcmanager "sigs.k8s.io/multicluster-runtime/pkg/manager"
	mcreconcile "sigs.k8s.io/multicluster-runtime/pkg/reconcile"
)

const platformMeshReconcilerName = "PlatformMeshReconciler"

// PlatformMeshReconciler reconciles PlatformMesh resources.
type PlatformMeshReconciler struct {
	lifecycle *lifecycle.Lifecycle
}

func NewPlatformMeshReconciler(mgr mcmanager.Manager) *PlatformMeshReconciler {
	subs := []subroutines.Subroutine{
		ready.New(),
	}
	lc := lifecycle.New(mgr, platformMeshReconcilerName, func() ctrlruntimeclient.Object {
		return &pmdeployerv1alpha1.PlatformMesh{}
	}, subs...).WithConditions(conditions.NewManager())

	return &PlatformMeshReconciler{lifecycle: lc}
}

func (r *PlatformMeshReconciler) SetupWithManager(mgr mcmanager.Manager) error {
	return ctrl.NewControllerManagedBy(mgr.GetLocalManager()).
		For(&pmdeployerv1alpha1.PlatformMesh{}).
		Named(platformMeshReconcilerName).
		WithOptions(controller.Options{SkipNameValidation: ptr.To(true)}).
		Complete(r)
}

func (r *PlatformMeshReconciler) Reconcile(ctx context.Context, req reconcile.Request) (reconcile.Result, error) {
	return r.lifecycle.Reconcile(ctx, mcreconcile.Request{Request: req, ClusterName: mcmanager.LocalCluster})
}
