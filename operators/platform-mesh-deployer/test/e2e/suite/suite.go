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

// Package suite provides a harness to setup kind clusters for deployer e2e tests.
package suite

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"go.platform-mesh.io/platform-mesh-deployer/pkg/config"
	"go.platform-mesh.io/platform-mesh-deployer/pkg/deployer"
	"go.platform-mesh.io/platform-mesh-deployer/pkg/ocm"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
	clientcmdapi "k8s.io/client-go/tools/clientcmd/api"
	ctrlruntimeclient "sigs.k8s.io/controller-runtime/pkg/client"
	metricsserver "sigs.k8s.io/controller-runtime/pkg/metrics/server"
	mcmanager "sigs.k8s.io/multicluster-runtime/pkg/manager"
	"sigs.k8s.io/multicluster-runtime/providers/multi"
)

const (
	// ProviderNamespace is where the deployer watches kubeconfig secrets and creates admin CRs.
	ProviderNamespace = "platform-mesh-system"

	defaultKCPOperatorImage = "ghcr.io/ntnn/kcp-operator:split"

	kindClusterPrefix = "pm-deployer-e2e"
)

// kcpOperatorImage is loaded into every kind cluster; overridable for local builds.
func kcpOperatorImage() string {
	if img := os.Getenv("KCP_OPERATOR_IMAGE"); img != "" {
		return img
	}
	return defaultKCPOperatorImage
}

// Cluster is a started kind cluster.
type Cluster struct {
	Name   string
	Config *rest.Config
	Client ctrlruntimeclient.Client
}

// Env is a started e2e environment: a config plane plus workload clusters.
type Env struct {
	Config    *Cluster
	Workloads []*Cluster
}

// Start provisions the config plane and workloadClusters workload clusters.
func Start(t *testing.T, workloadClusters int) *Env {
	t.Helper()

	cfgPlane := createCluster(t, "config")
	createNamespace(t, cfgPlane.Client, ProviderNamespace)
	applyKustomize(t, cfgPlane, base("crd"))
	applyKustomize(t, cfgPlane, base("bases", "cert-manager"))
	rolloutWait(t, cfgPlane, "cert-manager", "deployment/cert-manager")
	rolloutWait(t, cfgPlane, "cert-manager", "deployment/cert-manager-cainjector")
	rolloutWait(t, cfgPlane, "cert-manager", "deployment/cert-manager-webhook")
	applyKustomizeRetry(t, cfgPlane, base("bases", "cert-issuer"))
	applyKustomizeNS(t, cfgPlane, base("bases", "etcd"), ProviderNamespace)

	env := &Env{Config: cfgPlane}
	if workloadClusters == 0 {
		// Single cluster: one operator running both config and workload groups.
		applyKustomize(t, cfgPlane, base("bases", "kcp-operator", "default"))
		rolloutWait(t, cfgPlane, "kcp-operator-system", "deployment/kcp-operator-controller-manager")
		env.Workloads = []*Cluster{cfgPlane}
	} else {
		applyKustomize(t, cfgPlane, base("bases", "kcp-operator", "config"))
		rolloutWait(t, cfgPlane, "kcp-operator-system", "deployment/kcp-operator-controller-manager")
		for i := range workloadClusters {
			w := createCluster(t, fmt.Sprintf("workload-%d", i))
			applyKustomize(t, w, base("bases", "kcp-operator", "workload"))
			rolloutWait(t, w, "kcp-operator-system", "deployment/kcp-operator-controller-manager")
			env.Workloads = append(env.Workloads, w)
		}
	}

	startDeployer(t, cfgPlane.Config)
	t.Cleanup(func() {
		if t.Failed() {
			dumpDiagnostics(t, cfgPlane)
		}
	})
	return env
}

// EngageWorkload writes a kubeconfig Secret pointing at workload, labeled for the given components, onto the config plane so the deployer engages it.
func (e *Env) EngageWorkload(t *testing.T, platformMesh, clusterID string, workload *Cluster, components ...string) {
	t.Helper()
	kubeconfig, err := restToKubeconfig(workload.Config)
	require.NoError(t, err)

	labels := map[string]string{}
	for _, c := range components {
		labels["deployer.platform-mesh.io/"+c] = "true"
	}
	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      platformMesh + "--" + clusterID,
			Namespace: ProviderNamespace,
			Labels:    labels,
		},
		Data: map[string][]byte{"kubeconfig": kubeconfig},
	}
	require.NoError(t, e.Config.Client.Create(t.Context(), secret))
}

func createCluster(t *testing.T, role string) *Cluster {
	t.Helper()
	name := kindClusterPrefix + "-" + role
	kubeconfig := filepath.Join(t.TempDir(), name+".kubeconfig")

	sh(t, "kind", "create", "cluster", "--name", name, "--kubeconfig", kubeconfig)
	t.Cleanup(func() { _ = exec.Command("kind", "delete", "cluster", "--name", name).Run() })
	sh(t, "kind", "load", "docker-image", "--name", name, kcpOperatorImage())

	cfg, err := clientcmd.BuildConfigFromFlags("", kubeconfig)
	require.NoError(t, err)
	cl, err := ctrlruntimeclient.New(cfg, ctrlruntimeclient.Options{Scheme: deployer.NewScheme()})
	require.NoError(t, err)
	return &Cluster{Name: name, Config: cfg, Client: cl}
}

