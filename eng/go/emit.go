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

// fnIntrospectionWords READ a fn value (its type, arity, or type-algebra over
// it) and never INVOKE it — so a fn-value operand may bake as an inert const
// the handler inspects (`typeof (f/r)`, `Positive tcmp Function`). Words that
// invoke a fn value (apply, the higher-order forms, and `is` over a predicate
// fn, whose handler applies the predicate) are deliberately EXCLUDED: their
// handlers re-step the fn on the tape, which the VM cannot honour.
var fnIntrospectionWords = map[string]bool{
	"typeof": true, "inspect": true,
	"tcmp": true, "teq": true, "tand": true, "tor": true, "tnot": true,
}

// inertOperandWords are words whose NoEvalArgs clause/path list or QuoteArgs
// atom is inert DATA the handler consumes directly — parsed into SQL, decoded
// into a lens, read as an error code — never AQL code re-stepped on the tape.
// The operand therefore bakes as a const and the dispatch lowers to a plain
// CALL_NATIVE running the unchanged handler, so they are exempt from the blanket
// code-body and quoted-operand refusals (like the get/set field-name exemption).
// A list operand bakes even when it holds Words / paren-exprs, which the general
// isInertConst rejects as code.
var inertOperandWords = map[string]bool{
	// aql:query DSL — clause lists (column/expr specs) + bare table names.
	"select": true, "where": true, "order": true, "group": true,
	"having": true, "on": true, "using": true,
	"from": true, "join": true, "innerjoin": true, "leftjoin": true, "crossjoin": true,
	// reach — an inert lens path: field names, `!` strict markers, and raw
	// computed `(expr)` segments (stored unevaluated, applied later by the lens).
	"reach": true,
	// raise — the error-code atom (`raise bad_input "…"`).
	"raise": true,
}

// operandKind discriminates how an emitOperand sources its value. The kind
// is an explicit enum rather than a set of "-1 means unset" int fields so the
// struct's ZERO VALUE is the unambiguous opNone (an invalid operand, only ever
// returned paired with an ok=false flag) — never a valid-looking "const index
// 0." See eng/go/CLAUDE.md "No Zero-Value Overload (CRITICAL)": four parallel
// sentinel fields are exactly the smell that rule forbids, and a single missed
// initialization used to mean "Consts[0]" silently.
type operandKind uint8

const (
	opNone    operandKind = iota // zero value: unset / invalid (ok=false only)
	opConst                      // idx → Program.Consts index
	opEvent                      // idx → producing event sequence number
	opLocal                      // idx → frame-local slot (loop iterator / param)
	opType                       // idx → Program.Types index (canonical type)
	opClosure                    // closureUnit + closureCaps (a compiled body)
)

// emitOperand names where one dispatch argument comes from. For every kind but
// opClosure the single idx field carries the kind-specific index/seq; the
// constructors below are the only construction sites for the indexed kinds, so
// the kind↔idx pairing can never drift.
//
// resIdx is meaningful only for opEvent (P5 multi-result lowering): it names
// WHICH of the producing event's results this operand is. A single-result
// event uses resIdx 0; a multi-result call (e.g. `swap`, a multi-return fn)
// distinguishes its N outputs by 0..N-1, matching the order the VM pushes the
// handler's results (results[0] deepest, results[N-1] on top).
type emitOperand struct {
	kind   operandKind
	idx    int
	resIdx int
	// closureUnit indexes Program.Fns and lowers to OpPushClosure (plan P2);
	// closureCaps are the body's lexical captures, resolved in the ENCLOSING
	// scope and pushed (in CapturedBinding order) just before OpPushClosure,
	// which pops them into the closure value. Both are meaningful only when
	// kind == opClosure.
	closureUnit int
	closureCaps []emitOperand
}

// constOperand / eventOperand / localOperand / typeOperand build the indexed
// operand kinds — the only places that pair a kind with its idx. eventOperand
// additionally carries the result index within the producing event (P5).
func constOperand(idx int) emitOperand { return emitOperand{kind: opConst, idx: idx} }
func eventOperand(seq, resIdx int) emitOperand {
	return emitOperand{kind: opEvent, idx: seq, resIdx: resIdx}
}
func localOperand(slot int) emitOperand { return emitOperand{kind: opLocal, idx: slot} }
func typeOperand(idx int) emitOperand   { return emitOperand{kind: opType, idx: idx} }

// producer locates a recorded value: the producing event's seq and which of
// that event's results it is (idx 0 for the common single-result case). P5
// generalised producedBy from a bare seq so a multi-result call's N outputs
// stay distinguishable when a downstream operand resolves one of them.
type producer struct{ seq, idx int }

