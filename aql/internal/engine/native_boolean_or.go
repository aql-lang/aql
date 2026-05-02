package engine

func RegisterOr(r *Registry) {
	// Boolean or. BarrierPos=1 prevents greedy forward consumption of
	// chained `or` words.
	boolHandler := func(args []Value, _ map[string]Value, _ []Value, _ *Registry) ([]Value, error) {
		_as1, _ := args[0].AsBoolean()
		_as0, _ := args[1].AsBoolean()
		return []Value{NewBoolean(_as1 || _as0)}, nil
	}

	r.RegisterNativeFunc(NativeFunc{
		Name:              "or",
		ForwardPrecedence: true,
		Signatures: []NativeSig{
			{
				Args:       []Type{TBoolean, TBoolean},
				BarrierPos: 1,
				Handler:    boolHandler,
				Returns:    []Type{TBoolean},
			},
		},
	})
}
