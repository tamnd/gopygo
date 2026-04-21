package transpile

import (
	"fmt"
	"strconv"

	"github.com/tamnd/gopygo/pyc"
)

// emitFunc lowers one code object into a Go function definition,
// appended to g.body.
func (g *gen) emitFunc(c *pyc.Code) error {
	// Find jump targets with a scanner pass; labels are only emitted
	// at offsets that something jumps to, so Go does not complain
	// about unused labels.
	targets, err := scanTargets(c)
	if err != nil {
		return err
	}

	// Pre-allocate names for nested code constants so MAKE_FUNCTION
	// can reference them.
	for _, k := range c.Consts {
		if nested, ok := k.(*pyc.Code); ok {
			if _, seen := g.funcOf[nested]; !seen {
				name := "py_" + sanitise(nested.QualName)
				g.funcOf[nested] = fmt.Sprintf("%s__%d", name, g.idFor(nested))
				g.enqueue(nested)
			}
		}
	}

	w := &g.body
	name := g.funcOf[c]
	fmt.Fprintf(w, "// %s from %s:%d\n", c.QualName, c.Filename, c.FirstLineNo)
	fmt.Fprintf(w, "func %s(args []rt.Value) (rt.Value, error) {\n", name)
	// Locals.
	for i := 0; i < len(c.LocalsPlusNames); i++ {
		fmt.Fprintf(w, "\tvar _L%d rt.Value\n", i)
	}
	for i := 0; i < len(c.LocalsPlusNames); i++ {
		fmt.Fprintf(w, "\t_ = _L%d\n", i)
	}
	// Positional args fill the first ArgCount locals.
	if c.ArgCount > 0 {
		fmt.Fprintf(w, "\tfor i := 0; i < len(args) && i < %d; i++ {\n", c.ArgCount)
		fmt.Fprintf(w, "\t\tswitch i {\n")
		for i := 0; i < c.ArgCount; i++ {
			fmt.Fprintf(w, "\t\tcase %d: _L%d = args[%d]\n", i, i, i)
		}
		fmt.Fprintf(w, "\t\t}\n\t}\n")
	}
	// Stack.
	ss := c.Stacksize
	if ss < 4 {
		ss = 4
	}
	fmt.Fprintf(w, "\tvar stk [%d]rt.Value\n", ss)
	fmt.Fprintf(w, "\tvar sp int\n")
	fmt.Fprintf(w, "\t_ = stk; _ = sp\n")

	// Walk bytecode.
	bc := c.Bytecode
	ip := 0
	for ip < len(bc) {
		if targets[ip] {
			fmt.Fprintf(w, "L%d:\n", ip)
		}
		op := bc[ip]
		arg := bc[ip+1]
		startIP := ip
		ip += 2 + 2*int(pyc.Cache[op])
		if err := g.emitOp(w, c, op, int(arg), startIP, ip); err != nil {
			return fmt.Errorf("offset %d %s: %w", startIP, pyc.OpName(op), err)
		}
	}
	fmt.Fprintf(w, "\treturn rt.None, nil\n")
	fmt.Fprintf(w, "}\n\n")
	return nil
}

// idFor returns a stable counter for naming nested code objects.
func (g *gen) idFor(_ *pyc.Code) int {
	g.counter++
	return g.counter
}

// scanTargets returns a set of byte offsets that appear as jump
// destinations anywhere in the bytecode.
func scanTargets(c *pyc.Code) (map[int]bool, error) {
	targets := map[int]bool{}
	bc := c.Bytecode
	ip := 0
	for ip < len(bc) {
		op := bc[ip]
		arg := bc[ip+1]
		startIP := ip
		ip += 2 + 2*int(pyc.Cache[op])
		switch op {
		case pyc.JUMP_FORWARD, pyc.FOR_ITER:
			targets[ip+2*int(arg)] = true
		case pyc.POP_JUMP_IF_FALSE, pyc.POP_JUMP_IF_TRUE,
			pyc.POP_JUMP_IF_NONE, pyc.POP_JUMP_IF_NOT_NONE:
			targets[ip+2*int(arg)] = true
		case pyc.JUMP_BACKWARD:
			targets[ip-2*int(arg)] = true
		}
		_ = startIP
	}
	return targets, nil
}

