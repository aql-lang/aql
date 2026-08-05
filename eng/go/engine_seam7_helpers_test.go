package eng

// Wave-7 coverage, part 2: directly-callable pure helpers and small
// predicate methods in engine.go. These functions are white-box unit
// targets — hand-built values reach their guard/edge arms without a
// full Run(). See design/TEST-SEAMS.10.md.

import (
	"strings"
	"testing"
)

// --- resolveAtomReferents -------------------------------------------------

func TestS7ResolveAtomReferentsSkipsResolved(t *testing.T) {
	r := covRegistry(t, nil)
	a := SetAtomReferent(NewAtom("x"), NewInteger(5))
	vals := []Value{a}
	resolveAtomReferents(r, vals) // already has a referent → continue
	// Value unchanged (still has the same referent).
	if !IsAtom(vals[0]) {
		t.Fatal("expected atom preserved")
	}
}

// --- rawParenForward / rawFormForward / capturesForward -------------------

func TestS7RawForwardNilFn(t *testing.T) {
	if rawParenForward(nil, 0) {
		t.Error("rawParenForward(nil) must be false")
	}
	if rawFormForward(nil, 0) {
		t.Error("rawFormForward(nil) must be false")
	}
	if capturesForwardToken(nil, 0, NewWord("x")) {
		t.Error("capturesForwardToken(nil) must be false")
	}
}

// --- staticForwardType ----------------------------------------------------

func TestS7StaticForwardTypeGroup(t *testing.T) {
	e := &Engine{}
	if _, kind := e.staticForwardType(NewOpenParen()); kind != fwdGroup {
		t.Errorf("open paren classified as %v, want fwdGroup", kind)
	}
}

// --- forwardLiteralOperand ------------------------------------------------

func TestS7ForwardLiteralOperand(t *testing.T) {
	if !ForwardLiteralOperand(NewTypeLiteral(TInteger)) {
		t.Error("a bare type node is a forward literal operand")
	}
	// A carrier is non-concrete and not a bare type node → false.
	if ForwardLiteralOperand(NewCarrier(TInteger)) {
		t.Error("a carrier is not a forward literal operand")
	}
	// A scalar concrete is an operand.
	if !ForwardLiteralOperand(NewInteger(1)) {
		t.Error("an integer is a forward literal operand")
	}
	// A marker is not.
	if ForwardLiteralOperand(NewOpenParen()) {
		t.Error("open paren is not a forward literal operand")
	}
}

// --- isRecordableLiteral --------------------------------------------------

func TestS7IsRecordableLiteral(t *testing.T) {
	if IsRecordableLiteral(NewForward(ForwardInfo{})) {
		t.Error("Forward is not a recordable literal")
	}
	if IsRecordableLiteral(NewOpenParen()) {
		t.Error("OpenParen is not a recordable literal")
	}
	if IsRecordableLiteral(NewEnd()) {
		t.Error("End is not a recordable literal")
	}
	if !IsRecordableLiteral(NewInteger(1)) {
		t.Error("Integer IS a recordable literal")
	}
	// nil parent → false.
	if IsRecordableLiteral(Value{}) {
		t.Error("nil-parent value is not a recordable literal")
	}
}

// --- trivialDelegationTarget ----------------------------------------------

func TestS7TrivialDelegationTarget(t *testing.T) {
	// A one-word body with unnamed params → delegation.
	sig := &FnSig{Impl: Boru([]Value{NewWord("inner")})}
	name, ok := trivialDelegationTarget(sig)
	if !ok || name != "inner" {
		t.Errorf("got (%q,%v), want (inner,true)", name, ok)
	}
	// A named param disqualifies.
	sig2 := &FnSig{Params: []FnParam{{Name: "a", Type: TInteger}}, Impl: Boru([]Value{NewWord("inner")})}
	if _, ok := trivialDelegationTarget(sig2); ok {
		t.Error("named param must disqualify delegation shape")
	}
	// A non-word body element disqualifies.
	sig3 := &FnSig{Impl: Boru([]Value{NewInteger(1)})}
	if _, ok := trivialDelegationTarget(sig3); ok {
		t.Error("non-word body must disqualify")
	}
	// Wrong body length.
	sig4 := &FnSig{Impl: Boru([]Value{NewWord("a"), NewWord("b")})}
	if _, ok := trivialDelegationTarget(sig4); ok {
		t.Error("two-element body must disqualify")
	}
}

// --- singleOverloadRecoverable --------------------------------------------

