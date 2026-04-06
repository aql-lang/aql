package engine

import (
	"fmt"
	"strings"
)

func registerUpper(r *Registry) {
	upperHandler := func(args []Value, _ map[string]Value, _ []Value, _ *Registry) ([]Value, error) {
		s, ok := args[0].Data.(string)
		if !ok {
			return nil, fmt.Errorf("upper: expected string, got %s", args[0].String())
		}
		return []Value{NewString(strings.ToUpper(s))}, nil
	}
	r.RegisterNativeFunc(NativeFunc{
		Name:              "upper",
		ForwardPrecedence: true,
		Signatures: []NativeSig{
			{Args: []Type{TString}, Handler: upperHandler},
			{Args: []Type{TAtom}, Handler: upperHandler},
		},
	})
}
