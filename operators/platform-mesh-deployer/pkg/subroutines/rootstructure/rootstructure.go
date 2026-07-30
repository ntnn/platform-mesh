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

// Package rootstructure provisions the kcp workspaces a PlatformMesh needs
// before anything can be installed into it.
package rootstructure

import (
	"context"
	"errors"
	"time"

	pmdeployerv1alpha1 "go.platform-mesh.io/apis/deployer/v1alpha1"
	"go.platform-mesh.io/platform-mesh-deployer/pkg/kcp"
	"go.platform-mesh.io/platform-mesh-deployer/pkg/module"
	"go.platform-mesh.io/subroutines"

	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	ctrlruntimeclient "sigs.k8s.io/controller-runtime/pkg/client"
)

const Name = "RootStructureSubroutine"

// ConditionProvisioned reports the state of the kcp root structure.
const ConditionProvisioned = "RootStructureProvisioned"

// requeueWait is how long to wait for kcp to catch up.
const requeueWait = 10 * time.Second

// roots are the workspaces every PlatformMesh gets. Modules live below
// root:modules; organisations below root:orgs.
var roots = []string{module.WorkspaceBase, "root:orgs"}

// Subroutine creates the kcp root structure once the topology serves.
type Subroutine struct {
	access *kcp.Access
}

func New(access *kcp.Access) *Subroutine {
	return &Subroutine{access: access}
}

func (s *Subroutine) GetName() string { return Name }

func (s *Subroutine) Process(ctx context.Context, obj ctrlruntimeclient.Object) (subroutines.Result, error) {
	pm := obj.(*pmdeployerv1alpha1.PlatformMesh)

	// kcp only exists once the topology is up, and the topology subroutine
	// runs before this one in the same chain.
	if !meta.IsStatusConditionTrue(pm.Status.Conditions, "TopologySubroutine") {
		setCondition(pm, metav1.ConditionFalse, "WaitingForTopology", "kcp is not serving yet")
		return subroutines.StopWithRequeue(requeueWait, "waiting for the topology"), nil
	}

	cfg, err := s.access.Config(ctx, pm)
	if err != nil {
		if errors.Is(err, kcp.ErrPending) {
			setCondition(pm, metav1.ConditionFalse, "WaitingForKubeconfig", err.Error())
			return subroutines.StopWithRequeue(requeueWait, err.Error()), nil
		}
		return subroutines.Result{}, err
	}

	for _, path := range roots {
		if _, err := s.access.EnsurePath(ctx, cfg, path); err != nil {
			if errors.Is(err, kcp.ErrWorkspacePending) {
				setCondition(pm, metav1.ConditionFalse, "WaitingForWorkspace", err.Error())
				return subroutines.StopWithRequeue(requeueWait, err.Error()), nil
			}
			return subroutines.Result{}, err
		}
	}

	setCondition(pm, metav1.ConditionTrue, "Provisioned", "kcp root structure exists")
	return subroutines.OK(), nil
}

func setCondition(pm *pmdeployerv1alpha1.PlatformMesh, status metav1.ConditionStatus, reason, message string) {
	meta.SetStatusCondition(&pm.Status.Conditions, metav1.Condition{
		Type:               ConditionProvisioned,
		Status:             status,
		Reason:             reason,
		Message:            message,
		ObservedGeneration: pm.Generation,
	})
}
