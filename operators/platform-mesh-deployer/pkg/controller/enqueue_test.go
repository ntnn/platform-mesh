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

func names(reqs []reconcile.Request) []string {
	out := make([]string, 0, len(reqs))
	for _, r := range reqs {
		out = append(out, r.Name)
	}
	return out
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

// A failing client must not enqueue anything rather than panic.
func TestEnqueueWithFailingClient(t *testing.T) {
	cl := fake.NewClientBuilder().WithScheme(runtime.NewScheme()).Build()

	assert.Empty(t, enqueueModulesOfPlatformMesh(cl)(t.Context(), platformMesh("customer-a")))
}
