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

package config

import (
	"github.com/spf13/pflag"
)

// Default per-component labels selecting kubeconfig secrets.
const (
	DefaultRootShardLabel        = "deployer.platform-mesh.io/rootshard"
	DefaultShardLabel            = "deployer.platform-mesh.io/shard"
	DefaultFrontProxyLabel       = "deployer.platform-mesh.io/frontproxy"
	DefaultCacheServerLabel      = "deployer.platform-mesh.io/cacheserver"
	DefaultVirtualWorkspaceLabel = "deployer.platform-mesh.io/virtualworkspace"
)

type ProviderConfig struct {
	Namespace           string
	KubeconfigSecretKey string

	RootShardLabel        string
	ShardLabel            string
	FrontProxyLabel       string
	CacheServerLabel      string
	VirtualWorkspaceLabel string
}

type OperatorConfig struct {
	Provider ProviderConfig
}

func NewOperatorConfig() OperatorConfig {
	return OperatorConfig{
		Provider: ProviderConfig{
			Namespace:             "platform-mesh-system",
			KubeconfigSecretKey:   "kubeconfig",
			RootShardLabel:        DefaultRootShardLabel,
			ShardLabel:            DefaultShardLabel,
			FrontProxyLabel:       DefaultFrontProxyLabel,
			CacheServerLabel:      DefaultCacheServerLabel,
			VirtualWorkspaceLabel: DefaultVirtualWorkspaceLabel,
		},
	}
}

func (c *OperatorConfig) AddFlags(fs *pflag.FlagSet) {
	fs.StringVar(&c.Provider.Namespace, "provider-namespace", c.Provider.Namespace, "Namespace to watch for kubeconfig secrets")
	fs.StringVar(&c.Provider.KubeconfigSecretKey, "provider-kubeconfig-secret-key", c.Provider.KubeconfigSecretKey, "Key within the secret containing the kubeconfig")
	fs.StringVar(&c.Provider.RootShardLabel, "provider-rootshard-label", c.Provider.RootShardLabel, "Label selecting root shard kubeconfig secrets")
	fs.StringVar(&c.Provider.ShardLabel, "provider-shard-label", c.Provider.ShardLabel, "Label selecting shard kubeconfig secrets")
	fs.StringVar(&c.Provider.FrontProxyLabel, "provider-frontproxy-label", c.Provider.FrontProxyLabel, "Label selecting front proxy kubeconfig secrets")
	fs.StringVar(&c.Provider.CacheServerLabel, "provider-cacheserver-label", c.Provider.CacheServerLabel, "Label selecting cache server kubeconfig secrets")
	fs.StringVar(&c.Provider.VirtualWorkspaceLabel, "provider-virtualworkspace-label", c.Provider.VirtualWorkspaceLabel, "Label selecting virtual workspace kubeconfig secrets")
}
