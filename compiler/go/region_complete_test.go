package compiler

import (
	"testing"

	core "github.com/boru-lang/boru/core/go"
)

// beginRegionPass returns an EmitState seated on a live registry with a check
// run open — the state Phase A and Phase B both require. The recorder must be
// assigned AFTER Begin, which installs its own.
func beginRegionPass(t *testing.T) (*EmitState, *core.Registry, func()) {
	t.Helper()
	reg, err := core.NewRegistry()
	if err != nil {
		t.Fatal(err)
	}
	done := reg.Check.Begin()
	es := NewEmitState()
	es.reg = reg
	reg.Check.Emit = es
	return es, reg, done
}

// capture drives Phase A for a word at row 1 col 1 over the given slot tokens.
func capture(t *testing.T, es *EmitState, reg *core.Registry, word string, toks ...core.Value) core.SrcPos {
	t.Helper()
	pos := core.SrcPos{Row: 1, Col: 1}
	w := core.WithPosAt(core.NewWord(word), pos)
	win := core.NewTape(append([]core.Value{w}, toks...), 0)
	tryRecordRegion(win, reg, core.WordInfo{Name: word, ArgCount: -1}, 0)
	if es.PendingRegionCount() != 1 {
		t.Fatalf("Phase A captured %d regions, want 1", es.PendingRegionCount())
	}
	return pos
}

// TestCompleteRegionFillsTheClaim pins the central rule: for the LEADING
// positions the written order and the signature order are the same order, so
// slot i is sig position i and its source is simply ops[i].
func TestCompleteRegionFillsTheClaim(t *testing.T) {
	es, reg, done := beginRegionPass(t)
	defer done()

	a, b := core.NewInteger(1), core.NewInteger(2)
	pos := capture(t, es, reg, "pair", a, b)

	d := es.completeRegion("pair", pos, []core.Value{a, b},
		[]EmitOperand{ConstOperand(7), ConstOperand(9)})
	if d == nil {
		t.Fatal("a captured region with matching operands must complete")
	}
	if d.NFwd != 2 {
		t.Fatalf("NFwd = %d, want 2 (both slots were the dispatch's operands)", d.NFwd)
	}
	if d.Slots[0].Source != SlotConst || d.Slots[0].Idx != 7 {
		t.Errorf("slot 0 = %v/%d, want SlotConst/7", d.Slots[0].Source, d.Slots[0].Idx)
	}
	if d.Slots[1].Source != SlotConst || d.Slots[1].Idx != 9 {
		t.Errorf("slot 1 = %v/%d, want SlotConst/9", d.Slots[1].Source, d.Slots[1].Idx)
	}
	if err := d.Validate(10, 0, 0); err != nil {
		t.Errorf("a completed descriptor must validate, got %v", err)
	}
	// The capture is claimed, not copied out of the way: a second completion
	// of the same dispatch has nothing left to take. (The pending map is a
	// pool of offers; the next execution re-offers it.)
	if es.PendingRegionCount() != 0 {
		t.Errorf("the capture must be claimed, %d left pending", es.PendingRegionCount())
	}
}

// The claim STOPS at the first slot that is not the operand — it does not skip
// it and keep going. A region runs to the next hard delimiter, so the tokens
// after a dispatch's own operands belong to later statements; treating one of
// them as claimed would source a slot this dispatch never took.
func TestCompleteRegionStopsAtTheFirstNonOperand(t *testing.T) {
	es, reg, done := beginRegionPass(t)
	defer done()

	a, other, c := core.NewInteger(1), core.NewInteger(2), core.NewInteger(3)
	pos := capture(t, es, reg, "pair", a, other, c)

	// The dispatch took `a` forward and its second operand from the stack:
	// args[1] is a value the region's slot 1 is not.
	d := es.completeRegion("pair", pos, []core.Value{a, c},
		[]EmitOperand{ConstOperand(1), ConstOperand(2)})
	if d == nil {
		t.Fatal("completion must still produce the descriptor")
	}
	if d.NFwd != 1 {
		t.Fatalf("NFwd = %d, want 1 — the claim stops at slot 1", d.NFwd)
	}
	for i := 1; i < len(d.Slots); i++ {
		if d.Slots[i].Source != SlotNone {
			t.Errorf("slot %d beyond the claim was sourced (%v)", i, d.Slots[i].Source)
		}
	}
	if err := d.Validate(10, 0, 0); err != nil {
		t.Errorf("unsourced slots beyond the claim are legal, got %v", err)
	}
}

