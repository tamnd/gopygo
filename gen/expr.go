package gen

import (
	"bytes"
	"fmt"
	"strconv"
	"strings"

	"github.com/tamnd/gopygo/pyast"
	"github.com/tamnd/gopygo/types"
)

// emitExpr returns a Go source fragment plus the inferred gopygo type
// for the Python expression n. It never returns TAny unless the user
// annotated a value as Any — the transpiler fails instead of guessing.
func (g *gen) emitExpr(n pyast.Node) (string, types.Type, error) {
	switch n.Type() {
	case "Constant":
		return g.emitConstant(n)
	case "Name":
		return g.emitName(n)
	case "BinOp":
		return g.emitBinOp(n)
	case "UnaryOp":
		return g.emitUnaryOp(n)
	case "BoolOp":
		return g.emitBoolOp(n)
	case "Compare":
		return g.emitCompare(n)
	case "Call":
		return g.emitCall(n)
	case "Subscript":
		return g.emitSubscript(n)
	case "JoinedStr":
		return g.emitFString(n)
	case "FormattedValue":
		return g.emitExpr(n.Child("value"))
	case "IfExp":
		return g.emitIfExp(n)
	case "List":
		return g.emitListLit(n)
	case "Tuple":
		return g.emitTupleLit(n)
	case "Dict":
		return g.emitDictLit(n)
	}
	return "", nil, g.errf(n, "unsupported expression: %s", n.Type())
}

func (g *gen) emitConstant(n pyast.Node) (string, types.Type, error) {
	kind := n.Str("_vkind")
	v := n.Raw("value")
	switch kind {
	case "none":
		return "struct{}{}", types.TNone{}, nil
	case "bool":
		if v.(bool) {
			return "true", types.TBool{}, nil
		}
		return "false", types.TBool{}, nil
	case "int":
		return strconv.FormatInt(int64(v.(float64)), 10), types.TInt{}, nil
	case "float":
		return strconv.FormatFloat(v.(float64), 'g', -1, 64), types.TFloat{}, nil
	case "str":
		return strconv.Quote(v.(string)), types.TStr{}, nil
	}
	return "", nil, g.errf(n, "unsupported constant kind: %q", kind)
}

func (g *gen) emitName(n pyast.Node) (string, types.Type, error) {
	id := n.Str("id")
	switch id {
	case "True":
		return "true", types.TBool{}, nil
	case "False":
		return "false", types.TBool{}, nil
	case "None":
		return "struct{}{}", types.TNone{}, nil
	}
	t, ok := g.scope.lookup(id)
	if !ok {
		return "", nil, g.errf(n, "undefined name: %s", id)
	}
	return id, t, nil
}

func (g *gen) emitBinOp(n pyast.Node) (string, types.Type, error) {
	op := n.Child("op").Type()
	ls, lt, err := g.emitExpr(n.Child("left"))
	if err != nil {
		return "", nil, err
	}
	rs, rt, err := g.emitExpr(n.Child("right"))
	if err != nil {
		return "", nil, err
	}
	// String + string.
	if op == "Add" {
		if _, ok := lt.(types.TStr); ok {
			if _, ok2 := rt.(types.TStr); ok2 {
				return ls + " + " + rs, types.TStr{}, nil
			}
		}
	}
	// Numeric operators.
	wide := types.Widen(lt, rt)
	if wide == nil {
		return "", nil, g.errf(n, "binary %s on %s and %s", op, lt, rt)
	}
	ls = cast(ls, lt, wide)
	rs = cast(rs, rt, wide)
	switch op {
	case "Add":
		return "(" + ls + " + " + rs + ")", wide, nil
	case "Sub":
		return "(" + ls + " - " + rs + ")", wide, nil
	case "Mult":
		return "(" + ls + " * " + rs + ")", wide, nil
	case "Div":
		// Python 3 "/" is always float.
		ls = cast(ls, wide, types.TFloat{})
		rs = cast(rs, wide, types.TFloat{})
		return "(" + ls + " / " + rs + ")", types.TFloat{}, nil
	case "FloorDiv":
		if _, ok := wide.(types.TInt); ok {
			return "(" + ls + " / " + rs + ")", types.TInt{}, nil
		}
		g.need("math")
		return "math.Floor(" + ls + " / " + rs + ")", types.TFloat{}, nil
	case "Mod":
		if _, ok := wide.(types.TInt); ok {
			return "(" + ls + " % " + rs + ")", types.TInt{}, nil
		}
		g.need("math")
		return "math.Mod(" + ls + ", " + rs + ")", types.TFloat{}, nil
	case "Pow":
		g.need("math")
		ls = cast(ls, wide, types.TFloat{})
		rs = cast(rs, wide, types.TFloat{})
		if _, ok := wide.(types.TInt); ok {
			return "int64(math.Pow(" + ls + ", " + rs + "))", types.TInt{}, nil
		}
		return "math.Pow(" + ls + ", " + rs + ")", types.TFloat{}, nil
	}
	return "", nil, g.errf(n, "unsupported binary op: %s", op)
}

