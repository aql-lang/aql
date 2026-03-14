package engine

func registerMul(r *Registry) {
	registerBinaryIntOp(r, "mul", 2, func(a, b int64) (int64, error) { return a * b, nil })
	registerBinaryNumOp(r, "mul", 2, func(a, b float64) (float64, error) { return a * b, nil })
}
