package engine

func RegisterAnd(r *Registry) {
	// and short-circuits and returns the "winning" value rather than a
	// pure boolean. The first operand (in source order, = args[1] / the
	// farther/stack arg) wins when falsy; otherwise the second operand
	// (= args[0] / the nearest/forward arg) is returned. Truthiness is
	// determined by CoerceBoolean for non-boolean inputs.
	handler := func(args []Value, _ map[string]Value, _ []Value, _ *Registry) ([]Value, error) {
		if !CoerceBoolean(args[1]) {
			return []Value{args[1]}, nil
		}
		return []Value{args[0]}, nil
	}

	r.RegisterNativeFunc(NativeFunc{
		Name:              "and",
		ForwardPrecedence: true,
		Signatures: []NativeSig{
			{
				Args:    []Type{TBoolean, TBoolean},
				Handler: handler,
				Returns: []Type{TBoolean},
			},
			{
				Args:    []Type{TAny, TAny},
				Handler: handler,
				Returns: []Type{TAny},
			},
		},
	})
}
