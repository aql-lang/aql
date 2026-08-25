package check

import (
	"testing"

	core "github.com/boru-lang/boru/core/go"
)

// TestParenPlacedFnCarrierGuards covers parenPlacedFnCarrier's defensive
// declines. The hook is asked by fnReturnPark on EVERY user-paren collapse
// (NUR073's BROAD park), including passes with no live recorder and values
// carrying no id, so each guard is a live path rather than a formality.
func TestParenPlacedFnCarrierGuards(t *testing.T) {
	reg, err := core.NewRegistry()
	if err != nil {
		t.Fatalf("NewRegistry: %v", err)
	}
	e := core.NewTop(reg)
	e.Tape = core.NewTape([]core.Value{core.NewInteger(1)}, 4)
	if parenPlacedFnCarrier(e, 0) {
		t.Error("a pass with no live recorder must decline")
	}
}
