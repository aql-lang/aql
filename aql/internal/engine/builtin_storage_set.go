package engine

import "fmt"

// registerSet registers the "set" word for storing values in a Store.
//
// Two signatures (forward precedence):
//
//	[TString, TAny, TStore]   – set "key" value store
//	[TAtom/q, TAny, TStore]   – set key value store  (word captured as atom via /q)
//
// Values are set directly on the Store (no prototype chain walk).
func registerSet(r *Registry) {
	setHandler := func(args []Value, _ map[string]Value, _ []Value, _ *Registry) ([]Value, error) {
		store := args[2].AsStore()
		if store == nil {
			return nil, fmt.Errorf("set: expected a Store, got %s", args[2].VType.String())
		}
		key := storeKey(args[0])
		store.Set(key, args[1])
		return nil, nil
	}

	r.Register("set",
		Signature{
			Args:    []Type{TString, TAny, TStore},
			Handler: setHandler,
		},
		Signature{
			Args:      []Type{TAtom, TAny, TStore},
			QuoteArgs: map[int]bool{0: true},
			Handler:   setHandler,
		},
	)
}
