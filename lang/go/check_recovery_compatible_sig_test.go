package lang

import (
	"strings"
	"testing"
)

// TestRecoveryCompatibleSigNoFalseError pins the check-mode recovery
// (engine.go checkModeAssumeSig): a SINGLE-overload fn applied to a value of
// statically-unknown type whose RUNTIME value DOES satisfy the sole sig must
// not be reported. matchSignature can't commit statically (the operand type is
// unknown), but the recovery records a guarded CALL_USER whose param contract
// the VM enforces at entry — so the diagnostic is correctly withheld and the
// runtime dispatch matches.
//
// This must NOT be confused with suppressing the diagnostic whenever the
// recovery's lenient scoring finds a best-fit (`bestMatch >= 0`): that gate was
// UNSOUND. A positive best-fit does not imply a runtime match — the recovery
// fall-through can score a bare type literal (`class Map`), a wrong-typed value,
// or a dynamic operand whose runtime value still MISSES the sole sig (e.g. a
// recursive fn whose argument is a List where the sig wants an Integer, which
// `signature_error`s at run time). Those are genuine errors and ARE reported;
// the gate dropped 16 of them (TestCheckAccuracyRatchet coverage 208→192).
func TestRecoveryCompatibleSigNoFalseError(t *testing.T) {
	// `m get "k"` is a dynamic (statically-unknown) carrier; at run time it is
	// the Integer 5, which satisfies `g`'s sole `[n:Integer]` sig. The
	// single-overload recovery records a guarded CALL_USER, so no no_signature.
	const src = `def g fn [ [n:Integer] [Integer] [ n add 1 ] ]
def m (flex {k:5})
def x (m get "k")
def _ (g x)`

	a, _ := New()
	res, _ := a.Check(src)
	for _, d := range res.Diagnostics {
		if d.Code == "no_signature" && (d.Word == "g" || strings.Contains(d.Detail, "for g")) {
			t.Errorf("a dynamic operand that satisfies the sole sig at runtime must not emit no_signature: %s", d.Detail)
		}
	}

	// NEGATIVE: a provable concrete mismatch (no compatible sig) MUST still
	// raise no_signature (or a hard signature error).
	const bad = `def need-map fn [ [m:Map] [Integer] [ 1 ] ]
def _ (need-map "not-a-map")`
	b, _ := New()
	res2, _ := b.Check(bad)
	flagged := false
	for _, d := range res2.Diagnostics {
		if d.Code == "no_signature" || d.Code == "signature_error" || strings.Contains(d.Detail, "no matching signature") {
			flagged = true
			break
		}
	}
	if !flagged {
		t.Errorf("a provable concrete mismatch (String → Map param) must still be flagged")
	}
}

// TestRecoveryConcreteMismatchAlongsideAny pins PR #211 review #1: the recovery
// suppression must attribute the unmatched dispatch SOLELY to the Any-typed
// operand. A multi-arg single-overload call with one unknown (Any) operand AND
// one provably-wrong CONCRETE operand is a genuine error — the concrete arg can
// never satisfy its param at run time — so it must stay flagged, not be
// suppressed just because some OTHER operand is an Any carrier.
func TestRecoveryConcreteMismatchAlongsideAny(t *testing.T) {
	// x is a dynamic (Any) carrier that satisfies a:Integer at run time, but the
	// literal 123 can never satisfy b:String. `aql run` raises signature_error, so
	// `aql check` must report it (concreteArgsMatch requires every NON-Any arg to
	// match the sole sig before the diagnostic is withheld).
	const src = `def g fn [ [a:Integer b:String] [Integer] [ a add 1 ] ]
def m (flex {k:5})
def x (m get "k")
def _ (g x 123)`
	a, _ := New()
	res, _ := a.Check(src)
	flagged := false
	for _, d := range res.Diagnostics {
		if d.Code == "no_signature" || d.Code == "signature_error" ||
			strings.Contains(d.Detail, "no matching signature") {
			flagged = true
			break
		}
	}
	if !flagged {
		t.Error("a concrete arg that misses its param must stay flagged even alongside an Any carrier")
	}
}
