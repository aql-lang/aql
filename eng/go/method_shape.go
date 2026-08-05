package eng

// method_shape.go — the shaped-instance-method dispatch model (Phase 6
// Stage M2c, design/STAGE3-INLINING-DESIGN-ROUND.0.md §6 M2c).
//
// A module word like Log.with / Log.counter / Rand.with-seed returns an
// INSTANCE — a Map of trivial-delegation method wrappers closing over
// per-instance state. Its check-mode ReturnsFn builds a SHAPE-ONLY twin
// (state-independent method names + signatures) so the dot read resolves;
// but the runtime instance carries per-call state, so neither the member
// value nor its handler may ever bake into the program (the freeze-gate:
// a baked check-time closure would log with the shape state). The read
// therefore stays DYNAMIC — and before this model, a statement-position
// method call (`l.info "req" ; …`) stranded [dyn, args] mid-residual and
// the program refused ("dynamic value precedes residual args").
//
// The model, in three steps:
//
//  1. ANNOTATE — the accessor ReturnsFn (lang getNodeReturns) calls
//     NoteMethodShape with the read's fresh dynamic carrier and the
//     resolved member. The member is only NOTED (CheckState side table,
//     keyed by carrier ID — the fnRiskFields/CtxShapes precedent), never
//     surfaced: the carrier stays dynamic(Any), byte-identical to the
//     unannotated read, so plain-check behaviour is unchanged.
//  2. MODEL — when the compile pass steps the annotated carrier at the
//     pointer (exactly where the interpreter auto-dispatches the concrete
//     member), tryShapedMethodDispatch mirrors execFnDefLiteral: the
//     ENGINE'S OWN matcher picks the member overload over the same tape,
//     and the model commits only when the match is PURE-FORWARD over an
//     inert, evaluation-fixed statement window — so the compiled window
//     is the very token sequence the interpreter's forward collection
//     consumes, bounded by the same statement boundary. The dispatch is
//     then modelled through carrierResults over the member's inner
//     signature (declared returns, gradual contagion — identical to a
//     real dispatch of that native).
//  3. RECORD — the outcome seam routes the modelled dispatch to
//     RecordDynMethod (via CheckState.PendingMethodApply, consumed FIRST
//     in recordDispatchOutcome so the member's native can never record as
//     a check-time CALL_NATIVE): a mid-stream OpCallDynMethod whose fn
//     operand is the dot-read EVENT (the runtime value) and whose spec
//     claims the matched arity + declared result count. The VM enforces
//     the claim and defers to the interpreter via internal_error when the
//     runtime value ever fails it (RunCompiled's runtimeShouldFallback —
//     slow, not wrong).
//
// The miscompile-E auto-dispatch guard is NOT weakened: a member with a
// genuine 0-arg overload (Span.finish, Rand.bool) is never annotated
// (NoteMethodShape vets it), and the get-family read guards
// (containerFnAutoDispatchRisk / zeroArgFnOut) still refuse those reads
// outright before any model could run.

// evalFixedWindowToken reports whether a raw tape token is an INERT,
// evaluation-fixed VALUE the shaped-method model may bake as a const
// operand: a concrete scalar/atom, or a list/map whose members are
// recursively such. Evaluation-fixed means the interpreter's arg
// processing (forward collection + autoEvalList/Map at dispatch) is the
// identity on it — no words, parens, interpolations, reaches, splices,
// fn values, computed map keys, carriers, or undefined placeholders —
// so the baked const IS the value the interpreter's handler receives.
func evalFixedWindowToken(v Value) bool {
	if v.Carrier || v.Dynamic || v.Undefined {
		return false
	}
	switch d := v.Data.(type) {
	case IntPayload, StrPayload, BoolPayload, FloatPayload, AtomPayload,
		BigIntPayload, DecimalPayload:
		return true
	case ListPayload:
		for _, ev := range d.Elems {
			if !evalFixedWindowToken(ev) {
				return false
			}
		}
		return true
	case MapPayload:
		if d.M == nil {
			return false
		}
		if d.M.Meta != nil {
			if ck, _ := d.M.Meta["ck"].(map[string]bool); len(ck) > 0 {
				return false // computed keys evaluate at dispatch time
			}
		}
		for _, k := range d.M.Keys() {
			mv, _ := d.M.Get(k)
			if !evalFixedWindowToken(mv) {
				return false
			}
		}
		return true
	}
	return false
}

