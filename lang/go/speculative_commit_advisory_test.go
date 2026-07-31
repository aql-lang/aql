package lang_test

import (
	"testing"

	lang "github.com/boru-lang/boru/lang/go"
)

// The "speculative forward commit" advisory: in check mode, flag the
// else-less-guard shape — a parked word whose plan filled a trailing
// slot with the NEXT statement's word committed early at that
// statement boundary. The commit is correct (the guard fires before
// the next statement runs); the note exists because the source reads
// ambiguously, and an explicit `[]` else or `end` states the intent.
// Advisory only (info severity, never gating). See
// design/FORWARD-COLLECTION-PHASES.10.md.

func speculativeCommitCount(t *testing.T, src string) int {
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
		if d.Code == "speculative_forward_commit" {
			n++
		}
	}
	return n
}

func TestSpeculativeCommitAdvisory_FiresOnGuardShape(t *testing.T) {
	for _, src := range []string{
		// The reported shape: else-less guard, next statement's result
		// would have been the phantom else before the barrier commit.
		"def n 0 if (n eq 0) [99] add 1 2",
		// The next statement being a def is the original bug report.
		"def n 0 if (n eq 0) [99] def q 1 q",
		// Word condition (no paren): exercises the bestDeferred path
		// in matchSignature carrying specAt through.
		"def ok true if ok [99] add 1 2",
	} {
		if n := speculativeCommitCount(t, src); n == 0 {
			t.Errorf("expected a speculative_forward_commit advisory for %q, got none", src)
		}
	}
}

func TestSpeculativeCommitAdvisory_QuietOnIdiomatic(t *testing.T) {
	for _, src := range []string{
		// The core definition idiom: fn is planned speculatively but
		// the smaller-arity probe FAILS (def has no 1-arg sig), so no
		// commit happens and the advisory must stay silent.
		"def square fn [[x:Number] [Number] [mul x x]]\nsquare 4",
		// The advisory's own suggested fixes must silence it.
		"def n 0 if (n eq 0) [99] [] add 1 2",
		"def n 0 if (n eq 0) [99] end add 1 2",
		// A paren else is an arrival, not a barrier.
		"if (1 eq 1) [99] (add 1 2)",
		// Plain forms with no parked guard at all.
		"add 1 2",
		"if true [1] [2]",
	} {
		if n := speculativeCommitCount(t, src); n != 0 {
			t.Errorf("expected NO speculative_forward_commit advisory for %q, got %d", src, n)
		}
	}
}

// The advisory is non-gating: info severity, so a clean program with
// only this advisory reports zero errors.
func TestSpeculativeCommitAdvisory_NonGating(t *testing.T) {
	a, _ := lang.New()
	cr, _ := a.Check("def n 0 if (n eq 0) [99] add 1 2")
	if cr.Summary.Errors != 0 {
		t.Fatalf("advisory must not raise errors, got %d", cr.Summary.Errors)
	}
}
