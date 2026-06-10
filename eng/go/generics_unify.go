package eng

// Binding inference for generic fns (design/GENERICS.0.md §9.2.2,
// Phase 4): at each call of a generic fn, the type parameters bind
// from the actual arguments' types — a placeholder param slot binds
// typeof(arg); a typed-list pattern over a placeholder binds the
// union of the element types. Conflicting evidence for one parameter
// takes the union (`tor`-merge), per the design — runtime calls are
// never rejected for parameter inconsistency (the checker reports
// precision losses in Phase 5).

// ResolveSigChildParam resolves a typed-list/typed-map param pattern
// whose child constraint is a raw Word naming a LIVE gen placeholder
// (`xs:[:T]` while `gen [T]`'s bindings are pushed). Definition time
// is the only moment the name can resolve — the placeholder bindings
// pop when the generic fn's def completes — so ResolveSigType calls
// this while parsing the signature. Non-placeholder children (builtin
// literals, concrete patterns, user-type Words) pass through
// untouched.
func ResolveSigChildParam(r *Registry, v Value) Value {
	if r == nil {
		return v
	}
	ci, err := AsChildType(v)
	if err != nil || !IsWord(ci.Child) {
		return v
	}
	w, werr := AsWord(ci.Child)
	if werr != nil {
		return v
	}
	def := r.LookupTypeName(w.Name)
	if def == nil || !IsTypeParamNode(def) {
		return v
	}
	child := NewTypeLiteral(def)
	if IsTypedMap(v) {
		if len(ci.Entries) > 0 {
			return NewTypedMapWithEntries(child, ci.Entries)
		}
		return NewTypedMap(child)
	}
	if len(ci.Elements) > 0 {
		return NewTypedListWithElements(child, ci.Elements)
	}
	return NewTypedList(child)
}

// typeParamLitNode returns the placeholder node when v is a bare type
// literal of a minted gen-param node, nil otherwise. The Behavior
// pointer survives the literal's by-value copy, so detection works on
// the copy without a registry lookup.
func typeParamLitNode(v Value) *Type {
	if !IsBareTypeNode(v) {
		return nil
	}
	if IsTypeParamNode(&v) {
		return &v
	}
	return nil
}

// unifyTypeParam admits the other side of a unification against a
// placeholder literal by the parameter's BOUND (genParamUnifier.Match:
// unconstrained → anything; `extends C` → Is-membership in C). The
// fold exists because placeholder literals reach Unify embedded in
// structural patterns — a generic fn's `xs:[:T]` param unifies each
// list element against the placeholder — and the standard narrowing
// rule (ConformsTo) has no admission path from a concrete value into
// a Type/TypeParam node.
func unifyTypeParam(lit Value, node *Type, other Value) (Value, *UnifyError) {
	// The same placeholder on both sides (memo keys, `[T] vs [T]`).
	if IsBareTypeNode(other) && other.ID == node.ID {
		return lit, nil
	}
	if other.Is(node) {
		return other, nil
	}
	return Value{}, unifyFail("value does not satisfy the type parameter's bound", lit, other)
}

// InferGenBindings walks a signature's params alongside the call args
// and collects parameter-name → type-value bindings.
func InferGenBindings(spec *GenSpecInfo, params []FnParam, args []Value) map[string]Value {
	bindings := map[string]Value{}
	if spec == nil {
		return bindings
	}
	isParam := map[string]bool{}
	for _, p := range spec.Params {
		isParam[p.Name] = true
	}
	merge := func(name string, t Value) {
		if !isParam[name] {
			return
		}
		if prev, ok := bindings[name]; ok {
			bindings[name] = unionType(prev, t)
			return
		}
		bindings[name] = t
	}
	for i, p := range params {
		if i >= len(args) {
			break
		}
		arg := args[i]
		// Placeholder param slot: T binds the arg's own type.
		if name := TypeParamName(p.Type); name != "" {
			if arg.Parent != nil {
				merge(name, NewTypeLiteral(arg.Parent))
			}
			continue
		}
		// Typed-list pattern over a placeholder (`xs:[:T]`): T binds
		// the union of the element types. The child survives schema
		// parsing as either the placeholder literal or a raw Word.
		if p.Pattern != nil && IsTypedList(*p.Pattern) {
			ci, err := AsChildType(*p.Pattern)
			if err != nil {
				continue
			}
			name := TypeParamName(&ci.Child)
			if name == "" && IsWord(ci.Child) {
				if w, werr := AsWord(ci.Child); werr == nil {
					name = w.Name
				}
			}
			if name == "" || !isParam[name] {
				continue
			}
			if arg.Parent != nil && arg.Parent.Equal(TList) && IsConcrete(arg) {
				lst, lerr := AsList(arg)
				if lerr == nil {
					for j := 0; j < lst.Len(); j++ {
						el := lst.Get(j)
						if el.Parent != nil {
							merge(name, NewTypeLiteral(el.Parent))
						}
					}
				}
			}
		}
	}
	return bindings
}

// InstallGenCallBindings infers and installs the body-scoped type
// bindings for one generic-fn call. MUST be called AFTER the body's
// def snapshot is taken: the bindings are then torn down by the
// existing DefCleanup truncation — NOT by the undef tail, whose
// capitalised-name path would Retire the bound type's canonical node
// (undef T with T→Integer must never retire Integer).
func InstallGenCallBindings(r *Registry, spec *GenSpecInfo, params []FnParam, args []Value) {
	bindings := InferGenBindings(spec, params, args)
	for _, p := range spec.Params {
		b, ok := bindings[p.Name]
		if !ok {
			continue
		}
		node := typeArgNode(b)
		if node == nil {
			node = b.Parent
		}
		r.Defs.PushType(p.Name, CanonicalType(r, node), b)
	}
}
