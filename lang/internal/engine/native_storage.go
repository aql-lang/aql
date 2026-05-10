package engine

import "fmt"

// storageNatives covers the Store / container access words: set, get
// and the context entry point. All are forward-precedence words. Each
// has a fan of signatures so the same word can drive Stores
// (copy-on-write), Object instances (in-place mutation), Arrays
// (indexed) and plain Map/List/Options nodes.
//
// `set` and `get` carry ReturnsFn closures that thread the static
// type tracker (`r.RecordContextSet` / `r.LookupContextType`) so
// check-mode can recover a typed carrier from a previous set on the
// same key.
var storageNatives = []NativeFunc{
	{
		Name:              "set",
		ForwardPrecedence: true,
		Signatures: []NativeSig{
			// Store (copy-on-write)
			{
				Args:    []Type{TString, TAny, TStore},
				Handler: setStoreHandler,
				Returns: []Type{},
				// Record key → carrier for the check-mode context
				// tracker so `get key store` can later produce a
				// typed carrier instead of Any.
				ReturnsFn: setStoreReturnsFn,
			},
			{
				Args:      []Type{TAtom, TAny, TStore},
				QuoteArgs: map[int]bool{0: true},
				Handler:   setStoreHandler,
				Returns:   []Type{},
				ReturnsFn: setStoreReturnsFn,
			},
			// Array (indexed by integer)
			{
				Args:    []Type{TInteger, TAny, TArray},
				Handler: setArrayHandler,
				Returns: []Type{},
			},
			// Object
			{
				Args:    []Type{TString, TAny, TObject},
				Handler: setObjectHandler,
				Returns: []Type{},
			},
			{
				Args:      []Type{TAtom, TAny, TObject},
				QuoteArgs: map[int]bool{0: true},
				Handler:   setObjectHandler,
				Returns:   []Type{},
			},
		},
	},
	{
		Name:              "get",
		ForwardPrecedence: true,
		Signatures: []NativeSig{
			// [Key | Store] — key forward, container from stack.
			// In check mode, consult CheckContextTypes to produce a
			// typed carrier for previously-set keys.
			{
				Args: []Type{TString, TStore}, BarrierPos: 1, Handler: getStoreHandler,
				ReturnsFn: getStoreReturnsFn,
			},
			{
				Args: []Type{TAtom, TStore}, QuoteArgs: map[int]bool{0: true}, BarrierPos: 1, Handler: getStoreHandler,
				ReturnsFn: getStoreReturnsFn,
			},
			// [Key | Node] — covers Map, List, Options
			{Args: []Type{TAtom, TNode}, QuoteArgs: map[int]bool{0: true}, BarrierPos: 1, Handler: getNodeHandler, Returns: []Type{TAny}},
			{Args: []Type{TString, TNode}, BarrierPos: 1, Handler: getNodeHandler, Returns: []Type{TAny}},
			{Args: []Type{TInteger, TNode}, BarrierPos: 1, Handler: getNodeHandler, Returns: []Type{TAny}},
			// [Key | Array]
			{Args: []Type{TInteger, TArray}, BarrierPos: 1, Handler: getArrayHandler, Returns: []Type{TAny}},
			// [Key | Object]
			{Args: []Type{TAtom, TObject}, QuoteArgs: map[int]bool{0: true}, BarrierPos: 1, Handler: getObjectHandler, Returns: []Type{TAny}},
			{Args: []Type{TString, TObject}, BarrierPos: 1, Handler: getObjectHandler, Returns: []Type{TAny}},
			{Args: []Type{TInteger, TObject}, BarrierPos: 1, Handler: getObjectHandler, Returns: []Type{TAny}},
			// [Key | None]
			{Args: []Type{TAny, TNone}, BarrierPos: 1, Handler: getNoneHandler, Returns: []Type{TNone}},
		},
	},
	{
		Name:              "context",
		ForwardPrecedence: true,
		Signatures: []NativeSig{{
			Args:    []Type{},
			Handler: contextHandler,
			Returns: []Type{TStore},
		}},
	},
}

// ---- set handlers ----

// set: Store signatures (copy-on-write, forward precedence):
//
//	[TString, TAny, TStore]   – set "key" value store
//	[TAtom/q, TAny, TStore]   – set key value store
//
// Object signatures (in-place mutation, forward precedence):
//
//	[TString, TAny, TObject]  – set "field" value obj
//	[TAtom/q, TAny, TObject]  – set field value obj
//
// Array signatures (in-place mutation, forward precedence):
//
//	[TInteger, TAny, TArray]  – set index value arr
//
// Store set is copy-on-write: a new Store layer is created (prototype =
// old Store) and propagated up through parent Stores to the ctxStack.
// Nodes (Map, List) are immutable and cannot be used with set.

func setStoreHandler(args []Value, _ map[string]Value, _ []Value, reg *Registry) ([]Value, error) {
	store := args[2].AsStore()
	if store == nil {
		return nil, fmt.Errorf("set: expected a Store, got %s", args[2].VType.String())
	}
	key := StoreKey(args[0])
	CowSet(store, key, args[1], reg)
	return nil, nil
}

func setStoreReturnsFn(args []Value, r *Registry) []Value {
	r.RecordContextSet(StoreKey(args[0]), args[1])
	return nil
}

