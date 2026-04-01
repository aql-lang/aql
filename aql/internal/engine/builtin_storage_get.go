package engine

import "fmt"

// registerGet registers the "get" word for retrieving values from a Store.
//
// Two signatures (forward precedence):
//
//	[TString, TStore]   – get "key" store
//	[TAtom/q, TStore]   – get key store  (word captured as atom via /q)
//
// Key resolution walks the Store's prototype chain.
func registerGet(r *Registry) {
	getHandler := func(args []Value, _ map[string]Value, _ []Value, _ *Registry) ([]Value, error) {
		store := args[1].AsStore()
		if store == nil {
			return nil, fmt.Errorf("get: expected a Store, got %s", args[1].VType.String())
		}
		key := storeKey(args[0])
		val, ok := store.Get(key)
		if !ok {
			return nil, fmt.Errorf("unknown key: %s", key)
		}
		return []Value{val}, nil
	}

	r.Register("get",
		Signature{
			Args:    []Type{TString, TStore},
			Handler: getHandler,
		},
		Signature{
			Args:      []Type{TAtom, TStore},
			QuoteArgs: map[int]bool{0: true},
			Handler:   getHandler,
		},
	)
}
