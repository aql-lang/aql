package engine

func RegisterMul(r *Registry) {
	r.RegisterNativeFunc(NativeFunc{
		Name:              "mul",
		ForwardPrecedence: true,
		Signatures: []NativeSig{{
			Args: []Type{TNumber, TNumber},
			Handler: numericBinaryHandler(
				func(a, b int64) (Value, error) { return NewInteger(a * b), nil },
				func(a, b float64) (Value, error) { return NewDecimal(a * b), nil },
			),
			ReturnsFn: ReturnsNumericBinary(),
		}},
	})
}
