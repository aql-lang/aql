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
	wordMakeList  = "[…]"
	wordMakeMap   = "{…}"
	wordInterp    = "`…`"
	wordDynApply  = "(…fn)"
	wordTypedBind = "def:…"
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
	opNone     operandKind = iota // zero value: unset / invalid (ok=false only)
	opConst                       // idx → Program.Consts index
	opEvent                       // idx → producing event sequence number
	opLocal                       // idx → frame-local slot (loop iterator / param)
	opType                        // idx → Program.Types index (canonical type)
	opClosure                     // closureUnit + closureCaps (a compiled body)
	opDynScope                    // idx → Program.Consts index of the NAME a runtime dynamic-scope lookup reads
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
func dynScopeOperand(idx int) emitOperand {
	return emitOperand{kind: opDynScope, idx: idx}
}
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
	// applyLoop marks a LOOP event whose body runs a per-iteration dynamic
	// apply (setLoopBodyApply). Its variadic value region may sit in a fn-body
	// residual under an inert tail: the RET then takes the RetReplay trim
	// discipline (finish()'s loop-apply residual detection) instead of the
	// variadic refusal.
	applyLoop bool
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
	word            string
	sig             *Signature
	ops             []emitOperand
	nout            int // number of results the call pushes (0 for a side-effect word, N for multi-result)
	pos             SrcPos
	poly            bool      // dispatch via OpCallNativePoly (runtime MatchSignature)
	polyReg         *Registry // the sub-registry to re-match a module poly word in (nil = main registry)
	makeList        bool      // assemble len(ops) operands into a list (OpMakeList) instead of dispatching a word
	dynApply        int       // >0: apply the TOP operand (a runtime fn value) to the `dynApply` trailing args below it (OpCallDynTrailTop) — a paren-bounded trailing fn-value apply recorded as an EVENT so it seats like any computed result
	dynApplyUnquote bool      // the dynApply event came through the `apply` WORD (a consumed pendingApply): lower to OpCallDynApplyTop, which unquotes like applyHandler (Stage M2a)
	makeMap         bool      // assemble len(ops) value operands into a map (OpMakeMap) with mapKeys
	mapKeys         []string
	mapImpl         bool // the source map's Implicit flag
	interp          bool // assemble len(ops) hole operands into a template string (OpInterp) per interpSegs
	interpSegs      []InterpSeg
	diverges        bool // the word ALWAYS raises (CompileDiverges, e.g. raise): control never returns past this call
	// typedBind, when non-nil, marks this event as a typed value-def's runtime
	// validate/reparent step (OpBindTyped over the single operand) instead of a
	// word dispatch — recorded by RecordTypedBind from the def handler's
	// refinement branches. Riding emitCall keeps the whole generic evCall
	// machinery (operand provenance, value-def promotion, dead-drop, fragment
	// walks) working unchanged for the bind result.
	typedBind *TypedBindSpec
	// dynMethod, when non-nil, marks this event as a GUARDED shaped-instance-
	// method apply (Stage M2c, OpCallDynMethod): ops[0] is the runtime method
	// value (the dynamic dot-read result), ops[1..] the inert statement-window
	// args, and the spec pins the claimed arity + result count the VM enforces
	// (claim failure → internal_error → interpreter re-run). Riding emitCall
	// keeps the generic evCall machinery working unchanged for the result.
	dynMethod *DynMethodSpec
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
	// carried seeds the loop-carried def slots (a pre-loop `def` the body
	// REBINDS, read on a later iteration or after the loop) with their
	// pre-loop values — lowered right after FOR_SETUP, before the first
	// FOR_NEXT, so a zero-iteration loop leaves each cell at its pre-loop
	// value. See NoteLoopCarried.
	carried []carriedInit
}

// carriedInit seeds one loop-carried def slot: the unit frame slot and the
// operand holding the pre-loop value stored into it before the loop runs.
type carriedInit struct {
	slot int
	init emitOperand
}

// emitStore is a recorded frame-local store — the loop-carried def REBIND
// (`def found true` inside a for arm, read on a later iteration or after
// the loop). src resolves in the recording scope; the lowering pushes a
// const/local/type source (or pops an event source off the sim top) into
// locals[slot] via OpStoreLocal. A store produces no values, so a rebind
// nets 0 exactly like the interpreter's def.
type emitStore struct {
	src  emitOperand
	slot int
	pos  SrcPos
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
	evStore
	evDynBind
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
	// poly, when non-nil, marks a runtime-dispatched MULTI-OVERLOAD user call
	// (OpCallUserPoly): the checker could not commit to one same-arity overload
	// (a gradual-Any arg reached two or more), so EVERY arm's body compiled to
	// its own unit and the VM re-runs MatchSignature at entry to pick the arm.
	// unit is -1 for a poly call (there is no single committed unit); ops/nout
	// keep the ordinary CALL_USER operand accounting, so every generic
	// evCallUser consumer (promotion, provenance, fragment residuals) works
	// unchanged. A poly call is never tail-marked (markTailCalls skips it).
	poly *emitUserPolySpec
}

// emitUserPolySpec is the recorded arm table of one poly user call — the
// word, the registry the check pass dispatched it against, and the parallel
// sigIdx/units/impls arm slices lowered verbatim into a UserPolyRef.
type emitUserPolySpec struct {
	word   string
	reg    *Registry
	sigIdx []int
	units  []int
	impls  []SigImpl
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
	seq   int
	kind  int
	call  emitCall
	br    *emitBranch
	loop  *emitLoop
	uc    emitUserCall
	fb    emitFallback
	trap  emitTrap
	store *emitStore
	dyn   *emitDynBind
}

// emitDynBind is one recorded `def` site (every value def records one,
// cheaply): the binding name, its interned name const, and the bound
// value's operand, resolved AT RECORD TIME while the unit context is
// live. The lowering emits a registry-visible OpBindDynScope for it ONLY
// when the name is in dynScopeNames (some OpLookupDynScope reads it);
// otherwise the event lowers to nothing.
type emitDynBind struct {
	name string
	// src carries a LOCAL operand when the bound value is a live frame slot
	// (a param re-bound by `def` — resolved via the unit's localByID at
	// record time, a pure read). An EVENT-produced value rides srcSeq as a
	// plain int, NOT an operand — so forEachOperand's reference counting and
	// the scopeFloor guard never see a phantom extra reference from a def
	// site whose bind the lowering skips (every def records one of these;
	// only dynamically-read names lower). Any other value rides val VERBATIM
	// and is interned UNPOOLED at lowering, only if the bind actually lowers
	// — record-time interning polluted the canonical const pool (a
	// reparented `def y:Pos 2` merging with the plain literal 2).
	src    emitOperand
	srcSeq int // producing event seq for an event-sourced value; -1 otherwise
	val    Value
	pos    SrcPos
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
	// applyFn, when set, is the leading fn value of the per-iteration apply as a
	// RE-PUSHABLE operand (a frame-local read — an `args.N` fold of a Function
	// param inside a fn unit) instead of an event on the sim: the body events
	// leave NOTHING, and lowerFragment pushes this before the applyArgs. A
	// break/continue raised inside the applied body escapes the island with the
	// registry FlowCtrl flag set; the VM translates it to the cross-frame
	// break/continue unwind (escapedFlow), exactly the interpreter's shared-tape
	// flow resolution.
	applyFn *emitOperand
	// applyFirst places the per-iteration apply BEFORE the body's other
	// statements — the source order of `for n [(args.0 1) def acc …]`, where a
	// continue raised inside the applied body must skip the statements after
	// it. Set when every recorded body event follows the fn read in source
	// order; the apply-last hoist (the original layout) requires the opposite,
	// and a mid-body apply declines. Only meaningful with applyFn.
	applyFirst bool
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

	// storedGradualDepth marks a DETACHED stamp compile (StampDetachedFn
	// sets it on the fork's private EmitState). While non-zero,
	// buildFnBodyReturnsFn generalises an Any arg into an Any param as a
	// GRADUAL carrier (ParamInputCarrier) so a nested callee's body keeps
	// poly-matching optimistically — the same modality the unit's own
	// params get from fnValueInputs. Detached-only by design: the fork
	// owns its program AND its Finalize, so a gradual-caused failure at
	// ANY stage (analysis, unit compile, lowering) is contained to one
	// declined stamp. Applied to the in-program stored-handler path the
	// same generalisation leaked whole-program refusals at Finalize (a
	// gradual nested unit recorded into the main program whose Stage-2
	// lowering later refused — the module-repl corpus rows).
	storedGradualDepth int

	// reg is the registry the check pass runs against — set once at the top of
	// Engine.Run while emit is active. It lets recorder-internal helpers reuse
	// the lang-layer closure compiler (compileClosureBody) for a fn VALUE a body
	// RETURNS (the factory pattern), which needs r but is reached from a finish
	// closure that has only es. Nil outside a compile pass.
	reg *Registry
	// progReg is the TOP-LEVEL program registry — captured at the FIRST
	// bindRegistry (the outermost check Run, before any module body / island
	// sub-engine re-binds reg). Unlike reg it never re-binds, so Finalize can
	// tell an ordinary top-level fn (rec.reg == progReg) from a foreign
	// sub-registry fn (a module preamble body, rec.reg != progReg) and stamp
	// CompiledFn.Reg only for the latter — see the Finalize stamp site.
	progReg *Registry
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
	// storedFnRefs collects the CompiledFnRefs minted while baking store-fn
	// handler consts (compileStoredFnUnit). Finalize back-stamps each ref's
	// Prog once the *Program exists and copies the slice onto Program. Nil
	// until the first store-fn handler is compiled.
	storedFnRefs []*CompiledFnRef
	// storedFnProbeReason holds the LAST stored-fn probe's refusal reason
	// (compileStoredFnUnit), for the -compile-report attribution
	// (stamp_report.go). Set only on a probe failure; the compile-time
	// bake ignores it.
	storedFnProbeReason string
	producedBy          map[string]producer // value ID → producing (event seq, result idx)
	// trailingApplies maps a Function VALUE's ID → the arg count of a paren-bounded
	// TRAILING fn-value apply (`(prev key comp)`), registered at the paren-collapse
	// boundary (registerTrailingApply) where the paren-group size is known. The body
	// reconciliation reads it back to lower the apply to OpCallDynTrailTop — the
	// flattened residual cannot otherwise recover the apply's arity.
	trailingApplies map[string]int
	// freshenConst marks const-pool indices holding a compound VALUE literal
	// that was materialised while recording a FN UNIT and whose ID is not an
	// enclosing binding's — i.e. a literal written in the body, which the
	// interpreter re-constructs per call. finalize's freshenFnUnitConsts pass
	// rewrites a single-push-site marked const to OpPushConstFresh (per-call
	// identity), keeps a multi-push-site one shared when nothing compound can
	// escape the fn, and refuses otherwise. See OpPushConstFresh (bytecode.go)
	// and design/MISCOMPILE-HUNT-FINDINGS.0.md §A.
	freshenConst map[int]bool
	// fnRiskFields maps a constructed INSTANCE's value ID → the field keys
	// holding genuinely-0-param fn values (noteFnRiskFields /
	// instanceFnFieldRisk — the carrier-receiver auto-dispatch hazard).
	fnRiskFields map[string]map[string]bool
	// memberFnReads holds the value IDs of get-family reads that surfaced a
	// FUNCTION-valued container member (readsFnMember). The read's static type is
	// dynamic(Any), so downstream code cannot see it is a fn; this side table
	// lets the stranded-member-fn guard (refuseStrandedMemberFn) recognise the
	// value and refuse a mid-expression auto-apply
	// (design/EDGE-SPEC-FINDINGS.0.md §2). Over-approximate + append-only like
	// fnRiskFields — a stale entry only ever over-refuses (sound).
	memberFnReads map[string]bool

	// shapedReads mirrors CheckState.MethodShapes annotations (by read-out
	// value ID) so the get-family read guards can exempt a read whose
	// landing the shaped-method model owns. See noteShapedRead.
	shapedReads map[string]bool
	// defReads maps a value ID to the BINDING NAME the check pass read it
	// from (stepWord's simple-value substitution). When such a value later
	// fails operand resolution inside a fn unit — no param/capture/local/
	// event/const home — the read was a DYNAMIC-SCOPE reference (a callee
	// reading the caller's frame binding), and resolveOperand lowers it to
	// a runtime OpLookupDynScope instead of refusing, gated on the check's
	// binder/call-graph reachability model (dynamicScopeReachable). See
	// NoteDefRead.
	defReads map[string]string
	// dynScopeNames collects every name resolveOperand lowered as an
	// OpLookupDynScope read. The Finalize pass installs an OpBindDynScope
	// twin in every unit (params and body-local defs) and at every
	// top-level def that binds one of these names, so the runtime lookup
	// finds the same binding the interpreter's def stack holds.
	dynScopeNames map[string]bool
	// unitNames parallels the open-unit stack with each unit's FN NAME, so
	// the dynamic-scope rescue knows which fn is READING (the FnNameStack
	// may already have advanced past a memoised body analysis by the time
	// the unit finish resolves its residual).
	unitNames []string
	// openUnitRecs parallels the open-unit stack with each unit's fnRecs
	// INDEX (units[0], the top level, has no rec and no entry here), so a
	// mid-body dispatch can ask what KIND of unit it is recording into —
	// specifically whether the innermost open unit is a CLOSURE body
	// (inClosureUnit), where the `args` frame projection must decline: the
	// closure's analysis args frame is the CallableSpec inputs, not the
	// enclosing fn's per-call args the interpreter reads at run time.
	openUnitRecs []int
	// dynEnv arms the program-wide DYNAMIC-ENVIRONMENT mode: a CompileDynBody
	// dispatch was recorded (tryRecordDynBody — a computed or context-word
	// `do` body lowered to a CALL_NATIVE whose handler re-runs the body at
	// run time). The body's sub-run resolves names against r.Defs and reads
	// r.Args, so the compiled program must mirror the interpreter's whole
	// environment: EVERY def emits its OpBindDynScope twin, EVERY named unit
	// param dyn-binds at frame entry, tail calls are disabled (binding
	// lifetime), and the VM brackets each CALL_USER frame with an args-stack
	// push (Program.DynEnv). Costs are paid only by programs that use
	// dynamic code bodies.
	dynEnv bool
	// frozenReads holds the module-scope binding names whose CONCRETE values
	// a fn/closure UNIT analysis read (NoteFrozenRead) — the value bakes into
	// the unit (a const, or splice-fired tokens) that re-runs on every call,
	// where the interpreter re-resolves the name per call. A LATER module-
	// scope rebind of such a name (NotifyNameRebound) therefore marks the
	// program uncompilable: `def x 1  def f fn [… [x add y]]  f 0  def x 2
	// f 0` compiled to 1 1 against the interpreter's documented 1 2 (module-
	// level dynamic reads, lang/go/CLAUDE.md "Closures and Capture") before
	// this freeze discipline. Nil until the first unit-baked read.
	frozenReads map[string]bool

	// eventInfo holds the per-event compile flags, keyed by event seq. It
	// consolidates the former parallel zeroOutSeq/typeOut/valueDefs/genericSeq
	// maps: each is a "property of event N", read via a producer's seq
	// (producedBy[id].seq), so they stay seq-keyed rather than moving onto the
	// value-copied emitEvent struct (a flag set after append wouldn't reach the
	// frame/fragment copies).
	eventInfo map[int]eventFlags
	consts    []Value
	constIdx  map[string]int // CanonValue → Consts index
	// constIDIdx pools COMPOUND consts by value ID: the same materialised
	// List/Map value (same ID, same payload pointer — already identity-
	// aliased) reuses one Consts slot, so freshenFnUnitConsts' push-site
	// counting sees reads of one binding as pushes of ONE const. Distinct
	// source literals keep distinct IDs, so the never-CanonValue-dedup rule
	// (gotcha #13) is untouched.
	constIDIdx map[string]int
	types      []TypeRef
	typeIdx    map[string]int   // type ID → Types index
	fallbacks  []FallbackSpan   // Stage 5 interpreter islands
	origByID   map[string]Value // stripped literal ID → original value
	// trapAt is the seq of a recorded TOP-LEVEL terminal trap (a check-mode-
	// suppressed runtime error compiled as OpTrap), or 0 for none. When set,
	// Finalize ends the program at the trap. seqs start at 1, so 0 is a safe
	// "none" sentinel.
	trapAt int
	// loopCarried is the stack of OPEN armed-loop carried-def scopes (one per
	// nested AnalyseLoopBody running with loop capture). Each maps a rebound
	// pre-loop def NAME to the unit frame slot carrying it across iterations;
	// RecordDefRebind consults the stack at `def` sites, innermost-first.
	loopCarried []*loopCarriedScope
	// pendingCarried is the just-closed loop analysis's carried-slot init
	// list (EndLoopCarried), consumed by the RecordLoop that follows.
	pendingCarried []carriedInit
}

// loopCarriedScope is one armed loop's carried-def registrations: the unit
// depth the loop records in (defs inside a nested fn compilation must not
// match), the name→slot map, and the slot inits queued for RecordLoop.
type loopCarriedScope struct {
	unitDepth int
	slots     map[string]int
	inits     []carriedInit
}

