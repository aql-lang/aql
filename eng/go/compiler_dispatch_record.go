package eng

// The dispatch-outcome recording family — the compiler's half of check's
// carrierResults: constant folding, poly events, dyn-body closures, and
// fallback islands. Extracted from carrier.go in Stage 0b of the
// four-piece split (design/ENG-FOUR-PIECE.0.md): these functions hold
// the compiler piece's concrete *EmitState* and become the
// DispatchRecorder implementation when the packages cut (seam S3).

// tryFoldScalarConst const-folds a CompileScalarFold dispatch whose operands
// are ALL inert consts, by running the real handler concretely — twice, with
// the same determinism-agreement guard tryFoldModuleConst uses — and
// returning the concrete result when it is itself an inert const. An
// erroring dispatch (family-restricted ordering over cross-family operands)
// declines, keeping the ordinary diagnostic path. See CompileScalarFold
// (value.go) for the motivation (concrete-condition folding).
func tryFoldScalarConst(r *Registry, sig *Signature, args []Value) (Value, bool) {
	if sig == nil || !sig.CompileEffect.Has(CompileScalarFold) ||
		sig.dispatchHandler() == nil || len(sig.NoEvalArgs) > 0 || len(args) == 0 {
		return Value{}, false
	}
	for _, a := range args {
		if !scalarFoldOperand(a) {
			return Value{}, false
		}
	}
	one, ok := concreteHandlerEval(r, sig, args)
	if !ok {
		return Value{}, false
	}
	two, ok := concreteHandlerEval(r, sig, args)
	if !ok || !constFoldAgrees(one, two) {
		return Value{}, false
	}
	if !isInertConst(one) {
		return Value{}, false
	}
	return one, true
}

// recordDispatchOutcome is the ONE seam where the check pass hands a
// resolved dispatch to the bytecode recorder — every check-mode dispatch
// records through exactly one of the fold/closure/poly/fallback specialists
// or the generic RecordCall. Pure type analysis lives above this call;
// everything below it is compile-pass machinery (emit.go). Keeping the
// boundary to a single named call is the first step of the Emit/check
// decoupling (checker review, Tier 2).
func recordDispatchOutcome(r *Registry, word string, sig *Signature, args, out []Value, pos SrcPos, ownerReg *Registry) {
	// Tag a get-family read that surfaces a fn-valued container member so the
	// stranded-member-fn guard can recognise its dynamic(Any) result downstream
	// (design/EDGE-SPEC-FINDINGS.0.md §2). Independent of how the read itself
	// records — the tag rides the result ID onto the tape.
	if len(out) == 1 && (isGetWord(word) || isGetrWord(word)) {
		if es := r.Check.Recorder(); es.active() {
			if readsFnMember(args) {
				// The member VALUE rides the tag when the read pinpoints it (a
				// concrete container + key) so the §3 arrival-apply model can
				// claim its signature's window; a computed-key read tags alone
				// and the model declines (the stranded-fn refusal stands).
				member, _ := readFnMemberValue(args)
				es.noteMemberFnRead(out[0].ID, member)
			} else if rec, isEmit := es.(*EmitState); isEmit {
				// A CONSTRUCTED-instance receiver is a carrier here (the make
				// result — payload inspection sees only the schema), so the
				// member rides the construction-time fnMemberFields note
				// instead: tagging it routes the landing through the same §3
				// model / stranded-fn guard as a concrete-container read —
				// the fix for the pre-existing `o.f 21 eq 42` stranded-apply
				// miscompile.
				if member, ok := rec.instanceFnMember(args); ok {
					rec.noteMemberFnRead(out[0].ID, member)
				}
			}
		}
	}
	// NUR037 refusal: a code body naming a FN-LOCAL fn cannot lower soundly
	// on any path below — the closure probe, the island span, and the plain
	// CALL_NATIVE const-bake all bake a NAME the VM's runtime registry never
	// binds (the enclosing body's `def … fn` is compiled away), turning a
	// working program into undefined_word. Refuse the whole program up front
	// so the interpreter owns it. A dispatch a structured ReturnsFn hook
	// already recorded (case's branch-chain desugar — alreadyProduced)
	// lowered its clause bodies as inline events with no name bake, so it is
	// not a leak path and keeps compiling; see bodyRefsFnLocalFn for the
	// scope rule.
	if es := r.Check.Recorder(); es.active() && !(len(out) == 1 && es.alreadyProduced(out[0].ID)) {
		if name, hit := bodyRefsFnLocalFn(r, sig, args); hit {
			es.MarkUncompilable("code-body names fn-local fn `" + name + "` at `" + word +
				"` (a compiled unit cannot resolve an enclosing fn's local fn binding)")
			return
		}
	}
	if !tryRecordMethodApply(r, word, args, out, pos) &&
		!tryFoldStaticIndex(r, word, args, out) &&
		!tryFoldModuleConst(r, word, sig, args, out) &&
		!tryRecordDeferredList(r, sig, out) &&
		!tryRecordClosure(r, word, sig, args, out, pos) &&
		!tryRecordDynBody(r, word, sig, args, out, pos) &&
		!tryRecordPoly(r, word, sig, args, out, pos, false, ownerReg, false, nil) &&
		!tryRecordFallback(r, word, sig, args, out, pos) {
		quoteInertOK := quoteOperandInertOK(r, word, sig, args)
		// A CompileRunsBodyIsolated word (Test.check-prop) whose dynamic operands
		// all conform to its single sig (dynInputsProven) bakes a faithful CALL_
		// NATIVE; its declared-Map return rides as a dynamic (declared-Any) output
		// exactly as dynOutNativeOK admits for concrete-arg builtins — the handler
		// produces the real result value in both modes.
		forceDynOut := dynOutNativeOK(r, word, sig, args, out) || quoteInertOK ||
			r.Check.Recorder().dynInputsProven(sig, args)
		r.Check.Recorder().RecordCall(word, sig, args, out, pos, forceDynOut, quoteInertOK)
	}
}

