// Package types is gopygo's type lattice. It exists so the code
// generator can ask "what Go type do I emit for this expression?"
// without having to re-walk the AST. Inference lives in infer.go.
package types

import "fmt"

// Type is one node in gopygo's type lattice. Every value the
// transpiler sees has a Type; Gen (Type) returns the Go type name
// to emit for it.
type Type interface {
	Go() string
	String() string
	isType()
}

type (
	TInt    struct{}
	TFloat  struct{}
	TBool   struct{}
	TStr    struct{}
	TNone   struct{}
	TAny    struct{} // emitted as Go `any`; only set by explicit user annotation
	TNever  struct{} // function never returns normally
	TList   struct{ Elem Type }
	TDict   struct{ K, V Type }
	TTuple  struct{ Elems []Type }
	TFunc   struct {
		Params []Type
		Return Type
		Name   string // for diagnostics and direct-call emission
	}
)

func (TInt) isType()   {}
func (TFloat) isType() {}
func (TBool) isType()  {}
func (TStr) isType()   {}
func (TNone) isType()  {}
func (TAny) isType()   {}
func (TNever) isType() {}
func (TList) isType()  {}
func (TDict) isType()  {}
func (TTuple) isType() {}
func (TFunc) isType()  {}

func (TInt) Go() string   { return "int64" }
func (TFloat) Go() string { return "float64" }
func (TBool) Go() string  { return "bool" }
func (TStr) Go() string   { return "string" }
func (TNone) Go() string  { return "struct{}" }
func (TAny) Go() string   { return "any" }
func (TNever) Go() string { return "struct{}" }

func (t TList) Go() string  { return "[]" + t.Elem.Go() }
func (t TDict) Go() string  { return "map[" + t.K.Go() + "]" + t.V.Go() }
func (t TTuple) Go() string { panic("TTuple.Go: emit as named struct, not inline") }
func (t TFunc) Go() string {
	var ps string
	for i, p := range t.Params {
		if i > 0 {
			ps += ", "
		}
		ps += p.Go()
	}
	r := ""
	if _, ok := t.Return.(TNone); !ok {
		r = " " + t.Return.Go()
	}
	return fmt.Sprintf("func(%s)%s", ps, r)
}

func (TInt) String() string   { return "int" }
func (TFloat) String() string { return "float" }
func (TBool) String() string  { return "bool" }
func (TStr) String() string   { return "str" }
func (TNone) String() string  { return "None" }
func (TAny) String() string   { return "Any" }
func (TNever) String() string { return "Never" }

func (t TList) String() string  { return "list[" + t.Elem.String() + "]" }
func (t TDict) String() string  { return "dict[" + t.K.String() + ", " + t.V.String() + "]" }
func (t TTuple) String() string {
	s := "tuple["
	for i, e := range t.Elems {
		if i > 0 {
			s += ", "
		}
		s += e.String()
	}
	return s + "]"
}
func (t TFunc) String() string { return fmt.Sprintf("%s%s -> %s", t.Name, paramSig(t.Params), t.Return.String()) }

func paramSig(ps []Type) string {
	s := "("
	for i, p := range ps {
		if i > 0 {
			s += ", "
		}
		s += p.String()
	}
	return s + ")"
}

// Equal is structural equality on the lattice.
func Equal(a, b Type) bool {
	switch ax := a.(type) {
	case TInt:
		_, ok := b.(TInt)
		return ok
	case TFloat:
		_, ok := b.(TFloat)
		return ok
	case TBool:
		_, ok := b.(TBool)
		return ok
	case TStr:
		_, ok := b.(TStr)
		return ok
	case TNone:
		_, ok := b.(TNone)
		return ok
	case TAny:
		_, ok := b.(TAny)
		return ok
	case TNever:
		_, ok := b.(TNever)
		return ok
	case TList:
		bx, ok := b.(TList)
		return ok && Equal(ax.Elem, bx.Elem)
	case TDict:
		bx, ok := b.(TDict)
		return ok && Equal(ax.K, bx.K) && Equal(ax.V, bx.V)
	case TTuple:
		bx, ok := b.(TTuple)
		if !ok || len(ax.Elems) != len(bx.Elems) {
			return false
		}
		for i := range ax.Elems {
			if !Equal(ax.Elems[i], bx.Elems[i]) {
				return false
			}
		}
		return true
	case TFunc:
		bx, ok := b.(TFunc)
		if !ok || len(ax.Params) != len(bx.Params) || !Equal(ax.Return, bx.Return) {
			return false
		}
		for i := range ax.Params {
			if !Equal(ax.Params[i], bx.Params[i]) {
				return false
			}
		}
		return true
	}
	return false
}

// IsNumeric reports whether t is TInt or TFloat.
func IsNumeric(t Type) bool {
	switch t.(type) {
	case TInt, TFloat:
		return true
	}
	return false
}

// Widen returns the common numeric type if both a and b are numeric,
// or nil otherwise. TInt+TFloat widens to TFloat.
func Widen(a, b Type) Type {
	if !IsNumeric(a) || !IsNumeric(b) {
		return nil
	}
	if _, ok := a.(TFloat); ok {
		return TFloat{}
	}
	if _, ok := b.(TFloat); ok {
		return TFloat{}
	}
	return TInt{}
}
