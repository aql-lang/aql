package engine

func register2drop(r *Registry) {
	r.RegisterNativeFunc(NativeFunc{
		Name:              "2drop",
		ForwardPrecedence: false,
		Signatures: []NativeSig{{
			Args: []Type{TAny, TAny},
			Handler: func(args []Value, _ map[string]Value, _ []Value, _ *Registry) ([]Value, error) {
				return nil, nil
			},
		}},
	})
}
