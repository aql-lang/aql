package engine

import "strings"

func registerLower(r *Registry) {
	lowerHandler := func(args []Value, _ map[string]Value, _ []Value, _ *Registry) ([]Value, error) {
		s := args[0].Data.(string)
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
