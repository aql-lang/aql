package eng

// Code-body closure compilation (plan P2): a higher-order word whose body is a
// quoted code list compiles that body to its own fn unit, and the dispatch
// records the body operand as a closure (OpPushClosure) instead of a Stage-5
// interpreter island. At run time the word's native handler invokes the
// closure through the VM's re-entrant runner (vmContext.invokeClosure) via the
// InvokeBody seam — no interpreter sub-engine.

// The closure-eligible set is NOT a name-keyed table in eng. Each such word
// DECLARES its closure shape on its NativeFunc as a *CallableSpec (BodyPos /
// BodyOut / Inputs), which RegisterNativeFunc copies onto every signature; the
// recorder reads it back via the resolved sig.Callable. eng therefore names no
// specific word — the core transforms (each / fold / scan / do / filter /
// with-decimal) declare in lang/native, and module words (the aql:test case /
// describe bodies) declare in their own module, with no eng↔module coupling.
// See eng/go/value.go::CallableSpec and review §4.5.

// compileClosureBody compiles a code body (bodyToks) consuming the given input
// carriers into its own fn unit, returning (unitIndex, ok). The body is
// recorded into the CURRENT EmitState (r.Check.Emit) — callers run it once in
// a throwaway probe state to test compilability, then once in the real state.
// Mirrors the fn-def compile path (core_helpers.go): StartFnCompile arms the
// unit, AnalyseFnBody records the body under it, finish closes it. ok is false
// when the body refuses (StartFnCompile declined, or the analysis marked the
// state uncompilable).
// paramNames is the per-input name table: a NAMED slot (a lambda param,
// `([p] => …)`) binds the body's `p` to that input carrier in AnalyseFnBody;
// an empty name (the token-quotation form, `[body]`) leaves the input on the
// stack for the body to consume positionally. nil means all-unnamed.
func compileClosureBody(r *Registry, word string, bodyOut int, emptyBodyOK, takesTop bool, bodyToks, inputs []Value, paramNames []string, captures []CapturedBinding, shape ClosureInShape, pos SrcPos) (int, bool) {
	// Closure compilation is emit-cluster machinery: it writes recording
	// internals (fnRecs), so it needs the CONCRETE EmitState. A pass without
	// one (the inactive recorder) declines exactly as the nil field did —
	// the typed-nil receiver's StartFnCompile returns !ok below.
	es, _ := r.Check.Recorder().(*EmitState)
	// A side-effect body word (bodyOut 0 — a test case body) declares NO returns,
	// so its 0-value residual is taken as-is rather than count-refused against the
	// default single [TAny]. The fn-VALUE factory path (word "fnval") passes
	// bodyOut 1, so it keeps the single return. An EmptyBodyErrors word
	// (each/fold/scan) also declares no returns even at bodyOut 1: a 0-net body is
	// the handler's OWN runtime error, raised faithfully at invoke time, so the
	// closure is left count-agnostic rather than count-refused (which would
	// island).
	declared := []*Type{TAny}
	if bodyOut == 0 || bodyOut == BodyOutResidual || emptyBodyOK {
		// A side-effect body (0), a whole-residual body (do — the handler
		// returns everything the body nets, so the count is per-body), and an
		// EmptyBodyErrors body are all count-AGNOSTIC: declared nil skips the
		// closure count refusal and the unit RETs its actual residual.
		declared = nil
	}
	if paramNames == nil {
		paramNames = make([]string, len(inputs)) // all unnamed: body reads inputs off the stack
	}
	name := word + "$body"
	key := FnAnalysisKey(r.AnalysisScopeID(), name, inputs, captures, bodyToks)
	unit, finish, ok := es.StartFnCompile(key, name, r, inputs, declared, paramNames, captures, false, pos)
	if !ok {
		return -1, false
	}
	// Record the closure's input convention on the unit (consistent across a
	// memo hit: the key includes name+input types, which determine the shape).
	es.fnRecs[unit].inShape = shape
	es.fnRecs[unit].closure = true
	es.fnRecs[unit].takesTop = takesTop
	// The two stored-ref compile paths use these eng-internal synthetic
	// names; their rebind safety is the per-ref poisoning, so the frozen-
	// read discipline skips them (see fnUnitRec.storedRefUnit).
	es.fnRecs[unit].storedRefUnit = word == "storedfn" || word == "spawnbody"
	if finish == nil {
		// Memo hit: the unit is already compiled in this state.
		return unit, es.active()
	}
	// Drop any summary a suspended (non-recording) analysis cached so the
	// body re-runs under the armed unit and records.
	delete(r.Check.FnSummaries, key)
	stk := AnalyseFnBody(r, name, paramNames, bodyToks, inputs, captures, declared)
	if len(bodyToks) == 0 && len(stk) == 0 {
		// An EMPTY body's residual is its pushed inputs, verbatim: the runtime
		// InvokeBody pushes the per-call inputs and runs no tokens, so the frame
		// nets exactly them — `error []` leaves the caught error (pass-through),
		// `each []` leaves the element (identity map). AnalyseFnBody early-
		// returns nil for an empty body, which compiled a RET-nothing unit that
		// DIVERGED from the interpreter (each_error "body produced no result"
		// against the interpreter's identity map). Reconstruct the residual so
		// the unit resolves each input to its param slot (PUSH_LOCAL i … RET).
		stk = append(stk, inputs...)
	}
	finish(stk)
	return unit, es.Compilable
}

