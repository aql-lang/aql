package engine

func registerMax(r *Registry) {
	// max: [int, int] -> [int] (forward precedence)
	registerBinaryIntOp(r, "max", func(a, b int64) (int64, error) {
		if a > b {
			return a, nil
		}
		return b, nil
	})

	registerBinaryNumOp(r, "max", func(a, b float64) (float64, error) {
		if a > b {
			return a, nil
		}
		return b, nil
	})
}
