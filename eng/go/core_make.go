package eng

import (
	"fmt"
	"strconv"
	"strings"
)

// registerCoreMake installs `make TARGET data` — the universal
// constructor for typed values. Multiple overloads cover the major
// type-construction shapes:
//
//	make ScalarType data            cast / parse a scalar
//	make ScalarType {opts} data     scalar with options (Path abs flag)
//	make ObjectType data            instantiate a named object
//	make Object data Object         instantiate with prototype
//	make Array [list]               build an Array
//	make *Type *Type {opts}           three-arg shape with arbitrary options
//	make *Type Any                   two-arg fallback
//
// Mirrors the production lang `make` (formerly in
// lang/go/engine/native_type_make_helpers.go); the handlers are
// ported verbatim. Lang re-exports the helpers via aliases so any
// callers that reach into the package-private surface keep working.
// The `make` word registration has moved to
// lang/go/engine/native_make.go. The Make* handlers below are exported
// algorithm primitives — lang's registration wires the dispatch
// table without forking the algorithm.

// isTypeLike returns true if v looks like a type target for make
// (type literal with nil Data, record type, options type, table
// type, or object type).
func isTypeLike(v Value) bool {
	if IsBareTypeNode(v) {
		return true
	}
	return IsRecordType(v) || IsOptionsType(v) || IsTableType(v) ||
		IsObjectType(v) || IsHostTypeBody(v)
}

// MakeRecord creates a record instance from a source value and
// options. Used by `make` (via the Record Ideal's Instantiate) and
// by the 3-arg make-with-options path.
//
// Backward-compat wrapper — delegates to MakeRecordR with nil
// Registry. Use MakeRecordR when fields may carry predicate-type
// constraints that need RunPredicate to evaluate.
func MakeRecord(recType RecordTypeInfo, srcVal Value, useBase bool) ([]Value, error) {
	return MakeRecordR(recType, srcVal, useBase, nil)
}

// MakeRecordR is MakeRecord with Registry threading for
// predicate-typed field constraints. See MakeFieldValueR.
func MakeRecordR(recType RecordTypeInfo, srcVal Value, useBase bool, r *Registry) ([]Value, error) {
	fieldKeys := recType.Fields.Keys()
	result := NewOrderedMap()

	fillFromMap := func(provided *OrderedMap) error {
		for _, key := range provided.Keys() {
			if _, ok := recType.Fields.Get(key); !ok {
				return fmt.Errorf("make: unknown field %q", key)
			}
		}
		for _, key := range fieldKeys {
			constraint, _ := recType.Fields.Get(key)
			val, ok := provided.Get(key)
			if !ok {
				if useBase {
					bv, err := BaseValueForConstraint(constraint)
					if err != nil {
						return fmt.Errorf("make: field %q: %w", key, err)
					}
					result.Set(key, bv)
					continue
				}
				noneVal := NewTypeLiteral(TNone)
				if _, unifOK := Unify(constraint, noneVal); unifOK {
					result.Set(key, noneVal)
					continue
				}
				return fmt.Errorf("make: missing field %q", key)
			}
			converted, err := MakeFieldValueR(val, constraint, r)
			if err != nil {
				return fmt.Errorf("make: field %q: %w", key, err)
			}
			result.Set(key, converted)
		}
		return nil
	}

	if srcVal.Parent.ConformsTo(TMap) {
		provided, err := AsMutableMap(srcVal)
		if err != nil {
			return nil, fmt.Errorf("make: expected concrete map, got %s", srcVal.String())
		}
		if err := fillFromMap(provided); err != nil {
			return nil, err
		}
		return []Value{NewMap(result)}, nil
	}

	if !srcVal.Parent.ConformsTo(TList) {
		return nil, fmt.Errorf("make: record values must be a list or map, got %s", srcVal.String())
	}
	if !IsConcrete(srcVal) {
		return nil, fmt.Errorf("make: record values must be a concrete list, got type literal")
	}
	elems, _ := AsList(srcVal)

	isNamed := elems.Len() > 0 && elems.Get(0).Parent.ConformsTo(TMap)
	if isNamed {
		if _, err := AsMutableMap(elems.Get(0)); err != nil {
			isNamed = false
		}
	}

	if isNamed {
		provided := NewOrderedMap()
		for _, elem := range elems.Slice() {
			if !elem.Parent.ConformsTo(TMap) {
				return nil, fmt.Errorf("make: mixed named and positional fields")
			}
			m, err := AsMutableMap(elem)
			if err != nil {
				return nil, fmt.Errorf("make: expected concrete map pair, got %s", elem.String())
			}
			for _, key := range m.Keys() {
				val, _ := m.Get(key)
				provided.Set(key, val)
			}
		}
		if err := fillFromMap(provided); err != nil {
			return nil, err
		}
	} else {
		if elems.Len() != len(fieldKeys) {
			return nil, fmt.Errorf("make: expected %d values, got %d",
				len(fieldKeys), elems.Len())
		}
		for i, key := range fieldKeys {
			constraint, _ := recType.Fields.Get(key)
			converted, err := MakeFieldValueR(elems.Get(i), constraint, r)
			if err != nil {
				return nil, fmt.Errorf("make: field %q: %w", key, err)
			}
			result.Set(key, converted)
		}
	}

	return []Value{NewMap(result)}, nil
}

// parseMakeOptions extracts make options from an options map.
func parseMakeOptions(opts Value) (useBase bool, err error) {
	if !opts.Parent.ConformsTo(TMap) {
		return false, fmt.Errorf("make: options must be a map, got %s", opts.String())
	}
	m, err := AsMutableMap(opts)
	if err != nil {
		return false, fmt.Errorf("make: expected concrete options map")
	}
	if v, ok := m.Get("base"); ok {
		v = ResolveWordValue(v)
		if b, bErr := AsBoolean(v); bErr == nil && b {
			useBase = true
		}
	}
	return useBase, nil
}

