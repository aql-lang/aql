package engine

func registerSwap(r *Registry) {
	r.RegisterNativeFunc(NativeFunc{
		Name:              "swap",
		ForwardPrecedence: false,
		Signatures: []NativeSig{{
			Args: []Type{TAny, TAny},
			Handler: func(args []Value, _ map[string]Value, _ []Value, _ *Registry) ([]Value, error) {
				return []Value{args[1], args[0]}, nil
			},
		}},
	})
}
