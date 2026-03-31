package engine

import (
	"fmt"
	"strconv"
)

// registerDotr registers "dotr" and its "!." alias — a strict variant of
// dot that returns an error when the parent is none or the key/index is
// missing, instead of silently returning none.
//
// Usage:
//
//	{a:1} a dotr      => 1
//	{a:1} b dotr      => ERROR (key not found)
//	none a dotr       => ERROR (parent is none)
//	[10,20] 5 dotr    => ERROR (index out of bounds)
func registerDotr(r *Registry) {
	dotrMapAtomHandler := func(args []Value, _ map[string]Value, _ []Value, _ *Registry) ([]Value, error) {
		if args[0].Data == nil {
			return nil, fmt.Errorf("dotr: cannot access property on type literal")
		}
		m := args[0].AsMap()
		key := args[1].AsAtom()
		val, ok := m.Get(key)
		if !ok {
			return nil, fmt.Errorf("dotr: key %q not found in map", key)
		}
		return []Value{val}, nil
	}

	dotrMapStringHandler := func(args []Value, _ map[string]Value, _ []Value, _ *Registry) ([]Value, error) {
		if args[0].Data == nil {
			return nil, fmt.Errorf("dotr: cannot access property on type literal")
		}
		m := args[0].AsMap()
		key := args[1].AsString()
		val, ok := m.Get(key)
		if !ok {
			return nil, fmt.Errorf("dotr: key %q not found in map", key)
		}
		return []Value{val}, nil
	}

	dotrListHandler := func(args []Value, _ map[string]Value, _ []Value, _ *Registry) ([]Value, error) {
		if args[0].Data == nil {
			return nil, fmt.Errorf("dotr: cannot index type literal")
		}
		list := args[0].AsList()
		idx := int(args[1].AsInteger())
		if idx < 0 || idx >= len(list) {
			return nil, fmt.Errorf("dotr: index %d out of bounds (length %d)", idx, len(list))
		}
		return []Value{list[idx]}, nil
	}

	dotrMapIntegerHandler := func(args []Value, _ map[string]Value, _ []Value, _ *Registry) ([]Value, error) {
		if args[0].Data == nil {
			return nil, fmt.Errorf("dotr: cannot access property on type literal")
		}
		m := args[0].AsMap()
		key := strconv.FormatInt(args[1].AsInteger(), 10)
		val, ok := m.Get(key)
		if !ok {
			return nil, fmt.Errorf("dotr: key %q not found in map", key)
		}
		return []Value{val}, nil
	}

	dotrNoneHandler := func(args []Value, _ map[string]Value, _ []Value, _ *Registry) ([]Value, error) {
		return nil, fmt.Errorf("dotr: parent is None")
	}

	sigs := []Signature{
		{Args: []Type{TMap, TAtom}, Handler: dotrMapAtomHandler},
		{Args: []Type{TMap, TString}, Handler: dotrMapStringHandler},
		{Args: []Type{TList, TInteger}, Handler: dotrListHandler},
		{Args: []Type{TMap, TInteger}, Handler: dotrMapIntegerHandler},
		{Args: []Type{TNone, TAny}, Handler: dotrNoneHandler},
	}

	r.Register("dotr", sigs...)
	r.Register("!.", sigs...)
}
