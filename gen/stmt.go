package gen

import (
	"fmt"
	"strings"

	"github.com/tamnd/gopygo/pyast"
)

func (g *gen) emitStmt(s pyast.Node, indent string) error {
	switch s.Type() {
	case "Pass":
		return nil

	case "Expr":
		v, err := g.emitExpr(s.Child("value"))
		if err != nil {
			return err
		}
		fmt.Fprintf(&g.out, "%s_ = %s\n", indent, v)
		return nil

	case "Return":
		val := s.Child("value")
		if val == nil {
			fmt.Fprintf(&g.out, "%sreturn rt.None, nil\n", indent)
			return nil
		}
		v, err := g.emitExpr(val)
		if err != nil {
			return err
		}
		fmt.Fprintf(&g.out, "%sreturn %s, nil\n", indent, v)
		return nil

	case "Assign":
		return g.emitAssign(s, indent)

	case "AugAssign":
		return g.emitAugAssign(s, indent)

	case "If":
		return g.emitIf(s, indent, false)

	case "While":
		return g.emitWhile(s, indent)

	case "For":
		return g.emitFor(s, indent)

	case "Break":
		fmt.Fprintf(&g.out, "%sbreak\n", indent)
		return nil
	case "Continue":
		fmt.Fprintf(&g.out, "%scontinue\n", indent)
		return nil

	case "FunctionDef":
		return g.errf(s, "nested def not supported in v0.1")
	case "Import", "ImportFrom":
		return g.errf(s, "import not supported in v0.1")
	case "ClassDef":
		return g.errf(s, "class not supported in v0.1")
	case "Try", "Raise":
		return g.errf(s, "exceptions not supported in v0.1")
	}
	return g.errf(s, "unsupported stmt: %s", s.Type())
}

func (g *gen) emitAssign(s pyast.Node, indent string) error {
	targets := s.Children("targets")
	val, err := g.emitExpr(s.Child("value"))
	if err != nil {
		return err
	}
	if len(targets) == 1 {
		t := targets[0]
		switch t.Type() {
		case "Name":
			g.emitNameStore(t.Str("id"), val, indent)
			return nil
		case "Subscript":
			obj, err := g.emitExpr(t.Child("value"))
			if err != nil {
				return err
			}
			key, err := g.emitExpr(t.Child("slice"))
			if err != nil {
				return err
			}
			fmt.Fprintf(&g.out, "%sif err := rt.SetItem(%s, %s, %s); err != nil { panic(err) }\n",
				indent, obj, key, val)
			return nil
		case "Tuple", "List":
			return g.emitTupleAssign(t, val, indent)
		}
		return g.errf(t, "unsupported assign target: %s", t.Type())
	}
	tmp := g.fresh("_a")
	fmt.Fprintf(&g.out, "%s%s := %s\n", indent, tmp, val)
	for _, t := range targets {
		if t.Type() == "Name" {
			g.emitNameStore(t.Str("id"), tmp, indent)
			continue
		}
		return g.errf(t, "chained assign target not supported: %s", t.Type())
	}
	return nil
}

func (g *gen) emitNameStore(name, val, indent string) {
	fmt.Fprintf(&g.out, "%s%s = %s\n", indent, name, val)
	if g.scope != nil && g.scope.isModule {
		fmt.Fprintf(&g.out, "%sglobals[%q] = %s\n", indent, name, name)
	}
}

// emitTupleAssign expands `a, b = rhs` via the tuple_assign template.
func (g *gen) emitTupleAssign(t pyast.Node, rhs, indent string) error {
	buf := g.fresh("_tv")
	elts := t.Children("elts")
	var stores strings.Builder
	for i, el := range elts {
		idx := fmt.Sprintf("%s[%d]", buf, i)
		switch el.Type() {
		case "Name":
			fmt.Fprintf(&stores, "%s%s = %s\n", indent, el.Str("id"), idx)
			if g.scope != nil && g.scope.isModule {
				fmt.Fprintf(&stores, "%sglobals[%q] = %s\n", indent, el.Str("id"), el.Str("id"))
			}
		case "Tuple", "List":
			inner, err := g.withBuffer(func() error { return g.emitTupleAssign(el, idx, indent) })
			if err != nil {
				return err
			}
			stores.WriteString(inner)
		default:
			return g.errf(el, "unsupported tuple element: %s", el.Type())
		}
	}
	g.out.WriteString(render("tuple_assign.go.tmpl", map[string]any{
		"Indent": indent,
		"Buf":    buf,
		"Rhs":    rhs,
		"N":      len(elts),
		"Stores": stores.String(),
	}))
	return nil
}

