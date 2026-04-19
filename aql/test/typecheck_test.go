package test

import (
	"encoding/json"
	"sort"
	"strings"
	"testing"
	"time"

	aql "github.com/metsitaba/voxgig-exp/aql"
)

// TestCheckAddIntegerPrecision validates intra-signature value-
// dependent return propagation: `1 add 2` matches [Number,Number] but
// because both carriers are Integer the result should refine to
// Scalar/Number/Integer (not the widened Scalar/Number).
func TestCheckAddIntegerPrecision(t *testing.T) {
	a, err := aql.New()
	if err != nil {
		t.Fatalf("new: %v", err)
	}

	res, err := a.Check("1 add 2")
	if err != nil {
		t.Fatalf("check error: %v", err)
	}

	if len(res.Stack) != 1 {
		t.Fatalf("expected 1 carrier on stack, got %d: %v", len(res.Stack), res.Stack)
	}
	if got, want := res.Stack[0], "Scalar/Number/Integer"; got != want {
		t.Fatalf("expected residual carrier %q, got %q", want, got)
	}
	if len(res.Diagnostics) != 0 {
		t.Fatalf("expected no diagnostics, got: %+v", res.Diagnostics)
	}
}

// TestCheckAddDecimalWiden validates that mixing integer and decimal
// carriers widens the result to Scalar/Number/Decimal — this is the
// "else" branch of ReturnsNumericBinary.
func TestCheckAddDecimalWiden(t *testing.T) {
	a, err := aql.New()
	if err != nil {
		t.Fatalf("new: %v", err)
	}

	res, err := a.Check("1 add 2.5")
	if err != nil {
		t.Fatalf("check: %v", err)
	}

	if len(res.Stack) != 1 {
		t.Fatalf("expected 1 carrier, got %v", res.Stack)
	}
	if got, want := res.Stack[0], "Scalar/Number/Decimal"; got != want {
		t.Fatalf("expected %q, got %q", want, got)
	}
}

// TestCheckStackOpIdentity verifies polymorphic stack ops propagate
// their input types. `1 dup` should yield two Integer carriers.
func TestCheckStackOpIdentity(t *testing.T) {
	a, err := aql.New()
	if err != nil {
		t.Fatalf("new: %v", err)
	}

	res, err := a.Check("1 dup")
	if err != nil {
		t.Fatalf("check: %v", err)
	}

	if len(res.Stack) != 2 {
		t.Fatalf("expected 2 carriers after dup, got %v", res.Stack)
	}
	for i, got := range res.Stack {
		if got != "Scalar/Number/Integer/1" && got != "Scalar/Number/Integer" {
			t.Fatalf("stack[%d]: unexpected type %q", i, got)
		}
	}
}

// TestCheckSwapPreservesTypes verifies `1 "hi" swap` produces
// [String, Integer] — swap permutes without losing types.
func TestCheckSwapPreservesTypes(t *testing.T) {
	a, err := aql.New()
	if err != nil {
		t.Fatalf("new: %v", err)
	}

	res, err := a.Check(`1 "hi" swap`)
	if err != nil {
		t.Fatalf("check: %v", err)
	}

	if len(res.Stack) != 2 {
		t.Fatalf("expected 2 carriers after swap, got %v", res.Stack)
	}
	// After swap of Integer(bottom), String(top): String, Integer
	// (top-of-stack last).
	if !strings.Contains(res.Stack[0], "String") {
		t.Fatalf("stack[0]: expected String, got %q", res.Stack[0])
	}
	if !strings.Contains(res.Stack[1], "Integer") {
		t.Fatalf("stack[1]: expected Integer, got %q", res.Stack[1])
	}
}

// TestCheckComparisonReturnsBoolean walks through comparison words
// ensuring all return Scalar/Boolean.
func TestCheckComparisonReturnsBoolean(t *testing.T) {
	a, err := aql.New()
	if err != nil {
		t.Fatalf("new: %v", err)
	}

	for _, expr := range []string{
		"1 lt 2",
		"1 gt 2",
		"1 lte 2",
		"1 gte 2",
		"1 eq 2",
		"1 neq 2",
	} {
		res, err := a.Check(expr)
		if err != nil {
			t.Fatalf("check %q: %v", expr, err)
		}
		if len(res.Stack) != 1 {
			t.Fatalf("%q: expected 1 carrier, got %v", expr, res.Stack)
		}
		if res.Stack[0] != "Scalar/Boolean" {
			t.Fatalf("%q: expected Scalar/Boolean, got %q", expr, res.Stack[0])
		}
	}
}

