package runtime

import (
	"fmt"
	"math"
)

// Must unwraps (v, err) from a fallible runtime op. Generated code
// wraps most expressions in Must so the call-site can be a plain Go
// expression. A non-nil err becomes a panic with a Python-style
// message; the top-level main() in generated code recovers and exits.
func Must(v Value, err error) Value {
	if err != nil {
		panic(err)
	}
	return v
}

// Not is the `not x` unary. Never fails.
func Not(v Value) *Bool { return Boxed(!Truthy(v)) }

// Is is Python identity. None/True/False are singletons, so pointer
// equality matches CPython. For other types v0.1 uses pointer
// equality too (small-int interning is not modeled yet).
func Is(a, b Value) *Bool { return Boxed(a == b) }

// IsNot is `a is not b`.
func IsNot(a, b Value) *Bool { return Boxed(a != b) }

// Contains implements `x in container`. v0.1 covers str, tuple,
// list, dict.
func Contains(container, x Value) (*Bool, error) {
	switch c := container.(type) {
	case *Str:
		xs, ok := x.(*Str)
		if !ok {
			return nil, TypeError("'in <str>' requires string as left operand, not %s", TypeName(x))
		}
		return Boxed(len(xs.V) == 0 || indexStr(c.V, xs.V) >= 0), nil
	case *Tuple:
		for _, e := range c.V {
			if Equal(e, x) {
				return True, nil
			}
		}
		return False, nil
	case *List:
		for _, e := range c.V {
			if Equal(e, x) {
				return True, nil
			}
		}
		return False, nil
	case *Dict:
		_, ok := c.Lookup(x)
		return Boxed(ok), nil
	}
	return nil, TypeError("argument of type '%s' is not iterable", TypeName(container))
}

func indexStr(s, sub string) int {
	if len(sub) == 0 {
		return 0
	}
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return i
		}
	}
	return -1
}

// Pow implements `a ** b` for int/float. Int**Int with non-negative
// exponent stays int; negative exponent or either operand float
// promotes to float.
func Pow(a, b Value) (Value, error) {
	if ai, ok := a.(*Int); ok {
		if bi, ok := b.(*Int); ok {
			if bi.V.Sign() >= 0 {
				r := &Int{}
				r.V.Exp(&ai.V, &bi.V, nil)
				return r, nil
			}
		}
	}
	af, aok := toFloat(a)
	bf, bok := toFloat(b)
	if !aok || !bok {
		return nil, TypeError("unsupported operand type(s) for **: '%s' and '%s'", TypeName(a), TypeName(b))
	}
	return &Float{V: pyPow(af, bf)}, nil
}

func pyPow(a, b float64) float64 { return math.Pow(a, b) }

// FormatError stringifies a panicking error for the generated
// program's top-level recover handler.
func FormatError(err error) string { return fmt.Sprintf("%v", err) }