func (g *gen) emitAugAssign(s pyast.Node, indent string) error {
	target := s.Child("target")
	if target.Type() != "Name" {
		return g.errf(target, "aug-assign only supports Name targets in v0.1")
	}
	name := target.Str("id")
	rhs, err := g.emitExpr(s.Child("value"))
	if err != nil {
		return err
	}
	fn, err := binOpFunc(s.Child("op").Type())
	if err != nil {
		return g.errf(s, "%v", err)
	}
	lhs := g.nameExpr(name)
	g.emitNameStore(name, fmt.Sprintf("rt.Must(rt.%s(%s, %s))", fn, lhs, rhs), indent)
	return nil
}

// emitIf emits an if-chain. elseIf=true means the caller already
// wrote a trailing `} else ` and wants the body to open `if ...`.
func (g *gen) emitIf(s pyast.Node, indent string, elseIf bool) error {
	cond, err := g.emitExpr(s.Child("test"))
	if err != nil {
		return err
	}
	if elseIf {
		fmt.Fprintf(&g.out, "if rt.Truthy(%s) {\n", cond)
	} else {
		fmt.Fprintf(&g.out, "%sif rt.Truthy(%s) {\n", indent, cond)
	}
	for _, b := range s.Children("body") {
		if err := g.emitStmt(b, indent+"\t"); err != nil {
			return err
		}
	}
	orelse := s.Children("orelse")
	if len(orelse) == 1 && orelse[0].Type() == "If" {
		fmt.Fprintf(&g.out, "%s} else ", indent)
		return g.emitIf(orelse[0], indent, true)
	}
	if len(orelse) > 0 {
		fmt.Fprintf(&g.out, "%s} else {\n", indent)
		for _, b := range orelse {
			if err := g.emitStmt(b, indent+"\t"); err != nil {
				return err
			}
		}
	}
	fmt.Fprintf(&g.out, "%s}\n", indent)
	return nil
}

func (g *gen) emitWhile(s pyast.Node, indent string) error {
	if len(s.Children("orelse")) > 0 {
		return g.errf(s, "while-else not supported")
	}
	cond, err := g.emitExpr(s.Child("test"))
	if err != nil {
		return err
	}
	body, err := g.withBuffer(func() error {
		for _, b := range s.Children("body") {
			if err := g.emitStmt(b, indent+"\t"); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		return err
	}
	g.out.WriteString(render("while.go.tmpl", map[string]any{
		"Indent": indent,
		"Cond":   cond,
		"Body":   body,
	}))
	return nil
}

func (g *gen) emitFor(s pyast.Node, indent string) error {
	if len(s.Children("orelse")) > 0 {
		return g.errf(s, "for-else not supported")
	}
	iter, err := g.emitExpr(s.Child("iter"))
	if err != nil {
		return err
	}
	target := s.Child("target")
	var bind strings.Builder
	innerIndent := indent + "\t\t"
	switch target.Type() {
	case "Name":
		fmt.Fprintf(&bind, "%s%s = _v\n", innerIndent, target.Str("id"))
		if g.scope != nil && g.scope.isModule {
			fmt.Fprintf(&bind, "%sglobals[%q] = %s\n", innerIndent, target.Str("id"), target.Str("id"))
		}
	case "Tuple", "List":
		b, err := g.withBuffer(func() error { return g.emitTupleAssign(target, "_v", innerIndent) })
		if err != nil {
			return err
		}
		bind.WriteString(b)
	default:
		return g.errf(target, "for target not supported: %s", target.Type())
	}
	body, err := g.withBuffer(func() error {
		for _, b := range s.Children("body") {
			if err := g.emitStmt(b, innerIndent); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		return err
	}
	g.out.WriteString(render("for.go.tmpl", map[string]any{
		"Indent": indent,
		"It":     g.fresh("_it"),
		"Iter":   iter,
		"Bind":   bind.String(),
		"Body":   body,
	}))
	return nil
}