// mutableInstanceRef reports whether v is (or stands in for) a MUTABLE
// instance — a concrete flex node / class instance, or a check-mode CARRIER
// typed at one (the binding a `def acc (flex […])` holds during analysis is a
// TFlexList carrier, not the concrete store). Bare type nodes are excluded:
// a class/flex TYPE literal is not an instance.
func mutableInstanceRef(v Value) bool {
	if IsBareTypeNode(v) {
		return false
	}
	if IsFlexNode(v) || IsClassInstance(v) {
		return true
	}
	p := v.Parent
	if p == nil {
		return false
	}
	if p.ConformsTo(TFlexMap) || p.ConformsTo(TFlexList) || p.ConformsTo(TFlexXml) {
		return true
	}
	return v.Carrier && p.ConformsTo(TClass)
}

// moduleScopeMutableCaptures extends a closure body's lexical captures with
// MODULE-SCOPE bindings the body reads whose values are MUTABLE INSTANCES
// (flex nodes, class instances). Per the language these references are
// DYNAMIC — module scope sits below the fn baseline, so ComputeCaptures
// excludes them — and for const-bakeable data the compiled body's const bake
// IS the dynamic read. A mutable instance cannot bake (materialise refuses;
// the body then refused "code-body word … (Stage 2)"), but its IDENTITY is
// fixed for the whole dispatch: a compiled body cannot rebind a module-scope
// name (body defs are body-local and module-mutating meta words refuse
// compilation), so the value passed at OpPushClosure equals every per-run
// lookup the interpreter makes — a capture is exact. This is the accumulator
// shape: `def acc (flex [0])  … each [ var [[x] (acc set 0 …)] ]`.
// Scoped to mutable instances ONLY — fn values, module exports, and bakeable
// consts keep their existing paths.
func moduleScopeMutableCaptures(r *Registry, bodyToks []Value, existing []CapturedBinding) []CapturedBinding {
	have := map[string]bool{}
	for _, cb := range existing {
		have[cb.Name] = true
	}
	bodyLocals := map[string]bool{}
	collectBodyLocalDefs(bodyToks, bodyLocals)
	out := existing
	seen := map[string]bool{}
	WalkBodyWords(bodyToks, func(w WordInfo, _ Value) {
		name := w.Name
		if name == "" || have[name] || bodyLocals[name] || seen[name] {
			return
		}
		seen[name] = true
		v, ok := r.Defs.Top(name)
		if !ok {
			return
		}
		if !mutableInstanceRef(v) && !moduleScopeInstanceCarrier(v) {
			return
		}
		out = append(out, CapturedBinding{Name: name, Value: v})
	})
	return out
}

// moduleScopeInstanceCarrier reports whether v is an IMMUTABLE module-scope
// INSTANCE carrier — a non-concrete carrier that a module-scope `def` binds (a
// persistent TrieMap/TstMap, a built collection, any instance a module
// constructor returns; its declared type may be Map/List or an under-annotated
// Any). Module scope sits below the fn baseline, so ComputeCaptures excludes
// it, and it is NOT const-bakeable (materialise refuses a non-concrete carrier)
// — yet it IS produced by a module-scope event, so it resolves at the call site
// (the closure's construction, at module scope) while its event is unreachable
// inside the body. Riding it as a capture slot is exact for the same reason the
// mutable-accumulator capture is: a compiled body cannot rebind a module-scope
// name (body defs are body-local; module-mutating meta words refuse), so the
// value threaded at OpPushClosure equals every per-run lookup the interpreter
// makes. An unreachable carrier declines later in recordClosureDispatch (capOps
// resolveOperand) — sound fallback.
//
// A concrete value still const-bakes (excluded here); a bare type node is a
// type, never a carried instance (excluded). A Dynamic (gradual-Any) carrier is
// INCLUDED: an under-annotated module constructor (TstMap.make) binds its
// instance as a Dynamic Any, yet the binding is still fixed for the program.
// The capture only makes the value reachable in the body; any downstream
// dispatch on it (an `each` over a gradual collection) still refuses at its own
// ambiguity gate, so admitting the capture never introduces an unsound dispatch
// — it just resolves the operand.
func moduleScopeInstanceCarrier(v Value) bool {
	return !IsBareTypeNode(v) && !IsConcrete(v) && v.Carrier
}

