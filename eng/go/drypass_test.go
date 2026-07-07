package eng

import "testing"

// Direct pins for the dry-pass plumbing (drypass.go): operand
// canonicalisation and the concreteness gate.

func TestDryPassOperandsCanonicalisesNone(t *testing.T) {
	// A strict none-SHAPE arg becomes the canonical `none` sentinel, so
	// handler payload probes (IsNone, truthiness) agree with runtime;
	// concrete args pass through untouched (same backing slice when no
	// canonicalisation is needed).
	args := []Value{NewTypeLiteral(TNone), NewInteger(1)}
	got := dryPassOperands(args)
	if !IsNone(got[0]) {
		t.Errorf("none literal not canonicalised: %v", got[0])
	}
	if n, err := AsInteger(got[1]); err != nil || n != 1 {
		t.Errorf("concrete arg disturbed: %v", got[1])
	}

	allConcrete := []Value{NewInteger(2)}
	if same := dryPassOperands(allConcrete); &same[0] != &allConcrete[0] {
		t.Error("all-concrete args should pass through without copying")
	}
}

func TestDryPassConcretenessGate(t *testing.T) {
	if allConcreteArgs([]Value{NewInteger(1), NewCarrier(TInteger)}) {
		t.Error("a bare carrier arg must fail the dry-pass gate")
	}
	dynNone := NewCarrier(TNone)
	dynNone.Dynamic = true
	if allConcreteArgs([]Value{dynNone}) {
		t.Error("a DYNAMIC none carrier must fail the dry-pass gate")
	}
	if !allConcreteArgs([]Value{NewInteger(1), NewTypeLiteral(TNone)}) {
		t.Error("concrete + strict none-shape args must pass the gate")
	}
}

func TestEmitIndexOOBSilentInCaughtBody(t *testing.T) {
	// Inside a `do [...]` body the consuming word's runtime error is
	// trapped, so the provable-OOB mirror must stay silent there.
	r, err := NewRegistry()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(r.Check.Begin())
	r.Check.CaughtBodyDepth = 1
	CheckListIndex(r, NewInteger(5), NewList([]Value{NewInteger(1)}), "getr")
	if len(r.Check.Diagnostics) != 0 {
		t.Fatalf("caught-body OOB wrongly flagged: %+v", r.Check.Diagnostics)
	}
	r.Check.CaughtBodyDepth = 0
	CheckListIndex(r, NewInteger(5), NewList([]Value{NewInteger(1)}), "getr")
	if len(r.Check.Diagnostics) != 1 {
		t.Fatalf("uncaught OOB not flagged: %+v", r.Check.Diagnostics)
	}
}
