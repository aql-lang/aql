package eng

// The bytecode lowerer — the second half of the compile pass. EmitState
// (emit.go) RECORDS a classified event trace during the check run; the
// lowerer here LINEARISES that trace into a Program's instruction stream,
// walking a simulated stack of producing-event sequence numbers and emitting
// the pushes, SWAPs, calls, branches and loops the VM executes. Finalize
// (emit.go) is the bridge that drives it. Split out of emit.go purely for
// navigability; both halves are package eng and share no state beyond the
// emitEvent trace and the *EmitState pools the lowerer reads.

// lowerLoop lowers a counted/range for:
//
//	…step end start…  FOR_SETUP slot   ; pops start, end, step
//	head: FOR_NEXT -> end_pc           ; bind iterator or exit
//	…body…                             ; net ≤1 value per iteration
//	JMP -> head                        ; the back-edge
//	end_pc:
//
// The loop's stack contribution is variadic; the simulated stack
// carries one marker entry flagged in lw.variadic so only the
// program residual may absorb it. break/continue inside the body
// jump to end_pc / head via the lowerer's loop-context stack.
func (lw *lowerer) lowerLoop(ev *emitEvent) string {
	lp := ev.loop
	// Operand layout for FOR_SETUP: start on top, then end, then
	// step. start/step are consts (RecordLoop enforced); the end may
	// be a computed value already on top of the simulated stack — a
	// SWAP threads it under the step push.
	if lp.end.kind == opEvent {
		if lw.variadic[lp.end.idx] {
			return "loop results as a loop bound (Stage 2)"
		}
		if len(lw.vm) == 0 || !slotIs(lw.vm[len(lw.vm)-1], lp.end) {
			return "for: count is not on top of the stack"
		}
		lw.pushOperand(lp.step, lp.pos) // [end step]
		lw.emit(OpSwap, 0, lp.pos)      // [step end]
		lw.vm[len(lw.vm)-1], lw.vm[len(lw.vm)-2] = lw.vm[len(lw.vm)-2], lw.vm[len(lw.vm)-1]
	} else {
		lw.pushOperand(lp.step, lp.pos)
		lw.pushOperand(lp.end, lp.pos)
	}
	lw.pushOperand(lp.start, lp.pos)
	lw.emit(OpForSetup, lp.iterSlot, lp.pos)
	lw.vm = lw.vm[:len(lw.vm)-3] // start, end, step consumed
	head := len(*lw.code)
	fn := lw.emit(OpForNext, 0, lp.pos)
	endHoles := []int{}
	lw.loops = append(lw.loops, loopCtx{nextPC: head, endHoles: &endHoles})
	var out *emitOperand
	if lp.hasBodyOut {
		out = &lp.bodyOut
	}
	reason := lw.lowerFragment(lp.body, out, false, lp.pos)
	lw.loops = lw.loops[:len(lw.loops)-1]
	if reason != "" {
		return reason
	}
	lw.emit(OpJmp, head, lp.pos)
	endPC := len(*lw.code)
	(*lw.code)[fn].Arg = int32(endPC)
	for _, h := range endHoles {
		(*lw.code)[h].Arg = int32(endPC)
	}
	lw.vm = append(lw.vm, vmSlot{seq: ev.seq})
	lw.variadic[ev.seq] = true
	lw.note()
	return ""
}

// lowerer walks an event trace emitting instructions over a simulated
// stack of producing event seqs (-1 = const).
// loopCtx is the lowering context of one open loop: the FOR_NEXT pc
// (continue's target, and the back-edge) and the holes to patch with
// the loop's end pc (break's targets).
type loopCtx struct {
	nextPC   int
	endHoles *[]int
}

type lowerer struct {
	es       *EmitState
	p        *Program
	code     *[]Instr  // current emission target (main or one fn unit)
	debug    *[]SrcPos // 1:1 with code
	sigIdx   map[*Signature]int
	vm       []vmSlot
	variadic map[int]bool // loop seqs: N runtime values, not one
	promoted map[int]int  // value-def locals: producing event seq → frame local slot
	dead     map[int]bool // single-result value-defs referenced zero times: drop the result
	loops    []loopCtx
	maxDepth int
	// numLocals is the current unit's frame-local count, seeded from the unit's
	// recorded locals and bumped by allocLocal for spill temps (spillSeat). The
	// caller writes it back to the unit's NumLocals after lowering so the VM
	// allocates a frame large enough for the temps.
	numLocals int
}

// allocLocal reserves a fresh frame-local slot in the current unit (for a
// spill-and-reload temp). The unit's NumLocals is reconciled from lw.numLocals
// after lowering.
func (lw *lowerer) allocLocal() int {
	t := lw.numLocals
	lw.numLocals++
	return t
}

// spillSeat is the destination-driven (DDCG) fallback for an operand shape the
// cheap stack-only paths can't seat: a call's computed (event) operands sit on
// the contiguous top of the simulated stack (straight-line code leaves them
// there), possibly in a permutation or interleaved at sig positions with inert
// operands not yet pushed. It SPILLS each event operand to a fresh frame-local
// destination (OpStoreLocal, popping the top), then re-pushes EVERY operand in
// sig order (deepest first, so sig position 0 lands on top) — an event from its
// temp local (PUSH_LOCAL), an inert operand via pushOperand. This seats any
// shape without a 3-deep stack rotate. It declines (returning failMsg, the
// caller's original refusal wording) only when a top slot is NOT one of the
// call's event operands — a non-operand value is interleaved, which a local
// spill cannot reach (STORE_LOCAL pops the top). Promotion is sound: a local
// re-pushes the exact value in any order.
func (lw *lowerer) spillSeat(ops []emitOperand, results []int, n int, pos SrcPos, failMsg string) string {
	ne := len(results)
	if len(lw.vm) < ne {
		return failMsg
	}
	temp := make(map[int]int, ne) // ops index → spill-temp slot
	for k := 0; k < ne; k++ {
		slot := lw.vm[len(lw.vm)-1]
		oi := -1
		for _, i := range results {
			if _, seated := temp[i]; !seated && slotIs(slot, ops[i]) {
				oi = i
				break
			}
		}
		if oi < 0 {
			return failMsg
		}
		t := lw.allocLocal()
		lw.emit(OpStoreLocal, t, pos)
		lw.vm = lw.vm[:len(lw.vm)-1]
		temp[oi] = t
	}
	for i := n - 1; i >= 0; i-- {
		if t, ok := temp[i]; ok {
			lw.emit(OpPushLocal, t, pos)
			lw.vm = append(lw.vm, nonEventSlot)
		} else {
			lw.pushOperand(ops[i], pos)
		}
	}
	lw.note()
	return ""
}

