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

package controller

import (
	"context"

	pmdeployv1alpha1 "go.platform-mesh.io/apis/deploy/v1alpha1"
	"go.platform-mesh.io/platform-mesh-deployer/pkg/kcp"
	pmocm "go.platform-mesh.io/platform-mesh-deployer/pkg/ocm"
	"go.platform-mesh.io/platform-mesh-deployer/pkg/subroutines/provisioner"
	"go.platform-mesh.io/subroutines"
	"go.platform-mesh.io/subroutines/conditions"
	"go.platform-mesh.io/subroutines/lifecycle"

	"k8s.io/utils/ptr"
	ctrl "sigs.k8s.io/controller-runtime"
	ctrlruntimeclient "sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	mcmanager "sigs.k8s.io/multicluster-runtime/pkg/manager"
	mcreconcile "sigs.k8s.io/multicluster-runtime/pkg/reconcile"
)

const moduleSetupReconcilerName = "ModuleSetupReconciler"

// ModuleSetupReconciler performs the kcp side of a module.
type ModuleSetupReconciler struct {
	lifecycle *lifecycle.Lifecycle
}

func NewModuleSetupReconciler(mgr mcmanager.Manager, access *kcp.Access, resolver pmocm.Resolver) *ModuleSetupReconciler {
	local := mgr.GetLocalManager().GetClient()
	subs := []subroutines.Subroutine{
		provisioner.New(local, access, resolver),
	}
	lc := lifecycle.New(mgr, moduleSetupReconcilerName, func() ctrlruntimeclient.Object {
		return &pmdeployv1alpha1.ModuleSetup{}
	}, subs...).WithConditions(conditions.NewManager())

	return &ModuleSetupReconciler{lifecycle: lc}
}

func (r *ModuleSetupReconciler) SetupWithManager(mgr mcmanager.Manager) error {
	local := mgr.GetLocalManager()
	return ctrl.NewControllerManagedBy(local).
		For(&pmdeployv1alpha1.ModuleSetup{}).
		// The kcp side can only start once the root structure exists.
		Watches(&pmdeployv1alpha1.PlatformMesh{}, handler.EnqueueRequestsFromMapFunc(enqueueSetupsOfPlatformMesh(local.GetClient()))).
		Named(moduleSetupReconcilerName).
		WithOptions(controller.Options{SkipNameValidation: ptr.To(true)}).
		Complete(r)
}

func enqueueSetupsOfPlatformMesh(c ctrlruntimeclient.Client) handler.MapFunc {
	return func(ctx context.Context, obj ctrlruntimeclient.Object) []reconcile.Request {
		list := &pmdeployv1alpha1.ModuleSetupList{}
		if err := c.List(ctx, list); err != nil {
			return nil
		}
		var reqs []reconcile.Request
		for i := range list.Items {
			if list.Items[i].Spec.PlatformMeshRef.Name != obj.GetName() {
				continue
			}
			reqs = append(reqs, reconcile.Request{NamespacedName: ctrlruntimeclient.ObjectKeyFromObject(&list.Items[i])})
		}
		return reqs
	}
}

func (r *ModuleSetupReconciler) Reconcile(ctx context.Context, req reconcile.Request) (reconcile.Result, error) {
	return r.lifecycle.Reconcile(ctx, mcreconcile.Request{Request: req, ClusterName: mcmanager.LocalCluster})
}
