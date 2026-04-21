package runtime

// Call invokes a callable. v0.1 supports positional args only.
func Call(fn Value, args ...Value) (Value, error) {
	f, ok := fn.(*Func)
	if !ok {
		return nil, TypeError("'%s' object is not callable", TypeName(fn))
	}
	return f.Impl(args)
}
