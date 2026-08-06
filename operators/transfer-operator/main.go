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

package main

import (
	"flag"
	"os"

	transferconfig "go.platform-mesh.io/transfer-operator/pkg/config"

	ctrl "sigs.k8s.io/controller-runtime"
	ctrlconfig "sigs.k8s.io/controller-runtime/pkg/client/config"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
	mcmanager "sigs.k8s.io/multicluster-runtime/pkg/manager"
	"sigs.k8s.io/multicluster-runtime/providers/multi"
)

func main() {
	ctx := ctrl.SetupSignalHandler()

	opts := &transferconfig.Options{}
	zapOpts := zap.Options{Development: true}

	fs := flag.NewFlagSet(os.Args[0], flag.ExitOnError)
	opts.AddFlags(fs)
	zapOpts.BindFlags(fs)
	ctrlconfig.RegisterFlags(fs)
	if err := fs.Parse(os.Args[1:]); err != nil {
		os.Exit(1)
	}
	ctrl.SetLogger(zap.New(zap.UseFlagOptions(&zapOpts)))
	setupLog := logf.Log.WithName("setup")

	cfg, err := ctrlconfig.GetConfig()
	if err != nil {
		setupLog.Error(err, "unable to load kubeconfig")
		os.Exit(1)
	}

	provider := multi.New(multi.Options{})

	scheme, err := transferconfig.NewScheme()
	if err != nil {
		setupLog.Error(err, "unable to build the scheme")
		os.Exit(1)
	}

	mgr, err := mcmanager.New(cfg, provider, mcmanager.Options{Scheme: scheme})
	if err != nil {
		setupLog.Error(err, "unable to create manager")
		os.Exit(1)
	}

	if err := opts.SetupProviders(ctx, mgr, provider); err != nil {
		setupLog.Error(err, "unable to set up cluster providers")
		os.Exit(1)
	}

	if err := opts.SetupWithManager(ctx, mgr); err != nil {
		setupLog.Error(err, "unable to set up controllers")
		os.Exit(1)
	}

	if err := mgr.Start(ctx); err != nil {
		setupLog.Error(err, "manager exited")
		os.Exit(1)
	}
}
