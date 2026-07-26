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
	"go.platform-mesh.io/platform-mesh-deployer/pkg/clusters"
	"go.platform-mesh.io/platform-mesh-deployer/pkg/components"
	"go.platform-mesh.io/platform-mesh-deployer/pkg/controller"
	"go.platform-mesh.io/platform-mesh-deployer/pkg/ocm"

	mcmanager "sigs.k8s.io/multicluster-runtime/pkg/manager"
	"sigs.k8s.io/multicluster-runtime/pkg/multicluster"
	"sigs.k8s.io/multicluster-runtime/providers/multi"
)

// Config contains the necessary configuration to setup the deployer controllers with a manager.
type Config struct {
	Log      *logger.Logger
	Resolver ocm.Resolver

	RootShardProvider   multicluster.Provider
	ShardProviders      map[string]multicluster.Provider // keyed by ShardGroup.Name
	FrontProxyProvider  multicluster.Provider
	CacheServerProvider multicluster.Provider
}

// Setup registers the deployer controllers on the manager.
func Setup(mgr mcmanager.Manager, _ Config) error {
	registry := clusters.NewRegistry()
	if err := mgr.Add(registry); err != nil {
		return fmt.Errorf("adding cluster registry: %w", err)
	}
	if err := controller.NewPlatformMeshReconciler(mgr, registry).SetupWithManager(mgr); err != nil {
		return fmt.Errorf("setting up PlatformMesh reconciler: %w", err)
	}
	return nil
}

// AddProviders adds the configured [multicluster.Provider] to the [multi.Provider].
func AddProviders(mp *multi.Provider, mgr mcmanager.Manager, cfg Config) error {
	entries := map[string]multicluster.Provider{
		components.RootShard:   cfg.RootShardProvider,
		components.FrontProxy:  cfg.FrontProxyProvider,
		components.CacheServer: cfg.CacheServerProvider,
	}
	for group, provider := range cfg.ShardProviders {
		entries[components.Shard(group)] = provider
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
