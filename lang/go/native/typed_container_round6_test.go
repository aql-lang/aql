package native

import (
	"strings"
	"testing"

	"github.com/boru-lang/boru/eng/go"
)

// Codex round-6 fixes (design/TYPED-CONTAINER-TAG-RETENTION.0.md):
//
//	P1 a typed-param retag must PRESERVE flex reference identity — retagging a
//	   flex arg via Unify rebuilt a detached copy, so a body write mutated the
//	   copy and left the caller's flex untouched. Fixed by retagging the header
//	   in place (RetagTypedContainerValue), shared by interpreter + compiled.
//	P2 flexReturns / nodeReturns retain the element tag so the check-mode write
//	   mirror (d2CheckWrite) fires on a typed flex/node result.

// #1 — a body write into a typed FLEX param mutates the CALLER (reference
// semantics), and the compiled path agrees with the interpreter.
func TestFlexParamIdentityPreserved(t *testing.T) {
	src := `def p fn [[xs:[:Integer]] [FlexList] [(xs set 0 99)]] def u (flex [1 2 3]) (p u) u`
	out, err := tcRun(t, src)
	if err != nil {
		t.Fatalf("flex param write: %v", err)
	}
	// The last value is the caller's `u` — it must reflect the in-body mutation.
	u := out[len(out)-1]
	lst, lerr := eng.AsList(u)
	if lerr != nil {
		t.Fatalf("caller value is not a list: %v", lerr)
	}
	if got, _ := eng.AsInteger(lst.Get(0)); got != 99 {
		t.Errorf("flex identity lost: caller's u[0] = %v, want 99 (body write should mutate the shared flex)", got)
	}
	// The tag still enforces a bad write inside the body.
	if _, e := tcRun(t, `def p fn [[xs:[:Integer]] [FlexList] [(xs set 0 "bad")]] def u (flex [1 2 3]) (p u)`); e == nil ||
		!strings.Contains(e.Error(), "does not conform to element type Integer") {
		t.Errorf("typed flex param write should still enforce, got %v", e)
	}
}

// #1 (map) — a typed FLEX MAP param preserves identity too.
func TestFlexMapParamIdentityPreserved(t *testing.T) {
	out, err := tcRun(t, `def p fn [[m:{:Integer}] [FlexMap] [(m set b/q 9)]] def u (flex {a:1}) (p u) u`)
	if err != nil {
		t.Fatalf("flex map param write: %v", err)
	}
	m, merr := eng.AsMap(out[len(out)-1])
	if merr != nil || m == nil {
		t.Fatalf("caller value is not a map: %v", merr)
	}
	if _, ok := m.Get("b"); !ok {
		t.Errorf("flex map identity lost: caller's u has no key 'b' (body write should mutate the shared flex)")
	}
}

// #2 — flexReturns / nodeReturns retain the element tag, so the check-mode write
// mirror fires on a typed flex/node result.
func TestFlexNodeResidualRetainsTag(t *testing.T) {
	// append into a typed flex result is flagged in check.
	r := tcCheck(t, `def xs:[:Integer] [1 2 3] append "bad" (flex xs)`)
	if !tcHasTypeError(r, "does not conform to element type Integer") {
		t.Errorf("append of a bad element into (flex [:Integer]) should be flagged: %+v", r.Check.Diagnostics)
	}
	// A conforming append stays silent.
	r2 := tcCheck(t, `def xs:[:Integer] [1 2 3] append 9 (flex xs)`)
	if tcHasTypeError(r2, "does not conform") {
		t.Errorf("conforming append into a flex should be silent: %+v", r2.Check.Diagnostics)
	}
	// The flexReturns / nodeReturns residuals carry the elem tag directly.
	xs, _ := tcValue(t, `def xs:[:Integer] [1 2 3] xs`)
	fres := flexReturns([]Value{xs}, nil)
	if e, ok := fres[0].ElemConstraint(); !ok || !e.Equal(TInteger) {
		t.Errorf("flexReturns residual lost the [:Integer] tag")
	}
	nres := nodeReturns([]Value{xs}, nil)
	if e, ok := nres[0].ElemConstraint(); !ok || !e.Equal(TInteger) {
		t.Errorf("nodeReturns residual lost the [:Integer] tag")
	}
	// The negative: an untyped source yields an untagged residual.
	u, _ := tcValue(t, `def u [1 2 3] u`)
	if _, ok := flexReturns([]Value{u}, nil)[0].ElemConstraint(); ok {
		t.Errorf("flexReturns of an untyped list should carry no tag")
	}
}
