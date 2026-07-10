package lang

// Multi-out closure bodies (CallableSpec.BodyOutResidual): `do` returns its
// body's ENTIRE residual, so a multi-value body compiles to ONE closure unit
// whose frameless RET leaves all N values — replacing the baked-const list
// that re-ran through an interpreter sub-engine at run time (classes 3+8 of
// design/DO-STRUCTURE-COMPILATION.0.md). Companion recorder work pinned here:
//   - out-of-order fn-unit residuals (an event above an inert bottom) promote
//     to frame locals and re-push in exact order (Finalize's forceOrder,
//     mirrored per unit);
//   - `error`'s ignore-handler compiles via StripsUnconsumedInput (the
//     runtime identity probe strips the unconsumed error from the residual
//     bottom), instead of islanding;
//   - an EMPTY body's closure residual is its pushed inputs verbatim,
//     matching InvokeBody — fixing the each []/fold []/scan [] miscompile
//     (compiled raised "body produced no result" against the interpreter's
//     identity map).

import (
	"fmt"
	"strings"
	"testing"
)

// compileDisasm compiles src and returns the disassembly, failing on refusal.
func compileDisasm(t *testing.T, src string) string {
	t.Helper()
	a, err := New()
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	prog, reason, _, cerr := a.CompileCheck(src)
	if cerr != nil || prog == nil {
		t.Fatalf("%q: expected to compile; reason=%q err=%v", src, reason, cerr)
	}
	return prog.Disassemble()
}

// requireEngineParity runs src on both engines and asserts identical rendered
// results; wantCompiled additionally requires the compiled path to have run.
func requireEngineParity(t *testing.T, src string, wantCompiled bool) {
	t.Helper()
	gotC, compiled, errC, gotI, errI := runBothEngines(t, src)
	if fmt.Sprint(gotC) != fmt.Sprint(gotI) || fmt.Sprint(errC) != fmt.Sprint(errI) {
		t.Errorf("%q: engine divergence: compiled=%v/%v interp=%v/%v", src, gotC, errC, gotI, errI)
	}
	if wantCompiled && !compiled {
		t.Errorf("%q: expected the compiled path to run", src)
	}
}

// TestDoMultiOutClosure — a multi-value inert body lowers to a TRUE closure
// (PUSH_CLOSURE + a unit RETting all N), not a baked const list run through a
// sub-engine; and the results seat for downstream consumption.
func TestDoMultiOutClosure(t *testing.T) {
	dis := compileDisasm(t, `do [10 20 30]`)
	if !strings.Contains(dis, "PUSH_CLOSURE") {
		t.Errorf("do [10 20 30]: want a PUSH_CLOSURE lowering, got:\n%s", dis)
	}
	if strings.Contains(dis, "FALLBACK") {
		t.Errorf("do [10 20 30]: must not island:\n%s", dis)
	}
	for _, src := range []string{
		`do [10 20 30]`,
		`do [10 20 30] add 5`,      // downstream word consumes the residual top
		`do [(1 add 2) (3 mul 4)]`, // two computed results
		`do [1 add 2 10 mul 4]`,    // out-of-order residual (event above literal)
		`def x 5  do [x 1 add 2]`,  // binding read + computed result
		`def x 5  do [x 1 add 2] add 100`,
		`do []`,
	} {
		requireEngineParity(t, src, true)
	}
}

// TestDoOutOfOrderResidualPromotes — the unit stores the computed result to a
// frame local and re-pushes the residual in exact order (the forceOrder
// mirror), rather than refusing "result above a literal".
func TestDoOutOfOrderResidualPromotes(t *testing.T) {
	dis := compileDisasm(t, `def x 5  do [x 1 add 2]`)
	if !strings.Contains(dis, "STORE_LOCAL") {
		t.Errorf("out-of-order residual: want a STORE_LOCAL promotion, got:\n%s", dis)
	}
}

// TestErrorStripInputClosure — the error-ignoring handler compiles as a paired
// closure (no island): the runtime strip nets one value from the 2-value
// residual whose bottom is the unconsumed error.
func TestErrorStripInputClosure(t *testing.T) {
	dis := compileDisasm(t, `do [raise x "e"] error ["fallback"]`)
	if strings.Contains(dis, "FALLBACK") {
		t.Errorf("error ignore-handler: must compile as a closure, not island:\n%s", dis)
	}
	for _, src := range []string{
		`do [raise x "e"] error ["fallback"]`,   // ignores the error → strip
		`do [raise x "e"] error [drop 9]`,       // consumes the error
		`do [raise x "e"] error []`,             // empty handler → error passes through
		`do [raise x "e"] error [dup drop "k"]`, // touches then nets one
		`do [5] error ["fb"]`,                   // success pass-through
		`do [1 div 0] error [drop -1]`,          // value-divergent body
	} {
		requireEngineParity(t, src, true)
	}
	// NEGATIVE: a handler netting >1 post-strip declines the closure (the
	// island/fallback owns it — parity still holds, it just isn't a closure).
	requireEngineParity(t, `do [raise x "e"] error ["a" "b"]`, false)
	// NEGATIVE: an all-inert-args deep handler also declines the ISLAND
	// (the baked forward prefix would exceed the barrier) — whole-program
	// fallback, parity intact.
	requireEngineParity(t, `5 error ["a" "b"]`, false)
	// NEGATIVE: a VARIADIC multi-out do body (arm counts differ) fails the
	// whole-residual exactness screen — the count is runtime-variable, so
	// the closure declines and the fallback owns it.
	requireEngineParity(t, `def b true  do [1 2 (if b [] [9 9])]`, false)
	// NEGATIVE: the historical forward-collection miscompile shape
	// (design/EDGE-SPEC-FINDINGS.0.md) must refuse or be correct — never
	// silently wrong.
	requireEngineParity(t, `5 do [raise x "e"] error [drop 9] add 1`, false)
}

// TestEmptyBodyClosureParity — an empty body's compiled closure returns its
// pushed inputs verbatim, matching InvokeBody: each/fold/scan identity-map
// behaviour is preserved under compilation (this DIVERGED before: the
// RET-nothing unit made the handlers raise "body produced no result").
func TestEmptyBodyClosureParity(t *testing.T) {
	for _, src := range []string{
		`[1 2] each []`,
		`[1 2 3] fold [] 0`,
		`[1 2 3] scan [] 0`,
	} {
		requireEngineParity(t, src, true)
	}
}

// TestDoSentinelBodyStaysUncompiled — a body carrying a flow-control sentinel
// still declines the closure (the sentinel targets an enclosing loop the VM
// cannot reach across the call boundary); the fallback owns it, and parity
// holds.
func TestDoSentinelBodyStaysUncompiled(t *testing.T) {
	src := `for 3 [ do [break] drop ]`
	gotC, _, errC, gotI, errI := runBothEngines(t, src)
	if fmt.Sprint(gotC) != fmt.Sprint(gotI) || fmt.Sprint(errC) != fmt.Sprint(errI) {
		t.Errorf("%q: engine divergence: compiled=%v/%v interp=%v/%v", src, gotC, errC, gotI, errI)
	}
}
