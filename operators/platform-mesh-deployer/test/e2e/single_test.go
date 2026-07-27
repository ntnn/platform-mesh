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

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	ctrlruntimeclient "sigs.k8s.io/controller-runtime/pkg/client"

	deployv1alpha1 "github.com/kcp-dev/kcp-operator/sdk/apis/deploy/v1alpha1"
	operatorv1alpha1 "github.com/kcp-dev/kcp-operator/sdk/apis/operator/v1alpha1"
)

// TestSingleCluster runs the full pipeline on one cluster.
func TestSingleCluster(t *testing.T) {
	env := suite.Start(t, 0)
	env.EngageWorkload(t, "customer-a", env.Config, "rootshard", "frontproxy", "cacheserver", "shards-default")

	pm := platformMesh(env.EtcdEndpoint())
	require.NoError(t, env.Config.Client.Create(t.Context(), pm))

	rootName := "root-" + env.Config.NodeIP
	rootShard := ctrlruntimeclient.ObjectKey{Namespace: suite.ProviderNamespace, Name: rootName}

	require.Eventually(t, func() bool {
		return env.Config.Client.Get(t.Context(), rootShard, &operatorv1alpha1.RootShard{}) == nil
	}, 5*time.Minute, 2*time.Second, "deployer did not create the RootShard admin CR")

	require.Eventually(t, func() bool {
		return env.Config.Client.Get(t.Context(), rootShard, &deployv1alpha1.CompiledRootShard{}) == nil
	}, 8*time.Minute, 5*time.Second, "config kcp-operator did not compile the RootShard")

	workload := env.Workloads[0]
	rootShardDeployment := ctrlruntimeclient.ObjectKey{Namespace: suite.ProviderNamespace, Name: rootName + "-kcp"}
	require.Eventually(t, func() bool {
		return workload.Client.Get(t.Context(), rootShardDeployment, &appsv1.Deployment{}) == nil
	}, 3*time.Minute, 5*time.Second, "workload kcp-operator did not render the root shard Deployment")
}

func platformMesh(etcdEndpoint string) *pmdeployerv1alpha1.PlatformMesh {
	etcd := func(prefix string) *operatorv1alpha1.EtcdConfig {
		return &operatorv1alpha1.EtcdConfig{
			Endpoints: []string{strconv.Quote(etcdEndpoint)},
			TLSConfig: &operatorv1alpha1.EtcdTLSConfig{SecretRef: corev1.LocalObjectReference{Name: suite.EtcdClientSecret}},
			Prefix:    prefix,
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
					Name: "root",
					Template: &operatorv1alpha1.RootShardTemplateSpec{
						CommonShardSpecTemplate: operatorv1alpha1.CommonShardSpecTemplate{
							Etcd: etcd(`"/" + platformMesh + "/root"`),
						},
						Certificates: certs,
					},
					Exposure: exposure("api"),
					VirtualWorkspaces: pmdeployerv1alpha1.VirtualWorkspaceSpec{
						Exposure: exposure("vw"),
					},
				},
				FrontProxy: pmdeployerv1alpha1.FrontProxy{
					Name:     "fp",
					Exposure: exposure("front"),
				},
			},
		},
	}
}
