package engine

import (
	"fmt"
	"math"
)

func registerPow(r *Registry) {
	// pow: integer exponentiation [int, int] -> [int]
	registerBinaryIntOp(r, "pow", 2, func(a, b int64) (int64, error) {
		if b < 0 {
			return 0, fmt.Errorf("pow: negative exponent %d", b)
		}
		result := int64(1)
		base := a
		exp := b
		for exp > 0 {
			if exp%2 == 1 {
				result *= base
			}
			base *= base
			exp /= 2
		}
		return result, nil
	})
	// pow: decimal exponentiation
	registerBinaryNumOp(r, "pow", 2, func(a, b float64) (float64, error) {
		return math.Pow(a, b), nil
	})
}