// tryShapedMethodDispatch models the interpreter's auto-dispatch of an
// annotated dynamic method-read carrier sitting at the pointer (see the
// file comment). Returns true when it consumed the dispatch (tape spliced,
// event recorded or the program marked uncompilable); false leaves the
// carrier to today's paths (residual windows, refusals) untouched.
func tryShapedMethodDispatch(e *Engine, valIdx int) bool {
	r := e.Registry
	v := e.Tape.At(valIdx)
	if !v.Dynamic || v.Quoted || v.ID == "" {
		return false
	}
	member, ok := r.Check.MethodShapeMember(v.ID)
	if !ok {
		return false
	}
	if !r.Check.Recorder().Active() {
		// Plain-check surface (not compiling): a shaped dynamic method apply —
		// a delegation-wrapper method value followed by its contiguous inert
		// forward-arg window — collapses to a single dynamic(Any), consuming
		// the window, so the residual covers any runtime result. Without this
		// the args strand on the check stack and the checker mis-models the
		// arity (module-rand.tsv:37: `r.string "abc" 5` left
		// `[dynamic(Any) ProperString Integer]` vs runtime `[ProperString]`).
		// Gated to the non-compiling ratchet surface: the compile pass below
		// keeps the [dynamic, args] residual intact so resolveDynamicApply →
		// OpCallDynMethod still fires, and a suspended pass stays byte-identical.
		if !r.Check.Compiling {
			if sig, positions, wok := shapedMethodApplyWindow(e, valIdx, member); wok {
				// Collapse to the matched signature's ARITY, not always one value.
				// Side-effect-only shaped methods (logger info, metric add/record)
				// declare zero returns; splicing a lone dynamic(Any) would fabricate
				// a value the runtime never produces, so `(l.info "msg") add 1` would
				// wrongly check clean. Resolve the arity exactly as a real dispatch
				// would (ReturnsFn / declared Returns), then splice that many gradual
				// carriers — 0 for a side-effect method, 1 for a value method.
				args := make([]Value, len(positions))
				for i, p := range positions {
					args[i] = e.Tape.At(p)
					args[i].Eval = false
					args[i].Undefined = false
				}
				reps := make([]Value, shapedMethodReturnArity(e, sig, args, v.Pos()))
				for i := range reps {
					reps[i] = NewDynamicCarrier(TAny)
				}
				e.Tape.Splice(valIdx, 1+len(positions), reps...)
				return true
			}
		}
		return false
	}
	sig, positions, ok := shapedMethodApplyWindow(e, valIdx, member)
	if !ok {
		// Guard-owned decline: the get-family read guard was SKIPPED for this
		// annotated read (noteShapedRead), so a genuine-0-arg member whose
		// landing the model cannot claim must refuse HERE — the auto-dispatch
		// guard is re-homed onto the landing, never weakened.
		if FnValueZeroArg(member) {
			r.Check.Recorder().MarkUncompilable(
				"shaped 0-arg method landing not modelable at " + fnDefName(member))
		}
		return false
	}
	fnDef, _ := member.Data.(FnDefInfo) // validated by shapedMethodApplyWindow
	args := make([]Value, len(positions))
	for i, p := range positions {
		args[i] = e.Tape.At(p)
		args[i].Eval = false
		args[i].Undefined = false
	}
	// Model the dispatch through the shared check-mode machinery (declared
	// returns, folds, contagion — identical to a real dispatch of the inner
	// native) with the outcome seam routed to RecordDynMethod.
	r.Check.PendingMethodApply = &PendingMethodApply{Origin: v, Word: fnDef.Name}
	outs := carrierResults(r, fnDef.Name, sig, args, v.Pos(), nil, false)
	if r.Check.PendingMethodApply != nil { //covergate:allow compiler/VM defensive arm; unreachable without a bytecode-level fault (§compiler)
		// Not consumed — an unexpected short-circuit upstream of the outcome
		// seam. Decline wholesale; the carrier keeps today's paths.
		r.Check.PendingMethodApply = nil
		return false
	}
	// Consume the carrier + the matched window, splice in the modelled
	// results; the pointer re-steps them (spliceMatchResults' convention).
	k := len(positions)
	e.Tape.Splice(valIdx, 1+k, outs...)
	return true
}

