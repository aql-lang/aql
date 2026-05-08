package engine

func RegisterRot(r *Registry) {
	r.RegisterNativeFunc(NativeFunc{
		Name:              "rot",
		ForwardPrecedence: false,
		Signatures: []NativeSig{{
			Args: []Type{TAny, TAny, TAny},
			// Unified §1.4: args[0]=top (c), args[1]=middle (b), args[2]=deepest (a).
			// rot rotates the deepest to the top:
			// stack [a, b, c] → [b, c, a].
			// Splice in source order: [args[1], args[0], args[2]] → [b, c, a].
			Handler: func(args []Value, _ map[string]Value, _ []Value, _ *Registry) ([]Value, error) {
				return []Value{args[1], args[0], args[2]}, nil
			},
			ReturnsFn: ReturnsIdentity(1, 0, 2),
		}},
	})
}
