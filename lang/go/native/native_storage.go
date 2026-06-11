package native

import (
	"fmt"
	"strings"
)

// storageNatives covers `set` / `get` / `context`. The unified
// dispatch table mixes Node / Object / Array (kernel-territory
// containers) and Store (context-aware, copy-on-write) sigs in one
// place, keeping `set` and `get` polymorphic from the caller's
// perspective.
//
// `set` and `get` carry ReturnsFn closures on the Store sigs that
// thread the static type tracker (r.RecordContextSet /
// r.LookupContextType) so check-mode can recover a typed carrier
// from a previous set on the same key.
//
// Algorithms (GetKey, AsStore, AsArray, CowSet, AsObjectInstance,
// AsMutableMap, …) live in eng; this file owns the word names and
// dispatch wiring.
var storageNatives = []NativeFunc{
	{
		Name: "set",

		Signatures: []NativeSig{
			// Array (indexed by integer)
			{
				Args:    []*Type{TInteger, TAny, TArray},
				Handler: setArrayHandler,
				Returns: []*Type{}, BarrierPos:

				// Object
				-1,
			},

			{
				Args:    []*Type{TString, TAny, TObject},
				Handler: setObjectHandler,
				Returns: []*Type{}, BarrierPos: -1,
			},
			{
				Args:      []*Type{TAtom, TAny, TObject},
				QuoteArgs: map[int]bool{0: true},
				Handler:   setObjectHandler,
				Returns:   []*Type{}, BarrierPos:

				// Store (copy-on-write)
				-1,
			},

			{
				Args:      []*Type{TString, TAny, TStore},
				Handler:   setStoreHandler,
				Returns:   []*Type{},
				ReturnsFn: setStoreReturnsFn, BarrierPos: -1,
			},
			{
				Args:      []*Type{TAtom, TAny, TStore},
				QuoteArgs: map[int]bool{0: true},
				Handler:   setStoreHandler,
				Returns:   []*Type{},
				ReturnsFn: setStoreReturnsFn, BarrierPos: -1,
			},

			// Map (immutable — copy-returning). Unlike the three
			// mutable containers above, a Map is a value: set returns
			// a NEW map with the key bound and leaves the receiver
			// untouched — the same contract as push / StructUtil.setpath.
			{
				Args:    []*Type{TString, TAny, TMap},
				Handler: setMapHandler,
				Returns: []*Type{TMap}, BarrierPos: -1,
			},
			{
				Args:      []*Type{TAtom, TAny, TMap},
				QuoteArgs: map[int]bool{0: true},
				Handler:   setMapHandler,
				Returns:   []*Type{TMap}, BarrierPos: -1,
			},

			// List (immutable — copy-returning, completing the column
			// rule: Map and List both return the updated copy).
			{
				Args:    []*Type{TInteger, TAny, TList},
				Handler: setListHandler,
				Returns: []*Type{TList}, BarrierPos: -1,
			},

			// Class instance (in-place, SEALED): a declared field
			// writes in place and returns nothing; an undeclared
			// field is a loud sealed_field error — see
			// design/CLASS-OBJECT.10.md §3.3.
			{
				Args:    []*Type{TString, TAny, TClass},
				Handler: setClassInstanceHandler,
				Returns: []*Type{}, BarrierPos: -1,
			},
			{
				Args:      []*Type{TAtom, TAny, TClass},
				QuoteArgs: map[int]bool{0: true},
				Handler:   setClassInstanceHandler,
				Returns:   []*Type{}, BarrierPos: -1,
			},
		},
	},
	{
		Name: "get",

		Signatures: []NativeSig{
			// [Key | Node] — covers Map, List, Options, record-shape
			{Args: []*Type{TAtom, TNode}, QuoteArgs: map[int]bool{0: true}, BarrierPos: 1, Handler: getNodeHandler, Returns: []*Type{TAny}},
			{Args: []*Type{TString, TNode}, BarrierPos: 1, Handler: getNodeHandler, Returns: []*Type{TAny}},
			{Args: []*Type{TInteger, TNode}, BarrierPos: 1, Handler: getNodeHandler, Returns: []*Type{TAny}},
			// [Key | Array]
			{Args: []*Type{TInteger, TArray}, BarrierPos: 1, Handler: getArrayHandler, Returns: []*Type{TAny}},
			// [Key | Object]
			{Args: []*Type{TAtom, TObject}, QuoteArgs: map[int]bool{0: true}, BarrierPos: 1, Handler: getObjectHandler, Returns: []*Type{TAny}},
			{Args: []*Type{TString, TObject}, BarrierPos: 1, Handler: getObjectHandler, Returns: []*Type{TAny}},
			{Args: []*Type{TInteger, TObject}, BarrierPos: 1, Handler: getObjectHandler, Returns: []*Type{TAny}},
			// [Key | ModuleExport] — transparent export access + $module/$name
			{Args: []*Type{TAtom, TModuleExport}, QuoteArgs: map[int]bool{0: true}, BarrierPos: 1, Handler: getModuleExportHandler, ReturnsFn: moduleExportGetReturns},
			{Args: []*Type{TString, TModuleExport}, BarrierPos: 1, Handler: getModuleExportHandler, ReturnsFn: moduleExportGetReturns},
			// [Key | Module] — descriptor fields (id/kind/file/folder/exports)
			{Args: []*Type{TAtom, TModuleInst}, QuoteArgs: map[int]bool{0: true}, BarrierPos: 1, Handler: getModuleInstHandler, Returns: []*Type{TAny}},
			{Args: []*Type{TString, TModuleInst}, BarrierPos: 1, Handler: getModuleInstHandler, Returns: []*Type{TAny}},
			// [Key | Class instance] — flat field read (no prototype
			// chain; class instances resolve every field at make).
			{Args: []*Type{TAtom, TClass}, QuoteArgs: map[int]bool{0: true}, BarrierPos: 1, Handler: getObjectHandler, Returns: []*Type{TAny}},
			{Args: []*Type{TString, TClass}, BarrierPos: 1, Handler: getObjectHandler, Returns: []*Type{TAny}},
			// [Key | None] — chained-read propagation
			{Args: []*Type{TAny, TNone}, BarrierPos: 1, Handler: getNoneHandler, Returns: []*Type{TNone}},
			// [Key | Store] — check-mode-aware ReturnsFn picks up a
			// typed carrier from a previously-set key.
			{
				Args: []*Type{TString, TStore}, BarrierPos: 1, Handler: getStoreHandler,
				ReturnsFn: getStoreReturnsFn,
			},
			{
				Args: []*Type{TAtom, TStore}, QuoteArgs: map[int]bool{0: true}, BarrierPos: 1, Handler: getStoreHandler,
				ReturnsFn: getStoreReturnsFn,
			},
		},
	},
	{
		Name: "context",

		Signatures: []NativeSig{{
			Args:    []*Type{},
			Handler: contextHandler,
			Returns: []*Type{TStore}, BarrierPos: -1,
		}},
	},
}

