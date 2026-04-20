package engine

func registerDrop(r *Registry) {
	r.RegisterNativeFunc(NativeFunc{
		Name:              "drop",
		ForwardPrecedence: false,
		Signatures: []NativeSig{{
			Args: []Type{TAny},
			Handler: func(args []Value, _ map[string]Value, _ []Value, _ *Registry) ([]Value, error) {
				return nil, nil
			},
			Returns: []Type{}, // drop consumes its arg and returns nothing
		}},
	})
}
