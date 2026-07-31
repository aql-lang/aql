package lang_test

import (
	"testing"

	lang "github.com/boru-lang/boru/lang/go"
)

// A branch merge (`if`/loop/case result) where one arm is gradual (dynamic)
// must stay gradual, so a later concrete-typed consumer poly-matches it instead
// of a strict disjunct rejecting it. The canonical shape is the "default-or-
// self" rebind `def nd (if (nd eq none) [<fresh node>] [nd])` over a `nd:Any`
// (gradual) param: the then-arm is a concrete Map (a node constructor), the
// else-arm is the gradual `nd`, and the merge must remain dynamic(Map|…) so a
// downstream `nd:Map` rebuild helper matches — not a strict Disjunct(None|Map|…)
// failing no_signature (the tst/radix node-rebuild walkers).
func TestJoinCarriersDynamicArm(t *testing.T) {
	errCount := func(src string) int {
		a, _ := lang.New()
		cr, _ := a.Check(src)
		n := 0
		for _, d := range cr.Diagnostics {
			if d.Severity == "error" {
				n++
			}
		}
		return n
	}

	// Concrete-Map then-arm merged with a gradual else-arm, consumed by a
	// Map-typed helper. Must check clean.
	clean := `
def mk fn [[a:Any] [Map] [do {x: [a]}]] end
def g fn [[m:Map] [Integer] [1]] end
def f fn [[nd:Any] [Integer] [def nd2 (if (nd eq none) [none mk] [nd]) (nd2 g)]] end`
	if n := errCount(clean); n != 0 {
		t.Errorf("dynamic-arm merge consumed by Map param: expected 0 errors, got %d", n)
	}

	// NEGATIVE: two CONCRETE non-dynamic arms of disjoint type still merge to a
	// strict disjunct that a concrete-typed consumer rejects — the looseness is
	// scoped to a genuinely gradual arm, not invented for concrete unions.
	bad := `
def g fn [[m:Map] [Integer] [1]] end
def f fn [[flag:Boolean] [Integer] [def x (if flag [42] ["s"]) (x g)]] end`
	if n := errCount(bad); n == 0 {
		t.Errorf("a strict Integer|String union consumed by a Map param must still be flagged")
	}
}
