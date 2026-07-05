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

// PendingMethodApply threads ONE modelled shaped-method dispatch from
// tryShapedMethodDispatch into recordDispatchOutcome. Set immediately
// before the model's carrierResults call and consumed by
// tryRecordMethodApply; the model declines (no tape splice) if the
// pending was not consumed (an unexpected special-word short-circuit).
type PendingMethodApply struct {
	Origin Value  // the dynamic method-read carrier; its producing event supplies the runtime fn
	Word   string // the member word name (defensive re-key at the outcome seam)
}

// NoteMethodShape registers a method-shape annotation: the get-family read
// that produced the dynamic carrier `out` resolved the concrete member
// `member` on a check-mode shape instance. Vetting is centralised here so
// every caller inherits the guards:
//
//   - only a NAMED trivial-delegation wrapper carrying a foreign
//     sub-registry qualifies (the shaped-instance-method class — a plain
//     user fn stored in a map keeps today's refusal paths, so a capturing
//     method fn still refuses);
//   - a member with a GENUINE 0-arg overload is excluded: that is the
//     miscompile-E auto-dispatch family (Span.finish, Rand.bool), whose
//     reads the get-family guards refuse — the annotation must never
//     offer the model a way around them;
//   - macros stay data (applied only by name).
func (c *CheckState) NoteMethodShape(out, member Value) {
	if !c.IsActive() || out.ID == "" {
		return
	}
	fd, ok := member.Data.(FnDefInfo)
	if !ok || fd.Registry == nil || fd.Name == "" || fd.Macro {
		return
	}
	if !isDelegationFnDef(fd) || fnValueZeroArg(member) {
		return
	}
	if c.MethodShapes == nil {
		c.MethodShapes = map[string]Value{}
	}
	c.MethodShapes[out.ID] = member
}

// methodShapeMember returns the annotated member for a carrier ID.
func (c *CheckState) methodShapeMember(id string) (Value, bool) {
	if c == nil || id == "" || c.MethodShapes == nil {
		return Value{}, false
	}
	m, ok := c.MethodShapes[id]
	return m, ok
}

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
func (e *Engine) tryShapedMethodDispatch(valIdx int) bool {
	r := e.registry
	v := e.tape.At(valIdx)
	if !v.Dynamic || v.Quoted || v.ID == "" {
		return false
	}
	member, ok := r.Check.methodShapeMember(v.ID)
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
			if _, positions, wok := e.shapedMethodApplyWindow(valIdx, member); wok {
				e.tape.Splice(valIdx, 1+len(positions), NewDynamicCarrier(TAny))
				return true
			}
		}
		return false
	}
	sig, positions, ok := e.shapedMethodApplyWindow(valIdx, member)
	if !ok {
		return false
	}
	fnDef, _ := member.Data.(FnDefInfo) // validated by shapedMethodApplyWindow
	args := make([]Value, len(positions))
	for i, p := range positions {
		args[i] = e.tape.At(p)
		args[i].Eval = false
		args[i].Undefined = false
	}
	// Model the dispatch through the shared check-mode machinery (declared
	// returns, folds, contagion — identical to a real dispatch of the inner
	// native) with the outcome seam routed to RecordDynMethod.
	r.Check.PendingMethodApply = &PendingMethodApply{Origin: v, Word: fnDef.Name}
	outs := carrierResults(r, fnDef.Name, sig, args, v.Pos, nil, false)
	if r.Check.PendingMethodApply != nil {
		// Not consumed — an unexpected short-circuit upstream of the outcome
		// seam. Decline wholesale; the carrier keeps today's paths.
		r.Check.PendingMethodApply = nil
		return false
	}
	// Consume the carrier + the matched window, splice in the modelled
	// results; the pointer re-steps them (spliceMatchResults' convention).
	k := len(positions)
	e.tape.Splice(valIdx, 1+k, outs...)
	return true
}