// tryFoldStaticIndex folds a `get` / `getr` over a CONCRETE list with a STATIC,
// in-range, non-negative integer index to the element's existing operand —
// emitting nothing, since the result already has a compiled home. Its purpose is
// `args.N` (= `get N args`): the args projection is the list of param carriers,
// whose IDs are the frame locals, so `args.0` folds to PUSH_LOCAL 0. The fold is
// general but self-gating: it only fires when the element resolves to an operand
// (a local or interned const), so a literal-list element that was never interned
// declines and the normal poly/get path stands. outs[0] is rewritten to the
// element carrier so the value flowing on has the element's identity.
func tryFoldStaticIndex(r *Registry, word string, args, outs []Value) bool {
	es := r.Check.Recorder()
	if !es.active() || (!isGetWord(word) && !isGetrWord(word)) || len(args) != 2 || len(outs) != 1 {
		return false
	}
	key, recv := args[0], args[1]
	if !recv.Parent.ConformsTo(TList) || !IsConcrete(recv) ||
		!key.Parent.ConformsTo(TInteger) || !IsConcrete(key) {
		return false
	}
	n, err := AsInteger(key)
	if err != nil || n < 0 {
		return false
	}
	lst, lerr := AsList(recv)
	if lerr != nil || lst.IsNil() || int(n) >= lst.Len() {
		return false
	}
	elem := lst.Get(int(n))
	if _, ok := es.resolveOperand(elem); !ok {
		return false // element has no compiled home (e.g. an un-interned literal) — decline
	}
	outs[0] = elem
	return true
}

// The PURE reader words whose result over a compile-time-known module value is a
// compile-time constant (get / getr / convert / typeof / is / size / has) now
// DECLARE CompileModuleFold on their NativeFunc (lang layer). `import` binds an
// immutable, deterministic namespace map / Module instance, so a read over it
// always yields the same value — baked rather than re-read at run time. See
// tryFoldModuleConst.

// isModuleFamilyValue reports whether v is a concrete module value — an
// Ideal/Module descriptor, or a module NAMESPACE (a plain Map carrying the
// module-namespace facet — the value `import` binds; NUR038 retired the
// Ideal/ModuleExport wrapper type). The descriptor is identified by the
// kernel-declared TModule (moduletype.go — the former string-path probe
// is gone), the namespace by its kernel facet. These values are
// immutable and produced deterministically by `import`, so a pure read
// of one is a compile-time constant (runtime export growth is
// ledger-modelled — module_export_growth.go).
func isModuleFamilyValue(v Value) bool {
	if !IsConcrete(v) || v.Parent == nil {
		return false
	}
	return ModuleNSOf(v) != nil || v.Parent.Equal(TModule)
}

// constFoldAgrees reports whether two const-fold probe evaluations produced
// the SAME bakeable value, compared by CanonValue — the exact structural key
// the const interner dedups by (emit.go's constIdx). It is the shared
// determinism gate behind every twice-and-compare const-bake
// (tryFoldModuleConst, the macroexpand splice in carrierResults, and the
// engine's constFoldContainerVal): a clock / rand / mutation-bearing read
// whose two probes drift renders a different canon and is refused, so no
// nondeterministic value is ever frozen into the program.
//
// CanonValue, NOT String(): String() is a DISPLAY rendering that conflates
// values which bake DIFFERENTLY — a bare type node vs the string of its name
// (`Integer` vs 'Integer'), an atom vs a same-spelled string (name/q vs
// 'name'), an Integer vs an equal-magnitude Float, a fn vs a same-shaped fn
// with a different body — so two genuinely divergent probes could
// String()-match and freeze an UNSOUND const. CanonValue is the bake
// identity itself ("same canon" ⟺ "interns to one const"), and it is no
// coarser than String() on any bakeable shape, so a legitimately
// deterministic fold still agrees (no coverage change) while the
// conflations String() hid can no longer slip a frozen value through.
func constFoldAgrees(a, b Value) bool { return CanonValue(a) == CanonValue(b) }

