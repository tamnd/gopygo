// Package runtime is the support library that transpiled programs
// import. Every generated Go file begins with
//
//	import rt "github.com/tamnd/gopygo/runtime"
//
// and every Python value is carried around as rt.Value. Type checks
// are one type assertion deep, modeled on CPython's C-level dispatch
// rather than on Go's interface method semantics.
package runtime

import (
	"fmt"
	"math/big"
	"strconv"
)

// Value is any Python value a transpiled program passes around.
// Kept as a naked interface{} so we can box small ints and strings
// as the concrete pointer types below without needing a separate
// dispatch table.
type Value interface{}

// Int is an arbitrary-precision Python int, backed by math/big.
// The big.Int is embedded by value to keep small ints to one heap
// allocation instead of three.
type Int struct{ V big.Int }

// Str is a Python str. Utf-8 encoded Go string.
type Str struct{ V string }

// Float is a Python float.
type Float struct{ V float64 }

// Bool is a Python bool. Only two instances exist; use True and False.
type Bool struct{ V bool }

// Tuple is an immutable sequence of values.
type Tuple struct{ V []Value }

// List is a mutable sequence. Used by runtime builtins that need it;
// v0.1 transpiler does not emit BUILD_LIST yet beyond trivial cases.
type List struct{ V []Value }

// NoneType is the type of None. Only one instance: None.
type NoneType struct{}

// None is the Python None singleton.
var None = &NoneType{}

// True and False are the canonical bool singletons.
var (
	True  = &Bool{V: true}
	False = &Bool{V: false}
)

// Boxed returns the bool singleton for b.
func Boxed(b bool) *Bool {
	if b {
		return True
	}
	return False
}

// Func is a transpiled Python function. Impl is the generated Go
// closure; Arity and Name are carried for diagnostics.
type Func struct {
	Name  string
	Arity int
	Impl  func(args []Value) (Value, error)
}

// Iter is a lazy iterator produced by GetIter. Next returns the next
// value and a flag; flag=false means "exhausted".
type Iter struct {
	Next func() (Value, bool)
}

// Constructors used by generated code and by builtins.

// NewIntInt wraps a Go int64 as *Int.
func NewIntInt(v int64) *Int {
	i := &Int{}
	i.V.SetInt64(v)
	return i
}

// NewIntBig wraps a *big.Int as *Int (copy semantics).
func NewIntBig(v *big.Int) *Int {
	i := &Int{}
	i.V.Set(v)
	return i
}

// NewStr wraps a Go string as *Str.
func NewStr(s string) *Str { return &Str{V: s} }

// NewFloat wraps a Go float64 as *Float.
func NewFloat(f float64) *Float { return &Float{V: f} }

// NewTuple builds a tuple from its arguments.
func NewTuple(xs ...Value) *Tuple { return &Tuple{V: append([]Value(nil), xs...)} }

// Repr returns the repr(v) form, matching CPython for the supported
// types. Used by print() via Str.
func Repr(v Value) string {
	switch x := v.(type) {
	case *Int:
		return x.V.String()
	case *Str:
		// Python repr quotes strings; print(x) uses str(x) which is
		// unquoted. Callers choose via Print which calls Str, not Repr.
		return strconv.Quote(x.V)
	case *Float:
		return formatFloat(x.V)
	case *Bool:
		if x.V {
			return "True"
		}
		return "False"
	case *NoneType:
		return "None"
	case *Tuple:
		s := "("
		for i, e := range x.V {
			if i > 0 {
				s += ", "
			}
			s += Repr(e)
		}
		if len(x.V) == 1 {
			s += ","
		}
		return s + ")"
	case *List:
		s := "["
		for i, e := range x.V {
			if i > 0 {
				s += ", "
			}
			s += Repr(e)
		}
		return s + "]"
	default:
		return fmt.Sprintf("<%T>", v)
	}
}

// Stringify returns the str(v) form (unquoted, like print).
func Stringify(v Value) string {
	switch x := v.(type) {
	case *Str:
		return x.V
	case *Int:
		return x.V.String()
	case *Float:
		return formatFloat(x.V)
	case *Bool:
		if x.V {
			return "True"
		}
		return "False"
	case *NoneType:
		return "None"
	default:
		return Repr(v)
	}
}

// formatFloat matches CPython's default float repr closely enough for
// the v0.1 fixtures. It prints a trailing ".0" on whole-valued floats
// so `print(1.0)` emits `1.0`, not `1`.
func formatFloat(f float64) string {
	s := strconv.FormatFloat(f, 'g', -1, 64)
	for i := 0; i < len(s); i++ {
		if s[i] == '.' || s[i] == 'e' || s[i] == 'n' || s[i] == 'i' {
			return s
		}
	}
	return s + ".0"
}

// Truthy returns the truth value of v per Python rules.
func Truthy(v Value) bool {
	switch x := v.(type) {
	case *Bool:
		return x.V
	case *Int:
		return x.V.Sign() != 0
	case *Float:
		return x.V != 0
	case *Str:
		return x.V != ""
	case *Tuple:
		return len(x.V) != 0
	case *List:
		return len(x.V) != 0
	case *NoneType:
		return false
	case nil:
		return false
	default:
		return true
	}
}

// TypeName returns a best-effort Python type name.
func TypeName(v Value) string {
	switch v.(type) {
	case *Int:
		return "int"
	case *Str:
		return "str"
	case *Float:
		return "float"
	case *Bool:
		return "bool"
	case *NoneType:
		return "NoneType"
	case *Tuple:
		return "tuple"
	case *List:
		return "list"
	case *Func:
		return "function"
	case *Iter:
		return "iterator"
	default:
		return fmt.Sprintf("%T", v)
	}
}

// TypeError builds a Python-flavoured TypeError message.
func TypeError(format string, args ...any) error {
	return fmt.Errorf("TypeError: "+format, args...)
}
