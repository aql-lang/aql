package eng

import "fmt"

// Binding inference for generic fns (design/GENERICS.10.md §9.2.2,
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

// ResolveChildTypeExpr evaluates a typed-list/map child constraint
// that arrived as an unevaluated ParenExpr — `[:(Pair of [String
// Integer])]` in data context parses with the paren span as the
// child payload, and nothing downstream evaluates it. Returns the
// rebuilt typed list/map (or v unchanged when the child is not a
// ParenExpr). The expression must produce exactly one type value.
func ResolveChildTypeExpr(r *Registry, v Value) (Value, error) {
	if r == nil || (!IsTypedList(v) && !IsTypedMap(v)) {
		return v, nil
	}
	ci, err := AsChildType(v)
	if err != nil || !IsParenExpr(ci.Child) {
		return v, nil
	}
	toks, terr := AsParenExpr(ci.Child)
	if terr != nil {
		return v, nil
	}
	sub := New(r)
	input := make([]Value, 0, len(toks)+2)
	input = append(input, NewOpenParen())
	input = append(input, toks...)
	input = append(input, NewCloseParen())
	out, rerr := sub.Run(input)
	if rerr != nil {
		return v, fmt.Errorf("child type expression: %w", rerr)
	}
	if len(out) != 1 {
		return v, fmt.Errorf("child type expression must produce one type, got %d values", len(out))
	}
	child := out[0]
	if IsTypedMap(v) {
		if len(ci.Entries) > 0 {
			return NewTypedMapWithEntries(child, ci.Entries), nil
		}
		return NewTypedMap(child), nil
	}
	if len(ci.Elements) > 0 {
		return NewTypedListWithElements(child, ci.Elements), nil
	}
	return NewTypedList(child), nil
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
		// Typed-list/map pattern over a placeholder (`xs:[:T]`,
		// `m:{:T}`): T binds the union of the element types.
		if p.Pattern != nil && (IsTypedList(*p.Pattern) || IsTypedMap(*p.Pattern)) {
			inferFromChildPattern(merge, isParam, *p.Pattern, arg)
		}
	}
	return bindings
}

// inferFromChildPattern handles one `[:T]` / `{:T}` param pattern
// against its call argument. The pattern child survives signature
// parsing as either the placeholder literal (ResolveSigChildParam) or
// a raw Word naming the parameter. The argument contributes element
// evidence three ways: a concrete list/map merges each element's
// type; a typed-list/map value or CARRIER (check mode strips list
// literals to typed-list carriers) merges the child constraint
// directly.
func inferFromChildPattern(merge func(string, Value), isParam map[string]bool, pattern, arg Value) {
	ci, err := AsChildType(pattern)
	if err != nil {
		return
	}
	name := TypeParamName(&ci.Child)
	if name == "" && IsWord(ci.Child) {
		if w, werr := AsWord(ci.Child); werr == nil {
			name = w.Name
		}
	}
	if name == "" || !isParam[name] {
		return
	}
	switch {
	case IsTypedList(arg) || IsTypedMap(arg):
		if aci, aerr := AsChildType(arg); aerr == nil {
			switch {
			case IsBareTypeNode(aci.Child):
				merge(name, aci.Child)
			case aci.Child.Parent != nil:
				merge(name, NewTypeLiteral(aci.Child.Parent))
			}
		}
	case arg.Parent != nil && arg.Parent.Equal(TList) && IsConcrete(arg):
		if lst, lerr := AsList(arg); lerr == nil {
			for j := 0; j < lst.Len(); j++ {
				el := lst.Get(j)
				if el.Parent != nil {
					merge(name, NewTypeLiteral(el.Parent))
				}
			}
		}
	case arg.Parent != nil && arg.Parent.Equal(TMap) && IsConcrete(arg):
		if m, merr := AsMap(arg); merr == nil && m != nil {
			for _, k := range m.Keys() {
				if v, ok := m.Get(k); ok && v.Parent != nil {
					merge(name, NewTypeLiteral(v.Parent))
				}
			}
		}
	}
}