// emitUnit scopes local-slot numbering to one code unit (the
// top-level program, or one compiled fn body — locals are
// frame-relative at run time).
type emitUnit struct {
	localByID map[string]int
	numLocals int
	// reg is the compiled fn's OWNING registry (a module sub-registry for a
	// module-preamble fn; the main registry otherwise). Finalize stamps it
	// on the CompiledFn when it differs from the check registry, and the VM
	// then dispatches the unit's natives against it — the interpreter's
	// CallAQL runs a foreign fn's body IN its own registry, so handlers
	// with registry-visible effects (Net.listen forking per-connection
	// registries) see module scope on both engines.
	reg *Registry
	// capID marks the value IDs that are this unit's CAPTURES (enclosing-scope
	// values bound into trailing slots). A capture's value may ALSO carry a
	// producedBy entry from the ENCLOSING unit (a computed `def a (h add 1)`
	// snapshotted into the closure) — that event lives in the parent and is
	// unreachable here, so the body must resolve the reference to its own
	// capture SLOT, not the parent event. resolveOperand consults this to
	// override its usual events-first precedence for captured IDs. Pairs with
	// the parent-side promotion of the captured computed def to a frame local
	// (forEachOperand / promoteOperand closureCaps handling), which makes the
	// closureCaps operand a re-pushable LOCAL so the captured VALUE is correct.
	capID map[string]bool
	// enclosingIDs snapshots, at unit open, the value IDs of every compound
	// (List/Map) binding visible in the DefTable — i.e. values an ENCLOSING
	// scope already constructed. A compound const whose ID is in this set was
	// read from a binding (one shared instance per binding, interpreter
	// semantics), so it must NOT be freshened per call; a compound const whose
	// ID is absent was minted by evaluating a literal written in THIS body, so
	// the interpreter constructs it fresh per call and the VM must match
	// (OpPushConstFresh; miscompile mechanism A). Body-local defs bind their
	// IDs only DURING body analysis, after this snapshot — so they correctly
	// classify as body-fresh.
	enclosingIDs map[string]bool
	// pendingApply lists the value IDs of Function/FnDef-typed CARRIERS this
	// unit's body dispatched through the `apply` word (a param/captured
	// comparator: `v comp/r apply`). The check engine cannot re-step a carrier
	// the way it re-steps a concrete fn, so the dispatch is elided and the
	// carrier flows to the body residual; StartFnCompile's finish must either
	// lower the ONE pending apply as the whole-residual OpCallDynTrailTop
	// (fn on top, args below — exactly the interpreter's applyHandler re-step
	// against the preceding stack) or refuse, so an unconsumed pending apply
	// can never silently compile the fn+args as unapplied data (Stage M2a,
	// design/STAGE3-INLINING-DESIGN-ROUND.0.md).
	pendingApply []string
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
	name          string
	nParams       int
	nUnnamed      int // unnamed (stack-flowing) params — the RET trim allowance
	caps          []CapturedBinding
	generic       bool
	returns       []*Type  // declared return types — enforced at the VM's RET
	paramTypes    []*Type  // declared PARAM types — enforced at the VM's CALL_USER entry
	paramPatterns []*Value // per-param structural/value patterns — also enforced at CALL_USER
	// decl is the return-contract declaration site (FnSig.Decl), stamped
	// on CompiledFn.Decl so a compiled RET return error labels the
	// declaration as a secondary span exactly as the interpreter does.
	// Zero for closures / anonymous units (no meaningful declaration).
	decl   DeclSite
	locals []string // slot→name table (params then captures)
	// reg is the fn's owning registry; stamped on CompiledFn.Reg when it
	// differs from the check registry (see emitUnit.reg).
	reg  *Registry
	frag *EmitFragment
	// storedRefUnit marks a unit compiled for a STORED-REF carrier
	// (compileStoredFnUnit's "storedfn" / compileStoredBody's "spawnbody"):
	// it is reachable at run time only through its CompiledFnRef, whose
	// module-dep rebinds are handled PRECISELY by per-ref poisoning
	// (NotifyNameRebound → poisoned → InvokeCallback falls to CallAQL).
	// NoteFrozenRead therefore skips reads attributed to it — the whole-
	// program frozen-read hammer is for ordinary CALL_USER units, which
	// have no per-unit fallback.
	storedRefUnit bool
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
	outOps []emitOperand
	// dynTrailArity > 0 marks a body whose ENTIRE residual is a paren-bounded
	// TRAILING fn-value apply (`(prev key comp)`): outOps are [args…, fn] (fn on
	// top) and the unit lowering emits OpCallDynTrailTop(dynTrailArity) after seating
	// them, collapsing [args, fn] to the one applied value before the RET. The arity
	// is captured at the paren-collapse boundary (registerTrailingApply) where the
	// group size is known; the flattened residual cannot recover it.
	dynTrailArity int
	// dynTrailApply marks a dynTrail that came through the `apply` WORD (the
	// unit's pendingApply, Stage M2a) rather than a paren boundary: the unit
	// lowering emits OpCallDynApplyTop — applyHandler's unquote-then-apply —
	// instead of OpCallDynTrailTop (which leaves a Quoted fn as data, the
	// paren semantics).
	dynTrailApply bool
	// dynFrameW > 0 marks a body whose residual carries an UNAPPLIED runtime fn
	// value beyond the frame-bottom re-push window — the shape the interpreter
	// resolves by execFnDefLiteral's runtime rule against the LIVE frame. The
	// unit lowering seats the FULL residual (the unnamed-param re-push prefix
	// included) and emits OpCallDynFrame(dynFrameW) before the RET: the top
	// dynFrameW entries are the token region the interpreter's pointer stepped,
	// replayed with the prefix resolved (see the opcode doc). Sets
	// CompiledFn.RetReplay so the RET applies the CallAQL-path trim discipline.
	dynFrameW int
	// retPrefix seats the unnamed-param frame-bottom re-pushes at UNIT START
	// (before the body events), for a body whose residual holds a variadic
	// apply-loop value region under an inert tail — the values must sit BELOW
	// the loop's runtime region, which only exists once the loop has run, so
	// they cannot be re-pushed by the RET seating. Paired with retReplay.
	retPrefix []emitOperand
	// retReplay marks a body whose RET takes the replay trim discipline
	// (CompiledFn.RetReplay): a whole-frame dynamic apply (dynFrameW) or a
	// variadic apply-loop residual (retPrefix) leaves a runtime-variable
	// count the static contract cannot pin.
	retReplay bool
	numLoc    int
	pos       SrcPos
	finished  bool
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

// residualForceOrder returns the fn-unit residual's promotion set when it is
// OUT OF ORDER — an event result above a non-event bottom — so the finish can
// force every residual event to a frame local (the per-unit mirror of
// Finalize's program-residual forceOrder) and the RET reconciliation re-pushes
// the whole residual in exact order. Returns nil for an in-order residual and
// for any residual carrying a fn value or dynamic value (the auto-apply
// boundary's territory: its stack layout is the apply's contract).
func residualForceOrder(ops []emitOperand, vals []Value) map[int]bool {
	for _, v := range vals {
		if v.Dynamic || isFnValueResidual(v) {
			return nil
		}
	}
	seenNonEvent, outOfOrder := false, false
	for _, op := range ops {
		if op.kind == opEvent && op.resIdx == 0 {
			if seenNonEvent {
				outOfOrder = true
			}
		} else {
			seenNonEvent = true
		}
	}
	if !outOfOrder {
		return nil
	}
	forceOrder := map[int]bool{}
	for _, op := range ops {
		if op.kind == opEvent && op.resIdx == 0 {
			forceOrder[op.idx] = true
		}
	}
	return forceOrder
}

// forkForProbe returns a throwaway recording state for compiling a closure body
// speculatively (recordClosureDispatch's probe), seeded so a RECURSIVE call
// inside the body resolves to the enclosing in-progress unit. The closure body's
// emission (events / code / producedBy / consts) is FRESH — a refusal discards it
// without touching the real program. But the fn-unit resolution tables
// (fnUnits / fnRecs / units) are COPIED from the real state: a self-recursive call
// (`… msd-go` inside an `each` body) is then a fnUnits HIT against the enclosing
// fn's reserved unit — the same recursion guard the in-state non-closure path
// relies on — instead of a MISS that re-compiles the enclosing fn in the
// throwaway, where it re-hits its own closure and never registers its residual
// ("body result of unknown provenance"). The slices get fresh backing arrays so
// the probe's newly-appended units never write into the real state; the shared
// *fnUnitRec / *emitUnit pointers are only READ on the hit path (StartFnCompile
// returns finish==nil, so no re-analysis mutates them).
func (es *EmitState) forkForProbe() *EmitState {
	p := NewEmitState()
	p.reg = es.reg
	p.fnUnits = make(map[string]int, len(es.fnUnits))
	for k, v := range es.fnUnits {
		p.fnUnits[k] = v
	}
	p.fnRecs = make([]*fnUnitRec, len(es.fnRecs))
	copy(p.fnRecs, es.fnRecs)
	p.units = make([]*emitUnit, len(es.units))
	copy(p.units, es.units)
	p.openUnitRecs = make([]int, len(es.openUnitRecs))
	copy(p.openUnitRecs, es.openUnitRecs)
	// The probe inherits the gradual-nesting mode: probe and real must
	// compile under the SAME modality or the probe's verdict is about a
	// different unit (see compileStoredFnUnit / tryReturnedClosure).
	p.storedGradualDepth = es.storedGradualDepth
	return p
}

// inClosureUnit reports whether the innermost OPEN fn unit is a CLOSURE body
// (a higher-order word's each/do$body unit — compileClosureBody stamps
// rec.closure before running the body analysis, so the flag is visible to
// every dispatch the body records). Body dispatches with frame-context
// semantics consult this: the `args` projection (specialWordResults) reads
// the analysis args frame, which for a closure holds the CallableSpec inputs
// — NOT the enclosing fn's per-call args the interpreter reads when the body
// runs through InvokeBody — so the projection must decline and let the
// dispatch fall to RecordCall's context-dependent-word refusal.
func (es *EmitState) inClosureUnit() bool {
	if es == nil || len(es.openUnitRecs) == 0 {
		return false
	}
	rec := es.openUnitRecs[len(es.openUnitRecs)-1]
	if rec < 0 || rec >= len(es.fnRecs) {
		return false
	}
	return es.fnRecs[rec].closure
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

// armed reports that a REAL recording state exists (a compile pass installed
// one), whether or not it is currently live — the EmitRecorder twin of the
// historical `Check.Emit != nil` probe (an EmitState may be armed yet
// suspended or already uncompilable; the inactive no-op recorder is never
// armed).
func (es *EmitState) Armed() bool { return es != nil }

// bindRegistry installs the registry back-pointer used by returned-closure
// compilation (tryReturnedClosure) and the flex-hook sig-identity proof.
func (es *EmitState) bindRegistry(r *Registry) {
	if es == nil {
		return
	}
	// The FIRST bind names the program registry: the outermost check Run binds
	// the top-level registry before any module-body / island sub-engine run
	// re-binds reg to a foreign sub-registry. Captured once, never re-bound.
	if es.progReg == nil {
		es.progReg = r
	}
	es.reg = r
}

// topFrameOnly reports whether recording sits at the top event frame (no
// open branch/loop/fn capture) — the const-fold gate for computed container
// elements. A missing recorder counts as top-frame (nothing is being
// captured), matching the historical `es == nil || len(es.frames) == 1`.
func (es *EmitState) topFrameOnly() bool {
	return es == nil || len(es.frames) == 1
}

// suspendedNow reports whether an ARMED recorder is currently suspended —
// the "analysis probe is reading an enclosing dispatch's result" state the
// unmatched-dispatch recovery consults before latching a refusal.
func (es *EmitState) suspendedNow() bool {
	return es != nil && es.suspended > 0
}

// Sites returns the per-site-class dispatch tally (live map; read-only for
// callers). Nil when recording never started.
func (es *EmitState) Sites() map[string]int {
	if es == nil {
		return nil
	}
	return es.SiteCounts
}

// zeroOutProduced reports whether the value id was produced by an event
// whose outputs were zeroed (the 0-output `if` phantom None) — the residual
// strip probe (stripZeroOutResiduals).
func (es *EmitState) zeroOutProduced(id string) bool {
	if es == nil {
		return false
	}
	pr, ok := es.producedBy[id]
	return ok && es.eventInfo[pr.seq].zeroOut
}

// alreadyProduced reports whether the value id already has a recorded
// producer event — the double-record guard (a structured ReturnsFn hook may
// have recorded the dispatch before the generic path sees it).
func (es *EmitState) alreadyProduced(id string) bool {
	if es == nil {
		return false
	}
	_, ok := es.producedBy[id]
	return ok
}

// unitVariadic reports whether the fn unit's recorded body is
// variadic-returning — the user-poly gate (a poly call site bakes a fixed
// nout, so no arm may be variadic).
func (es *EmitState) unitVariadic(unit int) bool {
	if es == nil || unit < 0 || unit >= len(es.fnRecs) {
		return false
	}
	return es.fnRecs[unit].variadic
}

// newIsolatedEmit returns the FRESH EmitState IsolateEmit swaps in for a
// hermetic throwaway evaluation, inheriting only the registry back-pointer
// from the saved recorder. Construction knowledge stays in this file: the
// checker side (CheckState.IsolateEmit) sees only EmitRecorder values.
func newIsolatedEmit(saved EmitRecorder) EmitRecorder {
	fresh := NewEmitState()
	if prev, ok := saved.(*EmitState); ok && prev != nil {
		fresh.reg = prev.reg
	}
	return fresh
}

// RegisterTrailingApply records that the Function VALUE `fnID` is the trailing
// fn-value of a paren-bounded apply over `arity` preceding args (`(prev key comp)`),
// captured at the paren-collapse boundary where the group size is known. The body
// reconciliation reads it back (TrailingApplyArity) to lower the apply to
// OpCallDynTrailTop — the flattened residual cannot otherwise recover the arity.
func (es *EmitState) RegisterTrailingApply(fnID string, arity int) {
	if es == nil || fnID == "" || arity < 1 {
		return
	}
	if es.trailingApplies == nil {
		es.trailingApplies = map[string]int{}
	}
	es.trailingApplies[fnID] = arity
}

// TrailingApplyArity returns the registered arg count for a paren-bounded trailing
// apply whose fn value is `fnID`, or 0 if none registered.
func (es *EmitState) TrailingApplyArity(fnID string) int {
	if es == nil || es.trailingApplies == nil {
		return 0
	}
	return es.trailingApplies[fnID]
}

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

// peekCaptureArm reports whether the next RunCarrierBodyWithDefs will
// record its body into a fragment (branch/loop lowering armed and the
// recorder live) WITHOUT clearing the one-shot flag — the same condition
// consumeCaptureArm settles. RunCarrierBodyWithDefs consults it to mark
// the branch/loop sub-engine element-eval-recordable, so a residual
// computed container (`{a: x}` / `[x y]` returned from a branch arm) has
// its OpMakeMap / OpMakeList assembly recorded rather than left an
// unresolvable residual — the body runs in the LIVE frame here (unlike
// the end-of-run residual eval), so recording is sound.
func (es *EmitState) peekCaptureArm() bool {
	return es != nil && es.captureArm && es.active()
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
	n := len(es.frames) - 1
	// Dead code after a divergent terminal, IN A FRAGMENT: a diverging call
	// (raise, or a static-zero div/mod — CompileValueDiverges) never returns, so
	// any event recorded after it in the same CLOSURE/BRANCH fragment is
	// unreachable. Emitting it builds a malformed frame — an op wired to the
	// divergent call's absent result crashes the VM (`do [(raise "x") add 1]`,
	// `do [(0 mod 0) mod 0]` both did). DROP it: the fragment already diverges
	// (fragDiverges sees the divergent call as its terminal, so the closure body
	// compiles with no RET and the catching word wraps the raised error), exactly
	// as the interpreter stops at the raise. The dropped event still consumes a
	// seq so a caller registering its (dead) output writes a harmless slot Finalize
	// never linearises. Gated to n>0 (a pushed fragment frame): at the TOP LEVEL
	// the divergence is the program terminator — Finalize truncates the residual
	// after it (`0 and (20 div 0)` raises and aborts, matching the interpreter),
	// so top-level events past it stay put.
	if n > 0 && len(es.frames[n]) > 0 {
		if prev := es.frames[n][len(es.frames[n])-1]; prev.kind == evCall && prev.call.diverges {
			es.seq++
			return es.seq
		}
	}
	es.seq++
	ev.seq = es.seq
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
	// An identity-less value (a runtime mint under the mode-gated ID
	// elision — value.go checkPassDepth) must NEVER key the provenance
	// map: a "" insert would make every later ""-ID lookup a false hit
	// (all identity-less values would alias one producer) — an active
	// miscompile. Skipping keeps "" lookups guaranteed misses, which
	// degrade to dynScopeRescue / refusal.
	if out.ID == "" {
		return
	}
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
	// A CAPTURE of the CURRENT unit overrides events-first: the captured value
	// may carry a producedBy entry from the ENCLOSING unit (a computed
	// `def a (h add 1)` snapshotted into a closure), but that event lives in the
	// parent frame and is unreachable from inside the body — the reference must
	// resolve to this unit's own capture SLOT. Without this the each/scan/…$body
	// reference resolved to the parent event and refused "branch reads enclosing
	// computation". A param capture has no producedBy entry, so this only changes
	// the computed-capture case. (The captured VALUE is carried correctly by the
	// parent-side promotion of the computed def — see forEachOperand /
	// promoteOperand closureCaps handling.)
	if cur := es.units[len(es.units)-1]; cur != nil && cur.capID[v.ID] {
		if slot, ok := cur.localByID[v.ID]; ok {
			return localOperand(slot), true
		}
	}
	// Events first, locals second: a join can REUSE a local's value ID
	// for its result (JoinCarriers keeps the then-side ID when types
	// agree), and the event is then the value's stack-discipline truth
	// — the branch pushed it. A plain param/iterator reference has no
	// producing event and resolves to its local slot.
	if pr, ok := es.producedBy[v.ID]; ok {
		// A type operand whose ID matches a producing event whose own
		// output was NOT a type is an ID collision (a `make` result
		// inheriting the type literal's ID): resolve it as its own type
		// operand / const below, not the unrelated event. A DYNAMIC
		// carrier is exempt: it is the checker's gradual stand-in for a
		// runtime VALUE, never a type body the program manipulates as
		// data — narrowing-through-use rebinds a dynamic Any def to a
		// typed-list/typed-map bound ([:Any]) that trips IsTypeBody, but
		// its ID-matched event genuinely produced it (the identity-
		// preserving rebind in narrowDynamicUses).
		if !IsTypeBody(v) || v.Dynamic || es.eventInfo[pr.seq].typeOut {
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
	if !ok {
		return es.dynScopeRescue(v)
	}
	// At MODULE scope a NoEvalArgs body that is inert except for InterpStrings
	// (interpBodyInert) bakes as code-as-data and is re-interpreted against the
	// registry — sound where there is no enclosing VM frame to shadow (see
	// noEvalBodiesInertScoped). isInertConst stays strict so this admission never
	// reaches a compiled fn frame: there the body refuses at the Stage-2 gate
	// before resolveOperand, so the operand never gets here.
	if !isInertConst(lit) && !(len(es.units) == 1 && interpBodyInert(lit)) {
		return es.dynScopeRescue(v)
	}
	// An ACTIVE-token MAP (`{line: src}` — a word / paren / interpolation
	// member) must not bake inside a compiled fn frame: autoEvalMap runs on
	// EVERY map argument at dispatch (NoEvalArgs does not suppress it), so
	// the interpreter re-evaluates the member per dispatch against the LIVE
	// FRAME (src is a param), which the VM holds as a frame local the baked
	// const can never see. Lists stay bakeable — a NoEvalArgs code body
	// (`each [dup mul]`, `each $.1`) is legitimately code-as-data, guarded
	// by the position-aware body gates (execBodyRefsNames /
	// noEvalBodiesInertScoped). Module scope keeps the map bake too —
	// resolution there is registry-identical.
	if len(es.units) > 1 {
		if _, isMap := lit.Data.(MapPayload); isMap && bearsActiveTokens(lit) {
			return es.dynScopeRescue(v)
		}
		// A compound with NO identity (a runtime mint under the mode-gated
		// ID elision) cannot be placed inside a fn unit: the per-call
		// identity machinery below (freshen / share / embed detection) is
		// keyed on value IDs, and an identity-less compound would slip past
		// the enclosing-binding probe and be wrongly freshened (breaking
		// member identity with the live runtime instance). Rescue —
		// dynamic-scope read or refusal — never freshen.
		if v.ID == "" && freshenableConst(lit) {
			return es.dynScopeRescue(v)
		}
	}
	idx := es.intern(lit)
	// A compound VALUE literal materialised inside a fn unit gets per-call
	// identity treatment (OpPushConstFresh / share / refuse — see
	// freshenFnUnitConsts) unless its ID was already a binding's value at
	// unit open, i.e. an enclosing scope constructed it once and the body
	// merely reads it. Compounds are never pooled (intern), so idx belongs
	// to exactly this materialise context.
	if len(es.units) > 1 && freshenableConst(lit) &&
		!es.units[len(es.units)-1].enclosingIDs[v.ID] {
		// A body literal EMBEDDING an enclosing binding's container cannot
		// be freshened OR shared: the interpreter constructs the OUTER
		// literal fresh per call while the binding-read MEMBER keeps its
		// shared instance (`def c [9] def mk fn [[] [List] [[c]]]` —
		// `((mk) get 0) eq c` stays true, `(mk) eq (mk)` stays false).
		// A deep-clone freshen breaks the member identity; a shared const
		// breaks the outer's. Until a selective (spine-only) freshen
		// exists, refuse — the sound fallback (PR #225 P1).
		if embedsEnclosingCompound(lit, es.units[len(es.units)-1].enclosingIDs) {
			es.MarkUncompilable("fn body literal embeds an enclosing binding's container (per-call spine identity over a shared member)")
			return emitOperand{}, false
		}
		if es.freshenConst == nil {
			es.freshenConst = map[int]bool{}
		}
		es.freshenConst[idx] = true
	}
	return constOperand(idx), true
}

// embedsEnclosingCompound reports whether a compound literal (recursively)
// contains a compound member whose identity is an ENCLOSING binding's value
// — the shape whose interpreter semantics mix a per-call-fresh spine with a
// shared member, which neither OpPushConstFresh (deep clone) nor a shared
// pooled const can model. See the refusal at the resolveOperand marking
// site (PR #225 P1).
func embedsEnclosingCompound(v Value, enclosing map[string]bool) bool {
	switch d := v.Data.(type) {
	case ListPayload:
		for _, e := range d.Elems {
			if !freshenableConst(e) {
				continue
			}
			if enclosing[e.ID] || embedsEnclosingCompound(e, enclosing) {
				return true
			}
		}
	case MapPayload:
		if d.M == nil {
			return false
		}
		for _, k := range d.M.Keys() {
			mv, _ := d.M.Get(k)
			if !freshenableConst(mv) {
				continue
			}
			if enclosing[mv.ID] || embedsEnclosingCompound(mv, enclosing) {
				return true
			}
		}
	}
	return false
}

// freshenFnUnitConsts gives fn-unit compound body literals per-call identity
// (miscompile mechanism A — see OpPushConstFresh, bytecode.go). For each
// marked const (freshenConst) pushed by this unit's finished code:
//   - one push site → rewritten IN PLACE to OpPushConstFresh (no instruction
//     insertion, so jump targets are untouched): the interpreter constructs
//     the literal fresh on every call — and on every loop iteration, which a
//     single re-executed site models exactly;
//   - several push sites → the pushes stand for reads of ONE per-call
//     binding, so they must share within a call; the shared pooled const is
//     indistinguishable from that unless an instance escapes the fn. When
//     every declared return conforms to Scalar nothing compound can escape,
//     and the shared const is exact parity — keep it. Otherwise refuse:
//     cross-call identity of an escaping multi-read literal needs a per-call
//     local seat (a deferred lowering), and refusal is the sound fallback.
func freshenFnUnitConsts(cf *CompiledFn, es *EmitState, rec *fnUnitRec) string {
	if len(es.freshenConst) == 0 {
		return ""
	}
	counts := map[int]int{}
	last := map[int]int{}
	for pc, in := range cf.Code {
		if in.Op == OpPushConst && es.freshenConst[int(in.Arg)] {
			counts[int(in.Arg)]++
			last[int(in.Arg)] = pc
		}
	}
	for idx, n := range counts {
		if n == 1 {
			cf.Code[last[idx]].Op = OpPushConstFresh
			continue
		}
		if !returnsAllScalar(rec.returns) {
			return "fn " + rec.name + ": compound body literal read at multiple sites may escape (per-call identity needs a local seat)"
		}
	}
	return ""
}

// returnsAllScalar reports whether every declared return conforms to Scalar
// — the cheap sufficient condition that no container instance can escape a
// fn, making a shared multi-read compound const exact within-call parity
// (freshenFnUnitConsts). Unchecked or absent returns assume escape.
func returnsAllScalar(returns []*Type) bool {
	if len(returns) == 0 {
		return false
	}
	for _, t := range returns {
		if t == nil || !t.ConformsTo(TScalar) {
			return false
		}
	}
	return true
}

// snapshotCompoundBindingIDs collects the value IDs of every compound
// (List/Map) binding currently visible in the DefTable — the values a fn
// unit opening NOW would read from ENCLOSING scope. See
// emitUnit.enclosingIDs for the semantics.
func (es *EmitState) snapshotCompoundBindingIDs() map[string]bool {
	ids := map[string]bool{}
	if es.reg == nil || es.reg.Defs == nil {
		return ids
	}
	for _, name := range es.reg.Defs.Names() {
		for _, bv := range es.reg.Defs.Stack(name) {
			if freshenableConst(bv) && bv.ID != "" {
				ids[bv.ID] = true
			}
		}
	}
	return ids
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
// fnValueInputs builds the per-param input carriers and name table for compiling
// a fn value's body — a returned lambda (tryReturnedClosure) or a stored handler
// (compileStoredFnUnit). Each declared param type becomes a carrier (Any when the
// param carries only a pattern, hence no bare type), and a named param carries
// its name so AnalyseFnBody binds the body's references to that input.
func fnValueInputs(params []FnParam) (inputs []Value, names []string) {
	inputs = make([]Value, len(params))
	names = make([]string, len(params))
	for i, p := range params {
		t := p.Type
		if t == nil {
			t = TAny
		}
		// ParamInputCarrier (not a strict NewCarrier): an explicitly-`Any`
		// handler param binds a GRADUAL carrier, so a body word over it
		// poly-matches at runtime instead of failing no_signature against the
		// strict Any top — the same treatment the user-fn compile path gives
		// (user_poly.go). This lets a `serve-raw` handler `[sock:Any]` compile
		// its socket-word dispatch to the VM (the connection value IS a Socket
		// at runtime); a concrete-typed param stays strict.
		inputs[i] = ParamInputCarrier(t)
		names[i] = p.Name
	}
	return inputs, names
}

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
	if own != 1 || !hasOwn || len(lam.body()) == 0 || bodyToksHaveSentinel(lam.body()) {
		return emitOperand{}, false
	}
	inputs, paramNames := fnValueInputs(lam.Params)
	r := es.reg
	// PROBE in a throwaway emit state (mirrors recordClosureDispatch), so a body
	// that refuses leaves THIS program untouched and the value stays unresolved.
	probe := NewEmitState()
	probe.reg = r
	// The probe inherits the gradual-nesting mode (a detached stamp arms it on
	// the real state): probe and real must compile under the SAME modality or
	// the probe's verdict is about a different unit — without this, an inner
	// lambda inside a stored service handler (mini-s3's bucket-list filter,
	// whose body runs `convert … e.value` over an Any param) probed STRICT,
	// refused "unmatched dispatch recovered", and surfaced as the misleading
	// "function-valued operand at filter (Stage 3)".
	probe.storedGradualDepth = es.storedGradualDepth
	r.Check.Emit = probe
	// bodyOut 1: a fn VALUE body keeps the single declared return (it is not a
	// 0-output side-effect body like a test case).
	_, probeOK := compileClosureBody(r, "fnval", 1, false, false, lam.body(), inputs, paramNames, fd.Captured, ClosureInValue, pos)
	r.Check.Emit = es
	if !probeOK {
		return emitOperand{}, false
	}
	// REAL: compile into this program (deterministic success after a clean probe).
	unit, realOK := compileClosureBody(r, "fnval", 1, false, false, lam.body(), inputs, paramNames, fd.Captured, ClosureInValue, pos)
	if !realOK || unit < 0 { //covergate:allow compiler/VM defensive arm; unreachable without a bytecode-level fault (§compiler)
		return emitOperand{}, false
	}
	return emitOperand{kind: opClosure, closureUnit: unit, closureCaps: capOps}, true
}

// compileStoredFnUnit compiles a CAPTURE-FREE store-fn handler body (the fn a
// CompileStoresFn word stashes for later invocation — a serve-raw connection
// handler) to its own fn unit, so the native word can run it on the VM via
// RunUnit instead of CallAQL. Returns the unit index and true, or (0, false)
// when the body refuses (a refusal-class word, a flow sentinel, no single own
// sig) — the caller then bakes only the plain const and the handler falls back
// to the interpreter, per-body and sound. Mirrors tryReturnedClosure's
// probe-then-real compile, but bodyOut 0 (count-agnostic): a stored handler is
// invoked for effect and its residual, like CallAQL's, is the caller's to use
// or discard.
func (es *EmitState) compileStoredFnUnit(fd FnDefInfo, pos SrcPos) (int, bool) {
	if es == nil || es.reg == nil {
		return 0, false
	}
	lam, ok := storedFnUnitEligible(fd)
	if !ok {
		return 0, false
	}
	inputs, paramNames := fnValueInputs(lam.Params)
	r := es.reg
	// PROBE in a throwaway state so a refusing body leaves THIS program
	// untouched (mirrors tryReturnedClosure / recordClosureDispatch).
	probe := NewEmitState()
	probe.reg = r
	// The probe inherits the gradual-nesting mode (a detached stamp arms it
	// on the real state) — probe and real must compile under the SAME
	// modality or the probe's verdict is about a different unit.
	probe.storedGradualDepth = es.storedGradualDepth
	r.Check.Emit = probe
	_, probeOK := compileClosureBody(r, "storedfn", 0, true, false, lam.body(), inputs, paramNames, nil, ClosureInValue, pos)
	r.Check.Emit = es
	if !probeOK {
		// Surface the probe's refusal for the -compile-report attribution
		// (stamp_report.go); the compile-time bake ignores the field.
		es.storedFnProbeReason = probe.Reason
		return 0, false
	}
	unit, realOK := compileClosureBody(r, "storedfn", 0, true, false, lam.body(), inputs, paramNames, nil, ClosureInValue, pos)
	if !realOK || unit < 0 { //covergate:allow compiler/VM defensive arm; unreachable without a bytecode-level fault (§compiler)
		return 0, false
	}
	return unit, true
}

// storedFnUnitEligible reports whether fd is the SHAPE a stored-fn unit
// compile accepts — exactly one own (non-fallback) signature carrying a
// non-empty body with no flow-control sentinel — returning that signature.
// Shared by the compile-time store-fn bake (compileStoredFnUnit) and the
// runtime detached stamp (StampDetachedFn) so the two gates cannot drift.
// Capture-freedom is the CALLER's gate (recordCallOperands / StampFnValue):
// it is a property of the storing context, not of the unit shape.
func storedFnUnitEligible(fd FnDefInfo) (*Signature, bool) {
	own := 0
	for i := range fd.Signatures {
		if !fd.Signatures[i].Fallback {
			own++
		}
	}
	lam, hasOwn := fd.FirstOwnSig()
	if own != 1 || !hasOwn || len(lam.body()) == 0 || bodyToksHaveSentinel(lam.body()) {
		return nil, false
	}
	return lam, true
}

// compileStoredBody compiles a NoEvalArgs CODE-BODY list (spawn's process body) to
// a 0-param unit and returns a synthetic fn-value carrier holding both the raw
// Body tokens (interpreter fallback) and a CompiledFnRef (registered for the
// Finalize back-stamp). The storing word runs the carrier's unit via RunUnit on
// its own registry. Returns ok=false when the body is empty, carries a flow
// sentinel, or refuses to compile — the caller then bakes the plain const list
// and the word runs it on the interpreter, unchanged. Mirrors compileStoredFnUnit
// but for a bare token body rather than a fn value's sig body.
func (es *EmitState) compileStoredBody(bodyList Value) (Value, bool) {
	if es == nil || es.reg == nil {
		return Value{}, false
	}
	lst, err := AsList(bodyList)
	if err != nil || lst.IsNil() {
		return Value{}, false
	}
	tokens := lst.Slice()
	if len(tokens) == 0 || bodyToksHaveSentinel(tokens) {
		return Value{}, false
	}
	r := es.reg
	probe := NewEmitState()
	probe.reg = r
	// Same-modality probe (see tryReturnedClosure / compileStoredFnUnit).
	probe.storedGradualDepth = es.storedGradualDepth
	r.Check.Emit = probe
	_, probeOK := compileClosureBody(r, "spawnbody", 0, true, false, tokens, nil, nil, nil, ClosureInValue, bodyList.Pos())
	r.Check.Emit = es
	if !probeOK {
		return Value{}, false
	}
	unit, realOK := compileClosureBody(r, "spawnbody", 0, true, false, tokens, nil, nil, nil, ClosureInValue, bodyList.Pos())
	if !realOK || unit < 0 { //covergate:allow compiler/VM defensive arm; unreachable without a bytecode-level fault (§compiler)
		return Value{}, false
	}
	ref := &CompiledFnRef{Unit: unit, depNames: es.storedHandlerDeps(tokens)}
	es.storedFnRefs = append(es.storedFnRefs, ref)
	carrier := Value{Parent: TFunction, Data: FnDefInfo{
		Signatures: []Signature{{Impl: &AQLImpl{Body: tokens, Compiled: ref}}},
	}}
	return carrier, true
}

// stampCompiledRef records ref on the fn value's first own (non-fallback) AQL
// body sig, so the runtime FnDefInfo the store-fn word receives carries the VM
// edge alongside its raw Body. Mutates the shared *AQLImpl pointer, so the
// interned const reflects it. Returns false when no own AQL body sig exists (a
// Go-backed or fallback-only fn value — never a stored AQL handler).
func stampCompiledRef(fd FnDefInfo, ref *CompiledFnRef) bool {
	for i := range fd.Signatures {
		if fd.Signatures[i].Fallback {
			continue
		}
		if a, ok := fd.Signatures[i].Impl.(*AQLImpl); ok {
			a.Compiled = ref
			return true
		}
	}
	return false
}

// storedHandlerDeps returns the MODULE-LEVEL names a stored handler / spawn body
// reads — every body word bound as a user `def` at the store site. These are the
// names whose later undef/redefinition (NotifyNameRebound) makes the frozen unit
// stale, so the ref is left unstamped and falls back to CallAQL. Kernel natives
// and the handler's own params/locals are excluded: the store site sits OUTSIDE
// the handler frame, so those are not in r.Defs here. nil when the body reads no
// module-level def.
func (es *EmitState) storedHandlerDeps(body []Value) map[string]bool {
	if es == nil || es.reg == nil {
		return nil
	}
	var deps map[string]bool
	WalkBodyWords(body, func(w WordInfo, _ Value) {
		if w.Name == "" || deps[w.Name] {
			return
		}
		if _, ok := es.reg.Defs.Top(w.Name); ok {
			if deps == nil {
				deps = map[string]bool{}
			}
			deps[w.Name] = true
		}
	})
	return deps
}

// storedFnDeps is storedHandlerDeps for a stored fn VALUE: it reads the module
// deps from the fn's single own-sig body.
func (es *EmitState) storedFnDeps(fd FnDefInfo) map[string]bool {
	if lam, ok := fd.FirstOwnSig(); ok {
		return es.storedHandlerDeps(lam.body())
	}
	return nil
}

// NotifyNameRebound poisons any already-created stored-handler / spawn ref whose
// body reads `name`: a def or undef of that name AFTER the ref was created
// changes what the interpreter resolves at CALL time, but the compiled unit is
// frozen at the store-site definition. Poisoned refs are left unstamped at
// Finalize, so InvokeCallback falls back to CallAQL. A def/undef processed BEFORE
// a ref exists poisons nothing (the ref is not in storedFnRefs yet), so a handler
// over stable module helpers (todo-api's live-todos, mini-redis's arg-at/kv-read,
// never redefined) still compiles.
func (es *EmitState) NotifyNameRebound(name string) {
	if es == nil || !es.active() {
		return
	}
	for _, ref := range es.storedFnRefs {
		if ref.depNames[name] {
			ref.poisoned = true
		}
	}
	// A splice-expanded binding (expandStaticSplices) is FROZEN inside an
	// OpPushClosure unit, which — unlike a spawn ref — cannot be unstamped
	// post-hoc. Rebinding it means the interpreter would re-resolve where the
	// unit holds the old tokens: refuse the whole program (interpreter
	// fallback) rather than diverge. Macro-style defs never rebind in
	// practice, so this hammer stays cold on real programs.
	// A MODULE-SCOPE rebind (no unit open — a body-local def inside another
	// unit's analysis shadows independently and must not poison) of a name
	// some already-analysed unit baked CONCRETELY (frozenReads): the frozen
	// unit would keep the old value where the interpreter re-resolves, so
	// refuse the whole program (interpreter fallback) rather than diverge.
	if len(es.openUnitRecs) == 0 && es.frozenReads[name] {
		es.MarkUncompilable("module binding " + name + " rebound after a fn unit baked its value")
	}
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
		// Inside a compiled fn frame an ACTIVE-token original (a word, a
		// paren, an interpolation, a splice/reach) must not bake: the
		// interpreter re-evaluates it per dispatch against the live frame
		// (a param named by the word — `call {line: src} …` inside
		// repl-eval), which the VM holds as a frame local the baked const
		// can never see. Module scope keeps the recovery (code-as-data
		// consts re-run against the registry, where resolution is
		// identical).
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
// bearsActiveTokens reports whether a value (recursively through lists and
// maps) contains a token the interpreter evaluates or re-steps — a word, a
// paren expression, an interpolation, a splice or reach marker.
func bearsActiveTokens(v Value) bool {
	if IsWord(v) || IsParenExpr(v) || IsInterpString(v) || IsSplice(v) || IsReach(v) {
		return true
	}
	switch d := v.Data.(type) {
	case ListPayload:
		for _, e := range d.Elems {
			if bearsActiveTokens(e) {
				return true
			}
		}
	case MapPayload:
		if d.M == nil {
			return false
		}
		for _, k := range d.M.Keys() {
			mv, _ := d.M.Get(k)
			if bearsActiveTokens(mv) {
				return true
			}
		}
	}
	return false
}

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
	case evCall:
		// A CompileDiverges word (raise) never returns past this call — the
		// same divergence the shallow fragDiverges recognises. Without this,
		// an arm ending in `raise` is mis-seen as a 0-value merge contributor
		// (a false variadic merge), so a fn whose if-chain bottoms out in
		// `raise` reports a spurious variadic return.
		return ev.call.diverges
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
// stripZeroOutPhantoms drops 0-output statement guards' phantom (None)
// results from a residual stack. The program and fn-body residual
// reconciliations skip zeroOut phantoms; arm residuals (RecordBranch) and a
// loop body's per-iteration residual (RecordLoop) must too — otherwise the
// phantom reads as the merge / body-out value and the lowerer refuses with
// "branch leaves extra values" (out=opEvent, vm=0).
func (es *EmitState) stripZeroOutPhantoms(stk []Value) []Value {
	kept := stk
	for i, rv := range stk {
		pr, ok := es.producedBy[rv.ID]
		if ok && es.eventInfo[pr.seq].zeroOut {
			// First phantom found: copy the prefix and filter the rest.
			kept = append([]Value(nil), stk[:i]...)
			for _, r := range stk[i:] {
				if p, o := es.producedBy[r.ID]; o && es.eventInfo[p.seq].zeroOut {
					continue
				}
				kept = append(kept, r)
			}
			break
		}
	}
	return kept
}

func (es *EmitState) RecordBranch(b BranchRecord) {
	if !es.active() {
		return
	}
	// Strip 0-output statement guards' phantom (None) results from the arm
	// residuals BEFORE any counting. A nested both-arms-void `if` (the welford
	// `if … [set … s] [if … [set … s] []]` shape) registers a phantom result the
	// lowerer never seats; the program and fn-body residual reconciliations
	// already skip zeroOut phantoms, and an ARM must too — otherwise resolveArm
	// reads the phantom as the arm's merge value and the lowerer refuses with
	// "branch leaves extra values" (out=opEvent, vm=0). Stripping here keeps
	// resolveArm, residualN, and branchVariadicResult consistent, and lets the
	// both-arms-net-zero → zeroOut marking cascade through nested guards.
	b.ThenStk = es.stripZeroOutPhantoms(b.ThenStk)
	b.ElsStk = es.stripZeroOutPhantoms(b.ElsStk)
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
				if !computedArmCondOK(b, ev.br.cond) { //covergate:allow compiler/VM defensive arm; unreachable without a bytecode-level fault (§compiler)
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
						if !computedArmCondOK(b, ev.br.cond) { //covergate:allow compiler/VM defensive arm; unreachable without a bytecode-level fault (§compiler)
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
	thenDiv := b.Then != nil && fragDiverges(b.Then)
	elsDiv := b.Els != nil && fragDiverges(b.Els)
	// A diverging arm (raise / break / continue / tail) never reaches the
	// merge, so the surviving arm's count is unconditional. A runtime-variable
	// 0-or-1 result therefore arises only from a count MISMATCH between two
	// NON-diverging arms (`if c [99] []`); when either arm diverges the other's
	// fixed count governs. (Multi-value arms were rejected by thenN/elsN > 1
	// above; a nested-variadic surviving arm is caught by armOutVariadic below.)
	if !thenDiv && !elsDiv && (thenN > 0) != (elsN > 0) {
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
	// An identity-less value cannot own a local slot: registering two
	// ""-ID values would collapse them onto ONE slot (the documented
	// record-schema-carrier miscompile shape — see the eager ID mint in
	// core_helpers.go's record path). -1 mirrors the inactive recorder's
	// convention; every caller uses RegisterLocal for its side effect and
	// resolves slots later via localByID, where "" now always misses.
	if id == "" {
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

// BeginLoopCarried opens a carried-def scope for one armed loop analysis
// (AnalyseLoopBody with loop capture active). Pairs with EndLoopCarried.
func (es *EmitState) BeginLoopCarried() {
	if es == nil {
		return
	}
	es.loopCarried = append(es.loopCarried, &loopCarriedScope{
		unitDepth: len(es.units),
		slots:     map[string]int{},
	})
}

// EndLoopCarried closes the innermost carried-def scope, exposing its slot
// inits to the RecordLoop that follows the analysis.
func (es *EmitState) EndLoopCarried() {
	if es == nil || len(es.loopCarried) == 0 {
		return
	}
	top := es.loopCarried[len(es.loopCarried)-1]
	es.loopCarried = es.loopCarried[:len(es.loopCarried)-1]
	es.pendingCarried = top.inits
}

// NoteLoopCarried registers one loop-body REBIND of a pre-existing def as
// loop-carried: the name gets a unit frame slot (reusing an enclosing armed
// loop's slot for the same name, so nested loops over one def share one
// cell), each round's JOINED binding ID resolves to that slot, and — for a
// fresh slot — the pre-loop value is queued as the slot's init, stored
// before the loop's first iteration. Called by AnalyseLoopBody at the end
// of every analysis round; the init operand is re-resolved each round
// because a discarded round's Rollback truncates the const pool the prior
// resolution interned into. A name whose pre-loop value cannot resolve, or
// whose value is function-shaped, is left unregistered — its
// cross-iteration reads then refuse at resolveOperand exactly as before
// (sound interpreter fallback).
func (es *EmitState) NoteLoopCarried(name string, joined, pre Value) {
	if !es.active() || len(es.loopCarried) == 0 {
		return
	}
	scope := es.loopCarried[len(es.loopCarried)-1]
	if scope.unitDepth != len(es.units) {
		return
	}
	u := es.units[len(es.units)-1]
	slot, seen := scope.slots[name]
	if !seen {
		// An enclosing armed loop already carrying this name owns the cell —
		// reuse it (the outer slot is live and current at inner-loop entry;
		// a fresh inner slot with its own init would RESET the accumulation
		// every outer iteration). No init recorded on reuse.
		reused := false
		for i := len(es.loopCarried) - 2; i >= 0; i-- {
			outer := es.loopCarried[i]
			if outer.unitDepth != scope.unitDepth {
				break
			}
			if s, ok := outer.slots[name]; ok {
				slot, reused = s, true
				break
			}
		}
		if !reused {
			if isFnValueResidual(pre) || isFnValueResidual(joined) {
				return
			}
			init, ok := es.resolveOperand(pre)
			if !ok {
				return
			}
			slot = u.numLocals
			u.numLocals++
			scope.inits = append(scope.inits, carriedInit{slot: slot, init: init})
		}
		scope.slots[name] = slot
	} else {
		// Re-resolve the init on a repeat round so its const index targets
		// the pool that survives (a non-final round's Rollback truncated the
		// earlier interning). The pre-loop value is round-invariant, so a
		// resolution that now fails is a recording inconsistency — refuse
		// rather than bake a dangling operand.
		for i := range scope.inits {
			if scope.inits[i].slot != slot {
				continue
			}
			init, ok := es.resolveOperand(pre)
			if !ok {
				es.MarkUncompilable("loop-carried def `" + name + "`: pre-loop value no longer resolves")
				return
			}
			scope.inits[i].init = init
			break
		}
	}
	u.localByID[joined.ID] = slot
}

// RecordDefRebind records a `def` dispatch of a loop-carried name: the
// rebind STORES its value into the name's carried frame slot at the rebind
// site, so a conditional rebind updates the cell exactly when its arm runs
// and a break/continue skips it exactly as the interpreter skips the def.
// A def of a name no active armed loop carries records nothing. Once a
// name IS carried, every rebind must store or the cell goes stale — an
// unresolvable or function-shaped rebind value refuses the program (sound
// interpreter fallback, never a stale read).
func (es *EmitState) RecordDefRebind(name string, v Value, pos SrcPos) {
	if !es.active() || len(es.loopCarried) == 0 {
		return
	}
	slot, found := -1, false
	for i := len(es.loopCarried) - 1; i >= 0; i-- {
		scope := es.loopCarried[i]
		if scope.unitDepth != len(es.units) {
			continue
		}
		if s, ok := scope.slots[name]; ok {
			slot, found = s, true
			break
		}
	}
	if !found {
		return
	}
	if isFnValueResidual(v) {
		es.MarkUncompilable("loop-carried def `" + name + "` rebound to a function value (Stage 3)")
		return
	}
	src, ok := es.resolveOperand(v)
	if !ok {
		es.MarkUncompilable("loop-carried def `" + name + "` rebind of unknown provenance")
		return
	}
	es.appendEvent(emitEvent{kind: evStore, store: &emitStore{src: src, slot: slot, pos: pos}})
}

// RefuseCarriedUndef marks the program uncompilable when `undef` targets a
// name an active armed loop carries: the undef exposes the PREVIOUS binding
// while the carried slot still holds the rebound value, so compiled reads
// would diverge from the interpreter. An undef of any other name is
// untouched.
func (es *EmitState) RefuseCarriedUndef(name string) {
	if !es.active() {
		return
	}
	for i := len(es.loopCarried) - 1; i >= 0; i-- {
		if _, ok := es.loopCarried[i].slots[name]; ok {
			es.MarkUncompilable("undef of the loop-carried def `" + name + "` (Stage 3)")
			return
		}
	}
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
	for k, i := range es.constIDIdx {
		if i >= cp.consts {
			delete(es.constIDIdx, k)
		}
	}
	for i := range es.freshenConst {
		if i >= cp.consts {
			delete(es.freshenConst, i)
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
func (es *EmitState) StartFnCompile(key, name string, fnReg *Registry, args []Value, declared []*Type, paramNames []string, captures []CapturedBinding, generic bool, pos SrcPos) (unit int, finish func([]Value), ok bool) {
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
	nUnnamed := 0
	for i := range args {
		if i >= len(paramNames) || paramNames[i] == "" {
			nUnnamed++
		}
	}
	rec := &fnUnitRec{name: name, nParams: len(args), nUnnamed: nUnnamed, caps: captures, generic: generic, returns: declared, locals: locals, pos: pos, reg: fnReg}
	es.fnRecs = append(es.fnRecs, rec)
	es.fnUnits[key] = unit
	u := &emitUnit{localByID: map[string]int{}, capID: map[string]bool{}, enclosingIDs: es.snapshotCompoundBindingIDs(), reg: fnReg}
	es.units = append(es.units, u)
	es.unitNames = append(es.unitNames, name)
	es.openUnitRecs = append(es.openUnitRecs, unit)
	for _, a := range args {
		es.RegisterLocal(a.ID)
	}
	// Capture slots: the body analysis binds each captured name to
	// cb.Value (the construction-time snapshot — AnalyseFnBody pushes
	// the SAME Value), so body references resolve to these slots by
	// ID. Registered after params, locals nParams…nParams+nCaps-1 —
	// the numbering is POSITIONAL, so an identity-less capture (a value
	// minted at runtime under the mode-gated ID elision, later compiled)
	// cannot be skipped without shifting every subsequent capture's slot:
	// refuse the unit instead (conservative interpreter fallback).
	for _, cb := range captures {
		if cb.Value.ID == "" {
			es.MarkUncompilable("closure captures a runtime-minted value (no compile identity)")
		}
		es.RegisterLocal(cb.Value.ID)
		u.capID[cb.Value.ID] = true
	}
	resume := es.beginFragment()
	es.fnArm = true
	finish = func(bodyStk []Value) {
		resume()
		rec.frag = es.TakeFragment()
		// forceOrder mirrors Finalize's program-residual promotion for THIS
		// unit: when the residual is out of order (an event result above an
		// inert bottom), every residual event is promoted to a frame local so
		// the reconciliation re-pushes the whole residual in exact order.
		var forceOrder map[int]bool
		if !fragDiverges(rec.frag) {
			// Resolve every residual value to an operand, in stack order
			// (bottom→top), so the unit leaves the body's N results for its
			// caller. A 0-output statement guard (`if cond [raise]`) registered a
			// phantom None in the residual but produces 0 runtime values, so it
			// leaves NO operand — skip it, exactly as the top-level residual
			// reconciliation does. A 0-result body leaves outOps empty (a bare RET).
			ops := make([]emitOperand, 0, len(bodyStk))
			vals := make([]Value, 0, len(bodyStk))
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
				vals = append(vals, v)
			}
			// A paren-bounded TRAILING fn-value apply as the ENTIRE body residual
			// (`(prev key comp)` → residual [prev, key, comp], the fn on top with its
			// args below, registered at the paren-collapse boundary with its arity):
			// lower it to OpCallDynTrailTop rather than refusing/miscompiling. outOps
			// stay the full [args…, fn]; the unit lowering applies them to one value
			// before the RET. The args must be plain (non-fn) operands the apply
			// consumes — a nested unapplied fn falls through to refuse below.
			dynTrail := 0
			if len(bodyStk) >= 2 {
				top := bodyStk[len(bodyStk)-1]
				if a := es.TrailingApplyArity(top.ID); a > 0 && a == len(bodyStk)-1 {
					argsOK := true
					for _, v := range bodyStk[:len(bodyStk)-1] {
						if isFnValueResidual(v) { //covergate:allow compiler/VM defensive arm; unreachable without a bytecode-level fault (§compiler)
							argsOK = false
							break
						}
					}
					if argsOK {
						dynTrail = a
					}
				}
			}
			// An `apply`-word pending over a fn CARRIER (Stage M2a): the elided
			// dispatch left the fn on the residual top with its args below —
			// the interpreter's applyHandler re-steps the fn against the whole
			// preceding stack, so the window is the ENTIRE residual. Exactly
			// one pending apply may lower, and only as that whole-residual
			// window with no other fn value in it; any other pending shape
			// (mid-body apply, double apply, apply into a branch join) REFUSES
			// — never compiles the fn+args as unapplied data, and never drops
			// an apply the interpreter performed.
			if pend := u.pendingApply; len(pend) > 0 {
				if dynTrail == 0 && len(pend) == 1 && len(bodyStk) >= 2 &&
					bodyStk[len(bodyStk)-1].ID == pend[0] {
					argsOK := true
					for _, v := range bodyStk[:len(bodyStk)-1] {
						if isFnValueResidual(v) {
							argsOK = false
							break
						}
					}
					if argsOK {
						dynTrail = len(bodyStk) - 1
						// applyHandler unquotes: a /r-parked fn value still
						// applies (OpCallDynApplyTop), unlike the paren case.
						rec.dynTrailApply = true
					}
				}
				if dynTrail == 0 {
					es.MarkUncompilable("fn " + name + ": apply of a dynamic fn value not at the body tail (Stage 3)")
					return
				}
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
			if dynTrail == 0 && rec.closure && len(rec.returns) > 0 && len(ops) != len(rec.returns) {
				es.MarkUncompilable("closure " + name + ": body value count differs from declared returns")
				return
			}
			// A USER fn whose residual COUNT mismatches AND whose residual carries a
			// Function/FnDef VALUE is an UNAPPLIED fn-value-call the compiler missed —
			// `(fnv 100)` pushes [fnv, 100] without applying, where the interpreter
			// applies fnv → ONE value. The count-mismatch-compiles path above assumes
			// the interpreter ALSO mismatches (and the VM RET raises the matching
			// type_error), but here it APPLIES and succeeds. resolveDynamicApply runs
			// only for the MAIN program residual, not fn bodies (cluster E of the broad
			// hunt), so a fn-body fn-value apply is never lowered. Refuse → fall back. (A
			// GENUINE count mismatch that happens to carry a fn value still errors in
			// both engines, so the fallback is sound either way.)
			if dynTrail == 0 && !rec.closure && len(rec.returns) > 0 && len(ops) != len(rec.returns) &&
				!noteDynFrameReplay(u, rec, vals, len(ops)-len(rec.returns)) {
				es.MarkUncompilable("fn " + name + ": unapplied fn-value in body residual (dynamic apply not compiled in a fn body)")
				return
			}
			// A CLOSURE body whose residual carries an UNAPPLIED fn-value — a captured/
			// param comparator applied as `(a b comp)` leaves the residual [a, b, comp]
			// with the fn VALUE on top — must REFUSE, never be trimmed to that fn below:
			// the driving handler (BodyResultTop) would then map to the UNAPPLIED comp
			// instead of its applied result — the off-corpus comparator-each MISCOMPILE
			// the differential cannot see (`[1 2] each [(x x comp)]` → interp [0,0] vs
			// compiled [fn,fn]). This mirrors resolveDynamicApply's main-residual
			// fn-carrier / fn-precedes-args refusals (the fn-body path refuses the same
			// shape via the !rec.closure unapplied-fn-value check above). A SOLE inert
			// fn-reference body (`each [cmp/r]`, mapping every element to the reference)
			// is a CONCRETE const — not a carrier and not preceded by args — so it still
			// compiles; only an unapplied dynamic apply refuses. (Lowering the trailing
			// apply via OpCallDynamicTrailing in a closure body is the follow-on feature.)
			if dynTrail == 0 && rec.closure {
				for i, v := range bodyStk {
					if isFnTypedCarrier(v) || (isFnValueResidual(v) && (i > 0 || i+1 < len(bodyStk))) {
						es.MarkUncompilable("closure " + name + ": unapplied fn-value in body residual (dynamic apply not lowered)")
						return
					}
				}
			}
			// A TOP-TAKING closure's driving handler reads only the top of the body
			// residual (each / fold / scan / filter — CallableSpec.BodyResultTop), so
			// the values the body leaves BELOW its result — notably the per-invocation
			// input a body that ignores its element leaves on the stack (`each [add 1
			// 0]` → [input, 3]) — are never observed and are dropped here. Without this,
			// the residual [inert-input, computed-result] refuses at the RET (an event
			// above an inert, "result above a literal"), even though the handler only
			// ever reads the top.
			if dynTrail == 0 && rec.takesTop && len(ops) > 1 {
				ops = trimToTopResult(ops)
			}
			// A per-iteration APPLY-LOOP's variadic value region in the residual,
			// under an inert tail: the RET takes the replay trim discipline.
			if dynTrail == 0 && rec.dynFrameW == 0 && !rec.closure && len(rec.returns) > 0 {
				ops = es.noteApplyLoopReplay(rec, ops)
			}
			rec.dynTrailArity = dynTrail
			rec.outOps = ops
			// VARIADIC-RETURNING fn: the body residual's defining (top) event leaves
			// a runtime-variable count (a variadic branch / loop, or a call to an
			// already-variadic fn — RecordUserCall flags that on the event). A call
			// site marks its result variadic (lowerUserCall) so only a
			// variadic-absorbing position consumes it.
			if n := len(ops); n > 0 && ops[n-1].kind == opEvent && es.eventInfo[ops[n-1].idx].variadicResult {
				rec.variadic = true
			}
			// Out-of-order residual (an event result above an inert bottom —
			// `do [x 1 add 2]` → [const-x, event]): the in-order RET
			// reconciliation would refuse "result above a literal". Mirror
			// Finalize's program-residual promotion: force EVERY residual
			// event to a frame local, so the reconciliation pushes the whole
			// residual (locals + consts + types) in exact order. Guarded off
			// the fn-value / dynamic-apply shapes, whose stack layout IS the
			// apply's contract and must not reorder — and off a trimmed
			// residual (len(ops) != len(vals) after trimToTopResult, which is
			// in-order by construction).
			if dynTrail == 0 && rec.dynFrameW == 0 && len(ops) == len(vals) {
				forceOrder = residualForceOrder(ops, vals)
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
		rec.promoted, rec.dead = es.planValueDefLocals(u, rec.frag.events, outSeqs, forceOrder)
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
		es.unitNames = es.unitNames[:len(es.unitNames)-1]
		es.openUnitRecs = es.openUnitRecs[:len(es.openUnitRecs)-1]
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
// SetUnitParamTypes records the declared param types for a compiled fn unit, so
// the VM enforces them at CALL_USER entry (the gradual-Any param-guard, mirroring
// the RET return-check). Set from the matched sig at the dispatch site, where the
// param types are known; a memoised unit is harmlessly re-set with the same types.
func (es *EmitState) SetUnitParamTypes(unit int, paramTypes []*Type, paramPatterns []*Value) {
	if unit < 0 || unit >= len(es.fnRecs) {
		return
	}
	es.fnRecs[unit].paramTypes = paramTypes
	es.fnRecs[unit].paramPatterns = paramPatterns
}

// SetUnitDecl records the return-contract declaration site for a compiled fn
// unit (FnSig.Decl), so a compiled RET return-type/count error labels the
// declaration exactly as the interpreter does. Callers that have the FnSig in
// hand (the named-fn and user-poly compile paths) set it; anonymous closures
// leave it zero.
func (es *EmitState) SetUnitDecl(unit int, decl DeclSite) {
	if unit < 0 || unit >= len(es.fnRecs) {
		return
	}
	es.fnRecs[unit].decl = decl
}

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

// RecordUserPolyCall records one runtime-dispatched MULTI-OVERLOAD user-fn
// call (the user-fn mirror of RecordPolyCall): the checker could not commit
// to one same-arity overload (a gradual-Any arg reached two or more), every
// arm's body already compiled to its own unit (tryCompileUserPolyArms), and
// the call lowers to OpCallUserPoly, which re-runs MatchSignature over the
// recorded arm subset at run time — the SAME first-match the interpreter
// takes. args are in signature order; outs are the committed dispatch's
// result carriers (every arm declares the identical Returns, gated by the
// recorder). Captures are gated empty by the recorder, so no hidden trailing
// operands ride here. An operand of unknown provenance marks the program
// uncompilable, exactly as RecordUserCall does.
func (es *EmitState) RecordUserPolyCall(word string, ownerReg *Registry, sigIdx, units []int, impls []SigImpl, args, outs []Value, pos SrcPos) {
	if !es.active() {
		return
	}
	ops := make([]emitOperand, len(args))
	for i, a := range args {
		op, ok := es.resolveOperand(a)
		if !ok {
			es.MarkUncompilable("fn call operand of unknown provenance")
			return
		}
		ops[i] = op
	}
	seq := es.appendEvent(emitEvent{kind: evCallUser, uc: emitUserCall{
		unit: -1, ops: ops, nout: len(outs), pos: pos,
		poly: &emitUserPolySpec{word: word, reg: ownerReg, sigIdx: sigIdx, units: units, impls: impls},
	}})
	es.SiteCounts[SiteDynamic]++
	for i := range outs {
		es.setProducedAt(outs[i], seq, i)
	}
}

// RecordDynApply records a paren-bounded TRAILING fn-value apply (`(a b comp)`):
// the runtime fn VALUE `fn` applied to `args` (in source order — args[0] pushed
// first / deepest), netting the single result `out`. Recorded as an EVENT so the
// apply seats like any computed result — a def-local (`def c (a b comp)`), an `if`
// operand, a list member, OR the body's trailing residual — not ONLY the body
// residual (the old register-at-collapse / lower-at-reconciliation path handled
// just that). Returns false (sound interpreter fallback) when any operand is
// unresolvable or an ARG is itself an unapplied fn value (a nested apply the flat
// layout cannot order). ops are sig-order (ops[0] = top): the fn on TOP, then its
// args below it deepest-last — exactly the stack OpCallDynTrailTop reads (it
// reverses the arg window into forward order to match the interpreter's paren
// auto-dispatch, where the fn's first param is the arg just below it).
func (es *EmitState) RecordDynApply(args []Value, fn, out Value, pos SrcPos) bool {
	if !es.active() {
		return false
	}
	if !isFnValueResidual(fn) { // fn must be a genuine fn-value residual
		return false
	}
	// The lowered apply (OpCallDynTrailTop) nets EXACTLY ONE value (nout: 1
	// below). AQL fns can return 0 or multiple values, so refuse the lowering
	// for a CONCRETE callee (a baked `/r` reference) that is not provably
	// single-valued: a multi-return callee miscompiles (`(1 2 pair/r)` with
	// pair → [Integer Integer] compiled to [2 1] vs the interpreter's [1 2])
	// and a zero-return callee underflows the following STORE_LOCAL/DROP. A
	// fn-typed CARRIER (a `comp:Function` param/captured value) carries no
	// static return arity here; it stays lowered (the comparator convention),
	// matching the interpreter's single-result paren auto-dispatch.
	if !fnConcreteSingleValuedOrCarrier(fn) {
		return false
	}
	fnOp, ok := es.resolveOperand(fn)
	if !ok {
		return false
	}
	ops := make([]emitOperand, 0, len(args)+1)
	ops = append(ops, fnOp)
	for i := len(args) - 1; i >= 0; i-- {
		if isFnValueResidual(args[i]) {
			return false
		}
		op, ok := es.resolveOperand(args[i])
		if !ok {
			return false
		}
		ops = append(ops, op)
	}
	// A paren-bounded apply that ALSO dispatched through the `apply` word
	// (`(v comp/r apply)`) registered a pending unit apply before this event
	// collapsed the tape — the event now owns the apply, so consume the
	// pending entry rather than leaving it to refuse the unit at finish, and
	// lower with the apply word's UNQUOTE semantics (OpCallDynApplyTop).
	unquote := false
	if len(es.units) > 0 {
		u := es.units[len(es.units)-1]
		for i, id := range u.pendingApply {
			if id == fn.ID {
				u.pendingApply = append(u.pendingApply[:i], u.pendingApply[i+1:]...)
				unquote = true
				break
			}
		}
	}
	es.SiteCounts[SiteMono]++
	seq := es.appendEvent(emitEvent{kind: evCall, call: emitCall{word: wordDynApply, ops: ops, nout: 1, pos: pos, dynApply: len(args), dynApplyUnquote: unquote}})
	es.setProduced(out, seq)
	return true
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
	// Claim the just-closed analysis's carried-def slot inits (EndLoopCarried)
	// up front so an early refusal below never leaks them to a LATER loop.
	carried := es.pendingCarried
	es.pendingCarried = nil
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
	lp := &emitLoop{start: startOp, end: endOp, step: stepOp, iterSlot: -1, pos: pos, carried: carried}
	// A body ending in a 0-output statement guard (`if c [def …] []` — the
	// loop-carried conditional-rebind shape) leaves only the guard's phantom
	// None; stripping it classifies the body as the SIDE-EFFECT (zeroOut)
	// loop it is at run time, exactly like the arm/fn residual strips.
	bodyStk = es.stripZeroOutPhantoms(bodyStk)
	if len(bodyStk) > 0 && !fragDiverges(body) {
		if es.setLoopBodyApply(body, bodyStk) {
			// A per-iteration dynamic apply (`(mk2 i) 10`): the body nets one applied
			// value, lowered via OpCallDynamic over the leading fn (body.applyArgs).
			lp.hasBodyOut = true
		} else if len(bodyStk) > 1 {
			// The body nets MORE THAN ONE value per iteration (`for 2 ['e' 'f']`
			// pushes both 'e' and 'f' each pass). The lowered loop nets exactly one
			// value per iteration, so keeping only bodyStk[last] would silently DROP
			// the rest — a miscompile ([e f e f] -> [f f]). Refuse and let the
			// interpreter island run the multi-value body faithfully.
			es.MarkUncompilable("for: body nets multiple values per iteration")
			return
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
	f := es.eventInfo[seq]
	if len(body.applyArgs) > 0 {
		f.applyLoop = true
	}
	if lp.hasBodyOut {
		// A value-producing loop leaves a runtime-variable count (one per-iteration
		// value, N unknown at compile time) — variadic, like lowerLoop marks
		// lw.variadic. Only the program residual absorbs it.
		f.variadicResult = true
	} else {
		// A SIDE-EFFECT loop (body nets 0 per iteration — `for n [acc set …]`)
		// leaves ZERO runtime values, deterministically: a zero-output event, NOT a
		// variadic result. Marking it zeroOut lets a fn/closure body that ends in
		// (or discards, via `def _ (for …)`) such a loop drop its result and RET
		// cleanly, instead of refusing "body leaves extra values"/"variadic loop
		// value". The loop's side effects are emitted events and still run.
		f.zeroOut = true
	}
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
	if !ok {
		return false
	}
	// A frame-LOCAL fn read (an `args.N` fold of a Function param inside a fn
	// unit): nothing sits on the sim after the body events, so the fragment
	// re-pushes the local before the args (applyFn). An event-produced fn (the
	// factory `(mk2 i)`) is already on the sim top — the original layout.
	if fnOp.kind == opLocal {
		// The lowered apply is emitted at one of the iteration's two ends, so
		// the fn read must bound the body's other statements in source order:
		// every event strictly before it (apply-last, the hoist) or strictly
		// after it (apply-first — a continue in the applied body must skip
		// them). A mid-body apply declines and keeps the refusal.
		fnPos := bodyStk[0].Pos()
		if fnPos.Row == 0 && fnPos.Col == 0 {
			// A folded args.N read loses its own position; the apply's ARG
			// literals sit inside the same paren, so any of theirs stands in.
			for _, a := range bodyStk[1:] {
				if a.Pos().Row != 0 || a.Pos().Col != 0 {
					fnPos = a.Pos()
					break
				}
			}
		}
		if fnPos.Row == 0 && fnPos.Col == 0 {
			return false // no source anchor — cannot order the apply
		}
		before, after := 0, 0
		for i := range body.events {
			p := eventPos(body.events[i])
			if p.Row == 0 && p.Col == 0 {
				continue // no position recorded — count as neither side
			}
			if p.Row < fnPos.Row || (p.Row == fnPos.Row && p.Col < fnPos.Col) {
				before++
			} else {
				after++
			}
		}
		if before > 0 && after > 0 {
			return false
		}
		body.applyFn = &fnOp
		body.applyFirst = after > 0
	} else if fnOp.kind != opEvent {
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

// RecordTrapErr is RecordTrap for a fully-built interpreter AqlError: it
// serialises the whole diagnostic — code, detail, word, hint, and the
// structured payload (secondary spans, notes, suggestions) — into the trap so
// the compiled OpTrap raises a report byte-identical to the interpreter's. Used
// where the check pass builds the interpreter's own error at compile time (a
// statically-definite unmatched dispatch, whose runtime values are provably the
// same the check pass saw — tryRecordUnmatchedDispatchTrap). Same top-level-only
// guard as RecordTrap.
func (es *EmitState) RecordTrapErr(ae *AqlError, pos SrcPos) bool {
	if ae == nil {
		return false
	}
	if !es.active() || len(es.frames) != 1 || len(es.units) != 1 {
		return false
	}
	if es.trapAt != 0 {
		return true
	}
	es.trapAt = es.appendEvent(emitEvent{kind: evTrap, trap: emitTrap{
		spec: TrapSpec{
			Code: ae.Code, Detail: ae.Detail, Word: ae.Src, Hint: ae.Hint,
			Spans: ae.Spans, Notes: ae.Notes, Suggestions: ae.Suggestions,
		},
		pos: pos,
	}})
	return true
}

// RecordTypedBind records the runtime validate/reparent step of a typed
// value-def (`def x:Pos n`) whose constraint is a refinement and whose body is
// DYNAMIC — the compiled replacement for the "dynamic refinement
// reparent/validate is interpreter-only" refusal. The event pops the body
// operand, runs RunTypedBind (the interpreter-mirroring OpBindTyped), and
// pushes the value the interpreter binds; `out` is the CHECK-mode binding the
// def handler is about to install (the reparented carrier / the base-tagged
// carrier for a DepScalar), returned with a FRESH provenance ID registered
// against the new event so downstream references to the binding resolve to the
// bind's RESULT, not to the raw body operand (out shares the body's ID —
// ReparentValue preserves it — and without the remint a reference would
// resolve straight to the un-reparented param local: miscompile B's exact
// mechanism, design/MISCOMPILE-HUNT-FINDINGS.0.md §B).
//
// Declines (returning out unchanged and false) when recording is inactive or
// the body is CONCRETE — a static typed-def's reparent rides the const pool
// faithfully and must keep today's proven path — or when the body operand has
// no resolvable provenance; the caller then falls back to the refusal mark,
// which itself no-ops for the concrete/inactive cases.
func (es *EmitState) RecordTypedBind(spec TypedBindSpec, in, out Value, pos SrcPos) (Value, bool) {
	if !es.active() || IsConcrete(in) {
		return out, false
	}
	op, ok := es.resolveOperand(in)
	if !ok {
		return out, false
	}
	sp := spec
	seq := es.appendEvent(emitEvent{kind: evCall, call: emitCall{
		word: wordTypedBind, ops: []emitOperand{op}, nout: 1, pos: pos, typedBind: &sp,
	}})
	out.ID = GenerateID(IDPrefixForType(out.Parent))
	es.setProduced(out, seq)
	return out, true
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
	es.noteFnRiskFields(word, args, outs)
	if es.recordCallElided(word, sig, args, outs) {
		return
	}
	if es.recordShuffleElided(word, sig, args, outs) {
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
	// A CompileValueDiverges word (div/mod) raises value-dependently: its
	// check-mode ReturnsFn drops the declared result (len(outs)==0) exactly on
	// the divergent path (a static-zero divisor), so treat that call as a
	// divergent terminal too — a closure body ending in it compiles with no RET
	// and the catching word wraps the raised error, instead of islanding.
	diverges := sig.CompileEffect.Has(CompileDiverges) ||
		(sig.CompileEffect.Has(CompileValueDiverges) && len(outs) == 0)
	seq := es.appendEvent(emitEvent{kind: evCall, call: emitCall{word: word, sig: sig, ops: ops, nout: len(outs), pos: pos, diverges: diverges}})
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
	if sig != nil && sig.fnFrame() != nil && len(outs) == 0 {
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
		// `apply` of a Function/FnDef-typed CARRIER (a `comp:Function` param or
		// captured comparator inside a fn body — `v comp/r apply`, Stage M2a):
		// the check engine cannot re-step a carrier, so the identity result
		// flows to the body residual as an fn value. Elide the dispatch and
		// register the PENDING apply on the enclosing unit; the unit's finish
		// either lowers it as the whole-residual OpCallDynTrailTop (the fn
		// applied to every value below it, exactly the interpreter's
		// applyHandler re-step over the preceding stack) or refuses — a pending
		// apply can never be silently dropped. Top-level applies (len(units)
		// == 1) keep today's refusal: the program residual has no equivalent
		// single-consumer window.
		if len(es.units) > 1 && sig != nil && sig.fnFrame() == nil && isFnTypedCarrier(args[0]) {
			u := es.units[len(es.units)-1]
			u.pendingApply = append(u.pendingApply, args[0].ID)
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
	if (isGetWord(word) || isGetrWord(word)) && len(outs) == 1 && IsConcrete(outs[0]) {
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
	shuffleOK := es.dynamicStackShuffleOK(word, sig)
	switch {
	case sig == nil:
		es.MarkUncompilable("dispatch without a signature at " + word)
	case word == "":
		// Anonymous / fn-value dispatch (usurp wrappers, F4 value
		// calls): the callee is a runtime value, Stage 3 territory.
		es.SiteCounts[SiteMeta]++
		es.MarkUncompilable("anonymous function dispatch (Stage 3)")
	case sig.runInCheckMode():
		es.SiteCounts[SiteMeta]++
		es.MarkUncompilable("compile-time word " + word)
	case sig.fnFrame() != nil:
		es.SiteCounts[SiteMeta]++
		es.MarkUncompilable("user fn call " + word + " (Stage 3)")
	case sig.fullStack():
		es.SiteCounts[SiteMeta]++
		es.MarkUncompilable("full-stack word " + word)
	case isGetFamilyWord(word) && !es.shapedReadOut(outs) && (containerFnAutoDispatchRisk(args) || zeroArgFnOut(outs) || es.instanceFnFieldRisk(args)):
		// A get/dot/getr/dotr read from a container HOLDING a function member
		// may surface that fn, and the interpreter AUTO-DISPATCHES a surfaced
		// fn value in every delivery context (probe-verified: `{f:make42/r}.f`
		// → 42; bare `{b:f/r} dot b` → 7; `… dot b add 1` → 8 — it even
		// collects forward args). The VM would push it as inert data — a
		// silent wrong value (miscompile mechanism E, the deferred-field
		// auto-invoke, design/MISCOMPILE-HUNT-FINDINGS.0.md). Refuse on the
		// RECEIVER signal: reads from fn-free containers are unaffected. An
		// ANNOTATED shaped-method read (shapedReadOut) is exempt: its landing
		// is modelled by tryShapedMethodDispatch, whose guard-owned decline
		// re-refuses, so the guard is re-homed rather than weakened.
		es.SiteCounts[SiteMeta]++
		es.MarkUncompilable("fn value read from a container auto-dispatches (Stage 3)")
	case word == "args" || word == "__pa":
		// `args` reads the interpreter's per-call args stack, which the
		// VM's CALL_USER frame does not maintain (it binds params to
		// frame locals instead). A compiled fn body that reads `args`
		// would fail with "args: not inside a function" — refuse so the
		// program falls back to the interpreter.
		es.SiteCounts[SiteMeta]++
		es.MarkUncompilable("context-dependent word " + word)
	case len(sig.NoEvalArgs) > 0 && (sig.CompileEffect.Has(CompileExecutesBody) || (sig.Callable != nil && execBodyRefsNames(sig, args)) || (!sig.CompileEffect.Has(CompileRunsBodyIsolated) && !es.noEvalBodiesInertScoped(sig, args))):
		// A code-body word is refused when:
		//   - its body is not inert data (UNLESS the word runs the body in an
		//     ISOLATED CallAQL frame — CompileRunsBodyIsolated — where name
		//     resolution is identical under interpreter and VM: Test.check-prop's
		//     dynamic gen/property bodies bake as CALL_NATIVE operands and run
		//     through the same captured-parent handler in both modes); OR
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
	case hasUncoveredQuoteArg(sig) && !isGetWord(word) && !isGetrWord(word) && word != "set" && !quoteInertOK:
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
	case anyDynamicCarrier(args) && !shuffleOK && !es.dynInputsProven(sig, args):
		es.SiteCounts[SiteDynamic]++
		es.MarkUncompilable("dynamic input at " + word)
	case anyDynamicCarrier(outs) && !forceDynOut && !shuffleOK:
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

// recordShuffleElided reports whether a dispatch is a pure ID-PRESERVING stack
// shuffle (swap / rot / swap2 — a permutation, no duplication or drop) over
// gradual (dynamic) operands, in which case NOTHING is recorded: the outputs
// ARE the inputs by identity (ReturnsIdentity keeps each value's ID), so every
// downstream consumer resolves the same IDs to their ORIGINAL producers
// (params, captures, consts, prior events) and the lowerer re-derives any
// stack motion from dataflow (swapTop2 / spillSeat). Baking a CALL_NATIVE
// instead strands the pass-through copy of a frame-local operand on the
// simulated stack — the local re-pushes at its consumer, so the shuffle's
// output for it is never popped and the unit refuses "body leaves extra
// values" (the `input swap eval-pred` each-body shape). Gated to DYNAMIC
// operands — the case that refused outright before — so already-compiling
// concrete shuffles keep their existing CALL_NATIVE layout, byte-identical.
// A duplicating shuffle (dup/over/…) mints fresh output IDs (ReturnsIdentity),
// fails the multiset check, and falls through to the normal bake.
func (es *EmitState) recordShuffleElided(word string, sig *Signature, args, outs []Value) bool {
	if !anyDynamicCarrier(args) || !es.dynamicStackShuffleOK(word, sig) {
		return false
	}
	if len(outs) != len(args) {
		return false
	}
	remaining := make(map[string]int, len(args))
	for _, a := range args {
		remaining[a.ID]++
	}
	for _, o := range outs {
		if remaining[o.ID] == 0 {
			return false
		}
		remaining[o.ID]--
	}
	return true
}

// dynamicStackShuffleOK reports whether a dispatch with dynamic (gradual-Any)
// operands is still sound to bake as a plain CALL_NATIVE: the word is one of
// the kernel's pure stack-shuffle primitives. Their ONE signature is all-Any,
// so every runtime value matches it — there is no sibling overload the checker
// could have mis-committed against (the hazard the anyDynamicCarrier refusal
// guards) — and the handler moves values without inspecting them
// (ReturnsIdentity / empty returns), so the outputs ARE the inputs and KEEP
// their dynamic modality: every downstream typed dispatch still sees Dynamic
// and gates itself (poly re-match, guarded CALL_USER, or refusal) exactly as
// before. This is the `input swap eval-pred` shape inside an each body over an
// untyped list. Guarded on pointer identity with the registry's own binding so
// a shadowed name (a user `def swap …`, whose sig has an fnFrame anyway) never
// rides the exemption. depth/pick/roll are full-stack words and refused earlier.
// dynStackShuffleWords is the closed set of Forth-style stack words whose
// dispatch the compiler may trust over dynamically-shaped stacks: each is
// stack-only (BarrierPos 0 — it never forward-collects) and, when its
// registered signature is the single all-Any one, one extra or differently-
// typed stack value cannot flip which overload it takes. Shared by
// dynamicStackShuffleOK (the refusal bypass for a MATCHED dispatch over a
// dynamic operand) and dynShuffleConsumerAt (the gradual-arity modeling
// gate for a mixed-arity mutator's statement-position result).
var dynStackShuffleWords = map[string]bool{
	"dup": true, "swap": true, "drop": true, "over": true, "rot": true,
	"nip": true, "tuck": true, "dup2": true, "swap2": true, "drop2": true,
	"over2": true,
}

func (es *EmitState) dynamicStackShuffleOK(word string, sig *Signature) bool {
	if !dynStackShuffleWords[word] {
		return false
	}
	if es == nil || es.reg == nil || sig == nil {
		return false
	}
	fn := es.reg.Lookup(word)
	if fn == nil || len(fn.Signatures) != 1 || &fn.Signatures[0] != sig {
		return false
	}
	for _, t := range sig.ArgTypes() {
		if t == nil || !t.Equal(TAny) {
			return false
		}
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
	for i, t := range sig.ArgTypes() {
		if t != nil && (t.ConformsTo(TFunction) || t.ConformsTo(TFnDef)) {
			if inertFn || sig.FnInertArgs[i] {
				continue
			}
			es.SiteCounts[SiteMeta]++
			es.MarkUncompilable("function-valued operand at " + word + " (Stage 3)")
			return nil, false
		}
	}
	for i, a := range args {
		if _, ok := a.Data.(FnDefInfo); ok {
			if inertFn || sig.FnInertArgs[i] {
				continue
			}
			es.SiteCounts[SiteMeta]++
			es.MarkUncompilable("function value reaches " + word + " (Stage 3)")
			return nil, false
		}
	}
	ops := make([]emitOperand, len(args))
	for i, a := range args {
		// STORE-FN edge: a word that stashes a fn to invoke LATER (serve-raw)
		// gets its capture-free handler body compiled to its own unit, and a
		// durable CompiledFnRef stamped on the handler value's shared *AQLImpl —
		// so the interned const the word receives carries the VM edge alongside
		// its raw Body, and the word runs it on the VM (RunUnit) instead of the
		// interpreter (CallAQL). Done here, BEFORE resolveOperand's outcome is
		// consulted, because resolveOperand already interns a concrete fn value
		// as a const (ok=true) and would skip the inert-fn branch below; the
		// stamp mutates the pointer that interned copy shares, so it lands either
		// way. A body that refuses to compile leaves the const un-stamped and the
		// handler falls back to the interpreter, per-body and sound.
		if sig.CompileEffect.Has(CompileStoresFn) {
			if fd, isFn := a.Data.(FnDefInfo); isFn && IsConcrete(a) && len(fd.Captured) == 0 {
				if unit, cOK := es.compileStoredFnUnit(fd, a.Pos()); cOK {
					ref := &CompiledFnRef{Unit: unit, depNames: es.storedFnDeps(fd)}
					if stampCompiledRef(fd, ref) {
						es.storedFnRefs = append(es.storedFnRefs, ref)
					}
				}
			}
		}
		// STORE-BODY edge: a word that stashes a NoEvalArgs code-body to run later
		// on its own registry (spawn) gets that body compiled to a 0-param unit,
		// and receives a synthetic fn-value carrier (raw tokens + CompiledFnRef) in
		// place of the raw list — so it runs the unit via RunUnit on its fork. A
		// body that refuses to compile falls through to the plain list const and
		// the word runs it on the interpreter, unchanged.
		if sig.CompileEffect.Has(CompileStoresBody) && sig.NoEvalArgs != nil && sig.NoEvalArgs[i] {
			if carrier, cok := es.compileStoredBody(a); cok {
				ops[i] = constOperand(es.intern(carrier))
				continue
			}
		}
		op, ok := es.resolveOperand(a)
		if !ok && (introspect || inertFn || sig.FnInertArgs[i]) {
			if fd, isFn := a.Data.(FnDefInfo); isFn {
				switch {
				case IsConcrete(a) && len(fd.Captured) == 0:
					// A CAPTURE-FREE concrete (immutable) fn value bakes as a const
					// the introspection / fn-inert-slot / store-fn handler reads at
					// run time. A CAPTURING fn must NOT bake as a const: its baked
					// body leaves the captured names as bare Words with no binding
					// (`word(bse)`), so a later invocation reads them unbound — the
					// capturing-sink miscompile. It falls to the closure path below.
					op, ok = constOperand(es.intern(a)), true
				default:
					// A CAPTURING anonymous lambda in a store-fn / fn-inert slot
					// (`Net.serve-raw {…} ([conn] => [ handle conn store ])`,
					// mini-S3's per-connection actor closing over the shared store):
					// resolveOperand cannot materialise it as a const, but it lowers
					// to an OpPushClosure — the captures resolved in THIS frame and
					// packed into the closure value the storing native invokes per
					// call, exactly as the interpreter dispatches the AQL handler
					// with its captures. tryReturnedClosure declines (leaving es
					// untouched) when a capture is unreachable or the body refuses,
					// so an uncompilable handler still falls back faithfully.
					if cop, cok := es.tryReturnedClosure(a, a.Pos()); cok {
						op, ok = cop, true
					}
				}
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
	if isGetFamilyWord(word) && !es.shapedReadOut(outs) && (containerFnAutoDispatchRisk(args) || zeroArgFnOut(outs) || es.instanceFnFieldRisk(args)) {
		// Same auto-dispatch divergence as the mono path (recordCallRefusal):
		// the interpreter invokes a container-read fn value as it lands; the
		// VM would push it as data. Refuse the program (sound fallback).
		// Annotated shaped-method reads are exempt (see recordCallRefusal).
		es.SiteCounts[SiteMeta]++
		es.MarkUncompilable("fn value read from a container auto-dispatches (Stage 3)")
		return true
	}
	ops := make([]emitOperand, len(args))
	for i := range args {
		a := args[i]
		// A TYPE-NAME word in the claimed window — the un-stepped `Bytes` in
		// `convert Bytes <dynamic>`: the dispatch could not commit statically
		// (the data operand's type is unknown), so the plan's claimed args
		// still hold the RAW name token. At runtime the interpreter steps the
		// name to its canonical type literal before the word dispatches; give
		// the poly window the same value — the canonical literal lowers via
		// resolveOperand's type-operand path (OpPushType by ID), so callPoly's
		// re-match sees exactly the operand the interpreter's dispatch does.
		// Only a PLAIN word naming a live type qualifies; anything else keeps
		// the unresolvable-operand decline below.
		if IsWord(a) && es.reg != nil {
			if w, wErr := AsWord(a); wErr == nil &&
				w.ArgCount == -1 && !w.ForceStack && !w.ForceForward && !w.ForceRef && !w.ForceUsurp {
				// Mirror stepWord's type-name cascade exactly: the kernel
				// name table, then the external-builtin path resolver, then
				// the registry's dynamic type bindings.
				if t, tOK := typeNames[w.Name]; tOK {
					a = NewTypeLiteral(t)
				} else if t, tOK := ResolveTypePath(w.Name); tOK {
					a = NewTypeLiteral(t)
				}
			}
		}
		op, ok := es.resolveOperand(a)
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

// RecordDynMethod records a GUARDED shaped-instance-method apply (Stage M2c):
// fn is the dynamic method-read carrier (its producing event supplies the
// RUNTIME value — nothing of the check-mode shape instance is baked, the
// freeze-gate), args the inert statement-window operands the check-mode match
// consumed, outs the matched member signature's declared results. Lowers to
// OpCallDynMethod with the arity + result-count claim the VM enforces (claim
// failure → internal_error → interpreter re-run). Returns false, leaving es
// untouched, when an operand has no compiled home — the caller then refuses.
func (es *EmitState) RecordDynMethod(fn Value, args, outs []Value, word string, pos SrcPos) bool {
	if !es.active() {
		return false
	}
	fnOp, ok := es.resolveOperand(fn)
	if !ok {
		return false
	}
	ops := make([]emitOperand, 0, len(args)+1)
	ops = append(ops, fnOp)
	for i := range args {
		op, ok := es.resolveOperand(args[i])
		if !ok {
			return false
		}
		ops = append(ops, op)
	}
	es.SiteCounts[SiteDynamic]++
	seq := es.appendEvent(emitEvent{kind: evCall, call: emitCall{
		word: word, ops: ops, nout: len(outs), pos: pos,
		dynMethod: &DynMethodSpec{Word: word, NArgs: len(args), NOut: len(outs)},
	}})
	for i := range outs {
		es.setProducedAt(outs[i], seq, i)
	}
	return true
}

// containerFnAutoDispatchRisk reports whether a get-family dispatch may
// surface a container member the interpreter could AUTO-DISPATCH after it
// lands: a function value carrying a 0-arg-satisfiable signature. Probe-
// verified divergences: `{f:make42/r}.f` → 42, bare `{b:f/r} dot b` → 7,
// and `{f:add1/r}.f 5` → 6 (a landed fn even collects forward args), while
// the VM would push the fn as inert data (miscompile mechanism E). NOTE:
// parked user-fn values structurally carry a 0-arg Signature alongside
// their declared overloads, so in practice every user-fn member READ
// refuses — the safe side, since landing-then-forward-collecting makes even
// multi-param members invocable. The precision that pays is the KEY
// resolution: when another concrete arg resolves as the read key, only that
// member is inspected, so reads of NON-fn keys from fn-carrying containers
// keep compiling; an unresolvable (computed) key inspects every member
// conservatively. The receiver signal behind the get-family auto-dispatch
// refusals (recordCallRefusal, RecordPolyCall). Census cost at landing:
// 2 rows (3,875-row corpus), both previously divergence-exposed.
// noteFnRiskFields records, at a construction dispatch (make), which field
// keys of the produced INSTANCE hold genuinely-0-param fn values — the
// auto-dispatch-on-read hazard containerFnAutoDispatchRisk cannot see when
// the later get-family receiver is the instance CARRIER (schema only, no
// field values; probe: `def C class {f:Function} def o (make C
// {f:make42/r}) o.f` → 42 interpreted vs the fn value compiled — PR #225
// P1). The construction MAP is concrete at record time, so the hazard is
// decidable here and consulted by ID at the get-family guard sites. A field
// later overwritten with a non-fn value over-refuses (sound, rare —
// documented trade).
func (es *EmitState) noteFnRiskFields(word string, args, outs []Value) {
	if word != "make" || len(outs) == 0 {
		return
	}
	var risky map[string]bool
	for _, a := range args {
		mp, ok := a.Data.(MapPayload)
		if !ok || a.Carrier || mp.M == nil {
			continue
		}
		for _, k := range mp.M.Keys() {
			mv, _ := mp.M.Get(k)
			if fnValueZeroArg(mv) {
				if risky == nil {
					risky = map[string]bool{}
				}
				risky[k] = true
			}
		}
	}
	if risky == nil {
		return
	}
	if es.fnRiskFields == nil {
		es.fnRiskFields = map[string]map[string]bool{}
	}
	for _, o := range outs {
		if o.ID != "" {
			es.fnRiskFields[o.ID] = risky
		}
	}
}

// instanceFnFieldRisk consults noteFnRiskFields' record for a get-family
// dispatch whose receiver is (a carrier of) a tracked instance: a concrete
// key inspects only that field, an unresolvable key any tracked field.
func (es *EmitState) instanceFnFieldRisk(args []Value) bool {
	if len(es.fnRiskFields) == 0 {
		return false
	}
	for _, a := range args {
		risky := es.fnRiskFields[a.ID]
		if risky == nil {
			continue
		}
		if key, ok := concreteMapKey(args); ok {
			if risky[key] {
				return true
			}
			continue
		}
		return true
	}
	return false
}

// zeroArgFnOut is the OUT-side backstop to the receiver heuristic: when a
// get-family dispatch's check-mode result IS a concrete fn value with a
// genuine 0-param overload, the read demonstrably surfaced an
// auto-dispatchable member regardless of how the receiver was represented
// (a class-instance CARRIER receiver dodges the payload inspection —
// probe: `def C class {f:Function} def o (make C {f:make42/r}) o.f` → 42
// interpreted vs the fn value compiled; PR #225 P1). Zero FP risk: the out
// is the very value whose landing diverges.
func zeroArgFnOut(outs []Value) bool {
	for _, o := range outs {
		if !o.Carrier && fnValueZeroArg(o) {
			return true
		}
	}
	return false
}

// noteShapedRead mirrors a CheckState.NoteMethodShape annotation into the
// recorder: the get-family read that produced `id` resolved a shaped-
// instance member whose LANDING the shaped-method model owns, so the
// read-guard auto-dispatch refusals skip it (shapedReadOut).
// NoteDefRead records that value ID was produced by reading binding `name`
// (stepWord's simple-value substitution). Consulted by resolveOperand's
// dynamic-scope rescue when the value has no compiled home.
// NoteFrozenRead records a CONCRETE module-scope binding read that happened
// INSIDE an open fn/closure unit analysis: the value bakes into the unit
// (const / splice-fired tokens) and is frozen across calls, so a later
// module-scope rebind must refuse the program (NotifyNameRebound). No-op at
// top level, where analysis order equals program order and the bake is the
// read the interpreter makes.
func (es *EmitState) NoteFrozenRead(name string) {
	if !es.active() || name == "" || len(es.openUnitRecs) == 0 {
		return
	}
	// A read attributed to a STORED-REF unit (a service/minilang handler, a
	// spawn body) is already rebind-safe: NotifyNameRebound poisons the ref
	// itself and InvokeCallback falls back to CallAQL for just that handler,
	// keeping the rest of the program compiled (the PR #243 discipline).
	// Only ordinary CALL_USER units need the whole-program hammer.
	if rec := es.openUnitRecs[len(es.openUnitRecs)-1]; rec >= 0 && rec < len(es.fnRecs) && es.fnRecs[rec].storedRefUnit {
		return
	}
	if es.frozenReads == nil {
		es.frozenReads = map[string]bool{}
	}
	es.frozenReads[name] = true
}

// ModuleScopeBinding reports whether name's active binding sits at module /
// global scope — NOT an enclosing fn's param or body-local (the
// ComputeCaptures depth rule: Depth > baseline means enclosing-fn-local). A
// nil baseline (no enclosing fn) makes every binding module scope.
func ModuleScopeBinding(r *Registry, name string) bool {
	baseline := r.TopFnBaseline()
	if baseline == nil {
		return true
	}
	return r.Defs.Depth(name) <= baseline[name]
}

func (es *EmitState) NoteDefRead(id, name string) {
	if !es.active() || id == "" || name == "" {
		return
	}
	if es.defReads == nil {
		es.defReads = map[string]string{}
	}
	es.defReads[id] = name
}

// RecordDynBind records a value-def site (name + the bound value's operand)
// as an evDynBind event. Every def records one cheaply; the lowering emits a
// registry-visible OpBindDynScope ONLY for names in dynScopeNames (some
// OpLookupDynScope reads them) and skips the event otherwise. Engine-internal
// and capitalised (type) names never bind dynamically.
func (es *EmitState) RecordDynBind(name string, v Value, pos SrcPos) {
	if !es.active() || name == "" || name[0] == '_' || name[0] == '$' || IsCapitalisedName(name) {
		return
	}
	src, srcSeq := emitOperand{}, -1
	if pr, ok := es.producedBy[v.ID]; ok && pr.idx == 0 {
		srcSeq = pr.seq
	} else if slot, ok := es.units[len(es.units)-1].localByID[v.ID]; ok {
		src = localOperand(slot)
	}
	es.appendEvent(emitEvent{kind: evDynBind, dyn: &emitDynBind{
		name: name, src: src, srcSeq: srcSeq, val: v, pos: pos,
	}})
}

// dynScopeRescue is resolveOperand's last resort inside a fn unit: a value
// that was READ from a def binding (NoteDefRead) but has no compiled home is
// a DYNAMIC-SCOPE reference — the interpreter resolves the name against the
// live def stack at run time (a callee reading the caller's param, a
// recursive base case reading the previous frame's body-local). Lower it to
// a runtime OpLookupDynScope, gated on the check's binder/call-graph model
// (dynamicScopeReachable): some fn that BINDS the name must reach the
// reading fn, so a plain typo still refuses. The name joins dynScopeNames so
// Finalize installs the OpBindDynScope twin in every binding unit.
func (es *EmitState) dynScopeRescue(v Value) (emitOperand, bool) {
	if es.reg == nil || len(es.units) <= 1 {
		return emitOperand{}, false
	}
	name := v.DynFrom()
	if name == "" {
		name = es.defReads[v.ID]
	}
	if name == "" {
		return emitOperand{}, false
	}
	c := es.reg.Check
	reader := ""
	if n := len(es.unitNames); n > 0 {
		reader = es.unitNames[n-1]
	}
	if reader == "" {
		if n := len(c.FnNameStack); n > 0 {
			reader = c.FnNameStack[n-1]
		}
	}
	if !c.dynamicScopeReachable(name, reader) {
		return emitOperand{}, false
	}
	if es.dynScopeNames == nil {
		es.dynScopeNames = map[string]bool{}
	}
	es.dynScopeNames[name] = true
	return dynScopeOperand(es.intern(NewString(name))), true
}

func (es *EmitState) noteShapedRead(id string) {
	if !es.active() || id == "" {
		return
	}
	if es.shapedReads == nil {
		es.shapedReads = map[string]bool{}
	}
	es.shapedReads[id] = true
}

// shapedReadOut reports whether any dispatch out is an annotated shaped
// method read — the landing model's responsibility, not the read guard's.
func (es *EmitState) shapedReadOut(outs []Value) bool {
	if len(es.shapedReads) == 0 {
		return false
	}
	for _, o := range outs {
		if es.shapedReads[o.ID] {
			return true
		}
	}
	return false
}

// noteMemberFnRead records that the value with id came from a get-family read
// of a fn-valued container member. Lazily-inits the side table; nil-safe and
// active-gated (a suspended / uncompilable pass records nothing).
func (es *EmitState) noteMemberFnRead(id string) {
	if !es.active() || id == "" {
		return
	}
	if es.memberFnReads == nil {
		es.memberFnReads = map[string]bool{}
	}
	es.memberFnReads[id] = true
}

// memberFnRead reports whether id was tagged by noteMemberFnRead.
func (es *EmitState) memberFnRead(id string) bool {
	return es != nil && es.memberFnReads != nil && es.memberFnReads[id]
}

// readsFnMember reports whether a get-family dispatch surfaces a FUNCTION-valued
// member from a concrete container — the member the interpreter auto-applies
// the moment a value lands on it (design/EDGE-SPEC-FINDINGS.0.md §2). Unlike
// containerFnAutoDispatchRisk (which gates on GENUINE 0-param fns only, so the
// statement-tail apply `m.double 21` keeps compiling), this reports ANY arity:
// the caller tags the read so a MID-EXPRESSION stranded apply can refuse while
// the tail apply is unaffected. Mirrors containerFnAutoDispatchRisk's
// container/key walk (map member, list element, flat-instance field).
func readsFnMember(args []Value) bool {
	isFn := func(v Value) bool { _, ok := v.Data.(FnDefInfo); return ok }
	for _, a := range args {
		if a.Carrier {
			continue
		}
		switch d := a.Data.(type) {
		case ListPayload:
			if idx, ok := concreteIntKey(args); ok {
				if idx >= 0 && idx < int64(len(d.Elems)) && isFn(d.Elems[idx]) {
					return true
				}
				continue
			}
			for _, e := range d.Elems {
				if isFn(e) {
					return true
				}
			}
		case MapPayload:
			if d.M == nil {
				continue
			}
			if key, ok := concreteMapKey(args); ok {
				if mv, hit := d.M.Get(key); hit && isFn(mv) {
					return true
				}
				continue
			}
			for _, k := range d.M.Keys() {
				mv, _ := d.M.Get(k)
				if isFn(mv) {
					return true
				}
			}
		default:
			fields, _, isInst := flatInstanceParts(a)
			if !isInst || fields == nil {
				continue
			}
			if key, ok := concreteMapKey(args); ok {
				if mv, hit := fields.Get(key); hit && isFn(mv) {
					return true
				}
				continue
			}
			for _, k := range fields.Keys() {
				mv, _ := fields.Get(k)
				if isFn(mv) {
					return true
				}
			}
		}
	}
	return false
}

func containerFnAutoDispatchRisk(args []Value) bool {
	for _, a := range args {
		if a.Carrier {
			continue
		}
		switch d := a.Data.(type) {
		case ListPayload:
			if idx, ok := concreteIntKey(args); ok {
				if idx >= 0 && idx < int64(len(d.Elems)) && fnValueZeroArg(d.Elems[idx]) {
					return true
				}
				continue
			}
			for _, e := range d.Elems {
				if fnValueZeroArg(e) {
					return true
				}
			}
		case MapPayload:
			if d.M == nil {
				continue
			}
			if key, ok := concreteMapKey(args); ok {
				if mv, hit := d.M.Get(key); hit && fnValueZeroArg(mv) {
					return true
				}
				continue
			}
			for _, k := range d.M.Keys() {
				mv, _ := d.M.Get(k)
				if fnValueZeroArg(mv) {
					return true
				}
			}
		default:
			// Flat instances (class / Resource) expose FIELD reads through
			// the same get family, and the interpreter auto-dispatches a
			// landed fn field exactly like a map member (probe-verified:
			// `def C class {f:Function} def o (make C {f:make42/r}) o.f`
			// -> 42 interpreted, `fn make42` compiled — PR #225 P1).
			// Same key-precision rule as MapPayload.
			fields, _, isInst := flatInstanceParts(a)
			if !isInst || fields == nil {
				continue
			}
			if key, ok := concreteMapKey(args); ok {
				if mv, hit := fields.Get(key); hit && fnValueZeroArg(mv) {
					return true
				}
				continue
			}
			for _, k := range fields.Keys() {
				mv, _ := fields.Get(k)
				if fnValueZeroArg(mv) {
					return true
				}
			}
		}
	}
	return false
}

// fnValueZeroArg reports whether v is a function VALUE with a GENUINE
// zero-param overload — the shape the interpreter auto-dispatches the moment
// it lands with no operands (containerFnAutoDispatchRisk). A parked user fn
// structurally carries ONE phantom 0-arg Signature alongside its declared
// overloads (probe-verified: 0-param make42 parks as [0 0], 2-param cmp2 as
// [2 0], a 0+1-overload fn as [1 0 0], while builtins park phantom-free —
// `add` is [3 2 2 2 2]). So a genuine 0-param overload shows as EITHER two
// 0-arg sigs (real + phantom) OR a single sig that is 0-arg (phantom-free
// value). A lone 0-arg sig among others is just the phantom — the fn needs
// args, reads as data in both engines, and the applied member-call shapes
// (`m.b 2`, pinned by TestEmitFnValueFieldCallCompiles) keep compiling.
func fnValueZeroArg(v Value) bool {
	d, ok := v.Data.(FnDefInfo)
	if !ok {
		return false
	}
	zeros := 0
	for i := range d.Signatures {
		if d.Signatures[i].TotalArgs() == 0 {
			zeros++
		}
	}
	return zeros >= 2 || (zeros == 1 && len(d.Signatures) == 1)
}

// concreteMapKey returns the map-key string a get-family dispatch reads,
// when one of the args is a concrete Atom or String key.
func concreteMapKey(args []Value) (string, bool) {
	for _, a := range args {
		if a.Carrier {
			continue
		}
		switch k := a.Data.(type) {
		case AtomPayload:
			return k.Name, true
		case StrPayload:
			return k.S, true
		}
	}
	return "", false
}

// concreteIntKey returns the list index a get-family dispatch reads, when
// one of the args is a concrete Integer key.
func concreteIntKey(args []Value) (int64, bool) {
	for _, a := range args {
		if a.Carrier {
			continue
		}
		if k, ok := a.Data.(IntPayload); ok {
			return k.N, true
		}
	}
	return 0, false
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

// isGetFamilyWord reports whether word is a container member accessor
// (get/dot and their r-variants) — the words whose read of a FUNCTION value
// auto-dispatches it in the interpreter (recordCallRefusal).
func isGetFamilyWord(w string) bool {
	return w == "get" || w == "dot" || w == "getr" || w == "dotr"
}

// producerReturnedClosureArity resolves the declared arity of the closure a
// leading fn-typed CARRIER holds, when its producer is a compiled user call
// whose fn returns exactly one anonymous closure (the factory pattern).
// Recoverable arity lets resolveDynamicApply distinguish the single N-arg
// apply `(mk 5) 10 20` from a curried CHAIN `((mk 1) 2) 3` — the flattened
// residual is identical for both, and committing one OpCallDynamic over a
// chain leaks the intermediate closure (miscompile mechanism E,
// nested-factory apply, design/MISCOMPILE-HUNT-FINDINGS.0.md).
func (es *EmitState) producerReturnedClosureArity(id string) (int, bool) {
	pr, ok := es.producedBy[id]
	if !ok {
		return 0, false
	}
	for _, fr := range es.frames {
		for i := range fr {
			if fr[i].seq != pr.seq {
				continue
			}
			if fr[i].kind != evCallUser {
				return 0, false
			}
			unit := fr[i].uc.unit
			if unit < 0 || unit >= len(es.fnRecs) {
				return 0, false
			}
			rec := es.fnRecs[unit]
			if len(rec.outOps) != 1 || rec.outOps[0].kind != opClosure {
				return 0, false
			}
			cu := rec.outOps[0].closureUnit
			if cu < 0 || cu >= len(es.fnRecs) {
				return 0, false
			}
			return es.fnRecs[cu].nParams, true
		}
	}
	return 0, false
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
	return es.recordMakeListInner(r, ins, out, pos)
}

// recordMakeListInner is the guard-free core of RecordMakeList: it resolves the
// element operands and appends the OpMakeList event. RecordMakeList wraps it with
// the top-frame restriction (a fn-body residual list must fall back). The other
// caller is RecordMakeMap, for a LIST-valued map entry (`{n:[expr]}`) whose list
// is itself a CONSUMED-in-frame operand of the enclosing OpMakeMap — there the
// top-frame guard does NOT apply (OpMakeList re-assembles from its operands per
// run, exactly like OpMakeMap, so it is sound in a fn body / branch / loop).
func (es *EmitState) recordMakeListInner(r *Registry, ins []Value, out Value, pos SrcPos) bool {
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
		// A non-builtin (user-fn / module) producing word is NOT refused: the element
		// resolves to its recorded EVENT (CALL_USER / CALL_NATIVE_POLY / make), and
		// the OpMakeList assembly RE-RUNS that event at runtime — it never freezes the
		// check-mode result. So a list of user-fn / module results (`def specs
		// [(Test.test …) …]`, the voxgig spec-list pattern) assembles faithfully,
		// exactly as `make` instances and builtin results already do. A genuinely
		// stateful generator (`list-of [Rand.int] N`) is a NoEval CODE BODY run per
		// iteration, not a list-literal element, so it never reaches recordMakeListInner.
		// resolveOperand only ever yields a re-running event or an inert const here
		// (never a frozen module result), so the assembly stays sound; gated by the
		// bytecode differential + the voxgig --compile==interpret sweep.
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

// RecordInterp records the assembly of a template string whose holes are
// computed — “ `got ${x}` “, “ `n=${1 add 2}` “, “ `t=${typeof x}` “ —
// into an OpInterp dispatch. The hole expressions ran in evalInterpParts (their
// own dispatches already recorded their events); holeVals are the per-hole
// result values in source order, and out is the resulting String. Each hole
// resolves like a call operand (an event result, a frame local, a bare-type
// node, or an inert const); the literal segments ride in the Program's
// InterpSpec, so OpInterp only pops the hole VALUES — exactly the MakeMap shape.
//
// Like OpMakeMap (and unlike the const-baking OpMakeList), OpInterp re-assembles
// from its operands on every run, so it is sound inside a fn body / branch arm /
// loop — no top-frame restriction. Returns false, leaving es untouched (the
// caller then marks the program uncompilable and it falls back), when a hole has
// no compiled home or did not produce exactly one value.
func (es *EmitState) RecordInterp(parts []InterpPart, holeVals []Value, out Value, pos SrcPos) bool {
	if !es.active() || len(holeVals) == 0 {
		return false
	}
	// ops in stack order (ops[0] = top): OpInterp pops the run and reads it
	// deepest-first as hole 0, so reverse like RecordMakeMap — ops[0] is the LAST
	// hole (on top), ops[N-1] the first (deepest).
	ops := make([]emitOperand, len(holeVals))
	for k := range holeVals {
		op, ok := es.resolveOperand(holeVals[k])
		if !ok {
			return false
		}
		ops[len(holeVals)-1-k] = op
	}
	segs := make([]InterpSeg, 0, len(parts))
	for _, p := range parts {
		if p.Expr == nil {
			segs = append(segs, InterpSeg{Lit: p.Lit})
		} else {
			segs = append(segs, InterpSeg{Hole: true})
		}
	}
	es.SiteCounts[SiteMono]++
	seq := es.appendEvent(emitEvent{kind: evCall, call: emitCall{
		word: wordInterp, ops: ops, nout: 1, pos: pos,
		interp: true, interpSegs: segs,
	}})
	es.setProduced(out, seq)
	return true
}

// RecordClosureCall records a higher-order word's dispatch where the code BODY
// at position bodyPos was compiled to closure unit `unit` (plan P2). The body
// operand lowers to OpPushClosure (the handler invokes it through the VM via
// the InvokeBody seam); the other operands resolve normally, except any slot
// in extraOps (an extra LAMBDA hook — walk's ASCEND slot, Stage M2d — compiled
// to its own closure unit by recordClosureDispatch), which rides its prepared
// opClosure operand. Returns false, leaving es UNTOUCHED, when an operand is
// dynamic or of unknown provenance — the caller then keeps the island path.
func (es *EmitState) RecordClosureCall(word string, sig *Signature, args []Value, bodyPos, unit int, capOps []emitOperand, extraOps map[int]emitOperand, outs []Value, pos SrcPos) bool {
	// A whole-residual word (CallableSpec.BodyOutResidual — `do`) may seat
	// N > 1 results: recordClosureDispatch has already asserted the unit's
	// compiled residual count equals len(outs), and the multi-result seating
	// below mirrors the generic RecordCall tail. Other callers stay 0/1-out.
	if !es.active() || sig == nil ||
		(len(outs) > 1 && (sig.Callable == nil || sig.Callable.BodyOut != BodyOutResidual)) {
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
		if exOp, isExtra := extraOps[i]; isExtra {
			ops[i] = exOp
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
	for i := range outs {
		es.setProducedAt(outs[i], seq, i)
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

// noEvalBodiesInertScoped is noEvalBodiesInert plus a MODULE-SCOPE allowance for
// InterpString-bearing bodies. A re-interpreting NoEvalArgs handler (the
// property harness's test-check-prop, Callable==nil) bakes its body as data and
// re-runs it in a sub-engine against the registry; an InterpString `${name}`
// there resolves against the registry exactly as the interpreter does — SOUND
// only when there is no enclosing compiled fn frame (len(es.units)==1). Inside a
// fn frame a `${frame-local}` would resolve against the registry instead of the
// VM slot and diverge (`def f fn [[pfx] … check-prop … `${pfx}` …]`), so there
// the strict isInertConst path keeps the body refused → sound interpreter
// fallback. (The same fn-frame hazard already exists for a bare-Word/paren
// member; this widening deliberately does NOT extend it — it admits InterpString
// only where every name resolves through the registry.)
func (es *EmitState) noEvalBodiesInertScoped(sig *Signature, args []Value) bool {
	moduleScope := len(es.units) == 1
	for i := range args {
		if !sig.NoEvalArgs[i] {
			continue
		}
		inert := isInertConst(args[i])
		if !inert && moduleScope {
			inert = interpBodyInert(args[i])
		}
		if !inert {
			return false
		}
		if bodyHasSentinel(args[i]) {
			return false
		}
	}
	return true
}

// dynInputsProven reports whether a dynamic-operand dispatch is nonetheless a
// PROVEN sig match — safe to bake a plain CALL_NATIVE despite the dynamic
// carriers — for a word that runs its bodies in ISOLATED CallAQL frames
// (CompileRunsBodyIsolated, i.e. Test.check-prop). The dynamic-input refusal
// defends against a WIDENED sig: a gradual-Any operand (Parent=Any) matched a
// concrete sig position only by the Any→anything rule, so the runtime value
// could violate it and the compiled CALL_NATIVE would not raise where the
// interpreter's re-match would. That widening is visible as a bare-Any carrier.
// A dynamic operand whose CONCRETE Parent type strictly conforms to its sig
// position matched by REAL type membership, not widening — it is a runtime-
// valued value of a statically PROVEN type (e.g. `p get "runs"` over a
// `p:PropertySpec` param, an Integer guaranteed by the record contract the VM's
// guarded CALL_USER enforces at run-property's entry). So the runtime value
// conforms, the VM invokes the same single sig the interpreter matches, and the
// bodies run through the same captured-parent handler in both modes. The gate is
// deliberately strict: EVERY operand must conform by a non-Any concrete type, so
// a single genuinely-widened (Any) operand keeps the whole call refused.
func (es *EmitState) dynInputsProven(sig *Signature, args []Value) bool {
	if sig == nil || !sig.CompileEffect.Has(CompileRunsBodyIsolated) {
		return false
	}
	sigArgs := sig.ArgTypes()
	if len(sigArgs) != len(args) {
		return false
	}
	for i, a := range args {
		if !a.Dynamic {
			continue
		}
		// A widened gradual-Any operand: Parent conforms to nothing concrete,
		// so this is exactly the unproven case the guard defends — refuse.
		if a.Parent == nil || a.Parent.Equal(TAny) {
			return false
		}
		pt := sigArgs[i]
		if pt == nil {
			return false
		}
		if !a.Parent.ConformsTo(pt) {
			return false
		}
	}
	return true
}

// interpBodyInert is isInertConst widened so a NESTED InterpString counts as
// inert. Its TOP-LEVEL semantics match isInertConst EXACTLY — a standalone
// ParenExpr / Word / InterpString is NOT bakeable data (a standalone ParenExpr
// is deferred code; baking `(loopy 1)` would wrongly let a nested too-deep
// macroexpand compile) — so only a List/Map at the top recurses through the
// InterpString-admitting member check; everything else defers to isInertConst.
// Only noEvalBodiesInertScoped (and resolveOperand) call this, and only at
// MODULE scope — an InterpString must NEVER bake inside a compiled fn frame.
func interpBodyInert(v Value) bool {
	switch v.Data.(type) {
	case ListPayload, MapPayload:
		return interpMemberInert(v)
	default:
		return isInertConst(v)
	}
}

// interpMemberInert is isInertConstMember widened to admit an InterpString whose
// hole expressions are themselves inert members — immutable code-as-data the VM
// pushes verbatim and a re-interpreting handler runs against the registry.
// Container members (List/Map/ParenExpr) recurse HERE so a nested InterpString
// at any depth is reached; every other leaf defers to the strict
// isInertConstMember so leaf soundness (mutable-instance exclusion, canonical
// type pointers) stays single-sourced.
func interpMemberInert(v Value) bool {
	if v.Carrier || v.Dynamic {
		return false
	}
	if IsInterpString(v) {
		parts, err := AsInterpString(v)
		if err != nil {
			return false
		}
		for _, p := range parts {
			for _, tk := range p.Expr {
				if !interpMemberInert(tk) {
					return false
				}
			}
		}
		return true
	}
	switch d := v.Data.(type) {
	case ListPayload:
		for _, e := range d.Elems {
			if !interpMemberInert(e) {
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
			if !interpMemberInert(mv) {
				return false
			}
		}
		return true
	case ParenExprPayload:
		for _, tk := range d.Toks {
			if !interpMemberInert(tk) {
				return false
			}
		}
		return true
	default:
		return isInertConstMember(v)
	}
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

// internUnpooled appends a const WITHOUT registering it in the canonical
// pool: a dynamic-scope bind's value must never merge with (or be merged
// into by) a source literal of the same canon — the bind may carry a
// reparented tag the literal must not inherit. Lowering-time only.
func (es *EmitState) internUnpooled(v Value) int {
	es.consts = append(es.consts, v)
	return len(es.consts) - 1
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
		// CanonValue-deduped: like the list/map identity rule, two source
		// codequotes stay two distinct const values rather than merging into one.
		// The SAME materialised value (same non-empty ID) is one logical
		// instance though — its payload pointer is already identity-aliased —
		// so it pools by ID: semantics-preserving, and it keeps
		// freshenFnUnitConsts' push-site counting truthful (two reads of one
		// binding count as pushes of ONE const, never two slots).
		if v.ID != "" {
			if i, ok := es.constIDIdx[v.ID]; ok {
				return i
			}
			es.consts = append(es.consts, v)
			if es.constIDIdx == nil {
				es.constIDIdx = map[string]int{}
			}
			es.constIDIdx[v.ID] = len(es.consts) - 1
			return len(es.consts) - 1
		}
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
// class-schema field default — an Object/Store/flex value. Admitting one
// as a SCHEMA member (ONLY through typeBodyConstOK's memberOK, never standalone)
// is mutation-safe precisely because every `make` freshens it into its own copy;
// outside a type body nothing freshens it, so isInertConst keeps it out of the
// const pool. Pairs with the const-bake regression gate.
func isFreshenedInstance(v Value) bool {
	return IsClassInstance(v) || IsStore(v) || IsFlexList(v)
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
	case ClassTypeInfo:
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
	case MicronTypeInfo:
		// A Micron type body (`refine Micron {fields}`): field schema
		// like a record's — const-safe when every constraint/default is
		// inert. The canonical *Type rides the payload pointer.
		return allFieldsInert(d.Fields, memberOK)
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
// freshenableConst reports whether a value belongs to the compound
// VALUE-literal class whose interpreter evaluation CONSTRUCTS a fresh
// instance per evaluation, making per-call container identity observable
// through `eq` (miscompile mechanism A,
// design/MISCOMPILE-HUNT-FINDINGS.0.md §A). That is ListPayload and
// MapPayload — sameContainer (compare.go) identifies them by backing array /
// *OrderedMap pointer, and CloneValue mints both fresh. Everything else
// stays shared: scalars and Microns compare by value; type bodies, fn
// values, and surfaces are identity-inert descriptors; XML literals are
// parse-time constants in the INTERPRETER too (the same element instance is
// spliced per call — probe-verified `(mk) eq (mk)` → true interpreted), so
// sharing the pooled const IS parity there. The empty-list case needs no
// exclusion: sameContainer treats all empty lists as the single empty list,
// so interp (fresh empties, eq true) and VM (cloned empty, eq true) agree.
// Consumed by the resolveOperand fn-unit marking (freshenConst) and the
// enclosing-binding snapshot (snapshotCompoundBindingIDs).
func freshenableConst(v Value) bool {
	if v.Carrier || v.Dynamic {
		return false
	}
	switch v.Data.(type) {
	case ListPayload, MapPayload:
		return true
	}
	return false
}

func isInertConst(v Value) bool {
	if v.Carrier || v.Dynamic || IsBareTypeNode(v) {
		return false
	}
	switch d := v.Data.(type) {
	case IntPayload, FloatPayload, StrPayload, BoolPayload, AtomPayload,
		PathonPayload, NonePayload, BigIntPayload, DecimalPayload,
		TimePayload, DurationPayload, TimezonePayload:
		return true
	case MicronPayload:
		// A Micron instance (Emailon / Urlon / user kind) is inert ONLY
		// because the family is immutable — `set` on a Micron is an
		// explicit error (the erroring set signatures in the language
		// layer). The payload's OrderedMap is pointer-backed, so if
		// mutation were ever allowed, pooled consts would corrupt
		// across loop iterations and this arm must be removed.
		return true
	case ExtensionPayload:
		// The kernel cannot see into an extension payload, so mutability
		// is the OWNING TYPE's call: bake only when its Behavior declares
		// the values immutable (the ConstBakeable capability — Bytes),
		// reached by the same parent-chain walk every capability dispatch
		// uses. Extension types without the capability — Socket, Listener,
		// Timeout/Interval, Module instances, every plugin type that never
		// opted in — keep the historical refusal. This is what lets a
		// module-scope `def crlf (convert Bytes "\r\n")` bake into a
		// stored-fn unit (mini-s3's recv-until delimiter) with freshness
		// still owned by the existing frozen-read / dep-snapshot gates.
		for t := v.Parent; t != nil; t = t.Parent {
			if cb, ok := t.Behavior().(ConstBakeable); ok {
				return cb.BakeableConst(v)
			}
		}
		return false
	case MicronTypeInfo:
		// A Micron type body (`refine Micron {fields}`): structural
		// descriptor like the RecordTypeInfo arm below — sound when its
		// field map is carrier-free.
		return typeBodyConstOK(v)
	case DepScalarInfo:
		// A predicate / refinement type (`Integer gt 10`): self-contained
		// (base family + bound, no registry, no canonical-pointer hazard per
		// eng CLAUDE.md), so the value bakes by value into the const pool.
		// The bound is recovered for a stripped operand via origByID
		// (RememberOriginal at the constructor); type-algebra words
		// (tcmp/teq/tand/…) then run over the baked predicate at run time.
		return true
	case RecordTypeInfo, OptionsTypeInfo, ChildTypeInfo, DisjunctInfo, ClassTypeInfo, TableTypeInfo:
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
		// A module-export fn value bakes as DATA — a bare residual (`MathUtil.sqrt`),
		// a branch-arm operand, a container member, OR a comparator passed to another
		// fn (`xs M.sort M.by-num`). The sub-registry pointer it carries is the SAME
		// object the compiled run shares (RunProgram runs on the check-pass
		// registry), so the baked const and the live fn are one object — no
		// check/VM value divergence. The VM applies it faithfully at run time:
		//   - a TRIVIAL-DELEGATION wrapper (`MathUtil.sqrt`, every own sig a
		//     `[Word(inner)]` pass-through) via callDynamic → tryNativeFnApply, which
		//     re-resolves the inner native in that registry;
		//   - a REAL AQL body via the island sub-engine (callDynTrailTop/…'s
		//     `vc.island().Run([fn, args…])`), which INTERPRETS the fn in
		//     fnDef.Registry — CallAQL, module-private scope and all. So a real body
		//     applies soundly too (compile == interpret, verified). A macro stays
		//     refused (applied only by name / compile-time expansion, never as data).
		return !d.Macro
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
	// from data) with every following arg static. An ANNOTATED method-read
	// carrier is excluded: the statement-window model (method_shape.go) owns
	// its apply, and a decline there means the residual tail may cross the
	// value's statement boundary — see methodShapeAnnotated.
	applyDynamic := false
	if len(residual) >= 2 && residual[0].Dynamic && !es.methodShapeAnnotated(residual[0].ID) {
		applyDynamic = !anyDynamicTail(residual)
	}
	// Leading Function/FnDef CARRIER (the factory pattern: a returned closure now
	// on the stack) with no dynamic / fn value after it. A carrier always applies
	// — the one non-applied shape (an inert `f/r`) is a CONCRETE const, not a
	// carrier — so the carrier bit resolves the apply-vs-inert ambiguity.
	if !applyDynamic && len(residual) >= 2 && isFnTypedCarrier(residual[0]) {
		applyDynamic = !anyFnOrDynamicTail(residual)
		// When the carrier's closure arity is statically recoverable (its
		// producer is a compiled factory fn returning one anonymous closure),
		// a tail-arg count that disagrees is a curried CHAIN (`((mk 1) 2) 3`)
		// or a partial apply — one OpCallDynamic would leak the intermediate
		// closure (miscompile mechanism E). Refuse; the interpreter applies
		// the chain faithfully.
		if applyDynamic {
			if arity, known := es.producerReturnedClosureArity(residual[0].ID); known && arity != len(residual)-1 {
				return residual, 0, "fn-value apply arity mismatch — curried chain or partial apply (Stage 3)"
			}
		}
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
	// TRAILING window (Stage M2b): the single dynamic / fn value is LAST over
	// ≥2 static args (`10 3 m.s/s` → residual [10, 3, wrapper]). The verbatim
	// window island reproduces the interpreter exactly — the fn value lands ON
	// TOP of an already-populated stack and dispatches under its OWN barrier
	// discipline (a stack-args wrapper is BarrierPos 0 and collects nothing
	// forward, so the trailing-1 rotation contract of OpCallDynamicTrailing /
	// OpCallDynTrailTop cannot express it), and a non-callable value simply
	// stays put. The producing event was promoted to a frame local (Finalize's
	// gate below), so the whole window re-pushes in source order. The 2-entry
	// trailing shape stays with trailingApply above — landed and pinned.
	if es.trailingWindowApplyShape(residual) {
		return residual, OpCallDynamicMixed, ""
	}
	// Unhandled: a dynamic value mid-residual, a fn value preceding args, or an
	// unconsumed fn-value carrier (a VM closure renders unlike the interpreter's
	// FnDefInfo). All refuse so the program falls back faithfully — EXCEPT a
	// dynamic whose static bound provably excludes every callable (the
	// sigTypeMatches not-disjoint rule against Function/FnDef): such a value
	// can never auto-apply to the values above it, so both engines leave it
	// as data and the residual renders identically (a narrowed flex-shape
	// read — dynamic(FlexMap) — sitting under later statement results,
	// edge-containers-2.tsv:76).
	for i := 0; i+1 < len(residual); i++ {
		if residual[i].Dynamic &&
			(sigTypeMatches(residual[i], TFunction) || sigTypeMatches(residual[i], TFnDef)) {
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
	if len(lw.vm) < 1 || lw.vm[len(lw.vm)-1].seq != pr.seq || lw.vm[len(lw.vm)-1].idx != 0 { //covergate:allow compiler/VM defensive arm; unreachable without a bytecode-level fault (§compiler)
		return residual, false
	}
	arg := residual[0]
	if _, argIsEvent := es.producedBy[arg.ID]; argIsEvent || arg.Dynamic || isFnValueResidual(arg) {
		return residual, false
	}
	return []Value{fnv, arg}, true
}

// trailingWindowApplyShape detects the TRAILING fn-value window (Stage M2b):
// EXACTLY one residual entry is a dynamic / fn value, it is the LAST entry with
// ≥2 entries below it, and it is event-produced (so Finalize can promote the
// producer to a frame local and re-push the whole window in source order for
// OpCallDynamicMixed). The 2-entry trailing shape is deliberately excluded —
// that is trailingApply's landed rotation contract (OpCallDynamicTrailing).
func (es *EmitState) trailingWindowApplyShape(residual []Value) bool {
	if len(residual) < 3 {
		return false
	}
	last := len(residual) - 1
	for i, rv := range residual {
		if (rv.Dynamic || isFnValueResidual(rv)) != (i == last) {
			return false
		}
	}
	pr, ok := es.producedBy[residual[last].ID]
	return ok && pr.idx == 0
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
	// An ANNOTATED method-read carrier declines the mixed island too: its
	// after-args are the statement-window model's territory, and the island's
	// forward collection would cross the value's statement End boundary
	// (`3 c.add 2 ; {a:1} …` — the island could bind the NEXT statement's map
	// as the method's 2-arg overload's field arg). See methodShapeAnnotated.
	if es.methodShapeAnnotated(residual[dynIdx].ID) {
		return 0, false
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
// eventsBindDynScope reports whether any event in the slice (recursing into
// branch arms, list-form conditions, and loop bodies) records a def of a name
// in `names` — the unit then installs registry-visible bindings and must not
// TAIL-call (the interpreter keeps a frame's bindings live across the calls
// in its body tail; a tail replacement would tear them down early).
func eventsBindDynScope(events []emitEvent, names map[string]bool) bool {
	if len(names) == 0 {
		return false
	}
	var frag func(f *EmitFragment) bool
	walk := func(evs []emitEvent) bool {
		for i := range evs {
			ev := &evs[i]
			switch ev.kind {
			case evDynBind:
				if names[ev.dyn.name] {
					return true
				}
			case evBranch:
				if frag(ev.br.condFrag) || frag(ev.br.then) || frag(ev.br.els) {
					return true
				}
			case evLoop:
				if frag(ev.loop.body) {
					return true
				}
			}
		}
		return false
	}
	frag = func(f *EmitFragment) bool {
		if f == nil {
			return false
		}
		return walk(f.events)
	}
	return walk(events)
}

// unitBindsDynScope reports whether a unit installs dynamic-scope bindings:
// a body-local def of a dyn-read name (evDynBind in its fragment tree), or a
// PARAM some fn reads dynamically.
func (es *EmitState) unitBindsDynScope(rec *fnUnitRec) bool {
	// DynEnv mode: every unit with a NAMED param installs bindings at entry
	// (emitDynParamBinds widens to all names), so tail calls are disabled
	// exactly as for a targeted dyn-bound param.
	if es.dynEnv {
		for i := 0; i < rec.nParams && i < len(rec.locals); i++ {
			if rec.locals[i] != "" {
				return true
			}
		}
	}
	if eventsBindDynScope(rec.frag.events, es.dynScopeNames) {
		return true
	}
	if len(es.dynScopeNames) == 0 {
		return false
	}
	for i := 0; i < rec.nParams && i < len(rec.locals); i++ {
		if es.dynScopeNames[rec.locals[i]] {
			return true
		}
	}
	return false
}

// emitDynParamBinds installs a unit's dynamically-read PARAMS into r.Defs at
// frame entry (a callee reading the caller's `n` — recursion.tsv:72), exactly
// where the interpreter's InstallFrameBinding makes them visible; the frame's
// RET truncates them back.
func (es *EmitState) emitDynParamBinds(flw *lowerer, rec *fnUnitRec) {
	if len(es.dynScopeNames) == 0 && !es.dynEnv {
		return
	}
	for i := 0; i < rec.nParams && i < len(rec.locals); i++ {
		// DynEnv mode: every NAMED param dyn-binds (the interpreter's
		// InstallFrameBinding makes all of them registry-visible; a dynamic
		// code body may read any). Unnamed slots have no name to bind.
		if rec.locals[i] == "" || (!es.dynEnv && !es.dynScopeNames[rec.locals[i]]) {
			continue
		}
		flw.emit(OpPushLocal, i, rec.pos)
		flw.emit(OpBindDynScope, es.internUnpooled(NewString(rec.locals[i])), rec.pos)
		flw.vm = append(flw.vm, nonEventSlot)
		flw.note()
		flw.vm = flw.vm[:len(flw.vm)-1]
	}
}

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
	p := &Program{DynEnv: es.dynEnv}
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
	// MIXED fn-value-call boundary (`3 m.f 2`) and its TRAILING-window sibling
	// (`10 3 m.s/s`, Stage M2b): the fn sits above a before-arg literal, so the
	// in-order reconciliation would refuse "result above a literal". Promote
	// EVERY residual event to a frame local so the whole window (before-args,
	// the fn, any after-args) re-pushes in source order, ready for
	// OpCallDynamicMixed to island it. (residualHasFnOrDynamic suppressed the
	// reorder block above; these are the fn-value shapes that DO reorder, since
	// the island consumes the window in source order, not the apply-on-top
	// layout.)
	_, mixedOK := es.mixedDynamicApplyShape(residual)
	if mixedOK || es.trailingWindowApplyShape(residual) {
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
				// slot is the producer's BASE slot; output idx i lives at slot+i
				// (multi-output stack words store one slot per result).
				ops = append(ops, localOperand(slot+pr.idx))
				continue
			}
			ops = append(ops, eventOperand(pr.seq, pr.idx))
			continue
		}
		// A frame-0 local (a loop-carried def's slot — the post-loop read of a
		// module-scope rebind resolves the joined binding to its cell). Events
		// first, mirroring resolveOperand's precedence.
		if slot, okLoc := es.units[0].localByID[rv.ID]; okLoc {
			ops = append(ops, localOperand(slot))
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
	// A trap-truncated program seats nothing: the residual was dropped above,
	// and results the kept event prefix leaves on the stack are legitimately
	// dangling — the interpreter aborts at this same point with those values
	// still on its stack (`5 inc apply`: inc's result is live when apply
	// raises), and OpTrap aborts before anything could read them. Only a
	// program that RUNS TO COMPLETION owes the residual seating discipline.
	if es.trapAt == 0 {
		if reason := lw.seatResults(ops, false, false, seatMsgs{
			aboveLiteral: "residual shape beyond Stage 1 (call result above a literal)",
			reordered:    "residual shape beyond Stage 1 (call results reordered)",
			unconsumed:   "residual shape beyond Stage 1 (unconsumed call results)",
		}, lastPos); reason != "" { //covergate:allow compiler/VM defensive arm; unreachable without a bytecode-level fault (§compiler)
			return nil, reason, false
		}
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
			if es.trapAt != 0 {
				// The program ends at a terminal top-level trap, and an
				// UNFINISHED unit is unreachable from the kept prefix: a
				// CALL_USER event only records against a unit whose body
				// compile already finished, so no event at or before the trap
				// can enter this one (the unit was opened by a `def` whose
				// body compile the trapping dispatch cut short — e.g. `{f}`
				// evaluating a 1-arg fn with no args). Emit a defensive trap
				// stub so unit indices stay aligned and any future reach of
				// the stub fails loudly instead of silently returning nothing.
				ti := len(p.Traps)
				p.Traps = append(p.Traps, TrapSpec{Code: "internal_error",
					Detail: "unreachable fn unit " + rec.name + " entered after terminal trap", Word: rec.name})
				p.Fns = append(p.Fns, CompiledFn{Name: rec.name,
					Code:  []Instr{{Op: OpTrap, Arg: int32(ti)}},
					Debug: []SrcPos{rec.pos}})
				continue
			}
			return nil, "fn " + rec.name + " was never compiled", false
		}
		diverged := fragDiverges(rec.frag)
		// A unit that installs dynamic-scope bindings — a body-local def of a
		// dyn-read name, or a PARAM some other fn reads dynamically — must not
		// TAIL-call: the interpreter keeps the frame's bindings live until the
		// body's cleanup tail runs AFTER the call returns, so the compiled
		// frame must survive the call too (plain CALL_USER; RET pops the
		// bindings at the same point the interpreter's __DC does).
		bindsDyn := es.unitBindsDynScope(rec)
		if !rec.generic && !bindsDyn && len(rec.outOps) == 1 && rec.outOps[0].kind == opEvent {
			// Tail marking is single-result only: a tail call's results
			// become the fn's results wholesale, so a multi-return tail
			// boundary needs no rewrite here (it stays a plain CALL_USER).
			// Generic instantiations stay out of tail marking too — the
			// interpreter's HasGen exclusion, mirrored (plan Stage 4).
			if !es.markTailCalls(rec.frag, &rec.outOps[0], true, rec.returns) {
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
		cf := CompiledFn{Name: rec.name, NParams: rec.nParams + len(rec.caps), NArgs: rec.nParams, NCaptures: len(rec.caps), NUnnamed: rec.nUnnamed, NLocals: rec.numLoc, InShape: rec.inShape, Returns: rec.returns, Params: rec.paramTypes, ParamPatterns: rec.paramPatterns, Decl: rec.decl, LocalNames: names}
		if rec.reg != nil && rec.reg != es.progReg {
			// Stamp the unit's dispatch registry ONLY for a FOREIGN sub-registry
			// (a `module [...]` preamble fn — decision.cond, repl-eval-line):
			// its body resolves module-private words there, exactly where the
			// interpreter's CallAQL runs it, so the VM must override curReg. An
			// ORDINARY top-level fn (rec.reg == the program registry) must NOT be
			// stamped: the VM run may be handed a DIFFERENT registry
			// (ForkConcurrent gives each concurrent execution its own fork), so
			// stamping the shared compile registry would (a) defeat that
			// isolation and (b) race — every fork's CALL_NATIVE would call
			// ensureInvoker on the one shared registry, mutating its Invoker
			// field concurrently (the -race gate TestCompiledConcurrencyRaceFree).
			// progReg is the FIRST-bound registry (bindRegistry), stable across
			// the pass unlike es.reg which re-binds on every module-body / island
			// sub-engine run. A foreign sub-registry stays the correct shared
			// pointer (module bodies are not fork-parallelised in the corpus);
			// an ordinary fn falls through to curReg == vc.r (the fork).
			cf.Reg = rec.reg
		}
		flw := &lowerer{es: es, p: p, code: &cf.Code, debug: &cf.Debug, sigIdx: lw.sigIdx, variadic: map[int]bool{}, numLocals: rec.numLoc, promoted: rec.promoted, dead: rec.dead, isFnUnit: true}
		es.emitDynParamBinds(flw, rec)
		// The apply-loop replay's unnamed-param re-pushes seat at UNIT START —
		// they must sit BELOW the loop's runtime value region, which exists only
		// once the loop has run. They are runtime-only frame bottoms outside the
		// sim's model (the RET trim owns them), so the sim resets after.
		for _, op := range rec.retPrefix {
			flw.pushOperand(op, rec.pos)
		}
		flw.vm = flw.vm[:0]
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
			// The __RC unnamed-arg allowance, applied at LOWERING time: a
			// declared fn's residual bottoms that are (a) within the
			// NUnnamed window and (b) pure PARAM-LOCAL references are the
			// frame's unconsumed unnamed args — the interpreter's __RC
			// discards them, and since operands lower lazily they were
			// never emitted, so dropping them here is the trim with zero
			// runtime cost (the VM RET's NUnnamed trim remains as the
			// runtime backstop). Bottoms that are anything else keep the
			// full residual and let reconcileResults refuse as before.
			if len(rec.returns) > 0 && rec.nUnnamed > 0 && len(rec.outOps) > len(rec.returns) && !rec.retReplay {
				if extra := len(rec.outOps) - len(rec.returns); extra <= rec.nUnnamed {
					drop := 0
					for drop < extra && rec.outOps[drop].kind == opLocal && rec.outOps[drop].idx < rec.nParams {
						drop++
					}
					if drop == extra {
						rec.outOps = rec.outOps[extra:]
					}
				}
			}
			// Reconcile the body's N result operands with the simulated
			// stack and emit a RET. Event results must already sit on the
			// stack in order (they were left by their own events); inert
			// operands (const / local / type) are pushed as a trailing
			// tail above the last event result. This is the fn-unit mirror
			// of the program-residual reconciliation below.
			if reason := flw.reconcileResults(rec.outOps, "fn "+rec.name, len(rec.returns) == 0 || rec.dynTrailArity > 0 || rec.dynFrameW > 0, rec.retReplay, rec.pos); reason != "" {
				return nil, reason, false
			}
			// A paren-bounded trailing fn-value apply body: outOps were seated as the
			// full [args…, fn] (fn on top); collapse them to the one applied value with
			// OpCallDynTrailTop before the RET (the captured/param fn auto-applies to
			// its args exactly as the interpreter's paren auto-dispatch).
			if rec.dynTrailArity > 0 {
				op := OpCallDynTrailTop
				if rec.dynTrailApply {
					op = OpCallDynApplyTop // the `apply` word's unquote-then-apply
				}
				flw.emit(op, rec.dynTrailArity, rec.pos)
			}
			// A whole-frame dynamic-apply replay: outOps seated the FULL residual
			// (frame re-push prefix included); replay the top dynFrameW token-region
			// entries against it and let the RET apply the RetReplay discipline.
			if rec.dynFrameW > 0 {
				flw.emit(OpCallDynFrame, rec.dynFrameW, rec.pos)
			}
			cf.RetReplay = rec.retReplay
			flw.emit(OpRet, 0, rec.pos)
		}
		// A fully diverging body (every path tail-calls) emits no RET —
		// control leaves via the callee's eventual RET.
		if reason := freshenFnUnitConsts(&cf, es, rec); reason != "" {
			return nil, reason, false
		}
		p.Fns = append(p.Fns, cf)
	}

	lw.p.Consts = es.consts // interning may have grown during reconciliation
	lw.p.Types = es.types
	lw.p.Fallbacks = es.fallbacks
	lw.p.MaxStack = lw.maxDepth
	lw.p.NumLocals = es.units[0].numLocals
	// Back-stamp every stored-fn handler ref with the now-built *Program so a
	// callback invoked after this run returns (a serve-raw connection handler on
	// its own fork) can locate its unit. The refs are already reachable from the
	// baked consts (their *AQLImpl); the side-list keeps this O(refs) rather than
	// re-scanning Consts. Fns is fully assembled above, so every ref.Unit indexes
	// a valid entry.
	for _, ref := range es.storedFnRefs {
		if ref.poisoned {
			// A dep the body reads was undef'd/redefined after this ref was
			// created — the frozen unit is stale. Leave Prog nil so
			// InvokeCallback falls back to CallAQL (live resolution).
			continue
		}
		ref.Prog = lw.p
	}
	lw.p.storedFnRefs = es.storedFnRefs
	return lw.p, "", true
}

// noteApplyLoopReplay classifies a fn-body residual holding a per-iteration
// APPLY-LOOP's variadic value region under an inert tail (`def acc 0 for 5 […
// (args.0 1)] acc` → residual [param re-pushes…, loop values…, acc]): the
// loop's runtime count is variable, so the RET takes the replay trim
// discipline (retReplay). The param prefix seats at UNIT START (below the
// loop's region — it cannot be re-pushed after the loop has run), the loop
// event stays the seated residual, and the inert tail pushes above it as
// usual. Returns ops with the prefix split off (or unchanged when the shape
// does not fit — the normal reconciliation then refuses as before).
func (es *EmitState) noteApplyLoopReplay(rec *fnUnitRec, ops []emitOperand) []emitOperand {
	j := -1
	for i, op := range ops {
		if op.kind == opEvent && es.eventInfo[op.idx].variadicResult && es.eventInfo[op.idx].applyLoop {
			j = i
			break
		}
	}
	if j < 0 || j > rec.nUnnamed {
		return ops
	}
	for i := 0; i < j; i++ {
		if ops[i].kind != opLocal || ops[i].idx >= rec.nParams {
			return ops
		}
	}
	for i := j + 1; i < len(ops); i++ {
		if ops[i].kind == opEvent {
			return ops
		}
	}
	rec.retPrefix = ops[:j]
	rec.retReplay = true
	return ops[j:]
}

// noteDynFrameReplay scans a count-mismatched fn-body residual for an
// unapplied runtime fn VALUE beyond the frame-bottom re-push window. An
// UNNAMED PARAM's entry re-push in that window is inert DATA in both engines
// (arguments-are-inert: the interpreter never auto-fires a frame argument),
// and the RET trim discards it (NUnnamed allowance) — never an unapplied
// apply. A Function value ABOVE the window was produced at the pointer (an
// args.N fold, a paren apply), where the interpreter's execFnDefLiteral rule
// fires by RUNTIME Name/arity — statically unknowable in the recorder, so the
// WHOLE frame residual is replayed at run time (OpCallDynFrame /
// dynFrameWindow), where that same rule decides. Returns true when no such
// value exists or the replay window was seated; false keeps the refusal.
func noteDynFrameReplay(u *emitUnit, rec *fnUnitRec, vals []Value, extra int) bool {
	for i, v := range vals {
		if i < extra && i < rec.nUnnamed {
			if slot, isLocal := u.localByID[v.ID]; isLocal && slot < rec.nParams {
				continue
			}
		}
		if v.Parent != nil && (v.Parent.ConformsTo(TFunction) || v.Parent.ConformsTo(TFnDef)) {
			if w, ok := dynFrameWindow(u, rec, vals); ok && replayIsBodyTail(rec.frag, vals[len(vals)-w:]) {
				rec.dynFrameW = w
				rec.retReplay = true
				return true
			}
			return false
		}
	}
	return true
}

// replayIsBodyTail reports whether the replay window is the body's LAST
// statement in source order. The whole-frame replay fires at the RET, so any
// recorded event positioned AFTER the window would have its effects reordered
// ahead of the apply's (the interpreter runs the apply at its own source
// position: `[(args.0 args.1) print "after"]` prints the callee's output
// first) — such a body declines and falls back. Events BEFORE the window ran
// before the apply on both engines, and an event-free body is trivially a
// tail. When events exist but no window value carries a source position, the
// order cannot be proven and the replay declines.
func replayIsBodyTail(frag *EmitFragment, window []Value) bool {
	if frag == nil || len(frag.events) == 0 {
		return true
	}
	anchor := SrcPos{}
	for _, v := range window {
		if v.Pos().Row > anchor.Row || (v.Pos().Row == anchor.Row && v.Pos().Col > anchor.Col) {
			anchor = v.Pos()
		}
	}
	if anchor.Row == 0 && anchor.Col == 0 {
		return false
	}
	for i := range frag.events {
		p := eventPos(frag.events[i])
		if p.Row > anchor.Row || (p.Row == anchor.Row && p.Col > anchor.Col) {
			return false
		}
	}
	return true
}

// dynFrameWindow classifies a fn-body residual that carries an unapplied
// runtime fn value as the whole-frame dynamic-apply replay (OpCallDynFrame): a
// leading PREFIX of unnamed-param frame re-pushes (param locals, capped at
// nUnnamed — the values the interpreter's pointer never steps) under a TOKEN
// region holding everything the pointer did step, at least one entry of which
// is the fn value that could not statically dispatch. Returns the token-region
// width. The replay is faithful by construction — the region's values re-step
// under execFnDefLiteral's own runtime rule, with the prefix (and nothing
// deeper: both engines bound collection at the frame) as the stack below — so
// the only decline is a residual with no token region at all.
func dynFrameWindow(u *emitUnit, rec *fnUnitRec, vals []Value) (int, bool) {
	k := 0
	for k < len(vals) && k < rec.nUnnamed {
		slot, isLocal := u.localByID[vals[k].ID]
		if !isLocal || slot >= rec.nParams {
			break
		}
		k++
	}
	if w := len(vals) - k; w >= 1 {
		return w, true
	}
	return 0, false
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

// fnConcreteSingleValuedOrCarrier reports whether the fn VALUE v is safe to
// lower as a single-result paren-bounded apply (OpCallDynTrailTop, which nets
// EXACTLY ONE value). Two shapes pass:
//
//   - A fn-typed CARRIER (no concrete FnDefInfo payload) — a `comp:Function`
//     param/captured value whose return arity is unknown statically. The
//     comparator-apply convention relies on this still lowering, matching the
//     interpreter's single-result paren auto-dispatch; refusing it would lose
//     compiled coverage with no soundness gain.
//   - A concrete FnDefInfo whose EVERY authored overload declares exactly one
//     return type. A `fn […]`-declared callee always carries an explicit return
//     block (a single return → a length-1 Returns; a lambda → [Any]); only an
//     empty `[]` block yields nil/length-0, i.e. a genuine zero-return fn. So a
//     concrete callee with any sig of length != 1 (0 returns, or 2+ like
//     `pair → [Integer Integer]`) is NOT single-valued and the lowering is
//     unsound — underflow on zero-return, wrong-order/leftover values on
//     multi-return. Such a callee is refused so the faithful interpreter island
//     runs it.
func fnConcreteSingleValuedOrCarrier(v Value) bool {
	fd, ok := v.Data.(FnDefInfo)
	if !ok {
		return true // carrier — arity unknown; keep lowering (comparator convention)
	}
	sigs := fd.OwnSigs()
	if len(sigs) == 0 {
		return false
	}
	for i := range sigs {
		if len(sigs[i].Returns) != 1 {
			return false
		}
	}
	return true
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
	case evStore:
		return ev.store.pos
	case evDynBind:
		return ev.dyn.pos
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
