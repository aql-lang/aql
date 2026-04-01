package engine

import "fmt"

// registerSet registers the "set" word for storing values in a Store or
// mutating fields on an Object instance.
//
// Store signatures (forward precedence):
//
//	[TString, TAny, TStore]   – set "key" value store
//	[TAtom/q, TAny, TStore]   – set key value store
//
// Object signatures (forward precedence):
//
//	[TString, TAny, TObject]  – set "field" value obj
//	[TAtom/q, TAny, TObject]  – set field value obj
//
// Store values are set directly (no prototype chain walk).
// Object fields are set on the instance's own Fields map (mutable).
// Nodes (Map, List) are immutable and cannot be used with set.
func registerSet(r *Registry) {
	storeHandler := func(args []Value, _ map[string]Value, _ []Value, _ *Registry) ([]Value, error) {
		store := args[2].AsStore()
		if store == nil {
			return nil, fmt.Errorf("set: expected a Store, got %s", args[2].VType.String())
		}
		key := storeKey(args[0])
		store.Set(key, args[1])
		return nil, nil
	}

	objectHandler := func(args []Value, _ map[string]Value, _ []Value, _ *Registry) ([]Value, error) {
		container := args[2]
		if container.Data == nil {
			return nil, fmt.Errorf("set: cannot set field on type literal")
		}
		key := storeKey(args[0])
		oi, ok := container.Data.(ObjectInstanceInfo)
		if !ok {
			return nil, fmt.Errorf("set: expected an Object instance, got %s", container.VType.String())
		}
		oi.Fields.Set(key, args[1])
		return nil, nil
	}

	r.Register("set",
		// Store
		Signature{
			Args:    []Type{TString, TAny, TStore},
			Handler: storeHandler,
		},
		Signature{
			Args:      []Type{TAtom, TAny, TStore},
			QuoteArgs: map[int]bool{0: true},
			Handler:   storeHandler,
		},
		// Object
		Signature{
			Args:    []Type{TString, TAny, TObject},
			Handler: objectHandler,
		},
		Signature{
			Args:      []Type{TAtom, TAny, TObject},
			QuoteArgs: map[int]bool{0: true},
			Handler:   objectHandler,
		},
	)
}
