package lang

import (
	"fmt"
	"strings"
	"testing"
)

// nur101Refusal pins a program the compiler must REFUSE rather than answer,
// together with the interpreter's answer it must not contradict. It is the
// fence shape NUR101's landing uses: a refusal is sound (the fallback runs the
// tree-walker, which is by definition right), a silent wrong answer is not.
//
// wantInterp is asserted against RunInterp — NEVER against Run, which post
// Stage J is the compiled lane (NUR106).
func nur101Refusal(t *testing.T, src, wantInterp string) {
	t.Helper()
	prog, reason, _, _ := mustNew(t).CompileCheck(src)
	if prog != nil {
		t.Errorf("%q: compiled — graduate this fence to a parity row (reason was %q)", src, reason)
	}
	// Asserted on wasCompiled, not on the error: under the one-release
	// BORU_COMPILE_FALLBACK=1 hatch the library runs the fallback itself and
	// returns no error, so the error text is not a stable refusal signal.
	gotC, compiled, errC := mustNew(t).RunCompiled(src)
	if compiled {
		t.Errorf("%q: ran compiled; want the interpreter fallback", src)
	}
	got, err := mustNew(t).RunInterp(src)
	if err != nil || fmt.Sprint(got) != wantInterp {
		t.Errorf("%q: interp = %v (%v), want %s", src, got, err, wantInterp)
	}
	// The fallback must answer exactly as a plain interpreted run — when it
	// ran at all (without the hatch RunCompiled returns the refusal instead).
	if errC == nil && fmt.Sprint(gotC) != fmt.Sprint(got) {
		t.Errorf("%q: fallback=%v interp=%v", src, gotC, got)
	}
	if errC != nil && !strings.Contains(fmt.Sprint(errC), "compile_refused") {
		t.Errorf("%q: err=%v, want compile_refused", src, errC)
	}
}

// TestParenReStepRule is the standing measurement behind
// design/PAREN-RESTEP-RULE.0.md: for every shape the rule classifies, the
// compiled lane either AGREES with the tree-walking interpreter or REFUSES.
// It never answers differently.
//
// The rule: a Function a paren PLACED is re-stepped into a CALL exactly when
// it leads two or more survivors of an enclosing group that closes with a
// paren rewind — a user paren, an fn frame, or an `if` / `for` / `do` body.
// The program top level, list literals and map literals do not rewind.
//
// Five rows here were SILENT MISCOMPILES before 2026-08-27, in both
// directions, and all five were invisible to the suite because the tests that
// covered them used `Run` as their interpreter oracle (NUR106).
func TestParenReStepRule(t *testing.T) {
	const mk = `def mk fn [[a:Integer] [Function] [(fn [[b:Integer] [Integer] [a add b]])]] `
	const inc = `def inc fn [[n:Integer] [Integer] [n add 1]] `

	for _, c := range []struct {
		src  string
		want string // the INTERPRETER's answer — the only oracle
		why  string
	}{
		{mk + `(mk 1) 2`, "[fn (Integer) 2]", "program residual: nothing rewinds, so the carrier is placed"},
		{mk + `((mk 1) 2)`, "[3]", "the outer paren rewinds onto the carrier and dispatches it"},
		{mk + `[(mk 1) 2]`, "[[fn (Integer) 2]]", "a list literal does not rewind: two elements"},
		// `[((mk 1) 2)]` belongs here and is DELIBERATELY absent: it is the one
		// shape still answering differently, pinned on its own in
		// TestParenReStepKnownDivergence so the divergence is tracked rather
		// than mixed in with the rows that hold.
		{mk + `if true [(mk 1) 2]`, "[3]", "the arm's frame rewinds"},
		{mk + `for 2 [(mk 1) 2]`, "[3 3]", "the loop body's frame rewinds, once per iteration"},
		{mk + `do [(mk 1) 2]`, "[3]", "the do body's frame rewinds"},
		{mk + `case 1 [1] [(mk 1) 2]`, "[1 [fn (Integer) 2]]", "a case arm is a literal, not a frame"},
		{inc + `(inc/v) 7`, "[fn inc(Integer) 7]", "one survivor: a reference places, per NUR073 BROAD"},
		{inc + `((inc/v) 7)`, "[8]", "two survivors: the outer paren dispatches the placed reference"},
	} {
		gotI, errI := mustNew(t).RunInterp(c.src)
		if errI != nil {
			t.Errorf("%q: interp errored: %v", c.src, errI)
			continue
		}
		if fmt.Sprint(gotI) != c.want {
			t.Errorf("%q: interp = %v, want %s (%s)", c.src, gotI, c.want, c.why)
			continue
		}
		gotC, compiled, errC := mustNew(t).RunCompiled(c.src)
		if !compiled {
			continue // a refusal is sound; the per-shape fences below pin which ones
		}
		if errC != nil || fmt.Sprint(gotC) != fmt.Sprint(gotI) {
			t.Errorf("%q: DIVERGENCE — compiled=%v (%v) interp=%v (%s)", c.src, gotC, errC, gotI, c.why)
		}
	}
}

// TestParenReStepKnownDivergence pins the ONE shape still answering
// differently, so it cannot regress further or be fixed unnoticed.
//
// `[((mk 1) 2)]`: the list-literal assembly receives the paren's survivors as
// ELEMENTS, and in check mode the inner paren has not applied — so RecordMakeList
// sees exactly the same `[carrier, 2]` it sees for `[(mk 1) 2]`, which the
// interpreter really does place as two elements. The two shapes are
// indistinguishable at that point; refusing on the lead would break the correct
// one. Closing it needs the apply RECORDED at the paren, which is Stage 3.
func TestParenReStepKnownDivergence(t *testing.T) {
	const src = `def mk fn [[a:Integer] [Function] [(fn [[b:Integer] [Integer] [a add b]])]] [((mk 1) 2)]`
	gotI, errI := mustNew(t).RunInterp(src)
	gotC, compiled, errC := mustNew(t).RunCompiled(src)
	if errI != nil || errC != nil || !compiled {
		t.Fatalf("setup: interp=%v/%v compiled=%v/%v", gotI, errI, gotC, compiled)
	}
	if fmt.Sprint(gotI) != "[[3]]" {
		t.Errorf("interp = %v, want [[3]] — the inner paren rewinds and dispatches", gotI)
	}
	if fmt.Sprint(gotC) != "[[fn (Integer) 2]]" {
		t.Errorf("compiled = %v, want [[fn (Integer) 2]] — if this changed, the NUR101 "+
			"divergence is CLOSED: delete this test and add the row to TestParenReStepRule", gotC)
	}
}
