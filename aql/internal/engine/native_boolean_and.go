package engine

func RegisterAnd(r *Registry) {
	// and coerces non-boolean arguments via coerceBoolean (same rules as
	// `convert boolean`). The [TBoolean, TBoolean] signature wins for
	// boolean inputs so the static type checker keeps boolean precision;
	// other inputs fall through to the [TAny, TAny] coerce path.
	boolHandler := func(args []Value, _ map[string]Value, _ []Value, _ *Registry) ([]Value, error) {
		a, _ := args[0].AsBoolean()
		b, _ := args[1].AsBoolean()
		return []Value{NewBoolean(a && b)}, nil
	}
	coerceHandler := func(args []Value, _ map[string]Value, _ []Value, _ *Registry) ([]Value, error) {
		a := coerceBoolean(args[0])
		b := coerceBoolean(args[1])
		return []Value{NewBoolean(a && b)}, nil
	}

	r.RegisterNativeFunc(NativeFunc{
		Name:              "and",
		ForwardPrecedence: true,
		Signatures: []NativeSig{
			{
				Args:    []Type{TBoolean, TBoolean},
				Handler: boolHandler,
				Returns: []Type{TBoolean},
			},
			{
				Args:    []Type{TAny, TAny},
				Handler: coerceHandler,
				Returns: []Type{TBoolean},
			},
		},
	})
}
