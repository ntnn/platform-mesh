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
	"sort"
	"time"

	pmdeployerv1alpha1 "go.platform-mesh.io/apis/deployer/v1alpha1"
	"go.platform-mesh.io/platform-mesh-deployer/pkg/celtemplate"
	"go.platform-mesh.io/platform-mesh-deployer/pkg/module"
	"go.platform-mesh.io/platform-mesh-deployer/pkg/sync"

	"k8s.io/apimachinery/pkg/runtime/schema"
)

// requeueWait is how long a gated Module waits before re-checking.
const requeueWait = 15 * time.Second

// deploy renders every instance and applies it to its cluster, then prunes the
// objects of instances that no longer exist.
func (s *Subroutine) deploy(ctx context.Context, st *state) error {
	mod := st.resolved.Module

	// Objects to keep per cluster, so a component that lost a cluster or a
	// cluster that lost a component is cleaned up.
	keep := map[string]map[sync.ObjectKey]struct{}{}
	kinds := map[string]map[schema.GroupVersionKind]struct{}{}
	status := map[string]*pmdeployerv1alpha1.ModuleComponentStatus{}

	for _, inst := range st.instances {
		// The kubeconfigs must exist before the payload references them,
		// otherwise its pods block mounting a missing secret.
		if err := s.ensureKubeconfigs(ctx, st, inst); err != nil {
			return err
		}

		celCtx, err := st.resolved.Context(inst)
		if err != nil {
			return err
		}

		// A mapped component is fronted by the front proxy, which needs a
		// certificate it trusts before the topology can route to it.
		var mapping *pmdeployerv1alpha1.ResolvedMapping
		if inst.Component.Mapping != nil {
			if err := s.ensureServingCert(ctx, st, inst, celCtx); err != nil {
				return err
			}
			mapping, err = resolveMapping(inst, celCtx)
			if err != nil {
				return err
			}
		}

		objs, err := st.resolved.Render(ctx, inst)
		if err != nil {
			return err
		}

		cl := inst.Cluster.Cluster.GetClient()
		if err := sync.EnsureNamespace(ctx, cl, inst.Component.Namespace); err != nil {
			return err
		}
		for _, obj := range objs {
			sync.Strip(obj)
			if err := sync.Apply(ctx, cl, obj); err != nil {
				return err
			}
			id := inst.Cluster.ClusterID
			if keep[id] == nil {
				keep[id] = map[sync.ObjectKey]struct{}{}
				kinds[id] = map[schema.GroupVersionKind]struct{}{}
			}
			keep[id][sync.KeyOf(obj)] = struct{}{}
			kinds[id][obj.GroupVersionKind()] = struct{}{}
		}

		componentStatus(status, inst, module.ConfigMapName(mod.Name, inst.Component.Name), mapping)
	}

	if err := s.prune(ctx, st, keep, kinds); err != nil {
		return err
	}

	mod.Status.Components = sortedStatus(status)
	return nil
}

// prune deletes objects the module owns on a cluster that this reconcile did
// not apply.
func (s *Subroutine) prune(ctx context.Context, st *state, keep map[string]map[sync.ObjectKey]struct{}, kinds map[string]map[schema.GroupVersionKind]struct{}) error {
	mod := st.resolved.Module
	for _, c := range s.registry.AllClustersFor(mod.Spec.PlatformMeshRef.Name) {
		gvks := kindsOf(kinds[c.ClusterID])
		if len(gvks) == 0 {
			// Nothing was applied here this round; still prune what a
			// previous round left behind, using the kinds the module
			// declares it can produce.
			gvks = previousKinds(mod)
		}
		if len(gvks) == 0 {
			continue
		}
		if err := sync.Prune(ctx, c.Cluster.GetClient(), gvks,
			module.ModuleSelector(mod, c.ClusterID), keep[c.ClusterID]); err != nil {
			return fmt.Errorf("pruning on cluster %q: %w", c.ClusterID, err)
		}
	}
	return nil
}

// previousKinds are the kinds pruned on a cluster the module no longer places
// anything on. The generated ConfigMap is always applied, so it is enough to
// detect and clean up a stale instance.
func previousKinds(*pmdeployerv1alpha1.Module) []schema.GroupVersionKind {
	return []schema.GroupVersionKind{{Version: "v1", Kind: "ConfigMap"}}
}

func kindsOf(set map[schema.GroupVersionKind]struct{}) []schema.GroupVersionKind {
	out := make([]schema.GroupVersionKind, 0, len(set))
	for gvk := range set {
		out = append(out, gvk)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].String() < out[j].String() })
	return out
}

// resolveMapping interpolates a component's mapping into the concrete path and
// backend URL the front proxy routes with.
func resolveMapping(inst module.Instance, celCtx celtemplate.Context) (*pmdeployerv1alpha1.ResolvedMapping, error) {
	m := inst.Component.Mapping

	service, err := celtemplate.Interpolate(m.Service, celCtx)
	if err != nil {
		return nil, fmt.Errorf("component %q: mapping service: %w", inst.Component.Name, err)
	}
	name, ok := service.(string)
	if !ok {
		return nil, fmt.Errorf("component %q: mapping service evaluated to %T, want string", inst.Component.Name, service)
	}

	path, err := celtemplate.Interpolate(m.Path, celCtx)
	if err != nil {
		return nil, fmt.Errorf("component %q: mapping path: %w", inst.Component.Name, err)
	}
	uri, ok := path.(string)
	if !ok {
		return nil, fmt.Errorf("component %q: mapping path evaluated to %T, want string", inst.Component.Name, path)
	}

	return &pmdeployerv1alpha1.ResolvedMapping{
		Path: uri,
		Backend: fmt.Sprintf("https://%s.%s.svc:%d",
			name, inst.Component.Namespace, m.Port),
	}, nil
}

// componentStatus records one applied instance.
func componentStatus(status map[string]*pmdeployerv1alpha1.ModuleComponentStatus, inst module.Instance, configMap string, mapping *pmdeployerv1alpha1.ResolvedMapping) {
	cs, ok := status[inst.Component.Name]
	if !ok {
		cs = &pmdeployerv1alpha1.ModuleComponentStatus{
			Name:      inst.Component.Name,
			Placement: inst.Component.Placement,
		}
		status[inst.Component.Name] = cs
	}
	cs.Instances = append(cs.Instances, pmdeployerv1alpha1.ModuleInstanceStatus{
		Cluster:   inst.Cluster.ClusterID,
		Namespace: inst.Component.Namespace,
		ConfigMap: configMap,
		Mapping:   mapping,
		Ready:     true,
	})
}

func sortedStatus(status map[string]*pmdeployerv1alpha1.ModuleComponentStatus) []pmdeployerv1alpha1.ModuleComponentStatus {
	out := make([]pmdeployerv1alpha1.ModuleComponentStatus, 0, len(status))
	for _, cs := range status {
		sort.Slice(cs.Instances, func(i, j int) bool { return cs.Instances[i].Cluster < cs.Instances[j].Cluster })
		out = append(out, *cs)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out
}
