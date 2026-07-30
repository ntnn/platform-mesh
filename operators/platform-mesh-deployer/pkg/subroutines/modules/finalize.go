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

	pmdeployerv1alpha1 "go.platform-mesh.io/apis/deployer/v1alpha1"
	"go.platform-mesh.io/platform-mesh-deployer/pkg/module"
	"go.platform-mesh.io/platform-mesh-deployer/pkg/sync"
	"go.platform-mesh.io/subroutines"

	"k8s.io/apimachinery/pkg/runtime/schema"
	ctrlruntimeclient "sigs.k8s.io/controller-runtime/pkg/client"
)

// Finalizer keeps a Module around until its objects are gone from every
// cluster it was deployed to. Owner references only reach objects on the
// config plane, so without this the workloads leak.
const Finalizer = "deployer.platform-mesh.io/module-workloads"

func (s *Subroutine) Finalizers(ctrlruntimeclient.Object) []string {
	return []string{Finalizer}
}

// Finalize removes every object the module applied, on every cluster the
// PlatformMesh has engaged. It deliberately looks wider than the module's last
// known placement: a cluster that dropped out of the fan-out earlier may still
// be holding objects.
func (s *Subroutine) Finalize(ctx context.Context, obj ctrlruntimeclient.Object) (subroutines.Result, error) {
	mod := obj.(*pmdeployerv1alpha1.Module)

	kinds := appliedKinds(mod)
	for _, c := range s.registry.AllClustersFor(mod.Spec.PlatformMeshRef.Name) {
		if err := sync.Prune(ctx, c.Cluster.GetClient(), kinds,
			module.ModuleSelector(mod, c.ClusterID), nil); err != nil {
			return subroutines.Result{}, err
		}
	}
	return subroutines.OK(), nil
}

// appliedKinds are the kinds a module's objects can have. The payload is only
// known once resolved, which a deleted module may no longer be able to do, so
// the kinds recorded on the status are used and the always-applied ConfigMap
// and Secret are added.
func appliedKinds(mod *pmdeployerv1alpha1.Module) []schema.GroupVersionKind {
	seen := map[schema.GroupVersionKind]struct{}{
		{Version: "v1", Kind: "ConfigMap"}: {},
		{Version: "v1", Kind: "Secret"}:    {},
	}
	for _, gvk := range mod.Status.AppliedKinds {
		seen[schema.GroupVersionKind{Group: gvk.Group, Version: gvk.Version, Kind: gvk.Kind}] = struct{}{}
	}
	return kindsOf(seen)
}
