package engine

import "strings"

func registerUpper(r *Registry) {
	upperHandler := func(args []Value, _ map[string]Value, _ []Value, _ *Registry) ([]Value, error) {
		s := args[0].Data.(string)
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
