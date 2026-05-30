package test

import (
	"strings"
	"testing"
)

// --- inspect for fn-shape and DepScalar types ---
//
// `inspect Mapper` (Mapper = `def Mapper fnsig [[Integer] [Integer]]`)
// previously returned an empty `signatures` slot — buildTypeInspection
// had no case for the FnUndef shape. Same gap for DepScalar
// constraints: users had no introspection of bounds.
//
// These tests pin the new structural inspections so future
// buildTypeInspection changes don't regress.

// inspect on a fn-shape type renders one entry per spec, with
// params/returns lists of the type paths.
func TestInspect_FnUndefSingleSig(t *testing.T) {
	got := runOne(t, `def Mapper fnsig [[Integer] [Integer]]
inspect Mapper`)
	if len(got) != 1 {
		t.Fatalf("got %d results, want 1", len(got))
	}
	s := got[0].(string)
	for _, want := range []string{
		`name:'Mapper'`,
		`kind:function_shape`,
		`signatures:`,
		`params:['Integer']`,
		`returns:['Integer']`,
	} {
		if !strings.Contains(s, want) {
			t.Errorf("inspect output missing %q\nfull: %s", want, s)
		}
	}
}

// inspect on a DepScalar (single bound) shows the leaf and the
// active side (lo for gt/gte, hi for lt/lte) plus the kind/value.
func TestInspect_DepScalarSingleBound(t *testing.T) {
	got := runOne(t, `def G10 (Integer gt 10)
inspect G10`)
	if len(got) != 1 {
		t.Fatalf("got %d results, want 1", len(got))
	}
	s := got[0].(string)
	for _, want := range []string{
		`name:'G10'`,
		`kind:dependent_scalar`,
		`leaf:'Integer'`,
		`lo:{kind:'gt' value:10}`,
	} {
		if !strings.Contains(s, want) {
			t.Errorf("inspect output missing %q\nfull: %s", want, s)
		}
	}
	// No `hi` key when only the lower bound is set.
	if strings.Contains(s, "hi:") {
		t.Errorf("inspect output unexpectedly includes hi: %s", s)
	}
}

// `between` produces a closed interval — inspect should show both
// lo and hi with their respective kinds.
func TestInspect_DepScalarInterval(t *testing.T) {
	got := runOne(t, `def Range (Integer between 5 20)
inspect Range`)
	if len(got) != 1 {
		t.Fatalf("got %d results, want 1", len(got))
	}
	s := got[0].(string)
	for _, want := range []string{
		`leaf:'Integer'`,
		`lo:{kind:'gte' value:5}`,
		`hi:{kind:'lte' value:20}`,
	} {
		if !strings.Contains(s, want) {
			t.Errorf("inspect output missing %q\nfull: %s", want, s)
		}
	}
}
