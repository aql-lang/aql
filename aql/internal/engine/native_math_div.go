package engine

import "fmt"

func registerDiv(r *Registry) {
	registerBinaryMathWord(r, "div",
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
	)
}
