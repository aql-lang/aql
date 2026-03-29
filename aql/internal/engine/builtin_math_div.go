package engine

import "fmt"

func registerDiv(r *Registry) {
	registerBinaryIntOp(r, "div", func(a, b int64) (int64, error) {
		if b == 0 {
			return 0, fmt.Errorf("division by zero")
		}
		return a / b, nil
	})
	registerBinaryNumOp(r, "div", func(a, b float64) (float64, error) {
		if b == 0 {
			return 0, fmt.Errorf("division by zero")
		}
		return a / b, nil
	})
}
