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
	"encoding/base64"
	"errors"
	"fmt"

	"github.com/go-logr/logr"

	pmtransferv1alpha1 "go.platform-mesh.io/apis/transfer/v1alpha1"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/equality"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/client-go/tools/clientcmd"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/cluster"
	"sigs.k8s.io/multicluster-runtime/pkg/multicluster"
	mcreconcile "sigs.k8s.io/multicluster-runtime/pkg/reconcile"
)

type reconciler struct {
	opts Options
	req  mcreconcile.Request
	log  logr.Logger

	client client.Client

	old      *pmtransferv1alpha1.KubeconfigProvider
	provider *pmtransferv1alpha1.KubeconfigProvider

	secret *corev1.Secret
}

func (r *reconciler) reconcile(ctx context.Context) (ctrl.Result, error) {
	if cont, err := r.fetchClient(ctx); err != nil || !cont {
		return ctrl.Result{}, err
	}
	if cont, err := r.fetchProvider(ctx); err != nil || !cont {
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

func (r *reconciler) fetchProvider(ctx context.Context) (bool, error) {
	provider := &pmtransferv1alpha1.KubeconfigProvider{}
	if err := r.client.Get(ctx, r.req.NamespacedName, provider); err != nil {
		if apierrors.IsNotFound(err) {
			return false, nil
		}
		return false, fmt.Errorf("getting KubeconfigProvider: %w", err)
	}

	r.old, r.provider = provider, provider.DeepCopy()
	return true, nil
}

func (r *reconciler) run(ctx context.Context) (ctrl.Result, error) {
	secretKey := client.ObjectKey{
		Name:      r.provider.Spec.SecretRef.Name,
		Namespace: r.provider.Spec.SecretRef.Namespace,
	}
	if err := r.client.Get(ctx, secretKey, r.secret); err != nil {
		r.setConditionAvailable(false, "SecretNotFound", err.Error())
		return ctrl.Result{}, nil
	}

	b64data, ok := r.secret.Data[r.provider.Spec.Key]
	if !ok {
		r.setConditionAvailable(false, "KeyNotFound", fmt.Sprintf("could not find key %q in secret", r.provider.Spec.Key))
		return ctrl.Result{}, nil
	}

	var data []byte
	if _, err := base64.StdEncoding.Decode(data, b64data); err != nil {
		r.setConditionAvailable(false, "KubeconfigDataNotBase64", err.Error())
		return ctrl.Result{}, nil
	}

	cfg, err := clientcmd.RESTConfigFromKubeConfig(data)
	if err != nil {
		r.setConditionAvailable(false, "KubeconfigInvalid", err.Error())
		return ctrl.Result{}, nil
	}

	cl, err := cluster.New(cfg)
	if err != nil {
		r.setConditionAvailable(false, "ClusterCreationFailed", err.Error())
		return ctrl.Result{}, nil
	}

	// TODO cluster name should be ws#<consumer workspace>#<kubeconfig provider>
	// TODO engage cluster
	// TODO pass an aware
	r.opts.Clusters.AddOrReplace(ctx, multicluster.ClusterName(r.provider.Name), cl, nil)

	r.setConditionAvailable(true, "ClusterEngaged", "")
	return ctrl.Result{}, nil
}

func (r *reconciler) commitStatus(ctx context.Context) error {
	if r.provider == nil || equality.Semantic.DeepEqual(r.old.Status, r.provider.Status) {
		return nil
	}
	return r.client.Status().Patch(ctx, r.provider, client.MergeFrom(r.old))
}
