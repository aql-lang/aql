package compiler

import (
	"strings"
	"testing"

	core "github.com/boru-lang/boru/core/go"
)

func okDesc() *RegionDesc {
	return &RegionDesc{
		Lead:  LeadWord,
		Word:  "f",
		Slots: []SlotDesc{{Source: SlotConst, Idx: 0}},
	}
}

func TestRegionDescValidateAccepts(t *testing.T) {
	if err := okDesc().Validate(1, 0); err != nil {
		t.Errorf("a well-formed descriptor must validate, got %v", err)
	}
}

// The whole point of the sentinel: an unset source is REJECTED rather than
// silently read as a reference to Consts[0].
func TestRegionDescValidateRejectsUnsetSource(t *testing.T) {
	d := okDesc()
	d.Slots[0].Source = SlotNone
	err := d.Validate(1, 0)
	if err == nil {
		t.Fatal("SlotNone must be rejected, not treated as Consts[0]")
	}
	if !strings.Contains(err.Error(), "never given a source") {
		t.Errorf("error should name the missed initialisation, got %v", err)
	}
}

func TestRegionDescValidateRejectsNil(t *testing.T) {
	var d *RegionDesc
	if err := d.Validate(1, 0); err == nil {
		t.Error("a nil descriptor must be rejected")
	}
}

// A lead and its name must agree in both directions.
func TestRegionDescValidateRejectsLeadWordMismatch(t *testing.T) {
	d := okDesc()
	d.Word = ""
	if err := d.Validate(1, 0); err == nil {
		t.Error("LeadWord with no name must be rejected")
	}
	d2 := okDesc()
	d2.Lead = LeadApply
	if err := d2.Validate(1, 0); err == nil {
		t.Error("a non-word lead carrying a word name must be rejected")
	}
}

// Bounds are checked against the table the index actually addresses — an
// in-range-but-wrong index is what a sentinel alone cannot catch.
func TestRegionDescValidateRejectsOutOfRange(t *testing.T) {
	cases := []struct {
		name          string
		src           SlotSource
		idx           int
		nConsts, nFns int
	}{
		{"const past end", SlotConst, 5, 1, 0},
		{"const negative", SlotConst, -1, 1, 0},
		{"fragment past end", SlotGroup, 3, 1, 1},
		{"fragment negative", SlotGroup, -2, 1, 1},
		{"local negative", SlotLocal, -1, 1, 0},
		{"event negative", SlotEvent, -1, 1, 0},
	}
	for _, c := range cases {
		d := okDesc()
		d.Slots[0] = SlotDesc{Source: c.src, Idx: c.idx}
		if err := d.Validate(c.nConsts, c.nFns); err == nil {
			t.Errorf("%s: must be rejected", c.name)
		}
	}
}

// A local or event index is only sign-checked here: the lowerer owns the
// upper bound, since it knows the unit's own local count.
func TestRegionDescValidateAcceptsInRangeLocalAndEvent(t *testing.T) {
	for _, src := range []SlotSource{SlotLocal, SlotEvent} {
		d := okDesc()
		d.Slots[0] = SlotDesc{Source: src, Idx: 99}
		if err := d.Validate(0, 0); err != nil {
			t.Errorf("source %v index 99 should pass the sign check, got %v", src, err)
		}
	}
}

func TestRegionDescValidateRejectsUnknownSource(t *testing.T) {
	d := okDesc()
	d.Slots[0] = SlotDesc{Source: SlotSource(200)}
	if err := d.Validate(1, 0); err == nil {
		t.Error("an unknown source must be rejected")
	}
}

// A captured descriptor is malformed until the lowerer fills Source in —
// which is exactly the state CaptureRegionSlots leaves it in, so the two
// halves of the contract are pinned against each other.
func TestCapturedRegionIsMalformedUntilLowered(t *testing.T) {
	w := core.NewTape([]core.Value{core.NewInteger(1)}, core.StackHeadroom)
	d := &RegionDesc{Lead: LeadWord, Word: "f", Slots: CaptureRegionSlots(w, 0)}
	if err := d.Validate(1, 0); err == nil {
		t.Error("a freshly captured descriptor must not validate: the lowerer has not set Source")
	}
}
