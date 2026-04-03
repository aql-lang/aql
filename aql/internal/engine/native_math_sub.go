package engine

func registerSub(r *Registry) {
	// Signature [Integer, Integer]: args[0] = nearest to word (top/forward),
	// args[1] = farther (deeper/later). `a b sub` → args=[b,a] → a-b.
	registerBinaryIntOp(r, "sub", func(a, b int64) (int64, error) { return b - a, nil })
	registerBinaryNumOp(r, "sub", func(a, b float64) (float64, error) { return b - a, nil })
}
