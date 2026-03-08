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
	leaves := result[0].AsList()
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
	leaves := result[0].AsList()
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
