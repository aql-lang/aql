package engine

func registerInspect(r *Registry) {
	r.Register("inspect", Signature{
		Args: []Type{TWord},
		Handler: func(args []Value) ([]Value, error) {
			name := args[0].AsWord().Name

			// If the word names a user-defined type, return a type inspection.
			if stack := r.DefStacks[name]; len(stack) > 0 {
				top := stack[len(stack)-1]
				if isTypeValue(top) {
					return []Value{buildTypeInspection(name, top)}, nil
				}
			}

			return []Value{buildInspection(r, name)}, nil
		},
	})

	// Type literal (Data==nil): inspect number, inspect string, etc.
	typeHandler := func(args []Value) ([]Value, error) {
		return []Value{buildTypeInspection("", args[0])}, nil
	}

	r.Register("inspect", Signature{
		Args:    []Type{TNode},
		Handler: typeHandler,
	})
	r.Register("inspect", Signature{
		Args:    []Type{TScalar},
		Handler: typeHandler,
	})
}

// buildInspection constructs a word_inspection map for the named word.
func buildInspection(r *Registry, name string) Value {
	result := NewOrderedMap()
	result.Set("name", NewString(name))

	fn := r.Lookup(name)
	if fn == nil {
		result.Set("kind", NewAtom("unknown"))
		result.Set("signatures", NewList(nil))
		return newValue(TWordInspect, result)
	}

	// Determine kind: if there's a DefStacks entry, it's user-defined.
	if len(r.DefStacks[name]) > 0 {
		result.Set("kind", NewAtom("defined"))
	} else {
		result.Set("kind", NewAtom("builtin"))
	}

	// Add forward_precedence flag.
	result.Set("forward_precedence", NewBoolean(fn.ForwardPrecedence))

	// Build signature list.
	var sigMaps []Value
	for _, sig := range fn.Signatures {
		sm := NewOrderedMap()

		var argVals []Value
		for _, argType := range sig.Args {
			argVals = append(argVals, NewString(argType.String()))
		}
		if argVals == nil {
			argVals = []Value{}
		}
		sm.Set("args", NewList(argVals))
		sm.Set("precedence", NewInteger(int64(sig.Precedence)))

		sigMaps = append(sigMaps, NewMap(sm))
	}
	if sigMaps == nil {
		sigMaps = []Value{}
	}
	result.Set("signatures", NewList(sigMaps))

	return newValue(TWordInspect, result)
}

// buildTypeInspection constructs a type_inspection map for a type value.
func buildTypeInspection(name string, tv Value) Value {
	result := NewOrderedMap()

	if name != "" {
		result.Set("name", NewString(name))
	}

	result.Set("type", NewString(tv.VType.String()))

	switch {
	case tv.IsRecordType():
		result.Set("kind", NewAtom("record"))
		rt := tv.AsRecordType()
		fields := NewOrderedMap()
		for _, k := range rt.Fields.Keys() {
			v, _ := rt.Fields.Get(k)
			fields.Set(k, NewString(v.VType.String()))
		}
		result.Set("fields", NewMap(fields))

	case tv.IsTableType():
		result.Set("kind", NewAtom("table"))
		tt := tv.AsTableType()
		fields := NewOrderedMap()
		for _, k := range tt.Record.Fields.Keys() {
			v, _ := tt.Record.Fields.Get(k)
			fields.Set(k, NewString(v.VType.String()))
		}
		result.Set("fields", NewMap(fields))

	case tv.IsDisjunct():
		result.Set("kind", NewAtom("disjunct"))
		di := tv.AsDisjunct()
		alts := make([]Value, len(di.Alternatives))
		for i, alt := range di.Alternatives {
			alts[i] = NewString(alt.VType.String())
		}
		result.Set("alternatives", NewList(alts))

	case tv.IsTypedList():
		result.Set("kind", NewAtom("typed_list"))
		child := tv.AsChildType().Child
		result.Set("child", NewString(child.VType.String()))

	case tv.IsTypedMap():
		result.Set("kind", NewAtom("typed_map"))
		child := tv.AsChildType().Child
		result.Set("child", NewString(child.VType.String()))

	default:
		// Simple type literal (Data==nil): number, string, boolean, etc.
		result.Set("kind", NewAtom("literal"))
	}

	return newValue(TTypeInspect, result)
}
