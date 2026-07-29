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

// Packate celtemplate is a CEL helper for the deployer.
package celtemplate

import (
	"fmt"
	"sync"

	"github.com/google/cel-go/cel"
	"github.com/google/cel-go/ext"
)

// Context is the CEL context.
//
// The topology fields are always populated.
// The module fields only when rendering a module payload.
type Context struct {
	PlatformMesh string
	Component    string
	ShardGroup   string
	Cluster      string

	Module    string
	Placement string
	// TargetNamespace is the namespace, since `namespace` is a reserved CEL identifier.
	TargetNamespace string
	ConfigMap       string
	// Workspace is the module's own workspace path.
	Workspace string
	// Workspaces maps a declared child workspace name to its absolute path.
	Workspaces map[string]string
	// KubeconfigSecrets maps a declared kubeconfig name to the secret name it is minted into on the target cluster.
	KubeconfigSecrets map[string]string
	// Endpoints are the connection details published by the ModuleSetup.
	Endpoints map[string]string
	// Values is the module's spec.values.
	Values map[string]any
}

func (c Context) activation() map[string]any {
	return map[string]any{
		"platformMesh":      c.PlatformMesh,
		"component":         c.Component,
		"shardGroup":        c.ShardGroup,
		"cluster":           c.Cluster,
		"module":            c.Module,
		"placement":         c.Placement,
		"targetNamespace":   c.TargetNamespace,
		"configMap":         c.ConfigMap,
		"workspace":         c.Workspace,
		"workspaces":        orEmpty(c.Workspaces),
		"kubeconfigSecrets": orEmpty(c.KubeconfigSecrets),
		"endpoints":         orEmpty(c.Endpoints),
		"values":            orEmptyAny(c.Values),
	}
}

func orEmpty(m map[string]string) map[string]string {
	if m == nil {
		return map[string]string{}
	}
	return m
}

func orEmptyAny(m map[string]any) map[string]any {
	if m == nil {
		return map[string]any{}
	}
	return m
}

var (
	env      *cel.Env
	envOnce  sync.Once
	envErr   error
	programs sync.Map // expr -> cel.Program
)

func celEnv() (*cel.Env, error) {
	envOnce.Do(func() {
		strMap := cel.MapType(cel.StringType, cel.StringType)
		env, envErr = cel.NewEnv(
			ext.Strings(),
			cel.Variable("platformMesh", cel.StringType),
			cel.Variable("component", cel.StringType),
			cel.Variable("shardGroup", cel.StringType),
			cel.Variable("cluster", cel.StringType),
			cel.Variable("module", cel.StringType),
			cel.Variable("placement", cel.StringType),
			cel.Variable("targetNamespace", cel.StringType),
			cel.Variable("configMap", cel.StringType),
			cel.Variable("workspace", cel.StringType),
			cel.Variable("workspaces", strMap),
			cel.Variable("kubeconfigSecrets", strMap),
			cel.Variable("endpoints", strMap),
			cel.Variable("values", cel.MapType(cel.StringType, cel.DynType)),
		)
	})
	return env, envErr
}

// Eval compiles and evaluates a string-typed CEL expression against ctx.
func Eval(expr string, ctx Context) (string, error) {
	out, err := evalAny(expr, ctx)
	if err != nil {
		return "", err
	}
	s, ok := out.(string)
	if !ok {
		return "", fmt.Errorf("CEL expression %q evaluated to %T, want string", expr, out)
	}
	return s, nil
}

// evalAny compiles and evaluates a CEL expression.
func evalAny(expr string, ctx Context) (any, error) {
	prog, err := compile(expr)
	if err != nil {
		return nil, err
	}
	out, _, err := prog.Eval(ctx.activation())
	if err != nil {
		return nil, fmt.Errorf("evaluating CEL expression %q: %w", expr, err)
	}
	return out.Value(), nil
}

func compile(expr string) (cel.Program, error) {
	if p, ok := programs.Load(expr); ok {
		return p.(cel.Program), nil
	}
	e, err := celEnv()
	if err != nil {
		return nil, err
	}
	ast, iss := e.Compile(expr)
	if iss != nil && iss.Err() != nil {
		return nil, fmt.Errorf("compiling CEL expression %q: %w", expr, iss.Err())
	}
	prog, err := e.Program(ast)
	if err != nil {
		return nil, fmt.Errorf("building CEL program %q: %w", expr, err)
	}
	programs.Store(expr, prog)
	return prog, nil
}
