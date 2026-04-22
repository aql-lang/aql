package engine

import (
	"fmt"
)

// RegisterGet registers "get" for value access.
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
func RegisterGet(r *Registry) {
	// getKey extracts the key string from any key-typed value.
	getKey := func(v Value) string {
		if v.IsWord() {
			_as0, _ := v.AsWord()
			return _as0.Name
		}
		if v.VType.Matches(TString) {
			_as1, _ := v.AsString()
			return _as1
		}
		if v.IsAtom() {
			_as2, _ := v.AsAtom()
			return _as2
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
			idx, _ := key.AsInteger()
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
		oi, _ := container.AsObjectInstance()
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
		_as3, _ := args[0].AsInteger()
		val, ok := arr.Get(int(_as3))
		if !ok {
			return []Value{NewTypeLiteral(TNone)}, nil
		}
		return []Value{val}, nil
	}

	noneHandler := func(args []Value, _ map[string]Value, _ []Value, _ *Registry) ([]Value, error) {
		return []Value{NewTypeLiteral(TNone)}, nil
	}

	r.RegisterNativeFunc(NativeFunc{
		Name:              "get",
		ForwardPrecedence: true,
		Signatures: []NativeSig{
			// [Key | Store] — key forward, container from stack.
			// In check mode, consult CheckContextTypes to produce a
			// typed carrier for previously-set keys.
			{
				Args: []Type{TString, TStore}, BarrierPos: 1, Handler: storeHandler,
				ReturnsFn: func(args []Value) []Value {
					v, _ := r.LookupContextType(storeKey(args[0]))
					return []Value{v}
				},
			},
			{
				Args: []Type{TAtom, TStore}, QuoteArgs: map[int]bool{0: true}, BarrierPos: 1, Handler: storeHandler,
				ReturnsFn: func(args []Value) []Value {
					v, _ := r.LookupContextType(storeKey(args[0]))
					return []Value{v}
				},
			},
			// [Key | Node] — covers Map, List, Options
			{Args: []Type{TAtom, TNode}, QuoteArgs: map[int]bool{0: true}, BarrierPos: 1, Handler: nodeHandler, Returns: []Type{TAny}},
			{Args: []Type{TString, TNode}, BarrierPos: 1, Handler: nodeHandler, Returns: []Type{TAny}},
			{Args: []Type{TInteger, TNode}, BarrierPos: 1, Handler: nodeHandler, Returns: []Type{TAny}},
			// [Key | Array]
			{Args: []Type{TInteger, TArray}, BarrierPos: 1, Handler: arrayHandler, Returns: []Type{TAny}},
			// [Key | Object]
			{Args: []Type{TAtom, TObject}, QuoteArgs: map[int]bool{0: true}, BarrierPos: 1, Handler: objectHandler, Returns: []Type{TAny}},
			{Args: []Type{TString, TObject}, BarrierPos: 1, Handler: objectHandler, Returns: []Type{TAny}},
			{Args: []Type{TInteger, TObject}, BarrierPos: 1, Handler: objectHandler, Returns: []Type{TAny}},
			// [Key | None]
			{Args: []Type{TAny, TNone}, BarrierPos: 1, Handler: noneHandler, Returns: []Type{TNone}},
		},
	})
}
