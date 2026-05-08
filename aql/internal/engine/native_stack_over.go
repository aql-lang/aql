package engine

func RegisterOver(r *Registry) {
	r.RegisterNativeFunc(NativeFunc{
		Name:              "over",
		ForwardPrecedence: false,
		Signatures: []NativeSig{{
			Args: []Type{TAny, TAny},
			// Unified §1.4: args[0]=top, args[1]=next-deeper.
			// over duplicates the second-deepest value to the top:
			// stack [b, a] → [b, a, b] (a was on top, b deeper).
			// Splice in source order: [args[1], args[0], args[1]] → [b, a, b].
			Handler: func(args []Value, _ map[string]Value, _ []Value, _ *Registry) ([]Value, error) {
				return []Value{args[1], args[0], args[1]}, nil
			},
			ReturnsFn: ReturnsIdentity(1, 0, 1),
		}},
	})
}
