package native

import (
	"fmt"
	"math"
	"math/big"
	"strconv"
	"strings"

	"github.com/aql-lang/aql/eng/go"
	"github.com/cockroachdb/apd/v3"
)

// typeNatives covers the type-system words: refine, pathof, enum,
// typeof, is, teq, tpartial, guard, base, tor, tand, tany, tall,
// convert. New type ops follow the `t`-prefix convention — see
// design/TYPE-OPERATIONS.8.md.
//
// `Resource` and `Entity` (the builtin object types) are NOT installed
// via NativeFunc — they are user-typed values pushed onto the type
// stack. `installResourceTypes` handles those during Register.
var typeNatives = []NativeFunc{
	{
		// refine is the uniform type constructor — see
		// design/TYPE-UNIFORM.10.md. `refine BaseType arg`
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
		// class {schema} — define a class type: a sealed nominal record
		// minted under Ideal/Class. Schema entries: a TYPE value
		// (`{name:String}`) declares a required field; a CONCRETE value
		// (`{retries:3}`) declares a default (and the field's type is
		// the value's own type). Instances are flat (defaults resolved
		// eagerly at make) and sealed (writing an undeclared field is a
		// sealed_field error). Subclassing reuses refine:
		// `def Bar refine Foo {…}`. See design/CLASS-OBJECT.10.md.
		Name: "class",

		Signatures: []NativeSig{{
			Args:           []*Type{TMap},
			Handler:        classHandler,
			Returns:        []*Type{TType},
			RunInCheckMode: true, BarrierPos: -1,
		}},
	},
	{
		// surface {schema} — declare a pure operation contract: a map
		// of operation name → fnsig shape with Self marking the
		// conforming type's positions. `def Shape surface {…}` mints it
		// under Ideal/Surface; `<Type> exposes Shape` declares (and
		// loudly checks) conformance. See design/SURFACES.10.md.
		Name: "surface",

		Signatures: []NativeSig{{
			Args:           []*Type{TMap},
			Handler:        surfaceHandler,
			Returns:        []*Type{TType},
			RunInCheckMode: true, BarrierPos: -1,
		}},
	},
	{
		// <Type> exposes <Surface> — explicit conformance: check the
		// overload table against every required shape (Self := Type;
		// contravariant params, covariant returns) and register the
		// type in the surface's conformance set. All-or-nothing, loud
		// (surface_unsatisfied lists every gap), idempotent.
		Name: "exposes",

		Signatures: []NativeSig{{
			Args:           []*Type{TAny, TAny},
			Handler:        exposesHandler,
			Returns:        []*Type{},
			RunInCheckMode: true, BarrierPos: 1,
		}},
	},
	{
		// object {…} — construct a plain OPEN mutable keyed container,
		// sugar for `make Object {…}`. Open (any key writes, computed
		// keys via parens), fully enumerable, in-place set returning
		// nothing — the keyed sibling of Array in the container 2x2.
		// See design/CLASS-OBJECT.10.md §2.3.
		Name: "object",

		Signatures: []NativeSig{{
			Args:    []*Type{TMap},
			Handler: objectSugarHandler,
			Returns: []*Type{TObject}, BarrierPos: -1,
		}},
	},
	{
		// array […] — construct a mutable Array, sugar for
		// `make Array […]`. In-place bounds-checked set returning
		// nothing — the indexed sibling of Object.
		Name: "array",

		Signatures: []NativeSig{{
			Args:    []*Type{TList},
			Handler: arraySugarHandler,
			Returns: []*Type{TArray}, BarrierPos: -1,
		}},
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
		Name:          "typeof",
		CompileEffect: CompileModuleFold | CompileIslandPure,

		Signatures: []NativeSig{{
			Args:    []*Type{TAny},
			Handler: typeofHandler,
			Returns: []*Type{TType}, BarrierPos: -1,
			CompileEffect: CompileReadsFn, // reads a fn value's type, never invokes it
		}},
	},
	{
		Name:          "is",
		CompileEffect: CompileModuleFold | CompileIslandPure,

		Signatures: []NativeSig{{
			Args:       []*Type{TAny, TAny},
			BarrierPos: 1,
			Handler:    isHandler,
			Returns:    []*Type{TBoolean},
		}},
	},
	{
		Name:          "teq",
		CompileEffect: CompileIslandPure,

		Signatures: []NativeSig{{
			Args:          []*Type{TAny, TAny},
			BarrierPos:    1,
			Handler:       teqHandler,
			Returns:       []*Type{TBoolean},
			CompileEffect: CompileReadsFn, // type-algebra reads fn-value types, never invokes
		}},
	},
	{
		Name: "guard",

		Signatures: []NativeSig{{
			Args:       []*Type{TAny, TBoolean},
			BarrierPos: 1,
			Handler:    guardHandler,
			ReturnsFn:  guardReturns,
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
		Name:          "tor",
		CompileEffect: CompileIslandPure,

		Signatures: []NativeSig{{
			Args:          []*Type{TAny, TAny},
			BarrierPos:    1,
			Handler:       eng.TorHandler,
			ReturnsFn:     eng.TorReturnsFn,
			CompileEffect: CompileReadsFn, // type-algebra reads fn-value types, never invokes
		}},
	},
	{
		Name:          "tand",
		CompileEffect: CompileIslandPure,

		Signatures: []NativeSig{{
			Args:          []*Type{TAny, TAny},
			BarrierPos:    1,
			Handler:       eng.TandHandler,
			Returns:       []*Type{TAny},
			CompileEffect: CompileReadsFn, // type-algebra reads fn-value types, never invokes
		}},
	},
	// `tnot` (type negation / complement) — closes the type algebra
	// under Boolean operations. `tnot T` matches v iff v does not match
	// T. Algorithm lives in eng (eng.TnotHandler / eng.NegateType).
	{
		Name:          "tnot",
		CompileEffect: CompileIslandPure,

		Signatures: []NativeSig{{
			Args:          []*Type{TAny},
			BarrierPos:    -1,
			Handler:       eng.TnotHandler,
			ReturnsFn:     eng.TnotReturnsFn,
			CompileEffect: CompileReadsFn, // type-algebra reads fn-value types, never invokes
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
		Name:          "convert",
		CompileEffect: CompileModuleFold,

		Signatures: []NativeSig{
			// Ideal → Map / List (per-type IdealConverter; base Ideal → {} / [])
			{
				Args:     []*Type{TNode, TIdeal},
				TypeArgs: map[int]bool{0: true},
				Handler:  convertIdealHandler,
				// convert yields a VALUE of the target type (like make), not the
				// target type literal — ReturnsFreshInstance mints a carrier OF
				// arg0's type so a downstream consumer (e.g. arithmetic on a
				// `convert Float`ed scalar) sees an inhabitant, not a bare node.
				ReturnsFn: ReturnsFreshInstance(0), BarrierPos: -1,
			},
			// Map → Object (thaw): a fresh open mutable container
			// seeded from the map — the inverse of `convert Map o`
			// (freeze). Shallow, like the rest of the copy semantics.
			{
				Args:     []*Type{TObject, TMap},
				TypeArgs: map[int]bool{0: true},
				Handler: func(args []Value, _ map[string]Value, _ []Value, r *Registry) ([]Value, error) {
					return MakeOpenObject(args[1])
				},
				Returns: []*Type{TObject}, BarrierPos: -1,
			},
			{
				Args:     []*Type{TScalar, TMap, TScalar},
				TypeArgs: map[int]bool{0: true},
				Patterns: map[int]Value{1: convertOptsPattern()},
				Handler:  convert3Handler,
				// See the Ideal sig above: a VALUE of arg0's type, not the literal.
				ReturnsFn: ReturnsFreshInstance(0), BarrierPos: -1,
			},
			{
				Args:     []*Type{TScalar, TScalar},
				TypeArgs: map[int]bool{0: true},
				Handler:  convert2Handler,
				// See the Ideal sig above: a VALUE of arg0's type, not the literal.
				ReturnsFn: ReturnsFreshInstance(0), BarrierPos: -1,
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
// See design/IDEAL.10.md. `refine` does not bind — pair it
// with `def` (`def Foo (refine …)`).
func refineHandler(args []Value, _ map[string]Value, _ []Value, r *Registry) ([]Value, error) {
	base := args[0]
	arg := args[1]
	// A pending gen spec (the preceding `gen [...]`) turns this
	// construction into a generic SCHEMA: the body is built with the
	// placeholder bindings still live (so [value:T] resolves), then
	// the bindings pop and the result wraps as a TypeSchema for
	// InstallType. v1 supports Record schemas through refine; classes
	// go through `class {...}`, fn shapes through `fnsig [...]`.
	if spec := r.TakePendingGen(); spec != nil {
		out, err := refinePlain(base, arg, r)
		if err != nil {
			PopGenBindings(r, spec)
			return nil, err
		}
		if !IsRecordType(out[0]) {
			return nil, genUnsupported(r, spec, "refine", out[0].String())
		}
		return genWrapSchema(r, spec, out[0], SchemaRecord)
	}
	return refinePlain(base, arg, r)
}

func refinePlain(base, arg Value, r *Registry) ([]Value, error) {
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
// `behave` — see design/TYPE-UNIFORM.10.md.
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
	if IsBareTypeNode(base) {
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
// design/IDEAL.10.md.
func installIdeals(r *Registry) {
	if obj := r.Ideals.Get("Object"); obj != nil {
		obj.Construct = func(base, arg Value, r *Registry) ([]Value, error) {
			// An existing class type builds a subtype of it
			// (`def Bar refine Foo {…}`). The bare-Object form is
			// REMOVED: classes are defined with the `class` word
			// (design/CLASS-OBJECT.10.md — no deprecated aliases).
			if IsObjectType(base) {
				return objectWithParentHandler([]Value{arg, base}, nil, nil, r)
			}
			return nil, r.AqlErrorHint("refine_error",
				"refine Object is no longer the class form",
				"refine",
				"define a class instead: def Foo class {…}; subclass with def Bar refine Foo {…}")
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
	if !IsConcrete(list) {
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
	// Object/Table refinement RHS: the body is a populated type value
	// (Data carries ObjectTypeInfo/TableTypeInfo), but its denoted
	// lattice node is at b.Parent. For tag-identity ("does a carry
	// T's tag?") we want to compare a.Parent against the lattice
	// node. Without this, the handler falls through to UnifyR, which
	// returns the body (not the instance) and the downstream
	// ValuesEqual check rejects an instance-vs-body comparison even
	// though the instance plainly carries the type's tag.
	//
	// This is the `is` analogue of canonicalizing in dispatch — same
	// underlying question, different layers. Disjunct / DepScalar /
	// bare-refine bodies have Data==nil so they already route through
	// the b.Data==nil branch below; only Object/Table bodies need
	// this redirect.
	if (IsObjectType(b) || IsTableType(b)) && b.Parent != nil {
		latticeNode := b.Parent
		return []Value{NewBoolean(a.Parent.Equal(latticeNode) || a.Parent.IsSubtypeOf(latticeNode))}, nil
	}
	// Surface RHS: membership is the conformance set, answered by the
	// minted node's surfaceUnifier via the v.Is(t) doctrine — explicit
	// `exposes` declarations only, walking a's parent chain so subclass
	// instances of an exposer conform.
	if IsSurfaceType(b) && b.Parent != nil {
		return []Value{NewBoolean(a.Is(b.Parent))}, nil
	}
	// Note: a consolidation attempt to delegate structural-pattern
	// RHS (typed list, typed map, record shape) to IsValueOfType was
	// tried and reverted. `IsValueOfType` uses subset semantics on
	// record shapes ("extra keys in v are ignored") while production
	// `is` uses strict-exact-shape. They look like the same operation
	// but answer different sub-questions. Real consolidation requires
	// choosing one shape semantic and migrating the loser; that's a
	// separate design call documented in PLAN.md.
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
	if IsBareTypeNode(b) {
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
			return []Value{NewBoolean(IsBareTypeNode(a) || IsTypeBody(a) || IsRecordShape(a) || a.Parent.ConformsTo(TType))}, nil
		}
		if bNode.ConformsTo(TType) {
			// Type/-rooted subtype RHS (`Function` / `Disjunct` / `Enum`
			// / `FunctionSignature`): plain subtype check on the
			// value's Parent.
			return []Value{NewBoolean(a.Parent.ConformsTo(bNode))}, nil
		}
		// A const singleton RHS (and every other membership type) answers
		// `is` through the shared Unify path below — `1 is (const 1)` →
		// true, `1.0 is (const 1)` → false (same-base strict) — so it needs
		// no special-case here. (Const used to route through its bespoke
		// Behavior.Match; converging it onto MemberBehavior gave it a
		// matching Unify, making this branch redundant.)
		//
		// Both sides are bare type literals: the question is purely
		// lattice subtyping. Settle directly via IsSubtypeOf rather
		// than via Unify, whose List/Map/DepScalar/FnDef branches
		// short-circuit family relationships and would reject a
		// user-minted subtype (e.g. `def Foo refine List`) against
		// its base family literal.
		if IsBareTypeNode(a) {
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
	if IsBareTypeNode(a) && IsBareTypeNode(b) {
		aNode := &a
		bNode := &b
		return []Value{NewBoolean(aNode.Equal(bNode))}, nil
	}
	return []Value{NewBoolean(ValuesEqual(a, b))}, nil
}

// ---- tpartial ----

// TPartialModuleNatives holds `tpartial`, moved out of core into the
// aql:type-util module (TypeUtil.tpartial). Its handler stays here.
var TPartialModuleNatives = []NativeFunc{
	{
		Name: "tpartial",
		Signatures: []NativeSig{{
			Args:    []*Type{TAny},
			Handler: tpartialHandler,
			Returns: []*Type{TType}, BarrierPos: -1,
		}},
	},
}

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

// guardReturns is the check-mode return for `guard`: it yields the
// guarded value when the condition holds, else None — so the result type
// is value|None, not Any. Narrowing only; the runtime result is always
// the value or None, both covered by the join.
func guardReturns(args []Value, _ *Registry) []Value {
	if len(args) == 0 {
		return []Value{NewCarrier(TAny)}
	}
	return []Value{JoinCarriers(args[0], NewCarrier(TNone))}
}

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
	if IsBareTypeNode(v) {
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
	case targetType.ConformsTo(TString):
		if base == "" {
			return NewString(ValToString(src)), nil
		}
		if !src.Parent.ConformsTo(TInteger) {
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

	case targetType.ConformsTo(TFloat):
		// Big → Float is a deliberately lossy projection (the exact
		// value is approximated to the nearest binary float64).
		if src.Parent.ConformsTo(TBigInteger) || src.Parent.ConformsTo(TBigDecimal) {
			f, err := AsFloatApprox(src)
			if err != nil {
				return Value{}, fmt.Errorf("convert: cannot convert %s to float", src.Parent.Leaf())
			}
			return NewFloat(f), nil
		}
		text := ValToString(src)
		f, err := strconv.ParseFloat(text, 64)
		if err != nil {
			return Value{}, fmt.Errorf("convert: cannot convert %q to float", text)
		}
		return NewFloat(f), nil

	case targetType.ConformsTo(TBigInteger):
		return convertToBigInteger(src, base)

	case targetType.ConformsTo(TBigDecimal):
		return convertToBigDecimal(src, base)

	case targetType.ConformsTo(TNumber) || targetType.ConformsTo(TInteger):
		// Big → Integer: BigInteger range-checks int64; BigDecimal
		// truncates toward zero, then range-checks.
		if src.Parent.ConformsTo(TBigInteger) {
			n, _ := AsBigInteger(src)
			if !n.IsInt64() {
				return Value{}, fmt.Errorf("convert: %s overflows Integer (int64) range", FormatBigInteger(n))
			}
			return NewInteger(n.Int64()), nil
		}
		if src.Parent.ConformsTo(TBigDecimal) {
			n, err := bigDecimalToBigIntTrunc(src)
			if err != nil {
				return Value{}, err
			}
			if !n.IsInt64() {
				return Value{}, fmt.Errorf("convert: %s overflows Integer (int64) range", FormatBigInteger(n))
			}
			return NewInteger(n.Int64()), nil
		}
		// Numeric source: convert directly instead of stringifying and
		// re-parsing. An Integer passes through; a Float truncates toward
		// zero. Previously `convert Integer 3.0` stringified to "3.0" and
		// then failed ParseInt on the decimal point (WAT-AUDIT.5.md §Q/W).
		// A base implies a string source in a particular radix, so the
		// fast path only applies to the plain (base == "") form.
		if base == "" {
			if src.Parent.ConformsTo(TInteger) {
				n, _ := AsInteger(src)
				return NewInteger(n), nil
			}
			if src.Parent.ConformsTo(TFloat) {
				f, _ := AsFloat(src)
				if math.IsNaN(f) || math.IsInf(f, 0) || f < float64(math.MinInt64) || f >= float64(math.MaxInt64) {
					return Value{}, fmt.Errorf("convert: %s overflows Integer (int64) range", ValToString(src))
				}
				return NewInteger(int64(f)), nil
			}
		}
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

	case targetType.ConformsTo(TBoolean):
		// Explicit conversion PARSES a String's content (mirroring make's
		// MakeConvert): "true"/"false" map to the booleans, every other
		// non-empty string is truthy. This is deliberately distinct from
		// `if`-truthiness (CoerceBoolean), where a String is judged only by
		// emptiness and "false" is truthy (WAT-AUDIT.5.md §E) — `convert
		// Boolean` reads the text, `if` reads presence.
		switch {
		case src.Parent.ConformsTo(TBoolean):
			return src, nil
		case src.Parent.ConformsTo(TNumber):
			n, _ := AsNumber(src)
			return NewBoolean(n != 0), nil
		case src.Parent.ConformsTo(TString):
			switch ValToString(src) {
			case "true":
				return NewBoolean(true), nil
			case "false":
				return NewBoolean(false), nil
			default:
				return NewBoolean(ValToString(src) != ""), nil
			}
		default:
			return NewBoolean(CoerceBoolean(src)), nil
		}

	case targetType.Equal(TAtom):
		return NewAtom(ValToString(src)), nil

	default:
		return Value{}, fmt.Errorf("convert: unsupported target type %s", targetType)
	}
}

// convertToBigInteger converts a scalar source to BigInteger. Exact
// sources (Integer, BigInteger) and the truncated integer part of a
// BigDecimal are accepted; a String is parsed exactly (base-aware). A
// Float is REFUSED — a binary Float is inexact, so silently absorbing it
// into an arbitrary-precision exact type would re-introduce the very
// rounding error the Big types exist to avoid (convert to Integer first
// if a truncating projection is really wanted).
func convertToBigInteger(src Value, base string) (Value, error) {
	switch {
	case src.Parent.ConformsTo(TBigInteger):
		return src, nil
	case src.Parent.ConformsTo(TBigDecimal):
		n, err := bigDecimalToBigIntTrunc(src)
		if err != nil {
			return Value{}, err
		}
		return NewBigInteger(n), nil
	case src.Parent.ConformsTo(TFloat):
		return Value{}, fmt.Errorf("convert: cannot convert Float to BigInteger (a binary Float is inexact; convert to Integer first)")
	case src.Parent.ConformsTo(TInteger):
		i, _ := src.AsConcreteInteger()
		return NewBigInteger(big.NewInt(i)), nil
	default:
		text := ValToString(src)
		var numBase int
		switch base {
		case "", "dec":
			numBase = 10
		case "hex", "HEX":
			numBase = 16
		case "bin":
			numBase = 2
		case "oct":
			numBase = 8
		default:
			return Value{}, fmt.Errorf("convert: unknown base %q", base)
		}
		n, ok := new(big.Int).SetString(text, numBase)
		if !ok {
			return Value{}, fmt.Errorf("convert: cannot convert %q to BigInteger", text)
		}
		return NewBigInteger(n), nil
	}
}

// convertToBigDecimal converts a scalar source to BigDecimal. Integer and
// BigInteger widen exactly; BigDecimal is returned as-is; a String is
// parsed exactly via apd. A Float is REFUSED for the same reason as
// convertToBigInteger (the float is already rounded — WAT Exhibit L).
func convertToBigDecimal(src Value, base string) (Value, error) {
	if base != "" {
		return Value{}, fmt.Errorf("convert: base %q is not supported for BigDecimal", base)
	}
	switch {
	case src.Parent.ConformsTo(TBigDecimal):
		return src, nil
	case src.Parent.ConformsTo(TBigInteger):
		n, _ := AsBigInteger(src)
		d, _, err := apd.NewFromString(n.String())
		if err != nil {
			return Value{}, fmt.Errorf("convert: cannot convert %s to BigDecimal", FormatBigInteger(n))
		}
		return NewBigDecimal(d), nil
	case src.Parent.ConformsTo(TFloat):
		return Value{}, fmt.Errorf("convert: cannot convert Float to BigDecimal (a binary Float is already rounded; build the BigDecimal from a String or the 0d literal)")
	case src.Parent.ConformsTo(TInteger):
		i, _ := src.AsConcreteInteger()
		return NewBigDecimal(apd.New(i, 0)), nil
	default:
		text := ValToString(src)
		d, _, err := apd.NewFromString(text)
		if err != nil {
			return Value{}, fmt.Errorf("convert: cannot convert %q to BigDecimal", text)
		}
		return NewBigDecimal(d), nil
	}
}

// bigDecimalToBigIntTrunc returns the integer part of a BigDecimal,
// truncated toward zero (the C / Go float→int convention). Done via the
// plain decimal text so the sign and scale are handled uniformly.
func bigDecimalToBigIntTrunc(src Value) (*big.Int, error) {
	d, err := AsBigDecimal(src)
	if err != nil {
		return nil, err
	}
	s := d.Text('f')
	if i := strings.IndexByte(s, '.'); i >= 0 {
		s = s[:i]
	}
	if s == "" || s == "-" {
		s = "0"
	}
	n, ok := new(big.Int).SetString(s, 10)
	if !ok {
		return nil, fmt.Errorf("convert: cannot truncate %s to an integer", FormatBigDecimal(d))
	}
	return n, nil
}

func convertIdealHandler(args []Value, _ map[string]Value, _ []Value, r *Registry) ([]Value, error) {
	target := args[0]
	src := args[1]
	if target.Data != nil {
		return nil, r.AqlError("convert_error", "convert: first argument must be a type literal (Map or List)", "convert")
	}
	t := ValueType(target)
	switch {
	case t.Equal(TMap):
		m, err := eng.ConvertIdealToMap(src)
		if err != nil {
			return nil, r.AqlError("convert_error", "convert to Map: "+err.Error(), "convert")
		}
		return []Value{m}, nil
	case t.Equal(TList):
		l, err := eng.ConvertIdealToList(src)
		if err != nil {
			return nil, r.AqlError("convert_error", "convert to List: "+err.Error(), "convert")
		}
		return []Value{l}, nil
	default:
		return nil, r.AqlError("convert_error", "convert: an Ideal converts only to Map or List, got "+t.String(), "convert")
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
