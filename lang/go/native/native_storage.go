package native

import "fmt"

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
	if container.Data == nil {
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
	if container.Data == nil {
		return nil, r.AqlError("get_error", "get: cannot access property on type literal", "get")
	}
	// Integer key: list index access.
	if key.Parent.Matches(TInteger) {
		idx, _ := AsInteger(key)
		if list, _ := AsList(container); !list.IsNil() && container.Parent.Matches(TList) {
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
	if container.Data == nil {
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
	v, _ := r.Check.LookupContextType(StoreKey(args[0]))
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
