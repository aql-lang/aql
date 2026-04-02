package test

import (
	"strings"
	"testing"

	"github.com/metsitaba/voxgig-exp/aql/internal/engine"
	"github.com/metsitaba/voxgig-exp/aql/internal/fileops"
	"github.com/metsitaba/voxgig-exp/aql/internal/native"
	"github.com/metsitaba/voxgig-exp/aql/internal/nativemod"
	"github.com/metsitaba/voxgig-exp/aql/internal/parser"
)

func runNativeSteps(t *testing.T, files map[string]string, steps []string) ([]engine.Value, error) {
	t.Helper()
	mem := fileops.NewMem()
	for path, content := range files {
		mem.Files[path] = []byte(content)
	}

	reg, err := engine.DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	reg.SetFileOps(mem)
	native.Register(reg)
	nativemod.InstallMathExports(reg)

	eng := engine.NewTop(reg)
	var result []engine.Value
	for _, step := range steps {
		vals, err := parser.Parse(step)
		if err != nil {
			return nil, err
		}
		result, err = eng.Run(vals)
		if err != nil {
			return nil, err
		}
	}
	return result, nil
}

// def foo [read "data.csv"]  list foo — lists all rows via forward
func TestDefListAll(t *testing.T) {
	csv := "name,age,city\nAlice,30,London\nBob,30,Paris\nCharlie,30,London\n"
	result, err := runNativeSteps(t, map[string]string{"data.csv": csv}, []string{
		`def foo [read "data.csv"]`,
		`list foo`,
	})
	if err != nil {
		t.Fatal(err)
	}
	rows := result[0].AsList().Slice()
	if len(rows) != 3 {
		t.Fatalf("expected 3 rows, got %d", len(rows))
	}
}

// def foo [read "data.csv"]  foo list {age:"30" city:"London"} — prefix form with filter
func TestDefListFilterPrefix(t *testing.T) {
	csv := "name,age,city\nAlice,30,London\nBob,30,Paris\nCharlie,30,London\n"
	result, err := runNativeSteps(t, map[string]string{"data.csv": csv}, []string{
		`def foo [read "data.csv"]`,
		`foo list {age:"30" city:"London"}`,
	})
	if err != nil {
		t.Fatal(err)
	}
	rows := result[0].AsList().Slice()
	if len(rows) != 2 {
		t.Fatalf("expected 2 filtered rows, got %d", len(rows))
	}
	names := make([]string, len(rows))
	for i, row := range rows {
		m := row.AsMap()
		v, _ := m.Get("name")
		names[i] = v.AsString()
	}
	got := strings.Join(names, ",")
	if got != "Alice,Charlie" {
		t.Errorf("got %s, want Alice,Charlie", got)
	}
}

// list (read "data.csv") {age:"30" city:"London"} — parens force evaluation
func TestDefListFilterParens(t *testing.T) {
	csv := "name,age,city\nAlice,30,London\nBob,30,Paris\nCharlie,30,London\n"
	result, err := runNativeSteps(t, map[string]string{"data.csv": csv}, []string{
		`list (read "data.csv") {age:"30" city:"London"}`,
	})
	if err != nil {
		t.Fatal(err)
	}
	rows := result[0].AsList().Slice()
	if len(rows) != 2 {
		t.Fatalf("expected 2 filtered rows, got %d", len(rows))
	}
	names := make([]string, len(rows))
	for i, row := range rows {
		m := row.AsMap()
		v, _ := m.Get("name")
		names[i] = v.AsString()
	}
	got := strings.Join(names, ",")
	if got != "Alice,Charlie" {
		t.Errorf("got %s, want Alice,Charlie", got)
	}
}

// def foo [read "data.csv"]  list (foo) {age:"30" city:"London"} — parens around def'd word
func TestDefListFilterParensDef(t *testing.T) {
	csv := "name,age,city\nAlice,30,London\nBob,30,Paris\nCharlie,30,London\n"
	result, err := runNativeSteps(t, map[string]string{"data.csv": csv}, []string{
		`def foo [read "data.csv"]`,
		`list (foo) {age:"30" city:"London"}`,
	})
	if err != nil {
		t.Fatal(err)
	}
	rows := result[0].AsList().Slice()
	if len(rows) != 2 {
		t.Fatalf("expected 2 filtered rows, got %d", len(rows))
	}
}