func setObjectHandler(args []Value, _ map[string]Value, _ []Value, _ *Registry) ([]Value, error) {
	container := args[2]
	if container.Data == nil {
		return nil, fmt.Errorf("set: cannot set field on type literal")
	}
	key := StoreKey(args[0])
	oi, ok := container.Data.(ObjectInstanceInfo)
	if !ok {
		return nil, fmt.Errorf("set: expected an Object instance, got %s", container.VType.String())
	}
	oi.Fields.Set(key, args[1])
	return nil, nil
}

func setArrayHandler(args []Value, _ map[string]Value, _ []Value, _ *Registry) ([]Value, error) {
	arr := args[2].AsArray()
	if arr == nil {
		return nil, fmt.Errorf("set: expected an Array, got %s", args[2].VType.String())
	}
	_as0, _ := args[0].AsConcreteInteger()
	idx := int(_as0)
	if !arr.Set(idx, args[1]) {
		return nil, fmt.Errorf("set: index %d out of bounds (length %d)", idx, arr.Len())
	}
	return nil, nil
}

// ---- get handlers ----

// get retrieves values from a Store, Node (Map/List), or Object.
//
// Signature: [Key, Container] where Key is String|Integer|Atom|Word/q
// and Container is Node|Object|Store|Array|None.
//
// The /q modifier on atom/word key positions allows registered word names
// to be used as keys without being executed first (fixes dot-notation
// shadowing: matrix.trace does map lookup, not trace execution).

// getKey extracts the key string from any key-typed value.
func getKey(v Value) string {
	if v.IsWord() {
		_as0, _ := v.AsWord()
		return _as0.Name
	}
	if v.VType.Matches(TString) {
		_as1, _ := v.AsString()
		return _as1
	}
	if v.IsAtom() {
		_as2, _ := v.AsAtom()
		return _as2
	}
	return fmt.Sprintf("%v", v.Data)
}

func getNodeHandler(args []Value, _ map[string]Value, _ []Value, _ *Registry) ([]Value, error) {
	key := args[0]
	container := args[1]
	if container.Data == nil {
		return nil, fmt.Errorf("get: cannot access property on type literal")
	}
	// Integer key: list index access.
	if key.VType.Matches(TInteger) {
		idx, _ := key.AsInteger()
		if list := container.AsList(); !list.IsNil() && container.VType.Matches(TList) {
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
	if m := container.AsMap(); m != nil {
		val, ok := m.Get(k)
		if !ok {
			return []Value{NewTypeLiteral(TNone)}, nil
		}
		return []Value{val}, nil
	}
	return []Value{NewTypeLiteral(TNone)}, nil
}

func getObjectHandler(args []Value, _ map[string]Value, _ []Value, _ *Registry) ([]Value, error) {
	key := args[0]
	container := args[1]
	if container.Data == nil {
		return nil, fmt.Errorf("get: cannot access property on type literal")
	}
	k := getKey(key)
	if m, ok := container.Data.(*OrderedMap); ok {
		val, found := m.Get(k)
		if !found {
			return []Value{NewTypeLiteral(TNone)}, nil
		}
		return []Value{val}, nil
	}
	oi, _ := container.AsObjectInstance()
	val, ok := oi.GetField(k)
	if !ok {
		return []Value{NewTypeLiteral(TNone)}, nil
	}
	return []Value{val}, nil
}

func getStoreHandler(args []Value, _ map[string]Value, _ []Value, _ *Registry) ([]Value, error) {
	store := args[1].AsStore()
	if store == nil {
		return nil, fmt.Errorf("get: expected a Store, got %s", args[1].VType.String())
	}
	key := getKey(args[0])
	val, ok := store.Get(key)
	if !ok {
		return nil, fmt.Errorf("unknown key: %s", key)
	}
	return []Value{val}, nil
}

func getStoreReturnsFn(args []Value, r *Registry) []Value {
	v, _ := r.LookupContextType(StoreKey(args[0]))
	return []Value{v}
}

func getArrayHandler(args []Value, _ map[string]Value, _ []Value, _ *Registry) ([]Value, error) {
	arr := args[1].AsArray()
	if arr == nil {
		return nil, fmt.Errorf("get: expected an Array, got %s", args[1].VType.String())
	}
	_as3, _ := args[0].AsConcreteInteger()
	val, ok := arr.Get(int(_as3))
	if !ok {
		return []Value{NewTypeLiteral(TNone)}, nil
	}
	return []Value{val}, nil
}

func getNoneHandler(_ []Value, _ map[string]Value, _ []Value, _ *Registry) ([]Value, error) {
	return []Value{NewTypeLiteral(TNone)}, nil
}

// ---- context handler ----

// contextHandler implements the "context" word that pushes the
// current context Store onto the stack.
//
// The context is a Store (Object/Store), allowing get/set to operate on it
// directly and prototype chain resolution for nested scopes.
func contextHandler(_ []Value, _ map[string]Value, _ []Value, reg *Registry) ([]Value, error) {
	store := reg.ContextStore()
	if store == nil {
		return nil, fmt.Errorf("context: no active context")
	}
	return []Value{NewStoreValue(store)}, nil
}

// CowSet: re-exported from aqleng via aliases.go
