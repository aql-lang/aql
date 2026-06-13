package eng

// The bytecode recording pass — Stage 1 of design/aql-bytecode-plan.0.md.
//
// The compiler is the carrier checker with a recording side effect:
// when CheckState.Emit is set, every check-mode dispatch that flows
// through carrierResults records a call event with full operand
// provenance, and Finalize linearises the event trace into a Program.
//
// Site taxonomy (aql-bytecode-readiness.0.md gap 1) — every dispatch
// is classified from the first commit:
//
//   - mono                — single checker-selected signature; compiles.
//   - poly (partitioned)  — a strict disjunct straddles signatures;
//     Stage 1 marks the program uncompilable, later stages emit
//     CALL_NATIVE_POLY here.
//   - dynamic             — a dynamic carrier reached the site; fallback
//     territory.
//   - meta                — RunInCheckMode / code-body / user-fn /
//     recovered dispatches; compile-time-only or fallback.
//
// Operand provenance rides Value.ID: NewValueRaw mints a unique ID
// per value and toCarrier/copies preserve it, so a dispatch arg is
// (a) a recorded output of an earlier call, (b) a literal — concrete
// at the dispatch, or a stripped top-level literal whose original
// was saved by RecordStrip — or (c) unknown, which marks the program
// uncompilable rather than guessing.

// Site classes for SiteCounts.
const (
	SiteMono    = "mono"
	SitePoly    = "poly"
	SiteDynamic = "dynamic"
	SiteMeta    = "meta"
)

type emitOperand struct {
	constIdx  int // >=0: Consts index
	fromSeq   int // >=0: producing event sequence number
	localSlot int // >=0: loop-iterator local slot
	typeIdx   int // >=0: Types index (canonical type operand)
}

type emitCall struct {
	word string
	sig  *Signature
	ops  []emitOperand
	pos  SrcPos
}

// emitBranch is a recorded `if`: a resolved condition operand, the
// captured then/else fragments, and each fragment's single result
// operand. constCond non-nil means the condition was statically
// known and only the taken fragment was captured.
type emitBranch struct {
	cond                  emitOperand
	condFrag              *EmitFragment // list-form condition: lower inline, ends with one Boolean
	condOut               emitOperand
	constCond             *bool
	hasElse               bool // false = 2-arg if; its result is VARIADIC (0 or 1 values)
	then, els             *EmitFragment
	thenOut, elsOut       emitOperand
	hasThenOut, hasElsOut bool // false when the arm DIVERGES (ends in break/continue)
	pos                   SrcPos
}

// emitLoop is a recorded counted `for`: the count operand, the
// captured body fragment (final fixed-point round), its single
// per-iteration result, and the iterator's local slot. The loop's
// runtime contribution is VARIADIC — one value per iteration stays
// on the stack — so downstream consumption refuses; only the
// residual may absorb it.
type emitLoop struct {
	start, end, step emitOperand // start/step are always consts in Stage 2
	body             *EmitFragment
	bodyOut          emitOperand
	hasBodyOut       bool // false: the body nets no value per iteration (or diverges)
	iterSlot         int
	pos              SrcPos
}

const (
	evCall = iota + 1
	evBranch
	evLoop
	evBreak
	evContinue
	evCallUser
	evFallback
)

// emitUserCall is a recorded call of a compiled AQL fn: the target
// unit index, the args in sig order, and whether the lowering marked
// it a TAIL call (it then replaces the frame and control never
// returns to the site).
type emitUserCall struct {
	unit int
	ops  []emitOperand
	tail bool
	pos  SrcPos
}

// emitFallback is a recorded interpreter-island fallback (Stage 5): a
// construct the compiler can't lower, captured as a self-contained
// re-runnable token span plus its threaded stack inputs.
type emitFallback struct {
	spanIdx int
	ins     []emitOperand
	pos     SrcPos
}

type emitEvent struct {
	seq  int
	kind int
	call emitCall
	br   emitBranch
	loop emitLoop
	uc   emitUserCall
	fb   emitFallback
}

// EmitFragment is a captured sub-trace: the events a branch body
// recorded, plus the sequence floor — operands inside the fragment
// referencing events BELOW the floor read enclosing computation,
// which Stage 2's closed-branch lowering refuses.
type EmitFragment struct {
	events   []emitEvent
	startSeq int
}

// EmitState is the recording side of the compile pass. Set
// CheckState.Emit to a NewEmitState before a check run; call
// Finalize afterwards. All methods are nil-receiver-safe so hook
// sites need no guards.
type EmitState struct {
	// Compilable latches false at the first construct Stage 1 cannot
	// lower; Reason names the first offender.
	Compilable bool
	Reason     string
	// SiteCounts tallies dispatches per site class while recording is
	// active (counting stops once the program is marked
	// uncompilable, with the rest of the recording).
	SiteCounts map[string]int

	suspended  int
	seq        int            // monotonic event sequence
	frames     [][]emitEvent  // frames[0] = top level; fragments push
	fragFloors []int          // startSeq per open fragment frame
	captureArm bool           // next RunCarrierBodyWithDefs records a fragment
	loopArm    bool           // next AnalyseLoopBody records its final round
	fnArm      bool           // next AnalyseFnBody records (fn compilation open)
	captured   *EmitFragment  // last completed fragment, until TakeFragment
	units      []*emitUnit    // units[0] = top level; fn compilations push
	fnUnits    map[string]int // fn memo key → Program.Fns index
	fnRecs     []*fnUnitRec
	producedBy map[string]int // value ID → producing event seq
	consts     []Value
	constIdx   map[string]int // CanonValue → Consts index
	types      []TypeRef
	typeIdx    map[string]int   // type ID → Types index
	fallbacks  []FallbackSpan   // Stage 5 interpreter islands
	origByID   map[string]Value // stripped literal ID → original value
}

// emitUnit scopes local-slot numbering to one code unit (the
// top-level program, or one compiled fn body — locals are
// frame-relative at run time).
type emitUnit struct {
	localByID map[string]int
	numLocals int
}

// fnUnitRec is one compiled fn body awaiting (or holding) its
// lowered form. caps are the fn's construction-time captures: they
// ride as hidden trailing parameters (slots nParams…), so every call
// site re-supplies the captured values from ITS scope — inside the
// fn's own body a recursive call resolves them to the frame's own
// capture slots, which is exactly the snapshot semantics (captures
// are fixed at construction; the same values flow through).
// generic marks an instantiation of a gen fn — one unit per memoised
// instantiation (the key carries the instantiated arg types), kept
// OUT of tail marking, mirroring the interpreter's HasGen exclusion
// from frame elision (plan Stage 4).
type fnUnitRec struct {
	name     string
	nParams  int
	caps     []CapturedBinding
	generic  bool
	returns  []*Type  // declared return types — enforced at the VM's RET
	locals   []string // slot→name table (params then captures); debug only
	frag     *EmitFragment
	outOp    emitOperand
	hasOut   bool
	numLoc   int
	pos      SrcPos
	finished bool
}