// emitOp writes one opcode's Go fragment into w.
func (g *gen) emitOp(w byteWriter, c *pyc.Code, op uint8, arg, startIP, nextIP int) error {
	emit := func(format string, args ...any) {
		fmt.Fprintf(w, "\t"+format+"\n", args...)
	}
	switch op {
	case pyc.CACHE, pyc.RESUME, pyc.NOP, pyc.END_FOR, pyc.COPY_FREE_VARS, pyc.NOT_TAKEN:
		emit("// %s %d", pyc.OpName(op), arg)
	case pyc.UNARY_NEGATIVE:
		emit("{ r, err := rt.Neg(stk[sp-1]); if err != nil { return nil, err }; stk[sp-1] = r }")
	case pyc.UNARY_NOT:
		emit("stk[sp-1] = rt.Boxed(!rt.Truthy(stk[sp-1]))")
	case pyc.POP_TOP, pyc.POP_ITER:
		emit("sp--")
	case pyc.PUSH_NULL:
		emit("stk[sp] = nil; sp++")
	case pyc.COPY:
		emit("stk[sp] = stk[sp-%d]; sp++", arg)
	case pyc.SWAP:
		emit("stk[sp-1], stk[sp-%d] = stk[sp-%d], stk[sp-1]", arg, arg)
	case pyc.LOAD_SMALL_INT:
		emit("stk[sp] = rt.NewIntInt(%d); sp++", arg)
	case pyc.LOAD_CONST:
		expr, err := g.constExpr(c.Consts[arg])
		if err != nil {
			return err
		}
		emit("stk[sp] = %s; sp++", expr)
	case pyc.LOAD_FAST, pyc.LOAD_FAST_CHECK, pyc.LOAD_FAST_AND_CLEAR, pyc.LOAD_FAST_BORROW:
		emit("stk[sp] = _L%d; sp++", arg)
		if op == pyc.LOAD_FAST_AND_CLEAR {
			emit("_L%d = nil", arg)
		}
	case pyc.LOAD_FAST_LOAD_FAST, pyc.LOAD_FAST_BORROW_LOAD_FAST_BORROW:
		i1 := int(arg) >> 4
		i2 := int(arg) & 0xF
		emit("stk[sp] = _L%d; sp++", i1)
		emit("stk[sp] = _L%d; sp++", i2)
	case pyc.STORE_FAST:
		emit("sp--; _L%d = stk[sp]", arg)
	case pyc.STORE_FAST_STORE_FAST:
		i1 := int(arg) >> 4
		i2 := int(arg) & 0xF
		emit("sp--; _L%d = stk[sp]", i1)
		emit("sp--; _L%d = stk[sp]", i2)
	case pyc.LOAD_GLOBAL:
		pushNull := arg&1 == 1
		nameIdx := arg >> 1
		nm := c.Names[nameIdx]
		emit("stk[sp] = loadName(%q); sp++", nm)
		if pushNull {
			emit("stk[sp] = nil; sp++")
		}
	case pyc.LOAD_NAME:
		nm := c.Names[arg]
		emit("stk[sp] = loadName(%q); sp++", nm)
	case pyc.STORE_NAME:
		nm := c.Names[arg]
		emit("sp--; globals[%q] = stk[sp]", nm)
	case pyc.BINARY_OP:
		kind := uint8(arg) & 0x1F
		fn, ok := binaryOpFunc(kind)
		if !ok {
			return fmt.Errorf("BINARY_OP kind %d not supported in v0.1", kind)
		}
		emit("{ sp--; r, err := rt.%s(stk[sp-1], stk[sp]); if err != nil { return nil, err }; stk[sp-1] = r }", fn)
	case pyc.COMPARE_OP:
		cmp := int(uint(arg) >> 5)
		emit("{ sp--; r, err := rt.Compare(%d, stk[sp-1], stk[sp]); if err != nil { return nil, err }; stk[sp-1] = r }", cmp)
	case pyc.TO_BOOL:
		emit("stk[sp-1] = rt.Boxed(rt.Truthy(stk[sp-1]))")
	case pyc.CALL:
		n := arg
		emit("{")
		emit("\tcallable := stk[sp-%d-2]", n)
		emit("\t_ = stk[sp-%d-1] // NULL/self", n)
		emit("\tcallArgs := make([]rt.Value, %d)", n)
		emit("\tcopy(callArgs, stk[sp-%d:sp])", n)
		emit("\tr, err := rt.Call(callable, callArgs...)")
		emit("\tif err != nil { return nil, err }")
		emit("\tsp = sp - %d - 2", n)
		emit("\tstk[sp] = r; sp++")
		emit("}")
	case pyc.MAKE_FUNCTION:
		// TOS is already an *rt.Func because we wrap at LOAD_CONST.
		emit("// MAKE_FUNCTION (wrapped at LOAD_CONST)")
	case pyc.GET_ITER:
		emit("{ it, err := rt.GetIter(stk[sp-1]); if err != nil { return nil, err }; stk[sp-1] = it }")
	case pyc.FOR_ITER:
		target := nextIP + 2*arg
		emit("{ it := stk[sp-1].(*rt.Iter); v, ok := it.Next(); if !ok { goto L%d }; stk[sp] = v; sp++ }", target)
	case pyc.JUMP_FORWARD:
		target := nextIP + 2*arg
		emit("goto L%d", target)
	case pyc.JUMP_BACKWARD:
		target := nextIP - 2*arg
		emit("goto L%d", target)
	case pyc.POP_JUMP_IF_FALSE:
		target := nextIP + 2*arg
		emit("{ sp--; if !rt.Truthy(stk[sp]) { goto L%d } }", target)
	case pyc.POP_JUMP_IF_TRUE:
		target := nextIP + 2*arg
		emit("{ sp--; if rt.Truthy(stk[sp]) { goto L%d } }", target)
	case pyc.POP_JUMP_IF_NONE:
		target := nextIP + 2*arg
		emit("{ sp--; if _, ok := stk[sp].(*rt.NoneType); ok { goto L%d } }", target)
	case pyc.POP_JUMP_IF_NOT_NONE:
		target := nextIP + 2*arg
		emit("{ sp--; if _, ok := stk[sp].(*rt.NoneType); !ok { goto L%d } }", target)
	case pyc.BUILD_TUPLE:
		emit("{ els := make([]rt.Value, %d); copy(els, stk[sp-%d:sp]); sp -= %d; stk[sp] = &rt.Tuple{V: els}; sp++ }", arg, arg, arg)
	case pyc.BUILD_LIST:
		emit("{ els := make([]rt.Value, %d); copy(els, stk[sp-%d:sp]); sp -= %d; stk[sp] = &rt.List{V: els}; sp++ }", arg, arg, arg)
	case pyc.BUILD_STRING:
		emit("{ r := rt.ConcatStrings(stk[sp-%d:sp]); sp -= %d; stk[sp] = r; sp++ }", arg, arg)
	case pyc.FORMAT_SIMPLE:
		emit("stk[sp-1] = rt.Format(stk[sp-1])")
	case pyc.RETURN_VALUE:
		emit("sp--; return stk[sp], nil")
	case pyc.INTERPRETER_EXIT:
		emit("return rt.None, nil")
	default:
		return fmt.Errorf("opcode %s (%d) not supported in v0.1", pyc.OpName(op), op)
	}
	return nil
}

