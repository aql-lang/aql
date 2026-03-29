package engine

import (
	"fmt"
	"strconv"
)

// registerDot registers "dot" and its "." alias for property/index access.
//
// Usage (forward):
//
//	{a:1} dot a       => 1
//
// Usage (dot notation, handled by parser):
//
//	set foo a:b:1 foo.a.b  => 1
func registerDot(r *Registry) {
	// Forward handlers: args[0]=key, args[1]=container (reversed convention).
	fwdMapAtomHandler := func(args []Value) ([]Value, error) {
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

	fwdMapStringHandler := func(args []Value) ([]Value, error) {
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

	fwdListHandler := func(args []Value) ([]Value, error) {
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

	fwdMapIntegerHandler := func(args []Value) ([]Value, error) {
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

	fwdNoneHandler := func(args []Value) ([]Value, error) {
		return []Value{NewTypeLiteral(TNone)}, nil
	}

	fwdObjectAtomHandler := func(args []Value) ([]Value, error) {
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

	fwdObjectStringHandler := func(args []Value) ([]Value, error) {
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

	// Stack-only handlers: args[0]=container, args[1]=key (natural order).
	// Used by parser dot notation (bar.x → bar x dot~) with ForceStack.
	stackMapAtomHandler := func(args []Value) ([]Value, error) {
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

	stackMapStringHandler := func(args []Value) ([]Value, error) {
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

	stackListHandler := func(args []Value) ([]Value, error) {
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

	stackMapIntegerHandler := func(args []Value) ([]Value, error) {
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

	stackNoneHandler := func(args []Value) ([]Value, error) {
		return []Value{NewTypeLiteral(TNone)}, nil
	}

	stackObjectAtomHandler := func(args []Value) ([]Value, error) {
		if m, ok := args[0].Data.(*OrderedMap); ok {
			key := args[1].AsAtom()
			val, found := m.Get(key)
			if !found {
				return []Value{NewTypeLiteral(TNone)}, nil
			}
			return []Value{val}, nil
		}
		oi := args[0].AsObjectInstance()
		key := args[1].AsAtom()
		val, ok := oi.GetField(key)
		if !ok {
			return []Value{NewTypeLiteral(TNone)}, nil
		}
		return []Value{val}, nil
	}

	stackObjectStringHandler := func(args []Value) ([]Value, error) {
		if m, ok := args[0].Data.(*OrderedMap); ok {
			key := args[1].AsString()
			val, found := m.Get(key)
			if !found {
				return []Value{NewTypeLiteral(TNone)}, nil
			}
			return []Value{val}, nil
		}
		oi := args[0].AsObjectInstance()
		key := args[1].AsString()
		val, ok := oi.GetField(key)
		if !ok {
			return []Value{NewTypeLiteral(TNone)}, nil
		}
		return []Value{val}, nil
	}

	// Forward-precedence sigs with reversed arg order for forward/infix use.
	r.Register("dot",
		Signature{Args: []Type{TAtom, TObject}, Handler: fwdObjectAtomHandler},
		Signature{Args: []Type{TString, TObject}, Handler: fwdObjectStringHandler},
		Signature{Args: []Type{TAtom, TMap}, Handler: fwdMapAtomHandler},
		Signature{Args: []Type{TString, TMap}, Handler: fwdMapStringHandler},
		Signature{Args: []Type{TInteger, TList}, Handler: fwdListHandler},
		Signature{Args: []Type{TInteger, TMap}, Handler: fwdMapIntegerHandler},
		Signature{Args: []Type{TAny, TNone}, Handler: fwdNoneHandler},
	)
	r.Register(".",
		Signature{Args: []Type{TAtom, TObject}, Handler: fwdObjectAtomHandler},
		Signature{Args: []Type{TString, TObject}, Handler: fwdObjectStringHandler},
		Signature{Args: []Type{TAtom, TMap}, Handler: fwdMapAtomHandler},
		Signature{Args: []Type{TString, TMap}, Handler: fwdMapStringHandler},
		Signature{Args: []Type{TInteger, TList}, Handler: fwdListHandler},
		Signature{Args: []Type{TInteger, TMap}, Handler: fwdMapIntegerHandler},
		Signature{Args: []Type{TAny, TNone}, Handler: fwdNoneHandler},
	)

	// Stack-only sigs with natural arg order for ForceStack (parser dot notation).
	r.RegisterStackOnly("dot",
		Signature{Args: []Type{TObject, TAtom}, Handler: stackObjectAtomHandler},
		Signature{Args: []Type{TObject, TString}, Handler: stackObjectStringHandler},
		Signature{Args: []Type{TMap, TAtom}, Handler: stackMapAtomHandler},
		Signature{Args: []Type{TMap, TString}, Handler: stackMapStringHandler},
		Signature{Args: []Type{TList, TInteger}, Handler: stackListHandler},
		Signature{Args: []Type{TMap, TInteger}, Handler: stackMapIntegerHandler},
		Signature{Args: []Type{TNone, TAny}, Handler: stackNoneHandler},
	)
	r.RegisterStackOnly(".",
		Signature{Args: []Type{TObject, TAtom}, Handler: stackObjectAtomHandler},
		Signature{Args: []Type{TObject, TString}, Handler: stackObjectStringHandler},
		Signature{Args: []Type{TMap, TAtom}, Handler: stackMapAtomHandler},
		Signature{Args: []Type{TMap, TString}, Handler: stackMapStringHandler},
		Signature{Args: []Type{TList, TInteger}, Handler: stackListHandler},
		Signature{Args: []Type{TMap, TInteger}, Handler: stackMapIntegerHandler},
		Signature{Args: []Type{TNone, TAny}, Handler: stackNoneHandler},
	)
}
