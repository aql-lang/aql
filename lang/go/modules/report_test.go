package modules

import (
	"strings"
	"testing"

	"github.com/aql-lang/aql/eng/go/parser"
	"github.com/aql-lang/aql/lang/go/native"
)

// reportRegistry returns a registry with the aql:report module
// installed as defs (mirrors what `import "aql:report"` would do).
func reportRegistry(t *testing.T) *native.Registry {
	t.Helper()
	r, err := native.DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	r.SetParseFunc(parser.Parse)
	desc, err := BuildReportModule(r)
	if err != nil {
		t.Fatal(err)
	}
	for name, exp := range desc.Exports {
		r.Defs.Push(name, native.NewMap(exp))
	}
	return r
}

func runReportAQL(t *testing.T, r *native.Registry, src string) []native.Value {
	t.Helper()
	values, err := parser.Parse(src)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	out, err := native.NewTop(r).Run(values)
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	return out
}

func TestReportValueScalar(t *testing.T) {
	r := reportRegistry(t)
	result := runReportAQL(t, r, `42 report.value`)
	s, _ := native.AsString(result[0])
	if s != "42" {
		t.Errorf("report.value 42 = %q, want %q", s, "42")
	}
}

func TestReportRecordVerticalLayout(t *testing.T) {
	r := reportRegistry(t)
	result := runReportAQL(t, r, `{name:"alice" age:30} report.record`)
	s, _ := native.AsString(result[0])
	if !strings.Contains(s, "name : alice") {
		t.Errorf("missing aligned field; got:\n%s", s)
	}
	if !strings.Contains(s, "age  : 30") {
		t.Errorf("alignment broken; got:\n%s", s)
	}
}

func TestReportTableFromListOfMaps(t *testing.T) {
	r := reportRegistry(t)
	result := runReportAQL(t, r, `[{a:1 b:2} {a:3 b:4}] report.table`)
	s, _ := native.AsString(result[0])
	if !strings.Contains(s, "a | b") {
		t.Errorf("missing header; got:\n%s", s)
	}
	if !strings.Contains(s, "1 | 2") || !strings.Contains(s, "3 | 4") {
		t.Errorf("missing rows; got:\n%s", s)
	}
}

func TestReportListNumbered(t *testing.T) {
	r := reportRegistry(t)
	result := runReportAQL(t, r, `[10 20 30] report.list`)
	s, _ := native.AsString(result[0])
	for _, want := range []string{"[0] 10", "[1] 20", "[2] 30"} {
		if !strings.Contains(s, want) {
			t.Errorf("missing %q in:\n%s", want, s)
		}
	}
}
