package engine

func RegisterIs(r *Registry) {
	// is: [any, any] -> [boolean]
	// Returns true if a unifies with b and the result equals a.
	// This is a type/value check: "42 is Number" → true, "42 is String" → false.
	// args[0] = nearest (top/forward), args[1] = farther. `a is Type` → args=[Type,a].
	r.RegisterNativeFunc(NativeFunc{
		Name:              "is",
		ForwardPrecedence: true,
		Signatures: []NativeSig{{
			Args:       []Type{TAny, TAny},
			BarrierPos: 1,
			Handler: func(args []Value, _ map[string]Value, _ []Value, _ *Registry) ([]Value, error) {
				a, b := args[1], args[0]
				// Resolve `(quote name)` on the value side when the
				// pattern is a function-shape type (FnUndef). quote
				// produces an Atom, but a structural fn-type
				// constraint wants the underlying FnDef value to
				// compare against — same lookup defTypedHandler does.
				if b.VType.Equal(TFnUndef) && a.IsAtom() {
					name, _ := a.AsAtom()
					if ds := r.DefStacks[name]; len(ds) > 0 {
						top := ds[len(ds)-1]
						if top.VType.Equal(TFnDef) || top.VType.Equal(TFunction) {
							a = top
						}
					}
				}
				// Predicate-type check: if the pattern (b) is a fn
				// (Quoted=true so it didn't auto-execute on the way
				// here), call the predicate against a and report
				// success iff it returned a non-None value.
				// Mirrors defTypedHandler's invocation rules.
				if b.VType.Equal(TFnDef) || b.VType.Equal(TFunction) {
					if fnDef, ok := b.Data.(FnDefInfo); ok && len(fnDef.Sigs) > 0 && len(fnDef.Sigs[0].Params) == 1 {
						result, err := r.CallAQL(&fnDef.Sigs[0], []Value{a})
						if err != nil || len(result) != 1 {
							return []Value{NewBoolean(false)}, nil
						}
						return []Value{NewBoolean(!result[0].VType.Equal(TNone))}, nil
					}
				}
				// Metatype early-return: when pattern (b) is a metatype and
				// value (a) is a type literal, directly check metatype matching.
				// (a=value, b=pattern due to the swap above.)
				if b.Data == nil && IsMetaType(b.VType) && a.Data == nil {
					aMeta := MetatypeFor(a.VType)
					return []Value{NewBoolean(aMeta.Matches(b.VType))}, nil
				}
				unified, ok := Unify(a, b)
				if !ok {
					return []Value{NewBoolean(false)}, nil
				}
				// Compare against the resolved form of a so that words
				// (true/false, atoms) inside lists are treated as their
				// semantic values.
				resolved := resolveWordsDeep(a)
				if !unified.VType.Equal(resolved.VType) {
					return []Value{NewBoolean(false)}, nil
				}
				if !valuesEqual(unified, resolved) {
					return []Value{NewBoolean(false)}, nil
				}
				return []Value{NewBoolean(true)}, nil
			},
			Returns: []Type{TBoolean},
		}},
	})
}
