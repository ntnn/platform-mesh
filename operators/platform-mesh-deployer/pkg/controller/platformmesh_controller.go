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
	"go.platform-mesh.io/platform-mesh-deployer/pkg/clusters"
	"go.platform-mesh.io/platform-mesh-deployer/pkg/subroutines/exposure"
	"go.platform-mesh.io/platform-mesh-deployer/pkg/subroutines/ready"
	"go.platform-mesh.io/platform-mesh-deployer/pkg/subroutines/topology"
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

const platformMeshReconcilerName = "PlatformMeshReconciler"

// PlatformMeshReconciler reconciles PlatformMesh resources.
type PlatformMeshReconciler struct {
	lifecycle *lifecycle.Lifecycle
	registry  *clusters.Registry
}

func NewPlatformMeshReconciler(mgr mcmanager.Manager, registry *clusters.Registry) *PlatformMeshReconciler {
	subs := []subroutines.Subroutine{
		topology.New(mgr.GetLocalManager().GetClient(), registry),
		exposure.New(registry),
		ready.New(),
	}
	lc := lifecycle.New(mgr, platformMeshReconcilerName, func() ctrlruntimeclient.Object {
		return &pmdeployerv1alpha1.PlatformMesh{}
	}, subs...).WithConditions(conditions.NewManager())

	return &PlatformMeshReconciler{lifecycle: lc, registry: registry}
}

func (r *PlatformMeshReconciler) SetupWithManager(mgr mcmanager.Manager) error {
	local := mgr.GetLocalManager()
	return ctrl.NewControllerManagedBy(local).
		For(&pmdeployerv1alpha1.PlatformMesh{}).
		WatchesRawSource(source.Channel(
			r.registry.Events(),
			handler.EnqueueRequestsFromMapFunc(enqueuePlatformMeshByName(local.GetClient())),
		)).
		// A module publishes its resolved front proxy mapping in its
		// status, which the topology merges into the FrontProxy.
		Watches(&pmdeployerv1alpha1.Module{}, handler.EnqueueRequestsFromMapFunc(enqueuePlatformMeshOfModule())).
		Named(platformMeshReconcilerName).
		WithOptions(controller.Options{SkipNameValidation: ptr.To(true)}).
		Complete(r)
}

// enqueuePlatformMeshOfModule maps a Module to the PlatformMesh it belongs to.
func enqueuePlatformMeshOfModule() handler.MapFunc {
	return func(_ context.Context, obj ctrlruntimeclient.Object) []reconcile.Request {
		mod, ok := obj.(*pmdeployerv1alpha1.Module)
		if !ok {
			return nil
		}
		return []reconcile.Request{{NamespacedName: ctrlruntimeclient.ObjectKey{
			Namespace: mod.Namespace,
			Name:      mod.Spec.PlatformMeshRef.Name,
		}}}
	}
}

// enqueuePlatformMeshByName maps a signal from the deployer's
// clusters.Registry to a new request for the PlatformMesh with
// a matching name.
func enqueuePlatformMeshByName(c ctrlruntimeclient.Client) handler.MapFunc {
	return func(ctx context.Context, obj ctrlruntimeclient.Object) []reconcile.Request {
		name := obj.GetName()
		list := &pmdeployerv1alpha1.PlatformMeshList{}
		if err := c.List(ctx, list); err != nil {
			return nil
		}
		var reqs []reconcile.Request
		for i := range list.Items {
			if list.Items[i].Name != name {
				continue
			}
			reqs = append(reqs, reconcile.Request{NamespacedName: ctrlruntimeclient.ObjectKeyFromObject(&list.Items[i])})
		}
		return reqs
	}
}

func (r *PlatformMeshReconciler) Reconcile(ctx context.Context, req reconcile.Request) (reconcile.Result, error) {
	return r.lifecycle.Reconcile(ctx, mcreconcile.Request{Request: req, ClusterName: mcmanager.LocalCluster})
}