// vmSlot is one entry on the lowerer's simulated operand stack: the producing
// event seq and which of that event's results it is (P5 multi-result lowering).
// A const / local / type / closure push uses seq=-1 (no producing event); a
// single-result event uses idx 0; a multi-result call pushes one slot per
// result, idx 0..N-1 deepest-first.
type vmSlot struct{ seq, idx int }

// nonEventSlot is the simulated-stack entry for a freshly pushed const / local
// / type / closure value — no producing event (seq -1).
var nonEventSlot = vmSlot{seq: -1}

// slotIs reports whether a simulated-stack slot is the value an event operand
// names: same producing event seq AND same result index.
func slotIs(slot vmSlot, op emitOperand) bool {
	return slot.seq == op.idx && slot.idx == op.resIdx
}

// pushOperand emits the push for a const, local, or type operand.
func (lw *lowerer) pushOperand(op emitOperand, pos SrcPos) {
	if op.kind == opClosure {
		// Push the captures (enclosing-scope operands), then OpPushClosure
		// pops them into the closure value. Net stack effect: +1.
		for i := range op.closureCaps {
			lw.pushOperand(op.closureCaps[i], pos)
		}
		lw.emit(OpPushClosure, op.closureUnit, pos)
		lw.vm = lw.vm[:len(lw.vm)-len(op.closureCaps)]
		lw.vm = append(lw.vm, nonEventSlot)
		lw.note()
		return
	}
	// pushOperand only ever materialises const/local/type operands (event
	// operands are already on the stack); opNone never reaches here.
	switch op.kind {
	case opLocal:
		lw.emit(OpPushLocal, op.idx, pos)
	case opType:
		lw.emit(OpPushType, op.idx, pos)
	default: // opConst
		lw.emit(OpPushConst, op.idx, pos)
	}
	lw.vm = append(lw.vm, nonEventSlot)
	lw.note()
}

func (lw *lowerer) note() {
	if len(lw.vm) > lw.maxDepth {
		lw.maxDepth = len(lw.vm)
	}
}

func (lw *lowerer) emit(op Opcode, arg int, pos SrcPos) int {
	*lw.code = append(*lw.code, Instr{Op: op, Arg: int32(arg)})
	*lw.debug = append(*lw.debug, pos)
	return len(*lw.code) - 1
}

// lowerEvents lowers a trace. scopeFloor is the closed-fragment rule:
// an operand produced by an event with seq <= scopeFloor lives in the
// enclosing scope, which Stage 2 branch fragments must not read.
func (lw *lowerer) lowerEvents(events []emitEvent, scopeFloor int) string {
	for i := range events {
		ev := &events[i]
		if scopeFloor > 0 {
			var crossed bool
			forEachOperand(ev, func(op emitOperand) {
				if op.kind == opEvent && op.idx <= scopeFloor {
					crossed = true
				}
			})
			if crossed {
				return "branch reads enclosing computation (Stage 3)"
			}
		}
		var reason string
		switch ev.kind {
		case evCall:
			reason = lw.lowerCall(ev)
		case evBranch:
			reason = lw.lowerBranch(ev)
		case evLoop:
			reason = lw.lowerLoop(ev)
		case evBreak:
			reason = lw.lowerBreak(ev)
		case evContinue:
			reason = lw.lowerContinue(ev)
		case evCallUser:
			reason = lw.lowerUserCall(ev)
		case evFallback:
			reason = lw.lowerFallback(ev)
		case evTrap:
			reason = lw.lowerTrap(ev)
		default:
			reason = "unknown event kind"
		}
		if reason != "" {
			return reason
		}
	}
	return ""
}

// forEachOperand calls fn for every enclosing-scope operand an event references
// — call / user-call / fallback args, a branch condition and arm outs, a loop's
// range and body out. A callback (rather than a returned slice) keeps the hot
// planning loops — the scopeFloor guard and planValueDefLocals — allocation-free.
func forEachOperand(ev *emitEvent, fn func(emitOperand)) {
	switch ev.kind {
	case evCall:
		for _, op := range ev.call.ops {
			fn(op)
		}
	case evLoop:
		fn(ev.loop.start)
		fn(ev.loop.end)
		fn(ev.loop.step)
		fn(ev.loop.bodyOut)
	case evBreak, evContinue, evTrap:
		// no operands
	case evCallUser:
		for _, op := range ev.uc.ops {
			fn(op)
		}
	case evFallback:
		for _, op := range ev.fb.ins {
			fn(op)
		}
	default: // evBranch
		// thenVal / elsVal are the value-arm operands — meaningful only when
		// thenIsVal / elsIsVal, and an opEvent only for the computed-arm shapes
		// (`if c (expr) e` / `if c [t] (expr)`); for any other branch each is the
		// zero opNone, which both consumers (the scopeFloor enclosing-scope guard
		// and the value-def ref count) skip. Including them keeps the operand set
		// complete so a computed-arm event is REFERENCE-COUNTED — otherwise
		// planValueDefLocals sees it as zero-referenced, marks it dead, and the
		// lowerer drops the value the computed-branch lowering needs on the stack.
		fn(ev.br.cond)
		fn(ev.br.condOut)
		fn(ev.br.thenOut)
		fn(ev.br.elsOut)
		fn(ev.br.thenVal)
		fn(ev.br.elsVal)
	}
}

// promoteOperand rewrites one ENCLOSING-scope operand: a single-result
// event reference whose producer was promoted to a value-def local becomes
// a local push. Inner-fragment result operands (a branch arm's / loop body's
// out) are never enclosing references and are left untouched.
func promoteOperand(op *emitOperand, promoted map[int]int) {
	if op.kind == opEvent && op.resIdx == 0 {
		if slot, ok := promoted[op.idx]; ok {
			*op = localOperand(slot)
		}
	}
}

// childFragments returns the body fragments a branch / loop event owns — the
// list-form condition, both `if` arms, and a loop body. Nil entries are kept so
// callers iterate a fixed shape and skip them; a non-branch/loop event owns
// none.
func childFragments(ev *emitEvent) []*EmitFragment {
	switch ev.kind {
	case evBranch:
		return []*EmitFragment{ev.br.condFrag, ev.br.then, ev.br.els}
	case evLoop:
		return []*EmitFragment{ev.loop.body}
	}
	return nil
}

// forEachFragmentOperand calls fn for every operand recorded INSIDE ev's body
// fragments, recursing through nested branches / loops. A reference whose
// producer lives OUTSIDE the fragment (the enclosing computation) crosses the
// fragment's scope floor; the only way the fragment can read it is as a frame
// local, so planValueDefLocals uses this walk to force such a producer's
// promotion.
func forEachFragmentOperand(ev *emitEvent, fn func(emitOperand)) {
	for _, frag := range childFragments(ev) {
		if frag == nil {
			continue
		}
		for i := range frag.events {
			fe := &frag.events[i]
			forEachOperand(fe, fn)
			forEachFragmentOperand(fe, fn)
		}
	}
}

