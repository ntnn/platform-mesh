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

// Package ready rolls the resolved topology up into the PlatformMesh status.
package ready

import (
	"context"

	pmdeployerv1alpha1 "go.platform-mesh.io/apis/deployer/v1alpha1"
	"go.platform-mesh.io/subroutines"

	ctrlruntimeclient "sigs.k8s.io/controller-runtime/pkg/client"
)

const Name = "ReadySubroutine"

type Subroutine struct{}

func New() *Subroutine { return &Subroutine{} }

func (s *Subroutine) GetName() string { return Name }

func (s *Subroutine) Process(_ context.Context, obj ctrlruntimeclient.Object) (subroutines.Result, error) {
	pm := obj.(*pmdeployerv1alpha1.PlatformMesh)
	pm.Status.ResolvedVersion = pm.Spec.Version
	return subroutines.OK(), nil
}
