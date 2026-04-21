package runtime

import (
	"io"
	"math/big"
	"os"
	"strings"
)

// Stdout is where Print writes. Transpiled programs may assign this
// before running if they want to capture output. Defaults to os.Stdout.
var Stdout io.Writer = os.Stdout

// Builtins is the table LOAD_GLOBAL consults after failing to find a
// name in module globals. v0.1 only populates it with names the
// transpiler emits; extend with care.
var Builtins = map[string]Value{}

func init() {
	Builtins["print"] = &Func{Name: "print", Arity: -1, Impl: pyPrint}
	Builtins["range"] = &Func{Name: "range", Arity: -1, Impl: pyRange}
	Builtins["len"] = &Func{Name: "len", Arity: 1, Impl: pyLen}
	Builtins["abs"] = &Func{Name: "abs", Arity: 1, Impl: pyAbs}
	Builtins["True"] = True
	Builtins["False"] = False
	Builtins["None"] = None
}

func pyPrint(args []Value) (Value, error) {
	var b strings.Builder
	for i, a := range args {
		if i > 0 {
			b.WriteByte(' ')
		}
		b.WriteString(Stringify(a))
	}
	b.WriteByte('\n')
	_, err := io.WriteString(Stdout, b.String())
	if err != nil {
		return nil, err
	}
	return None, nil
}

func pyLen(args []Value) (Value, error) {
	if len(args) != 1 {
		return nil, TypeError("len() takes exactly one argument (%d given)", len(args))
	}
	switch x := args[0].(type) {
	case *Str:
		return NewIntInt(int64(len(x.V))), nil
	case *Tuple:
		return NewIntInt(int64(len(x.V))), nil
	case *List:
		return NewIntInt(int64(len(x.V))), nil
	}
	return nil, TypeError("object of type '%s' has no len()", TypeName(args[0]))
}

func pyAbs(args []Value) (Value, error) {
	if len(args) != 1 {
		return nil, TypeError("abs() takes exactly one argument (%d given)", len(args))
	}
	switch x := args[0].(type) {
	case *Int:
		r := &Int{}
		r.V.Abs(&x.V)
		return r, nil
	case *Float:
		if x.V < 0 {
			return &Float{V: -x.V}, nil
		}
		return x, nil
	}
	return nil, TypeError("bad operand type for abs(): '%s'", TypeName(args[0]))
}

// pyRange mirrors CPython's range() for positional int args. Returns
// an Iter; v0.1 generated code invokes GET_ITER which is a no-op on
// an Iter.
func pyRange(args []Value) (Value, error) {
	var start, stop, step int64
	step = 1
	switch len(args) {
	case 1:
		n, err := asInt64(args[0])
		if err != nil {
			return nil, err
		}
		stop = n
	case 2:
		a, err := asInt64(args[0])
		if err != nil {
			return nil, err
		}
		b, err := asInt64(args[1])
		if err != nil {
			return nil, err
		}
		start, stop = a, b
	case 3:
		a, err := asInt64(args[0])
		if err != nil {
			return nil, err
		}
		b, err := asInt64(args[1])
		if err != nil {
			return nil, err
		}
		c, err := asInt64(args[2])
		if err != nil {
			return nil, err
		}
		if c == 0 {
			return nil, TypeError("range() arg 3 must not be zero")
		}
		start, stop, step = a, b, c
	default:
		return nil, TypeError("range expected 1..3 arguments, got %d", len(args))
	}
	cur := start
	return &Iter{Next: func() (Value, bool) {
		if (step > 0 && cur >= stop) || (step < 0 && cur <= stop) {
			return nil, false
		}
		v := NewIntInt(cur)
		cur += step
		return v, true
	}}, nil
}

// asInt64 extracts an int64 from a Python int that fits.
func asInt64(v Value) (int64, error) {
	i, ok := v.(*Int)
	if !ok {
		return 0, TypeError("'%s' object cannot be interpreted as an integer", TypeName(v))
	}
	if !i.V.IsInt64() {
		return 0, TypeError("OverflowError: Python int too large for Go int64")
	}
	return i.V.Int64(), nil
}

// GetIter implements GET_ITER. If v is already an Iter, return it.
// Tuples/lists/strings are wrapped. Ranges come in as Iter already.
func GetIter(v Value) (*Iter, error) {
	switch x := v.(type) {
	case *Iter:
		return x, nil
	case *Tuple:
		i := 0
		return &Iter{Next: func() (Value, bool) {
			if i >= len(x.V) {
				return nil, false
			}
			v := x.V[i]
			i++
			return v, true
		}}, nil
	case *List:
		i := 0
		return &Iter{Next: func() (Value, bool) {
			if i >= len(x.V) {
				return nil, false
			}
			v := x.V[i]
			i++
			return v, true
		}}, nil
	case *Str:
		// Iterate codepoints, not bytes.
		runes := []rune(x.V)
		i := 0
		return &Iter{Next: func() (Value, bool) {
			if i >= len(runes) {
				return nil, false
			}
			v := &Str{V: string(runes[i])}
			i++
			return v, true
		}}, nil
	}
	return nil, TypeError("'%s' object is not iterable", TypeName(v))
}

// Format implements FORMAT_SIMPLE: str() the value for f-string
// interpolation when no spec is attached.
func Format(v Value) *Str {
	return &Str{V: Stringify(v)}
}

// ConcatStrings builds a single *Str from a sequence of values whose
// stringified forms should be joined. Used for BUILD_STRING inside
// f-strings.
func ConcatStrings(parts []Value) *Str {
	var b strings.Builder
	for _, p := range parts {
		b.WriteString(Stringify(p))
	}
	return &Str{V: b.String()}
}

// BigIntFromDecimal is used by generated code when a literal int
// does not fit an int64. It panics on bad input because the
// transpiler is the sole caller.
func BigIntFromDecimal(s string) *Int {
	x, ok := new(big.Int).SetString(s, 10)
	if !ok {
		panic("runtime.BigIntFromDecimal: " + s)
	}
	r := &Int{}
	r.V.Set(x)
	return r
}
