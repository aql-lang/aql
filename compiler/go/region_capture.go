package compiler

import core "github.com/boru-lang/boru/core/go"

// CaptureRegionSlots reads the tokens of the region starting at `from` into
// written-order SlotDescs (design/FULL-COMPILATION.0.md §6.2, Stage 4).
//
// It READS the window and never evaluates: no group is run, no sugar
// lowered, no forward collected. That is deliberate and it is why this can
// take a CollectWindow without a collectHost — the recorder's job is to
// record what is THERE, not to decide what a dispatch would claim. Deciding
// is OpCollect's, at run time, against the live binding set.
//
// Every slot is captured with its Token and its Source left at SlotNone —
// the INVALID zero, not a valid-looking SlotConst. The operand source is not
// knowable from the token alone (it comes from the recorder's own operand
// model: which values are interned, which are frame locals, which are prior
// events), so the lowerer fills it in, and a slot that reaches execution
// still holding SlotNone is a missed initialisation rather than a silent
// reference to constant 0. What this establishes is the region's EXTENT and
// its written order, which are properties of the tape and nothing else.
//
// Returns nil for an empty region, so a caller can distinguish "no region"
// from "a region of zero slots" without a second call.
func CaptureRegionSlots(w core.CollectWindow, from int) []SlotDesc {
	n := core.RegionSlotCount(w, from)
	if n == 0 {
		return nil
	}
	if from < 0 {
		from = 0
	}
	slots := make([]SlotDesc, 0, n)
	for i := from; i < from+n; i++ {
		slots = append(slots, SlotDesc{
			Quote: quoteOfToken(w.At(i)),
			Token: w.At(i),
		})
	}
	return slots
}

// quoteOfToken reads the STATIC dispatch-control modifier a token carries.
//
// Unlike a slot's class, a modifier is syntax: it is on the token, no
// binding can move it, and it is therefore sound to freeze at record time —
// the one half of a slot's description that genuinely is record-time-final.
func quoteOfToken(v core.Value) SlotQuote {
	if core.IsAtom(v) {
		// A /q slot has already become an Atom by the time it is on the
		// tape: the quote is not a pending instruction, it is done.
		return QuoteAtom
	}
	if core.IsDispatchMod(v) {
		// /v and /q are distinct facts on ONE marker type and mean different
		// things downstream: /v disables the call, /q treats the RESULT as
		// data. Collapsing them loses the distinction the payload carries.
		if info, ok := v.Data.(core.DispatchModInfo); ok && info.Quote {
			return QuoteData
		}
		return QuoteValue
	}
	return QuoteNone
}
