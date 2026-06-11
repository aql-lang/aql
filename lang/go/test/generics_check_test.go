package test

import (
	"strings"
	"testing"

	"github.com/aql-lang/aql/lang/go"
)

// Static check mode for generics (design/GENERICS.10.md Phase 5).
// Three pillars:
//
//  1. CALL-SITE refinement — a generic fn call's declared `[T]`
//     return refines to the binding inferred from the arg carriers
//     (the FnSummaries memo makes each instantiation analyse once).
//  2. DECLARATION-TIME abstract check — a generic fn's body is
//     analysed at construction with placeholder-parent carriers, so
//     undefined words and bound-unjustified operations surface at
//     the def (§9.4: ops on a bare T need the bound's justification).
//  3. Diagnostics — constraint_violation at `of` (loud), and
//     unbound_param + dynamic(Any) for returns naming a parameter no
//     argument can bind (never a silent strict Any).

// TestGenCheckCallSiteRefinement pins pillar 1: `(idg 5) add 1`
// checks clean because T binds Integer at the call.
func TestGenCheckCallSiteRefinement(t *testing.T) {
	res, err := checkSrc(t, `
def idg gen [T] fn [[x:T] [T] [x]]
(idg 5) add 1
`)
	if err != nil {
		t.Fatalf("check: %v", err)
	}
	for _, d := range res.Diagnostics {
		if d.Code == "no_signature" {
			t.Errorf("refined return should reach add: %s", d.Detail)
		}
	}
	if len(res.Stack) != 1 || !strings.Contains(res.Stack[0], "Integer") {
		t.Errorf("residual = %v, want Integer", res.Stack)
	}
}

// TestGenCheckTypedListElementRefinement pins the [:T] half of
// pillar 1: check mode strips the list literal to a typed-list
// carrier, whose child binds T.
func TestGenCheckTypedListElementRefinement(t *testing.T) {
	res, err := checkSrc(t, `
def first1 gen [T] fn [[xs:[:T]] [T] [xs get 0]]
(first1 [10 20 30]) add 1
`)
	if err != nil {
		t.Fatalf("check: %v", err)
	}
	for _, d := range res.Diagnostics {
		if d.Code == "no_signature" {
			t.Errorf("element-refined return should reach add: %s", d.Detail)
		}
	}
}

// TestGenCheckDeclarationTimeBody pins pillar 2's two halves: an
// undefined word inside an (uncalled) generic fn body is reported at
// the definition; an operation a bare unconstrained T cannot justify
// is reported (§9.4); the same operation under a Number bound is
// clean.
func TestGenCheckDeclarationTimeBody(t *testing.T) {
	res, err := checkSrc(t, `
def bad gen [T] fn [[x:T] [T] [x undefinedword]]
`)
	if err != nil {
		t.Fatalf("check: %v", err)
	}
	found := false
	for _, d := range res.Diagnostics {
		if d.Code == "undefined_word" && strings.Contains(d.Detail, "undefinedword") {
			found = true
		}
	}
	if !found {
		t.Errorf("uncalled generic fn body not analysed at declaration; diags = %v", diagCodes(res))
	}

	res, err = checkSrc(t, `
def f gen [T] fn [[x:T] [T] [x add 1]]
`)
	if err != nil {
		t.Fatalf("check: %v", err)
	}
	found = false
	for _, d := range res.Diagnostics {
		if d.Code == "no_signature" {
			found = true
		}
	}
	if !found {
		t.Errorf("unconstrained T admitted add — §9.4 wants the bound's justification; diags = %v", diagCodes(res))
	}

	res, err = checkSrc(t, `
def g gen [(T extends Number)] fn [[x:T] [T] [x add 1]]
(g 5) add 1
`)
	if err != nil {
		t.Fatalf("check: %v", err)
	}
	for _, d := range res.Diagnostics {
		if d.Code == "no_signature" {
			t.Errorf("Number-bounded T should justify add: %s", d.Detail)
		}
	}
}

