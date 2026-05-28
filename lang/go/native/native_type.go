package native

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/aql-lang/aql/eng/go"
)

// typeNatives covers the type-system words: refine, pathof, enum,
// typeof, is, teq, tpartial, guard, base, tor, tand, tany, tall,
// convert. New type ops follow the `t`-prefix convention — see
// lang/doc/design/TYPE-OPERATIONS.0.md.
//
// `Resource` and `Entity` (the builtin object types) are NOT installed
// via NativeFunc — they are user-typed values pushed onto the type
// stack. `installResourceTypes` handles those during Register.
var typeNatives = []NativeFunc{
	{
		// refine is the uniform type constructor — see
		// lang/doc/design/TYPE-UNIFORM.0.md. `refine BaseType arg`
		// builds a (sub)type:
		//   refine Object {fields}     → object type
		//   refine <objtype> {fields}  → object subtype (inheritance)
		//   refine Record [a:T b:U]    → record type (list of pairs)
		//   refine Table  (refine Record …) → table type
		//   refine BaseType            → a bare nominal subtype, no
		//                                added structure (the 1-arg form)
		//
		// Two signatures: a 2-arg structural form and a 1-arg bare form.
		// Because the 1-arg signature lets `refine` succeed with a
		// single argument, the word never defers to take a body from the
		// stack — so a nested constructor must be parenthesised:
		// `refine Table (refine Record […])`, not `refine Table refine
		// Record […]`. The 2-arg body is always a Node (a map or list
		// literal, or a record/object type value), typed TNode so the
		// matcher falls through to the 1-arg form when a non-Node token
		// (a following `def` / `behave` / `;`) comes next.
		Name: "refine",

		Signatures: []NativeSig{
			{
				Args:           []*Type{TAny, TNode},
				Handler:        refineHandler,
				Returns:        []*Type{TType},
				RunInCheckMode: true, BarrierPos: -1,
			},
			{
				Args:           []*Type{TAny},
				Handler:        refineBareHandler,
				Returns:        []*Type{TType},
				RunInCheckMode: true, BarrierPos: -1,
			},
		},
	},
	{
		Name: "pathof",

		Signatures: []NativeSig{{
			Args:     []*Type{TAny},
			TypeArgs: map[int]bool{0: true},
			Handler: func(args []Value, _ map[string]Value, _ []Value, _ *Registry) ([]Value, error) {
				return []Value{eng.PathOf(args[0])}, nil
			},
			Returns: []*Type{TList}, BarrierPos: -1,
		}},
	},
	{
		Name: "enum",

		Signatures: []NativeSig{{
			Args:       []*Type{TList},
			NoEvalArgs: map[int]bool{0: true},
			Handler:    enumHandler,
			Returns:    []*Type{TEnum}, BarrierPos: -1,
		}},
	},
	{
		Name: "typeof",

		Signatures: []NativeSig{{
			Args:    []*Type{TAny},
			Handler: typeofHandler,
			Returns: []*Type{TType}, BarrierPos: -1,
		}},
	},
	{
		Name: "is",

		Signatures: []NativeSig{{
			Args:       []*Type{TAny, TAny},
			BarrierPos: 1,
			Handler:    isHandler,
			Returns:    []*Type{TBoolean},
		}},
	},
	{
		Name: "teq",

		Signatures: []NativeSig{{
			Args:       []*Type{TAny, TAny},
			BarrierPos: 1,
			Handler:    teqHandler,
			Returns:    []*Type{TBoolean},
		}},
	},
	{
		Name: "tpartial",

		Signatures: []NativeSig{{
			Args:    []*Type{TAny},
			Handler: tpartialHandler,
			Returns: []*Type{TType}, BarrierPos: -1,
		}},
	},
	{
		Name: "guard",

		Signatures: []NativeSig{{
			Args:       []*Type{TAny, TBoolean},
			BarrierPos: 1,
			Handler:    guardHandler,
			Returns:    []*Type{TAny},
		}}},
	{
		Name: "base",

		Signatures: []NativeSig{{
			Args:      []*Type{TAny},
			Handler:   baseHandler,
			ReturnsFn: ReturnsIdentity(0), BarrierPos: -1,
		}},
	},
	// `tor` (disjunct union) and `tand` (intersection) — type-level
	// connective words. Algorithm primitives live in eng
	// (eng.TorHandler / eng.TandHandler / eng.TandValues); the
	// registrations here own the names and dispatch wiring.
	{
		Name: "tor",

		Signatures: []NativeSig{{
			Args:       []*Type{TAny, TAny},
			BarrierPos: 1,
			Handler:    eng.TorHandler,
			ReturnsFn:  eng.TorReturnsFn,
		}},
	},
	{
		Name: "tand",

		Signatures: []NativeSig{{
			Args:       []*Type{TAny, TAny},
			BarrierPos: 1,
			Handler:    eng.TandHandler,
			Returns:    []*Type{TAny},
		}},
	},
	{
		Name: "tany",

		Signatures: []NativeSig{
			{Args: []*Type{TList}, Handler: tanyHandler, Returns: []*Type{TAny}, BarrierPos: -1},
		},
	},
	{
		Name: "tall",

		Signatures: []NativeSig{
			{Args: []*Type{TList}, Handler: tallHandler, Returns: []*Type{TAny}, BarrierPos: -1},
		},
	},
	{
		Name: "convert",

		Signatures: []NativeSig{
			{
				Args:      []*Type{TScalar, TMap, TScalar},
				TypeArgs:  map[int]bool{0: true},
				Patterns:  map[int]Value{1: convertOptsPattern()},
				Handler:   convert3Handler,
				ReturnsFn: ReturnsIdentity(0), BarrierPos: -1,
			},
			{
				Args:      []*Type{TScalar, TScalar},
				TypeArgs:  map[int]bool{0: true},
				Handler:   convert2Handler,
				ReturnsFn: ReturnsIdentity(0), BarrierPos: -1,
			},
		},
	},
}

