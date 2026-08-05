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

package controller

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	pmdeployv1alpha1 "go.platform-mesh.io/apis/deploy/v1alpha1"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

func scheme(t *testing.T) *runtime.Scheme {
	t.Helper()
	s := runtime.NewScheme()
	require.NoError(t, clientgoscheme.AddToScheme(s))
	require.NoError(t, pmdeployv1alpha1.AddToScheme(s))
	return s
}

func platformMesh(name string) *pmdeployv1alpha1.PlatformMesh {
	return &pmdeployv1alpha1.PlatformMesh{
		ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: "pm"},
	}
}

func module(name, platformMesh string) *pmdeployv1alpha1.Module {
	return &pmdeployv1alpha1.Module{
		ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: "pm"},
		Spec: pmdeployv1alpha1.ModuleSpec{
			PlatformMeshRef: corev1.LocalObjectReference{Name: platformMesh},
		},
	}
}

func moduleSetup(name, platformMesh string) *pmdeployv1alpha1.ModuleSetup {
	return &pmdeployv1alpha1.ModuleSetup{
		ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: "pm"},
		Spec: pmdeployv1alpha1.ModuleSetupSpec{
			PlatformMeshRef: corev1.LocalObjectReference{Name: platformMesh},
		},
	}
}

func names(reqs []reconcile.Request) []string {
	out := make([]string, 0, len(reqs))
	for _, r := range reqs {
		out = append(out, r.Name)
	}
	return out
}

// The cluster registry signals by PlatformMesh name, so the mapping has to find
// the object with that name rather than the signalling object itself.
func TestEnqueuePlatformMeshByName(t *testing.T) {
	cl := fake.NewClientBuilder().WithScheme(scheme(t)).
		WithObjects(platformMesh("customer-a"), platformMesh("customer-b")).Build()

	signal := &metav1.PartialObjectMetadata{ObjectMeta: metav1.ObjectMeta{Name: "customer-a"}}
	got := enqueuePlatformMeshByName(cl)(t.Context(), signal)

	require.Len(t, got, 1)
	assert.Equal(t, "customer-a", got[0].Name)
	assert.Equal(t, "pm", got[0].Namespace)
}

func TestEnqueuePlatformMeshByNameUnknown(t *testing.T) {
	cl := fake.NewClientBuilder().WithScheme(scheme(t)).WithObjects(platformMesh("customer-a")).Build()

	signal := &metav1.PartialObjectMetadata{ObjectMeta: metav1.ObjectMeta{Name: "nope"}}
	assert.Empty(t, enqueuePlatformMeshByName(cl)(t.Context(), signal))
}

// A module publishes its front proxy mapping in its status, which the topology
// merges, so the PlatformMesh has to be reconciled again.
func TestEnqueuePlatformMeshOfModule(t *testing.T) {
	got := enqueuePlatformMeshOfModule()(t.Context(), module("acme", "customer-a"))

	require.Len(t, got, 1)
	assert.Equal(t, "customer-a", got[0].Name)
	assert.Equal(t, "pm", got[0].Namespace)
}

func TestEnqueuePlatformMeshOfModuleWrongType(t *testing.T) {
	assert.Empty(t, enqueuePlatformMeshOfModule()(t.Context(), platformMesh("customer-a")))
}

// Modules of a PlatformMesh are reconciled when it changes, since they gate on
// its topology.
func TestEnqueueModulesOfPlatformMesh(t *testing.T) {
	cl := fake.NewClientBuilder().WithScheme(scheme(t)).WithObjects(
		module("acme", "customer-a"),
		module("other", "customer-a"),
		module("elsewhere", "customer-b"),
	).Build()

	got := enqueueModulesOfPlatformMesh(cl)(t.Context(), platformMesh("customer-a"))
	assert.ElementsMatch(t, []string{"acme", "other"}, names(got))
}

// The kcp side can only start once the root structure exists, so a PlatformMesh
// change re-drives its setups.
func TestEnqueueSetupsOfPlatformMesh(t *testing.T) {
	cl := fake.NewClientBuilder().WithScheme(scheme(t)).WithObjects(
		moduleSetup("acme", "customer-a"),
		moduleSetup("elsewhere", "customer-b"),
	).Build()

	got := enqueueSetupsOfPlatformMesh(cl)(t.Context(), platformMesh("customer-a"))
	assert.Equal(t, []string{"acme"}, names(got))
}

// A failing client must not enqueue anything rather than panic.
func TestEnqueueWithFailingClient(t *testing.T) {
	cl := fake.NewClientBuilder().WithScheme(runtime.NewScheme()).Build()

	signal := &metav1.PartialObjectMetadata{ObjectMeta: metav1.ObjectMeta{Name: "customer-a"}}
	assert.Empty(t, enqueuePlatformMeshByName(cl)(t.Context(), signal))
	assert.Empty(t, enqueueModulesOfPlatformMesh(cl)(t.Context(), platformMesh("customer-a")))
	assert.Empty(t, enqueueSetupsOfPlatformMesh(cl)(t.Context(), platformMesh("customer-a")))
}
