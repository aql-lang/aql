package engine

func RegisterTuck(r *Registry) {
	r.RegisterNativeFunc(NativeFunc{
		Name:              "tuck",
		ForwardPrecedence: false,
		Signatures: []NativeSig{{
			Args: []Type{TAny, TAny},
			Handler: func(args []Value, _ map[string]Value, _ []Value, _ *Registry) ([]Value, error) {
				return []Value{args[1], args[0], args[1]}, nil
			},
			ReturnsFn: ReturnsIdentity(1, 0, 1),
		}},
	})
}
