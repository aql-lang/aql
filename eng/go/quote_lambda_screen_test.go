package eng

// quote_lambda_screen_test.go pins the quote-polarity screen
// (checker-compiler-completeness-review.0.md §2.2): an Atom-typed lambda
// param is a quote-capture slot the runtime never binds from a delivered
// stack value, so (a) lambdaHookCompatible must decline such a lambda as a
// HOF callback, and (b) quoteParamCarrierBind must report a matched sig
// binding a computed carrier to such a slot (the closure-unit guard that
// declines the check-mode apply the runtime rejects).

import "testing"

func TestLambdaHookQuoteScreen(t *testing.T) {
	body := Boru([]Value{NewWord("x")})

	// An Atom-typed param — the /q quote-capture class — declines even
	// though the input carrier satisfies the type: the runtime leaves the
	// lambda as data, so compiling an applying callback would diverge.
	atomFd := &FnDefInfo{Signatures: []Signature{{
		Params: []FnParam{{Name: "k", Type: TAtom}}, Impl: body,
	}}}
	if _, ok := lambdaHookCompatible(atomFd, []Value{NewCarrier(TAtom)}, ClosureInValue, true); ok {
		t.Error("an Atom-typed lambda param must decline the callback admission")
	}
	// The explicit Quote flag declines identically, whatever the type.
	quoteFd := &FnDefInfo{Signatures: []Signature{{
		Params: []FnParam{{Name: "k", Type: TInteger, Quote: true}}, Impl: body,
	}}}
	normalizeSig(&quoteFd.Signatures[0])
	if _, ok := lambdaHookCompatible(quoteFd, []Value{NewCarrier(TInteger)}, ClosureInValue, true); ok {
		t.Error("a /q lambda param must decline the callback admission")
	}
	// Control: a plain Integer param keeps admitting.
	intFd := &FnDefInfo{Signatures: []Signature{{
		Params: []FnParam{{Name: "n", Type: TInteger}}, Impl: body,
	}}}
	if _, ok := lambdaHookCompatible(intFd, []Value{NewCarrier(TInteger)}, ClosureInValue, true); !ok {
		t.Error("a value-typed lambda param must keep admitting")
	}
}

func TestQuoteParamCarrierBind(t *testing.T) {
	atomCarrier := NewCarrier(TAtom)
	concreteAtom := NewAtom("meta")

	// A carrier bound to an Atom-typed param: the diverging shape.
	if !quoteParamCarrierBind([]FnParam{{Name: "k", Type: TAtom}}, []Value{atomCarrier}) {
		t.Error("a carrier at an Atom-typed param must report the bind")
	}
	// The explicit Quote flag reports too.
	if !quoteParamCarrierBind([]FnParam{{Name: "k", Type: TInteger, Quote: true}}, []Value{NewCarrier(TInteger)}) {
		t.Error("a carrier at a /q param must report the bind")
	}
	// A CONCRETE atom (a bare-word collection's legitimate arrival) does not.
	if quoteParamCarrierBind([]FnParam{{Name: "k", Type: TAtom}}, []Value{concreteAtom}) {
		t.Error("a concrete atom arrival is the legitimate /q bind — no report")
	}
	// A value-typed param never reports.
	if quoteParamCarrierBind([]FnParam{{Name: "n", Type: TInteger}}, []Value{NewCarrier(TInteger)}) {
		t.Error("a value-typed param must not report")
	}
	// Fewer args than params: the missing slot is not a bind.
	if quoteParamCarrierBind([]FnParam{{Name: "k", Type: TAtom}}, nil) {
		t.Error("an absent arg is not a bind")
	}
}