// ---- kernel-container handlers (Node / Object / Array / None) ----

func setObjectHandler(args []Value, _ map[string]Value, _ []Value, r *Registry) ([]Value, error) {
	container := args[2]
	if !IsConcrete(container) {
		return nil, r.AqlError("set_error", "set: cannot set field on type literal", "set")
	}
	key := StoreKey(args[0])
	oi, ok := container.Data.(ObjectInstanceInfo)
	if !ok {
		return nil, fmt.Errorf("set: expected an Object instance, got %s", container.Parent.String())
	}
	oi.Fields.Set(key, args[1])
	return nil, nil
}

// setMapHandler is the Map form of set. A Map stays immutable: the
// handler returns a NEW map with the key bound (overwriting an existing
// entry), leaving the receiver untouched. This is the language's rule
// of thumb made concrete — mutable containers (Store / Object / Array)
// mutate in place and return nothing; immutable values return the
// updated copy. Keys are strings or atoms, computed keys via parens:
// `m set (k) v`.
func setMapHandler(args []Value, _ map[string]Value, _ []Value, _ *Registry) ([]Value, error) {
	m, err := RequireConcreteMap(args[2], "set")
	if err != nil {
		return nil, err
	}
	key := StoreKey(args[0])
	out := NewOrderedMap()
	for _, k := range m.Keys() {
		v, _ := m.Get(k)
		out.Set(k, v)
	}
	out.Set(key, args[1])
	return []Value{NewMap(out)}, nil
}