// buildBasePrototype creates a prototype instance with base values
// for a type that has no explicit prototype. If the type has a
// parent, it recursively builds prototypes up the chain.
func buildBasePrototype(objType ObjectTypeInfo) (*ObjectInstanceInfo, error) {
	var proto *ObjectInstanceInfo
	if objType.Parent != nil {
		var err error
		proto, err = buildBasePrototype(*objType.Parent)
		if err != nil {
			return nil, err
		}
	}

	fields := NewOrderedMap()
	for _, key := range objType.Fields.Keys() {
		constraint, _ := objType.Fields.Get(key)
		if constraint.Data != nil {
			fields.Set(key, constraint)
		} else {
			bv, err := BaseValueForConstraint(constraint)
			if err != nil {
				return nil, fmt.Errorf("make: field %q: %w", key, err)
			}
			fields.Set(key, bv)
		}
	}

	return &ObjectInstanceInfo{
		TypeRef:   &objType,
		Fields:    fields,
		Prototype: proto,
	}, nil
}

// makeObject creates an object instance from an ObjectTypeInfo, a
// map source, and an optional prototype instance.
// MakeObject is the exported wrapper around the internal object
// construction path. Used by lang-side `def x:T body` to build a
// Person-typed ObjectInstance from a raw Map body when the typed
// binding's constraint is an ObjectType — closes the
// structural-vs-nominal dispatch gap for object types.
func MakeObject(objType ObjectTypeInfo, srcVal Value, prototype *ObjectInstanceInfo, r *Registry) ([]Value, error) {
	return makeObject(objType, srcVal, prototype, r)
}

func makeObject(objType ObjectTypeInfo, srcVal Value, prototype *ObjectInstanceInfo, r *Registry) ([]Value, error) {
	if !srcVal.Parent.ConformsTo(TMap) {
		return nil, fmt.Errorf("make: object values must be a map, got %s", srcVal.String())
	}
	provided, err := AsMutableMap(srcVal)
	if err != nil {
		return nil, fmt.Errorf("make: expected concrete map, got %s", srcVal.String())
	}

	// Class types take the flat path: every field (own + inherited)
	// resolves eagerly into one field map — no prototype chain, no
	// delegation at get. See design/CLASS-OBJECT.10.md §3.
	if objType.Class {
		return makeClassInstance(objType, provided, r)
	}

	if prototype == nil && objType.Parent != nil {
		prototype, err = buildBasePrototype(*objType.Parent)
		if err != nil {
			return nil, err
		}
	}

	if prototype != nil && objType.Parent != nil {
		// An open object (nil TypeRef) can never satisfy a typed
		// parent requirement — report it rather than dereferencing
		// the absent schema.
		if prototype.TypeRef == nil {
			return nil, fmt.Errorf("make: prototype is an open Object (no type) — expected a %s instance",
				objType.Parent.Name)
		}
		if prototype.TypeRef.ID != objType.Parent.ID {
			return nil, fmt.Errorf("make: prototype type %s does not match parent type %s",
				prototype.TypeRef.Name, objType.Parent.Name)
		}
	}

	allFields := objType.AllFields()

	for _, key := range provided.Keys() {
		if _, ok := allFields.Get(key); !ok {
			return nil, fmt.Errorf("make: unknown field %q for object type %s", key, objType.Name)
		}
	}

	ownFields := objType.Fields
	result := NewOrderedMap()

	for _, key := range ownFields.Keys() {
		constraint, _ := ownFields.Get(key)
		val, hasVal := provided.Get(key)

		if !hasVal {
			if constraint.Data != nil {
				result.Set(key, constraint)
				continue
			}
			return nil, fmt.Errorf("make: missing field %q for object type %s", key, objType.Name)
		}

		val = ResolveWordValue(val)

		if val.Parent.ConformsTo(ValueType(constraint)) {
			result.Set(key, val)
		} else {
			converted, err := MakeConvert(val, ValueType(constraint))
			if err != nil {
				return nil, fmt.Errorf("make: field %q: %w", key, err)
			}
			result.Set(key, converted)
		}
	}

	if prototype != nil {
		for _, key := range provided.Keys() {
			if _, ownOk := ownFields.Get(key); !ownOk {
				val, _ := provided.Get(key)
				val = ResolveWordValue(val)
				setPrototypeField(prototype, key, val)
			}
		}
	}

	instanceType := objType.Type
	if instanceType == nil {
		instanceType = TObject
	}
	return []Value{NewObjectInstance(instanceType, ObjectInstanceInfo{
		TypeRef:   &objType,
		Fields:    result,
		Prototype: prototype,
	})}, nil
}