// A dispatch that filled every position from the value stack claimed nothing
// forward. Zero is a legitimate NFwd, not a failure to complete.
func TestCompleteRegionClaimsNothingWhenTheStackFilledIt(t *testing.T) {
	es, reg, done := beginRegionPass(t)
	defer done()

	slot, arg := core.NewInteger(1), core.NewInteger(2)
	pos := capture(t, es, reg, "pair", slot)

	d := es.completeRegion("pair", pos, []core.Value{arg}, []EmitOperand{ConstOperand(0)})
	if d == nil {
		t.Fatal("completion must produce the descriptor")
	}
	if d.NFwd != 0 {
		t.Errorf("NFwd = %d, want 0 — the operand did not come from the region", d.NFwd)
	}
}

// A WORD slot is finished at capture and must STAY finished. Sourcing it from
// the operand would freeze the binding the word had during the pass, which is
// the miscompile the descriptor model exists to refuse: the same token is a
// value slot or a collection barrier depending on what it is bound to NOW.
func TestCompleteRegionKeepsAWordSlotLive(t *testing.T) {
	es, reg, done := beginRegionPass(t)
	defer done()

	bound := core.NewInteger(5)
	reg.Defs.Push("k", bound)
	pos := capture(t, es, reg, "w", core.NewWord("k"))

	d := es.completeRegion("w", pos, []core.Value{bound}, []EmitOperand{ConstOperand(3)})
	if d == nil {
		t.Fatal("completion must produce the descriptor")
	}
	if d.NFwd != 1 {
		t.Fatalf("NFwd = %d, want 1 — the word resolved to the operand", d.NFwd)
	}
	if d.Slots[0].Source != SlotWordRef {
		t.Errorf("slot 0 source = %v, want SlotWordRef — a word is resolved live", d.Slots[0].Source)
	}
	if d.Slots[0].Idx != 0 {
		t.Errorf("a wordRef addresses no table, got index %d", d.Slots[0].Idx)
	}
}

// An UNBOUND word cannot have supplied an operand, so it does not correspond
// and the claim stops there. Without the live resolution the comparison would
// be word-token against binding and would stop anyway — this pins that it
// stops for the right reason, by asking the def stack.
func TestCompleteRegionDeclinesAnUnboundWord(t *testing.T) {
	es, reg, done := beginRegionPass(t)
	defer done()

	pos := capture(t, es, reg, "w", core.NewWord("nosuch"))
	d := es.completeRegion("w", pos, []core.Value{core.NewInteger(5)}, []EmitOperand{ConstOperand(0)})
	if d == nil {
		t.Fatal("completion must produce the descriptor")
	}
	if d.NFwd != 0 {
		t.Errorf("NFwd = %d, want 0 — an unbound word supplied nothing", d.NFwd)
	}
}

// A word slot whose Token is not actually a Word is a capture defect, not a
// live-resolution question: it must stop the claim rather than resolve
// something else. Paired with the registry-less state, which the seam can be
// in when a descriptor is completed outside a pass.
func TestCompleteRegionDeclinesAMalformedWordSlot(t *testing.T) {
	es, _, done := beginRegionPass(t)
	defer done()

	v := core.NewInteger(1)
	s := SlotDesc{Source: SlotWordRef, Token: v}
	if es.slotIsOperand(s, v) {
		t.Error("a wordRef whose token is not a Word must not correspond")
	}
	es.reg = nil
	if es.slotIsOperand(s, v) {
		t.Error("a wordRef cannot be resolved with no registry")
	}
}