func startDeployer(t *testing.T, restCfg *rest.Config) {
	t.Helper()
	provider := multi.New(multi.Options{})
	mgr, err := mcmanager.New(restCfg, provider, mcmanager.Options{
		Scheme:                 deployer.NewScheme(),
		Metrics:                metricsserver.Options{BindAddress: "0"},
		HealthProbeBindAddress: "0",
	})
	require.NoError(t, err)

	opCfg := config.NewOperatorConfig()
	cfg := opCfg.DeployerConfig(mgr, nil, ocm.New())
	require.NoError(t, deployer.AddProviders(provider, mgr, cfg))
	require.NoError(t, deployer.Setup(mgr, cfg))

	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	go func() {
		if err := mgr.Start(ctx); err != nil {
			t.Errorf("manager stopped: %v", err)
		}
	}()
}

func createNamespace(t *testing.T, cl ctrlruntimeclient.Client, name string) {
	t.Helper()
	ns := &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: name}}
	if err := cl.Create(t.Context(), ns); err != nil && !apierrors.IsAlreadyExists(err) {
		t.Fatalf("creating namespace %q: %v", name, err)
	}
}

// base resolves a path under the deployer's config/ directory from this source file's location.
func base(elem ...string) string {
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		panic("runtime.Caller failed")
	}
	root := filepath.Join(filepath.Dir(file), "..", "..", "..", "config")
	return filepath.Join(append([]string{root}, elem...)...)
}

func applyKustomize(t *testing.T, c *Cluster, kustomization string) {
	t.Helper()
	kubectlRun(t, c.Config, "apply", "-k", kustomization, "--server-side", "--force-conflicts")
}

// applyKustomizeRetry applies a kustomization, retrying transient errors such as
// the cert-manager webhook not yet serving.
func applyKustomizeRetry(t *testing.T, c *Cluster, kustomization string) {
	t.Helper()
	path := writeKubeconfig(t, c.Config)
	var out []byte
	var err error
	for range 20 {
		out, err = exec.Command("kubectl", "--kubeconfig", path, "apply", "-k", kustomization, "--server-side", "--force-conflicts").CombinedOutput() //nolint:gosec // test-controlled args
		if err == nil {
			return
		}
		time.Sleep(3 * time.Second)
	}
	t.Fatalf("kubectl apply -k %s:\n%s", kustomization, out)
}

func applyKustomizeNS(t *testing.T, c *Cluster, kustomization, namespace string) {
	t.Helper()
	kubectlRun(t, c.Config, "apply", "-k", kustomization, "-n", namespace, "--server-side", "--force-conflicts")
}

func rolloutWait(t *testing.T, c *Cluster, namespace, object string) {
	t.Helper()
	kubectlRun(t, c.Config, "-n", namespace, "rollout", "status", object, "--timeout=180s")
}

func kubectlRun(t *testing.T, cfg *rest.Config, args ...string) {
	t.Helper()
	path := writeKubeconfig(t, cfg)
	sh(t, "kubectl", append([]string{"--kubeconfig", path}, args...)...)
}

func writeKubeconfig(t *testing.T, cfg *rest.Config) string {
	t.Helper()
	kubeconfig, err := restToKubeconfig(cfg)
	require.NoError(t, err)
	path := filepath.Join(t.TempDir(), "kubeconfig")
	require.NoError(t, os.WriteFile(path, kubeconfig, 0o600))
	return path
}

func sh(t *testing.T, name string, args ...string) {
	t.Helper()
	cmd := exec.Command(name, args...) //nolint:gosec // test-controlled args
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("%s %v:\n%s", name, args, out)
	}
}

// dumpDiagnostics logs the config-plane state to help debug a failed pipeline.
func dumpDiagnostics(t *testing.T, c *Cluster) {
	t.Helper()
	kubeconfig, err := restToKubeconfig(c.Config)
	if err != nil {
		return
	}
	path := filepath.Join(t.TempDir(), "kubeconfig")
	if err := os.WriteFile(path, kubeconfig, 0o600); err != nil {
		return
	}
	for _, args := range [][]string{
		{"-n", ProviderNamespace, "get", "rootshards,compiledrootshards,frontproxies,compiledfrontproxies,shards,compiledshards,cacheservers,compiledcacheservers,virtualworkspaces,compiledvirtualworkspaces"},
		{"-n", ProviderNamespace, "get", "certificates,issuers,clusterissuers,secrets"},
		{"-n", ProviderNamespace, "get", "events", "--sort-by=.lastTimestamp"},
		{"-n", "kcp-operator-system", "logs", "deployment/kcp-operator-controller-manager", "--tail=200"},
	} {
		out, _ := exec.Command("kubectl", append([]string{"--kubeconfig", path}, args...)...).CombinedOutput() //nolint:gosec // test-controlled args
		t.Logf("=== kubectl %v ===\n%s", args, out)
	}
}

func restToKubeconfig(cfg *rest.Config) ([]byte, error) {
	c := clientcmdapi.NewConfig()
	c.Clusters["default"] = &clientcmdapi.Cluster{
		Server:                   cfg.Host,
		CertificateAuthorityData: cfg.CAData,
	}
	c.AuthInfos["default"] = &clientcmdapi.AuthInfo{
		ClientCertificateData: cfg.CertData,
		ClientKeyData:         cfg.KeyData,
		Token:                 cfg.BearerToken,
	}
	c.Contexts["default"] = &clientcmdapi.Context{Cluster: "default", AuthInfo: "default"}
	c.CurrentContext = "default"
	return clientcmd.Write(*c)
}
