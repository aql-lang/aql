package engine

func RegisterBase(r *Registry) {
	baseHandler := func(args []Value, _ map[string]Value, _ []Value, _ *Registry) ([]Value, error) {
		v := args[0]
		t := v.VType
		result, err := BaseValue(t)
		if err != nil {
			return nil, err
		}
		return []Value{result}, nil
	}

	r.RegisterNativeFunc(NativeFunc{
		Name:              "base",
		ForwardPrecedence: true,
		Signatures: []NativeSig{{
			Args:    []Type{TAny},
			Handler: baseHandler,
			// base returns the zero value of the arg's described
			// type; at the carrier level, this is the arg's type.
			ReturnsFn: ReturnsIdentity(0),
		}},
	})
}

// BaseValue: re-exported from aqleng via aliases.go

// BaseValueForConstraint: re-exported from aqleng via aliases.go
