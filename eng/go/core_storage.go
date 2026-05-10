package eng

import "fmt"

// registerCoreStorage installs the `get` and `set` words — the
// universal container-access pair. Both are forward-precedence with
// a fan of signatures so the same word can drive Stores
// (copy-on-write), Object instances (in-place mutation), Arrays
// (indexed by integer), and plain Map/List/Options nodes (read-only
// for `get`).
//
// `set` and `get` carry ReturnsFn closures that thread the static
// type tracker (r.RecordContextSet / r.LookupContextType) so
// check-mode can recover a typed carrier from a previous set on the
// same key.
//
// Mirrors the production lang storageNatives in
// lang/internal/engine/native_storage.go, ported verbatim to eng so
// the kernel can express container access without depending on the
// lang layer.
func registerCoreStorage(r *Registry) {
	r.RegisterNativeFunc(NativeFunc{
		Name:              "set",
		ForwardPrecedence: true,
		Signatures: []NativeSig{
			// Store (copy-on-write)
			{
				Args:      []Type{TString, TAny, TStore},
				Handler:   setStoreHandler,
				Returns:   []Type{},
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
	})

	r.RegisterNativeFunc(NativeFunc{
		Name:              "get",
		ForwardPrecedence: true,
		Signatures: []NativeSig{
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
	})
}

// ---- set handlers ----

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

// GetKey extracts the key string from any key-typed value (Word,
// String, Atom, or any other value via Sprintf fallback). Exported
// so lang's accessor handlers (.dotted notation, getr, etc.) reuse
// the same key-coercion rules as the kernel's `get`.
func GetKey(v Value) string {
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
	k := GetKey(key)
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
	k := GetKey(key)
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
	key := GetKey(args[0])
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