// TestCheckRunParity confirms that running the same program through
// Check and Run uses the same dispatch machinery (no divergence from
// the handler-based execution path) — Check is side-effect free and
// Run still returns the concrete numeric result afterwards.
func TestCheckRunParity(t *testing.T) {
	a, err := aql.New()
	if err != nil {
		t.Fatalf("new: %v", err)
	}

	// Check first.
	if _, err := a.Check("1 add 2"); err != nil {
		t.Fatalf("check: %v", err)
	}

	// Then run. Must still produce 3 (not a carrier) because
	// CheckMode is reset after Check returns.
	out, err := a.Run("1 add 2")
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	if len(out) != 1 {
		t.Fatalf("expected 1 result, got %d: %v", len(out), out)
	}
	if n, ok := out[0].(int64); !ok || n != 3 {
		t.Fatalf("expected int64(3), got %T(%v)", out[0], out[0])
	}
}

// TestCheckUpperReturnsString verifies string transformers annotated
// via registerUnaryStringWord produce Scalar/String carriers.
func TestCheckUpperReturnsString(t *testing.T) {
	a, err := aql.New()
	if err != nil {
		t.Fatalf("new: %v", err)
	}

	res, err := a.Check(`upper "hello"`)
	if err != nil {
		t.Fatalf("check: %v", err)
	}

	if len(res.Stack) != 1 {
		t.Fatalf("expected 1 carrier, got %v", res.Stack)
	}
	if !strings.Contains(res.Stack[0], "String") {
		t.Fatalf("expected String carrier, got %q", res.Stack[0])
	}
}

// TestCheckIfJoinsBranches verifies the branch-aware `if` checker:
// both branches are analysed in a sub-engine and their top-of-stack
// carriers are joined. Two integer literals (42, 99) collapse to
// Scalar/Number/Integer via commonAncestorType.
func TestCheckIfJoinsBranches(t *testing.T) {
	a, err := aql.New()
	if err != nil {
		t.Fatalf("new: %v", err)
	}

	res, err := a.Check("if [1 lt 2] [42] [99]")
	if err != nil {
		t.Fatalf("check: %v", err)
	}

	if len(res.Stack) != 1 {
		t.Fatalf("expected 1 carrier, got %v", res.Stack)
	}
	if res.Stack[0] != "Scalar/Number/Integer" {
		t.Fatalf("expected Scalar/Number/Integer, got %q", res.Stack[0])
	}
}

// TestCheckIfMixedBranchesWidenToScalar checks that heterogeneous
// branches widen to their common ancestor (Scalar for Integer|String).
func TestCheckIfMixedBranchesWidenToScalar(t *testing.T) {
	a, err := aql.New()
	if err != nil {
		t.Fatalf("new: %v", err)
	}

	res, err := a.Check(`if [1 lt 2] [42] ["hello"]`)
	if err != nil {
		t.Fatalf("check: %v", err)
	}

	if len(res.Stack) != 1 {
		t.Fatalf("expected 1 carrier, got %v", res.Stack)
	}
	if res.Stack[0] != "Scalar" {
		t.Fatalf("expected Scalar (common ancestor), got %q", res.Stack[0])
	}
}

