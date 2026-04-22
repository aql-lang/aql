package test

import (
	"strings"
	"testing"

	"github.com/metsitaba/voxgig-exp/aql/internal/engine"
	"github.com/metsitaba/voxgig-exp/aql/internal/fileops"
	"github.com/metsitaba/voxgig-exp/aql/internal/native"
	"github.com/metsitaba/voxgig-exp/aql/internal/parser"
)

// runNativeWithFiles creates a registry with native functions and in-memory files.
func runNativeWithFiles(t *testing.T, files map[string]string, expr string) ([]engine.Value, error) {
	t.Helper()
	mem := fileops.NewMem()
	for path, content := range files {
		mem.Files[path] = []byte(content)
	}

	reg, err := engine.DefaultRegistry(native.Register)
	if err != nil {
		t.Fatal(err)
	}
	reg.SetFileOps(mem)
	native.Register(reg)

	values, err := parser.Parse(expr)
	if err != nil {
		return nil, err
	}

	eng := engine.NewTop(reg)
	return eng.Run(values)
}

// --- list: load CSV and list all rows ---

func TestListAllFromCSV(t *testing.T) {
	csv := "name,age,city\nAlice,30,London\nBob,25,Paris\nCharlie,35,Tokyo\n"

	result, err := runNativeWithFiles(t, map[string]string{
		"people.csv": csv,
	}, `read "people.csv" list`)
	if err != nil {
		t.Fatal(err)
	}
	if len(result) != 1 {
		t.Fatalf("expected 1 value, got %d", len(result))
	}

	rows := result[0].AsList().Slice()
	if len(rows) != 3 {
		t.Fatalf("expected 3 rows, got %d", len(rows))
	}

	names := make(map[string]bool)
	for _, row := range rows {
		m := row.AsMap()
		v, _ := m.Get("name")
		vs, _ := v.AsString()
		names[vs] = true
	}
	for _, want := range []string{"Alice", "Bob", "Charlie"} {
		if !names[want] {
			t.Errorf("missing name %q in list result", want)
		}
	}
}

// --- list: load CSV and filter by matching fields ---

func TestListFilterFromCSV(t *testing.T) {
	csv := "name,age,city\nAlice,30,London\nBob,25,Paris\nCharlie,35,London\n"

	result, err := runNativeWithFiles(t, map[string]string{
		"people.csv": csv,
	}, `read "people.csv" list {city:"London"}`)
	if err != nil {
		t.Fatal(err)
	}
	if len(result) != 1 {
		t.Fatalf("expected 1 value, got %d", len(result))
	}

	rows := result[0].AsList().Slice()
	if len(rows) != 2 {
		t.Fatalf("expected 2 matching rows, got %d", len(rows))
	}

	for _, row := range rows {
		m := row.AsMap()
		cityVal, _ := m.Get("city")
		cityStr, _ := cityVal.AsString()
		if cityStr != "London" {
			t.Errorf("expected city London, got %s", cityStr)
		}
	}
}

// --- list: filter with no matches returns empty list ---

func TestListFilterNoMatches(t *testing.T) {
	csv := "name,age\nAlice,30\nBob,25\n"

	result, err := runNativeWithFiles(t, map[string]string{
		"data.csv": csv,
	}, `read "data.csv" list {name:"Nobody"}`)
	if err != nil {
		t.Fatal(err)
	}

	rows := result[0].AsList().Slice()
	if len(rows) != 0 {
		t.Errorf("expected 0 rows, got %d", len(rows))
	}
}

// --- list: filter on multiple fields ---

func TestListFilterMultipleFields(t *testing.T) {
	csv := "name,age,city\nAlice,30,London\nBob,30,Paris\nCharlie,30,London\n"

	result, err := runNativeWithFiles(t, map[string]string{
		"data.csv": csv,
	}, `read "data.csv" list {age:"30" city:"London"}`)
	if err != nil {
		t.Fatal(err)
	}

	rows := result[0].AsList().Slice()
	if len(rows) != 2 {
		t.Fatalf("expected 2 rows, got %d", len(rows))
	}

	names := make([]string, len(rows))
	for i, row := range rows {
		m := row.AsMap()
		v, _ := m.Get("name")
		vs, _ := v.AsString()
		names[i] = vs
	}
	got := strings.Join(names, ",")
	if got != "Alice,Charlie" {
		t.Errorf("got names %s, want Alice,Charlie", got)
	}
}