// tryRecordClosure attempts to compile a code-body higher-order word's body to
// a closure unit and record a normal dispatch (the body operand lowering to
// OpPushClosure). Returns true on success. A body that does not compile leaves
// the REAL emit state untouched — the probe runs in a throwaway state — so the
// caller falls through to the island path.
func tryRecordClosure(r *Registry, word string, sig *Signature, args, outs []Value, pos SrcPos) bool {
	if sig == nil || sig.Callable == nil {
		return false
	}
	spec := *sig.Callable
	// Concrete EmitState needed (extraNoEvalHookSlotsOK reads recording
	// internals); the inactive recorder falls to the !active() decline.
	es, _ := r.Check.Recorder().(*EmitState)
	// 0 or 1 outputs (a side-effect case body nets 0; a map/transform body nets
	// 1) — RecordClosureCall lowers both. A whole-residual word (BodyOutResidual:
	// `do` returns the body's ENTIRE residual) admits any statically-exact count:
	// the closure compiles count-agnostic, the VM's frameless RET returns the full
	// residual, and the dispatch seats all N results. Other multi-output words
	// stay beyond this path.
	if !es.active() || (len(outs) > 1 && spec.BodyOut != BodyOutResidual) || spec.BodyPos >= len(args) {
		return false
	}
	body := args[spec.BodyPos]

	// A SECOND NoEvalArgs hook slot (walk's optional ASCEND hook — the only
	// such slot on a Callable word today) rides as a plain VALUE operand next
	// to the compiled body closure, and the driving handler runs it as a token
	// QUOTATION through a sub-engine at run time. That is sound only for the
	// one provably-name-free shape extraNoEvalHookSlotsOK admits, plus — the
	// Stage M2d extension — a LAMBDA hook on a LambdaSharesTokenShape word,
	// which compiles to its OWN closure unit inside recordClosureDispatch
	// (walkClassifyHook already classifies a compiled closure per hook slot).
	// Anything else declines here so the Stage-2 refusal (the sound
	// interpreter fallback) stands.
	extraLamSlots, extrasOK := es.extraNoEvalHookSlots(sig, spec, args)
	if !extrasOK {
		return false
	}

	// A gradual-Any (Dynamic) DATA arg to a higher-order word whose overloads
	// differ on that arg's type (each/fold/scan: TList vs TMap) is an AMBIGUOUS
	// dispatch: the checker optimistically committed to ONE overload, but the
	// runtime value could need the SIBLING — a gradual collection bound to each's
	// Map overload errors ("expected concrete map") where the interpreter iterates
	// the runtime List. The body arg is never Dynamic, so anyDynamicCarrier here
	// means a DATA arg is. The ≥2-reachable gate is INHERENTLY token-form-specific:
	// a TList token body matches BOTH the {…,TList} and {…,TMap} overloads (count
	// 2), while a TFunction lambda body matches only the single {TFunction,TMap}
	// overload (count 1) — so a lambda never reaches here.
	//
	// For a CrossCollectionTokenShape word the token body is shape-generic (both
	// overloads present the bare value), so we DON'T refuse: fall through to record
	// the first-reachable (List) overload's closure ONCE, relying on the committed
	// handler being runtime-robust to the sibling collection type (it delegates to
	// the map/list iteration by the value's concrete type), so the same closure
	// drives either shape == the interpreter. A non-robust word (no flag) still
	// refuses → sound interpreter fallback.
	_, bodyIsLambda := body.Data.(FnDefInfo)
	tokenShapeGeneric := spec.CrossCollectionTokenShape && IsConcrete(body) && !bodyIsLambda
	if anyDynamicCarrier(args) && dynamicReachableOverloadCount(r, word, args) >= 2 && !tokenShapeGeneric {
		// A CompileDynBody word DECLINES instead: tryRecordDynBody records a
		// POLY re-match over the word's own sigs — the runtime value picks
		// the overload exactly as the interpreter's dispatch does.
		if sig.CompileEffect.Has(CompileDynBody) {
			return false
		}
		es.MarkUncompilable("higher-order `" + word + "` over a gradual-Any collection: ambiguous overload (List vs Map), no static commit and no poly re-match")
		return true
	}

	// A lambda VALUE body (`filter ([p:Any] => …) data`): the afn's named
	// param binds to the WORD'S callback shape ({key,value} pair for list
	// filter, KeyVal for the map forms), not to its declared `Any`. Compile
	// the lambda body against that representative shape so `p.value`/`kv.v`
	// typechecks, then record the dispatch with the body as a closure the
	// handler drives through InvokeBody.
	if fd, isFn := body.Data.(FnDefInfo); isFn {
		return tryRecordLambdaClosure(r, word, spec, sig, args, &fd, extraLamSlots, outs, pos)
	}

	// A token-list body (`filter [body] data`): the body consumes its inputs
	// positionally off the stack.
	if !IsConcrete(body) {
		return false
	}
	bodyList, err := AsList(body)
	if err != nil || bodyList.IsNil() {
		return false
	}
	// A body with a flow-control sentinel (break/continue/return) targets an
	// enclosing loop the VM can't reach across the call boundary — keep it on
	// the whole-program fallback path.
	if bodyHasSentinel(body) {
		return false
	}
	inputs := spec.Inputs(args)
	if inputs == nil { //covergate:allow compiler/VM defensive arm; unreachable without a bytecode-level fault (§compiler)
		return false
	}
	bodyToks := bodyList.Slice()

	// Lexical captures: body words resolving to an ENCLOSING fn's binding
	// (a param or body-local of a fn currently being compiled) ride as the
	// closure's captures — resolved here in the enclosing scope, bound into
	// the body unit's trailing slots at invocation. A module/global ref is
	// not a capture (it bakes as a const in the body, or refuses the probe).
	captures := moduleScopeMutableCaptures(r, bodyToks, ComputeCaptures(r, &FnSig{Impl: AQL(bodyToks)}))
	return recordClosureDispatch(r, word, spec, sig, args, bodyToks, inputs, nil, captures, ClosureInValue, extraLamSlots, outs, pos)
}

