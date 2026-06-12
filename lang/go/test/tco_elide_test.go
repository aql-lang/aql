package test

import (
	"fmt"
	"strings"
	"testing"

	"github.com/aql-lang/aql/eng/go/parser"
	"github.com/aql-lang/aql/lang/go/native"
)

// End-to-end battery for the shell-variant tail-call elision
// (design/TCO-STAGED.0.md Stage 3). Two things are pinned per shape:
// the RESULT, which must be byte-identical with elision on, off, or
// inapplicable — correctness never depends on TCO firing — and the
// Elided/Detected counters, which pin exactly where the gate fires.
// (The full spec suite runs in both modes in test/go/langspec — that
// is the broad differential; these are the targeted counters.)

func runTCO(t *testing.T, src string, disable bool) (string, int64, int64) {
	t.Helper()
	reg, err := native.DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	reg.TCO.Disable = disable
	values, err := parser.Parse(src)
	if err != nil {
		t.Fatal(err)
	}
	res, err := native.NewTop(reg).Run(values)
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	out := ""
	for i, v := range res {
		if i > 0 {
			out += " "
		}
		out += v.String()
	}
	return out, reg.TCO.Detected, reg.TCO.Elided
}

// assertDual runs src with elision on and off, asserting the result is
// identical in both modes and the counters match expectations.
func assertDual(t *testing.T, name, src, want string, wantDetected, wantElided int64) {
	t.Helper()
	got, detected, elided := runTCO(t, src, false)
	if got != want {
		t.Errorf("%s: result %q, want %q", name, got, want)
	}
	if detected != wantDetected || elided != wantElided {
		t.Errorf("%s: detected/elided = %d/%d, want %d/%d", name, detected, elided, wantDetected, wantElided)
	}
	off, _, offElided := runTCO(t, src, true)
	if off != got {
		t.Errorf("%s: result differs with TCO disabled: %q vs %q", name, off, got)
	}
	if offElided != 0 {
		t.Errorf("%s: Disable=true still elided %d frames", name, offElided)
	}
}

func TestElideTailSelfRecursion(t *testing.T) {
	assertDual(t, "accumulator",
		`def s2 fn [[n:Integer acc:Integer] [Integer] [if (n lte 0) [acc] [s2 (n sub 1) (acc add n)]]] s2 5 0`,
		"15", 5, 5)
}

func TestElideKeepsArgsPerFrame(t *testing.T) {
	// args.0 must read the CURRENT frame's args after the previous
	// frame's Args entry was popped eagerly and the new one pushed.
	assertDual(t, "args visibility",
		`def f fn [[n:Integer] [Integer] [if (n lte 0) [args.0] [f (n sub 1)]]] f 3`,
		"0", 3, 3)
}

func TestElideKeepsCapturesAcrossChain(t *testing.T) {
	// The recursive inner fn captures an enclosing local; every elided
	// call reinstalls the same construction-time snapshot. Detected is
	// 4: the initial `go 3` is itself a tail call of out1's frame —
	// detected, but declined (different overload, the mutual shape);
	// the 3 self-recursive calls elide.
	assertDual(t, "captured local",
		`def out1 fn [[] [Integer] [def x 7 def go fn [[n:Integer] [Integer] [if (n lte 0) [x] [go (n sub 1)]]] go 3]] out1`,
		"7", 4, 3)
}

func TestElideValueAccretingBodyKeepsValues(t *testing.T) {
	// Values parked below the call live in the kept shell — the shell
	// variant elides these frames WITHOUT dropping the values. The
	// result must be identical to nesting.
	assertDual(t, "value-accreting",
		`def m fn [[n:Integer] [] [if (n lte 0) [] [n mul 2 m (n sub 1)]]] m 3`,
		"6 4 2", 3, 3)
}

