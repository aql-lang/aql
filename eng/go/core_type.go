package eng

import (
	"strings"
)

// registerCoreType installs the language-fundamental type words:
//
//	type NAME body    — bind a type name (NAME must start [A-Z]) to a
//	                    type body (a type literal, disjunct, implicit
//	                    map shape, or any value satisfying IsTypeBody).
//	                    Pushes onto the type stack — `type Foo Integer;
//	                    type Foo String` shadows; `untype Foo` pops.
//
//	untype NAME       — pop the most-recent binding for NAME from the
//	                    type stack. Errors if no binding exists.
//
//	typeof v          — return the type of v as a type-literal Value
//	                    (e.g. typeof 5 → Integer, typeof Integer →
//	                    Type, typeof none → None). The type-of any
//	                    type literal is uniformly `Type` — there is no
//	                    ScalarType / NodeType / ObjectType layer.
//
//	v is T            — Boolean: does v satisfy type T? Forward sig is
//	                    [Any | Any], so `5 is Integer` reads naturally
//	                    (T from forward, v from stack).
//
// Mirrors the production aql `type` / `untype` / `typeof` / `is`
// (see lang/engine/native_type.go); these are the foundational
// type-system surface that the richer words (record, table, object,
// make, guard, inspect, …) build on. The richer words are NOT in
// aqleng's core — they layer on top in the production engine.
func registerCoreType(r *Registry) {
	registerCoreTypeWord(r)
	registerCoreUntypeWord(r)
	registerCoreTypeof(r)
	registerCorePathOf(r)
	registerCoreIs(r)
	registerCoreEnum(r)
}

// registerCoreEnum installs `enum [a b c]` — fixed-enumeration type
// builder. Returns an Enum value (type Type/Disjunct/Enum — a
// subtype of Disjunct) whose alternatives are the list's elements
// (Words become Atoms so `enum [red green blue]` defines an enum of
// three named values without requiring `quote`). `typeof` reports
// `Enum`; structurally it behaves as a disjunct for `is` / unify.
//
// When the list carries a child-type constraint (`[ :T a b c]`), each
// element is validated against T before being added to the enum.
//
// Used in the same shape as `fn`:
//
//	def Color enum [red green blue]
//	red is Color    → true
//	pink is Color   → false
//
// And with child types:
//
//	def Codes enum [: Integer 200 404 500]
//	200 is Codes  → true
//	201 is Codes  → false
func registerCoreEnum(r *Registry) {
	r.RegisterNativeFunc(NativeFunc{
		Name:        "enum",
		ForwardArgs: true,
		Signatures: []NativeSig{{
			Args:       []*Type{TList},
			NoEvalArgs: map[int]bool{0: true},
			Handler: func(args []Value, _ map[string]Value, _ []Value, _ *Registry) ([]Value, error) {
				list := args[0]
				if list.Data == nil {
					return nil, &AqlError{Code: "type_error", Detail: "enum: argument must be a concrete list"}
				}
				var childType Value
				hasChild := false
				if IsTypedList(list) {
					ci, _ := AsChildType(list)
					childType = ci.Child
					hasChild = childType.VType != nil
				}
				elems := AsList(list)
				alts := make([]Value, 0, elems.Len())
				for i := 0; i < elems.Len(); i++ {
					e := elems.Get(i)
					// Word → Atom conversion: enum members are
					// typically named, and in word context the parser
					// produces Words. Convert here so users don't need
					// to wrap each element in `quote`.
					if IsWord(e) {
						w, _ := AsWord(e)
						e = NewAtom(w.Name)
					}
					if hasChild && !IsValueOfType(e, childType) {
						return nil, &AqlError{
							Code:   "type_error",
							Detail: "enum: element " + e.String() + " does not satisfy child type " + childType.String(),
						}
					}
					alts = append(alts, e)
				}
				return []Value{NewEnum(alts)}, nil
			},
			Returns: []*Type{TEnum},
		}},
	})
}