// NewEmitState returns a fresh recording state.
func NewEmitState() *EmitState {
	return &EmitState{
		Compilable: true,
		SiteCounts: map[string]int{},
		frames:     [][]emitEvent{nil},
		units:      []*emitUnit{{localByID: map[string]int{}}},
		fnUnits:    map[string]int{},
		producedBy: map[string]int{},
		constIdx:   map[string]int{},
		typeIdx:    map[string]int{},
		origByID:   map[string]Value{},
	}
}

func (es *EmitState) active() bool {
	return es != nil && es.Compilable && es.suspended == 0
}

// MarkUncompilable latches the program uncompilable, keeping the
// FIRST reason (later marks are consequences of the first).
func (es *EmitState) MarkUncompilable(reason string) {
	if es == nil || !es.Compilable {
		return
	}
	es.Compilable = false
	es.Reason = reason
}

// Suspend pauses recording for the duration of a nested body
// analysis (branch bodies, fn bodies, higher-order bodies run their
// own sub-engines over the shared registry; their dispatches are not
// part of THIS program's straight line). Returns the resume func.
func (es *EmitState) Suspend() func() {
	if es == nil {
		return func() {}
	}
	es.suspended++
	return func() { es.suspended-- }
}

// ArmBranchCapture makes the NEXT RunCarrierBodyWithDefs record its
// body into a fragment instead of suspending — the branch-lowering
// hook (`if`). One-shot; consumed by the body run.
func (es *EmitState) ArmBranchCapture() {
	if !es.active() {
		return
	}
	es.captureArm = true
}

// consumeCaptureArm reports and clears the one-shot capture flag.
func (es *EmitState) consumeCaptureArm() bool {
	if es == nil || !es.captureArm {
		return false
	}
	es.captureArm = false
	return es.active()
}

// beginFragment opens a recording frame; the returned func closes it
// into es.captured.
func (es *EmitState) beginFragment() func() {
	es.frames = append(es.frames, nil)
	es.fragFloors = append(es.fragFloors, es.seq)
	return func() {
		n := len(es.frames) - 1
		es.captured = &EmitFragment{
			events:   es.frames[n],
			startSeq: es.fragFloors[len(es.fragFloors)-1],
		}
		es.frames = es.frames[:n]
		es.fragFloors = es.fragFloors[:len(es.fragFloors)-1]
	}
}

// bodyAnalysisGuard is called by RunCarrierBodyWithDefs: capture a
// fragment when armed, otherwise suspend recording for the nested
// body. Nil-safe.
func (es *EmitState) bodyAnalysisGuard() func() {
	if es.consumeCaptureArm() {
		return es.beginFragment()
	}
	return es.Suspend()
}

// TakeFragment returns the last captured fragment (nil when the
// capture never armed — plain check runs, suspended recordings).
func (es *EmitState) TakeFragment() *EmitFragment {
	if es == nil {
		return nil
	}
	f := es.captured
	es.captured = nil
	return f
}

// appendEvent adds an event to the current frame and returns its seq.
func (es *EmitState) appendEvent(ev emitEvent) int {
	es.seq++
	ev.seq = es.seq
	n := len(es.frames) - 1
	es.frames[n] = append(es.frames[n], ev)
	return ev.seq
}

// resolveOperand maps a dispatch value to its provenance: a prior
// event's output, or an inert constant (concrete at the dispatch, or
// a stripped literal whose original RecordStrip saved).
func (es *EmitState) resolveOperand(v Value) (emitOperand, bool) {
	// Events first, locals second: a join can REUSE a local's value ID
	// for its result (JoinCarriers keeps the then-side ID when types
	// agree), and the event is then the value's stack-discipline truth
	// — the branch pushed it. A plain param/iterator reference has no
	// producing event and resolves to its local slot.
	if idx, ok := es.producedBy[v.ID]; ok {
		return emitOperand{constIdx: -1, fromSeq: idx, localSlot: -1, typeIdx: -1}, true
	}
	if slot, ok := es.units[len(es.units)-1].localByID[v.ID]; ok {
		return emitOperand{constIdx: -1, fromSeq: -1, localSlot: slot, typeIdx: -1}, true
	}
	// A bare type node is a TYPE operand: it must reach the runtime
	// as the CANONICAL registry node (a pooled by-value copy goes
	// stale against behaviour/field installs), so it gets its own
	// table, resolved by ID at run time via OpPushType.
	if IsBareTypeNode(v) && v.ID != "" {
		return emitOperand{constIdx: -1, fromSeq: -1, localSlot: -1, typeIdx: es.internType(v)}, true
	}
	lit, ok := es.materialise(v)
	if !ok || !isInertConst(lit) {
		return emitOperand{}, false
	}
	return emitOperand{constIdx: es.intern(lit), fromSeq: -1, localSlot: -1, typeIdx: -1}, true
}

// materialise recovers the fully concrete value behind a stripped
// literal: the value itself, its RecordStrip original, or — for a
// concrete container whose MEMBERS were stripped by a sub-engine run
// (autoEvalMap evaluates each field through Run, which strips) — a
// rebuilt copy with each carrier member replaced by its recorded
// original, recursively. ok=false when any member's original is
// unknown.
func (es *EmitState) materialise(v Value) (Value, bool) {
	if v.Carrier || v.Dynamic {
		orig, ok := es.origByID[v.ID]
		if !ok {
			return v, false
		}
		return orig, true
	}
	switch d := v.Data.(type) {
	case ListPayload:
		rebuilt := false
		elems := d.Elems
		for i, e := range elems {
			m, ok := es.materialise(e)
			if !ok {
				return v, false
			}
			if m.Carrier != e.Carrier || m.ID != e.ID {
				if !rebuilt {
					elems = append([]Value(nil), d.Elems...)
					rebuilt = true
				}
				elems[i] = m
			}
		}
		if rebuilt {
			nv := v
			nv.Data = ListPayload{Elems: elems}
			return nv, true
		}
		return v, true
	case MapPayload:
		if d.M == nil {
			return v, false
		}
		var nm *OrderedMap
		for _, k := range d.M.Keys() {
			mv, _ := d.M.Get(k)
			m, ok := es.materialise(mv)
			if !ok {
				return v, false
			}
			if nm == nil && (m.Carrier != mv.Carrier || m.ID != mv.ID) {
				nm = NewOrderedMap()
				for _, pk := range d.M.Keys() {
					if pk == k {
						break
					}
					pv, _ := d.M.Get(pk)
					nm.Set(pk, pv)
				}
			}
			if nm != nil {
				nm.Set(k, m)
			}
		}
		if nm != nil {
			nv := v
			nv.Data = MapPayload{M: nm}
			return nv, true
		}
		return v, true
	}
	return v, true
}

