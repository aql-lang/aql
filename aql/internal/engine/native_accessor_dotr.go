package engine

import (
	"fmt"
)

// registerGetr registers "getr" — a strict variant of get that returns an
// error when the parent is none or the key/index is missing, instead of
// silently returning none.
//
// Signature: [Key, Container] — same arg order as get.
//
// Usage:
//
//	{a:1} getr a       → 1
//	getr a {a:1}       → 1
//	{a:1} b getr       → ERROR (key not found)
//	none a getr        → ERROR (parent is none)
//	[10,20] 5 getr     → ERROR (index out of bounds)
func registerGetr(r *Registry) {
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

	mapHandler := func(args []Value, _ map[string]Value, _ []Value, _ *Registry) ([]Value, error) {
		key := args[0]
		container := args[1]
		if container.Data == nil {
			return nil, fmt.Errorf("getr: cannot access property on type literal")
		}
		// Integer key on list.
		if key.VType.Matches(TInteger) {
			if list := container.AsList(); !list.IsNil() && container.VType.Matches(TList) {
				idx := int(key.AsInteger())
				if idx < 0 || idx >= list.Len() {
					return nil, fmt.Errorf("getr: index %d out of bounds (length %d)", idx, list.Len())
				}
				return []Value{list.Get(idx)}, nil
			}
		}
		k := getKey(key)
		m := container.AsMap()
		if m == nil {
			return nil, fmt.Errorf("getr: expected a map, got %s", container.VType.String())
		}
		val, ok := m.Get(k)
		if !ok {
			return nil, fmt.Errorf("getr: key %q not found in map", k)
		}
		return []Value{val}, nil
	}

	objectHandler := func(args []Value, _ map[string]Value, _ []Value, _ *Registry) ([]Value, error) {
		key := args[0]
		container := args[1]
		if container.Data == nil {
			return nil, fmt.Errorf("getr: cannot access property on type literal")
		}
		k := getKey(key)
		if m, ok := container.Data.(*OrderedMap); ok {
			val, found := m.Get(k)
			if !found {
				return nil, fmt.Errorf("getr: key %q not found in object", k)
			}
			return []Value{val}, nil
		}
		oi := container.AsObjectInstance()
		val, ok := oi.GetField(k)
		if !ok {
			return nil, fmt.Errorf("getr: field %q not found in object", k)
		}
		return []Value{val}, nil
	}

	noneHandler := func(args []Value, _ map[string]Value, _ []Value, _ *Registry) ([]Value, error) {
		return nil, fmt.Errorf("getr: parent is None")
	}

	sigs := []Signature{
		// [Key | Node] — key forward, container from stack
		{Args: []Type{TAtom, TNode}, QuoteArgs: map[int]bool{0: true}, BarrierPos: 1, Handler: mapHandler},
		{Args: []Type{TString, TNode}, BarrierPos: 1, Handler: mapHandler},
		{Args: []Type{TInteger, TNode}, BarrierPos: 1, Handler: mapHandler},
		// [Key | Object]
		{Args: []Type{TAtom, TObject}, QuoteArgs: map[int]bool{0: true}, BarrierPos: 1, Handler: objectHandler},
		{Args: []Type{TString, TObject}, BarrierPos: 1, Handler: objectHandler},
		{Args: []Type{TInteger, TObject}, BarrierPos: 1, Handler: objectHandler},
		// [Key | None]
		{Args: []Type{TNone, TAny}, BarrierPos: 1, Handler: noneHandler},
	}

	r.Register("getr", sigs...)
}
