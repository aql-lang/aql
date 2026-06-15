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
	lp := &ev.loop
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
	reason := lw.lowerFragment(lp.body, out, lp.pos)
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
	loops    []loopCtx
	maxDepth int
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
			for _, op := range collectOperands(ev) {
				if op.kind == opEvent && op.idx <= scopeFloor {
					return "branch reads enclosing computation (Stage 3)"
				}
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
		default:
			reason = "unknown event kind"
		}
		if reason != "" {
			return reason
		}
	}
	return ""
}

func collectOperands(ev *emitEvent) []emitOperand {
	switch ev.kind {
	case evCall:
		return ev.call.ops
	case evLoop:
		return []emitOperand{ev.loop.start, ev.loop.end, ev.loop.step, ev.loop.bodyOut}
	case evBreak, evContinue:
		return nil
	case evCallUser:
		return ev.uc.ops
	case evFallback:
		return ev.fb.ins
	}
	return []emitOperand{ev.br.cond, ev.br.condOut, ev.br.thenOut, ev.br.elsOut}
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

// rewritePromotedRefs redirects an event's enclosing-scope operands (call /
// user-call / fallback args, a branch condition, a loop count) to value-def
// locals. Only enclosing references are rewritten — the producing event of a
// promoted value lives at the same scope, so its consumers do too.
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
	case evLoop:
		promoteOperand(&ev.loop.end, promoted)
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
func (es *EmitState) planValueDefLocals(unit *emitUnit, events []emitEvent, extra []int) map[int]int {
	refs := map[int]int{}
	for i := range events {
		for _, op := range collectOperands(&events[i]) {
			if op.kind == opEvent && op.resIdx == 0 {
				refs[op.idx]++
			}
		}
	}
	for _, seq := range extra {
		refs[seq]++
	}
	var promoted map[int]int
	for i := range events {
		ev := &events[i]
		if ev.kind != evCall || ev.call.nout != 1 {
			continue
		}
		if refs[ev.seq] <= 1 {
			continue
		}
		if promoted == nil {
			promoted = map[int]int{}
		}
		if _, done := promoted[ev.seq]; !done {
			promoted[ev.seq] = unit.numLocals
			unit.numLocals++
		}
	}
	if promoted == nil {
		return nil
	}
	for i := range events {
		rewritePromotedRefs(&events[i], promoted)
	}
	return promoted
}

// layoutMsgs holds the call-site-specific diagnostic wording for
// layoutOperands, so the shared engine can stay word/fn-agnostic while
// preserving each caller's exact reason strings.
type layoutMsgs struct {
	loopResults  string // an operand is a variadic loop result
	resultNotTop string // the lone prior-result operand is not on top
	reorder      string // a single result operand needs reordering (>2 ops)
	shapeBeyond  string // operand count/shape beyond the current stage
	underflow    string // fewer than 2 sim entries for a 2-result shape
	notAdjacent  string // two result operands are not adjacent on top
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
			// seating it needs a 3-deep rotate the VM has no opcode for. No
			// spec row hits this shape; refuse so the program falls back.
			return msg.reorder
		}
	case 2:
		if n != 2 {
			return msg.shapeBeyond
		}
		if len(lw.vm) < 2 {
			return msg.underflow
		}
		top, below := lw.vm[len(lw.vm)-1], lw.vm[len(lw.vm)-2]
		switch {
		case slotIs(top, ops[0]) && slotIs(below, ops[1]):
			// already in layout
		case slotIs(top, ops[1]) && slotIs(below, ops[0]):
			lw.swapTop2(pos)
		default:
			return msg.notAdjacent
		}
	default:
		return msg.shapeBeyond
	}
	return ""
}

// reconcileResults arranges a unit's N result operands (bottom→top) as the
// final stack, ready for a RET. Each event operand must already sit on the
// simulated stack in order — it was left there by its own event — and inert
// operands (const / local / type) are pushed as a trailing tail above the
// last event result. who prefixes the refusal reason ("fn name"). This is
// the fn-unit mirror of Finalize's program-residual reconciliation, the
// multi-result generalisation of the old single-result body tail.
func (lw *lowerer) reconcileResults(ops []emitOperand, who string, pos SrcPos) string {
	vi := 0
	var tail []emitOperand
	for _, op := range ops {
		if op.kind == opEvent {
			if lw.variadic[op.idx] {
				return who + ": result is a variadic loop value (Stage 3)"
			}
			if len(tail) > 0 {
				return who + ": result above a literal (Stage 3)"
			}
			if vi >= len(lw.vm) || !slotIs(lw.vm[vi], op) {
				return who + ": body leaves extra values (Stage 3 lowers in-order results)"
			}
			vi++
			continue
		}
		tail = append(tail, op)
	}
	if vi != len(lw.vm) {
		return who + ": body leaves extra values (Stage 3 lowers in-order results)"
	}
	for _, op := range tail {
		lw.pushOperand(op, pos)
	}
	return ""
}

