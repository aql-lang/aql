package engine

func registerDepth(r *Registry) {
	r.RegisterNativeFunc(NativeFunc{
		Name:              "depth",
		ForwardPrecedence: false,
		Signatures: []NativeSig{{
			FullStack: true,
			Handler: func(args []Value, _ map[string]Value, stack []Value, _ *Registry) ([]Value, error) {
				return append(stack, NewInteger(int64(len(stack)))), nil
			},
			// depth appends a new Integer to the full stack.
			// The carrier path only models the appended value.
			Returns: []Type{TInteger},
		}},
	})
}