// fragDiverges reports whether a captured fragment's trailing event
// is a flow-control terminator — the arm never reaches the branch
// join (its lowering emitted the loop jump).
func fragDiverges(frag *EmitFragment) bool {
	if frag == nil || len(frag.events) == 0 {
		return false
	}
	last := &frag.events[len(frag.events)-1]
	if last.kind == evCallUser && last.uc.tail {
		return true
	}
	return last.kind == evBreak || last.kind == evContinue
}

// BranchRecord carries one `if` dispatch into RecordBranch.
type BranchRecord struct {
	Cond            Value         // pre-evaluated condition (paren/value form)
	CondFrag        *EmitFragment // list-form condition body, when analysed
	CondStk         []Value       // its residual stack
	ConstCond       *bool         // statically-known condition: only Then captured
	HasElse         bool
	Then, Els       *EmitFragment
	ThenStk, ElsStk []Value
	Out             Value
	Pos             SrcPos
}

// RecordBranch records an `if` dispatch: condition (pre-evaluated,
// list-form fragment, or statically known), the captured arm
// fragments and their residual stacks, and the dispatch's joined
// result carrier. An arm may DIVERGE (end in break/continue) — it
// then contributes no value and never reaches the join. The 2-arg
// form (HasElse=false) has a VARIADIC result (0 or 1 values), which
// only the program residual may absorb. Any shape Stage 2 cannot
// lower marks the program uncompilable.
func (es *EmitState) RecordBranch(b BranchRecord) {
	if !es.active() {
		return
	}
	ev := emitEvent{kind: evBranch, br: emitBranch{
		constCond: b.ConstCond, hasElse: b.HasElse, pos: b.Pos,
	}}
	resolveArm := func(frag *EmitFragment, stk []Value, name string) (emitOperand, bool, bool) {
		if frag == nil {
			es.MarkUncompilable("if: " + name + "-branch not captured")
			return emitOperand{}, false, false
		}
		if fragDiverges(frag) {
			return emitOperand{}, false, true
		}
		if len(stk) == 0 {
			es.MarkUncompilable("if: " + name + "-branch produces no value (Stage 2 lowers single-result branches)")
			return emitOperand{}, false, false
		}
		op, ok := es.resolveOperand(stk[len(stk)-1])
		if !ok {
			es.MarkUncompilable("if: " + name + "-branch result of unknown provenance")
			return emitOperand{}, false, false
		}
		return op, true, true
	}
	// Condition.
	if b.ConstCond == nil {
		switch {
		case b.CondFrag != nil:
			if fragDiverges(b.CondFrag) || len(b.CondStk) == 0 {
				es.MarkUncompilable("if: condition body produces no value")
				return
			}
			op, ok := es.resolveOperand(b.CondStk[len(b.CondStk)-1])
			if !ok {
				es.MarkUncompilable("if: condition result of unknown provenance")
				return
			}
			ev.br.condFrag, ev.br.condOut = b.CondFrag, op
		default:
			condOp, ok := es.resolveOperand(b.Cond)
			if !ok {
				es.MarkUncompilable("if: condition of unknown provenance")
				return
			}
			ev.br.cond = condOp
		}
	}
	// Arms.
	if b.ConstCond != nil {
		out, has, ok := resolveArm(b.Then, b.ThenStk, "taken")
		if !ok {
			return
		}
		ev.br.then, ev.br.thenOut, ev.br.hasThenOut = b.Then, out, has
	} else {
		thenOut, hasThen, ok := resolveArm(b.Then, b.ThenStk, "then")
		if !ok {
			return
		}
		ev.br.then, ev.br.thenOut, ev.br.hasThenOut = b.Then, thenOut, hasThen
		if b.HasElse {
			elsOut, hasEls, ok := resolveArm(b.Els, b.ElsStk, "else")
			if !ok {
				return
			}
			ev.br.els, ev.br.elsOut, ev.br.hasElsOut = b.Els, elsOut, hasEls
			if !hasThen && !hasEls {
				es.MarkUncompilable("if: both branches diverge (Stage 2)")
				return
			}
		}
	}
	seq := es.appendEvent(ev)
	es.SiteCounts[SiteMono]++
	es.producedBy[b.Out.ID] = seq
}

// ArmLoopCapture makes the NEXT AnalyseLoopBody record its final
// fixed-point round as a fragment with the loop bindings registered
// as locals — the loop-lowering hook (`for`). One-shot.
func (es *EmitState) ArmLoopCapture() {
	if !es.active() {
		return
	}
	es.loopArm = true
}

// ConsumeLoopArm reports and clears the one-shot loop-capture flag.
func (es *EmitState) ConsumeLoopArm() bool {
	if es == nil || !es.loopArm {
		return false
	}
	es.loopArm = false
	return es.active()
}

// RegisterLocal assigns (or returns) the local slot backing a loop
// binding's carrier, keyed by the carrier's value ID — body
// references to the binding resolve to PUSH_LOCAL.
func (es *EmitState) RegisterLocal(id string) int {
	if es == nil {
		return -1
	}
	u := es.units[len(es.units)-1]
	if slot, ok := u.localByID[id]; ok {
		return slot
	}
	slot := u.numLocals
	u.numLocals++
	u.localByID[id] = slot
	return slot
}

// fnArm bypasses AnalyseFnBody's suspension exactly once — set by
// StartFnCompile so the armed body analysis records into the open
// fn fragment.
func (es *EmitState) fnBodyGuard() func() {
	if es != nil && es.fnArm {
		es.fnArm = false
		return func() {}
	}
	return es.Suspend()
}

