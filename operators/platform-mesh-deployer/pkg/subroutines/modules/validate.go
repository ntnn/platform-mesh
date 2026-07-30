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
	"strings"

	pmdeployerv1alpha1 "go.platform-mesh.io/apis/deployer/v1alpha1"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	ctrlruntimeclient "sigs.k8s.io/controller-runtime/pkg/client"
)

// validate rejects a Module whose references cannot be satisfied. These are
// cross-field rules the CRD cannot express, so they are checked before
// anything is created.
func validate(mod *pmdeployerv1alpha1.Module) error {
	declared := make(map[string]struct{}, len(mod.Spec.Workspaces))
	for _, ws := range mod.Spec.Workspaces {
		declared[ws.Name] = struct{}{}
	}

	for _, kc := range mod.Spec.Kubeconfigs {
		if _, ok := declared[kc.Workspace]; !ok {
			// Otherwise the kubeconfig is scoped to a workspace nobody
			// provisions, and the module gets credentials for nothing.
			return fmt.Errorf("kubeconfig %q references workspace %q, which the module does not declare",
				kc.Name, kc.Workspace)
		}
	}

	known := make(map[string]struct{}, len(mod.Spec.Kubeconfigs))
	for _, kc := range mod.Spec.Kubeconfigs {
		known[kc.Name] = struct{}{}
	}
	for _, component := range mod.Spec.Components {
		for _, name := range component.Kubeconfigs {
			if _, ok := known[name]; !ok {
				return fmt.Errorf("component %q references kubeconfig %q, which the module does not declare",
					component.Name, name)
			}
		}
	}

	seen := make(map[string]struct{}, len(mod.Spec.DependsOn))
	for _, dep := range mod.Spec.DependsOn {
		if _, dup := seen[dep.Name]; dup {
			return fmt.Errorf("module %q depends on %q more than once", mod.Name, dep.Name)
		}
		seen[dep.Name] = struct{}{}
	}
	return nil
}

// detectCycle walks the dependency graph of the module's PlatformMesh and
// reports the cycle the module takes part in, if any. Without this two modules
// depending on each other would requeue forever instead of failing.
func (s *Subroutine) detectCycle(ctx context.Context, mod *pmdeployerv1alpha1.Module) error {
	list := &pmdeployerv1alpha1.ModuleList{}
	if err := s.client.List(ctx, list, ctrlruntimeclient.InNamespace(mod.Namespace)); err != nil {
		if apierrors.IsNotFound(err) {
			return nil
		}
		return fmt.Errorf("listing modules: %w", err)
	}

	deps := map[string][]string{}
	for i := range list.Items {
		m := &list.Items[i]
		if m.Spec.PlatformMeshRef.Name != mod.Spec.PlatformMeshRef.Name {
			continue
		}
		names := make([]string, 0, len(m.Spec.DependsOn))
		for _, dep := range m.Spec.DependsOn {
			names = append(names, dep.Name)
		}
		sort.Strings(names)
		deps[m.Name] = names
	}

	// Depth first from this module; a name already on the stack closes a cycle.
	stack := map[string]struct{}{}
	done := map[string]struct{}{}
	var walk func(name string, path []string) error
	walk = func(name string, path []string) error {
		if _, ok := stack[name]; ok {
			return fmt.Errorf("dependency cycle: %s", cyclePath(path, name))
		}
		if _, ok := done[name]; ok {
			return nil
		}
		stack[name] = struct{}{}
		for _, dep := range deps[name] {
			if err := walk(dep, append(path, name)); err != nil {
				return err
			}
		}
		delete(stack, name)
		done[name] = struct{}{}
		return nil
	}
	return walk(mod.Name, nil)
}

// cyclePath renders the cycle, starting where it closes.
func cyclePath(path []string, repeated string) string {
	start := 0
	for i, name := range path {
		if name == repeated {
			start = i
			break
		}
	}
	return strings.Join(append(append([]string{}, path[start:]...), repeated), " -> ")
}
