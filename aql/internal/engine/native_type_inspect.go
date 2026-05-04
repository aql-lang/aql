package engine

func RegisterInspect(r *Registry) {
	wordHandler := func(args []Value, _ map[string]Value, _ []Value, _ *Registry) ([]Value, error) {
		_as0, _ := args[0].AsWord()
		name := _as0.Name

		// User-defined types live in r.Types — predicate types (FnDef)
		// reach inspect through this branch and get the type-inspection
		// view. Native fn-shadow entries in DefStacks (every native
		// word has one for fallback dispatch) used to fool the old
		// "isTypeBody(DefStacks top)" check; that's now bypassed
		// because r.Types is the single source of truth for named
		// types and native fns are NOT in it.
		if tv, ok := r.TopOfTypeStack(name); ok {
			return []Value{buildTypeInspection(name, tv)}, nil
		}
		if top, ok := r.TopOfDefStack(name); ok {
			if isTypeBody(top) && !top.VType.Equal(TFnDef) && !top.VType.Equal(TFunction) {
				return []Value{buildTypeInspection(name, top)}, nil
			}
		}

		return []Value{buildInspection(r, name)}, nil
	}
	// Atom (now Scalar/Atom): inspect by name, same as words.
	atomHandler := func(args []Value, _ map[string]Value, _ []Value, _ *Registry) ([]Value, error) {
		name, _ := args[0].AsAtom()
		if tv, ok := r.TopOfTypeStack(name); ok {
			return []Value{buildTypeInspection(name, tv)}, nil
		}
		if top, ok := r.TopOfDefStack(name); ok {
			if isTypeBody(top) {
				return []Value{buildTypeInspection(name, top)}, nil
			}
		}
		return []Value{buildInspection(r, name)}, nil
	}

	// Type literal (Data==nil): inspect number, inspect string, etc.
	typeHandler := func(args []Value, _ map[string]Value, _ []Value, _ *Registry) ([]Value, error) {
		return []Value{buildTypeInspection("", args[0])}, nil
	}

	r.RegisterNativeFunc(NativeFunc{
		Name:              "inspect",
		ForwardPrecedence: true,
		Signatures: []NativeSig{
			{
				Args:    []Type{TWord},
				Handler: wordHandler,
				Returns: []Type{TInspect},
			},
			{
				Args:    []Type{TAtom},
				Handler: atomHandler,
				Returns: []Type{TInspect},
			},
			{
				Args:    []Type{TNode},
				Handler: typeHandler,
				Returns: []Type{TInspect},
			},
			{
				Args:    []Type{TScalar},
				Handler: typeHandler,
				Returns: []Type{TInspect},
			},
		},
	})
}

// buildInspection constructs a word_inspection map for the named word.
func buildInspection(r *Registry, name string) Value {
	result := NewOrderedMap()
	result.Set("name", NewString(name))

	fn := r.Lookup(name)
	if fn == nil {
		// No registered function — check if it's a simple def (list body).
		if r.HasDef(name) {
			result.Set("kind", NewAtom("defined"))
			result.Set("signatures", NewList(nil))
			return newValue(TInspect, result)
		}
		result.Set("kind", NewAtom("unknown"))
		result.Set("signatures", NewList(nil))
		return newValue(TInspect, result)
	}

	// Determine kind: AQL-defined functions have Sigs, Go-implemented do not.
	if len(fn.Sigs) > 0 {
		result.Set("kind", NewAtom("defined"))
	} else {
		result.Set("kind", NewAtom("native"))
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

		sigMaps = append(sigMaps, NewMap(sm))
	}
	if sigMaps == nil {
		sigMaps = []Value{}
	}
	result.Set("signatures", NewList(sigMaps))

	return newValue(TInspect, result)
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
		rt, _ := tv.AsRecordType()
		fields := NewOrderedMap()
		for _, k := range rt.Fields.Keys() {
			v, _ := rt.Fields.Get(k)
			fields.Set(k, NewString(v.VType.String()))
		}
		result.Set("fields", NewMap(fields))

	case tv.IsTableType():
		result.Set("kind", NewAtom("table"))
		tt, _ := tv.AsTableType()
		fields := NewOrderedMap()
		for _, k := range tt.Record.Fields.Keys() {
			v, _ := tt.Record.Fields.Get(k)
			fields.Set(k, NewString(v.VType.String()))
		}
		result.Set("fields", NewMap(fields))

	case tv.IsDisjunct():
		result.Set("kind", NewAtom("disjunct"))
		di, _ := tv.AsDisjunct()
		alts := make([]Value, len(di.Alternatives))
		for i, alt := range di.Alternatives {
			alts[i] = NewString(alt.VType.String())
		}
		result.Set("alternatives", NewList(alts))

	case tv.IsTypedList():
		result.Set("kind", NewAtom("typed_list"))
		_as1, _ := tv.AsChildType()
		child := _as1.Child
		result.Set("child", NewString(child.VType.String()))

	case tv.IsTypedMap():
		result.Set("kind", NewAtom("typed_map"))
		_as2, _ := tv.AsChildType()
		child := _as2.Child
		result.Set("child", NewString(child.VType.String()))

	case tv.VType.Equal(TFnUndef):
		// Structural fn-shape type: a FnUndef carries one or more
		// (Params, Returns) specs. Render each spec as a `params` /
		// `returns` pair so users can `inspect Mapper` and see the
		// signature shape they need to satisfy. Without this case
		// inspect's `signatures` slot would be empty for fn types.
		result.Set("kind", NewAtom("function_shape"))
		uInfo, _ := tv.Data.(FnUndefInfo)
		sigs := make([]Value, 0, len(uInfo.Sigs))
		for _, spec := range uInfo.Sigs {
			sig := NewOrderedMap()
			params := make([]Value, len(spec.Params))
			for i, p := range spec.Params {
				params[i] = NewString(p.Type.String())
			}
			sig.Set("params", NewList(params))
			rets := make([]Value, len(spec.Returns))
			for i, r := range spec.Returns {
				rets[i] = NewString(r.String())
			}
			sig.Set("returns", NewList(rets))
			sigs = append(sigs, NewMap(sig))
		}
		result.Set("signatures", NewList(sigs))

	case tv.IsDepScalar():
		// Dependent scalar: render the leaf and the populated
		// bound(s). Either Lo, Hi, or both may be set; each is
		// rendered as {kind: "gte"|"gt"|"lte"|"lt", value: <bound>}.
		result.Set("kind", NewAtom("dependent_scalar"))
		info, _ := tv.AsDepScalar()
		leaf := dependentLeafFromType(tv.VType)
		result.Set("leaf", NewString(leaf))
		if info.Lo != nil {
			lo := NewOrderedMap()
			lo.Set("kind", NewString(boundToKind(info.Lo, true).String()))
			lo.Set("value", info.Lo.Value)
			result.Set("lo", NewMap(lo))
		}
		if info.Hi != nil {
			hi := NewOrderedMap()
			hi.Set("kind", NewString(boundToKind(info.Hi, false).String()))
			hi.Set("value", info.Hi.Value)
			result.Set("hi", NewMap(hi))
		}

	default:
		// Simple type literal (Data==nil): number, string, boolean, etc.
		result.Set("kind", NewAtom("literal"))
	}

	return newValue(TInspect, result)
}