// tryRecordLambdaClosure compiles a higher-order word's LAMBDA argument
// (`([p] => …)`, an anonymous FnDefInfo) to a closure unit. The lambda's body
// is compiled with the word's per-callback input shape (lambdaCallbackInputs)
// bound to the lambda's NAMED params, so a body that destructures the entry
// (`p.value`, `kv.v`, `acc`+`kv.v`) typechecks. Returns false — leaving the
// refusal to stand — for a shape the word has no lambda convention for, an
// arity mismatch, a capturing lambda (deferred), or a body that does not
// compile.
func tryRecordLambdaClosure(r *Registry, word string, spec CallableSpec, sig *Signature, args []Value, fd *FnDefInfo, extraLamSlots []int, outs []Value, pos SrcPos) bool {
	inputs, shape, ok := lambdaCallbackInputs(r, word, spec, args)
	if !ok {
		return false
	}
	lam, ok := lambdaHookCompatible(fd, inputs, shape)
	if !ok {
		return false
	}
	names := make([]string, len(lam.Params))
	for i := range lam.Params {
		names[i] = lam.Params[i].Name
	}
	// Module-scope MUTABLE-INSTANCE reads in the lambda body (the flex
	// accumulator `def acc (flex [])  walk … (m => [acc (m.path) append])`)
	// ride as closure captures, exactly as the token-body path admits them:
	// the binding's identity is fixed for the dispatch (a compiled body cannot
	// rebind a module-scope name), so the value threaded at OpPushClosure
	// equals every per-run lookup the interpreter's CallAQL makes, and the
	// pointer-backed instance shares mutations across the boundary. See
	// moduleScopeMutableCaptures.
	captures := moduleScopeMutableCaptures(r, lam.body(), nil)
	return recordClosureDispatch(r, word, spec, sig, args, lam.body(), inputs, names, captures, shape, extraLamSlots, outs, pos)
}

// lambdaHookCompatible reports whether a LAMBDA hook value can compile to a
// closure unit against the word's callback `inputs`, returning its single own
// signature. The constraints are the landed lambda-body ones, shared by the
// BODY lambda and the extra hook-slot lambdas (Stage M2d):
//
//   - exactly ONE own signature: an OVERLOADED fn value is dispatched by
//     MatchFnSig at runtime — FirstOwnSig is not necessarily the matched
//     overload, so compiling its body could run the wrong one;
//   - no LEXICAL captures (they would need resolving in this scope and
//     threading onto the closure; MODULE-SCOPE mutable-instance reads are not
//     lexical captures — the callers admit those via
//     moduleScopeMutableCaptures);
//   - no flow-control sentinel in the body;
//   - param count matches the callback inputs, and every declared param TYPE
//     accepts its input — the same membership the runtime MatchFnSig checks
//     at dispatch. A param whose type rejects the shape (`[p:String]` against
//     filter's {key,value} pair, or `[kv:KeyVal]` against a list's plain
//     pair) makes the interpreter raise a callback error; compiling the body
//     anyway would silently keep the element. A map-iteration ENTRY input is
//     a KeyVal (a Map subtype) the carrier conservatively under-types as a
//     plain Map, so any Map-family param is accepted there and only a
//     provably-incompatible param (a scalar, a sibling container) refuses.
func lambdaHookCompatible(fd *FnDefInfo, inputs []Value, shape ClosureInShape) (*Signature, bool) {
	lam, ok := fd.FirstOwnSig()
	if !ok || len(lam.body()) == 0 {
		return nil, false
	}
	own := 0
	for i := range fd.Signatures {
		if !fd.Signatures[i].Fallback {
			own++
		}
	}
	if own > 1 {
		return nil, false
	}
	if len(fd.Captured) > 0 {
		return nil, false
	}
	if bodyToksHaveSentinel(lam.body()) {
		return nil, false
	}
	if len(lam.Params) != len(inputs) {
		return nil, false
	}
	for i := range lam.Params {
		pt := lam.Params[i].Type
		if pt == nil {
			continue
		}
		if shape == ClosureInKeyVal && inputs[i].Parent.ConformsTo(TMap) {
			if !pt.ConformsTo(TMap) && !TMap.ConformsTo(pt) {
				return nil, false
			}
			continue
		}
		if !sigTypeMatches(inputs[i], pt) {
			return nil, false
		}
	}
	return lam, true
}

