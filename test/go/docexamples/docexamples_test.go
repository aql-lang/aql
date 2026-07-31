// docexamples_test.go runs every checkable `# returns` example extracted from
// the prose docs against the production language layer and compares the
// rendered stack to the documented result.
//
// Render path: the comparison string is eng.Canon of the residual stack
// — canonical boru source, which is the value form the docs are written
// in (quoted strings, lowercase `none`, comma-free lists/maps, `name/q`
// atoms). This is the same renderer the .tsv spec suites use
// (test/go/specrunner), so a passing example round-trips as written Boru.
package docexamples

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/boru-lang/boru/eng/go"
	"github.com/boru-lang/boru/eng/go/parser"
	"github.com/boru-lang/boru/lang/go/modules"
	"github.com/boru-lang/boru/lang/go/native"
)

// docFiles are the prose docs scanned for `# returns` examples, relative to
// this package (repo-root files two dirs up from test/go/docexamples).
var docFiles = []string{
	"README.md",
	"REFERENCE.md",
	"TUTORIAL.md",
	"HOWTO.md",
	"EXPLANATION.md",
}

// mismatchKey identifies a flagged example by its source file and its
// expression text. Keying on the expression (not a line number) keeps the
// registry stable when unrelated doc edits shift lines around — the
// recurring papercut of a file:line scheme. The file is part of the key
// because the same expression can appear in more than one doc (e.g.
// `Integer lt 0` lives in both REFERENCE and EXPLANATION).
type mismatchKey struct {
	File string
	Expr string
}

// knownMismatch records deterministic doc-vs-engine disagreements that
// need an author's judgment to resolve (the documented behavior may be a
// stale concept, or the engine may have a real bug) — so they are flagged
// here rather than silently rewritten. The value is the note. An entry
// downgrades a failure to a logged xfail; an xfail that unexpectedly
// PASSES fails loudly ("stale xfail") and an entry that matches no example
// fails loudly too, so the list can't rot. See the package's completion
// report for the triage rationale behind each entry.
// The registry should run at zero entries in the steady state: resolve
// a new mismatch as a doc bug or an engine bug promptly rather than
// letting entries accumulate. (The four historical entries — the
// `Integer lt 0` refinement-vs-ordering confusion, MathUtil.log float
// precision, the absent-optional `none`/`None` render convention, and
// the bare `set`/`get` `end` demo — were resolved in the June 2026 doc
// review; see design/REVIEW-NOTES.*.md.) The two entries that carried the
// project rename — README.md documenting the `boru:*` module names ahead
// of the engine — retired when the rename landed and the engine started
// serving those names.
var knownMismatch = map[mismatchKey]string{}

func docRoot() string { return filepath.Join("..", "..", "..") }

func TestDocExamples(t *testing.T) {
	// Sanity-pin the render path so a future change to lang.Run/Sprint
	// rendering can't silently invalidate every comparison below.
	if got := runProgram(t, "[1 2 3]"); got != "[1 2 3]" {
		t.Fatalf("render sanity check: got %q, want %q", got, "[1 2 3]")
	}

	// matchedKeys is every knownMismatch key that matched an extracted
	// example this run. Populated during extraction (which always runs,
	// independent of `-run` subtest filtering), so the dead-entry check
	// below doesn't false-positive when the test is filtered to a subset.
	matchedKeys := map[mismatchKey]bool{}
	// keyCounts catches the one ambiguity an expr-based key can introduce:
	// two examples in the same file with identical expression text. A
	// flagged entry then can't tell them apart, so we fail loudly.
	keyCounts := map[mismatchKey]int{}

	for _, name := range docFiles {
		path := filepath.Join(docRoot(), name)
		body, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("read %s: %v", path, err)
		}
		examples := Extract(name, string(body))
		if len(examples) == 0 {
			t.Errorf("%s: no `# returns` examples extracted", name)
			continue
		}
		for _, ex := range examples {
			key := mismatchKey{ex.File, ex.Expr}
			keyCounts[key]++
			if _, isKnown := knownMismatch[key]; isKnown {
				matchedKeys[key] = true
			}
		}

		t.Run(name, func(t *testing.T) {
			for _, ex := range examples {
				ex := ex
				sub := fmt.Sprintf("L%d_%s", ex.Line, sanitise(ex.Expr))
				t.Run(sub, func(t *testing.T) {
					key := mismatchKey{ex.File, ex.Expr}
					ok, detail := checkExample(t, ex)
					if note, isKnown := knownMismatch[key]; isKnown {
						if keyCounts[key] > 1 {
							t.Fatalf("ambiguous knownMismatch key %s:%q matches %d examples — disambiguate (the expr-based key needs unique expressions)",
								ex.File, ex.Expr, keyCounts[key])
						}
						if ok {
							t.Fatalf("stale xfail %s:%q (%s): now PASSES — remove from knownMismatch", ex.File, ex.Expr, note)
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
	// example (doc rewritten / example removed): they're dead weight and
	// hide drift. Guarded so `-run`-filtered subset runs don't
	// false-positive.
	for key, note := range knownMismatch {
		if !matchedKeys[key] {
			t.Errorf("knownMismatch entry %s:%q (%s) matched no example — update or remove it", key.File, key.Expr, note)
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
// renders the residual stack as canonical boru source (eng.Canon).
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
	// (`import "boru:math-util"`) resolve as they do for a CLI user.
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
