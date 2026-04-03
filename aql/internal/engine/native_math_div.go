package engine

import "fmt"

func registerDiv(r *Registry) {
	// Signature [Integer, Integer]: args[0] = nearest to word (top/forward),
	// args[1] = farther (deeper/later). `a b div` → args=[b,a] → a/b.
	intHandler := func(args []Value, _ map[string]Value, _ []Value, _ *Registry) ([]Value, error) {
		_as2, _ := args[0].AsInteger()
		_as1, _ := args[1].AsInteger()
		if _as2 == 0 {
			return nil, fmt.Errorf("division by zero")
		}
		return []Value{NewInteger(_as1 / _as2)}, nil
	}

	numHandler := func(args []Value, _ map[string]Value, _ []Value, _ *Registry) ([]Value, error) {
		_as4, _ := args[0].AsNumber()
		_as3, _ := args[1].AsNumber()
		if _as4 == 0 {
			return nil, fmt.Errorf("division by zero")
		}
		return []Value{NewDecimal(_as3 / _as4)}, nil
	}

	r.RegisterNativeFunc(NativeFunc{
		Name:              "div",
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
