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

package e2e

import (
	"strconv"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	pmdeployerv1alpha1 "go.platform-mesh.io/apis/deployer/v1alpha1"
	"go.platform-mesh.io/platform-mesh-deployer/test/e2e/suite"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	ctrlruntimeclient "sigs.k8s.io/controller-runtime/pkg/client"

	deployv1alpha1 "github.com/kcp-dev/kcp-operator/sdk/apis/deploy/v1alpha1"
	operatorv1alpha1 "github.com/kcp-dev/kcp-operator/sdk/apis/operator/v1alpha1"
)

// TestDistributedClusters deploys each component onto its own workload cluster.
func TestDistributedClusters(t *testing.T) {
	env := suite.Start(t, 4)
	rs, sh, fp, cs := env.Workloads[0], env.Workloads[1], env.Workloads[2], env.Workloads[3]

	env.EngageWorkload(t, "customer-a", rs, "rootshard")
	env.EngageWorkload(t, "customer-a", sh, "shards-default")
	env.EngageWorkload(t, "customer-a", fp, "frontproxy")
	env.EngageWorkload(t, "customer-a", cs, "cacheserver")
	env.CopyEtcdClientCert(t, rs)
	env.CopyEtcdClientCert(t, sh)

	pm := distributedPlatformMesh(env.EtcdEndpoint())
	require.NoError(t, env.Config.Client.Create(t.Context(), pm))

	cases := []struct {
		kind    string
		name    string
		cluster *suite.Cluster
	}{
		{"CompiledRootShard", "root-" + rs.NodeIP, rs},
		{"CompiledShard", "default-" + sh.NodeIP, sh},
		{"CompiledFrontProxy", "fp-" + fp.NodeIP, fp},
		{"CompiledCacheServer", "global-" + cs.NodeIP, cs},
	}
	for _, c := range cases {
		require.Eventuallyf(t, func() bool {
			return compiledExists(t, env.Config.Client, c.kind, c.name)
		}, 15*time.Minute, 5*time.Second, "config kcp-operator did not compile %s %q", c.kind, c.name)

		require.Eventuallyf(t, func() bool {
			return compiledExists(t, c.cluster.Client, c.kind, c.name)
		}, 5*time.Minute, 5*time.Second, "deployer did not copy %s %q to its workload cluster", c.kind, c.name)
	}
}

func compiledExists(t *testing.T, cl ctrlruntimeclient.Client, kind, name string) bool {
	t.Helper()
	obj := &unstructured.Unstructured{}
	obj.SetGroupVersionKind(deployv1alpha1.SchemeGroupVersion.WithKind(kind))
	return cl.Get(t.Context(), ctrlruntimeclient.ObjectKey{Namespace: suite.ProviderNamespace, Name: name}, obj) == nil
}

func distributedPlatformMesh(etcdEndpoint string) *pmdeployerv1alpha1.PlatformMesh {
	etcd := func() *operatorv1alpha1.EtcdConfig {
		return &operatorv1alpha1.EtcdConfig{
			Endpoints: []string{strconv.Quote(etcdEndpoint)},
			TLSConfig: &operatorv1alpha1.EtcdTLSConfig{SecretRef: corev1.LocalObjectReference{Name: suite.EtcdClientSecret}},
			Prefix:    `"/" + platformMesh + "/" + component`,
		}
	}
	certs := &operatorv1alpha1.Certificates{
		IssuerRef: &operatorv1alpha1.ObjectReference{Name: "kcp", Kind: "ClusterIssuer", Group: "cert-manager.io"},
	}
	exposure := func(prefix string) pmdeployerv1alpha1.Exposure {
		return pmdeployerv1alpha1.Exposure{HostnameTemplate: `"` + prefix + `." + platformMesh + ".example.com"`, Port: 6443}
	}

	return &pmdeployerv1alpha1.PlatformMesh{
		ObjectMeta: metav1.ObjectMeta{Name: "customer-a", Namespace: suite.ProviderNamespace},
		Spec: pmdeployerv1alpha1.PlatformMeshSpec{
			Version: "0.0.0",
			OCM:     pmdeployerv1alpha1.OCMRepository{URL: "oci://example.com/platform-mesh"},
			Topology: pmdeployerv1alpha1.Topology{
				RootShard: pmdeployerv1alpha1.RootShard{
					Name:              "root",
					Template:          &operatorv1alpha1.RootShardTemplateSpec{CommonShardSpecTemplate: operatorv1alpha1.CommonShardSpecTemplate{Etcd: etcd()}, Certificates: certs},
					Exposure:          exposure("api"),
					VirtualWorkspaces: pmdeployerv1alpha1.VirtualWorkspaceSpec{Exposure: exposure("vw")},
				},
				ShardGroups: []pmdeployerv1alpha1.ShardGroup{{
					Name:              "default",
					Template:          &operatorv1alpha1.ShardTemplateSpec{CommonShardSpecTemplate: operatorv1alpha1.CommonShardSpecTemplate{Etcd: etcd()}},
					VirtualWorkspaces: pmdeployerv1alpha1.VirtualWorkspaceSpec{Exposure: exposure("vw-shard")},
				}},
				FrontProxy: pmdeployerv1alpha1.FrontProxy{Name: "fp", Exposure: exposure("front")},
				CacheServer: &pmdeployerv1alpha1.CacheServer{
					Name:     "global",
					Template: &operatorv1alpha1.CacheServerTemplateSpec{Certificates: certs},
				},
			},
		},
	}
}