// installResourceTypes pushes the builtin Resource and Entity object
// types onto the type stack. Called once during engine.Register.
//
//   - Object/Resource has field kind:String
//   - Object/Resource/Entity inherits kind from Resource and adds
//     spec:String, entity:String
//
// These are registered via InstallDef so they get proper handler
// resolution and can be referenced by name in AQL code (e.g. make
// Entity {...}).
func installResourceTypes(r *Registry) {
	resourceFields := NewOrderedMap()
	resourceFields.Set("kind", NewTypeLiteral(TString))

	resourceInfo := ObjectTypeInfo{
		Fields: resourceFields,
		Parent: nil,
		ID:     BuiltinIDForPath("Ideal/Object/Resource"),
	}

	InstallDef(r, "Resource", NewObjectType(TResource, resourceInfo))

	resourceVal, _ := r.Defs.Top("Resource")
	installedResource, _ := AsObjectType(resourceVal)

	entityFields := NewOrderedMap()
	entityFields.Set("spec", NewTypeLiteral(TString))
	entityFields.Set("entity", NewTypeLiteral(TString))

	entityInfo := ObjectTypeInfo{
		Fields: entityFields,
		Parent: &installedResource,
		ID:     BuiltinIDForPath("Ideal/Object/Resource/Entity"),
	}

	InstallDef(r, "Entity", NewObjectType(TResourceEntity, entityInfo))
}

// ---- table ----

func tableHandler(args []Value, _ map[string]Value, _ []Value, _ *Registry) ([]Value, error) {
	target := args[0]
	if !IsRecordType(target) {
		return nil, fmt.Errorf("table: argument must be a record type, got %s", target.String())
	}
	_as0, _ := AsRecordType(target)
	return []Value{NewTableType(_as0)}, nil
}

// ---- refine (the type constructor) ----

// refineHandler implements `refine BaseType arg`, the uniform type
// constructor. It does not branch on the base type itself — dispatch
// is data-driven through the Ideal registry (r.Ideals): whichever
// type-kind claims the base value supplies the construction logic.
// See lang/doc/design/IDEAL.0.md. `refine` does not bind — pair it
// with `def` (`def Foo (refine …)`).
func refineHandler(args []Value, _ map[string]Value, _ []Value, r *Registry) ([]Value, error) {
	base := args[0]
	arg := args[1]
	ideal := r.Ideals.For(base)
	if ideal == nil {
		// Distinguish a disabled kind from an unknown base.
		if m := r.Ideals.Match(base); m != nil {
			return nil, r.AqlError("type_error",
				fmt.Sprintf("refine: the %s type-kind is not available in this registry", m.Name),
				"refine")
		}
		return nil, r.AqlError("type_error",
			fmt.Sprintf("refine: base must be Object, Record, Table, or an object type, got %s", base.String()),
			"refine")
	}
	if ideal.Construct == nil {
		return nil, r.AqlError("type_error",
			fmt.Sprintf("refine: the %s type-kind cannot be constructed with `refine`", ideal.Name),
			"refine")
	}
	return ideal.Construct(base, arg, r)
}

