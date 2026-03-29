package engine

import (
	"fmt"
	"strconv"
)

// registerDot registers "dot" and its "." alias for property/index access.
//
// Usage:
//
//	{a:1} dot "x"     => 1   (infix)
//	{a:1} "x" dot     => 1   (postfix)
//	dot "x" {a:1}     => 1   (prefix)
//	def bar {x:42} bar.x  => 42  (dot notation, handled by parser)
func registerDot(r *Registry) {
	// With forward-first matching, args are reversed: args[0]=key, args[1]=container.
	// `a dot b` means access b on a, so container=args[1], key=args[0].
	dotMapAtomHandler := func(args []Value) ([]Value, error) {
		if args[1].Data == nil {
			return nil, fmt.Errorf("dot: cannot access property on type literal")
		}
		m := args[1].AsMap()
		key := args[0].AsAtom()
		val, ok := m.Get(key)
		if !ok {
			return []Value{NewTypeLiteral(TNone)}, nil
		}
		return []Value{val}, nil
	}

	dotMapStringHandler := func(args []Value) ([]Value, error) {
		if args[1].Data == nil {
			return nil, fmt.Errorf("dot: cannot access property on type literal")
		}
		m := args[1].AsMap()
		key := args[0].AsString()
		val, ok := m.Get(key)
		if !ok {
			return []Value{NewTypeLiteral(TNone)}, nil
		}
		return []Value{val}, nil
	}

	dotListHandler := func(args []Value) ([]Value, error) {
		if args[1].Data == nil {
			return nil, fmt.Errorf("dot: cannot index type literal")
		}
		list := args[1].AsList()
		idx := int(args[0].AsInteger())
		if idx < 0 || idx >= len(list) {
			return []Value{NewTypeLiteral(TNone)}, nil
		}
		return []Value{list[idx]}, nil
	}

	dotMapIntegerHandler := func(args []Value) ([]Value, error) {
		if args[1].Data == nil {
			return nil, fmt.Errorf("dot: cannot access property on type literal")
		}
		m := args[1].AsMap()
		key := strconv.FormatInt(args[0].AsInteger(), 10)
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
		if m, ok := args[1].Data.(*OrderedMap); ok {
			key := args[0].AsAtom()
			val, found := m.Get(key)
			if !found {
				return []Value{NewTypeLiteral(TNone)}, nil
			}
			return []Value{val}, nil
		}
		oi := args[1].AsObjectInstance()
		key := args[0].AsAtom()
		val, ok := oi.GetField(key)
		if !ok {
			return []Value{NewTypeLiteral(TNone)}, nil
		}
		return []Value{val}, nil
	}

	dotObjectStringHandler := func(args []Value) ([]Value, error) {
		if m, ok := args[1].Data.(*OrderedMap); ok {
			key := args[0].AsString()
			val, found := m.Get(key)
			if !found {
				return []Value{NewTypeLiteral(TNone)}, nil
			}
			return []Value{val}, nil
		}
		oi := args[1].AsObjectInstance()
		key := args[0].AsString()
		val, ok := oi.GetField(key)
		if !ok {
			return []Value{NewTypeLiteral(TNone)}, nil
		}
		return []Value{val}, nil
	}

	sigs := []Signature{
		{Args: []Type{TAtom, TObject}, Handler: dotObjectAtomHandler},
		{Args: []Type{TString, TObject}, Handler: dotObjectStringHandler},
		{Args: []Type{TAtom, TMap}, Handler: dotMapAtomHandler},
		{Args: []Type{TString, TMap}, Handler: dotMapStringHandler},
		{Args: []Type{TInteger, TList}, Handler: dotListHandler},
		{Args: []Type{TInteger, TMap}, Handler: dotMapIntegerHandler},
		{Args: []Type{TAny, TNone}, Handler: dotNoneHandler},
	}

	r.Register("dot", sigs...)
	r.Register(".", sigs...)
}