// tryFoldModuleConst const-folds a PURE read whose result is a compile-time
// constant because it depends only on a module value (immutable, import-bound)
// plus inert consts / type operands — `MathUtil.$name` -> 'MathUtil',
// `convert Map Foo` -> the export map, `MathUtil.$module.name` ->
// 'boru:math-util', `typeof MathUtil.$module` -> Module. The checker's recorded
// RESULT is NOT enough: a word like `convert`/`is`/`typeof` returns its declared
// TYPE (a Map carrier, a Boolean carrier) in check mode, not the concrete value,
// so baking that would render `Map` where the interpreter rebuilds `{a:1 b:2}`.
// Instead the dispatch is RE-EVALUATED concretely (check mode off) — twice, and
// only folded when both runs agree on the same resolvable value (an inert const
// or a bare type node), so a clock/rand/mutation-bearing read never freezes.
// The fold emits nothing; the concrete result rides as that const / type
// operand — the get/getr module-RESOLUTION elision (RecordCall), generalised to
// the synthetic accessors and projections whose result is data, not a fn.
// Declines unless the word is a known pure reader with a direct handler, at
// least one operand is a module value, and every other operand is itself a
// compile-time constant (an inert const or a type node) — a runtime operand
// never folds.
func tryFoldModuleConst(r *Registry, word string, sig *Signature, args, outs []Value) bool {
	es := r.Check.Recorder()
	if !es.active() || sig == nil || !sig.CompileEffect.Has(CompileModuleFold) || len(outs) != 1 ||
		sig.dispatchHandler() == nil || len(sig.NoEvalArgs) > 0 {
		return false
	}
	sawModule := false
	for _, a := range args {
		switch {
		case isModuleFamilyValue(a):
			sawModule = true
		case IsBareTypeNode(a):
			// a type operand (the target of `convert Map …` / `… is Module`)
		case isInertConst(a):
			// an inert const operand (a quoted key atom, a scalar)
		default:
			return false // a runtime / non-const operand — not a compile-time fold
		}
	}
	if !sawModule {
		return false
	}
	one, ok := concreteHandlerEval(r, sig, args)
	if !ok {
		return false
	}
	two, ok := concreteHandlerEval(r, sig, args)
	if !ok || !constFoldAgrees(one, two) {
		return false
	}
	// A `get` that resolves to None is a MISSING key — but a module's keyspace
	// can GROW at runtime: minilang/parselang `register` installs new exports
	// (`parse_<name>`) AFTER the check pass folded the program. Folding the
	// missing-key get to None bakes a stale absence: the compiled program then
	// pushes None + leaves the call's args on the stack instead of dispatching
	// the registered word, diverging from the interpreter (which sees the
	// registered key). Decline the fold so the get stays dynamic and the
	// program falls back / islands faithfully. A PRESENT key (any non-None
	// value) folds as before; this only blocks the absent-key case.
	//
	// EXCEPTION (Phase 6 M3): a receiver whose export map carries a growth
	// LEDGER — every program-reachable runtime grower is check-modelled — and
	// whose ledger proves the requested key is NOT among this pass's possible
	// installs folds the stable absence (`MiniLang.Gen` after registering a
	// non-filter kind `gen` is None on every run, because a non-filter kind
	// mints no member type). An unregistered map, a poisoned ledger, or a key
	// a grower may add keeps the decline. See module_export_growth.go.
	if isGetWord(word) && IsNoneShape(one) && !moduleExportAbsenceStable(r, args) {
		return false
	}
	switch {
	case isInertConst(one):
		outs[0] = one // ride as an inert const
	case IsBareTypeNode(one) && one.ID != "":
		outs[0] = one // ride as a type operand (OpPushType)
	default:
		return false
	}
	return true
}

// concreteHandlerEval runs sig.dispatchHandler() on the already-resolved args with check
// mode OFF, so a pure reader produces its REAL value rather than the declared-
// type carrier the check-mode ReturnsFn emits. Nothing is recorded (the handler
// is called directly, off the emit path) and the def stack is snapshotted /
// restored so a stray binding cannot leak. Returns the single result when it is
// a concrete value or a bare type node (typeof's type literal). Mirrors
// concreteEvalOnce, but dispatches the one matched native instead of re-running
// a token stream — the args are already in sig order.
func concreteHandlerEval(r *Registry, sig *Signature, args []Value) (Value, bool) {
	snap := r.Defs.Snapshot()
	prev := r.Check.Mode
	r.Check.Mode = false
	res, err := sig.dispatchHandler()(args, nil, nil, r)
	r.Check.Mode = prev
	r.Defs.Restore(snap)
	if err != nil || len(res) != 1 {
		return Value{}, false
	}
	if IsConcrete(res[0]) || (IsBareTypeNode(res[0]) && res[0].ID != "") {
		return res[0], true
	}
	return Value{}, false
}

// dynOutNativeOK reports whether a dispatch with a DYNAMIC output but CONCRETE
// args may still bake a plain CALL_NATIVE despite the dynamic result. Concrete
// args mean the checker RESOLVED the sig by real matching (not widening), so a
// dynamic output is just a declared-Any return (e.g. unify's [Any, Boolean]),
// not a best-guess sig — for a CORE builtin native the handler runs faithfully.
// The dynamic result is still registered, so any downstream TYPED consumer of
// it refuses via the dynamic-input guard, keeping it contained. Mirrors
// tryRecordPoly's safety (core sig, no meta/fn-value), and is the escape hatch
// RecordCall's anyDynamicCarrier(outs) refusal consults via forceDynOut.
func dynOutNativeOK(r *Registry, word string, sig *Signature, args, outs []Value) bool {
	es := r.Check.Recorder()
	if !es.active() || sig == nil || len(outs) == 0 {
		return false
	}
	// Concrete args + dynamic output only — a dynamic INPUT means the sig was
	// widened (a guess), which stays refused.
	if anyDynamicCarrier(args) || !anyDynamicCarrier(outs) {
		return false
	}
	if sig.CompileEffect.Has(CompileFallbackBody) {
		return false
	}
	// Meta / re-stepping shapes never bake (RecordCall refuses them regardless;
	// screen here so they don't slip through forceDynOut).
	if sig.fnFrame() != nil || sig.fullStack() || sig.runInCheckMode() || len(sig.QuoteArgs) > 0 {
		return false
	}
	// A code-body (NoEvalArgs) native bakes only when its bodies are INERT consts
	// with no enclosing-loop sentinel — the SAME screen RecordCall's code-body
	// refusal uses (noEvalBodiesInert), not a blanket NoEvalArgs exclusion. An
	// inert body bakes as a code-as-data const and the handler sub-runs it
	// faithfully (`await` runs its parallels; a body-running native runs its
	// body), so the dynamic (declared-Any) result is sound to bake. A non-inert
	// or sentinel-bearing body stays refused.
	if len(sig.NoEvalArgs) > 0 && !noEvalBodiesInert(sig, args) {
		return false
	}
	for _, t := range sig.ArgTypes() {
		if t != nil && t.ConformsTo(TFunction) {
			return false
		}
	}
	for _, a := range args {
		if _, ok := a.Data.(FnDefInfo); ok {
			return false
		}
	}
	// The VM bakes this exact sig and calls sig.dispatchHandler() DIRECTLY, so it must be
	// a REAL native binding: the word's own main-registry BUILTIN sig, OR a
	// trivial-delegation module inner-native sig reached via dot-access
	// (`StructUtil.clone …`). Both are sound to bake with the main registry —
	// for a module inner native the interpreter ALSO dispatches via execMatch
	// on the main engine (the wrapper's trivial delegation), so the call is
	// identical. The IsBuiltinWord gate on the core path is load-bearing: a
	// user `def ifu (usurp if)` makes r.Lookup("ifu") return the usurp-MODIFIED
	// if sig (pointer-equal to the match) — but ifu is not a builtin, so it is
	// excluded here and stays refused (a usurp'd if re-steps and returns
	// tape-coupled values). A usurp synthetic also matches no module export.
	if r.IsBuiltinWord(word) {
		if fn := r.Lookup(word); fn != nil {
			for i := range fn.Signatures {
				if &fn.Signatures[i] == sig {
					return true
				}
			}
		}
	}
	return isModuleInnerSig(r, word, sig)
}

