package engine

func registerNand(r *Registry) {
	handler := func(args []Value, _ map[string]Value, _ []Value, _ *Registry) ([]Value, error) {
		_as7, _ := args[0].AsBoolean()
		_as6, _ := args[1].AsBoolean()
		return []Value{NewBoolean(!(_as7 && _as6))}, nil
	}

	r.RegisterNativeFunc(NativeFunc{
		Name:              "nand",
		ForwardPrecedence: true,
		SkipSafetyCheck:   true,
		Signatures: []NativeSig{{
			Args:    []Type{TBoolean, TBoolean},
			Handler: handler,
		}},
	})
}
