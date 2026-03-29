package engine

import (
	"fmt"
	"math"
)

func registerMod(r *Registry) {
	registerBinaryIntOp(r, "mod", func(a, b int64) (int64, error) {
		if b == 0 {
			return 0, fmt.Errorf("modulo by zero")
		}
		return a % b, nil
	})
	registerBinaryNumOp(r, "mod", func(a, b float64) (float64, error) {
		if b == 0 {
			return 0, fmt.Errorf("modulo by zero")
		}
		return math.Mod(a, b), nil
	})
}
