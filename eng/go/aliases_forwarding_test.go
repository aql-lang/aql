package eng

import (
	"testing"

	core "github.com/boru-lang/boru/core/go"
)

// The facade's job is to FORWARD, so the contract worth pinning is that a
// wrapper and the core symbol it wraps answer identically. Most wrappers
// are covered incidentally by the suites that call them; this file pins
// the few whose only callers live outside Go code (boru source inside
// string literals, which the generator's AST scan cannot count as use)
// so the ADR-008 gate has no permanently-unreachable wrapper body.
func TestFacadeForwardsToCore(t *testing.T) {
	v := core.NewInteger(7)

	if got, want := Shape(v), core.Shape(v); got != want {
		t.Errorf("Shape forwards wrong: facade %v, core %v", got, want)
	}

	// A carrier and a bare type node exercise the other shape classes, so
	// the forwarding is pinned across the branch, not on one input.
	for _, probe := range []core.Value{
		core.NewCarrier(core.TInteger),
		core.NewTypeLiteral(core.TString),
		core.NewList([]core.Value{core.NewInteger(1)}),
	} {
		if got, want := Shape(probe), core.Shape(probe); got != want {
			t.Errorf("Shape(%v) forwards wrong: facade %v, core %v", probe, got, want)
		}
	}
}
