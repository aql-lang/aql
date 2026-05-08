package engine

import "fmt"

func RegisterDiv(r *Registry) {
	r.RegisterNativeFunc(NativeFunc{
		Name:              "div",
		ForwardPrecedence: true,
		Signatures: []NativeSig{{
			Args: []Type{TNumber, TNumber},
			Handler: numericBinaryHandler(
				func(a, b int64) (Value, error) {
					if b == 0 {
						return Value{}, fmt.Errorf("division by zero")
					}
					return NewInteger(a / b), nil
				},
				func(a, b float64) (Value, error) {
					if b == 0 {
						return Value{}, fmt.Errorf("division by zero")
					}
					return NewDecimal(a / b), nil
				},
			),
			ReturnsFn: ReturnsNumericBinary(),
		}},
	})
}