// makeClassInstance constructs a flat, sealed class instance: the
// full field set (inherited first, then own — AllFields order) is
// resolved eagerly into a single field map. A provided key the schema
// doesn't declare is an error; a missing field takes its schema
// default when one exists (constraint with a concrete payload) and is
// otherwise a loud missing-field error. The instance carries no
// Prototype — reads are a single map lookup.
//
// Field validation is STRICT (no silent conversion — unlike the
// legacy object path): a typed field rejects non-conforming values
// loudly, predicate-typed fields run their predicate via Unify, and
// a defaulted field rejects values outside the default's own type.
// See design/CLASS-OBJECT.10.md §3c.
func makeClassInstance(objType ObjectTypeInfo, provided *OrderedMap, r *Registry) ([]Value, error) {
	allFields := objType.AllFields()

	for _, key := range provided.Keys() {
		if _, ok := allFields.Get(key); !ok {
			return nil, fmt.Errorf("make: unknown field %q for class %s", key, objType.Name)
		}
	}

	result := NewOrderedMap()
	for _, key := range allFields.Keys() {
		constraint, _ := allFields.Get(key)
		val, hasVal := provided.Get(key)

		if !hasVal {
			if constraint.Data != nil {
				// A concrete default fills the omitted field — as a
				// FRESH copy when it is (or contains) shared-mutable
				// state, so every instance gets its own FlexList /
				// FlexMap / Array / Store / instance rather than all
				// instances aliasing the single schema value (the
				// Python mutable-default trap, silent until two
				// instances cross-talk).
				result.Set(key, FreshenDefault(constraint))
				continue
			}
			return nil, fmt.Errorf("make: missing field %q for class %s", key, objType.Name)
		}

		checked, err := MakeClassFieldValue(val, constraint, r)
		if err != nil {
			return nil, fmt.Errorf("make: field %q: %w", key, err)
		}
		result.Set(key, checked)
	}

	instanceType := objType.Type
	if instanceType == nil {
		instanceType = TClass
	}
	return []Value{NewObjectInstance(instanceType, ObjectInstanceInfo{
		TypeRef: &objType,
		Fields:  result,
	})}, nil
}

// MakeClassInstance constructs a class instance from a field map,
// running the same strict validation as `make` — exported for the
// struct-utility writers (StructUtil.setpath, clone) whose instance
// edits round-trip through construction so schema checks run.
func MakeClassInstance(objType ObjectTypeInfo, provided *OrderedMap, r *Registry) (Value, error) {
	vals, err := makeClassInstance(objType, provided, r)
	if err != nil {
		return Value{}, err
	}
	return vals[0], nil
}

// containsSharedMutable reports whether v is — or transitively
// contains — a payload whose mutations are visible through shared
// Value copies: a flex node (FlexList's *FlexListData, FlexMap's
// pointer-backed *OrderedMap), an Array (*ArrayInstanceInfo), a Store
// (*StoreInstanceInfo), or an object/class instance (whose Fields
// *OrderedMap `set` writes in place). Drives FreshenDefault's
// identity fast path: scalars and purely-immutable nodes share
// safely and are returned unchanged.
func containsSharedMutable(v Value) bool {
	if IsFlexNode(v) || IsArray(v) || IsStore(v) || IsObjectInstance(v) {
		return true
	}
	if !IsConcrete(v) {
		return false
	}
	if v.Parent.ConformsTo(TMap) {
		if m, err := AsMap(v); err == nil && m != nil {
			for _, k := range m.Keys() {
				val, _ := m.Get(k)
				if containsSharedMutable(val) {
					return true
				}
			}
		}
		return false
	}
	if v.Parent.ConformsTo(TList) {
		if lst, err := AsList(v); err == nil && !lst.IsNil() {
			for i := 0; i < lst.Len(); i++ {
				if containsSharedMutable(lst.Get(i)) {
					return true
				}
			}
		}
		return false
	}
	return false
}

// FreshenDefault returns v with every shared-mutable container payload
// it transitively contains replaced by a fresh, independent copy —
// same kind, same type tag, new identity. Flex nodes copy to flex
// nodes, Arrays to Arrays, Stores to a fresh own-data layer, and
// instances to a fresh Fields map; immutable containers are rebuilt
// only on the path down to a mutable payload, and a value with no
// shared-mutable state anywhere inside is returned unchanged.
//
// This is what makes a concrete mutable default in a class schema
// per-instance: `def Foo class {items:(flex [])}` hands each `make`
// its own FlexList instead of aliasing the schema's single one.
// (Pointer payloads the kernel cannot meaningfully duplicate — a
// running Timer/Interval, module/extension payloads — pass through
// unchanged.)
func FreshenDefault(v Value) Value {
	if !containsSharedMutable(v) {
		return v
	}
	out := v
	switch {
	case IsObjectInstance(v):
		info, err := AsObjectInstance(v)
		if err != nil {
			return v
		}
		fields := NewOrderedMap()
		for _, k := range info.Fields.Keys() {
			fv, _ := info.Fields.Get(k)
			fields.Set(k, FreshenDefault(fv))
		}
		ninfo := info // struct copy: TypeRef / Prototype stay shared
		ninfo.Fields = fields
		out.Data = ninfo
	case IsArray(v):
		ai, err := AsArray(v)
		if err != nil || ai == nil {
			return v
		}
		elems := make([]Value, len(ai.Elems))
		for i := range ai.Elems {
			elems[i] = FreshenDefault(ai.Elems[i])
		}
		out.Data = &ArrayInstanceInfo{Elems: elems}
	case IsStore(v):
		si, err := AsStore(v)
		if err != nil || si == nil {
			return v
		}
		nsi := *si // Prototype chain stays shared (read-only fallback)
		nsi.Data = make(map[string]Value, len(si.Data))
		for k, val := range si.Data {
			nsi.Data[k] = FreshenDefault(val)
		}
		out.Data = &nsi
	case v.Parent.ConformsTo(TMap):
		// Plain Map and FlexMap share the MapPayload shape; the Parent
		// tag (kept by the struct copy) preserves the flavor.
		m, err := AsMap(v)
		if err != nil || m == nil {
			return v
		}
		om := NewOrderedMap()
		for _, k := range m.Keys() {
			val, _ := m.Get(k)
			om.Set(k, FreshenDefault(val))
		}
		out.Data = MapPayload{M: om}
	case v.Parent.ConformsTo(TList):
		lst, err := AsList(v)
		if err != nil || lst.IsNil() {
			return v
		}
		elems := make([]Value, lst.Len())
		for i := 0; i < lst.Len(); i++ {
			elems[i] = FreshenDefault(lst.Get(i))
		}
		if IsFlexList(v) {
			out.Data = &FlexListData{Elems: elems}
		} else {
			out.Data = ListPayload{Elems: elems}
		}
	}
	return out
}