// recordClosureDispatch is the shared tail of the token and lambda closure
// paths: it resolves the lexical captures, probe-compiles the body in a
// throwaway state (a refusal leaves the real program untouched), then
// real-compiles it and records the dispatch (the body operand lowering to
// OpPushClosure). paramNames is nil for the token form (stack-consumed inputs)
// and the lambda's param names for the lambda form. extraLamSlots are the
// NON-body NoEvalArgs slots holding a LAMBDA hook (walk's ASCEND slot, Stage
// M2d): each compiles to its OWN closure unit under the SAME shared token
// shape (extraNoEvalHookSlots only nominates them on a LambdaSharesTokenShape
// word) and rides as a second opClosure operand.
func recordClosureDispatch(r *Registry, word string, spec CallableSpec, sig *Signature, args, bodyToks, inputs []Value, paramNames []string, captures []CapturedBinding, shape ClosureInShape, extraLamSlots []int, outs []Value, pos SrcPos) bool {
	// The probe fork below needs the CONCRETE EmitState; both callers only
	// reach here through an active recording state, so a non-EmitState
	// recorder (the inactive no-op) declining is the unreachable belt.
	real, isReal := r.Check.Recorder().(*EmitState)
	if !isReal || real == nil {
		return false
	}
	capOps := make([]emitOperand, len(captures))
	for i, cb := range captures {
		op, ok := real.resolveOperand(cb.Value)
		if !ok { //covergate:allow compiler/VM defensive arm; unreachable without a bytecode-level fault (§compiler)
			return false // an unreachable capture — keep the island path
		}
		capOps[i] = op
	}

	// Extra LAMBDA hooks (walk's ASCEND slot, Stage M2d): validate each against
	// the SAME shared token-shape inputs the body uses (the nominating gate is
	// LambdaSharesTokenShape-only) and resolve its module-scope captures, so
	// the probe/real passes below compile them alongside the body. Any
	// incompatibility declines the whole closure path — the refusal stands.
	type extraHook struct {
		slot  int
		toks  []Value
		names []string
		caps  []CapturedBinding
		ops   []emitOperand
	}
	extras := make([]extraHook, 0, len(extraLamSlots))
	for _, slot := range extraLamSlots {
		fd, isFn := args[slot].Data.(FnDefInfo)
		if !isFn { //covergate:allow compiler/VM defensive arm; unreachable without a bytecode-level fault (§compiler)
			return false
		}
		hookIns, hookShape, insOK := lambdaCallbackInputs(r, word, spec, args)
		if !insOK { //covergate:allow compiler/VM defensive arm; unreachable without a bytecode-level fault (§compiler)
			return false
		}
		lam, lamOK := lambdaHookCompatible(&fd, hookIns, hookShape)
		if !lamOK {
			return false
		}
		names := make([]string, len(lam.Params))
		for i := range lam.Params {
			names[i] = lam.Params[i].Name
		}
		hookCaps := moduleScopeMutableCaptures(r, lam.body(), nil)
		hookCapOps := make([]emitOperand, len(hookCaps))
		for i, cb := range hookCaps {
			op, ok := real.resolveOperand(cb.Value)
			if !ok { //covergate:allow compiler/VM defensive arm; unreachable without a bytecode-level fault (§compiler)
				return false
			}
			hookCapOps[i] = op
		}
		extras = append(extras, extraHook{slot: slot, toks: lam.body(), names: names, caps: hookCaps, ops: hookCapOps})
	}

	// The body compile re-runs the body (the ReturnsFn pass already emitted
	// its diagnostics); drop any it re-emits so counts do not double.
	diagBase := len(r.Check.Diagnostics)
	defer r.Check.TruncateDiagnostics(diagBase)

	// PROBE: compile the body in a throwaway state so a refusal leaves the
	// real program untouched (graceful fall-through to the island). The throwaway
	// is SEEDED with the enclosing fn-unit tables (forkForProbe) so a self-
	// recursive call inside the body — `… msd-go` in an `each` body — resolves to
	// the enclosing in-progress unit instead of re-compiling it in the throwaway
	// (which re-hits the same closure and fails its own residual). The REAL
	// compile below resolves the recursion naturally (the enclosing unit's key is
	// already in the real state).
	// A strip-input word compiles count-agnostic (the runtime nets one value
	// from either admitted shape — stripResidualShapeOK screens the rest).
	countAgnostic := spec.EmptyBodyErrors || spec.StripsUnconsumedInput
	probe := real.forkForProbe()
	r.Check.Emit = probe
	probeUnit, probeOk := compileClosureBody(r, word, spec.BodyOut, countAgnostic, spec.BodyResultTop, bodyToks, inputs, paramNames, captures, shape, pos)
	for _, ex := range extras {
		if !probeOk { //covergate:allow compiler/VM defensive arm; unreachable without a bytecode-level fault (§compiler)
			break
		}
		_, exOk := compileClosureBody(r, word, spec.BodyOut, countAgnostic, spec.BodyResultTop, ex.toks, inputs, ex.names, ex.caps, shape, pos)
		probeOk = probeOk && exOk
	}
	r.Check.Emit = real
	if !probeOk {
		return false
	}
	// Whole-residual multi-out exactness (BodyOutResidual, N > 1): the dispatch
	// seats exactly len(outs) results, so the compiled unit's residual count
	// must be statically EXACT and equal — a variadic merge, a dynamic-apply
	// tail, or any count mismatch declines to the refusal path. 0/1-out bodies
	// keep today's shapes untouched (a diverging body's single Error out, a
	// dynTrail apply netting one value).
	if spec.BodyOut == BodyOutResidual && len(outs) > 1 && !closureResidualExact(probe, probeUnit, len(outs)) {
		return false
	}
	// Strip-input shape screen (`error`): admit only the two residual shapes
	// the runtime nets ONE value from — everything else declines to the
	// refusal path, exactly as before.
	if spec.StripsUnconsumedInput && !stripResidualShapeOK(probe, probeUnit) {
		return false
	}

	// REAL: compile the body into the program (deterministic success after a
	// clean probe), then record the dispatch with the body as a closure.
	unit, realOk := compileClosureBody(r, word, spec.BodyOut, countAgnostic, spec.BodyResultTop, bodyToks, inputs, paramNames, captures, shape, pos)
	if !realOk || unit < 0 { //covergate:allow compiler/VM defensive arm; unreachable without a bytecode-level fault (§compiler)
		return false
	}
	var extraOps map[int]emitOperand
	for _, ex := range extras {
		exUnit, exOk := compileClosureBody(r, word, spec.BodyOut, countAgnostic, spec.BodyResultTop, ex.toks, inputs, ex.names, ex.caps, shape, pos)
		if !exOk || exUnit < 0 { //covergate:allow compiler/VM defensive arm; unreachable without a bytecode-level fault (§compiler)
			return false
		}
		if extraOps == nil {
			extraOps = map[int]emitOperand{}
		}
		extraOps[ex.slot] = emitOperand{kind: opClosure, closureUnit: exUnit, closureCaps: ex.ops}
	}
	return real.RecordClosureCall(word, sig, args, spec.BodyPos, unit, capOps, extraOps, outs, pos)
}

