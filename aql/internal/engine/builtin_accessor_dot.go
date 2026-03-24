package engine

import (
	"fmt"
	"strconv"
)

// registerDot registers "dot" and its "." alias for property/index access.
//
// Usage (suffix):
//
//	{a:1} dot a       => 1
//
// Usage (dot notation, handled by parser):
//
//	set foo a:b:1 foo.a.b  => 1
func registerDot(r *Registry) {
	dotMapAtomHandler := func(args []Value) ([]Value, error) {
		if args[0].Data == nil {
			return nil, fmt.Errorf("dot: cannot access property on type literal")
		}
		m := args[0].AsMap()
		key := args[1].AsAtom()
		val, ok := m.Get(key)
		if !ok {
			return []Value{NewTypeLiteral(TNone)}, nil
		}
		return []Value{val}, nil
	}

	dotMapStringHandler := func(args []Value) ([]Value, error) {
		if args[0].Data == nil {
			return nil, fmt.Errorf("dot: cannot access property on type literal")
		}
		m := args[0].AsMap()
		key := args[1].AsString()
		val, ok := m.Get(key)
		if !ok {
			return []Value{NewTypeLiteral(TNone)}, nil
		}
		return []Value{val}, nil
	}

	dotListHandler := func(args []Value) ([]Value, error) {
		if args[0].Data == nil {
			return nil, fmt.Errorf("dot: cannot index type literal")
		}
		list := args[0].AsList()
		idx := int(args[1].AsInteger())
		if idx < 0 || idx >= len(list) {
			return []Value{NewTypeLiteral(TNone)}, nil
		}
		return []Value{list[idx]}, nil
	}

	dotMapIntegerHandler := func(args []Value) ([]Value, error) {
		if args[0].Data == nil {
			return nil, fmt.Errorf("dot: cannot access property on type literal")
		}
		m := args[0].AsMap()
		key := strconv.FormatInt(args[1].AsInteger(), 10)
		val, ok := m.Get(key)
		if !ok {
			return []Value{NewTypeLiteral(TNone)}, nil
		}
		return []Value{val}, nil
	}

	dotNoneHandler := func(args []Value) ([]Value, error) {
		return []Value{NewTypeLiteral(TNone)}, nil
	}

	dotObjectAtomHandler := func(args []Value) ([]Value, error) {
		oi := args[0].AsObjectInstance()
		key := args[1].AsAtom()
		val, ok := oi.GetField(key)
		if !ok {
			return []Value{NewTypeLiteral(TNone)}, nil
		}
		return []Value{val}, nil
	}

	dotObjectStringHandler := func(args []Value) ([]Value, error) {
		oi := args[0].AsObjectInstance()
		key := args[1].AsString()
		val, ok := oi.GetField(key)
		if !ok {
			return []Value{NewTypeLiteral(TNone)}, nil
		}
		return []Value{val}, nil
	}

	sigs := []Signature{
		{Args: []Type{TObject, TAtom}, Handler: dotObjectAtomHandler},
		{Args: []Type{TObject, TString}, Handler: dotObjectStringHandler},
		{Args: []Type{TMap, TAtom}, Handler: dotMapAtomHandler},
		{Args: []Type{TMap, TString}, Handler: dotMapStringHandler},
		{Args: []Type{TList, TInteger}, Handler: dotListHandler},
		{Args: []Type{TMap, TInteger}, Handler: dotMapIntegerHandler},
		{Args: []Type{TNone, TAny}, Handler: dotNoneHandler},
	}

	r.Register("dot", sigs...)
	r.Register(".", sigs...)
}
