package eng

import (
	"strings"
)

// This file owns the algorithms behind the type-system words —
// PathOf, TypeOf, IsRecordShape, IsValueOfType, InstallType — plus
// the helper rules that `is` / `typeof` / `pathof` / `enum` build on.
// The matching word registrations live in lang/go/engine/native_type.go;
// the engspec spec-runner installs minimal kernel-side fixtures.

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
//   - For a concrete value (Data != nil), return its full Parent.
//
// typeof's result is itself a type literal so it round-trips: passing
// the result to `is` or chaining `typeof typeof v` (always `Type`)
// produces the expected answers.
// PathOf returns the ancestry path of the type T (a Type literal, or
// any value whose Parent is a Type subtype — e.g. a Function/Disjunct
// value) as a List of Type literals, root first, leaf last (the
// declared signature contract is [:Type]; the runtime value is a
// regular List so that `is [literal-list]` comparisons against an
// untyped list template work):
//
//	pathof Integer          → [Scalar Number Integer]
//	pathof ProperString     → [Scalar String ProperString]
//	pathof List             → [Node List]
//	pathof Function         → [Type Function]
//	pathof Enum             → [Type Disjunct Enum]
//	pathof Type             → [Type]              (Type has no ancestors)
//	pathof None             → [None]
//
// Exported so lang's `pathof` registration (lang/go/engine/native_type.go)
// can wire dispatch into it without forking the algorithm.
func PathOf(t Value) Value {
	// Walk the ancestry root-first. A bare type literal IS its leaf
	// node, so start from t itself; any other value (a Function /
	// Disjunct / Enum value) contributes the ancestry of its type.
	start := t.Parent
	if t.Data == nil && !t.Carrier {
		start = &t
	}
	var chain []*Type
	for d := start; d != nil; d = d.Parent {
		// Any is the universal lattice top; skip it as an ANCESTOR
		// so paths stay [Scalar Number Integer], not [Any Scalar
		// Number Integer]. When `pathof Any` is called directly the
		// chain is still [Any] — the skip only triggers once we've
		// already accumulated a leaf.
		if d.Equal(TAny) && len(chain) > 0 {
			break
		}
		chain = append([]*Type{d}, chain...)
	}
	elems := make([]Value, 0, len(chain))
	for _, d := range chain {
		elems = append(elems, NewTypeLiteral(d))
	}
	return NewList(elems)
}

