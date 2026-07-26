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

package v1alpha1

import (
	"fmt"
	"strings"

	"sigs.k8s.io/yaml"
)

// Parse decodes a module descriptor from YAML, rejecting unknown fields and validating the apiVersion/kind and the placement/target enums.
func Parse(data []byte) (*ModuleDescriptor, error) {
	var md ModuleDescriptor
	if err := yaml.UnmarshalStrict(data, &md); err != nil {
		return nil, fmt.Errorf("decoding module descriptor: %w", err)
	}
	if err := md.validate(); err != nil {
		return nil, err
	}
	return &md, nil
}

func (md *ModuleDescriptor) validate() error {
	if md.APIVersion != APIVersion {
		return fmt.Errorf("unsupported apiVersion %q, want %q", md.APIVersion, APIVersion)
	}
	if md.Kind != Kind {
		return fmt.Errorf("unsupported kind %q, want %q", md.Kind, Kind)
	}
	for i := range md.Components {
		if err := md.Components[i].validate(); err != nil {
			return fmt.Errorf("component %q: %w", md.Components[i].Name, err)
		}
	}
	seen := make(map[string]struct{}, len(md.Workspaces))
	for i := range md.Workspaces {
		name := md.Workspaces[i].Name
		if strings.Contains(name, ":") {
			return fmt.Errorf("workspace %q: name must not be nested (contains ':')", name)
		}
		if _, ok := seen[name]; ok {
			return fmt.Errorf("duplicate workspace %q", name)
		}
		seen[name] = struct{}{}
	}
	return nil
}

func (c *Component) validate() error {
	switch c.Placement {
	case PlacementGlobal, PlacementPerShard, PlacementPerFrontProxy:
	default:
		return fmt.Errorf("invalid placement %q", c.Placement)
	}
	for i := range c.Kubeconfigs {
		switch c.Kubeconfigs[i].Target {
		case KubeconfigTargetVirtualWorkspace, KubeconfigTargetFrontProxy, KubeconfigTargetShard:
		default:
			return fmt.Errorf("kubeconfig %q: invalid target %q", c.Kubeconfigs[i].Name, c.Kubeconfigs[i].Target)
		}
	}
	return nil
}
