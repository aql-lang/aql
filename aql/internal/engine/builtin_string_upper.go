package engine

import "strings"

func registerUpper(r *Registry) {
	upperHandler := func(args []Value, _ map[string]Value, _ []Value, _ *Registry) ([]Value, error) {
		s := args[0].Data.(string)
		return []Value{NewString(strings.ToUpper(s))}, nil
	}
	r.Register("upper",
		Signature{Args: []Type{TString}, Handler: upperHandler},
		Signature{Args: []Type{TAtom}, Handler: upperHandler},
	)
}