// registerCoreTypeWord installs `type NAME body`. NAME arrives as a
// Word (the bare aqleng tokenizer never quotes; the production parser's
// /q machinery captures the name as an Atom — both forms route here).
//
// The body is unquoted (NoEvalArgs[1] = true) because type bodies are
// literal type expressions, not code to evaluate.
func registerCoreTypeWord(r *Registry) {
	r.RegisterNativeFunc(NativeFunc{
		Name:        "type",
		ForwardArgs: true,
		Signatures: []NativeSig{{
			Args:       []*Type{TAtom, TAny},
			QuoteArgs:  map[int]bool{0: true},
			NoEvalArgs: map[int]bool{1: true},
			Handler: func(args []Value, _ map[string]Value, _ []Value, reg *Registry) ([]Value, error) {
				name, _ := args[0].AsConcreteAtom()
				body := args[1]
				if err := InstallType(reg, name, body); err != nil {
					return nil, err
				}
				return nil, nil
			},
			Returns: []*Type{},
		}},
	})
}

// registerCoreUntypeWord installs `untype NAME`. Name arrives as a Word.
func registerCoreUntypeWord(r *Registry) {
	r.RegisterNativeFunc(NativeFunc{
		Name:        "untype",
		ForwardArgs: true,
		Signatures: []NativeSig{{
			Args:      []*Type{TAtom},
			QuoteArgs: map[int]bool{0: true},
			Handler: func(args []Value, _ map[string]Value, _ []Value, reg *Registry) ([]Value, error) {
				name, _ := args[0].AsConcreteAtom()
				if !IsCapitalisedName(name) {
					return nil, &AqlError{
						Code:   "type_error",
						Detail: "untype " + name + ": type names must start with a capital letter",
					}
				}
				if _, ok := reg.Types.PopType(name); !ok {
					return nil, &AqlError{
						Code:   "type_error",
						Detail: "untype " + name + ": no such type binding",
					}
				}
				return nil, nil
			},
			Returns: []*Type{},
		}},
	})
}

// registerCoreTypeof installs `typeof v`. Returns a Type literal —
// the type of v, expressed as a value:
//
//	typeof 5            → Integer        (concrete value's exact type)
//	typeof 5.0          → Decimal
//	typeof "x"          → String
//	typeof true         → Boolean
//	typeof none         → None           (None has a single inhabitant)
//	typeof [ 1 2 ]      → List
//	typeof { a:1 }      → Map
//	typeof Integer      → Type            (ANY type literal → Type)
//	typeof List         → Type
//	typeof Any          → Type
//
// Rules:
//   - For `none` (the unique inhabitant of None), return None itself.
//   - For any other type literal (Data == nil), return `Type`.
//     Metatypes are collapsed — there is no ScalarType / NodeType /
//     ObjectType layer; the type-of-a-type-literal is uniformly `Type`.
//   - For a concrete value (Data != nil), return its full VType.
//
// typeof's result is itself a type literal so it round-trips: passing
// the result to `is` or chaining `typeof typeof v` (always `Type`)
// produces the expected answers.
func registerCoreTypeof(r *Registry) {
	r.RegisterNativeFunc(NativeFunc{
		Name:        "typeof",
		ForwardArgs: true,
		Signatures: []NativeSig{{
			Args: []*Type{TAny},
			Handler: func(args []Value, _ map[string]Value, _ []Value, _ *Registry) ([]Value, error) {
				return []Value{TypeOf(args[0])}, nil
			},
			Returns: []*Type{TType},
		}},
	})
}