// rewritePromotedRefs redirects an event's enclosing-scope operands (call /
// user-call / fallback args, a branch condition, a loop count) to value-def
// locals, then recurses into any body fragments so a cross-floor reference
// inside a branch arm / loop body is rewritten the same way. Only producers in
// the `promoted` map are touched — an intra-fragment result reference (a branch
// arm's own out, a fragment-local temp) is never promoted, so it is left as the
// event operand the closed-fragment lowering expects.
func rewritePromotedRefs(ev *emitEvent, promoted map[int]int) {
	switch ev.kind {
	case evCall:
		for i := range ev.call.ops {
			promoteOperand(&ev.call.ops[i], promoted)
		}
	case evCallUser:
		for i := range ev.uc.ops {
			promoteOperand(&ev.uc.ops[i], promoted)
		}
	case evFallback:
		for i := range ev.fb.ins {
			promoteOperand(&ev.fb.ins[i], promoted)
		}
	case evBranch:
		promoteOperand(&ev.br.cond, promoted)
		// A computed-arm value (`if c (expr) e` / `if c [t] (expr)`) is an
		// enclosing-scope reference too; rewrite it in lockstep with
		// forEachOperand counting it. (A promoted computed arm then refuses at
		// lowerBranch's stack-layout check and falls back — sound, never a wrong
		// result.)
		promoteOperand(&ev.br.thenVal, promoted)
		promoteOperand(&ev.br.elsVal, promoted)
	case evLoop:
		promoteOperand(&ev.loop.end, promoted)
	}
	for _, frag := range childFragments(ev) {
		if frag == nil {
			continue
		}
		for i := range frag.events {
			rewritePromotedRefs(&frag.events[i], promoted)
		}
	}
}

// planValueDefLocals decides which of a unit's top-level computed results are
// referenced more than once and so must be promoted to a frame local (the
// carrier-identity item's value-def locals). A single VM-stack copy of a
// COMPUTED value is consumed by its first use; a `def`-bound result used
// several times (`def a (make …) a eq a`) needs it stored once and re-pushed.
//
// References are counted across the unit's top-level event operands AND the
// extra references (the program residual, for frame 0). Only single-result
// native-call events are promotable — a multi-result word (dup) needs the
// carrier-identity DUP path, not a single local; branch/loop variadic results
// never reach here. Slots are allocated from the unit's local namespace.
// Returns seq → slot and rewrites every reference in place to a local operand.
// forceOrder names event seqs that MUST be promoted to a frame local even when
// referenced once — the program residual is not in seatable event*-literal*
// order (an event sits above a literal), so every residual event becomes a
// local and the reconciliation re-pushes the whole residual in exact order.
func (es *EmitState) planValueDefLocals(unit *emitUnit, events []emitEvent, extra []int, forceOrder map[int]bool) (map[int]int, map[int]bool) {
	refs := map[int]int{}
	fragRef := map[int]bool{} // referenced from INSIDE a branch/loop fragment
	for i := range events {
		forEachOperand(&events[i], func(op emitOperand) {
			if op.kind == opEvent && op.resIdx == 0 {
				refs[op.idx]++
			}
		})
		// A reference inside a body fragment crosses the fragment's scope floor:
		// the producer is only reachable there as a frame local, so count it AND
		// flag the producer for forced promotion regardless of the top-level
		// count (a single cross-floor read still needs the local).
		forEachFragmentOperand(&events[i], func(op emitOperand) {
			if op.kind == opEvent && op.resIdx == 0 {
				refs[op.idx]++
				fragRef[op.idx] = true
			}
		})
	}
	for _, seq := range extra {
		refs[seq]++
	}
	var promoted map[int]int
	var dead map[int]bool
	for i := range events {
		ev := &events[i]
		if ev.kind != evCall || ev.call.nout != 1 {
			continue
		}
		switch {
		case refs[ev.seq] == 0:
			// A single-result value-def bound to a name referenced zero times (a
			// dead binding, e.g. `def b (make C {…})` with b never used) — never the
			// program residual or a fragment read, both of which count toward refs.
			// The call still runs (side effects preserved), but its result is
			// discarded: lowerCall drops it rather than leaving it unconsumed on the
			// stack.
			if dead == nil {
				dead = map[int]bool{}
			}
			dead[ev.seq] = true
		case refs[ev.seq] >= 2 || es.eventInfo[ev.seq].valueDef || fragRef[ev.seq] || forceOrder[ev.seq]:
			// Promote to a frame slot (store now, re-push per reference) when the
			// result is referenced more than once, OR it is a NAMED value-def
			// (`def x (expr)`, marked via MarkValueDef), OR it is read from INSIDE a
			// branch/loop fragment (fragRef): a binding may be consumed in any order,
			// not just stack order — `def a (make…) def b (make…) a.x … b.x` reads a
			// (produced first, now deeper) before b — and a cross-floor fragment read
			// cannot reach the parent stack at all. The single-consume simulated
			// stack seats neither; a frame local re-pushes freely from any scope. The
			// extra STORE/PUSH for an in-order single use is harmless.
			if promoted == nil {
				promoted = map[int]int{}
			}
			if _, done := promoted[ev.seq]; !done {
				promoted[ev.seq] = unit.numLocals
				unit.numLocals++
			}
		default:
			// A single anonymous (non-def) use — stays on the simulated stack for
			// its one consumer; no local needed.
		}
	}
	if promoted != nil {
		for i := range events {
			rewritePromotedRefs(&events[i], promoted)
		}
	}
	return promoted, dead
}

// layoutMsgs holds the call-site-specific diagnostic wording for
// layoutOperands, so the shared engine can stay word/fn-agnostic while
// preserving each caller's exact reason strings.
// Each field is the refusal wording for one shape the spill fallback could not
// seat (a non-operand value interleaved on top of the simulated stack); when the
// spill succeeds — the common case — no message is used.
type layoutMsgs struct {
	loopResults  string // an operand is a variadic loop result
	resultNotTop string // the lone prior-result operand is not on top
	reorder      string // a single result operand needs reordering (>2 ops)
	shapeBeyond  string // operand count/shape the spill could not seat
	notAdjacent  string // two result operands not adjacent / not on top
}

// swapTop2 emits OpSwap and mirrors it on the simulated stack.
func (lw *lowerer) swapTop2(pos SrcPos) {
	lw.emit(OpSwap, 0, pos)
	lw.vm[len(lw.vm)-1], lw.vm[len(lw.vm)-2] = lw.vm[len(lw.vm)-2], lw.vm[len(lw.vm)-1]
}

