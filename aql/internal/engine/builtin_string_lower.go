package engine

import "strings"

func registerLower(r *Registry) {
	lowerHandler := func(args []Value) ([]Value, error) {
		s := args[0].Data.(string)
		return []Value{NewString(strings.ToLower(s))}, nil
	}
	r.Register("lower",
		Signature{Args: []Type{TString}, Handler: lowerHandler},
		Signature{Args: []Type{TAtom}, Handler: lowerHandler},
	)
}