// def foo (read "data.csv")  list foo {age:"30" city:"London"} — parens in def evaluate eagerly
func TestDefParensListFilter(t *testing.T) {
	csv := "name,age,city\nAlice,30,London\nBob,30,Paris\nCharlie,30,London\n"
	result, err := runNativeSteps(t, map[string]string{"data.csv": csv}, []string{
		`def foo (read "data.csv")`,
		`list foo {age:"30" city:"London"}`,
	})
	if err != nil {
		t.Fatal(err)
	}
	rows := result[0].AsList().Slice()
	if len(rows) != 2 {
		t.Fatalf("expected 2 filtered rows, got %d", len(rows))
	}
	names := make([]string, len(rows))
	for i, row := range rows {
		m := row.AsMap()
		v, _ := m.Get("name")
		names[i] = v.AsString()
	}
	got := strings.Join(names, ",")
	if got != "Alice,Charlie" {
		t.Errorf("got %s, want Alice,Charlie", got)
	}
}

// def foo (read "data.csv")  list foo — parens in def, list all rows
func TestDefParensListAll(t *testing.T) {
	csv := "name,age,city\nAlice,30,London\nBob,30,Paris\nCharlie,30,London\n"
	result, err := runNativeSteps(t, map[string]string{"data.csv": csv}, []string{
		`def foo (read "data.csv")`,
		`list foo`,
	})
	if err != nil {
		t.Fatal(err)
	}
	rows := result[0].AsList().Slice()
	if len(rows) != 3 {
		t.Fatalf("expected 3 rows, got %d", len(rows))
	}
}

// def foo (read "data.csv")  create foo {id:"4" name:"Dave" city:"Berlin"}
func TestDefParensCreate(t *testing.T) {
	csv := "id,name,city\n1,Alice,London\n2,Bob,Paris\n3,Charlie,London\n"
	result, err := runNativeSteps(t, map[string]string{"data.csv": csv}, []string{
		`def foo (read "data.csv")`,
		`create foo {id:"4" name:"Dave" city:"Berlin"}`,
	})
	if err != nil {
		t.Fatal(err)
	}
	rows := result[0].AsList().Slice()
	if len(rows) != 4 {
		t.Fatalf("expected 4 rows, got %d", len(rows))
	}
	m := rows[3].AsMap()
	v, _ := m.Get("name")
	if v.AsString() != "Dave" {
		t.Errorf("expected Dave, got %s", v.AsString())
	}
}

// def foo (read "data.csv")  load foo {id:"2"}
func TestDefParensLoad(t *testing.T) {
	csv := "id,name,city\n1,Alice,London\n2,Bob,Paris\n3,Charlie,London\n"
	result, err := runNativeSteps(t, map[string]string{"data.csv": csv}, []string{
		`def foo (read "data.csv")`,
		`load foo {id:"2"}`,
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(result) != 1 {
		t.Fatalf("expected 1 result, got %d", len(result))
	}
	m := result[0].AsMap()
	v, _ := m.Get("name")
	if v.AsString() != "Bob" {
		t.Errorf("expected Bob, got %s", v.AsString())
	}
}

// def foo (read "data.csv")  update foo {id:"1" city:"Berlin"}
func TestDefParensUpdate(t *testing.T) {
	csv := "id,name,city\n1,Alice,London\n2,Bob,Paris\n"
	result, err := runNativeSteps(t, map[string]string{"data.csv": csv}, []string{
		`def foo (read "data.csv")`,
		`update foo {id:"1" city:"Berlin"}`,
	})
	if err != nil {
		t.Fatal(err)
	}
	rows := result[0].AsList().Slice()
	if len(rows) != 2 {
		t.Fatalf("expected 2 rows, got %d", len(rows))
	}
	m := rows[0].AsMap()
	city, _ := m.Get("city")
	if city.AsString() != "Berlin" {
		t.Errorf("expected Berlin, got %s", city.AsString())
	}
	name, _ := m.Get("name")
	if name.AsString() != "Alice" {
		t.Errorf("expected Alice preserved, got %s", name.AsString())
	}
}

// def foo (read "data.csv")  remove foo {id:"2"}
func TestDefParensRemove(t *testing.T) {
	csv := "id,name,city\n1,Alice,London\n2,Bob,Paris\n3,Charlie,London\n"
	result, err := runNativeSteps(t, map[string]string{"data.csv": csv}, []string{
		`def foo (read "data.csv")`,
		`remove foo {id:"2"}`,
	})
	if err != nil {
		t.Fatal(err)
	}
	rows := result[0].AsList().Slice()
	if len(rows) != 2 {
		t.Fatalf("expected 2 rows, got %d", len(rows))
	}
	for _, row := range rows {
		m := row.AsMap()
		v, _ := m.Get("name")
		if v.AsString() == "Bob" {
			t.Error("Bob should have been removed")
		}
	}
}
