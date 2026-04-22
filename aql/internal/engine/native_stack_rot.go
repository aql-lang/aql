package engine

func RegisterRot(r *Registry) {
	r.RegisterNativeFunc(NativeFunc{
		Name:              "rot",
		ForwardPrecedence: false,
		Signatures: []NativeSig{{
			Args: []Type{TAny, TAny, TAny},
			Handler: func(args []Value, _ map[string]Value, _ []Value, _ *Registry) ([]Value, error) {
				return []Value{args[1], args[2], args[0]}, nil
			},
			ReturnsFn: ReturnsIdentity(1, 2, 0),
		}},
	})
}