// closureResidualExact reports whether a probe-compiled closure unit's residual
// is statically EXACT with exactly want outputs: no variadic merge (`if c []
// [a b]`), no dynamic-apply tail (dynTrailArity), no dyn-frame window
// (dynFrameW), and want resolved residual operands. A whole-residual dispatch
// (`do`, BodyOutResidual) seats want results at the call site, so a unit whose
// runtime count could differ from the check run's must decline to the refusal
// path rather than mis-seat the simulated stack.
func closureResidualExact(es *EmitState, unit, want int) bool {
	if es == nil || unit < 0 || unit >= len(es.fnRecs) {
		return false
	}
	rec := es.fnRecs[unit]
	return !rec.variadic && rec.dynTrailArity == 0 && rec.dynFrameW == 0 && len(rec.outOps) == want
}

// stripResidualShapeOK reports whether a strip-input word's probe-compiled
// closure unit leaves one of the two residual shapes its handler nets ONE
// runtime value from: a single-value residual (the body consumed or replaced
// the input — including the empty `error []` body, whose residual is the
// input itself), or a 2-value residual whose BOTTOM is the unconsumed input
// (param local 0) that the handler's identity probe strips (errorHandler's
// stack-neutrality rule). A diverging body never RETs (the re-raise
// propagates out of InvokeBody in both engines) and is exempt. Anything else
// — variadic, dynamic-apply tails, deeper residuals — declines.
func stripResidualShapeOK(es *EmitState, unit int) bool {
	if es == nil || unit < 0 || unit >= len(es.fnRecs) {
		return false
	}
	rec := es.fnRecs[unit]
	if rec.frag != nil && fragDiverges(rec.frag) {
		return true
	}
	if rec.variadic || rec.dynTrailArity > 0 || rec.dynFrameW > 0 {
		return false
	}
	switch len(rec.outOps) {
	case 1:
		return true
	case 2:
		op := rec.outOps[0]
		return op.kind == opLocal && op.idx == 0
	}
	return false
}

