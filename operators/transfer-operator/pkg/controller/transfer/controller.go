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
	"context"
	"errors"
	"fmt"

	pmtransferv1alpha1 "go.platform-mesh.io/apis/transfer/v1alpha1"

	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/cluster"
	ctrllog "sigs.k8s.io/controller-runtime/pkg/log"
	mcbuilder "sigs.k8s.io/multicluster-runtime/pkg/builder"
	mcmanager "sigs.k8s.io/multicluster-runtime/pkg/manager"
	"sigs.k8s.io/multicluster-runtime/pkg/multicluster"
	mcreconcile "sigs.k8s.io/multicluster-runtime/pkg/reconcile"
)

// ControllerName is what this controller is called in metrics and logs.
const ControllerName = "transfer"

// Options configures the controller.
type Options struct {
	// GetCluster resolves an engaged cluster by name.
	GetCluster func(ctx context.Context, name multicluster.ClusterName) (cluster.Cluster, error)

	// IsConsumerWorkspace returns true if the cluster is a consumer workspace.
	// Used to only engage consumer workspaces.
	IsConsumerWorkspace func(multicluster.ClusterName) bool
}

func (o *Options) validate() error {
	if o.GetCluster == nil {
		return errors.New("a GetCluster func is required")
	}
	if o.IsConsumerWorkspace == nil {
		return errors.New("IsConsumerWorkspace is required")
	}
	return nil
}

// Controller reconciles Transfers.
type Controller struct {
	opts Options
}

// NewController returns a controller for the given options.
func NewController(opts Options) (*Controller, error) {
	if err := opts.validate(); err != nil {
		return nil, fmt.Errorf("invalid transfer controller options: %w", err)
	}
	return &Controller{opts: opts}, nil
}

// SetupWithManager registers the controller with the manager.
func (c *Controller) SetupWithManager(mgr mcmanager.Manager) error {
	onWorkspaces := mcbuilder.WithClusterFilter(
		func(name multicluster.ClusterName, _ cluster.Cluster) bool {
			return c.opts.IsConsumerWorkspace(name)
		},
	)

	// TODO(ntnn): Watch *Providers? Or build indizes from Transfer to
	// *Provider to enqueue a Transfer when its reference *Provider
	// changed?
	// Might need some form of annotation propagation to nudge events
	// along if e.g. a secret of a provider is changed the provider is
	// nudged, which then nudges the Transfer.
	// Anyway - for now only watching Transfer.

	return mcbuilder.ControllerManagedBy(mgr).
		Named(ControllerName).
		For(&pmtransferv1alpha1.Transfer{}, onWorkspaces).
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