// registerCorePathOf installs `pathof T` — the ancestry path of a
// TYPE. It takes a Type literal (not an arbitrary value) and returns
// the chain of progressively-deeper Type literals from the top-level
// root down to T itself, as a List of Type (`[Type [:Type]]`):
//
//	pathof Integer          → [Scalar Number Integer]
//	pathof ProperString     → [Scalar String ProperString]
//	pathof List             → [Node List]
//	pathof Function         → [Type Function]
//	pathof Enum             → [Type Disjunct Enum]
//	pathof Type             → [Type]              (Type has no ancestors)
//	pathof None             → [None]
//
// The last element always equals T. To get the path of a *value*'s
// type, compose with `typeof`:
//
//	pathof ( typeof 5 )     → [Scalar Number Integer]   (= pathof Integer)
//	pathof ( typeof none )  → [None]
//	pathof ( typeof Integer ) → [Type]                  (= pathof Type)
//
// `pathof 5` is a signature_error — the argument must be a Type.
func registerCorePathOf(r *Registry) {
	r.RegisterNativeFunc(NativeFunc{
		Name:        "pathof",
		ForwardArgs: true,
		Signatures: []NativeSig{{
			Args: []*Type{TType},
			Handler: func(args []Value, _ map[string]Value, _ []Value, _ *Registry) ([]Value, error) {
				return []Value{PathOf(args[0])}, nil
			},
			Returns: []*Type{TList},
		}},
	})
}

// PathOf returns the ancestry path of the type T (a Type literal, or
// any value whose VType is a Type subtype — e.g. a Function/Disjunct
// value) as a List of Type literals, root first, leaf last. See
// registerCorePathOf.
func PathOf(t Value) Value {
	// Walk the def's ancestry from root down to t, producing one type
	// literal per ancestor.
	var chain []*Type
	for d := t.VType; d != nil; d = d.Parent {
		chain = append([]*Type{d}, chain...)
	}
	elems := make([]Value, 0, len(chain))
	for _, d := range chain {
		elems = append(elems, NewTypeLiteral(d))
	}
	return NewList(elems)
}

// TypeOf returns the Type of v as a type-literal Value. See
// registerCoreTypeof for the dispatch rules.
func TypeOf(v Value) Value {
	// The VALUE `none` (Data != nil, VType == TNone) → None type literal.
	if IsNone(v) {
		return NewTypeLiteral(TNone)
	}
	// A type literal (Data == nil) → `Type`. Metatypes are collapsed:
	// the type-of-a-type-literal is uniformly `Type` (not ScalarType /
	// NodeType / ObjectType).
	if v.Data == nil {
		return NewTypeLiteral(TType)
	}
	// Typed list `[:T]` or typed map `{:T}` — Node-family TYPE
	// declarations carrying a child-type constraint, not concrete
	// containers. They are types → `Type`.
	if IsTypedList(v) || IsTypedMap(v) {
		return NewTypeLiteral(TType)
	}
	// An implicit-map record shape (every entry's value is itself a
	// type body — type literal or nested shape) is a Node-family TYPE,
	// not a concrete map → `Type`, so user code can branch on shape
	// vs concrete map without inspecting the data.
	if IsRecordShape(v) {
		return NewTypeLiteral(TType)
	}
	// Concrete value — its exact VType.
	return NewTypeLiteral(v.VType)
}

// IsRecordShape reports whether v is a non-empty map all of whose
// field values are themselves type bodies (type literals or nested
// record shapes). Independent of how the map was constructed
// (production aql `{x:Integer}` produces an explicit OrderedMap;
// the implicit-pair syntax inside fn signatures produces an Implicit
// map; both are treated as record shapes here when their values are
// type-shape values).
//
// The empty map `{}` is treated as a concrete value, not a shape,
// so `typeof { } → Map`. A mixed-content map like `{x:1 y:String}`
// has a concrete x payload and so is also NOT a record shape (typeof
// returns Map). Singleton-typed shapes still go via `is`'s structural
// unification path.
func IsRecordShape(v Value) bool {
	if !v.VType.Equal(TMap) || v.Data == nil {
		return false
	}
	m := AsMap(v)
	if m == nil || m.Len() == 0 {
		return false
	}
	for _, k := range m.Keys() {
		fv, _ := m.Get(k)
		if fv.Data == nil {
			continue // type literal (or None type literal)
		}
		if IsRecordShape(fv) {
			continue // nested shape
		}
		return false
	}
	return true
}