// shapedMethodApplyWindow returns the matched signature and forward-arg
// positions for a shaped dynamic method carrier at valIdx followed by a
// contiguous inert forward-arg window — or ok=false when the shape is not a
// plain-native statement-window apply. Pure (no tape mutation): shared by the
// compile-pass model (which runs the real dispatch) and the plain-check
// collapse (which folds the apply to dynamic(Any)).
func shapedMethodApplyWindow(e *Engine, valIdx int, member Value) (*Signature, []int, bool) {
	fnDef, ok := member.Data.(FnDefInfo)
	if !ok || fnDef.Registry == nil {
		return nil, nil, false
	}
	fn := fnDef.Registry.Lookup(fnDef.Name)
	if fn == nil {
		return nil, nil, false
	}
	// The statement window: every token from the carrier to the statement
	// boundary must be inert and evaluation-fixed. Anything else — a word, a
	// paren, a marker, a carrier — declines the whole model (the interpreter
	// could dispatch or collect through it in ways the window cannot bake).
	// An ALL-0-ARG member skips the scan entirely: it NEVER forward-collects
	// (it auto-fires with no operands the moment it lands), so the following
	// tokens belong to the NEXT dispatch and are irrelevant to the arity-0
	// model — `r get "bool" eq false` was declining on the `eq` word the
	// member cannot consume (probe-verified; the dot form of the same
	// landing already compiled through the identical arity-0 claim).
	winEnd := valIdx
	if !allZeroArgSigs(fn) {
		var winOK bool
		if winEnd, winOK = inertStatementWindow(e, valIdx); !winOK {
			return nil, nil, false
		}
	}
	if winEnd == valIdx || allZeroArgSigs(fn) {
		// No forward window (or every overload is 0-arg, so the landing
		// never collects): the interpreter auto-fires a GENUINE 0-arg
		// overload the moment the member lands — the miscompile-E family
		// (Span.finish, Rand.bool). Model it as an arity-0 apply through
		// the member's 0-arg signature; a member without a genuine 0-arg
		// overload stays data in both engines.
		if !FnValueZeroArg(member) {
			return nil, nil, false
		}
		for i := range fn.Signatures {
			sg := &fn.Signatures[i]
			if sg.Fallback || sg.TotalArgs() != 0 {
				continue
			}
			if sg.DispatchHandler() == nil || sg.FnFrame() != nil || sg.FullStack() ||
				sg.RunInCheckMode() || sg.Callable != nil || len(sg.NoEvalArgs) > 0 ||
				sg.ParkResult() {
				return nil, nil, false // not a plain Go-handler apply
			}
			return sg, nil, true
		}
		return nil, nil, false
	}
	// The interpreter's own overload choice over the same tape. The window
	// admission above guarantees no paren pre-evaluation is needed
	// (resolveForwardArgs would be a no-op), so the plan-time match here IS
	// the match the interpreter performs on the concrete member.
	w := WordInfo{Name: fnDef.Name, ArgCount: -1}
	sig, positions, _ := e.MatchSignature(fn, w, e.EffectiveResolved())
	if sig == nil || sig.Fallback || len(positions) == 0 { //covergate:allow compiler/VM defensive arm; unreachable without a bytecode-level fault (§compiler)
		return nil, nil, false
	}
	// Pure-forward, contiguous-prefix coverage: the matched positions must be
	// exactly the window slots valIdx+1 .. valIdx+k. A match that reaches the
	// stack below the carrier (a trailing/mixed shape) or skips a slot is not
	// the statement-window apply — those keep today's paths.
	for i, p := range positions {
		if p != valIdx+1+i { //covergate:allow compiler/VM defensive arm; unreachable without a bytecode-level fault (§compiler)
			return nil, nil, false
		}
	}
	if positions[len(positions)-1] > winEnd { //covergate:allow compiler/VM defensive arm; unreachable without a bytecode-level fault (§compiler)
		return nil, nil, false
	}
	// Only a plain Go-handler native models: no code bodies, no quotation
	// beyond atom capture, no check-mode side effects, no user-fn frames —
	// the shaped-method class is exactly the delegation-wrapper methods.
	if sig.DispatchHandler() == nil || sig.FnFrame() != nil || sig.FullStack() ||
		sig.RunInCheckMode() || sig.Callable != nil || len(sig.NoEvalArgs) > 0 ||
		sig.ParkResult() { //covergate:allow compiler/VM defensive arm; unreachable without a bytecode-level fault (§compiler)
		return nil, nil, false
	}
	return sig, positions, true
}

