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
	dotrMapAtomHandler := func(args []Value) ([]Value, error) {
		key := args[0].AsAtom()
		m := args[1].AsMap()
		val, ok := m.Get(key)
		if !ok {
			return nil, fmt.Errorf("dotr: key %q not found in map", key)
		}
		return []Value{val}, nil
	}

	dotrMapStringHandler := func(args []Value) ([]Value, error) {
		key := args[0].AsString()
		m := args[1].AsMap()
		val, ok := m.Get(key)
		if !ok {
			return nil, fmt.Errorf("dotr: key %q not found in map", key)
		}
		return []Value{val}, nil
	}

	dotrListHandler := func(args []Value) ([]Value, error) {
		idx := int(args[0].AsInteger())
		list := args[1].AsList()
		if idx < 0 || idx >= len(list) {
			return nil, fmt.Errorf("dotr: index %d out of bounds (length %d)", idx, len(list))
		}
		return []Value{list[idx]}, nil
	}

	dotrMapIntegerHandler := func(args []Value) ([]Value, error) {
		key := strconv.FormatInt(args[0].AsInteger(), 10)
		m := args[1].AsMap()
		val, ok := m.Get(key)
		if !ok {
			return nil, fmt.Errorf("dotr: key %q not found in map", key)
		}
		return []Value{val}, nil
	}

	dotrNoneHandler := func(args []Value) ([]Value, error) {
		return nil, fmt.Errorf("dotr: parent is None")
	}

	sigs := []Signature{
		{Args: []Type{TAtom, TMap}, Handler: dotrMapAtomHandler},
		{Args: []Type{TString, TMap}, Handler: dotrMapStringHandler},
		{Args: []Type{TInteger, TList}, Handler: dotrListHandler},
		{Args: []Type{TInteger, TMap}, Handler: dotrMapIntegerHandler},
		{Args: []Type{TAny, TNone}, Handler: dotrNoneHandler},
	}

	r.Register("dotr", sigs...)
	r.Register("!.", sigs...)
}
