package runtime

// Dict is Python's dict. v0.1 backs it with a slice of pairs to keep
// insertion order and to sidestep writing a hash table over rt.Value.
// This is O(n) lookup but the fixtures do not stress it; a proper
// hash can come when the semantics stop moving.
type Dict struct {
	Keys []Value
	Vals []Value
}

func NewDict() *Dict { return &Dict{} }

// Lookup returns (value, true) if k is present.
func (d *Dict) Lookup(k Value) (Value, bool) {
	for i, kk := range d.Keys {
		if Equal(kk, k) {
			return d.Vals[i], true
		}
	}
	return nil, false
}

// Set inserts or overwrites.
func (d *Dict) Set(k, v Value) {
	for i, kk := range d.Keys {
		if Equal(kk, k) {
			d.Vals[i] = v
			return
		}
	}
	d.Keys = append(d.Keys, k)
	d.Vals = append(d.Vals, v)
}

// NewDictFromPairs builds a dict from interleaved k,v args. Generated
// code emits this for dict literals.
func NewDictFromPairs(kv ...Value) *Dict {
	d := &Dict{}
	for i := 0; i+1 < len(kv); i += 2 {
		d.Set(kv[i], kv[i+1])
	}
	return d
}

// GetItem implements `x[k]` for list/tuple/str/dict.
func GetItem(x, k Value) (Value, error) {
	switch c := x.(type) {
	case *List:
		i, err := asInt64(k)
		if err != nil {
			return nil, TypeError("list indices must be integers, not %s", TypeName(k))
		}
		n := int64(len(c.V))
		if i < 0 {
			i += n
		}
		if i < 0 || i >= n {
			return nil, TypeError("IndexError: list index out of range")
		}
		return c.V[i], nil
	case *Tuple:
		i, err := asInt64(k)
		if err != nil {
			return nil, TypeError("tuple indices must be integers, not %s", TypeName(k))
		}
		n := int64(len(c.V))
		if i < 0 {
			i += n
		}
		if i < 0 || i >= n {
			return nil, TypeError("IndexError: tuple index out of range")
		}
		return c.V[i], nil
	case *Str:
		i, err := asInt64(k)
		if err != nil {
			return nil, TypeError("string indices must be integers, not %s", TypeName(k))
		}
		runes := []rune(c.V)
		n := int64(len(runes))
		if i < 0 {
			i += n
		}
		if i < 0 || i >= n {
			return nil, TypeError("IndexError: string index out of range")
		}
		return &Str{V: string(runes[i])}, nil
	case *Dict:
		if v, ok := c.Lookup(k); ok {
			return v, nil
		}
		return nil, TypeError("KeyError: %s", Repr(k))
	}
	return nil, TypeError("'%s' object is not subscriptable", TypeName(x))
}

// SetItem implements `x[k] = v`.
func SetItem(x, k, v Value) error {
	switch c := x.(type) {
	case *List:
		i, err := asInt64(k)
		if err != nil {
			return TypeError("list indices must be integers, not %s", TypeName(k))
		}
		n := int64(len(c.V))
		if i < 0 {
			i += n
		}
		if i < 0 || i >= n {
			return TypeError("IndexError: list assignment index out of range")
		}
		c.V[i] = v
		return nil
	case *Dict:
		c.Set(k, v)
		return nil
	}
	return TypeError("'%s' object does not support item assignment", TypeName(x))
}
