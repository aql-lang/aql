package engine

import "strings"

func registerLower(r *Registry) {
	lowerHandler := func(args []Value, _ map[string]Value, _ []Value, _ *Registry) ([]Value, error) {
		s := args[0].Data.(string)
		return []Value{NewString(strings.ToLower(s))}, nil
	}
	r.Register("lower",
		Signature{Args: []Type{TString}, Handler: lowerHandler},
		Signature{Args: []Type{TAtom}, Handler: lowerHandler},
	)
}
