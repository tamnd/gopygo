// Package gen walks a parsed Python AST (from pyast) and emits a Go
// source file whose block structure mirrors the Python source. Every
// Python name stays a Go name; every Python control flow construct
// becomes its Go counterpart wrapping rt.Truthy / rt.Compare / etc.
//
// Code shape is assembled from a handful of Go text/template files
// under templates/; the walker fills in the per-node fields. This
// keeps the "what does emitted Go look like" question answerable by
// reading the .tmpl files rather than chasing fmt.Fprintf sites.
package gen

import (
	"bytes"
	"fmt"

	"github.com/tamnd/gopygo/pyast"
)

// Compile takes a Module node and returns a gofmt-ready Go source
// string. pkg is the package name to put at the top (usually "main").
func Compile(module pyast.Node, pkg string) ([]byte, error) {
	g := &gen{pkg: pkg}
	if err := g.emitModule(module); err != nil {
		return nil, err
	}
	return g.out.Bytes(), nil
}

type gen struct {
	pkg      string
	out      bytes.Buffer
	scope    *scope
	funcs    []string
	tmp      int
}

type scope struct {
	params   map[string]bool
	locals   map[string]bool
	isModule bool
}

func (g *gen) fresh(prefix string) string {
	g.tmp++
	return fmt.Sprintf("%s%d", prefix, g.tmp)
}

func (g *gen) errf(n pyast.Node, format string, args ...any) error {
	return fmt.Errorf("gopygo: line %d col %d: %s", n.Line(), n.Col(),
		fmt.Sprintf(format, args...))
}

// emitModule lowers a whole Module node.
func (g *gen) emitModule(mod pyast.Node) error {
	if mod.Type() != "Module" {
		return fmt.Errorf("gopygo: expected Module at root, got %s", mod.Type())
	}
	body := mod.Children("body")

	modScope := &scope{locals: map[string]bool{}, params: map[string]bool{}, isModule: true}
	collectAssigns(body, modScope.locals)

	g.out.WriteString(render("header.go.tmpl", map[string]any{"Pkg": g.pkg}))

	// Emit nested defs first so main() can reference them.
	for _, n := range body {
		if n.Type() == "FunctionDef" {
			g.funcs = append(g.funcs, n.Str("name"))
			if err := g.emitFunctionDef(n); err != nil {
				return err
			}
		}
	}

	g.scope = modScope
	var mainBody bytes.Buffer
	prev := g.out
	g.out = mainBody
	for _, n := range body {
		if n.Type() == "FunctionDef" {
			continue
		}
		if err := g.emitStmt(n, "\t"); err != nil {
			return err
		}
	}
	mainBody = g.out
	g.out = prev

	g.out.WriteString(render("main.go.tmpl", map[string]any{
		"Locals": sortedKeys(modScope.locals),
		"Funcs":  g.funcs,
		"Body":   mainBody.String(),
	}))
	return nil
}

// emitFunctionDef lowers one `def` at module scope.
func (g *gen) emitFunctionDef(n pyast.Node) error {
	name := n.Str("name")
	args := n.Child("args")
	if args == nil {
		return g.errf(n, "FunctionDef without args")
	}
	params, err := extractParams(args)
	if err != nil {
		return err
	}
	body := n.Children("body")

	sc := &scope{
		params: map[string]bool{},
		locals: map[string]bool{},
	}
	for _, p := range params {
		sc.params[p] = true
	}
	collectAssigns(body, sc.locals)
	for p := range sc.params {
		delete(sc.locals, p)
	}

	var inner bytes.Buffer
	prev := g.out
	g.out = inner
	saved := g.scope
	g.scope = sc
	for _, s := range body {
		if err := g.emitStmt(s, "\t"); err != nil {
			g.scope = saved
			g.out = prev
			return err
		}
	}
	g.scope = saved
	inner = g.out
	g.out = prev

	g.out.WriteString(render("func.go.tmpl", map[string]any{
		"Name":   name,
		"NameQ":  fmt.Sprintf("%q", name),
		"Arity":  len(params),
		"Params": params,
		"Locals": sortedKeys(sc.locals),
		"Body":   inner.String(),
	}))
	return nil
}

func extractParams(args pyast.Node) ([]string, error) {
	var out []string
	for _, a := range args.Children("args") {
		out = append(out, a.Str("arg"))
	}
	if len(args.Children("posonlyargs")) > 0 {
		return nil, fmt.Errorf("gopygo: positional-only args not supported")
	}
	if len(args.Children("kwonlyargs")) > 0 {
		return nil, fmt.Errorf("gopygo: keyword-only args not supported")
	}
	if args.Child("vararg") != nil {
		return nil, fmt.Errorf("gopygo: *args not supported")
	}
	if args.Child("kwarg") != nil {
		return nil, fmt.Errorf("gopygo: **kwargs not supported")
	}
	if len(args.Children("defaults")) > 0 {
		return nil, fmt.Errorf("gopygo: default args not supported")
	}
	return out, nil
}

// collectAssigns walks stmts (not into nested defs) and records names
// bound by assignment.
func collectAssigns(stmts []pyast.Node, out map[string]bool) {
	for _, s := range stmts {
		switch s.Type() {
		case "Assign":
			for _, t := range s.Children("targets") {
				collectTarget(t, out)
			}
		case "AugAssign", "AnnAssign":
			collectTarget(s.Child("target"), out)
		case "For":
			collectTarget(s.Child("target"), out)
			collectAssigns(s.Children("body"), out)
			collectAssigns(s.Children("orelse"), out)
		case "While", "If":
			collectAssigns(s.Children("body"), out)
			collectAssigns(s.Children("orelse"), out)
		case "FunctionDef":
			out[s.Str("name")] = true
		}
	}
}

func collectTarget(t pyast.Node, out map[string]bool) {
	if t == nil {
		return
	}
	switch t.Type() {
	case "Name":
		out[t.Str("id")] = true
	case "Tuple", "List":
		for _, e := range t.Children("elts") {
			collectTarget(e, out)
		}
	}
}

func sortedKeys(m map[string]bool) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	for i := 1; i < len(out); i++ {
		for j := i; j > 0 && out[j-1] > out[j]; j-- {
			out[j-1], out[j] = out[j], out[j-1]
		}
	}
	return out
}

func (g *gen) isLocal(name string) bool {
	if g.scope == nil {
		return false
	}
	return g.scope.params[name] || g.scope.locals[name]
}

func (g *gen) nameExpr(name string) string {
	if g.isLocal(name) {
		return name
	}
	return fmt.Sprintf("loadName(%q)", name)
}

// quote returns a Go double-quoted literal for s.
func quote(s string) string {
	return fmt.Sprintf("%q", s)
}

// withBuffer runs fn with g.out redirected to a fresh buffer and
// returns the captured bytes. Used when a parent needs the rendered
// child text before emitting its own template.
func (g *gen) withBuffer(fn func() error) (string, error) {
	var b bytes.Buffer
	prev := g.out
	g.out = b
	if err := fn(); err != nil {
		g.out = prev
		return "", err
	}
	b = g.out
	g.out = prev
	return b.String(), nil
}
