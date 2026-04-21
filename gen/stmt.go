package gen

import (
	"fmt"

	"github.com/tamnd/gopygo/pyast"
	"github.com/tamnd/gopygo/types"
)

// emitStmt writes one Python statement as Go. indent is the leading
// whitespace for the opening line; nested blocks add one more \t.
func (g *gen) emitStmt(s pyast.Node, indent string) error {
	switch s.Type() {
	case "Assign":
		return g.emitAssign(s, indent)
	case "AnnAssign":
		return g.emitAnnAssign(s, indent)
	case "AugAssign":
		return g.emitAugAssign(s, indent)
	case "Expr":
		code, _, err := g.emitExpr(s.Child("value"))
		if err != nil {
			return err
		}
		fmt.Fprintf(&g.out, "%s%s\n", indent, code)
		return nil
	case "If":
		return g.emitIf(s, indent)
	case "While":
		return g.emitWhile(s, indent)
	case "For":
		return g.emitFor(s, indent)
	case "Return":
		return g.emitReturn(s, indent)
	case "Break":
		fmt.Fprintf(&g.out, "%sbreak\n", indent)
		return nil
	case "Continue":
		fmt.Fprintf(&g.out, "%scontinue\n", indent)
		return nil
	case "Pass":
		return nil
	case "FunctionDef":
		return g.errf(s, "nested functions not supported in v0.3")
	case "ClassDef":
		return g.errf(s, "classes not supported in v0.3")
	case "Import", "ImportFrom":
		return g.errf(s, "import not supported in v0.3 (only the Python stdlib subset baked in)")
	case "Try", "Raise", "With":
		return g.errf(s, "%s not supported in v0.3", s.Type())
	}
	return g.errf(s, "unsupported statement: %s", s.Type())
}

func (g *gen) emitAssign(s pyast.Node, indent string) error {
	targets := s.Children("targets")
	if len(targets) != 1 {
		return g.errf(s, "multi-target assignment not supported in v0.3")
	}
	tgt := targets[0]
	vs, vt, err := g.emitExpr(s.Child("value"))
	if err != nil {
		return err
	}
	if _, ok := vt.(types.TNone); ok {
		return g.errf(s, "cannot bind None to a variable in v0.3 (would be untyped)")
	}
	switch tgt.Type() {
	case "Name":
		name := tgt.Str("id")
		if prev, ok := g.scope.lookup(name); ok {
			if !types.Equal(prev, vt) {
				if types.IsNumeric(prev) && types.IsNumeric(vt) {
					vs = cast(vs, vt, prev)
				} else {
					return g.errf(s, "cannot reassign %s from %s to %s", name, prev, vt)
				}
			}
			fmt.Fprintf(&g.out, "%s%s = %s\n", indent, name, vs)
			return nil
		}
		g.scope.names[name] = vt
		fmt.Fprintf(&g.out, "%svar %s %s = %s\n", indent, name, vt.Go(), vs)
		return nil
	case "Subscript":
		cs, _, err := g.emitExpr(tgt)
		if err != nil {
			return err
		}
		fmt.Fprintf(&g.out, "%s%s = %s\n", indent, cs, vs)
		return nil
	}
	return g.errf(tgt, "unsupported assignment target: %s", tgt.Type())
}

func (g *gen) emitAnnAssign(s pyast.Node, indent string) error {
	tgt := s.Child("target")
	if tgt.Type() != "Name" {
		return g.errf(s, "annotated assignment only supports plain names in v0.3")
	}
	name := tgt.Str("id")
	ann := s.Child("annotation")
	t, err := g.parseAnnotation(ann)
	if err != nil {
		return err
	}
	val := s.Child("value")
	if val == nil {
		// `x: list[int]` — declare zero value.
		if _, ok := g.scope.names[name]; ok {
			return g.errf(s, "redeclaration of %s", name)
		}
		g.scope.names[name] = t
		fmt.Fprintf(&g.out, "%svar %s %s\n", indent, name, t.Go())
		return nil
	}
	vs, vt, err := g.emitExpr(val)
	if err != nil {
		return err
	}
	if !types.Equal(vt, t) {
		if types.IsNumeric(vt) && types.IsNumeric(t) {
			vs = cast(vs, vt, t)
		} else {
			return g.errf(s, "annotation %s does not match value type %s", t, vt)
		}
	}
	g.scope.names[name] = t
	fmt.Fprintf(&g.out, "%svar %s %s = %s\n", indent, name, t.Go(), vs)
	return nil
}