func cast(code string, from, to types.Type) string {
	if types.Equal(from, to) {
		return code
	}
	return to.Go() + "(" + code + ")"
}

func (g *gen) emitUnaryOp(n pyast.Node) (string, types.Type, error) {
	op := n.Child("op").Type()
	s, t, err := g.emitExpr(n.Child("operand"))
	if err != nil {
		return "", nil, err
	}
	switch op {
	case "USub":
		if !types.IsNumeric(t) {
			return "", nil, g.errf(n, "unary - on %s", t)
		}
		return "-" + s, t, nil
	case "UAdd":
		return s, t, nil
	case "Not":
		if _, ok := t.(types.TBool); !ok {
			return "", nil, g.errf(n, "not on %s (need bool)", t)
		}
		return "!" + s, types.TBool{}, nil
	}
	return "", nil, g.errf(n, "unsupported unary op: %s", op)
}

func (g *gen) emitBoolOp(n pyast.Node) (string, types.Type, error) {
	op := n.Child("op").Type()
	var glue string
	switch op {
	case "And":
		glue = " && "
	case "Or":
		glue = " || "
	default:
		return "", nil, g.errf(n, "unsupported bool op: %s", op)
	}
	parts := []string{}
	for _, v := range n.Children("values") {
		s, t, err := g.emitExpr(v)
		if err != nil {
			return "", nil, err
		}
		if _, ok := t.(types.TBool); !ok {
			return "", nil, g.errf(v, "%s operand must be bool, got %s", op, t)
		}
		parts = append(parts, s)
	}
	return "(" + strings.Join(parts, glue) + ")", types.TBool{}, nil
}

func (g *gen) emitCompare(n pyast.Node) (string, types.Type, error) {
	left := n.Child("left")
	ops := n.Children("ops")
	comps := n.Children("comparators")
	// Evaluate each operand once. For chained compares a < b < c we
	// emit (a < b) && (b < c) after binding b to a temporary only when
	// it has side effects; for v0.3 we just re-emit — operands are
	// cheap in the typed subset.
	var parts []string
	prevCode, prevT, err := g.emitExpr(left)
	if err != nil {
		return "", nil, err
	}
	for i, op := range ops {
		rightCode, rightT, err := g.emitExpr(comps[i])
		if err != nil {
			return "", nil, err
		}
		code, err := g.compareOne(n, op.Type(), prevCode, prevT, rightCode, rightT)
		if err != nil {
			return "", nil, err
		}
		parts = append(parts, code)
		prevCode, prevT = rightCode, rightT
	}
	if len(parts) == 1 {
		return parts[0], types.TBool{}, nil
	}
	return "(" + strings.Join(parts, " && ") + ")", types.TBool{}, nil
}

func (g *gen) compareOne(n pyast.Node, op, ls string, lt types.Type, rs string, rt types.Type) (string, error) {
	goOp := ""
	switch op {
	case "Eq":
		goOp = "=="
	case "NotEq":
		goOp = "!="
	case "Lt":
		goOp = "<"
	case "LtE":
		goOp = "<="
	case "Gt":
		goOp = ">"
	case "GtE":
		goOp = ">="
	default:
		return "", g.errf(n, "unsupported compare op: %s", op)
	}
	if types.IsNumeric(lt) && types.IsNumeric(rt) {
		w := types.Widen(lt, rt)
		return "(" + cast(ls, lt, w) + " " + goOp + " " + cast(rs, rt, w) + ")", nil
	}
	if types.Equal(lt, rt) {
		return "(" + ls + " " + goOp + " " + rs + ")", nil
	}
	return "", g.errf(n, "cannot compare %s and %s", lt, rt)
}

