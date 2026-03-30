package engine

// registerSet registers the "set" word for storing values in the global store.
//
// Two signatures:
//
//	[TString, TAny]   – set "key" value
//	[TAtom/q, TAny]   – set key value  (word captured as atom via /q)
//
// Forward precedence handles all orderings without infix signatures.
func registerSet(r *Registry) {
	setHandler := func(args []Value) ([]Value, error) {
		key := storeKey(args[0])
		r.Store[key] = args[1]
		return nil, nil
	}

	r.Register("set",
		Signature{
			Args:    []Type{TString, TAny},
			Handler: setHandler,
		},
		Signature{
			Args:      []Type{TAtom, TAny},
			QuoteArgs: map[int]bool{0: true},
			Handler:   setHandler,
		},
	)
}
