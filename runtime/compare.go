package runtime

import "math/big"

// cmp returns -1, 0, or +1 for ordered numeric comparison, or a
// non-nil error if the operands are not comparable under v0.1.
func cmpNumeric(a, b Value) (int, bool) {
	if ai, ok := a.(*Int); ok {
		if bi, ok := b.(*Int); ok {
			return ai.V.Cmp(&bi.V), true
		}
		if bf, ok := b.(*Float); ok {
			af, _ := new(big.Float).SetInt(&ai.V).Float64()
			return floatCmp(af, bf.V), true
		}
	}
	if af, ok := a.(*Float); ok {
		if bf, ok := b.(*Float); ok {
			return floatCmp(af.V, bf.V), true
		}
		if bi, ok := b.(*Int); ok {
			fb, _ := new(big.Float).SetInt(&bi.V).Float64()
			return floatCmp(af.V, fb), true
		}
	}
	return 0, false
}

func floatCmp(x, y float64) int {
	switch {
	case x < y:
		return -1
	case x > y:
		return 1
	default:
		return 0
	}
}

// Equal implements `a == b`. Numeric types and strings are compared
// by value; everything else falls back to pointer identity.
func Equal(a, b Value) bool {
	if c, ok := cmpNumeric(a, b); ok {
		return c == 0
	}
	if as, ok := a.(*Str); ok {
		if bs, ok := b.(*Str); ok {
			return as.V == bs.V
		}
	}
	if _, ok := a.(*NoneType); ok {
		_, bn := b.(*NoneType)
		return bn
	}
	if ab, ok := a.(*Bool); ok {
		if bb, ok := b.(*Bool); ok {
			return ab.V == bb.V
		}
	}
	return a == b
}

// Compare implements COMPARE_OP for the six ordered relations.
func Compare(op int, a, b Value) (Value, error) {
	c, ok := cmpNumeric(a, b)
	if !ok {
		// strings support ordering too.
		if as, sok := a.(*Str); sok {
			if bs, sok := b.(*Str); sok {
				switch {
				case as.V < bs.V:
					c = -1
				case as.V > bs.V:
					c = 1
				default:
					c = 0
				}
				ok = true
			}
		}
	}
	switch op {
	case 2: // ==
		return Boxed(Equal(a, b)), nil
	case 3: // !=
		return Boxed(!Equal(a, b)), nil
	}
	if !ok {
		return nil, TypeError("'%s' not supported between instances of '%s' and '%s'",
			cmpSym(op), TypeName(a), TypeName(b))
	}
	switch op {
	case 0: // <
		return Boxed(c < 0), nil
	case 1: // <=
		return Boxed(c <= 0), nil
	case 4: // >
		return Boxed(c > 0), nil
	case 5: // >=
		return Boxed(c >= 0), nil
	}
	return nil, TypeError("compare: unknown op %d", op)
}

func cmpSym(op int) string {
	switch op {
	case 0:
		return "<"
	case 1:
		return "<="
	case 2:
		return "=="
	case 3:
		return "!="
	case 4:
		return ">"
	case 5:
		return ">="
	}
	return "?"
}