// registerCoreIs installs `v is T` — Boolean: does v satisfy T?
//
// Sig is [TAny, TAny] with BarrierPos=1: T forward-eligible (sig[0]),
// v from the stack (sig[1]). Reading order in source is `v is T` —
// the natural infix reading binds v from the prefix and T from the
// upcoming forward token.
//
// Type-check rules (kept minimal — this is the aqleng core; the
// richer fn-predicate / disjunct narrowing lives in production aql):
//
//   - T is a type literal (Data == nil): true iff v's VType matches T
//     and v is not a type literal of a strict supertype. Concrete
//     values pass; type literals at metatype slots compare metatypes.
//   - T is an implicit-map record shape: every key in T must be
//     present in v with a matching type.
//   - T is anything else: structurally unify v with T.
func registerCoreIs(r *Registry) {
	r.RegisterNativeFunc(NativeFunc{
		Name:        "is",
		ForwardArgs: true,
		Signatures: []NativeSig{{
			Args:       []*Type{TAny, TAny},
			BarrierPos: 1,
			Handler: func(args []Value, _ map[string]Value, _ []Value, _ *Registry) ([]Value, error) {
				return []Value{NewBoolean(IsValueOfType(args[1], args[0]))}, nil
			},
			Returns: []*Type{TBoolean},
		}},
	})
}

// IsValueOfType reports whether v satisfies type T. Used by the `is`
// word's handler.
//
// Rules:
//   - T is a typed list `[:T]`: v must be a concrete list and every
//     element must satisfy T (recursive IsValueOfType).
//   - T is a typed map `{:T}`: v must be a concrete map and every
//     value must satisfy T.
//   - T is a record-shape implicit map (`{x:Integer y:String}`):
//     every declared key must be present in v with a matching field
//     type. v must be a concrete map; extra keys in v are ignored.
//   - T is the bare metatype `Type` (Data == nil, VType == Type): true
//     iff v is itself a type — any bare type literal, any structural
//     type body (record shape, typed list/map, disjunct, fn-shape),
//     or any Function / FunctionSignature / Disjunct / Enum value.
//     Concrete scalars / lists / maps and the value `none` are not.
//   - T is any other type literal (Data == nil), including `Function` /
//     `Disjunct` / `Enum` / `FunctionSignature`: v's VType must be a
//     subtype of T's via the type lattice.
//   - T is anything else: structural unification on (v, t).
func IsValueOfType(v, t Value) bool {
	if IsTypedList(t) {
		if !v.VType.Equal(TList) || v.Data == nil {
			return false
		}
		ci, _ := AsChildType(t)
		lst := AsList(v)
		if lst.IsNil() {
			return false
		}
		for i := 0; i < lst.Len(); i++ {
			if !IsValueOfType(lst.Get(i), ci.Child) {
				return false
			}
		}
		return true
	}
	if IsTypedMap(t) {
		if !v.VType.Equal(TMap) || v.Data == nil {
			return false
		}
		ci, _ := AsChildType(t)
		vMap := AsMap(v)
		if vMap == nil {
			return false
		}
		for _, k := range vMap.Keys() {
			vv, _ := vMap.Get(k)
			if !IsValueOfType(vv, ci.Child) {
				return false
			}
		}
		return true
	}
	// Map-as-type — record-shape conformance. Fires for both
	// Implicit (fn-sig pair-syntax) and explicit (`{x:Integer}`)
	// maps. The recursive IsValueOfType handles concrete-as-singleton
	// fields via the Unify fallback when t's field is a literal.
	// Subtypes like RecordTypeInfo / OptionsTypeInfo (whose AsMap
	// returns nil) fall through to Unify below.
	if t.VType.Equal(TMap) && t.Data != nil && AsMap(t) != nil {
		if !v.VType.Equal(TMap) || v.Data == nil {
			return false
		}
		vMap := AsMap(v)
		tMap := AsMap(t)
		if vMap == nil || tMap == nil {
			return false
		}
		for _, k := range tMap.Keys() {
			tv, _ := tMap.Get(k)
			vv, ok := vMap.Get(k)
			if !ok {
				return false
			}
			if !IsValueOfType(vv, tv) {
				return false
			}
		}
		return true
	}
	if t.Data == nil {
		// `v is Type` — the bare metatype: v satisfies it iff v is
		// itself a TYPE — not merely a value whose type would qualify:
		//
		//   - any bare type literal (`Integer`, `List`, `Any`, `Type`,
		//     …, Data == nil) — `Integer is Type`, `List is Type`, … are
		//     all true;
		//   - any structural type body (record shape `{x:Integer}`,
		//     typed list/map `[:T]` / `{:T}`, disjunct, fn-shape, …) and
		//     any Function / Disjunct / Enum / FunctionSignature *value*
		//     (whose VType lives under Type/) — types too;
		//   - a concrete scalar / list / map, and the value `none`, are
		//     NOT types — `5 is Type`, `[1 2 3] is Type`, `none is Type`
		//     are false. Carriers are abstract VALUES, not types.
		//
		// Other Type/-rooted RHS (`Function`, `Disjunct`, `Enum`,
		// `FunctionSignature`, the legacy `ScalarType` / `NodeType` /
		// `ObjectType` metatypes) keep the plain subtype check below, so
		// `fn […] is Function` / `enum […] is Disjunct` still hold.
		if t.VType.Equal(TType) {
			if v.Carrier {
				return false
			}
			return v.Data == nil || IsTypeBody(v) || IsRecordShape(v) || v.VType.Matches(TType)
		}
		// Canonical dispatch site: route through Behavior so custom
		// type semantics (predicate types, dependent scalars, future
		// plugin types) get consulted. Default Behavior delegates to
		// the historical lattice walk.
		return v.Is(t.VType)
	}
	_, ok := Unify(v, t)
	return ok
}

