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

// Package deployer wires the deployer controllers into a multicluster manager.
package deployer

import (
	"fmt"

	"go.platform-mesh.io/golang-commons/logger"
	"go.platform-mesh.io/platform-mesh-deployer/pkg/controller"
	"go.platform-mesh.io/platform-mesh-deployer/pkg/ocm"

	mcmanager "sigs.k8s.io/multicluster-runtime/pkg/manager"
	"sigs.k8s.io/multicluster-runtime/pkg/multicluster"
	"sigs.k8s.io/multicluster-runtime/providers/multi"
)

// The prefixes passed to the [multi.Provider] for each [multicluster.Provider] of each component.
const (
	ComponentRootShard        = "rootshard"
	ComponentFrontProxy       = "frontproxy"
	ComponentCacheServer      = "cacheserver"
	ComponentVirtualWorkspace = "virtualworkspace"
	ShardComponentPrefix      = "shards-"
)

// ShardComponent returns the multi-provider prefix for a shard group.
func ShardComponent(group string) string { return ShardComponentPrefix + group }

// Config contains the necessary configuration to setup the deployer controllers with a manager.
type Config struct {
	Log      *logger.Logger
	Resolver ocm.Resolver

	RootShardProvider        multicluster.Provider
	ShardProviders           map[string]multicluster.Provider // keyed by ShardGroup.Name
	FrontProxyProvider       multicluster.Provider
	CacheServerProvider      multicluster.Provider
	VirtualWorkspaceProvider multicluster.Provider
}

// Setup registers the deployer controllers on the manager.
func Setup(mgr mcmanager.Manager, _ Config) error {
	if err := controller.NewPlatformMeshReconciler(mgr).SetupWithManager(mgr); err != nil {
		return fmt.Errorf("setting up PlatformMesh reconciler: %w", err)
	}
	return nil
}

// AddProviders adds the configured [multicluster.Provider] to the [multi.Provider].
func AddProviders(mp *multi.Provider, mgr mcmanager.Manager, cfg Config) error {
	entries := map[string]multicluster.Provider{
		ComponentRootShard:        cfg.RootShardProvider,
		ComponentFrontProxy:       cfg.FrontProxyProvider,
		ComponentCacheServer:      cfg.CacheServerProvider,
		ComponentVirtualWorkspace: cfg.VirtualWorkspaceProvider,
	}
	for group, provider := range cfg.ShardProviders {
		entries[ShardComponent(group)] = provider
	}

	for name, provider := range entries {
		if provider == nil {
			continue
		}
		if err := mp.AddProvider(name, provider); err != nil {
			return fmt.Errorf("adding %s provider: %w", name, err)
		}
	}
	return nil
}
