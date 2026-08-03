package lang

import (
	gofmt "fmt"
	"strings"
	"testing"
)

// check_fn_param_apply_def_fp_test.go pins an OPEN checker false positive
// (triaged 2026-08, surfaced by the widened property fuzzer —
// checker-compiler-completeness-review.0.md §8.1(3), the +74 entry in
// test/go/langspec/check_run_fp_test.go):
//
//	def zh fn [[f:Function x:Integer] [Integer] [def zr (f x) f zr]]
//	zh ([za:Integer] => [za add 1]) 5
//
// The def-split idiom — binding a Function-PARAM application's result, then
// using it — flags `undefined_word: zr` at error severity on a program that
// RUNS CLEAN (7). Mechanism: the body analysis leaves `(f x)` as the
// un-collapsed [Function-carrier, arg] residual (a strict Function carrier
// is not modelled as an application; tryDynamicFnValueDispatch admits only
// DYNAMIC bounds, and its evaluation-fixed window scanner rejects param
// reads), so the pending `def` collection never completes and `zr` stays
// unbound in the model. The graduation is a checker-precision widening —
// collapse a strict Function-typed carrier's application window to ONE
// dynamic(Any) on the plain-check surface, without disturbing the compile
// pass's residual model (the RetReplay / resolveDynamicApply machinery
// depends on the un-collapsed window).
//
// This test is the EXPECTED-OPEN ledger entry (the frontier-ledger
// discipline): the first assertion FAILS when the FP is fixed — flip both
// assertions to their positive forms and drop the check_run_fp pin's +74
// share when that happens.
func TestFnParamApplyDefFalsePositiveOpen(t *testing.T) {
	src := `def zh fn [[f:Function x:Integer] [Integer] [def zr (f x) f zr]] zh ([za:Integer] => [za add 1]) 5`

	// 1. The FP still fires: an error-severity undefined_word on zr.
	a, _ := New()
	res, err := a.Check(src)
	if err != nil {
		t.Fatalf("check harness error: %v", err)
	}
	found := false
	for _, d := range res.Diagnostics {
		if d.Code == "undefined_word" && strings.Contains(d.Detail, "zr") && d.Severity == SeverityError {
			found = true
		}
	}
	if !found {
		t.Errorf("the def-split FP no longer fires — GRADUATE this ledger entry: "+
			"flip the assertions, drop the check_run_fp +74 pin share, and record the fix. Diagnostics: %v", res.Diagnostics)
	}

	// 2. The program the checker rejects runs clean — that is what makes the
	// diagnostic a FALSE positive rather than a mirror.
	b, _ := New()
	out, rerr := b.RunInterp(src)
	if rerr != nil {
		t.Fatalf("the FP witness must run clean on the interpreter, got %v", rerr)
	}
	if len(out) != 1 || gofmt.Sprint(out[0]) != "7" {
		t.Errorf("witness result drifted: %v (want [7])", out)
	}
}