// The source translation, arm by arm. The corpus reaches const, wordRef and
// (once) event; local and type are reachable shapes the corpus happens not to
// claim, and the dynamic-scope kinds are the declined pair whose whole
// justification is that they are countable — 12 and 0 occurrences. Each is
// pinned here so the table cannot drift silently in either direction.
func TestRegionSourceOfTranslatesTheOperandModel(t *testing.T) {
	cases := []struct {
		name   string
		op     EmitOperand
		src    SlotSource
		idx    int
		resIdx int
		ok     bool
	}{
		{"const", ConstOperand(4), SlotConst, 4, 0, true},
		{"local", localOperand(2), SlotLocal, 2, 0, true},
		{"type", typeOperand(6), SlotType, 6, 0, true},
		{"event", EventOperand(8, 0), SlotEvent, 8, 0, true},
		{"event/result", EventOperand(8, 3), SlotEvent, 8, 3, true},
		{"unset", EmitOperand{}, SlotNone, 0, 0, false},
		{"dyn-scope", dynScopeOperand(1), SlotNone, 0, 0, false},
		{"data-scope", dataScopeOperand(1), SlotNone, 0, 0, false},
	}
	for _, c := range cases {
		src, idx, resIdx, ok := regionSourceOf(c.op)
		if ok != c.ok || src != c.src || idx != c.idx || resIdx != c.resIdx {
			t.Errorf("%s: got %v/%d/%d ok=%v, want %v/%d/%d ok=%v",
				c.name, src, idx, resIdx, ok, c.src, c.idx, c.resIdx, c.ok)
		}
	}
}

// An operand the descriptor model cannot express stops the claim rather than
// sourcing the slot with the invalid zero — the dynamic-scope decline, seen
// through completeRegion rather than through the translation table alone.
func TestCompleteRegionStopsOnAnInexpressibleOperand(t *testing.T) {
	es, reg, done := beginRegionPass(t)
	defer done()

	a := core.NewInteger(1)
	pos := capture(t, es, reg, "pair", a)
	d := es.completeRegion("pair", pos, []core.Value{a}, []EmitOperand{dynScopeOperand(0)})
	if d == nil {
		t.Fatal("completion must produce the descriptor")
	}
	if d.NFwd != 0 {
		t.Errorf("NFwd = %d, want 0 — a dynamic-scope operand has no slot source", d.NFwd)
	}
	if d.Slots[0].Source != SlotNone {
		t.Errorf("slot 0 = %v, want the invalid zero left in place", d.Slots[0].Source)
	}
}

// The misses are ORDINARY, not errors: not every recorded call has a region
// (a stack-only dispatch never reaches forward collection), and a region of
// zero slots is not a region. Both must return nil without disturbing
// anything.
func TestCompleteRegionMisses(t *testing.T) {
	es, reg, done := beginRegionPass(t)
	defer done()

	if d := es.completeRegion("nosuch", core.SrcPos{Row: 9, Col: 9}, nil, nil); d != nil {
		t.Error("a dispatch with no capture must complete to nil")
	}
	// A capture whose slot list is empty is stored as nil by Phase A; drive
	// the empty-slot arm directly, since Phase A cannot produce it.
	pos := core.SrcPos{Row: 2, Col: 2}
	es.pendingRegions = map[regionKey]*RegionDesc{
		keyOf("empty", pos): {Lead: LeadWord, Word: "empty"},
	}
	if d := es.completeRegion("empty", pos, nil, nil); d != nil {
		t.Error("a region of zero slots must complete to nil")
	}
	var nilES *EmitState
	if d := nilES.completeRegion("x", pos, nil, nil); d != nil {
		t.Error("a nil recorder must complete to nil")
	}
	_ = reg
}

