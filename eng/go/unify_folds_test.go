package eng

import "testing"

// The compound-type / degenerate-root dispatch in unifyInner is now a
// priority-ordered table (unifyFolds). These tests pin the behaviour the
// table order encodes — each positive case paired with the negative that
// proves the rule is actually being enforced, not merely permissive.

// Disjunct fold runs BEFORE the degenerate-root rules, so a disjunct
// containing None admits None; a bare None still rejects a non-None.
func TestUnifyFold_DisjunctBeforeNoneRoot(t *testing.T) {
	disj := NewDisjunct([]Value{NewTypeLiteral(TString), NewNone()})

	if _, ok := Unify(disj, NewNone()); !ok {
		t.Fatalf("`String or None` should unify with none (disjunct fold precedes the None root rule)")
	}
	// Negative: the None root rule still rejects a non-None value.
	if _, ok := Unify(NewNone(), NewInteger(5)); ok {
		t.Fatalf("none must not unify with an Integer")
	}
}

// The Any rule yields the other (more specific) side, from either
// position; Never's self-only rule rejects a non-Never partner.
func TestUnifyFold_AnyYieldsOtherNeverIsStrict(t *testing.T) {
	five := NewInteger(5)
	if got, ok := Unify(NewTypeLiteral(TAny), five); !ok || !ValuesEqual(got, five) {
		t.Fatalf("Any unify 5 should yield 5, got %v ok=%v", got, ok)
	}
	if got, ok := Unify(five, NewTypeLiteral(TAny)); !ok || !ValuesEqual(got, five) {
		t.Fatalf("5 unify Any should yield 5, got %v ok=%v", got, ok)
	}

	if _, ok := Unify(NewTypeLiteral(TNever), NewTypeLiteral(TNever)); !ok {
		t.Fatalf("Never should unify with Never")
	}
	// Negative: Never unifies with nothing else.
	if _, ok := Unify(NewTypeLiteral(TNever), five); ok {
		t.Fatalf("Never must not unify with an Integer")
	}
}

// The negation fold admits a value that does NOT satisfy the inner type
// and rejects one that does — exercised from both operand positions to
// confirm the table's swap (fold(b, a)) is wired correctly.
func TestUnifyFold_NegationComplement(t *testing.T) {
	notString := NewNegation(NewTypeLiteral(TString))

	if _, ok := Unify(notString, NewInteger(5)); !ok {
		t.Fatalf("5 should satisfy `tnot String`")
	}
	if _, ok := Unify(NewInteger(5), notString); !ok {
		t.Fatalf("`tnot String` should admit 5 from the right-hand position too")
	}
	// Negative: a String fails the complement.
	if _, ok := Unify(notString, NewString("hi")); ok {
		t.Fatalf("a String must not satisfy `tnot String`")
	}
}
