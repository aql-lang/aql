package test

import (
	"testing"
)

// --- merge ---

func TestMergeMaps(t *testing.T) {
	result, err := runNativeSteps(t, nil, []string{
		`merge {a:1 b:2} {b:3 c:4}`,
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(result) != 1 {
		t.Fatalf("expected 1 result, got %d", len(result))
	}
	m := result[0].AsMap()
	a, _ := m.Get("a")
	b, _ := m.Get("b")
	c, _ := m.Get("c")
	if a.AsInteger() != 1 {
		t.Errorf("expected a=1, got %d", a.AsInteger())
	}
	if b.AsInteger() != 3 {
		t.Errorf("expected b=3 (overridden), got %d", b.AsInteger())
	}
	if c.AsInteger() != 4 {
		t.Errorf("expected c=4, got %d", c.AsInteger())
	}
}

func TestMergeNested(t *testing.T) {
	result, err := runNativeSteps(t, nil, []string{
		`merge {a:{x:1 y:2}} {a:{y:3 z:4}}`,
	})
	if err != nil {
		t.Fatal(err)
	}
	m := result[0].AsMap()
	inner, _ := m.Get("a")
	im := inner.AsMap()
	x, _ := im.Get("x")
	y, _ := im.Get("y")
	z, _ := im.Get("z")
	if x.AsInteger() != 1 {
		t.Errorf("expected x=1, got %d", x.AsInteger())
	}
	if y.AsInteger() != 3 {
		t.Errorf("expected y=3 (overridden), got %d", y.AsInteger())
	}
	if z.AsInteger() != 4 {
		t.Errorf("expected z=4, got %d", z.AsInteger())
	}
}

// --- merge list+map ---

func TestMergeListMap(t *testing.T) {
	result, err := runNativeSteps(t, nil, []string{
		`["a","b","c"] merge {1:"d"}`,
	})
	if err != nil {
		t.Fatal(err)
	}
	list := result[0].AsList().Slice()
	if len(list) != 3 {
		t.Fatalf("expected 3 elements, got %d", len(list))
	}
	if list[0].AsString() != "a" {
		t.Errorf("expected [0]=a, got %s", list[0].AsString())
	}
	if list[1].AsString() != "d" {
		t.Errorf("expected [1]=d, got %s", list[1].AsString())
	}
	if list[2].AsString() != "c" {
		t.Errorf("expected [2]=c, got %s", list[2].AsString())
	}
}

func TestMergeMapList(t *testing.T) {
	result, err := runNativeSteps(t, nil, []string{
		`{3:"d"} merge ["a","b","c"]`,
	})
	if err != nil {
		t.Fatal(err)
	}
	list := result[0].AsList().Slice()
	if len(list) != 4 {
		t.Fatalf("expected 4 elements, got %d", len(list))
	}
	if list[3].AsString() != "d" {
		t.Errorf("expected [3]=d, got %s", list[3].AsString())
	}
}

func TestMergeMapListIgnoreNonInt(t *testing.T) {
	result, err := runNativeSteps(t, nil, []string{
		`{x:"X",y:"Y"} merge ["a","b"]`,
	})
	if err != nil {
		t.Fatal(err)
	}
	list := result[0].AsList().Slice()
	if len(list) != 2 {
		t.Fatalf("expected 2 elements, got %d", len(list))
	}
}

// --- push/pop/shift/unshift ---

func TestPush(t *testing.T) {
	result, err := runNativeSteps(t, nil, []string{
		`push ["a","b"] "c"`,
	})
	if err != nil {
		t.Fatal(err)
	}
	list := result[0].AsList().Slice()
	if len(list) != 3 || list[2].AsString() != "c" {
		t.Errorf("expected [a,b,c], got %v", result[0].String())
	}
}

func TestPushSpread(t *testing.T) {
	result, err := runNativeSteps(t, nil, []string{
		`push ["a","b"] ["c","d"]`,
	})
	if err != nil {
		t.Fatal(err)
	}
	list := result[0].AsList().Slice()
	if len(list) != 4 {
		t.Errorf("expected 4 elements, got %d", len(list))
	}
}

func TestPop(t *testing.T) {
	result, err := runNativeSteps(t, nil, []string{
		`pop ["a","b","c"]`,
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(result) != 2 {
		t.Fatalf("expected 2 results (list + popped), got %d", len(result))
	}
	list := result[0].AsList().Slice()
	if len(list) != 2 {
		t.Errorf("expected list of 2, got %d", len(list))
	}
	if result[1].AsString() != "c" {
		t.Errorf("expected popped 'c', got %s", result[1].AsString())
	}
}

func TestUnshift(t *testing.T) {
	result, err := runNativeSteps(t, nil, []string{
		`unshift ["a","b"] "c"`,
	})
	if err != nil {
		t.Fatal(err)
	}
	list := result[0].AsList().Slice()
	if len(list) != 3 || list[0].AsString() != "c" {
		t.Errorf("expected [c,a,b], got %v", result[0].String())
	}
}

func TestUnshiftSpread(t *testing.T) {
	result, err := runNativeSteps(t, nil, []string{
		`unshift ["a","b"] ["c","d"]`,
	})
	if err != nil {
		t.Fatal(err)
	}
	list := result[0].AsList().Slice()
	if len(list) != 4 || list[0].AsString() != "c" || list[1].AsString() != "d" {
		t.Errorf("expected [c,d,a,b], got %v", result[0].String())
	}
}

func TestShift(t *testing.T) {
	result, err := runNativeSteps(t, nil, []string{
		`shift ["a","b","c"]`,
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(result) != 2 {
		t.Fatalf("expected 2 results, got %d", len(result))
	}
	list := result[0].AsList().Slice()
	if len(list) != 2 {
		t.Errorf("expected list of 2, got %d", len(list))
	}
	if result[1].AsString() != "a" {
		t.Errorf("expected shifted 'a', got %s", result[1].AsString())
	}
}

// --- getpath ---

func TestGetpathSimple(t *testing.T) {
	result, err := runNativeSteps(t, nil, []string{
		`getpath {a:{b:42}} "a.b"`,
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(result) != 1 {
		t.Fatalf("expected 1 result, got %d", len(result))
	}
	if result[0].AsInteger() != 42 {
		t.Errorf("expected 42, got %d", result[0].AsInteger())
	}
}

func TestGetpathTopLevel(t *testing.T) {
	result, err := runNativeSteps(t, nil, []string{
		`getpath {name:"Alice"} "name"`,
	})
	if err != nil {
		t.Fatal(err)
	}
	if result[0].AsString() != "Alice" {
		t.Errorf("expected Alice, got %s", result[0].AsString())
	}
}

// --- setpath ---

func TestSetpathSimple(t *testing.T) {
	result, err := runNativeSteps(t, nil, []string{
		`setpath {a:1} "b" 99`,
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(result) != 1 {
		t.Fatalf("expected 1 result, got %d", len(result))
	}
	m := result[0].AsMap()
	a, _ := m.Get("a")
	b, _ := m.Get("b")
	if a.AsInteger() != 1 {
		t.Errorf("expected a=1, got %d", a.AsInteger())
	}
	if b.AsInteger() != 99 {
		t.Errorf("expected b=99, got %d", b.AsInteger())
	}
}

func TestSetpathNewKey(t *testing.T) {
	result, err := runNativeSteps(t, nil, []string{
		`setpath {a:1} "b" 2`,
	})
	if err != nil {
		t.Fatal(err)
	}
	m := result[0].AsMap()
	a, _ := m.Get("a")
	b, _ := m.Get("b")
	if a.AsInteger() != 1 {
		t.Errorf("expected a=1, got %d", a.AsInteger())
	}
	if b.AsInteger() != 2 {
		t.Errorf("expected b=2, got %d", b.AsInteger())
	}
}

// --- clone ---

func TestCloneMap(t *testing.T) {
	result, err := runNativeSteps(t, nil, []string{
		`{a:1 b:2} clone`,
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(result) != 1 {
		t.Fatalf("expected 1 result, got %d", len(result))
	}
	m := result[0].AsMap()
	a, _ := m.Get("a")
	b, _ := m.Get("b")
	if a.AsInteger() != 1 {
		t.Errorf("expected a=1, got %d", a.AsInteger())
	}
	if b.AsInteger() != 2 {
		t.Errorf("expected b=2, got %d", b.AsInteger())
	}
}

// --- inject ---

func TestInjectPaths(t *testing.T) {
	bt := string(rune(96)) // backtick character
	input := `inject {greeting:"` + bt + `name` + bt + `"} {name:"Alice"}`
	result, err := runNativeSteps(t, nil, []string{input})
	if err != nil {
		t.Fatal(err)
	}
	if len(result) != 1 {
		t.Fatalf("expected 1 result, got %d", len(result))
	}
	m := result[0].AsMap()
	v, ok := m.Get("greeting")
	if !ok {
		t.Fatal("expected key 'greeting' in result")
	}
	if v.AsString() != "Alice" {
		t.Errorf("expected Alice, got %s", v.AsString())
	}
}

// --- validate ---

func TestValidateReturnsSpec(t *testing.T) {
	result, err := runNativeSteps(t, nil, []string{
		`validate {name:"Alice" age:30} {name:"$STRING" age:"$NUMBER"}`,
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(result) != 1 {
		t.Fatalf("expected 1 result, got %d", len(result))
	}
	m := result[0].AsMap()
	name, ok := m.Get("name")
	if !ok {
		t.Fatal("expected key 'name' in result")
	}
	if name.AsString() != "$STRING" {
		t.Errorf("expected $STRING, got %s", name.AsString())
	}
	age, ok := m.Get("age")
	if !ok {
		t.Fatal("expected key 'age' in result")
	}
	if age.AsString() != "$NUMBER" {
		t.Errorf("expected $NUMBER, got %s", age.AsString())
	}
}

// --- walk ---

func TestWalkFlat(t *testing.T) {
	result, err := runNativeSteps(t, nil, []string{
		`{a:1 b:"hello"} walk`,
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(result) != 1 {
		t.Fatalf("expected 1 result, got %d", len(result))
	}
	leaves := result[0].AsList().Slice()
	if len(leaves) != 2 {
		t.Fatalf("expected 2 leaves, got %d", len(leaves))
	}

	paths := make(map[string]string)
	for _, leaf := range leaves {
		m := leaf.AsMap()
		p, _ := m.Get("path")
		v, _ := m.Get("value")
		paths[p.AsString()] = v.String()
	}
	if _, ok := paths["a"]; !ok {
		t.Error("missing path 'a'")
	}
	if _, ok := paths["b"]; !ok {
		t.Error("missing path 'b'")
	}
}

func TestWalkNested(t *testing.T) {
	result, err := runNativeSteps(t, nil, []string{
		`{a:{x:1 y:2} b:3} walk`,
	})
	if err != nil {
		t.Fatal(err)
	}
	leaves := result[0].AsList().Slice()
	if len(leaves) != 3 {
		t.Fatalf("expected 3 leaves, got %d", len(leaves))
	}

	paths := make(map[string]bool)
	for _, leaf := range leaves {
		m := leaf.AsMap()
		p, _ := m.Get("path")
		paths[p.AsString()] = true
	}
	for _, want := range []string{"a.x", "a.y", "b"} {
		if !paths[want] {
			t.Errorf("missing path %q", want)
		}
	}
}

// --- walk with before callback ---

func TestWalkBeforeIdentity(t *testing.T) {
	// AQL: {a:1 b:2} (fn [[m:Map] [Any] [m.value]]) walk
	// Before callback returns m.value (identity) — tree is preserved unchanged.
	// The before callback is called pre-order on every node; returning m.value
	// leaves each node as-is, so the walk produces the original structure.
	result, err := runNativeSteps(t, nil, []string{
		`{a:1 b:2} (fn [[m:Map] [Any] [m.value]]) walk`,
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(result) != 1 {
		t.Fatalf("expected 1 result, got %d", len(result))
	}
	m := result[0].AsMap()
	a, _ := m.Get("a")
	b, _ := m.Get("b")
	if a.AsInteger() != 1 {
		t.Errorf("expected a=1, got %v", a)
	}
	if b.AsInteger() != 2 {
		t.Errorf("expected b=2, got %v", b)
	}
}

func TestWalkBeforeIdentityNested(t *testing.T) {
	// AQL: {a:{x:1 y:2} b:3} (fn [[m:Map] [Any] [m.value]]) walk
	// Identity before callback on a nested structure — entire tree preserved.
	result, err := runNativeSteps(t, nil, []string{
		`{a:{x:1 y:2} b:3} (fn [[m:Map] [Any] [m.value]]) walk`,
	})
	if err != nil {
		t.Fatal(err)
	}
	m := result[0].AsMap()
	inner, _ := m.Get("a")
	im := inner.AsMap()
	x, _ := im.Get("x")
	y, _ := im.Get("y")
	b, _ := m.Get("b")
	if x.AsInteger() != 1 {
		t.Errorf("expected x=1, got %v", x)
	}
	if y.AsInteger() != 2 {
		t.Errorf("expected y=2, got %v", y)
	}
	if b.AsInteger() != 3 {
		t.Errorf("expected b=3, got %v", b)
	}
}

func TestWalkBeforeReplace(t *testing.T) {
	// AQL: {a:1 b:2} (fn [[m:Map] [Any] [99]]) walk
	// Before callback replaces the root node with 99 (a non-node value).
	// Since 99 is not a map/list, walk does NOT descend into children.
	// This demonstrates that the before callback controls traversal:
	// replacing a node with a scalar stops descent into that subtree.
	result, err := runNativeSteps(t, nil, []string{
		`{a:1 b:2} (fn [[m:Map] [Any] [99]]) walk`,
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(result) != 1 {
		t.Fatalf("expected 1 result, got %d", len(result))
	}
	if result[0].AsInteger() != 99 {
		t.Errorf("expected 99, got %v", result[0])
	}
}

func TestWalkBeforeReturnPath(t *testing.T) {
	// AQL: {a:1 b:2} (fn [[m:Map] [Any] [m.path]]) walk
	// Before callback returns the path string for every node.
	// The root path is "" (empty string), which replaces the root map.
	// Since a string is not a node, descent stops — result is "".
	result, err := runNativeSteps(t, nil, []string{
		`{a:1 b:2} (fn [[m:Map] [Any] [m.path]]) walk`,
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(result) != 1 {
		t.Fatalf("expected 1 result, got %d", len(result))
	}
	if result[0].AsString() != "" {
		t.Errorf("expected empty string (root path), got %q", result[0].AsString())
	}
}

// --- walk with before AND after callbacks ---

func TestWalkBeforeAfterIdentity(t *testing.T) {
	// AQL: {a:1 b:2} (fn [[m:Map] [Any] [m.value]]) (fn [[m:Map] [Any] [m.value]]) walk
	// Both before and after return m.value (identity) — tree is preserved.
	result, err := runNativeSteps(t, nil, []string{
		`{a:1 b:2} (fn [[m:Map] [Any] [m.value]]) (fn [[m:Map] [Any] [m.value]]) walk`,
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(result) != 1 {
		t.Fatalf("expected 1 result, got %d", len(result))
	}
	m := result[0].AsMap()
	a, _ := m.Get("a")
	b, _ := m.Get("b")
	if a.AsInteger() != 1 {
		t.Errorf("expected a=1, got %v", a)
	}
	if b.AsInteger() != 2 {
		t.Errorf("expected b=2, got %v", b)
	}
}

func TestWalkBeforeAfterPostOrder(t *testing.T) {
	// AQL: {a:1 b:2} (fn [[m:Map] [Any] [m.value]]) (fn [[m:Map] [Any] [99]]) walk
	// Before callback is identity (allows descent), after callback replaces
	// every node with 99 (post-order). Processing order:
	//   1. before(root) → {a:1 b:2} (identity, descent proceeds)
	//   2. before(a=1) → 1 (identity)
	//   3. after(a=1) → 99
	//   4. before(b=2) → 2 (identity)
	//   5. after(b=2) → 99
	//   6. after(root={a:99 b:99}) → 99
	// Final result: 99
	result, err := runNativeSteps(t, nil, []string{
		`{a:1 b:2} (fn [[m:Map] [Any] [m.value]]) (fn [[m:Map] [Any] [99]]) walk`,
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(result) != 1 {
		t.Fatalf("expected 1 result, got %d", len(result))
	}
	if result[0].AsInteger() != 99 {
		t.Errorf("expected 99 (after replaces all), got %v", result[0])
	}
}

func TestWalkBeforeAfterNested(t *testing.T) {
	// AQL: {a:{x:1 y:2} b:3}
	//        (fn [[m:Map] [Any] [m.value]])
	//        (fn [[m:Map] [Any] [m.value]]) walk
	// Both callbacks are identity — nested tree preserved through full traversal.
	result, err := runNativeSteps(t, nil, []string{
		`{a:{x:1 y:2} b:3} (fn [[m:Map] [Any] [m.value]]) (fn [[m:Map] [Any] [m.value]]) walk`,
	})
	if err != nil {
		t.Fatal(err)
	}
	m := result[0].AsMap()
	inner, _ := m.Get("a")
	im := inner.AsMap()
	x, _ := im.Get("x")
	y, _ := im.Get("y")
	b, _ := m.Get("b")
	if x.AsInteger() != 1 {
		t.Errorf("expected x=1, got %v", x)
	}
	if y.AsInteger() != 2 {
		t.Errorf("expected y=2, got %v", y)
	}
	if b.AsInteger() != 3 {
		t.Errorf("expected b=3, got %v", b)
	}
}
