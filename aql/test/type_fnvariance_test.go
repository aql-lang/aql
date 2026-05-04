package test

import (
	"testing"
)

// --- Structural fn-shape variance ---
//
// `type Foo fn [[input] [output]]` is a structural function-shape
// constraint. A candidate function value satisfies the constraint
// under the standard rules:
//
//   - Inputs are CONTRAVARIANT: candidate's input must be a
//     supertype-or-equal of spec's input. (A function that accepts
//     Number also accepts Integer; that's why `(Number)->X` can
//     stand in for `(Integer)->X`.)
//   - Returns are COVARIANT: candidate's return must be a
//     subtype-or-equal of spec's return. (A function that returns
//     Integer also returns Number; that's why `X->(Integer)` can
//     stand in for `X->(Number)`.)
//
// Exact match remains a special case (most-restrictive form);
// variance widens what was previously accepted, so old tests that
// asserted exact matches still pass.

// (Number)→(Integer) satisfies (Integer)→(Number):
// - Input spec=Integer, sig=Number: Integer ⊆ Number → ok.
// - Return spec=Number, sig=Integer: Integer ⊆ Number → ok.
func TestVariance_BroaderInputNarrowerReturn(t *testing.T) {
	got := runOne(t, `type M fn [[Integer] [Number]]
def f fn [[Number] [Integer] [convert Integer]]
(quote f) is M`)
	if len(got) != 1 || got[0] != "true" {
		t.Errorf("(Number)→(Integer) is (Integer)→(Number) = %v, want true", got)
	}
}

// Reversed direction: a candidate that takes only Integer can't stand
// in where Number is required. Inputs go the wrong way (Number ⊄ Integer).
func TestVariance_NarrowerInputBroaderReturnFails(t *testing.T) {
	got := runOne(t, `type M fn [[Number] [Integer]]
def f fn [[Integer] [Number] [add 0.5]]
(quote f) is M`)
	if len(got) != 1 || got[0] != "false" {
		t.Errorf("(Integer)→(Number) is (Number)→(Integer) = %v, want false", got)
	}
}

// Exact match still satisfies (regression).
func TestVariance_ExactStillMatches(t *testing.T) {
	got := runOne(t, `type M fn [[Integer] [Integer]]
def f fn [[Integer] [Integer] [1 add]]
(quote f) is M`)
	if len(got) != 1 || got[0] != "true" {
		t.Errorf("exact match = %v, want true", got)
	}
}

// Input-only widening: candidate accepts Any, spec demands Integer.
// Any covers Integer → satisfied.
func TestVariance_AnyInputAcceptsConcreteSpec(t *testing.T) {
	got := runOne(t, `type M fn [[Integer] [Integer]]
def f fn [[Any] [Integer] [convert Integer]]
(quote f) is M`)
	if len(got) != 1 || got[0] != "true" {
		t.Errorf("(Any)→(Integer) is (Integer)→(Integer) = %v, want true", got)
	}
}

// Return-only narrowing: candidate returns Integer, spec promises Any.
// Integer ⊆ Any → satisfied.
func TestVariance_NarrowReturnSatisfiesAnySpec(t *testing.T) {
	got := runOne(t, `type M fn [[Integer] [Any]]
def f fn [[Integer] [Integer] [1 add]]
(quote f) is M`)
	if len(got) != 1 || got[0] != "true" {
		t.Errorf("(Integer)→(Integer) is (Integer)→(Any) = %v, want true", got)
	}
}

// Spec demands Any input ("I'll pass anything"); candidate accepts
// only Integer → fails. Any ⊄ Integer.
func TestVariance_AnySpecInputRejectsNarrowSig(t *testing.T) {
	got := runOne(t, `type M fn [[Any] [Integer]]
def f fn [[Integer] [Integer] [1 add]]
(quote f) is M`)
	if len(got) != 1 || got[0] != "false" {
		t.Errorf("(Integer)→(Integer) is (Any)→(Integer) = %v, want false", got)
	}
}