func (g *gen) emitAugAssign(s pyast.Node, indent string) error {
	tgt := s.Child("target")
	if tgt.Type() != "Name" {
		return g.errf(s, "augmented assignment only supports plain names in v0.3")
	}
	name := tgt.Str("id")
	lt, ok := g.scope.lookup(name)
	if !ok {
		return g.errf(s, "undefined name: %s", name)
	}
	vs, vt, err := g.emitExpr(s.Child("value"))
	if err != nil {
		return err
	}
	op := s.Child("op").Type()
	goOp := ""
	switch op {
	case "Add":
		goOp = "+="
	case "Sub":
		goOp = "-="
	case "Mult":
		goOp = "*="
	case "Div":
		if _, isF := lt.(types.TFloat); !isF {
			return g.errf(s, "/= produces float; LHS %s is not float", lt)
		}
		goOp = "/="
	case "FloorDiv":
		if _, isI := lt.(types.TInt); !isI {
			return g.errf(s, "//= on %s not supported in v0.3", lt)
		}
		goOp = "/="
	case "Mod":
		if _, isI := lt.(types.TInt); !isI {
			return g.errf(s, "%%= on %s not supported in v0.3", lt)
		}
		goOp = "%="
	default:
		return g.errf(s, "unsupported aug op: %s", op)
	}
	if !types.Equal(lt, vt) {
		if types.IsNumeric(lt) && types.IsNumeric(vt) {
			vs = cast(vs, vt, lt)
		} else {
			return g.errf(s, "aug-assign type mismatch: %s %s= %s", lt, op, vt)
		}
	}
	fmt.Fprintf(&g.out, "%s%s %s %s\n", indent, name, goOp, vs)
	return nil
}

func (g *gen) emitIf(s pyast.Node, indent string) error {
	cs, ct, err := g.emitExpr(s.Child("test"))
	if err != nil {
		return err
	}
	if _, ok := ct.(types.TBool); !ok {
		return g.errf(s, "if condition must be bool, got %s", ct)
	}
	fmt.Fprintf(&g.out, "%sif %s {\n", indent, cs)
	saved := g.scope
	g.scope = newScope(saved)
	for _, b := range s.Children("body") {
		if err := g.emitStmt(b, indent+"\t"); err != nil {
			g.scope = saved
			return err
		}
	}
	g.scope = saved
	orelse := s.Children("orelse")
	if len(orelse) == 1 && orelse[0].Type() == "If" {
		// elif: emit `} else if ... {`
		fmt.Fprintf(&g.out, "%s} else ", indent)
		// Recurse but without leading indent (we already wrote it).
		if err := g.emitElif(orelse[0], indent); err != nil {
			return err
		}
		return nil
	}
	if len(orelse) > 0 {
		fmt.Fprintf(&g.out, "%s} else {\n", indent)
		g.scope = newScope(saved)
		for _, b := range orelse {
			if err := g.emitStmt(b, indent+"\t"); err != nil {
				g.scope = saved
				return err
			}
		}
		g.scope = saved
	}
	fmt.Fprintf(&g.out, "%s}\n", indent)
	return nil
}

func (g *gen) emitElif(s pyast.Node, indent string) error {
	cs, ct, err := g.emitExpr(s.Child("test"))
	if err != nil {
		return err
	}
	if _, ok := ct.(types.TBool); !ok {
		return g.errf(s, "elif condition must be bool, got %s", ct)
	}
	fmt.Fprintf(&g.out, "if %s {\n", cs)
	saved := g.scope
	g.scope = newScope(saved)
	for _, b := range s.Children("body") {
		if err := g.emitStmt(b, indent+"\t"); err != nil {
			g.scope = saved
			return err
		}
	}
	g.scope = saved
	orelse := s.Children("orelse")
	if len(orelse) == 1 && orelse[0].Type() == "If" {
		fmt.Fprintf(&g.out, "%s} else ", indent)
		return g.emitElif(orelse[0], indent)
	}
	if len(orelse) > 0 {
		fmt.Fprintf(&g.out, "%s} else {\n", indent)
		g.scope = newScope(saved)
		for _, b := range orelse {
			if err := g.emitStmt(b, indent+"\t"); err != nil {
				g.scope = saved
				return err
			}
		}
		g.scope = saved
	}
	fmt.Fprintf(&g.out, "%s}\n", indent)
	return nil
}

func (g *gen) emitWhile(s pyast.Node, indent string) error {
	if len(s.Children("orelse")) > 0 {
		return g.errf(s, "while-else not supported in v0.3")
	}
	cs, ct, err := g.emitExpr(s.Child("test"))
	if err != nil {
		return err
	}
	if _, ok := ct.(types.TBool); !ok {
		return g.errf(s, "while condition must be bool, got %s", ct)
	}
	fmt.Fprintf(&g.out, "%sfor %s {\n", indent, cs)
	saved := g.scope
	g.scope = newScope(saved)
	for _, b := range s.Children("body") {
		if err := g.emitStmt(b, indent+"\t"); err != nil {
			g.scope = saved
			return err
		}
	}
	g.scope = saved
	fmt.Fprintf(&g.out, "%s}\n", indent)
	return nil
}

