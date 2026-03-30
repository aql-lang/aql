package engine

func registerSub(r *Registry) {
	// With forward-first matching, args[0] is the forward/top value and
	// args[1] is the deeper/earlier value. To make `a b sub` = a minus b,
	// the handler computes args[1] - args[0].
	registerBinaryIntOp(r, "sub", func(a, b int64) (int64, error) { return b - a, nil })
	registerBinaryNumOp(r, "sub", func(a, b float64) (float64, error) { return b - a, nil })
}
