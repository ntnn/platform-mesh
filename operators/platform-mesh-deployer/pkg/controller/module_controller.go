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

	pmdeployv1alpha1 "go.platform-mesh.io/apis/deploy/v1alpha1"
	"go.platform-mesh.io/platform-mesh-deployer/pkg/clusters"
	"go.platform-mesh.io/platform-mesh-deployer/pkg/ocm"
	"go.platform-mesh.io/platform-mesh-deployer/pkg/subroutines/modules"
	"go.platform-mesh.io/subroutines"
	"go.platform-mesh.io/subroutines/conditions"
	"go.platform-mesh.io/subroutines/lifecycle"

	"k8s.io/utils/ptr"
	ctrl "sigs.k8s.io/controller-runtime"
	ctrlruntimeclient "sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	"sigs.k8s.io/controller-runtime/pkg/source"
	mcmanager "sigs.k8s.io/multicluster-runtime/pkg/manager"
	mcreconcile "sigs.k8s.io/multicluster-runtime/pkg/reconcile"
)

const moduleReconcilerName = "ModuleReconciler"

// ModuleReconciler reconciles Module resources.
type ModuleReconciler struct {
	lifecycle *lifecycle.Lifecycle
	registry  *clusters.Registry
}

func NewModuleReconciler(mgr mcmanager.Manager, registry *clusters.Registry, resolver ocm.Resolver) *ModuleReconciler {
	local := mgr.GetLocalManager().GetClient()
	subs := []subroutines.Subroutine{
		modules.New(local, registry, resolver),
	}
	lc := lifecycle.New(
		mgr,
		moduleReconcilerName,
		func() ctrlruntimeclient.Object {
			return &pmdeployv1alpha1.Module{}
		},
		subs...,
	).WithConditions(conditions.NewManager())

	return &ModuleReconciler{lifecycle: lc, registry: registry}
}

func (r *ModuleReconciler) SetupWithManager(mgr mcmanager.Manager) error {
	local := mgr.GetLocalManager()
	return ctrl.NewControllerManagedBy(local).
		For(&pmdeployv1alpha1.Module{}).
		// A Module is gated on its PlatformMesh's topology and on the clusters engaged for it, so both drive a reconcile.
		Watches(&pmdeployv1alpha1.PlatformMesh{}, handler.EnqueueRequestsFromMapFunc(enqueueModulesOfPlatformMesh(local.GetClient()))).
		WatchesRawSource(source.Channel(
			r.registry.Events(),
			handler.EnqueueRequestsFromMapFunc(enqueueModulesOfPlatformMesh(local.GetClient())),
		)).
		Named(moduleReconcilerName).
		WithOptions(controller.Options{SkipNameValidation: ptr.To(true)}).
		Complete(r)
}

// enqueueModulesOfPlatformMesh maps a PlatformMesh, or a cluster registry
// signal carrying its name, to the Modules installed into it.
func enqueueModulesOfPlatformMesh(c ctrlruntimeclient.Client) handler.MapFunc {
	return func(ctx context.Context, obj ctrlruntimeclient.Object) []reconcile.Request {
		list := &pmdeployv1alpha1.ModuleList{}
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

func (r *ModuleReconciler) Reconcile(ctx context.Context, req reconcile.Request) (reconcile.Result, error) {
	return r.lifecycle.Reconcile(ctx, mcreconcile.Request{Request: req, ClusterName: mcmanager.LocalCluster})
}
