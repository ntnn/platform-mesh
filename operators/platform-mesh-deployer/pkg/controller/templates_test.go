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

	pmdeployerv1alpha1 "go.platform-mesh.io/apis/deployer/v1alpha1"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	ctrlruntimeclient "sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

func ref(name, namespace string) *pmdeployerv1alpha1.TemplateReference {
	return &pmdeployerv1alpha1.TemplateReference{Name: name, Namespace: namespace}
}

func templatedPlatformMesh(name, namespace string) *pmdeployerv1alpha1.PlatformMesh {
	return &pmdeployerv1alpha1.PlatformMesh{
		ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: namespace},
		Spec: pmdeployerv1alpha1.PlatformMeshSpec{
			Topology: pmdeployerv1alpha1.Topology{
				RootShard: pmdeployerv1alpha1.RootShard{
					Name:        "root",
					TemplateRef: ref("root", ""),
					VirtualWorkspaces: pmdeployerv1alpha1.VirtualWorkspaceSpec{
						TemplateRef: ref("vw", "shared"),
					},
				},
				FrontProxy: pmdeployerv1alpha1.FrontProxy{
					Name:        "fp",
					TemplateRef: ref("fp", ""),
				},
				CacheServer: &pmdeployerv1alpha1.CacheServer{
					Name:        "cache",
					TemplateRef: ref("cache", ""),
				},
				ShardGroups: []pmdeployerv1alpha1.ShardGroup{{
					Name:        "default",
					TemplateRef: ref("default", ""),
					VirtualWorkspaces: pmdeployerv1alpha1.VirtualWorkspaceSpec{
						TemplateRef: ref("vw", "shared"),
					},
				}},
			},
		},
	}
}

func TestTemplateRefs(t *testing.T) {
	t.Run("defaults the namespace and dedupes", func(t *testing.T) {
		got := templateRefs(templatedPlatformMesh("customer-a", "pm"))
		assert.Equal(t, map[templateKey]struct{}{
			{kind: "RootShardTemplate", namespace: "pm", name: "root"}:          {},
			{kind: "FrontProxyTemplate", namespace: "pm", name: "fp"}:           {},
			{kind: "CacheServerTemplate", namespace: "pm", name: "cache"}:       {},
			{kind: "ShardTemplate", namespace: "pm", name: "default"}:           {},
			{kind: "VirtualWorkspaceTemplate", namespace: "shared", name: "vw"}: {},
		}, got)
	})

	t.Run("ignores unset refs", func(t *testing.T) {
		pm := &pmdeployerv1alpha1.PlatformMesh{ObjectMeta: metav1.ObjectMeta{Name: "a", Namespace: "pm"}}
		assert.Empty(t, templateRefs(pm))
	})
}

func TestEnqueuePlatformMeshesUsingTemplate(t *testing.T) {
	// Two installations in different namespaces sharing one template.
	a := templatedPlatformMesh("customer-a", "pm-a")
	b := templatedPlatformMesh("customer-b", "pm-b")
	unrelated := templatedPlatformMesh("customer-c", "pm-c")
	unrelated.Spec.Topology.RootShard.VirtualWorkspaces.TemplateRef = ref("other", "shared")
	unrelated.Spec.Topology.ShardGroups[0].VirtualWorkspaces.TemplateRef = nil

	cl := fake.NewClientBuilder().WithScheme(scheme(t)).WithObjects(a, b, unrelated).Build()
	shared := &pmdeployerv1alpha1.VirtualWorkspaceTemplate{
		ObjectMeta: metav1.ObjectMeta{Name: "vw", Namespace: "shared"},
	}

	got := enqueuePlatformMeshesUsingTemplate(cl, "VirtualWorkspaceTemplate")(t.Context(), shared)
	assert.ElementsMatch(t, []reconcile.Request{
		{NamespacedName: ctrlruntimeclient.ObjectKey{Namespace: "pm-a", Name: "customer-a"}},
		{NamespacedName: ctrlruntimeclient.ObjectKey{Namespace: "pm-b", Name: "customer-b"}},
	}, got)
}

func TestTemplateReconcilerFinalizer(t *testing.T) {
	newReconciler := func(t *testing.T, objs ...ctrlruntimeclient.Object) *TemplateReconciler {
		t.Helper()
		return &TemplateReconciler{
			client: fake.NewClientBuilder().WithScheme(scheme(t)).WithObjects(objs...).Build(),
			kind:   "RootShardTemplate",
			object: func() ctrlruntimeclient.Object { return &pmdeployerv1alpha1.RootShardTemplate{} },
		}
	}
	key := reconcile.Request{NamespacedName: ctrlruntimeclient.ObjectKey{Namespace: "pm", Name: "root"}}
	template := func() *pmdeployerv1alpha1.RootShardTemplate {
		return &pmdeployerv1alpha1.RootShardTemplate{
			ObjectMeta: metav1.ObjectMeta{Name: "root", Namespace: "pm"},
		}
	}

	t.Run("holds a referenced template", func(t *testing.T) {
		r := newReconciler(t, template(), templatedPlatformMesh("customer-a", "pm"))
		_, err := r.Reconcile(t.Context(), key)
		require.NoError(t, err)

		got := &pmdeployerv1alpha1.RootShardTemplate{}
		require.NoError(t, r.client.Get(t.Context(), key.NamespacedName, got))
		assert.True(t, controllerutil.ContainsFinalizer(got, pmdeployerv1alpha1.TemplateFinalizer))
	})

	t.Run("releases an unreferenced template", func(t *testing.T) {
		held := template()
		controllerutil.AddFinalizer(held, pmdeployerv1alpha1.TemplateFinalizer)
		r := newReconciler(t, held)

		_, err := r.Reconcile(t.Context(), key)
		require.NoError(t, err)

		got := &pmdeployerv1alpha1.RootShardTemplate{}
		require.NoError(t, r.client.Get(t.Context(), key.NamespacedName, got))
		assert.False(t, controllerutil.ContainsFinalizer(got, pmdeployerv1alpha1.TemplateFinalizer))
	})

	t.Run("keeps holding while any installation still refers", func(t *testing.T) {
		held := template()
		controllerutil.AddFinalizer(held, pmdeployerv1alpha1.TemplateFinalizer)
		gone := templatedPlatformMesh("customer-a", "pm")
		gone.Spec.Topology.RootShard.TemplateRef = nil
		r := newReconciler(t, held, gone, templatedPlatformMesh("customer-b", "pm"))

		_, err := r.Reconcile(t.Context(), key)
		require.NoError(t, err)

		got := &pmdeployerv1alpha1.RootShardTemplate{}
		require.NoError(t, r.client.Get(t.Context(), key.NamespacedName, got))
		assert.True(t, controllerutil.ContainsFinalizer(got, pmdeployerv1alpha1.TemplateFinalizer))
	})

	t.Run("ignores a deleted template", func(t *testing.T) {
		r := newReconciler(t)
		_, err := r.Reconcile(t.Context(), key)
		require.NoError(t, err)
	})
}

func TestEnqueueTemplatesOfPlatformMesh(t *testing.T) {
	got := enqueueTemplatesOfPlatformMesh("VirtualWorkspaceTemplate")(t.Context(), templatedPlatformMesh("customer-a", "pm"))
	assert.Equal(t, []reconcile.Request{
		{NamespacedName: ctrlruntimeclient.ObjectKey{Namespace: "shared", Name: "vw"}},
	}, got)
}