// quoteOperandInertOK reports whether a dispatch with implicit-quote operands
// (sig.QuoteArgs) may still bake a plain CALL_NATIVE despite the quoted
// positions. It holds only for a MODULE INNER NATIVE (`Pkg.word`, confirmed by
// pointer identity) whose every quoted operand is an inert Atom const — the
// query DSL's table-name operands (`Query.from people`, `Query.join visits`):
// the name is captured unevaluated as a symbol the handler resolves at run time.
// This is the QuoteArgs analogue of dynOutNativeOK and of the get/getr/set
// exemption in RecordCall: the inner native is reached via the wrapper's trivial
// delegation, so the interpreter dispatches the SAME handler via execMatch on
// this engine, and a baked Atom is the same value either way. Restricting to
// module inner natives keeps it off the core meta words (usurp / force-arity /
// ref-family) whose quoted operands drive re-stepping dispatch, and off any
// user word (those refuse earlier as a user-fn call). Mutation-safety holds:
// the query builders return fresh lazy-query values, they do not mutate a
// pooled const.
func quoteOperandInertOK(r *Registry, word string, sig *Signature, args []Value) bool {
	if sig == nil || len(sig.QuoteArgs) == 0 {
		return false
	}
	// Meta / re-stepping / code-body shapes never bake (RecordCall refuses them
	// regardless; screen here so they cannot slip through this exemption).
	if sig.fnFrame() != nil || sig.fullStack() || sig.runInCheckMode() || len(sig.NoEvalArgs) > 0 {
		return false
	}
	// A word that DECLARES CompileQuoteInert (quote / codequote / raise / timeout
	// / interval): its quoted operand is inert data the handler consumes verbatim
	// — a quoted symbol, or a quoted code body held as data — so it bakes as a
	// plain CALL_NATIVE once every quoted operand is an inert const (an Atom, or a
	// quoted code list). The VM runs the same handler over the same baked value.
	// Unlike the module-inner branch below, this admits a non-Atom inert operand
	// (a `[body]` list) and a core builtin, but a non-inert quoted operand still
	// declines so the program falls back.
	if sig.CompileEffect.Has(CompileQuoteInert) {
		for i := range args {
			if sig.QuoteArgs[i] && !isInertConst(args[i]) {
				return false
			}
		}
		return true
	}
	for i := range args {
		if !sig.QuoteArgs[i] {
			continue
		}
		if _, ok := args[i].Data.(AtomPayload); !ok || !isInertConst(args[i]) {
			return false
		}
	}
	return isModuleInnerSig(r, word, sig)
}

// isModuleInnerSig reports whether sig is a native signature exported by a
// LOADED module's trivial-delegation wrapper for word — i.e. the inner native
// `word` dispatches when called as `Pkg.word`. The wrapper (an FnDefInfo
// carrying its sub-Registry) lives in the module's export map; the inner sig is
// `wrapper.Registry.Lookup(word).Signatures`. Pointer-identity confirms sig is
// THAT native, not a usurp-synthetic copy. O(loaded modules × exports) — only
// consulted on a dynamic-output dispatch the core-sig check already missed.
func isModuleInnerSig(r *Registry, word string, sig *Signature) bool {
	if r == nil || r.Modules == nil {
		return false
	}
	for _, md := range r.Modules.loaded {
		for _, em := range md.Exports {
			if em == nil {
				continue
			}
			for _, k := range em.Keys() {
				v, _ := em.Get(k)
				fd, ok := v.Data.(FnDefInfo)
				if !ok || fd.Registry == nil || fd.Name != word {
					continue
				}
				inner := fd.Registry.Lookup(fd.Name)
				if inner == nil {
					continue
				}
				for i := range inner.Signatures {
					if &inner.Signatures[i] == sig {
						return true
					}
				}
			}
		}
	}
	return false
}

