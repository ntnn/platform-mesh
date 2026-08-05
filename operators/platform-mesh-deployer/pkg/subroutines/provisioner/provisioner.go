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

// Package provisioner performs the kcp side of a module: it creates the
// workspaces the module declared and applies the content the module ships for
// them. It is the only part of the deployer that writes inside kcp.
package provisioner

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"strings"
	"time"

	pmdeployv1alpha1 "go.platform-mesh.io/apis/deploy/v1alpha1"
	"go.platform-mesh.io/platform-mesh-deployer/pkg/kcp"
	"go.platform-mesh.io/platform-mesh-deployer/pkg/module"
	pmocm "go.platform-mesh.io/platform-mesh-deployer/pkg/ocm"
	"go.platform-mesh.io/platform-mesh-deployer/pkg/sync"
	"go.platform-mesh.io/subroutines"

	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	k8syaml "k8s.io/apimachinery/pkg/util/yaml"
	ctrlruntimeclient "sigs.k8s.io/controller-runtime/pkg/client"
)

const Name = "ProvisionerSubroutine"

// ConditionReady is the handshake the module controller waits on.
const ConditionReady = "Ready"

const requeueWait = 10 * time.Second

// Subroutine reconciles one ModuleSetup.
type Subroutine struct {
	client   ctrlruntimeclient.Client
	access   *kcp.Access
	resolver pmocm.Resolver
}

func New(client ctrlruntimeclient.Client, access *kcp.Access, resolver pmocm.Resolver) *Subroutine {
	return &Subroutine{client: client, access: access, resolver: resolver}
}

func (s *Subroutine) GetName() string { return Name }

func (s *Subroutine) Process(ctx context.Context, obj ctrlruntimeclient.Object) (subroutines.Result, error) {
	setup := obj.(*pmdeployv1alpha1.ModuleSetup)

	pm := &pmdeployv1alpha1.PlatformMesh{}
	key := ctrlruntimeclient.ObjectKey{Namespace: setup.Namespace, Name: setup.Spec.PlatformMeshRef.Name}
	if err := s.client.Get(ctx, key, pm); err != nil {
		return subroutines.Result{}, fmt.Errorf("getting PlatformMesh %q: %w", key.Name, err)
	}
	if !meta.IsStatusConditionTrue(pm.Status.Conditions, "RootStructureProvisioned") {
		return s.pending(setup, "WaitingForRootStructure", "kcp root structure is not provisioned yet")
	}

	cfg, err := s.access.Config(ctx, pm)
	if err != nil {
		if errors.Is(err, kcp.ErrPending) {
			return s.pending(setup, "WaitingForKubeconfig", err.Error())
		}
		return subroutines.Result{}, err
	}

	for _, ws := range setup.Spec.Workspaces {
		client, err := s.access.EnsurePath(ctx, cfg, ws.Path)
		if err != nil {
			if errors.Is(err, kcp.ErrWorkspacePending) {
				return s.pending(setup, "WaitingForWorkspace", err.Error())
			}
			return subroutines.Result{}, err
		}
		if err := s.applyContent(ctx, setup, client, ws); err != nil {
			return subroutines.Result{}, err
		}
	}

	setup.Status.Endpoints = workspaceEndpoints(cfg.Host, setup.Spec.Workspaces)

	meta.SetStatusCondition(&setup.Status.Conditions, metav1.Condition{
		Type:               ConditionReady,
		Status:             metav1.ConditionTrue,
		Reason:             "Provisioned",
		Message:            "workspaces and content applied",
		ObservedGeneration: setup.Generation,
	})
	return subroutines.OK(), nil
}

// workspaceEndpoints publishes the URL of each provisioned workspace, so a
// module's payload can address its own kcp workspaces without knowing how the
// front proxy is exposed. The module workspace is published as "workspace";
// children keep their own name.
func workspaceEndpoints(host string, workspaces []pmdeployv1alpha1.ModuleSetupWorkspace) map[string]string {
	if len(workspaces) == 0 {
		return nil
	}
	out := make(map[string]string, len(workspaces))
	shortest := ""
	for _, ws := range workspaces {
		if shortest == "" || len(ws.Path) < len(shortest) {
			shortest = ws.Path
		}
	}
	for _, ws := range workspaces {
		name := "workspace"
		if ws.Path != shortest {
			name = ws.Path[strings.LastIndex(ws.Path, ":")+1:]
		}
		out[name] = host + "/clusters/" + ws.Path
	}
	return out
}

