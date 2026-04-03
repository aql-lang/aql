package engine

func register2over(r *Registry) {
	r.RegisterNativeFunc(NativeFunc{
		Name:              "2over",
		ForwardPrecedence: false,
		Signatures: []NativeSig{{
			Args: []Type{TAny, TAny, TAny, TAny},
			Handler: func(args []Value, _ map[string]Value, _ []Value, _ *Registry) ([]Value, error) {
				return []Value{args[0], args[1], args[2], args[3], args[0], args[1]}, nil
			},
		}},
	})
}