// tryRecordPoly records a genuinely-dynamic typed dispatch (get/size/is/
// make/typeof/type-algebra over an Any-widened operand) as an
// OpCallNativePoly that re-matches the word's signatures at run time — the
// SAME first-match the interpreter takes — instead of islanding through a
// sub-engine (plan P3). Returns false (leaving the island path) for a
// concrete-operand call, a non-core sig, or an operand of unknown provenance.
//
// disjunctStraddle marks the OTHER legitimate poly trigger: a STRICT (non-
// dynamic) disjunct operand that straddles the word's signatures
// (disjunctPartitionReturns) — e.g. `5 is (tnot (Integer gt 0))`, where the
// complement type reaches more than one `is` overload. The runtime value is a
// single concrete alternative, so the same runtime MatchSignature dispatches it
// faithfully; only the dynamic-only gate is bypassed, every other safety gate
// (core builtin, no meta/fn-value/code-body sig, sig identity, resolvable
// operands) still applies.
// noMatch, when non-nil, is the faithful-raise plan for the runtime no-match
// arm (PolyNoMatchSpec, plan 3c) — derived by the caller at the failed-
// dispatch tape state it recovered from; nil keeps the sound defer.
func tryRecordPoly(r *Registry, word string, sig *Signature, args, outs []Value, pos SrcPos, disjunctStraddle bool, ownerReg *Registry, dynamicRecovery bool, noMatch *PolyNoMatchSpec) bool {
	es := r.Check.Recorder()
	// Any result count records. Multi-result seating rides the same per-index
	// registration RecordCall's generic path uses (setProducedAt), and the VM
	// enforces the recorded result-count claim (PolyRef.NOut) — so `pop`'s
	// [remaining, popped] pair lands as faithfully as a 1-output get.
	if !es.active() || sig == nil {
		return false
	}
	// A pure stack-shuffle word (dup/swap/over/…) is owned by the shuffle-
	// elision path (recordShuffleElided): it seats residual values by identity,
	// and a poly event would mint fresh result IDs that break that residual
	// identity. Decline here so the shuffle path keeps them.
	if ems, ok := es.(*EmitState); ok && ems.dynamicStackShuffleOK(word, sig) {
		return false
	}
	// matchReg is the registry whose signatures the VM re-matches over: a module
	// sub-registry for a delegation-dispatched module word (`StructUtil.getpath`),
	// else the main registry. The recorder validates the matched sig against it
	// below, and the PolyRef carries it so callPoly looks up the right word.
	matchReg := r
	if ownerReg != nil {
		matchReg = ownerReg
	}
	// Code-body higher-order words compile to closures, not poly.
	if sig.CompileEffect.Has(CompileFallbackBody) {
		return false
	}
	// Only a REGISTERED builtin native — never a user-def fn or a usurp/ref
	// wrapper (`def ifu (usurp if)`), whose dispatch re-steps tokens and
	// returns tape-coupled values the VM cannot push. The runtime
	// MatchSignature re-dispatches over the builtin's own signatures. For a
	// module word the builtin lives in its OWN sub-registry (matchReg).
	if !matchReg.IsBuiltinWord(word) {
		return false
	}
	// Only a genuinely dynamic dispatch (the case the checker could not
	// commit to one overload — an island or a refusal today), a strict-
	// disjunct straddle (disjunctStraddle), a no-signature recovery over an
	// Any-typed operand (dynamicRecovery — matchSignature found no overload
	// because an operand's type is statically unknown, e.g. a List/Map element),
	// or a CoreDefault overload matched over a NON-CONCRETE operand: a
	// CoreDefault is unlocked, so a runtime value whose tag is a strict
	// subtype of the carrier's type (the refinement escape — `refine
	// Boolean` with a merged [Flag Flag] overload) re-matches to the more
	// specific overload; the VM's runtime re-match over the LIVE table is
	// exactly the interpreter's dispatch, so poly keeps parity where a
	// baked CALL_NATIVE would freeze the wrong overload.
	// A fully concrete, single-overload call lowers to a faithful baked
	// CALL_NATIVE, not poly.
	coreDefaultCarrier := sig.CoreDefault && anyNonConcreteOperand(args)
	if !disjunctStraddle && !dynamicRecovery && !anyDynamicCarrier(args) && !anyDynamicCarrier(outs) &&
		!coreDefaultCarrier {
		return false
	}
	// Shapes the VM re-match cannot faithfully dispatch: code bodies,
	// quoted/meta operands, user-fn frames, full-stack words, compile-time
	// words. (a CompileIslandPure get passes these — get's key is its only
	// QuoteArg and is handled below.)
	if sig.fnFrame() != nil || sig.fullStack() || sig.runInCheckMode() || len(sig.NoEvalArgs) > 0 {
		return false
	}
	// get/getr/set/del carry exactly ONE QuoteArg — the inert Atom key — which
	// bakes as a const operand; the rest of the operands resolve normally. The
	// receiver mutation (set: Store/Object/Array; del: FlexMap) and copy-return
	// (Map/List) are faithful under runtime re-match: callPoly runs the same
	// handler over the same concrete receiver the interpreter would. Other
	// quoted-operand words (usurp / ref-family meta) re-step tokens and stay out.
	if len(sig.QuoteArgs) > 0 && !isGetWord(word) && !isGetrWord(word) && word != "set" && word != "del" {
		return false
	}
	// A fn-valued operand or result means a fn-invoking / fn-returning word
	// (apply/usurp, an atom-keyed method get): the value would need dynamic
	// INVOCATION (the fn-value-call boundary, P4). Keep those out of poly.
	for _, t := range sig.ArgTypes() {
		if t != nil && t.ConformsTo(TFunction) {
			return false
		}
	}
	for _, a := range args {
		if _, ok := a.Data.(FnDefInfo); ok {
			return false
		}
	}
	// get/getr over a Map/Object/Module receiver can return a Function FIELD
	// (a method). RecordPolyCall's read guard refuses the risky reads
	// (containerFnAutoDispatchRisk / zeroArgFnOut / instanceFnFieldRisk)
	// unless the landing model owns them (an ANNOTATED shaped read — the
	// recorder then lays an explicit arity-0 OpCallDynMethod after the poly);
	// a method needing args (`r.int`) stays a value and flows to
	// CALL_DYNAMIC — so both atom- and integer-keyed gets poly.
	// CORE-dispatch guard: the matched sig must be the word's binding IN THE
	// REGISTRY the VM will re-match over (matchReg), since callPoly re-matches
	// over matchReg.Lookup's signatures — a sig that is not that registry's
	// binding would re-run a different word of the same name.
	fn := matchReg.Lookup(word)
	if fn == nil {
		return false
	}
	sigOK := false
	for i := range fn.Signatures {
		if &fn.Signatures[i] == sig {
			sigOK = true
			break
		}
	}
	if !sigOK {
		return false
	}
	// Persist matchReg (NOT ownerReg): callPoly re-matches over the PolyRef's
	// registry, and for a module sub-registry native (boru:test `test-record`)
	// dispatched inside a module-fn body, ownerReg arrives nil (native dispatch
	// sets no match.Reg) while matchReg is the sub-registry that actually holds the
	// word — see the comment above. Passing ownerReg left pr.Reg nil, so callPoly
	// looked the word up in the main registry, found 0 sigs, and deferred. Safe for
	// core words too: poly only ever records BUILTINS (guarded above), which exist
	// identically in every registry instance, so matchReg.Lookup always resolves.
	return es.RecordPolyCall(word, args, outs, pos, matchReg, noMatch)
}