func (g *gen) emitFor(s pyast.Node, indent string) error {
	if len(s.Children("orelse")) > 0 {
		return g.errf(s, "for-else not supported in v0.3")
	}
	tgt := s.Child("target")
	if tgt.Type() != "Name" {
		return g.errf(s, "for target must be a plain name in v0.3")
	}
	name := tgt.Str("id")
	iter := s.Child("iter")

	saved := g.scope
	g.scope = newScope(saved)
	defer func() { g.scope = saved }()

	// Special-case range(...).
	if iter.Type() == "Call" && iter.Child("func").Type() == "Name" && iter.Child("func").Str("id") == "range" {
		args := iter.Children("args")
		var start, stop, step string = "0", "", "1"
		switch len(args) {
		case 1:
			s1, t1, err := g.emitExpr(args[0])
			if err != nil {
				return err
			}
			if _, ok := t1.(types.TInt); !ok {
				return g.errf(iter, "range() arg must be int")
			}
			stop = s1
		case 2, 3:
			s1, t1, err := g.emitExpr(args[0])
			if err != nil {
				return err
			}
			s2, t2, err := g.emitExpr(args[1])
			if err != nil {
				return err
			}
			if _, ok := t1.(types.TInt); !ok {
				return g.errf(iter, "range() args must be int")
			}
			if _, ok := t2.(types.TInt); !ok {
				return g.errf(iter, "range() args must be int")
			}
			start, stop = s1, s2
			if len(args) == 3 {
				s3, t3, err := g.emitExpr(args[2])
				if err != nil {
					return err
				}
				if _, ok := t3.(types.TInt); !ok {
					return g.errf(iter, "range() step must be int")
				}
				step = s3
			}
		default:
			return g.errf(iter, "range() takes 1 to 3 arguments")
		}
		g.scope.names[name] = types.TInt{}
		cmp := "<"
		// If the step is a negative literal, flip the comparison.
		if len(step) > 0 && step[0] == '-' {
			cmp = ">"
		}
		inc := name + " += " + step
		if step == "1" {
			inc = name + "++"
		} else if step == "-1" {
			inc = name + "--"
		}
		fmt.Fprintf(&g.out, "%sfor %s := int64(%s); %s %s %s; %s {\n",
			indent, name, start, name, cmp, stop, inc)
		for _, b := range s.Children("body") {
			if err := g.emitStmt(b, indent+"\t"); err != nil {
				return err
			}
		}
		fmt.Fprintf(&g.out, "%s}\n", indent)
		return nil
	}

	// Generic `for x in seq`.
	is, it, err := g.emitExpr(iter)
	if err != nil {
		return err
	}
	var elem types.Type
	switch x := it.(type) {
	case types.TList:
		elem = x.Elem
	case types.TStr:
		elem = types.TStr{}
		// Go's `for _, r := range s` yields runes; we want 1-char strs.
		// Fall through via a dedicated emission below.
	case types.TDict:
		elem = x.K
	default:
		return g.errf(iter, "cannot iterate over %s", it)
	}
	g.scope.names[name] = elem

	if _, ok := it.(types.TStr); ok {
		fmt.Fprintf(&g.out, "%sfor _, _r := range %s {\n", indent, is)
		fmt.Fprintf(&g.out, "%s\t%s := string(_r)\n", indent, name)
	} else if _, ok := it.(types.TDict); ok {
		fmt.Fprintf(&g.out, "%sfor %s := range %s {\n", indent, name, is)
	} else {
		fmt.Fprintf(&g.out, "%sfor _, %s := range %s {\n", indent, name, is)
	}
	for _, b := range s.Children("body") {
		if err := g.emitStmt(b, indent+"\t"); err != nil {
			return err
		}
	}
	fmt.Fprintf(&g.out, "%s}\n", indent)
	return nil
}

func (g *gen) emitReturn(s pyast.Node, indent string) error {
	val := s.Child("value")
	if val == nil {
		if _, ok := g.returnType.(types.TNone); !ok {
			return g.errf(s, "return without value in function returning %s", g.returnType)
		}
		fmt.Fprintf(&g.out, "%sreturn\n", indent)
		return nil
	}
	vs, vt, err := g.emitExpr(val)
	if err != nil {
		return err
	}
	if _, ok := g.returnType.(types.TNone); ok {
		return g.errf(s, "return value in function declared to return None")
	}
	if !types.Equal(vt, g.returnType) {
		if types.IsNumeric(vt) && types.IsNumeric(g.returnType) {
			vs = cast(vs, vt, g.returnType)
		} else {
			return g.errf(s, "return type mismatch: declared %s, got %s", g.returnType, vt)
		}
	}
	fmt.Fprintf(&g.out, "%sreturn %s\n", indent, vs)
	return nil
}