// MakeClassFieldValue validates one value against a class-schema
// field constraint, strictly: a bare type-node constraint requires a
// conforming value (no conversion fallback — a Float is not an
// Integer; say so); a predicate / disjunction constraint runs through
// Unify so the predicate body executes; a concrete default constrains
// the field to the default's own (declared) type, which makes
// refined-typed defaults enforce their refinement. Shared by `make`
// and the class-instance `set` handler so write-time enforcement
// matches construction.
func MakeClassFieldValue(val Value, constraint Value, r *Registry) (Value, error) {
	val = ResolveWordValue(val)

	// Concrete default — the field's type is the default value's own
	// type (its Parent), so {x:1} accepts Integers and rejects the
	// rest, a default carrying a refined type enforces it, and a
	// const-singleton default admits exactly its inhabitant. The
	// membership question routes through v.Is(t) — the one-predicate
	// rule — so the type's Behavior decides: DefaultBehavior is plain
	// lattice conformance, bareRefineUnifier stays nominal, and a
	// const singleton's Behavior matches by value equality.
	if constraint.Data != nil && !IsTypeBody(constraint) {
		if val.Is(constraint.Parent) {
			return val, nil
		}
		return Value{}, fmt.Errorf("expected %s (the default's type), got %s (%s)",
			constraint.Parent.Name, val.Parent.Name, val.String())
	}

	// Class-typed field ({i:Inner}) — nominal check: the value must be
	// an instance of that class (or a subclass, via the lattice).
	if IsObjectType(constraint) {
		info, _ := AsObjectType(constraint)
		if IsObjectInstance(val) && info.Type != nil && val.Parent.ConformsTo(info.Type) {
			return val, nil
		}
		return Value{}, fmt.Errorf("expected a %s instance, got %s (%s)",
			info.Name, val.Parent.Name, val.String())
	}

	// Type constraint — bare node, predicate type, disjunction.
	// UnifyExplainR runs predicates and reports the reason on failure.
	unified, uerr := UnifyExplainR(constraint, val, r)
	if uerr != nil {
		return Value{}, fmt.Errorf("expected %s, got %s (%s): %s",
			constraint.String(), val.Parent.Name, val.String(), uerr.Error())
	}
	return unified, nil
}

// makePath creates a Path value from a source: a string ("a/b") or a
// list of segments (["a" "b"]). Slashes are normalised — every "/"
// separates segments and empty segments (from a "//" run, or a
// leading/trailing "/") are dropped, so "a//b" and ["a/" "b"] both
// yield ["a" "b"]. A leading "/" on the source — the string, or the
// first list element — marks the path absolute; the abs argument
// (from a `{ abs:… }` option map) forces it absolute regardless.
func makePath(srcVal Value, abs bool) ([]Value, error) {
	var raw []string
	switch {
	case srcVal.Parent.ConformsTo(TList) && srcVal.Data != nil:
		elems, _ := AsList(srcVal)
		raw = make([]string, elems.Len())
		for i := 0; i < elems.Len(); i++ {
			raw[i] = ValToString(elems.Get(i))
		}
	case srcVal.Parent.ConformsTo(TString) && srcVal.Data != nil:
		s, _ := AsString(srcVal)
		raw = []string{s}
	default:
		return nil, fmt.Errorf("make: Path source must be a list or string, got %s", srcVal.String())
	}

	if len(raw) > 0 && strings.HasPrefix(raw[0], "/") {
		abs = true
	}
	var parts []string
	for _, r := range raw {
		for _, seg := range strings.Split(r, "/") {
			if seg != "" {
				parts = append(parts, seg)
			}
		}
	}
	return []Value{NewPath(parts, abs)}, nil
}

