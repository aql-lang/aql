package test

import (
	"strings"
	"testing"

	"github.com/aql-lang/aql/eng/go/parser"
	"github.com/aql-lang/aql/lang/go/engine"
	"github.com/aql-lang/aql/lang/go/internal/fileops"
	"github.com/aql-lang/aql/lang/go/internal/nativemod"
	"github.com/aql-lang/aql/lang/go/native"
)

func runNativeSteps(t *testing.T, files map[string]string, steps []string) ([]engine.Value, error) {
	t.Helper()
	mem := fileops.NewMem()
	for path, content := range files {
		mem.Files[path] = []byte(content)
	}

	reg, err := engine.DefaultRegistry(native.Register)
	if err != nil {
		t.Fatal(err)
	}
	engine.SetHostFileOps(reg, mem)
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
	_lst, _ := engine.AsList(result[0])
	rows := _lst.Slice()
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
	_lst, _ := engine.AsList(result[0])
	rows := _lst.Slice()
	if len(rows) != 2 {
		t.Fatalf("expected 2 filtered rows, got %d", len(rows))
	}
	names := make([]string, len(rows))
	for i, row := range rows {
		m, _ := engine.AsMap(row)
		v, _ := m.Get("name")
		ns, _ := engine.AsString(v)
		names[i] = ns
	}
	got := strings.Join(names, ",")
	if got != "Alice,Charlie" {
		t.Errorf("got %s, want Alice,Charlie", got)
	}
}

// (read "data.csv") list {age:"30" city:"London"} — parens force evaluation
func TestDefListFilterParens(t *testing.T) {
	csv := "name,age,city\nAlice,30,London\nBob,30,Paris\nCharlie,30,London\n"
	result, err := runNativeSteps(t, map[string]string{"data.csv": csv}, []string{
		`(read "data.csv") list {age:"30" city:"London"}`,
	})
	if err != nil {
		t.Fatal(err)
	}
	_lst, _ := engine.AsList(result[0])
	rows := _lst.Slice()
	if len(rows) != 2 {
		t.Fatalf("expected 2 filtered rows, got %d", len(rows))
	}
	names := make([]string, len(rows))
	for i, row := range rows {
		m, _ := engine.AsMap(row)
		v, _ := m.Get("name")
		ns, _ := engine.AsString(v)
		names[i] = ns
	}
	got := strings.Join(names, ",")
	if got != "Alice,Charlie" {
		t.Errorf("got %s, want Alice,Charlie", got)
	}
}

// def foo [read "data.csv"]  (foo) list {age:"30" city:"London"} — parens around def'd word
func TestDefListFilterParensDef(t *testing.T) {
	csv := "name,age,city\nAlice,30,London\nBob,30,Paris\nCharlie,30,London\n"
	result, err := runNativeSteps(t, map[string]string{"data.csv": csv}, []string{
		`def foo [read "data.csv"]`,
		`(foo) list {age:"30" city:"London"}`,
	})
	if err != nil {
		t.Fatal(err)
	}
	_lst, _ := engine.AsList(result[0])
	rows := _lst.Slice()
	if len(rows) != 2 {
		t.Fatalf("expected 2 filtered rows, got %d", len(rows))
	}
}

