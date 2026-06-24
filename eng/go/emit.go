package eng

// The bytecode recording pass — Stage 1 of design/aql-bytecode-plan.0.md.
//
// For the POSITIVE statement of what compiles and why — the rule each refusal
// gate below is defending — see design/COMPILABLE-SUBSET.md. Keep it in lockstep
// when widening the subset; the gates here are a checklist against that rule.
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
// was saved by RememberOriginal — or (c) unknown, which marks the
// program uncompilable rather than guessing.

// Site classes for SiteCounts.
const (
	SiteMono    = "mono"
	SitePoly    = "poly"
	SiteDynamic = "dynamic"
	SiteMeta    = "meta"
)

// Synthetic word names for the compiler-internal assembly events — a computed
// list literal (OpMakeList) and a computed map / make body (OpMakeMap). They are
// not real AQL words; the recorder stamps them on the emitCall it records and
// producerWord / makeListRange read them back, so the two sides must use the
// same string. Centralised here rather than repeated as literals.
const (
	wordMakeList = "[…]"
	wordMakeMap  = "{…}"
)

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

// eventFlags are the per-event compile flags, keyed by event seq in
// EmitState.eventInfo. Each is a property of the producing event:
//   - zeroOut:  branch seq → 0-output statement guard (residual skips it)
//   - typeOut:  event seq → its output is itself a type body
//   - valueDef: event seq → bound to a NAMED value-def (`def x (expr)`)
//   - generic:  event seq → recorded by the GENERIC RecordCall path (a plain
//     native), not a structured hook
type eventFlags struct {
	zeroOut  bool
	typeOut  bool
	valueDef bool
	generic  bool
	// mayBeFn marks a BRANCH event whose result may be a Function at run time
	// (an arm is an fn value), so a trailing arg over it in the residual lowers
	// to a runtime-conditional OpCallDynamic (`if c [99] MathUtil.sqrt 16`).
	mayBeFn bool
	// variadicResult marks an event whose result count is RUNTIME-VARIABLE — a
	// loop, or a branch whose arms leave different / multiple counts (`if c [] [a
	// b]`). Only a variadic-absorbing position (the program residual or a
	// no-contract `[]`-declared fn RET) may consume it; feeding it to a
	// fixed-arity operand refuses. A fn whose body residual is such an event is a
	// VARIADIC-RETURNING fn (fnUnitRec.variadic), so its call sites mark the call
	// result variadic too. Set structurally at record time (RecordBranch /
	// RecordLoop), mirroring lowerArms' merge-variadic accounting; over-marking is
	// sound (refuses more), under-marking is not.
	variadicResult bool
}

type emitCall struct {
	word     string
	sig      *Signature
	ops      []emitOperand
	nout     int // number of results the call pushes (0 for a side-effect word, N for multi-result)
	pos      SrcPos
	poly     bool      // dispatch via OpCallNativePoly (runtime MatchSignature)
	polyReg  *Registry // the sub-registry to re-match a module poly word in (nil = main registry)
	makeList bool      // assemble len(ops) operands into a list (OpMakeList) instead of dispatching a word
	makeMap  bool      // assemble len(ops) value operands into a map (OpMakeMap) with mapKeys
	mapKeys  []string
	mapImpl  bool // the source map's Implicit flag
	diverges bool // the word ALWAYS raises (CompileDiverges, e.g. raise): control never returns past this call
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
	thenIsVal             bool        // then arm is a plain VALUE operand (`if cond 99 88`), not a body fragment
	thenVal               emitOperand // the value-then operand (const/local/type, OR a COMPUTED event when thenComputed) when thenIsVal
	thenComputed          bool        // then value is a COMPUTED event eagerly on the stack below the cond (`if c (expr) e`): SWAP cond up, DROP it on the FALSE path
	elsIsVal              bool        // else arm is a plain VALUE operand (not a body fragment)
	elsVal                emitOperand // the value-else operand (const/local/type, OR a COMPUTED event when elsComputed) when elsIsVal
	elsComputed           bool        // else value is a COMPUTED event eagerly on the stack below the cond (`if c [t] (expr)`): SWAP cond up, DROP it on the taken path
	pos                   SrcPos
}

// armKind classifies one `if` arm, collapsing the emitBranch boolean flags
// (thenIsVal / elsIsVal / elsComputed / hasThenOut / hasElsOut / hasElse) into a
// single discriminant. RecordBranch sets the flags as it discovers each arm's
// shape; consumers ask thenArm() / elseArm() and SWITCH on the result instead of
// re-deriving the kind from flag combinations. The flags stay the storage (one
// — hasThenOut/hasElsOut — is mutated by markTailCalls, so the kind tracks tail
// marking automatically).
type armKind uint8

const (
	armAbsent   armKind = iota // arm not present (a 2-arg if's missing else)
	armBodyOut                 // a […] body fragment that nets one value
	armBodyVoid                // a […] body that nets 0 values or diverges (still runs)
	armValue                   // a plain already-evaluated value operand (`if c 99 88`)
	armComputed                // an eagerly-computed (event) arm value (`if c [t] (expr)` / `if c (expr) e`)
)

// thenArm classifies the then arm. A then arm is always present (even a 2-arg
// if has one); it is a computed event value, a plain value, a value-netting
// body, or a void/diverging body.
func (br *emitBranch) thenArm() armKind {
	switch {
	case br.thenComputed:
		return armComputed
	case br.thenIsVal:
		return armValue
	case br.hasThenOut:
		return armBodyOut
	default:
		return armBodyVoid
	}
}

// elseArm classifies the else arm — armAbsent for a 2-arg if.
func (br *emitBranch) elseArm() armKind {
	switch {
	case !br.hasElse:
		return armAbsent
	case br.elsComputed:
		return armComputed
	case br.elsIsVal:
		return armValue
	case br.hasElsOut:
		return armBodyOut
	default:
		return armBodyVoid
	}
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
	evTrap
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

// emitTrap is a recorded terminal trap: a check-mode-suppressed runtime error
// (an orphan gen, an unpack of a missing key) compiled as an OpTrap. The
// program ends at the trap — the recorder drops everything after it.
type emitTrap struct {
	spec TrapSpec
	pos  SrcPos
}

// emitEvent is one node of the recorded trace, tagged by kind. The two largest
// payloads — emitBranch and emitLoop, each carrying several inline emitOperands
// — ride behind pointers (set only for their kind, nil otherwise) so the common
// evCall event does not pay their size on every copy through frames / fragments;
// the small payloads stay inline. Every consumer is kind-guarded, so a nil
// br/loop on a non-matching event is never dereferenced.
type emitEvent struct {
	seq  int
	kind int
	call emitCall
	br   *emitBranch
	loop *emitLoop
	uc   emitUserCall
	fb   emitFallback
	trap emitTrap
}

// EmitFragment is a captured sub-trace: the events a branch body
// recorded, plus the sequence floor — operands inside the fragment
// referencing events BELOW the floor read enclosing computation,
// which Stage 2's closed-branch lowering refuses.
type EmitFragment struct {
	events   []emitEvent
	startSeq int
	// residualN is the RECORDED carrier-residual count for a branch ARM (the
	// number of runtime values the interpreter leaves) — set by RecordBranch. The
	// lowerer requires the lowered sim residual to match it exactly: a genuine
	// multi-value arm (`[n mul 2 m (n sub 1)]` records 2) is accepted, while a
	// single-value expression whose lowering happens to leave an extra sim slot
	// (`[({a:(get…)} get a)]` records 1 but lowers to 2) is REFUSED as before —
	// without this the extra slot would compile an unsound duplicate value. 0 ==
	// unset (non-arm fragments: a loop body / condition expects one value).
	residualN int
	// applyArgs, when non-empty, marks a loop body that left a LEADING fn VALUE
	// (a returned closure / Function carrier on the sim top after the body events)
	// with these trailing STATIC arg operands above it — the per-iteration dynamic
	// apply `for n [(mk2 i) 10]`. lowerFragment pushes the args and emits a single
	// OpCallDynamic, netting one applied value per iteration (the leading-fn case
	// resolveDynamicApply lowers for the program residual, here inside a loop body).
	// Resolved at RecordLoop time in the enclosing scope; restricted to re-pushable
	// operands (const / local / type) so a computed arg — already on the sim — never
	// double-pushes (such a residual fails the sole-fn check and refuses instead).
	applyArgs []emitOperand
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

	// reg is the registry the check pass runs against — set once at the top of
	// Engine.Run while emit is active. It lets recorder-internal helpers reuse
	// the lang-layer closure compiler (compileClosureBody) for a fn VALUE a body
	// RETURNS (the factory pattern), which needs r but is reached from a finish
	// closure that has only es. Nil outside a compile pass.
	reg *Registry
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
	// eventInfo holds the per-event compile flags, keyed by event seq. It
	// consolidates the former parallel zeroOutSeq/typeOut/valueDefs/genericSeq
	// maps: each is a "property of event N", read via a producer's seq
	// (producedBy[id].seq), so they stay seq-keyed rather than moving onto the
	// value-copied emitEvent struct (a flag set after append wouldn't reach the
	// frame/fragment copies).
	eventInfo map[int]eventFlags
	consts    []Value
	constIdx  map[string]int // CanonValue → Consts index
	types     []TypeRef
	typeIdx   map[string]int   // type ID → Types index
	fallbacks []FallbackSpan   // Stage 5 interpreter islands
	origByID  map[string]Value // stripped literal ID → original value
	// trapAt is the seq of a recorded TOP-LEVEL terminal trap (a check-mode-
	// suppressed runtime error compiled as OpTrap), or 0 for none. When set,
	// Finalize ends the program at the trap. seqs start at 1, so 0 is a safe
	// "none" sentinel.
	trapAt int
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
	// variadic marks a VARIADIC-RETURNING fn: its body residual leaves a
	// runtime-variable count (a `[]`-declared recursive accumulator, an
	// `if c [] [a b]`). A call to it marks the call result variadic (lowerUserCall)
	// so only the program residual or another no-contract RET absorbs it. Set at
	// finish from the body residual's defining event (eventFlags.variadicResult).
	variadic bool
	// outOps are the body's residual operands in stack order (bottom→top):
	// the values the unit leaves for its caller. Empty for a 0-result body
	// OR a diverging body (every path tail-calls) — fragDiverges(frag)
	// distinguishes them (a diverging body emits no RET; a 0-result body
	// emits a bare RET).
	outOps   []emitOperand
	numLoc   int
	pos      SrcPos
	finished bool
	// inShape is the closure input convention recorded for a closure body unit
	// (ClosureInValue by default; ClosureInKeyVal for a map-iteration lambda).
	// Copied into CompiledFn.InShape at lowering. Zero (value) for user fns.
	inShape ClosureInShape
	// closure marks a higher-order body unit (each/scan/…$body) compiled via
	// compileClosureBody, as opposed to a genuine user fn. A return-count
	// mismatch in a closure body is the higher-order word's OWN runtime error
	// (each_error "body produced no result"), not the fn return-count
	// type_error — so a closure keeps refusing the mismatch (islands) while a
	// user fn compiles the error path (the VM RET raises the matching error).
	closure bool
	// takesTop marks a closure whose driving handler reads only the TOP of the
	// body residual (each / fold / scan / filter / rand-list-of —
	// CallableSpec.BodyResultTop). For such a unit, finish DROPS the unconsumed
	// values the body leaves below its top result (notably the per-invocation
	// input a body that ignores its element leaves on the stack, `each [add 1 0]`
	// → `[input, 3]`): the handler never reads below the top, so the RET keeps
	// only the events plus the trailing tail. A whole-residual handler (`do`)
	// leaves this false, keeping the strict in-order reconciliation.
	takesTop bool
	// promoted / dead are the value-def-local plan for THIS unit's body —
	// computed by planValueDefLocals at finish (while the unit is still live) and
	// read by the lowerer (flw.promoted / flw.dead) when the unit lowers. Mirrors
	// the top-level program's plan; nil for a unit with no promotions. A computed
	// result read across a body fragment (a `case` scrutinee re-tested down the
	// if-chain) is promoted to a frame slot here, the same as at frame 0.
	promoted map[int]int
	dead     map[int]bool
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
		eventInfo:  map[int]eventFlags{},
		constIdx:   map[string]int{},
		typeIdx:    map[string]int{},
		origByID:   map[string]Value{},
	}
}

func (es *EmitState) active() bool {
	return es != nil && es.Compilable && es.suspended == 0
}

// Active is the exported view of active() for native handlers that
// must mirror the bytecode-recording state — e.g. a 0-output `if`
// statement guard only puts its phantom None on the carrier stack
// while recording is live (the lowering tracks it); a plain or
// uncompilable check must net 0, like the runtime.
func (es *EmitState) Active() bool { return es.active() }