// StartFnCompile reserves (or finds) the compiled unit for one fn
// overload at one arg shape and, when fresh, opens a fn recording
// scope: a new local-numbering unit with the arg carriers registered
// as param slots 0..n-1 (sig order — frame locals at run time) and
// the captures as hidden trailing slots, and a fragment capturing
// the body analysis that the caller runs next. finish (non-nil only
// when fresh) closes the scope with the body's residual stack.
// Generic fns compile one unit per memoised instantiation — the key
// carries the instantiated arg types, and the caller has the gen
// bindings installed around the recorded analysis. ok=false when the
// fn is beyond Stage 4 (unchecked or multi-value returns) — the
// program is then marked uncompilable.
func (es *EmitState) StartFnCompile(key, name string, args []Value, declared []*Type, paramNames []string, captures []CapturedBinding, generic bool) (unit int, finish func([]Value), ok bool) {
	if !es.active() {
		return -1, nil, false
	}
	if len(declared) != 1 {
		es.MarkUncompilable("fn " + name + " without exactly one declared return (Stage 3 lowers single-return fns)")
		return -1, nil, false
	}
	if u, hit := es.fnUnits[key]; hit {
		return u, nil, true
	}
	unit = len(es.fnRecs)
	// Slot→name table (debug only): params in slots 0..n-1, then captures.
	locals := make([]string, 0, len(args)+len(captures))
	for i := range args {
		if i < len(paramNames) {
			locals = append(locals, paramNames[i])
		} else {
			locals = append(locals, "")
		}
	}
	for _, cb := range captures {
		locals = append(locals, cb.Name)
	}
	rec := &fnUnitRec{name: name, nParams: len(args), caps: captures, generic: generic, returns: declared, locals: locals}
	es.fnRecs = append(es.fnRecs, rec)
	es.fnUnits[key] = unit
	u := &emitUnit{localByID: map[string]int{}}
	es.units = append(es.units, u)
	for _, a := range args {
		es.RegisterLocal(a.ID)
	}
	// Capture slots: the body analysis binds each captured name to
	// cb.Value (the construction-time snapshot — AnalyseFnBody pushes
	// the SAME Value), so body references resolve to these slots by
	// ID. Registered after params, locals nParams…nParams+nCaps-1.
	for _, cb := range captures {
		es.RegisterLocal(cb.Value.ID)
	}
	resume := es.beginFragment()
	es.fnArm = true
	finish = func(bodyStk []Value) {
		resume()
		rec.frag = es.TakeFragment()
		// The body must leave exactly the declared number of values. A
		// different count is a return-COUNT error the interpreter raises;
		// the single-result lowering would otherwise silently keep just
		// the last value, so refuse and let the program fall back.
		if !fragDiverges(rec.frag) && len(bodyStk) != len(rec.returns) {
			es.MarkUncompilable("fn " + name + ": body value count differs from declared returns")
			return
		}
		if len(bodyStk) > 0 && !fragDiverges(rec.frag) {
			if op, okOut := es.resolveOperand(bodyStk[len(bodyStk)-1]); okOut {
				rec.outOp, rec.hasOut = op, true
			} else {
				es.MarkUncompilable("fn " + name + ": body result of unknown provenance")
			}
		} else if !fragDiverges(rec.frag) {
			es.MarkUncompilable("fn " + name + ": body produces no value")
		}
		// The unit is closed: its events' outputs cannot be stack
		// values in the enclosing scope, so drop their provenance
		// entries (after resolving the body result above, which DOES
		// reference them). Without this, a join inside the body that
		// reused a capture/param ID (JoinCarriers keeps the then-side
		// ID) would make an ENCLOSING call site resolve that value to
		// an event of this closed unit.
		for id, seq := range es.producedBy {
			if seq > rec.frag.startSeq {
				delete(es.producedBy, id)
			}
		}
		rec.numLoc = u.numLocals
		rec.finished = true
		es.units = es.units[:len(es.units)-1]
	}
	return unit, finish, true
}

// RecordUserCall records one call of a compiled fn unit. args are in
// signature order; out is the dispatch's declared-return carrier.
// The unit's captures ride as hidden trailing operands, resolved in
// the CALLER's scope: at the construction site they are the enclosing
// frame's values; inside the fn's own body (recursion) they resolve
// to the frame's own capture slots and re-flow unchanged. A capture
// unreachable from the call site marks the program uncompilable —
// the interpreter keeps owning that shape.
func (es *EmitState) RecordUserCall(unit int, args []Value, out Value, pos SrcPos) {
	if !es.active() || unit < 0 {
		return
	}
	rec := es.fnRecs[unit]
	ops := make([]emitOperand, len(args), len(args)+len(rec.caps))
	for i, a := range args {
		op, ok := es.resolveOperand(a)
		if !ok {
			es.MarkUncompilable("fn call operand of unknown provenance")
			return
		}
		ops[i] = op
	}
	for _, cb := range rec.caps {
		op, ok := es.resolveOperand(cb.Value)
		if !ok {
			es.MarkUncompilable("capture " + cb.Name + " of " + rec.name + " unreachable at a call site")
			return
		}
		ops = append(ops, op)
	}
	seq := es.appendEvent(emitEvent{kind: evCallUser, uc: emitUserCall{unit: unit, ops: ops, pos: pos}})
	es.SiteCounts[SiteMono]++
	es.producedBy[out.ID] = seq
}

// RecordLoop records a counted/range `for`: start/end/step operand
// values (start and step must resolve to constants in Stage 2; the
// end may be computed), the body fragment (final fixed-point round),
// and the iterator's slot. The body either nets exactly one value
// per iteration or nothing (a net-0 or diverging body). out is the
// dispatch's result carrier — registered so the dispatch isn't
// re-recorded, and marked VARIADIC at lowering, so only the program
// residual may absorb the accumulation.
func (es *EmitState) RecordLoop(start, end, step Value, body *EmitFragment, bodyStk []Value, iterID string, out Value, pos SrcPos) {
	if !es.active() {
		return
	}
	if body == nil {
		es.MarkUncompilable("for: body not captured")
		return
	}
	startOp, ok1 := es.resolveOperand(start)
	endOp, ok2 := es.resolveOperand(end)
	stepOp, ok3 := es.resolveOperand(step)
	if !ok1 || !ok2 || !ok3 {
		es.MarkUncompilable("for: range of unknown provenance")
		return
	}
	if startOp.constIdx < 0 || stepOp.constIdx < 0 {
		es.MarkUncompilable("for: computed range start/step (Stage 2 follow-on)")
		return
	}
	lp := emitLoop{start: startOp, end: endOp, step: stepOp, iterSlot: -1, pos: pos}
	if len(bodyStk) > 0 && !fragDiverges(body) {
		bodyOut, ok := es.resolveOperand(bodyStk[len(bodyStk)-1])
		if !ok {
			es.MarkUncompilable("for: body result of unknown provenance")
			return
		}
		lp.bodyOut, lp.hasBodyOut = bodyOut, true
	}
	slot, ok := es.units[len(es.units)-1].localByID[iterID]
	if !ok {
		es.MarkUncompilable("for: iterator slot not registered")
		return
	}
	lp.body, lp.iterSlot = body, slot
	seq := es.appendEvent(emitEvent{kind: evLoop, loop: lp})
	es.SiteCounts[SiteMono]++
	es.producedBy[out.ID] = seq
}

// RecordFallback records an interpreter-island fallback (Stage 5). span
// is the self-contained re-runnable token sequence; ins are the threaded
// stack inputs (sig order, deepest first) — empty for a fully-baked
// span; out is the dispatch's single result carrier, registered so the
// island's result flows downstream (to the residual, or another
// fallback; a downstream TYPED dispatch consuming a dynamic result still
// refuses via anyDynamicCarrier, preserving soundness). Returns false
// when an input's provenance is unknown — the caller then lets the
// normal refusal stand.
func (es *EmitState) RecordFallback(span FallbackSpan, ins []Value, out Value, pos SrcPos) bool {
	if !es.active() {
		return false
	}
	ops := make([]emitOperand, len(ins))
	for i, in := range ins {
		op, ok := es.resolveOperand(in)
		if !ok {
			return false
		}
		ops[i] = op
	}
	span.NIn = len(ins)
	idx := len(es.fallbacks)
	es.fallbacks = append(es.fallbacks, span)
	seq := es.appendEvent(emitEvent{kind: evFallback, fb: emitFallback{spanIdx: idx, ins: ops, pos: pos}})
	es.SiteCounts[SiteDynamic]++
	es.producedBy[out.ID] = seq
	return true
}

