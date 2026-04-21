package runtime

import (
	"math"
	"math/big"
)

// Add implements Python `a + b` for int/float/str in v0.1.
func Add(a, b Value) (Value, error) {
	// Fast paths.
	if ai, ok := a.(*Int); ok {
		if bi, ok := b.(*Int); ok {
			r := &Int{}
			r.V.Add(&ai.V, &bi.V)
			return r, nil
		}
		if bf, ok := b.(*Float); ok {
			fa, _ := new(big.Float).SetInt(&ai.V).Float64()
			return &Float{V: fa + bf.V}, nil
		}
	}
	if af, ok := a.(*Float); ok {
		if bf, ok := b.(*Float); ok {
			return &Float{V: af.V + bf.V}, nil
		}
		if bi, ok := b.(*Int); ok {
			fb, _ := new(big.Float).SetInt(&bi.V).Float64()
			return &Float{V: af.V + fb}, nil
		}
	}
	if as, ok := a.(*Str); ok {
		if bs, ok := b.(*Str); ok {
			return &Str{V: as.V + bs.V}, nil
		}
	}
	return nil, TypeError("unsupported operand type(s) for +: '%s' and '%s'", TypeName(a), TypeName(b))
}

// Sub implements `a - b`.
func Sub(a, b Value) (Value, error) {
	if ai, ok := a.(*Int); ok {
		if bi, ok := b.(*Int); ok {
			r := &Int{}
			r.V.Sub(&ai.V, &bi.V)
			return r, nil
		}
		if bf, ok := b.(*Float); ok {
			fa, _ := new(big.Float).SetInt(&ai.V).Float64()
			return &Float{V: fa - bf.V}, nil
		}
	}
	if af, ok := a.(*Float); ok {
		if bf, ok := b.(*Float); ok {
			return &Float{V: af.V - bf.V}, nil
		}
		if bi, ok := b.(*Int); ok {
			fb, _ := new(big.Float).SetInt(&bi.V).Float64()
			return &Float{V: af.V - fb}, nil
		}
	}
	return nil, TypeError("unsupported operand type(s) for -: '%s' and '%s'", TypeName(a), TypeName(b))
}

// Mul implements `a * b`.
func Mul(a, b Value) (Value, error) {
	if ai, ok := a.(*Int); ok {
		if bi, ok := b.(*Int); ok {
			r := &Int{}
			r.V.Mul(&ai.V, &bi.V)
			return r, nil
		}
		if bf, ok := b.(*Float); ok {
			fa, _ := new(big.Float).SetInt(&ai.V).Float64()
			return &Float{V: fa * bf.V}, nil
		}
	}
	if af, ok := a.(*Float); ok {
		if bf, ok := b.(*Float); ok {
			return &Float{V: af.V * bf.V}, nil
		}
		if bi, ok := b.(*Int); ok {
			fb, _ := new(big.Float).SetInt(&bi.V).Float64()
			return &Float{V: af.V * fb}, nil
		}
	}
	return nil, TypeError("unsupported operand type(s) for *: '%s' and '%s'", TypeName(a), TypeName(b))
}

// TrueDiv implements `a / b` (always float result in Python 3).
func TrueDiv(a, b Value) (Value, error) {
	af, aok := toFloat(a)
	bf, bok := toFloat(b)
	if !aok || !bok {
		return nil, TypeError("unsupported operand type(s) for /: '%s' and '%s'", TypeName(a), TypeName(b))
	}
	if bf == 0 {
		return nil, zeroDivision("division")
	}
	return &Float{V: af / bf}, nil
}

// FloorDiv implements `a // b`. Python semantics: integer result for
// int/int (floored toward negative infinity), float result otherwise.
func FloorDiv(a, b Value) (Value, error) {
	if ai, ok := a.(*Int); ok {
		if bi, ok := b.(*Int); ok {
			if bi.V.Sign() == 0 {
				return nil, zeroDivision("integer division or modulo")
			}
			q, _ := new(big.Int).QuoRem(&ai.V, &bi.V, new(big.Int))
			// Go Quo truncates toward zero; Python floor-divides.
			r := new(big.Int).Sub(&ai.V, new(big.Int).Mul(q, &bi.V))
			if r.Sign() != 0 && (r.Sign() != bi.V.Sign()) {
				q.Sub(q, big.NewInt(1))
			}
			out := &Int{}
			out.V.Set(q)
			return out, nil
		}
	}
	af, aok := toFloat(a)
	bf, bok := toFloat(b)
	if !aok || !bok {
		return nil, TypeError("unsupported operand type(s) for //: '%s' and '%s'", TypeName(a), TypeName(b))
	}
	if bf == 0 {
		return nil, zeroDivision("float floor division")
	}
	return &Float{V: math.Floor(af / bf)}, nil
}

// Mod implements `a % b`.
func Mod(a, b Value) (Value, error) {
	if ai, ok := a.(*Int); ok {
		if bi, ok := b.(*Int); ok {
			if bi.V.Sign() == 0 {
				return nil, zeroDivision("integer modulo")
			}
			r := new(big.Int).Mod(&ai.V, &bi.V) // big.Mod already Euclidean-ish
			// Python %: result has sign of divisor.
			if r.Sign() != 0 && r.Sign() != bi.V.Sign() {
				r.Add(r, &bi.V)
			}
			out := &Int{}
			out.V.Set(r)
			return out, nil
		}
	}
	af, aok := toFloat(a)
	bf, bok := toFloat(b)
	if !aok || !bok {
		return nil, TypeError("unsupported operand type(s) for %%: '%s' and '%s'", TypeName(a), TypeName(b))
	}
	if bf == 0 {
		return nil, zeroDivision("float modulo")
	}
	r := math.Mod(af, bf)
	if r != 0 && ((r < 0) != (bf < 0)) {
		r += bf
	}
	return &Float{V: r}, nil
}

// Neg implements unary `-a`.
func Neg(a Value) (Value, error) {
	switch x := a.(type) {
	case *Int:
		r := &Int{}
		r.V.Neg(&x.V)
		return r, nil
	case *Float:
		return &Float{V: -x.V}, nil
	}
	return nil, TypeError("bad operand type for unary -: '%s'", TypeName(a))
}

func toFloat(v Value) (float64, bool) {
	switch x := v.(type) {
	case *Int:
		f, _ := new(big.Float).SetInt(&x.V).Float64()
		return f, true
	case *Float:
		return x.V, true
	case *Bool:
		if x.V {
			return 1, true
		}
		return 0, true
	}
	return 0, false
}

func zeroDivision(what string) error {
	return TypeError("ZeroDivisionError: %s by zero", what)
}