func TestElideDeclinesMutualRecursion(t *testing.T) {
	// Tail calls, detected — but the callee is a different overload,
	// so Stage 3 nests them (Elided 0).
	assertDual(t, "mutual",
		`def isod fn [[n:Integer] [Boolean] [if (n eq 0) [false] [isev (n sub 1)]]] `+
			`def isev fn [[n:Integer] [Boolean] [if (n eq 0) [true] [isod (n sub 1)]]] isev 4`,
		"true", 4, 0)
}

func TestElideDeclinesArgAutoEvalMutations(t *testing.T) {
	// Each tail call's map arg runs `def zz 1` during auto-eval: a
	// binding mutation between collection and dispatch. Today that
	// binding is visible to the callee until the caller's DefCleanup
	// runs; eager teardown would remove it first — so the gate must
	// decline and nest.
	assertDual(t, "auto-eval def",
		`def s3 fn [[m:Map n:Integer] [Integer] [if (n lte 0) [n] [s3 {v:(def zz 1 zz)} (n sub 1)]]] s3 {v:0} 3`,
		"0", 3, 0)
}

func TestElideExtendsTapeHeadroom(t *testing.T) {
	// The shell variant's resource claim, pinned under a tight tape
	// ceiling (2048 growing once to 4096 entries): depth 500 exhausts
	// the tape with elision off (~13 parked tokens/call) and fits with
	// it on (~4-5 shell tokens/call). NOT yet constant space — the
	// shell still accretes, so a much deeper chain exhausts either
	// way, degrading gracefully (elisions fire until the ceiling).
	// Full frame replacement (design/TCO-STAGED.0.md Stage 4) removes
	// the shell residue and flips the deep case to evaluation_limit.
	src := `def s2 fn [[n:Integer acc:Integer] [Integer] [if (n lte 0) [acc] [s2 (n sub 1) (acc add n)]]] `
	tight := native.TapeConfig{InitialSize: 2048, MaxGrows: 1, GrowthFactor: 2.0}

	run := func(depth string, disable bool) (string, int64, error) {
		reg, err := native.DefaultRegistry()
		if err != nil {
			t.Fatal(err)
		}
		reg.TCO.Disable = disable
		reg.TapeConfig = tight
		values, err := parser.Parse(src + `s2 ` + depth + ` 0`)
		if err != nil {
			t.Fatal(err)
		}
		res, err := native.NewTop(reg).Run(values)
		out := ""
		if len(res) == 1 {
			out = res[0].String()
		}
		return out, reg.TCO.Elided, err
	}

	if got, elided, err := run("500", false); err != nil || got != "125250" || elided != 500 {
		t.Errorf("depth 500 elided: got %q elided=%d err=%v, want 125250/500/nil", got, elided, err)
	}
	if _, _, err := run("500", true); err == nil || !strings.Contains(err.Error(), "tape_exhausted") {
		t.Errorf("depth 500 disabled: err=%v, want tape_exhausted", err)
	}
	if _, elided, err := run("5000", false); err == nil || !strings.Contains(err.Error(), "tape_exhausted") {
		t.Errorf("depth 5000 elided: err=%v, want tape_exhausted (shell residue is O(depth) until Stage 4)", err)
	} else if elided < 500 {
		t.Errorf("depth 5000: only %d elisions before exhaustion — elision should degrade gracefully", elided)
	}
}

func TestElideDeepChainStaysFlat(t *testing.T) {
	// A longer chain: every recursive call elides, the per-call stacks
	// stay O(1), and the result is exact.
	src := `def s2 fn [[n:Integer acc:Integer] [Integer] [if (n lte 0) [acc] [s2 (n sub 1) (acc add n)]]] s2 5000 0`
	got, detected, elided := runTCO(t, src, false)
	if got != fmt.Sprintf("%d", 5000*5001/2) {
		t.Errorf("s2 5000 0 = %s, want %d", got, 5000*5001/2)
	}
	if detected != 5000 || elided != 5000 {
		t.Errorf("detected/elided = %d/%d, want 5000/5000", detected, elided)
	}
}
