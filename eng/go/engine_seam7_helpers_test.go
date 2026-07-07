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
	if capturesForward(nil, 0) {
		t.Error("capturesForward(nil) must be false")
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
	if !forwardLiteralOperand(NewTypeLiteral(TInteger)) {
		t.Error("a bare type node is a forward literal operand")
	}
	// A carrier is non-concrete and not a bare type node → false.
	if forwardLiteralOperand(NewCarrier(TInteger)) {
		t.Error("a carrier is not a forward literal operand")
	}
	// A scalar concrete is an operand.
	if !forwardLiteralOperand(NewInteger(1)) {
		t.Error("an integer is a forward literal operand")
	}
	// A marker is not.
	if forwardLiteralOperand(NewOpenParen()) {
		t.Error("open paren is not a forward literal operand")
	}
}

// --- isRecordableLiteral --------------------------------------------------

func TestS7IsRecordableLiteral(t *testing.T) {
	if isRecordableLiteral(NewForward(ForwardInfo{})) {
		t.Error("Forward is not a recordable literal")
	}
	if isRecordableLiteral(NewOpenParen()) {
		t.Error("OpenParen is not a recordable literal")
	}
	if isRecordableLiteral(NewEnd()) {
		t.Error("End is not a recordable literal")
	}
	if !isRecordableLiteral(NewInteger(1)) {
		t.Error("Integer IS a recordable literal")
	}
	// nil parent → false.
	if isRecordableLiteral(Value{}) {
		t.Error("nil-parent value is not a recordable literal")
	}
}

// --- trivialDelegationTarget ----------------------------------------------

func TestS7TrivialDelegationTarget(t *testing.T) {
	// A one-word body with unnamed params → delegation.
	sig := &FnSig{Impl: AQL([]Value{NewWord("inner")})}
	name, ok := trivialDelegationTarget(sig)
	if !ok || name != "inner" {
		t.Errorf("got (%q,%v), want (inner,true)", name, ok)
	}
	// A named param disqualifies.
	sig2 := &FnSig{Params: []FnParam{{Name: "a", Type: TInteger}}, Impl: AQL([]Value{NewWord("inner")})}
	if _, ok := trivialDelegationTarget(sig2); ok {
		t.Error("named param must disqualify delegation shape")
	}
	// A non-word body element disqualifies.
	sig3 := &FnSig{Impl: AQL([]Value{NewInteger(1)})}
	if _, ok := trivialDelegationTarget(sig3); ok {
		t.Error("non-word body must disqualify")
	}
	// Wrong body length.
	sig4 := &FnSig{Impl: AQL([]Value{NewWord("a"), NewWord("b")})}
	if _, ok := trivialDelegationTarget(sig4); ok {
		t.Error("two-element body must disqualify")
	}
}

// --- singleOverloadRecoverable --------------------------------------------

func TestS7SingleOverloadRecoverableGuards(t *testing.T) {
	// nil / no ReturnsFn → false.
	if singleOverloadRecoverable(nil, nil) {
		t.Error("nil sig → false")
	}
	rf := func(args []Value, r *Registry) []Value { return nil }

	// Sole real sig with a real 0-arg sibling → has0ArgReal → false,
	// and exercises the s.TotalArgs()==0 assignment.
	argSig := Signature{Params: []FnParam{{Name: "n", Type: TInteger}}, ReturnsFn: rf, BarrierPos: -1}
	zeroSig := Signature{ReturnsFn: rf, BarrierPos: -1}
	fn := &FnDefInfo{Signatures: []Signature{argSig, zeroSig}}
	if singleOverloadRecoverable(&fn.Signatures[0], fn) {
		t.Error("a real 0-arg sibling makes it non-recoverable")
	}

	// Sole real sig that is a trivial delegation → false.
	delSig := Signature{
		Params:    []FnParam{{Type: TInteger}},
		ReturnsFn: rf,
		Impl:      AQL([]Value{NewWord("inner")}),
	}
	delSig.BarrierPos = -1
	fn2 := &FnDefInfo{Signatures: []Signature{delSig}}
	if singleOverloadRecoverable(&fn2.Signatures[0], fn2) {
		t.Error("a trivial-delegation sole sig is not recoverable")
	}
}