// setClassInstanceHandler is the sealed in-place write for class
// instances: a field declared in the class schema (own or inherited)
// writes into the flat field map and returns nothing; an undeclared
// field raises sealed_field loudly — the open-bag use case belongs to
// plain maps/Objects, not class instances (design/CLASS-OBJECT.10.md).
func setClassInstanceHandler(args []Value, _ map[string]Value, _ []Value, r *Registry) ([]Value, error) {
	container := args[2]
	if !IsConcrete(container) {
		return nil, r.AqlError("set_error", "set: cannot set field on type literal", "set")
	}
	oi, ok := container.Data.(ObjectInstanceInfo)
	if !ok {
		return nil, fmt.Errorf("set: expected a class instance, got %s", container.Parent.String())
	}
	key := StoreKey(args[0])
	val := args[1]
	if oi.TypeRef != nil {
		all := oi.TypeRef.AllFields()
		constraint, declared := all.Get(key)
		if !declared {
			name := oi.TypeRef.Name
			if name == "" {
				name = container.Parent.Name
			}
			return nil, r.AqlErrorHint("sealed_field",
				fmt.Sprintf("set: %q is not a field of %s (fields: %s)", key, name, strings.Join(all.Keys(), " ")),
				"set",
				"class instances are sealed — declare the field in the class schema, or use a plain map for open data")
		}
		// Write-time enforcement matches construction: the same strict
		// field check make runs (typed fields conform, predicates run,
		// defaulted fields constrain to the default's own type).
		checked, err := MakeClassFieldValue(val, constraint, r)
		if err != nil {
			return nil, r.AqlError("type_error",
				fmt.Sprintf("set: field %q: %s", key, err.Error()), "set")
		}
		val = checked
	}
	oi.Fields.Set(key, val)
	return nil, nil
}

func setArrayHandler(args []Value, _ map[string]Value, _ []Value, _ *Registry) ([]Value, error) {
	arr, err := AsArray(args[2])
	if err != nil {
		return nil, fmt.Errorf("set: expected an Array, got %s", args[2].Parent.String())
	}
	asInt, _ := args[0].AsConcreteInteger()
	idx := int(asInt)
	if !arr.Set(idx, args[1]) {
		return nil, fmt.Errorf("set: index %d out of bounds (length %d)", idx, arr.Len())
	}
	return nil, nil
}

func getNodeHandler(args []Value, _ map[string]Value, _ []Value, r *Registry) ([]Value, error) {
	key := args[0]
	container := args[1]
	if !IsConcrete(container) {
		return nil, r.AqlError("get_error", "get: cannot access property on type literal", "get")
	}
	// Integer key: list index access.
	if key.Parent.ConformsTo(TInteger) {
		idx, _ := AsInteger(key)
		if list, _ := AsList(container); !list.IsNil() && container.Parent.ConformsTo(TList) {
			i := int(idx)
			if i < 0 || i >= list.Len() {
				return []Value{NewTypeLiteral(TNone)}, nil
			}
			return []Value{list.Get(i)}, nil
		}
		// Fall through to map lookup with stringified key.
	}
	// String/atom/word key: map property access.
	k := getKey(key)
	if m, _ := AsMap(container); m != nil {
		val, ok := m.Get(k)
		if !ok {
			return []Value{NewTypeLiteral(TNone)}, nil
		}
		return []Value{val}, nil
	}
	return []Value{NewTypeLiteral(TNone)}, nil
}

func getObjectHandler(args []Value, _ map[string]Value, _ []Value, r *Registry) ([]Value, error) {
	key := args[0]
	container := args[1]
	if !IsConcrete(container) {
		return nil, r.AqlError("get_error", "get: cannot access property on type literal", "get")
	}
	k := getKey(key)
	if m, err := AsMutableMap(container); err == nil {
		val, found := m.Get(k)
		if !found {
			return []Value{NewTypeLiteral(TNone)}, nil
		}
		return []Value{val}, nil
	}
	oi, _ := AsObjectInstance(container)
	val, ok := oi.GetField(k)
	if !ok {
		return []Value{NewTypeLiteral(TNone)}, nil
	}
	return []Value{val}, nil
}