// refineBareHandler implements the 1-arg `refine BaseType` form — a
// bare nominal subtype of BaseType with no added structure. It
// validates that the argument is a type and returns it unchanged; the
// paired `def Name` then mints a fresh subtype parented at BaseType
// (InstallType → MintType). `def Foo refine List` thus produces a
// distinct List subtype that can serve as a dispatch surface for
// `behave` — see lang/doc/design/TYPE-UNIFORM.0.md.
func refineBareHandler(args []Value, _ map[string]Value, _ []Value, r *Registry) ([]Value, error) {
	base := args[0]
	if !IsTypeBody(base) {
		return nil, r.AqlError("type_error",
			fmt.Sprintf("refine: argument must be a type, got %s", base.String()),
			"refine")
	}
	// Bare type-literal base: mint an anonymous user subtype now and
	// return its type literal. The paired `def Foo` (via InstallType)
	// renames the anonymous lattice node to "Foo". This split
	// distinguishes the subtype path (`def Foo refine Integer`) from
	// the alias path (`def Foo Integer`, where the body remains the
	// input type literal verbatim) — without this differentiation the
	// two surfaces would be indistinguishable downstream.
	if base.Data == nil && !base.Carrier {
		// Mint the refine prefab against the canonical lattice node
		// for base, so any user-installed Behavior on base (via
		// `behave`) propagates to the LCA walk for sibling subtypes
		// downstream. The prefab carries no Name; the paired `def`
		// recognises it (eng.IsRefinePrefab) and renames-and-binds.
		anon := r.Types.MintRefinePrefab(CanonicalType(r, &base))
		return []Value{NewTypeLiteral(anon)}, nil
	}
	return []Value{base}, nil
}

// installIdeals fills in the type-level constructor (Ideal.Construct)
// on the kernel Ideals. The descriptors — Name, the Accepts dispatch
// predicate, and the value-level Instantiate — are registered by the
// eng kernel (registerKernelIdeals); type construction additionally
// reuses the surface object/record/table handlers, wired here. See
// lang/doc/design/IDEAL.0.md.
func installIdeals(r *Registry) {
	if obj := r.Ideals.Get("Object"); obj != nil {
		obj.Construct = func(base, arg Value, r *Registry) ([]Value, error) {
			// A bare Object literal builds a fresh object type; an
			// existing object type builds a subtype of it.
			if IsObjectType(base) {
				return objectWithParentHandler([]Value{arg, base}, nil, nil, r)
			}
			return objectHandler([]Value{arg}, nil, nil, r)
		}
	}
	if rec := r.Ideals.Get("Record"); rec != nil {
		rec.Construct = func(base, arg Value, r *Registry) ([]Value, error) {
			// Records have no subtyping — only the bare Record literal
			// is a valid construction base.
			if base.Data != nil {
				return nil, r.AqlError("type_error",
					"refine: a record type has no subtyping — construct a Record from the bare Record literal",
					"refine")
			}
			// A record takes a LIST of field pairs — field order is
			// part of a record type's identity.
			if !arg.Parent.Equal(TList) {
				return nil, r.AqlError("type_error",
					"refine Record: a record takes a list of field pairs, e.g. [a:Integer b:String]",
					"refine")
			}
			return recordHandler([]Value{arg}, nil, nil, r)
		}
	}
	if tbl := r.Ideals.Get("Table"); tbl != nil {
		tbl.Construct = func(base, arg Value, r *Registry) ([]Value, error) {
			if base.Data != nil {
				return nil, r.AqlError("type_error",
					"refine: a table type has no subtyping — construct a Table from the bare Table literal",
					"refine")
			}
			return tableHandler([]Value{arg}, nil, nil, r)
		}
	}
}

// ---- enum ----

