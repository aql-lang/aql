package compiler

import (
	"testing"

	core "github.com/boru-lang/boru/core/go"
)

func capTape(vals []core.Value) *core.Tape {
	return core.NewTape(vals, core.StackHeadroom)
}

// Written order is preserved and the extent stops at the delimiter.
func TestCaptureRegionSlotsWrittenOrder(t *testing.T) {
	w := capTape([]core.Value{
		core.NewWord("f"), core.NewInteger(1), core.NewInteger(2),
		core.NewEnd(), core.NewInteger(9),
	})
	got := CaptureRegionSlots(w, 0)
	if len(got) != 3 {
		t.Fatalf("captured %d slots, want 3 (stops at `end`)", len(got))
	}
	if !core.IsWord(got[0].Token) {
		t.Errorf("slot 0 should be the word token, got %v", got[0].Token)
	}
	if v, _ := core.AsInteger(got[2].Token); v != 2 {
		t.Errorf("slot 2 token = %v, want the integer 2 (written order)", got[2].Token)
	}
}

// An empty region returns nil, so "no region" and "zero slots" are
// distinguishable without a second call.
func TestCaptureRegionSlotsEmptyIsNil(t *testing.T) {
	w := capTape([]core.Value{core.NewEnd(), core.NewInteger(1)})
	if got := CaptureRegionSlots(w, 0); got != nil {
		t.Errorf("empty region = %v, want nil", got)
	}
	if got := CaptureRegionSlots(nil, 0); got != nil {
		t.Errorf("nil window = %v, want nil", got)
	}
}

// A negative start clamps rather than reading out of range.
func TestCaptureRegionSlotsClampsNegative(t *testing.T) {
	w := capTape([]core.Value{core.NewInteger(1), core.NewInteger(2)})
	if got := CaptureRegionSlots(w, -4); len(got) != 2 {
		t.Errorf("captured %d slots from a negative start, want 2", len(got))
	}
}

// A nested group is captured whole — the region contains it.
func TestCaptureRegionSlotsContainsGroup(t *testing.T) {
	w := capTape([]core.Value{
		core.NewWord("f"), core.NewOpenParen(), core.NewInteger(1),
		core.NewCloseParen(), core.NewEnd(),
	})
	if got := CaptureRegionSlots(w, 0); len(got) != 4 {
		t.Errorf("captured %d slots, want 4 (the group is contained)", len(got))
	}
}

// The static modifier is read off the token. It is the one half of a slot's
// description that is record-time-final: no binding can move it.
func TestCaptureRegionSlotsReadsQuote(t *testing.T) {
	w := capTape([]core.Value{
		core.NewInteger(1), core.NewAtom("nm"), core.NewDispatchMod(core.DispatchModInfo{Val: true}),
	})
	got := CaptureRegionSlots(w, 0)
	if len(got) != 3 {
		t.Fatalf("captured %d slots, want 3", len(got))
	}
	if got[0].Quote != QuoteNone {
		t.Errorf("plain value quote = %v, want QuoteNone", got[0].Quote)
	}
	if got[1].Quote != QuoteAtom {
		t.Errorf("atom quote = %v, want QuoteAtom (a /q slot is already an Atom)", got[1].Quote)
	}
	if got[2].Quote != QuoteValue {
		t.Errorf("/v mod quote = %v, want QuoteValue", got[2].Quote)
	}
	// …and /q on the same marker type is a DIFFERENT fact, not the same one.
	w2 := capTape([]core.Value{core.NewDispatchMod(core.DispatchModInfo{Quote: true})})
	if g2 := CaptureRegionSlots(w2, 0); len(g2) != 1 || g2[0].Quote != QuoteData {
		t.Errorf("/q mod quote = %v, want QuoteData", g2[0].Quote)
	}
}

// Source is left at its zero for the lowerer to fill: it is not knowable
// from the token, and guessing it here would be the frozen-class mistake in
// a different place.
func TestCaptureRegionSlotsLeavesSourceToLowerer(t *testing.T) {
	w := capTape([]core.Value{core.NewInteger(1)})
	got := CaptureRegionSlots(w, 0)
	if len(got) != 1 || got[0].Source != SlotConst {
		t.Errorf("Source = %v, want the SlotConst zero (lowerer fills it)", got[0].Source)
	}
}
