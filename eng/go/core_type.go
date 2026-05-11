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
// (see lang/internal/engine/native_type.go); these are the foundational
// type-system surface that the richer words (record, table, object,
// make, guard, inspect, …) build on. The richer words are NOT in
// aqleng's core — they layer on top in the production engine.
func registerCoreType(r *Registry) {
	registerCoreTypeWord(r)
	registerCoreUntypeWord(r)
	registerCoreTypeof(r)
	registerCoreTypePathOf(r)
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
		Name:              "enum",
		ForwardPrecedence: true,
		Signatures: []NativeSig{{
			Args:       []Type{TList},
			NoEvalArgs: map[int]bool{0: true},
			Handler: func(args []Value, _ map[string]Value, _ []Value, _ *Registry) ([]Value, error) {
				list := args[0]
				if list.Data == nil {
					return nil, &AqlError{Code: "type_error", Detail: "enum: argument must be a concrete list"}
				}
				var childType Value
				hasChild := false
				if list.IsTypedList() {
					ci, _ := list.AsChildType()
					childType = ci.Child
					hasChild = childType.VType.Parts != nil
				}
				elems := list.AsList()
				alts := make([]Value, 0, elems.Len())
				for i := 0; i < elems.Len(); i++ {
					e := elems.Get(i)
					// Word → Atom conversion: enum members are
					// typically named, and in word context the parser
					// produces Words. Convert here so users don't need
					// to wrap each element in `quote`.
					if e.IsWord() {
						w, _ := e.AsWord()
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
			Returns: []Type{TEnum},
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
		Name:              "type",
		ForwardPrecedence: true,
		Signatures: []NativeSig{{
			Args:       []Type{TWord, TAny},
			NoEvalArgs: map[int]bool{1: true},
			Handler: func(args []Value, _ map[string]Value, _ []Value, reg *Registry) ([]Value, error) {
				w, _ := args[0].AsWord()
				body := args[1]
				if err := installType(reg, w.Name, body); err != nil {
					return nil, err
				}
				return nil, nil
			},
			Returns: []Type{},
		}},
	})
}

// registerCoreUntypeWord installs `untype NAME`. Name arrives as a Word.
func registerCoreUntypeWord(r *Registry) {
	r.RegisterNativeFunc(NativeFunc{
		Name:              "untype",
		ForwardPrecedence: true,
		Signatures: []NativeSig{{
			Args: []Type{TWord},
			Handler: func(args []Value, _ map[string]Value, _ []Value, reg *Registry) ([]Value, error) {
				w, _ := args[0].AsWord()
				if !IsCapitalisedName(w.Name) {
					return nil, &AqlError{
						Code:   "type_error",
						Detail: "untype " + w.Name + ": type names must start with a capital letter",
					}
				}
				if !reg.PopType(w.Name) {
					return nil, &AqlError{
						Code:   "type_error",
						Detail: "untype " + w.Name + ": no such type binding",
					}
				}
				return nil, nil
			},
			Returns: []Type{},
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
		Name:              "typeof",
		ForwardPrecedence: true,
		Signatures: []NativeSig{{
			Args: []Type{TAny},
			Handler: func(args []Value, _ map[string]Value, _ []Value, _ *Registry) ([]Value, error) {
				return []Value{TypeOf(args[0])}, nil
			},
			Returns: []Type{TType},
		}},
	})
}

// registerCoreTypePathOf installs `typepathof v`. Returns a List of
// Type literals — the ancestry path of `typeof v`, from the top-level
// root down to the leaf type:
//
//	typepathof "a"     → [Scalar String ProperString]
//	typepathof 5       → [Scalar Number Integer]
//	typepathof 5.0     → [Scalar Number Decimal]
//	typepathof true    → [Scalar Boolean]
//	typepathof [ 1 2 ] → [Node List]
//	typepathof { a:1 } → [Node Map]
//	typepathof none    → [None]
//	typepathof Integer → [Type]              (every type literal collapses to [Type])
//
// The last element always equals `typeof v`. Each element is a Type
// literal naming a progressively-deeper type along the path, so they
// satisfy `is` against the corresponding type name.
func registerCoreTypePathOf(r *Registry) {
	r.RegisterNativeFunc(NativeFunc{
		Name:              "typepathof",
		ForwardPrecedence: true,
		Signatures: []NativeSig{{
			Args: []Type{TAny},
			Handler: func(args []Value, _ map[string]Value, _ []Value, _ *Registry) ([]Value, error) {
				return []Value{TypePathOf(args[0])}, nil
			},
			Returns: []Type{TList},
		}},
	})
}

// TypePathOf returns the ancestry path of `typeof v` as a List of
// Type literals (root first, leaf last). See registerCoreTypePathOf.
func TypePathOf(v Value) Value {
	parts := TypeOf(v).VType.Parts
	elems := make([]Value, 0, len(parts))
	for i := 1; i <= len(parts); i++ {
		seg := append([]string(nil), parts[:i]...)
		pt := Type{Parts: seg}
		fullPath := strings.Join(seg, "/")
		if num, ok := BuiltinTypeIDs[fullPath]; ok {
			pt.ID = FormatFixedTypeID(fullPath, num)
		}
		elems = append(elems, NewTypeLiteral(pt))
	}
	return NewList(elems)
}

// TypeOf returns the Type of v as a type-literal Value. See
// registerCoreTypeof for the dispatch rules.
func TypeOf(v Value) Value {
	// The VALUE `none` (Data != nil, VType == TNone) → None type literal.
	if v.IsNone() {
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
	if v.IsTypedList() || v.IsTypedMap() {
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
	m := v.AsMap()
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
		Name:              "is",
		ForwardPrecedence: true,
		Signatures: []NativeSig{{
			Args:       []Type{TAny, TAny},
			BarrierPos: 1,
			Handler: func(args []Value, _ map[string]Value, _ []Value, _ *Registry) ([]Value, error) {
				return []Value{NewBoolean(IsValueOfType(args[1], args[0]))}, nil
			},
			Returns: []Type{TBoolean},
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
//   - T is a type literal (Data == nil): v's VType must match T's
//     via the type lattice. Covers scalar / metatype / None cases.
//   - T is anything else: structural unification on (v, t).
func IsValueOfType(v, t Value) bool {
	if t.IsTypedList() {
		if !v.VType.Equal(TList) || v.Data == nil {
			return false
		}
		ci, _ := t.AsChildType()
		lst := v.AsList()
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
	if t.IsTypedMap() {
		if !v.VType.Equal(TMap) || v.Data == nil {
			return false
		}
		ci, _ := t.AsChildType()
		vMap := v.AsMap()
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
	if t.VType.Equal(TMap) && t.Data != nil && t.AsMap() != nil {
		if !v.VType.Equal(TMap) || v.Data == nil {
			return false
		}
		vMap := v.AsMap()
		tMap := t.AsMap()
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
		return v.VType.Matches(t.VType)
	}
	_, ok := Unify(v, t)
	return ok
}

// installType validates a (name, body) pair and pushes the body onto
// the type stack. Mirrors production aql validateAndInstallType minus
// the ObjectType naming (ObjectType bodies are constructed by the
// production-only `object` word, which isn't in aqleng's core).
//
// Body acceptance is broad: a structural type body (IsTypeBody — type
// literal, disjunct, implicit map, typed list/map, …) OR a concrete
// scalar / list / map literal (IsLiteralTypeBody — `type Foo 1`, the
// singleton type whose only inhabitant is 1). The split keeps the
// inspect / fn-shape paths aligned with structural typing while
// letting users name singletons and value-shape types.
func installType(r *Registry, name string, body Value) error {
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
	if !r.HasType(name) {
		if err := ValidateTypeNameParts(name, r.KnownTypeParts); err != nil {
			return err
		}
	}
	if r.Lookup(name) != nil {
		return &AqlError{
			Code:   "type_error",
			Detail: "type " + name + ": name clash — already a registered function",
		}
	}
	if r.HasDef(name) {
		return &AqlError{
			Code:   "type_error",
			Detail: "type " + name + ": name clash — already a def'd value",
		}
	}
	r.PushType(name, body)
	for _, p := range strings.Split(name, "/") {
		r.KnownTypeParts[p] = true
	}
	return nil
}
