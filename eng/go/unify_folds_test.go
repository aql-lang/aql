package eng

import (
	"testing"

	core "github.com/boru-lang/boru/core/go"
)

// The compound-type / degenerate-root dispatch in unifyInner is now a
// priority-ordered table (unifyFolds). These tests pin the behaviour the
// table order encodes — each positive case paired with the negative that
// proves the rule is actually being enforced, not merely permissive.

// Disjunct fold runs BEFORE the degenerate-root rules, so a disjunct
// containing None admits None; a bare None still rejects a non-None.
func TestUnifyFold_DisjunctBeforeNoneRoot(t *testing.T) {
	disj := core.NewDisjunct([]core.Value{core.NewTypeLiteral(core.TString), core.NewNone()})

	if _, ok := core.Unify(disj, core.NewNone()); !ok {
		t.Fatalf("`String or None` should unify with none (disjunct fold precedes the None root rule)")
	}
	// Negative: the None root rule still rejects a non-None value.
	if _, ok := core.Unify(core.NewNone(), core.NewInteger(5)); ok {
		t.Fatalf("none must not unify with an Integer")
	}
}

// The Any rule yields the other (more specific) side, from either
// position; Never's self-only rule rejects a non-Never partner.
func TestUnifyFold_AnyYieldsOtherNeverIsStrict(t *testing.T) {
	five := core.NewInteger(5)
	if got, ok := core.Unify(core.NewTypeLiteral(core.TAny), five); !ok || !core.ValuesEqual(got, five) {
		t.Fatalf("Any unify 5 should yield 5, got %v ok=%v", got, ok)
	}
	if got, ok := core.Unify(five, core.NewTypeLiteral(core.TAny)); !ok || !core.ValuesEqual(got, five) {
		t.Fatalf("5 unify Any should yield 5, got %v ok=%v", got, ok)
	}

	if _, ok := core.Unify(core.NewTypeLiteral(core.TNever), core.NewTypeLiteral(core.TNever)); !ok {
		t.Fatalf("Never should unify with Never")
	}
	// Negative: Never unifies with nothing else.
	if _, ok := core.Unify(core.NewTypeLiteral(core.TNever), five); ok {
		t.Fatalf("Never must not unify with an Integer")
	}
}

// The negation fold admits a value that does NOT satisfy the inner type
// and rejects one that does — exercised from both operand positions to
// confirm the table's swap (fold(b, a)) is wired correctly.
func TestUnifyFold_NegationComplement(t *testing.T) {
	notString := core.NewNegation(core.NewTypeLiteral(core.TString))

	if _, ok := core.Unify(notString, core.NewInteger(5)); !ok {
		t.Fatalf("5 should satisfy `tnot String`")
	}
	if _, ok := core.Unify(core.NewInteger(5), notString); !ok {
		t.Fatalf("`tnot String` should admit 5 from the right-hand position too")
	}
	// Negative: a String fails the complement.
	if _, ok := core.Unify(notString, core.NewString("hi")); ok {
		t.Fatalf("a String must not satisfy `tnot String`")
	}
}