// MakeHandler is the position-agnostic 2-arg make dispatcher.
func MakeHandler(args []Value, _ map[string]Value, _ []Value, reg *Registry) ([]Value, error) {
	targetVal, srcVal := args[0], args[1]
	if !isTypeLike(targetVal) && isTypeLike(srcVal) {
		targetVal, srcVal = srcVal, targetVal
	}

	targetVal = ResolveTypeLiteralDef(targetVal, reg)

	// A generic SCHEMA as the make target — `make Box {value:42}` —
	// infers its type arguments from the construction body and
	// instantiates first (design/GENERICS.10.md Phase 7 / D12); the
	// instantiation then takes the ordinary path below. Uninferable,
	// undefaulted parameters error (unbound_param) — never silent Any.
	if IsTypeSchema(targetVal) {
		inst, err := InferAndInstantiateSchema(reg, targetVal, srcVal)
		if err != nil {
			return nil, err
		}
		return MakeHandler([]Value{inst, srcVal}, nil, nil, reg)
	}

	// Structural kinds (object / record / table) instantiate through
	// the Ideal registry — see ideal.go and design/IDEAL.10.md.
	if reg != nil {
		if ideal := reg.Ideals.For(targetVal); ideal != nil && ideal.Instantiate != nil {
			return ideal.Instantiate(targetVal, srcVal, reg)
		}
		if m := reg.Ideals.Match(targetVal); m != nil && !m.available() {
			return nil, fmt.Errorf("make: the %s type-kind is not available in this registry", m.Name)
		}
	}

	if IsBareTypeNode(targetVal) && targetVal.Equal(TPath) {
		return makePath(srcVal, false)
	}

	if targetVal.Equal(TOptions) && IsBareTypeNode(targetVal) {
		if !srcVal.Parent.Equal(TMap) || !IsConcrete(srcVal) {
			return nil, fmt.Errorf("make: Options requires a concrete map")
		}
		src, err := AsMutableMap(srcVal)
		if err != nil {
			return nil, fmt.Errorf("make: Options requires a concrete map")
		}
		return []Value{NewOptionsType(src)}, nil
	}

	if targetVal.Data != nil {
		return nil, fmt.Errorf("make: first argument must be a type literal or record type, got %s", targetVal.String())
	}

	targetType := &targetVal
	if srcVal.Parent.ConformsTo(targetType) {
		return []Value{srcVal}, nil
	}

	// A user-minted scalar refinement (def Foo refine Integer) is a
	// nominal newtype, and `make Foo v` is its constructor: cast v to
	// the BASE type if needed, then tag the result with the refinement
	// — the same reparent the typed-def path (`def x:Foo v`) performs.
	// Without this, make silently returned a base-tagged value
	// (design/CLASS-OBJECT.10.md §3c typed-defaults gap 1), so
	// `(make Foo 1) is Foo` was false and a Foo-typed schema default
	// could not be expressed.
	if canon := CanonicalType(reg, targetType); reg != nil && canon != nil && canon.Origin == OriginUserDef {
		targetType = canon
		base := builtinBaseOf(targetType)
		if base != nil && base.ConformsTo(TScalar) {
			conv := srcVal
			if !srcVal.Parent.ConformsTo(base) {
				c, cerr := MakeConvert(srcVal, base)
				if cerr != nil {
					return nil, cerr
				}
				conv = c
			}
			return []Value{ReparentValue(conv, targetType)}, nil
		}
	}

	result, err := MakeConvert(srcVal, targetType)
	if err != nil {
		return nil, err
	}
	return []Value{result}, nil
}

// builtinBaseOf walks a user-minted type's parent chain to the first
// builtin ancestor — the conversion base for `make <Refinement> v`.
func builtinBaseOf(t *Type) *Type {
	for p := t.Parent; p != nil; p = p.Parent {
		if p.Origin == OriginBuiltin {
			return p
		}
	}
	return nil
}

// MakeTable instantiates a table value — a list of record-conforming
// rows — from a table type and a list of row data. Each row may be
// positional or named. Backs the Table Ideal's Instantiate.
func MakeTable(tt TableTypeInfo, srcVal Value) ([]Value, error) {
	return MakeTableR(tt, srcVal, nil)
}

// MakeTableR is MakeTable with Registry threading for predicate-typed
// field constraints in table rows. See MakeFieldValueR.
func MakeTableR(tt TableTypeInfo, srcVal Value, r *Registry) ([]Value, error) {
	recType := tt.Record
	if !srcVal.Parent.ConformsTo(TList) {
		return nil, fmt.Errorf("make: table values must be a list of row lists, got %s", srcVal.String())
	}
	if !IsConcrete(srcVal) {
		return nil, fmt.Errorf("make: table values must be a concrete list, got type literal")
	}
	rows, _ := AsList(srcVal)
	fieldKeys := recType.Fields.Keys()
	resultRows := make([]Value, 0, rows.Len())

	for rowIdx, rowVal := range rows.Slice() {
		if !rowVal.Parent.ConformsTo(TList) {
			return nil, fmt.Errorf("make: table row %d must be a list, got %s", rowIdx, rowVal.String())
		}
		if !IsConcrete(rowVal) {
			return nil, fmt.Errorf("make: table row %d must be a concrete list, got type literal", rowIdx)
		}
		rowElems, _ := AsList(rowVal)

		isNamed := rowElems.Len() > 0 && rowElems.Get(0).Parent.ConformsTo(TMap)
		if isNamed {
			if _, err := AsMutableMap(rowElems.Get(0)); err != nil {
				isNamed = false
			}
		}

		result := NewOrderedMap()
		if isNamed {
			provided := NewOrderedMap()
			for _, elem := range rowElems.Slice() {
				if !elem.Parent.ConformsTo(TMap) {
					return nil, fmt.Errorf("make: table row %d: mixed named and positional fields", rowIdx)
				}
				m, err := AsMutableMap(elem)
				if err != nil {
					return nil, fmt.Errorf("make: table row %d: expected concrete map pair, got %s", rowIdx, elem.String())
				}
				for _, key := range m.Keys() {
					val, _ := m.Get(key)
					provided.Set(key, val)
				}
			}
			for _, key := range fieldKeys {
				val, ok := provided.Get(key)
				if !ok {
					return nil, fmt.Errorf("make: table row %d: missing field %q", rowIdx, key)
				}
				constraint, _ := recType.Fields.Get(key)
				converted, err := MakeFieldValueR(val, constraint, r)
				if err != nil {
					return nil, fmt.Errorf("make: table row %d: field %q: %w", rowIdx, key, err)
				}
				result.Set(key, converted)
			}
			for _, key := range provided.Keys() {
				if _, ok := recType.Fields.Get(key); !ok {
					return nil, fmt.Errorf("make: table row %d: unknown field %q", rowIdx, key)
				}
			}
		} else {
			if rowElems.Len() != len(fieldKeys) {
				return nil, fmt.Errorf("make: table row %d: expected %d values, got %d",
					rowIdx, len(fieldKeys), rowElems.Len())
			}
			for i, key := range fieldKeys {
				constraint, _ := recType.Fields.Get(key)
				converted, err := MakeFieldValueR(rowElems.Get(i), constraint, r)
				if err != nil {
					return nil, fmt.Errorf("make: table row %d: field %q: %w", rowIdx, key, err)
				}
				result.Set(key, converted)
			}
		}

		resultRows = append(resultRows, NewMap(result))
	}

	return []Value{NewList(resultRows)}, nil
}

