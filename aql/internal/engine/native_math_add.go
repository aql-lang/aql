package engine

func registerAdd(r *Registry) {
	intHandler := func(args []Value, _ map[string]Value, _ []Value, _ *Registry) ([]Value, error) {
		_as2, _ := args[0].AsInteger()
		_as1, _ := args[1].AsInteger()
		result := _as2 + _as1
		return []Value{NewInteger(result)}, nil
	}

	// String concatenation for add: [TScalar, TScalar] converts both
	// args to strings and concatenates. More specific signatures
	// (e.g. [TInteger, TInteger]) win due to higher specificity.
	// args[0] = nearest (top/forward), args[1] = farther. `a add b` → args=[b,a] → a+b.
	concatHandler := func(args []Value, _ map[string]Value, _ []Value, _ *Registry) ([]Value, error) {
		return []Value{NewString(valToString(args[1]) + valToString(args[0]))}, nil
	}

	numHandler := func(args []Value, _ map[string]Value, _ []Value, _ *Registry) ([]Value, error) {
		_as4, _ := args[0].AsNumber()
		_as3, _ := args[1].AsNumber()
		result := _as4 + _as3
		return []Value{NewDecimal(result)}, nil
	}

	r.RegisterNativeFunc(NativeFunc{
		Name:              "add",
		ForwardPrecedence: true,
		Signatures: []NativeSig{
			{
				Args:    []Type{TInteger, TInteger},
				Handler: intHandler,
			},
			{
				Args:    []Type{TScalar, TScalar},
				Handler: concatHandler,
			},
			{
				Args:    []Type{TDecimal, TDecimal},
				Handler: numHandler,
			},
			{
				Args:    []Type{TNumber, TDecimal},
				Handler: numHandler,
			},
			{
				Args:    []Type{TDecimal, TNumber},
				Handler: numHandler,
			},
		},
	})
}