// RecordStrip remembers the original concrete value behind a
// top-level literal that StripToCarriers reduced to a carrier — the
// ID is preserved by the strip, so a later dispatch arg with this ID
// is that literal.
func (es *EmitState) RecordStrip(orig, stripped Value) {
	if es == nil || es.suspended > 0 {
		return
	}
	if stripped.Carrier && !orig.Carrier && orig.ID != "" && orig.ID == stripped.ID {
		es.origByID[orig.ID] = orig
	}
}

// RecordPoly classifies a partitioned (per-alternative) dispatch.
// Stage 1 cannot lower it; later stages emit CALL_NATIVE_POLY.
func (es *EmitState) RecordPoly(word string) {
	if !es.active() {
		return
	}
	es.SiteCounts[SitePoly]++
	es.MarkUncompilable("polymorphic dispatch at " + word)
}

// RecordCall records one resolved dispatch. args are in signature
// order (position 0 = top of stack); outs are the carrier results.
func (es *EmitState) RecordCall(word string, sig *Signature, args, outs []Value, pos SrcPos) {
	if !es.active() {
		return
	}
	// A dispatch whose output is already registered was recorded by a
	// structured hook (RecordBranch owns the `if` dispatch) — the
	// generic path must not double-record or refuse it.
	if len(outs) == 1 {
		if _, ok := es.producedBy[outs[0].ID]; ok {
			return
		}
	}
	// Compile-time NAME RESOLUTION: a get/getr whose result is a
	// statically-known callable or namespace (a module export wrapper,
	// a module-export instance) executed during the check pass —
	// `MathUtil.sqrt 16.0` is the tokens `MathUtil get sqrt 16.0`, and
	// the resolved wrapper's own dispatch records the REAL call (the
	// inner native's sig through execMatch on this engine, so CALL_
	// NATIVE parity holds). Elide the resolution event; if the value
	// instead flows somewhere data-like, downstream provenance refuses
	// and the program falls back.
	if (word == "get" || word == "getr") && len(outs) == 1 && IsConcrete(outs[0]) {
		switch outs[0].Data.(type) {
		case FnDefInfo, ExtensionPayload:
			return
		}
	}
	switch {
	case sig == nil:
		es.MarkUncompilable("dispatch without a signature at " + word)
		return
	case word == "":
		// Anonymous / fn-value dispatch (usurp wrappers, F4 value
		// calls): the callee is a runtime value, Stage 3 territory.
		es.SiteCounts[SiteMeta]++
		es.MarkUncompilable("anonymous function dispatch (Stage 3)")
		return
	case sig.RunInCheckMode:
		es.SiteCounts[SiteMeta]++
		es.MarkUncompilable("compile-time word " + word)
		return
	case sig.FnFrame != nil:
		es.SiteCounts[SiteMeta]++
		es.MarkUncompilable("user fn call " + word + " (Stage 3)")
		return
	case sig.FullStack:
		es.SiteCounts[SiteMeta]++
		es.MarkUncompilable("full-stack word " + word)
		return
	case word == "args" || word == "__pa":
		// `args` reads the interpreter's per-call args stack, which the
		// VM's CALL_USER frame does not maintain (it binds params to
		// frame locals instead). A compiled fn body that reads `args`
		// would fail with "args: not inside a function" — refuse so the
		// program falls back to the interpreter.
		es.SiteCounts[SiteMeta]++
		es.MarkUncompilable("context-dependent word " + word)
		return
	case len(sig.NoEvalArgs) > 0:
		es.SiteCounts[SiteMeta]++
		es.MarkUncompilable("code-body word " + word + " (Stage 2)")
		return
	case len(sig.QuoteArgs) > 0 && word != "get" && word != "getr":
		// Implicit-quote operands (usurp, force-arity, ref-family):
		// dispatch-manipulating meta words whose results the engine
		// re-steps. get/getr are exempt — plain accessors whose quoted
		// key is an inert Atom const and whose results are data (their
		// fn-valued module-resolution case is elided above; a dynamic
		// or fn-valued result still refuses via the later cases).
		es.SiteCounts[SiteMeta]++
		es.MarkUncompilable("quoted-operand word " + word)
		return
	case anyDynamicCarrier(args):
		es.SiteCounts[SiteDynamic]++
		es.MarkUncompilable("dynamic input at " + word)
		return
	case anyDynamicCarrier(outs):
		// Dynamic outputs mean the checker could not type the word
		// (missing annotations, opaque wrappers like a def-bound
		// usurp value): the recorded signature is a best guess, not a
		// proof — don't bake it in.
		es.SiteCounts[SiteDynamic]++
		es.MarkUncompilable("unannotated or opaque word " + word)
		return
	case word == "break" && len(outs) == 0:
		// A flow-control terminator, not a call: the enclosing loop's
		// lowering turns it into a JMP to the loop end.
		es.appendEvent(emitEvent{kind: evBreak, call: emitCall{word: word, pos: pos}})
		return
	case word == "continue" && len(outs) == 0:
		es.appendEvent(emitEvent{kind: evContinue, call: emitCall{word: word, pos: pos}})
		return
	case len(outs) != 1:
		es.SiteCounts[SiteMeta]++
		es.MarkUncompilable(word + " returns " + itoaSmall(len(outs)) + " values (Stage 1 lowers single-result calls)")
		return
	}
	// Function-valued operands mean a fn-invoking word (apply, usurp,
	// higher-order forms): their handlers return values the ENGINE
	// re-steps on the tape, which a VM cannot honour. Stage 3
	// territory.
	for _, t := range sig.Args {
		if t != nil && (t.ConformsTo(TFunction) || t.ConformsTo(TFnDef)) {
			es.SiteCounts[SiteMeta]++
			es.MarkUncompilable("function-valued operand at " + word + " (Stage 3)")
			return
		}
	}
	for _, a := range args {
		if _, ok := a.Data.(FnDefInfo); ok {
			es.SiteCounts[SiteMeta]++
			es.MarkUncompilable("function value reaches " + word + " (Stage 3)")
			return
		}
	}
	ops := make([]emitOperand, len(args))
	for i, a := range args {
		op, ok := es.resolveOperand(a)
		if !ok {
			es.MarkUncompilable("operand of unknown provenance or not statically materialisable at " + word)
			return
		}
		ops[i] = op
	}
	es.SiteCounts[SiteMono]++
	seq := es.appendEvent(emitEvent{kind: evCall, call: emitCall{word: word, sig: sig, ops: ops, pos: pos}})
	es.producedBy[outs[0].ID] = seq
}