// registerKernelIdeals installs the kernel type-kind descriptors with
// their dispatch predicate (Accepts) and value constructor
// (Instantiate). The type-level constructor (Ideal.Construct) is
// filled in by the language layer's installIdeals — type construction
// reuses the surface-registered object/record handlers. Called from
// NewRegistry so every Registry, including the bare eng spec runner,
// can `make` the structural kinds.
func registerKernelIdeals(r *Registry) {
	r.Ideals.Register(&Ideal{
		Name:    "Object",
		Enabled: true,
		Accepts: func(v Value) bool {
			return (IsBareTypeNode(v) && v.Equal(TObject)) || IsObjectType(v)
		},
		Instantiate: func(typ, data Value, r *Registry) ([]Value, error) {
			// Bare Object: construct a plain OPEN mutable keyed
			// container — the 2x2's keyed sibling of Array (design/
			// CLASS-OBJECT.10.md Phase B). Open (any key writes),
			// fully enumerable, in-place set, no schema, no seal.
			if IsBareTypeNode(typ) && typ.Equal(TObject) {
				return MakeOpenObject(data)
			}
			objType, err := AsObjectType(typ)
			if err != nil {
				return nil, fmt.Errorf("make: expected a constructed object type, got %s", typ.String())
			}
			return makeObject(objType, data, nil, r)
		},
	})
	r.Ideals.Register(&Ideal{
		Name:    "Record",
		Enabled: true,
		Accepts: func(v Value) bool {
			return (IsBareTypeNode(v) && v.Equal(TRecord)) || IsRecordType(v)
		},
		Instantiate: func(typ, data Value, r *Registry) ([]Value, error) {
			recType, err := AsRecordType(typ)
			if err != nil {
				return nil, fmt.Errorf("make: expected a constructed record type, got %s", typ.String())
			}
			return MakeRecordR(recType, data, false, r)
		},
	})
	r.Ideals.Register(&Ideal{
		Name:    "Table",
		Enabled: true,
		Accepts: func(v Value) bool {
			return (IsBareTypeNode(v) && v.Equal(TTable)) || IsTableType(v)
		},
		Instantiate: func(typ, data Value, r *Registry) ([]Value, error) {
			tt, err := AsTableType(typ)
			if err != nil {
				return nil, fmt.Errorf("make: expected a constructed table type, got %s", typ.String())
			}
			return MakeTableR(tt, data, r)
		},
	})
}

// MakeWithPrototype is the 3-arg make-with-prototype dispatcher.
func MakeWithPrototype(args []Value, _ map[string]Value, _ []Value, reg *Registry) ([]Value, error) {
	resolved := make([]Value, len(args))
	for i, a := range args {
		resolved[i] = ResolveTypeLiteralDef(a, reg)
	}
	var targetVal, srcVal, protoVal Value
	for _, a := range resolved {
		switch {
		case IsObjectType(a) && targetVal.Parent.Equal(nil):
			targetVal = a
		case IsObjectInstance(a):
			protoVal = a
		default:
			srcVal = a
		}
	}

	if !IsObjectType(targetVal) {
		return nil, fmt.Errorf("make: prototype can only be used with object types, got %s", targetVal.String())
	}
	if !IsObjectInstance(protoVal) {
		return nil, fmt.Errorf("make: prototype must be an object instance, got %s", protoVal.String())
	}

	objType, _ := AsObjectType(targetVal)
	protoInfo, _ := AsObjectInstance(protoVal)
	if objType.Class {
		return nil, fmt.Errorf("make: a class has no prototypes — instances are flat; construct with `make %s {…}`", objType.Name)
	}
	return makeObject(objType, srcVal, &protoInfo, reg)
}

// MakeWithOpts is the 3-arg make-with-options dispatcher.
func MakeWithOpts(args []Value, _ map[string]Value, _ []Value, reg *Registry) ([]Value, error) {
	var targetVal, srcVal, optsVal Value
	for _, a := range args {
		resolved := ResolveTypeLiteralDef(a, reg)
		switch {
		case isTypeLike(resolved) && targetVal.Parent.Equal(nil):
			targetVal = resolved
		default:
			if srcVal.Parent.Equal(nil) {
				srcVal = a
			} else {
				optsVal = a
			}
		}
	}
	if optsVal.Parent.ConformsTo(TList) && srcVal.Parent.ConformsTo(TMap) && srcVal.Data != nil {
		srcVal, optsVal = optsVal, srcVal
	}

	useBase, err := parseMakeOptions(optsVal)
	if err != nil {
		return nil, err
	}

	if IsObjectType(targetVal) {
		objType, _ := AsObjectType(targetVal)
		return makeObject(objType, srcVal, nil, reg)
	}

	if IsRecordType(targetVal) {
		recType, _ := AsRecordType(targetVal)
		return MakeRecordR(recType, srcVal, useBase, reg)
	}

	if IsBareTypeNode(targetVal) && targetVal.Equal(TPath) {
		abs := false
		if optsMap, _ := AsMap(optsVal); optsMap != nil {
			if v, ok := optsMap.Get("abs"); ok && v.Parent.ConformsTo(TBoolean) {
				abs, _ = AsBoolean(v)
			}
		}
		return makePath(srcVal, abs)
	}

	// Pass reg through so the Ideal-registry dispatch in MakeHandler
	// can reach the structural kinds (e.g. a table target with opts).
	return MakeHandler([]Value{srcVal, targetVal}, nil, nil, reg)
}