// enumHandler — `enum [a b c]` builds a fixed-enumeration type
// (Enum, a subtype of Disjunct) whose alternatives are the list's
// elements. Words become Atoms automatically so `enum [red green
// blue]` doesn't require quoting. When the list carries a child-type
// constraint (`[ :T a b c]`), each element is validated against T
// before being added.
func enumHandler(args []Value, _ map[string]Value, _ []Value, _ *Registry) ([]Value, error) {
	list := args[0]
	if list.Data == nil {
		return nil, &AqlError{Code: "type_error", Detail: "enum: argument must be a concrete list"}
	}
	var childType Value
	hasChild := false
	if IsTypedList(list) {
		ci, _ := AsChildType(list)
		childType = ci.Child
		hasChild = childType.Parent != nil
	}
	elems, _ := AsList(list)
	alts := make([]Value, 0, elems.Len())
	for i := 0; i < elems.Len(); i++ {
		e := elems.Get(i)
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
}

// ---- typeof ----

func typeofHandler(args []Value, _ map[string]Value, _ []Value, _ *Registry) ([]Value, error) {
	// Delegate to the canonical aqleng implementation, which returns
	// a Type literal: concrete value → exact Parent; type literal →
	// its metatype (Type); implicit-map record shape → its metatype;
	// the value `none` (unique inhabitant of None) → None.
	return []Value{TypeOf(args[0])}, nil
}

// ---- is ----

func isHandler(args []Value, _ map[string]Value, _ []Value, r *Registry) ([]Value, error) {
	a, b := args[1], args[0]
	if b.Parent.Equal(TFnUndef) && IsAtom(a) {
		name, _ := AsAtom(a)
		if top, ok := r.Defs.Top(name); ok {
			if top.Parent.Equal(TFnDef) || top.Parent.Equal(TFunction) {
				a = top
			}
		}
	}
	if b.Parent.Equal(TFnDef) || b.Parent.Equal(TFunction) {
		_, matched, err := r.RunPredicate(b, a)
		if err != nil {
			return []Value{NewBoolean(false)}, nil
		}
		return []Value{NewBoolean(matched)}, nil
	}
	if b.Data == nil && !b.Carrier {
		// b is a type literal; its denoted lattice node is &b (a
		// type literal is a by-value copy of its node). Post the
		// Any-root unification TType.Parent == TAny, so the old
		// `b.Parent.Root() == TType` check no longer identifies the
		// Type/-hierarchy — we test the node itself directly.
		bNode := &b
		if bNode.Equal(TType) {
			// `v is Type` — v must be a TYPE: a bare type literal, a
			// structural type body (record shape, typed list/map,
			// disjunct, fn-shape), or a Function / Disjunct / Enum /
			// FunctionSignature value. Concrete scalars / lists / maps
			// and the value `none` are not types; carriers are abstract
			// values, not types.
			if a.Carrier {
				return []Value{NewBoolean(false)}, nil
			}
			return []Value{NewBoolean(a.Data == nil || IsTypeBody(a) || IsRecordShape(a) || a.Parent.Matches(TType))}, nil
		}
		if bNode.Matches(TType) {
			// Type/-rooted subtype RHS (`Function` / `Disjunct` / `Enum`
			// / `FunctionSignature`): plain subtype check on the
			// value's Parent.
			return []Value{NewBoolean(a.Parent.Matches(bNode))}, nil
		}
		// Both sides are bare type literals: the question is purely
		// lattice subtyping. Settle directly via IsSubtypeOf rather
		// than via Unify, whose List/Map/DepScalar/FnDef branches
		// short-circuit family relationships and would reject a
		// user-minted subtype (e.g. `def Foo refine List`) against
		// its base family literal.
		if a.Data == nil && !a.Carrier {
			aNode := &a
			return []Value{NewBoolean(aNode.Equal(bNode) || aNode.IsSubtypeOf(bNode))}, nil
		}
	}
	unified, ok := eng.UnifyR(a, b, r)
	if !ok {
		return []Value{NewBoolean(false)}, nil
	}
	resolved := ResolveWordsDeep(a)
	if !unified.Parent.Equal(resolved.Parent) {
		return []Value{NewBoolean(false)}, nil
	}
	if !ValuesEqual(unified, resolved) {
		return []Value{NewBoolean(false)}, nil
	}
	return []Value{NewBoolean(true)}, nil
}

// ---- teq ----

// teqHandler implements strict type equality. Both args must be
// IsTypeBody; otherwise return false. Bare type literals compare by
// lattice node Equal (ID identity), structural type bodies (record /
// disjunct / object / etc.) compare via ValuesEqual. Distinct from
// `is`, which is subtype-membership and is asymmetric on its RHS
// (`5 is Integer` true, `Integer is 5` false). `teq` is symmetric and
// rejects non-type values from either side.
func teqHandler(args []Value, _ map[string]Value, _ []Value, _ *Registry) ([]Value, error) {
	a, b := args[1], args[0]
	if !IsTypeBody(a) || !IsTypeBody(b) {
		return []Value{NewBoolean(false)}, nil
	}
	if a.Data == nil && !a.Carrier && b.Data == nil && !b.Carrier {
		aNode := &a
		bNode := &b
		return []Value{NewBoolean(aNode.Equal(bNode))}, nil
	}
	return []Value{NewBoolean(ValuesEqual(a, b))}, nil
}

// ---- tpartial ----

// tpartialHandler wraps every field of a Record or Object type in
// `T | None`. Idempotent: a field whose value already includes None
// is left unchanged. For Object types, inherited fields are flattened
// into the result's own field map and the result is registered as a
// fresh anonymous Object type (lattice parent: Object root) — the
// partial is NOT a subtype of the input because AQL's lattice runs
// the other way (a child requires more, not less).
func tpartialHandler(args []Value, _ map[string]Value, _ []Value, r *Registry) ([]Value, error) {
	t := args[0]
	switch {
	case IsRecordType(t):
		rec, _ := AsRecordType(t)
		return []Value{NewRecordType(partializeFields(rec.Fields))}, nil
	case IsObjectType(t):
		info, _ := AsObjectType(t)
		newFields := partializeFields(info.AllFields())
		id := GenerateObjectTypeID()
		newInfo := ObjectTypeInfo{
			Fields: newFields,
			Parent: nil,
			ID:     id,
		}
		def := r.Types.MintType(id, TObject)
		return []Value{NewObjectType(def, newInfo)}, nil
	default:
		return nil, r.AqlError("type_error",
			fmt.Sprintf("tpartial: argument must be a Record or Object type, got %s", t.String()),
			"tpartial")
	}
}

func partializeFields(fields *OrderedMap) *OrderedMap {
	result := NewOrderedMap()
	for _, k := range fields.Keys() {
		ft, _ := fields.Get(k)
		result.Set(k, makeOptionalType(ft))
	}
	return result
}

// makeOptionalType returns `t | None` as a Disjunct, or `t` unchanged
// if t already includes None as an alternative (or IS None).
func makeOptionalType(t Value) Value {
	if IsNoneShape(t) {
		return t
	}
	alts := FlattenDisjunctAlts(t)
	for _, alt := range alts {
		if IsNoneShape(alt) {
			return t
		}
	}
	alts = append(alts, NewTypeLiteral(TNone))
	simplified := SimplifyDisjunctAlts(alts)
	if len(simplified) == 1 {
		return simplified[0]
	}
	return NewDisjunct(simplified)
}

// ---- guard ----

func guardHandler(args []Value, _ map[string]Value, _ []Value, _ *Registry) ([]Value, error) {
	val := args[0]
	cond, err := args[1].AsConcreteBoolean()
	if err != nil {
		return nil, fmt.Errorf("guard: condition must be Boolean, got %s", args[1].Parent.String())
	}
	if cond {
		return []Value{val}, nil
	}
	return []Value{NewTypeLiteral(TNone)}, nil
}

// ---- base ----

func baseHandler(args []Value, _ map[string]Value, _ []Value, _ *Registry) ([]Value, error) {
	v := args[0]
	// For a type literal the denoted lattice node is &v (the value
	// IS the type); for a concrete value it's v.Parent.
	var t *Type
	if v.Data == nil && !v.Carrier {
		t = &v
	} else {
		t = v.Parent
	}
	result, err := BaseValue(t)
	if err != nil {
		return nil, err
	}
	return []Value{result}, nil
}

// torHandler, torReturnsFn, tandHandler: moved to eng/go/core_boolean.go.
// tand's `tall` reduction (below) calls eng.TandValues directly.

// ---- tany / tall ----

func tanyHandler(args []Value, _ map[string]Value, _ []Value, r *Registry) ([]Value, error) {
	if !IsConcrete(args[0]) {
		return nil, r.AqlError("tany_error", "tany: expected a concrete list", "tany")
	}
	list, _ := AsList(args[0])
	n := list.Len()
	if n == 0 {
		return []Value{NewTypeLiteral(TNever)}, nil
	}
	if n == 1 {
		return []Value{list.Get(0)}, nil
	}
	var alts []Value
	for i := 0; i < n; i++ {
		alts = append(alts, FlattenDisjunctAlts(list.Get(i))...)
	}
	simplified := SimplifyDisjunctAlts(alts)
	if len(simplified) == 0 {
		return []Value{NewTypeLiteral(TNever)}, nil
	}
	if len(simplified) == 1 {
		return []Value{simplified[0]}, nil
	}
	return []Value{NewDisjunct(simplified)}, nil
}

func tallHandler(args []Value, _ map[string]Value, _ []Value, r *Registry) ([]Value, error) {
	if !IsConcrete(args[0]) {
		return nil, r.AqlError("tall_error", "tall: expected a concrete list", "tall")
	}
	list, _ := AsList(args[0])
	n := list.Len()
	if n == 0 {
		return []Value{NewTypeLiteral(TAny)}, nil
	}
	acc := list.Get(0)
	for i := 1; i < n; i++ {
		acc = TandValues(acc, list.Get(i))
	}
	return []Value{acc}, nil
}

// ---- convert ----

// convertOptsPattern returns the Options pattern for the 3-arg
// `convert` variant: {base?: String|None}.
func convertOptsPattern() Value {
	baseOpts := NewOrderedMap()
	baseOpts.Set("base", NewDisjunct([]Value{NewTypeLiteral(TString), NewTypeLiteral(TNone)}))
	return NewOptionsType(baseOpts)
}

// convertTo performs the actual scalar-type conversion.
func convertTo(src Value, targetType *Type, base string) (Value, error) {
	switch {
	case targetType.Matches(TString):
		if base == "" {
			return NewString(ValToString(src)), nil
		}
		if !src.Parent.Matches(TInteger) {
			return Value{}, fmt.Errorf("convert: base %q only supported for integer to string", base)
		}
		n, _ := AsInteger(src)
		var s string
		switch base {
		case "hex":
			s = strconv.FormatInt(n, 16)
		case "HEX":
			s = strings.ToUpper(strconv.FormatInt(n, 16))
		case "bin":
			s = strconv.FormatInt(n, 2)
		case "oct":
			s = strconv.FormatInt(n, 8)
		default:
			return Value{}, fmt.Errorf("convert: unknown base %q", base)
		}
		return NewString(s), nil

	case targetType.Matches(TDecimal):
		text := ValToString(src)
		f, err := strconv.ParseFloat(text, 64)
		if err != nil {
			return Value{}, fmt.Errorf("convert: cannot convert %q to decimal", text)
		}
		return NewDecimal(f), nil

	case targetType.Matches(TNumber) || targetType.Matches(TInteger):
		text := ValToString(src)
		if base == "" {
			n, err := strconv.ParseInt(text, 10, 64)
			if err != nil {
				return Value{}, fmt.Errorf("convert: cannot convert %q to number", text)
			}
			return NewInteger(n), nil
		}
		var numBase int
		switch base {
		case "hex":
			numBase = 16
		case "bin":
			numBase = 2
		case "oct":
			numBase = 8
		default:
			return Value{}, fmt.Errorf("convert: unknown base %q", base)
		}
		n, err := strconv.ParseInt(text, numBase, 64)
		if err != nil {
			return Value{}, fmt.Errorf("convert: cannot convert %q to number (base %d)", text, numBase)
		}
		return NewInteger(n), nil

	case targetType.Matches(TBoolean):
		return NewBoolean(CoerceBoolean(src)), nil

	case targetType.Equal(TAtom):
		return NewAtom(ValToString(src)), nil

	default:
		return Value{}, fmt.Errorf("convert: unsupported target type %s", targetType)
	}
}

func convert2Handler(args []Value, _ map[string]Value, _ []Value, r *Registry) ([]Value, error) {
	targetType := args[0]
	src := args[1]
	if targetType.Data != nil {
		return nil, r.AqlError("convert_error", fmt.Sprintf("convert: first argument must be a type literal, got %s", targetType.Parent), "convert")
	}
	result, err := convertTo(src, ValueType(targetType), "")
	if err != nil {
		return nil, err
	}
	return []Value{result}, nil
}

func convert3Handler(args []Value, _ map[string]Value, _ []Value, r *Registry) ([]Value, error) {
	targetType := args[0]
	opts := args[1]
	src := args[2]
	if targetType.Data != nil {
		return nil, r.AqlError("convert_error", fmt.Sprintf("convert: first argument must be a type literal, got %s", targetType.Parent), "convert")
	}

	base := ""
	if opts.Data != nil {
		m, _ := AsMap(opts)
		if m != nil {
			if bv, ok := m.Get("base"); ok {
				base = ValToString(bv)
			}
		}
	}

	result, err := convertTo(src, ValueType(targetType), base)
	if err != nil {
		return nil, err
	}
	return []Value{result}, nil
}