// Completion must not write through to the pending capture. The same source
// position dispatches again on every loop iteration and every call of an
// enclosing fn, and a completion that mutated the capture in place would edit
// a descriptor an earlier Finalize may already have stamped into a Program.
func TestCompleteRegionDoesNotMutateTheCapture(t *testing.T) {
	es, reg, done := beginRegionPass(t)
	defer done()

	a := core.NewInteger(1)
	pos := capture(t, es, reg, "pair", a)
	held, _ := es.TakePendingRegion("pair", pos)
	es.pendingRegions[keyOf("pair", pos)] = held

	d := es.completeRegion("pair", pos, []core.Value{a}, []EmitOperand{ConstOperand(2)})
	if d == nil || d.Slots[0].Source != SlotConst {
		t.Fatal("the completed copy must carry the source")
	}
	if held.Slots[0].Source != SlotNone || held.NFwd != 0 {
		t.Errorf("the capture was mutated: source %v, NFwd %d", held.Slots[0].Source, held.NFwd)
	}
}

// The recorder keeps NO region table of its own, and that is what makes the
// emitted one rollback-safe. A descriptor lives on its call EVENT and is
// appended to Program.Regions by the lowerer, so a discarded loop-analysis
// round — whose events ride a captured fragment Rollback drops — takes its
// descriptors with it. A table beside the recorder would not: measured, moving
// the append from the recorder to the lowerer removed 55 descriptors from the
// corpus table, every one of them a round that was never lowered.
//
// Pinned as the structural fact rather than by driving Rollback, because
// Rollback does not truncate frames — it clears the capture (emit.go), and the
// discarded fragment is what carries the events.
func TestCompletedRegionLivesOnItsEventOnly(t *testing.T) {
	es, reg, done := beginRegionPass(t)
	defer done()

	a := core.NewInteger(1)
	pos := capture(t, es, reg, "pair", a)
	d := es.completeRegion("pair", pos, []core.Value{a}, []EmitOperand{ConstOperand(0)})
	if d == nil {
		t.Fatal("completion must produce the descriptor")
	}
	// Completion hands the descriptor BACK; it files nothing. The only place a
	// descriptor can be reached from is the event the caller puts it on.
	p, _, ok := es.Finalize(nil)
	if !ok {
		t.Fatalf("finalize: %v", es.Reason)
	}
	if len(p.Regions) != 0 {
		t.Errorf("%d descriptors reached the Program without a lowered event — "+
			"completion is filing them somewhere of its own", len(p.Regions))
	}
}

// Two id-less values must NOT correspond. core.NewValueRaw stamps an id only
// for a payload-less value or one minted inside the check pass, so a value
// built outside a pass carries "" — and comparing those by id would say every
// such pair is the same instance. That is the coincidental match in its worst
// form: it extends the claim over slots the dispatch never took. The guard
// makes the failure an UNDER-claim, which defers.
//
// Built deliberately outside the pass, because inside one every value has an
// id and the hazard is invisible — which is exactly why it needed pinning.
func TestSlotIsOperandRefusesIdlessValues(t *testing.T) {
	es := NewEmitState()
	a, b := core.NewInteger(1), core.NewInteger(2)
	if a.ID != "" || b.ID != "" {
		t.Skip("values minted here carry ids; the hazard this pins cannot arise")
	}
	if es.slotIsOperand(SlotDesc{Token: a}, b) {
		t.Error("two id-less values must not correspond — a claim would extend over a slot the dispatch never took")
	}
	if es.slotIsOperand(SlotDesc{Token: a}, a) {
		t.Error("even the same id-less value must not correspond: identity is unprovable without an id")
	}
	// A word slot takes the same guard before any registry work, so an
	// id-less operand cannot be matched by a live resolution either.
	if es.slotIsOperand(SlotDesc{Source: SlotWordRef, Token: core.NewWord("k")}, a) {
		t.Error("an id-less operand must not correspond to a word slot")
	}
}
