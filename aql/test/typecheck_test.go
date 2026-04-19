package test

import (
	"strings"
	"testing"

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
