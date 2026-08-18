package modules

import (
	"strings"
	"testing"

	"github.com/boru-lang/boru/lang/go/native"
	parser "github.com/boru-lang/boru/parser/go"
)

// The check-prop count guard (requirePropCount). A non-positive `runs`
// used to run the property loop ZERO times and report `{ok: true,
// runs: 0}` — which Test.report prints as a PASS even when the property
// body is literally `[false]`. A vacuous run must not report success, so
// the driver raises `range_error` naming the offending argument instead
// of silently clamping to 1: clamping would run once and pass, hiding
// the caller's wrong count rather than reporting it.

// runTestBoruErr runs boru source expecting a raise, and returns the
// error. The counterpart of runTestBoru, which fails on any error.
func runTestBoruErr(t *testing.T, r *native.Registry, src string) error {
	t.Helper()
	values, err := parser.Parse(src)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	out, err := native.NewTop(r).Run(values)
	if err == nil {
		t.Fatalf("expected an error, got out=%v\n--- src ---\n%s", out, src)
	}
	return err
}

// assertCountGuard checks a raise is the count guard's, names the
// offending argument, and quotes the value the caller passed.
func assertCountGuard(t *testing.T, err error, arg, got string) {
	t.Helper()
	msg := err.Error()
	for _, want := range []string{"range_error", "Test.check-prop", arg, got} {
		if !strings.Contains(msg, want) {
			t.Errorf("error %q does not mention %q", msg, want)
		}
	}
}

// TestCheckPropGuard_MinimalRunsPasses is the POSITIVE case: the
// smallest legal configuration — runs=1 (the floor) with max-shrinks=0
// (the documented "do not shrink" setting, also a floor) — still runs
// the property and reports a genuine pass with runs=1.
func TestCheckPropGuard_MinimalRunsPasses(t *testing.T) {
	r := testRegistry(t)
	src := `Test.check-prop "floor" [r.int 0 100] [0 gte] 1 1 0`
	ok := runTestAndGetField(t, r, src, "ok")
	b, err := ok.AsConcreteBoolean()
	if err != nil {
		t.Fatalf("ok field not Boolean: %v", err)
	}
	if !b {
		t.Errorf("runs=1 max-shrinks=0 should pass, got ok=false")
	}
	runsV := runTestAndGetField(t, r, src, "runs")
	n, err := runsV.AsConcreteInteger()
	if err != nil {
		t.Fatalf("runs field not Integer: %v", err)
	}
	if n != 1 {
		t.Errorf("runs=%d, want 1 — the property must actually run", n)
	}
}

// TestCheckPropGuard_ZeroRunsRaises is the headline NEGATIVE case: the
// vacuous run. The property body is `[false]`, so any run at all fails;
// runs=0 used to report ok=true.
func TestCheckPropGuard_ZeroRunsRaises(t *testing.T) {
	r := testRegistry(t)
	err := runTestBoruErr(t, r, `Test.check-prop "vacuous" [r.int 0 10] [false] 0 1 0`)
	assertCountGuard(t, err, "runs", "(got 0)")

	// And nothing was recorded: a rejected configuration must not leave
	// a result in the run for Test.report to print as a pass.
	if got := len(activeRun(r).results); got != 0 {
		t.Errorf("rejected call recorded %d result(s), want 0", got)
	}
}

// TestCheckPropGuard_NegativeRunsRaises pins the other side of the
// floor — a negative count is the same caller mistake.
func TestCheckPropGuard_NegativeRunsRaises(t *testing.T) {
	r := testRegistry(t)
	err := runTestBoruErr(t, r, `Test.check-prop "neg" [r.int 0 10] [false] -3 1 0`)
	assertCountGuard(t, err, "runs", "-3")
}

// TestCheckPropGuard_NegativeMaxShrinksRaises pins the second numeric
// argument with the same hole: a negative max-shrinks silently behaved
// as 0 (both reducers bail on `maxShrinks <= 0`). Zero is legal — it is
// the documented no-shrink setting, exercised by
// TestCheckPropGuard_MinimalRunsPasses — so only negatives raise.
func TestCheckPropGuard_NegativeMaxShrinksRaises(t *testing.T) {
	r := testRegistry(t)
	err := runTestBoruErr(t, r, `Test.check-prop "neg-shrinks" [r.int 0 10] [false] 5 1 -7`)
	assertCountGuard(t, err, "max-shrinks", "-7")
}

// TestCheckPropGuard_NegativeSeedAllowed pins what is deliberately NOT
// guarded: every integer is a legal seed for the per-iteration rand
// instance, so a negative seed must still run.
func TestCheckPropGuard_NegativeSeedAllowed(t *testing.T) {
	r := testRegistry(t)
	src := `Test.check-prop "neg-seed" [r.int 0 100] [0 gte] 3 -11 0`
	ok := runTestAndGetField(t, r, src, "ok")
	b, _ := ok.AsConcreteBoolean()
	if !b {
		t.Errorf("a negative seed should be accepted and pass, got ok=false")
	}
}

// TestCheckPropGuard_ViaRunProperty confirms the guard also covers the
// declarative surface: run-property feeds the PropertySpec map's own
// runs/seed/max-shrinks straight into the same driver, so a spec built
// with runs=0 raises rather than reporting a vacuous pass.
func TestCheckPropGuard_ViaRunProperty(t *testing.T) {
	r := testRegistry(t)
	src := `
		def p (Test.prop "spec-vacuous" [r.int 0 10] [false])
		def q (p set "runs" 0)
		q Test.run-property
	`
	err := runTestBoruErr(t, r, src)
	assertCountGuard(t, err, "runs", "(got 0)")
}
