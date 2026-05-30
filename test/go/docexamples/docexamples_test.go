// docexamples_test.go runs every checkable `=>` example extracted from
// the prose docs against the production language layer and compares the
// rendered stack to the documented result.
//
// Render path: the comparison string is eng.Canon of the residual stack
// — canonical AQL source, which is the value form the docs are written
// in (quoted strings, lowercase `none`, comma-free lists/maps, `name/q`
// atoms). This is the same renderer the .tsv spec suites use
// (test/go/specrunner), so a passing example round-trips as written AQL.
package docexamples

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/aql-lang/aql/eng/go"
	"github.com/aql-lang/aql/eng/go/parser"
	"github.com/aql-lang/aql/lang/go/modules"
	"github.com/aql-lang/aql/lang/go/native"
)

// docFiles are the prose docs scanned for `=>` examples, relative to
// this package (repo-root files two dirs up from test/go/docexamples).
var docFiles = []string{
	"README.md",
	"REFERENCE.md",
	"TUTORIAL.md",
	"HOWTO.md",
	"EXPLANATION.md",
}

// knownMismatch records deterministic doc-vs-engine disagreements that
// need an author's judgment to resolve (the documented behavior may be a
// stale concept, or the engine may have a real bug) — so they are flagged
// here rather than silently rewritten. Key is "File:Line" (matching
// Example.File/Line); value is the note. An entry downgrades a failure to
// a logged xfail; an xfail that unexpectedly PASSES fails loudly
// ("stale xfail") so the list can't rot. See the package's completion
// report for the triage rationale behind each entry.
var knownMismatch = map[string]string{
	// `lt` with a type-literal left operand builds a DepScalar refinement
	// `(Integer lt 0)`, it does not perform a boolean ordering compare —
	// so the doc's `=> true` (illustrating type-literal-sorts-low) never
	// matches. Needs author decision: change the example to `cmp`, or
	// reconsider `lt`'s type-literal overload.
	"REFERENCE.md:274":   "Integer lt 0 builds a DepScalar refinement, not a boolean; doc shows true",
	"EXPLANATION.md:260": "Integer lt 0 builds a DepScalar refinement, not a boolean; doc shows true",

	// math.log of e is 0.9999999998311266 (float), not the exact 1.0 the
	// doc shows. Either round in the example or accept the float form.
	"TUTORIAL.md:167": "math.log float precision: engine 0.9999999998311266 vs doc 1.0",

	// An absent optional record field renders as the None type literal
	// (Canon: `None`); the doc writes lowercase `none`. Render-convention
	// call for the author (None type-literal vs none value).
	"TUTORIAL.md:453": "absent optional renders as None type-literal; doc shows lowercase none",

	// Core concept conflict: the docs state list literals are quotations
	// (NOT evaluated) by default, but the engine auto-evaluates an
	// unconsumed list — `[1 add 2]` => `[3]`, not `[1 add 2]`. The
	// surrounding prose (and `def double [dup add]` claims) rest on the
	// old model; this needs a section rewrite, not a one-line output fix.
	"TUTORIAL.md:483":    "lists auto-evaluate; doc claims quotation-by-default ([1 add 2] => [3])",
	"EXPLANATION.md:297": "lists auto-evaluate; doc claims quotation-by-default ([1 add 2] => [3])",

	// `3 4 add 2 mul` yields 18 (forward collection regroups the
	// operators), not the 14 the stack-machine walkthrough in the prose
	// describes. The whole explanatory paragraph assumes 14; needs author
	// rework of the example or the narrative.
	"EXPLANATION.md:55": "forward collection: 3 4 add 2 mul => 18, doc narrative assumes 14",

	// `set foo 99 end get foo` has no matching `set` signature (bare
	// set/get need a context store). The example illustrates `end` but
	// uses a non-working set/get form; author should pick an `end` demo
	// that runs.
	"EXPLANATION.md:146": "set/get need a context store; bare form has no signature",
}

func docRoot() string { return filepath.Join("..", "..", "..") }

