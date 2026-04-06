package engine

import (
	"fmt"
	"strings"
)

func registerLower(r *Registry) {
	lowerHandler := func(args []Value, _ map[string]Value, _ []Value, _ *Registry) ([]Value, error) {
		s, ok := args[0].Data.(string)
		if !ok {
			return nil, fmt.Errorf("lower: expected string, got %s", args[0].String())
		}
		return []Value{NewString(strings.ToLower(s))}, nil
	}
	r.RegisterNativeFunc(NativeFunc{
		Name:              "lower",
		ForwardPrecedence: true,
		Signatures: []NativeSig{
			{Args: []Type{TString}, Handler: lowerHandler},
			{Args: []Type{TAtom}, Handler: lowerHandler},
		},
	})
}
