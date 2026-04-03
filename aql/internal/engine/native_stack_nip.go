package engine

func registerNip(r *Registry) {
	r.RegisterNativeFunc(NativeFunc{
		Name:              "nip",
		ForwardPrecedence: false,
		Signatures: []NativeSig{{
			Args: []Type{TAny, TAny},
			Handler: func(args []Value, _ map[string]Value, _ []Value, _ *Registry) ([]Value, error) {
				return []Value{args[1]}, nil
			},
		}},
	})
}