// layoutOperands arranges call operands on the simulated stack so sig
// position 0 lands on top, emitting const/local/type pushes for inert
// operands and at most one SWAP to seat prior-result operands. It is the
// shared Stage-1/3 operand-shape engine for native calls (lowerCall) and
// user-fn calls (lowerUserCall) — they differ only in the diagnostic
// wording (msg) and the terminal call instruction each emits afterward.
// On success the len(ops) operands occupy the top slots in sig order and
// the caller pops them with its CALL_*; a non-empty return is the refusal.
func (lw *lowerer) layoutOperands(ops []emitOperand, pos SrcPos, msg layoutMsgs) string {
	n := len(ops)
	results := []int{}
	for i, op := range ops {
		if op.kind == opEvent {
			if lw.variadic[op.idx] {
				return msg.loopResults
			}
			results = append(results, i)
		}
	}
	// N-event already-in-layout fast path. When EVERY operand is a prior-result
	// (event) operand AND they already sit on top of the simulated stack in sig
	// order (ops[0] on top, ops[1] next-deeper, …), no reordering is needed —
	// verify the positions and accept, emitting nothing. This generalises the
	// case-1 (ri==0/n-1) and case-2 already-in-layout paths to any N: a computed
	// list `[gensym gensym gensym]` or a call over N computed args
	// (`is-between (a) (b) (c)`) leaves its results adjacent and in order, which
	// the old 3+-results default refused outright. Soundness: it only ACCEPTS a
	// layout already correct (slotIs verifies each slot), so it can never seat an
	// operand wrongly; a non-matching layout falls through to the switch (and its
	// existing refusals) unchanged.
	if n > 0 && len(results) == n && len(lw.vm) >= n {
		allInPlace := true
		for i := 0; i < n; i++ {
			if !slotIs(lw.vm[len(lw.vm)-1-i], ops[i]) {
				allInPlace = false
				break
			}
		}
		if allInPlace {
			return ""
		}
	}
	// N-event reverse fast path (N≥3). When every operand is a prior-result
	// (event) operand sitting in exact REVERSE sig order on top — ops[n-1] on
	// top, ops[0] deepest — one OpReverse seats them in sig order. This is the
	// common forward-call shape `f (a)(b)(c)`: the args evaluate left→right so
	// sig position 0 lands DEEPEST, but the call wants it on top. N=2 is the
	// case-2 SWAP below; N≥3 needed the 3-deep rotate the VM lacked. Like the
	// in-layout fast path it only ACCEPTS an exactly-recognised layout (slotIs
	// verifies every slot), so it can never seat an operand wrongly.
	if n >= 3 && len(results) == n && len(lw.vm) >= n {
		allReversed := true
		for i := 0; i < n; i++ {
			if !slotIs(lw.vm[len(lw.vm)-1-i], ops[n-1-i]) {
				allReversed = false
				break
			}
		}
		if allReversed {
			lw.emit(OpReverse, n, pos)
			for a, b := len(lw.vm)-n, len(lw.vm)-1; a < b; a, b = a+1, b-1 {
				lw.vm[a], lw.vm[b] = lw.vm[b], lw.vm[a]
			}
			return ""
		}
	}
	switch len(results) {
	case 0:
		// Push consts/locals deepest-first so sig position 0 lands on top.
		for i := n - 1; i >= 0; i-- {
			lw.pushOperand(ops[i], pos)
		}
	case 1:
		ri := results[0]
		if len(lw.vm) == 0 || !slotIs(lw.vm[len(lw.vm)-1], ops[ri]) {
			return msg.resultNotTop
		}
		switch {
		case ri == n-1:
			// The lone prior-result operand is the DEEPEST sig position: push
			// the const/local operands above it deepest-first, so sig 0 lands
			// on top. No reordering needed.
			for i := n - 1; i >= 0; i-- {
				if i == ri {
					continue
				}
				lw.pushOperand(ops[i], pos)
			}
		case ri == 0:
			// The result operand is sig position 0 — it must end on TOP, with
			// the const/local operands below it. Push each deeper operand above
			// the result then SWAP the result back to the top, settling that
			// operand into its (deeper) place; repeat sig(n-1) down to sig 1.
			// n==2 is the single push+swap; n>2 chains it (e.g. a computed
			// receiver `setpath (make…) "k" v`, sig 0 = receiver on top).
			for j := n - 1; j >= 1; j-- {
				lw.pushOperand(ops[j], pos)
				lw.swapTop2(pos)
			}
		default:
			// The result operand sits in a MIDDLE sig position (0 < ri < n-1):
			// the cheap stack paths can't seat it (a 3-deep rotate). Spill the
			// event operand to a frame-local destination and re-push in sig order.
			return lw.spillSeat(ops, results, n, pos, msg.reorder)
		}
	case 2:
		if n == 2 && len(lw.vm) >= 2 {
			top, below := lw.vm[len(lw.vm)-1], lw.vm[len(lw.vm)-2]
			switch {
			case slotIs(top, ops[0]) && slotIs(below, ops[1]):
				return "" // already in layout
			case slotIs(top, ops[1]) && slotIs(below, ops[0]):
				lw.swapTop2(pos)
				return ""
			}
		}
		// Two events with inert operands (n>2), or two events not cleanly adjacent
		// on top: spill the events to frame-local destinations and re-push all
		// operands in sig order.
		return lw.spillSeat(ops, results, n, pos, msg.notAdjacent)
	default:
		// Three or more event operands not already in (forward or reverse) sig
		// order: spill them to frame-local destinations and re-push in sig order.
		return lw.spillSeat(ops, results, n, pos, msg.shapeBeyond)
	}
	return ""
}

// seatMsgs carries a seatResults caller's exact refusal wording, so the shared
// seat primitive stays caller-agnostic while preserving each site's reasons.
type seatMsgs struct {
	variadic     string // an event operand is a variadic loop result (when rejected)
	aboveLiteral string // an event operand sits above an already-pushed inert tail
	reordered    string // an event operand is not the next simulated-stack slot
	unconsumed   string // simulated-stack results remain after seating all operands
}

// seatResults arranges a sequence of result operands (bottom→top) as the final
// stack. Each event operand must already sit on the simulated stack in order —
// left there by its own event — and inert operands (const / local / type) are
// pushed as a trailing tail above the last event result. It is the shared core
// of the program-residual reconciliation (Finalize) and the fn-unit RET
// reconciliation (reconcileResults). rejectVariadic refuses a variadic (loop)
// event result: a fn body may not return one (Stage 3), though the program
// residual may absorb it. msgs supplies the caller's refusal wording.
func (lw *lowerer) seatResults(ops []emitOperand, rejectVariadic bool, msgs seatMsgs, pos SrcPos) string {
	vi := 0
	var tail []emitOperand
	for _, op := range ops {
		if op.kind == opEvent {
			if rejectVariadic && lw.variadic[op.idx] {
				return msgs.variadic
			}
			if len(tail) > 0 {
				return msgs.aboveLiteral
			}
			if vi >= len(lw.vm) || !slotIs(lw.vm[vi], op) {
				return msgs.reordered
			}
			vi++
			continue
		}
		tail = append(tail, op)
	}
	if vi != len(lw.vm) {
		return msgs.unconsumed
	}
	for _, op := range tail {
		lw.pushOperand(op, pos)
	}
	return ""
}

