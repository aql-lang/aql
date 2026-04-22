package engine

func RegisterMul(r *Registry) {
	registerBinaryMathWord(r, "mul",
		func(a, b float64) (Value, error) { return NewDecimal(a * b), nil },
		func(a, b int64) (Value, error) { return NewInteger(a * b), nil },
	)
}
