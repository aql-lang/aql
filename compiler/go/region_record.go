package compiler

import (
	"sort"

	core "github.com/boru-lang/boru/core/go"
)

// Region capture, PHASE A — the compiler's seat on core's RegionRecorder seam
// (design/FULL-COMPILATION.0.md §6.2, Stage 4).
//
// A region descriptor is assembled from two places, because its two halves are
// known at different times and neither site can see the other's:
//
//   - PHASE A, here, at collection time: the region's EXTENT and its
//     written-order Tokens. Both are properties of the tape, and the tape is
//     only in scope during collection. No operands exist yet.
//   - PHASE B, at RecordCall: SlotDesc.Source — which values are interned
//     consts, frame locals, prior events, compiled fragments. That is the
//     recorder's own operand model, and no tape index is in scope there.
//
// They are joined on (word, SrcPos), which both sides already hold — the
// dispatching word's own position. That was measured across the corpus with a
// two-phase probe rather than assumed, and the same measurement produced the
// two asymmetries this file must not get wrong:
//
//   - NOT EVERY RECORDED CALL HAS A REGION. `concat "a" "b"` never reaches
//     forward collection at all, so Phase B will find no capture for it. That
//     is correct: a region IS a forward-collecting dispatch, and a word that
//     took its operands from the stack has none.
//   - NOT EVERY REGION BECOMES A RECORDED CALL. `def` captures here and is
//     never recorded as a call, being a check-mode word. Its capture is simply
//     never claimed.
//
// So the map is a pool of OFFERS, not a ledger that must balance. Code that
// assumed a 1:1 pairing would be wrong in both directions.
func init() {
	core.RegionRecorder = tryRecordRegion
}

// regionKey is the (word, LOCATION) join between the two phases.
//
// The location is Row and Col ONLY — deliberately not a whole core.SrcPos.
// SrcPos carries a third field, Src, the source TEXT of the token, and
// including it would make the key depend on how a position was constructed
// rather than on where it points. Two positions naming the same place but
// built by different paths would then miss each other.
//
// That failure would have been near-invisible, which is why this is spelled
// out: a missed join looks exactly like "this call had no region", and that
// is a documented, EXPECTED outcome (a stack-only dispatch has no region).
// The bug would have hidden inside the asymmetry rather than surfacing as
// one. Caught by an end-to-end test asserting a specific capture was
// reachable, which a count-only assertion would have passed.
type regionKey struct {
	word string
	row  int
	col  int
}

func keyOf(word string, pos core.SrcPos) regionKey {
	return regionKey{word: word, row: pos.Row, col: pos.Col}
}

// tryRecordRegion captures the region a forward-collecting dispatch is about
// to walk. It READS the window and never evaluates — deciding what a dispatch
// claims is the collection walk's job, and later OpCollect's; recording what
// is THERE is this one's.
func tryRecordRegion(win core.CollectWindow, reg *core.Registry, w core.WordInfo, at int) {
	if reg == nil || win == nil || at < 0 || at >= win.Len() {
		return
	}
	es, _ := reg.Check.Recorder().(*EmitState)
	if es == nil || !es.Active() || es.SuspendedNow() {
		return
	}
	// Slots begin AFTER the dispatching word — the same start the collection
	// walk uses (resolveForwardArgs passes e.Pointer+1), so the extent
	// recorded is the extent walked.
	slots := CaptureRegionSlots(win, at+1)
	if slots == nil {
		return
	}
	if es.pendingRegions == nil {
		es.pendingRegions = map[regionKey]*RegionDesc{}
	}
	pos := win.At(at).Pos()
	es.pendingRegions[keyOf(w.Name, pos)] = &RegionDesc{
		Lead:  LeadWord,
		Word:  w.Name,
		Slots: slots,
		Pos:   pos,
	}
}

// PendingRegionCount reports how many Phase-A captures are held. It exists for
// tests: the captures are not observable any other way until Phase B completes
// them, and a capture step nothing can see is a capture step nothing can pin.
func (es *EmitState) PendingRegionCount() int {
	if es == nil {
		return 0
	}
	return len(es.pendingRegions)
}

// TakePendingRegion claims the Phase-A capture for (word, pos), reporting
// whether one existed. Phase B calls it to complete a descriptor; a miss is
// ordinary, not an error, per the asymmetries above.
func (es *EmitState) TakePendingRegion(word string, pos core.SrcPos) (*RegionDesc, bool) {
	if es == nil || es.pendingRegions == nil {
		return nil, false
	}
	d, ok := es.pendingRegions[keyOf(word, pos)]
	return d, ok
}

// PendingRegions returns the Phase-A captures in a stable order (by position,
// then word), so a caller can inspect what was recorded without depending on
// map iteration order. For tests and for Phase B's eventual drain.
func (es *EmitState) PendingRegions() []*RegionDesc {
	if es == nil || len(es.pendingRegions) == 0 {
		return nil
	}
	out := make([]*RegionDesc, 0, len(es.pendingRegions))
	for _, d := range es.pendingRegions {
		out = append(out, d)
	}
	sort.Slice(out, func(i, j int) bool {
		a, b := out[i], out[j]
		if a.Pos.Row != b.Pos.Row {
			return a.Pos.Row < b.Pos.Row
		}
		if a.Pos.Col != b.Pos.Col {
			return a.Pos.Col < b.Pos.Col
		}
		return a.Word < b.Word
	})
	return out
}
