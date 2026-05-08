package engine

func Register2dup(r *Registry) {
	r.RegisterNativeFunc(NativeFunc{
		Name:              "2dup",
		ForwardPrecedence: false,
		Signatures: []NativeSig{{
			Args: []Type{TAny, TAny},
			// Unified §1.4: args[0]=top (a), args[1]=next-deeper (b).
			// 2dup duplicates the top two:
			// stack [b, a] → [b, a, b, a].
			// Output [args[1], args[0], args[1], args[0]].
			Handler: func(args []Value, _ map[string]Value, _ []Value, _ *Registry) ([]Value, error) {
				return []Value{args[1], args[0], args[1], args[0]}, nil
			},
			ReturnsFn: ReturnsIdentity(1, 0, 1, 0),
		}},
	})
}
