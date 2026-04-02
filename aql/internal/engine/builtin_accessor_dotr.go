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
	// atomKey extracts the key string from a value that is either an Atom
	// or a Word (when captured via /q modifier).
	atomKey := func(v Value) string {
		if v.IsWord() {
			return v.AsWord().Name
		}
		return v.AsAtom()
	}

	getrMapAtomHandler := func(args []Value, _ map[string]Value, _ []Value, _ *Registry) ([]Value, error) {
		if args[0].Data == nil {
			return nil, fmt.Errorf("getr: cannot access property on type literal")
		}
		m := args[0].AsMap()
		key := atomKey(args[1])
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
		key := atomKey(args[1])
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

	// swap2 creates a handler wrapper that swaps the two arguments before
	// calling the original handler. Used for forward-compatible signatures
	// where the arg order is reversed from the stack-only signatures.
	swap2 := func(h Handler) Handler {
		return func(args []Value, named map[string]Value, extra []Value, reg *Registry) ([]Value, error) {
			return h([]Value{args[1], args[0]}, named, extra, reg)
		}
	}

	sigs := []Signature{
		// Map — stack-only: {map} key getr
		{Args: []Type{TMap, TAtom}, QuoteArgs: map[int]bool{1: true}, Handler: getrMapAtomHandler},
		{Args: []Type{TMap, TString}, Handler: getrMapStringHandler},
		{Args: []Type{TMap, TInteger}, Handler: getrMapIntegerHandler},
		// Map — forward: map getr key (dot notation: p!.x)
		{Args: []Type{TAtom, TMap}, QuoteArgs: map[int]bool{0: true}, Handler: swap2(getrMapAtomHandler)},
		{Args: []Type{TString, TMap}, Handler: swap2(getrMapStringHandler)},
		{Args: []Type{TInteger, TMap}, Handler: swap2(getrMapIntegerHandler)},
		// List — stack-only: [list] idx getr
		{Args: []Type{TList, TInteger}, Handler: getrListHandler},
		// List — forward: [list] getr idx
		{Args: []Type{TInteger, TList}, Handler: swap2(getrListHandler)},
		// Object — stack-only
		{Args: []Type{TObject, TAtom}, QuoteArgs: map[int]bool{1: true}, Handler: getrObjectAtomHandler},
		{Args: []Type{TObject, TString}, Handler: getrObjectStringHandler},
		{Args: []Type{TObject, TInteger}, Handler: getrObjectIntegerHandler},
		// Object — forward
		{Args: []Type{TAtom, TObject}, QuoteArgs: map[int]bool{0: true}, Handler: swap2(getrObjectAtomHandler)},
		{Args: []Type{TString, TObject}, Handler: swap2(getrObjectStringHandler)},
		{Args: []Type{TInteger, TObject}, Handler: swap2(getrObjectIntegerHandler)},
		// None
		{Args: []Type{TNone, TAny}, Handler: getrNoneHandler},
	}

	r.Register("getr", sigs...)
	r.Register("!.", sigs...)
}
