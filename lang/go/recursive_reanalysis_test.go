package lang_test

import (
	"testing"

	lang "github.com/boru-lang/boru/lang/go"
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

// add's string-concat overloads commit whenever an operand fills the String
// slot — but a GRADUAL (dynamic) operand fills it OPTIMISTICALLY (it might be a
// String, or a Number taking the numeric overload at run time). Typing that
// result as a definite String wrongly rejects a downstream numeric use. The
// concat overloads' ReturnsFn (ReturnsAddConcat) now returns String only when an
// operand is PROVABLY String, else a gradual Scalar — so `Integer add
// (get-result)` types as a gradual Scalar that accepts a later Integer use. This
// is the root-cause fix for the sort.boru `msd-go` no_signature false positive
// (the recursive self-call fed `lo add (Array-get)` back to its own lo:Integer
// param). The negatives lock that provable string-concat typing is preserved and
// no false negative was introduced.
func TestAddGradualConcatReturnTyping(t *testing.T) {
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

	// POSITIVE 1 (the msd-go shape): `lo add (steps get lo)` (Integer + dynamic
	// Array-get) fed back into the recursive `lo:Integer` param. Pre-fix the add
	// result was typed String and the self-call emitted no_signature; now 0.
	recShape := `
def go fn [[lo:Integer arr:Array] [Array] [
  if (lo gt 9) [arr] [
    def steps (make Array (iota 11 each [drop 1]))
    def nlo (lo add (steps get lo))
    def _r (arr nlo go)
    arr
  ]
]] end
def run fn [[] [Array] [def arr (make Array (iota 11 each [drop 0])) arr 0 go]] end
(run)`
	if n := errCount(t, recShape); n != 0 {
		t.Errorf("recursive `lo add (dynamic get)` into a recursive Integer param: expected 0 errors, got %d", n)
	}

	// POSITIVE 2 (direct, non-recursive): a gradual `add` result must accept a
	// numeric downstream use.
	directShape := `
def takesint fn [[n:Integer] [Integer] [n]] end
def go fn [[m:Map] [Integer] [ def g (m get k/q) (g add 1) takesint ]] end
(go {})`
	if n := errCount(t, directShape); n != 0 {
		t.Errorf("(dynamic add 1) into an Integer param: expected 0 errors, got %d", n)
	}

	// NEGATIVE 1: a CONCRETE String operand makes the concat result PROVABLY
	// String, which must still be rejected by an Integer param (no false
	// negative — the gradual relaxation is for gradual operands only).
	concatIntoInt := `
def takesint fn [[n:Integer] [Integer] [n]] end
def go fn [[] [Integer] [("x" add 1) takesint]] end
(go)`
	if n := errCount(t, concatIntoInt); n == 0 {
		t.Errorf("a provable String-concat result fed into an Integer param must still be flagged")
	}

	// NEGATIVE 2: string-or-bust is preserved — two non-String scalars match
	// neither the numeric nor the concat overload.
	stringOrBust := `print ((add true 1)) end`
	if n := errCount(t, stringOrBust); n == 0 {
		t.Errorf("add of two non-String scalars (string-or-bust) must still be flagged")
	}
}