// applyContent applies the manifests declared for one workspace into it.
func (s *Subroutine) applyContent(ctx context.Context, setup *pmdeployv1alpha1.ModuleSetup, client ctrlruntimeclient.Client, ws pmdeployv1alpha1.ModuleSetupWorkspace) error {
	if len(ws.Content) == 0 {
		return nil
	}

	resolved, err := s.resolveModule(ctx, setup)
	if err != nil {
		return err
	}

	for _, ref := range ws.Content {
		res, err := module.Resource(resolved.CV, ref.Name)
		if err != nil {
			return err
		}
		blob, err := resolved.CV.Download(ctx, res)
		if err != nil {
			return fmt.Errorf("downloading %q: %w", ref.Name, err)
		}
		raw, err := readBlob(blob)
		if err != nil {
			return fmt.Errorf("reading %q: %w", ref.Name, err)
		}
		objs, err := decode(raw)
		if err != nil {
			return fmt.Errorf("decoding %q: %w", ref.Name, err)
		}
		for _, o := range objs {
			if err := sync.Apply(ctx, client, o); err != nil {
				return fmt.Errorf("applying %q into %s: %w", ref.Name, ws.Path, err)
			}
		}
	}
	return nil
}

// resolveModule fetches the component version the setup belongs to.
func (s *Subroutine) resolveModule(ctx context.Context, setup *pmdeployv1alpha1.ModuleSetup) (*module.Resolved, error) {
	mod := &pmdeployv1alpha1.Module{}
	key := ctrlruntimeclient.ObjectKey{Namespace: setup.Namespace, Name: setup.Spec.ModuleRef.Name}
	if err := s.client.Get(ctx, key, mod); err != nil {
		return nil, fmt.Errorf("getting Module %q: %w", key.Name, err)
	}

	pm := &pmdeployv1alpha1.PlatformMesh{}
	pmKey := ctrlruntimeclient.ObjectKey{Namespace: setup.Namespace, Name: setup.Spec.PlatformMeshRef.Name}
	if err := s.client.Get(ctx, pmKey, pm); err != nil {
		return nil, fmt.Errorf("getting PlatformMesh %q: %w", pmKey.Name, err)
	}

	resolved, err := module.Resolve(ctx, s.resolver, mod, &pm.Spec.OCM)
	if err != nil {
		return nil, fmt.Errorf("resolving module %q: %w", mod.Name, err)
	}
	return resolved, nil
}

func (s *Subroutine) pending(setup *pmdeployv1alpha1.ModuleSetup, reason, message string) (subroutines.Result, error) {
	meta.SetStatusCondition(&setup.Status.Conditions, metav1.Condition{
		Type:               ConditionReady,
		Status:             metav1.ConditionFalse,
		Reason:             reason,
		Message:            message,
		ObservedGeneration: setup.Generation,
	})
	return subroutines.StopWithRequeue(requeueWait, message), nil
}

func readBlob(b interface{ ReadCloser() (io.ReadCloser, error) }) ([]byte, error) {
	rc, err := b.ReadCloser()
	if err != nil {
		return nil, err
	}
	defer rc.Close() //nolint:errcheck // read-only
	return io.ReadAll(rc)
}

func decode(raw []byte) ([]*unstructured.Unstructured, error) {
	reader := k8syaml.NewYAMLOrJSONDecoder(bytes.NewReader(raw), 4096)
	var out []*unstructured.Unstructured
	for {
		fields := map[string]any{}
		if err := reader.Decode(&fields); err != nil {
			if errors.Is(err, io.EOF) {
				return out, nil
			}
			return nil, err
		}
		if len(fields) == 0 {
			continue
		}
		out = append(out, &unstructured.Unstructured{Object: fields})
	}
}