func TestDocExamples(t *testing.T) {
	// Sanity-pin the render path so a future change to lang.Run/Sprint
	// rendering can't silently invalidate every comparison below.
	if got := runProgram(t, "[1 2 3]"); got != "[1 2 3]" {
		t.Fatalf("render sanity check: got %q, want %q", got, "[1 2 3]")
	}

	// extractedKeys is every example key the extractor produced this run,
	// independent of `-run` subtest filtering (extraction always runs).
	// Used to detect knownMismatch entries that no longer match any
	// example (doc edited / line moved) without false positives when the
	// test is filtered to a subset of files.
	extractedKeys := map[string]bool{}

	for _, name := range docFiles {
		path := filepath.Join(docRoot(), name)
		body, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("read %s: %v", path, err)
		}
		examples := Extract(name, string(body))
		if len(examples) == 0 {
			t.Errorf("%s: no => examples extracted", name)
			continue
		}
		for _, ex := range examples {
			extractedKeys[fmt.Sprintf("%s:%d", ex.File, ex.Line)] = true
		}

		t.Run(name, func(t *testing.T) {
			for _, ex := range examples {
				ex := ex
				sub := fmt.Sprintf("L%d_%s", ex.Line, sanitise(ex.Expr))
				t.Run(sub, func(t *testing.T) {
					key := fmt.Sprintf("%s:%d", ex.File, ex.Line)
					ok, detail := checkExample(t, ex)
					if note, isKnown := knownMismatch[key]; isKnown {
						if ok {
							t.Fatalf("stale xfail %s (%s): now PASSES — remove from knownMismatch", key, note)
						}
						t.Skipf("known mismatch (%s): %s", note, detail)
						return
					}
					if !ok {
						t.Error(detail)
					}
				})
			}
		})
	}

	// Flag knownMismatch entries that no longer match any extracted
	// example (doc edited / line moved): they're dead weight and hide
	// drift. Guarded so `-run`-filtered subset runs don't false-positive.
	for key, note := range knownMismatch {
		if !extractedKeys[key] {
			t.Errorf("knownMismatch entry %s (%s) matched no example — update or remove it", key, note)
		}
	}
}

// checkExample runs one example and reports (pass, detail-on-failure).
func checkExample(t *testing.T, ex Example) (bool, string) {
	t.Helper()
	got, runErr := runProgramErr(ex.Program)

	if ex.WantErr {
		if runErr == nil {
			return false, fmt.Sprintf("expected error, got result %q", got)
		}
		if ex.ErrSubstr != "" && !strings.Contains(runErr.Error(), ex.ErrSubstr) {
			return false, fmt.Sprintf("error %q does not contain %q", runErr.Error(), ex.ErrSubstr)
		}
		return true, ""
	}

	if runErr != nil {
		return false, fmt.Sprintf("unexpected error: %v", runErr)
	}
	if got != ex.Expected {
		return false, fmt.Sprintf("got %q, want %q", got, ex.Expected)
	}
	return true, ""
}

// runProgram is checkExample's render helper for non-error programs; it
// fails the test on an unexpected evaluation error.
func runProgram(t *testing.T, src string) string {
	t.Helper()
	got, err := runProgramErr(src)
	if err != nil {
		t.Fatalf("run %q: %v", src, err)
	}
	return got
}

// runProgramErr evaluates src against a fresh production registry and
// renders the residual stack as canonical AQL source (eng.Canon).
func runProgramErr(src string) (string, error) {
	values, err := parser.Parse(src)
	if err != nil {
		return "", err
	}
	reg, err := native.DefaultRegistry()
	if err != nil {
		return "", err
	}
	// Mirror lang.New's registry wiring so module imports
	// (`"aql:math" import end`) resolve as they do for a CLI user.
	reg.SetParseFunc(parser.Parse)
	modules.InstallResolver(reg)
	result, err := native.NewTop(reg).Run(values)
	if err != nil {
		return "", err
	}
	return eng.Canon(result), nil
}

// sanitise makes a short, filesystem-safe subtest fragment from an expr.
func sanitise(s string) string {
	s = strings.ReplaceAll(s, " ", "_")
	s = strings.ReplaceAll(s, "/", "_")
	if len(s) > 40 {
		s = s[:40]
	}
	return s
}
