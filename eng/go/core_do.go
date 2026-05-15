package eng

// registerCoreDo installs `do BODY` — the explicit "evaluate this list
// or map" word. Two overloads:
//
//	do [body]   — runs the list as a sub-program against a fresh
//	              sub-engine and returns its residual stack. The body
//	              is captured raw (NoEvalArgs) so callers can pass
//	              lists that would otherwise auto-evaluate.
//
//	do {map}    — recursively walks a map literal, evaluating any
//	              embedded list values as sub-programs. The result is
//	              a parallel map with each list slot replaced by its
//	              evaluation result (a single value if the body
//	              produced one, otherwise a list of all results).
//
// Mirrors the production aql `do` (see
// lang/engine/native_control.go::doListHandler /
// doMapHandler). Errors in the list form are wrapped as Error values
// rather than aborting — this matches the production semantics where
// `do` participates in carrier-style flows that want to observe
// failures without stopping the host program.
func registerCoreDo(r *Registry) {
	r.RegisterNativeFunc(NativeFunc{
		Name:        "do",
		ForwardArgs: true,
		Signatures: []NativeSig{
			{
				Args:       []*Type{TList},
				NoEvalArgs: map[int]bool{0: true},
				Handler:    doListHandler,
				Returns:    []*Type{TAny},
			},
			{
				Args:    []*Type{TMap},
				Handler: doMapHandler,
				Returns: []*Type{TAny},
			},
		},
	})
}

func doListHandler(args []Value, _ map[string]Value, _ []Value, r *Registry) ([]Value, error) {
	if !IsConcrete(args[0]) {
		return nil, &AqlError{
			Code:   "type_error",
			Detail: "do: argument must be a concrete list, got type literal",
		}
	}
	return doEvalList(r, args[0].AsList().Slice())
}

func doMapHandler(args []Value, _ map[string]Value, _ []Value, r *Registry) ([]Value, error) {
	result, err := doEvalMapValue(r, args[0])
	if err != nil {
		return nil, err
	}
	return []Value{result}, nil
}

// doEvalList evaluates a top-level list of tokens in a sub-engine.
// Errors are caught and returned as a single Error value on the stack
// rather than propagating — same behaviour as the production aql
// implementation.
func doEvalList(r *Registry, elems []Value) ([]Value, error) {
	sub := New(r)
	input := make([]Value, len(elems))
	copy(input, elems)
	result, err := sub.Run(input)
	if err != nil {
		return []Value{NewError(err)}, nil
	}
	return result, nil
}

// doEvalDataList evaluates a list value pulled from data context
// (i.e. inside a map). Strings and atoms whose names match a
// registered word are promoted to Words so the sub-engine treats them
// as callables — this lets users write `{op:[1 "add" 2]}` and have
// "add" dispatch as the registered word.
func doEvalDataList(r *Registry, elems []Value) ([]Value, error) {
	sub := New(r)
	input := make([]Value, len(elems))
	for i, e := range elems {
		input[i] = doPromoteToWord(r, e)
	}
	return sub.Run(input)
}

// doPromoteToWord converts a string or atom value to a Word if its
// name resolves to a registered native word. Otherwise the value is
// returned unchanged.
func doPromoteToWord(r *Registry, v Value) Value {
	if v.VType.Matches(TString) || v.VType.Matches(TAtom) {
		name, _ := AsString(v)
		if r.Lookup(name) != nil {
			return NewWord(name)
		}
	}
	return v
}

// doEvalMapValue recursively evaluates list values within a map.
// Records, options, table types, typed lists / maps are left
// untouched — only plain concrete lists and maps are walked.
func doEvalMapValue(r *Registry, v Value) (Value, error) {
	if v.VType.Equal(TList) && v.Data != nil && !v.IsTypedList() && !v.IsTableType() {
		results, err := doEvalDataList(r, v.AsList().Slice())
		if err != nil {
			return Value{}, err
		}
		if len(results) == 1 {
			return results[0], nil
		}
		return NewList(results), nil
	}
	if v.VType.Equal(TMap) && v.Data != nil && !v.IsTypedMap() && !v.IsRecordType() && !v.IsOptionsType() {
		m := v.AsMap()
		if m == nil {
			return v, nil
		}
		out := NewOrderedMap()
		for _, key := range m.Keys() {
			val, _ := m.Get(key)
			evaluated, err := doEvalMapValue(r, val)
			if err != nil {
				return Value{}, err
			}
			out.Set(key, evaluated)
		}
		return NewMap(out), nil
	}
	return v, nil
}
