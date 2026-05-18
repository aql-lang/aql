package test

import (
	"github.com/aql-lang/aql/lang/go/engine"
	"testing"
)

// {a:"hello"} transform {x:"`a`"} — injects value from data into spec
func TestTransformInject(t *testing.T) {
	bt := string(rune(96)) // backtick character
	input := `{a:"hello"} transform {x:"` + bt + `a` + bt + `"}`
	result, err := runNativeSteps(t, nil, []string{input})
	if err != nil {
		t.Fatal(err)
	}
	if len(result) != 1 {
		t.Fatalf("expected 1 result, got %d", len(result))
	}
	m, _ := engine.AsMap(result[0])
	v, ok := m.Get("x")
	if !ok {
		t.Fatal("expected key 'x' in result")
	}
	vs1, _ := engine.AsString(v)
	if vs1 != "hello" {
		t.Errorf("expected hello, got %s", vs1)
	}
}

// {a:"1"} transform {x:99} — literal spec passthrough
func TestTransformPassthrough(t *testing.T) {
	result, err := runNativeSteps(t, nil, []string{
		`{a:"1"} transform {x:99}`,
	})
	if err != nil {
		t.Fatal(err)
	}
	m, _ := engine.AsMap(result[0])
	v, _ := m.Get("x")
	vi1, _ := engine.AsInteger(v)
	if vi1 != 99 {
		t.Errorf("expected 99, got %d", vi1)
	}
}

// transform with nested path: {a:{b:42}} transform {val:"`a.b`"}
func TestTransformNestedPath(t *testing.T) {
	bt := string(rune(96)) // backtick character
	input := `{a:{b:42}} transform {val:"` + bt + `a.b` + bt + `"}`
	result, err := runNativeSteps(t, nil, []string{input})
	if err != nil {
		t.Fatal(err)
	}
	m, _ := engine.AsMap(result[0])
	v, _ := m.Get("val")
	vi2, _ := engine.AsInteger(v)
	if vi2 != 42 {
		t.Errorf("expected 42, got %d", vi2)
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
	m, _ := engine.AsMap(result[0])
	v, ok := m.Get("greeting")
	if !ok {
		t.Fatal("expected key 'greeting' in result")
	}
	vs2, _ := engine.AsString(v)
	if vs2 != "Alice" {
		t.Errorf("expected Alice, got %s", vs2)
	}
}
