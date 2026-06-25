package lang_test

import (
	"testing"

	lang "github.com/aql-lang/aql/lang/go"
)

// A recursive fn whose body is clean against its DECLARED param types must not
// be flagged once a caller forces a call-shape re-analysis whose recursion
// re-runs the body with a param narrowed to a dynamic Any. AnalyseFnBody is
// memo-keyed on arg-types, so a self-call with a different shape re-analyses the
// same body tokens and would otherwise re-emit a spurious no_signature/
// undefined_word (the trie client's `fuzzy-go` recursing on a `get`-result as
// nd:Map, cascading into kid-items/get/build-row). The outer, non-recursive
// analysis of the same body already reports any real defect, so the recursive
// re-entry's error-level body diagnostics are suppressed.
func TestRecursiveReanalysisNoFalsePositives(t *testing.T) {
	errCount := func(t *testing.T, src string) int {
		t.Helper()
		a, err := lang.New()
		if err != nil {
			t.Fatal(err)
		}
		cr, _ := a.Check(src)
		n := 0
		for _, d := range cr.Diagnostics {
			if d.Severity == "error" {
				n++
			}
		}
		return n
	}

	// A self-recursive node walker: the body calls a Map-typed helper and
	// recurses on a `get`-result (statically Any) bound to its nd:Map param.
	// Clean at def-time (the recursion bails as a forward ref); the `go` caller
	// forces the call-shape re-analysis that used to surface the cascade.
	clean := `
def helper fn [[m:Map] [Integer] [1]] end
def rec fn [[nd:Map n:Integer] [Integer] [
  if (n lte 0) [nd helper] [ (n sub 1) (nd "x" get) rec ]
]] end
def go fn [[nd:Map] [Integer] [5 nd rec]] end`
	if n := errCount(t, clean); n != 0 {
		t.Errorf("recursive walker + caller: expected 0 errors, got %d", n)
	}

	// NEGATIVE 1: a genuine undefined word in a NON-recursive helper is still
	// reported — suppression is scoped to same-name recursion, not helpers.
	badHelper := `
def helper fn [[n:Integer] [Integer] [n totally-undefined-word]] end
def rec fn [[n:Integer] [Integer] [if (n lte 0) [0] [ (n helper) (n sub 1) rec ]]] end
def go fn [[n:Integer] [Integer] [n rec]] end`
	if n := errCount(t, badHelper); n == 0 {
		t.Errorf("undefined word in a non-recursive helper must still be flagged")
	}

	// NEGATIVE 2: a genuine undefined word in the FIRST (non-recursive) analysis
	// of a recursive fn's own body is still reported — only the re-entry is
	// suppressed, and the first analysis covers the body.
	badSelf := `
def rec fn [[n:Integer] [Integer] [if (n lte 0) [definitely-not-a-word] [ (n sub 1) rec ]]] end
def go fn [[n:Integer] [Integer] [n rec]] end`
	if n := errCount(t, badSelf); n == 0 {
		t.Errorf("undefined word in a recursive fn's own body must still be flagged")
	}
}
