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

// Package pretopology blocks the kcp topology until the modules that have to
// exist before it are ready, such as the etcd the shards will store into.
package pretopology

import (
	"context"
	"fmt"
	"sort"
	"time"

	pmdeployv1alpha1 "go.platform-mesh.io/apis/deploy/v1alpha1"
	"go.platform-mesh.io/subroutines"

	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	ctrlruntimeclient "sigs.k8s.io/controller-runtime/pkg/client"
)

const Name = "PreTopologySubroutine"

// ConditionReady reports whether the pre-topology modules are all ready.
const ConditionReady = "PreTopologyModulesReady"

const requeueWait = 15 * time.Second

// Subroutine gates the topology on the PlatformMesh's pre-topology modules.
type Subroutine struct {
	client ctrlruntimeclient.Client
}

func New(client ctrlruntimeclient.Client) *Subroutine {
	return &Subroutine{client: client}
}

func (s *Subroutine) GetName() string { return Name }

func (s *Subroutine) Process(ctx context.Context, obj ctrlruntimeclient.Object) (subroutines.Result, error) {
	pm := obj.(*pmdeployv1alpha1.PlatformMesh)

	pending, err := s.pending(ctx, pm)
	if err != nil {
		return subroutines.Result{}, err
	}
	if len(pending) > 0 {
		message := fmt.Sprintf("waiting for pre-topology modules: %v", pending)
		meta.SetStatusCondition(&pm.Status.Conditions, metav1.Condition{
			Type:               ConditionReady,
			Status:             metav1.ConditionFalse,
			Reason:             "WaitingForModules",
			Message:            message,
			ObservedGeneration: pm.Generation,
		})
		return subroutines.StopWithRequeue(requeueWait, message), nil
	}

	meta.SetStatusCondition(&pm.Status.Conditions, metav1.Condition{
		Type:               ConditionReady,
		Status:             metav1.ConditionTrue,
		Reason:             "Ready",
		Message:            "pre-topology modules are ready",
		ObservedGeneration: pm.Generation,
	})
	return subroutines.OK(), nil
}

// pending lists the PlatformMesh's pre-topology modules that are not ready.
// A module that has not been reconciled yet counts as pending, so the topology
// never races ahead of one that was only just created.
func (s *Subroutine) pending(ctx context.Context, pm *pmdeployv1alpha1.PlatformMesh) ([]string, error) {
	list := &pmdeployv1alpha1.ModuleList{}
	if err := s.client.List(ctx, list, ctrlruntimeclient.InNamespace(pm.Namespace)); err != nil {
		return nil, fmt.Errorf("listing modules: %w", err)
	}

	var pending []string
	for i := range list.Items {
		mod := &list.Items[i]
		if mod.Spec.PlatformMeshRef.Name != pm.Name {
			continue
		}
		if mod.Spec.Stage != pmdeployv1alpha1.StagePreTopology {
			continue
		}
		if !meta.IsStatusConditionTrue(mod.Status.Conditions, "Ready") {
			pending = append(pending, mod.Name)
		}
	}
	sort.Strings(pending)
	return pending, nil
}
