package engine

import (
	"fmt"
	"math"
)

func registerPow(r *Registry) {
	// Signature [Integer, Integer]: args[0] = nearest to word (top/forward),
	// args[1] = farther (deeper/later). `a b pow` → args=[b,a] → a^b.
	intHandler := func(args []Value, _ map[string]Value, _ []Value, _ *Registry) ([]Value, error) {
		_as2, _ := args[0].AsInteger()
		_as1, _ := args[1].AsInteger()
		if _as2 < 0 {
			return nil, fmt.Errorf("pow: negative exponent %d", _as2)
		}
		result := int64(1)
		base := _as1
		exp := _as2
		for exp > 0 {
			if exp%2 == 1 {
				result *= base
			}
			base *= base
			exp /= 2
		}
		return []Value{NewInteger(result)}, nil
	}

	// pow: decimal exponentiation
	numHandler := func(args []Value, _ map[string]Value, _ []Value, _ *Registry) ([]Value, error) {
		_as4, _ := args[0].AsNumber()
		_as3, _ := args[1].AsNumber()
		return []Value{NewDecimal(math.Pow(_as3, _as4))}, nil
	}

	r.RegisterNativeFunc(NativeFunc{
		Name:              "pow",
		ForwardPrecedence: true,
		Signatures: []NativeSig{
			{
				Args:    []Type{TInteger, TInteger},
				Handler: intHandler,
			},
			{
				Args:    []Type{TDecimal, TDecimal},
				Handler: numHandler,
			},
			{
				Args:    []Type{TNumber, TDecimal},
				Handler: numHandler,
			},
			{
				Args:    []Type{TDecimal, TNumber},
				Handler: numHandler,
			},
		},
	})
}
