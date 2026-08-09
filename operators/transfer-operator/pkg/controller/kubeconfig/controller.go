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

package kubeconfig

import (
	"context"
	"errors"
	"fmt"

	pmtransferv1alpha1 "go.platform-mesh.io/apis/transfer/v1alpha1"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	ctrlruntimeclient "sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/cluster"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	ctrlhandler "sigs.k8s.io/controller-runtime/pkg/handler"
	ctrllog "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	mcbuilder "sigs.k8s.io/multicluster-runtime/pkg/builder"
	"sigs.k8s.io/multicluster-runtime/pkg/clusters"
	mchandler "sigs.k8s.io/multicluster-runtime/pkg/handler"
	mcmanager "sigs.k8s.io/multicluster-runtime/pkg/manager"
	"sigs.k8s.io/multicluster-runtime/pkg/multicluster"
	mcreconcile "sigs.k8s.io/multicluster-runtime/pkg/reconcile"
)

// ControllerName is what this controller is called in metrics and logs.
const ControllerName = "kubeconfigprovider"

// Options configures the controller.
type Options struct {
	// GetCluster returns the cluster.
	GetCluster func(context.Context, multicluster.ClusterName) (cluster.Cluster, error)

	// IsConsumerWorkspace returns true if the cluster is a consumer workspace.
	// Used to only engage consumer workspaces.
	IsConsumerWorkspace func(multicluster.ClusterName) bool

	// WorkspaceIndexer indexes the workspace clusters.
	// Used to watch secrets of reconciled *Provider resources.
	WorkspaceIndexer ctrlruntimeclient.FieldIndexer

	// Clusters is the registry for clusters registered through *Providers.
	Clusters *clusters.Clusters[cluster.Cluster]
}

func (o *Options) validate() error {
	if o.GetCluster == nil {
		return errors.New("GetCluster is required")
	}
	if o.IsConsumerWorkspace == nil {
		return errors.New("IsConsumerWorkspace is required")
	}
	if o.WorkspaceIndexer == nil {
		return errors.New("WorkspaceIndexer is required")
	}
	if o.Clusters == nil {
		return errors.New("Clusters is required")
	}
	return nil
}

// Controller reconciles KubeconfigProviders.
type Controller struct {
	opts Options
}

// NewController returns a controller for the given options.
func NewController(opts Options) (*Controller, error) {
	if err := opts.validate(); err != nil {
		return nil, fmt.Errorf("invalid kubeconfig provider controller options: %w", err)
	}
	return &Controller{opts: opts}, nil
}

// SetupWithManager registers the controller with the manager.
func (c *Controller) SetupWithManager(ctx context.Context, mgr mcmanager.Manager) error {
	if err := indexProviders(ctx, c.opts.WorkspaceIndexer); err != nil {
		return fmt.Errorf("indexing KubeconfigProviders by %s: %w", indexSecretRef, err)
	}

	onWorkspaces := mcbuilder.WithClusterFilter(
		func(name multicluster.ClusterName, _ cluster.Cluster) bool {
			return c.opts.IsConsumerWorkspace(name)
		},
	)

	return mcbuilder.ControllerManagedBy(mgr).
		Named(ControllerName).
		// Two passes over one provider would race on its scope.
		WithOptions(controller.TypedOptions[mcreconcile.Request]{MaxConcurrentReconciles: 1}).
		For(&pmtransferv1alpha1.KubeconfigProvider{}, onWorkspaces).
		Watches(&corev1.Secret{}, enqueueProviderForSecret, onWorkspaces).
		Complete(c)
}

// Reconcile implements the multi-cluster reconciler.
func (c *Controller) Reconcile(ctx context.Context, req mcreconcile.Request) (ctrl.Result, error) {
	r := &reconciler{
		opts: c.opts,
		req:  req,
		log:  ctrllog.FromContext(ctx).WithValues("cluster", req.ClusterName),
	}
	return r.reconcile(ctx)
}

// indexSecretRef is the field to index on the KubeconfigProvider.
const indexSecretRef = "spec.secretRef"

func indexProviders(ctx context.Context, indexer ctrlruntimeclient.FieldIndexer) error {
	// index providers by the secret they reference
	return indexer.IndexField(
		ctx,
		&pmtransferv1alpha1.KubeconfigProvider{},
		indexSecretRef,
		func(obj ctrlruntimeclient.Object) []string {
			provider, ok := obj.(*pmtransferv1alpha1.KubeconfigProvider)
			if !ok {
				return nil
			}
			ref := provider.Spec.SecretRef
			if ref.Name == "" {
				return nil
			}
			if ref.Namespace == "" {
				ref.Namespace = "default"
			}
			key := ctrlruntimeclient.ObjectKey{
				Namespace: ref.Namespace,
				Name:      ref.Name,
			}
			return []string{key.String()}
		},
	)
}

func enqueueProviderForSecret(name multicluster.ClusterName, cl cluster.Cluster) mchandler.EventHandler {
	mapFn := func(ctx context.Context, obj ctrlruntimeclient.Object) []reconcile.Request {
		// obj is a Secret
		key := ctrlruntimeclient.ObjectKeyFromObject(obj)
		// use the index from indexProviders to map secrets to *Providers referencing it
		listOpt := ctrlruntimeclient.MatchingFields{
			indexSecretRef: key.String(),
		}

		// get all *Providers matching it
		providers := &pmtransferv1alpha1.KubeconfigProviderList{}
		if err := cl.GetClient().List(ctx, providers, listOpt); err != nil {
			ctrllog.FromContext(ctx).Error(err, "listing KubeconfigProviders for Secret", "cluster", name, "secret", key)
			return nil
		}

		// build new requests for the *Providers
		reqs := make([]reconcile.Request, 0, len(providers.Items))
		for _, provider := range providers.Items {
			reqs = append(
				reqs,
				reconcile.Request{
					NamespacedName: types.NamespacedName{
						Name: provider.Name,
						// *Provider is cluster-scoped
					},
				},
			)
		}
		return reqs
	}

	return mchandler.ForCluster(
		ctrlhandler.EnqueueRequestsFromMapFunc(mapFn),
		name,
	)
}