// reconcileResults arranges a unit's N result operands (bottom→top) as the
// final stack, ready for a RET. who prefixes the refusal reason ("fn name").
// This is the fn-unit caller of the shared seatResults primitive — it rejects a
// variadic loop result (a fn body may not return one in Stage 3), the one way it
// differs from Finalize's program-residual reconciliation.
func (lw *lowerer) reconcileResults(ops []emitOperand, who string, pos SrcPos) string {
	extra := who + ": body leaves extra values (Stage 3 lowers in-order results)"
	return lw.seatResults(ops, true, seatMsgs{
		variadic:     who + ": result is a variadic loop value (Stage 3)",
		aboveLiteral: who + ": result above a literal (Stage 3)",
		reordered:    extra,
		unconsumed:   extra,
	}, pos)
}

func (lw *lowerer) lowerCall(ev *emitEvent) string {
	c := &ev.call
	n := len(c.ops)
	if reason := lw.layoutOperands(c.ops, c.pos, layoutMsgs{
		loopResults:  "consumes loop results (Stage 2 loops only feed the program residual)",
		resultNotTop: "stack discipline: result operand of " + c.word + " is not on top",
		reorder:      "operand shape at " + c.word + " needs reordering beyond Stage 1",
		shapeBeyond:  "operand shape at " + c.word + " beyond Stage 1",
		notAdjacent:  "stack discipline: operands of " + c.word + " not adjacent on top",
	}); reason != "" {
		return reason
	}
	if c.makeList {
		// Assemble the n laid-out operands into a list (a computed list literal,
		// `[1 add 2]`). No sig, no dispatch — OpMakeList pops the n and pushes one.
		lw.emit(OpMakeList, n, c.pos)
	} else if c.makeMap {
		// Assemble the n laid-out VALUE operands into a map (a computed make
		// body, `make Outer {i:(make Inner …)}`); the keys ride in MakeMaps.
		mi := len(lw.p.MakeMaps)
		lw.p.MakeMaps = append(lw.p.MakeMaps, MakeMapSpec{Keys: c.mapKeys, Implicit: c.mapImpl})
		lw.emit(OpMakeMap, mi, c.pos)
	} else if c.poly {
		// Runtime-matched dispatch: no baked sig, the VM re-matches over the
		// word's signatures against the n stack values.
		pi := len(lw.p.PolyRefs)
		lw.p.PolyRefs = append(lw.p.PolyRefs, PolyRef{Word: c.word, Arity: n})
		lw.emit(OpCallNativePoly, pi, c.pos)
	} else {
		si, ok := lw.sigIdx[c.sig]
		if !ok {
			lw.p.Sigs = append(lw.p.Sigs, SigRef{Word: c.word, Sig: c.sig})
			si = len(lw.p.Sigs) - 1
			lw.sigIdx[c.sig] = si
		}
		lw.emit(OpCallNative, si, c.pos)
	}
	lw.vm = lw.vm[:len(lw.vm)-n]
	// A value-def local: this single result is referenced more than once, so
	// store it into a frame slot now and re-push it per reference (the
	// references were rewritten to local operands). The promotion pre-pass
	// only marks single-result events, so nothing is left on the stack.
	if slot, ok := lw.promoted[ev.seq]; ok {
		lw.emit(OpStoreLocal, slot, c.pos)
		lw.note()
		return ""
	}
	if lw.dead[ev.seq] {
		// A single-result value-def referenced zero times: the call ran for its
		// side effects, but the result is discarded — drop it so it is not left
		// unconsumed on the stack (planValueDefLocals marks only nout==1 events).
		lw.emit(OpDrop, 0, c.pos)
		lw.note()
		return ""
	}
	// Push one simulated slot per result the call leaves (P5): 0 for a
	// side-effect word, N for a multi-result word — idx 0..N-1 deepest-first,
	// matching the VM's append of the handler's results (results[N-1] on top).
	for i := 0; i < c.nout; i++ {
		lw.vm = append(lw.vm, vmSlot{seq: ev.seq, idx: i})
	}
	lw.note()
	return ""
}

// lowerFragment lowers a closed body: a fresh stack scope that must
// end as exactly [out] (out non-nil), or empty (out nil — a net-0 or
// diverging fragment; a diverging fragment's terminator already
// emitted its jump, so whatever its scope holds is unreachable and
// ignored). Restores the parent scope afterwards.
func (lw *lowerer) lowerFragment(frag *EmitFragment, out *emitOperand, allowVariadic bool, pos SrcPos) string {
	parent := lw.vm
	lw.vm = nil
	if reason := lw.lowerEvents(frag.events, frag.startSeq); reason != "" {
		return reason
	}
	switch {
	case fragDiverges(frag):
		// Control left via break/continue; the residual scope is
		// unreachable.
	case out == nil:
		if len(lw.vm) != 0 {
			return "body leaves extra values (Stage 2 lowers single-result bodies)"
		}
	case out.kind == opEvent:
		if lw.variadic[out.idx] && !allowVariadic {
			// The fragment's result is itself a VARIADIC (0-or-1) event — a
			// nested variadic `if` (e.g. a no-default `case` chain). A BRANCH ARM
			// may carry it (allowVariadic — the parent if propagates the
			// variadic-ness up to its own merge), but a loop body / condition may
			// not: they need a definite single value per iteration.
			return "loop results as a branch/body result (Stage 2)"
		}
		if len(lw.vm) != 1 || !slotIs(lw.vm[0], *out) {
			return "branch leaves extra values (Stage 2 lowers single-result branches)"
		}
	default:
		if len(lw.vm) != 0 {
			return "branch leaves extra values (Stage 2 lowers single-result branches)"
		}
		lw.pushOperand(*out, pos)
		lw.vm = lw.vm[:len(lw.vm)-1] // pushOperand tracked it; the scope owns the count
	}
	lw.vm = parent
	return ""
}

// lowerBreak / lowerContinue: flow-control terminators inside a loop
// body fragment. break jumps to the loop end (hole patched by
// lowerLoop); continue jumps back to FOR_NEXT — a back-edge the VM
// accepts because it targets the loop header.
func (lw *lowerer) lowerBreak(ev *emitEvent) string {
	if len(lw.loops) == 0 {
		return "break outside a compiled loop (Stage 2)"
	}
	h := lw.emit(OpJmp, 0, ev.call.pos)
	ctx := lw.loops[len(lw.loops)-1]
	*ctx.endHoles = append(*ctx.endHoles, h)
	return ""
}

func (lw *lowerer) lowerContinue(ev *emitEvent) string {
	if len(lw.loops) == 0 {
		return "continue outside a compiled loop (Stage 2)"
	}
	lw.emit(OpJmp, lw.loops[len(lw.loops)-1].nextPC, ev.call.pos)
	return ""
}

