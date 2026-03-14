package engine

// registerDo registers the "do" word.
//
// For lists, do evaluates the list as a sub-program:
//
//	do [1 add 2]  →  3
//
// For maps, do evaluates any list values (depth-first), leaving non-list
// values unchanged:
//
//	do {x: [3 add 4], y: [upper a]}  →  {x:7, y:"A"}
func registerDo(r *Registry) {
	// promoteToWord converts a string value to a word if the string
	// names a registered function. This is needed because list elements
	// inside maps are parsed in data context (bare text → string),
	// but do needs to evaluate them as code (bare text → word).
	promoteToWord := func(v Value) Value {
		if v.VType.Matches(TString) || v.VType.Matches(TAtom) {
			name := v.AsString()
			if r.Lookup(name) != nil {
				return NewWord(name)
			}
		}
		return v
	}

	evalList := func(elems []Value) ([]Value, error) {
		sub := New(r)
		input := make([]Value, len(elems))
		copy(input, elems)
		return sub.Run(input)
	}

	// evalDataList evaluates a list from data context (inside a map).
	// Strings that name registered functions are promoted to words.
	evalDataList := func(elems []Value) ([]Value, error) {
		sub := New(r)
		input := make([]Value, len(elems))
		for i, e := range elems {
			input[i] = promoteToWord(e)
		}
		return sub.Run(input)
	}

	var evalMapValue func(v Value) (Value, error)
	evalMapValue = func(v Value) (Value, error) {
		if v.VType.Equal(TList) && !v.IsTypedList() && !v.IsTableType() {
			results, err := evalDataList(v.AsList())
			if err != nil {
				return Value{}, err
			}
			if len(results) == 1 {
				return results[0], nil
			}
			return NewList(results), nil
		}
		if v.VType.Equal(TMap) && !v.IsTypedMap() && !v.IsRecordType() {
			m := v.AsMap()
			out := NewOrderedMap()
			for _, key := range m.Keys() {
				val, _ := m.Get(key)
				evaluated, err := evalMapValue(val)
				if err != nil {
					return Value{}, err
				}
				out.Set(key, evaluated)
			}
			return NewMap(out), nil
		}
		return v, nil
	}

	r.Register("do",
		Signature{
			Args: []Type{TList},
			Handler: func(args []Value) ([]Value, error) {
				return evalList(args[0].AsList())
			},
		},
		Signature{
			Args: []Type{TMap},
			Handler: func(args []Value) ([]Value, error) {
				result, err := evalMapValue(args[0])
				if err != nil {
					return nil, err
				}
				return []Value{result}, nil
			},
		},
	)
}