// lambdaCallbackInputs returns the representative input carriers a higher-order
// word presents to a LAMBDA callback — the word's callback shape, which differs
// from the token-quotation form (spec.Inputs) — plus the runtime ClosureInShape
// the driving handler reads to present each entry:
//
//   - filter over a LIST: one {key, value} pair Map (the element via `.value`).
//   - filter/each over a MAP: one KeyVal {k v i n} (the value via `.v`).
//   - fold (init form) / scan over a MAP: (accumulator, KeyVal).
//
// The carriers are GENERALISED (field types, not one call's values) so the body
// is compiled once for every entry. ok is false for a shape with no lambda
// convention (the caller then leaves the refusal to stand): a list each/fold,
// a no-init map fold, and for-each (whose check-mode output count does not match
// its 0-result runtime) all stay on the refusal path.
func lambdaCallbackInputs(r *Registry, word string, spec CallableSpec, args []Value) ([]Value, ClosureInShape, bool) {
	// A word whose lambda callback sees the SAME inputs as its token form
	// (walk's payload map) declares LambdaSharesTokenShape: Inputs(args) IS the
	// lambda convention, no per-word shape below — and no data-follows-body
	// operand layout is assumed (walk's data PRECEDES its hooks).
	if spec.LambdaSharesTokenShape {
		if spec.Inputs == nil {
			return nil, ClosureInValue, false
		}
		if ins := spec.Inputs(args); ins != nil {
			return ins, ClosureInValue, true
		}
		return nil, ClosureInValue, false
	}
	if spec.BodyPos+1 >= len(args) {
		return nil, ClosureInValue, false
	}
	data := args[spec.BodyPos+1] // the data operand follows the body operand
	if !IsConcrete(data) {
		return nil, ClosureInValue, false
	}
	elem := DataListElemTypeFromValue(data)
	isMap := data.Parent.ConformsTo(TMap)
	isList := data.Parent.ConformsTo(TList)
	switch word {
	case "filter":
		switch {
		case isMap:
			return []Value{keyValCarrier(r, elem)}, ClosureInKeyVal, true
		case isList:
			return []Value{pairCarrier(elem)}, ClosureInValue, true
		}
	case "each":
		if isMap {
			return []Value{keyValCarrier(r, elem)}, ClosureInKeyVal, true
		}
	case "fold":
		// Init form only (`init fold (lambda) {m}` → args [lambda, map, init]):
		// the accumulator carries the seed's type, the entry rides as a KeyVal.
		if isMap && len(args) > spec.BodyPos+2 {
			acc := args[spec.BodyPos+2]
			return []Value{NewCarrier(acc.Parent), keyValCarrier(r, elem)}, ClosureInKeyVal, true
		}
	case "scan":
		// scan seeds the accumulator from the first value (no init operand): the
		// accumulator carries the value type, the entry rides as a KeyVal.
		if isMap {
			return []Value{NewCarrier(elem), keyValCarrier(r, elem)}, ClosureInKeyVal, true
		}
	}
	return nil, ClosureInValue, false
}

// pairCarrier builds a representative {key, value} pair Map carrier — the shape
// filter's list Function form hands its callback (key = the index, value = the
// element). Field VALUES are carriers (Integer key, elem value) so the compiled
// body reads field TYPES, never one call's concrete values.
func pairCarrier(elem *Type) Value {
	om := NewOrderedMap()
	om.Set("key", NewCarrier(TInteger))
	om.Set("value", NewCarrier(elem))
	return NewValueRaw(TMap, MapPayload{M: om})
}

// keyValCarrier builds a representative KeyVal {k v i n} carrier — the shape the
// map Function forms (filter/each/fold/scan over a map) hand their callback. The
// value field carries the map's common value type; k/i/n carry String/Integer/
// Integer. Tagged Node/Map/KeyVal when that type is registered (the language
// layer), else a plain Map carrier — either way the body's `kv.v`/`kv.i` reads
// resolve by ordinary dotted access.
func keyValCarrier(r *Registry, elem *Type) Value {
	om := NewOrderedMap()
	om.Set("k", NewCarrier(TString))
	om.Set("v", NewCarrier(elem))
	om.Set("i", NewCarrier(TInteger))
	om.Set("n", NewCarrier(TInteger))
	t := TMap
	if r != nil {
		if kv := r.Types.Lookup("Node/Map/KeyVal"); kv != nil {
			t = kv
		}
	}
	return NewValueRaw(t, MapPayload{M: om})
}

// extraNoEvalHookSlots classifies every NON-body NoEvalArgs operand of a
// Callable word (walk's optional ASCEND hook — the only such slot today) for
// riding next to the compiled body closure. The driving handler classifies
// such a hook at run time and, for a concrete list, runs its element snapshot
// as a token QUOTATION in a sub-engine (walkClassifyHook → runQuotationBody),
// where a NAME resolves against the REGISTRY — but the compiled program holds
// top-level value defs as VM frame locals, so a name-bearing hook would
// diverge (the same registry asymmetry execBodyRefsNames defends the body
// slot against). Two shapes are admitted:
//
//   - a flex reference whose runtime tokens are PROVABLY EMPTY
//     (emptyFlexHookOperand) — it rides as a plain value operand;
//   - on a LambdaSharesTokenShape word, a LAMBDA (a concrete FnDefInfo) —
//     returned in `lamSlots` for recordClosureDispatch to compile to its OWN
//     closure unit (Stage M2d, the two-hook walk row): the handler classifies
//     a compiled closure per hook slot, so a second closure operand runs
//     byte-identically to the interpreter's per-node lambda call. The full
//     lambda constraints (single own sig, capture-free, param-compatible) are
//     enforced at compile time there; a decline leaves the refusal standing.
//
// Anything else declines — the caller then leaves the Stage-2 refusal
// standing (an INERT token hook needs no admission here: when the closure
// path declines, the existing noEvalBodiesInertScoped const bake still
// applies).
func (es *EmitState) extraNoEvalHookSlots(sig *Signature, spec CallableSpec, args []Value) (lamSlots []int, ok bool) {
	for i := range args {
		if i == spec.BodyPos || !sig.NoEvalArgs[i] {
			continue
		}
		if es.emptyFlexHookOperand(args[i]) {
			continue
		}
		if _, isFn := args[i].Data.(FnDefInfo); isFn && spec.LambdaSharesTokenShape {
			lamSlots = append(lamSlots, i)
			continue
		}
		return nil, false
	}
	return lamSlots, true
}

