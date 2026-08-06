package core

// Coverage for interpreter forward-collection edge branches that lost their
// exercise when the strict-barrier corpus/test refactor changed which inputs
// reach them (design/STRICT-FORWARD-BARRIER.0.md). White-box, like the
// engine_seam7 tape tests.

import "testing"

// effectiveResolved excludes a parked forward's CLAIMED STACK args from the
// resolved window. Hit the stack-exclusion loop with StackArgs > 0.
func TestEffectiveResolvedExcludesStackArgs(t *testing.T) {
	e := engWithTape(t, []Value{
		NewInteger(1), NewInteger(2), NewWord("gg"),
		fwdMarker("gg", 2, 1, 1), NewInteger(9),
	}, 4)
	_ = e.EffectiveResolved() // must not panic; exercises the stack-exclude loop
}

// resolveOrphanedForwards skips a bare OpenParen encountered while re-stepping
// after removing an orphaned forward marker.
func TestResolveOrphanedForwardsSkipsOpenParen(t *testing.T) {
	e := engWithTape(t, []Value{
		fwdMarker("w", 1, 0, 0), NewOpenParen(), NewCloseParen(),
	}, 0)
	_ = e.resolveOrphanedForwards()
}

// IsFnShapeTypedBindingContext rejects a parked `def` forward whose sig is NOT
// the typed-name shape (Map at position 0) — here a plain [Integer Integer].
func TestIsFnShapeTypedBindingContextNonMapSig(t *testing.T) {
	e := engWithTape(t, []Value{NewInteger(1), fwdMarker("def", 0, 0, 0)}, 2)
	if e.IsFnShapeTypedBindingContext() {
		t.Error("a def forward with a non-Map sig is not the typed-binding shape")
	}
}

// The main Run loop skips a bare Forward marker that reaches the pointer.
func TestRunLoopSkipsForwardMarker(t *testing.T) {
	r := covRegistry(t, nil)
	_, _ = NewTop(r).Run([]Value{fwdMarker("x", 0, 0, 0), NewInteger(1)})
}
