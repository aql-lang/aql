package engine

func Register2dup(r *Registry) {
	r.RegisterNativeFunc(NativeFunc{
		Name:              "2dup",
		ForwardPrecedence: false,
		Signatures: []NativeSig{{
			Args: []Type{TAny, TAny},
			Handler: func(args []Value, _ map[string]Value, _ []Value, _ *Registry) ([]Value, error) {
				return []Value{args[0], args[1], args[0], args[1]}, nil
			},
			ReturnsFn: ReturnsIdentity(0, 1, 0, 1),
		}},
	})
}