// TypeOf returns the type of v — uniformly its Parent, expressed as
// a type-literal Value. After the type/value merge every value is a
// lattice node, so typeof is a single Parent hop, climbing the
// unified lattice that has Any at the top of the main hierarchy:
//
//	typeof 5        → Integer
//	typeof Integer  → Number
//	typeof Number   → Scalar
//	typeof Scalar   → Any        (Scalar's lattice parent is Any)
//	typeof Any      → Any        (saturates — top of the main hierarchy)
//	typeof none     → None       (none is None's sole inhabitant)
//	typeof None     → None       (None is a degenerate root — saturates)
//	typeof Never    → Never      (Never is a degenerate root — saturates)
func TypeOf(v Value) Value {
	if v.Parent == nil {
		return v
	}
	return NewTypeLiteral(v.Parent)
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
	if !v.Parent.Equal(TMap) || v.Data == nil {
		return false
	}
	m, _ := AsMap(v)
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
//   - T is the bare metatype `Type` (Data == nil, Parent == Type): true
//     iff v is itself a type — any bare type literal, any structural
//     type body (record shape, typed list/map, disjunct, fn-shape),
//     or any Function / FunctionSignature / Disjunct / Enum value.
//     Concrete scalars / lists / maps and the value `none` are not.
//   - T is any other type literal (Data == nil), including `Function` /
//     `Disjunct` / `Enum` / `FunctionSignature`: v's Parent must be a
//     subtype of T's via the type lattice.
//   - T is anything else: structural unification on (v, t).
func IsValueOfType(v, t Value) bool {
	if IsTypedList(t) {
		if !v.Parent.Equal(TList) || v.Data == nil {
			return false
		}
		ci, _ := AsChildType(t)
		lst, _ := AsList(v)
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
		if !v.Parent.Equal(TMap) || v.Data == nil {
			return false
		}
		ci, _ := AsChildType(t)
		vMap, _ := AsMap(v)
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
	if _tMap, _tErr := AsMap(t); t.Parent.Equal(TMap) && t.Data != nil && _tErr == nil && _tMap != nil {
		if !v.Parent.Equal(TMap) || v.Data == nil {
			return false
		}
		vMap, _ := AsMap(v)
		tMap, _ := AsMap(t)
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
		//     (whose Parent lives under Type/) — types too;
		//   - a concrete scalar / list / map, and the value `none`, are
		//     NOT types — `5 is Type`, `[1 2 3] is Type`, `none is Type`
		//     are false. Carriers are abstract VALUES, not types.
		//
		// Other Type/-rooted RHS (`Function`, `Disjunct`, `Enum`,
		// `FunctionSignature`, the legacy `ScalarType` / `NodeType` /
		// `ObjectType` metatypes) keep the plain subtype check below, so
		// `fn […] is Function` / `enum […] is Disjunct` still hold.
		if t.Equal(TType) {
			if v.Carrier {
				return false
			}
			return v.Data == nil || IsTypeBody(v) || IsRecordShape(v) || v.Parent.Matches(TType)
		}
		// Canonical dispatch site: route through Behavior so custom
		// type semantics (predicate types, dependent scalars, future
		// plugin types) get consulted. Default Behavior delegates to
		// the historical lattice walk.
		return v.Is(&t)
	}
	_, ok := Unify(v, t)
	return ok
}

// InstallType is the single kernel entry point for installing a
// named type body (`def Foo body`). Validates the body shape,
// rejects name clashes, and pushes onto the registry's type
// stack. Used by both the eng-internal core `def` word and the
// production aql `def` word in lang/go/engine. Changes to
// type-installation policy go here, not in a per-surface duplicate.
//
// Body acceptance is broad: a structural type body (IsTypeBody — type
// literal, disjunct, implicit map, typed list/map, ObjectType, …) OR a
// concrete scalar / list / map literal (IsLiteralTypeBody — `def Foo
// 1`, the singleton type whose only inhabitant is 1). The split keeps
// the inspect / fn-shape paths aligned with structural typing while
// letting users name singletons and value-shape types.
//
// When the body is an anonymous ObjectType (from `refine Object {…}`),
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
	if !r.Defs.IsType(name) {
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
	if r.Defs.Has(name) && !r.Defs.IsType(name) {
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
		r.Defs.PushType(name, def, body)
	} else if inputT := PredicateInputType(body); inputT != nil {
		// Predicate type with a concrete input type: mint the *Type
		// parented at the input rather than at TFnDef so values
		// rewrapped by the typed-bind path inherit input-side
		// capabilities (Integer's Number-branch Comparer, etc.)
		// through the lattice walk. Predicate types declared with
		// `Any` input (the historical `fn [x:Any Any […]]` pattern)
		// fall through to the regular PushType path — they remain
		// gates, not dispatch categories.
		def := r.Types.MintType(name, inputT)
		// Attach a Unifier so the predicate runs at every Unify call
		// site (signature matching, options fields, record fields,
		// `make` constraints, the `unify` word). Without this, Unify
		// would take the lattice subtype path and admit any
		// base-compatible value without checking the predicate.
		installPredicateUnifier(def, body, r, name)
		r.Defs.PushType(name, def, body)
	} else if IsRefinePrefab(body) {
		// `def Foo refine Integer` route: `refineBareHandler` minted
		// an anonymous refine prefab (MintRefinePrefab) and returned
		// its type literal. Rename the lattice node and bind the
		// renamed Foo literal as the body so resolving `Foo` pushes
		// the new subtype node (Parent = base, Rank in the external
		// band) rather than the original input type literal.
		def := r.Types.LookupByID(body.ID)
		if def == nil {
			return &AqlError{
				Code:   "type_error",
				Detail: "type " + name + ": refine prefab missing from lattice",
			}
		}
		def.Name = name
		body.Name = name
		r.Defs.PushType(name, def, body)
	} else {
		// A bare type-literal body IS the parent type after the
		// type/value merge; structural/singleton bodies parent at
		// their container type (Map / List / Integer / …). For a
		// bare type-literal body, route through CanonicalType so the
		// minted subtype's lattice Parent is the canonical *Type and
		// not a non-canonical copy.
		parent := body.Parent
		if body.Data == nil {
			parent = CanonicalType(r, &body)
		}
		def := r.Types.MintType(name, parent)
		r.Defs.PushType(name, def, body)
	}
	for _, p := range strings.Split(name, "/") {
		r.RegisterPart(p)
	}
	return nil
}
