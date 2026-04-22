package engine

func Register2swap(r *Registry) {
	r.RegisterNativeFunc(NativeFunc{
		Name:              "2swap",
		ForwardPrecedence: false,
		Signatures: []NativeSig{{
			Args: []Type{TAny, TAny, TAny, TAny},
			Handler: func(args []Value, _ map[string]Value, _ []Value, _ *Registry) ([]Value, error) {
				return []Value{args[2], args[3], args[0], args[1]}, nil
			},
			ReturnsFn: ReturnsIdentity(2, 3, 0, 1),
		}},
	})
}