// lowerTrap emits the terminal OpTrap for a check-mode-suppressed runtime error,
// pooling its TrapSpec. Execution never continues past it.
func (lw *lowerer) lowerTrap(ev *emitEvent) string {
	idx := len(lw.p.Traps)
	lw.p.Traps = append(lw.p.Traps, ev.trap.spec)
	lw.emit(OpTrap, idx, ev.trap.pos)
	return ""
}

// lowerUserCall pushes the args (sig position 0 on top — frame
// locals bind by pop order) and calls or tail-calls the unit. A tail
// call replaces the frame: control never returns here, so nothing is
// pushed to the simulated stack (the marking pass already cleared
// the consumer's out expectation).
func (lw *lowerer) lowerUserCall(ev *emitEvent) string {
	uc := &ev.uc
	n := len(uc.ops)
	// Stage 3 operand shape: all args const/local (results-on-stack
	// shapes work when the single result operand is on top, mirroring
	// lowerCall's n<=2 rules — keep it simple: allow one trailing
	// result operand at position 0). Shared with lowerCall via
	// layoutOperands; only the diagnostic wording differs.
	if reason := lw.layoutOperands(uc.ops, uc.pos, layoutMsgs{
		loopResults:  "loop results as fn args (Stage 3)",
		resultNotTop: "stack discipline: fn arg result is not on top (call of " + lw.es.fnRecs[uc.unit].name + ")",
		reorder:      "fn arg shape needs reordering beyond Stage 3",
		shapeBeyond:  "fn arg shape beyond Stage 3",
		notAdjacent:  "stack discipline: fn args not adjacent on top",
	}); reason != "" {
		return reason
	}
	if uc.tail {
		lw.emit(OpTailCallUser, uc.unit, uc.pos)
		lw.vm = lw.vm[:len(lw.vm)-n]
		return ""
	}
	lw.emit(OpCallUser, uc.unit, uc.pos)
	lw.vm = lw.vm[:len(lw.vm)-n]
	// Push one simulated slot per result the unit returns (P5 multi-result):
	// 0 for a 0-return fn, N for a multi-return fn — idx 0..N-1 deepest-first,
	// matching the order the VM's CALL_USER leaves the unit's residual.
	for i := 0; i < uc.nout; i++ {
		lw.vm = append(lw.vm, vmSlot{seq: ev.seq, idx: i})
	}
	lw.note()
	return ""
}

// lowerFallback emits OpFallback. A fully-baked island (no threaded
// inputs) just runs its span; a single threaded input is the computed
// data arg (a "computed receiver" like `(iota 5) each […]`): its
// runtime value must sit on top of the operand stack when OpFallback
// runs, so the VM can preload it onto the island and back-fill the
// deepest sig position. A result operand is already on top; a
// const/local operand is pushed first. The island's single residual
// lands on the simulated stack as this event's product, read by a
// downstream consumer (or the program residual) like any computed
// value. Multiple threaded inputs are a documented follow-on.
func (lw *lowerer) lowerFallback(ev *emitEvent) string {
	fb := &ev.fb
	switch len(fb.ins) {
	case 0:
		lw.emit(OpFallback, fb.spanIdx, fb.pos)
		lw.vm = append(lw.vm, vmSlot{seq: ev.seq})
		lw.note()
		return ""
	case 1:
		op := fb.ins[0]
		if op.kind == opEvent {
			if lw.variadic[op.idx] {
				return "fallback threads a loop result (Stage 5 follow-on)"
			}
			// The computed value is already on top of the simulated
			// stack; OpFallback consumes it as the threaded input.
			if len(lw.vm) == 0 || !slotIs(lw.vm[len(lw.vm)-1], op) {
				return "stack discipline: fallback input is not on top"
			}
		} else {
			// A const / local / type input: materialise it on top first.
			lw.pushOperand(op, fb.pos)
		}
		lw.emit(OpFallback, fb.spanIdx, fb.pos)
		lw.vm = lw.vm[:len(lw.vm)-1]
		lw.vm = append(lw.vm, vmSlot{seq: ev.seq})
		lw.note()
		return ""
	default:
		return "fallback island with multiple threaded inputs (Stage 5 follow-on)"
	}
}

// markTailCalls rewrites tail-position user calls in a fn body
// fragment: the final event when it produces the body's result, and
// recursively the final event of branch arms whose result is that
// arm's own trailing call. Marked calls lower as TAIL_CALL_USER and
// count as divergence (control leaves via the callee's eventual RET),
// so the arm/body contributes no merge value. A statically-taken
// (const-condition) branch inlines only its taken arm, so a tail call
// there is the body's tail call too and is marked through the same
// recursion — otherwise the dynamic-condition path would honour the
// O(1)-frame guarantee while the const-condition path silently lost it.
func markTailCalls(frag *EmitFragment, out *emitOperand, hasOut bool) (stillHasOut bool) {
	if frag == nil || len(frag.events) == 0 || !hasOut || out.kind != opEvent {
		return hasOut
	}
	last := &frag.events[len(frag.events)-1]
	switch last.kind {
	case evCallUser:
		if last.seq == out.idx {
			last.uc.tail = true
			return false
		}
	case evBranch:
		if last.seq != out.idx {
			return hasOut
		}
		if last.br.constCond != nil {
			// Statically-taken branch: lowerBranch inlines ONLY the taken
			// (then) arm and its result IS the branch's result, so a tail
			// call there is the body's tail call. Mark it; if the arm fully
			// tail-diverges, the whole branch does (lowerBranch's const-cond
			// path then emits no merge slot, matching hasThenOut=false).
			if last.br.hasThenOut {
				last.br.hasThenOut = markTailCalls(last.br.then, &last.br.thenOut, true)
				if !last.br.hasThenOut {
					return false
				}
			}
			return hasOut
		}
		if last.br.hasElse {
			if last.br.hasThenOut {
				last.br.hasThenOut = markTailCalls(last.br.then, &last.br.thenOut, true)
			}
			if last.br.hasElsOut {
				last.br.hasElsOut = markTailCalls(last.br.els, &last.br.elsOut, true)
			}
			// The branch still merges normally for non-tail arms; if
			// EVERY arm tail-calls, control never reaches the merge —
			// the whole body diverges.
			if !last.br.hasThenOut && !last.br.hasElsOut {
				return false
			}
		}
	}
	return hasOut
}

