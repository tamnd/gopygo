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
		fmt.Fprintf(&g.body, "%s_ = %s\n", indent, v)
		return nil

	case "Return":
		val := s.Child("value")
		if val == nil {
			fmt.Fprintf(&g.body, "%sreturn rt.None, nil\n", indent)
			return nil
		}
		v, err := g.emitExpr(val)
		if err != nil {
			return err
		}
		fmt.Fprintf(&g.body, "%sreturn %s, nil\n", indent, v)
		return nil

	case "Assign":
		return g.emitAssign(s, indent)

	case "AugAssign":
		return g.emitAugAssign(s, indent)

	case "If":
		return g.emitIf(s, indent)

	case "While":
		return g.emitWhile(s, indent)

	case "For":
		return g.emitFor(s, indent)

	case "Break":
		fmt.Fprintf(&g.body, "%sbreak\n", indent)
		return nil
	case "Continue":
		fmt.Fprintf(&g.body, "%scontinue\n", indent)
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
	// Single simple target fast path.
	if len(targets) == 1 {
		t := targets[0]
		if t.Type() == "Name" {
			g.emitNameStore(t.Str("id"), val, indent)
			return nil
		}
		if t.Type() == "Subscript" {
			obj, err := g.emitExpr(t.Child("value"))
			if err != nil {
				return err
			}
			key, err := g.emitExpr(t.Child("slice"))
			if err != nil {
				return err
			}
			fmt.Fprintf(&g.body, "%sif err := rt.SetItem(%s, %s, %s); err != nil { panic(err) }\n",
				indent, obj, key, val)
			return nil
		}
		if t.Type() == "Tuple" || t.Type() == "List" {
			return g.emitTupleAssign(t, val, indent)
		}
		return g.errf(t, "unsupported assign target: %s", t.Type())
	}
	// Chained a = b = expr: evaluate once, assign to each target.
	tmp := g.fresh("_a")
	fmt.Fprintf(&g.body, "%s%s := %s\n", indent, tmp, val)
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
	fmt.Fprintf(&g.body, "%s%s = %s\n", indent, name, val)
	if g.scope != nil && g.scope.isModule {
		fmt.Fprintf(&g.body, "%sglobals[%q] = %s\n", indent, name, name)
	}
}

// emitTupleAssign expands `a, b = rhs` into a fresh tuple temp and
// per-element stores. Supports nested unpacking.
func (g *gen) emitTupleAssign(t pyast.Node, rhs, indent string) error {
	tmp := g.fresh("_t")
	fmt.Fprintf(&g.body, "%s%s := rt.MustIter(rt.GetIter(%s))\n", indent, tmp, rhs)
	// Collect into Go []rt.Value so we can index.
	buf := g.fresh("_tv")
	fmt.Fprintf(&g.body, "%svar %s []rt.Value\n", indent, buf)
	fmt.Fprintf(&g.body, "%sfor { v, ok := %s.Next(); if !ok { break }; %s = append(%s, v) }\n",
		indent, tmp, buf, buf)
	elts := t.Children("elts")
	fmt.Fprintf(&g.body, "%sif len(%s) != %d { panic(rt.TypeError(\"unpack: expected %d, got %%d\", len(%s))) }\n",
		indent, buf, len(elts), len(elts), buf)
	for i, el := range elts {
		idx := fmt.Sprintf("%s[%d]", buf, i)
		switch el.Type() {
		case "Name":
			g.emitNameStore(el.Str("id"), idx, indent)
		case "Tuple", "List":
			if err := g.emitTupleAssign(el, idx, indent); err != nil {
				return err
			}
		default:
			return g.errf(el, "unsupported tuple element: %s", el.Type())
		}
	}
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

func (g *gen) emitIf(s pyast.Node, indent string) error {
	cond, err := g.emitExpr(s.Child("test"))
	if err != nil {
		return err
	}
	fmt.Fprintf(&g.body, "%sif rt.Truthy(%s) {\n", indent, cond)
	for _, b := range s.Children("body") {
		if err := g.emitStmt(b, indent+"\t"); err != nil {
			return err
		}
	}
	orelse := s.Children("orelse")
	if len(orelse) == 1 && orelse[0].Type() == "If" {
		// elif chain: emit as `} else if ...`
		fmt.Fprintf(&g.body, "%s} else ", indent)
		// Inline the nested If without an extra indent.
		return g.emitIfChainTail(orelse[0], indent)
	}
	if len(orelse) > 0 {
		fmt.Fprintf(&g.body, "%s} else {\n", indent)
		for _, b := range orelse {
			if err := g.emitStmt(b, indent+"\t"); err != nil {
				return err
			}
		}
	}
	fmt.Fprintf(&g.body, "%s}\n", indent)
	return nil
}

func (g *gen) emitIfChainTail(s pyast.Node, indent string) error {
	cond, err := g.emitExpr(s.Child("test"))
	if err != nil {
		return err
	}
	fmt.Fprintf(&g.body, "if rt.Truthy(%s) {\n", cond)
	for _, b := range s.Children("body") {
		if err := g.emitStmt(b, indent+"\t"); err != nil {
			return err
		}
	}
	orelse := s.Children("orelse")
	if len(orelse) == 1 && orelse[0].Type() == "If" {
		fmt.Fprintf(&g.body, "%s} else ", indent)
		return g.emitIfChainTail(orelse[0], indent)
	}
	if len(orelse) > 0 {
		fmt.Fprintf(&g.body, "%s} else {\n", indent)
		for _, b := range orelse {
			if err := g.emitStmt(b, indent+"\t"); err != nil {
				return err
			}
		}
	}
	fmt.Fprintf(&g.body, "%s}\n", indent)
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
	fmt.Fprintf(&g.body, "%sfor rt.Truthy(%s) {\n", indent, cond)
	for _, b := range s.Children("body") {
		if err := g.emitStmt(b, indent+"\t"); err != nil {
			return err
		}
	}
	fmt.Fprintf(&g.body, "%s}\n", indent)
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
	it := g.fresh("_it")
	fmt.Fprintf(&g.body, "%s{\n", indent)
	fmt.Fprintf(&g.body, "%s\t%s := rt.MustIter(rt.GetIter(%s))\n", indent, it, iter)
	fmt.Fprintf(&g.body, "%s\tfor {\n", indent)
	fmt.Fprintf(&g.body, "%s\t\t_v, _ok := %s.Next()\n", indent, it)
	fmt.Fprintf(&g.body, "%s\t\tif !_ok { break }\n", indent)
	target := s.Child("target")
	switch target.Type() {
	case "Name":
		g.emitNameStore(target.Str("id"), "_v", indent+"\t\t")
	case "Tuple", "List":
		if err := g.emitTupleAssign(target, "_v", indent+"\t\t"); err != nil {
			return err
		}
	default:
		return g.errf(target, "for target not supported: %s", target.Type())
	}
	for _, b := range s.Children("body") {
		if err := g.emitStmt(b, indent+"\t\t"); err != nil {
			return err
		}
	}
	fmt.Fprintf(&g.body, "%s\t}\n", indent)
	fmt.Fprintf(&g.body, "%s}\n", indent)
	return nil
}

// emitStmts emits a block of statements with the given indent. (not
// currently used; kept here for readability of the interface.)
var _ = strings.Join
