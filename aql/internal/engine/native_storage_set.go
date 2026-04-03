package engine

import "fmt"

// registerSet registers the "set" word for storing values in a Store,
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
func registerSet(r *Registry) {
	storeHandler := func(args []Value, _ map[string]Value, _ []Value, reg *Registry) ([]Value, error) {
		store := args[2].AsStore()
		if store == nil {
			return nil, fmt.Errorf("set: expected a Store, got %s", args[2].VType.String())
		}
		key := storeKey(args[0])
		cowSet(store, key, args[1], reg)
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

	arrayHandler := func(args []Value, _ map[string]Value, _ []Value, _ *Registry) ([]Value, error) {
		arr := args[2].AsArray()
		if arr == nil {
			return nil, fmt.Errorf("set: expected an Array, got %s", args[2].VType.String())
		}
		_as0, _ := args[0].AsInteger()
		idx := int(_as0)
		if !arr.Set(idx, args[1]) {
			return nil, fmt.Errorf("set: index %d out of bounds (length %d)", idx, arr.Len())
		}
		return nil, nil
	}

	r.Register("set",
		// Store (copy-on-write)
		Signature{
			Args:    []Type{TString, TAny, TStore},
			Handler: storeHandler,
		},
		Signature{
			Args:      []Type{TAtom, TAny, TStore},
			QuoteArgs: map[int]bool{0: true},
			Handler:   storeHandler,
		},
		// Array (indexed by integer)
		Signature{
			Args:    []Type{TInteger, TAny, TArray},
			Handler: arrayHandler,
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

// cowSet performs a copy-on-write set on a Store. It creates a new Store
// layer whose prototype is the old Store, sets the key in the new layer,
// and propagates the update up through parent Stores to the ctxStack.
func cowSet(store *StoreInstanceInfo, key string, val Value, r *Registry) {
	// Create new COW layer: only the changed key, prototype = old store.
	newStore := &StoreInstanceInfo{
		TypeName:  store.TypeName,
		Data:      map[string]Value{key: val},
		Prototype: store,
		Parent:    store.Parent,
		ParentKey: store.ParentKey,
	}

	// Track parent for nested Store values.
	if childStore, ok := val.Data.(*StoreInstanceInfo); ok {
		childStore.Parent = newStore
		childStore.ParentKey = key
	}

	// Propagate up the parent chain: each parent Store gets a new COW
	// layer with the updated child reference.
	current := newStore
	parent := store.Parent
	parentKey := store.ParentKey

	for parent != nil {
		newParent := &StoreInstanceInfo{
			TypeName:  parent.TypeName,
			Data:      map[string]Value{parentKey: NewStoreValue(current)},
			Prototype: parent,
			Parent:    parent.Parent,
			ParentKey: parent.ParentKey,
		}
		current.Parent = newParent
		current.ParentKey = parentKey

		current = newParent
		parentKey = parent.ParentKey
		parent = parent.Parent
	}

	// current is the topmost COW'd Store. Update the ctxStack entry that
	// references the original store (either directly or via prototype chain).
	// The topmost COW'd store's prototype is the original root store.
	// Walk each ctxStack entry's prototype chain to see if it passes
	// through the original root, and if so, create a new ctxStack entry
	// that uses the COW'd store.
	origRoot := current.Prototype
	if origRoot == nil {
		origRoot = store
	}
	r.UpdateCtxStoreChain(origRoot, current)
}