// internType pools a type operand by canonical ID.
func (es *EmitState) internType(v Value) int {
	if i, ok := es.typeIdx[v.ID]; ok {
		return i
	}
	es.types = append(es.types, TypeRef{Name: v.String(), ID: v.ID})
	es.typeIdx[v.ID] = len(es.types) - 1
	return len(es.types) - 1
}

// intern pools a constant by canonical form. Compounds (lists,
// maps) are NEVER pooled: `eq` on compounds compares by identity
// (compare-restrict), so two source literals must stay two constants
// with their two distinct IDs — pooling them made `[1] eq [1]` true
// under the VM where the interpreter says false (the report's
// gotcha #13, caught by the differential gate).
func (es *EmitState) intern(v Value) int {
	if v.Parent.Equal(TList) || v.Parent.Equal(TMap) || isTypeBodyPayload(v) {
		es.consts = append(es.consts, v)
		return len(es.consts) - 1
	}
	key := CanonValue(v)
	if i, ok := es.constIdx[key]; ok {
		return i
	}
	es.consts = append(es.consts, v)
	es.constIdx[key] = len(es.consts) - 1
	return len(es.consts) - 1
}

// isTypeBodyPayload reports a structural type-body payload — pooled
// without dedup, like compounds (identity must not merge).
func isTypeBodyPayload(v Value) bool {
	switch v.Data.(type) {
	case RecordTypeInfo, OptionsTypeInfo, ChildTypeInfo, DisjunctInfo:
		return true
	}
	return false
}

// typeBodyConstOK walks a structural type body's interior: every
// reachable constraint/default must be a bare type node, an inert
// constant, or another clean type body. A check-mode CARRIER inside
// (a generic instantiation built over a class body whose default was
// stripped) would bake the analysis artefact into the const — the
// caught differential mismatch rendered `r:Float` where the
// interpreter rebuilds `r:1.0` — so any carrier, or any payload this
// walk doesn't know, refuses.
func typeBodyConstOK(v Value) bool {
	if v.Carrier || v.Dynamic {
		return false
	}
	if IsBareTypeNode(v) {
		return true
	}
	memberOK := func(m Value) bool {
		return typeBodyConstOK(m) || isInertConst(m)
	}
	fieldsOK := func(m *OrderedMap) bool {
		if m == nil {
			return false
		}
		for _, k := range m.Keys() {
			fv, _ := m.Get(k)
			if !memberOK(fv) {
				return false
			}
		}
		return true
	}
	switch d := v.Data.(type) {
	case RecordTypeInfo:
		return fieldsOK(d.Fields)
	case OptionsTypeInfo:
		return fieldsOK(d.Fields)
	case ChildTypeInfo:
		if !memberOK(d.Child) {
			return false
		}
		for _, e := range d.Elements {
			if !memberOK(e) {
				return false
			}
		}
		for _, en := range d.Entries {
			if !memberOK(en.Value) {
				return false
			}
		}
		return true
	case DisjunctInfo:
		for _, a := range d.Alternatives {
			if !memberOK(a) {
				return false
			}
		}
		return true
	case ObjectTypeInfo:
		// A class / object type body is const-bakeable iff every field
		// default is plain data — a method (fn-value) field is not, so a
		// class with methods (the surface-body case) still refuses. The
		// canonical *Type rides the body's payload pointer (shared, not
		// copied), so it stays canonical at run time; `make` recovers the
		// field schema from the baked body. The parent chain's fields must
		// be data too (AllFields merges them).
		return fieldsOK(d.AllFields())
	}
	return false
}

