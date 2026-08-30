package compiler

import (
	"fmt"

	core "github.com/boru-lang/boru/core/go"
)

// Validate rejects a malformed region descriptor before anything executes
// it (design/FULL-COMPILATION.0.md §6.2, Stage 4).
//
// The sentinel zero is only half a defence. SlotNone exists so a missed
// initialisation cannot masquerade as "Consts[0]" — the failure
// eng/go/CLAUDE.md's "No Zero-Value Overload (CRITICAL)" names — but a
// sentinel nothing checks just moves the silent wrong answer one step later,
// to whatever reads Idx. This is the check.
//
// It returns an error rather than panicking: a malformed descriptor is a
// compiler defect, and the design's own Stage 9 note says such a case
// becomes a structured internal_error return, never a panic (ADR-005 has no
// exception for "this should be impossible").
func (d *RegionDesc) Validate(nConsts, nFns, nTypes int) error {
	if d == nil {
		return fmt.Errorf("region descriptor is nil")
	}
	if d.Lead == LeadWord && d.Word == "" {
		return fmt.Errorf("region at %v: LeadWord with no word name", d.Pos)
	}
	if d.Lead != LeadWord && d.Word != "" {
		return fmt.Errorf("region at %v: word name %q on a non-word lead", d.Pos, d.Word)
	}
	for i := range d.Slots {
		if err := d.Slots[i].validate(i, d.Pos, nConsts, nFns, nTypes); err != nil {
			return err
		}
	}
	return nil
}

// validate checks one slot. The bounds are checked against the tables the
// index actually addresses, because an in-range-but-wrong index is the
// failure a sentinel cannot catch.
func (s *SlotDesc) validate(i int, pos core.SrcPos, nConsts, nFns, nTypes int) error {
	switch s.Source {
	case SlotNone:
		return fmt.Errorf("region at %v: slot %d was never given a source "+
			"(SlotNone is the invalid zero, not a reference to Consts[0])", pos, i)
	case SlotConst:
		if s.Idx < 0 || s.Idx >= nConsts {
			return fmt.Errorf("region at %v: slot %d const index %d out of range (%d consts)",
				pos, i, s.Idx, nConsts)
		}
	case SlotGroup:
		if s.Idx < 0 || s.Idx >= nFns {
			return fmt.Errorf("region at %v: slot %d fragment index %d out of range (%d fns)",
				pos, i, s.Idx, nFns)
		}
	case SlotType:
		if s.Idx < 0 || s.Idx >= nTypes {
			return fmt.Errorf("region at %v: slot %d type index %d out of range (%d types)",
				pos, i, s.Idx, nTypes)
		}
	case SlotLocal, SlotEvent:
		// Frame-local and event indices are validated by the lowerer against
		// the unit being built, which knows its own local count and event
		// sequence; there is no program-wide table to check them against
		// here. Only their sign is meaningful at this level.
		if s.Idx < 0 {
			return fmt.Errorf("region at %v: slot %d has a negative index %d", pos, i, s.Idx)
		}
	default:
		return fmt.Errorf("region at %v: slot %d has an unknown source %d", pos, i, s.Source)
	}
	// ResIdx names a result WITHIN a producing event, so it is meaningless
	// anywhere else. A non-zero value on another source is a lowerer defect
	// that would otherwise be read as "result N" by whatever consumes the
	// slot — the same class of silent wrong answer SlotNone exists to stop,
	// one field over.
	if s.ResIdx < 0 {
		return fmt.Errorf("region at %v: slot %d has a negative result index %d", pos, i, s.ResIdx)
	}
	if s.ResIdx != 0 && s.Source != SlotEvent {
		return fmt.Errorf("region at %v: slot %d carries result index %d on a non-event source %d",
			pos, i, s.ResIdx, s.Source)
	}
	return nil
}
