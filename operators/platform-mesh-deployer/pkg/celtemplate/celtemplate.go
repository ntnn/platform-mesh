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
type Context struct {
	PlatformMesh string
	Component    string
	ShardGroup   string
	Cluster      string
}

func (c Context) activation() map[string]any {
	return map[string]any{
		"platformMesh": c.PlatformMesh,
		"component":    c.Component,
		"shardGroup":   c.ShardGroup,
		"cluster":      c.Cluster,
	}
}

var (
	env      *cel.Env
	envOnce  sync.Once
	envErr   error
	programs sync.Map // expr -> cel.Program
)

func celEnv() (*cel.Env, error) {
	envOnce.Do(func() {
		env, envErr = cel.NewEnv(
			ext.Strings(),
			cel.Variable("platformMesh", cel.StringType),
			cel.Variable("component", cel.StringType),
			cel.Variable("shardGroup", cel.StringType),
			cel.Variable("cluster", cel.StringType),
		)
	})
	return env, envErr
}

// Eval compiles and evaluates a string-typed CEL expression against ctx.
func Eval(expr string, ctx Context) (string, error) {
	prog, err := compile(expr)
	if err != nil {
		return "", err
	}
	out, _, err := prog.Eval(ctx.activation())
	if err != nil {
		return "", fmt.Errorf("evaluating CEL expression %q: %w", expr, err)
	}
	s, ok := out.Value().(string)
	if !ok {
		return "", fmt.Errorf("CEL expression %q evaluated to %T, want string", expr, out.Value())
	}
	return s, nil
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
	if ast.OutputType() != cel.StringType {
		return nil, fmt.Errorf("CEL expression %q returns %s, want string", expr, ast.OutputType())
	}
	prog, err := e.Program(ast)
	if err != nil {
		return nil, fmt.Errorf("building CEL program %q: %w", expr, err)
	}
	programs.Store(expr, prog)
	return prog, nil
}
