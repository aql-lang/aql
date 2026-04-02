package engine

func registerMul(r *Registry) {
	registerBinaryIntOp(r, "mul", func(a, b int64) (int64, error) { return a * b, nil })
	registerBinaryNumOp(r, "mul", func(a, b float64) (float64, error) { return a * b, nil })
}
