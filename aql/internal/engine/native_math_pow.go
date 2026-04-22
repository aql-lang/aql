package engine

import (
	"fmt"
	"math"
)

func RegisterPow(r *Registry) {
	registerBinaryMathWord(r, "pow",
		func(base, exp float64) (Value, error) { return NewDecimal(math.Pow(base, exp)), nil },
		func(base, exp int64) (Value, error) {
			if exp < 0 {
				return Value{}, fmt.Errorf("pow: negative exponent %d", exp)
			}
			result := int64(1)
			b := base
			e := exp
			for e > 0 {
				if e%2 == 1 {
					result *= b
				}
				b *= b
				e /= 2
			}
			return NewInteger(result), nil
		},
	)
}
