package engine

func registerAdd(r *Registry) {
	registerBinaryIntOp(r, "add", 1, func(a, b int64) (int64, error) { return a + b, nil })

	// String concatenation for add: [TScalar, TScalar] converts both
	// args to strings and concatenates. More specific signatures
	// (e.g. [TInteger, TInteger]) win due to higher specificity.
	concatHandler := func(args []Value) ([]Value, error) {
		return []Value{NewString(valToString(args[0]) + valToString(args[1]))}, nil
	}
	r.Register("add", Signature{
		Args:       []Type{TScalar, TScalar},
		Precedence: 1,
		Handler:    concatHandler,
	})
	// Atom overloads: atoms are now under Word/Atom, not Scalar.
	r.Register("add", Signature{
		Args:       []Type{TAtom, TScalar},
		Precedence: 1,
		Handler:    concatHandler,
	})
	r.Register("add", Signature{
		Args:       []Type{TScalar, TAtom},
		Precedence: 1,
		Handler:    concatHandler,
	})
	r.Register("add", Signature{
		Args:       []Type{TAtom, TAtom},
		Precedence: 1,
		Handler:    concatHandler,
	})
	registerBinaryNumOp(r, "add", 1, func(a, b float64) (float64, error) { return a + b, nil })
}