// emptyFlexHookOperand reports whether v is a flex reference that is PROVABLY
// EMPTY when the word being recorded dispatches, so its classify-time hook
// snapshot (`bl.Slice()` at handler entry, before any traversal) is a
// zero-length token list — and stays zero-length for the whole call: the
// descend closure's in-traversal appends land beyond the snapshot's length.
// Every hook invocation over it then runs ZERO tokens, byte-identical to the
// interpreter classifying the SAME (empty) flex through the same handler —
// this is the `def acc (flex [])  walk … (m => [acc … append])  acc` corpus
// shape, where the trailing accumulator reference is collected as the ascend
// hook. The proof obligations, all conservative:
//
//   - flex-typed operand (list or map family; an empty flex MAP hook raises
//     the same handler-side walk_error in both engines);
//   - produced by the MAIN-registry `flex` native over EMPTY const containers
//     (sig pointer identity, so a module/user shadow never matches);
//   - recorded at TOP-LEVEL straight-line scope (one unit, one frame):
//     recording order IS runtime order and the dispatch runs exactly once —
//     inside a loop/branch/fn a later-recorded (or unrecorded-iteration)
//     mutator could precede this dispatch at run time;
//   - NO event recorded since the construction (pr.seq == es.seq): every
//     runtime effect between the two — a mutator call, a user call, a branch,
//     an island — records an event or refuses, so the flex still holds its
//     constructed (empty) contents when the word dispatches.
func (es *EmitState) emptyFlexHookOperand(v Value) bool {
	if len(es.units) != 1 || len(es.frames) != 1 {
		return false
	}
	p := v.Parent
	if p == nil || (!p.ConformsTo(TFlexList) && !p.ConformsTo(TFlexMap)) {
		return false
	}
	pr, ok := es.producedBy[v.ID]
	if !ok || pr.idx != 0 { //covergate:allow compiler/VM defensive arm; unreachable without a bytecode-level fault (§compiler)
		return false
	}
	evs := es.frames[0]
	// Trailing evDynBind events are def-site BOOKKEEPING (the dynamic-scope
	// binder trail): their only runtime effect is a registry install, which
	// cannot touch the flex contents — skip them when locating the last
	// EFFECTFUL event, which must still be this flex's construction (the
	// "no event recorded since" proof, dyn-bind-tolerant).
	i := len(evs) - 1
	for i >= 0 && evs[i].kind == evDynBind {
		i--
	}
	if i < 0 {
		return false
	}
	last := evs[i]
	if last.seq != pr.seq || last.kind != evCall || last.call.word != "flex" {
		return false
	}
	// The matched sig must be the MAIN registry's `flex` binding (pointer
	// identity into its sig backing array) — a same-named module inner native
	// or user fn must never satisfy this proof.
	if es.reg == nil {
		return false
	}
	fn := es.reg.Lookup("flex")
	if fn == nil {
		return false
	}
	sigOK := false
	for i := range fn.Signatures {
		if &fn.Signatures[i] == last.call.sig {
			sigOK = true
			break
		}
	}
	if !sigOK || len(last.call.ops) == 0 { //covergate:allow compiler/VM defensive arm; unreachable without a bytecode-level fault (§compiler)
		return false
	}
	for _, op := range last.call.ops {
		if op.kind != opConst || op.idx < 0 || op.idx >= len(es.consts) { //covergate:allow compiler/VM defensive arm; unreachable without a bytecode-level fault (§compiler)
			return false
		}
		if !emptyContainerConst(es.consts[op.idx]) {
			return false
		}
	}
	return true
}

// emptyContainerConst reports whether v is a concrete container literal with
// ZERO members ([] or {}) — the only construction input emptyFlexHookOperand
// accepts, so the flex provably starts with no elements.
func emptyContainerConst(v Value) bool {
	if !IsConcrete(v) {
		return false
	}
	if lst, err := AsList(v); err == nil && !lst.IsNil() {
		return lst.Len() == 0
	}
	if m, err := AsMap(v); err == nil && m != nil {
		return m.Len() == 0
	}
	return false
}

// bodyToksHaveSentinel reports whether a lambda body's token slice contains a
// flow-control sentinel (break/continue/return) — the token-slice form of
// bodyHasSentinel, for a lambda whose Body is already []Value.
func bodyToksHaveSentinel(toks []Value) bool {
	found := false
	WalkBodyWords(toks, func(w WordInfo, _ Value) {
		switch w.Name {
		case "break", "continue", "return":
			found = true
		}
	})
	return found
}