type emitCall struct {
	word string
	sig  *Signature
	ops  []emitOperand
	nout int // number of results the call pushes (0 for a side-effect word, N for multi-result)
	pos  SrcPos
	poly bool // dispatch via OpCallNativePoly (runtime MatchSignature)
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
	hasThenOut, hasElsOut bool        // false when the arm DIVERGES (ends in break/continue)
	elsIsVal              bool        // else arm is a plain VALUE operand (not a body fragment)
	elsVal                emitOperand // the value-else operand (const/local/type, OR a COMPUTED event when elsComputed) when elsIsVal
	elsComputed           bool        // else value is a COMPUTED event eagerly on the stack below the cond (`if c [t] (expr)`): SWAP cond up, DROP it on the taken path
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
// unit index, the args in sig order, the number of results the unit
// returns (0 for a side-effect / 0-return fn, N for a multi-return
// fn), and whether the lowering marked it a TAIL call (it then
// replaces the frame and control never returns to the site).
type emitUserCall struct {
	unit int
	ops  []emitOperand
	nout int
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
	producedBy map[string]producer // value ID → producing (event seq, result idx)
	zeroOutSeq map[int]bool        // branch seq → 0-output statement guard (residual skips it)
	typeOut    map[int]bool        // event seq → its output is itself a type body
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
	name    string
	nParams int
	caps    []CapturedBinding
	generic bool
	returns []*Type  // declared return types — enforced at the VM's RET
	locals  []string // slot→name table (params then captures); debug only
	frag    *EmitFragment
	// outOps are the body's residual operands in stack order (bottom→top):
	// the values the unit leaves for its caller. Empty for a 0-result body
	// OR a diverging body (every path tail-calls) — fragDiverges(frag)
	// distinguishes them (a diverging body emits no RET; a 0-result body
	// emits a bare RET).
	outOps   []emitOperand
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
		producedBy: map[string]producer{},
		zeroOutSeq: map[int]bool{},
		typeOut:    map[int]bool{},
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

// setProduced registers an event's output ID against its sequence,
// recording whether that output is itself a type body (a bare type
// node, or a structural/nominal type literal like an ObjectType). The
// flag lets resolveOperand tell a real type-producing event (typeof,
// type algebra) apart from an ID COLLISION — a `make` result is an
// ObjectInstance that can inherit the type literal's ID, so a later
// type operand (`(make Point {}) is Point`) would otherwise resolve
// the `Point` literal to the make event.
func (es *EmitState) setProduced(out Value, seq int) {
	es.setProducedAt(out, seq, 0)
}

// setProducedAt is setProduced for the idx-th result of a multi-result event
// (P5). idx 0 is the single-result case. When two outputs share an ID — a
// stack word like `dup` returns `[args[0], args[0]]`, both the same Value —
// the LAST registration wins; the lowerer's operand layout then refuses the
// ambiguous consume (sound: the program falls back) until carrier-identity
// (the next runtime-independence item) mints distinct ids.
func (es *EmitState) setProducedAt(out Value, seq, idx int) {
	es.producedBy[out.ID] = producer{seq: seq, idx: idx}
	if IsTypeBody(out) {
		es.typeOut[seq] = true
	}
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
	if pr, ok := es.producedBy[v.ID]; ok {
		// A type operand whose ID matches a producing event whose own
		// output was NOT a type is an ID collision (a `make` result
		// inheriting the type literal's ID): resolve it as its own type
		// operand / const below, not the unrelated event.
		if !IsTypeBody(v) || es.typeOut[pr.seq] {
			return eventOperand(pr.seq, pr.idx), true
		}
	}
	if slot, ok := es.units[len(es.units)-1].localByID[v.ID]; ok {
		return localOperand(slot), true
	}
	// A bare type node is a TYPE operand: it must reach the runtime
	// as the CANONICAL registry node (a pooled by-value copy goes
	// stale against behaviour/field installs), so it gets its own
	// table, resolved by ID at run time via OpPushType.
	if IsBareTypeNode(v) && v.ID != "" {
		return typeOperand(es.internType(v)), true
	}
	lit, ok := es.materialise(v)
	if !ok || !isInertConst(lit) {
		return emitOperand{}, false
	}
	return constOperand(es.intern(lit)), true
}

// OperandRepushable reports whether v resolves to a FREELY RE-PUSHABLE
// operand — a const, a frame local, or a (canonical) type node — as opposed
// to a computed EVENT result (on the simulated stack exactly once) or an
// unresolvable value. It mirrors resolveOperand's decision but is SIDE-
// EFFECT FREE (no interning), so a caller can classify an operand WITHOUT
// recording. Used by a multi-reference desugar (the `case` value is tested
// against every clause guard) to decide up front whether it can compile —
// avoiding a probe whose rollback would otherwise pollute the recording.
func (es *EmitState) OperandRepushable(v Value) bool {
	if es == nil {
		return false
	}
	// An event operand is the value's stack-discipline truth (pushed once);
	// it cannot be re-pushed for a second reference.
	if pr, ok := es.producedBy[v.ID]; ok && (!IsTypeBody(v) || es.typeOut[pr.seq]) {
		return false
	}
	if _, ok := es.units[len(es.units)-1].localByID[v.ID]; ok {
		return true
	}
	if IsBareTypeNode(v) && v.ID != "" {
		return true
	}
	lit, ok := es.materialise(v)
	return ok && isInertConst(lit)
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
	ElsValue        *Value // non-nil: the else arm is this already-evaluated VALUE, not a body
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
			// A 0-value arm (an empty `[]`, a 0-value word, or a raise that
			// fragDiverges doesn't classify): no merge value, like a diverging
			// arm. When the SIBLING arm nets a value the branch is VARIADIC
			// (0-or-1) — lowerArms marks it and only the program residual
			// absorbs it; when BOTH arms net 0 the caller refuses below.
			return emitOperand{}, false, true
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
	zeroOut := false
	if b.ConstCond != nil {
		out, has, ok := resolveArm(b.Then, b.ThenStk, "taken")
		if !ok {
			return
		}
		ev.br.then, ev.br.thenOut, ev.br.hasThenOut = b.Then, out, has
	} else if !b.HasElse && (len(b.ThenStk) == 0 || fragDiverges(b.Then)) {
		// 2-arg if (no else) whose then produces 0 values — a 0-value word
		// (raise/set/printstr) or a diverging arm (break/continue/raise): the
		// if produces 0 values on BOTH paths (true→0/diverge, false→0), so it
		// is a statement guard with NO merge value. Record the then for
		// lowering (its body still runs on the true path) and mark the event
		// zeroOut: the lowerer emits no slot, and Finalize skips the (phantom
		// None) result it still registers below — the registration is kept so
		// RecordCall's double-record guard still elides this if dispatch.
		if b.Then == nil {
			es.MarkUncompilable("if: then-branch not captured")
			return
		}
		ev.br.then, ev.br.hasThenOut = b.Then, false
		zeroOut = true
	} else {
		thenOut, hasThen, ok := resolveArm(b.Then, b.ThenStk, "then")
		if !ok {
			return
		}
		ev.br.then, ev.br.thenOut, ev.br.hasThenOut = b.Then, thenOut, hasThen
		if b.HasElse {
			if b.ElsValue != nil {
				// Value-else: the else arm is an already-evaluated value
				// (`if cond [then] 42`), not a `[…]` body. It lowers to a
				// single push in the else arm. A COMPUTED else (an event
				// result of a paren like `(add 1 2)`) is eagerly on the
				// stack BEFORE the branch — that needs stack juggling the
				// current single-result lowering doesn't do, so refuse it.
				op, ok := es.resolveOperand(*b.ElsValue)
				if !ok {
					es.MarkUncompilable("if: else value of unknown provenance")
					return
				}
				if op.kind == opEvent {
					// A COMPUTED else value (`if cond [then] (add 1 2)`) is
					// eagerly on the stack BELOW the cond event. The lowerer
					// SWAPs the cond to the top, branches, and DROPs the else on
					// the taken path (it survives on the false path as the
					// result). Only the plain-event-cond layout [cond, elseVal]
					// is handled; a const / condFrag / const-cond condition sits
					// elsewhere, so refuse those (unchanged).
					if b.ConstCond != nil || b.CondFrag != nil || ev.br.cond.kind != opEvent {
						es.MarkUncompilable("if: computed else value with non-stack condition (Stage 2)")
						return
					}
					ev.br.elsIsVal, ev.br.elsVal, ev.br.hasElsOut, ev.br.elsComputed = true, op, true, true
				} else {
					ev.br.elsIsVal, ev.br.elsVal, ev.br.hasElsOut = true, op, true
				}
			} else {
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
	}
	seq := es.appendEvent(ev)
	es.SiteCounts[SiteMono]++
	es.setProduced(b.Out, seq)
	if zeroOut {
		// The if produces 0 runtime values; its registered (None) result is a
		// phantom. Mark the seq so Finalize's residual reconciliation skips it
		// rather than expecting a stack slot. Keeping the setProduced above
		// lets RecordCall's double-record guard elide this if dispatch.
		es.zeroOutSeq[seq] = true
	}
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

// emitCheckpoint snapshots the append-only recording pools and counters so a
// DISCARDED loop-analysis round can be rolled back. AnalyseLoopBody re-runs a
// loop body to a binding fixed point but keeps only the final (stabilised)
// round's fragment; without rollback the earlier rounds' interned consts and
// island fallback spans orphan into the Program and their dispatches inflate
// SiteCounts (the metric surfaced via CheckResult.SiteCounts). The snapshot is
// by LENGTH for the slice pools (intern/internType/RecordFallback only append)
// and by VALUE for the small SiteCounts map.
type emitCheckpoint struct {
	seq        int
	consts     int
	types      int
	fallbacks  int
	fnRecs     int
	siteCounts map[string]int
}

// Checkpoint captures the rollback point. Nil-safe (returns a zero checkpoint
// that Rollback ignores via the nil-receiver guard at its call site).
func (es *EmitState) Checkpoint() emitCheckpoint {
	if es == nil {
		return emitCheckpoint{}
	}
	sc := make(map[string]int, len(es.SiteCounts))
	for k, v := range es.SiteCounts {
		sc[k] = v
	}
	return emitCheckpoint{
		seq:        es.seq,
		consts:     len(es.consts),
		types:      len(es.types),
		fallbacks:  len(es.fallbacks),
		fnRecs:     len(es.fnRecs),
		siteCounts: sc,
	}
}

// Rollback discards everything recorded since cp — used to drop a non-final
// loop-analysis round so only the stabilised round lands in the Program.
//
// SAFETY: a round that compiled a fn UNIT (a closure/island body via
// StartFnCompile) cannot be unwound — the unit's lowered code references shared
// const/type indices, so trimming the pools would dangle them. That shape (a
// fn body inside a loop that ALSO needs multiple fixed-point rounds) is rare;
// for it we keep the round's recording (the prior behaviour: a little bloat,
// always correct) rather than risk corrupting an emitted unit.
func (es *EmitState) Rollback(cp emitCheckpoint) {
	if es == nil || len(es.fnRecs) != cp.fnRecs {
		return
	}
	for k, i := range es.constIdx {
		if i >= cp.consts {
			delete(es.constIdx, k)
		}
	}
	es.consts = es.consts[:cp.consts]
	for k, i := range es.typeIdx {
		if i >= cp.types {
			delete(es.typeIdx, k)
		}
	}
	es.types = es.types[:cp.types]
	es.fallbacks = es.fallbacks[:cp.fallbacks]
	// Provenance entries minted after cp belong to the discarded round's
	// carriers (fresh ids never referenced again); drop them, mirroring
	// StartFnCompile's fn-unit cleanup so the live map stays tight.
	for id, pr := range es.producedBy {
		if pr.seq > cp.seq {
			delete(es.producedBy, id)
		}
	}
	es.SiteCounts = cp.siteCounts
	es.seq = cp.seq
	es.captured = nil
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
func (es *EmitState) StartFnCompile(key, name string, args []Value, declared []*Type, paramNames []string, captures []CapturedBinding, generic bool, pos SrcPos) (unit int, finish func([]Value), ok bool) {
	if !es.active() {
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
	rec := &fnUnitRec{name: name, nParams: len(args), caps: captures, generic: generic, returns: declared, locals: locals, pos: pos}
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
		if !fragDiverges(rec.frag) {
			// A DECLARED fn must leave exactly len(returns) values: a
			// different count is the return-COUNT error the interpreter
			// raises, so refuse and let the program fall back. An UNDECLARED
			// fn (an anonymous lambda whose Returns were nilled, or a
			// 0-return fn) is NOT count-checked by the interpreter, so its
			// body residual — 0 or N values — is taken as-is.
			if len(rec.returns) > 0 && len(bodyStk) != len(rec.returns) {
				es.MarkUncompilable("fn " + name + ": body value count differs from declared returns")
				return
			}
			// Resolve every residual value to an operand, in stack order
			// (bottom→top), so the unit leaves the body's N results for its
			// caller. A 0-result body leaves outOps empty (a bare RET).
			ops := make([]emitOperand, len(bodyStk))
			for i, v := range bodyStk {
				op, okOut := es.resolveOperand(v)
				if !okOut {
					es.MarkUncompilable("fn " + name + ": body result of unknown provenance")
					return
				}
				ops[i] = op
			}
			rec.outOps = ops
		}
		// The unit is closed: its events' outputs cannot be stack
		// values in the enclosing scope, so drop their provenance
		// entries (after resolving the body result above, which DOES
		// reference them). Without this, a join inside the body that
		// reused a capture/param ID (JoinCarriers keeps the then-side
		// ID) would make an ENCLOSING call site resolve that value to
		// an event of this closed unit.
		//
		// O(live-IDs) scan per fn finish — i.e. O(fns × live-IDs) over a
		// compile. Negligible at current fn densities; revisit (e.g. track
		// per-unit produced IDs) if deeply nested fn-heavy programs make
		// compile time bite.
		for id, pr := range es.producedBy {
			if pr.seq > rec.frag.startSeq {
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
// signature order; outs are the dispatch's result carriers (0 for a
// 0-return fn, 1 for the common single-return case, N for a multi-
// return fn — registered idx 0..N-1, matching the order the VM's
// CALL_USER leaves them on the stack). The unit's captures ride as
// hidden trailing operands, resolved in the CALLER's scope: at the
// construction site they are the enclosing frame's values; inside the
// fn's own body (recursion) they resolve to the frame's own capture
// slots and re-flow unchanged. A capture unreachable from the call
// site marks the program uncompilable — the interpreter keeps owning
// that shape.
func (es *EmitState) RecordUserCall(unit int, args []Value, outs []Value, pos SrcPos) {
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
	seq := es.appendEvent(emitEvent{kind: evCallUser, uc: emitUserCall{unit: unit, ops: ops, nout: len(outs), pos: pos}})
	es.SiteCounts[SiteMono]++
	for i := range outs {
		es.setProducedAt(outs[i], seq, i)
	}
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
	if startOp.kind != opConst || stepOp.kind != opConst {
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
	es.setProduced(out, seq)
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
	es.setProduced(out, seq)
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

// RememberOriginal records a CONCRETE value produced during the check
// pass against its own ID, so that when a later carrier-strip reduces it
// (preserving the ID — toCarrier keeps Value.ID) the lowerer can recover
// the original via materialise/origByID. Used by impure-but-pure-data
// constructors that run in check mode and whose result is otherwise
// stripped before reaching a downstream operand — notably the predicate
// constructors (`Integer gt 10`), whose DepScalarInfo payload toCarrier
// strips to a bare base-type carrier, losing the bound.
func (es *EmitState) RememberOriginal(v Value) {
	if es == nil || es.suspended > 0 || !es.active() {
		return
	}
	if v.Carrier || v.Dynamic || v.ID == "" {
		return
	}
	es.origByID[v.ID] = v
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
// forceDynOut bypasses the dynamic-output refusal when the caller
// (dynOutNativeOK) has proven the dispatch is a concrete-args core builtin
// whose dynamic result is merely a declared-Any return.
func (es *EmitState) RecordCall(word string, sig *Signature, args, outs []Value, pos SrcPos, forceDynOut bool) {
	if !es.active() {
		return
	}
	// A dispatch whose output is already registered was recorded by a
	// structured hook (RecordBranch owns the `if` dispatch; a user-fn
	// ReturnsFn owns its RecordUserCall — including multi-return calls) —
	// the generic path must not double-record or refuse it. A structured
	// hook registers all results together, so checking the first suffices.
	if len(outs) > 0 {
		if _, ok := es.producedBy[outs[0].ID]; ok {
			return
		}
	}
	// `apply` of a fn VALUE (`…args fn apply`): apply's ReturnsFn returns the
	// fn concrete, so the check engine RE-STEPS it — the fn then dispatches
	// against its preceding stack args and records as an ordinary CALL_USER.
	// Elide apply's own dispatch (it produces nothing the VM runs); without
	// this the generic path below refuses it as "function value reaches apply".
	// The Reach-apply sig (a TReach operand, not an FnDef) is untouched.
	if word == "apply" && len(args) >= 1 {
		if _, ok := args[0].Data.(FnDefInfo); ok {
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
	case len(sig.NoEvalArgs) > 0 && !inertOperandWords[word]:
		es.SiteCounts[SiteMeta]++
		es.MarkUncompilable("code-body word " + word + " (Stage 2)")
		return
	case len(sig.QuoteArgs) > 0 && word != "get" && word != "getr" && word != "set" && !inertOperandWords[word]:
		// Implicit-quote operands (usurp, force-arity, ref-family):
		// dispatch-manipulating meta words whose results the engine
		// re-steps. get/getr/set are exempt — plain accessors/mutators whose
		// quoted key is an inert Atom const (its fn-valued module-resolution
		// case is elided above; a dynamic or fn-valued result still refuses via
		// the later cases). For `set` the quoted key is the atom field name of
		// an object/class/store/flex field write (`p set x 7`); the receiver is
		// a non-const instance (mutation-safety holds — instance types are
		// absent from isInertConst, exactly as the integer-keyed array `set 1 v
		// a` already relies on), and `set` cannot be shadowed (it is a builtin),
		// so the word-name match admits only the real mutator, never a usurp.
		es.SiteCounts[SiteMeta]++
		es.MarkUncompilable("quoted-operand word " + word)
		return
	case anyDynamicCarrier(args):
		es.SiteCounts[SiteDynamic]++
		es.MarkUncompilable("dynamic input at " + word)
		return
	case anyDynamicCarrier(outs) && !forceDynOut:
		// Dynamic outputs mean the checker could not type the word
		// (missing annotations, opaque wrappers like a def-bound
		// usurp value): the recorded signature is a best guess, not a
		// proof — don't bake it in. forceDynOut (dynOutNativeOK) is the
		// exception: a CONCRETE-args core builtin whose dynamic result is a
		// declared-Any return bakes faithfully and falls through here.
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
	}
	// P5 multi-result lowering: a 0-result side-effect word (set/raise/drop/
	// printstr/sleep…) or a genuine N-result word records like any call — the
	// VM's OpCallNative already pushes every handler result, so no VM change is
	// needed. MUTATION-SAFETY (load-bearing, see isInertConst): the only
	// 0-result words are in-place mutators on Array/Object/Store/context
	// receivers, and those instance types are NOT const-bakeable (absent from
	// isInertConst's whitelist), so a pooled compound const can never reach one
	// — the receiver is always a computed event or a frame local.
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
	introspect := fnIntrospectionWords[word]
	for _, a := range args {
		if _, ok := a.Data.(FnDefInfo); ok {
			// An INTROSPECTION word READS a fn value (its type/arity) and never
			// invokes it, so the immutable fn value rides as a plain const
			// operand the handler inspects — unlike a fn-INVOKING word (apply,
			// higher-order, or `is` over a predicate fn), whose handler re-steps
			// the fn on the tape, which the VM cannot honour.
			if introspect {
				continue
			}
			es.SiteCounts[SiteMeta]++
			es.MarkUncompilable("function value reaches " + word + " (Stage 3)")
			return
		}
	}
	ops := make([]emitOperand, len(args))
	for i, a := range args {
		op, ok := es.resolveOperand(a)
		if !ok && introspect && IsConcrete(a) {
			if _, isFn := a.Data.(FnDefInfo); isFn {
				// Bake the concrete (immutable) fn value as a const the
				// introspection handler reads at run time.
				op, ok = constOperand(es.intern(a)), true
			}
		}
		if !ok && inertOperandWords[word] && IsConcrete(a) && a.Parent.ConformsTo(TList) && !listHasParenExpr(a) {
			// An inert operand list (query column/expr spec, reach path) is DATA
			// the handler consumes directly — never re-stepped — so it bakes as a
			// const even though it holds Words (which the general isInertConst
			// rejects as code). Compounds are never pooled, so each keeps its own
			// const slot. A list holding a ParenExpr is EXCLUDED: that is deferred
			// code (a reach computed segment `(k)`) needing the live def scope at
			// evaluation time, which a baked const cannot reach — so it falls back.
			op, ok = constOperand(es.intern(a)), true
		}
		if !ok && (introspect || word == "is") && isBuiltinStructuralType(a) {
			// A STRUCTURAL type operand (`[Integer]`, `{a:Integer}`) that a
			// type-reading word matches against. isInertConst rejects its
			// type-literal members, but a BUILTIN type literal's Parent is the
			// canonical package-level *Type, so baking by value is sound (no
			// behave-staleness). `5 is Integer` already lowers via OpPushType;
			// this is its structural counterpart.
			op, ok = constOperand(es.intern(a)), true
		}
		if !ok {
			es.MarkUncompilable("operand of unknown provenance or not statically materialisable at " + word)
			return
		}
		ops[i] = op
	}
	es.SiteCounts[SiteMono]++
	seq := es.appendEvent(emitEvent{kind: evCall, call: emitCall{word: word, sig: sig, ops: ops, nout: len(outs), pos: pos}})
	for i := range outs {
		es.setProducedAt(outs[i], seq, i)
	}
}

// RecordPolyCall records a native dispatch the checker could not commit to
// one overload for (a dynamic operand widened to Any): the call lowers to
// OpCallNativePoly, which re-matches the word's signatures at run time (plan
// P3). Operands resolve normally (the dynamic one is a prior event's result);
// returns false, leaving es untouched, when one is of unknown provenance.
func (es *EmitState) RecordPolyCall(word string, args, outs []Value, pos SrcPos) bool {
	if !es.active() || len(outs) != 1 {
		return false
	}
	ops := make([]emitOperand, len(args))
	for i := range args {
		op, ok := es.resolveOperand(args[i])
		if !ok {
			return false
		}
		ops[i] = op
	}
	es.SiteCounts[SiteDynamic]++
	seq := es.appendEvent(emitEvent{kind: evCall, call: emitCall{word: word, ops: ops, nout: 1, pos: pos, poly: true}})
	es.setProduced(outs[0], seq)
	return true
}

// RecordClosureCall records a higher-order word's dispatch where the code BODY
// at position bodyPos was compiled to closure unit `unit` (plan P2). The body
// operand lowers to OpPushClosure (the handler invokes it through the VM via
// the InvokeBody seam); the other operands resolve normally. Returns false,
// leaving es UNTOUCHED, when an operand is dynamic or of unknown provenance —
// the caller then keeps the island path.
func (es *EmitState) RecordClosureCall(word string, sig *Signature, args []Value, bodyPos, unit int, capOps []emitOperand, outs []Value, pos SrcPos) bool {
	if !es.active() || sig == nil || len(outs) != 1 {
		return false
	}
	// A DYNAMIC non-body operand can't resolve; refuse. A dynamic OUTPUT is
	// fine — the body is compiled and the data operand concrete, so the
	// dispatch is faithful; the result type being Any only means a downstream
	// typed dispatch over it polys or refuses (e.g. filter's element-typed
	// result). So check inputs, not the output.
	for i := range args {
		if i != bodyPos && (args[i].Carrier && args[i].Dynamic) {
			return false
		}
	}
	ops := make([]emitOperand, len(args))
	for i := range args {
		if i == bodyPos {
			ops[i] = emitOperand{kind: opClosure, closureUnit: unit, closureCaps: capOps}
			continue
		}
		op, ok := es.resolveOperand(args[i])
		if !ok {
			return false
		}
		ops[i] = op
	}
	es.SiteCounts[SiteMono]++
	seq := es.appendEvent(emitEvent{kind: evCall, call: emitCall{word: word, sig: sig, ops: ops, nout: 1, pos: pos}})
	es.setProduced(outs[0], seq)
	return true
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
	if _, isFn := v.Data.(FnDefInfo); isFn {
		// A fn value (introspection operand): never pool — CanonValue is not a
		// reliable identity key for fn bodies, so dedup could merge distinct
		// fns. Each bakes as its own const.
		es.consts = append(es.consts, v)
		return len(es.consts) - 1
	}
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
		// default is a TEMPLATE `make` deep-copies per instance: a required
		// field's type node, an inert const, a clean nested type body, OR any
		// concrete data default — a flex container, a class/object instance, an
		// array (`{items:(flex [])}`, `{i:(make Inner {})}`, materialised at
		// schema construction). The const template is only ever READ (copied),
		// never mutated, so a mutable default is sound; the per-instance COPY
		// isolation is pinned by class.tsv:112. A METHOD field (fn value) is NOT
		// data and still refuses (the surface-body case falls back). The
		// canonical *Type rides the body payload pointer, staying canonical.
		fields := d.AllFields()
		if fields == nil {
			return false
		}
		for _, k := range fields.Keys() {
			fv, _ := fields.Get(k)
			if memberOK(fv) || isSchemaDefaultOK(fv) {
				continue
			}
			return false
		}
		return true
	}
	return false
}

// isSchemaDefaultOK reports whether v may ride as a class/object field DEFAULT
// in a const-baked schema: a concrete data template `make` deep-copies per
// instance. A fn value (a method field) is excluded — it is code, not data, and
// keeps the schema on the fallback path.
func isSchemaDefaultOK(v Value) bool {
	if !IsConcrete(v) {
		return false
	}
	if _, isFn := v.Data.(FnDefInfo); isFn {
		return false
	}
	return true
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
//
// MUTATION-SAFETY (load-bearing): a pooled const is pushed by the SAME
// Value — and thus the same pointer-backed payload — on every execution,
// including each iteration of a loop, so it must never reach a word that
// mutates it in place. That holds because the whitelist admits only
// immutable shapes: the mutable containers (Array/Object/Store instances)
// are absent, so their constructors never produce an inert const. The
// in-place mutators (Array/Object/Store `set`) return 0 values and DO now
// compile (P5 multi-result lowering), but their receiver is always a
// computed event (the constructor) or a frame local — never a pooled const,
// precisely because those instance types are absent here. Map/List `set` is
// copy-returning. Keep this whitelist free of mutable instance types: adding
// one would let a pooled compound const reach an in-place mutator and corrupt
// it across iterations.
// isBuiltinStructuralType reports whether v is a structural TYPE literal whose
// every leaf is a BUILTIN type node — `[Integer]`, `[Integer String]`,
// `{a:Integer}`, and nestings thereof. Such a value is sound to bake by value
// for a type-reading word (`is`, `typeof`, the type-algebra words): a builtin
// type literal's Parent is the canonical package-level *Type pointer, which is
// stable and never retired, so no canonical-pointer staleness arises (eng
// CLAUDE.md "Canonical *Type Pointers"). USER-type leaves are excluded — their
// Behavior can be mutated later via `behave`, which a by-value const would not
// see — so those structural operands stay refused.
func isBuiltinStructuralType(v Value) bool {
	if v.Carrier || v.Dynamic {
		return false
	}
	switch d := v.Data.(type) {
	case ListPayload:
		if len(d.Elems) == 0 {
			return false
		}
		for _, e := range d.Elems {
			if !isBuiltinTypeLeaf(e) {
				return false
			}
		}
		return true
	case MapPayload:
		if d.M == nil || d.M.Len() == 0 {
			return false
		}
		for _, k := range d.M.Keys() {
			mv, _ := d.M.Get(k)
			if !isBuiltinTypeLeaf(mv) {
				return false
			}
		}
		return true
	}
	return false
}

// isBuiltinTypeLeaf reports whether v is a builtin bare type node or a nested
// builtin structural type — the per-leaf rule of isBuiltinStructuralType.
func isBuiltinTypeLeaf(v Value) bool {
	if IsBareTypeNode(v) {
		// A bare type literal IS its lattice node (typeNodeOf), not its Parent;
		// a builtin node has a stable canonical package-level pointer.
		return typeNodeOf(v).IsNative()
	}
	return isBuiltinStructuralType(v)
}

// listHasParenExpr reports whether a concrete list holds a ParenExpr element —
// deferred code that needs the live def scope when evaluated (a reach computed
// segment `(k)`), so the list is NOT inert and must not bake into the const pool.
func listHasParenExpr(v Value) bool {
	lst, err := AsList(v)
	if err != nil || lst.IsNil() {
		return false
	}
	for i := 0; i < lst.Len(); i++ {
		if IsParenExpr(lst.Get(i)) {
			return true
		}
	}
	return false
}

// isInertReach reports whether a Reach lens value is fully inert and so safe to
// bake as a const: every segment is a literal field-name key (no computed
// `(expr)` segment, which is deferred code needing the live def scope), and the
// receiver (if any) is itself a const. A field-name Word key counts as inert
// data here — the lens handler reads it as a key, never re-steps it.
func isInertReach(ri ReachInfo) bool {
	for _, seg := range ri.Segments {
		if seg.Computed {
			return false
		}
		if _, isWord := seg.KeyLit.Data.(WordInfo); isWord {
			continue // a field-name Word is inert lens data
		}
		if !isInertConstMember(seg.KeyLit) {
			return false
		}
	}
	for _, rv := range ri.Receiver {
		if !isInertConstMember(rv) {
			return false
		}
	}
	return true
}

func isInertConst(v Value) bool {
	if v.Carrier || v.Dynamic || IsBareTypeNode(v) {
		return false
	}
	switch d := v.Data.(type) {
	case IntPayload, FloatPayload, StrPayload, BoolPayload, AtomPayload,
		PathPayload, NonePayload, BigIntPayload, DecimalPayload,
		TimePayload, DurationPayload, TimezonePayload:
		return true
	case DepScalarInfo:
		// A predicate / refinement type (`Integer gt 10`): self-contained
		// (base family + bound, no registry, no canonical-pointer hazard per
		// eng CLAUDE.md), so the value bakes by value into the const pool.
		// The bound is recovered for a stripped operand via origByID
		// (RememberOriginal at the constructor); type-algebra words
		// (tcmp/teq/tand/…) then run over the baked predicate at run time.
		_ = d
		return true
	case ReachInfo:
		// An inert lens VALUE (`$.name`, `$.a.b`, a constructed reach): the
		// segments are field-name keys the apply/get/set handlers read as data
		// and the receiver (if any) is a const, so the whole lens bakes. A
		// COMPUTED `(expr)` segment is deferred code needing the live def scope at
		// apply time (it would go stale in a const), so it disqualifies the lens.
		return isInertReach(d)
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
			if !isInertConstMember(e) {
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
			if !isInertConstMember(mv) {
				return false
			}
		}
		return true
	default:
		return false
	}
}

// isInertConstMember reports whether v may ride as a MEMBER of a const
// compound (a list element or map field): an inert const, OR a fn VALUE. A fn
// value is immutable code, so it is safe inside a READ-ONLY const container — a
// method field of a data map (`{f: fn}`), the receiver of `m.f`. It is admitted
// only as a member, never as a standalone const: the top-level isInertConst
// switch still rejects a bare FnDefInfo (a top-level fn value is the apply /
// closure case, not bakeable data). At run time a poly `get` of the field
// returns the fn, which the fn-value-call boundary (OpCallDynamic) applies.
func isInertConstMember(v Value) bool {
	if !v.Carrier && !v.Dynamic {
		if fd, ok := v.Data.(FnDefInfo); ok {
			// Only an UNNAMED (inline `fn […]`) fn value. A named ref (`f/r`,
			// Name="f") re-dispatches by NAME when applied through the island
			// sub-engine, where forward collection of the trailing arg differs
			// from the interpreter — that diverges, so keep named refs
			// non-const (they refuse and fall back faithfully).
			return fd.Name == ""
		}
	}
	return isInertConst(v)
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
	// Value-def locals: a top-level computed result referenced more than once
	// (counting the program residual) is promoted to a frame local so the
	// single-consume stack discipline holds. Count the residual references,
	// then plan + rewrite before lowering.
	var residualSeqs []int
	for _, rv := range residual {
		if pr, ok := es.producedBy[rv.ID]; ok && pr.idx == 0 {
			residualSeqs = append(residualSeqs, pr.seq)
		}
	}
	lw.promoted = es.planValueDefLocals(es.units[0], es.frames[0], residualSeqs)
	if reason := lw.lowerEvents(es.frames[0], 0); reason != "" {
		return nil, reason, false
	}

	// Residual reconciliation.
	lastPos := SrcPos{}
	if n := len(es.frames[0]); n > 0 {
		lastPos = eventPos(es.frames[0][n-1])
	}
	// Fn-value-call boundary (report §9.1): the interpreter auto-applies a
	// FUNCTION value sitting in the residual to the values that follow it
	// (`r.int 0 100` — a method field applied to 0 100). A dynamic carrier
	// is statically Any, so the checker cannot tell a Function from data.
	// When a dynamic value leads the residual and the rest are non-dynamic
	// args, emit OpCallDynamic: at run time it applies the value IF it is a
	// Function, else leaves value+args as-is — faithful either way (plan
	// P4). Any OTHER dynamic-precedes-args shape (a dynamic value mid-
	// residual, or dynamic args) refuses.
	applyDynamic := false
	if len(residual) >= 2 && residual[0].Dynamic {
		restStatic := true
		for i := 1; i < len(residual); i++ {
			if residual[i].Dynamic {
				restStatic = false
				break
			}
		}
		applyDynamic = restStatic
	}
	if !applyDynamic {
		for i := 0; i+1 < len(residual); i++ {
			if residual[i].Dynamic {
				return nil, "dynamic value precedes residual args (fn-value-call boundary)", false
			}
		}
	}
	vi := 0
	tail := []emitOperand{}
	for _, rv := range residual {
		if pr, ok := es.producedBy[rv.ID]; ok {
			// A 0-output statement guard (`if cond [raise]`) registered a
			// phantom None result but produces 0 runtime values: it left no
			// stack slot, so skip it here rather than expecting one.
			if es.zeroOutSeq[pr.seq] {
				continue
			}
			// A promoted value-def local is no longer on the simulated stack
			// (STORE_LOCAL consumed it); re-push it from its slot like any
			// other materialised tail operand.
			if slot, isProm := lw.promoted[pr.seq]; isProm {
				tail = append(tail, localOperand(slot))
				continue
			}
			if len(tail) > 0 {
				return nil, "residual shape beyond Stage 1 (call result above a literal)", false
			}
			if vi >= len(lw.vm) || lw.vm[vi].seq != pr.seq || lw.vm[vi].idx != pr.idx {
				return nil, "residual shape beyond Stage 1 (call results reordered)", false
			}
			vi++
			continue
		}
		if IsBareTypeNode(rv) && rv.ID != "" {
			tail = append(tail, typeOperand(es.internType(rv)))
			continue
		}
		lit, okLit := es.materialise(rv)
		if !okLit {
			return nil, "residual value of unknown provenance", false
		}
		if !isInertConst(lit) {
			return nil, "residual value not statically materialisable", false
		}
		tail = append(tail, constOperand(es.intern(lit)))
	}
	if vi != len(lw.vm) {
		return nil, "residual shape beyond Stage 1 (unconsumed call results)", false
	}
	for _, op := range tail {
		lw.pushOperand(op, lastPos)
	}
	if applyDynamic {
		// The leading dynamic value (residual[0]) and its args are now on
		// the stack; apply the value to the trailing args at run time.
		lw.emit(OpCallDynamic, len(residual)-1, lastPos)
	}

	// Lower the compiled fn units. Tail positions are marked first so
	// the lowering emits TAIL_CALL_USER (frame replacement — the
	// language's tail-call guarantee carries into compiled mode).
	for i, rec := range es.fnRecs {
		if !rec.finished {
			return nil, "fn " + rec.name + " was never compiled", false
		}
		diverged := fragDiverges(rec.frag)
		if !rec.generic && len(rec.outOps) == 1 && rec.outOps[0].kind == opEvent {
			// Tail marking is single-result only: a tail call's results
			// become the fn's results wholesale, so a multi-return tail
			// boundary needs no rewrite here (it stays a plain CALL_USER).
			// Generic instantiations stay out of tail marking too — the
			// interpreter's HasGen exclusion, mirrored (plan Stage 4).
			if !markTailCalls(rec.frag, &rec.outOps[0], true) {
				// Every path tail-called: the body diverges (a trailing
				// all-arms-tail branch isn't caught by fragDiverges, so
				// track it here) and emits no reachable RET.
				rec.outOps = nil
				diverged = true
			}
		}
		// NParams counts everything the call site pushes — declared
		// params AND hidden capture slots; the VM pops them into frame
		// locals uniformly.
		// Pad the slot→name table to NLocals; body-local iterator slots
		// (added during loop lowering) stay anonymous.
		names := make([]string, rec.numLoc)
		copy(names, rec.locals)
		cf := CompiledFn{Name: rec.name, NParams: rec.nParams + len(rec.caps), NCaptures: len(rec.caps), NLocals: rec.numLoc, Returns: rec.returns, LocalNames: names}
		flw := &lowerer{es: es, p: p, code: &cf.Code, debug: &cf.Debug, sigIdx: lw.sigIdx, variadic: map[int]bool{}}
		if reason := flw.lowerEvents(rec.frag.events, rec.frag.startSeq); reason != "" {
			return nil, "fn " + rec.name + ": " + reason, false
		}
		if !diverged {
			// Reconcile the body's N result operands with the simulated
			// stack and emit a RET. Event results must already sit on the
			// stack in order (they were left by their own events); inert
			// operands (const / local / type) are pushed as a trailing
			// tail above the last event result. This is the fn-unit mirror
			// of the program-residual reconciliation below.
			if reason := flw.reconcileResults(rec.outOps, "fn "+rec.name, rec.pos); reason != "" {
				return nil, reason, false
			}
			flw.emit(OpRet, 0, rec.pos)
		}
		// A fully diverging body (every path tail-calls) emits no RET —
		// control leaves via the callee's eventual RET.
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