// MarkUncompilable latches the program uncompilable, keeping the
// FIRST reason (later marks are consequences of the first).
func (es *EmitState) MarkUncompilable(reason string) {
	if es == nil || !es.Compilable {
		return
	}
	if es.trapAt != 0 {
		// A terminal top-level trap already ends the program here. Traps are
		// recorded in execution order and only while active() (Compilable
		// true), so any construct that marks uncompilable AFTER trapAt is set
		// is at-or-after the trap and therefore unreachable — the interpreter
		// raises at the trap and never reaches it. Finalize truncates to the
		// trap and drops the residual, so its refusal is moot (e.g. `getr` of a
		// missing ModuleExport key: the getr IS the trap, and its own
		// operand-provenance residual must not refuse the program).
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
		f := es.eventInfo[seq]
		f.typeOut = true
		es.eventInfo[seq] = f
	}
}

// MarkValueDef records that value v is bound to a NAMED `def x (expr)` binding,
// so the lowerer promotes v's producing event to a frame LOCAL (STORE_LOCAL +
// PUSH_LOCAL per reference) instead of leaving it on the simulated stack. A
// named binding may be consumed in ANY order — `def a (make …) def b (make …)
// a.x … b.x` uses a (produced first) before b (on top), which the single-
// consume stack discipline cannot seat — whereas a local re-pushes freely. Only
// a single-result producing event qualifies (a multi-result `dup` needs the
// carrier-identity path); a const / local / unproduced value is a no-op.
// Nil-receiver-safe; called from the def handler's install choke point.
func (es *EmitState) MarkValueDef(v Value) {
	if es == nil || !es.Compilable {
		return
	}
	if pr, ok := es.producedBy[v.ID]; ok && pr.idx == 0 {
		f := es.eventInfo[pr.seq]
		f.valueDef = true
		es.eventInfo[pr.seq] = f
	}
}

