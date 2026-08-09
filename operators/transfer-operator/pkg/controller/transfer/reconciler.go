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

	"github.com/go-logr/logr"

	pmtransferv1alpha1 "go.platform-mesh.io/apis/transfer/v1alpha1"

	"k8s.io/apimachinery/pkg/api/equality"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/multicluster-runtime/pkg/multicluster"
	mcreconcile "sigs.k8s.io/multicluster-runtime/pkg/reconcile"
)

type reconciler struct {
	opts Options
	req  mcreconcile.Request
	log  logr.Logger

	client client.Client

	old      *pmtransferv1alpha1.Transfer
	transfer *pmtransferv1alpha1.Transfer
}

func (r *reconciler) reconcile(ctx context.Context) (ctrl.Result, error) {
	if cont, err := r.fetchClient(ctx); err != nil || !cont {
		return ctrl.Result{}, err
	}
	if cont, err := r.fetchTransfer(ctx); err != nil || !cont {
		return ctrl.Result{}, err
	}

	result, err := r.run(ctx)
	return result, errors.Join(err, r.commitStatus(ctx))
}

func (r *reconciler) fetchClient(ctx context.Context) (bool, error) {
	cl, err := r.opts.GetCluster(ctx, r.req.ClusterName)
	if err != nil {
		if errors.Is(err, multicluster.ErrClusterNotFound) {
			return false, nil
		}
		return false, fmt.Errorf("getting cluster %q: %w", r.req.ClusterName, err)
	}
	r.client = cl.GetClient()
	return true, nil
}

func (r *reconciler) fetchTransfer(ctx context.Context) (bool, error) {
	transfer := &pmtransferv1alpha1.Transfer{}
	if err := r.client.Get(ctx, r.req.NamespacedName, transfer); err != nil {
		if apierrors.IsNotFound(err) {
			return false, nil
		}
		return false, fmt.Errorf("getting Transfer: %w", err)
	}

	r.old, r.transfer = transfer, transfer.DeepCopy()
	return true, nil
}

func (r *reconciler) run(ctx context.Context) (ctrl.Result, error) {
	// TODO provieer -> cluster

	// TODO resolve resources

	// TODO transfer

	return ctrl.Result{}, nil
}

func (r *reconciler) commitStatus(ctx context.Context) error {
	if r.transfer == nil || equality.Semantic.DeepEqual(r.old.Status, r.transfer.Status) {
		return nil
	}
	return r.client.Status().Patch(ctx, r.transfer, client.MergeFrom(r.old))
}
