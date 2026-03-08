package test

import (
	"testing"
)

// transform {a:"hello"} {x:"`a`"} — injects value from data into spec
func TestTransformInject(t *testing.T) {
	bt := string(rune(96)) // backtick character
	input := `transform {a:"hello"} {x:"` + bt + `a` + bt + `"}`
	result, err := runNativeSteps(t, nil, []string{input})
	if err != nil {
		t.Fatal(err)
	}
	if len(result) != 1 {
		t.Fatalf("expected 1 result, got %d", len(result))
	}
	m := result[0].AsMap()
	v, ok := m.Get("x")
	if !ok {
		t.Fatal("expected key 'x' in result")
	}
	if v.AsString() != "hello" {
		t.Errorf("expected hello, got %s", v.AsString())
	}
}

// transform {a:"1"} {x:99} — literal spec passthrough
func TestTransformPassthrough(t *testing.T) {
	result, err := runNativeSteps(t, nil, []string{
		`transform {a:"1"} {x:99}`,
	})
	if err != nil {
		t.Fatal(err)
	}
	m := result[0].AsMap()
	v, _ := m.Get("x")
	if v.AsInteger() != 99 {
		t.Errorf("expected 99, got %d", v.AsInteger())
	}
}

// transform with nested path: {a:{b:42}} {val:"`a.b`"}
func TestTransformNestedPath(t *testing.T) {
	bt := string(rune(96)) // backtick character
	input := `transform {a:{b:42}} {val:"` + bt + `a.b` + bt + `"}`
	result, err := runNativeSteps(t, nil, []string{input})
	if err != nil {
		t.Fatal(err)
	}
	m := result[0].AsMap()
	v, _ := m.Get("val")
	if v.AsInteger() != 42 {
		t.Errorf("expected 42, got %d", v.AsInteger())
	}
}

// foo load {id:"1"} transform {greeting:"`name`"} — chained prefix
func TestDefTransformWithLoad(t *testing.T) {
	csv := "id,name,city\n1,Alice,London\n2,Bob,Paris\n"
	bt := string(rune(96)) // backtick character
	result, err := runNativeSteps(t, map[string]string{"data.csv": csv}, []string{
		`def foo (read "data.csv")`,
		`foo load {id:"1"} transform {greeting:"` + bt + `name` + bt + `"}`,
	})
	if err != nil {
		t.Fatal(err)
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
