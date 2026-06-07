package lang_test

import (
	"testing"

	lang "github.com/aql-lang/aql/lang/go"
)

// The "forward greediness" advisory (prototype): in check mode, flag the
// `1 2 add 3 mul → 5` surprise — a word that forward-collects an argument
// while a SIBLING operand (same type as the slot it just took) is left
// unconsumed on the stack below it. Advisory only (info severity, never
// gating). See the DX-report gotcha and the design discussion.

func strandCount(t *testing.T, src string) int {
	t.Helper()
	a, err := lang.New()
	if err != nil {
		t.Fatal(err)
	}
	cr, err := a.Check(src)
	if err != nil {
		// check may report unrelated errors; we only care about our code.
		_ = err
	}
	n := 0
	for _, d := range cr.Diagnostics {
		if d.Code == "forward_strands_operand" {
			n++
		}
	}
	return n
}

func TestForwardStrandAdvisory_FiresOnGotcha(t *testing.T) {
	// add grabs the forward `3`, strands the `1`.
	for _, src := range []string{
		"1 2 add 3 mul",
		"1 2 add 3",
		"1 2 sub 3",
		// inside a function body: `a b add c add` strands `a`.
		"def f fn [[a:Integer b:Integer c:Integer] Integer [a b add c add]]\nf 1 2 3",
	} {
		if n := strandCount(t, src); n == 0 {
			t.Errorf("expected a forward_strands_operand advisory for %q, got none", src)
		}
	}
}

func TestForwardStrandAdvisory_QuietOnIdiomatic(t *testing.T) {
	for _, src := range []string{
		"10 sub 3",       // swap form: nothing stranded
		"1 2 3",          // bare values, no word
		"1 2 3 add",      // no forward collection (add takes top two)
		"1 2 add",        // exact arity, nothing left
		"add 1 2",        // pure forward
		"1 dup add",      // stack idiom
		"10 sub 3 sub 1", // chained swap
		"\"hi\" 2 add 3", // sibling type mismatch (String below a Number slot)
		// the advisory's own suggested fix must silence it:
		"def g fn [[a:Integer b:Integer c:Integer] Integer [(a b add) c add]]\ng 1 2 3",
		"(1 2 add) 3 mul",
	} {
		if n := strandCount(t, src); n != 0 {
			t.Errorf("expected NO forward_strands_operand advisory for %q, got %d", src, n)
		}
	}
}

// The advisory is non-gating: it is info severity, so a clean program with
// only this advisory reports zero errors.
func TestForwardStrandAdvisory_NonGating(t *testing.T) {
	a, _ := lang.New()
	cr, _ := a.Check("1 2 add 3 mul")
	if cr.Summary.Errors != 0 {
		t.Fatalf("advisory must not raise errors, got %d", cr.Summary.Errors)
	}
}
