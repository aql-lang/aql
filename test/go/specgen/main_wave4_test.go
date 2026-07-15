// Wave-4 coverage for the specgen generator helpers: the runtime
// failure whose engine error carries no [aql/<code>] tag (the bare
// "ERROR:" class, which classifyFrontier maps to the generic "error"
// detail), and the front-coded reader's remaining edge rows.
package main

import (
	"os"
	"path/filepath"
	"testing"

	lang "github.com/aql-lang/aql/lang/go"
)

// TestW4ClassifyFrontierBareErrorDetail pins the classifyFrontier arm
// for a runtime error whose message has no [aql/<code>] prefix. A
// TOP-LEVEL "1 0 div" no longer qualifies — the checker's arith mirror
// flags it statically (design/CHECKER-COMPLETION.0.md) so it classifies
// classCheck now; the div is moved inside a fn body, where the mirror's
// reachability gate keeps the checker silent: the program checks clean
// and compiles, yet fails at run with a bare out-of-bounds read —
// errorClass yields the generic "ERROR:", and the classifier
// substitutes the "error" detail.
func TestW4ClassifyFrontierBareErrorDetail(t *testing.T) {
	a, err := lang.New()
	if err != nil {
		t.Fatal(err)
	}
	a.SetClock(specClock)

	cl, detail, compiled := classifyFrontier(a, "def w4d fn [[n:Integer] [Integer] [[1 2] getr n]] w4d 5")
	if cl != classRuntime {
		t.Fatalf("class = %d, want classRuntime", cl)
	}
	if detail != "error" {
		t.Errorf("detail = %q, want the generic \"error\"", detail)
	}
	if !compiled {
		t.Error("expected the OOB getr to compile")
	}
}

// TestW4ErrorClassBareMessage pins errorClass on an engine error with
// no bracketed code at all.
func TestW4ErrorClassBareMessage(t *testing.T) {
	a, err := lang.New()
	if err != nil {
		t.Fatal(err)
	}
	if _, runErr := a.RunInterp("[1 2] 5 getr"); runErr == nil {
		t.Fatal("[1 2] 5 getr ran clean, want a runtime error")
	} else if got := errorClass(runErr); got != "ERROR:" {
		t.Errorf("errorClass = %q, want the bare ERROR: sentinel", got)
	}
}

// TestW4ForEachDataRowFrontCodedNoExtra covers a front-coded row with
// no third field: extra comes back empty.
func TestW4ForEachDataRowFrontCodedNoExtra(t *testing.T) {
	path := filepath.Join(t.TempDir(), "fc.tsv")
	content := "# header\n" + frontCodeMarker + "\n" +
		"0\tabc\n" +
		"2\txy\textra1\n"
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}

	var inputs, extras []string
	err := forEachDataRow(path, func(input, expected, note string, line int) {
		inputs = append(inputs, input)
		extras = append(extras, expected)
	})
	if err != nil {
		t.Fatalf("forEachDataRow: %v", err)
	}
	if len(inputs) != 2 || inputs[0] != "abc" || inputs[1] != "abxy" {
		t.Errorf("inputs = %v, want [abc abxy]", inputs)
	}
	if extras[0] != "" || extras[1] != "extra1" {
		t.Errorf("extras = %v, want [\"\" extra1]", extras)
	}
}
