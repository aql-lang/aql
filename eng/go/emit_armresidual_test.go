package eng

import (
	"strings"
	"testing"
)

// captureInertArmResidual (the branch-arm twin of RecordLoop's all-inert
// capture): an all-inert multi-value arm residual is captured for the
// lowering's re-push; a parked Function, a value with no resolvable
// provenance, an event-produced entry, and a single-value arm all decline
// (residualOps stays nil — the arm keeps its refusal or its single-out path).
func TestCaptureInertArmResidualArms(t *testing.T) {
	r := seam7Reg(t)
	es := NewEmitState()
	es.bindRegistry(r)

	frag := &EmitFragment{}
	es.captureInertArmResidual(frag, []Value{NewInteger(1), NewInteger(2)})
	if len(frag.residualOps) != 2 {
		t.Fatalf("all-inert capture = %d ops, want 2", len(frag.residualOps))
	}

	es.captureInertArmResidual(nil, []Value{NewInteger(1), NewInteger(2)}) // nil frag: no panic

	single := &EmitFragment{}
	es.captureInertArmResidual(single, []Value{NewInteger(1)})
	if single.residualOps != nil {
		t.Error("a single-value arm must not capture")
	}

	fnArm := &EmitFragment{}
	es.captureInertArmResidual(fnArm, []Value{NewInteger(1), NewCarrier(TFunction)})
	if fnArm.residualOps != nil {
		t.Error("a parked-fn arm must not capture (the auto-apply hazard)")
	}

	dyn := NewCarrier(TAny)
	dyn.Dynamic = true
	dyn.ID = ""
	dynArm := &EmitFragment{}
	es.captureInertArmResidual(dynArm, []Value{NewInteger(1), dyn})
	if dynArm.residualOps != nil {
		t.Error("an unresolvable entry must not capture")
	}
}

// The variadic-region rematch seat: a trap whose leading operand is a
// variadic branch merge requires that merge slot on the sim TOP (the region
// tops the runtime stack); a mismatched sim declines and the whole-program
// refusal stands.
func TestLowerTrapVariadicRegionNotOnTop(t *testing.T) {
	es := NewEmitState()
	cf := &CompiledFn{}
	lw := &lowerer{es: es, p: &Program{}, code: &cf.Code, debug: &cf.Debug,
		sigIdx: map[*Signature]int{}, variadic: map[int]bool{7: true}, promoted: map[int]int{}}
	ev := emitEvent{kind: evTrap, trap: emitTrap{
		rematchWord:       "w",
		rematchOps:        []emitOperand{eventOperand(7, 0), constOperand(0)},
		rematchWrittenOff: 1,
		rematchNWritten:   1,
	}}
	if reason := lw.lowerTrap(&ev); reason == "" ||
		reason != "stack discipline: variadic rematch region is not on top" {
		t.Fatalf("empty-sim variadic rematch reason = %q, want the region-not-on-top decline", reason)
	}
}

// A multi-value arm that is neither all-inert-captured nor event-seated is
// irreconstructible: lowerFragment's default keeps the whole-program refusal
// (the sound fallback) — the arm the each row covered before its all-inert
// capture graduated it.
func TestLowerFragmentIrreconstructibleMultiArm(t *testing.T) {
	es := NewEmitState()
	cf := &CompiledFn{}
	lw := &lowerer{es: es, p: &Program{}, code: &cf.Code, debug: &cf.Debug,
		sigIdx: map[*Signature]int{}, variadic: map[int]bool{}, promoted: map[int]int{}}
	frag := &EmitFragment{residualN: 2}
	out := constOperand(0)
	reason := lw.lowerFragment(frag, &out, true, SrcPos{})
	if !strings.Contains(reason, "branch leaves extra values") {
		t.Fatalf("irreconstructible multi-arm reason = %q, want the branch-residual refusal", reason)
	}
}

// RecordTypedBind's inactive-recorder decline (the guard split when concrete
// PREDICATE operands were admitted — the fn-predicate bind is a runtime
// evaluation, so concreteness no longer short-circuits that kind).
func TestRecordTypedBindInactiveDeclines(t *testing.T) {
	es := NewEmitState()
	resume := es.Suspend()
	defer resume()
	if _, ok := es.RecordTypedBind(TypedBindSpec{Kind: TypedBindPredicate, Name: "x"},
		NewInteger(1), NewInteger(1), SrcPos{}); ok {
		t.Fatal("a suspended recorder must decline the typed-bind record")
	}
}