// MakeScalarHandler converts a scalar value to a target scalar type.
func MakeScalarHandler(args []Value, _ map[string]Value, _ []Value, reg *Registry) ([]Value, error) {
	targetVal, srcVal := args[0], args[1]
	if targetVal.Data != nil {
		return nil, fmt.Errorf("make: expected a type literal, got %s", targetVal.String())
	}
	targetType := &targetVal
	if targetType.Equal(TPath) {
		return makePath(srcVal, false)
	}
	if srcVal.Parent.ConformsTo(targetType) {
		return []Value{srcVal}, nil
	}
	// A user-minted scalar refinement (def Foo refine Integer) is a
	// nominal newtype, and `make Foo v` is its constructor: cast v to
	// the BASE type if needed, then tag the result with the refinement
	// — the same reparent the typed-def path (`def x:Foo v`) performs.
	// Without this, make silently returned a base-tagged value, so
	// `(make Foo 1) is Foo` was false and a Foo-typed schema default
	// could not be expressed (design/CLASS-OBJECT.10.md §3c gap 1).
	if canon := CanonicalType(reg, targetType); reg != nil && canon != nil && canon.Origin == OriginUserDef {
		if base := builtinBaseOf(canon); base != nil && base.ConformsTo(TScalar) {
			conv := srcVal
			if !srcVal.Parent.ConformsTo(base) {
				c, cerr := MakeConvert(srcVal, base)
				if cerr != nil {
					return nil, cerr
				}
				conv = c
			}
			return []Value{ReparentValue(conv, canon)}, nil
		}
	}
	result, err := MakeConvert(srcVal, targetType)
	if err != nil {
		return nil, err
	}
	return []Value{result}, nil
}

// MakeObjHandler is the 2-arg [IdealType, Map] make handler. It
// instantiates object types and Ideal-kind types; a non-object
// IdealType target (e.g. Options) defers to the generic make
// dispatcher.
func MakeObjHandler(args []Value, _ map[string]Value, _ []Value, reg *Registry) ([]Value, error) {
	targetVal, srcVal := args[0], args[1]
	targetVal = ResolveTypeLiteralDef(targetVal, reg)
	if reg != nil {
		if ideal := reg.Ideals.For(targetVal); ideal != nil && ideal.Instantiate != nil {
			return ideal.Instantiate(targetVal, srcVal, reg)
		}
		if m := reg.Ideals.Match(targetVal); m != nil && !m.available() {
			return nil, fmt.Errorf("make: the %s type-kind is not available in this registry", m.Name)
		}
	}
	if IsObjectType(targetVal) {
		objType, _ := AsObjectType(targetVal)
		return makeObject(objType, srcVal, nil, reg)
	}
	// Not an object type and unclaimed by an Ideal kind (e.g.
	// Options) — defer to the generic make dispatcher.
	return MakeHandler([]Value{targetVal, srcVal}, nil, nil, reg)
}

// MakeOpenObject constructs a plain open Object instance — the
// mutable keyed container — seeded from a concrete map. The field
// map is copied so the new container is decoupled from the literal;
// the instance has no TypeRef (no schema, no sealing) and no
// prototype. `object {…}` and `make Object {…}` both route here.
func MakeOpenObject(data Value) ([]Value, error) {
	m, err := RequireConcreteMap(data, "make")
	if err != nil {
		return nil, fmt.Errorf("make: Object source must be a concrete map: %w", err)
	}
	fields := NewOrderedMap()
	for _, k := range m.Keys() {
		v, _ := m.Get(k)
		fields.Set(k, ResolveWordValue(v))
	}
	return []Value{NewObjectInstance(TObject, ObjectInstanceInfo{Fields: fields})}, nil
}

// MakeArrayHandler is the 2-arg [Array, List] make handler.
func MakeArrayHandler(args []Value, _ map[string]Value, _ []Value, _ *Registry) ([]Value, error) {
	srcVal := args[1]
	if !srcVal.Parent.ConformsTo(TList) || !IsConcrete(srcVal) {
		return nil, fmt.Errorf("make: Array source must be a concrete list, got %s", srcVal.String())
	}
	srcList, _ := AsList(srcVal)
	return []Value{NewArray(srcList.Slice())}, nil
}

// MakeScalarOptsHandler is the 3-arg [ScalarType, Map, Any] make handler.
func MakeScalarOptsHandler(args []Value, _ map[string]Value, _ []Value, _ *Registry) ([]Value, error) {
	targetVal, optsVal, srcVal := args[0], args[1], args[2]
	if IsBareTypeNode(targetVal) && targetVal.Equal(TPath) {
		abs := false
		if optsMap, _ := AsMap(optsVal); optsMap != nil {
			if v, ok := optsMap.Get("abs"); ok && v.Parent.ConformsTo(TBoolean) {
				abs, _ = AsBoolean(v)
			}
		}
		return makePath(srcVal, abs)
	}
	return MakeScalarHandler([]Value{targetVal, srcVal}, nil, nil, nil)
}

