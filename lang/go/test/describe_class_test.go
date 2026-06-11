package test

import (
	"bytes"
	"strings"
	"testing"

	"github.com/aql-lang/aql/eng/go/parser"
	"github.com/aql-lang/aql/lang/go/native"
)

// runDescribeSteps evaluates steps on a fresh registry with Output
// captured, returning everything the steps printed. `describe` writes
// to r.Output rather than the stack, so the schema-view tests below
// assert on the captured text.
func runDescribeSteps(t *testing.T, steps []string) string {
	t.Helper()
	reg, err := native.DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	native.Register(reg)
	var buf bytes.Buffer
	reg.Output = &buf
	eng := native.NewTop(reg)
	for _, step := range steps {
		vals, err := parser.Parse(step)
		if err != nil {
			t.Fatalf("parse %q: %v", step, err)
		}
		if _, err := eng.Run(vals); err != nil {
			t.Fatalf("run %q: %v", step, err)
		}
	}
	return buf.String()
}

// TestDescribeClassSchemaView pins the `describe <Class>` schema view:
// a name def'd to a class type renders kind + lattice path, the field
// table with required-vs-default annotations, and the sealing note —
// not the word-help "<not described>" view.
func TestDescribeClassSchemaView(t *testing.T) {
	out := runDescribeSteps(t, []string{
		`def Point class {x:Float y:0.0}`,
		`describe Point`,
	})
	for _, want := range []string{
		"Point — class (Class/Point)",
		"x : Float  (required)",
		"y : Float = 0.0  (default)",
		"make Point {…} — sealed",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("describe Point output missing %q:\n%s", want, out)
		}
	}
	if strings.Contains(out, "<not described>") {
		t.Errorf("describe Point fell through to the word-help view:\n%s", out)
	}
}

// TestDescribeSubclassSchemaView pins the subclass view: the refine
// chain shows in the lattice path and Refines line, and inherited
// fields are listed and marked.
func TestDescribeSubclassSchemaView(t *testing.T) {
	out := runDescribeSteps(t, []string{
		`def Point class {x:Float y:0.0}`,
		`def Point3 refine Point {z:Float}`,
		`describe Point3`,
	})
	for _, want := range []string{
		"Point3 — class (Class/Point/Point3)",
		"Refines: Class/Point",
		"x : Float  (required)  (inherited)",
		"y : Float = 0.0  (default)  (inherited)",
		"z : Float  (required)",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("describe Point3 output missing %q:\n%s", want, out)
		}
	}
}

// TestDescribeWordStillWorks pins that the schema-view branch does not
// shadow ordinary word describe: a registered word still renders the
// word-help view with its signatures.
func TestDescribeWordStillWorks(t *testing.T) {
	out := runDescribeSteps(t, []string{`describe set`})
	if !strings.Contains(out, "Signatures") {
		t.Errorf("describe set lost the word-help view:\n%s", out)
	}
}
