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

// Package modules reconciles Module resources: resolve the component version,
// gate on dependencies and the topology, fan out over the engaged clusters and
// apply the payloads.
package modules

import (
	"context"
	"errors"
	"fmt"

	pmdeployerv1alpha1 "go.platform-mesh.io/apis/deployer/v1alpha1"
	"go.platform-mesh.io/platform-mesh-deployer/pkg/clusters"
	"go.platform-mesh.io/platform-mesh-deployer/pkg/module"
	"go.platform-mesh.io/platform-mesh-deployer/pkg/ocm"
	"go.platform-mesh.io/subroutines"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	ctrlruntimeclient "sigs.k8s.io/controller-runtime/pkg/client"
)

// state carries what resolve produced to the later steps of one reconcile.
// The subroutine lifecycle only hands each step the object, so the chain is
// driven from a single Process instead of one subroutine per step.
type state struct {
	resolved     *module.Resolved
	platformMesh *pmdeployerv1alpha1.PlatformMesh
	instances    []module.Instance
	// endpoints are published by the provisioner once the kcp side is done.
	endpoints map[string]string
}

const Name = "ModuleSubroutine"

// Condition types reported on a Module.
const (
	ConditionResolved = "Resolved"
	ConditionGated    = "DependenciesReady"
	ConditionDeployed = "Deployed"
)

// Subroutine reconciles one Module.
type Subroutine struct {
	client   ctrlruntimeclient.Client
	registry *clusters.Registry
	resolver ocm.Resolver
}

func New(client ctrlruntimeclient.Client, registry *clusters.Registry, resolver ocm.Resolver) *Subroutine {
	return &Subroutine{client: client, registry: registry, resolver: resolver}
}

func (s *Subroutine) GetName() string { return Name }

func (s *Subroutine) Process(ctx context.Context, obj ctrlruntimeclient.Object) (subroutines.Result, error) {
	mod := obj.(*pmdeployerv1alpha1.Module)

	// Post-topology modules are the only ones implemented; pre-topology
	// modules additionally gate the topology itself and land later.
	pm, err := s.platformMesh(ctx, mod)
	if err != nil {
		return subroutines.Result{}, err
	}

	ready, reason := topologyReady(pm)
	if mod.Spec.Stage == pmdeployerv1alpha1.StagePostTopology && !ready {
		setCondition(mod, ConditionGated, metav1.ConditionFalse, "WaitingForTopology", reason)
		return subroutines.StopWithRequeue(requeueWait, reason), nil
	}

	if ok, reason := s.dependenciesReady(ctx, mod); !ok {
		setCondition(mod, ConditionGated, metav1.ConditionFalse, "WaitingForDependency", reason)
		return subroutines.StopWithRequeue(requeueWait, reason), nil
	}
	setCondition(mod, ConditionGated, metav1.ConditionTrue, "Ready", "dependencies ready")

	resolved, err := module.Resolve(ctx, s.resolver, mod, &pm.Spec.OCM)
	if err != nil {
		setCondition(mod, ConditionResolved, metav1.ConditionFalse, "ResolveFailed", err.Error())
		return subroutines.Result{}, err
	}
	setCondition(mod, ConditionResolved, metav1.ConditionTrue, "Resolved", "component version resolved")
	mod.Status.ResolvedDigest = resolved.Digest()

	instances, err := module.FanOut(s.registry, mod)
	if err != nil {
		return subroutines.Result{}, err
	}

	st := &state{resolved: resolved, platformMesh: pm, instances: instances}

	if err := s.ensureSetup(ctx, st); err != nil {
		if errors.Is(err, errSetupPending) {
			setCondition(mod, ConditionDeployed, metav1.ConditionFalse, "WaitingForSetup", err.Error())
			return subroutines.StopWithRequeue(requeueWait, err.Error()), nil
		}
		return subroutines.Result{}, err
	}

	if err := s.deploy(ctx, st); err != nil {
		// kcp-operator mints kubeconfigs asynchronously; waiting for one
		// is ordinary progress, not a failure.
		if errors.Is(err, errKubeconfigPending) || errors.Is(err, errServingCertPending) {
			setCondition(mod, ConditionDeployed, metav1.ConditionFalse, "WaitingForKubeconfig", err.Error())
			return subroutines.StopWithRequeue(requeueWait, err.Error()), nil
		}
		setCondition(mod, ConditionDeployed, metav1.ConditionFalse, "DeployFailed", err.Error())
		return subroutines.Result{}, err
	}
	setCondition(mod, ConditionDeployed, metav1.ConditionTrue, "Deployed", "payloads applied")
	return subroutines.OK(), nil
}

