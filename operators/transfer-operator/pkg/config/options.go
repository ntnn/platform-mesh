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

// Package config configures transport-operator.
package config

import (
	"context"
	"flag"

	mcmanager "sigs.k8s.io/multicluster-runtime/pkg/manager"
	"sigs.k8s.io/multicluster-runtime/providers/multi"
)

type Options struct {
	// APIExportEndpointSlice names the APIExportEndpointSlice serving the transfer APIExport.
	// Empty runs against a single control plane.
	APIExportEndpointSlice string

	// AdminKubeconfig reaches consumer workspaces directly, bypassing the APIExport virtual workspace, and should point at the front-proxy.
	// Empty reuses the config the manager was built with.
	AdminKubeconfig string
}

// AddFlags binds into a FlagSet the caller owns.
func (o *Options) AddFlags(fs *flag.FlagSet) {
	fs.StringVar(&o.APIExportEndpointSlice, "apiexport-endpointslice", o.APIExportEndpointSlice,
		"APIExportEndpointSlice serving the transfer APIExport. Empty runs against a single control plane.")
	fs.StringVar(&o.AdminKubeconfig, "admin-kubeconfig", o.AdminKubeconfig,
		"Kubeconfig reaching consumer workspaces directly, pointed at the kcp front-proxy. Empty reuses the manager's config.")
}

// Validate validates the Options.
func (o *Options) Validate() error {
	return nil
}

// SetupProviders sets up the providers.
func (o *Options) SetupProviders(_ context.Context, _ mcmanager.Manager, _ *multi.Provider) error {
	return o.Validate()
}

// SetupWithManager registers the controllers.
func (o *Options) SetupWithManager(_ context.Context, _ mcmanager.Manager) error {
	return o.Validate()
}
