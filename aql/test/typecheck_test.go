package test

import (
	"strings"
	"testing"

	aql "github.com/metsitaba/voxgig-exp/aql"
)

// TestCheckAddReturnsNumber validates the minimal carrier-based static
// type-checker. `1 add 2` should produce a single carrier typed
// Scalar/Number (the declared Returns on the add [Number,Number]
// signature) with zero diagnostics.
func TestCheckAddReturnsNumber(t *testing.T) {
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
	if got, want := res.Stack[0], "Scalar/Number"; got != want {
		t.Fatalf("expected residual carrier %q, got %q", want, got)
	}
	if len(res.Diagnostics) != 0 {
		t.Fatalf("expected no diagnostics, got: %+v", res.Diagnostics)
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

// TestCheckUnannotatedWordDiagnoses ensures un-annotated words produce
// a "missing_returns" diagnostic and fall back to an Any carrier so
// checking can continue. `upper` is a string word that has no Returns
// annotation yet.
func TestCheckUnannotatedWordDiagnoses(t *testing.T) {
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
	if got := res.Stack[0]; got != "Any" {
		t.Fatalf("expected Any fallback, got %q", got)
	}

	found := false
	for _, d := range res.Diagnostics {
		if d.Code == "missing_returns" && strings.Contains(d.Detail, "upper") {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("expected missing_returns diagnostic for 'upper', got: %+v", res.Diagnostics)
	}
}
