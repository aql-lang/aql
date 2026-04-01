package engine

import (
	"fmt"
	"strconv"
)

// registerGet registers "get" and its "." alias for value access.
//
// get retrieves values from a Store, Node (Map/List), or Object.
// Forward precedence: key collected forward, container from stack.
//
// Signatures:
//
//	[TAtom/q, TStore]   – get key store (Store lookup, prototype chain)
//	[TString, TStore]   – get "key" store
//	[TAtom, TNode]      – get key {a:1}  (Map property access)
//	[TString, TNode]    – get "key" {a:1}
//	[TInteger, TNode]   – get 0 [10 20]  (List index access)
//	[TAtom, TObject]    – get key obj    (Object field access)
//	[TString, TObject]  – get "key" obj
//	[TInteger, TObject] – get 0 obj
//	[TAny, TNone]       – get key none   (None propagation)
//
// Usage:
//
//	context get foo              – Store lookup
//	{a:1} get a                  – Map property
//	{a:1} . a                    – same, dot alias
//	{a:{b:1}} . a . b            – chained
//	[10 20 30] get 1             – List index
//	def m {x:42} m.x             – dot notation (parser expands to . x)
func registerGet(r *Registry) {

	// --- Store handlers ---

	storeHandler := func(args []Value, _ map[string]Value, _ []Value, _ *Registry) ([]Value, error) {
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

	// --- Node handlers (Map, List, Options) ---

	nodeAtomHandler := func(args []Value, _ map[string]Value, _ []Value, _ *Registry) ([]Value, error) {
		container := args[1]
		if container.Data == nil {
			return nil, fmt.Errorf("get: cannot access property on type literal")
		}
		key := args[0].AsAtom()
		if m := container.AsMap(); m != nil {
			val, ok := m.Get(key)
			if !ok {
				return []Value{NewTypeLiteral(TNone)}, nil
			}
			return []Value{val}, nil
		}
		return []Value{NewTypeLiteral(TNone)}, nil
	}

	nodeStringHandler := func(args []Value, _ map[string]Value, _ []Value, _ *Registry) ([]Value, error) {
		container := args[1]
		if container.Data == nil {
			return nil, fmt.Errorf("get: cannot access property on type literal")
		}
		key := args[0].AsString()
		if m := container.AsMap(); m != nil {
			val, ok := m.Get(key)
			if !ok {
				return []Value{NewTypeLiteral(TNone)}, nil
			}
			return []Value{val}, nil
		}
		return []Value{NewTypeLiteral(TNone)}, nil
	}

	nodeIntegerHandler := func(args []Value, _ map[string]Value, _ []Value, _ *Registry) ([]Value, error) {
		container := args[1]
		if container.Data == nil {
			return nil, fmt.Errorf("get: cannot access property on type literal")
		}
		idx := args[0].AsInteger()
		if list := container.AsList(); !list.IsNil() && container.VType.Matches(TList) {
			i := int(idx)
			if i < 0 || i >= list.Len() {
				return []Value{NewTypeLiteral(TNone)}, nil
			}
			return []Value{list.Get(i)}, nil
		}
		if m := container.AsMap(); m != nil {
			key := strconv.FormatInt(idx, 10)
			val, ok := m.Get(key)
			if !ok {
				return []Value{NewTypeLiteral(TNone)}, nil
			}
			return []Value{val}, nil
		}
		return []Value{NewTypeLiteral(TNone)}, nil
	}

	// --- Object handlers (Record, Entity, etc.) ---

	objectAtomHandler := func(args []Value, _ map[string]Value, _ []Value, _ *Registry) ([]Value, error) {
		container := args[1]
		if container.Data == nil {
			return nil, fmt.Errorf("get: cannot access property on type literal")
		}
		key := args[0].AsAtom()
		if m, ok := container.Data.(*OrderedMap); ok {
			val, found := m.Get(key)
			if !found {
				return []Value{NewTypeLiteral(TNone)}, nil
			}
			return []Value{val}, nil
		}
		oi := container.AsObjectInstance()
		val, ok := oi.GetField(key)
		if !ok {
			return []Value{NewTypeLiteral(TNone)}, nil
		}
		return []Value{val}, nil
	}

	objectStringHandler := func(args []Value, _ map[string]Value, _ []Value, _ *Registry) ([]Value, error) {
		container := args[1]
		if container.Data == nil {
			return nil, fmt.Errorf("get: cannot access property on type literal")
		}
		key := args[0].AsString()
		if m, ok := container.Data.(*OrderedMap); ok {
			val, found := m.Get(key)
			if !found {
				return []Value{NewTypeLiteral(TNone)}, nil
			}
			return []Value{val}, nil
		}
		oi := container.AsObjectInstance()
		val, ok := oi.GetField(key)
		if !ok {
			return []Value{NewTypeLiteral(TNone)}, nil
		}
		return []Value{val}, nil
	}

	objectIntegerHandler := func(args []Value, _ map[string]Value, _ []Value, _ *Registry) ([]Value, error) {
		container := args[1]
		if container.Data == nil {
			return nil, fmt.Errorf("get: cannot access property on type literal")
		}
		key := strconv.FormatInt(args[0].AsInteger(), 10)
		if m, ok := container.Data.(*OrderedMap); ok {
			val, found := m.Get(key)
			if !found {
				return []Value{NewTypeLiteral(TNone)}, nil
			}
			return []Value{val}, nil
		}
		oi := container.AsObjectInstance()
		val, ok := oi.GetField(key)
		if !ok {
			return []Value{NewTypeLiteral(TNone)}, nil
		}
		return []Value{val}, nil
	}

	// --- None handler ---

	noneHandler := func(args []Value, _ map[string]Value, _ []Value, _ *Registry) ([]Value, error) {
		return []Value{NewTypeLiteral(TNone)}, nil
	}

	sigs := []Signature{
		// Store containers
		{Args: []Type{TString, TStore}, Handler: storeHandler},
		{Args: []Type{TAtom, TStore}, QuoteArgs: map[int]bool{0: true}, Handler: storeHandler},
		// Node containers (Map, List, Options)
		{Args: []Type{TAtom, TNode}, Handler: nodeAtomHandler},
		{Args: []Type{TString, TNode}, Handler: nodeStringHandler},
		{Args: []Type{TInteger, TNode}, Handler: nodeIntegerHandler},
		// Object containers (Record, Entity, etc.)
		{Args: []Type{TAtom, TObject}, Handler: objectAtomHandler},
		{Args: []Type{TString, TObject}, Handler: objectStringHandler},
		{Args: []Type{TInteger, TObject}, Handler: objectIntegerHandler},
		// None propagation
		{Args: []Type{TAny, TNone}, Handler: noneHandler},
	}

	r.Register("get", sigs...)
	r.Register(".", sigs...)
}
