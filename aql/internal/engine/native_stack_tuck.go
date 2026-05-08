package engine

func RegisterTuck(r *Registry) {
	r.RegisterNativeFunc(NativeFunc{
		Name:              "tuck",
		ForwardPrecedence: false,
		Signatures: []NativeSig{{
			Args: []Type{TAny, TAny},
			// Unified §1.4: args[0]=top (a), args[1]=next-deeper (b).
			// tuck copies the top below the second-deepest:
			// stack [b, a] → [a, b, a]. Output [args[0], args[1], args[0]].
			Handler: func(args []Value, _ map[string]Value, _ []Value, _ *Registry) ([]Value, error) {
				return []Value{args[0], args[1], args[0]}, nil
			},
			ReturnsFn: ReturnsIdentity(0, 1, 0),
		}},
	})
}
