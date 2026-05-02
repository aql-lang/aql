package engine

func RegisterOtherwise(r *Registry) {
	// otherwise is null-coalescing: returns the first operand when it
	// is not None, else returns the second operand. Distinct from `or`
	// which short-circuits on falsy (so `0 or 5` = 5 but `0 otherwise 5` = 0).
	//
	// args[0] = nearest (top/forward), args[1] = farther (stack).
	// Source order: `a otherwise b` → args[1]=a, args[0]=b.
	handler := func(args []Value, _ map[string]Value, _ []Value, _ *Registry) ([]Value, error) {
		if args[1].VType.Equal(TNone) {
			return []Value{args[0]}, nil
		}
		return []Value{args[1]}, nil
	}

	r.RegisterNativeFunc(NativeFunc{
		Name:              "otherwise",
		ForwardPrecedence: true,
		Signatures: []NativeSig{
			{
				Args:       []Type{TAny, TAny},
				BarrierPos: 1,
				Handler:    handler,
				Returns:    []Type{TAny},
			},
		},
	})
}