func (g *gen) emitCall(n pyast.Node) (string, types.Type, error) {
	fn := n.Child("func")
	args := n.Children("args")
	if fn.Type() == "Name" {
		if s, t, ok, err := g.emitBuiltin(n, fn.Str("id"), args); ok {
			return s, t, err
		}
	}
	fs, ft, err := g.emitExpr(fn)
	if err != nil {
		return "", nil, err
	}
	tf, ok := ft.(types.TFunc)
	if !ok {
		return "", nil, g.errf(n, "cannot call non-function of type %s", ft)
	}
	if len(args) != len(tf.Params) {
		return "", nil, g.errf(n, "%s expects %d args, got %d", tf.Name, len(tf.Params), len(args))
	}
	var buf bytes.Buffer
	buf.WriteString(fs)
	buf.WriteString("(")
	for i, a := range args {
		if i > 0 {
			buf.WriteString(", ")
		}
		as, at, err := g.emitExpr(a)
		if err != nil {
			return "", nil, err
		}
		if !types.Equal(at, tf.Params[i]) {
			if types.IsNumeric(at) && types.IsNumeric(tf.Params[i]) {
				as = cast(as, at, tf.Params[i])
			} else {
				return "", nil, g.errf(a, "argument %d to %s: expected %s, got %s", i, tf.Name, tf.Params[i], at)
			}
		}
		buf.WriteString(as)
	}
	buf.WriteString(")")
	return buf.String(), tf.Return, nil
}

func (g *gen) emitBuiltin(n pyast.Node, name string, args []pyast.Node) (string, types.Type, bool, error) {
	switch name {
	case "print":
		g.need("fmt")
		g.need("strconv")
		g.need("strings")
		g.needPyPrint = true
		var buf bytes.Buffer
		buf.WriteString("pyPrintln(")
		for i, a := range args {
			if i > 0 {
				buf.WriteString(", ")
			}
			s, _, err := g.emitExpr(a)
			if err != nil {
				return "", nil, true, err
			}
			buf.WriteString(s)
		}
		buf.WriteString(")")
		return buf.String(), types.TNone{}, true, nil
	case "len":
		if len(args) != 1 {
			return "", nil, true, g.errf(n, "len takes 1 argument")
		}
		s, t, err := g.emitExpr(args[0])
		if err != nil {
			return "", nil, true, err
		}
		switch t.(type) {
		case types.TStr, types.TList, types.TDict:
			return "int64(len(" + s + "))", types.TInt{}, true, nil
		}
		return "", nil, true, g.errf(n, "len on %s", t)
	case "abs":
		if len(args) != 1 {
			return "", nil, true, g.errf(n, "abs takes 1 argument")
		}
		s, t, err := g.emitExpr(args[0])
		if err != nil {
			return "", nil, true, err
		}
		switch t.(type) {
		case types.TInt:
			g.needAbsInt = true
			return "absInt(" + s + ")", types.TInt{}, true, nil
		case types.TFloat:
			g.need("math")
			return "math.Abs(" + s + ")", types.TFloat{}, true, nil
		}
		return "", nil, true, g.errf(n, "abs on %s", t)
	case "min", "max":
		if len(args) < 1 {
			return "", nil, true, g.errf(n, "%s needs at least 1 argument", name)
		}
		g.needMinMaxI = true
		var parts []string
		var resT types.Type
		for _, a := range args {
			s, t, err := g.emitExpr(a)
			if err != nil {
				return "", nil, true, err
			}
			if _, ok := t.(types.TInt); !ok {
				return "", nil, true, g.errf(a, "%s: only int supported in v0.3, got %s", name, t)
			}
			resT = t
			parts = append(parts, s)
		}
		fn := "minInt"
		if name == "max" {
			fn = "maxInt"
		}
		return fn + "(" + strings.Join(parts, ", ") + ")", resT, true, nil
	case "int":
		if len(args) != 1 {
			return "", nil, true, g.errf(n, "int takes 1 argument")
		}
		s, t, err := g.emitExpr(args[0])
		if err != nil {
			return "", nil, true, err
		}
		switch t.(type) {
		case types.TInt:
			return s, types.TInt{}, true, nil
		case types.TFloat:
			return "int64(" + s + ")", types.TInt{}, true, nil
		case types.TBool:
			return "func() int64 { if " + s + " { return 1 }; return 0 }()", types.TInt{}, true, nil
		case types.TStr:
			g.needAtoi = true
			g.need("strconv")
			return "mustAtoi64(" + s + ")", types.TInt{}, true, nil
		}
		return "", nil, true, g.errf(n, "int() on %s", t)
	case "float":
		if len(args) != 1 {
			return "", nil, true, g.errf(n, "float takes 1 argument")
		}
		s, t, err := g.emitExpr(args[0])
		if err != nil {
			return "", nil, true, err
		}
		if !types.IsNumeric(t) {
			return "", nil, true, g.errf(n, "float() on %s", t)
		}
		return cast(s, t, types.TFloat{}), types.TFloat{}, true, nil
	case "str":
		if len(args) != 1 {
			return "", nil, true, g.errf(n, "str takes 1 argument")
		}
		s, t, err := g.emitExpr(args[0])
		if err != nil {
			return "", nil, true, err
		}
		g.need("fmt")
		switch t.(type) {
		case types.TStr:
			return s, types.TStr{}, true, nil
		}
		return "fmt.Sprint(" + s + ")", types.TStr{}, true, nil
	case "bool":
		if len(args) != 1 {
			return "", nil, true, g.errf(n, "bool takes 1 argument")
		}
		s, t, err := g.emitExpr(args[0])
		if err != nil {
			return "", nil, true, err
		}
		switch t.(type) {
		case types.TBool:
			return s, types.TBool{}, true, nil
		case types.TInt:
			return "(" + s + " != 0)", types.TBool{}, true, nil
		case types.TFloat:
			return "(" + s + " != 0)", types.TBool{}, true, nil
		case types.TStr:
			return "(" + s + " != \"\")", types.TBool{}, true, nil
		}
		return "", nil, true, g.errf(n, "bool() on %s", t)
	case "range":
		// range is only valid inside a for; emitFor handles it. If it
		// escapes into an expression position we reject.
		return "", nil, true, g.errf(n, "range() is only supported as a for-loop iterable in v0.3")
	}
	return "", nil, false, nil
}

