package engine

import (
	"fmt"
)

// registerGet registers "get" for value access.
//
// get retrieves values from a Store, Node (Map/List), or Object.
//
// Signature: [Key, Container] where Key is String|Integer|Atom|Word/q
// and Container is Node|Object|Store|Array|None.
//
// The /q modifier on atom/word key positions allows registered word names
// to be used as keys without being executed first (fixes dot-notation
// shadowing: matrix.trace does map lookup, not trace execution).
//
// All argument orderings work via standard AQL arg matching:
//
//	get a {a:1}        → 1   (forward key, stack container)
//	{a:1} get a        → 1   (stack container, forward key)
//	a {a:1} get        → 1   (all stack)
//	{a:{b:1}} get a get b → 1 (chained: get cannot pass get, matches stack)
func registerGet(r *Registry) {
	// getKey extracts the key string from any key-typed value.
	getKey := func(v Value) string {
		if v.IsWord() {
			return v.AsWord().Name
		}
		if v.VType.Matches(TString) {
			return v.AsString()
		}
		if v.IsAtom() {
			return v.AsAtom()
		}
		return fmt.Sprintf("%v", v.Data)
	}

	nodeHandler := func(args []Value, _ map[string]Value, _ []Value, _ *Registry) ([]Value, error) {
		key := args[0]
		container := args[1]
		if container.Data == nil {
			return nil, fmt.Errorf("get: cannot access property on type literal")
		}
		// Integer key: list index access.
		if key.VType.Matches(TInteger) {
			idx := key.AsInteger()
			if list := container.AsList(); !list.IsNil() && container.VType.Matches(TList) {
				i := int(idx)
				if i < 0 || i >= list.Len() {
					return []Value{NewTypeLiteral(TNone)}, nil
				}
				return []Value{list.Get(i)}, nil
			}
			// Fall through to map lookup with stringified key.
		}
		// String/atom/word key: map property access.
		k := getKey(key)
		if m := container.AsMap(); m != nil {
			val, ok := m.Get(k)
			if !ok {
				return []Value{NewTypeLiteral(TNone)}, nil
			}
			return []Value{val}, nil
		}
		return []Value{NewTypeLiteral(TNone)}, nil
	}

	objectHandler := func(args []Value, _ map[string]Value, _ []Value, _ *Registry) ([]Value, error) {
		key := args[0]
		container := args[1]
		if container.Data == nil {
			return nil, fmt.Errorf("get: cannot access property on type literal")
		}
		k := getKey(key)
		if m, ok := container.Data.(*OrderedMap); ok {
			val, found := m.Get(k)
			if !found {
				return []Value{NewTypeLiteral(TNone)}, nil
			}
			return []Value{val}, nil
		}
		oi := container.AsObjectInstance()
		val, ok := oi.GetField(k)
		if !ok {
			return []Value{NewTypeLiteral(TNone)}, nil
		}
		return []Value{val}, nil
	}

	storeHandler := func(args []Value, _ map[string]Value, _ []Value, _ *Registry) ([]Value, error) {
		store := args[1].AsStore()
		if store == nil {
			return nil, fmt.Errorf("get: expected a Store, got %s", args[1].VType.String())
		}
		key := getKey(args[0])
		val, ok := store.Get(key)
		if !ok {
			return nil, fmt.Errorf("unknown key: %s", key)
		}
		return []Value{val}, nil
	}

	arrayHandler := func(args []Value, _ map[string]Value, _ []Value, _ *Registry) ([]Value, error) {
		arr := args[1].AsArray()
		if arr == nil {
			return nil, fmt.Errorf("get: expected an Array, got %s", args[1].VType.String())
		}
		val, ok := arr.Get(int(args[0].AsInteger()))
		if !ok {
			return []Value{NewTypeLiteral(TNone)}, nil
		}
		return []Value{val}, nil
	}

	noneHandler := func(args []Value, _ map[string]Value, _ []Value, _ *Registry) ([]Value, error) {
		return []Value{NewTypeLiteral(TNone)}, nil
	}

	sigs := []Signature{
		// [Key | Store] — key forward, container from stack
		{Args: []Type{TString, TStore}, BarrierPos: 1, Handler: storeHandler},
		{Args: []Type{TAtom, TStore}, QuoteArgs: map[int]bool{0: true}, BarrierPos: 1, Handler: storeHandler},
		// [Key | Node] — covers Map, List, Options
		{Args: []Type{TAtom, TNode}, QuoteArgs: map[int]bool{0: true}, BarrierPos: 1, Handler: nodeHandler},
		{Args: []Type{TString, TNode}, BarrierPos: 1, Handler: nodeHandler},
		{Args: []Type{TInteger, TNode}, BarrierPos: 1, Handler: nodeHandler},
		// [Key | Array]
		{Args: []Type{TInteger, TArray}, BarrierPos: 1, Handler: arrayHandler},
		// [Key | Object]
		{Args: []Type{TAtom, TObject}, QuoteArgs: map[int]bool{0: true}, BarrierPos: 1, Handler: objectHandler},
		{Args: []Type{TString, TObject}, BarrierPos: 1, Handler: objectHandler},
		{Args: []Type{TInteger, TObject}, BarrierPos: 1, Handler: objectHandler},
		// [Key | None]
		{Args: []Type{TAny, TNone}, BarrierPos: 1, Handler: noneHandler},
	}

	r.Register("get", sigs...)
}
