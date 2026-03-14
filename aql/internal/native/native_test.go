package native

import (
	"fmt"
	"testing"

	"github.com/metsitaba/voxgig-exp/aql/internal/engine"
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

// --- All / Register ---

func TestAll(t *testing.T) {
	fns := All()
	if len(fns) == 0 {
		t.Fatal("expected at least one native function")
	}
	names := make(map[string]bool)
	for _, fn := range fns {
		if fn.Name == "" {
			t.Error("empty function name")
		}
		if names[fn.Name] {
			t.Errorf("duplicate function name: %s", fn.Name)
		}
		names[fn.Name] = true
		if len(fn.Signatures) == 0 {
			t.Errorf("function %s has no signatures", fn.Name)
		}
	}
}

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
	if v.AsInteger() != 1 {
		t.Errorf("expected 1, got %d", v.AsInteger())
	}
	v, _ = m.Get("b")
	if v.AsString() != "hello" {
		t.Errorf("expected hello, got %s", v.AsString())
	}
}

func TestCloneHandlerList(t *testing.T) {
	orig := newList(engine.NewInteger(1), engine.NewInteger(2))
	result, err := cloneHandler([]engine.Value{orig}, nil, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	list := result[0].AsList()
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
	list := result[0].AsList()
	if len(list) != 4 {
		t.Errorf("expected 4 elements, got %d", len(list))
	}
}

func TestFlattenDepthHandler(t *testing.T) {
	deep := newList(engine.NewInteger(4))
	mid := newList(engine.NewInteger(3), deep)
	data := newList(engine.NewInteger(1), mid)

	// depth=1 should only flatten one level
	result, err := flattenDepthHandler([]engine.Value{data, engine.NewInteger(1)}, nil, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	list := result[0].AsList()
	// [1, 3, [4]] -> 3 elements
	if len(list) != 3 {
		t.Errorf("expected 3 elements, got %d", len(list))
	}
}

// --- getpath ---

func TestGetpathHandler(t *testing.T) {
	inner := newMap("b", engine.NewInteger(42))
	data := newMap("a", inner)
	result, err := getpathHandler([]engine.Value{data, engine.NewString("a.b")}, nil, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	if result[0].AsInteger() != 42 {
		t.Errorf("expected 42, got %d", result[0].AsInteger())
	}
}

func TestGetpathHandlerTopLevel(t *testing.T) {
	data := newMap("x", engine.NewString("hello"))
	result, err := getpathHandler([]engine.Value{data, engine.NewString("x")}, nil, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	if result[0].AsString() != "hello" {
		t.Errorf("expected hello, got %s", result[0].AsString())
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
	if v.AsInteger() != 2 {
		t.Errorf("expected 2, got %d", v.AsInteger())
	}
}

func TestSetpathHandlerNewKey(t *testing.T) {
	data := newMap("a", engine.NewInteger(1))
	result, err := setpathHandler([]engine.Value{data, engine.NewString("c"), engine.NewString("new")}, nil, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	// Verify the new key was set by reading it back with getpath
	check, err := getpathHandler([]engine.Value{result[0], engine.NewString("c")}, nil, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	if check[0].AsString() != "new" {
		t.Errorf("expected new, got %s", check[0].AsString())
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
	if v.AsString() != "Alice" {
		t.Errorf("expected Alice, got %s", v.AsString())
	}
}

// --- items ---

func TestItemsHandler(t *testing.T) {
	data := newMap("x", engine.NewInteger(1), "y", engine.NewInteger(2))
	result, err := itemsHandler([]engine.Value{data}, nil, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	list := result[0].AsList()
	if len(list) != 2 {
		t.Errorf("expected 2 pairs, got %d", len(list))
	}
	// Each item is [key, value]
	pair := list[0].AsList()
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
	s := result[0].AsString()
	if s != "a,b,c" {
		t.Errorf("expected a,b,c got %s", s)
	}
}

func TestJoinSepHandler(t *testing.T) {
	data := newList(engine.NewString("a"), engine.NewString("b"))
	result, err := joinSepHandler([]engine.Value{data, engine.NewString("-")}, nil, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	s := result[0].AsString()
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
	s := result[0].AsString()
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
	s := result[0].AsString()
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
	if !ok || v.AsInteger() != 1 {
		t.Error("expected x=1")
	}
	v, ok = m.Get("y")
	if !ok || v.AsInteger() != 2 {
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
	if v.AsInteger() != 99 {
		t.Errorf("expected 99, got %d", v.AsInteger())
	}
}

// --- pad ---

func TestPadDefaultHandler(t *testing.T) {
	result, err := padDefaultHandler([]engine.Value{engine.NewString("hi")}, nil, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	s := result[0].AsString()
	if len(s) == 0 {
		t.Error("expected non-empty padded string")
	}
}

func TestPadWidthHandler(t *testing.T) {
	result, err := padWidthHandler([]engine.Value{engine.NewString("hi"), engine.NewInteger(10)}, nil, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	s := result[0].AsString()
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
	if result[0].AsInteger() != 3 {
		t.Errorf("expected 3, got %d", result[0].AsInteger())
	}
}

func TestSizeHandlerMap(t *testing.T) {
	data := newMap("a", engine.NewInteger(1), "b", engine.NewInteger(2))
	result, err := sizeHandler([]engine.Value{data}, nil, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	if result[0].AsInteger() != 2 {
		t.Errorf("expected 2, got %d", result[0].AsInteger())
	}
}

func TestSizeHandlerString(t *testing.T) {
	result, err := sizeHandler([]engine.Value{engine.NewString("hello")}, nil, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	if result[0].AsInteger() != 5 {
		t.Errorf("expected 5, got %d", result[0].AsInteger())
	}
}

// --- slice ---

func TestSliceAllHandler(t *testing.T) {
	data := newList(engine.NewInteger(1), engine.NewInteger(2), engine.NewInteger(3))
	result, err := sliceAllHandler([]engine.Value{data}, nil, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	list := result[0].AsList()
	if len(list) != 3 {
		t.Errorf("expected 3, got %d", len(list))
	}
}

func TestSliceStartHandler(t *testing.T) {
	data := newList(engine.NewInteger(1), engine.NewInteger(2), engine.NewInteger(3))
	result, err := sliceStartHandler([]engine.Value{data, engine.NewInteger(1)}, nil, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	list := result[0].AsList()
	if len(list) != 2 {
		t.Errorf("expected 2, got %d", len(list))
	}
}

func TestSliceStartEndHandler(t *testing.T) {
	data := newList(engine.NewInteger(1), engine.NewInteger(2), engine.NewInteger(3), engine.NewInteger(4))
	result, err := sliceStartEndHandler([]engine.Value{data, engine.NewInteger(1), engine.NewInteger(3)}, nil, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	list := result[0].AsList()
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
	list := result[0].AsList()
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
	list := result[0].AsList()
	if len(list) != 0 {
		t.Errorf("expected 0 leaves for empty map, got %d", len(list))
	}
}

// --- *Func definitions ---

func TestFuncDefinitions(t *testing.T) {
	tests := []struct {
		name string
		fn   func() NativeFunc
	}{
		{"clone", cloneFunc},
		{"create", createFunc},
		{"filter", filterFunc},
		{"flatten", flattenFunc},
		{"getpath", getpathFunc},
		{"inject", injectFunc},
		{"items", itemsFunc},
		{"join", joinFunc},
		{"jsonify", jsonifyFunc},
		{"list", listFunc},
		{"load", loadFunc},
		{"merge", mergeFunc},
		{"pad", padFunc},
		{"remove", removeFunc},
		{"selector", selectorFunc},
		{"setpath", setpathFunc},
		{"size", sizeFunc},
		{"slice", sliceFunc},
		{"transform", transformFunc},
		{"update", updateFunc},
		{"validate", validateFunc},
		{"walk", walkFunc},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			nf := tt.fn()
			if nf.Name != tt.name {
				t.Errorf("expected name %s, got %s", tt.name, nf.Name)
			}
			if len(nf.Signatures) == 0 {
				t.Error("expected at least one signature")
			}
			for i, sig := range nf.Signatures {
				if sig.Handler == nil {
					t.Errorf("signature %d has nil handler", i)
				}
			}
		})
	}
}

// --- makeFullStackHandler ---

func TestMakeFullStackHandler(t *testing.T) {
	r, err := engine.NewRegistry()
	if err != nil {
		t.Fatal(err)
	}
	inner := func(args []engine.Value, ctx map[string]engine.Value, stack []engine.Value, reg *engine.Registry) ([]engine.Value, error) {
		return []engine.Value{engine.NewInteger(42)}, nil
	}
	handler := makeFullStackHandler(r, inner)

	stackBefore := []engine.Value{engine.NewString("bottom")}
	result, herr := handler(nil, stackBefore)
	if herr != nil {
		t.Fatal(herr)
	}
	// Should be [bottom, 42]
	if len(result) != 2 {
		t.Fatalf("expected 2, got %d", len(result))
	}
	if result[0].AsString() != "bottom" {
		t.Errorf("expected bottom, got %s", result[0].AsString())
	}
	if result[1].AsInteger() != 42 {
		t.Errorf("expected 42, got %d", result[1].AsInteger())
	}
}

func TestMakeFullStackHandlerError(t *testing.T) {
	r, err := engine.NewRegistry()
	if err != nil {
		t.Fatal(err)
	}
	inner := func(args []engine.Value, ctx map[string]engine.Value, stack []engine.Value, reg *engine.Registry) ([]engine.Value, error) {
		return nil, fmt.Errorf("test error")
	}
	handler := makeFullStackHandler(r, inner)

	_, herr := handler(nil, nil)
	if herr == nil {
		t.Fatal("expected error")
	}
}

// --- record handlers ---

func TestListRecordAllHandler(t *testing.T) {
	recType := newMap()
	result, err := listRecordAllHandler([]engine.Value{recType}, nil, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	list := result[0].AsList()
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
	list := result[0].AsList()
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
	list := result[0].AsList()
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
	list := result[0].AsList()
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
	list := result[0].AsList()
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
	result, err := filterHandler([]engine.Value{data, fn}, nil, nil, r)
	if err != nil {
		t.Fatal(err)
	}
	if len(result) != 1 {
		t.Fatalf("expected 1 result, got %d", len(result))
	}
	// All items should pass the filter (fn always returns true).
	// voxgigstruct.Filter on a map may return a map or list; just check non-nil.
	if result[0].VType.Equal(engine.TList) {
		list := result[0].AsList()
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
// from the walk node map. Body: [node getpath "value"]
func makeWalkValueFn() engine.Value {
	return engine.NewFnDef(engine.FnDefInfo{
		Sigs: []engine.FnSig{
			{
				Params: []engine.FnParam{
					{Name: "node", Type: engine.TMap},
				},
				Body: []engine.Value{
					engine.NewWord("node"),
					engine.NewWord("getpath"),
					engine.NewString("value"),
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
	result, err := walkBeforeHandler([]engine.Value{data, fn}, nil, nil, r)
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
	result, err := walkBeforeAfterHandler([]engine.Value{data, fn, fn}, nil, nil, r)
	if err != nil {
		t.Fatal(err)
	}
	if len(result) != 1 {
		t.Fatalf("expected 1 result, got %d", len(result))
	}
}
