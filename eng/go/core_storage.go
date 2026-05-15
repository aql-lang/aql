package eng

import (
	"fmt"
	"strconv"
)

// registerCoreStorage installs the `get` and `set` words — the
// universal container-access pair. Both collect their args forward
// (by default), with a fan of signatures for the value-side container kinds:
//
//   - Node (Map / List / Options / record-shape) — read via `get`.
//     Maps and Lists are immutable in the kernel, so `set` is NOT
//     installed for them; in-place mutation is reserved for Object
//     instances and Arrays.
//   - Object instances — read and in-place write.
//   - Array — read and indexed-write.
//   - None — `get key None` short-circuits to None (chained-read
//     propagation).
//
// Context-Store access (copy-on-write `set` and check-mode-tracking
// `get`) is intentionally NOT in the kernel: those depend on the
// per-engine context stack and live in the production lang layer
// (see lang/engine/native_storage.go). Lang's
// storageNatives append the Store sigs to the eng-installed
// dispatch.
func registerCoreStorage(r *Registry) {
	r.RegisterNativeFunc(NativeFunc{
		Name:        "set",
		ForwardArgs: true,
		Signatures: []NativeSig{
			// Array (indexed by integer)
			{
				Args:    []*Type{TInteger, TAny, TArray},
				Handler: setArrayHandler,
				Returns: []*Type{},
			},
			// Object
			{
				Args:    []*Type{TString, TAny, TObject},
				Handler: setObjectHandler,
				Returns: []*Type{},
			},
			{
				Args:      []*Type{TAtom, TAny, TObject},
				QuoteArgs: map[int]bool{0: true},
				Handler:   setObjectHandler,
				Returns:   []*Type{},
			},
		},
	})

	r.RegisterNativeFunc(NativeFunc{
		Name:        "get",
		ForwardArgs: true,
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
			// [Key | None] — chained-read propagation.
			{Args: []*Type{TAny, TNone}, BarrierPos: 1, Handler: getNoneHandler, Returns: []*Type{TNone}},
		},
	})
}

// ---- set handlers ----

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
	arr, err := AsArray(args[2])
	if err != nil {
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
// so lang's accessor handlers (.dotted notation, getr, etc.) and
// the production Store-side set/get reuse the same key-coercion
// rules as the kernel's `get`.
func GetKey(v Value) string {
	if IsWord(v) {
		_as0, _ := AsWord(v)
		return _as0.Name
	}
	if v.VType.Matches(TString) {
		_as1, _ := AsString(v)
		return _as1
	}
	if IsAtom(v) {
		_as2, _ := AsAtom(v)
		return _as2
	}
	// Primitive scalar fallbacks: render the payload via the
	// dedicated accessors rather than `%v` on the boxed payload
	// (post Step 5b: Data is e.g. IntPayload{N:5}, not int64(5)).
	if v.VType.Matches(TInteger) {
		n, _ := AsInteger(v)
		return strconv.FormatInt(n, 10)
	}
	if v.VType.Matches(TDecimal) {
		f, _ := AsDecimal(v)
		return FormatDecimal(f)
	}
	if v.VType.Matches(TBoolean) {
		b, _ := AsBoolean(v)
		if b {
			return "true"
		}
		return "false"
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
		idx, _ := AsInteger(key)
		if list := AsList(container); !list.IsNil() && container.VType.Matches(TList) {
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
	if m := AsMap(container); m != nil {
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
