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

	"github.com/stretchr/testify/require"

	pmdeployerv1alpha1 "go.platform-mesh.io/apis/deployer/v1alpha1"
	"go.platform-mesh.io/platform-mesh-deployer/test/e2e/suite"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/ptr"
	ctrlruntimeclient "sigs.k8s.io/controller-runtime/pkg/client"

	operatorv1alpha1 "github.com/kcp-dev/kcp-operator/sdk/apis/operator/v1alpha1"
)

// createPlatformMesh creates the installation under test along with the
// topology templates it references.
func createPlatformMesh(t *testing.T, c ctrlruntimeclient.Client, etcdEndpoint string) *pmdeployerv1alpha1.PlatformMesh {
	t.Helper()
	pm, templates := platformMesh(etcdEndpoint)
	for _, tpl := range templates {
		require.NoError(t, c.Create(t.Context(), tpl))
	}
	require.NoError(t, c.Create(t.Context(), pm))
	return pm
}

func platformMesh(etcdEndpoint string) (*pmdeployerv1alpha1.PlatformMesh, []ctrlruntimeclient.Object) {
	etcd := func(prefix string) *operatorv1alpha1.EtcdConfig {
		return &operatorv1alpha1.EtcdConfig{
			Endpoints: []string{strconv.Quote(etcdEndpoint)},
			TLSConfig: &operatorv1alpha1.EtcdTLSConfig{
				SecretRef: corev1.LocalObjectReference{
					Name: suite.EtcdClientSecret,
				},
			},
			Prefix: prefix,
		}
	}
	certs := &operatorv1alpha1.Certificates{
		IssuerRef: &operatorv1alpha1.ObjectReference{
			Name:  "kcp",
			Kind:  "ClusterIssuer",
			Group: "cert-manager.io",
		},
	}
	// host builds an sslip.io exposure; cluster carries the dashed node IP so it self-resolves.
	host := func(expr string) pmdeployerv1alpha1.Exposure {
		return pmdeployerv1alpha1.Exposure{
			HostnameTemplate: expr,
			Port:             31443,
		}
	}

	templates := []ctrlruntimeclient.Object{
		&pmdeployerv1alpha1.RootShardTemplate{
			ObjectMeta: metav1.ObjectMeta{Name: "root", Namespace: suite.ProviderNamespace},
			Spec: operatorv1alpha1.RootShardTemplateSpec{
				CommonShardSpecTemplate: operatorv1alpha1.CommonShardSpecTemplate{
					Etcd: etcd(`"/" + platformMesh + "/root"`),
				},
				Certificates: certs,
			},
		},
		&pmdeployerv1alpha1.ShardTemplate{
			ObjectMeta: metav1.ObjectMeta{Name: "default", Namespace: suite.ProviderNamespace},
			Spec: operatorv1alpha1.ShardTemplateSpec{
				CommonShardSpecTemplate: operatorv1alpha1.CommonShardSpecTemplate{
					Etcd: etcd(`"/" + platformMesh + "/" + component + "/" + cluster`),
				},
			},
		},
	}

	return &pmdeployerv1alpha1.PlatformMesh{
		ObjectMeta: metav1.ObjectMeta{
			Name:      suite.PlatformMeshName,
			Namespace: suite.ProviderNamespace,
		},
		Spec: pmdeployerv1alpha1.PlatformMeshSpec{
			Version: "0.0.0",
			OCM: pmdeployerv1alpha1.OCMRepository{
				URL: "oci://example.com/platform-mesh",
			},
			Ingress: []pmdeployerv1alpha1.IngressStack{{
				Name: "gateway",
				Type: pmdeployerv1alpha1.IngressTypeGatewayAPI,
				GatewayAPI: &pmdeployerv1alpha1.GatewayAPIValues{
					GatewayName:      "eg",
					GatewayNamespace: "envoy-gateway-system",
					SectionName:      "passthrough",
				},
			}},
			Topology: pmdeployerv1alpha1.Topology{
				RootShard: pmdeployerv1alpha1.RootShard{
					Name:        "root",
					TemplateRef: &pmdeployerv1alpha1.TemplateReference{Name: "root"},
					Exposure:    host(`"root." + cluster + ".sslip.io"`),
					VirtualWorkspaces: pmdeployerv1alpha1.VirtualWorkspaceSpec{
						Exposure: host(`"vw-root." + cluster + ".sslip.io"`),
					},
				},
				ShardGroups: []pmdeployerv1alpha1.ShardGroup{{
					Name:        "default",
					TemplateRef: &pmdeployerv1alpha1.TemplateReference{Name: "default"},
					Exposure:    ptr.To(host(`component + "." + cluster + ".sslip.io"`)),
					VirtualWorkspaces: pmdeployerv1alpha1.VirtualWorkspaceSpec{
						Exposure: host(`"vw." + component + "." + cluster + ".sslip.io"`),
					},
				}},
				FrontProxy: pmdeployerv1alpha1.FrontProxy{
					Name:     "fp",
					Exposure: host(`"fp." + cluster + ".sslip.io"`),
				},
			},
		},
	}, templates
}
