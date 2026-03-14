package engine

import (
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
		key := args[0].AsAtom()
		m := args[1].AsMap()
		val, ok := m.Get(key)
		if !ok {
			return []Value{NewTypeLiteral(TNone)}, nil
		}
		return []Value{val}, nil
	}

	dotMapStringHandler := func(args []Value) ([]Value, error) {
		key := args[0].AsString()
		m := args[1].AsMap()
		val, ok := m.Get(key)
		if !ok {
			return []Value{NewTypeLiteral(TNone)}, nil
		}
		return []Value{val}, nil
	}

	dotListHandler := func(args []Value) ([]Value, error) {
		idx := int(args[0].AsInteger())
		list := args[1].AsList()
		if idx < 0 || idx >= len(list) {
			return []Value{NewTypeLiteral(TNone)}, nil
		}
		return []Value{list[idx]}, nil
	}

	dotMapIntegerHandler := func(args []Value) ([]Value, error) {
		key := strconv.FormatInt(args[0].AsInteger(), 10)
		m := args[1].AsMap()
		val, ok := m.Get(key)
		if !ok {
			return []Value{NewTypeLiteral(TNone)}, nil
		}
		return []Value{val}, nil
	}

	dotNoneHandler := func(args []Value) ([]Value, error) {
		return []Value{NewTypeLiteral(TNone)}, nil
	}

	sigs := []Signature{
		{Args: []Type{TAtom, TMap}, Handler: dotMapAtomHandler},
		{Args: []Type{TString, TMap}, Handler: dotMapStringHandler},
		{Args: []Type{TInteger, TList}, Handler: dotListHandler},
		{Args: []Type{TInteger, TMap}, Handler: dotMapIntegerHandler},
		{Args: []Type{TAny, TNone}, Handler: dotNoneHandler},
	}

	r.Register("dot", sigs...)
	r.Register(".", sigs...)
}