func (lw *lowerer) lowerCall(ev *emitEvent) string {
	c := &ev.call
	n := len(c.ops)
	if reason := lw.layoutOperands(c.ops, c.pos, layoutMsgs{
		loopResults:  "consumes loop results (Stage 2 loops only feed the program residual)",
		resultNotTop: "stack discipline: result operand of " + c.word + " is not on top",
		reorder:      "operand shape at " + c.word + " needs reordering beyond Stage 1",
		shapeBeyond:  "operand shape at " + c.word + " beyond Stage 1",
		underflow:    "stack discipline underflow at " + c.word,
		notAdjacent:  "stack discipline: operands of " + c.word + " not adjacent on top",
	}); reason != "" {
		return reason
	}
	if c.poly {
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
func (lw *lowerer) lowerFragment(frag *EmitFragment, out *emitOperand, pos SrcPos) string {
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
		if lw.variadic[out.idx] {
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
		underflow:    "stack discipline underflow at fn call",
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
// so the arm/body contributes no merge value.
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
		if last.seq == out.idx && last.br.constCond == nil && last.br.hasElse {
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
	br := &ev.br
	armOut := func(has bool, op *emitOperand) *emitOperand {
		if has {
			return op
		}
		return nil
	}
	if br.constCond != nil {
		// Statically-taken branch: inline the taken fragment.
		if reason := lw.lowerFragment(br.then, armOut(br.hasThenOut, &br.thenOut), br.pos); reason != "" {
			return reason
		}
		if br.hasThenOut {
			lw.vm = append(lw.vm, vmSlot{seq: ev.seq})
			lw.note()
		}
		return ""
	}
	if br.elsComputed {
		// Computed else (`if cond [then] (expr)`): the else value is eagerly on
		// the stack BELOW the cond event. Bring the cond to the top so
		// JMP_IF_FALSE can consume it; the else value stays below — the result
		// on the false path, DROPped before the then-body on the true path.
		if len(lw.vm) < 2 || !slotIs(lw.vm[len(lw.vm)-1], br.elsVal) || !slotIs(lw.vm[len(lw.vm)-2], br.cond) {
			return "if: computed-else stack layout (Stage 2)"
		}
		if !br.hasThenOut {
			return "if: computed else with diverging then (Stage 2)"
		}
		lw.swapTop2(br.pos)
		jf := lw.emit(OpJmpIfFalse, 0, br.pos)
		lw.vm = lw.vm[:len(lw.vm)-1] // cond consumed; the else value stays
		return lw.lowerArmsComputed(ev, jf)
	}
	// Condition on top of stack: a pre-evaluated value, or an inline
	// list-form condition body lowered here (it nets one Boolean).
	switch {
	case br.condFrag != nil:
		if reason := lw.lowerFragment(br.condFrag, &br.condOut, br.pos); reason != "" {
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

// lowerArms emits the then/else arms after the JMP_IF_FALSE at jf.
// A diverging arm's terminator already jumped out of the construct,
// so it needs no jump-to-end and contributes no value; the merge
// point then carries only the surviving arm's value. The 2-arg form
// (no else) merges with 0-or-1 values — a VARIADIC result.
func (lw *lowerer) lowerArms(ev *emitEvent, jf int) string {
	br := &ev.br
	thenOut := func() *emitOperand {
		if br.hasThenOut {
			return &br.thenOut
		}
		return nil
	}()
	if reason := lw.lowerFragment(br.then, thenOut, br.pos); reason != "" {
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
	if !fragDiverges(br.then) {
		jend = lw.emit(OpJmp, 0, br.pos)
	}
	(*lw.code)[jf].Arg = int32(len(*lw.code))
	if br.elsIsVal {
		// Value-else: push the literal/local/type operand as the arm's
		// single result (mirrors lowerFragment's const-out accounting —
		// pushOperand tracked it; the merge slot below owns the count).
		lw.pushOperand(br.elsVal, br.pos)
		lw.vm = lw.vm[:len(lw.vm)-1]
	} else {
		elsOut := func() *emitOperand {
			if br.hasElsOut {
				return &br.elsOut
			}
			return nil
		}()
		if reason := lw.lowerFragment(br.els, elsOut, br.pos); reason != "" {
			return reason
		}
	}
	if jend >= 0 {
		(*lw.code)[jend].Arg = int32(len(*lw.code))
	}
	if br.hasThenOut || br.hasElsOut {
		lw.vm = append(lw.vm, vmSlot{seq: ev.seq})
		lw.note()
	}
	return ""
}

// lowerArmsComputed lowers a computed-else `if cond [then] (expr)` after the
// JMP_IF_FALSE at jf. Entry sim/runtime stack is [.., elseVal] (the cond was
// just consumed; the eagerly-computed else value remains). The TAKEN path drops
// the else value and runs the then-body; the FALSE path falls through with the
// else value as the result. Both arms net exactly one value, so the result is a
// single (non-variadic) merge slot.
func (lw *lowerer) lowerArmsComputed(ev *emitEvent, jf int) string {
	br := &ev.br
	// True path: discard the else value, then run the then-body.
	lw.emit(OpDrop, 0, br.pos)
	lw.vm = lw.vm[:len(lw.vm)-1] // else value dropped on this path
	if reason := lw.lowerFragment(br.then, &br.thenOut, br.pos); reason != "" {
		return reason
	}
	jend := lw.emit(OpJmp, 0, br.pos)
	(*lw.code)[jf].Arg = int32(len(*lw.code)) // false path lands here, else value intact
	(*lw.code)[jend].Arg = int32(len(*lw.code))
	lw.vm = append(lw.vm, vmSlot{seq: ev.seq})
	lw.note()
	return ""
}