// statementWindowBoundary reports whether v ends a statement window for
// the check-side apply models: an engine marker or an explicit
// statement/paren boundary. The single boundary set shared by every
// scanner below — it must stay aligned with what the interpreter's own
// forward collection treats as a hard stop.
func statementWindowBoundary(v Value) bool {
	return IsMark(v) || IsMove(v) || IsCloseParen(v) || IsEnd(v)
}

// inertStatementWindow scans the statement window after valIdx: every
// token up to the first boundary (statementWindowBoundary) must be inert
// and evaluation-fixed. Returns the index of the last window token
// (valIdx itself when the window is empty) and ok=false when a non-fixed
// token — a word, a paren, a carrier — sits inside the window: the
// interpreter could dispatch or collect through it in ways a flat
// consume cannot mirror, so the caller's model declines. The ONE scanner
// behind both the shaped-method window and the dynamic fn-value window;
// tryMemberFnArrivalDispatch applies the same per-token test over its
// arity-bounded span.
func inertStatementWindow(e *Engine, valIdx int) (winEnd int, ok bool) {
	winEnd = valIdx
	for i := valIdx + 1; i < e.Tape.Len(); i++ {
		tv := e.Tape.At(i)
		if statementWindowBoundary(tv) {
			break
		}
		if !evalFixedWindowToken(tv) {
			return winEnd, false
		}
		winEnd = i
	}
	return winEnd, true
}

// allZeroArgSigs delegates to the kernel's canonical pure-property-fn
// predicate (fnValueOnlyZeroArgSigs, engine.go) — the shaped-method
// 0-arg landing model and the interpreter's NUR035 deferral exemption
// must answer this question identically, so there is exactly one
// implementation.
func allZeroArgSigs(fn *FnDefInfo) bool {
	return FnValueOnlyZeroArgSigs(*fn)
}

// fnDefName names a function value for a refusal message.
func fnDefName(v Value) string {
	if fd, ok := v.Data.(FnDefInfo); ok && fd.Name != "" {
		return fd.Name
	}
	return "fn value"
}

// shapedMethodReturnArity is the runtime result count of a shaped-method apply,
// resolved exactly as declaredReturnCarriers does — a ReturnsFn's produced arity,
// else the declared Returns length (nil → 0) — but WITHOUT the missing-returns
// diagnostic, since the plain-check collapse is silent. This keeps the collapse
// arity-faithful: a side-effect-only method (0 returns) collapses to 0 values,
// not a fabricated one.
func shapedMethodReturnArity(e *Engine, sig *Signature, args []Value, pos SrcPos) int {
	if sig.ReturnsFn != nil {
		e.Registry.Check.CurCallPos = pos
		return len(sig.ReturnsFn(args, e.Registry))
	}
	return len(sig.Returns)
}

// dynamicBoundConformsToFunction reports whether a dynamic carrier's static
// BOUND could be a callable — its Parent conforms to Function, or (the
// typed-patrun `find` shape) one alternative of its disjunct bound does. Only
// a Function-bearing bound may auto-dispatch a forward window; a
// dynamic(String|None) etc. must strand its trailing values as data (the
// runtime never calls them), keeping the residual stack depth honest.
func dynamicBoundConformsToFunction(v Value) bool {
	if v.Parent.ConformsTo(TFunction) {
		return true
	}
	if disj, err := AsDisjunct(v); err == nil {
		for _, alt := range disj.Alternatives {
			// A disjunct alternative is a bare type-literal Value whose own
			// lattice identity (typeNodeOf) is the represented type — its
			// .Parent is TType, not the type itself.
			at := TypeNodeOf(alt)
			if at.ConformsTo(TFunction) {
				return true
			}
		}
	}
	return false
}