// InstallType is the single kernel entry point for installing a
// named type body (`type Foo body`). Validates the body shape,
// rejects name clashes, and pushes onto the registry's type
// stack. Used by both the eng-internal core `type` word and the
// production aql `type` word in lang/engine. Changes to
// type-installation policy go here, not in a per-surface duplicate.
//
// Body acceptance is broad: a structural type body (IsTypeBody — type
// literal, disjunct, implicit map, typed list/map, ObjectType, …) OR a
// concrete scalar / list / map literal (IsLiteralTypeBody — `type Foo
// 1`, the singleton type whose only inhabitant is 1). The split keeps
// the inspect / fn-shape paths aligned with structural typing while
// letting users name singletons and value-shape types.
//
// When the body is an anonymous ObjectType (from the `object` word),
// binding it under NAME renames it `Object/NAME` (or `<parent>/NAME`
// when it inherits) so `typeof` / `is` report the nominal name.
func InstallType(r *Registry, name string, body Value) error {
	if !IsTypeBody(body) && !IsLiteralTypeBody(body) {
		return &AqlError{
			Code:   "type_error",
			Detail: "type: body must be a type value or literal, got " + body.String(),
		}
	}
	if !IsCapitalisedName(name) {
		return &AqlError{
			Code:   "type_error",
			Detail: "type " + name + ": type names must start with a capital letter",
		}
	}
	if !r.Types.Has(name) {
		if err := ValidateTypeNameParts(name, r.IsKnownPart); err != nil {
			return err
		}
	}
	if r.Lookup(name) != nil {
		return &AqlError{
			Code:   "type_error",
			Detail: "type " + name + ": name clash — already a registered function",
		}
	}
	if r.Defs.Has(name) {
		return &AqlError{
			Code:   "type_error",
			Detail: "type " + name + ": name clash — already a def'd value",
		}
	}
	if IsObjectType(body) {
		info, _ := AsObjectType(body)
		if info.Parent != nil {
			info.Name = info.Parent.Name + "/" + name
		} else {
			info.Name = "Object/" + name
		}
		for _, p := range strings.Split(info.Name, "/") {
			r.RegisterPart(p)
		}
		parentDef := TObject
		if info.Parent != nil && info.Parent.Type != nil {
			parentDef = info.Parent.Type
		}
		def := r.Types.MintType(name, parentDef)
		body = NewObjectType(def, info)
		r.Types.Bind(name, def, body)
	} else {
		r.Types.PushType(name, body)
	}
	for _, p := range strings.Split(name, "/") {
		r.RegisterPart(p)
	}
	return nil
}
