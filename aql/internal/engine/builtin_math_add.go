package engine

func registerAdd(r *Registry) {
	registerBinaryIntOp(r, "add", 1, func(a, b int64) (int64, error) { return a + b, nil })

	// String concatenation for add: [TScalar, TScalar] converts both
	// args to strings and concatenates. The [TInteger, TInteger] sig
	// from registerBinaryIntOp has higher specificity (204 vs 202) so
	// it wins for two integers when combined with prefix/peek bonuses.
	r.Register("add", Signature{
		Args:       []Type{TScalar, TScalar},
		Precedence: 1,
		Handler: func(args []Value) ([]Value, error) {
			return []Value{NewString(valToString(args[0]) + valToString(args[1]))}, nil
		},
	})
	registerBinaryNumOp(r, "add", 1, func(a, b float64) (float64, error) { return a + b, nil })
}
