package engine

import "fmt"

func registerDiv(r *Registry) {
	// Signature [Integer, Integer]: args[0] = nearest to word (top/forward),
	// args[1] = farther (deeper/later). `a b div` → args=[b,a] → a/b.
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
