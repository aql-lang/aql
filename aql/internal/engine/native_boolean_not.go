package engine

func registerNot(r *Registry) {
	r.RegisterNativeFunc(NativeFunc{
		Name:              "not",
		ForwardPrecedence: true,
		Signatures: []NativeSig{{
			Args: []Type{TBoolean},
			Handler: func(args []Value, _ map[string]Value, _ []Value, _ *Registry) ([]Value, error) {
				_as0, _ := args[0].AsBoolean()
				return []Value{NewBoolean(!_as0)}, nil
			},
			Returns: []Type{TBoolean},
		}},
	})
}
