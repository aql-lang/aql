package engine

import (
	"fmt"
	"strconv"
)

// registerDot registers "dot" and its "." alias for property/index access.
//
// Signature: [(Atom or String or Integer) (Node or Record or Entity or Options)]
//
// dot is a forward-precedence function: it collects the key from forward
// and takes the container from the stack.
//
// Usage:
//
//	{a:1} dot a       => 1   (forward key)
//	{a:{b:1}} dot a dot b => 1   (chained access)
//	[10 20 30] dot 1  => 20  (list index)
//	def m {x:42} m.x  => 42  (dot notation, expanded by parser)
func registerDot(r *Registry) {
	// With forward-first matching, args[0]=key (collected forward),
	// args[1]=container (from stack).

	// --- Node handlers (Map, List, Options) ---

	dotNodeAtomHandler := func(args []Value, _ map[string]Value, _ []Value, _ *Registry) ([]Value, error) {
		container := args[1]
		if container.Data == nil {
			return nil, fmt.Errorf("dot: cannot access property on type literal")
		}
		key := args[0].AsAtom()
		// Node can be Map/Options or List. Atom keys only work on maps.
		if m := container.AsMap(); m != nil {
			val, ok := m.Get(key)
			if !ok {
				return []Value{NewTypeLiteral(TNone)}, nil
			}
			return []Value{val}, nil
		}
		// Atom key on a List → None.
		return []Value{NewTypeLiteral(TNone)}, nil
	}

	dotNodeStringHandler := func(args []Value, _ map[string]Value, _ []Value, _ *Registry) ([]Value, error) {
		container := args[1]
		if container.Data == nil {
			return nil, fmt.Errorf("dot: cannot access property on type literal")
		}
		key := args[0].AsString()
		if m := container.AsMap(); m != nil {
			val, ok := m.Get(key)
			if !ok {
				return []Value{NewTypeLiteral(TNone)}, nil
			}
			return []Value{val}, nil
		}
		// String key on a List → None.
		return []Value{NewTypeLiteral(TNone)}, nil
	}

	dotNodeIntegerHandler := func(args []Value, _ map[string]Value, _ []Value, _ *Registry) ([]Value, error) {
		container := args[1]
		if container.Data == nil {
			return nil, fmt.Errorf("dot: cannot access property on type literal")
		}
		idx := args[0].AsInteger()
		// Integer key: index into List, or string-convert for Map.
		if list := container.AsList(); list != nil && container.VType.Matches(TList) {
			i := int(idx)
			if i < 0 || i >= len(list) {
				return []Value{NewTypeLiteral(TNone)}, nil
			}
			return []Value{list[i]}, nil
		}
		if m := container.AsMap(); m != nil {
			key := strconv.FormatInt(idx, 10)
			val, ok := m.Get(key)
			if !ok {
				return []Value{NewTypeLiteral(TNone)}, nil
			}
			return []Value{val}, nil
		}
		return []Value{NewTypeLiteral(TNone)}, nil
	}

	// --- Object handlers (Record, Entity) ---

	dotObjectAtomHandler := func(args []Value, _ map[string]Value, _ []Value, _ *Registry) ([]Value, error) {
		container := args[1]
		if container.Data == nil {
			return nil, fmt.Errorf("dot: cannot access property on type literal")
		}
		key := args[0].AsAtom()
		if m, ok := container.Data.(*OrderedMap); ok {
			val, found := m.Get(key)
			if !found {
				return []Value{NewTypeLiteral(TNone)}, nil
			}
			return []Value{val}, nil
		}
		oi := container.AsObjectInstance()
		val, ok := oi.GetField(key)
		if !ok {
			return []Value{NewTypeLiteral(TNone)}, nil
		}
		return []Value{val}, nil
	}

	dotObjectStringHandler := func(args []Value, _ map[string]Value, _ []Value, _ *Registry) ([]Value, error) {
		container := args[1]
		if container.Data == nil {
			return nil, fmt.Errorf("dot: cannot access property on type literal")
		}
		key := args[0].AsString()
		if m, ok := container.Data.(*OrderedMap); ok {
			val, found := m.Get(key)
			if !found {
				return []Value{NewTypeLiteral(TNone)}, nil
			}
			return []Value{val}, nil
		}
		oi := container.AsObjectInstance()
		val, ok := oi.GetField(key)
		if !ok {
			return []Value{NewTypeLiteral(TNone)}, nil
		}
		return []Value{val}, nil
	}

	dotObjectIntegerHandler := func(args []Value, _ map[string]Value, _ []Value, _ *Registry) ([]Value, error) {
		container := args[1]
		if container.Data == nil {
			return nil, fmt.Errorf("dot: cannot access property on type literal")
		}
		key := strconv.FormatInt(args[0].AsInteger(), 10)
		if m, ok := container.Data.(*OrderedMap); ok {
			val, found := m.Get(key)
			if !found {
				return []Value{NewTypeLiteral(TNone)}, nil
			}
			return []Value{val}, nil
		}
		oi := container.AsObjectInstance()
		val, ok := oi.GetField(key)
		if !ok {
			return []Value{NewTypeLiteral(TNone)}, nil
		}
		return []Value{val}, nil
	}

	// --- None handler ---

	dotNoneHandler := func(args []Value, _ map[string]Value, _ []Value, _ *Registry) ([]Value, error) {
		return []Value{NewTypeLiteral(TNone)}, nil
	}

	sigs := []Signature{
		// Node containers (Map, List, Options)
		{Args: []Type{TAtom, TNode}, Handler: dotNodeAtomHandler},
		{Args: []Type{TString, TNode}, Handler: dotNodeStringHandler},
		{Args: []Type{TInteger, TNode}, Handler: dotNodeIntegerHandler},
		// Object containers (Record, Entity)
		{Args: []Type{TAtom, TObject}, Handler: dotObjectAtomHandler},
		{Args: []Type{TString, TObject}, Handler: dotObjectStringHandler},
		{Args: []Type{TInteger, TObject}, Handler: dotObjectIntegerHandler},
		// None propagation
		{Args: []Type{TAny, TNone}, Handler: dotNoneHandler},
	}

	r.Register("dot", sigs...)
	r.Register(".", sigs...)
}