// tryRecordDynBody is the universal `do` backstop (the always-compile goal):
// a body the CLOSURE path declined — a COMPUTED (carrier) body whose tokens
// exist only at run time, or a concrete body carrying context-dependent words
// (`args`) — lowers to a plain CALL_NATIVE under the program's DynEnv mode
// instead of refusing. Soundness: the handler's runtime execution (InvokeBody
// → a pooled sub-engine over the concrete tokens) IS the interpreter's own
// semantics, PROVIDED the name/args environment matches — which DynEnv
// guarantees: every def emits its OpBindDynScope twin, every named unit param
// dyn-binds at frame entry, and the VM brackets each CALL_USER frame with an
// args-stack push. The result is marked VARIADIC (the runtime count is the
// body's own residual), so only variadic-absorbing positions (the program
// residual, a drop) consume it; a fixed-arity downstream consumer keeps the
// refusal. A body with a flow-control sentinel stays refused: the sub-run
// cannot propagate break/continue across the handler boundary.
func tryRecordDynBody(r *Registry, word string, sig *Signature, args, outs []Value, pos SrcPos) bool {
	es, _ := r.Check.Recorder().(*EmitState)
	if es == nil || !es.active() || sig == nil || sig.Callable == nil ||
		!sig.CompileEffect.Has(CompileDynBody) || len(outs) == 0 {
		return false
	}
	bp := sig.Callable.BodyPos
	if bp >= len(args) {
		return false
	}
	body := args[bp]
	// A concrete body must be sentinel-free (break/continue target an
	// enclosing loop the handler boundary cannot cross) — including
	// TRANSITIVELY through resolvable callees (bodyHasSentinelDeep: a called
	// user fn's bare break unwinds the CALLER's loop in the interpreter). A
	// computed body's tokens are unknowable — the interpreter faces the same
	// tokens through the same sub-engine, so a runtime sentinel behaves
	// identically there; what differs is only tape-coupled RE-STEPPING, which
	// the sub-run contains entirely. It must also be replay-hazard-free: a
	// capitalised def / import inside the baked body re-runs a registry
	// mutation the check pass already applied and half-rolled-back (the
	// do-unit registry-replay miscompile — see bodyHasReplayHazard).
	if IsConcrete(body) && (bodyHasSentinelDeep(r, body) || bodyHasReplayHazard(body)) {
		return false
	}
	// Every operand must have a compiled home: the body rides as a threaded
	// runtime value (a param local / event result) or an inert const; other
	// operands resolve normally. An unresolvable operand leaves the refusal.
	ops := make([]emitOperand, len(args))
	for i := range args {
		op, ok := es.resolveOperand(args[i])
		if !ok {
			return false
		}
		ops[i] = op
	}
	es.SiteCounts[SiteDynamic]++
	// A GRADUAL (Any-widened) operand could be either overload (do's List
	// code-body vs Map value-eval): record a POLY re-match over the word's
	// own sigs — the runtime value picks the overload exactly as the
	// interpreter's dispatch does. A strictly-List operand bakes the sig.
	call := emitCall{word: word, sig: sig, ops: ops, nout: len(outs), pos: pos}
	if body.Dynamic {
		call.sig = nil
		call.poly = true
	}
	seq := es.appendEvent(emitEvent{kind: evCall, call: call})
	f := es.eventInfo[seq]
	f.dynBodyResult = true
	// A VALUE-EVAL body (`do {map}`) — a CONCRETE, non-dynamic Map arg on the
	// non-fallback (value-eval) sig — produces EXACTLY len(outs) values
	// deterministically (the evaluated map: always one). Its result count is
	// FIXED, not runtime-variable, so it must NOT be marked variadic: the lowerer
	// already lowers it to a fixed-nout CALL_NATIVE (lowerCall never flags a
	// dyn-body event in lw.variadic), and a spurious record-time variadic mark
	// only poisons an enclosing branch/fn residual (armOutVariadic →
	// branchVariadicResult → rec.variadic), refusing a downstream fixed-arity
	// consumer (`print (if c [do {a:1}] [do {b:2}])`) the VM runs correctly. A
	// CODE-BODY (List/CompileFallbackBody) or a GRADUAL (Dynamic) body — whose
	// runtime net count / overload is genuinely variable — keeps the marking.
	fixedValueEval := IsConcrete(body) && !body.Dynamic && !sig.CompileEffect.Has(CompileFallbackBody)
	if !fixedValueEval {
		f.variadicResult = true
	}
	// The dyn-body backstop already marks every code-body result variadic
	// above; consume the ReturnsFn's catch-variadic latch so it cannot leak
	// past this dispatch (L-DO — see catchVariadicFor).
	es.catchVariadicFor(sig)
	es.eventInfo[seq] = f
	// Carrier-identity de-collision, extended to INTRA-event repeats: the
	// modeled outs of a dyn-body sub-run may repeat one value — an unrolled
	// loop body (`do [for 3 [1]]`) models [1 1 1] as the SAME Value, whose
	// shared ID would collapse producedBy to the LAST result index and refuse
	// "call results reordered" at the residual. Unlike the generic RecordCall
	// (which skips same-event collisions — dup/swap identity is the DUP
	// lowering's job), a dyn-body CALL_NATIVE's results are N distinct runtime
	// stack values, so every repeated out mints a fresh ID; an out that IS one
	// of the call's inputs keeps its ID (a pass-through resolves to its
	// operand). The outs slice is the dispatch's live result values, so the
	// fresh IDs flow to the downstream consumers exactly as in RecordCall.
	argIDs := make(map[string]bool, len(args))
	for _, a := range args {
		argIDs[a.ID] = true
	}
	seen := make(map[string]bool, len(outs))
	for i := range outs {
		_, prior := es.producedBy[outs[i].ID]
		// An IDENTITY-LESS registry-instance out (a module-export instance
		// minted outside any check pass — `do [M 3]`, §9.1) gets a fresh ID
		// too: without one the engine's tape tracking cannot place it (the
		// region inverted around it) and producedBy cannot link it to this
		// event. NARROW to ExtensionPayload instances — scalar outs elided
		// by the mode-gated ID discipline must STAY elided (a blanket mint
		// miscompiled the each-body value-def promotion).
		_, isExt := outs[i].Data.(ExtensionPayload)
		if (outs[i].ID == "" && isExt) || ((prior || seen[outs[i].ID]) && !argIDs[outs[i].ID]) {
			outs[i].ID = GenerateID(IDPrefixForType(outs[i].Parent))
		}
		seen[outs[i].ID] = true
		es.setProducedAt(outs[i], seq, i)
	}
	// Arm the program-wide environment mirror (see the EmitState.dynEnv doc) —
	// but ONLY for a body whose handler RE-RUNS a code body at run time
	// (resolving names against r.Defs / reading r.Args). A value-eval `do {map}`
	// runs no such sub-body: its map arg is fully assembled (OpMakeMap) before the
	// baked CALL_NATIVE, and doMapHandler just returns it — no dynamic-scope
	// mirror is needed. Arming dynEnv for it would force every unrelated def in
	// the program to a registry-visible OpBindDynScope twin and refuse the ones
	// whose value has no compiled home (`def found None` → "dynamic-scope def of
	// unknown provenance"), an unnecessary, whole-program refusal.
	if !fixedValueEval {
		es.dynEnv = true
	}
	return true
}