// tryDynamicFnValueDispatch is the general-value analogue of
// tryShapedMethodDispatch: a DYNAMIC carrier whose bound is Function-bearing
// (a typed-patrun `find` result — dynamic(Function ∪ None) — or any dynamic
// fn value) sitting at the pointer with a contiguous inert forward-arg window
// after it is the interpreter's auto-dispatch site. WHICH concrete fn the
// carrier holds is a runtime fact, so the model is optimistic on the callable
// alternative (like every dynamic-modality escape hatch): consume the window
// and produce a single dynamic(Any), which the oracle's Dynamic rule covers.
// This clears the arg-stranding that leaves `h {x:3 y:4}` as
// `[dynamic(Function|None) Map]` (vs runtime `[Integer]`) — patrun.tsv:40.
//
// PLAIN-CHECK ONLY (not compiling, no live recorder). The compile pass keeps
// the [dynamic, args] residual so resolveDynamicApply / OpCallDynamicTrailing
// still lowers the call, and a suspended pass stays byte-identical
// (TestSpecCompiledDifferential). Optimism is sound for every clean corpus row
// — a clean miss-then-call (a None reader immediately applied) does not occur;
// it is the same gradual gap dynamic modality accepts elsewhere.
func tryDynamicFnValueDispatch(e *Engine, valIdx int) bool {
	r := e.Registry
	if r.Check.Compiling || r.Check.Recorder().Active() {
		return false
	}
	v := e.Tape.At(valIdx)
	if !v.Dynamic || v.Quoted || !dynamicBoundConformsToFunction(v) {
		return false
	}
	// The forward window: the same inert, evaluation-fixed statement-window
	// admission the shaped-method model uses (one shared scanner).
	winEnd, winOK := inertStatementWindow(e, valIdx)
	if !winOK {
		return false
	}
	if winEnd == valIdx {
		return false // no args — a bare dynamic fn value stays data (both engines)
	}
	e.Tape.Splice(valIdx, 1+(winEnd-valIdx), NewDynamicCarrier(TAny))
	return true
}

// tryRecordMethodApply is the FIRST specialist in recordDispatchOutcome's
// chain: it consumes a pending shaped-method model (set by
// tryShapedMethodDispatch around its carrierResults call) and records the
// guarded OpCallDynMethod event. It must run before every other recorder —
// falling through would record the member's inner native as a check-time
// CALL_NATIVE against the shape instance's sub-registry, baking shape
// state (the freeze-gate violation this model exists to avoid).
func tryRecordMethodApply(r *Registry, word string, args, out []Value, pos SrcPos) bool {
	pm := r.Check.PendingMethodApply
	if pm == nil {
		return false
	}
	if pm.Word != word {
		return false // an interleaved dispatch — leave the pending for its owner
	}
	r.Check.PendingMethodApply = nil
	es := r.Check.Recorder()
	if !es.RecordDynMethod(pm.Origin, args, out, word, pos) {
		es.MarkUncompilable("shaped method apply: operand of unknown provenance at " + word)
	}
	return true
}

