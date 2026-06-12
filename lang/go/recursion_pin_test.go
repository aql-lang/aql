package lang_test

import (
	"strings"
	"testing"

	"github.com/aql-lang/aql/lang/go"
)

// These tests pin the RESOURCE shape of recursion under the current
// execution model (design/TCO-STAGED.0.md Stage 0): every fn call —
// tail or not — parks its frame's cleanup tail on the tape until the
// chain unwinds, so deep recursion consumes tape proportional to depth
// and a tight tape ceiling trips tape_exhausted, while an iterative
// loop of the same length runs in O(1) tape.
//
// Stage 4 of the TCO plan deliberately flips ONE of these pins: tail
// recursion becomes O(1) tape (TestTailRecursionParksTapePerCall's
// exhaustion case starts passing), while non-tail recursion keeps
// exhausting and loops stay flat. When that stage lands, update the
// tail-recursion expectation here and assert the new taxonomy (a tail
// runaway trips evaluation_limit, not tape_exhausted).
//
// Value-correctness rows for recursion live in lang/spec/recursion.tsv.

const defTailSum = `def s2 fn [[n:Integer acc:Integer] [Integer] [if (n lte 0) [acc] [s2 (n sub 1) (acc add n)]]] `

const defNonTailSum = `def s fn [[n:Integer] [Integer] [if (n lte 0) [0] [n add (s (n sub 1))]]] `

// tightTape is a deliberately small ceiling: 2048 entries growing once
// by 2.0 = 4096 max. Big enough for the program and a shallow call
// chain, far too small for a deep one.
var tightTape = lang.TapeOptions{InitialSize: 2048, MaxGrows: 1, GrowthFactor: 2.0}

func runWithTape(t *testing.T, tape lang.TapeOptions, src string) ([]any, error) {
	t.Helper()
	a, err := lang.New(lang.Options{Tape: tape})
	if err != nil {
		t.Fatal(err)
	}
	return a.Run(src)
}

func TestTailRecursionParksTapePerCall(t *testing.T) {
	// Shallow tail recursion fits under the tight ceiling…
	res, err := runWithTape(t, tightTape, defTailSum+`s2 50 0`)
	if err != nil {
		t.Fatalf("shallow tail recursion under tight tape: %v", err)
	}
	if len(res) != 1 || res[0] != int64(1275) {
		t.Fatalf("s2 50 0 = %v, want 1275", res)
	}

	// …deep tail recursion does not: each tail call parks a frame tail
	// on the tape, so depth 2000 blows the 4096-entry ceiling. THIS is
	// the expectation TCO Stage 4 flips.
	_, err = runWithTape(t, tightTape, defTailSum+`s2 2000 0`)
	if err == nil {
		t.Fatal("deep tail recursion under tight tape succeeded — has TCO landed? update this pin to the new taxonomy (see design/TCO-STAGED.0.md Stage 4)")
	}
	if !strings.Contains(err.Error(), "tape_exhausted") {
		t.Fatalf("deep tail recursion error = %v, want tape_exhausted", err)
	}
}

func TestNonTailRecursionParksTapePerCall(t *testing.T) {
	// Non-tail recursion parks a pending forward AND the frame tail per
	// level; it must exhaust the tight ceiling too — and must STILL do
	// so after any TCO stage (it is never a TCO candidate).
	//
	// KNOWN MISDIAGNOSIS (pinned as-is, Stage 0): the error surfaced
	// today is not tape_exhausted but a phantom downstream error — the
	// ceiling-dropped splice starves the frame's ReturnCheck, which
	// reports `type_error: expected 1 return value(s), got 0` before
	// the Run loop's Exhausted() check gets a chance. The same genre as
	// the phantom "unmatched opening parenthesis" the evaluation_limit
	// fix removed (RECURSION-PERFORMANCE.10.md §secondary issue). The
	// TCO taxonomy cleanup (design/TCO-STAGED.0.md Stage 6) should make
	// this reliably tape_exhausted; until then only the FAILURE is
	// pinned, not the code, so unrelated tail-size changes can't flap
	// this test.
	_, err := runWithTape(t, tightTape, defNonTailSum+`s 2000`)
	if err == nil {
		t.Fatal("deep non-tail recursion under tight tape succeeded; it should exhaust the tape regardless of TCO")
	}
	t.Logf("non-tail exhaustion surfaced as: %v", err)
}

func TestIterationRunsInConstantTapeSpace(t *testing.T) {
	// The same tight ceiling comfortably runs MORE iterations as a for
	// loop: the loop body reuses one tape region. This is the baseline
	// that makes the recursion pins meaningful (the ceiling is not
	// simply "too small for 2000 of anything").
	res, err := runWithTape(t, tightTape, `def acc 0 for [1 2001] [def acc (acc add i)] acc`)
	if err != nil {
		t.Fatalf("2000-iteration loop under tight tape: %v", err)
	}
	if len(res) == 0 || res[len(res)-1] != int64(2001000) {
		t.Fatalf("loop sum = %v, want 2001000", res)
	}
}

// TestTailRecursionDepthHeadroomDefaults documents (without brittle
// exact pins) that the DEFAULT tape config covers four-digit recursion
// depths — the working headroom users have today. TCO stages must not
// shrink it; Stage 4 makes tail depth unbounded.
func TestTailRecursionDepthHeadroomDefaults(t *testing.T) {
	res, err := runWithTape(t, lang.TapeOptions{}, defTailSum+`s2 2000 0`)
	if err != nil {
		t.Fatalf("s2 2000 0 under default tape: %v", err)
	}
	if len(res) != 1 || res[0] != int64(2001000) {
		t.Fatalf("s2 2000 0 = %v, want 2001000", res)
	}
}
