package eng

import (
	"testing"

	core "github.com/boru-lang/boru/core/go"
)

// TestGuardArgs pins the clamps on guardArgs, the helper a runtime
// param-contract guard uses to slice the leading n locals (the real
// dispatch arguments, excluding a closure's trailing capture slots) as the
// failing tuple it rebuilds a rich no-signature diagnostic from. In a
// well-formed frame n == fn.NArgs is always within [0, len(locals)]; the
// clamps are the defensive floor/ceiling for any other n.
func TestGuardArgs(t *testing.T) {
	a, b := core.NewInteger(1), core.NewInteger(2)
	locals := []core.Value{a, b}

	// n within range returns the leading n locals.
	if got := guardArgs(locals, 1); len(got) != 1 || got[0].String() != a.String() {
		t.Fatalf("guardArgs(_, 1) = %v, want [1]", got)
	}

	// n above the available locals clamps to the whole slice.
	if got := guardArgs(locals, 5); len(got) != 2 {
		t.Fatalf("guardArgs(_, 5) = %v, want len 2", got)
	}

	// negative n clamps to the empty tuple.
	if got := guardArgs(locals, -1); len(got) != 0 {
		t.Fatalf("guardArgs(_, -1) = %v, want empty", got)
	}
}
