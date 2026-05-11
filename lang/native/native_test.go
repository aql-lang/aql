package native

import (
	"testing"

	"github.com/aql-lang/aql/lang/engine"
)

// --- helpers ---

func newMap(kvs ...any) engine.Value {
	m := engine.NewOrderedMap()
	for i := 0; i < len(kvs); i += 2 {
		m.Set(kvs[i].(string), kvs[i+1].(engine.Value))
	}
	return engine.NewMap(m)
}

func newList(vals ...engine.Value) engine.Value {
	return engine.NewList(vals)
}

// --- Register ---

func TestRegister(t *testing.T) {
	r, err := engine.NewRegistry()
	if err != nil {
		t.Fatal(err)
	}
	Register(r)
	// If we get here without panic, registration worked.
}

// --- clone ---

func TestCloneHandler(t *testing.T) {
	orig := newMap("a", engine.NewInteger(1), "b", engine.NewString("hello"))
	result, err := cloneHandler([]engine.Value{orig}, nil, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(result) != 1 {
		t.Fatalf("expected 1 result, got %d", len(result))
	}
	m := result[0].AsMap()
	v, _ := m.Get("a")
	vi, _ := v.AsInteger()
	if vi != 1 {
		t.Errorf("expected 1, got %d", vi)
	}
	v, _ = m.Get("b")
	vs, _ := v.AsString()
	if vs != "hello" {
		t.Errorf("expected hello, got %s", vs)
	}
}

func TestCloneHandlerList(t *testing.T) {
	orig := newList(engine.NewInteger(1), engine.NewInteger(2))
	result, err := cloneHandler([]engine.Value{orig}, nil, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	list := result[0].AsList().Slice()
	if len(list) != 2 {
		t.Fatalf("expected 2, got %d", len(list))
	}
}

// --- flatten ---

func TestFlattenDefaultHandler(t *testing.T) {
	inner := newList(engine.NewInteger(3), engine.NewInteger(4))
	data := newList(engine.NewInteger(1), engine.NewInteger(2), inner)
	result, err := flattenDefaultHandler([]engine.Value{data}, nil, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	list := result[0].AsList().Slice()
	if len(list) != 4 {
		t.Errorf("expected 4 elements, got %d", len(list))
	}
}

func TestFlattenDepthHandler(t *testing.T) {
	deep := newList(engine.NewInteger(4))
	mid := newList(engine.NewInteger(3), deep)
	data := newList(engine.NewInteger(1), mid)

	// depth=1 should only flatten one level
	result, err := flattenDepthHandler([]engine.Value{engine.NewInteger(1), data}, nil, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	list := result[0].AsList().Slice()
	// [1, 3, [4]] -> 3 elements
	if len(list) != 3 {
		t.Errorf("expected 3 elements, got %d", len(list))
	}
}

// --- getpath ---

func TestGetpathHandler(t *testing.T) {
	inner := newMap("b", engine.NewInteger(42))
	data := newMap("a", inner)
	result, err := getpathHandler([]engine.Value{engine.NewString("a.b"), data}, nil, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	ri, _ := result[0].AsInteger()
	if ri != 42 {
		t.Errorf("expected 42, got %d", ri)
	}
}

func TestGetpathHandlerTopLevel(t *testing.T) {
	data := newMap("x", engine.NewString("hello"))
	result, err := getpathHandler([]engine.Value{engine.NewString("x"), data}, nil, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	rs, _ := result[0].AsString()
	if rs != "hello" {
		t.Errorf("expected hello, got %s", rs)
	}
}

// --- setpath ---

func TestSetpathHandler(t *testing.T) {
	data := newMap("a", engine.NewInteger(1))
	result, err := setpathHandler([]engine.Value{data, engine.NewString("b"), engine.NewInteger(2)}, nil, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	m := result[0].AsMap()
	v, ok := m.Get("b")
	if !ok {
		t.Fatal("expected key 'b'")
	}
	vi, _ := v.AsInteger()
	if vi != 2 {
		t.Errorf("expected 2, got %d", vi)
	}
}

func TestSetpathHandlerNewKey(t *testing.T) {
	data := newMap("a", engine.NewInteger(1))
	result, err := setpathHandler([]engine.Value{data, engine.NewString("c"), engine.NewString("new")}, nil, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	// Verify the new key was set by reading it back with getpath
	check, err := getpathHandler([]engine.Value{engine.NewString("c"), result[0]}, nil, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	cs, _ := check[0].AsString()
	if cs != "new" {
		t.Errorf("expected new, got %s", cs)
	}
}

// --- inject ---

func TestInjectHandler(t *testing.T) {
	tmpl := newMap("greeting", engine.NewString("`name`"))
	store := newMap("name", engine.NewString("Alice"))
	result, err := injectHandler([]engine.Value{tmpl, store}, nil, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	m := result[0].AsMap()
	v, _ := m.Get("greeting")
	vs, _ := v.AsString()
	if vs != "Alice" {
		t.Errorf("expected Alice, got %s", vs)
	}
}

// --- items ---

func TestItemsHandler(t *testing.T) {
	data := newMap("x", engine.NewInteger(1), "y", engine.NewInteger(2))
	result, err := itemsHandler([]engine.Value{data}, nil, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	list := result[0].AsList().Slice()
	if len(list) != 2 {
		t.Errorf("expected 2 pairs, got %d", len(list))
	}
	// Each item is [key, value]
	pair := list[0].AsList().Slice()
	if len(pair) != 2 {
		t.Fatalf("expected pair of 2, got %d", len(pair))
	}
}

// --- join ---

func TestJoinDefaultHandler(t *testing.T) {
	data := newList(engine.NewString("a"), engine.NewString("b"), engine.NewString("c"))
	result, err := joinDefaultHandler([]engine.Value{data}, nil, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	s, _ := result[0].AsString()
	if s != "a,b,c" {
		t.Errorf("expected a,b,c got %s", s)
	}
}

func TestJoinSepHandler(t *testing.T) {
	data := newList(engine.NewString("a"), engine.NewString("b"))
	result, err := joinSepHandler([]engine.Value{engine.NewString("-"), data}, nil, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	s, _ := result[0].AsString()
	if s != "a-b" {
		t.Errorf("expected a-b got %s", s)
	}
}

// --- jsonify ---

func TestJsonifyDefaultHandler(t *testing.T) {
	data := newMap("a", engine.NewInteger(1))
	result, err := jsonifyDefaultHandler([]engine.Value{data}, nil, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	s, _ := result[0].AsString()
	if s == "" {
		t.Error("expected non-empty JSON string")
	}
}

func TestJsonifyFlagsHandler(t *testing.T) {
	data := newMap("a", engine.NewInteger(1))
	flags := newMap()
	result, err := jsonifyFlagsHandler([]engine.Value{data, flags}, nil, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	s, _ := result[0].AsString()
	if s == "" {
		t.Error("expected non-empty JSON string")
	}
}

// --- merge ---

func TestMergeHandler(t *testing.T) {
	a := newMap("x", engine.NewInteger(1))
	b := newMap("y", engine.NewInteger(2))
	result, err := mergeHandler([]engine.Value{a, b}, nil, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	m := result[0].AsMap()
	v, ok := m.Get("x")
	vi, _ := v.AsInteger()
	if !ok || vi != 1 {
		t.Error("expected x=1")
	}
	v, ok = m.Get("y")
	vi, _ = v.AsInteger()
	if !ok || vi != 2 {
		t.Error("expected y=2")
	}
}

func TestMergeHandlerOverwrite(t *testing.T) {
	a := newMap("x", engine.NewInteger(1))
	b := newMap("x", engine.NewInteger(99))
	result, err := mergeHandler([]engine.Value{a, b}, nil, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	m := result[0].AsMap()
	v, _ := m.Get("x")
	vi, _ := v.AsInteger()
	if vi != 99 {
		t.Errorf("expected 99, got %d", vi)
	}
}

// --- pad ---

func TestPadDefaultHandler(t *testing.T) {
	result, err := padDefaultHandler([]engine.Value{engine.NewString("hi")}, nil, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	s, _ := result[0].AsString()
	if len(s) == 0 {
		t.Error("expected non-empty padded string")
	}
}

func TestPadWidthHandler(t *testing.T) {
	result, err := padWidthHandler([]engine.Value{engine.NewInteger(10), engine.NewString("hi")}, nil, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	s, _ := result[0].AsString()
	if len(s) < 10 {
		t.Errorf("expected at least 10 chars, got %d", len(s))
	}
}

// --- selector ---

func TestSelectorHandler(t *testing.T) {
	children := newMap(
		"a", newMap("color", engine.NewString("red")),
		"b", newMap("color", engine.NewString("blue")),
	)
	query := newMap("color", engine.NewString("red"))
	result, err := selectorHandler([]engine.Value{children, query}, nil, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(result) != 1 {
		t.Fatalf("expected 1 result, got %d", len(result))
	}
}

// --- size ---

func TestSizeHandlerList(t *testing.T) {
	data := newList(engine.NewInteger(1), engine.NewInteger(2), engine.NewInteger(3))
	result, err := sizeHandler([]engine.Value{data}, nil, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	ri, _ := result[0].AsInteger()
	if ri != 3 {
		t.Errorf("expected 3, got %d", ri)
	}
}

func TestSizeHandlerMap(t *testing.T) {
	data := newMap("a", engine.NewInteger(1), "b", engine.NewInteger(2))
	result, err := sizeHandler([]engine.Value{data}, nil, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	ri, _ := result[0].AsInteger()
	if ri != 2 {
		t.Errorf("expected 2, got %d", ri)
	}
}

func TestSizeHandlerString(t *testing.T) {
	result, err := sizeHandler([]engine.Value{engine.NewString("hello")}, nil, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	ri, _ := result[0].AsInteger()
	if ri != 5 {
		t.Errorf("expected 5, got %d", ri)
	}
}

// --- slice ---

func TestSliceAllHandler(t *testing.T) {
	data := newList(engine.NewInteger(1), engine.NewInteger(2), engine.NewInteger(3))
	result, err := sliceAllHandler([]engine.Value{data}, nil, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	list := result[0].AsList().Slice()
	if len(list) != 3 {
		t.Errorf("expected 3, got %d", len(list))
	}
}

func TestSliceStartHandler(t *testing.T) {
	data := newList(engine.NewInteger(1), engine.NewInteger(2), engine.NewInteger(3))
	result, err := sliceStartHandler([]engine.Value{engine.NewInteger(1), data}, nil, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	list := result[0].AsList().Slice()
	if len(list) != 2 {
		t.Errorf("expected 2, got %d", len(list))
	}
}

func TestSliceStartEndHandler(t *testing.T) {
	data := newList(engine.NewInteger(1), engine.NewInteger(2), engine.NewInteger(3), engine.NewInteger(4))
	result, err := sliceStartEndHandler([]engine.Value{engine.NewInteger(1), engine.NewInteger(3), data}, nil, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	list := result[0].AsList().Slice()
	if len(list) != 2 {
		t.Errorf("expected 2, got %d", len(list))
	}
}

// --- validate ---

func TestValidateHandler(t *testing.T) {
	data := newMap("name", engine.NewString("Alice"))
	spec := newMap("name", engine.NewString("required$"))
	result, err := validateHandler([]engine.Value{data, spec}, nil, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(result) != 1 {
		t.Fatalf("expected 1 result, got %d", len(result))
	}
}

// --- walk (no callback) ---

func TestWalkHandler(t *testing.T) {
	data := newMap(
		"a", engine.NewInteger(1),
		"b", newMap("c", engine.NewInteger(2)),
	)
	result, err := walkHandler([]engine.Value{data}, nil, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	list := result[0].AsList().Slice()
	if len(list) < 2 {
		t.Errorf("expected at least 2 leaf nodes, got %d", len(list))
	}
	// Each leaf should have "path" and "value" keys
	for _, leaf := range list {
		m := leaf.AsMap()
		if _, ok := m.Get("path"); !ok {
			t.Error("missing 'path' key")
		}
		if _, ok := m.Get("value"); !ok {
			t.Error("missing 'value' key")
		}
	}
}

func TestWalkHandlerEmpty(t *testing.T) {
	data := newMap()
	result, err := walkHandler([]engine.Value{data}, nil, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	list := result[0].AsList().Slice()
	if len(list) != 0 {
		t.Errorf("expected 0 leaves for empty map, got %d", len(list))
	}
}

// --- Register functions ---

func TestRegisterFunctions(t *testing.T) {
	// After consolidation, registration is driven by a single Natives slice
	// installed via the public Register entry point. Verify each formerly
	// per-word name still resolves to a registered function with at least
	// one signature whose handler is non-nil.
	names := []string{
		"clone", "create", "filter", "flatten", "getpath", "inject",
		"items", "join", "jsonify", "list", "load", "merge", "pad",
		"remove", "selector", "setpath", "size", "slice", "transform",
		"update", "validate", "walk",
	}
	for _, name := range names {
		t.Run(name, func(t *testing.T) {
			r, err := engine.NewRegistry()
			if err != nil {
				t.Fatal(err)
			}
			Register(r)
			fn := r.Lookup(name)
			if fn == nil {
				t.Fatalf("expected word %q to be registered", name)
			}
			if len(fn.Signatures) == 0 {
				t.Error("expected at least one signature")
			}
			for i, sig := range fn.Signatures {
				if sig.Handler == nil {
					t.Errorf("signature %d has nil handler", i)
				}
			}
		})
	}
}

// --- record handlers ---

func TestListRecordAllHandler(t *testing.T) {
	recType := newMap()
	result, err := listRecordAllHandler([]engine.Value{recType}, nil, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	list := result[0].AsList().Slice()
	if len(list) != 0 {
		t.Errorf("expected 0 rows, got %d", len(list))
	}
}

func TestListRecordFilterHandler(t *testing.T) {
	recType := newMap()
	filter := newMap("city", engine.NewString("paris"))
	result, err := listRecordFilterHandler([]engine.Value{recType, filter}, nil, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	list := result[0].AsList().Slice()
	if len(list) != 0 {
		t.Errorf("expected 0 rows, got %d", len(list))
	}
}

func TestCreateRecordHandler(t *testing.T) {
	recType := newMap()
	table := makeEntityTable(nil)
	rec := newMap("id", engine.NewString("1"), "name", engine.NewString("Alice"))
	result, err := createRecordHandler([]engine.Value{recType, table, rec}, nil, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	list := result[0].AsList().Slice()
	if len(list) != 0 {
		t.Errorf("expected 0 rows, got %d", len(list))
	}
}

func TestLoadRecordHandler(t *testing.T) {
	recType := newMap()
	table := makeEntityTable(nil)
	filter := newMap("id", engine.NewString("1"))
	result, err := loadRecordHandler([]engine.Value{recType, table, filter}, nil, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	// Returns an empty map
	m := result[0].AsMap()
	if m.Len() != 0 {
		t.Errorf("expected empty map, got %d keys", m.Len())
	}
}

func TestUpdateRecordHandler(t *testing.T) {
	recType := newMap()
	table := makeEntityTable(nil)
	patch := newMap("id", engine.NewString("1"), "city", engine.NewString("Berlin"))
	result, err := updateRecordHandler([]engine.Value{recType, table, patch}, nil, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	list := result[0].AsList().Slice()
	if len(list) != 0 {
		t.Errorf("expected 0 rows, got %d", len(list))
	}
}

func TestRemoveRecordHandler(t *testing.T) {
	recType := newMap()
	table := makeEntityTable(nil)
	filter := newMap("id", engine.NewString("1"))
	result, err := removeRecordHandler([]engine.Value{recType, table, filter}, nil, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	list := result[0].AsList().Slice()
	if len(list) != 0 {
		t.Errorf("expected 0 rows, got %d", len(list))
	}
}

// --- helpers for callback-based tests ---

// makeTrueFilterFn creates an AQL function that takes one map arg and returns true.
func makeTrueFilterFn() engine.Value {
	return engine.NewFnDef(engine.FnDefInfo{
		Sigs: []engine.FnSig{
			{
				Params: []engine.FnParam{
					{Name: "item", Type: engine.TMap},
				},
				Body: []engine.Value{engine.NewBoolean(true)},
			},
		},
	})
}

func defaultRegistry(t *testing.T) *engine.Registry {
	t.Helper()
	r, err := engine.DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	return r
}

// --- filter (callback) ---

func TestFilterHandler(t *testing.T) {
	r := defaultRegistry(t)
	data := newMap("a", engine.NewInteger(1), "b", engine.NewInteger(2))
	fn := makeTrueFilterFn()
	result, err := filterHandler([]engine.Value{fn, data}, nil, nil, r)
	if err != nil {
		t.Fatal(err)
	}
	if len(result) != 1 {
		t.Fatalf("expected 1 result, got %d", len(result))
	}
	// All items should pass the filter (fn always returns true).
	// voxgigstruct.Filter on a map may return a map or list; just check non-nil.
	if result[0].VType.Equal(engine.TList) {
		list := result[0].AsList().Slice()
		if len(list) != 2 {
			t.Errorf("expected 2 entries, got %d", len(list))
		}
	} else {
		m := result[0].AsMap()
		if m.Len() != 2 {
			t.Errorf("expected 2 keys, got %d", m.Len())
		}
	}
}

// --- walk with before callback ---

// makeWalkValueFn creates an AQL function that extracts the "value" field
// from the walk node map. Body: [getpath node "value"]
func makeWalkValueFn() engine.Value {
	return engine.NewFnDef(engine.FnDefInfo{
		Sigs: []engine.FnSig{
			{
				Params: []engine.FnParam{
					{Name: "node", Type: engine.TMap},
				},
				Body: []engine.Value{
					engine.NewWord("getpath"),
					engine.NewString("value"),
					engine.NewWord("node"),
				},
			},
		},
	})
}

func TestWalkBeforeHandler(t *testing.T) {
	r := defaultRegistry(t)
	Register(r)
	data := newMap("a", engine.NewInteger(1))
	fn := makeWalkValueFn()
	result, err := walkBeforeHandler([]engine.Value{fn, data}, nil, nil, r)
	if err != nil {
		t.Fatal(err)
	}
	if len(result) != 1 {
		t.Fatalf("expected 1 result, got %d", len(result))
	}
}

// --- walk with before+after callbacks ---

func TestWalkBeforeAfterHandler(t *testing.T) {
	r := defaultRegistry(t)
	Register(r)
	data := newMap("x", engine.NewString("hello"))
	fn := makeWalkValueFn()
	result, err := walkBeforeAfterHandler([]engine.Value{fn, fn, data}, nil, nil, r)
	if err != nil {
		t.Fatal(err)
	}
	if len(result) != 1 {
		t.Fatalf("expected 1 result, got %d", len(result))
	}
}
