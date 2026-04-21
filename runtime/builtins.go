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
	Builtins["min"] = &Func{Name: "min", Arity: -1, Impl: pyMin}
	Builtins["max"] = &Func{Name: "max", Arity: -1, Impl: pyMax}
	Builtins["str"] = &Func{Name: "str", Arity: 1, Impl: pyStr}
	Builtins["int"] = &Func{Name: "int", Arity: -1, Impl: pyInt}
	Builtins["bool"] = &Func{Name: "bool", Arity: -1, Impl: pyBool}
	Builtins["list"] = &Func{Name: "list", Arity: -1, Impl: pyList}
	Builtins["True"] = True
	Builtins["False"] = False
	Builtins["None"] = None
}

func pyMin(args []Value) (Value, error) { return minMax(args, -1) }
func pyMax(args []Value) (Value, error) { return minMax(args, +1) }

func minMax(args []Value, sign int) (Value, error) {
	var xs []Value
	if len(args) == 1 {
		it, err := GetIter(args[0])
		if err != nil {
			return nil, err
		}
		for {
			v, ok := it.Next()
			if !ok {
				break
			}
			xs = append(xs, v)
		}
	} else {
		xs = args
	}
	if len(xs) == 0 {
		return nil, TypeError("min/max() arg is an empty sequence")
	}
	best := xs[0]
	for _, v := range xs[1:] {
		c, ok := cmpNumeric(v, best)
		if !ok {
			if vs, ok := v.(*Str); ok {
				if bs, ok := best.(*Str); ok {
					switch {
					case vs.V < bs.V:
						c = -1
					case vs.V > bs.V:
						c = 1
					}
					ok = true
					_ = ok
				}
			}
		}
		if (sign < 0 && c < 0) || (sign > 0 && c > 0) {
			best = v
		}
	}
	return best, nil
}

func pyStr(args []Value) (Value, error) {
	if len(args) != 1 {
		return nil, TypeError("str() takes exactly one argument (%d given)", len(args))
	}
	return &Str{V: Stringify(args[0])}, nil
}

func pyInt(args []Value) (Value, error) {
	if len(args) == 0 {
		return NewIntInt(0), nil
	}
	if len(args) != 1 {
		return nil, TypeError("int() takes at most 1 argument (%d given)", len(args))
	}
	switch x := args[0].(type) {
	case *Int:
		return x, nil
	case *Bool:
		if x.V {
			return NewIntInt(1), nil
		}
		return NewIntInt(0), nil
	case *Float:
		r := &Int{}
		r.V.SetInt64(int64(x.V))
		return r, nil
	case *Str:
		n, ok := new(big.Int).SetString(x.V, 10)
		if !ok {
			return nil, TypeError("invalid literal for int(): %q", x.V)
		}
		r := &Int{}
		r.V.Set(n)
		return r, nil
	}
	return nil, TypeError("int() argument must be a number or string, not '%s'", TypeName(args[0]))
}

func pyBool(args []Value) (Value, error) {
	if len(args) == 0 {
		return False, nil
	}
	if len(args) != 1 {
		return nil, TypeError("bool() takes at most 1 argument (%d given)", len(args))
	}
	return Boxed(Truthy(args[0])), nil
}

func pyList(args []Value) (Value, error) {
	if len(args) == 0 {
		return &List{}, nil
	}
	if len(args) != 1 {
		return nil, TypeError("list() takes at most 1 argument (%d given)", len(args))
	}
	it, err := GetIter(args[0])
	if err != nil {
		return nil, err
	}
	var xs []Value
	for {
		v, ok := it.Next()
		if !ok {
			break
		}
		xs = append(xs, v)
	}
	return &List{V: xs}, nil
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
