package aqleng

import (
	"strings"
)

// registerCoreType installs the language-fundamental type words:
//
//   type NAME body    — bind a type name (NAME must start [A-Z]) to a
//                       type body (a type literal, disjunct, implicit
//                       map shape, or any value satisfying IsTypeBody).
//                       Pushes onto the type stack — `type Foo Integer;
//                       type Foo String` shadows; `untype Foo` pops.
//
//   untype NAME       — pop the most-recent binding for NAME from the
//                       type stack. Errors if no binding exists.
//
//   typeof v          — return the type of v as a type-literal Value
//                       (e.g. typeof 5 → Integer, typeof Integer →
//                       ScalarType, typeof none → None).
//
//   v is T            — Boolean: does v satisfy type T? Forward sig is
//                       [Any | Any], so `5 is Integer` reads naturally
//                       (T from forward, v from stack).
//
// Mirrors the production aql `type` / `untype` / `typeof` / `is`
// (see aql/internal/engine/native_type.go); these are the foundational
// type-system surface that the richer words (record, table, object,
// make, guard, inspect, …) build on. The richer words are NOT in
// aqleng's core — they layer on top in the production engine.
func registerCoreType(r *Registry) {
	registerCoreTypeWord(r)
	registerCoreUntypeWord(r)
	registerCoreTypeof(r)
	registerCoreIs(r)
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
//   typeof 5            → Integer        (concrete value's exact type)
//   typeof 5.0          → Decimal
//   typeof "x"          → String
//   typeof true         → Boolean
//   typeof none         → None           (None has a single inhabitant)
//   typeof [ 1 2 ]      → List
//   typeof { a:1 }      → Map
//   typeof Integer      → ScalarType     (type literals → their metatype)
//   typeof List         → NodeType
//   typeof Any          → Type
//
// Rules:
//   - For `none` (the unique inhabitant of None), return None itself.
//   - For any other type literal (Data == nil), return its metatype
//     (Type/* — ScalarType / NodeType / ObjectType / Type).
//   - For a concrete value (Data != nil), return its full VType.
//
// typeof's result is itself a type literal so it round-trips: passing
// the result to `is` or chaining `typeof typeof v` produces the
// expected metatype-of-metatype answers.
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

// TypeOf returns the Type of v as a type-literal Value. See
// registerCoreTypeof for the dispatch rules.
func TypeOf(v Value) Value {
	// The VALUE `none` (Data != nil, VType == TNone) → None type literal.
	if v.IsNone() {
		return NewTypeLiteral(TNone)
	}
	// A type literal (Data == nil, includes the None type literal) →
	// metatype.
	if v.Data == nil {
		return NewTypeLiteral(MetatypeFor(v.VType))
	}
	// An implicit-map record shape (every entry's value is itself a
	// type body — type literal or nested shape) is a Node-family TYPE,
	// not a concrete map. Report its metatype (NodeType) so user code
	// can branch on shape vs concrete map without inspecting the data.
	if IsRecordShape(v) {
		return NewTypeLiteral(MetatypeFor(v.VType))
	}
	// Concrete value — its exact VType.
	return NewTypeLiteral(v.VType)
}

// IsRecordShape reports whether v is a non-empty implicit-map type
// body — `{x:Integer y:String}` and friends. Every entry's value
// must itself be a type body (a type literal or a nested record
// shape). The empty implicit map `{}` is treated as a concrete map,
// not a shape, so `typeof { } → Map`.
func IsRecordShape(v Value) bool {
	if !v.IsImplicitMap() {
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
//   - T is a record-shape implicit map (`{x:Integer y:String}`):
//     every declared key must be present in v with a matching field
//     type. v must be a concrete map; extra keys in v are ignored.
//   - T is a type literal (Data == nil): v's VType must match T's
//     via the type lattice. Covers scalar / metatype / None cases.
//   - T is anything else: structural unification on (v, t).
func IsValueOfType(v, t Value) bool {
	if t.IsImplicitMap() {
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