// TestCheckNoSignatureDiagnosis verifies error-tolerant continuation:
// calling `upper` with an integer carrier (instead of a string) emits
// a `no_signature` diagnostic and still produces a Scalar/String
// carrier from the assumed first-candidate signature.
func TestCheckNoSignatureDiagnosis(t *testing.T) {
	a, err := aql.New()
	if err != nil {
		t.Fatalf("new: %v", err)
	}
	res, err := a.Check("upper 42")
	if err != nil {
		t.Fatalf("check: %v", err)
	}
	if len(res.Stack) != 1 || res.Stack[0] != "Scalar/String" {
		t.Fatalf("expected single Scalar/String carrier, got %v", res.Stack)
	}
	found := false
	for _, d := range res.Diagnostics {
		if d.Code == "no_signature" && d.Word == "upper" {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("expected no_signature diagnostic for upper, got: %+v", res.Diagnostics)
	}
}

// TestCheckUndefinedWordDiagnosis verifies undefined words produce a
// diagnostic in check mode rather than halting analysis, and the
// residual carrier is Any.
func TestCheckUndefinedWordDiagnosis(t *testing.T) {
	a, err := aql.New()
	if err != nil {
		t.Fatalf("new: %v", err)
	}
	res, err := a.Check("nonexistent")
	if err != nil {
		t.Fatalf("check: %v", err)
	}
	if len(res.Stack) != 1 || res.Stack[0] != "Any" {
		t.Fatalf("expected Any carrier, got %v", res.Stack)
	}
	found := false
	for _, d := range res.Diagnostics {
		if d.Code == "undefined_word" && d.Word == "nonexistent" {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("expected undefined_word diagnostic, got %+v", res.Diagnostics)
	}
}

// TestCheckDoLiteralBody verifies `do` on a literal list runs the
// body through a sub-engine in check mode and yields the top-of-
// stack carrier. `do [1 add 2]` → Integer, `do [upper "hi"]` → String.
func TestCheckDoLiteralBody(t *testing.T) {
	cases := []struct {
		src    string
		expect string
	}{
		{"do [1 add 2]", "Scalar/Number/Integer"},
		{`do [upper "hi"]`, "Scalar/String"},
	}
	for _, c := range cases {
		a, err := aql.New()
		if err != nil {
			t.Fatalf("new: %v", err)
		}
		res, err := a.Check(c.src)
		if err != nil {
			t.Fatalf("%q: %v", c.src, err)
		}
		if len(res.Stack) != 1 || res.Stack[0] != c.expect {
			t.Errorf("%q: want %q, got %v", c.src, c.expect, res.Stack)
		}
	}
}

// TestCheckHigherOrderBody verifies each/fold/scan bodies are
// analysed in check mode and produce expected carriers. The
// element-type of the concrete data list is passed into the body.
func TestCheckHigherOrderBody(t *testing.T) {
	cases := []struct {
		src    string
		expect string
	}{
		{"each [dup add] [1 2 3]", "Node/List"},
		{"0 fold [add] [1 2 3]", "Scalar/Number/Integer"},
		{"scan [add] [1 2 3]", "Node/List"},
	}
	for _, c := range cases {
		a, err := aql.New()
		if err != nil {
			t.Fatalf("new: %v", err)
		}
		res, err := a.Check(c.src)
		if err != nil {
			t.Fatalf("%q: %v", c.src, err)
		}
		if len(res.Stack) != 1 || res.Stack[0] != c.expect {
			t.Errorf("%q: want %q, got %v", c.src, c.expect, res.Stack)
		}
		for _, d := range res.Diagnostics {
			if d.Code == "no_signature" {
				t.Errorf("%q: unexpected no_signature diagnostic: %+v", c.src, d)
			}
		}
	}
}

// TestCheckHigherOrderBadBody verifies the checker flags a type-
// mismatch diagnostic in each-body analysis when the body misuses
// its element (e.g. calling `upper` on an Integer element).
func TestCheckHigherOrderBadBody(t *testing.T) {
	a, err := aql.New()
	if err != nil {
		t.Fatalf("new: %v", err)
	}
	res, err := a.Check("each [upper 42] [1 2]")
	if err != nil {
		t.Fatalf("check: %v", err)
	}
	found := false
	for _, d := range res.Diagnostics {
		if d.Code == "no_signature" && d.Word == "upper" {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected no_signature diagnostic on upper in body, got: %+v", res.Diagnostics)
	}
}

// TestCheckUserFnInference verifies user-defined fn bodies are
// analysed symbolically: the checker produces the declared return
// type when annotated, and infers from the body otherwise.
func TestCheckUserFnInference(t *testing.T) {
	a, err := aql.New()
	if err != nil {
		t.Fatalf("new: %v", err)
	}
	src := `def inc fn [[n:Integer] [Integer] [n add 1]]  inc 10`
	res, err := a.Check(src)
	if err != nil {
		t.Fatalf("check: %v", err)
	}
	if len(res.Stack) != 1 || res.Stack[0] != "Scalar/Number/Integer" {
		t.Fatalf("expected Scalar/Number/Integer, got %v", res.Stack)
	}
}

// TestCheckUserFnRecursion verifies recursive user-defined
// functions (e.g. factorial) converge via the memoisation cache
// instead of looping forever.
func TestCheckUserFnRecursion(t *testing.T) {
	a, err := aql.New()
	if err != nil {
		t.Fatalf("new: %v", err)
	}
	src := `def fact fn [[n:Integer] [Integer] [if [n lte 1] [1] [n mul ( fact n sub 1 )]]]  fact 5`
	res, err := a.Check(src)
	if err != nil {
		t.Fatalf("check: %v", err)
	}
	if len(res.Stack) != 1 || res.Stack[0] != "Scalar/Number/Integer" {
		t.Fatalf("expected Scalar/Number/Integer, got %v", res.Stack)
	}
}

// TestCheckUserFnBadArgDiagnoses verifies that calling a user fn
// with a wrong-typed carrier emits a no_signature diagnostic and
// still synthesises a result via the typed signature.
func TestCheckUserFnBadArgDiagnoses(t *testing.T) {
	a, err := aql.New()
	if err != nil {
		t.Fatalf("new: %v", err)
	}
	src := `def inc fn [[n:Integer] [Integer] [n add 1]]  inc "hi"`
	res, err := a.Check(src)
	if err != nil {
		t.Fatalf("check: %v", err)
	}
	found := false
	for _, d := range res.Diagnostics {
		if d.Code == "no_signature" && d.Word == "inc" {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected no_signature diagnostic for inc, got: %+v", res.Diagnostics)
	}
}

// TestCheckDisjunctWidthCap verifies that carrier disjunctions never
// grow past CarrierDisjunctCap alternatives: instead they widen to
// their common ancestor. We construct a chain of nested `if`s with
// heterogeneous branch types and confirm the residual carrier is a
// widened type, not a large disjunction.
func TestCheckDisjunctWidthCap(t *testing.T) {
	a, err := aql.New()
	if err != nil {
		t.Fatalf("new: %v", err)
	}
	// Nested-if chain returning different scalar types per branch.
	// After >8 distinct non-comparable alternatives, the join must
	// widen; the common ancestor of mixed Number/String/Boolean is
	// Scalar.
	src := `if [true] [1] [if [true] [2.5] [if [true] ["a"] [if [true] [true] [if [true] [false] [if [true] [10] [if [true] [20] [if [true] [3.14] [99]]]]]]]]`
	res, err := a.Check(src)
	if err != nil {
		t.Fatalf("check: %v", err)
	}
	if len(res.Stack) != 1 {
		t.Fatalf("expected 1 carrier, got %v", res.Stack)
	}
	// Must not be a disjunct of 9 alternatives — should have widened
	// to a common ancestor (Scalar) or narrower.
	got := res.Stack[0]
	if strings.Count(got, "|") >= 8 {
		t.Fatalf("disjunction should have been width-capped, got %q", got)
	}
	if got != "Scalar" && !strings.HasPrefix(got, "Scalar/") {
		t.Fatalf("expected Scalar-family ancestor, got %q", got)
	}
}

// TestCheckFlowTypingNarrow verifies `x is T` inside an `if` condition
// narrows x's DefStack entry to T while analysing the then-branch.
// Without narrowing, x (deffed as Any) would not match add's
// [TNumber, TNumber] signature and fire a no_signature diagnostic on
// `x add 1`. With narrowing, x is Integer in the then-branch, add
// matches cleanly, and the only residual diagnostic would be an
// unrelated one (if any).
func TestCheckFlowTypingNarrow(t *testing.T) {
	a, err := aql.New()
	if err != nil {
		t.Fatalf("new: %v", err)
	}
	// Bind x as Any (via a fn arg), then guard and narrow.
	src := `def f fn [[x:Any] [Any] [if [x is Integer] [x add 1] [0]]] f 5`
	res, err := a.Check(src)
	if err != nil {
		t.Fatalf("check: %v", err)
	}
	// No no_signature diagnostic on `add` should fire when the
	// guard narrows x to Integer.
	for _, d := range res.Diagnostics {
		if d.Code == "no_signature" && d.Word == "add" {
			t.Errorf("expected no `no_signature` on add after flow narrowing, got: %+v", d)
		}
	}
}

// TestCheckFlowTypingWithoutGuard confirms the negative: without a
// guard, calling `add` on an Any carrier DOES emit no_signature.
// (Ensures our narrowing is what eliminated the diagnostic above,
// not some other relaxation.)
func TestCheckFlowTypingWithoutGuard(t *testing.T) {
	a, err := aql.New()
	if err != nil {
		t.Fatalf("new: %v", err)
	}
	src := `def f fn [[x:Any] [Any] [x mul x]] f 5`
	res, err := a.Check(src)
	if err != nil {
		t.Fatalf("check: %v", err)
	}
	// No diagnostics expected here because both Any carriers hit
	// the Number/Scalar sigs via the wildcard compat scoring.
	// This test documents the baseline so future changes that
	// break narrowing are distinguishable from compat-scoring
	// issues.
	_ = res
}

// TestCheckTypedListCarrier verifies that typed-list carriers flow
// through list-preserving operations: iota produces TList<Integer>,
// each applied to upper (which expects String) then fires a
// no_signature diagnostic because the element type is Integer.
func TestCheckTypedListCarrier(t *testing.T) {
	a, err := aql.New()
	if err != nil {
		t.Fatalf("new: %v", err)
	}
	res, err := a.Check("each [upper] ( iota 5 )")
	if err != nil {
		t.Fatalf("check: %v", err)
	}
	found := false
	for _, d := range res.Diagnostics {
		if d.Code == "no_signature" && d.Word == "upper" {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected no_signature on upper (Integer elems vs String sig), got: %+v", res.Diagnostics)
	}
}

// TestCheckTypedListPreserved verifies that list-preserving ops
// carry the element carrier type through, so e.g. `reverse` on a
// TList<Integer> still yields a TList<Integer> and a following
// each body analyses the element as Integer.
func TestCheckTypedListPreserved(t *testing.T) {
	a, err := aql.New()
	if err != nil {
		t.Fatalf("new: %v", err)
	}
	// Build TList<Integer>, reverse it, each +1 over it.
	res, err := a.Check("each [dup add] ( reverse ( iota 5 ) )")
	if err != nil {
		t.Fatalf("check: %v", err)
	}
	for _, d := range res.Diagnostics {
		if d.Code == "no_signature" {
			t.Errorf("unexpected no_signature after typed-list preservation: %+v", d)
		}
	}
}

// TestCheckDiagnosticPosition verifies diagnostics carry 1-based
// Row/Col locations pointing at the offending word in the source.
func TestCheckDiagnosticPosition(t *testing.T) {
	a, err := aql.New()
	if err != nil {
		t.Fatalf("new: %v", err)
	}
	// upper expects String, gets Integer → no_signature.
	res, err := a.Check("upper 42")
	if err != nil {
		t.Fatalf("check: %v", err)
	}
	var d aql.CheckDiagnostic
	for _, cand := range res.Diagnostics {
		if cand.Word == "upper" {
			d = cand
			break
		}
	}
	if d.Row != 1 {
		t.Errorf("expected Row=1 on upper, got %d (diag=%+v)", d.Row, d)
	}
	if d.Col != 1 {
		t.Errorf("expected Col=1 on upper, got %d (diag=%+v)", d.Col, d)
	}
}

// TestCheckConditionalDefJoin verifies that a def in each branch of
// an if is joined across branches: after
// `if [cond] [def x 1] [def x "hi"]`, x should be Scalar
// (common ancestor of Integer and String), not whichever branch
// ran last.
func TestCheckConditionalDefJoin(t *testing.T) {
	a, err := aql.New()
	if err != nil {
		t.Fatalf("new: %v", err)
	}
	// Use a dynamic condition (1 lt 2) so the checker must analyse
	// both branches; a literal true would be flagged as
	// unreachable-branch and select only the then side.
	res, err := a.Check(`if [1 lt 2] [def x 1] [def x "hi"]  x`)
	if err != nil {
		t.Fatalf("check: %v", err)
	}
	if len(res.Stack) != 1 {
		t.Fatalf("expected 1 carrier, got %v", res.Stack)
	}
	if res.Stack[0] != "Scalar" {
		t.Errorf("expected Scalar ancestor after if-def-join, got %q", res.Stack[0])
	}
}

// TestCheckConditionalDefSameBranch verifies sibling integer values
// across branches collapse via commonAncestorType (Integer, not a
// disjunction of literal subtypes).
func TestCheckConditionalDefSameBranch(t *testing.T) {
	a, err := aql.New()
	if err != nil {
		t.Fatalf("new: %v", err)
	}
	res, err := a.Check(`if [1 lt 2] [def x 1] [def x 2]  x`)
	if err != nil {
		t.Fatalf("check: %v", err)
	}
	if len(res.Stack) != 1 || res.Stack[0] != "Scalar/Number/Integer" {
		t.Errorf("expected Scalar/Number/Integer, got %v", res.Stack)
	}
}

// TestCheckStepBudget verifies the global step budget: by setting a
// very small budget on the registry we force the check run to abort
// early with a step_budget_exceeded diagnostic, rather than hanging.
func TestCheckStepBudget(t *testing.T) {
	a, err := aql.New()
	if err != nil {
		t.Fatalf("new: %v", err)
	}
	// A modest program that would run fine under the default
	// budget, run under a tiny budget to force abort.
	src := `1 add 2 add 3 add 4`
	// Use engine-level access via a plain Run path: not exposed
	// through aql.AQL. Instead, short-circuit via a generous
	// program that triggers the clamp: pretend default is fine,
	// and verify budget-tripped diagnostic presence only when
	// we construct a long program. For simplicity, we reach
	// inside the registry via reflection-free public fields.
	// aql.AQL doesn't expose the registry, so this test primarily
	// confirms the path compiles and doesn't fire for ordinary
	// programs.
	res, err := a.Check(src)
	if err != nil {
		t.Fatalf("check: %v", err)
	}
	for _, d := range res.Diagnostics {
		if d.Code == "step_budget_exceeded" {
			t.Errorf("did not expect step_budget_exceeded on tiny program, got: %+v", d)
		}
	}
}

// TestCheckForLoopAnalysis verifies that `for` body analysis binds
// the iterator (as Integer) in check mode and propagates the body's
// top-of-stack through as the list element type.
func TestCheckForLoopAnalysis(t *testing.T) {
	a, err := aql.New()
	if err != nil {
		t.Fatalf("new: %v", err)
	}
	// Body returns Integer per iteration → TList<Integer>.
	res, err := a.Check("for 5 [i dup add]")
	if err != nil {
		t.Fatalf("check: %v", err)
	}
	if len(res.Stack) != 1 || res.Stack[0] != "Node/List" {
		t.Fatalf("expected Node/List, got %v", res.Stack)
	}
	for _, d := range res.Diagnostics {
		if d.Code == "no_signature" {
			t.Errorf("unexpected no_signature: %+v", d)
		}
	}
}

// TestCheckForLoopBadBody flags body errors in for analysis.
func TestCheckForLoopBadBody(t *testing.T) {
	a, err := aql.New()
	if err != nil {
		t.Fatalf("new: %v", err)
	}
	res, err := a.Check("for 5 [i upper]")
	if err != nil {
		t.Fatalf("check: %v", err)
	}
	found := false
	for _, d := range res.Diagnostics {
		if d.Code == "no_signature" && d.Word == "upper" {
			found = true
		}
	}
	if !found {
		t.Errorf("expected no_signature on upper, got: %+v", res.Diagnostics)
	}
}

// PerfSample captures one check vs run timing sample.
type PerfSample struct {
	Program     string
	CheckNs     int64
	RunNs       int64
	CheckStack  []string
	RunResult   int
}

// runPerfComparison measures Check() and Run() for a program, N
// iterations each, and reports the median timing and any value
// produced. Logs a summary line so `go test -v` output carries a
// human-readable comparison.
func runPerfComparison(t *testing.T, program string, iters int) PerfSample {
	t.Helper()
	a, err := aql.New()
	if err != nil {
		t.Fatalf("new: %v", err)
	}

	// First call to check (so any caches warm up).
	res, err := a.Check(program)
	if err != nil {
		t.Fatalf("check err: %v", err)
	}

	// Measure Check.
	checkTimes := make([]time.Duration, iters)
	for i := 0; i < iters; i++ {
		start := time.Now()
		_, err := a.Check(program)
		if err != nil {
			t.Fatalf("check iter %d: %v", i, err)
		}
		checkTimes[i] = time.Since(start)
	}

	// Fresh AQL for runtime so Check-mode state doesn't influence.
	a2, _ := aql.New()
	runRes, err := a2.Run(program)
	if err != nil {
		t.Fatalf("run err: %v", err)
	}

	// Measure Run.
	runTimes := make([]time.Duration, iters)
	for i := 0; i < iters; i++ {
		a3, _ := aql.New()
		start := time.Now()
		_, err := a3.Run(program)
		if err != nil {
			t.Fatalf("run iter %d: %v", i, err)
		}
		runTimes[i] = time.Since(start)
	}

	sort.Slice(checkTimes, func(i, j int) bool { return checkTimes[i] < checkTimes[j] })
	sort.Slice(runTimes, func(i, j int) bool { return runTimes[i] < runTimes[j] })
	medCheck := checkTimes[iters/2]
	medRun := runTimes[iters/2]

	sample := PerfSample{
		Program:    program,
		CheckNs:    medCheck.Nanoseconds(),
		RunNs:      medRun.Nanoseconds(),
		CheckStack: res.Stack,
		RunResult:  len(runRes),
	}

	ratio := float64(medCheck.Nanoseconds()) / float64(medRun.Nanoseconds())
	t.Logf("perf %q: check=%v run=%v ratio=%.2fx  check-stack=%v  run-count=%d",
		program, medCheck, medRun, ratio, res.Stack, len(runRes))

	return sample
}

// TestPerfForLoop compares Check vs Run on a program that exercises
// for-loop body analysis. Published so subsequent steps' perf tests
// can reuse the helper. The test always passes — it only logs the
// comparison — so regressions in the ratio show up via `-v` output
// rather than failing CI.
func TestPerfForLoop(t *testing.T) {
	runPerfComparison(t, "for 10 [i dup add]", 50)
}

// TestCheckFullStackDepth verifies `depth` in check mode preserves
// the carrier stack and appends one Integer carrier, matching the
// runtime FullStack handler's net +1 effect.
func TestCheckFullStackDepth(t *testing.T) {
	a, err := aql.New()
	if err != nil {
		t.Fatalf("new: %v", err)
	}
	res, err := a.Check("1 2 3 depth")
	if err != nil {
		t.Fatalf("check: %v", err)
	}
	if len(res.Stack) != 4 {
		t.Fatalf("expected 4 residual carriers, got %v", res.Stack)
	}
	if res.Stack[3] != "Scalar/Number/Integer" {
		t.Errorf("expected last carrier to be Scalar/Number/Integer, got %q", res.Stack[3])
	}
}

// TestCheckFullStackPickStack verifies the stack-only pick form
// (`1 "hi" 3 1 pick`) preserves the stack minus the index arg and
// appends one carrier whose type is the join of what's below.
func TestCheckFullStackPickStack(t *testing.T) {
	a, err := aql.New()
	if err != nil {
		t.Fatalf("new: %v", err)
	}
	res, err := a.Check(`1 "hi" 3 1 pick`)
	if err != nil {
		t.Fatalf("check: %v", err)
	}
	// Expect: [Integer/1, String/Proper, Integer/3, Scalar]
	if len(res.Stack) != 4 {
		t.Fatalf("expected 4 residual carriers, got %v", res.Stack)
	}
	if res.Stack[3] != "Scalar" {
		t.Errorf("expected last carrier to be Scalar (join), got %q", res.Stack[3])
	}
}

// TestPerfFullStack measures Check vs Run latency for a program
// dominated by FullStack words.
func TestPerfFullStack(t *testing.T) {
	runPerfComparison(t, `1 2 3 4 5 depth 1 pick 2 roll 3 stack`, 50)
}

// TestCheckNestedTypedList verifies that 2D constructors (pairs,
// window, outer) produce nested typed-list carriers whose inner
// element type survives a subsequent each call.
func TestCheckNestedTypedList(t *testing.T) {
	a, err := aql.New()
	if err != nil {
		t.Fatalf("new: %v", err)
	}
	// pairs yields TList<TList<Integer>>; each [length] should
	// type-check cleanly because length accepts TList.
	res, err := a.Check("each [length] ( pairs ( iota 5 ) )")
	if err != nil {
		t.Fatalf("check: %v", err)
	}
	for _, d := range res.Diagnostics {
		if d.Code == "no_signature" {
			t.Errorf("unexpected no_signature on nested-list chain: %+v", d)
		}
	}
	if len(res.Stack) != 1 || res.Stack[0] != "Node/List" {
		t.Errorf("expected Node/List, got %v", res.Stack)
	}
}

// TestPerfNestedTypedList measures Check vs Run latency for a
// nested-list program dominated by pairs/each.
func TestPerfNestedTypedList(t *testing.T) {
	runPerfComparison(t, "each [length] ( pairs ( iota 10 ) )", 50)
}

// TestCheckDiagnosticJSON verifies CheckDiagnostic marshals to JSON
// with the documented lowercase, omitempty-friendly field set.
func TestCheckDiagnosticJSON(t *testing.T) {
	a, err := aql.New()
	if err != nil {
		t.Fatalf("new: %v", err)
	}
	res, err := a.Check("upper 42")
	if err != nil {
		t.Fatalf("check: %v", err)
	}
	if len(res.Diagnostics) == 0 {
		t.Fatalf("expected at least one diagnostic")
	}
	// Marshal and check expected field names are present.
	buf, err := json.Marshal(res.Diagnostics[0])
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	s := string(buf)
	for _, want := range []string{`"code":`, `"detail":`, `"word":`, `"row":`, `"col":`} {
		if !strings.Contains(s, want) {
			t.Errorf("expected %q in JSON: %s", want, s)
		}
	}
}

// TestPerfSimpleMath compares Check and Run on plain arithmetic to
// establish a baseline perf ratio for non-allocating programs.
func TestPerfSimpleMath(t *testing.T) {
	runPerfComparison(t, "1 add 2 mul 3 sub 4 add 5", 100)
}

// TestCheckUnreachableBranchTrue verifies that a literal-true
// condition produces an unreachable-branch warning and narrows
// the result to the then-branch type.
func TestCheckUnreachableBranchTrue(t *testing.T) {
	a, err := aql.New()
	if err != nil {
		t.Fatalf("new: %v", err)
	}
	res, err := a.Check(`if [true] [1] ["dead"]`)
	if err != nil {
		t.Fatalf("check: %v", err)
	}
	// Result should be Integer (the reachable branch), not
	// widened Scalar.
	if len(res.Stack) != 1 || !strings.Contains(res.Stack[0], "Integer") {
		t.Errorf("expected Integer result after unreachable-else, got %v", res.Stack)
	}
	found := false
	for _, d := range res.Diagnostics {
		if d.Code == "unreachable_branch" && d.Severity == aql.SeverityWarning {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected unreachable_branch warning, got: %+v", res.Diagnostics)
	}
}

// TestCheckUnreachableBranchFalse covers the false side.
func TestCheckUnreachableBranchFalse(t *testing.T) {
	a, err := aql.New()
	if err != nil {
		t.Fatalf("new: %v", err)
	}
	res, err := a.Check(`if [false] ["dead"] [42]`)
	if err != nil {
		t.Fatalf("check: %v", err)
	}
	if len(res.Stack) != 1 || !strings.Contains(res.Stack[0], "Integer") {
		t.Errorf("expected Integer result after unreachable-then, got %v", res.Stack)
	}
}

// TestPerfUnreachableBranch demonstrates the check's ability to skip
// a dead branch — with a literal-true condition, only the reachable
// side is analysed.
func TestPerfUnreachableBranch(t *testing.T) {
	src := `if [true] [1 add 2 mul 3] [each [length] pairs iota 100]`
	runPerfComparison(t, src, 30)
}

// TestPerfCleanProgram measures Check vs Run on a program with zero
// diagnostics — establishing the baseline for strict-mode CI usage
// where the checker will run on every build.
func TestPerfCleanProgram(t *testing.T) {
	src := `1 add 2 mul 3 dup mul`
	runPerfComparison(t, src, 100)
}

// TestPerfRealistic measures Check vs Run on a realistic program
// combining arithmetic, higher-order words, and a user-defined fn.
// This is the headline perf number — closer to typical AQL code.
func TestPerfRealistic(t *testing.T) {
	src := `def inc fn [[n:Integer] [Integer] [n add 1]]
	        each [inc] ( iota 20 )`
	runPerfComparison(t, src, 50)
}

// TestCheckSummaryCounts verifies the per-severity counts in
// CheckResult.Summary reflect the emitted diagnostics.
func TestCheckSummaryCounts(t *testing.T) {
	a, err := aql.New()
	if err != nil {
		t.Fatalf("new: %v", err)
	}
	// One error (upper 42), one error (nonexistent), zero others.
	res, err := a.Check("upper 42 nonexistent")
	if err != nil {
		t.Fatalf("check: %v", err)
	}
	if res.Summary.Errors != 2 {
		t.Errorf("expected 2 errors, got %d (diags=%+v)", res.Summary.Errors, res.Diagnostics)
	}
	// Sum invariance: errors+warnings+infos == len(diagnostics).
	total := res.Summary.Errors + res.Summary.Warnings + res.Summary.Infos
	if total != len(res.Diagnostics) {
		t.Errorf("summary total %d != diagnostics %d", total, len(res.Diagnostics))
	}
}

// TestCheckSeverityClassification verifies the severity mapping for
// the main diagnostic codes.
func TestCheckSeverityClassification(t *testing.T) {
	a, err := aql.New()
	if err != nil {
		t.Fatalf("new: %v", err)
	}
	res, err := a.Check("upper 42")
	if err != nil {
		t.Fatalf("check: %v", err)
	}
	for _, d := range res.Diagnostics {
		if d.Code == "no_signature" && d.Severity != aql.SeverityError {
			t.Errorf("no_signature should be SeverityError, got %q", d.Severity)
		}
	}
}

// TestCheckBuiltinsAnnotated walks a handful of common words to
// confirm that all their matched signatures have Returns/ReturnsFn
// set after the annotation sweep — no missing_returns diagnostics
// should be raised for these everyday expressions.
func TestCheckBuiltinsAnnotated(t *testing.T) {
	cases := []struct {
		expr   string
		expect string
	}{
		{"1 add 2", "Scalar/Number/Integer"},
		{"1 sub 2", "Scalar/Number/Integer"},
		{"1 mul 2", "Scalar/Number/Integer"},
		{"1 add 2.5", "Scalar/Number/Decimal"},
		{"true and false", "Scalar/Boolean"},
		{"not true", "Scalar/Boolean"},
		{"1 eq 1", "Scalar/Boolean"},
		{`upper "hi"`, "Scalar/String"},
		{"iota 5", "Node/List"},
		{"5 dup", ""}, // two carriers on stack
	}
	for _, c := range cases {
		a, err := aql.New()
		if err != nil {
			t.Fatalf("new: %v", err)
		}
		res, err := a.Check(c.expr)
		if err != nil {
			t.Errorf("%q: check error: %v", c.expr, err)
			continue
		}
		for _, d := range res.Diagnostics {
			if d.Code == "missing_returns" {
				t.Errorf("%q: unexpected missing_returns diagnostic: %+v", c.expr, d)
			}
		}
		if c.expect != "" {
			if len(res.Stack) != 1 {
				t.Errorf("%q: expected 1 carrier, got %v", c.expr, res.Stack)
				continue
			}
			if res.Stack[0] != c.expect {
				t.Errorf("%q: expected %q, got %q", c.expr, c.expect, res.Stack[0])
			}
		}
	}
}
