package core

import "testing"

// The `fnpred` declaration seam (NUR099). A predicate type used to be
// recognised by COUNTING PARAMETERS — one param meant "membership test",
// anything else meant "ordinary function" — which ADR-016 forbids outright:
// arity must never decide how a function behaves. `fnpred` replaces the
// inference with a declaration, and these pin the core half of it.

func predFn(pred bool, params ...FnParam) Value {
	return NewFunction(FnDefInfo{
		Predicate:  pred,
		Signatures: []Signature{{Params: params}},
	})
}

// TestIsDeclaredPredicateFn — the reader. Only a fn value that SAYS it is a
// predicate answers true; the shape of its parameters never enters into it.
func TestIsDeclaredPredicateFn(t *testing.T) {
	if !IsDeclaredPredicateFn(predFn(true, FnParam{Name: "n", Type: TInteger})) {
		t.Error("a fnpred-declared fn must read as a declared predicate")
	}
	// Two parameters, still declared: the count is not consulted.
	if !IsDeclaredPredicateFn(predFn(true,
		FnParam{Name: "a", Type: TAny}, FnParam{Name: "b", Type: TAny})) {
		t.Error("the declaration holds whatever the parameter count — ADR-016")
	}
	// One parameter, NOT declared: shape alone must not answer true.
	if IsDeclaredPredicateFn(predFn(false, FnParam{Name: "n", Type: TInteger})) {
		t.Error("a plain 1-param fn is not a DECLARED predicate")
	}
	if IsDeclaredPredicateFn(NewInteger(5)) {
		t.Error("a non-Function value must decline")
	}
	// Function-parented but carrying no FnDefInfo (a carrier).
	if IsDeclaredPredicateFn(NewCarrier(TFunction)) {
		t.Error("a Function carrier with no fn payload must decline")
	}
}

// TestMarkPredicateFn — the writer, and its totality. `fnpred`'s handler
// relies on it never needing a payload guard at the call site.
func TestMarkPredicateFn(t *testing.T) {
	marked := MarkPredicateFn(predFn(false, FnParam{Name: "n", Type: TInteger}))
	if !IsDeclaredPredicateFn(marked) {
		t.Error("marking must make the value read as a declared predicate")
	}
	// Total: a value with no fn payload comes back unchanged.
	plain := NewInteger(5)
	if got := MarkPredicateFn(plain); !ValuesEqual(got, plain) {
		t.Errorf("marking a non-fn value must be the identity, got %v", got)
	}
}

// TestPredicateInputTypeBelievesTheDeclaration — the input type is read off
// the FIRST parameter for a declared predicate whatever its arity, where an
// undeclared fn still falls back to the deprecated one-param inference.
func TestPredicateInputTypeBelievesTheDeclaration(t *testing.T) {
	// Declared, two params: believed, and the input is param 0.
	two := predFn(true, FnParam{Name: "a", Type: TInteger}, FnParam{Name: "b", Type: TString})
	if got := PredicateInputType(two); got != TInteger {
		t.Errorf("a declared predicate's input is its first param, got %v", got)
	}
	// Undeclared, two params: the deprecated arity route still declines.
	if got := PredicateInputType(predFn(false,
		FnParam{Name: "a", Type: TInteger}, FnParam{Name: "b", Type: TString})); got != nil {
		t.Errorf("an undeclared two-param fn must not read as a predicate, got %v", got)
	}
	// Zero params: no input to name, declared or not.
	if got := PredicateInputType(predFn(true)); got != nil {
		t.Errorf("a zero-param declaration has no input type, got %v", got)
	}
}