func TestS7SingleOverloadRecoverableGuards(t *testing.T) {
	// nil / no ReturnsFn → false.
	if SingleOverloadRecoverable(nil, nil) {
		t.Error("nil sig → false")
	}
	rf := func(args []Value, r *Registry) []Value { return nil }

	// Sole real sig with a real 0-arg sibling → has0ArgReal → false,
	// and exercises the s.TotalArgs()==0 assignment.
	argSig := Signature{Params: []FnParam{{Name: "n", Type: TInteger}}, ReturnsFn: rf, BarrierPos: -1}
	zeroSig := Signature{ReturnsFn: rf, BarrierPos: -1}
	fn := &FnDefInfo{Signatures: []Signature{argSig, zeroSig}}
	if SingleOverloadRecoverable(&fn.Signatures[0], fn) {
		t.Error("a real 0-arg sibling makes it non-recoverable")
	}

	// Sole real sig that is a trivial delegation → false.
	delSig := Signature{
		Params:    []FnParam{{Type: TInteger}},
		ReturnsFn: rf,
		Impl:      Boru([]Value{NewWord("inner")}),
	}
	delSig.BarrierPos = -1
	fn2 := &FnDefInfo{Signatures: []Signature{delSig}}
	if SingleOverloadRecoverable(&fn2.Signatures[0], fn2) {
		t.Error("a trivial-delegation sole sig is not recoverable")
	}
}

// --- concreteArgsMatch ----------------------------------------------------

func TestS7ConcreteArgsMatchArityShortfall(t *testing.T) {
	sig := &Signature{Params: []FnParam{{Type: TInteger}, {Type: TString}}, BarrierPos: -1}
	if ConcreteArgsMatch(sig, []Value{NewInteger(1)}, 0) {
		t.Error("arity shortfall must not report a match")
	}
}

func TestS7ConcreteArgsMatchAnyCarrierContinues(t *testing.T) {
	sig := &Signature{Params: []FnParam{{Type: TAny}, {Type: TString}}, BarrierPos: -1}
	// window[0] = Any carrier (deferrable), window[1] = String (matches).
	args := []Value{NewCarrier(TAny), NewString("x")}
	if !ConcreteArgsMatch(sig, args, 0) {
		t.Error("Any-carrier operand should be skipped, remaining args match")
	}
}

func TestS7ConcreteArgsMatchConcreteMismatch(t *testing.T) {
	sig := &Signature{Params: []FnParam{{Type: TInteger}}, BarrierPos: -1}
	if ConcreteArgsMatch(sig, []Value{NewString("x")}, 0) {
		t.Error("a wrong concrete operand must fail the match")
	}
}

// --- argTypeSummary / sigTypeSummary --------------------------------------

func TestS7ArgTypeSummaryNilParent(t *testing.T) {
	got := ArgTypeSummary([]Value{{}}) // nil Parent → "?"
	if got != "?" {
		t.Errorf("argTypeSummary(nil parent) = %q, want ?", got)
	}
	// dynamic carrier rendering.
	got = ArgTypeSummary([]Value{NewDynamicCarrier(TInteger)})
	if !strings.HasPrefix(got, "dynamic(") {
		t.Errorf("dynamic carrier summary = %q", got)
	}
	if ArgTypeSummary(nil) != "" {
		t.Error("empty args → empty summary")
	}
}

func TestS7SigTypeSummary(t *testing.T) {
	if SigTypeSummary(nil) != "" {
		t.Error("nil sig → empty")
	}
	if SigTypeSummary(&Signature{}) != "" {
		t.Error("0-arg sig → empty")
	}
	// A nil arg type renders "?".
	sig := &Signature{Args: []*Type{nil, TInteger}}
	got := SigTypeSummary(sig)
	if !strings.Contains(got, "?") || !strings.Contains(got, "Integer") {
		t.Errorf("sigTypeSummary = %q, want ? and Integer", got)
	}
}

// --- mixedFormStackSlotAny ------------------------------------------------

func TestS7MixedFormStackSlotAny(t *testing.T) {
	e := engWithTape(t, nil, 3)
	sig := &Signature{Params: []FnParam{{Type: TAny}, {Type: TInteger}}, BarrierPos: -1}
	// positions before the pointer → the min is an Any slot → true.
	if !MixedFormStackSlotAny(e, sig, []int{0, 1}) {
		t.Error("min stack slot is Any → true")
	}
	// No positions before the pointer → false.
	if MixedFormStackSlotAny(e, sig, []int{5, 6}) {
		t.Error("no stack slot before pointer → false")
	}
}