// isInertConst reports whether v can live in a Program's constant
// pool: a fully concrete value whose payload is PLAIN DATA. The rule
// is a whitelist — scalars, temporal values, structural type bodies,
// and lists/maps of the same — because anything else is
// engine-coupled in some way: a check-mode carrier must not be
// materialised; structural tokens (Word, Splice, Reach, ParenExpr)
// would be expanded or re-stepped by the engine; bare type NODES go
// stale by value-copy against the canonical pointer (eng/go/
// CLAUDE.md, Canonical *Type Pointers — they route through
// OpPushType instead); class/surface bodies can embed method
// fn-values; function values are fn-value call sites (Stage 4 F4).
func isInertConst(v Value) bool {
	if v.Carrier || v.Dynamic || IsBareTypeNode(v) {
		return false
	}
	switch d := v.Data.(type) {
	case IntPayload, FloatPayload, StrPayload, BoolPayload, AtomPayload,
		PathPayload, NonePayload, BigIntPayload, DecimalPayload,
		TimePayload, DurationPayload, TimezonePayload:
		return true
	case RecordTypeInfo, OptionsTypeInfo, ChildTypeInfo, DisjunctInfo, ObjectTypeInfo:
		// STRUCTURAL type bodies (what a bound type name pushes at a
		// use site — make's operand). Sound as consts when their
		// interior is carrier-free (typeBodyConstOK): the payload is
		// pointer-backed (shared, not copied) and the minted lattice
		// node rides the body's Parent POINTER, which stays
		// canonical. Never deduped. A class/object body qualifies only
		// when every field default is data (no method fn-values).
		return typeBodyConstOK(v)
	case ListPayload:
		for _, e := range d.Elems {
			if !isInertConst(e) {
				return false
			}
		}
		return true
	case MapPayload:
		if d.M == nil {
			return false
		}
		for _, k := range d.M.Keys() {
			mv, _ := d.M.Get(k)
			if !isInertConst(mv) {
				return false
			}
		}
		return true
	default:
		return false
	}
}

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
	if lp.end.fromSeq >= 0 {
		if lw.variadic[lp.end.fromSeq] {
			return "loop results as a loop bound (Stage 2)"
		}
		if len(lw.vm) == 0 || lw.vm[len(lw.vm)-1] != lp.end.fromSeq {
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
	lw.vm = append(lw.vm, ev.seq)
	lw.variadic[ev.seq] = true
	lw.note()
	return ""
}

// Finalize linearises the recorded events into a Program. residual
// is the check run's final carrier stack — the program's declared
// result: event-produced entries must match the simulated stack in
// order; literal entries may only sit above the last event-produced
// entry and are pushed at the end. ok=false (with reason) when the
// source was marked uncompilable or a shape is beyond the current
// stage's lowering.
func (es *EmitState) Finalize(residual []Value) (*Program, string, bool) {
	if es == nil {
		return nil, "no emit state", false
	}
	if !es.Compilable {
		return nil, es.Reason, false
	}
	p := &Program{}
	lw := &lowerer{es: es, p: p, code: &p.Code, debug: &p.Debug, sigIdx: map[*Signature]int{}, variadic: map[int]bool{}}
	if reason := lw.lowerEvents(es.frames[0], 0); reason != "" {
		return nil, reason, false
	}

	// Residual reconciliation.
	lastPos := SrcPos{}
	if n := len(es.frames[0]); n > 0 {
		lastPos = eventPos(es.frames[0][n-1])
	}
	// Fn-value-call boundary (report §9.1): the interpreter auto-applies
	// a FUNCTION value sitting in the residual to the values that follow
	// it (`r.int 0 100` — a map field that is a method, applied to 0
	// 100). A dynamic carrier is statically Any, so the checker cannot
	// tell a Function from data; if a dynamic value appears BEFORE the
	// last residual position, the runtime auto-application would diverge
	// from the compiled program, which leaves it unapplied. Refuse so the
	// program falls back. A dynamic value as the LAST residual is fine —
	// nothing follows it to apply.
	for i := 0; i+1 < len(residual); i++ {
		if residual[i].Dynamic {
			return nil, "dynamic value precedes residual args (fn-value-call boundary)", false
		}
	}
	vi := 0
	tail := []emitOperand{}
	for _, rv := range residual {
		if seq, ok := es.producedBy[rv.ID]; ok {
			if len(tail) > 0 {
				return nil, "residual shape beyond Stage 1 (call result above a literal)", false
			}
			if vi >= len(lw.vm) || lw.vm[vi] != seq {
				return nil, "residual shape beyond Stage 1 (call results reordered)", false
			}
			vi++
			continue
		}
		if IsBareTypeNode(rv) && rv.ID != "" {
			tail = append(tail, emitOperand{constIdx: -1, fromSeq: -1, localSlot: -1, typeIdx: es.internType(rv)})
			continue
		}
		lit, okLit := es.materialise(rv)
		if !okLit {
			return nil, "residual value of unknown provenance", false
		}
		if !isInertConst(lit) {
			return nil, "residual value not statically materialisable", false
		}
		tail = append(tail, emitOperand{constIdx: es.intern(lit), fromSeq: -1, localSlot: -1, typeIdx: -1})
	}
	if vi != len(lw.vm) {
		return nil, "residual shape beyond Stage 1 (unconsumed call results)", false
	}
	for _, op := range tail {
		lw.pushOperand(op, lastPos)
	}

	// Lower the compiled fn units. Tail positions are marked first so
	// the lowering emits TAIL_CALL_USER (frame replacement — the
	// language's tail-call guarantee carries into compiled mode).
	for i, rec := range es.fnRecs {
		if !rec.finished {
			return nil, "fn " + rec.name + " was never compiled", false
		}
		if !rec.generic {
			// Generic instantiations stay out of tail marking — the
			// interpreter's HasGen exclusion, mirrored (plan Stage 4).
			rec.hasOut = markTailCalls(rec.frag, &rec.outOp, rec.hasOut)
		}
		// NParams counts everything the call site pushes — declared
		// params AND hidden capture slots; the VM pops them into frame
		// locals uniformly.
		// Pad the slot→name table to NLocals; body-local iterator slots
		// (added during loop lowering) stay anonymous.
		names := make([]string, rec.numLoc)
		copy(names, rec.locals)
		cf := CompiledFn{Name: rec.name, NParams: rec.nParams + len(rec.caps), NLocals: rec.numLoc, Returns: rec.returns, LocalNames: names}
		flw := &lowerer{es: es, p: p, code: &cf.Code, debug: &cf.Debug, sigIdx: lw.sigIdx, variadic: map[int]bool{}}
		if reason := flw.lowerEvents(rec.frag.events, rec.frag.startSeq); reason != "" {
			return nil, "fn " + rec.name + ": " + reason, false
		}
		switch {
		case rec.hasOut && rec.outOp.fromSeq >= 0:
			if len(flw.vm) != 1 || flw.vm[0] != rec.outOp.fromSeq {
				return nil, "fn " + rec.name + ": body leaves extra values (Stage 3 lowers single-result bodies)", false
			}
			flw.emit(OpRet, 0, rec.pos)
		case rec.hasOut:
			if len(flw.vm) != 0 {
				return nil, "fn " + rec.name + ": body leaves extra values (Stage 3 lowers single-result bodies)", false
			}
			flw.pushOperand(rec.outOp, rec.pos)
			flw.emit(OpRet, 0, rec.pos)
		default:
			// Fully diverging body (every path tail-calls): the RET is
			// unreachable; nothing to emit.
		}
		p.Fns = append(p.Fns, cf)
		_ = i
	}

	lw.p.Consts = es.consts // interning may have grown during reconciliation
	lw.p.Types = es.types
	lw.p.Fallbacks = es.fallbacks
	lw.p.MaxStack = lw.maxDepth
	lw.p.NumLocals = es.units[0].numLocals
	return lw.p, "", true
}

func eventPos(ev emitEvent) SrcPos {
	switch ev.kind {
	case evCall:
		return ev.call.pos
	case evFallback:
		return ev.fb.pos
	case evLoop:
		return ev.loop.pos
	case evCallUser:
		return ev.uc.pos
	}
	return ev.br.pos
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
	vm       []int
	variadic map[int]bool // loop seqs: N runtime values, not one
	loops    []loopCtx
	maxDepth int
}

// pushOperand emits the push for a const, local, or type operand.
func (lw *lowerer) pushOperand(op emitOperand, pos SrcPos) {
	switch {
	case op.localSlot >= 0:
		lw.emit(OpPushLocal, op.localSlot, pos)
	case op.typeIdx >= 0:
		lw.emit(OpPushType, op.typeIdx, pos)
	default:
		lw.emit(OpPushConst, op.constIdx, pos)
	}
	lw.vm = append(lw.vm, -1)
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
				if op.fromSeq > 0 && op.fromSeq <= scopeFloor {
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

func (lw *lowerer) lowerCall(ev *emitEvent) string {
	c := &ev.call
	n := len(c.ops)
	results := []int{}
	for i, op := range c.ops {
		if op.fromSeq >= 0 {
			if lw.variadic[op.fromSeq] {
				return "consumes loop results (Stage 2 loops only feed the program residual)"
			}
			results = append(results, i)
		}
	}
	switch len(results) {
	case 0:
		// Push consts/locals deepest-first so sig position 0 lands on
		// top.
		for i := n - 1; i >= 0; i-- {
			lw.pushOperand(c.ops[i], c.pos)
		}
	case 1:
		ri := results[0]
		if len(lw.vm) == 0 || lw.vm[len(lw.vm)-1] != c.ops[ri].fromSeq {
			return "stack discipline: result operand of " + c.word + " is not on top"
		}
		// The prior result is on top. Push the const operands
		// deepest-first; they land ABOVE the result, so when the
		// result must be sig position 0 (top) a final SWAP restores
		// the layout — only the 2-arg shape is fixable.
		for i := n - 1; i >= 0; i-- {
			if i == ri {
				continue
			}
			lw.pushOperand(c.ops[i], c.pos)
		}
		if ri == 0 && n == 2 {
			lw.emit(OpSwap, 0, c.pos)
			lw.vm[len(lw.vm)-1], lw.vm[len(lw.vm)-2] = lw.vm[len(lw.vm)-2], lw.vm[len(lw.vm)-1]
		} else if ri != n-1 && n > 1 {
			return "operand shape at " + c.word + " needs reordering beyond Stage 1"
		}
	case 2:
		if n != 2 {
			return "operand shape at " + c.word + " beyond Stage 1"
		}
		if len(lw.vm) < 2 {
			return "stack discipline underflow at " + c.word
		}
		top, below := lw.vm[len(lw.vm)-1], lw.vm[len(lw.vm)-2]
		switch {
		case top == c.ops[0].fromSeq && below == c.ops[1].fromSeq:
			// already in layout
		case top == c.ops[1].fromSeq && below == c.ops[0].fromSeq:
			lw.emit(OpSwap, 0, c.pos)
			lw.vm[len(lw.vm)-1], lw.vm[len(lw.vm)-2] = lw.vm[len(lw.vm)-2], lw.vm[len(lw.vm)-1]
		default:
			return "stack discipline: operands of " + c.word + " not adjacent on top"
		}
	default:
		return "operand shape at " + c.word + " beyond Stage 1"
	}
	si, ok := lw.sigIdx[c.sig]
	if !ok {
		lw.p.Sigs = append(lw.p.Sigs, SigRef{Word: c.word, Sig: c.sig})
		si = len(lw.p.Sigs) - 1
		lw.sigIdx[c.sig] = si
	}
	lw.emit(OpCallNative, si, c.pos)
	lw.vm = lw.vm[:len(lw.vm)-n]
	lw.vm = append(lw.vm, ev.seq)
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
	case out.fromSeq >= 0:
		if lw.variadic[out.fromSeq] {
			return "loop results as a branch/body result (Stage 2)"
		}
		if len(lw.vm) != 1 || lw.vm[0] != out.fromSeq {
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
	// result operand at position 0).
	results := []int{}
	for i, op := range uc.ops {
		if op.fromSeq >= 0 {
			if lw.variadic[op.fromSeq] {
				return "loop results as fn args (Stage 3)"
			}
			results = append(results, i)
		}
	}
	switch len(results) {
	case 0:
		for i := n - 1; i >= 0; i-- {
			lw.pushOperand(uc.ops[i], uc.pos)
		}
	case 1:
		ri := results[0]
		if len(lw.vm) == 0 || lw.vm[len(lw.vm)-1] != uc.ops[ri].fromSeq {
			return "stack discipline: fn arg result is not on top (call of " + lw.es.fnRecs[uc.unit].name + ")"
		}
		for i := n - 1; i >= 0; i-- {
			if i == ri {
				continue
			}
			lw.pushOperand(uc.ops[i], uc.pos)
		}
		if ri == 0 && n == 2 {
			lw.emit(OpSwap, 0, uc.pos)
			lw.vm[len(lw.vm)-1], lw.vm[len(lw.vm)-2] = lw.vm[len(lw.vm)-2], lw.vm[len(lw.vm)-1]
		} else if ri != n-1 && n > 1 {
			return "fn arg shape needs reordering beyond Stage 3"
		}
	case 2:
		if n != 2 {
			return "fn arg shape beyond Stage 3"
		}
		if len(lw.vm) < 2 {
			return "stack discipline underflow at fn call"
		}
		top, below := lw.vm[len(lw.vm)-1], lw.vm[len(lw.vm)-2]
		switch {
		case top == uc.ops[0].fromSeq && below == uc.ops[1].fromSeq:
			// already in layout
		case top == uc.ops[1].fromSeq && below == uc.ops[0].fromSeq:
			lw.emit(OpSwap, 0, uc.pos)
			lw.vm[len(lw.vm)-1], lw.vm[len(lw.vm)-2] = lw.vm[len(lw.vm)-2], lw.vm[len(lw.vm)-1]
		default:
			return "stack discipline: fn args not adjacent on top"
		}
	default:
		return "fn arg shape beyond Stage 3"
	}
	if uc.tail {
		lw.emit(OpTailCallUser, uc.unit, uc.pos)
		lw.vm = lw.vm[:len(lw.vm)-n]
		return ""
	}
	lw.emit(OpCallUser, uc.unit, uc.pos)
	lw.vm = lw.vm[:len(lw.vm)-n]
	lw.vm = append(lw.vm, ev.seq)
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
		lw.vm = append(lw.vm, ev.seq)
		lw.note()
		return ""
	case 1:
		op := fb.ins[0]
		if op.fromSeq >= 0 {
			if lw.variadic[op.fromSeq] {
				return "fallback threads a loop result (Stage 5 follow-on)"
			}
			// The computed value is already on top of the simulated
			// stack; OpFallback consumes it as the threaded input.
			if len(lw.vm) == 0 || lw.vm[len(lw.vm)-1] != op.fromSeq {
				return "stack discipline: fallback input is not on top"
			}
		} else {
			// A const / local / type input: materialise it on top first.
			lw.pushOperand(op, fb.pos)
		}
		lw.emit(OpFallback, fb.spanIdx, fb.pos)
		lw.vm = lw.vm[:len(lw.vm)-1]
		lw.vm = append(lw.vm, ev.seq)
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
	if frag == nil || len(frag.events) == 0 || !hasOut || out.fromSeq < 0 {
		return hasOut
	}
	last := &frag.events[len(frag.events)-1]
	switch last.kind {
	case evCallUser:
		if last.seq == out.fromSeq {
			last.uc.tail = true
			return false
		}
	case evBranch:
		if last.seq == out.fromSeq && last.br.constCond == nil && last.br.hasElse {
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
			lw.vm = append(lw.vm, ev.seq)
			lw.note()
		}
		return ""
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
	case br.cond.fromSeq >= 0:
		if lw.variadic[br.cond.fromSeq] {
			return "loop results as a condition (Stage 2)"
		}
		if len(lw.vm) == 0 || lw.vm[len(lw.vm)-1] != br.cond.fromSeq {
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
		lw.vm = append(lw.vm, ev.seq)
		lw.variadic[ev.seq] = true
		lw.note()
		return ""
	}
	jend := -1
	if !fragDiverges(br.then) {
		jend = lw.emit(OpJmp, 0, br.pos)
	}
	(*lw.code)[jf].Arg = int32(len(*lw.code))
	elsOut := func() *emitOperand {
		if br.hasElsOut {
			return &br.elsOut
		}
		return nil
	}()
	if reason := lw.lowerFragment(br.els, elsOut, br.pos); reason != "" {
		return reason
	}
	if jend >= 0 {
		(*lw.code)[jend].Arg = int32(len(*lw.code))
	}
	if br.hasThenOut || br.hasElsOut {
		lw.vm = append(lw.vm, ev.seq)
		lw.note()
	}
	return ""
}

func itoaSmall(n int) string {
	if n >= 0 && n < 10 {
		return string(rune('0' + n))
	}
	return "many"
}
