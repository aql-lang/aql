package engine

func RegisterSwap(r *Registry) {
	r.RegisterNativeFunc(NativeFunc{
		Name:              "swap",
		ForwardPrecedence: false,
		Signatures: []NativeSig{{
			Args: []Type{TAny, TAny},
			// Unified §1.4 dispatch: args[0]=stack top, args[1]=next-deeper.
			// Returning [args[0], args[1]] writes them back in source order
			// (deeper-then-top) — the two values come out swapped.
			Handler: func(args []Value, _ map[string]Value, _ []Value, _ *Registry) ([]Value, error) {
				return []Value{args[0], args[1]}, nil
			},
			ReturnsFn: ReturnsIdentity(0, 1),
		}},
	})
}
