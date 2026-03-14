package engine

func registerMin(r *Registry) {
	// min: [int, int] -> [int] (suffix precedence)
	registerBinaryIntOp(r, "min", 1, func(a, b int64) (int64, error) {
		if a < b {
			return a, nil
		}
		return b, nil
	})

	registerBinaryNumOp(r, "min", 1, func(a, b float64) (float64, error) {
		if a < b {
			return a, nil
		}
		return b, nil
	})
}
