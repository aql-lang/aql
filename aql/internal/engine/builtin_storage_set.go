package engine

func registerSet(r *Registry) {
	// All-suffix handler: "set foo 99" → args=[foo(key), 99(value)]
	setSuffixHandler := func(args []Value) ([]Value, error) {
		key := storeKey(args[0])
		r.Store[key] = args[1]
		return nil, nil
	}

	// Infix handler: "99 set foo" → args=[99(value), foo(key)]
	setInfixHandler := func(args []Value) ([]Value, error) {
		key := storeKey(args[1])
		r.Store[key] = args[0]
		return nil, nil
	}

	r.Register("set",
		// All-suffix: key first, value second
		Signature{
			Args:    []Type{TString, TAny},
			Handler: setSuffixHandler,
		},
		Signature{
			Args:    []Type{TWord, TAny},
			Handler: setSuffixHandler,
		},
		// Infix: value first (prefix), key second (suffix)
		Signature{
			Args:    []Type{TAny, TString},
			Handler: setInfixHandler,
		},
		Signature{
			Args:    []Type{TAny, TWord},
			Handler: setInfixHandler,
		},
		// Fallback
		Signature{
			Args:    []Type{TAny, TAny},
			Handler: setSuffixHandler,
		},
	)
}