func (g *gen) emitSubscript(n pyast.Node) (string, types.Type, error) {
	vs, vt, err := g.emitExpr(n.Child("value"))
	if err != nil {
		return "", nil, err
	}
	slice := n.Child("slice")
	if slice.Type() == "Slice" {
		return "", nil, g.errf(n, "slicing not supported in v0.3")
	}
	is, it, err := g.emitExpr(slice)
	if err != nil {
		return "", nil, err
	}
	switch x := vt.(type) {
	case types.TList:
		if _, ok := it.(types.TInt); !ok {
			return "", nil, g.errf(n, "list index must be int, got %s", it)
		}
		return vs + "[" + is + "]", x.Elem, nil
	case types.TStr:
		if _, ok := it.(types.TInt); !ok {
			return "", nil, g.errf(n, "str index must be int, got %s", it)
		}
		// Python s[i] returns a 1-char str; emit string(s[i:i+1])
		// but that doubles work, so slice via indices.
		return "string([]rune(" + vs + ")[" + is + "])", types.TStr{}, nil
	case types.TDict:
		if !types.Equal(x.K, it) {
			return "", nil, g.errf(n, "dict key type mismatch: want %s, got %s", x.K, it)
		}
		return vs + "[" + is + "]", x.V, nil
	}
	return "", nil, g.errf(n, "subscript on %s", vt)
}

func (g *gen) emitFString(n pyast.Node) (string, types.Type, error) {
	g.need("fmt")
	var fmtStr bytes.Buffer
	var args []string
	for _, p := range n.Children("values") {
		switch p.Type() {
		case "Constant":
			v, _ := p.Raw("value").(string)
			fmtStr.WriteString(strings.ReplaceAll(v, "%", "%%"))
		case "FormattedValue":
			s, t, err := g.emitExpr(p.Child("value"))
			if err != nil {
				return "", nil, err
			}
			fmtStr.WriteString(verbFor(t))
			args = append(args, s)
		default:
			return "", nil, g.errf(p, "unsupported f-string part: %s", p.Type())
		}
	}
	call := "fmt.Sprintf(" + strconv.Quote(fmtStr.String())
	if len(args) > 0 {
		call += ", " + strings.Join(args, ", ")
	}
	call += ")"
	return call, types.TStr{}, nil
}

