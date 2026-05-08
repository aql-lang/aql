package engine

func Register2swap(r *Registry) {
	r.RegisterNativeFunc(NativeFunc{
		Name:              "2swap",
		ForwardPrecedence: false,
		Signatures: []NativeSig{{
			Args: []Type{TAny, TAny, TAny, TAny},
			// Unified §1.4: args[0]=top (a), args[1]=b, args[2]=c, args[3]=deepest (d).
			// 2swap exchanges the top two pairs:
			// stack [d, c, b, a] → [b, a, d, c].
			// Output [args[1], args[0], args[3], args[2]].
			Handler: func(args []Value, _ map[string]Value, _ []Value, _ *Registry) ([]Value, error) {
				return []Value{args[1], args[0], args[3], args[2]}, nil
			},
			ReturnsFn: ReturnsIdentity(1, 0, 3, 2),
		}},
	})
}
