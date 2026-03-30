package engine

import (
	"fmt"
	"math"
)

func registerMod(r *Registry) {
	// With forward-first matching, args are reversed relative to natural order.
	// Swap operands so `a b mod` = a % b and `mod b a` = a % b.
	registerBinaryIntOp(r, "mod", func(a, b int64) (int64, error) {
		if a == 0 {
			return 0, fmt.Errorf("modulo by zero")
		}
		return b % a, nil
	})
	registerBinaryNumOp(r, "mod", func(a, b float64) (float64, error) {
		if a == 0 {
			return 0, fmt.Errorf("modulo by zero")
		}
		return math.Mod(b, a), nil
	})
}