func verbFor(t types.Type) string {
	switch t.(type) {
	case types.TInt:
		return "%d"
	case types.TFloat:
		return "%g"
	case types.TBool:
		return "%t"
	case types.TStr:
		return "%s"
	}
	return "%v"
}

func (g *gen) emitIfExp(n pyast.Node) (string, types.Type, error) {
	cs, ct, err := g.emitExpr(n.Child("test"))
	if err != nil {
		return "", nil, err
	}
	if _, ok := ct.(types.TBool); !ok {
		return "", nil, g.errf(n, "ternary condition must be bool, got %s", ct)
	}
	ts, tt, err := g.emitExpr(n.Child("body"))
	if err != nil {
		return "", nil, err
	}
	fs, ft, err := g.emitExpr(n.Child("orelse"))
	if err != nil {
		return "", nil, err
	}
	out := tt
	if types.IsNumeric(tt) && types.IsNumeric(ft) {
		out = types.Widen(tt, ft)
		ts = cast(ts, tt, out)
		fs = cast(fs, ft, out)
	} else if !types.Equal(tt, ft) {
		return "", nil, g.errf(n, "ternary branches disagree: %s vs %s", tt, ft)
	}
	code := fmt.Sprintf("func() %s { if %s { return %s }; return %s }()", out.Go(), cs, ts, fs)
	return code, out, nil
}

func (g *gen) emitListLit(n pyast.Node) (string, types.Type, error) {
	elts := n.Children("elts")
	if len(elts) == 0 {
		return "", nil, g.errf(n, "empty list literal needs a type annotation (not supported in v0.3 expression context)")
	}
	var parts []string
	var et types.Type
	for i, e := range elts {
		s, t, err := g.emitExpr(e)
		if err != nil {
			return "", nil, err
		}
		if i == 0 {
			et = t
		} else if !types.Equal(t, et) {
			if types.IsNumeric(t) && types.IsNumeric(et) {
				et = types.Widen(et, t)
			} else {
				return "", nil, g.errf(e, "list element type %s mismatches %s", t, et)
			}
		}
		parts = append(parts, s)
	}
	// second pass to cast if widened
	for i, e := range elts {
		_, t, _ := g.emitExpr(e)
		parts[i] = cast(parts[i], t, et)
	}
	lt := types.TList{Elem: et}
	return lt.Go() + "{" + strings.Join(parts, ", ") + "}", lt, nil
}

func (g *gen) emitTupleLit(n pyast.Node) (string, types.Type, error) {
	return "", nil, g.errf(n, "tuple literal not supported in expression position in v0.3 (only as multi-return in unpacking)")
}

func (g *gen) emitDictLit(n pyast.Node) (string, types.Type, error) {
	keys := n.Children("keys")
	vals := n.Children("values")
	if len(keys) == 0 {
		return "", nil, g.errf(n, "empty dict literal needs a type annotation (not supported in v0.3 expression context)")
	}
	var kt, vt types.Type
	var kparts, vparts []string
	for i, k := range keys {
		ks, kti, err := g.emitExpr(k)
		if err != nil {
			return "", nil, err
		}
		vs, vti, err := g.emitExpr(vals[i])
		if err != nil {
			return "", nil, err
		}
		if i == 0 {
			kt, vt = kti, vti
		} else {
			if !types.Equal(kti, kt) {
				return "", nil, g.errf(k, "dict key type %s mismatches %s", kti, kt)
			}
			if !types.Equal(vti, vt) {
				return "", nil, g.errf(vals[i], "dict value type %s mismatches %s", vti, vt)
			}
		}
		kparts = append(kparts, ks)
		vparts = append(vparts, vs)
	}
	dt := types.TDict{K: kt, V: vt}
	var buf bytes.Buffer
	buf.WriteString(dt.Go())
	buf.WriteString("{")
	for i := range kparts {
		if i > 0 {
			buf.WriteString(", ")
		}
		buf.WriteString(kparts[i])
		buf.WriteString(": ")
		buf.WriteString(vparts[i])
	}
	buf.WriteString("}")
	return buf.String(), dt, nil
}
