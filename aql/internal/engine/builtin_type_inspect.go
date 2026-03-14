package engine

func registerInspect(r *Registry) {
	r.Register("inspect", Signature{
		Args: []Type{TWord},
		Handler: func(args []Value) ([]Value, error) {
			name := args[0].AsWord().Name
			return []Value{buildInspection(r, name)}, nil
		},
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
		return Value{VType: TWordInspection, Data: result}
	}

	// Determine kind: if there's a DefStacks entry, it's user-defined.
	if len(r.DefStacks[name]) > 0 {
		result.Set("kind", NewAtom("defined"))
	} else {
		result.Set("kind", NewAtom("builtin"))
	}

	// Add suffix_precedence flag.
	result.Set("suffix_precedence", NewBoolean(fn.SuffixPrecedence))

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

	return Value{VType: TWordInspection, Data: result}
}