// --- concreteArgsMatch ----------------------------------------------------

func TestS7ConcreteArgsMatchArityShortfall(t *testing.T) {
	sig := &Signature{Params: []FnParam{{Type: TInteger}, {Type: TString}}, BarrierPos: -1}
	if concreteArgsMatch(sig, []Value{NewInteger(1)}, 0) {
		t.Error("arity shortfall must not report a match")
	}
}

func TestS7ConcreteArgsMatchAnyCarrierContinues(t *testing.T) {
	sig := &Signature{Params: []FnParam{{Type: TAny}, {Type: TString}}, BarrierPos: -1}
	// window[0] = Any carrier (deferrable), window[1] = String (matches).
	args := []Value{NewCarrier(TAny), NewString("x")}
	if !concreteArgsMatch(sig, args, 0) {
		t.Error("Any-carrier operand should be skipped, remaining args match")
	}
}

func TestS7ConcreteArgsMatchConcreteMismatch(t *testing.T) {
	sig := &Signature{Params: []FnParam{{Type: TInteger}}, BarrierPos: -1}
	if concreteArgsMatch(sig, []Value{NewString("x")}, 0) {
		t.Error("a wrong concrete operand must fail the match")
	}
}

// --- argTypeSummary / sigTypeSummary --------------------------------------

func TestS7ArgTypeSummaryNilParent(t *testing.T) {
	got := argTypeSummary([]Value{{}}) // nil Parent → "?"
	if got != "?" {
		t.Errorf("argTypeSummary(nil parent) = %q, want ?", got)
	}
	// dynamic carrier rendering.
	got = argTypeSummary([]Value{NewDynamicCarrier(TInteger)})
	if !strings.HasPrefix(got, "dynamic(") {
		t.Errorf("dynamic carrier summary = %q", got)
	}
	if argTypeSummary(nil) != "" {
		t.Error("empty args → empty summary")
	}
}

func TestS7SigTypeSummary(t *testing.T) {
	if sigTypeSummary(nil) != "" {
		t.Error("nil sig → empty")
	}
	if sigTypeSummary(&Signature{}) != "" {
		t.Error("0-arg sig → empty")
	}
	// A nil arg type renders "?".
	sig := &Signature{Args: []*Type{nil, TInteger}}
	got := sigTypeSummary(sig)
	if !strings.Contains(got, "?") || !strings.Contains(got, "Integer") {
		t.Errorf("sigTypeSummary = %q, want ? and Integer", got)
	}
}

// --- carrierSlotProvablyDisjoint ------------------------------------------

func TestS7CarrierSlotProvablyDisjointGuards(t *testing.T) {
	// t == nil → false.
	if carrierSlotProvablyDisjoint(NewCarrier(TInteger), nil) {
		t.Error("nil type → false")
	}
	// non-carrier → false.
	if carrierSlotProvablyDisjoint(NewInteger(1), TString) {
		t.Error("non-carrier → false")
	}
	// dynamic carrier → false.
	if carrierSlotProvablyDisjoint(NewDynamicCarrier(TInteger), TString) {
		t.Error("dynamic carrier → false")
	}
}

func TestS7CarrierSlotProvablyDisjointDisjunctEmpty(t *testing.T) {
	// A disjunct carrier with no alternatives → false (opaque union).
	d := NewValueRaw(TDisjunct, DisjunctInfo{Alternatives: nil})
	d.Carrier = true
	if carrierSlotProvablyDisjoint(d, TBoolean) {
		t.Error("empty disjunct → not provable")
	}
}

func TestS7CarrierSlotProvablyDisjointDisjunctOneOverlaps(t *testing.T) {
	// alternatives {Integer, String}, slot Integer → the Integer alt is
	// NOT disjoint from Integer, so the whole disjunct is not disjoint.
	d := NewDisjunct([]Value{NewTypeLiteral(TInteger), NewTypeLiteral(TString)})
	d.Carrier = true
	if carrierSlotProvablyDisjoint(d, TInteger) {
		t.Error("a disjunct containing Integer is not disjoint from Integer")
	}
}

