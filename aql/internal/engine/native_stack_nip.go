package engine

func RegisterNip(r *Registry) {
	r.RegisterNativeFunc(NativeFunc{
		Name:              "nip",
		ForwardPrecedence: false,
		Signatures: []NativeSig{{
			Args: []Type{TAny, TAny},
			// Unified §1.4: args[0]=top, args[1]=next-deeper.
			// nip drops the second-deepest, keeps the top:
			// stack [b, a] → [a]. Output [args[0]].
			Handler: func(args []Value, _ map[string]Value, _ []Value, _ *Registry) ([]Value, error) {
				return []Value{args[0]}, nil
			},
			ReturnsFn: ReturnsIdentity(0),
		}},
	})
}
