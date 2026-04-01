package engine

import (
	"fmt"
	"strconv"
)

// registerGetr registers "getr" and its "!." alias — a strict variant of
// get that returns an error when the parent is none or the key/index is
// missing, instead of silently returning none.
//
// Works on Maps, Lists, and Object instances.
//
// Usage:
//
//	{a:1} a getr      => 1
//	{a:1} b getr      => ERROR (key not found)
//	none a getr       => ERROR (parent is none)
//	[10,20] 5 getr    => ERROR (index out of bounds)
func registerGetr(r *Registry) {
	getrMapAtomHandler := func(args []Value, _ map[string]Value, _ []Value, _ *Registry) ([]Value, error) {
		if args[0].Data == nil {
			return nil, fmt.Errorf("getr: cannot access property on type literal")
		}
		m := args[0].AsMap()
		key := args[1].AsAtom()
		val, ok := m.Get(key)
		if !ok {
			return nil, fmt.Errorf("getr: key %q not found in map", key)
		}
		return []Value{val}, nil
	}

	getrMapStringHandler := func(args []Value, _ map[string]Value, _ []Value, _ *Registry) ([]Value, error) {
		if args[0].Data == nil {
			return nil, fmt.Errorf("getr: cannot access property on type literal")
		}
		m := args[0].AsMap()
		key := args[1].AsString()
		val, ok := m.Get(key)
		if !ok {
			return nil, fmt.Errorf("getr: key %q not found in map", key)
		}
		return []Value{val}, nil
	}

	getrListHandler := func(args []Value, _ map[string]Value, _ []Value, _ *Registry) ([]Value, error) {
		if args[0].Data == nil {
			return nil, fmt.Errorf("getr: cannot index type literal")
		}
		list := args[0].AsList()
		idx := int(args[1].AsInteger())
		if idx < 0 || idx >= list.Len() {
			return nil, fmt.Errorf("getr: index %d out of bounds (length %d)", idx, list.Len())
		}
		return []Value{list.Get(idx)}, nil
	}

	getrMapIntegerHandler := func(args []Value, _ map[string]Value, _ []Value, _ *Registry) ([]Value, error) {
		if args[0].Data == nil {
			return nil, fmt.Errorf("getr: cannot access property on type literal")
		}
		m := args[0].AsMap()
		key := strconv.FormatInt(args[1].AsInteger(), 10)
		val, ok := m.Get(key)
		if !ok {
			return nil, fmt.Errorf("getr: key %q not found in map", key)
		}
		return []Value{val}, nil
	}

	getrObjectAtomHandler := func(args []Value, _ map[string]Value, _ []Value, _ *Registry) ([]Value, error) {
		if args[0].Data == nil {
			return nil, fmt.Errorf("getr: cannot access property on type literal")
		}
		key := args[1].AsAtom()
		if m, ok := args[0].Data.(*OrderedMap); ok {
			val, found := m.Get(key)
			if !found {
				return nil, fmt.Errorf("getr: key %q not found in object", key)
			}
			return []Value{val}, nil
		}
		oi := args[0].AsObjectInstance()
		val, ok := oi.GetField(key)
		if !ok {
			return nil, fmt.Errorf("getr: field %q not found in object", key)
		}
		return []Value{val}, nil
	}

	getrObjectStringHandler := func(args []Value, _ map[string]Value, _ []Value, _ *Registry) ([]Value, error) {
		if args[0].Data == nil {
			return nil, fmt.Errorf("getr: cannot access property on type literal")
		}
		key := args[1].AsString()
		if m, ok := args[0].Data.(*OrderedMap); ok {
			val, found := m.Get(key)
			if !found {
				return nil, fmt.Errorf("getr: key %q not found in object", key)
			}
			return []Value{val}, nil
		}
		oi := args[0].AsObjectInstance()
		val, ok := oi.GetField(key)
		if !ok {
			return nil, fmt.Errorf("getr: field %q not found in object", key)
		}
		return []Value{val}, nil
	}

	getrObjectIntegerHandler := func(args []Value, _ map[string]Value, _ []Value, _ *Registry) ([]Value, error) {
		if args[0].Data == nil {
			return nil, fmt.Errorf("getr: cannot access property on type literal")
		}
		key := strconv.FormatInt(args[1].AsInteger(), 10)
		if m, ok := args[0].Data.(*OrderedMap); ok {
			val, found := m.Get(key)
			if !found {
				return nil, fmt.Errorf("getr: key %q not found in object", key)
			}
			return []Value{val}, nil
		}
		oi := args[0].AsObjectInstance()
		val, ok := oi.GetField(key)
		if !ok {
			return nil, fmt.Errorf("getr: field %q not found in object", key)
		}
		return []Value{val}, nil
	}

	getrNoneHandler := func(args []Value, _ map[string]Value, _ []Value, _ *Registry) ([]Value, error) {
		return nil, fmt.Errorf("getr: parent is None")
	}

	sigs := []Signature{
		// Map
		{Args: []Type{TMap, TAtom}, Handler: getrMapAtomHandler},
		{Args: []Type{TMap, TString}, Handler: getrMapStringHandler},
		{Args: []Type{TMap, TInteger}, Handler: getrMapIntegerHandler},
		// List
		{Args: []Type{TList, TInteger}, Handler: getrListHandler},
		// Object
		{Args: []Type{TObject, TAtom}, Handler: getrObjectAtomHandler},
		{Args: []Type{TObject, TString}, Handler: getrObjectStringHandler},
		{Args: []Type{TObject, TInteger}, Handler: getrObjectIntegerHandler},
		// None
		{Args: []Type{TNone, TAny}, Handler: getrNoneHandler},
	}

	r.Register("getr", sigs...)
	r.Register("!.", sigs...)
}
