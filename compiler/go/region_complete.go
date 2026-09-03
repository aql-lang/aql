package compiler

import core "github.com/boru-lang/boru/core/go"

// Region capture, PHASE B — completing a Phase-A capture against the
// recorder's own operand model (design/FULL-COMPILATION.0.md §6.2, Stage 4).
//
// Phase A (region_record.go) records what the region CONTAINS: its extent,
// its written-order tokens, each token's static quote modifier, and — for a
// word — the finished SlotWordRef source, because a word's binding is
// resolved live and there is nothing a later pass could learn. Every other
// slot is left at the invalid SlotNone, because its source comes from the
// recorder's operand model and no tape index is in scope there.
//
// THIS IS NOT THE JOIN THREE MODELS WERE REVERTED FOR. Those tried to match
// a written-order slot against an already-RESOLVED operand by searching for
// it. The rule makes searching unnecessary: matching fills sig positions from
// the FORWARD tokens in written order, then fills every remaining position
// from the value stack. So for the leading positions the two orders are the
// SAME order, and slot i is sig position i — a direct index, not a search.
//
// What the reverted models actually got wrong was ASSUMING that. Measured
// over the corpus, the prefix rule holds at 99.83% and the residue is real:
// a `none` word rendering against a `None` value, a record type against its
// expanded object form, and a handful whose slots and args genuinely
// disagree. The recorder holds both the slots and the operands here, so the
// correspondence costs one comparison per claimed slot and becomes a CHECKED
// precondition. Where it stops, the claim stops: that is NFwd.
//
// Nothing here interns, allocates a const, or otherwise touches a program
// table. Phase A must stay side-effect-free because a region is offered on
// EVERY execution of its dispatch (a capture that interned would append a
// fresh const per loop iteration — es.intern never pools compounds); Phase B
// runs once per RECORDED call, and it only copies indices the operand model
// has already assigned.

// completeRegion claims the Phase-A capture for this dispatch and fills the
// sources of the slots it actually claimed, reporting the completed
// descriptor. A miss is ordinary and returns nil: not every recorded call has
// a region (a stack-only dispatch never reaches forward collection), and not
// every region becomes a recorded call (`def` captures and is never recorded,
// being a check-mode word).
//
// args are in SIGNATURE order and ops are their operands, index for index —
// RecordCallOperands' own contract.
func (es *EmitState) completeRegion(word string, pos core.SrcPos, args []core.Value, ops []EmitOperand) *RegionDesc {
	if es == nil {
		return nil
	}
	d, ok := es.TakePendingRegion(word, pos)
	if !ok || d == nil || len(d.Slots) == 0 {
		return nil
	}
	// The capture is keyed by position and re-offered on every execution, so
	// completing in place would mutate a descriptor an earlier completion may
	// already have stamped. Work on a copy; the slots are copied with it.
	out := &RegionDesc{Lead: d.Lead, Word: d.Word, Pos: d.Pos,
		Slots: append([]SlotDesc(nil), d.Slots...)}
	n := len(out.Slots)
	if len(args) < n {
		n = len(args)
	}
	for i := 0; i < n; i++ {
		if !es.slotIsOperand(out.Slots[i], args[i]) {
			break
		}
		src, idx, resIdx, ok := regionSourceOf(ops[i])
		if !ok {
			break
		}
		// A word slot is already finished, and finishing it AGAIN from the
		// operand is the frozen-class mistake this model exists to refuse:
		// the operand is the binding the word had during the pass, and the
		// whole point of SlotWordRef is that the next execution may find a
		// different one (region_desc.go's `k` pair).
		//
		// EXCEPT when the operand is a FRAME LOCAL, and that exception is the
		// fn-unit hazard rather than an exception to the rule. Inside a fn
		// body the analysis binds each param into the def stack so the body
		// can be analysed, so a param's word slot resolves here — but the
		// emitted body reads it from the FRAME, and at run time the def stack
		// holds no such binding. Live re-derivation would miss it or, worse,
		// find an outer binding of the same name. The frame slot is the
		// truth, and it is not a frozen class: no module-scope rebind can
		// move a param or a loop iterator between executions.
		if out.Slots[i].Source != SlotWordRef || src == SlotLocal {
			out.Slots[i].Source, out.Slots[i].Idx, out.Slots[i].ResIdx = src, idx, resIdx
		}
		out.NFwd = i + 1
	}
	return out
}

// slotIsOperand reports whether written-order slot s is the operand that
// filled its sig position — the checked half of the prefix rule.
//
// The comparison is by VALUE IDENTITY, not by structural equality, and that
// is the point: a forward-collected operand IS the tape token the capture
// recorded, the same instance, so its ID matches. Structural equality would
// admit a coincidence — `add 1 1` fills sig position 1 from the stack with a
// value equal to the forward token at written position 1 — and a coincidental
// match would extend the claim past what the dispatch really took forward.
//
// A WORD slot is the one indirection. Its operand is the BINDING, not the
// word token, so the comparison resolves the name through the live def stack
// exactly as the matcher's own patternsOk does before unifying. An unbound
// word never corresponds: it cannot have supplied an operand.
// An EMPTY id never corresponds, and that guard is load-bearing rather than
// defensive. core.NewValueRaw stamps an id only for a payload-less value or
// one minted inside the check pass, so a value built outside a pass carries
// "" — and two of those would compare EQUAL, which is the coincidental match
// this function exists to refuse, in its worst form: it would extend the
// claim over a slot the dispatch never took. Tape tokens do carry ids today
// (the parser stamps them, measured), so the guard costs nothing; what it
// buys is that the failure mode is an UNDER-claim, which defers, rather than
// an over-claim, which miscompiles.
func (es *EmitState) slotIsOperand(s SlotDesc, arg core.Value) bool {
	if arg.ID == "" {
		return false
	}
	if s.Source == SlotWordRef {
		if es.reg == nil {
			return false
		}
		wi, err := core.AsWord(s.Token)
		if err != nil {
			return false
		}
		top, ok := es.reg.Defs.Top(wi.Name)
		return ok && top.ID == arg.ID
	}
	return s.Token.ID == arg.ID
}

// regionSourceOf translates one operand of the recorder's model into the
// descriptor's slot source. The two enumerations are deliberately separate
// types — the operand model describes an EMISSION, the slot source describes
// what OpCollect reads — but for the kinds a region can carry they correspond
// exactly, so the translation is a table rather than a reinterpretation.
//
// opDynScope and opDataScope decline, and the count is the argument: they are
// runtime dynamic-scope lookups by name, 12 and 0 occurrences in the corpus
// (region_desc.go's SlotSource doc records the same measurement declining a
// member for them). A bounded, countable decline beats an enumeration member
// nothing fills.
func regionSourceOf(op EmitOperand) (SlotSource, int, int, bool) {
	switch op.kind {
	case opConst:
		return SlotConst, op.idx, 0, true
	case opLocal:
		return SlotLocal, op.idx, 0, true
	case opType:
		return SlotType, op.idx, 0, true
	case opEvent:
		return SlotEvent, op.idx, op.resIdx, true
	default:
		return SlotNone, 0, 0, false
	}
}
