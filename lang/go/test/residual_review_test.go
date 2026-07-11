package test

import (
	"fmt"
	"strings"
	"testing"

	"github.com/aql-lang/aql/eng/go/parser"
	"github.com/aql-lang/aql/lang/go/native"
)

// End-to-end pins for the PR #260 review findings on the in-frame
// residual evaluation (the residual-timing unification): grouping must
// not change where a body's residual names resolve, a failing residual
// evaluation must tear the frame down before the error propagates,
// multiple residuals evaluate in stack order, and TCO elision must not
// reorder a pending residual around a tail callee.

// runResidualReviewSrc runs src on a fresh production registry and returns
// the rendered residual stack and the run error (unlike wantOne, error
// outcomes are the caller's to assert).
func runResidualReviewSrc(t *testing.T, src string) (string, error) {
	t.Helper()
	reg, err := native.DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	values, err := parser.Parse(src)
	if err != nil {
		t.Fatal(err)
	}
	res, rerr := native.NewTop(reg).Run(values)
	parts := make([]string, 0, len(res))
	for _, v := range res {
		parts = append(parts, v.String())
	}
	return strings.Join(parts, " "), rerr
}

func wantResidual(t *testing.T, src, want string) {
	t.Helper()
	got, err := runResidualReviewSrc(t, src)
	if err != nil {
		t.Fatalf("%q: %v", src, err)
	}
	if got != want {
		t.Errorf("%q: got %q, want %q", src, got, want)
	}
}

func wantResidualError(t *testing.T, src, wantSub string) {
	t.Helper()
	_, err := runResidualReviewSrc(t, src)
	if err == nil || !strings.Contains(fmt.Sprint(err), wantSub) {
		t.Errorf("%q: err = %v, want substring %q", src, err, wantSub)
	}
}

// TestResidualParenBodyBindsParam: a paren-wrapped computed body is ONE
// token but not one literal — it evaluates in-frame exactly like its
// unwrapped spelling, so the PARAM wins in both. The single bare
// literal keeps the consumer-scope transparency (the module binding) —
// the def-node-binding.tsv §3 contract the paren must NOT inherit.
func TestResidualParenBodyBindsParam(t *testing.T) {
	wantResidual(t,
		`def c1 1 def mk fn [[c1:Integer] [Map] [(def z 0 {a: c1})]] def r (mk 9) (r get "a")`,
		"9")
	wantResidual(t,
		`def c1 1 def mk fn [[c1:Integer] [Map] [def z 0 {a: c1}]] def r (mk 9) (r get "a")`,
		"9")
	// Transparency negative: the single-literal body still resolves in
	// the consumer's scope.
	wantResidual(t,
		`def c1 1 def mk fn [[c1:Integer] [Map] [{a: c1}]] def r (mk 9) (r get "a")`,
		"1")
}

// TestResidualErrorTearsDownFrame: when the in-frame residual
// evaluation raises, the frame's parked tail effects replay before the
// error propagates — a do-error trap upstream resumes WITHOUT the
// callee's body-locals, params, or args list. (Before the fix, z and s
// stayed bound after the trap.)
func TestResidualErrorTearsDownFrame(t *testing.T) {
	const def = `def pr fn [[s:String] [Any] [def z 0 {a:(raise "boom")}]] `
	wantResidualError(t, def+`do [pr "x"] error [drop] z`, "undefined word: z")
	wantResidualError(t, def+`do [pr "x"] error [drop] s`, "undefined word: s")
	// The residual's own error still propagates untrapped.
	wantResidualError(t, def+`pr "x"`, "boom")
}

// TestResidualOrderMatchesTopLevel: two computed containers left by a
// fn body evaluate BOTTOM-UP (source order), identical to the same
// residuals at top level — the end-of-run sweep's stack order. (Before
// the fix, the in-frame scan ran top-down and reversed their side
// effects.)
func TestResidualOrderMatchesTopLevel(t *testing.T) {
	const marks = `def n (flex {})
n set "c" 0 drop
def mark fn [[] [Integer] [def cur ((n get "c") add 1) n set "c" cur drop cur]]
`
	wantResidual(t, marks+`def f fn [[] [Any Any] [def z 0 {a:(mark)} {b:(mark)}]] f`,
		"{a:1} {b:2}")
	wantResidual(t, marks+`def z 0 {a:(mark)} {b:(mark)}`,
		"{a:1} {b:2}")
}

// TestTCODeclinesPendingResidualBelowTail: a pending residual container
// parked below a tail call reads state the callee writes — the eager
// teardown would evaluate it BEFORE the callee runs, so the gate
// declines and the frame nests: the residual sees the callee's write in
// both TCO modes (assertDual also pins the counters: detected, neither
// elided nor replaced).
func TestTCODeclinesPendingResidualBelowTail(t *testing.T) {
	const src = `def cell (flex {})
cell set "g" 0 drop
def g fn [[x:Integer] [Integer] [cell set "g" 1 drop x]]
def f fn [[x:Integer] [Any Any] [[(cell get "g")] g x]]
f 1`
	assertDual(t, "pending-residual-below-tail", src, "[1] 1", 1, 0, 0)
}