// tryMemberFnArrivalDispatch models the interpreter's ARRIVAL-APPLY of a
// container-member fn read mid-expression (REFUSAL-CLOSURE.0 §3): the
// interpreter applies a surfaced member fn (`m.double`) the moment its
// argument window fills — `m.double 21 eq 42` runs `(m.double 21)` BEFORE
// `eq` — while the recorder previously only saw word dispatches, so the
// downstream word stole the operand and refuseStrandedMemberFn refused the
// program. This hook fires where the check pass steps the member-read
// carrier: when the read pinpointed the member (memberFnReadValue — a
// concrete container + key) and the member's SINGLE plain signature's whole
// arity of inert tokens sits immediately after the carrier, it models the
// dispatch through the shared PendingMethodApply → RecordDynMethod seam
// (the M2c machinery: a guarded mid-stream OpCallDynMethod whose runtime
// value — the real map read's product — drives the apply, byte-identical to
// the interpreter's forward auto-dispatch of the same window).
//
// The window claim is the ARITY, not the statement: the interpreter's parked
// fn fires the moment its single signature's args arrive, so the token after
// the window (a word, `eq`) never enters the collection. Everything this
// hook declines keeps today's paths — the statement-tail Finalize apply for
// shapes it never sees, refuseStrandedMemberFn's sound refusal for the rest:
//   - COMPILE pass only (live recording; plain checks and suspended passes
//     stay byte-identical);
//   - a uniquely-resolved, NAMED, non-anonymous, non-macro, capture-free
//     member with exactly ONE non-fallback signature of plain params (the
//     model's carrierResults and the arity claim assume plain value args —
//     multi-sig first-match and captures are follow-on scope);
//   - arity >= 1 (a 0-arg auto-fire is the read-guard's own class) and the
//     full arity of evaluation-fixed tokens inside the statement.
func tryMemberFnArrivalDispatch(e *Engine, valIdx int) bool {
	r := e.Registry
	es := r.Check.Recorder()
	if !es.Active() || es.SuspendedNow() {
		return false
	}
	v := e.Tape.At(valIdx)
	if !v.Dynamic || v.Quoted || v.ID == "" {
		return false
	}
	member, ok := es.MemberFnReadValue(v.ID)
	if !ok {
		return false
	}
	fnDef, _ := member.Data.(FnDefInfo) // validated by memberFnReadValue
	if fnDef.Name == "" || fnDef.Anonymous || fnDef.Macro || len(fnDef.Captured) != 0 {
		return false
	}
	var sig *Signature
	for i := range fnDef.Signatures {
		s := &fnDef.Signatures[i]
		if s.Fallback {
			continue
		}
		if sig != nil {
			return false // multi-overload: runtime first-match not modelled here
		}
		sig = s
	}
	if sig == nil || sig.TotalArgs() < 1 || len(sig.Body()) == 0 {
		return false
	}
	// Plain value params only (the model and the arity claim assume them).
	// FnParam.Quote needs no separate probe: the body gate above proves a
	// boru impl, whose normalizeSig derives QuoteArgs FROM the params.
	if len(sig.QuoteArgs) != 0 || len(sig.TypeArgs) != 0 ||
		len(sig.NoEvalArgs) != 0 || len(sig.NoEvalMapArgs) != 0 ||
		len(sig.RawParens) != 0 || len(sig.FormArgs) != 0 {
		return false
	}
	if len(sig.Returns) != 1 {
		return false // the arity/result claim assumes one downstream value
	}
	n := sig.TotalArgs()
	if valIdx+n >= e.Tape.Len() {
		return false
	}
	args := make([]Value, n)
	for i := 1; i <= n; i++ {
		tv := e.Tape.At(valIdx + i)
		if statementWindowBoundary(tv) || !evalFixedWindowToken(tv) {
			return false
		}
		args[i-1] = tv
		args[i-1].Eval = false
		args[i-1].Undefined = false
	}
	// Model the dispatch SUSPENDED (a plain user fn's ReturnsFn records its
	// own CALL_USER through core_helpers, not the outcome seam — a live probe
	// would leak a phantom event the lowering cannot seat), then record the
	// guarded dyn-method event directly. No body unit is needed: the VM's
	// callDynMethod applies the RUNTIME member value — a plain user fn takes
	// the island path, byte-identical to the interpreter's auto-dispatch —
	// and the member's declared return contract is engine-enforced at run
	// time, so the modelled out carrier is sound.
	resume := es.Suspend()
	outs := carrierResults(r, fnDef.Name, sig, args, v.Pos(), nil, false)
	resume()
	if len(outs) != 1 { //covergate:allow the declared-single-return gate above fixes carrierResults' count for a boru-bodied sig (buildFnBodyReturnsFn returns one carrier per declared return) — unreachable without a model fault (§compiler)
		return false
	}
	if !es.RecordDynMethod(v, args, outs, fnDef.Name, v.Pos()) { //covergate:allow the fn carrier is event-produced (memberFnRead tags recorded reads only) and the window args are inert consts, so operand resolution cannot fail — unreachable without a recorder fault (§compiler)
		return false
	}
	e.Tape.Splice(valIdx, 1+n, outs...)
	return true
}
