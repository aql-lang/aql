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
		{mk + `[((mk 1) 2)]`, "[[3]]", "the inner paren rewinds: one element (refused — see below)"},
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

// TestParenReStepPlacedLayoutCompiles ratchets what the placed record BUYS,
// not just what it prevents. A residual whose lead a user paren placed and no
// enclosing paren re-stepped is inert on both lanes, so the layout may simply
// lay it out — where before it refused, reading the absence of a record as
// evidence of a hazard.
//
// These graduated 2026-08-27 with Stage 3's first increment. Pinned as
// POSITIVE so the coverage cannot quietly regress to a refusal: the rule test
// above tolerates refusals by design, which is what makes it safe to extend
// and useless as a ratchet.
func TestParenReStepPlacedLayoutCompiles(t *testing.T) {
	for _, c := range []struct{ src, want, why string }{
		{`def inc fn [[n:Integer] [Integer] [n add 1]] (inc/v) 7`,
			"[fn inc(Integer) 7]",
			"a CONCRETE fn value placed by a user paren — the record's fourth shape"},
		{`def tbl {double:(a:Integer => [mul 2 a])} def k 'double' (tbl get k) 5`,
			"[fn (Integer) 5]",
			"a DYNAMIC maybe-callable placed by a user paren"},
		{`def f fn [[x:Any] [Any] [x]] end def m {p: f/v} end (m.p 5) (m.p 7)`,
			"[5 7]",
			"the graduated NUR038 seal row: each paren applies, and two placed results are not a hazard"},
	} {
		prog, reason, _, cerr := mustNew(t).CompileCheck(c.src)
		if cerr != nil || prog == nil {
			t.Errorf("%q: REGRESSED to a refusal (%s) — %s", c.src, reason, c.why)
			continue
		}
		gotC, compiled, errC := mustNew(t).RunCompiled(c.src)
		gotI, errI := mustNew(t).RunInterp(c.src)
		if !compiled || errC != nil || errI != nil {
			t.Errorf("%q: compiled=%v errC=%v errI=%v", c.src, compiled, errC, errI)
			continue
		}
		if fmt.Sprint(gotC) != fmt.Sprint(gotI) || fmt.Sprint(gotI) != c.want {
			t.Errorf("%q: compiled=%v interp=%v, want %s on both — %s", c.src, gotC, gotI, c.want, c.why)
		}
	}
}

// TestParenReStepListElementRefusal pins the one shape in the rule table that
// the compiler REFUSES rather than answers, and why the refusal is not the
// lazy reading.
//
// `[((mk 1) 2)]` is `[3]` interpreted: the inner paren leaves two survivors,
// the park declines, and the rewind dispatches the carrier. It compiled to
// `[[fn (Integer) 2]]` until 2026-08-27 — NUR101's original symptom, silent,
// exit 0, `boru check` clean.
//
// It cannot be fixed by testing the lead's SHAPE, because `[(mk 1) 2]` reaches
// RecordMakeList with byte-identical `[carrier, 2]` elements and really is two
// elements on both lanes. Only the re-step record taken at the collapse
// separates them — which is why that row above still compiles and this one
// refuses.
//
// GRADUATION: Stage 3 records the apply as an element event and this becomes a
// parity row.
func TestParenReStepListElementRefusal(t *testing.T) {
	const src = `def mk fn [[a:Integer] [Function] [(fn [[b:Integer] [Integer] [a add b]])]] [((mk 1) 2)]`
	nur101Refusal(t, src, "[[3]]")

	// The twin that MUST keep compiling: no inner rewind, so two elements.
	const placed = `def mk fn [[a:Integer] [Function] [(fn [[b:Integer] [Integer] [a add b]])]] [(mk 1) 2]`
	gotC, compiled, errC := mustNew(t).RunCompiled(placed)
	gotI, errI := mustNew(t).RunInterp(placed)
	if !compiled || errC != nil || errI != nil {
		t.Fatalf("placed list twin: compiled=%v errC=%v errI=%v", compiled, errC, errI)
	}
	if fmt.Sprint(gotC) != fmt.Sprint(gotI) || fmt.Sprint(gotI) != "[[fn (Integer) 2]]" {
		t.Errorf("placed list twin: compiled=%v interp=%v, want [[fn (Integer) 2]] on both", gotC, gotI)
	}
}