// platformMesh fetches the Module's PlatformMesh.
func (s *Subroutine) platformMesh(ctx context.Context, mod *pmdeployerv1alpha1.Module) (*pmdeployerv1alpha1.PlatformMesh, error) {
	pm := &pmdeployerv1alpha1.PlatformMesh{}
	key := ctrlruntimeclient.ObjectKey{Namespace: mod.Namespace, Name: mod.Spec.PlatformMeshRef.Name}
	if err := s.client.Get(ctx, key, pm); err != nil {
		return nil, fmt.Errorf("getting PlatformMesh %q: %w", key.Name, err)
	}
	return pm, nil
}

// topologyReady reports whether the PlatformMesh has finished bringing kcp up.
func topologyReady(pm *pmdeployerv1alpha1.PlatformMesh) (bool, string) {
	cond := meta.FindStatusCondition(pm.Status.Conditions, "Ready")
	if cond == nil {
		return false, fmt.Sprintf("PlatformMesh %q has no Ready condition yet", pm.Name)
	}
	if cond.Status != metav1.ConditionTrue {
		return false, fmt.Sprintf("PlatformMesh %q is not ready: %s", pm.Name, cond.Message)
	}
	return true, ""
}

// dependenciesReady reports whether every Module in spec.dependsOn is ready.
func (s *Subroutine) dependenciesReady(ctx context.Context, mod *pmdeployerv1alpha1.Module) (bool, string) {
	for _, ref := range mod.Spec.DependsOn {
		if ref.Name == mod.Name {
			return false, fmt.Sprintf("module %q depends on itself", mod.Name)
		}
		dep := &pmdeployerv1alpha1.Module{}
		key := ctrlruntimeclient.ObjectKey{Namespace: mod.Namespace, Name: ref.Name}
		if err := s.client.Get(ctx, key, dep); err != nil {
			if apierrors.IsNotFound(err) {
				return false, fmt.Sprintf("dependency %q does not exist", ref.Name)
			}
			return false, fmt.Sprintf("getting dependency %q: %v", ref.Name, err)
		}
		if dep.Spec.PlatformMeshRef.Name != mod.Spec.PlatformMeshRef.Name {
			return false, fmt.Sprintf("dependency %q belongs to another PlatformMesh", ref.Name)
		}
		if mod.Spec.Stage == pmdeployerv1alpha1.StagePreTopology && dep.Spec.Stage == pmdeployerv1alpha1.StagePostTopology {
			return false, fmt.Sprintf("pre-topology module %q cannot depend on post-topology module %q", mod.Name, ref.Name)
		}
		if !meta.IsStatusConditionTrue(dep.Status.Conditions, "Ready") {
			return false, fmt.Sprintf("dependency %q is not ready", ref.Name)
		}
	}
	return true, ""
}

func setCondition(mod *pmdeployerv1alpha1.Module, condType string, status metav1.ConditionStatus, reason, message string) {
	meta.SetStatusCondition(&mod.Status.Conditions, metav1.Condition{
		Type:               condType,
		Status:             status,
		Reason:             reason,
		Message:            message,
		ObservedGeneration: mod.Generation,
	})
}
