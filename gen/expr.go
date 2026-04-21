package gen

import (
	"fmt"
	"strings"

	"github.com/tamnd/gopygo/pyast"
)

// emitExpr returns a Go expression for the Python expression e.
// Fallible ops are wrapped in rt.Must so callers can splice the
// result directly into any Go expression position.
func (g *gen) emitExpr(e pyast.Node) (string, error) {
	switch e.Type() {
	case "Constant":
		return constExpr(e.Raw("value")), nil

	case "Name":
		return g.nameExpr(e.Str("id")), nil

	case "BinOp":
		left, err := g.emitExpr(e.Child("left"))
		if err != nil {
			return "", err
		}
		right, err := g.emitExpr(e.Child("right"))
		if err != nil {
			return "", err
		}
		fn, err := binOpFunc(e.Child("op").Type())
		if err != nil {
			return "", g.errf(e, "%v", err)
		}
		return fmt.Sprintf("rt.Must(rt.%s(%s, %s))", fn, left, right), nil

	case "UnaryOp":
		operand, err := g.emitExpr(e.Child("operand"))
		if err != nil {
			return "", err
		}
		switch e.Child("op").Type() {
		case "USub":
			return fmt.Sprintf("rt.Must(rt.Neg(%s))", operand), nil
		case "UAdd":
			return operand, nil
		case "Not":
			return fmt.Sprintf("rt.Not(%s)", operand), nil
		}
		return "", g.errf(e, "unsupported UnaryOp: %s", e.Child("op").Type())

	case "BoolOp":
		return g.emitBoolOp(e)

	case "Compare":
		return g.emitCompare(e)

	case "Call":
		return g.emitCall(e)

	case "IfExp":
		cond, err := g.emitExpr(e.Child("test"))
		if err != nil {
			return "", err
		}
		t, err := g.emitExpr(e.Child("body"))
		if err != nil {
			return "", err
		}
		f, err := g.emitExpr(e.Child("orelse"))
		if err != nil {
			return "", err
		}
		return fmt.Sprintf("func() rt.Value { if rt.Truthy(%s) { return %s }; return %s }()", cond, t, f), nil

	case "Tuple":
		parts, err := g.emitArgs(e.Children("elts"))
		if err != nil {
			return "", err
		}
		return fmt.Sprintf("rt.NewTuple(%s)", strings.Join(parts, ", ")), nil

	case "List":
		parts, err := g.emitArgs(e.Children("elts"))
		if err != nil {
			return "", err
		}
		return fmt.Sprintf("&rt.List{V: []rt.Value{%s}}", strings.Join(parts, ", ")), nil

	case "Dict":
		keys := e.Children("keys")
		vals := e.Children("values")
		if len(keys) != len(vals) {
			return "", g.errf(e, "dict keys/values mismatch")
		}
		var kv []string
		for i := range keys {
			k, err := g.emitExpr(keys[i])
			if err != nil {
				return "", err
			}
			v, err := g.emitExpr(vals[i])
			if err != nil {
				return "", err
			}
			kv = append(kv, k, v)
		}
		return fmt.Sprintf("rt.NewDictFromPairs(%s)", strings.Join(kv, ", ")), nil

	case "Subscript":
		val, err := g.emitExpr(e.Child("value"))
		if err != nil {
			return "", err
		}
		idx, err := g.emitExpr(e.Child("slice"))
		if err != nil {
			return "", err
		}
		return fmt.Sprintf("rt.Must(rt.GetItem(%s, %s))", val, idx), nil

	case "JoinedStr":
		return g.emitFString(e)

	case "FormattedValue":
		v, err := g.emitExpr(e.Child("value"))
		if err != nil {
			return "", err
		}
		return fmt.Sprintf("rt.Format(%s)", v), nil
	}
	return "", g.errf(e, "unsupported expr: %s", e.Type())
}

func (g *gen) emitArgs(nodes []pyast.Node) ([]string, error) {
	out := make([]string, len(nodes))
	for i, n := range nodes {
		s, err := g.emitExpr(n)
		if err != nil {
			return nil, err
		}
		out[i] = s
	}
	return out, nil
}

func (g *gen) emitBoolOp(e pyast.Node) (string, error) {
	values := e.Children("values")
	if len(values) < 2 {
		return "", g.errf(e, "BoolOp with <2 values")
	}
	parts, err := g.emitArgs(values)
	if err != nil {
		return "", err
	}
	and := e.Child("op").Type() == "And"
	// Short-circuit via a closure. Value of `a and b` is `b` if a is truthy.
	var b strings.Builder
	b.WriteString("func() rt.Value {\n")
	for i := 0; i < len(parts)-1; i++ {
		if and {
			fmt.Fprintf(&b, "\tif !rt.Truthy(%s) { return %s }\n", parts[i], parts[i])
		} else {
			fmt.Fprintf(&b, "\tif rt.Truthy(%s) { return %s }\n", parts[i], parts[i])
		}
	}
	fmt.Fprintf(&b, "\treturn %s\n", parts[len(parts)-1])
	b.WriteString("}()")
	return b.String(), nil
}