// shapedMethodApplyWindow returns the matched signature and forward-arg
// positions for a shaped dynamic method carrier at valIdx followed by a
// contiguous inert forward-arg window — or ok=false when the shape is not a
// plain-native statement-window apply. Pure (no tape mutation): shared by the
// compile-pass model (which runs the real dispatch) and the plain-check
// collapse (which folds the apply to dynamic(Any)).
func (e *Engine) shapedMethodApplyWindow(valIdx int, member Value) (*Signature, []int, bool) {
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
	winEnd := valIdx
	for i := valIdx + 1; i < e.tape.Len(); i++ {
		tv := e.tape.At(i)
		if IsMark(tv) || IsMove(tv) || IsCloseParen(tv) || IsEnd(tv) {
			break
		}
		if !evalFixedWindowToken(tv) {
			return nil, nil, false
		}
		winEnd = i
	}
	if winEnd == valIdx {
		return nil, nil, false // no args — a bare landed value stays data in both engines
	}
	// The interpreter's own overload choice over the same tape. The window
	// admission above guarantees no paren pre-evaluation is needed
	// (resolveForwardArgs would be a no-op), so the plan-time match here IS
	// the match the interpreter performs on the concrete member.
	w := WordInfo{Name: fnDef.Name, ArgCount: -1}
	sig, positions, _ := e.matchSignature(fn, w, e.effectiveResolved())
	if sig == nil || sig.Fallback || len(positions) == 0 {
		return nil, nil, false
	}
	// Pure-forward, contiguous-prefix coverage: the matched positions must be
	// exactly the window slots valIdx+1 .. valIdx+k. A match that reaches the
	// stack below the carrier (a trailing/mixed shape) or skips a slot is not
	// the statement-window apply — those keep today's paths.
	for i, p := range positions {
		if p != valIdx+1+i {
			return nil, nil, false
		}
	}
	if positions[len(positions)-1] > winEnd {
		return nil, nil, false
	}
	// Only a plain Go-handler native models: no code bodies, no quotation
	// beyond atom capture, no check-mode side effects, no user-fn frames —
	// the shaped-method class is exactly the delegation-wrapper methods.
	if sig.dispatchHandler() == nil || sig.fnFrame() != nil || sig.fullStack() ||
		sig.runInCheckMode() || sig.Callable != nil || len(sig.NoEvalArgs) > 0 ||
		sig.parkResult() {
		return nil, nil, false
	}
	return sig, positions, true
}

// dynamicBoundConformsToFunction reports whether a dynamic carrier's static
// BOUND could be a callable — its Parent conforms to Function/FnDef, or (the
// typed-patrun `find` shape) one alternative of its disjunct bound does. Only
// a Function-bearing bound may auto-dispatch a forward window; a
// dynamic(String|None) etc. must strand its trailing values as data (the
// runtime never calls them), keeping the residual stack depth honest.
func dynamicBoundConformsToFunction(v Value) bool {
	if v.Parent.ConformsTo(TFunction) || v.Parent.ConformsTo(TFnDef) {
		return true
	}
	if disj, err := AsDisjunct(v); err == nil {
		for _, alt := range disj.Alternatives {
			// A disjunct alternative is a bare type-literal Value whose own
			// lattice identity (typeNodeOf) is the represented type — its
			// .Parent is TType, not the type itself.
			at := typeNodeOf(alt)
			if at.ConformsTo(TFunction) || at.ConformsTo(TFnDef) {
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
func (e *Engine) tryDynamicFnValueDispatch(valIdx int) bool {
	r := e.registry
	if r.Check.Compiling || r.Check.Recorder().Active() {
		return false
	}
	v := e.tape.At(valIdx)
	if !v.Dynamic || v.Quoted || !dynamicBoundConformsToFunction(v) {
		return false
	}
	// The forward window: every token from the carrier to the statement
	// boundary must be inert and evaluation-fixed (the same admission the
	// shaped-method window uses). A word/paren/marker in the window declines
	// the model — the interpreter would dispatch or collect through it in ways
	// a flat consume cannot mirror.
	winEnd := valIdx
	for i := valIdx + 1; i < e.tape.Len(); i++ {
		tv := e.tape.At(i)
		if IsMark(tv) || IsMove(tv) || IsCloseParen(tv) || IsEnd(tv) {
			break
		}
		if !evalFixedWindowToken(tv) {
			return false
		}
		winEnd = i
	}
	if winEnd == valIdx {
		return false // no args — a bare dynamic fn value stays data (both engines)
	}
	e.tape.Splice(valIdx, 1+(winEnd-valIdx), NewDynamicCarrier(TAny))
	return true
}

// methodShapeAnnotated reports whether a value ID carries a method-shape
// annotation this pass. Consulted by resolveDynamicApply: an ANNOTATED
// carrier reaching the program residual as a LEADING or INTERIOR apply
// window means the statement-window model DECLINED it (a computed arg, a
// word in the window, a stack-reaching match), so the residual window's
// "tail = args" assumption is unverified for it — the leading/mixed
// windows may absorb values from LATER statements into the apply (the
// interpreter's forward collection stops at the statement End; the
// flattened residual has no such boundary), a silent-divergence class
// probe-confirmed pre-existing (`c.add (1 add 2) ; Log.measurements
// size` compiled to the wrong value). Such carriers now refuse instead —
// sound fallback. Trailing shapes are unaffected: they draw args from
// the STACK below the value, exactly the interpreter's stack-form
// dispatch, which crosses statements by design.
func (es *EmitState) methodShapeAnnotated(id string) bool {
	if es == nil || es.reg == nil {
		return false
	}
	_, ok := es.reg.Check.methodShapeMember(id)
	return ok
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