func TestS7CarrierSlotProvablyDisjointDisjunctAllDisjoint(t *testing.T) {
	// alternatives {Integer, Float}, slot Boolean → both disjoint → true.
	d := NewDisjunct([]Value{NewTypeLiteral(TInteger), NewTypeLiteral(TFloat)})
	d.Carrier = true
	if !carrierSlotProvablyDisjoint(d, TBoolean) {
		t.Error("a disjunct of Integer|Float is disjoint from Boolean")
	}
}

// --- definiteSlotFail (word resolution chain) -----------------------------

func TestS7DefiniteSlotFailWordBranches(t *testing.T) {
	r := covRegistry(t, nil)
	// covWords registers cadd both in the function table AND as a def
	// value. Drop the def binding so the word-resolution chain falls
	// through Defs.Top / TopTypeBody to the Lookup (function-boundary)
	// branch — a function word never fills a value slot.
	delete(r.Defs.stacks, "cadd")
	e := NewTop(r)
	e.tape = NewTape(nil, stackHeadroom)
	sig := &Signature{Args: []*Type{TInteger}, BarrierPos: -1}

	// A function word is a scan boundary → definite fail (true).
	if !e.definiteSlotFail(sig, 0, NewWord("cadd")) {
		t.Error("a function word never fills a value slot → definite fail")
	}

	// A builtin type-leaf name word ("Integer") resolves to a type
	// literal; against an Integer slot it MATCHES → not a definite fail.
	if e.definiteSlotFail(sig, 0, NewWord("Integer")) {
		t.Error("Integer type name matches an Integer slot → not a fail")
	}
	// Against a String slot the Integer type literal does not match → fail.
	strSig := &Signature{Args: []*Type{TString}, BarrierPos: -1}
	if !e.definiteSlotFail(strSig, 0, NewWord("Integer")) {
		t.Error("Integer type name vs String slot → definite fail")
	}

	// A slashed type path resolves via ResolveTypePath.
	if e.definiteSlotFail(sig, 0, NewWord("Scalar/Number/Integer")) {
		t.Error("Scalar/Number/Integer path vs Integer slot → matches, not a fail")
	}
}

func TestS7DefiniteSlotFailTypeBodyWord(t *testing.T) {
	r := covRegistry(t, nil)
	// Bind a capitalised type alias so TopTypeBody resolves the word.
	if err := InstallType(r, "MyInt", NewTypeLiteral(TInteger)); err != nil {
		t.Fatalf("InstallType: %v", err)
	}
	e := NewTop(r)
	e.tape = NewTape(nil, stackHeadroom)
	// MyInt aliases Integer; against an Integer slot it matches → not a fail.
	sig := &Signature{Args: []*Type{TInteger}, BarrierPos: -1}
	if e.definiteSlotFail(sig, 0, NewWord("MyInt")) {
		t.Error("type-alias word matching an Integer slot → not a fail")
	}
	// Against a String slot → fail.
	strSig := &Signature{Args: []*Type{TString}, BarrierPos: -1}
	if !e.definiteSlotFail(strSig, 0, NewWord("MyInt")) {
		t.Error("MyInt (Integer) vs String slot → definite fail")
	}
}

// --- mixedFormStackSlotAny ------------------------------------------------

func TestS7MixedFormStackSlotAny(t *testing.T) {
	e := engWithTape(t, nil, 3)
	sig := &Signature{Params: []FnParam{{Type: TAny}, {Type: TInteger}}, BarrierPos: -1}
	// positions before the pointer → the min is an Any slot → true.
	if !mixedFormStackSlotAny(e, sig, []int{0, 1}) {
		t.Error("min stack slot is Any → true")
	}
	// No positions before the pointer → false.
	if mixedFormStackSlotAny(e, sig, []int{5, 6}) {
		t.Error("no stack slot before pointer → false")
	}
}
