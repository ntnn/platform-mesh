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
	"slices"

	"go.platform-mesh.io/golang-commons/logger"
	"go.platform-mesh.io/platform-mesh-deployer/pkg/clusters"
	"go.platform-mesh.io/platform-mesh-deployer/pkg/components"
	"go.platform-mesh.io/platform-mesh-deployer/pkg/controller"
	"go.platform-mesh.io/platform-mesh-deployer/pkg/kcp"
	"go.platform-mesh.io/platform-mesh-deployer/pkg/ocm"

	mcmanager "sigs.k8s.io/multicluster-runtime/pkg/manager"
	"sigs.k8s.io/multicluster-runtime/pkg/multicluster"
	"sigs.k8s.io/multicluster-runtime/providers/multi"
)

// Controllers, enabled independently like kcp-operator config/workload.
const (
	// ControllerConfig creates the admin CRs on the config plane.
	ControllerConfig = "config"
	// ControllerCopy copies compiled CRs to their workload cluster.
	ControllerCopy = "copy"
	// ControllerModule deploys Modules onto the engaged clusters.
	ControllerModule = "module"
	// ControllerProvisioner performs the kcp side of a PlatformMesh and its
	// modules. It is the only controller that writes inside kcp.
	ControllerProvisioner = "provisioner"
)

// Config contains the necessary configuration to setup the deployer controllers with a manager.
type Config struct {
	Log      *logger.Logger
	Resolver ocm.Resolver

	// EnabledControllers selects which controllers run (ControllerConfig, ControllerCopy, ControllerModule).
	EnabledControllers []string

	// KcpDial overrides how the kcp front proxy is reached. The e2e runs
	// the deployer outside the cluster, where the external hostname does
	// not resolve; a normal deployment leaves this nil.
	KcpDial kcp.DialFunc

	RootShardProvider   multicluster.Provider
	ShardProviders      map[string]multicluster.Provider // keyed by ShardGroup.Name
	FrontProxyProvider  multicluster.Provider
	CacheServerProvider multicluster.Provider
}

func (c Config) controllerEnabled(name string) bool {
	return slices.Contains(c.EnabledControllers, name)
}

// Setup registers the deployer controllers on the manager.
func Setup(mgr mcmanager.Manager, cfg Config) error {
	registry := clusters.NewRegistry()
	if err := mgr.Add(registry); err != nil {
		return fmt.Errorf("adding cluster registry: %w", err)
	}

	local := mgr.GetLocalManager()
	var access *kcp.Access
	if cfg.controllerEnabled(ControllerProvisioner) {
		access = kcp.New(local.GetClient(), registry, local.GetScheme(), cfg.KcpDial)
	}

	if cfg.controllerEnabled(ControllerConfig) {
		if err := controller.NewPlatformMeshReconciler(mgr, registry, access).SetupWithManager(mgr); err != nil {
			return fmt.Errorf("setting up config controller: %w", err)
		}
		for _, r := range controller.NewTemplateReconcilers(mgr) {
			if err := r.SetupWithManager(mgr); err != nil {
				return fmt.Errorf("setting up template controller: %w", err)
			}
		}
	}
	if cfg.controllerEnabled(ControllerCopy) {
		if err := controller.NewCopyReconciler(mgr, registry).SetupWithManager(mgr); err != nil {
			return fmt.Errorf("setting up copy controller: %w", err)
		}
	}
	if cfg.controllerEnabled(ControllerModule) {
		if err := controller.NewModuleReconciler(mgr, registry, cfg.Resolver).SetupWithManager(mgr); err != nil {
			return fmt.Errorf("setting up module controller: %w", err)
		}
	}
	if cfg.controllerEnabled(ControllerProvisioner) {
		if err := controller.NewModuleSetupReconciler(mgr, access, cfg.Resolver).SetupWithManager(mgr); err != nil {
			return fmt.Errorf("setting up provisioner controller: %w", err)
		}
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
