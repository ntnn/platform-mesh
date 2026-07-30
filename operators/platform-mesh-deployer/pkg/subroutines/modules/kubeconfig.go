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

package modules

import (
	"context"
	"fmt"
	"time"

	pmdeployerv1alpha1 "go.platform-mesh.io/apis/deployer/v1alpha1"
	"go.platform-mesh.io/platform-mesh-deployer/pkg/components"
	"go.platform-mesh.io/platform-mesh-deployer/pkg/module"
	"go.platform-mesh.io/platform-mesh-deployer/pkg/sync"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	ctrlruntimeclient "sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"

	operatorv1alpha1 "github.com/kcp-dev/kcp-operator/sdk/apis/operator/v1alpha1"
)

// kubeconfigValidity is the lifetime of a module's client certificate.
// kcp-operator regenerates the secret before it expires.
const kubeconfigValidity = 30 * 24 * time.Hour

// errKubeconfigPending signals that a minted secret has not appeared yet.
var errKubeconfigPending = fmt.Errorf("kubeconfig secret not minted yet")

// ensureKubeconfigs mints the kubeconfigs an instance's component references
// and copies them to the cluster the component runs on. A module is granted
// cluster-admin inside its own workspaces: kcp authorises per logical cluster,
// so the workspace is the boundary.
func (s *Subroutine) ensureKubeconfigs(ctx context.Context, st *state, inst module.Instance) error {
	mod := st.resolved.Module

	for _, kc := range st.resolved.Kubeconfigs(inst.Component) {
		target, err := s.kubeconfigTarget(st, kc, inst)
		if err != nil {
			return err
		}

		name := module.KubeconfigName(mod.Name, kc.Name, inst.Cluster.ClusterID)
		obj := &operatorv1alpha1.Kubeconfig{
			ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: mod.Namespace},
		}
		if _, err := controllerutil.CreateOrUpdate(ctx, s.client, obj, func() error {
			obj.Labels = module.ModuleSelector(mod, inst.Cluster.ClusterID)
			obj.Spec = operatorv1alpha1.KubeconfigSpec{
				Target:          target,
				TargetWorkspace: module.WorkspacePath(mod.Name, kc.Workspace),
				Username:        "module:" + mod.Name + ":" + kc.Name,
				Validity:        metav1.Duration{Duration: kubeconfigValidity},
				SecretRef:       corev1.LocalObjectReference{Name: name},
				Authorization: &operatorv1alpha1.KubeconfigAuthorization{
					ClusterRoleBindings: operatorv1alpha1.KubeconfigClusterRoleBindings{
						ClusterRoles: []string{"cluster-admin"},
					},
				},
			}
			return controllerutil.SetControllerReference(mod, obj, s.client.Scheme())
		}); err != nil {
			return fmt.Errorf("reconciling Kubeconfig %q: %w", name, err)
		}

		if err := s.syncKubeconfig(ctx, st, inst, kc, name); err != nil {
			return err
		}
	}
	return nil
}

// syncKubeconfig copies a minted kubeconfig secret to the component's cluster,
// renaming it to the cluster-local name the payload references.
func (s *Subroutine) syncKubeconfig(ctx context.Context, st *state, inst module.Instance, kc pmdeployerv1alpha1.ModuleKubeconfig, minted string) error {
	mod := st.resolved.Module

	src := &corev1.Secret{}
	key := ctrlruntimeclient.ObjectKey{Namespace: mod.Namespace, Name: minted}
	if err := s.client.Get(ctx, key, src); err != nil {
		if apierrors.IsNotFound(err) {
			return fmt.Errorf("%w: %s", errKubeconfigPending, minted)
		}
		return fmt.Errorf("reading kubeconfig secret %q: %w", minted, err)
	}

	cl := inst.Cluster.Cluster.GetClient()
	if err := sync.EnsureNamespace(ctx, cl, inst.Component.Namespace); err != nil {
		return err
	}

	dst := &corev1.Secret{ObjectMeta: metav1.ObjectMeta{
		Name:      module.KubeconfigSecretName(mod.Name, kc.Name),
		Namespace: inst.Component.Namespace,
	}}
	if _, err := controllerutil.CreateOrUpdate(ctx, cl, dst, func() error {
		dst.Labels = module.ModuleSelector(mod, inst.Cluster.ClusterID)
		dst.Type = src.Type
		dst.Data = src.Data
		return nil
	}); err != nil {
		return fmt.Errorf("copying kubeconfig secret %q: %w", dst.Name, err)
	}
	return nil
}

// kubeconfigTarget resolves which kcp endpoint a kubeconfig is minted against.
// The referenced topology object is named "<spec name>-<cluster ID>".
func (s *Subroutine) kubeconfigTarget(st *state, kc pmdeployerv1alpha1.ModuleKubeconfig, inst module.Instance) (operatorv1alpha1.KubeconfigTarget, error) {
	pm := st.platformMesh

	switch kc.Target {
	case pmdeployerv1alpha1.KubeconfigTargetFrontProxy:
		name, err := s.singleTarget(pm.Name, components.FrontProxy, pm.Spec.Topology.FrontProxy.Name, kc.Name)
		if err != nil {
			return operatorv1alpha1.KubeconfigTarget{}, err
		}
		return operatorv1alpha1.KubeconfigTarget{FrontProxyRef: &corev1.LocalObjectReference{Name: name}}, nil

	case pmdeployerv1alpha1.KubeconfigTargetRootShard:
		name, err := s.singleTarget(pm.Name, components.RootShard, pm.Spec.Topology.RootShard.Name, kc.Name)
		if err != nil {
			return operatorv1alpha1.KubeconfigTarget{}, err
		}
		return operatorv1alpha1.KubeconfigTarget{RootShardRef: &corev1.LocalObjectReference{Name: name}}, nil

	case pmdeployerv1alpha1.KubeconfigTargetShard:
		// A shard kubeconfig belongs to the shard the instance runs
		// beside, which the placement validation guarantees exists.
		if inst.ShardGroup == "" {
			return operatorv1alpha1.KubeconfigTarget{}, fmt.Errorf(
				"kubeconfig %q targets a shard but component %q is not placed per shard", kc.Name, inst.Component.Name)
		}
		name := inst.ShardGroup + "-" + inst.Cluster.ClusterID
		return operatorv1alpha1.KubeconfigTarget{ShardRef: &corev1.LocalObjectReference{Name: name}}, nil

	default:
		return operatorv1alpha1.KubeconfigTarget{}, fmt.Errorf("kubeconfig %q: unknown target %q", kc.Name, kc.Target)
	}
}

// singleTarget resolves a component that must be engaged on exactly one
// cluster. Several front proxies would each need their own kubeconfig, which
// the payload cannot express yet.
func (s *Subroutine) singleTarget(pm, component, specName, kubeconfig string) (string, error) {
	engaged := s.registry.ClustersFor(pm, component)
	switch len(engaged) {
	case 1:
		return specName + "-" + engaged[0].ClusterID, nil
	case 0:
		return "", fmt.Errorf("kubeconfig %q: no %s cluster engaged yet", kubeconfig, component)
	default:
		return "", fmt.Errorf("kubeconfig %q: %s is engaged on %d clusters, which is not supported yet",
			kubeconfig, component, len(engaged))
	}
}
