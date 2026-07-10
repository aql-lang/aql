package eng

// PROTOTYPE coverage for the strict-barrier rule (strictForwardBarrier /
// strandedForwardError): a function word beginning its own dispatch is
// always a forward-collection barrier, and a parked forward that cannot
// commit is reported stranded instead of waiting for the fn word's
// result. Tests are white-box and in-package, mirroring the
// commitBarrierForward seams in engine_seam7_barrier_test.go.

import (
	"strings"
	"testing"
)

// setStrict flips the prototype gate for one test and restores it.
func setStrict(t *testing.T, on bool) {
	t.Helper()
	prev := strictForwardBarrier
	strictForwardBarrier = on
	t.Cleanup(func() { strictForwardBarrier = prev })
}

// --- strandedForwardError direct seams ------------------------------------

func TestStrictBarrierInternalWordExempt(t *testing.T) {
	e := engWithTape(t, []Value{fwdMarker("size", 0, 0, 0)}, 1)
	if err := e.strandedForwardError("__pa"); err != nil {
		t.Errorf("engine-internal boundary word must be exempt, got %v", err)
	}
}

func TestStrictBarrierNoPendingForward(t *testing.T) {
	e := engWithTape(t, []Value{NewInteger(1)}, 1)
	if err := e.strandedForwardError("cadd"); err != nil {
		t.Errorf("no pending forward → nil, got %v", err)
	}
}

func TestStrictBarrierParenScopeStops(t *testing.T) {
	// A forward below an OpenParen is outside the current scope.
	e := engWithTape(t, []Value{fwdMarker("size", 0, 0, 0), NewOpenParen()}, 2)
	if err := e.strandedForwardError("cadd"); err != nil {
		t.Errorf("open-paren scope barrier → nil, got %v", err)
	}
}

// fwdMarkerRest parks a forward whose matched signature declares its
// second slot as statement-rest collection (Signature.RestArgs) — the
// shape def's body slot presents.
func fwdMarkerRest(name string, funcIdx, collected, stack int) Value {
	sig := &Signature{
		Args:       []*Type{TAtom, TAny},
		RestArgs:   map[int]bool{1: true},
		BarrierPos: -1,
	}
	return NewForward(ForwardInfo{
		FuncName: name, Sig: sig, FuncIndex: funcIdx,
		CollectedArgs: collected, StackArgs: stack,
	})
}

func TestStrictBarrierRestSlotExempt(t *testing.T) {
	// A waiting slot DECLARED statement-rest collects through the
	// boundary (def's body slot: `def f fn [...]`).
	e := engWithTape(t, []Value{fwdMarkerRest("def", 0, 1, 0)}, 1)
	if err := e.strandedForwardError("fn"); err != nil {
		t.Errorf("rest-declared slot must be exempt, got %v", err)
	}
}

func TestStrictBarrierRestSlotPositional(t *testing.T) {
	// The rest declaration is per-POSITION: the same sig waiting at
	// slot 0 (nothing collected) is NOT exempt.
	e := engWithTape(t, []Value{fwdMarkerRest("def", 0, 0, 0)}, 1)
	if err := e.strandedForwardError("fn"); err == nil {
		t.Error("waiting at a non-rest position must strand")
	}
}

func TestStrictBarrierDefNameAloneNotExempt(t *testing.T) {
	// The exemption is signature-declared, not by-name: a word named
	// "def" parked on a sig WITHOUT RestArgs is stranded like any other.
	e := engWithTape(t, []Value{fwdMarker("def", 0, 1, 0)}, 1)
	if err := e.strandedForwardError("fn"); err == nil {
		t.Error("name alone must not exempt — RestArgs is the policy")
	}
}

func TestStrictBarrierStrandedError(t *testing.T) {
	e := engWithTape(t, []Value{fwdMarker("size", 0, 0, 0)}, 1)
	err := e.strandedForwardError("iota")
	if err == nil {
		t.Fatal("expected a stranded-forward error")
	}
	if err.Code != "signature_error" {
		t.Errorf("code = %q, want signature_error", err.Code)
	}
	for _, want := range []string{"size", "iota", "strict rule", "size (iota …)"} {
		if !strings.Contains(err.Detail, want) {
			t.Errorf("detail %q missing %q", err.Detail, want)
		}
	}
}

// --- the stepWord seam ------------------------------------------------------

// strictTape parks an uncommittable forward (its word is unregistered,
// so commitBarrierForward declines) below a dispatching "cadd".
func strictTape() []Value {
	return []Value{
		NewInteger(1), NewWord("waiting"), fwdMarker("waiting", 1, 1, 0),
		NewWord("cadd"), NewInteger(2), NewInteger(3),
	}
}

func TestStrictBarrierStepWordOff(t *testing.T) {
	setStrict(t, false)
	e := engWithTape(t, strictTape(), 3)
	if err := e.stepWord(e.tape.At(3)); err != nil {
		t.Errorf("gate off keeps shipped behaviour, got %v", err)
	}
}

func TestStrictBarrierStepWordOn(t *testing.T) {
	setStrict(t, true)
	e := engWithTape(t, strictTape(), 3)
	err := e.stepWord(e.tape.At(3))
	if err == nil {
		t.Fatal("gate on: expected stranded-forward error from stepWord")
	}
	if !strings.Contains(err.Error(), "strict rule") {
		t.Errorf("error %q missing strict-rule detail", err.Error())
	}
}

// --- the stepWordUsurp seam -------------------------------------------------

func TestStrictBarrierStepWordUsurpOn(t *testing.T) {
	setStrict(t, true)
	vals := []Value{
		NewInteger(1), NewWord("waiting"), fwdMarker("waiting", 1, 1, 0),
		NewWordUsurp("cadd", false), NewInteger(2), NewInteger(3),
	}
	e := engWithTape(t, vals, 3)
	w, _ := AsWord(e.tape.At(3))
	err := e.stepWordUsurp(e.tape.At(3), w)
	if err == nil {
		t.Fatal("gate on: expected stranded-forward error from stepWordUsurp")
	}
	if !strings.Contains(err.Error(), "strict rule") {
		t.Errorf("error %q missing strict-rule detail", err.Error())
	}
}
