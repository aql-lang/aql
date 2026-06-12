package test

import (
	"fmt"
	"strings"
	"testing"

	"github.com/aql-lang/aql/eng/go/parser"
	"github.com/aql-lang/aql/lang/go/native"
)

// End-to-end battery for tail-call elimination on direct
// self-recursion (design/TCO-STAGED.0.md Stages 3+4a). Two things are
// pinned per shape: the RESULT, which must be byte-identical with TCO
// on, off, or inapplicable — correctness never depends on firing — and
// the Detected/Elided/Replaced counters, which pin exactly which path
// each shape takes: FULL replacement (clean frame interior, zero
// residue), SHELL elision (values parked below the call, shell kept),
// or nesting (gate declined). (The full spec suite runs in both modes
// in test/go/langspec — that is the broad differential; these are the
// targeted counters.)

func runTCO(t *testing.T, src string, disable bool) (string, int64, int64, int64) {
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
	return out, reg.TCO.Detected, reg.TCO.Elided, reg.TCO.Replaced
}

// assertDual runs src with TCO on and off, asserting the result is
// identical in both modes and the counters land on the expected path.
func assertDual(t *testing.T, name, src, want string, wantDetected, wantElided, wantReplaced int64) {
	t.Helper()
	got, detected, elided, replaced := runTCO(t, src, false)
	if got != want {
		t.Errorf("%s: result %q, want %q", name, got, want)
	}
	if detected != wantDetected || elided != wantElided || replaced != wantReplaced {
		t.Errorf("%s: detected/elided/replaced = %d/%d/%d, want %d/%d/%d",
			name, detected, elided, replaced, wantDetected, wantElided, wantReplaced)
	}
	off, _, offElided, offReplaced := runTCO(t, src, true)
	if off != got {
		t.Errorf("%s: result differs with TCO disabled: %q vs %q", name, off, got)
	}
	if offElided != 0 || offReplaced != 0 {
		t.Errorf("%s: Disable=true still fired %d/%d", name, offElided, offReplaced)
	}
}

func TestElideTailSelfRecursion(t *testing.T) {
	// Clean frame interior → full replacement on every recursive call.
	assertDual(t, "accumulator",
		`def s2 fn [[n:Integer acc:Integer] [Integer] [if (n lte 0) [acc] [s2 (n sub 1) (acc add n)]]] s2 5 0`,
		"15", 5, 0, 5)
}

func TestElideKeepsArgsPerFrame(t *testing.T) {
	// args.0 must read the CURRENT frame's args after the previous
	// frame's Args entry was popped eagerly and the new one pushed.
	assertDual(t, "args visibility",
		`def f fn [[n:Integer] [Integer] [if (n lte 0) [args.0] [f (n sub 1)]]] f 3`,
		"0", 3, 0, 3)
}

func TestElideKeepsCapturesAcrossChain(t *testing.T) {
	// The recursive inner fn captures an enclosing local; every
	// replaced call reinstalls the same construction-time snapshot.
	// Detected is 4: the initial `go 3` is itself a tail call of
	// out1's frame — detected, but declined (different overload, the
	// mutual shape); the 3 self-recursive calls replace.
	assertDual(t, "captured local",
		`def out1 fn [[] [Integer] [def x 7 def go fn [[n:Integer] [Integer] [if (n lte 0) [x] [go (n sub 1)]]] go 3]] out1`,
		"7", 4, 0, 3)
}

func TestElideValueAccretingBodyKeepsValues(t *testing.T) {
	// Values parked below the call live in the kept shell — these
	// frames take the SHELL path (full replacement would drop the
	// values), and the result must be identical to nesting.
	assertDual(t, "value-accreting",
		`def m fn [[n:Integer] [] [if (n lte 0) [] [n mul 2 m (n sub 1)]]] m 3`,
		"6 4 2", 3, 3, 0)
}

func TestElideDeclinesMutualRecursion(t *testing.T) {
	// Tail calls, detected — but the callee is a different overload,
	// so this stage nests them.
	assertDual(t, "mutual",
		`def isod fn [[n:Integer] [Boolean] [if (n eq 0) [false] [isev (n sub 1)]]] `+
			`def isev fn [[n:Integer] [Boolean] [if (n eq 0) [true] [isod (n sub 1)]]] isev 4`,
		"true", 4, 0, 0)
}

func TestElideDeclinesArgAutoEvalMutations(t *testing.T) {
	// Each tail call's map arg runs `def zz 1` during auto-eval: a
	// binding mutation between collection and dispatch. Today that
	// binding is visible to the callee until the caller's DefCleanup
	// runs; eager teardown would remove it first — so the gate must
	// decline and nest.
	assertDual(t, "auto-eval def",
		`def s3 fn [[m:Map n:Integer] [Integer] [if (n lte 0) [n] [s3 {v:(def zz 1 zz)} (n sub 1)]]] s3 {v:0} 3`,
		"0", 3, 0, 0)
}

func TestReplaceConstantSpace(t *testing.T) {
	// THE Stage-4a claim: a self-recursive tail chain runs in O(1)
	// tape. Depth 10000 under a TINY ceiling (512 entries growing once
	// to 1024 — far below the ~13 tokens/call the chain would park
	// without TCO, and below the ~5/call the shell variant leaves).
	src := `def s2 fn [[n:Integer acc:Integer] [Integer] [if (n lte 0) [acc] [s2 (n sub 1) (acc add n)]]] s2 10000 0`
	reg, err := native.DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	reg.TapeConfig = native.TapeConfig{InitialSize: 512, MaxGrows: 1, GrowthFactor: 2.0}
	values, err := parser.Parse(src)
	if err != nil {
		t.Fatal(err)
	}
	res, err := native.NewTop(reg).Run(values)
	if err != nil {
		t.Fatalf("depth 10000 under a 1024-entry ceiling: %v", err)
	}
	if len(res) != 1 || res[0].String() != fmt.Sprintf("%d", 10000*10001/2) {
		t.Fatalf("s2 10000 0 = %v, want %d", res, 10000*10001/2)
	}
	if reg.TCO.Replaced != 10000 {
		t.Errorf("replaced = %d, want 10000", reg.TCO.Replaced)
	}

	// And the same chain with TCO disabled must exhaust that ceiling —
	// the pin that proves the test would catch a silently broken gate.
	reg2, err := native.DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	reg2.TCO.Disable = true
	reg2.TapeConfig = native.TapeConfig{InitialSize: 512, MaxGrows: 1, GrowthFactor: 2.0}
	values2, err := parser.Parse(src)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := native.NewTop(reg2).Run(values2); err == nil || !strings.Contains(err.Error(), "tape_exhausted") {
		t.Errorf("disabled run err = %v, want tape_exhausted", err)
	}
}

func TestElideDeepChainStaysFlat(t *testing.T) {
	// A longer chain under the DEFAULT tape: every recursive call
	// takes full replacement and the result is exact.
	src := `def s2 fn [[n:Integer acc:Integer] [Integer] [if (n lte 0) [acc] [s2 (n sub 1) (acc add n)]]] s2 5000 0`
	got, detected, _, replaced := runTCO(t, src, false)
	if got != fmt.Sprintf("%d", 5000*5001/2) {
		t.Errorf("s2 5000 0 = %s, want %d", got, 5000*5001/2)
	}
	if detected != 5000 || replaced != 5000 {
		t.Errorf("detected/replaced = %d/%d, want 5000/5000", detected, replaced)
	}
}
