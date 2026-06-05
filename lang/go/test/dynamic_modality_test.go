package test

import (
	"testing"

	"github.com/aql-lang/aql/lang/go"
)

// TestDynamicContextGetGradualMatch demonstrates the bounded dynamic(T)
// modality end to end (design/dynamic-modality-report.0.md): `context
// get` on a statically-untracked key is an escape hatch that emits
// dynamic(Any) — optimistically compatible — instead of strict
// Carry<Any>. That dynamic carrier then matches a typed slot under
// `aql check` (here add's Number) where strict Carry<Any> would not.
//
// `add` has only a [Number, Number] signature (no Any catch-all), so a
// successful match is the gradual not-disjoint rule at work, not a
// fallback. Strict Carry<Any> would fail it (proven directly in
// eng/go/dynamic_match_test.go).
func TestDynamicContextGetGradualMatch(t *testing.T) {
	a, err := lang.New()
	if err != nil {
		t.Fatalf("new: %v", err)
	}

	res, err := a.Check(`context get "k" 1 add`)
	if err != nil {
		t.Fatalf("check: %v", err)
	}
	for _, d := range res.Diagnostics {
		if d.Code == "no_signature" {
			t.Fatalf("dynamic(Any) from context get should match the Number slot; got no_signature: %+v", res.Diagnostics)
		}
	}
	if len(res.Stack) != 1 || (res.Stack[0] != "Decimal" && res.Stack[0] != "Integer") {
		t.Fatalf("expected a numeric result from the gradual match, got stack=%v", res.Stack)
	}
}

// hasDiag reports whether a check produced a diagnostic with the given
// code.
func hasDiag(diags []lang.CheckDiagnostic, code string) bool {
	for _, d := range diags {
		if d.Code == code {
			return true
		}
	}
	return false
}

// TestDynamicContagionFlows pins gradual contagion: the dynamic modality
// flows through a dispatch chain instead of dying after one hop. `get`
// on a dynamic value yields a dynamic result, which then matches add's
// Number slot — where a strict Carry<Any> from `get` would have failed.
func TestDynamicContagionFlows(t *testing.T) {
	a, err := lang.New()
	if err != nil {
		t.Fatalf("new: %v", err)
	}
	res, err := a.Check(`context get "k" "x" get 1 add`)
	if err != nil {
		t.Fatalf("check: %v", err)
	}
	if hasDiag(res.Diagnostics, "no_signature") {
		t.Fatalf("dynamic should flow through get and match add; got no_signature: %+v", res.Diagnostics)
	}
}

// TestDynamicGuardDischarge pins the bridge back to strict typing: a
// guard on a dynamic binding discharges the modality — inside the
// then-branch the value is strictly its guarded type, so a provably
// disjoint use is flagged (which the bare dynamic value would have
// admitted), and a valid use passes.
func TestDynamicGuardDischarge(t *testing.T) {
	a, err := lang.New()
	if err != nil {
		t.Fatalf("new: %v", err)
	}

	// Without a guard: dynamic(Any) is optimistically compatible with
	// join's slot, so the disjoint use is admitted (no no_signature).
	bare, err := a.Check(`def x (context get "k") end x "s" join`)
	if err != nil {
		t.Fatalf("check bare: %v", err)
	}
	if hasDiag(bare.Diagnostics, "no_signature") {
		t.Fatalf("bare dynamic(Any) should match join optimistically, got no_signature: %+v", bare.Diagnostics)
	}

	// Guarded: in the then-branch x is strictly Integer (discharged), so
	// `x "s" join` (Integer into a List slot) is provably disjoint and
	// flagged.
	disjoint, err := a.Check(`def x (context get "k") end if [x is Integer] [x "s" join] [0]`)
	if err != nil {
		t.Fatalf("check disjoint: %v", err)
	}
	if !hasDiag(disjoint.Diagnostics, "no_signature") {
		t.Fatalf("guard should discharge to strict Integer and flag the disjoint join, got: %+v", disjoint.Diagnostics)
	}

	// Guarded + valid use: strict Integer into add's Number slot passes.
	valid, err := a.Check(`def x (context get "k") end if [x is Integer] [x 1 add] [0]`)
	if err != nil {
		t.Fatalf("check valid: %v", err)
	}
	if hasDiag(valid.Diagnostics, "no_signature") {
		t.Fatalf("guarded Integer should match add cleanly, got: %+v", valid.Diagnostics)
	}
}
