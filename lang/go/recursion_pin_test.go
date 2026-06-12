package lang_test

import (
	"testing"

	"github.com/aql-lang/aql/lang/go"
)

// These tests pin the RESOURCE shape of recursion through the public
// lang API (design/TCO-STAGED.0.md Stage 0, updated at Stage 4a):
//
//   - TAIL self-recursion runs in O(1) tape — full frame replacement
//     elides each frame, so depth is bounded by CPU (evaluation_limit),
//     not memory. This pin originally asserted the opposite (every
//     call parked its frame until the chain unwound); Stage 4a flipped
//     it, exactly as the original tripwire anticipated.
//   - NON-tail recursion still parks a pending forward and the frame
//     per level: a tight tape ceiling stops it. Never a TCO candidate.
//   - Iteration (for) stays O(1) tape, as always.
//
// Value-correctness rows for recursion live in lang/spec/recursion.tsv;
// the TCO path counters live in lang/go/test/tco_elide_test.go.

const defTailSum = `def s2 fn [[n:Integer acc:Integer] [Integer] [if (n lte 0) [acc] [s2 (n sub 1) (acc add n)]]] `

const defNonTailSum = `def s fn [[n:Integer] [Integer] [if (n lte 0) [0] [n add (s (n sub 1))]]] `

// tightTape is a deliberately small ceiling: 2048 entries growing once
// by 2.0 = 4096 max. Big enough for the program, far too small for a
// deep nested call chain.
var tightTape = lang.TapeOptions{InitialSize: 2048, MaxGrows: 1, GrowthFactor: 2.0}

func runWithTape(t *testing.T, tape lang.TapeOptions, src string) ([]any, error) {
	t.Helper()
	a, err := lang.New(lang.Options{Tape: tape})
	if err != nil {
		t.Fatal(err)
	}
	return a.Run(src)
}

func TestTailRecursionRunsInConstantTapeSpace(t *testing.T) {
	// Shallow and deep tail recursion BOTH fit under the tight
	// ceiling: each tail call's frame replaces the previous one.
	res, err := runWithTape(t, tightTape, defTailSum+`s2 50 0`)
	if err != nil {
		t.Fatalf("shallow tail recursion under tight tape: %v", err)
	}
	if len(res) != 1 || res[0] != int64(1275) {
		t.Fatalf("s2 50 0 = %v, want 1275", res)
	}

	res, err = runWithTape(t, tightTape, defTailSum+`s2 2000 0`)
	if err != nil {
		t.Fatalf("deep tail recursion under tight tape: %v (tail recursion is O(1) tape since TCO Stage 4a — a regression here means the elimination gate stopped firing)", err)
	}
	if len(res) != 1 || res[0] != int64(2001000) {
		t.Fatalf("s2 2000 0 = %v, want 2001000", res)
	}
}

func TestNonTailRecursionParksTapePerCall(t *testing.T) {
	// Non-tail recursion parks a pending forward AND the frame per
	// level; it must exhaust the tight ceiling — with or without TCO
	// (it is never a candidate: the parked add consumes each frame's
	// result).
	//
	// KNOWN MISDIAGNOSIS (pinned as-is): the error surfaced today is
	// not always tape_exhausted but can be a phantom downstream error —
	// the ceiling-dropped splice starves the frame's ReturnCheck,
	// which reports `type_error: expected 1 return value(s), got 0`
	// before the Run loop's Exhausted() check gets a chance. The same
	// genre as the phantom "unmatched opening parenthesis" the
	// evaluation_limit fix removed (RECURSION-PERFORMANCE.10.md
	// §secondary issue). The TCO taxonomy cleanup
	// (design/TCO-STAGED.0.md Stage 6) should make this reliably
	// tape_exhausted; until then only the FAILURE is pinned, not the
	// code.
	_, err := runWithTape(t, tightTape, defNonTailSum+`s 2000`)
	if err == nil {
		t.Fatal("deep non-tail recursion under tight tape succeeded; it should exhaust the tape regardless of TCO")
	}
	t.Logf("non-tail exhaustion surfaced as: %v", err)
}

func TestIterationRunsInConstantTapeSpace(t *testing.T) {
	// The same tight ceiling comfortably runs MORE iterations as a for
	// loop: the loop body reuses one tape region. This is the baseline
	// that makes the non-tail pin meaningful (the ceiling is not
	// simply "too small for 2000 of anything").
	res, err := runWithTape(t, tightTape, `def acc 0 for [1 2001] [def acc (acc add i)] acc`)
	if err != nil {
		t.Fatalf("2000-iteration loop under tight tape: %v", err)
	}
	if len(res) == 0 || res[len(res)-1] != int64(2001000) {
		t.Fatalf("loop sum = %v, want 2001000", res)
	}
}

// TestTailRecursionDepthHeadroomDefaults documents that the DEFAULT
// tape config covers four-digit recursion depths — the working
// headroom users had before TCO. Constant-space replacement makes this
// trivially true; it stays as a guard against a silently-disabled
// gate combined with a shrunken default ceiling.
func TestTailRecursionDepthHeadroomDefaults(t *testing.T) {
	res, err := runWithTape(t, lang.TapeOptions{}, defTailSum+`s2 2000 0`)
	if err != nil {
		t.Fatalf("s2 2000 0 under default tape: %v", err)
	}
	if len(res) != 1 || res[0] != int64(2001000) {
		t.Fatalf("s2 2000 0 = %v, want 2001000", res)
	}
}
