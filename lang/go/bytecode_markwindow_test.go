package lang

import (
	"fmt"
	"testing"
)

// The mark-window island (plan Phase 5, L-DO part 2b): a fallible do-catch
// residual — 1 caught Error vs N values at run time — plus the values above
// it lowers as ONE verbatim window: Finalize's markWindowShape opens an
// OpStackMark before the region-starting do event, nothing re-pushes
// (verifyMarkWindow pins the residual IS the lowered stack), and
// OpCallDynMixedFromMark re-steps stack[mark:] through the island exactly as
// the interpreter — auto-apply hazard included.

const mwDocMod = `import module [ def dec fn [[bad:Boolean x:Any] [Any] [ if bad [raise bad_input "boom"] [x] ]] def boom fn [[x:Any] [Any] [ raise bad_input "always" ]] export "M" {dec: dec/r, boom: boom/r} ] end `

// mwParityCompiled asserts the row force-compiles and runs COMPILED with
// value/error parity against the interpreter.
func mwParityCompiled(t *testing.T, src string) {
	t.Helper()
	a := mustNew(t)
	if _, reason, _, err := a.CompileCheck(src); err != nil || reason != "" {
		t.Fatalf("the shape must compile, got refusal %q / err %v\n  src: %s", reason, err, src)
	}
	b := mustNew(t)
	outC, ran, errC := b.RunCompiled(src)
	if !ran {
		t.Fatalf("the shape must run COMPILED, fell back (err %v)", errC)
	}
	c := mustNew(t)
	outI, errI := c.RunInterp(src)
	if (errC == nil) != (errI == nil) || fmt.Sprint(outC) != fmt.Sprint(outI) {
		t.Errorf("parity: compiled %v/%v != interp %v/%v\n  src: %s", outC, errC, outI, errI, src)
	}
}

// mwRefusedWithParity asserts the row still refuses and the fallback matches
// the interpreter exactly.
func mwRefusedWithParity(t *testing.T, src, wantReason string) {
	t.Helper()
	a := mustNew(t)
	prog, reason, _, err := a.CompileCheck(src)
	if err != nil {
		t.Fatalf("CompileCheck: %v", err)
	}
	if prog != nil {
		t.Fatalf("the shape GRADUATED (it now compiles) — move it into the compiles battery and graduate its ledger row\n  src: %s", src)
	}
	if wantReason != "" && reason != wantReason {
		t.Errorf("refusal reason drifted: %q, want %q — re-diagnose", reason, wantReason)
	}
	b := mustNew(t)
	outC, ran, errC := b.RunCompiled(src)
	if ran {
		t.Fatal("refused program must fall back")
	}
	c := mustNew(t)
	outI, errI := c.RunInterp(src)
	if (errC == nil) != (errI == nil) || fmt.Sprint(outC) != fmt.Sprint(outI) {
		t.Errorf("fallback parity: compiled %v/%v != interp %v/%v", outC, errC, outI, errI)
	}
}

func TestMarkWindowDoCatchCompiles(t *testing.T) {
	rows := []string{
		// The always-raising module fn: the caught path (1 Error in the
		// region, "x" never pushed at run time).
		mwDocMod + `do [(M.boom 5) "x"] error [dot code]`,
		// The [Any]-returning user fn twin.
		`def f fn [[x:Any] [Any] [raise bad_input "nope"]]  do [(f 5) 2] error [dot code]`,
		// The branch-arm nesting: the raise sits inside a taken if arm.
		mwDocMod + `do [(if true [M.boom 5] [7]) 8] error [dot code]`,
		// The dry-pass-proven raising constant (the StructUtil chained leaf).
		`import "aql:struct-util"  def g StructUtil.parse/r  do [(g "") 2] error [dot code]`,
	}
	for i, src := range rows {
		t.Run(fmt.Sprintf("row-%d", i), func(t *testing.T) {
			mwParityCompiled(t, src)
		})
	}
}

// TestMarkWindowDeclinesKeepParity — the shapes the window rightly declines:
// a PROMOTED def read (`def msg (…) msg` — the def popped the result to a
// frame slot, so it is not live in the window). Its ledger row stays in
// frontier-do-catch.tsv; this pin fires when a later widening graduates it.
// (The module-export-in-region sibling graduated 2026-07-17 — §9.1.)
func TestMarkWindowDeclinesKeepParity(t *testing.T) {
	// Legacy refusal+fallback-parity contract: pins the one-release
	// AQL_COMPILE_FALLBACK=1 hatch behavior (Stage J flipped the default
	// to compile_refused; migrate this contract or retire it with the hatch).
	t.Setenv("AQL_COMPILE_FALLBACK", "1")
	mwRefusedWithParity(t,
		mwDocMod+`def msg (do [(true 5 M.dec) "no-raise"] error [dot code])  msg`,
		"residual shape beyond Stage 1 (call result above a literal)")
	// GRADUATED 2026-07-17 (§9.1): the module-export-in-region row compiles —
	// the identity-less ExtensionPayload out mints an ID at the dyn-body
	// record, restoring its event linkage, so the mark window owns the
	// region. Pinned by its frontier spec row (frontier-do-catch.tsv:18), whose
	// ledger entry graduated.
	// The 0-arg-lambda wrap: the do-catch runs inside a FN UNIT, whose finish
	// seats its residual through the RET reconciliation — the program-level
	// window arms on the variadic CALL event but the lowered stack is the
	// call's seated results, not the window's event slots, so verifyMarkWindow
	// pins the mismatch and the program falls back whole.
	mwRefusedWithParity(t,
		`def wrap ([] => [def f fn [[x:Any] [Any] [raise bad_input "nope"]]  do [(f 5) 2] error [dot code]]) wrap`,
		"mark-window residual does not match the lowered stack")
}