// MakeConvert converts a source value to a target scalar type.
// Exported so production lang and downstream tooling can reuse the
// same scalar-coercion logic that backs `make`.
func MakeConvert(src Value, targetType *Type) (Value, error) {
	switch {
	case targetType.ConformsTo(TString):
		return NewString(ValToString(src)), nil

	case targetType.ConformsTo(TFloat):
		text := ValToString(src)
		f, err := strconv.ParseFloat(text, 64)
		if err != nil {
			return Value{}, fmt.Errorf("make: cannot convert %q to float", text)
		}
		return NewFloat(f), nil

	case targetType.ConformsTo(TNumber) || targetType.ConformsTo(TInteger):
		text := ValToString(src)
		n, err := strconv.ParseInt(text, 10, 64)
		if err != nil {
			f, ferr := strconv.ParseFloat(text, 64)
			if ferr != nil {
				return Value{}, fmt.Errorf("make: cannot convert %q to number", text)
			}
			return NewInteger(int64(f)), nil
		}
		return NewInteger(n), nil

	case targetType.ConformsTo(TBoolean):
		switch {
		case src.Parent.ConformsTo(TBoolean):
			return src, nil
		case src.Parent.ConformsTo(TNumber):
			_as0, _ := AsNumber(src)
			return NewBoolean(_as0 != 0), nil
		default:
			text := ValToString(src)
			switch text {
			case "true":
				return NewBoolean(true), nil
			case "false":
				return NewBoolean(false), nil
			default:
				return NewBoolean(text != ""), nil
			}
		}

	case targetType.Equal(TAtom):
		return NewAtom(ValToString(src)), nil

	default:
		return Value{}, fmt.Errorf("make: unsupported target type %s", targetType)
	}
}

// MakeFieldValue converts a value to match a record field's type
// constraint. Exported for the same reason as MakeConvert — keeps
// the production lang's record-make path on the engine's canonical
// implementation.
//
// Backward-compat wrapper that delegates to MakeFieldValueR with a
// nil Registry — sufficient for non-predicate constraints (type
// literals, structural records, scalars). For predicate-type field
// constraints use MakeFieldValueR to thread a Registry so the
// predicate body can run.
func MakeFieldValue(val Value, constraint Value) (Value, error) {
	return MakeFieldValueR(val, constraint, nil)
}

// MakeFieldValueR is MakeFieldValue with Registry threading so
// predicate-type field constraints (`def Rec refine Record [x:Pos]`
// where Pos is a predicate fn type) can run the predicate body via
// RunPredicate. When the constraint is an FnDef/Function value (a
// predicate), the candidate is gated by the predicate's input type
// and admitted only if the predicate accepts.
func MakeFieldValueR(val Value, constraint Value, r *Registry) (Value, error) {
	val = ResolveWordValue(val)

	if IsBareTypeNode(constraint) {
		constraintType := ValueType(constraint)
		if val.Parent.ConformsTo(constraintType) {
			return val, nil
		}
		return MakeConvert(val, constraintType)
	}

	// Predicate-type constraint (and disjunct-with-predicate) — route
	// through UnifyExplainR so the predicate body runs via RunPredicate
	// instead of failing with "incompatible types" when Unify tries to
	// match an FnDef against a scalar.
	unified, uerr := UnifyExplainR(constraint, val, r)
	if uerr != nil {
		return Value{}, fmt.Errorf("value %s does not match constraint %s: %s",
			val.String(), constraint.String(), uerr.Error())
	}
	return unified, nil
}

// ResolveFieldType resolves a record field's type constraint value.
//
// Three resolution strategies:
//  1. String matching a user-defined type name in DefStacks → replaced
//     with the defined type value (e.g., disjunctions by name).
//  2. Concrete list → evaluated as code in a sub-engine so that
//     expressions like [string or none] produce a disjunction.
//  3. Everything else passes through unchanged.
func ResolveFieldType(r *Registry, v Value) Value {
	if v.Data != nil && (v.Parent.ConformsTo(TString) || v.Parent.ConformsTo(TAtom) || IsWord(v)) {
		var name string
		if IsWord(v) {
			_as2, _ := AsWord(v)
			name = _as2.Name
		} else {
			name, _ = AsString(v)
		}
		// Only CAPITALISED names are type references (the type-name
		// convention). Without this gate, a lowercase string default
		// that happens to spell a registered word name resolved to
		// that word's FnDef — `class {op:"add"}` seeded the field
		// with add's function value instead of the string "add"
		// (IsTypeBody admits FnDef values because predicate types are
		// fn-bodied).
		if !IsCapitalisedName(name) {
			return v
		}
		if tv, ok := r.TopTypeBody(name); ok {
			if IsTypeBody(tv) {
				return tv
			}
		}
		if top, ok := r.Defs.Top(name); ok {
			if IsTypeBody(top) {
				return top
			}
		}
		return v
	}

	if v.Parent.Equal(TList) && !IsTypedList(v) && !IsTableType(v) {
		elems, _ := AsList(v)
		input := make([]Value, elems.Len())
		for i, e := range elems.Slice() {
			if (e.Parent.ConformsTo(TString) || e.Parent.ConformsTo(TAtom)) && e.Data != nil {
				name, _ := AsString(e)
				if r.Lookup(name) != nil {
					input[i] = NewWord(name)
					continue
				}
			}
			input[i] = e
		}
		sub := New(r)
		results, err := sub.Run(input)
		if err == nil && len(results) == 1 {
			return results[0]
		}
		return v
	}

	return v
}

// setPrototypeField sets a field value on the appropriate level of a
// prototype chain. A level's declared fields come from its schema
// (TypeRef); an OPEN object level (nil TypeRef — no schema) declares
// whatever its own field map currently holds.
func setPrototypeField(proto *ObjectInstanceInfo, key string, val Value) {
	for p := proto; p != nil; p = p.Prototype {
		declared := false
		if p.TypeRef != nil {
			_, declared = p.TypeRef.Fields.Get(key)
		} else if p.Fields != nil {
			_, declared = p.Fields.Get(key)
		}
		if declared {
			p.Fields.Set(key, val)
			return
		}
	}
}