func (lw *lowerer) lowerBranch(ev *emitEvent) string {
	br := ev.br
	if br.constCond != nil {
		// Statically-taken branch: inline the taken fragment (always a body in
		// const-cond form — never a value-then).
		if reason := lw.lowerArm(br.thenArm(), br.thenVal, br.then, &br.thenOut, true, br.pos); reason != "" {
			return reason
		}
		if br.hasThenOut {
			lw.vm = append(lw.vm, vmSlot{seq: ev.seq})
			if lw.variadic[br.thenOut.idx] {
				lw.variadic[ev.seq] = true // taken arm was itself variadic
			}
			lw.note()
		}
		return ""
	}
	if br.thenComputed && br.elsComputed {
		// `if (c) (a) (b)` — BOTH arms eagerly computed: the three events stack
		// as [cond, then, else]; select one with OpReverse + JMP_IF_FALSE.
		return lw.lowerBothComputed(ev)
	}
	if br.elsComputed || br.thenComputed {
		// One arm's value is an eagerly-computed event (`if c [t] (expr)` or
		// `if c (expr) e`). It is the last thing evaluated before the branch, so
		// it sits on TOP of the sim stack; the OTHER (non-eager) arm DROPs it and
		// produces its own value, so it must net a value here.
		eager, nonEagerHasOut := br.elsVal, br.hasThenOut
		if br.thenComputed {
			eager, nonEagerHasOut = br.thenVal, br.hasElsOut
		}
		if !nonEagerHasOut {
			return "if: computed-branch non-eager arm diverges (Stage 2)"
		}
		if len(lw.vm) == 0 || !slotIs(lw.vm[len(lw.vm)-1], eager) {
			return "if: computed-branch eager value not on top (Stage 2)"
		}
		jf, reason := lw.lowerComputedCond(br)
		if reason != "" {
			return reason
		}
		return lw.lowerComputedBranch(ev, jf)
	}
	// Condition on top of stack: a pre-evaluated value, or an inline
	// list-form condition body lowered here (it nets one Boolean).
	switch {
	case br.condFrag != nil:
		if reason := lw.lowerFragment(br.condFrag, &br.condOut, false, br.pos); reason != "" {
			return reason
		}
		// The Boolean is on the runtime stack but not in the parent
		// scope's sim — JMP_IF_FALSE consumes it net-zero.
		jf := lw.emit(OpJmpIfFalse, 0, br.pos)
		return lw.lowerArms(ev, jf)
	case br.cond.kind == opEvent:
		if lw.variadic[br.cond.idx] {
			return "loop results as a condition (Stage 2)"
		}
		if len(lw.vm) == 0 || !slotIs(lw.vm[len(lw.vm)-1], br.cond) {
			return "if: condition is not on top of the stack"
		}
		jf := lw.emit(OpJmpIfFalse, 0, br.pos)
		lw.vm = lw.vm[:len(lw.vm)-1] // cond consumed
		return lw.lowerArms(ev, jf)
	default:
		lw.pushOperand(br.cond, br.pos)
		jf := lw.emit(OpJmpIfFalse, 0, br.pos)
		lw.vm = lw.vm[:len(lw.vm)-1]
		return lw.lowerArms(ev, jf)
	}
}

// lowerArm emits one if-arm as a single (or zero) merge value: a plain value
// operand is pushed; a body fragment is lowered with its out (nil when the arm
// nets nothing or diverges — it still runs). allowVariadic passes through to
// lowerFragment (the merge of two body arms may be variadic; a computed-branch
// arm may not). Shared by lowerArms, lowerBranch's const-cond inline, and the
// non-eager arm of lowerComputedBranch.
func (lw *lowerer) lowerArm(kind armKind, val emitOperand, frag *EmitFragment, out *emitOperand, allowVariadic bool, pos SrcPos) string {
	switch kind {
	case armValue:
		// Push the literal/local/type operand as the arm's single result
		// (pushOperand tracked it; the merge slot owns the count).
		lw.pushOperand(val, pos)
		lw.vm = lw.vm[:len(lw.vm)-1]
		return ""
	case armBodyOut:
		return lw.lowerFragment(frag, out, allowVariadic, pos)
	default: // armBodyVoid — a 0-value / diverging body
		return lw.lowerFragment(frag, nil, allowVariadic, pos)
	}
}

// lowerArms emits the then/else arms after the JMP_IF_FALSE at jf.
// A diverging arm's terminator already jumped out of the construct,
// so it needs no jump-to-end and contributes no value; the merge
// point then carries only the surviving arm's value. The 2-arg form
// (no else) merges with 0-or-1 values — a VARIADIC result.
func (lw *lowerer) lowerArms(ev *emitEvent, jf int) string {
	br := ev.br
	if reason := lw.lowerArm(br.thenArm(), br.thenVal, br.then, &br.thenOut, true, br.pos); reason != "" {
		return reason
	}
	if !br.hasElse {
		// 2-arg if: false path jumps straight to the merge.
		(*lw.code)[jf].Arg = int32(len(*lw.code))
		// A value-producing then yields a VARIADIC result (the value on true,
		// nothing on false) — only the program residual may absorb it. A
		// 0-value / diverging then (raise/set/break) produces 0 values on both
		// paths: a statement guard with no merge slot.
		if br.hasThenOut {
			lw.vm = append(lw.vm, vmSlot{seq: ev.seq})
			lw.variadic[ev.seq] = true
			lw.note()
		}
		return ""
	}
	jend := -1
	if br.thenIsVal || !fragDiverges(br.then) {
		jend = lw.emit(OpJmp, 0, br.pos)
	}
	(*lw.code)[jf].Arg = int32(len(*lw.code))
	if reason := lw.lowerArm(br.elseArm(), br.elsVal, br.els, &br.elsOut, true, br.pos); reason != "" {
		return reason
	}
	if jend >= 0 {
		(*lw.code)[jend].Arg = int32(len(*lw.code))
	}
	if br.hasThenOut || br.hasElsOut {
		lw.vm = append(lw.vm, vmSlot{seq: ev.seq})
		// Mismatched arm value-counts: one arm nets a value, the other nets 0
		// WITHOUT diverging (it reaches the merge with nothing) → the merge
		// carries 0-or-1 values, a VARIADIC result only the program residual may
		// absorb (`if cond [99] []`, `if cond [raise] [99]`). A DIVERGING 0-arm
		// never reaches the merge, so the surviving arm's value is
		// unconditional — non-variadic, left as-is.
		if br.hasThenOut != br.hasElsOut {
			// Use the DEEP divergence check: an arm whose result is a
			// fully-diverging nested branch (a const-condition branch whose
			// taken arm tail-calls) leaves via that callee's RET and never
			// reaches the merge, so the surviving arm's value is unconditional
			// — non-variadic. Shallow fragDiverges misses the nested-branch
			// shape and would over-mark the merge variadic.
			thenDiv := fragDivergesDeep(br.then)
			elsDiv := br.els != nil && fragDivergesDeep(br.els)
			if (!br.hasThenOut && !thenDiv) || (!br.hasElsOut && !elsDiv) {
				lw.variadic[ev.seq] = true
			}
		}
		// Variadic ARM: an arm whose own result is a nested variadic (0-or-1)
		// event — a no-default `case`'s inner `if` chain — makes this merge
		// variadic too, so the 0-or-1 propagates up to the residual.
		if (br.hasThenOut && lw.variadic[br.thenOut.idx]) ||
			(br.hasElsOut && lw.variadic[br.elsOut.idx]) {
			lw.variadic[ev.seq] = true
		}
		lw.note()
	}
	return ""
}

