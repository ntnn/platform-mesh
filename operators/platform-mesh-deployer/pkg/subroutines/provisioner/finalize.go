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

package provisioner

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"

	pmdeployerv1alpha1 "go.platform-mesh.io/apis/deployer/v1alpha1"
	"go.platform-mesh.io/platform-mesh-deployer/pkg/kcp"
	"go.platform-mesh.io/subroutines"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	ctrlruntimeclient "sigs.k8s.io/controller-runtime/pkg/client"
)

// Finalizer keeps a ModuleSetup until its kcp workspaces are gone. Workspaces
// live inside kcp and have no owner on the config plane, so nothing else would
// ever remove them.
const Finalizer = "deployer.platform-mesh.io/module-workspaces"

func (s *Subroutine) Finalizers(ctrlruntimeclient.Object) []string {
	return []string{Finalizer}
}

// Finalize deletes the module's workspaces, deepest first so a parent is never
// removed while a child still exists.
func (s *Subroutine) Finalize(ctx context.Context, obj ctrlruntimeclient.Object) (subroutines.Result, error) {
	setup := obj.(*pmdeployerv1alpha1.ModuleSetup)

	pm := &pmdeployerv1alpha1.PlatformMesh{}
	key := ctrlruntimeclient.ObjectKey{Namespace: setup.Namespace, Name: setup.Spec.PlatformMeshRef.Name}
	if err := s.client.Get(ctx, key, pm); err != nil {
		// The whole installation is going away, so there is nothing left
		// to clean up inside it.
		if apierrors.IsNotFound(err) {
			return subroutines.OK(), nil
		}
		return subroutines.Result{}, fmt.Errorf("getting PlatformMesh %q: %w", key.Name, err)
	}

	cfg, err := s.access.Config(ctx, pm)
	if err != nil {
		if errors.Is(err, kcp.ErrPending) {
			// kcp is not reachable; without it the workspaces cannot be
			// removed, and they are gone anyway once kcp is.
			return subroutines.OK(), nil
		}
		return subroutines.Result{}, err
	}

	for _, path := range deepestFirst(setup.Spec.Workspaces) {
		if err := kcp.DeleteWorkspace(ctx, s.access, cfg, path); err != nil {
			if errors.Is(err, kcp.ErrWorkspacePending) {
				return subroutines.StopWithRequeue(requeueWait, err.Error()), nil
			}
			return subroutines.Result{}, err
		}
	}
	return subroutines.OK(), nil
}

// deepestFirst orders workspace paths so children are deleted before parents.
func deepestFirst(workspaces []pmdeployerv1alpha1.ModuleSetupWorkspace) []string {
	paths := make([]string, 0, len(workspaces))
	for _, ws := range workspaces {
		paths = append(paths, ws.Path)
	}
	sort.Slice(paths, func(i, j int) bool {
		di, dj := strings.Count(paths[i], ":"), strings.Count(paths[j], ":")
		if di != dj {
			return di > dj
		}
		return paths[i] < paths[j]
	})
	return paths
}