// resolveOperand maps a dispatch value to its provenance: a prior
// event's output, or an inert constant (concrete at the dispatch, or
// a stripped literal whose original RememberOriginal saved).
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
		if !IsTypeBody(v) || es.eventInfo[pr.seq].typeOut {
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

// tryReturnedClosure compiles a fn VALUE that a body RETURNS — the factory
// pattern `def mk fn [[x:Integer] [Function] [([y:Integer] => […])]]`, whose
// body residual is an anonymous lambda — into its own closure unit, yielding an
// opClosure operand (lowering to OpPushClosure). The caller (the fn-finish
// residual resolution) falls back here when resolveOperand cannot place the
// value. Probe-guarded so a refusal leaves the real state untouched.
//
// Only an ANONYMOUS, single-own-sig FnDefInfo qualifies: a named ref
// re-dispatches by name; an overloaded fn needs runtime MatchFnSig; a sentinel
// body targets an enclosing frame. Anything else returns false and the program
// falls back faithfully.
//
// A CAPTURING lambda is admitted: the factory pattern `def mk fn [[x][Function]
// [([y]=>[x add y])]]` returns a closure over the factory's PARAM x. Each
// captured binding is resolved in THIS (the factory body's) scope to an operand
// — x is the factory's frame local — and threaded as a closureCap, so the
// lowering pushes the captured value before OpPushClosure exactly as the in-place
// closure-dispatch path does (lower.go opClosure). A capture that cannot resolve
// here (an unreachable enclosing binding) declines and the program falls back.
func (es *EmitState) tryReturnedClosure(v Value, pos SrcPos) (emitOperand, bool) {
	if es == nil || es.reg == nil || v.Carrier || v.Dynamic {
		return emitOperand{}, false
	}
	fd, ok := v.Data.(FnDefInfo)
	if !ok || !fd.Anonymous {
		return emitOperand{}, false
	}
	// Resolve the lambda's captures in the ENCLOSING (factory body) scope, the
	// same operand resolution recordClosureDispatch uses for an in-place closure.
	// A capture that does not resolve (unreachable enclosing binding) declines.
	capOps := make([]emitOperand, len(fd.Captured))
	for i, cb := range fd.Captured {
		op, okCap := es.resolveOperand(cb.Value)
		if !okCap {
			return emitOperand{}, false
		}
		capOps[i] = op
	}
	// Exactly one own (non-fallback) signature — FirstOwnSig is otherwise not
	// guaranteed to be the overload a runtime MatchFnSig would pick.
	own := 0
	for i := range fd.Signatures {
		if !fd.Signatures[i].Fallback {
			own++
		}
	}
	lam, hasOwn := fd.FirstOwnSig()
	if own != 1 || !hasOwn || len(lam.Body) == 0 || bodyToksHaveSentinel(lam.Body) {
		return emitOperand{}, false
	}
	inputs := make([]Value, len(lam.Params))
	paramNames := make([]string, len(lam.Params))
	for i, p := range lam.Params {
		t := p.Type
		if t == nil {
			t = TAny
		}
		inputs[i] = NewCarrier(t)
		paramNames[i] = p.Name
	}
	r := es.reg
	// PROBE in a throwaway emit state (mirrors recordClosureDispatch), so a body
	// that refuses leaves THIS program untouched and the value stays unresolved.
	r.Check.Emit = NewEmitState()
	r.Check.Emit.reg = r
	// bodyOut 1: a fn VALUE body keeps the single declared return (it is not a
	// 0-output side-effect body like a test case).
	_, probeOK := compileClosureBody(r, "fnval", 1, false, false, lam.Body, inputs, paramNames, fd.Captured, ClosureInValue, pos)
	r.Check.Emit = es
	if !probeOK {
		return emitOperand{}, false
	}
	// REAL: compile into this program (deterministic success after a clean probe).
	unit, realOK := compileClosureBody(r, "fnval", 1, false, false, lam.Body, inputs, paramNames, fd.Captured, ClosureInValue, pos)
	if !realOK || unit < 0 {
		return emitOperand{}, false
	}
	return emitOperand{kind: opClosure, closureUnit: unit, closureCaps: capOps}, true
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
	if pr, ok := es.producedBy[v.ID]; ok && (!IsTypeBody(v) || es.eventInfo[pr.seq].typeOut) {
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

// CanSeatAcrossFragment reports whether v can be read INSIDE a branch / loop
// fragment that a multi-reference desugar is about to record. Either v is
// already re-pushable (OperandRepushable: a const, frame local, or type node),
// OR it is a COMPUTED single-result event — which planValueDefLocals promotes to
// a frame local once a fragment read is seen, reachable across the scope floor.
// The promotion now runs for EVERY unit (the top-level program AND each fn body
// — see the planValueDefLocals call in StartFnCompile's finish), so a computed
// scrutinee inside a fn unit (a `case` in an `error`/`do` handler closure) seats
// too. A non-repushable computed event reaching here is always produced in the
// CURRENT unit — an enclosing-scope value is a capture, hence a local (repushable
// above) — so its promotion lands in the right unit's slots. Side-effect free,
// like OperandRepushable — the promotion itself happens later, driven purely by
// the fragment reference the desugar then records.
func (es *EmitState) CanSeatAcrossFragment(v Value) bool {
	if es == nil {
		return false
	}
	if es.OperandRepushable(v) {
		return true
	}
	pr, ok := es.producedBy[v.ID]
	return ok && pr.idx == 0 && (!IsTypeBody(v) || es.eventInfo[pr.seq].typeOut)
}

// materialise recovers the fully concrete value behind a stripped
// literal: the value itself, its RememberOriginal original, or — for a
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
		elems := d.Elems
		rebuilt := false
		for i, e := range d.Elems {
			m, changed, ok := es.materialiseMember(e)
			if !ok {
				return v, false
			}
			if changed {
				if !rebuilt { // copy-on-first-change, then patch in place
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
		keys := d.M.Keys()
		var nm *OrderedMap
		for i, k := range keys {
			mv, _ := d.M.Get(k)
			m, changed, ok := es.materialiseMember(mv)
			if !ok {
				return v, false
			}
			if changed && nm == nil { // copy the unchanged prefix, then this member
				nm = NewOrderedMap()
				for _, pk := range keys[:i] {
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

// materialiseMember materialises one container member and reports whether it
// CHANGED — a stripped carrier recovered to a concrete original (different
// Carrier flag or ID). ok=false when the member's original is unknown, so the
// whole container cannot be recovered. The shared per-member step of
// materialise's list and map arms.
func (es *EmitState) materialiseMember(e Value) (m Value, changed, ok bool) {
	m, ok = es.materialise(e)
	if !ok {
		return e, false, false
	}
	return m, m.Carrier != e.Carrier || m.ID != e.ID, true
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
	if last.kind == evCall && last.call.diverges {
		// A CompileDiverges word (raise) always raises: the fragment never
		// produces a residual past it, so it diverges like break/continue. A
		// closure body ending here compiles with no RET; the error propagates
		// out of the VM run and the catching word (do/error) wraps it.
		return true
	}
	return last.kind == evBreak || last.kind == evContinue
}

// fragDivergesDeep reports whether a LOWERED fragment definitely leaves the
// enclosing construct on every reachable path: break, continue, a tail call,
// or a branch all of whose reachable arms do. Unlike fragDiverges (a shallow
// last-event check), it recurses through branch arms, so a fully-diverging
// NESTED branch — e.g. a const-condition branch whose taken arm tail-calls —
// is seen as divergent by the variadic-merge accounting. Sound only AFTER
// markTailCalls has run (it reads the tail flags set there); the only caller
// is lowerArms, in Finalize's lowering pass.
func fragDivergesDeep(frag *EmitFragment) bool {
	if frag == nil || len(frag.events) == 0 {
		return false
	}
	return eventDivergesDeep(&frag.events[len(frag.events)-1])
}

func eventDivergesDeep(ev *emitEvent) bool {
	switch ev.kind {
	case evBreak, evContinue:
		return true
	case evCallUser:
		return ev.uc.tail
	case evBranch:
		if ev.br.constCond != nil {
			// Only the taken (then) arm is reachable.
			return fragDivergesDeep(ev.br.then)
		}
		switch ev.br.elseArm() {
		case armAbsent:
			return false // the implicit false path falls through to the merge
		case armValue, armComputed:
			return false // a value / computed else arm never diverges
		}
		// Both arms are bodies: the branch diverges only if both do.
		return fragDivergesDeep(ev.br.then) && fragDivergesDeep(ev.br.els)
	}
	return false
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
	ThenValue       *Value // non-nil: the then arm is this already-evaluated VALUE, not a body
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
	ev := emitEvent{kind: evBranch, br: &emitBranch{
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
	// mayBeFn: an arm is an fn VALUE, so the branch result may be callable at
	// run time — a trailing residual arg over it is a conditional apply.
	mayBeFn := false
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
		var hasThen bool
		if b.ThenValue != nil {
			// Value-then: the then arm is an already-evaluated value
			// (`if cond 99 88`), not a `[…]` body — it lowers to a single push in
			// the then arm, mirroring the value-else below. A COMPUTED then (an
			// event result eagerly on the stack) would need the stack juggling the
			// single-result lowering does not do, so refuse it.
			op, ok := es.resolveOperand(*b.ThenValue)
			if !ok {
				es.MarkUncompilable("if: then value of unknown provenance")
				return
			}
			if isFnValueResidual(*b.ThenValue) {
				mayBeFn = true
			}
			if op.kind == opEvent {
				// A COMPUTED then value (`if cond (add 1 2) 88`) is eagerly on the
				// stack BELOW the cond event — the mirror of the computed-else case.
				// The lowerer SWAPs the cond above it, branches, and DROPs it on the
				// FALSE path (it survives on the TRUE path as the result). Only the
				// plain-event-cond layout [cond, thenVal] is handled; a const /
				// condFrag / const-cond condition sits elsewhere, so refuse those.
				if !computedArmCondOK(b, ev.br.cond) {
					es.MarkUncompilable("if: computed then value with non-stack condition (Stage 2)")
					return
				}
				ev.br.thenIsVal, ev.br.thenVal, ev.br.hasThenOut, ev.br.thenComputed, hasThen = true, op, true, true, true
			} else {
				ev.br.thenIsVal, ev.br.thenVal, ev.br.hasThenOut, hasThen = true, op, true, true
			}
		} else {
			thenOut, h, ok := resolveArm(b.Then, b.ThenStk, "then")
			if !ok {
				return
			}
			ev.br.then, ev.br.thenOut, ev.br.hasThenOut, hasThen = b.Then, thenOut, h, h
		}
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
				if isFnValueResidual(*b.ElsValue) {
					mayBeFn = true
				}
				if op.kind == opEvent {
					// A COMPUTED else value (`if cond [then] (add 1 2)`) is
					// eagerly on the stack below the cond/then events. The lowerer
					// branches and DROPs the unselected eager value(s).
					if ev.br.thenComputed {
						// `if cond (a) (b)` — BOTH arms computed. The three events
						// stack as [cond, then, else]; lowerBothComputed selects one
						// with OpReverse + JMP_IF_FALSE. It needs the cond on the
						// stack (an event), so a const / condFrag / const-cond
						// condition (where thenComputed was set under
						// computedArmCondOK's wider rule) refuses here.
						if ev.br.cond.kind != opEvent {
							es.MarkUncompilable("if: both computed arms need an event condition (Stage 2)")
							return
						}
						ev.br.elsIsVal, ev.br.elsVal, ev.br.hasElsOut, ev.br.elsComputed = true, op, true, true
					} else {
						// Single computed else: only the plain-event-cond layout
						// [cond, elseVal] (SWAP), a list-form cond (inline), or a
						// const/local cond (pushed) is handled — computedArmCondOK.
						if !computedArmCondOK(b, ev.br.cond) {
							es.MarkUncompilable("if: computed else value with non-stack condition (Stage 2)")
							return
						}
						ev.br.elsIsVal, ev.br.elsVal, ev.br.hasElsOut, ev.br.elsComputed = true, op, true, true
					}
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
					// Both arms produce 0 values — an empty `[]`, a 0-value word
					// (set/printstr), or a diverging break/continue/raise on BOTH
					// sides: the if is a 0-value STATEMENT on every path. Record it
					// zeroOut (no merge slot, like the 2-arg no-else guard) rather
					// than refusing; both arm fragments still lower, so their effects
					// and divergence run.
					zeroOut = true
				}
			}
		}
	}
	// Record each body arm's true (carrier) residual count so the lowerer can
	// distinguish a GENUINE multi-value arm from a single-value expression whose
	// lowering leaves an extra sim artifact (see EmitFragment.residualN).
	if ev.br.then != nil {
		ev.br.then.residualN = len(b.ThenStk)
	}
	if ev.br.els != nil {
		ev.br.els.residualN = len(b.ElsStk)
	}
	seq := es.appendEvent(ev)
	es.SiteCounts[SiteMono]++
	es.setProduced(b.Out, seq)
	if zeroOut {
		// The if produces 0 runtime values; its registered (None) result is a
		// phantom. Mark the seq so Finalize's residual reconciliation skips it
		// rather than expecting a stack slot. Keeping the setProduced above
		// lets RecordCall's double-record guard elide this if dispatch.
		f := es.eventInfo[seq]
		f.zeroOut = true
		es.eventInfo[seq] = f
	}
	if mayBeFn {
		f := es.eventInfo[seq]
		f.mayBeFn = true
		es.eventInfo[seq] = f
	}
	if !zeroOut && es.branchVariadicResult(b) {
		f := es.eventInfo[seq]
		f.variadicResult = true
		es.eventInfo[seq] = f
	}
}

// branchVariadicResult reports whether an `if` produces a RUNTIME-VARIABLE result
// count — the structural mirror of lowerArms' merge-variadic marking, computed at
// record time so a fn's variadic-return-ness is known before its call sites lower.
// Variadic when: a 2-arg if leaves 0-or-N; the two arms leave different
// non-diverging counts (`if c [] [a]`); either arm is multi-value (`if c [a b]
// [c]`); or either arm's own result is itself variadic (a nested variadic if).
// Over-marking is sound (it only refuses external fixed-arity consumption); a
// diverging arm never reaches the merge, so it does not count toward a mismatch.
func (es *EmitState) branchVariadicResult(b BranchRecord) bool {
	thenN, elsN := len(b.ThenStk), len(b.ElsStk)
	if b.ConstCond != nil {
		// Only the taken (then) arm is inlined; its result IS the branch result.
		return thenN > 1 || es.armOutVariadic(b.ThenStk)
	}
	if !b.HasElse {
		return thenN >= 1 // value on true, nothing on false → 0-or-N
	}
	if thenN > 1 || elsN > 1 {
		return true
	}
	thenHas := thenN > 0 && !(b.Then != nil && fragDiverges(b.Then))
	elsHas := elsN > 0 && !(b.Els != nil && fragDiverges(b.Els))
	if thenHas != elsHas {
		return true
	}
	return es.armOutVariadic(b.ThenStk) || es.armOutVariadic(b.ElsStk)
}

// armOutVariadic reports whether an arm's result value (its residual top) is
// itself a variadic-producing event — a nested variadic if / loop the parent
// branch propagates up to its own merge.
func (es *EmitState) armOutVariadic(stk []Value) bool {
	if len(stk) == 0 {
		return false
	}
	pr, ok := es.producedBy[stk[len(stk)-1].ID]
	return ok && es.eventInfo[pr.seq].variadicResult
}

// computedArmCondOK reports whether the branch condition is one the computed-arm
// (eager-value) lowering can materialise as a Boolean on top of the eager value:
// an event cond (SWAPped up from just below the eager value), a list-form
// condition body (lowered inline), or a const / local / type cond (pushed). A
// statically-known (const-folded) condition is handled by the disjoint
// const-cond path, never here.
func computedArmCondOK(b BranchRecord, cond emitOperand) bool {
	if b.ConstCond != nil {
		return false
	}
	if b.CondFrag != nil {
		return true
	}
	switch cond.kind {
	case opEvent, opConst, opLocal, opType:
		return true
	default:
		return false
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
			// Resolve every residual value to an operand, in stack order
			// (bottom→top), so the unit leaves the body's N results for its
			// caller. A 0-output statement guard (`if cond [raise]`) registered a
			// phantom None in the residual but produces 0 runtime values, so it
			// leaves NO operand — skip it, exactly as the top-level residual
			// reconciliation does. A 0-result body leaves outOps empty (a bare RET).
			ops := make([]emitOperand, 0, len(bodyStk))
			for _, v := range bodyStk {
				if pr, ok := es.producedBy[v.ID]; ok && es.eventInfo[pr.seq].zeroOut {
					continue
				}
				// A body that RETURNS an anonymous capture-free lambda (the factory
				// pattern `def mk fn [[x][Function][([y]=>…)]]`) compiles the
				// returned fn to its own closure unit, so the body leaves a runtime
				// closure VALUE on the stack — applied VM-native by a later stack
				// OpCallDynamic (`(mk 5) 10`). Tried BEFORE resolveOperand because a
				// capture-free anonymous fn would otherwise bake as an inert const
				// and apply through a callDynamic interpreter island instead of the
				// VM. A non-fn / named / capturing / multi-sig value declines here
				// and falls through to the normal resolution.
				op, okOut := es.tryReturnedClosure(v, rec.pos)
				if !okOut {
					op, okOut = es.resolveOperand(v)
				}
				if !okOut {
					es.MarkUncompilable("fn " + name + ": body result of unknown provenance")
					return
				}
				ops = append(ops, op)
			}
			// A DECLARED fn must leave exactly len(returns) RUNTIME values; a
			// different count (measured over real operands, phantom guards
			// excluded) is a return-COUNT mismatch. For a genuine USER fn that is
			// the type_error the interpreter raises at __RC, so rather than refuse
			// we COMPILE the body and let the VM's RET enforce the count — it raises
			// the byte-identical type_error (shared returnCountErrorText), erroring
			// exactly where the interpreter does instead of falling back. For a
			// CLOSURE body (each/scan/…$body) the mismatch is instead the
			// higher-order word's OWN runtime error (each_error "body produced no
			// result"), a different taxonomy — so a closure keeps refusing and
			// islands, letting the interpreter raise the matching error. An
			// UNDECLARED fn (an anonymous lambda whose Returns were nilled, or a
			// 0-return fn) is NOT count-checked: its residual is taken as-is.
			if rec.closure && len(rec.returns) > 0 && len(ops) != len(rec.returns) {
				es.MarkUncompilable("closure " + name + ": body value count differs from declared returns")
				return
			}
			// A TOP-TAKING closure's driving handler reads only the top of the body
			// residual (each / fold / scan / filter — CallableSpec.BodyResultTop), so
			// the values the body leaves BELOW its result — notably the per-invocation
			// input a body that ignores its element leaves on the stack (`each [add 1
			// 0]` → [input, 3]) — are never observed and are dropped here. Without this,
			// the residual [inert-input, computed-result] refuses at the RET (an event
			// above an inert, "result above a literal"), even though the handler only
			// ever reads the top.
			if rec.takesTop && len(ops) > 1 {
				ops = trimToTopResult(ops)
			}
			rec.outOps = ops
			// VARIADIC-RETURNING fn: the body residual's defining (top) event leaves
			// a runtime-variable count (a variadic branch / loop, or a call to an
			// already-variadic fn — RecordUserCall flags that on the event). A call
			// site marks its result variadic (lowerUserCall) so only a
			// variadic-absorbing position consumes it.
			if n := len(ops); n > 0 && ops[n-1].kind == opEvent && es.eventInfo[ops[n-1].idx].variadicResult {
				rec.variadic = true
			}
		}
		// Value-def locals for THIS unit (the top-level program's promotion,
		// scoped to a fn body): a computed result referenced more than once, read
		// from INSIDE a body fragment (a `case` scrutinee re-tested down the
		// desugared if-chain — the case-in-closure enabler), or bound to a named
		// def must be stored in a frame slot and re-pushed, not left on the
		// single-consume stack. Run while the unit `u` is live so its slot
		// allocations land in u.numLocals (→ rec.numLoc below); the unit's OUTPUT
		// operands are its residual-equivalent extra references, and are rewritten
		// for any promoted producer (planValueDefLocals rewrote the events in
		// place, but outOps is a separate slice).
		outSeqs := make([]int, 0, len(rec.outOps))
		for _, op := range rec.outOps {
			if op.kind == opEvent && op.resIdx == 0 {
				outSeqs = append(outSeqs, op.idx)
			}
		}
		rec.promoted, rec.dead = es.planValueDefLocals(u, rec.frag.events, outSeqs, nil)
		for i := range rec.outOps {
			promoteOperand(&rec.outOps[i], rec.promoted)
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

// trimToTopResult drops the operands a TOP-TAKING closure body leaves BELOW its
// result. The driving handler (each / fold / scan / filter) reads only the top of
// the body residual, so everything beneath it is never observed. The residual is
// [leading-inerts…, events…, trailing-inerts…]; the leading inerts are the values
// the body left below its first COMPUTED result — the per-invocation input a body
// that ignores its element leaves on the stack (`each [add 1 0]` → [input, 3]).
// Keeping from the first EVENT operand preserves the events (physically on the
// sim stack) and the trailing tail (which carries the top) while dropping those
// leading inerts; a residual with NO event is pure data whose only result is the
// TOP operand. Only called for len(ops) > 1; never drops an event (the top is at
// or after the first event) or a trailing inert.
func trimToTopResult(ops []emitOperand) []emitOperand {
	for i := range ops {
		if ops[i].kind == opEvent {
			return ops[i:]
		}
	}
	return ops[len(ops)-1:]
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
	// A call to an ALREADY-variadic fn yields a runtime-variable count itself, so
	// the result propagates variadic (a branch arm / body residual carrying it is
	// variadic too). The self-recursive call records before its own fn finishes
	// (rec.variadic not yet set) — fine: that result flows only to the body's own
	// variadic merge, never to a fixed operand.
	if es.fnRecs[unit].variadic {
		f := es.eventInfo[seq]
		f.variadicResult = true
		es.eventInfo[seq] = f
	}
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
	lp := &emitLoop{start: startOp, end: endOp, step: stepOp, iterSlot: -1, pos: pos}
	if len(bodyStk) > 0 && !fragDiverges(body) {
		if es.setLoopBodyApply(body, bodyStk) {
			// A per-iteration dynamic apply (`(mk2 i) 10`): the body nets one applied
			// value, lowered via OpCallDynamic over the leading fn (body.applyArgs).
			lp.hasBodyOut = true
		} else {
			bodyOut, ok := es.resolveOperand(bodyStk[len(bodyStk)-1])
			if !ok {
				es.MarkUncompilable("for: body result of unknown provenance")
				return
			}
			lp.bodyOut, lp.hasBodyOut = bodyOut, true
		}
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
	// A loop leaves a runtime-variable count (one per-iteration value, N
	// unknown at compile time) — variadic, like lowerLoop marks lw.variadic.
	f := es.eventInfo[seq]
	f.variadicResult = true
	es.eventInfo[seq] = f
}

// setLoopBodyApply detects a loop body whose residual is a LEADING fn VALUE with
// trailing STATIC args — the per-iteration dynamic apply `for n [(mk2 i) 10]`,
// where each iteration mk2 returns a closure that is applied to 10. It mirrors
// resolveDynamicApply's leading-fn-carrier case, but seats the apply on the loop
// body fragment (body.applyArgs) so lowerFragment emits the OpCallDynamic per
// iteration. Returns true when the shape was recognised and seated.
//
// Soundness: bodyStk[0] must be a Function/FnDef-typed CARRIER (always callable —
// the one inert fn shape, an `f/r` reference, is a concrete const, not a carrier)
// produced by an event (so it lands on the sim top after the body events lower),
// and every trailing arg must be a non-fn, non-dynamic value resolving to a
// RE-PUSHABLE operand (const / local / type). A computed (event) arg is already
// on the sim, so it is excluded here and the residual instead fails lowerFragment's
// sole-fn check and refuses — never a double-push.
func (es *EmitState) setLoopBodyApply(body *EmitFragment, bodyStk []Value) bool {
	if es == nil || body == nil || len(bodyStk) < 2 || !isFnTypedCarrier(bodyStk[0]) {
		return false
	}
	fnOp, ok := es.resolveOperand(bodyStk[0])
	if !ok || fnOp.kind != opEvent {
		return false
	}
	applyArgs := make([]emitOperand, 0, len(bodyStk)-1)
	for _, a := range bodyStk[1:] {
		if a.Dynamic || isFnValueResidual(a) {
			return false
		}
		op, okArg := es.resolveOperand(a)
		if !okArg || (op.kind != opConst && op.kind != opLocal && op.kind != opType) {
			return false
		}
		applyArgs = append(applyArgs, op)
	}
	body.applyArgs = applyArgs
	return true
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

// RememberOriginal records a CONCRETE value against its own ID so that
// when a carrier-strip reduces it — toCarrier keeps Value.ID, so the
// stripped carrier shares the original's ID — the lowerer can recover the
// concrete value via materialise/origByID. The single recorder behind both
// strip provenance paths:
//
//   - top-level literals that StripToCarriers reduces to carriers (the
//     caller in engine.Run gates on the strip actually having happened:
//     same ID, now a carrier);
//   - impure-but-pure-data constructors that run in check mode and whose
//     result is stripped before reaching a downstream operand — notably the
//     predicate constructors (`Integer gt 10`), whose DepScalarInfo payload
//     toCarrier strips to a bare base-type carrier, losing the bound.
//
// A value that is already a carrier/dynamic, or has no ID, is a no-op
// (nothing concrete to recover). Skips while suspended or once the program
// is uncompilable (an uncompilable program is never lowered, so its
// origByID entries are never read).
func (es *EmitState) RememberOriginal(v Value) {
	if es == nil || es.suspended > 0 || !es.active() {
		return
	}
	if v.Carrier || v.Dynamic || v.ID == "" {
		return
	}
	es.origByID[v.ID] = v
}

// RecordTrap records a TERMINAL trap for a check-mode-suppressed runtime error
// (an orphan gen, an unpack of a missing key): the checker is lenient at this
// point but the interpreter errors, so the compiled program raises the
// byte-identical error here via OpTrap instead of refusing the whole program.
// Only a TOP-LEVEL trap is recorded (frames and units both at depth 1): a trap
// inside a branch/loop/fn fragment is conditional and not yet modelled, so it
// returns false and the caller keeps the SuppressedRuntimeError blanket refusal.
// The first trap wins (execution can reach only one). Returns true when the trap
// is owned here (recorded now, or already trapping).
func (es *EmitState) RecordTrap(code, detail, word, hint string, pos SrcPos) bool {
	if !es.active() || len(es.frames) != 1 || len(es.units) != 1 {
		return false
	}
	if es.trapAt != 0 {
		return true
	}
	es.trapAt = es.appendEvent(emitEvent{kind: evTrap, trap: emitTrap{
		spec: TrapSpec{Code: code, Detail: detail, Word: word, Hint: hint},
		pos:  pos,
	}})
	return true
}

// RememberStrippedOriginals records the pre-strip original of each value that
// StripToCarriers actually reduced to a carrier (same preserved ID), so the
// lowerer can later recover the concrete literal. Values toCarrier kept
// concrete need no recovery and are skipped. pre and stripped are parallel.
func (es *EmitState) RememberStrippedOriginals(pre, stripped []Value) {
	for i := range stripped {
		if stripped[i].Carrier && pre[i].ID != "" && pre[i].ID == stripped[i].ID {
			es.RememberOriginal(pre[i])
		}
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
// forceDynOut bypasses the dynamic-output refusal when the caller
// (dynOutNativeOK) has proven the dispatch is a concrete-args core builtin
// whose dynamic result is merely a declared-Any return. quoteInertOK bypasses
// the quoted-operand refusal when the caller (quoteOperandInertOK) has proven
// the dispatch is a module inner native whose quoted operands are inert Atom
// consts (the query DSL's table names).
func (es *EmitState) RecordCall(word string, sig *Signature, args, outs []Value, pos SrcPos, forceDynOut, quoteInertOK bool) {
	if !es.active() {
		return
	}
	if es.recordCallElided(word, sig, args, outs) {
		return
	}
	if es.recordCallRefusal(word, sig, args, outs, pos, forceDynOut, quoteInertOK) {
		return
	}
	ops, ok := es.recordCallOperands(word, sig, args)
	if !ok {
		return
	}
	es.SiteCounts[SiteMono]++
	seq := es.appendEvent(emitEvent{kind: evCall, call: emitCall{word: word, sig: sig, ops: ops, nout: len(outs), pos: pos, diverges: sig.CompileEffect.Has(CompileDiverges)}})
	// Carrier-identity de-collision (the deferred runtime-independence item, in
	// its targeted form). A call OUTPUT whose ID already maps to a PRIOR event is
	// a repeated identical computed call: `(context get 'n') add (context get
	// 'n')` issues two get events that each return the same deterministic-ID
	// result, so the second registration would overwrite the first and `add`
	// would resolve BOTH operands to the second event — the residual layout then
	// refuses "call results reordered". Mint a fresh ID so the two stack values
	// stay distinct (the outs slice is the carrierResults return, so the fresh ID
	// flows to the downstream consumer). Guarded twice: a SAME-event collision is
	// `dup`/`swap` returning an input by identity (`[args[0], args[0]]`), handled
	// by the DUP lowering, so skip pr.seq == seq; and a result that IS one of the
	// call's inputs (an identity/stack word passing a value through) keeps its ID,
	// so skip an output whose ID appears in args.
	gf := es.eventInfo[seq]
	gf.generic = true
	es.eventInfo[seq] = gf
	var argIDs map[string]bool
	for i := range outs {
		if pr, ok := es.producedBy[outs[i].ID]; ok && pr.seq != seq {
			if argIDs == nil {
				argIDs = make(map[string]bool, len(args))
				for _, a := range args {
					argIDs[a.ID] = true
				}
			}
			if !argIDs[outs[i].ID] {
				outs[i].ID = GenerateID(IDPrefixForType(outs[i].Parent))
			}
		}
		es.setProducedAt(outs[i], seq, i)
	}
}

// recordCallElided reports whether a dispatch is ELIDED — already recorded by a
// structured hook, or a compile-time name resolution that produces nothing the
// VM runs. The caller returns without recording when this is true.
func (es *EmitState) recordCallElided(word string, sig *Signature, args, outs []Value) bool {
	// A dispatch whose output is already registered was recorded by a
	// structured hook (RecordBranch owns the `if` dispatch; a user-fn
	// ReturnsFn owns its RecordUserCall — including multi-return calls) —
	// the generic path must not double-record or refuse it. A structured
	// hook registers all results together, so checking the first suffices.
	// The producer must be a STRUCTURED hook, not a prior GENERIC native call:
	// a repeated identical computed call (`(context get 'n') add (context get
	// 'n')` — both gets return the same deterministic-ID result once their key
	// is concrete) collides with a prior generic event, and skipping the second
	// would orphan its receiver push. Those fall through to the carrier-identity
	// de-collision in RecordCall (which mints a fresh ID), so guard on !generic.
	if len(outs) > 0 {
		if pr, ok := es.producedBy[outs[0].ID]; ok && !es.eventInfo[pr.seq].generic {
			return true
		}
	}
	// A user fn returning ZERO values (`def v fn [[x:Integer] [] []]`): its
	// ReturnsFn recorded a 0-output CALL_USER and returned no carriers, so there
	// is no output ID to key the elision above on. The compiled 0-output call is
	// unambiguous — a user-fn dispatch whose body unit DECLINED to compile
	// returns a single Any approximation instead (len(outs)==1), so it falls
	// through to recordCallRefusal and the program falls back. Only the compiled
	// case reaches here with an empty residual.
	if sig != nil && sig.FnFrame != nil && len(outs) == 0 {
		return true
	}
	// `apply` of a fn VALUE (`…args fn apply`): apply's ReturnsFn returns the
	// fn concrete, so the check engine RE-STEPS it — the fn then dispatches
	// against its preceding stack args and records as an ordinary CALL_USER.
	// Elide apply's own dispatch (it produces nothing the VM runs); without
	// this RecordCall would refuse it as "function value reaches apply". The
	// Reach-apply sig (a TReach operand, not an FnDef) is untouched.
	if word == "apply" && len(args) >= 1 {
		if _, ok := args[0].Data.(FnDefInfo); ok {
			return true
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
			return true
		}
	}
	return false
}

// recordCallRefusal classifies a dispatch the recorder cannot lower as a generic
// CALL_NATIVE. It returns true (the dispatch is handled) after either marking the
// program uncompilable, or recording a flow-control terminator (break/continue —
// the enclosing loop's lowering turns these into jumps). It returns false for an
// ordinary lowerable call, which RecordCall then records.
func (es *EmitState) recordCallRefusal(word string, sig *Signature, args, outs []Value, pos SrcPos, forceDynOut, quoteInertOK bool) bool {
	switch {
	case sig == nil:
		es.MarkUncompilable("dispatch without a signature at " + word)
	case word == "":
		// Anonymous / fn-value dispatch (usurp wrappers, F4 value
		// calls): the callee is a runtime value, Stage 3 territory.
		es.SiteCounts[SiteMeta]++
		es.MarkUncompilable("anonymous function dispatch (Stage 3)")
	case sig.RunInCheckMode:
		es.SiteCounts[SiteMeta]++
		es.MarkUncompilable("compile-time word " + word)
	case sig.FnFrame != nil:
		es.SiteCounts[SiteMeta]++
		es.MarkUncompilable("user fn call " + word + " (Stage 3)")
	case sig.FullStack:
		es.SiteCounts[SiteMeta]++
		es.MarkUncompilable("full-stack word " + word)
	case word == "args" || word == "__pa":
		// `args` reads the interpreter's per-call args stack, which the
		// VM's CALL_USER frame does not maintain (it binds params to
		// frame locals instead). A compiled fn body that reads `args`
		// would fail with "args: not inside a function" — refuse so the
		// program falls back to the interpreter.
		es.SiteCounts[SiteMeta]++
		es.MarkUncompilable("context-dependent word " + word)
	case len(sig.NoEvalArgs) > 0 && (sig.CompileEffect.Has(CompileExecutesBody) || (sig.Callable != nil && execBodyRefsNames(sig, args)) || !noEvalBodiesInert(sig, args)):
		// A code-body word is refused when:
		//   - its body is not inert data; OR
		//   - it SPLICES the body onto the tape (CompileExecutesBody, e.g. `var`):
		//     the handler returns tape-coupled tokens the VM cannot run, so baking a
		//     CALL_NATIVE (which an inert word-list body would otherwise permit)
		//     trips the VM's tape-coupled-result screen; OR
		//   - it EXECUTES the body via InvokeBody (sig.Callable != nil — each / fold
		//     / scan / filter / do / case / …) AND the body references a NAME
		//     (execBodyRefsNames). Such a word is normally compiled by the closure
		//     path (PUSH_CLOSURE); if that path declined, const-baking the body and
		//     RE-RUNNING it in a sub-engine is unsound when the body names a binding,
		//     because the sub-engine resolves a name the COMPILED context holds as a
		//     VM frame local (a fn param/capture, a `for` iterator, a promoted
		//     value-`def`) against the registry instead — diverging (the property
		//     fuzzer's var-block bodies caught this across all three frame-local
		//     kinds). A body of pure inert DATA with no name references (`do [10 20
		//     30]`) re-runs identically, so it still bakes. (Data-reading code words
		//     like the query-DSL clauses declare NO CallableSpec, so their inert
		//     clause always bakes a plain CALL_NATIVE.)
		es.SiteCounts[SiteMeta]++
		es.MarkUncompilable("code-body word " + word + " (Stage 2)")
	case hasUncoveredQuoteArg(sig) && word != "get" && word != "getr" && word != "set" && !quoteInertOK:
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
		// quoteInertOK is the principled extension of that exemption to a MODULE
		// INNER NATIVE whose quoted operands are inert Atom consts — the query
		// DSL's table names (`Query.from people`, `Query.join visits`): the inner
		// native is reached via the wrapper's trivial delegation, so the
		// interpreter runs the SAME handler with the same baked atom.
		es.SiteCounts[SiteMeta]++
		es.MarkUncompilable("quoted-operand word " + word)
	case anyDynamicCarrier(args):
		es.SiteCounts[SiteDynamic]++
		es.MarkUncompilable("dynamic input at " + word)
	case anyDynamicCarrier(outs) && !forceDynOut:
		// Dynamic outputs mean the checker could not type the word
		// (missing annotations, opaque wrappers like a def-bound
		// usurp value): the recorded signature is a best guess, not a
		// proof — don't bake it in. forceDynOut (dynOutNativeOK) is the
		// exception: a CONCRETE-args core builtin whose dynamic result is a
		// declared-Any return bakes faithfully and falls through here.
		es.SiteCounts[SiteDynamic]++
		es.MarkUncompilable("unannotated or opaque word " + word)
	case word == "break" && len(outs) == 0:
		// A flow-control terminator, not a call: the enclosing loop's
		// lowering turns it into a JMP to the loop end.
		es.appendEvent(emitEvent{kind: evBreak, call: emitCall{word: word, pos: pos}})
	case word == "continue" && len(outs) == 0:
		es.appendEvent(emitEvent{kind: evContinue, call: emitCall{word: word, pos: pos}})
	case word == "for" && makeListRange(es, args):
		// `for` over a COMPUTED range list (`for [1, (1 add 2)] [i]`) — the range
		// assembled via OpMakeList. A literal-const or local range lowers fine
		// (OpForSetup, or a CALL_NATIVE over the const list), but the CALL_NATIVE
		// for-handler over a RUNTIME-assembled range diverges, so keep it refused
		// (the interpreter handles the computed range).
		es.MarkUncompilable("for: computed range list (Stage 2 follow-on)")
	default:
		return false
	}
	return true
}

// recordCallOperands resolves a lowerable native dispatch's operands. It refuses
// (marking the program uncompilable, returning ok=false) when a fn-valued operand
// would reach a fn-INVOKING word — that handler re-steps the fn on the tape,
// which the VM cannot honour (Stage 3). A word declaring CompileReadsFn
// (introspection) or CompileStoresFn (minilang/parselang register) treats its
// fn-valued argument as INERT data — read or stashed, never invoked on the VM
// tape — so the fn rides as a plain const operand; introspect (READS only) bakes
// even a CAPTURING fn, since only the immutable shape is read.
//
// MUTATION-SAFETY (load-bearing, see isInertConst): the only 0-result words are
// in-place mutators on Array/Object/Store/context receivers, and those instance
// types are NOT const-bakeable (absent from isInertConst's whitelist), so a
// pooled compound const can never reach one — the receiver is always a computed
// event or a frame local.
func (es *EmitState) recordCallOperands(word string, sig *Signature, args []Value) ([]emitOperand, bool) {
	introspect := sig.CompileEffect.Has(CompileReadsFn)
	inertFn := introspect || sig.CompileEffect.Has(CompileStoresFn)
	for _, t := range sig.Args {
		if t != nil && (t.ConformsTo(TFunction) || t.ConformsTo(TFnDef)) {
			if inertFn {
				continue
			}
			es.SiteCounts[SiteMeta]++
			es.MarkUncompilable("function-valued operand at " + word + " (Stage 3)")
			return nil, false
		}
	}
	for _, a := range args {
		if _, ok := a.Data.(FnDefInfo); ok {
			if inertFn {
				continue
			}
			es.SiteCounts[SiteMeta]++
			es.MarkUncompilable("function value reaches " + word + " (Stage 3)")
			return nil, false
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
		if !ok {
			es.MarkUncompilable("operand of unknown provenance or not statically materialisable at " + word)
			return nil, false
		}
		ops[i] = op
	}
	return ops, true
}

// RecordPolyCall records a native dispatch the checker could not commit to
// one overload for (a dynamic operand widened to Any): the call lowers to
// OpCallNativePoly, which re-matches the word's signatures at run time (plan
// P3). Operands resolve normally (the dynamic one is a prior event's result);
// returns false, leaving es untouched, when one is of unknown provenance.
func (es *EmitState) RecordPolyCall(word string, args, outs []Value, pos SrcPos, ownerReg *Registry) bool {
	if !es.active() || len(outs) > 1 {
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
	seq := es.appendEvent(emitEvent{kind: evCall, call: emitCall{word: word, ops: ops, nout: len(outs), pos: pos, poly: true, polyReg: ownerReg}})
	// A 0-output poly (a side-effect word like the test framework's
	// `test-record`) produces no stack value to register.
	if len(outs) == 1 {
		es.setProduced(outs[0], seq)
	}
	return true
}

// producerWord returns the word of the event that produced value id, when id
// resolves to a recorded event (not a const / local / unproduced value). Used to
// gate makelist on its elements being core-builtin (deterministic) results.
func (es *EmitState) producerWord(id string) (string, bool) {
	pr, ok := es.producedBy[id]
	if !ok {
		return "", false
	}
	for _, fr := range es.frames {
		for i := range fr {
			if fr[i].seq == pr.seq {
				return fr[i].call.word, true
			}
		}
	}
	return "", false
}

// makeListRange reports whether any of a dispatch's args was produced by an
// OpMakeList assembly (the synthetic "[…]" word) — used to keep `for` off a
// computed range list.
func makeListRange(es *EmitState, args []Value) bool {
	for i := range args {
		if w, ok := es.producerWord(args[i].ID); ok && w == wordMakeList {
			return true
		}
	}
	return false
}

// RecordMakeList records the assembly of a COMPUTED list literal (`[1 add 2]`,
// `[1 (2 add 3) 4]`): autoEvalList evaluated the elements (their dispatches
// already recorded their own events) and `ins` are the resulting element
// values, in order; `out` is the list. The N element operands resolve normally
// (an event result, a const, a local) and the dispatch lowers to OpMakeList N,
// which pops them and pushes the list. Returns false — leaving es untouched, so
// the list stays an unresolvable residual and the program falls back — when an
// element has no compiled home (e.g. a fn value, a nested dynamic carrier).
func (es *EmitState) RecordMakeList(r *Registry, ins []Value, out Value, pos SrcPos) bool {
	if !es.active() {
		return false
	}
	// Only the TOP-LEVEL frame. A list inside a fn body / higher-order closure /
	// branch arm (a nested fragment) is RE-EVALUATED per call or iteration, often
	// with a different scope (`fn […[[c1]]]`, where c1 rebinds). Baking ONE
	// assembly of the check-mode evaluation would freeze that — so those keep
	// refusing and fall back. A top-level computed list is evaluated once.
	if len(es.frames) != 1 {
		return false
	}
	// ops are in SIG order (ops[0] = top of stack), but a list assembles with
	// element 0 DEEPEST, so reverse: ops[0] is the LAST element (laid out on
	// top), ops[N-1] the first (deepest). OpMakeList then pops [first..last] and
	// builds [first..last] in order. Each element must be produced by a CORE
	// BUILTIN (or be a const): a builtin that yields a value is deterministic and
	// side-effect-free, so the lowered re-computation matches. A MODULE / user
	// word may be stateful — `list-of [Rand.int 0 10] 3` leaks its NoEval
	// generator to the residual, and freezing one `rand-int` (which advances the
	// seed) would replicate it instead of re-running per iteration. Those refuse.
	ops := make([]emitOperand, len(ins))
	for i := range ins {
		// A TYPE-pattern list (`[Integer]`, `[Integer String]`) is the operand of
		// the type machinery (`Box of [Integer]`, `x is [Integer String]`), not a
		// DATA list to assemble — baking it as an OpMakeList breaks that dispatch.
		// A genuine data list never holds a bare type node.
		if IsBareTypeNode(ins[i]) {
			return false
		}
		if w, isEvent := es.producerWord(ins[i].ID); isEvent && !r.IsBuiltinWord(w) {
			// A MODULE / user word may be stateful (`list-of [Rand.int …] N` — the
			// generator advances a seed), so freezing one assembly of its check-mode
			// result would replicate it instead of re-running per use. Those refuse.
			// `make` is NO LONGER excluded: an OpMakeList of make EVENTS rebuilds
			// fresh instances per run (sound, like OpMakeMap), so an instance list —
			// `def xs [(make Box …) (make Box …)]`, typed or not — assembles natively
			// and feeds each/fold/get rather than refusing on the const-bake path
			// (where isInertConst rejects the mutable members).
			return false
		}
		op, ok := es.resolveOperand(ins[i])
		if !ok {
			return false
		}
		ops[len(ins)-1-i] = op
	}
	es.SiteCounts[SiteMono]++
	seq := es.appendEvent(emitEvent{kind: evCall, call: emitCall{word: wordMakeList, ops: ops, nout: 1, pos: pos, makeList: true}})
	es.setProduced(out, seq)
	return true
}

// RecordMakeMap records the assembly of a COMPUTED map literal whose values are
// not bakeable as an inert const — `make`'s construction body with a computed
// field value (`make Outer {i:(make Inner …)}`, `{a:(context get 'x')}`).
// autoEvalMap evaluated each value (their dispatches already recorded their own
// events) and `vals` are the resulting values in `keys` order; `out` is the map.
// The N value operands resolve normally (an event result, a const, a local) and
// the dispatch lowers to OpMakeMap, which pops them and pairs each with its key.
// The keys ride in the Program (MakeMapSpec) rather than the stack, so only the
// values are operands. Returns false — leaving es untouched, so the map stays an
// unresolvable residual and the program falls back — when a value has no compiled
// home (a fn value, a nested dynamic carrier) or is a bare type node (a
// type-pattern map, not a data map). The sole caller (autoEvalMap) gates this on
// dataMap — make's CONSUMED construction body — which make evaluates in the
// current scope, so it is sound inside a fn body / branch arm: the OpMakeMap
// re-assembles from its operands per call, never frozen, and is never a deferred
// residual. No top-frame restriction here.
func (es *EmitState) RecordMakeMap(r *Registry, keys []string, vals []Value, implicit bool, out Value, pos SrcPos) bool {
	if !es.active() || len(keys) != len(vals) || len(keys) == 0 {
		return false
	}
	// ops are in value order (vals[0] pairs with keys[0]); OpMakeMap reads the
	// popped run deepest-first as value 0, so reverse like RecordMakeList:
	// ops[0] is the LAST value (laid out on top), ops[N-1] the first (deepest).
	ops := make([]emitOperand, len(vals))
	for i := range vals {
		// A bare type node value means a TYPE-pattern map (`{a:Integer}`) — the
		// operand of `is`/`typeof`, not a data map to assemble. Never a make body.
		if IsBareTypeNode(vals[i]) {
			return false
		}
		// A computed value produced by `make` (a mutable instance) is exactly what
		// this assembly exists to thread as a fresh per-run event — unlike
		// RecordMakeList, which keeps instance lists on the typed-def const-bake
		// path. So no make-producer exclusion here.
		op, ok := es.resolveOperand(vals[i])
		if !ok {
			return false
		}
		ops[len(vals)-1-i] = op
	}
	es.SiteCounts[SiteMono]++
	seq := es.appendEvent(emitEvent{kind: evCall, call: emitCall{
		word: wordMakeMap, ops: ops, nout: 1, pos: pos,
		makeMap: true, mapKeys: append([]string(nil), keys...), mapImpl: implicit,
	}})
	es.setProduced(out, seq)
	return true
}

// RecordClosureCall records a higher-order word's dispatch where the code BODY
// at position bodyPos was compiled to closure unit `unit` (plan P2). The body
// operand lowers to OpPushClosure (the handler invokes it through the VM via
// the InvokeBody seam); the other operands resolve normally. Returns false,
// leaving es UNTOUCHED, when an operand is dynamic or of unknown provenance —
// the caller then keeps the island path.
func (es *EmitState) RecordClosureCall(word string, sig *Signature, args []Value, bodyPos, unit int, capOps []emitOperand, outs []Value, pos SrcPos) bool {
	if !es.active() || sig == nil || len(outs) > 1 {
		return false
	}
	// A dynamic OUTPUT is fine — the body is compiled and the result type being
	// Any only means a downstream typed dispatch over it polys or refuses. A
	// dynamic non-body INPUT is handled by resolveOperand below: when it is a
	// resolvable event (a caught `do` error reaching `error [handler]`, whose
	// closure input is a FIXED carrier independent of the dynamic value — so the
	// handler runs faithfully over whatever the value turns out to be) it rides
	// as a stack operand; when it cannot resolve, resolveOperand declines and the
	// island path stands. Faithfulness rides the differential gate.
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
	seq := es.appendEvent(emitEvent{kind: evCall, call: emitCall{word: word, sig: sig, ops: ops, nout: len(outs), pos: pos}})
	if len(outs) == 1 {
		es.setProduced(outs[0], seq)
	}
	return true
}

// hasUncoveredQuoteArg reports whether sig has a QuoteArgs position that is NOT
// also a NoEvalArgs position. The QuoteArgs refusal targets dispatch-
// manipulating meta operands (usurp / force-arity / ref-family) whose result the
// engine re-steps; a quoted operand that is ALSO a NoEvalArgs code body
// (`timeout 1000 [body]`, `interval`) is already validated as inert by the
// noEvalBodiesInert path and bakes as a plain const, so it must not be double-
// refused here.
func hasUncoveredQuoteArg(sig *Signature) bool {
	for i := range sig.QuoteArgs {
		if sig.QuoteArgs[i] && !sig.NoEvalArgs[i] {
			return true
		}
	}
	return false
}

// noEvalBodiesInert reports whether every NoEvalArgs (un-evaluated code-body)
// position holds INERT data — a const-bakeable word-list / scalar with no
// computed paren or carrier (e.g. a query clause `[name age]` / `[age gt 1]`,
// read or stored as data by the handler). Such a body bakes as a const and the
// dispatch lowers to a plain CALL_NATIVE: the handler does exactly what it does
// under the interpreter, byte-identically. A body with a computed paren (a test
// assertion `[(1 add 1) …]`) is NOT inert and keeps the conservative refusal.
func noEvalBodiesInert(sig *Signature, args []Value) bool {
	for i := range args {
		if !sig.NoEvalArgs[i] {
			continue
		}
		if !isInertConst(args[i]) {
			return false
		}
		// A flow-control sentinel (break/continue/return) inside the body
		// targets an ENCLOSING loop/frame; running the body inside the handler
		// (the CALL_NATIVE this enables) cannot propagate that across the call
		// boundary, so it would diverge (`each [break]`). Keep those refused.
		if bodyHasSentinel(args[i]) {
			return false
		}
	}
	return true
}

// execBodyRefsNames reports whether a body-EXECUTING word's (sig.Callable != nil)
// inert NoEvalArgs body references a NAME — any Word or Reach token, at any depth.
// Such a token is resolved at run time: if the const-baked body is re-run in a
// sub-engine, the name resolves against the registry, but the compiled context may
// hold it as a VM frame local (fn param/capture, `for` iterator, promoted
// value-`def`) — so the re-run diverges. A body of pure inert DATA (scalars, data
// lists/maps — `do [10 20 30]`) references nothing and re-runs identically. Found
// by the property fuzzer's var-block closure bodies.
func execBodyRefsNames(sig *Signature, args []Value) bool {
	for i := range args {
		if !sig.NoEvalArgs[i] {
			continue
		}
		if valueRefsName(args[i]) {
			return true
		}
	}
	return false
}

// valueRefsName recursively reports whether v contains a Word or Reach token —
// a name that resolves to a binding at run time — anywhere in its structure
// (list elements, map values, paren-expr tokens). A Splice marker also wraps a
// name-bearing payload, so it counts. Conservative: an unknown payload that
// could carry a name is NOT data, so only the known pure-data shapes return
// false.
func valueRefsName(v Value) bool {
	if IsWord(v) || IsReach(v) || IsSplice(v) {
		return true
	}
	switch d := v.Data.(type) {
	case ListPayload:
		for _, e := range d.Elems {
			if valueRefsName(e) {
				return true
			}
		}
	case MapPayload:
		if d.M == nil {
			return false
		}
		for _, k := range d.M.Keys() {
			mv, _ := d.M.Get(k)
			if valueRefsName(mv) {
				return true
			}
		}
	case ParenExprPayload:
		for _, tk := range d.Toks {
			if valueRefsName(tk) {
				return true
			}
		}
	}
	return false
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
	if v.Parent.Equal(TList) || v.Parent.Equal(TMap) || isTypeBodyPayload(v) || IsParenExpr(v) {
		// Compounds, structural type bodies, and codequote'd ParenExprs are never
		// deduped: like the list/map identity rule, two source codequotes stay two
		// distinct const values rather than CanonValue-merging into one.
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

// isFreshenedInstance reports whether v is a concrete MUTABLE instance that
// make's FreshenDefault (core_make.go) copies per instance when v is a
// class-schema field default — an Object/Array/Store/flex value. Admitting one
// as a SCHEMA member (ONLY through typeBodyConstOK's memberOK, never standalone)
// is mutation-safe precisely because every `make` freshens it into its own copy;
// outside a type body nothing freshens it, so isInertConst keeps it out of the
// const pool. Pairs with the const-bake regression gate.
func isFreshenedInstance(v Value) bool {
	return IsObjectInstance(v) || IsArray(v) || IsStore(v) || IsFlexList(v)
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

// allInert reports whether pred holds for every value in ms — the shared
// "every member must be const-safe" loop of the type-body const-bake checks.
func allInert(ms []Value, pred func(Value) bool) bool {
	for _, m := range ms {
		if !pred(m) {
			return false
		}
	}
	return true
}

// allFieldsInert reports whether pred holds for every value in an ordered map
// (a nil map is not const-bakeable). Shared by the structural-type-body and
// surface-type const-bake checks.
func allFieldsInert(m *OrderedMap, pred func(Value) bool) bool {
	if m == nil {
		return false
	}
	for _, k := range m.Keys() {
		fv, _ := m.Get(k)
		if !pred(fv) {
			return false
		}
	}
	return true
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
	return typeBodyConstOKParam(v, nil)
}

// typeBodyConstOKParam is typeBodyConstOK with an optional type-parameter
// predicate. When isParam != nil (the generic-schema path), a Word member
// naming one of the schema's parameters is admitted — those Words are the
// schema body's placeholder references (`[:T]`), resolved by name at
// instantiation, never re-stepped at the engine pointer. With isParam == nil
// (every other caller) the check is exactly the strict original.
func typeBodyConstOKParam(v Value, isParam func(string) bool) bool {
	if v.Carrier || v.Dynamic {
		return false
	}
	if IsBareTypeNode(v) {
		return true
	}
	if isParam != nil {
		if w, err := AsWord(v); err == nil && isParam(w.Name) {
			return true
		}
	}
	memberOK := func(m Value) bool {
		// A concrete mutable instance default (`class {x:(make Foo 1)}`,
		// `{items:(flex [])}`, `{bits:(make Array [0 0 0])}`) is a const-safe SCHEMA
		// member: `make` runs FreshenDefault over every field default
		// (core_make.go), handing each instance its OWN fresh copy, so the baked
		// body is a READ-ONLY TEMPLATE — never the mutable value a later `set`
		// writes. The mutation-safety invariant therefore holds for a default
		// INSIDE a type body, even though isInertConst still (correctly) rejects
		// the same instance standing alone or as a data-list member, where nothing
		// freshens it. Const-folded by constFoldContainerVal.
		if !m.Carrier && !m.Dynamic && isFreshenedInstance(m) {
			return true
		}
		return typeBodyConstOKParam(m, isParam) || isInertConst(m)
	}
	switch d := v.Data.(type) {
	case RecordTypeInfo:
		return allFieldsInert(d.Fields, memberOK)
	case OptionsTypeInfo:
		return allFieldsInert(d.Fields, memberOK)
	case ChildTypeInfo:
		if !memberOK(d.Child) {
			return false
		}
		if !allInert(d.Elements, memberOK) {
			return false
		}
		for _, en := range d.Entries {
			if !memberOK(en.Value) {
				return false
			}
		}
		return true
	case DisjunctInfo:
		return allInert(d.Alternatives, memberOK)
	case ObjectTypeInfo:
		// A class / object type body is const-bakeable iff every field
		// default is plain data — a method (fn-value) field is not, so a
		// class with methods (the surface-body case) still refuses. The
		// canonical *Type rides the body's payload pointer (shared, not
		// copied), so it stays canonical at run time; `make` recovers the
		// field schema from the baked body. The parent chain's fields must
		// be data too (AllFields merges them).
		return allFieldsInert(d.AllFields(), memberOK)
	case TableTypeInfo:
		// A Table type body (`make Test.TestSet …`, a module-exported
		// Table type as a get-fold result or residual) is a thin wrapper
		// over the row RecordType — const-safe exactly when that record's
		// field types are. The canonical *Type rides the body's payload
		// pointer (shared, not copied), like every structural body.
		return allFieldsInert(d.Record.Fields, memberOK)
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
		return true
	case RecordTypeInfo, OptionsTypeInfo, ChildTypeInfo, DisjunctInfo, ObjectTypeInfo, TableTypeInfo:
		// STRUCTURAL type bodies (what a bound type name pushes at a
		// use site — make's operand). Sound as consts when their
		// interior is carrier-free (typeBodyConstOK): the payload is
		// pointer-backed (shared, not copied) and the minted lattice
		// node rides the body's Parent POINTER, which stays
		// canonical. Never deduped. A class/object body qualifies only
		// when every field default is data (no method fn-values). A
		// Table type (`Test.TestSet`) is a thin wrapper over its row
		// RecordType, so it folds whenever that record does.
		return typeBodyConstOK(v)
	case FnUndefInfo:
		// A function SIGNATURE value (`fnsig [[Integer] [String]]`,
		// `typeof (Mapper of [Integer String])`): pure descriptor data —
		// param/return *Type pointers, no invocable body and no mutable state —
		// so it bakes by value (the *Type pointers are shared, already
		// canonical from construction). typeof/teq/is then read the baked
		// signature at run time. A pattern-bearing param (rare in signatures)
		// could embed non-const data, so it refuses conservatively.
		return fnSigConstOK(d)
	case FnDefInfo:
		// A function VALUE used as DATA — a residual (`f/r`), a map/list member
		// (`{b:f/r}`), or an introspection operand (`arityof (fn …)`) — NOT a
		// call site. It bakes as a const only with no closure state: no captured
		// bindings (which would snapshot check-pass values, divergent from the VM
		// pass) and no module sub-registry. The body tokens ride inside the
		// payload and are never re-stepped while the value is data; a CALL of the
		// value is a separate dispatch path (a bare `(fn …) args` auto-dispatch
		// records the fn-body splice and refuses; a `/r`-referenced fn does not
		// auto-dispatch, so `f/r` / `{b:f/r}` are pure data).
		if len(d.Captured) > 0 {
			return false
		}
		if d.Registry == nil {
			return true
		}
		// A module-export TRIVIAL-DELEGATION wrapper (`MathUtil.sqrt`): every own
		// sig is a `[Word(inner)]` pass-through, so it holds no closure state to
		// snapshot. The sub-registry pointer it carries is the SAME object the
		// compiled run shares (RunProgram runs on the check-pass registry), and
		// the VM applies it faithfully at run time via callDynamic →
		// tryNativeFnApply (which re-resolves the inner native in that registry).
		// So it bakes as DATA — a bare residual (`MathUtil.sqrt`), a branch-arm
		// operand (`if c [99] MathUtil.sqrt`), or a container member. A module fn
		// with a REAL AQL body is NOT a delegation wrapper and stays refused (its
		// body would need CallAQL in the sub-registry, which the VM cannot do).
		return isDelegationFnDef(d)
	case *SurfaceInfo:
		// A surface type (`def Shape surface {area: (fnsig …)}`): an immutable
		// contract descriptor riding its canonical minted node via the Type
		// pointer (shared, not copied). Its conformance set (Conform, filled in
		// by `exposes`) is consulted through the SAME shared payload the
		// canonical node's installed unifier holds — and the compiled path runs
		// the VM over the check-pass registry without re-minting, so the baked
		// const and the live surface are one object, never divergent. Admitting
		// it lets `Shape` as a residual / operand to is/typeof/teq/tand/tor/tnot/
		// unify all compile. The Required method shapes are fnsig values, const
		// by fnSigConstOK above.
		return surfaceConstOK(d)
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
	case XmlElementPayload:
		// An immutable Node/Xml literal (`<a x="1"><b/>text</a>`). It is a
		// constant value at parse time (parser/xml_literal.go emits
		// NewXmlElement; only a ${}-interpolated literal becomes the deferred
		// Word/__XI builder, which is genuine runtime construction and stays
		// refused). Value-semantics with structural sharing — never mutated in
		// place: the MUTABLE FlexXml is *FlexXmlData, which falls to default and
		// never bakes (the bytecode_constbake_test mutation-safety guard) — so it
		// is sound to pool, exactly like the List / Map cases. Bakes when its
		// attribute values and child nodes are themselves inert members (text
		// scalars + nested immutable elements, recursed via isInertConstMember →
		// isInertConst).
		for _, c := range d.Cren {
			if !isInertConstMember(c) {
				return false
			}
		}
		if d.Attr != nil {
			for _, k := range d.Attr.Keys() {
				av, _ := d.Attr.Get(k)
				if !isInertConstMember(av) {
					return false
				}
			}
		}
		return true
	case ReachInfo:
		// A receiverless inert lens (`$.name`, `$.a.b`, `$!.x`, `$.1`). See
		// isInertReach: only the non-eval, no-receiver, all-literal-key shape
		// qualifies — the dot-access Eval reach (which the engine expands in
		// place) is excluded.
		return isInertReach(v)
	case ParenExprPayload:
		// A codequote'd (Quoted) ParenExpr — `codequote (1 add 2)` →
		// `paren([1 word(add) 2])`. It is immutable CODE-AS-DATA: stepLiteral
		// leaves a Quoted ParenExpr unevaluated (engine.go step 4), and the VM
		// never re-steps a const, so it bakes by value exactly like the
		// macroexpand token list. An UNQUOTED ParenExpr is expanded and
		// re-stepped in place, so it is NEVER a const — gate strictly on Quoted.
		// Its tokens must themselves be inert members (Words / atoms / scalars /
		// nested inert parens), screened by isInertConstMember.
		return isInertQuotedParen(v)
	case *TypeSchemaInfo:
		// An installed generic schema (`def Box gen [T] class {value:T}`). The
		// schema is immutable data — its instantiation memo lives in the
		// registry, not this struct — and rides the canonical minted node via
		// its Type pointer (shared, not copied), so it bakes as a const exactly
		// like a structural type body. Admitting it lets `make Box {…}` (T
		// inferred at run time), `is`/`typeof`/`teq` over the schema, and the
		// schema as a residual all compile; the instantiated `Box of [Integer]`
		// type already baked via typeBodyConstOK.
		return schemaConstOK(d)
	default:
		return false
	}
}

// fnSigConstOK reports whether a function-signature value bakes as a const.
// The signature is pure type/descriptor data; the only embeddable Value is an
// optional structural pattern on a param, which must itself be const-safe (a
// pattern that carried a carrier or data map would not be inert).
func fnSigConstOK(info FnUndefInfo) bool {
	for _, sig := range info.Sigs {
		for _, p := range sig.Params {
			if p.Pattern != nil && !(isInertConst(*p.Pattern) || typeBodyConstOK(*p.Pattern)) {
				return false
			}
		}
	}
	return true
}

// surfaceConstOK reports whether a surface type bakes as a const: it must ride
// a canonical node (Type != nil) and every required-operation shape must be a
// const-safe value (a fnsig descriptor, or any other inert member).
func surfaceConstOK(s *SurfaceInfo) bool {
	if s == nil || s.Type == nil {
		return false
	}
	if s.Required == nil {
		return true
	}
	return allFieldsInert(s.Required, func(v Value) bool {
		return isInertConst(v) || typeBodyConstOK(v)
	})
}

// schemaConstOK reports whether a generic schema bakes as a const: its body
// must be a const-safe structural type body (or itself inert / a bare node)
// and every parameter's extends-bound / default must be inert or a bare type
// node. A computed constraint or a non-data (method-bearing fn) body refuses,
// falling back faithfully.
func schemaConstOK(s *TypeSchemaInfo) bool {
	if s == nil || s.Type == nil {
		return false
	}
	// The schema body embeds its type variables as unresolved Words (`[:T]`
	// stores the Word `T`, which `of`/`make` later resolve by name). Those
	// Words are inert schema data — the body is consumed by make/of/is/typeof,
	// never re-stepped at the engine pointer — so the body check admits a Word
	// naming one of this schema's own parameters.
	isParam := func(name string) bool {
		for _, p := range s.Params {
			if p.Name == name {
				return true
			}
		}
		return false
	}
	memberOK := func(m Value) bool {
		return IsBareTypeNode(m) || typeBodyConstOKParam(m, isParam) || isInertConst(m)
	}
	for _, p := range s.Params {
		if p.HasBound && !memberOK(p.Bound) {
			return false
		}
		if p.HasDefault && !memberOK(p.Default) {
			return false
		}
	}
	return memberOK(s.Body)
}

// isInertQuotedParen reports whether v is a codequote'd (Quoted) ParenExpr that
// bakes as a const. A Quoted ParenExpr is CODE-AS-DATA: the interpreter's
// stepLiteral leaves it unevaluated (engine.go step 4 gates on `!v.Quoted`),
// and the VM never re-steps a const, so it pushes verbatim like the macroexpand
// token list — the compiled residual renders byte-identically to the
// interpreter's data value. An UNQUOTED ParenExpr is expanded and re-stepped in
// place, so it is NEVER baked here. Every token must itself be an inert const
// member (Words / atoms / scalars / nested inert parens), via isInertConstMember.
func isInertQuotedParen(v Value) bool {
	if !v.Quoted || v.Carrier || v.Dynamic {
		return false
	}
	toks, err := AsParenExpr(v)
	if err != nil {
		return false
	}
	for _, tk := range toks {
		if !isInertConstMember(tk) {
			return false
		}
	}
	return true
}

// isInertReach reports whether v is an INERT receiverless lens — a first-class
// Reach value that evaluates to ITSELF (`$.name`, `$.a.b`, `$!.x`, `$.1`):
// Eval=false, no Receiver tokens, and every segment a LITERAL key (no computed
// paren to evaluate at run time). Such a lens is immutable data, not code: it
// renders as itself, `typeof` reads Reach, and `apply`/`each`/`filter`/`sortby`
// /`getpath` walk its segments against a FRESH receiver — none of which the
// engine expands or re-steps. That is the opposite of a dot-access Eval reach
// (`m.a.b`), which `isEvalReach`/`expandReach` lower to a get-chain IN PLACE;
// isInertConst rightly keeps THAT one out (it is a structural token, not data).
// The lens keys carry no canonical-*Type staleness hazard: they are Words /
// Atoms / scalars whose Parents are the canonical kernel types, copied by value
// safely (isInertConstMember screens out a bare type node). So the inert lens
// pools into the const table like an atom or a path.
func isInertReach(v Value) bool {
	if !IsReach(v) || v.Carrier || v.Dynamic {
		return false
	}
	info, err := AsReach(v)
	if err != nil || info.Eval || len(info.Receiver) > 0 {
		return false
	}
	for _, seg := range info.Segments {
		if seg.Computed || !isInertConstMember(seg.KeyLit) {
			return false
		}
	}
	return true
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
		// A Word token riding inside a quoted (non-eval) compound — what
		// `macroexpand` returns as data (`[5 word(add) 5]`). Safe as a const
		// MEMBER: the compound is pushed as inert data and never auto-evaluated
		// (a source eval-list is reduced before baking), and a word's Parent is
		// the canonical kernel TWord, so the by-value copy carries no stale
		// behaviour (unlike a bare type node — the canonical-*Type hazard — which
		// is deliberately NOT admitted here). The standalone isInertConst switch
		// still rejects a bare Word, so a top-level word never bakes as code.
		if IsWord(v) {
			return true
		}
		// A bare type node as a structural-pattern MEMBER — the type leaves of
		// `{a:Integer}`, `[Integer String]`, `[Resource Entity]`: the inert
		// operand of a static `is` / `typeof` / `size`. Admitted as a const
		// member (it was previously excluded outright). Soundness: the member's
		// Parent is the canonical lattice pointer the parser resolved, copied by
		// value inside a READ-ONLY container, and the whole pattern bakes as one
		// const the VM pushes verbatim — the `is`/`typeof` handler then runs
		// byte-identically to the interpreter over the same value. The standalone
		// isInertConst switch still rejects a bare type node, so a top-level type
		// operand keeps reaching the runtime via the by-ID type table
		// (OpPushType), the canonical path that survives a later `behave`.
		if IsBareTypeNode(v) && v.ID != "" {
			return true
		}
		if fd, ok := v.Data.(FnDefInfo); ok {
			return len(fd.Captured) == 0 && fd.Registry == nil
		}
		// A dot-access reach (`r.int`, `m.a.b`) riding inside a NEVER-evaluated
		// compound — a NoEvalArgs code body the driving word stores or drops
		// (Test.prop builds a PropertySpec map; Test.skip discards it;
		// Test.check-prop CallAQLs it via its native handler), or a quoted code
		// list. Unlike isInertReach (the STANDALONE detached lens, which must be
		// receiverless + Eval=false so the engine never expands it at the
		// pointer), a reach as a MEMBER is pure DATA: the VM pushes the baked
		// compound verbatim and never expands a reach (in-place expansion is an
		// interpreter stepLiteral behaviour), and the interpreter equally keeps it
		// as data inside the inert compound — so the reach bakes by value,
		// differential-identical. Its receiver / literal-key tokens must
		// themselves be inert members (Words / atoms / scalars, canonical
		// Parents); a COMPUTED segment (a paren to evaluate) is code, so refuse.
		if IsReach(v) {
			return inertReachMember(v)
		}
		// A deferred paren expression (`(add 1 2)`, `(k)`) riding inside a
		// NEVER-evaluated compound — a `reach` key list's computed segment
		// (`reach 5 [a (add 1 2) c]`), where the paren is stored unevaluated and
		// re-run at APPLY time over the shared registry, not stepped at the VM
		// pointer. Like the Word / Reach member cases this is pure DATA: the VM
		// pushes the baked compound verbatim and the reach handler builds the
		// same Computed segment the interpreter does, so it bakes by value
		// differential-identically. Its tokens must themselves be inert members
		// (Words / atoms / scalars / nested inert parens) — a token that would
		// drag in a carrier or mutable instance refuses.
		if IsParenExpr(v) {
			toks, err := AsParenExpr(v)
			if err != nil {
				return false
			}
			for _, tk := range toks {
				if !isInertConstMember(tk) {
					return false
				}
			}
			return true
		}
	}
	return isInertConst(v)
}

// inertReachMember reports whether a Reach may ride as a MEMBER of an inert
// const compound (see isInertConstMember's reach clause). It is deliberately
// more permissive than isInertReach: a member reach is never expanded at the
// engine pointer (the containing compound is inert), so a receiver and Eval=true
// are fine — only a computed segment (a ParenExpr to run) or a non-inert
// receiver/key token disqualifies it.
func inertReachMember(v Value) bool {
	if !IsReach(v) || v.Carrier || v.Dynamic {
		return false
	}
	info, err := AsReach(v)
	if err != nil {
		return false
	}
	for _, rt := range info.Receiver {
		if !isInertConstMember(rt) {
			return false
		}
	}
	for _, seg := range info.Segments {
		if seg.Computed || !isInertConstMember(seg.KeyLit) {
			return false
		}
	}
	return true
}

// resolveDynamicApply classifies the residual's fn-value-call boundary (report
// §9.1) and returns the residual (rotated for a trailing apply), the apply
// opcode to emit once the residual is on the stack (0 = none), and a refusal
// reason for an fn-value shape the static residual cannot reproduce.
//
// Handled: a dynamic value LEADING the residual with static args after it
// (`r.int 0 100`); a Function/FnDef CARRIER leading it (the factory `(mk2 5)
// 10`); and a single dynamic / fn value TRAILING one static arg (`5 m.f`,
// `[..] r.one-of`) — rotated to [fn, arg] so the reconciliation lays it out
// like the leading boundary, with OpCallDynamicTrailing restoring the fn-on-top
// order if the value is not callable. Every other dynamic / fn-value-precedes-
// args shape, and any unconsumed fn-value carrier, refuses.
func (es *EmitState) resolveDynamicApply(lw *lowerer, residual []Value) ([]Value, Opcode, string) {
	// Leading dynamic value (statically Any — the checker cannot tell a Function
	// from data) with every following arg static.
	applyDynamic := false
	if len(residual) >= 2 && residual[0].Dynamic {
		applyDynamic = !anyDynamicTail(residual)
	}
	// Leading Function/FnDef CARRIER (the factory pattern: a returned closure now
	// on the stack) with no dynamic / fn value after it. A carrier always applies
	// — the one non-applied shape (an inert `f/r`) is a CONCRETE const, not a
	// carrier — so the carrier bit resolves the apply-vs-inert ambiguity.
	if !applyDynamic && len(residual) >= 2 && isFnTypedCarrier(residual[0]) {
		applyDynamic = !anyFnOrDynamicTail(residual)
	}
	// Leading BRANCH result that MAY be a fn (an arm is an fn value, so the
	// merge produced a value that is sometimes callable — `if c [99]
	// MathUtil.sqrt 16`) over static args: a runtime-conditional apply.
	// callDynamic applies the value when the branch produced the callable and
	// leaves [value, arg] when it produced the data alternative, so the single
	// OpCallDynamic is faithful on both runtime branches. (The merge widens the
	// fn arm's type to its lattice parent — Word — so the disjunct carrier no
	// longer reads as Function; the branch-event mayBeFn flag is the precise
	// signal the static residual type cannot recover.)
	if !applyDynamic && len(residual) >= 2 {
		if pr, ok := es.producedBy[residual[0].ID]; ok && es.eventInfo[pr.seq].mayBeFn {
			applyDynamic = !anyFnOrDynamicTail(residual)
		}
	}
	if applyDynamic {
		return residual, OpCallDynamic, ""
	}
	if rot, ok := es.trailingApply(lw, residual); ok {
		return rot, OpCallDynamicTrailing, ""
	}
	// MIXED boundary: a single dynamic / fn value INTERIOR to the residual with
	// static args both below and above it (`3 m.f 2`). The producing event was
	// promoted to a frame local (Finalize's mixedDynamicApplyShape gate), so the
	// residual now re-pushes in source order; OpCallDynamicMixed islands the whole
	// window verbatim. The promoted entry stays Dynamic in `residual`, so the
	// in-order ops loop maps it to its local and the window lays out unchanged.
	if _, ok := es.mixedDynamicApplyShape(residual); ok {
		return residual, OpCallDynamicMixed, ""
	}
	// Unhandled: a dynamic value mid-residual, a fn value preceding args, or an
	// unconsumed fn-value carrier (a VM closure renders unlike the interpreter's
	// FnDefInfo). All refuse so the program falls back faithfully.
	for i := 0; i+1 < len(residual); i++ {
		if residual[i].Dynamic {
			return residual, 0, "dynamic value precedes residual args (fn-value-call boundary)"
		}
		if isFnValueResidual(residual[i]) {
			return residual, 0, "fn value precedes residual args (auto-dispatch boundary)"
		}
	}
	for i := range residual {
		if isFnTypedCarrier(residual[i]) {
			return residual, 0, "unconsumed fn-value carrier in residual (closure render)"
		}
	}
	return residual, 0, ""
}

// anyDynamicTail reports whether any residual entry after the first is dynamic.
func anyDynamicTail(residual []Value) bool {
	for i := 1; i < len(residual); i++ {
		if residual[i].Dynamic {
			return true
		}
	}
	return false
}

// anyFnOrDynamicTail reports whether any residual entry after the first is a
// dynamic or fn value (so a leading carrier's args are not all static).
func anyFnOrDynamicTail(residual []Value) bool {
	for i := 1; i < len(residual); i++ {
		if residual[i].Dynamic || isFnValueResidual(residual[i]) {
			return true
		}
	}
	return false
}

// trailingApply detects the trailing fn-value auto-apply shape (`5 m.f`,
// `[data] r.one-of`): the residual's LAST entry is a dynamic / fn value produced
// by an event on top of the simulated stack, and the SINGLE entry before it is
// the static value it auto-applies to. On a match it returns the residual
// ROTATED to [fn, arg] (so the reconciliation lays it out like the leading
// boundary) and true. Bounded to one arg: with >1 the island's forward
// collection would order them opposite to the interpreter's top-down stack
// collection.
func (es *EmitState) trailingApply(lw *lowerer, residual []Value) ([]Value, bool) {
	if len(residual) != 2 {
		return residual, false
	}
	fnv := residual[1]
	pr, isEvent := es.producedBy[fnv.ID]
	if !isEvent || pr.idx != 0 || !(fnv.Dynamic || isFnValueResidual(fnv)) {
		return residual, false
	}
	if len(lw.vm) < 1 || lw.vm[len(lw.vm)-1].seq != pr.seq || lw.vm[len(lw.vm)-1].idx != 0 {
		return residual, false
	}
	arg := residual[0]
	if _, argIsEvent := es.producedBy[arg.ID]; argIsEvent || arg.Dynamic || isFnValueResidual(arg) {
		return residual, false
	}
	return []Value{fnv, arg}, true
}

// mixedDynamicApplyShape detects the MIXED fn-value-call boundary: EXACTLY one
// residual entry is a dynamic / fn value, it sits STRICTLY INTERIOR (≥1 entry
// below it AND ≥1 above it), and it is event-produced (so the producing event
// can be promoted to a frame local and the residual re-pushed in source order).
// Returns the interior index. `3 m.f 2` → residual [3, m.f, 2], index 1. The
// before/after entries need not be checked here for materialisability — the ops
// loop refuses any that cannot resolve, falling back faithfully.
func (es *EmitState) mixedDynamicApplyShape(residual []Value) (int, bool) {
	if len(residual) < 3 {
		return 0, false
	}
	dynIdx := -1
	for i, rv := range residual {
		if rv.Dynamic || isFnValueResidual(rv) {
			if dynIdx != -1 {
				return 0, false // more than one dynamic / fn value
			}
			dynIdx = i
		}
	}
	if dynIdx <= 0 || dynIdx >= len(residual)-1 {
		return 0, false // not strictly interior (leading / trailing are separate)
	}
	if pr, ok := es.producedBy[residual[dynIdx].ID]; !ok || pr.idx != 0 {
		return 0, false // not event-produced — cannot promote to a local
	}
	return dynIdx, true
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
	if es.trapAt != 0 {
		// A terminal trap (a check-mode-suppressed runtime error compiled as an
		// OpTrap) ends the program: keep the events up to and including the trap
		// — their side effects run first, exactly as in the interpreter — and
		// drop everything after it plus the residual (both unreachable: the trap
		// aborts the run, and no result is produced).
		es.frames[0] = eventsThroughSeq(es.frames[0], es.trapAt)
		residual = nil
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
	// Residual ordering. The reconciliation below seats event results in stack
	// order with literals/types as a trailing tail (on top), so it requires the
	// residual to be in event*-literal* order. A residual with an event ABOVE a
	// literal (`1 2 word [add] 10` → [literal, event]) refused as "call result
	// above a literal". When the residual is out of order — and it is NOT the
	// fn-value / dynamic auto-apply lead shape, which lays the residual out on the
	// stack for OpCallDynamic and must not be promoted — force EVERY residual
	// event to a frame local: a local re-pushes in any order, so the
	// reconciliation pushes the whole residual (locals + consts + types) in exact
	// order. The promotion only fires for an out-of-order residual, so the common
	// in-order case is untouched.
	// A residual that holds ANY fn value or dynamic value is the auto-apply
	// boundary's territory (a leading or TRAILING fn applied to its neighbours —
	// `5 m.f` → [5, fn], `[..] r.one-of`): reordering it would drop the apply and
	// diverge. Leave those to the fn-value / refusal logic below; only a residual
	// of pure resolvable data reorders.
	var forceOrder map[int]bool
	residualHasFnOrDynamic := false
	for _, rv := range residual {
		if rv.Dynamic || isFnValueResidual(rv) {
			residualHasFnOrDynamic = true
			break
		}
	}
	if !residualHasFnOrDynamic {
		seenLiteral, outOfOrder := false, false
		for _, rv := range residual {
			pr, isEvent := es.producedBy[rv.ID]
			if isEvent && pr.idx == 0 && !es.eventInfo[pr.seq].zeroOut {
				if seenLiteral {
					outOfOrder = true
				}
			} else {
				seenLiteral = true
			}
		}
		if outOfOrder {
			forceOrder = make(map[int]bool, len(residualSeqs))
			for _, seq := range residualSeqs {
				forceOrder[seq] = true
			}
		}
	}
	// MIXED fn-value-call boundary (`3 m.f 2`): the interior fn sits above a
	// before-arg literal, so the in-order reconciliation would refuse "result
	// above a literal". Promote EVERY residual event to a frame local so the whole
	// window (before-args, the fn, after-args) re-pushes in source order, ready for
	// OpCallDynamicMixed to island it. (residualHasFnOrDynamic suppressed the
	// reorder block above; this is the one fn-value shape that DOES reorder, since
	// the island consumes the window in source order, not the apply-on-top layout.)
	if _, ok := es.mixedDynamicApplyShape(residual); ok {
		forceOrder = make(map[int]bool, len(residualSeqs))
		for _, seq := range residualSeqs {
			forceOrder[seq] = true
		}
	}
	lw.promoted, lw.dead = es.planValueDefLocals(es.units[0], es.frames[0], residualSeqs, forceOrder)
	lw.markBefore, lw.variadicElse = planVariadicClaims(es.frames[0])
	// Seed the lowerer's frame-local counter from the unit's planned locals;
	// spillSeat bumps it for spill temps. Written back below so Program.NumLocals
	// covers them.
	lw.numLocals = es.units[0].numLocals
	if reason := lw.lowerEvents(es.frames[0], 0); reason != "" {
		return nil, reason, false
	}
	es.units[0].numLocals = lw.numLocals

	// Residual reconciliation.
	lastPos := SrcPos{}
	if n := len(es.frames[0]); n > 0 {
		lastPos = eventPos(es.frames[0][n-1])
	}
	// Fn-value-call boundary (report §9.1): classify the residual for a runtime
	// fn-value apply (a leading dynamic / carrier value, or a trailing dynamic /
	// fn value over one arg), returning the apply opcode and refusing the
	// shapes the static residual cannot reproduce. dynOp is emitted after the
	// residual is laid out below.
	residual, dynOp, dynReason := es.resolveDynamicApply(lw, residual)
	if dynReason != "" {
		return nil, dynReason, false
	}
	// Resolve each residual value to its operand — skipping a 0-output statement
	// guard's phantom None (zeroOut), re-pushing a promoted value-def local from
	// its slot, and materialising a bare type node / inert const — then hand the
	// operand sequence to the shared seat primitive. A variadic loop result is
	// allowed here (the program residual may absorb it), unlike a fn body.
	ops := make([]emitOperand, 0, len(residual))
	for _, rv := range residual {
		if pr, ok := es.producedBy[rv.ID]; ok {
			if es.eventInfo[pr.seq].zeroOut {
				continue
			}
			if slot, isProm := lw.promoted[pr.seq]; isProm {
				ops = append(ops, localOperand(slot))
				continue
			}
			ops = append(ops, eventOperand(pr.seq, pr.idx))
			continue
		}
		if IsBareTypeNode(rv) && rv.ID != "" {
			ops = append(ops, typeOperand(es.internType(rv)))
			continue
		}
		lit, okLit := es.materialise(rv)
		if !okLit {
			return nil, "residual value of unknown provenance", false
		}
		if !isInertConst(lit) {
			return nil, "residual value not statically materialisable", false
		}
		ops = append(ops, constOperand(es.intern(lit)))
	}
	if reason := lw.seatResults(ops, false, false, seatMsgs{
		aboveLiteral: "residual shape beyond Stage 1 (call result above a literal)",
		reordered:    "residual shape beyond Stage 1 (call results reordered)",
		unconsumed:   "residual shape beyond Stage 1 (unconsumed call results)",
	}, lastPos); reason != "" {
		return nil, reason, false
	}
	if dynOp != 0 {
		// The fn value (leading, or trailing rotated to the front) sits at the
		// base of the residual with its args above; apply it at run time. The
		// trailing op additionally restores the fn-on-top order if the value
		// turns out not to be callable (see resolveDynamicApply / callDynamic).
		// The MIXED op instead islands the WHOLE window (the fn is interior), so it
		// takes the full residual length, not len-1.
		arg := len(residual) - 1
		if dynOp == OpCallDynamicMixed {
			arg = len(residual)
		}
		lw.emit(dynOp, arg, lastPos)
	}

	// Lower the compiled fn units. Tail positions are marked first so
	// the lowering emits TAIL_CALL_USER (frame replacement — the
	// language's tail-call guarantee carries into compiled mode).
	for _, rec := range es.fnRecs {
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
		cf := CompiledFn{Name: rec.name, NParams: rec.nParams + len(rec.caps), NCaptures: len(rec.caps), NLocals: rec.numLoc, InShape: rec.inShape, Returns: rec.returns, LocalNames: names}
		flw := &lowerer{es: es, p: p, code: &cf.Code, debug: &cf.Debug, sigIdx: lw.sigIdx, variadic: map[int]bool{}, numLocals: rec.numLoc, promoted: rec.promoted, dead: rec.dead, isFnUnit: true}
		if reason := flw.lowerEvents(rec.frag.events, rec.frag.startSeq); reason != "" {
			return nil, "fn " + rec.name + ": " + reason, false
		}
		// spillSeat may have allocated frame-local temps during lowering; grow
		// NLocals (and the debug name table) so the VM frame holds them.
		if flw.numLocals > cf.NLocals {
			cf.NLocals = flw.numLocals
			for len(cf.LocalNames) < cf.NLocals {
				cf.LocalNames = append(cf.LocalNames, "")
			}
		}
		if !diverged {
			// Reconcile the body's N result operands with the simulated
			// stack and emit a RET. Event results must already sit on the
			// stack in order (they were left by their own events); inert
			// operands (const / local / type) are pushed as a trailing
			// tail above the last event result. This is the fn-unit mirror
			// of the program-residual reconciliation below.
			if reason := flw.reconcileResults(rec.outOps, "fn "+rec.name, len(rec.returns) == 0, rec.pos); reason != "" {
				return nil, reason, false
			}
			flw.emit(OpRet, 0, rec.pos)
		}
		// A fully diverging body (every path tail-calls) emits no RET —
		// control leaves via the callee's eventual RET.
		p.Fns = append(p.Fns, cf)
	}

	lw.p.Consts = es.consts // interning may have grown during reconciliation
	lw.p.Types = es.types
	lw.p.Fallbacks = es.fallbacks
	lw.p.MaxStack = lw.maxDepth
	lw.p.NumLocals = es.units[0].numLocals
	return lw.p, "", true
}

// isFnTypedCarrier reports whether v is a Function/FnDef-typed CARRIER — a
// [Function]-returning call result on the simulated stack (e.g. `(mk2 5)`), as
// distinct from a CONCRETE baked fn value (Carrier false, the introspection /
// inert-`/r` case). The carrier bit is what resolves the apply-vs-inert
// ambiguity in Finalize: a carrier lead auto-applies, a concrete fn does not.
func isFnTypedCarrier(v Value) bool {
	return v.Carrier && v.Parent != nil &&
		(v.Parent.ConformsTo(TFunction) || v.Parent.ConformsTo(TFnDef))
}

// isFnValueResidual reports whether v is ANY fn value — a concrete FnDefInfo (a
// baked /r reference) or a Function/FnDef-typed value (carrier or not). Used to
// keep a fn value out of the trailing-arg positions of a leading-fn apply.
func isFnValueResidual(v Value) bool {
	if _, ok := v.Data.(FnDefInfo); ok {
		return true
	}
	return v.Parent != nil && (v.Parent.ConformsTo(TFunction) || v.Parent.ConformsTo(TFnDef))
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
	case evTrap:
		return ev.trap.pos
	}
	return ev.br.pos
}

// eventsThroughSeq returns the prefix of events up to and including the one with
// the given seq (top-level events are appended in seq order). Used to truncate
// the trace at a terminal trap, dropping the unreachable tail the lenient check
// pass recorded after it.
func eventsThroughSeq(events []emitEvent, seq int) []emitEvent {
	for i := range events {
		if events[i].seq == seq {
			return events[:i+1]
		}
	}
	return events
}
