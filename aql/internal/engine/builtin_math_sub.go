package engine

func registerSub(r *Registry) {
	registerBinaryIntOp(r, "sub", func(a, b int64) (int64, error) { return a - b, nil })
	registerBinaryNumOp(r, "sub", func(a, b float64) (float64, error) { return a - b, nil })
}