// lowerBothComputed lowers `if (c) (a) (b)` where BOTH arms are eagerly-computed
// events. Entry sim/runtime stack is [.., cond, thenVal, elsVal] (elsVal on top).
// Both arms ran already (paren args evaluate eagerly — faithful to the
// interpreter, which also evaluates both), so this only SELECTS one: it rotates
// the cond to the top (OpReverse 3 → [elsVal, thenVal, cond]), branches, and
// DROPs the unselected value on each path. The merge is a single (non-variadic)
// slot.
func (lw *lowerer) lowerBothComputed(ev *emitEvent) string {
	br := ev.br
	if len(lw.vm) < 3 ||
		!slotIs(lw.vm[len(lw.vm)-1], br.elsVal) ||
		!slotIs(lw.vm[len(lw.vm)-2], br.thenVal) ||
		!slotIs(lw.vm[len(lw.vm)-3], br.cond) {
		return "if: both-computed stack layout (Stage 2)"
	}
	if lw.variadic[br.thenVal.idx] || lw.variadic[br.elsVal.idx] {
		return "if: both-computed arm is a variadic loop value (Stage 2)"
	}
	// Reverse the top three so the cond lands on top: [elsVal, thenVal, cond].
	lw.emit(OpReverse, 3, br.pos)
	n := len(lw.vm)
	lw.vm[n-3], lw.vm[n-1] = lw.vm[n-1], lw.vm[n-3]
	jf := lw.emit(OpJmpIfFalse, 0, br.pos) // pop cond → [elsVal, thenVal]
	// TRUE/fall-through: the result is thenVal (now on top), so drop elsVal
	// beneath it: SWAP then DROP → [thenVal].
	lw.emit(OpSwap, 0, br.pos)
	lw.emit(OpDrop, 0, br.pos)
	jend := lw.emit(OpJmp, 0, br.pos)
	// FALSE: stack is [elsVal, thenVal] (thenVal on top); the result is elsVal,
	// so drop thenVal → [elsVal].
	(*lw.code)[jf].Arg = int32(len(*lw.code))
	lw.emit(OpDrop, 0, br.pos)
	(*lw.code)[jend].Arg = int32(len(*lw.code))
	// Sim: the three input slots (cond/then/else) collapse to one merge slot.
	lw.vm = lw.vm[:len(lw.vm)-3]
	lw.vm = append(lw.vm, vmSlot{seq: ev.seq})
	lw.note()
	return ""
}

// lowerComputedCond materialises a computed-arm branch's condition as a Boolean
// on TOP of the already-on-stack eager arm value and emits JMP_IF_FALSE
// (consuming it), returning the jf pc. The eager value is the top sim slot on
// entry and remains so on return. It handles the three condition shapes the
// recorder admits for a computed arm:
//
//   - a list-form condition body (`if [x gt 0] (expr) e`): lowered inline above
//     the eager value, netting one Boolean (not tracked in the parent sim);
//   - an event cond (`if (x eq 0) (expr) e`): the cond event sits just BELOW the
//     eager value — SWAP it to the top;
//   - a const / local / type cond (`if flag (expr) e`): pushed above the eager.
func (lw *lowerer) lowerComputedCond(br *emitBranch) (int, string) {
	switch {
	case br.condFrag != nil:
		if reason := lw.lowerFragment(br.condFrag, &br.condOut, false, br.pos); reason != "" {
			return 0, reason
		}
		return lw.emit(OpJmpIfFalse, 0, br.pos), ""
	case br.cond.kind == opEvent:
		if len(lw.vm) < 2 || !slotIs(lw.vm[len(lw.vm)-2], br.cond) {
			return 0, "if: computed-branch condition not below the eager value (Stage 2)"
		}
		lw.swapTop2(br.pos)
		jf := lw.emit(OpJmpIfFalse, 0, br.pos)
		lw.vm = lw.vm[:len(lw.vm)-1] // cond consumed; eager value stays on top
		return jf, ""
	default:
		lw.pushOperand(br.cond, br.pos)
		jf := lw.emit(OpJmpIfFalse, 0, br.pos)
		lw.vm = lw.vm[:len(lw.vm)-1] // cond consumed; eager value stays on top
		return jf, ""
	}
}

// lowerComputedBranch lowers an `if` whose THEN or ELSE value is an eagerly-
// computed event, after the JMP_IF_FALSE at jf. Entry sim/runtime stack is
// [.., eager] (the cond was just consumed; the eager arm value remains). The
// eager value is the result of ITS path; the OTHER (non-eager) arm DROPs it and
// produces its own value (a plain value push or a body fragment). Both arms net
// exactly one value, so the merge is a single (non-variadic) slot.
//
//   - computed ELSE (`if c [t] (expr)`): the eager value is the FALSE-path
//     result; the TRUE (fall-through) path drops it and runs the then arm.
//   - computed THEN (`if c (expr) e`): the eager value is the TRUE-path result;
//     the FALSE (jump-target) path drops it and runs the else arm.
func (lw *lowerer) lowerComputedBranch(ev *emitEvent, jf int) string {
	br := ev.br
	if br.thenComputed {
		// TRUE/fall-through keeps the eager then value; jump over the else arm.
		jend := lw.emit(OpJmp, 0, br.pos)
		(*lw.code)[jf].Arg = int32(len(*lw.code)) // FALSE lands here
		lw.emit(OpDrop, 0, br.pos)                // discard the eager then value
		lw.vm = lw.vm[:len(lw.vm)-1]
		if reason := lw.lowerArm(br.elseArm(), br.elsVal, br.els, &br.elsOut, false, br.pos); reason != "" {
			return reason
		}
		(*lw.code)[jend].Arg = int32(len(*lw.code))
	} else {
		// TRUE/fall-through drops the eager else value and runs the then arm;
		// the FALSE path falls through with the eager value intact.
		lw.emit(OpDrop, 0, br.pos)
		lw.vm = lw.vm[:len(lw.vm)-1]
		if reason := lw.lowerArm(br.thenArm(), br.thenVal, br.then, &br.thenOut, false, br.pos); reason != "" {
			return reason
		}
		jend := lw.emit(OpJmp, 0, br.pos)
		(*lw.code)[jf].Arg = int32(len(*lw.code)) // FALSE lands here, eager value intact
		(*lw.code)[jend].Arg = int32(len(*lw.code))
	}
	lw.vm = append(lw.vm, vmSlot{seq: ev.seq})
	lw.note()
	return ""
}