func getArrayHandler(args []Value, _ map[string]Value, _ []Value, _ *Registry) ([]Value, error) {
	arr, err := AsArray(args[1])
	if err != nil {
		return nil, fmt.Errorf("get: expected an Array, got %s", args[1].Parent.String())
	}
	idx, _ := args[0].AsConcreteInteger()
	val, ok := arr.Get(int(idx))
	if !ok {
		return []Value{NewTypeLiteral(TNone)}, nil
	}
	return []Value{val}, nil
}

func getNoneHandler(_ []Value, _ map[string]Value, _ []Value, _ *Registry) ([]Value, error) {
	return []Value{NewTypeLiteral(TNone)}, nil
}

// ---- set Store handler ----

func setStoreHandler(args []Value, _ map[string]Value, _ []Value, reg *Registry) ([]Value, error) {
	store, err := AsStore(args[2])
	if err != nil {
		return nil, fmt.Errorf("set: expected a Store, got %s", args[2].Parent.String())
	}
	key := StoreKey(args[0])
	CowSet(store, key, args[1], reg)
	return nil, nil
}

func setStoreReturnsFn(args []Value, r *Registry) []Value {
	r.Check.RecordContextSet(StoreKey(args[0]), args[1])
	return nil
}

// ---- get Store handler ----

func getStoreHandler(args []Value, _ map[string]Value, _ []Value, r *Registry) ([]Value, error) {
	store, err := AsStore(args[1])
	if err != nil {
		return nil, fmt.Errorf("get: expected a Store, got %s", args[1].Parent.String())
	}
	key := getKey(args[0])
	val, ok := store.Get(key)
	if !ok {
		return nil, r.AqlError("unknown key_error", fmt.Sprintf("unknown key: %s", key), "unknown key")
	}
	return []Value{val}, nil
}

func getStoreReturnsFn(args []Value, r *Registry) []Value {
	v, ok := r.Check.LookupContextType(StoreKey(args[0]))
	if !ok {
		// Escape hatch: the checker has no proven type for this key.
		// Emit a bounded gradual carrier dynamic(Any) — optimistically
		// compatible with any slot — rather than strict Carry<Any>, which
		// would fail every typed slot downstream and force a no_signature
		// or Any catch-all. (design/dynamic-modality-report.10.md, escape
		// hatch 1.) A key recorded by a prior `set` keeps its real, strict
		// carrier.
		return []Value{NewDynamicCarrier(TAny)}
	}
	return []Value{v}
}

// ---- context handler ----

// contextHandler implements the "context" word that pushes the
// current context Store onto the stack.
//
// The context is a Store (Object/Store), allowing get/set to operate on it
// directly and prototype chain resolution for nested scopes.
func contextHandler(_ []Value, _ map[string]Value, _ []Value, reg *Registry) ([]Value, error) {
	store := reg.Contexts.Top()
	if store == nil {
		return nil, reg.AqlError("context_error", "context: no active context", "context")
	}
	return []Value{NewStoreValue(TStore, store)}, nil
}

// setListHandler is the List form of set: copy-returning, like Map —
// a NEW list with the element at the index replaced; the receiver is
// untouched. Out-of-range indices are a loud error (edits, not
// lookups). Completes the immutable column of the container 2x2.
func setListHandler(args []Value, _ map[string]Value, _ []Value, r *Registry) ([]Value, error) {
	_idx, err := args[0].AsConcreteInteger()
	if err != nil {
		return nil, r.AqlError("set_error", "set: expected a concrete Integer index", "set")
	}
	lst, err2 := RequireConcreteList(args[2], "set")
	if err2 != nil {
		return nil, err2
	}
	idx := int(_idx)
	n := lst.Len()
	if idx < 0 || idx >= n {
		return nil, r.AqlError("index_out_of_range",
			fmt.Sprintf("set: index %d out of range for list of length %d", idx, n), "set")
	}
	out := make([]Value, n)
	for i := 0; i < n; i++ {
		out[i] = lst.Get(i)
	}
	out[idx] = args[1]
	return []Value{NewList(out)}, nil
}