// def foo (read "data.csv")  foo list {age:"30" city:"London"} — parens in def evaluate eagerly
func TestDefParensListFilter(t *testing.T) {
	csv := "name,age,city\nAlice,30,London\nBob,30,Paris\nCharlie,30,London\n"
	result, err := runNativeSteps(t, map[string]string{"data.csv": csv}, []string{
		`def foo (read "data.csv")`,
		`foo list {age:"30" city:"London"}`,
	})
	if err != nil {
		t.Fatal(err)
	}
	_lst, _ := engine.AsList(result[0])
	rows := _lst.Slice()
	if len(rows) != 2 {
		t.Fatalf("expected 2 filtered rows, got %d", len(rows))
	}
	names := make([]string, len(rows))
	for i, row := range rows {
		m, _ := engine.AsMap(row)
		v, _ := m.Get("name")
		ns, _ := engine.AsString(v)
		names[i] = ns
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
	_lst, _ := engine.AsList(result[0])
	rows := _lst.Slice()
	if len(rows) != 3 {
		t.Fatalf("expected 3 rows, got %d", len(rows))
	}
}

// def foo (read "data.csv")  foo create {id:"4" name:"Dave" city:"Berlin"}
func TestDefParensCreate(t *testing.T) {
	csv := "id,name,city\n1,Alice,London\n2,Bob,Paris\n3,Charlie,London\n"
	result, err := runNativeSteps(t, map[string]string{"data.csv": csv}, []string{
		`def foo (read "data.csv")`,
		`foo create {id:"4" name:"Dave" city:"Berlin"}`,
	})
	if err != nil {
		t.Fatal(err)
	}
	_lst, _ := engine.AsList(result[0])
	rows := _lst.Slice()
	if len(rows) != 4 {
		t.Fatalf("expected 4 rows, got %d", len(rows))
	}
	m, _ := engine.AsMap(rows[3])
	v, _ := m.Get("name")
	vs1, _ := engine.AsString(v)
	if vs1 != "Dave" {
		t.Errorf("expected Dave, got %s", vs1)
	}
}

// def foo (read "data.csv")  foo load {id:"2"}
func TestDefParensLoad(t *testing.T) {
	csv := "id,name,city\n1,Alice,London\n2,Bob,Paris\n3,Charlie,London\n"
	result, err := runNativeSteps(t, map[string]string{"data.csv": csv}, []string{
		`def foo (read "data.csv")`,
		`foo load {id:"2"}`,
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(result) != 1 {
		t.Fatalf("expected 1 result, got %d", len(result))
	}
	m, _ := engine.AsMap(result[0])
	v, _ := m.Get("name")
	vs2, _ := engine.AsString(v)
	if vs2 != "Bob" {
		t.Errorf("expected Bob, got %s", vs2)
	}
}

// def foo (read "data.csv")  foo update {id:"1" city:"Berlin"}
func TestDefParensUpdate(t *testing.T) {
	csv := "id,name,city\n1,Alice,London\n2,Bob,Paris\n"
	result, err := runNativeSteps(t, map[string]string{"data.csv": csv}, []string{
		`def foo (read "data.csv")`,
		`foo update {id:"1" city:"Berlin"}`,
	})
	if err != nil {
		t.Fatal(err)
	}
	_lst, _ := engine.AsList(result[0])
	rows := _lst.Slice()
	if len(rows) != 2 {
		t.Fatalf("expected 2 rows, got %d", len(rows))
	}
	m, _ := engine.AsMap(rows[0])
	city, _ := m.Get("city")
	cityS, _ := engine.AsString(city)
	if cityS != "Berlin" {
		t.Errorf("expected Berlin, got %s", cityS)
	}
	name, _ := m.Get("name")
	nameS, _ := engine.AsString(name)
	if nameS != "Alice" {
		t.Errorf("expected Alice preserved, got %s", nameS)
	}
}

// def foo (read "data.csv")  foo remove {id:"2"}
func TestDefParensRemove(t *testing.T) {
	csv := "id,name,city\n1,Alice,London\n2,Bob,Paris\n3,Charlie,London\n"
	result, err := runNativeSteps(t, map[string]string{"data.csv": csv}, []string{
		`def foo (read "data.csv")`,
		`foo remove {id:"2"}`,
	})
	if err != nil {
		t.Fatal(err)
	}
	_lst, _ := engine.AsList(result[0])
	rows := _lst.Slice()
	if len(rows) != 2 {
		t.Fatalf("expected 2 rows, got %d", len(rows))
	}
	for _, row := range rows {
		m, _ := engine.AsMap(row)
		v, _ := m.Get("name")
		vs3, _ := engine.AsString(v)
		if vs3 == "Bob" {
			t.Error("Bob should have been removed")
		}
	}
}
