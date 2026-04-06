package engine

import (
	"fmt"
	"math"
)

func registerMod(r *Registry) {
	registerBinaryMathWord(r, "mod",
		func(a, b int64) (Value, error) {
			if b == 0 {
				return Value{}, fmt.Errorf("modulo by zero")
			}
			return NewInteger(a % b), nil
		},
		func(a, b float64) (Value, error) {
			if b == 0 {
				return Value{}, fmt.Errorf("modulo by zero")
			}
			return NewDecimal(math.Mod(a, b)), nil
		},
	)
}
