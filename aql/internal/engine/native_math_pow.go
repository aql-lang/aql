package engine

import (
	"fmt"
	"math"
)

func registerPow(r *Registry) {
	// Signature [Integer, Integer]: args[0] = nearest to word (top/forward),
	// args[1] = farther (deeper/later). `a b pow` → args=[b,a] → a^b.
	registerBinaryIntOp(r, "pow", func(a, b int64) (int64, error) {
		if a < 0 {
			return 0, fmt.Errorf("pow: negative exponent %d", a)
		}
		result := int64(1)
		base := b
		exp := a
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
	registerBinaryNumOp(r, "pow", func(a, b float64) (float64, error) {
		return math.Pow(b, a), nil
	})
}
