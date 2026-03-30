package engine

import "fmt"

func registerDiv(r *Registry) {
	// With forward-first matching, args are reversed relative to natural order.
	// Swap operands so `a b div` = a / b and `div b a` = a / b.
	registerBinaryIntOp(r, "div", func(a, b int64) (int64, error) {
		if a == 0 {
			return 0, fmt.Errorf("division by zero")
		}
		return b / a, nil
	})
	registerBinaryNumOp(r, "div", func(a, b float64) (float64, error) {
		if a == 0 {
			return 0, fmt.Errorf("division by zero")
		}
		return b / a, nil
	})
}