// byteWriter is satisfied by *bytes.Buffer; defined as an interface
// so emitOp's signature is testable without importing bytes in
// consumer tests.
type byteWriter interface {
	Write(p []byte) (int, error)
}

// constExpr returns a Go expression that pushes the given marshal
// object onto the stack. For code objects we reference the nested
// function we already scheduled for emission.
func (g *gen) constExpr(o pyc.Object) (string, error) {
	switch x := o.(type) {
	case *pyc.Int:
		if x.V.IsInt64() && x.V.BitLen() < 63 {
			return fmt.Sprintf("rt.NewIntInt(%d)", x.V.Int64()), nil
		}
		// Large int: fall back to decimal literal.
		return fmt.Sprintf("rt.BigIntFromDecimal(%q)", x.V.String()), nil
	case *pyc.Str:
		return fmt.Sprintf("rt.NewStr(%s)", strconv.Quote(x.V)), nil
	case *pyc.Float:
		return fmt.Sprintf("rt.NewFloat(%s)", strconv.FormatFloat(x.V, 'g', -1, 64)), nil
	case *pyc.Bool:
		if x.V {
			return "rt.True", nil
		}
		return "rt.False", nil
	case *pyc.NoneType:
		return "rt.None", nil
	case *pyc.Tuple:
		var parts []string
		for _, e := range x.V {
			s, err := g.constExpr(e)
			if err != nil {
				return "", err
			}
			parts = append(parts, s)
		}
		return fmt.Sprintf("rt.NewTuple(%s)", joinStrs(parts, ", ")), nil
	case *pyc.Code:
		fn := g.funcOf[x]
		if fn == "" {
			// Should have been pre-registered in emitFunc's preamble.
			return "", fmt.Errorf("code const %q not registered", x.QualName)
		}
		return fmt.Sprintf("&rt.Func{Name: %q, Arity: %d, Impl: %s}",
			x.QualName, x.ArgCount, fn), nil
	case *pyc.Bytes:
		return fmt.Sprintf("rt.NewStr(%s) /* bytes-as-str */", strconv.Quote(string(x.V))), nil
	}
	return "", fmt.Errorf("const type %T not supported in v0.1", o)
}

func joinStrs(xs []string, sep string) string {
	out := ""
	for i, x := range xs {
		if i > 0 {
			out += sep
		}
		out += x
	}
	return out
}

// binaryOpFunc maps an NB_* kind to the runtime function name.
func binaryOpFunc(kind uint8) (string, bool) {
	switch kind {
	case pyc.NB_ADD, pyc.NB_INPLACE_ADD:
		return "Add", true
	case pyc.NB_SUBTRACT, pyc.NB_INPLACE_SUB:
		return "Sub", true
	case pyc.NB_MULTIPLY, pyc.NB_INPLACE_MUL:
		return "Mul", true
	case pyc.NB_TRUE_DIVIDE, pyc.NB_INPLACE_TDIV:
		return "TrueDiv", true
	case pyc.NB_FLOOR_DIVIDE, pyc.NB_INPLACE_FDIV:
		return "FloorDiv", true
	case pyc.NB_REMAINDER, pyc.NB_INPLACE_REM:
		return "Mod", true
	}
	return "", false
}

