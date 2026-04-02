package engine

func registerAdd(r *Registry) {
	registerBinaryIntOp(r, "add", func(a, b int64) (int64, error) { return a + b, nil })

	// String concatenation for add: [TScalar, TScalar] converts both
	// args to strings and concatenates. More specific signatures
	// (e.g. [TInteger, TInteger]) win due to higher specificity.
	// args[0] = nearest (top/forward), args[1] = farther. `a add b` → args=[b,a] → a+b.
	concatHandler := func(args []Value, _ map[string]Value, _ []Value, _ *Registry) ([]Value, error) {
		return []Value{NewString(valToString(args[1]) + valToString(args[0]))}, nil
	}
	r.Register("add", Signature{
		Args:    []Type{TScalar, TScalar},
		Handler: concatHandler,
	})
	registerBinaryNumOp(r, "add", func(a, b float64) (float64, error) { return a + b, nil })
}
