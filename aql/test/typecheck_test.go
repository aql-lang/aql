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