// TestGenCheckSurfaceBoundedBody pins the S4×S2 composition: a
// surface-bounded placeholder carrier types its contract ops via
// checkModeSurfaceShape (the placeholder's lattice parent IS the
// surface node, so SurfaceInfoOf's parent-chain walk finds it).
func TestGenCheckSurfaceBoundedBody(t *testing.T) {
	res, err := checkSrc(t, surfaceCheckPreamble+`
def hh gen [(T extends Shape)] fn [[x:T] [Float] [area x]]
(hh (make Circle {r:2.0})) add 1.0
`)
	if err != nil {
		t.Fatalf("check: %v", err)
	}
	for _, d := range res.Diagnostics {
		if d.Code == "no_signature" || d.Code == "undefined_word" {
			t.Errorf("surface-bounded op degraded: %s: %s", d.Code, d.Detail)
		}
	}
}

// TestGenCheckUnboundParam pins pillar 3's unbound_param: a return
// slot naming a parameter no argument can bind reports ONCE and
// degrades to dynamic(Any) — visible in the residual stack.
func TestGenCheckUnboundParam(t *testing.T) {
	res, err := checkSrc(t, `
def loose gen [T] fn [[x:Integer] [T] [x]]
loose 1
`)
	if err != nil {
		t.Fatalf("check: %v", err)
	}
	count := 0
	for _, d := range res.Diagnostics {
		if d.Code == "unbound_param" {
			count++
		}
	}
	if count != 1 {
		t.Errorf("want exactly 1 unbound_param (deduped), got %d; diags = %v", count, diagCodes(res))
	}
	if len(res.Stack) != 1 || !strings.Contains(res.Stack[0], "dynamic") {
		t.Errorf("residual = %v, want dynamic(Any)", res.Stack)
	}
}

// TestGenCheckConstraintViolationLoud pins pillar 3's
// constraint_violation: `of` runs for real in check mode, so a bound
// violation is loud (the same failure `aql run` would raise).
func TestGenCheckConstraintViolationLoud(t *testing.T) {
	_, err := checkSrc(t, `
def Sorted gen [(T extends Number)] refine Record [items:[:T]]
def X (Sorted of [String])
`)
	if err == nil || !strings.Contains(err.Error(), "constraint_violation") {
		t.Errorf("of with a violating arg should be loud in check mode, got %v", err)
	}
}

// TestGenCheckJoinNoExplosion pins the JoinCarriers interaction: a
// branch join across several sibling instantiations stays within the
// carrier disjunct cap (under the cap: a disjunct; over it: the
// common ancestor — which for sibling instantiations is the SCHEMA
// node, a precise widening). No per-schema collapse machinery is
// needed in v1; this test is the canary that decides otherwise.
func TestGenCheckJoinNoExplosion(t *testing.T) {
	res, err := checkSrc(t, `
def Box gen [T] class {value:T}
def pickn fn [[n:Integer] [Any] [
  case n [
    1 [make (Box of [Integer]) {value:1}]
    2 [make (Box of [String]) {value:'s'}]
    3 [make (Box of [Float]) {value:1.0}]
    4 [make (Box of [Boolean]) {value:true}]
    [make (Box of [Number]) {value:9}]
  ]
]]
def b (pickn 3)
b is Box
`)
	if err != nil {
		t.Fatalf("check: %v", err)
	}
	for _, d := range res.Diagnostics {
		if d.Code == "step_budget_exceeded" {
			t.Errorf("cross-instantiation join exploded the step budget: %s", d.Detail)
		}
	}
}

// TestGenCheckBodyInternalInstantiation pins that the per-call
// bindings reach a body-internal `of [T]` during call-site analysis
// (the binding install around AnalyseFnBody).
func TestGenCheckBodyInternalInstantiation(t *testing.T) {
	res, err := checkSrc(t, `
def Box gen [T] class {value:T}
def boxit gen [T] fn [[x:T] [Any] [make (Box of [T]) {value:x}]]
def b (boxit 42)
b is Box
`)
	if err != nil {
		t.Fatalf("check: %v", err)
	}
	for _, d := range res.Diagnostics {
		if d.Severity == lang.SeverityError {
			t.Errorf("body-internal of [T] should check clean: %s: %s", d.Code, d.Detail)
		}
	}
}
