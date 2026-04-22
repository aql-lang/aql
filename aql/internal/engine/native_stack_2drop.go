package engine

func Register2drop(r *Registry) {
	r.RegisterNativeFunc(NativeFunc{
		Name:              "2drop",
		ForwardPrecedence: false,
		Signatures: []NativeSig{{
			Args: []Type{TAny, TAny},
			Handler: func(args []Value, _ map[string]Value, _ []Value, _ *Registry) ([]Value, error) {
				return nil, nil
			},
			Returns: []Type{}, // 2drop consumes both args and returns nothing
		}},
	})
}
