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

package modules

import (
	"context"
	"fmt"

	"go.platform-mesh.io/platform-mesh-deployer/pkg/celtemplate"
	"go.platform-mesh.io/platform-mesh-deployer/pkg/components"
	"go.platform-mesh.io/platform-mesh-deployer/pkg/module"
	"go.platform-mesh.io/platform-mesh-deployer/pkg/names"
	"go.platform-mesh.io/platform-mesh-deployer/pkg/sync"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	ctrlruntimeclient "sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
)

// certificateGVK is handled unstructured: the deployer only ever writes
// Certificates, so it does not need the cert-manager Go types.
var certificateGVK = schema.GroupVersionKind{
	Group:   "cert-manager.io",
	Version: "v1",
	Kind:    "Certificate",
}

// errServingCertPending signals that a mapped component's certificate has not
// been issued yet.
var errServingCertPending = fmt.Errorf("serving certificate not issued yet")

// ensureServingCert issues a TLS certificate for a mapped component's backend
// Service and copies it to the component's cluster.
//
// The front proxy validates a mapped backend against the CA it already mounts,
// which is the root shard's root CA. Issuing from the root shard's server CA
// therefore produces a chain the front proxy trusts without kcp-operator having
// to mount anything extra.
func (s *Subroutine) ensureServingCert(ctx context.Context, st *state, inst module.Instance, celCtx celtemplate.Context) error {
	mod := st.resolved.Module
	mapping := inst.Component.Mapping

	service, err := celtemplate.Interpolate(mapping.Service, celCtx)
	if err != nil {
		return fmt.Errorf("component %q: mapping service: %w", inst.Component.Name, err)
	}
	name, ok := service.(string)
	if !ok {
		return fmt.Errorf("component %q: mapping service evaluated to %T, want string", inst.Component.Name, service)
	}

	issuer, err := s.rootShardIssuer(st)
	if err != nil {
		return err
	}

	certName := module.ServingCertName(mod.Name, inst.Component.Name, inst.Cluster.ClusterID)
	cert := &unstructured.Unstructured{}
	cert.SetGroupVersionKind(certificateGVK)
	cert.SetName(certName)
	cert.SetNamespace(mod.Namespace)
	if _, err := controllerutil.CreateOrUpdate(ctx, s.client, cert, func() error {
		cert.SetLabels(module.ModuleSelector(mod, inst.Cluster.ClusterID))
		spec := map[string]any{
			"secretName": certName,
			"dnsNames":   toAnySlice(serviceDNSNames(name, inst.Component.Namespace)),
			"usages":     []any{"server auth"},
			"issuerRef": map[string]any{
				"name":  issuer,
				"kind":  "Issuer",
				"group": certificateGVK.Group,
			},
		}
		if err := unstructured.SetNestedMap(cert.Object, spec, "spec"); err != nil {
			return err
		}
		return controllerutil.SetControllerReference(mod, cert, s.client.Scheme())
	}); err != nil {
		return fmt.Errorf("reconciling Certificate %q: %w", certName, err)
	}

	src := &corev1.Secret{}
	key := ctrlruntimeclient.ObjectKey{Namespace: mod.Namespace, Name: certName}
	if err := s.client.Get(ctx, key, src); err != nil {
		if apierrors.IsNotFound(err) {
			return fmt.Errorf("%w: %s", errServingCertPending, certName)
		}
		return fmt.Errorf("reading serving certificate %q: %w", certName, err)
	}

	cl := inst.Cluster.Cluster.GetClient()
	if err := sync.EnsureNamespace(ctx, cl, inst.Component.Namespace); err != nil {
		return err
	}
	dst := &corev1.Secret{ObjectMeta: metav1.ObjectMeta{
		Name:      module.ServingCertSecretName(mod.Name, inst.Component.Name),
		Namespace: inst.Component.Namespace,
	}}
	if _, err := controllerutil.CreateOrUpdate(ctx, cl, dst, func() error {
		dst.Labels = module.ModuleSelector(mod, inst.Cluster.ClusterID)
		dst.Type = src.Type
		dst.Data = src.Data
		return nil
	}); err != nil {
		return fmt.Errorf("copying serving certificate %q: %w", dst.Name, err)
	}
	return nil
}

func toAnySlice(in []string) []any {
	out := make([]any, len(in))
	for i, s := range in {
		out[i] = s
	}
	return out
}

// serviceDNSNames are the in-cluster names the front proxy dials the backend by.
func serviceDNSNames(service, namespace string) []string {
	return []string{
		service,
		service + "." + namespace,
		service + "." + namespace + ".svc",
		service + "." + namespace + ".svc.cluster.local",
	}
}

// rootShardIssuer is the name of the cert-manager Issuer for the root shard's
// server CA, which kcp-operator creates alongside the root shard.
func (s *Subroutine) rootShardIssuer(st *state) (string, error) {
	pm := st.platformMesh
	engaged := s.registry.ClustersFor(pm.Name, components.RootShard)
	if len(engaged) != 1 {
		return "", fmt.Errorf("expected exactly one root shard cluster, found %d", len(engaged))
	}
	return names.RootShard(pm.Name, pm.Spec.Topology.RootShard.Name, engaged[0].ClusterID) + "-server-ca", nil
}