// The code-body higher-order words that may compile as Stage-5 interpreter
// islands (each / fold / scan / filter / select / group / outer / inner / do /
// case / where / having / order — pure data transforms applying a code body to
// data, no registry mutation) DECLARE CompileFallbackBody on their NativeFunc.
//
// The F4 general dynamic-dispatch words — pure typed dispatches (get / getr /
// size / make / is / typeof / type-algebra) with no side effects, whose
// forward-form span re-DISPATCHES faithfully through a sub-engine when the
// checker widened the site to a dynamic carrier — DECLARE CompileIslandPure. The
// sub-engine picks the overload at run time exactly as the interpreter would, so
// soundness holds without a static sig commitment; the dynamic result flows on
// and a downstream TYPED dispatch still refuses via anyDynamicCarrier. (Report
// §9.1's TYPE_CHECK boundary, realised as an interpreter island.)

// tryRecordFallback attempts to compile a refused code-body higher-order
// word as an interpreter island: the construct re-runs through a
// sub-engine over `word arg0 arg1 …` in forward form. The baked args
// ride inside the island token span; a COMPUTED data arg (a prior
// compiled event's result, or a loop local) whose value the check pass
// can't materialise is THREADED instead — the VM preloads its runtime
// value onto the island and the span re-runs against it ("computed
// receiver" islands, e.g. `(iota 5) each […]`).
//
// Eligible iff the word is allow-listed, fully forward-eligible (so the
// forward-form span is faithful), single-result, every code-body arg is
// concrete AND free of references the VM can't honour (only registered
// words / known literals — a check-time `def` binding is a carrier at
// run time and would diverge), and at most ONE data arg is threaded and
// it is the TRAILING run of positions (so the baked args fill the
// forward prefix and the one threaded value back-fills the deepest sig
// position — positionally faithful by the split rule). A baked data arg
// must be deeply concrete. Returns true when recorded; false leaves the
// normal refusal (whole-program fallback) to stand. Soundness rides on
// the differential gate: a threaded value is the program's real runtime
// value, and the island's dynamic result still refuses any downstream
// TYPED dispatch via anyDynamicCarrier.
func tryRecordFallback(r *Registry, word string, sig *Signature, args, outs []Value, pos SrcPos) bool {
	es := r.Check.Recorder()
	if !es.active() || sig == nil || !sig.CompileEffect.Has(CompileFallbackBody|CompileIslandPure) || len(outs) != 1 {
		return false
	}
	// A higher-order callable word dispatched on its LENS (Reach) form, not a
	// code body (`filter $.on data`, `each $.name data`): the island mechanism
	// exists to run an interpreted CODE BODY, and a reach lens is inert data, not
	// code. Now that an inert lens bakes as a const (isInertReach), letting these
	// island would convert a clean refusal into a NEW interpreter island (a
	// regression on islandCeiling) for no gain — the reach form has no body to
	// run. Decline so it refuses; the lens-as-const value/apply/getpath forms
	// (which do not route here) still compile natively.
	if sig.Callable != nil && sig.Callable.BodyPos < len(args) && IsReach(args[sig.Callable.BodyPos]) {
		return false
	}
	// A dispatch whose output is already recorded was handled by a structured
	// ReturnsFn hook (e.g. `case`'s desugar to a branch chain) — islanding it
	// would DOUBLE-record (the island fallback PLUS the structured event),
	// leaving the extra event unconsumed on the simulated stack. Skip it; the
	// generic RecordCall path that follows likewise early-returns.
	if es.alreadyProduced(outs[0].ID) {
		return false
	}
	// A pure typed word (get/make/is/typeof/size/type-algebra) is
	// islanded ONLY when the dispatch is genuinely dynamic — a dynamic
	// operand or a dynamic (Any-widened) result the normal path would
	// refuse anyway. A concrete-operand one compiles as a faithful
	// CALL_NATIVE and must NOT be islanded: islanding poisons its result
	// to dynamic, refusing every downstream typed dispatch (a net
	// coverage LOSS). The code-body words always island (they never lower
	// to CALL_NATIVE).
	if sig.CompileEffect.Has(CompileIslandPure) && !sig.CompileEffect.Has(CompileFallbackBody) &&
		!anyDynamicCarrier(args) && !anyDynamicCarrier(outs) {
		return false
	}
	// CORE-dispatch guard: the matched sig must belong to the word's
	// MAIN-registry binding (pointer identity into its sig backing
	// array). A module-qualified call (`ArrayUtil.group`) dispatches
	// the inner native through a SUB-registry, so its sig is a
	// different pointer — baking the bare name would re-run the core
	// word of that name (different semantics). The guard rejects those
	// so only a faithful bare-name re-run compiles.
	fn := r.Lookup(word)
	if fn == nil {
		return false
	}
	sigOK := false
	for i := range fn.Signatures {
		if &fn.Signatures[i] == sig {
			sigOK = true
			break
		}
	}
	if !sigOK {
		return false
	}
	span := make([]Value, 0, len(args)+1)
	span = append(span, NewWord(word))
	var ins []Value
	for i, a := range args {
		// A TYPE operand (a bare type node — `make Point …`, `x is Foo`,
		// the type-algebra args) bakes as a token in the span: the island
		// re-resolves it against the registry's lattice at run time, the
		// same place OpPushType resolves canonical types. It is never
		// threaded (a stack PUSH_TYPE would mis-order the dispatch).
		if IsBareTypeNode(a) && a.ID != "" {
			if len(ins) > 0 {
				return false
			}
			span = append(span, a)
			continue
		}
		cv, ok := es.materialise(a)
		baked := ok && IsConcrete(cv)
		if baked && sig.NoEvalArgs[i] {
			// A code body: legitimately contains words, but every one
			// must be VM-resolvable (no check-time def carriers).
			if !bodyFreeForFallback(r, cv) {
				return false
			}
		} else if baked && !isInertConst(cv) {
			// A baked data arg must be DEEPLY concrete plain data — a
			// carrier element anywhere (e.g. a def-bound list whose
			// interior the check pass stripped) would bake a type-only
			// artefact into the island and diverge (`[ProperString …]`
			// vs the real strings). isInertConst rejects any
			// carrier/dynamic/bare node in the tree.
			return false
		}
		if baked {
			if len(ins) > 0 {
				// A baked arg AFTER a threaded one would break the
				// forward-prefix / stack-suffix split — the threaded
				// values must be the trailing run. Refuse.
				return false
			}
			span = append(span, cv)
			continue
		}
		// Not bakeable: thread the runtime value. A code body must be
		// baked (its tokens carry the island's program); only a data
		// arg can thread. resolveOperand (in RecordFallback) refuses
		// anything without compiled provenance.
		if sig.NoEvalArgs[i] {
			return false
		}
		ins = append(ins, a)
	}
	if len(ins) > 1 {
		// Multi-threaded islands need the trailing run laid out on the
		// operand stack deepest-first; that ordering is a Stage-5
		// follow-on. One threaded value (the common computed-receiver
		// shape) is positionally unambiguous.
		return false
	}
	barrier := sig.BarrierPos
	if barrier < 0 || barrier > sig.TotalArgs() {
		barrier = sig.TotalArgs()
	}
	// Span faithfulness for a BARRIERED sig (e.g. `get`: key forward,
	// receiver stack). The forward-form span `word arg0 arg1 …` can only
	// place forward-eligible (position < barrier) args after the word; a
	// stack arg must reach the dispatch from the operand stack.
	//   - A THREADED stack arg already does (the island preloads it), so
	//     the baked args must all be forward-eligible (k <= barrier).
	//   - An ALL-BAKED island (no thread) on a PURE typed word with no
	//     code body rebuilds the span in STACK form: the stack args (B..N)
	//     ride before the word deepest-first, then the word, then the
	//     forward args — `{m} get a` instead of `get a {m}`. Code-body
	//     words can't (a baked body list would auto-evaluate when stepped
	//     on the stack), so they keep the forward-eligible constraint.
	canStackForm := len(ins) == 0 && sig.CompileEffect.Has(CompileIslandPure) && !sig.CompileEffect.Has(CompileFallbackBody)
	if !canStackForm && len(args)-len(ins) > barrier {
		return false
	}
	if canStackForm && barrier < len(args) {
		baked := span[1:] // sig order, parallel to args
		ns := make([]Value, 0, len(span))
		for i := len(baked) - 1; i >= barrier; i-- { // stack args, deepest first
			ns = append(ns, baked[i])
		}
		ns = append(ns, NewWord(word))
		for i := 0; i < barrier; i++ { // forward args, in order
			ns = append(ns, baked[i])
		}
		span = ns
	}
	return es.RecordFallback(FallbackSpan{Tokens: span, Desc: word}, ins, outs[0], pos)
}

// tryRecordDeferredList makes a deferred-list-body user fn TRANSPARENT in the
// recorder: when the dispatch result is the raw deferred list that
// buildFnBodyReturnsFn handed back (an `Eval` list still holding raw Words — the
// def-node-binding `[[c1]]` residual), record NOTHING for the call. The list
// then rides as the dispatch result and folds downstream exactly as a top-level
// `[[c1]]` literal does (the args become dead pushes, pruned at lowering). Without
// this the user-fn dispatch would hit recordCallRefusal ("user fn call … Stage 3")
// since no fn unit was compiled. Returns true when it claimed the dispatch.
func tryRecordDeferredList(r *Registry, sig *Signature, outs []Value) bool {
	if !r.Check.Recorder().active() || sig == nil || sig.fnFrame() == nil || len(outs) != 1 {
		return false
	}
	return isDeferredWordList(outs[0])
}
