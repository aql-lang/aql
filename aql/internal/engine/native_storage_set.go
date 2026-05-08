package engine

import "fmt"

// RegisterSet registers the "set" word for storing values in a Store,
// mutating fields on an Object instance, or setting Array elements.
//
// Store signatures (copy-on-write, forward precedence):
//
//	[TString, TAny, TStore]   – set "key" value store
//	[TAtom/q, TAny, TStore]   – set key value store
//
// Object signatures (in-place mutation, forward precedence):
//
//	[TString, TAny, TObject]  – set "field" value obj
//	[TAtom/q, TAny, TObject]  – set field value obj
//
// Array signatures (in-place mutation, forward precedence):
//
//	[TInteger, TAny, TArray]  – set index value arr
//
// Store set is copy-on-write: a new Store layer is created (prototype =
// old Store) and propagated up through parent Stores to the ctxStack.
// Nodes (Map, List) are immutable and cannot be used with set.
func RegisterSet(r *Registry) {
	storeHandler := func(args []Value, _ map[string]Value, _ []Value, reg *Registry) ([]Value, error) {
		store := args[2].AsStore()
		if store == nil {
			return nil, fmt.Errorf("set: expected a Store, got %s", args[2].VType.String())
		}
		key := StoreKey(args[0])
		CowSet(store, key, args[1], reg)
		return nil, nil
	}

	objectHandler := func(args []Value, _ map[string]Value, _ []Value, _ *Registry) ([]Value, error) {
		container := args[2]
		if container.Data == nil {
			return nil, fmt.Errorf("set: cannot set field on type literal")
		}
		key := StoreKey(args[0])
		oi, ok := container.Data.(ObjectInstanceInfo)
		if !ok {
			return nil, fmt.Errorf("set: expected an Object instance, got %s", container.VType.String())
		}
		oi.Fields.Set(key, args[1])
		return nil, nil
	}

	arrayHandler := func(args []Value, _ map[string]Value, _ []Value, _ *Registry) ([]Value, error) {
		arr := args[2].AsArray()
		if arr == nil {
			return nil, fmt.Errorf("set: expected an Array, got %s", args[2].VType.String())
		}
		_as0, _ := args[0].AsConcreteInteger()
		idx := int(_as0)
		if !arr.Set(idx, args[1]) {
			return nil, fmt.Errorf("set: index %d out of bounds (length %d)", idx, arr.Len())
		}
		return nil, nil
	}

	r.RegisterNativeFunc(NativeFunc{
		Name:              "set",
		ForwardPrecedence: true,
		Signatures: []NativeSig{
			// Store (copy-on-write)
			{
				Args:    []Type{TString, TAny, TStore},
				Handler: storeHandler,
				Returns: []Type{},
				// Record key → carrier for the check-mode context
				// tracker so `get key store` can later produce a
				// typed carrier instead of Any.
				ReturnsFn: func(args []Value) []Value {
					r.RecordContextSet(StoreKey(args[0]), args[1])
					return nil
				},
			},
			{
				Args:      []Type{TAtom, TAny, TStore},
				QuoteArgs: map[int]bool{0: true},
				Handler:   storeHandler,
				Returns:   []Type{},
				ReturnsFn: func(args []Value) []Value {
					r.RecordContextSet(StoreKey(args[0]), args[1])
					return nil
				},
			},
			// Array (indexed by integer)
			{
				Args:    []Type{TInteger, TAny, TArray},
				Handler: arrayHandler,
				Returns: []Type{},
			},
			// Object
			{
				Args:    []Type{TString, TAny, TObject},
				Handler: objectHandler,
				Returns: []Type{},
			},
			{
				Args:      []Type{TAtom, TAny, TObject},
				QuoteArgs: map[int]bool{0: true},
				Handler:   objectHandler,
				Returns:   []Type{},
			},
		},
	})
}

// CowSet: re-exported from aqleng via aliases.go