// InferSchemaBindings infers a generic schema's type-parameter
// bindings from a concrete construction body (Phase 7, §9.2.2 + D12):
// each schema field whose declared constraint is a placeholder merges
// typeof(field value); a `[:T]`/`{:T}` child constraint merges the
// union of the element types. Conflicting evidence tor-merges — same
// contract as call-site inference for generic fns.
func InferSchemaBindings(info *TypeSchemaInfo, body Value) map[string]Value {
	bindings := map[string]Value{}
	if info == nil {
		return bindings
	}
	isParam := map[string]bool{}
	for _, p := range info.Params {
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
	fields := schemaFields(info)
	if fields == nil || body.Parent == nil || !body.Parent.Equal(TMap) || !IsConcrete(body) {
		return bindings
	}
	bm, err := AsMap(body)
	if err != nil || bm == nil {
		return bindings
	}
	for _, fname := range fields.Keys() {
		c, _ := fields.Get(fname)
		v, ok := bm.Get(fname)
		if !ok {
			continue
		}
		// Direct placeholder constraint: the field survives schema
		// construction as the placeholder literal (fields resolve while
		// the gen bindings are live) or, defensively, a raw Word.
		pname := TypeParamName(&c)
		if pname == "" && IsWord(c) {
			if w, werr := AsWord(c); werr == nil {
				pname = w.Name
			}
		}
		if pname != "" && isParam[pname] {
			if v.Parent != nil {
				merge(pname, NewTypeLiteral(v.Parent))
			}
			continue
		}
		if IsTypedList(c) || IsTypedMap(c) {
			inferFromChildPattern(merge, isParam, c, v)
		}
	}
	return bindings
}

// schemaFields returns the field-constraint map of a record or class
// schema body, nil for fn-shape schemas (no fields to infer from).
func schemaFields(info *TypeSchemaInfo) *OrderedMap {
	if IsRecordType(info.Body) {
		rt, _ := AsRecordType(info.Body)
		return rt.Fields
	}
	if oi, err := AsObjectType(info.Body); err == nil {
		return oi.Fields
	}
	return nil
}

// InferAndInstantiateSchema resolves a generic schema used directly as
// a construction constraint — `make Box {value:42}` or `def b:Box
// {value:42}` — by inferring the parameter bindings from the body and
// instantiating (Phase 7, resolving D12: a fully-defaulted schema
// auto-instantiates even without field evidence). An uninferable,
// undefaulted parameter is a loud unbound_param — never a silent Any.
func InferAndInstantiateSchema(r *Registry, schema Value, body Value) (Value, error) {
	info, err := AsTypeSchema(schema)
	if err != nil {
		return Value{}, err
	}
	bindings := InferSchemaBindings(info, body)
	args := make([]Value, 0, len(info.Params))
	for i := range info.Params {
		p := &info.Params[i]
		if b, ok := bindings[p.Name]; ok {
			args = append(args, b)
			continue
		}
		if p.HasDefault {
			// Defaults fill positionally at the tail inside
			// InstantiateSchema — stop here, but a BOUND parameter
			// after this one would be mis-positioned, so reject that
			// mix loudly rather than guessing.
			for j := i + 1; j < len(info.Params); j++ {
				if _, later := bindings[info.Params[j].Name]; later {
					return Value{}, r.AqlError("unbound_param",
						fmt.Sprintf("%s: parameter %s could not be inferred but a later parameter could — instantiate explicitly with `%s of [...]`",
							info.Name, p.Name, info.Name), "make")
				}
			}
			break
		}
		return Value{}, r.AqlErrorHint("unbound_param",
			fmt.Sprintf("%s: type parameter %s cannot be inferred from the construction body", info.Name, p.Name),
			"make",
			"instantiate explicitly: "+info.Name+" of [...]")
	}
	return InstantiateSchema(r, info, args)
}

// GenBindingCarrier converts one inferred binding value into a
// check-mode carrier: a binding that denotes a lattice node becomes a
// carrier of that node; a tor-merged disjunct becomes a disjunct
// carrier (the JoinCarriers representation); anything else falls back
// to its Parent.
func GenBindingCarrier(r *Registry, b Value) Value {
	if node := typeArgNode(b); node != nil {
		return NewCarrier(CanonicalType(r, node))
	}
	if IsDisjunct(b) {
		c := b
		c.Carrier = true
		return c
	}
	if b.Parent != nil {
		return NewCarrier(b.Parent)
	}
	return NewCarrier(TAny)
}

// InstallGenCallBindings infers and installs the body-scoped type
// bindings for one generic-fn call. At RUNTIME it MUST be called
// AFTER the body's def snapshot is taken: the bindings are then torn
// down by the existing DefCleanup truncation — NOT by the undef tail,
// whose capitalised-name path would Retire the bound type's canonical
// node (undef T with T→Integer must never retire Integer). Returns
// the installed names so check-mode callers (which have no DefCleanup
// frame) can pop them explicitly via Defs.Pop — also non-retiring.
func InstallGenCallBindings(r *Registry, spec *GenSpecInfo, params []FnParam, args []Value) []string {
	return InstallGenBindingMap(r, spec, InferGenBindings(spec, params, args))
}

// InstallGenBindingMap installs an already-inferred binding map as
// type bindings, in declared-parameter order. See
// InstallGenCallBindings for the teardown contract.
func InstallGenBindingMap(r *Registry, spec *GenSpecInfo, bindings map[string]Value) []string {
	var installed []string
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
		installed = append(installed, p.Name)
	}
	return installed
}