func (g *gen) emitCompare(e pyast.Node) (string, error) {
	left, err := g.emitExpr(e.Child("left"))
	if err != nil {
		return "", err
	}
	ops := e.Children("ops")
	comparators := e.Children("comparators")
	if len(ops) != len(comparators) {
		return "", g.errf(e, "Compare ops/comparators mismatch")
	}
	// Non-chained: emit a single compareCall.
	if len(ops) == 1 {
		rhs, err := g.emitExpr(comparators[0])
		if err != nil {
			return "", err
		}
		return compareCall(ops[0].Type(), left, rhs), nil
	}
	// Chained a OP1 b OP2 c: one IIFE binds every sub-expression once,
	// then short-circuits through each pairwise compare.
	var b strings.Builder
	b.WriteString("func() rt.Value {\n")
	lhs := g.fresh("_cl")
	fmt.Fprintf(&b, "\t%s := %s\n", lhs, left)
	prev := lhs
	for i, op := range ops {
		rhs, err := g.emitExpr(comparators[i])
		if err != nil {
			return "", err
		}
		cur := g.fresh("_cr")
		fmt.Fprintf(&b, "\t%s := %s\n", cur, rhs)
		fmt.Fprintf(&b, "\tif !rt.Truthy(%s) { return rt.False }\n",
			compareCall(op.Type(), prev, cur))
		prev = cur
	}
	b.WriteString("\treturn rt.True\n}()")
	return b.String(), nil
}

// compareCall returns an expression whose value is a *rt.Bool.
func compareCall(op, lhs, rhs string) string {
	switch op {
	case "Eq":
		return fmt.Sprintf("rt.Boxed(rt.Equal(%s, %s))", lhs, rhs)
	case "NotEq":
		return fmt.Sprintf("rt.Boxed(!rt.Equal(%s, %s))", lhs, rhs)
	case "Lt":
		return fmt.Sprintf("rt.Must(rt.Compare(0, %s, %s))", lhs, rhs)
	case "LtE":
		return fmt.Sprintf("rt.Must(rt.Compare(1, %s, %s))", lhs, rhs)
	case "Gt":
		return fmt.Sprintf("rt.Must(rt.Compare(4, %s, %s))", lhs, rhs)
	case "GtE":
		return fmt.Sprintf("rt.Must(rt.Compare(5, %s, %s))", lhs, rhs)
	case "Is":
		return fmt.Sprintf("rt.Is(%s, %s)", lhs, rhs)
	case "IsNot":
		return fmt.Sprintf("rt.IsNot(%s, %s)", lhs, rhs)
	case "In":
		return fmt.Sprintf("rt.Must(rt.Contains(%s, %s))", rhs, lhs)
	case "NotIn":
		return fmt.Sprintf("rt.Not(rt.Must(rt.Contains(%s, %s)))", rhs, lhs)
	}
	return "/* unknown cmpop */ rt.False"
}

func (g *gen) emitCall(e pyast.Node) (string, error) {
	if len(e.Children("keywords")) > 0 {
		return "", g.errf(e, "keyword arguments not supported")
	}
	fn, err := g.emitExpr(e.Child("func"))
	if err != nil {
		return "", err
	}
	args, err := g.emitArgs(e.Children("args"))
	if err != nil {
		return "", err
	}
	all := []string{fn}
	all = append(all, args...)
	return fmt.Sprintf("rt.Must(rt.Call(%s))", strings.Join(all, ", ")), nil
}

func (g *gen) emitFString(e pyast.Node) (string, error) {
	parts, err := g.emitArgs(e.Children("values"))
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("rt.ConcatStrings([]rt.Value{%s})", strings.Join(parts, ", ")), nil
}

// binOpFunc maps Python AST BinOp operator class names to the name of
// the runtime function that implements it.
func binOpFunc(op string) (string, error) {
	switch op {
	case "Add":
		return "Add", nil
	case "Sub":
		return "Sub", nil
	case "Mult":
		return "Mul", nil
	case "Div":
		return "TrueDiv", nil
	case "FloorDiv":
		return "FloorDiv", nil
	case "Mod":
		return "Mod", nil
	case "Pow":
		return "Pow", nil
	}
	return "", fmt.Errorf("unsupported binary op: %s", op)
}

// constExpr renders a Python Constant.value as a Go expression that
// produces the right rt.Value.
func constExpr(v any) string {
	switch x := v.(type) {
	case nil:
		return "rt.None"
	case bool:
		if x {
			return "rt.True"
		}
		return "rt.False"
	case string:
		return "rt.NewStr(" + quote(x) + ")"
	case float64:
		// JSON decodes both Python int and float as float64. Distinguish
		// by checking whether it is integral; this collapses float/int
		// apart because Python ints always serialise without a fractional
		// part. ast.Constant keeps them as ints in the JSON helper since
		// json.dumps emits "1" not "1.0" for int values.
		if float64(int64(x)) == x {
			return fmt.Sprintf("rt.NewIntInt(%d)", int64(x))
		}
		return fmt.Sprintf("rt.NewFloat(%v)", x)
	}
	return "/* unsupported constant */ rt.None"
}
