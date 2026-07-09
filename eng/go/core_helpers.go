package eng

import (
	"fmt"
	"sort"
	"strings"
)

// InstallDef registers a new word as a literal substitution or a typed
// function definition. Multiple defs for the same name stack; undef pops
// the top.
//
// When body is a FnDefInfo value (produced by the fn word), InstallDef
// registers typed signatures. Otherwise, body is stored directly as a
// literal substitution.
//
// This is the REDEFINITION install: a fn-valued body whose signatures
// overlap an existing binding for the SAME name drops the colliding
// overload (so the stale one doesn't race the new one). For a per-call
// fn-frame param/capture — which enters a NEW lexical scope and must
// SHADOW, not replace, an outer same-named binding — use
// InstallFrameBinding instead.
func InstallDef(r *Registry, name string, body Value, stackOnly ...bool) {
	installDef(r, name, body, false, stackOnly...)
}

// InstallFrameBinding installs a per-call fn-frame binding (a named
// param or a lexical capture). Unlike InstallDef it is a lexical
// SHADOW: a Function/FnDef-valued binding pushes a fresh dispatch entry
// that shadows — never removes — an outer same-named binding, so the
// caller's binding is restored intact when the frame's teardown pops
// this entry. InstallDef's overlap-removal models top-level
// REDEFINITION (it must drop the colliding overload); a param entering
// a new scope must not, or it destroys the caller's binding (e.g. a
// fn-valued arg whose param name collides with a live caller param —
// design/ACCESSOR-SPLIT-AND-CLEANUP-BUG.md).
func InstallFrameBinding(r *Registry, name string, body Value) {
	installDef(r, name, body, true)
}

func installDef(r *Registry, name string, body Value, shadow bool, stackOnly ...bool) {
	isStackOnly := len(stackOnly) > 0 && stackOnly[0]

	// Attribute a body-local def to its enclosing fn for the dynamic-scope
	// undefined-word rescue (check mode only; no-op at the top level or outside
	// check). A name bound as a local inside fn F is visible — via AQL's
	// dynamic scoping — to any fn F reaches on the call stack.
	r.Check.RecordFnBinder(name)

	// FnDefInfo body (from fn word): install typed signatures.
	// Only fn-based defs register functions; simple value defs just use DefStacks.
	if body.Parent.Equal(TFnDef) || body.Parent.Equal(TFunction) {
		fnDef, ok := body.Data.(FnDefInfo)
		if !ok {
			return
		}
		fnDef.Name = name

		// Module-wrapper rebinding: a trivial-delegation wrapper
		// (`Body=[Word(inner)]`, all-unnamed params, carrying its own
		// sub-registry) is what `import` produces for each export and
		// what `unpack` extracts. Dot-access (`pkg.word`) dispatches it
		// via execFnDefLiteral's short-circuit, which looks up the
		// INNER native's real Signatures (carrying QuoteArgs /
		// NoEvalArgs / the Go handler) in the sub-registry. When the
		// same value is rebound to a bare name (`def w pkg.word` or
		// `unpack [word] pkg`), the body-splice path below would instead
		// re-dispatch `Word(inner)` in THIS registry — where the inner
		// native isn't registered — and drop the wrapper's special
		// arg-handling (FnSig has no QuoteArgs field). Mirror dot-access
		// instead: bind the inner native's Signatures verbatim under the
		// new name so bare-word dispatch behaves exactly like pkg.word.
		if reg := fnDef.Registry; reg != nil && reg != r {
			own := fnDef.OwnSigs()
			// EVERY own sig must be a trivial delegation to the SAME
			// inner native — a multi-overload wrapper (e.g. IO.write)
			// carries one delegation FnSig per overload. Requiring only
			// a single sig here used to drop multi-sig wrappers onto the
			// body-splice path below, where the wrapper's own UNLOCKED
			// FnSigs were installed — so a later overlapping `def` could
			// silently replace a module word instead of raising
			// locked_signature (the inner native's sigs are locked).
			innerName := ""
			allTrivial := len(own) > 0
			for i := range own {
				target, ok := trivialDelegationTarget(&own[i])
				if !ok || (innerName != "" && target != innerName) {
					allTrivial = false
					break
				}
				innerName = target
			}
			if allTrivial {
				if inner := reg.Lookup(innerName); inner != nil && len(inner.Signatures) > 0 {
					rebound := FnDefInfo{
						Name:           name,
						Signatures:     append([]Signature(nil), inner.Signatures...),
						MaxForwardArgs: inner.MaxForwardArgs,
						Registry:       reg,
					}
					r.Defs.Push(name, NewFnDef(rebound))
					if r.ready && r.OnRegisterHook != nil {
						r.OnRegisterHook(name)
					}
					return
				}
			}
		}

		// Remove any previous DefStack entries whose signatures overlap
		// with the new definition. Each entry holds its own overloads and
		// the dispatch aggregate (Registry.Lookup) unions them, so an
		// overlapping redefinition must drop the old entry — otherwise the
		// stale overload races the new one (equal scores, first match wins).
		//
		// SKIPPED for a shadowing frame-binding install: a fn param/capture
		// that collides with an outer same-named binding must SHADOW it (a
		// fresh entry the teardown pops to restore the outer), not drop it.
		// Dropping the outer entry here is the per-call cleanup over-pop bug
		// (design/ACCESSOR-SPLIT-AND-CLEANUP-BUG.md): the colliding outer
		// param vanishes, then the frame's undef tail pops the wrong level.
		// Entries carrying LOCKED signatures (native registrations, module-
		// wrapper rebindings) are never dropped: locked sigs can never be
		// replaced or removed (design/OPEN-WORDS.0.md §2.3). The def-merge
		// path (BuildWordExtension) intercepts fn defs over locked-bearing
		// words before InstallDef, so this guard is defence in depth for
		// direct InstallDef callers.
		if stack := r.Defs.Stack(name); !shadow && len(stack) > 0 {
			filtered := stack[:0:0]
			changed := false
			for _, entry := range stack {
				oldFn, ok := entry.Data.(FnDefInfo)
				if ok && !hasLockedSig(oldFn.Signatures) && FnDefsOverlap(oldFn, fnDef) {
					changed = true
					continue
				}
				filtered = append(filtered, entry)
			}
			if changed {
				r.Defs.Set(name, filtered)
			}
		}

		// Compile this definition's own overloads and push a single
		// DefStack entry. The 0-arg fallback and cross-stack overloading
		// are synthesised on demand by Registry.Lookup → aggregateDispatch.
		InstallFnDef(r, name, fnDef, isStackOnly)
		return
	}

	// FnUndefInfo body (from fn word in pair mode): remove targeted signatures.
	if body.Parent.Equal(TFnUndef) {
		undefInfo, ok := body.Data.(FnUndefInfo)
		if !ok {
			return
		}
		UninstallFnSigs(r, name, undefInfo)
		return
	}

	// ClassTypeInfo body: set the proper name in the type hierarchy.
	if IsClassType(body) {
		info, _ := AsClassType(body)
		if info.Parent != nil {
			// Child type: full name is Parent/Name (e.g. Ideal/Foo/Bar)
			info.Name = info.Parent.Name + "/" + name
		} else {
			// Direct child of the Object kind: Ideal/Name
			info.Name = "Ideal/" + name
		}
		// Register the name parts as known type parts.
		for _, p := range strings.Split(info.Name, "/") {
			r.RegisterPart(p)
		}
		// Preserve the body's *Type identity (set by the caller via
		// NewClassType). InstallDef rewrites info.Name based on the
		// def name and parent, then re-wraps the value — but the def
		// itself stays the caller's choice. For builtin object types
		// (Resource, Entity) the caller passes the canonical builtin
		// *Type; for user-defined object types installed as defs the
		// caller is responsible for minting first.
		def := body.Parent
		if def == nil {
			def = TClass
		}
		body = NewClassType(def, info)
		r.Defs.Push(name, body)
		return
	}

	r.Defs.Push(name, body)
}

// UninstallDef removes the most recent def for a word, exposing whatever
// binding it shadowed. For an fn def this drops the top DefStack entry's own
// overloads; the dispatch table is rebuilt on demand from the remaining
// entries by Registry.Lookup → aggregateDispatch, so no explicit re-register
// is needed.
func UninstallDef(r *Registry, name string) {
	r.Defs.Pop(name)
}

// buildFnBodyHandler produces the dispatch Handler for one AQL fn
// signature. Rather than computing a final result, the handler returns
// a PAREN-WRAPPED TOKEN SEQUENCE — `( unnamed-args… body DefCleanup __pa
// undef-tail… ReturnCheck )` — that execMatch splices back onto the
// stack to be RE-STEPPED inline. Named params and lexical captures are
// installed as defs up front (captures first so params shadow them);
// the paren keeps the whole expansion atomic so an outer forward can't
// grab an intermediate body value (e.g. recursive factorial).
//
// The handler closes over the install-time registry `r`: an FnDef
// registered into a name dispatches in the registry it was installed in.
// (The registry passed to the Handler at call time is intentionally
// ignored here — this mirrors the historical InstallFnDef closure.)
//
// Extracted verbatim from InstallFnDef so the same body-runner can be
// shared with the Function-value dispatch path (function-model
// consolidation). `s` is the signature; `fnDefCopy` supplies Captured;
// `meta` is the overload identity stamped on each spliced frame's open
// paren (the same pointer the compile boundary stores in
// Signature.FnFrame — see eng/go/fn_frame.go).
func buildFnBodyHandler(r *Registry, name string, s FnSig, fnDefCopy FnDefInfo, meta *FnFrameMeta) Handler {
	// Computed ONCE at construction: does this body install any body-local
	// def or construct an inner fn? A generic fn always does (it installs
	// inferred type-param bindings per call). When false — the common
	// leaf-fn / recursion case (fib, a tail-accumulator) — every call skips
	// BOTH per-call DefTable.Snapshot maps (each O(all bound names)) and the
	// def-cleanup name scan, the dominant term behind the ~340 allocs/frame
	// (design/INTERPRETER-SPEED-PLAN.10.md #5). Stack balance is preserved:
	// the fn baseline still pushes/pops (a nil entry) and the DefCleanup
	// marker still rides the tape (carrying SkipCleanup).
	needsFrameState := fnDefCopy.Gen != nil || bodyNeedsFrameState(s.body())
	return func(args []Value, _ map[string]Value, _ []Value, callReg *Registry) ([]Value, error) {
		// Reached from a FOREIGN registry (callReg != the install registry r) — a
		// module fn dispatched through the unified execMatch path from an outer
		// engine. The inline-re-step token expansion below binds params and resolves
		// the body in r, but execMatch re-steps the returned tokens in the CALLER's
		// registry (callReg), where r's params + module-private words are absent
		// ("undefined word: x"). So run the body via CallAQL and return the result —
		// the SAME execution the old execFnDefSig path did, now behind ONE dispatch
		// path. A same-registry fn (callReg == r, the common top-level case) keeps the
		// inline re-step (recursion / forward-ref intact).
		//
		// Pointer inequality alone is too broad: a registry FORK (await/timer/model
		// background branch — ForkConcurrent) is a NEW *Registry yet shares r's whole
		// binding lineage PLUS the branch's own local defs. Discriminate by scope id:
		// a fork inherits r's regID (AnalysisScopeID), a true module sub-registry has
		// its own. For a fork, run in callReg (the fork) so a branch-local binding
		// (`TimeUtil.await [[def x 1 read] …]`) the body reads resolves — r lacks it;
		// for a genuine module sub-registry, run in r where its private words live.
		if callReg != nil && callReg != r {
			target := r
			if callReg.AnalysisScopeID() == r.AnalysisScopeID() {
				target = callReg
			}
			return target.CallAQL(&s, args, fnDefCopy.Captured)
		}
		var result []Value
		var names []string
		// Wrap the entire expansion (unnamed args + body + undef
		// cleanup) in parens so it evaluates as a single
		// sub-expression. Without this, an outer forward can grab
		// intermediate values from the body before the body
		// finishes executing (e.g. recursive factorial: the outer
		// mul's forward grabs x=1 from the inner body instead of
		// waiting for the full result).
		result = append(result, NewFrameOpen(meta))

		// Push the fn-entry baseline BEFORE installing anything
		// for this call. Closure-capture detection on inner fn
		// constructions (afn/fn) inside this body consults
		// TopFnBaseline to identify enclosing-fn-local bindings:
		// names installed AFTER this snapshot (this call's
		// captures + named params + body-local defs) are
		// capturable; names already present at module/global
		// scope are dynamic.
		// Skip the (O(all names)) snapshot for a body that constructs no
		// inner fn — the baseline is read only by inner-fn capture
		// detection. Still push a (nil) entry so __pa's paired pop stays
		// balanced.
		if needsFrameState {
			r.PushFnBaseline(r.Defs.Snapshot())
		} else {
			r.PushFnBaseline(nil)
		}

		// Push args list onto the args stack for access via the
		// "args" word (args.0, args.1, etc.). Paired with __pa
		// at the body tail, which also pops the FnBaseline.
		argsCopy := make([]Value, len(args))
		copy(argsCopy, args)
		argsList := NewList(argsCopy)
		if err := r.Args.Push(argsList); err != nil {
			r.PopFnBaseline()
			return nil, err
		}

		// Install lexical captures BEFORE named params so params
		// shadow captures with the same name (innermost binding
		// wins). Captures are appended to `names` so the
		// synthesized undef tail tears them down alongside params.
		for _, cb := range fnDefCopy.Captured {
			InstallFrameBinding(r, cb.Name, cb.Value)
			names = append(names, cb.Name)
		}

		unnamedCount := 0
		for i, p := range s.Params {
			if p.Name != "" {
				arg := args[i]
				// Quote list params so they're treated as data values
				// when referenced in the body, not expanded as code bodies.
				if arg.Parent.Equal(TList) && !arg.Quoted {
					arg.Quoted = true
				}
				InstallFrameBinding(r, p.Name, arg)
				names = append(names, p.Name)
			} else {
				// Unnamed parameter: push value back for the body to use
				result = append(result, args[i])
				unnamedCount++
			}
		}
		// Stamp the resolved-argument span on the frame open so the step
		// loop skips the unnamed args — they enter as stack data and are
		// never re-stepped (arguments are inert; FrameOpenInfo.ArgSpan).
		if unnamedCount > 0 {
			result[0] = NewFrameOpenSpan(meta, unnamedCount)
		}
		// Snapshot DefStacks lengths after installing named params
		// so we can clean up any defs created during body execution
		// (fixes def leakage from fn bodies — DX-REPORT Issue 2). Skipped
		// (nil) when the body installs no body-local defs; the DefCleanup
		// marker then rides with SkipCleanup and allocates no map.
		var defSnapshot map[string]int
		if needsFrameState {
			defSnapshot = r.Defs.Snapshot()
		}

		// Generic fn: install the inferred type-parameter bindings for
		// the body (`of [T]`, `make (Box of [T])`). AFTER the snapshot,
		// so the existing DefCleanup truncation tears them down — the
		// undef tail's capitalised path would Retire the bound type's
		// canonical node (design/GENERICS.10.md Phase 4).
		if fnDefCopy.Gen != nil {
			InstallGenCallBindings(r, fnDefCopy.Gen, s.Params, args)
		}

		body := make([]Value, len(s.body()))
		copy(body, s.body())
		result = append(result, body...)
		// The canonical cleanup tail: DefCleanup (undoes body-local
		// defs), __pa (pops Args + FnBaseline), the undef pairs for
		// captures+params, and the ReturnCheck when returns are
		// declared (Pos left zero — execMatch stamps the call site).
		result = AppendFrameTail(result, FrameTailSpec{
			Registry:     r,
			Snapshot:     defSnapshot,
			SkipCleanup:  !needsFrameState,
			Names:        names,
			Returns:      s.Returns,
			UnnamedCount: unnamedCount,
			FuncName:     name,
		})
		result = append(result, NewCloseParen())
		return result, nil
	}
}

// buildFnBodyReturnsFn produces the check-mode ReturnsFn for one AQL fn
// signature. In static-check mode the engine skips the runtime handler
// and calls this with carrier-typed args; it analyses the body via
// AnalyseFnBody (so body diagnostics propagate) and returns the carrier
// result — declared return types when present, else the analyser's
// residual top-of-stack carrier(s). It also performs record-shape
// checking for pattern params.
//
// Anonymous lambdas (afn / =>) declare Returns=[Any] as a placeholder;
// for them the declared-returns fast path is dropped so the analyser's
// inferred residual wins (this is what makes `([n:Integer] => [n add 1])`
// infer Integer rather than Any in check mode).
//
// Extracted verbatim from InstallFnDef so the same return-inference can
// be shared with the Function-value dispatch path.
// narrowArgsToParams returns args with each gradual (dynamic) arg whose bound
// is strictly BROADER than its declared param type narrowed to a dynamic
// carrier of that param type. The arg already passed the param match at the
// call site (a disjoint arg never reaches body analysis), so narrowing its
// gradual bound to the declared contract is sound and stays optimistic — it
// just ensures a body word sees the param's declared shape. Without it, a
// recursive call threading a `get`-result `dynamic(Any)` into a `ch:List` param
// makes the body's `each` match the map-each overload (→ Map) instead of
// list-each (→ List), so a downstream `all`/`any`/`[List]` consumer then fails
// no_signature against the spurious Map (the decision `eval-pred-all` cycle).
// A concrete arg, an arg already conforming to the param, or an Any/untyped
// param is left untouched (no precision lost).
// recordSchemaCarrier returns the body-analysis view of a map/gradual-Any arg
// bound to a RECORD-shape param (declared `c:SomeRecord` — Type=Map + a NewMap
// field→type schema pattern): a carrier CARRYING the record schema
// (RecordTypeInfo), so a body `c get "field"` recovers the field's declared type
// instead of degrading to Any. The arg already passed the param match at the
// call site and the runtime CALL_USER guard re-checks it, so committing to the
// schema for body analysis is sound; recovered field types are GRADUAL
// (getNodeReturns), discharged by a guard for a non-conforming value. Returns
// (zero,false) for Options-typed params (OptionsTypeInfo, not MapPayload),
// patternless params, and non-map args. Applied at BOTH body-analysis paths —
// narrowArgsToParams (result/summary) and the armed-compile genArgs — so the
// schema survives into the closure CAPTURE that the armed compile records.
func recordSchemaCarrier(p FnParam, a Value) (Value, bool) {
	if p.Pattern == nil || a.Parent == nil {
		return Value{}, false
	}
	if !(a.Parent.ConformsTo(TMap) || a.Parent.Equal(TAny)) {
		return Value{}, false
	}
	if pm, ok := p.Pattern.Data.(MapPayload); ok && pm.M != nil {
		// A fresh, distinct ID per carrier: the bytecode compiler keys a
		// param's frame-local slot by Value.ID (StartFnCompile →
		// RegisterLocal). A struct-literal carrier left with the zero ID would
		// make every record-typed param collapse onto the SAME slot (id ""),
		// so a fn reading >1 record param (`fn [[c:C d:D] …]`) miscompiles to a
		// single local → RunCompiledStrict VM error / wrong local. Mint a
		// unique node ID exactly as NewValueRaw does for ordinary values.
		return Value{ID: GenerateID(IDPrefixForType(TMap)), Parent: TMap, Carrier: true, Dynamic: true, Data: RecordTypeInfo{Fields: pm.M}}, true
	}
	return Value{}, false
}

func narrowArgsToParams(args []Value, params []FnParam) []Value {
	var out []Value
	for i := range args {
		if i >= len(params) {
			break
		}
		a := args[i]
		pt := params[i].Type
		if rc, ok := recordSchemaCarrier(params[i], a); ok {
			if out == nil {
				out = append([]Value(nil), args...)
			}
			out[i] = rc
			continue
		}
		switch {
		case a.Dynamic && pt != nil && !pt.Equal(TAny) && a.Parent != nil &&
			!a.Parent.ConformsTo(pt):
			// A gradual arg whose bound is NOT already within the declared
			// (concrete) param type: re-bind the body's view of the param to a
			// dynamic carrier of the DECLARED type. The annotation is the
			// authoritative contract for body analysis — the body is written
			// assuming the param's type, and a dynamic arg means "statically
			// unknown, optimistically anything", so binding to dynamic(pt) keeps
			// optimism while letting the body's typed operations match. Covers
			// both a strictly-broader bound (`Any` → `String`) and a disjoint /
			// sibling bound (radix's edge splitter feeds a `slice`/`add` result
			// of unknown shape — a dynamic `List` — into `set-edge`'s
			// `newlabel:String`; without this the body's `newlabel slice 0 1`
			// stayed `List` and failed `set`'s String-key Map overload).
			if out == nil {
				out = append([]Value(nil), args...)
			}
			nc := NewCarrier(pt)
			nc.Dynamic = true
			out[i] = nc
		case !a.Dynamic && pt != nil && pt.Equal(TAny) && a.Parent != nil && a.Parent.Equal(TAny) && !IsBareTypeNode(a):
			// A STRICT Any arg bound to a declared-`Any` param. The param is
			// gradual by declaration (ParamInputCarrier gives dynamic Any), so a
			// body word over it must match optimistically — a strict Any conforms
			// to no concrete slot and would cascade no_signature. The case arises
			// when a fold accumulator that started `none` and was rebuilt into a
			// node is threaded into a `nd:Any` insert receiver (tst/radix's
			// `none entries [(acc … tst-insert)] fold`). Only a bare strict-Any
			// VALUE is lifted (not a typed/structural carrier).
			if out == nil {
				out = append([]Value(nil), args...)
			}
			nc := NewCarrier(TAny)
			nc.Dynamic = true
			out[i] = nc
		}
	}
	if out == nil {
		return args
	}
	return out
}

// checkBodyReturnConformance is the check-mode mirror of the runtime RET
// check (validateReturnTypes): the analysed body residual's top K values are
// compared slot-wise against the K declared return types, and a mismatch is
// reported as the SAME type_error the runtime raises (returnTypeErrorText, so
// the two surfaces are byte-identical). Unlike the runtime's membership check
// (`got.Is(exp)`), only a PROVABLY IMPOSSIBLE return flags here: the residual
// type neither conforms to the declaration nor the declaration to it, and
// their tand meet is Never — the same disjointness proof the gradual param
// match uses (sigTypeMatches). A subtype/refine declaration stays
// value-dependent (`[n add 1]` against a declared `Big (Integer gt 10)` may
// pass at runtime) and is left to the RET check. Dynamic residuals are the
// gradual contract; a short residual (recursion bail, zero-net body shape),
// a None placeholder, and a bare type node are left to the runtime arity /
// membership errors; a disjunct flags only when EVERY alternative is
// impossible. Deduped by detail — the ReturnsFn runs once per analysed call
// shape, but shapes repeat across call sites.
//
// The COUNT mirror of the same boundary: a residual provably LONGER than the
// declaration tolerates is the runtime's "expected N return value(s), got M"
// (returnCountErrorText — the __RC arity rule, which discards bottom extras
// only up to unnamedCount, the fn's unconsumed unnamed-arg allowance). Only a
// count the analysis knows exactly flags: a variadic spread models 0-or-more
// values, and a Function/FnDef in the residual may be an unapplied fn-value
// call the static model over-counts (the emit.go cluster-E shape) — both
// skip. The short side (len < declared) stays with the runtime arity error —
// EXCEPT the all-concrete-call EMPTY residual on the top-level straight
// line: a declared fn whose per-call analysis (real argument values, no
// generalisation) nets NOTHING either diverged (a taken `raise` branch —
// the divergence model leaves no carrier) or under-returns, and the runtime
// errors EITHER way (the raise, or "expected N…, got 0"), so the call is a
// guaranteed program error. argsConcrete gates it to real concrete-arg
// calls — the install-time synthetic example eval and generalised analyses
// use carriers and never fire it.
func checkBodyReturnConformance(r *Registry, name string, declared []*Type, unnamedCount int, argsConcrete bool, stk []Value, pos, bodyEnd SrcPos) {
	if len(declared) == 0 || !r.Check.IsActive() {
		return
	}
	if len(stk) < len(declared) {
		if len(stk) == 0 && argsConcrete && CheckAtUncaughtTopLevel(r) &&
			!fnBodyUndefinedWordShield(r, name, pos, bodyEnd) {
			detail := fmt.Sprintf(
				"%s: the body produces no return value for this call (declared %d) — the call always errors",
				name, len(declared))
			if !hasCheckDiagnostic(r, "type_error", detail) {
				r.Check.AddDiagnostic(CheckDiagnostic{
					Code:          "type_error",
					Detail:        detail,
					Word:          name,
					Row:           pos.Row,
					Col:           pos.Col,
					RuntimeMirror: true,
				})
			}
		}
		return
	}
	// A residual computed while this body contained an UNDEFINED word is not
	// evidence: a mutually-recursive sibling defined later (`def isod … isev
	// … def isev …`) makes the install-time analysis see an incomplete world,
	// and the FnSummaries memo then serves that broken residual to every
	// later call. The undefined_word diagnostic tagged with this fn's name is
	// still in the list here (RescueForwardRefDiagnostics drops it only at
	// end of pass), so its presence is the reliable "analysis ran early"
	// signal — skip; the runtime RET check still guards these bodies.
	//
	// Scoped to THIS body's source span [pos, bodyEnd]: an undefined forward
	// ref in a DIFFERENT overload/redefinition of the same name must not
	// shield this body's conformance check (`def f fn [… [g a]] def f fn
	// [… [String] [42]] …` — the second f's Integer return is provably
	// wrong regardless of the first f's rescued `g`). An unattributed
	// diagnostic (Row 0) or an unpositioned body keeps the name-wide skip —
	// conservative, never a new false positive.
	if fnBodyUndefinedWordShield(r, name, pos, bodyEnd) {
		return
	}
	extra := len(stk) - len(declared)
	// The count mirror flags only an exactly-known count: a DYNAMIC value
	// in the residual marks a modelling seam (a mid-body `apply`, a
	// gradual branch join) where the static count can diverge from the
	// runtime one — skip, like the variadic/fn-value shapes. It is a
	// RuntimeMirror: the compile pass deliberately COMPILES the
	// count-mismatched body and lets the VM RET raise the byte-identical
	// error (emit.go — TestEmitP5MultiResult pins it), so the refusal
	// loop skips mirror diagnostics.
	if extra > unnamedCount &&
		!stackHasVariadic(stk) && !stackHasFnValue(stk) && !stackHasDynamic(stk) {
		detail := returnCountErrorText(name, len(declared), len(stk)-unnamedCount)
		if !hasCheckDiagnostic(r, "type_error", detail) {
			r.Check.AddDiagnostic(CheckDiagnostic{
				Code:          "type_error",
				Detail:        detail,
				Word:          name,
				Row:           pos.Row,
				Col:           pos.Col,
				RuntimeMirror: true,
			})
		}
	}
	for k, exp := range declared {
		if exp == nil || exp.Equal(TAny) {
			continue
		}
		got := stk[extra+k]
		if got.Dynamic || got.Parent == nil || IsBareTypeNode(got) || got.Parent.Equal(TNone) {
			continue
		}
		if !residualProvablyDisjoint(got, exp) {
			// Value-level membership for a compile-time-known scalar residual:
			// the runtime RET boundary asks got.Is(exp) on this SAME value, so
			// a concrete scalar that fails membership NOW is a guaranteed
			// runtime type_error — flag it statically. This closes the
			// smart-constructor holes the disjointness test cannot see:
			//   - predicate returns: `def mkbad fn [[] [Big] [5]]` (Big =
			//     Integer gt 10; 5 fails the predicate) — previously
			//     check-clean, runtime-only;
			//   - nominal newtype returns: `def mk fn [[] [UserId] [42]]`
			//     (a raw Integer is not a UserId; a reparented `def x:UserId
			//     42  x` residual carries the UserId tag and passes).
			// Restricted to scalarFoldOperand shapes (concrete scalar
			// payloads — literals and folded comparisons), where the checked
			// value provably equals the runtime value; abstract carriers keep
			// the runtime check (value-dependent returns bail by design).
			if !scalarFoldOperand(got) || got.Is(exp) {
				continue
			}
		}
		detail, _ := returnTypeErrorText(name, k+1, exp, got)
		if !hasCheckDiagnostic(r, "type_error", detail) {
			// A RuntimeMirror like the count path: the VM RET re-checks the
			// runtime value and raises the byte-identical type_error, so the
			// compiled error path is exact.
			r.Check.AddDiagnostic(CheckDiagnostic{
				Code:          "type_error",
				Detail:        detail,
				Word:          name,
				Row:           pos.Row,
				Col:           pos.Col,
				RuntimeMirror: true,
			})
		}
	}
}

// fnBodyUndefinedWordShield reports whether an undefined_word diagnostic is
// attributed to THIS fn body's source span — the "analysis ran early"
// signal (a mutually-recursive sibling defined later makes the install-time
// analysis see an incomplete world, and FnSummaries then serves that broken
// residual to every later call). Scoped to [pos, bodyEnd]: an undefined
// forward ref in a DIFFERENT overload/redefinition of the same name must
// not shield this body's conformance check. An unattributed diagnostic
// (Row 0) or an unpositioned body keeps the name-wide shield —
// conservative, never a new false positive.
func fnBodyUndefinedWordShield(r *Registry, name string, pos, bodyEnd SrcPos) bool {
	for _, d := range r.Check.Diagnostics {
		if d.Code != "undefined_word" || d.FnName != name {
			continue
		}
		if d.Row != 0 && (bodyEnd.Row != 0 || bodyEnd.Col != 0) &&
			(posBefore(d.Row, d.Col, pos) || posBefore(bodyEnd.Row, bodyEnd.Col, SrcPos{Row: d.Row, Col: d.Col})) {
			continue // attributed outside this body — not this body's signal
		}
		return true
	}
	return false
}

// hasCheckDiagnostic reports whether a diagnostic with the given code and
// exact detail is already recorded — the dedupe the per-call-shape ReturnsFn
// paths use (one analysed shape can repeat across call sites). A caught
// (downgraded) entry does not count: it must never mask a later REAL
// emission of the same finding outside the trapping region.
func hasCheckDiagnostic(r *Registry, code, detail string) bool {
	for _, d := range r.Check.Diagnostics {
		if d.Code == code && d.Detail == detail && !d.CaughtAtRuntime {
			return true
		}
	}
	return false
}

// stackHasFnValue reports whether any residual value is a Function/FnDef —
// the shape whose static count can over-report (an unapplied fn-value call
// the interpreter applies at runtime; see emit.go's cluster-E refusal).
func stackHasFnValue(stk []Value) bool {
	for _, v := range stk {
		if v.Parent != nil && (v.Parent.ConformsTo(TFunction) || v.Parent.ConformsTo(TFnDef)) {
			return true
		}
	}
	return false
}

// stackHasDynamic reports whether any residual value is gradual (Dynamic)
// — the marker of a modelling seam where the analysis count may not equal
// the runtime count.
func stackHasDynamic(stk []Value) bool {
	for _, v := range stk {
		if v.Dynamic {
			return true
		}
	}
	return false
}

// posBefore reports whether source position (row, col) strictly precedes p
// in reading order. Used to test a diagnostic's attribution against a fn
// body's source span.
func posBefore(row, col int, p SrcPos) bool {
	return row < p.Row || (row == p.Row && col < p.Col)
}

// bodySpanEnd walks a parsed fn body's tokens (paren exprs, reaches, lists,
// map values — the exprRefsCarrier shapes) and returns the maximum source
// position seen, i.e. the start of the body's LAST token at any depth.
// Together with the first token's position it bounds the body's source span
// for diagnostic attribution. Zero when no token carries a position.
func bodySpanEnd(body []Value) SrcPos {
	var end SrcPos
	var walk func(vs []Value)
	walk = func(vs []Value) {
		for _, v := range vs {
			if posBefore(end.Row, end.Col, v.Pos) {
				end = v.Pos
			}
			if IsParenExpr(v) {
				if toks, err := AsParenExpr(v); err == nil {
					walk(toks)
				}
				continue
			}
			if IsReach(v) {
				if ri, err := AsReach(v); err == nil {
					walk(ri.Receiver)
					for i := range ri.Segments {
						if ri.Segments[i].Computed {
							walk(ri.Segments[i].KeyExpr)
						}
					}
				}
				continue
			}
			if lst, err := AsList(v); err == nil && !lst.IsNil() {
				walk(lst.Slice())
				continue
			}
			if mp, err := AsMap(v); err == nil && mp != nil {
				for _, k := range mp.Keys() {
					mv, _ := mp.Get(k)
					walk([]Value{mv})
				}
			}
		}
	}
	walk(body)
	return end
}

// residualProvablyDisjoint reports whether a body-residual value's type can
// NEVER inhabit the declared return type: no conformance in either direction
// AND a Never tand meet. The either-direction conformance mirrors
// sigTypeMatches' gradual rule (tand wrongly reports two container-family
// types as disjoint when one conforms to the other). A disjunct residual is
// disjoint only if every alternative is.
func residualProvablyDisjoint(got Value, exp *Type) bool {
	if IsDisjunct(got) {
		di, err := AsDisjunct(got)
		if err != nil || len(di.Alternatives) == 0 {
			return false
		}
		for _, alt := range di.Alternatives {
			probe := alt
			if IsBareTypeNode(alt) {
				probe = carrierOfLiteral(alt)
			}
			if !residualProvablyDisjoint(probe, exp) {
				return false
			}
		}
		return true
	}
	p := got.Parent
	if p == nil || p.ConformsTo(exp) || exp.ConformsTo(p) {
		return false
	}
	// An OPAQUE union residual (Parent=Disjunct with no readable
	// alternatives — the payload was stripped upstream) denotes "one of
	// several types" — nothing is provable about it. And a Function/FnDef
	// residual sits on the fn-value-call frontier: `v f/r apply` leaves an
	// UNAPPLIED Function in the abstract residual where the runtime calls
	// it (the "fn-value-call boundary" imprecision class), so a Function
	// residual routinely means "not modeled", not "returns a function".
	// Both stay with the runtime RET check.
	if p.ConformsTo(TDisjunct) || p.ConformsTo(TFunction) || p.ConformsTo(TFnDef) {
		return false
	}
	// A declared type that admits values by VALUE-level membership — a
	// disjunct/enum (`def T (Integer tor String)`), a negation, a predicate
	// type, a Go member type — accepts residuals whose TAG does not
	// nominally conform: the runtime `v.Is(T)` runs the membership, so
	// `[x]` against a declared T passes for an Integer x ∈ T. A nominal
	// disjointness proof says nothing there — skip. The carrier-Is probe is
	// the backstop for wrapped behaviors (a behave-augmented union still
	// answers membership through its Match chain).
	if membershipBeyondNominal(exp) || NewCarrier(p).Is(exp) {
		return false
	}
	return isNeverShape(TandValues(NewCarrier(p), NewCarrier(exp)))
}

// membershipBeyondNominal reports whether t's installed Behavior admits
// values by VALUE-level membership rather than nominal tag conformance —
// the unifier families whose Match can accept a value from a foreign
// lattice family. Bare refines stay nominal (provable); DepScalar nodes
// are parented at their base scalar, so the conformance shortcut above
// already skips them — listed here as a belt.
func membershipBeyondNominal(t *Type) bool {
	if t == nil {
		return false
	}
	switch t.Behavior.(type) {
	case *disjunctUnifier, *negationUnifier, *predicateUnifier, memberBehavior, *depScalarUnifier:
		return true
	}
	return false
}

// checkRecordShapeArgs is the pattern / record-shape check for one analysed
// call: for each declared record-typed param, verify the arg map carries each
// declared field key with a conforming type. Skips calls whose arg is empty
// or whose key set doesn't overlap the pattern at all (that shape is
// typically the synthetic/default arg map used during fn body analysis, not
// a real user call).
func checkRecordShapeArgs(r *Registry, name string, paramPatterns []*Value, args []Value) {
	for i, pat := range paramPatterns {
		if pat == nil || i >= len(args) {
			continue
		}
		val := args[i]
		if !pat.Parent.Equal(TMap) || !val.Parent.Equal(TMap) ||
			!IsConcrete(*pat) || !IsConcrete(val) {
			continue
		}
		pMap, _ := AsMap(*pat)
		vMap, _ := AsMap(val)
		if pMap == nil || vMap == nil || vMap.Len() == 0 {
			continue
		}
		overlap := 0
		for _, k := range pMap.Keys() {
			if _, ok := vMap.Get(k); ok {
				overlap++
			}
		}
		if overlap == 0 {
			continue
		}
		for _, key := range pMap.Keys() {
			pv, _ := pMap.Get(key)
			av, hasKey := vMap.Get(key)
			if !hasKey {
				r.Check.AddDiagnostic(CheckDiagnostic{
					Code:   "record_shape_mismatch",
					Detail: "argument to " + name + " missing field: " + key,
					Word:   name,
				})
				continue
			}
			if IsBareTypeNode(pv) && !av.Parent.ConformsTo(pv.Parent) && !av.Parent.Equal(TAny) {
				r.Check.AddDiagnostic(CheckDiagnostic{
					Code:   "record_shape_mismatch",
					Detail: "argument to " + name + ": field " + key + " expected " + pv.Parent.String() + ", got " + av.Parent.String(),
					Word:   name,
				})
			}
		}
	}
}

func buildFnBodyReturnsFn(r *Registry, name string, s FnSig, fnDef FnDefInfo) ReturnsFunc {
	paramNames := make([]string, len(s.Params))
	paramPatterns := make([]*Value, len(s.Params))
	for i, p := range s.Params {
		paramNames[i] = p.Name
		paramPatterns[i] = p.Pattern
	}
	declaredReturns := append([]*Type(nil), s.Returns...)
	if fnDef.Anonymous {
		declaredReturns = nil
	}
	bodyCopy := append([]Value(nil), s.body()...)
	nameCopy := name
	capturesCopy := fnDef.Captured
	genSpec := fnDef.Gen
	sigParams := append([]FnParam(nil), s.Params...)
	return func(args []Value, caller *Registry) []Value {
		// The MERGED-WORD seam (Stage M1, design/STAGE3-INLINING-DESIGN-ROUND.0.md
		// §2.4a/§5): a transplanted word-extension sig (open words — a module-
		// defined `add` merged into the importer's dispatch table) dispatches as a
		// BARE word on the importer's engine, so no execFnDefLiteral wrapper
		// shares the check state with this closure's captured registry r. Without
		// sharing, `es` below read the module's own INACTIVE CheckState:
		// StartFnCompile declined, RecordUserCall never ran, every merged dispatch
		// refused "user fn call … (Stage 3)", and the body was ANALYSED
		// CONCRETELY with its diagnostics stranded on the invisible module Check.
		// Sharing caller → r at the ReturnsFn boundary itself converts the WHOLE
		// user-fn seam (every dispatch path that reaches a foreign-registry fn
		// body records identically — the §2.2 one-recording-path rule): body
		// diagnostics, the FnSummaries/FnInflight memos (scopeID-disjoint keys)
		// and the compiled unit all land on the dispatching pass, while name
		// resolution stays in the module's own Defs/Types. No-op when caller == r
		// (ordinary same-registry fns), when an enclosing execFnDefLiteral /
		// execFnDefSig dispatch already shared (pointer-equal swap), and outside
		// check mode — interpretation is untouched.
		restoreCheck := shareCheckStateFrom(r, caller)
		defer restoreCheck()
		checkRecordShapeArgs(r, nameCopy, paramPatterns, args)
		// Generic fns (Phase 5): infer the parameter bindings from the
		// call's arg carriers and install them around the body
		// analysis, so body-internal `of [T]` / `make (Box of [T])`
		// resolve per call site — the FnSummaries memo key already
		// includes the arg types, so each distinct instantiation is
		// analysed once (monomorphization for free). The bindings are
		// popped explicitly (Defs.Pop is non-retiring); there is no
		// DefCleanup frame on this path.
		var genBindings map[string]Value
		var genNames []string
		if genSpec != nil {
			genBindings = InferGenBindings(genSpec, sigParams, args)
			genNames = InstallGenBindingMap(r, genSpec, genBindings)
		}
		// Deferred-list body (def-node-binding.tsv:54) — `def mk fn
		// [[c1:Integer] [List] [[c1]]]`. The body is a single list literal that
		// references a parameter, but a returned list NEVER closes over the param
		// (only a `=>` lambda captures); the interpreter returns it RAW and
		// auto-evaluates it later, in MODULE scope, against the live binding
		// (`mk 9` → `[1]`, the module `c1`, not the arg `9`). Compiling it as a
		// fn UNIT cannot model this: a unit's result is fixed at CALL time and
		// cannot defer to a later module rebind. So make the fn TRANSPARENT —
		// hand the raw deferred list back as the call's residual (no unit) and let
		// the check pass fold it in module scope EXACTLY as a top-level
		// `def c1 1 [[c1]]` already does: at end-of-run, at a `def`-bind, or at a
		// downstream consumer (each resolves `c1` against the binding live at that
		// point). The folded const is what the program bakes; no VM change. Still
		// analyse the body first so its diagnostics propagate.
		if raw, ok := deferredParamListResidual(bodyCopy, paramNames); ok {
			AnalyseFnBody(r, nameCopy, paramNames, bodyCopy, args, capturesCopy, declaredReturns)
			for i := len(genNames) - 1; i >= 0; i-- {
				r.Defs.Pop(genNames[i])
			}
			return []Value{raw}
		}
		// Always analyse the body so diagnostics emitted by stepWord
		// (undefined_word, no_signature, …) inside the body propagate
		// up to the parent registry. When the fn declares an explicit
		// return type, we use that for the carrier result and drop
		// the analyser's residual stack — the analyser is run purely
		// for its side-effecting diagnostic collection. Memoisation
		// inside AnalyseFnBody keeps recursive / repeated calls cheap.
		// Bytecode (Stage 3): compile this overload's body as its own
		// code unit (params as frame locals) and record the call site.
		// The memo key mirrors AnalyseFnBody's so the unit is compiled
		// exactly when the body is analysed.
		es := r.Check.Recorder()
		fnUnit := -1
		var finishFn func([]Value)
		// Cluster C (broad miscompile hunt): a gradual-Any arg to a MULTI-overload
		// user fn is an AMBIGUOUS dispatch. The checker commits to one overload
		// (whose CALL_USER param guard then RAISES at runtime when the value matches
		// a SIBLING overload), but the interpreter runtime-re-matches and dispatches
		// the sibling — `def g fn [[a:Integer]['i'] [a:String]['s']] (g (id 5))`
		// returned 'i' interpreted but signature_error compiled. Natives re-match via
		// OpCallNativePoly; user-fn overloads have NO poly path. Refuse → fall back.
		clusterCRefuse := es.active() && anyDynamicCarrier(args) &&
			dynamicReachableOverloadCount(r, nameCopy, args) >= 2
		var polyPlan *userPolyPlan
		if clusterCRefuse {
			// OpCallUserPoly (zero-refusals Stage 2): instead of refusing, try to
			// bake EVERY same-arity overload's body unit and let the VM re-run
			// MatchSignature at entry — the sound user-fn mirror of callPoly.
			// declaredReturns is the committed sig's contract (nil for an
			// anonymous fn, which the poly gate then refuses).
			polyPlan = tryCompileUserPolyArms(r, es, nameCopy, args, declaredReturns)
			if polyPlan == nil {
				// All-or-nothing: any arm the poly compile cannot own keeps the
				// original refusal, byte-identical.
				es.MarkUncompilable("gradual-Any arg to multi-overload user fn `" + nameCopy + "`: ambiguous dispatch, no poly re-match")
			}
		}
		// A FOREIGN-registry fn whose body constructs a fn value USED to
		// refuse wholesale (the compiled unit executed against the
		// dispatching registry, losing module scope for the constructed
		// lambda). Units now carry their owning registry (CompiledFn.Reg):
		// the VM dispatches the unit's natives against it — exactly the
		// registry the interpreter's foreign-wrapper CallAQL runs the body
		// in — so a constructed lambda's downstream capability state (a
		// listener's per-connection registries) forks module scope on both
		// engines, and the refusal is retired (aql:repl rows 12/16/18
		// compile; the remaining statement-position recovery strand refuses
		// through the ordinary provenance paths).
		if es.Armed() && !clusterCRefuse {
			// The body unit must be compiled against GENERALISED args
			// — pure carriers of the call's arg types. The call's
			// kept-concrete values would constant-fold inside the body
			// (`n sub 1` with n=10 folds to 9), baking one call's
			// constants into the shared unit. Same Parents → same memo
			// key, so the generalised analysis is the one that caches.
			genArgs := make([]Value, len(args))
			for i, a := range args {
				if i < len(sigParams) {
					if rc, ok := recordSchemaCarrier(sigParams[i], a); ok {
						genArgs[i] = rc // preserve record schema into the armed compile + closure capture
						continue
					}
					// A RECOVERED call: an Any arg flowed into a CONCRETELY-typed
					// param (matchSignature couldn't statically commit, so dispatch
					// recovered — tryRecordRecoveredUserFn). Compile the body against
					// the DECLARED param type, not strict-Any: the body's words then
					// dispatch against the real type (e.g. `convert Float` over an
					// Integer param inside a chain whose source was an Options `get`),
					// instead of refusing "unmatched dispatch" on strict-Any. Sound by
					// the param contract — SetUnitParamTypes installs a CALL_USER guard
					// that raises == the interpreter when a runtime arg misses the
					// declared type, so assuming it here can only narrow, never admit a
					// value the interpreter would reject.
					if pt := sigParams[i].Type; pt != nil && !pt.Equal(TAny) &&
						a.Parent != nil && a.Parent.Equal(TAny) && !IsBareTypeNode(a) {
						genArgs[i] = NewCarrier(pt)
						continue
					}
				}
				if a.Parent == nil {
					// A root-node carrier (None / Any / Never) has a nil Parent
					// because it IS its own lattice node — it is already an
					// abstract, constant-free generalisation, so keep it (with the
					// Carrier flag set) rather than calling NewCarrier(nil), which
					// would propagate a nil-typed value into the body analysis.
					g := a
					g.Carrier = true
					g.Data = nil
					genArgs[i] = g
					continue
				}
				genArgs[i] = NewCarrier(a.Parent)
			}
			// Key the compiled unit on the GENERALISED args, matching the body
			// analysis (AnalyseFnBody runs on genArgs) and the FnSummaries memo. A
			// RECURSIVE call reaches this fn with DIFFERENT concrete args (a sub-spec
			// vs the top spec) but the SAME generalisation, so an args-keyed unit
			// missed the in-flight unit and allocated a SECOND unit that the
			// in-flight bail left EMPTY (the recursive `subspec run-spec` called an
			// empty RET → sub-specs silently skipped). Keying on genArgs makes the
			// recursive call REUSE the in-flight unit (the generalised body that
			// handles any arg shape at run time).
			key := FnAnalysisKey(r.AnalysisScopeID(), nameCopy, genArgs, capturesCopy, bodyCopy)
			paramNames := make([]string, len(sigParams))
			for i, p := range sigParams {
				paramNames[i] = p.Name
			}
			var okFn bool
			// The body's first-token position locates the compiled unit for
			// a return-type error stamped at the VM's RET. It cannot equal
			// the interpreter's call-site column (one unit serves every call
			// site), but it keeps the compiled error from reporting an
			// unknown position. Empty body falls back to a zero pos.
			var fnPos SrcPos
			if len(bodyCopy) > 0 {
				fnPos = bodyCopy[0].Pos
			}
			fnUnit, finishFn, okFn = es.StartFnCompile(key, nameCopy, r, genArgs, declaredReturns, paramNames, capturesCopy, genSpec != nil, fnPos)
			if !okFn {
				fnUnit = -1
			}
			if fnUnit >= 0 {
				// Record the declared PARAM types so the VM enforces them at
				// CALL_USER entry (the gradual-Any param-guard, mirroring the RET
				// return-check). A gradual (Dynamic) arg optimistically matched a
				// concrete param at check time; the compiled call must re-check the
				// runtime value, or a laundered mismatch silently runs the body.
				pts := make([]*Type, len(sigParams))
				pats := make([]*Value, len(sigParams))
				for i := range sigParams {
					pts[i] = sigParams[i].Type
					pats[i] = sigParams[i].Pattern
				}
				es.SetUnitParamTypes(fnUnit, pts, pats)
			}
			if finishFn != nil {
				// A fresh compilation must RECORD the body into THIS unit — drop any
				// summary cached by a prior analysis (the install-time synthetic
				// example eval, or a DISCARDED closure PROBE compile that shares
				// r.Check.FnSummaries) so AnalyseFnBody re-runs and re-records the
				// body's events instead of returning the cached residual with an
				// empty fragment. The cache (and the AnalyseFnBody call below) is
				// keyed on genArgs, NOT args — deleting the args key left the
				// genArgs-keyed probe summary live, so a fn dispatched under a
				// recursive closure probe (aql:test run-case) compiled to an EMPTY
				// stub unit in the real pass (silent 0-cases miscompile).
				delete(r.Check.FnSummaries, FnAnalysisKey(r.AnalysisScopeID(), nameCopy, genArgs, capturesCopy, bodyCopy))
				stkGen := AnalyseFnBody(r, nameCopy, paramNames, bodyCopy, genArgs, capturesCopy, declaredReturns)
				finishFn(stkGen)
			}
		}
		stk := AnalyseFnBody(r, nameCopy, paramNames, bodyCopy, narrowArgsToParams(args, sigParams), capturesCopy, declaredReturns)
		for i := len(genNames) - 1; i >= 0; i-- {
			r.Defs.Pop(genNames[i])
		}
		if genSpec == nil {
			var retPos SrcPos
			if len(bodyCopy) > 0 {
				retPos = bodyCopy[0].Pos
			}
			unnamedCount := 0
			for _, p := range sigParams {
				if p.Name == "" {
					unnamedCount++
				}
			}
			checkBodyReturnConformance(r, nameCopy, declaredReturns, unnamedCount,
				allConcreteArgs(args), stk, retPos, bodySpanEnd(bodyCopy))
		}
		if len(declaredReturns) > 0 {
			out := make([]Value, len(declaredReturns))
			for i, t := range declaredReturns {
				// A return slot naming a type parameter refines to the
				// call's inferred binding. An uninferable parameter is
				// reported (unbound_param) and degrades to dynamic(Any)
				// — never a silent strict Any.
				if genSpec != nil {
					if pname := TypeParamName(t); pname != "" {
						if b, ok := genBindings[pname]; ok {
							out[i] = GenBindingCarrier(r, b)
							continue
						}
						// Dedupe identical emissions: the ReturnsFn runs
						// once per analysed call shape AND once for the
						// dynamic-help example generator's synthetic
						// invocation at install — the same text twice
						// helps nobody (the FnSummaries memo dedupes the
						// body analysis but not this substitution).
						detail := fmt.Sprintf(
							"%s: type parameter %s cannot be inferred from this call's arguments — the declared return type is unknown here",
							nameCopy, pname)
						dup := false
						for _, d := range r.Check.Diagnostics {
							if d.Code == "unbound_param" && d.Detail == detail {
								dup = true
								break
							}
						}
						if !dup {
							r.Check.AddDiagnostic(CheckDiagnostic{
								Code:   "unbound_param",
								Detail: detail,
								Word:   nameCopy,
								// A FN whose return names an uninferable type
								// parameter still RUNS (it returns whatever the
								// body produces; the result degrades to
								// dynamic(Any) below) — unlike an uninferable
								// `make` parameter, which cannot construct the
								// instance and is a hard error. So this is a
								// non-gating precision REPORT, not a defect
								// (spec: generics-fn.tsv §8 `loose` "the checker
								// reports the precision loss"). Warning severity
								// overrides the code map's default Error.
								Severity: SeverityWarning,
							})
						}
						c := NewCarrier(TAny)
						c.Dynamic = true
						out[i] = c
						continue
					}
				}
				// A fn DECLARED to return a Function whose body produced a
				// CONCRETE closure value (a real FnDefInfo — e.g. a returned
				// lambda `([] => [c1])`) surfaces that value rather than an
				// abstract Function carrier. The carrier has no FnDefInfo, so
				// binding it (`def g (mkfn)`) and later referencing `g` would
				// fail to dispatch (undefined_word) — the closure-render gap.
				// Cloned for a fresh ID (operand-provenance, like the concrete
				// list/map fold in getNodeReturns). Only for a concrete fn
				// residual aligned to this return slot; anything else keeps the
				// carrier below.
				//
				// PLAIN-CHECK ONLY (!Compiling): during a REAL compile pass the
				// emitter models a returned closure through its own PUSH_CLOSURE
				// machinery, and surfacing the concrete FnDefInfo here instead
				// reorders the recorded call residual and detaches the capture
				// from its construction site — the factory-apply / returned-
				// capturing-closure units then refuse ("capture … unreachable",
				// "call results reordered"). The checker's precision win (a
				// bindable, dispatchable closure) is only needed on the plain
				// check pass; the compile pass keeps the carrier and compiles the
				// closure as before.
				if !r.Check.Compiling &&
					(t.ConformsTo(TFunction) || t.ConformsTo(TFnDef)) && len(stk) >= len(declaredReturns) {
					if bv := stk[len(stk)-len(declaredReturns)+i]; IsConcrete(bv) &&
						(bv.Parent.ConformsTo(TFunction) || bv.Parent.ConformsTo(TFnDef)) {
						out[i] = CloneValue(bv)
						continue
					}
				}
				// A declared USER-UNION return (`def id fn [[x:T] [T] [x]]`
				// with `def T (Integer tor String)`) must DISTRIBUTE over
				// downstream dispatch: a bare carrier TAGGED T carries no
				// DisjunctInfo, so sigTypeMatches' strict-disjunct branch
				// never fires and `(id 1) add 1` failed no_signature against
				// add's Number slot — the THIRD multi-denotation carrier shape
				// after dynamic and payload-bearing disjuncts (the distribute-
				// over-dispatch invariant, checker-accuracy-review.10.md §3).
				// Surface the alternatives so disjunctPartitionReturns joins
				// the per-alternative dispatches, like an inline `tor` result.
				if dv, ok := UnionCarrierForType(CanonicalType(r, t)); ok {
					out[i] = dv
					continue
				}
				c := NewCarrier(t)
				// A declared `Any` return is "statically unknown", not "the Any
				// root": a STRICT Any conforms to no typed slot, so a user fn
				// declaring `[Any]` poisoned every typed consumer downstream with
				// false no_signature errors (a `[Any]`-returning node lookup fed
				// into another fn's `nd:Map` param — the trie/decision walkers).
				// Mark it dynamic for optimistic matching, mirroring the native
				// `[Any]`-return handling in carrierResults.
				if t.Equal(TAny) {
					c.Dynamic = true
				}
				out[i] = c
			}
			if fnUnit >= 0 {
				pos := SrcPos{}
				if len(args) > 0 {
					pos = args[0].Pos
				}
				es.RecordUserCall(fnUnit, args, out, pos)
			} else if polyPlan != nil {
				// Ambiguous multi-overload dispatch with every arm baked: record
				// the runtime-re-matched poly call (OpCallUserPoly). The out
				// carriers are the committed sig's declared returns — identical
				// across arms by the poly gate, so downstream typing is sound
				// whichever arm the VM selects.
				pos := SrcPos{}
				if len(args) > 0 {
					pos = args[0].Pos
				}
				es.RecordUserPolyCall(nameCopy, r, polyPlan.sigIdx, polyPlan.units, polyPlan.impls, args, out, pos)
			}
			return out
		}
		if len(stk) == 0 {
			// No declared returns and an empty body residual: the fn produces
			// ZERO values at run time (`def v fn [[x:Integer] [] []]  v 1 99`
			// → [99]; `def r (f 1)` → def_error: nothing to bind). When the body
			// unit compiled, record a 0-output CALL_USER so the residual matches
			// runtime exactly (the call runs for its effects; the next token is
			// the residual) and return zero values. When the unit did NOT compile
			// (fnUnit < 0, plain check mode), keep the lenient Any approximation
			// so downstream provenance refuses and the program falls back.
			if fnUnit >= 0 {
				pos := SrcPos{}
				if len(args) > 0 {
					pos = args[0].Pos
				}
				es.RecordUserCall(fnUnit, args, nil, pos)
				return nil
			}
			return []Value{NewCarrier(TAny)}
		}
		// Undeclared fn (anonymous lambda, 0-return fn) with a non-empty body
		// residual: the body's residual IS the result. Record the call with
		// those N carriers so downstream resolves them to this dispatch.
		if fnUnit >= 0 {
			pos := SrcPos{}
			if len(args) > 0 {
				pos = args[0].Pos
			}
			es.RecordUserCall(fnUnit, args, stk, pos)
		}
		return stk
	}
}

// InstallFnDef compiles a function definition's own overloads and pushes a
// single DefStack entry holding them. Each compiled Signature keeps its
// authored shape (Params with names, Returns, Body) and gains a body-splicing
// Handler (binds named params via InstallDef, returns body tokens, appends
// undef cleanup) plus a check-mode ReturnsFn. The cross-stack dispatch table
// (union of stacked entries + sort + synthetic 0-arg fallback) is assembled on
// demand by Registry.Lookup → aggregateDispatch, so this entry stays its own
// authored unit for targeted undef and overlap detection.
func InstallFnDef(r *Registry, name string, fnDef FnDefInfo, stackOnly ...bool) {
	isStackOnly := len(stackOnly) > 0 && stackOnly[0]
	entry := fnDef
	entry.Name = name
	entry.Signatures = compileFnSigs(r, name, fnDef, isStackOnly)
	SortSignatures(entry.Signatures)
	entry.MaxForwardArgs = calcMaxForwardArgs(entry.Signatures)
	r.Defs.Push(name, NewFnDef(entry))
	// Construction-time body check (first-class, post-binding). Replaces the
	// dynamic-help example eval's accidental side-channel: that eval ran each fn
	// body against SYNTHETIC example args ({a:1,b:2}), producing false positives
	// (decision.aql), and is now hermetic. Here we analyse each AQL-bodied
	// overload ONCE against CARRIER args (NewCarrier(param) — an abstract Map/List
	// reads dynamic(Any)), post-binding so recursive self-refs resolve, so a fn
	// that is DEFINED BUT NEVER CALLED still has its body statically checked
	// (an undefined word, an in-body forward strand). Bytecode recording is
	// suspended (a registration is not part of the program's straight line; the
	// armed compile at first dispatch deletes this suspended summary and re-records).
	checkFnBodyAtConstruction(r, name, entry)
	if r.ready && r.OnRegisterHook != nil {
		r.OnRegisterHook(name)
	}
}

// compileFnSigs turns a definition's authored overloads into dispatch-
// ready Signatures: optional params expand into extra sigs, AQL-bodied
// sigs get the body-splicing runner (buildFnBodyHandler) plus the
// check-mode ReturnsFn, and the BarrierPos sentinel resolves. Shared by
// InstallFnDef (ordinary `def name fn […]`) and BuildWordExtension
// (`def <locked word> fn […]` — the open-words merge), so an added
// signature dispatches byte-identically to an installed fn's.
func compileFnSigs(r *Registry, name string, fnDef FnDefInfo, isStackOnly bool) []Signature {
	// Expand optional parameters into additional signatures.
	sigs := ExpandOptionalSigs(name, fnDef.OwnSigs())
	compiled := make([]Signature, 0, len(sigs))
	for _, sig := range sigs {
		s := sig           // capture for closure
		fnDefCopy := fnDef // capture for closure (we need Captured at call time)

		// BarrierPos sentinel resolution (No Zero-Value Overload): -1 =
		// default all-forward → len(Params); 0 = explicit all-stack; >0 =
		// explicit barrier. The `/s` modifier on the def name pins it to 0
		// so the fn is stack-only regardless of its FnSig default.
		barrier := s.BarrierPos
		if isStackOnly {
			barrier = 0
		} else if barrier == BarrierAllForward {
			barrier = len(s.Params)
		}

		cs := s // keep Params (names), Returns, NoEval*, Impl
		// AQL-bodied sigs get the body-splicing runner in a fresh AQLImpl. A sig
		// that already carries a Go handler with NO AQL Body is a pre-compiled /
		// synthetic overload — a `usurp` wrapper (whose handler re-dispatches
		// the wrapped fn through a paren) or a captured native fn value bound
		// to a name. Wrapping the body-runner around its empty Body would
		// emit zero values and fail the return check, so preserve the existing
		// Impl/ReturnsFn instead; the fn it forwards to performs its own
		// arg-binding and return validation. (An already-compiled AQL fn
		// re-bound by name still has Body>0, so it correctly gets a fresh
		// body-runner — only Body-less handler sigs are preserved.)
		if s.dispatchHandler() == nil || len(s.body()) > 0 {
			meta := &FnFrameMeta{
				Name:         name,
				HasGen:       fnDefCopy.Gen != nil,
				InstallNames: fnInstallNames(s, fnDefCopy.Captured),
			}
			cs.Impl = &AQLImpl{
				Body:     s.body(),
				FnFrame:  meta,
				dispatch: buildFnBodyHandler(r, name, s, fnDefCopy, meta),
			}
			cs.ReturnsFn = buildFnBodyReturnsFn(r, name, s, fnDefCopy)
		}
		cs.BarrierPos = barrier
		normalizeSig(&cs)
		compiled = append(compiled, cs)
	}
	return compiled
}

// checkFnBodyAtConstruction runs a static body pass for each AQL-bodied overload
// of a freshly-installed fn, against generalised (carrier) args, so an UNCALLED
// fn's body is still checked (a called fn is additionally checked per call site
// via buildFnBodyReturnsFn). Check-mode only; bytecode recording suspended; the
// fn name must already be bound (recursion). Generic and Body-less (native /
// handler) overloads are skipped — a generic body needs per-call type bindings,
// and a native handler has no AQL body to analyse.
func checkFnBodyAtConstruction(r *Registry, name string, fnDef FnDefInfo) {
	if r == nil || !r.Check.IsActive() || fnDef.Gen != nil {
		return
	}
	for i := range fnDef.Signatures {
		s := &fnDef.Signatures[i]
		if s.Fallback || len(s.body()) == 0 {
			continue
		}
		paramNames := make([]string, len(s.Params))
		genArgs := make([]Value, len(s.Params))
		for j, p := range s.Params {
			paramNames[j] = p.Name
			genArgs[j] = ParamInputCarrier(p.Type)
		}
		var declared []*Type
		if !fnDef.Anonymous {
			declared = append([]*Type(nil), s.Returns...)
		}
		// ISOLATE the analysis in a throwaway EmitState (not just Suspend): the
		// body is analysed against the DECLARED param types, which for an abstract
		// param (a surface like `s:Shape`) takes the surface-shape dispatch path
		// and calls MarkUncompilable. Suspend stops recording but does NOT prevent
		// MarkUncompilable from latching the program's EmitState.Compilable — so a
		// construction-time check of a fn with a surface param would wrongly mark
		// the WHOLE program uncompilable (surface.tsv:32 refused for exactly this).
		// IsolateEmit swaps a fresh EmitState for the analysis (discarded on
		// restore), so MarkUncompilable / recording land on the throwaway; the
		// emitted DIAGNOSTICS (undefined_word for an uncalled body typo, the strand
		// advisory) live on r.Check.Diagnostics and are unaffected — exactly the
		// diagnostics-only contract this pass needs.
		restore := r.Check.IsolateEmit()
		AnalyseFnBody(r, name, paramNames, s.body(), genArgs, fnDef.Captured, declared)
		restore()
	}
}

// UninstallFnSigs removes specific function signatures from a word's DefStack.
// For each spec in the FnUndefInfo, it finds and removes the most recent
// DefStack entry whose own overloads include a matching signature. Removing
// the entry is sufficient — the dispatch table is rebuilt on demand by
// Registry.Lookup → aggregateDispatch from whatever entries remain.
func UninstallFnSigs(r *Registry, name string, specs FnUndefInfo) {
	stack := r.Defs.Stack(name)
	if len(stack) == 0 {
		return
	}
	stack = append([]Value(nil), stack...)

	// For each spec, find and remove the most recent matching DefStack entry.
	for _, spec := range specs.Sigs {
		for j := len(stack) - 1; j >= 0; j-- {
			fnDef, ok := stack[j].Data.(FnDefInfo)
			if !ok {
				continue
			}
			// Locked signatures can never be removed — a targeted undef
			// spec matching a native registration (or a word-extension
			// clone, which carries the locked base sigs) must not drop
			// the entry (design/OPEN-WORDS.0.md §2.3).
			if hasLockedSig(fnDef.Signatures) {
				continue
			}
			matched := false
			for _, sig := range fnDef.OwnSigs() {
				if FnSigMatchesSpec(sig, spec) {
					matched = true
					break
				}
			}
			if matched {
				stack = append(stack[:j], stack[j+1:]...)
				break
			}
		}
	}

	r.Defs.Set(name, stack)
}

// CoerceBoolean converts any value to a boolean using the same rules
// as `convert boolean`: booleans pass through, numbers are non-zero,
// none is false, lists/maps are non-empty, "true"/"false" parse
// literally, all other values are non-empty.
func CoerceBoolean(v Value) bool {
	switch {
	case ValueType(v).ConformsTo(TBoolean):
		b, _ := AsBoolean(v)
		return b
	case ValueType(v).ConformsTo(TNumber):
		n, _ := AsNumber(v)
		return n != 0
	case ValueType(v).Equal(TNone):
		return false
	// nodeFamily folds FlexList / FlexMap into their immutable parents
	// so empty flex containers are falsey exactly like empty plain
	// ones (PR #123 review note).
	case nodeFamily(ValueType(v)).Equal(TList):
		if !IsConcrete(v) {
			return false
		}
		if elems, err := AsMutableList(v); err == nil {
			return len(elems) > 0
		}
		// Non-[]Value list backings (table types, query builders) are truthy.
		return true
	case nodeFamily(ValueType(v)).Equal(TMap):
		if !IsConcrete(v) {
			return false
		}
		if om, err := AsMutableMap(v); err == nil {
			return om.Len() > 0
		}
		// Non-*OrderedMap map backings (record/options/child types) are truthy.
		return true
	}
	text := ValToString(v)
	// A String is falsy only when empty. The former magic-token rule —
	// where the String "false" was the one non-empty string treated as
	// falsy, so "FALSE"/"0"/"no" were truthy but "false" was not — is
	// removed (WAT-AUDIT.5.md §E): string CONTENT is never inspected.
	if ValueType(v).ConformsTo(TString) {
		return text != ""
	}
	// A non-String that renders as "false" is an unresolved boolean
	// literal reaching truthiness as a Word/Atom (a bare `false` clause
	// condition in `if [false …]`, a quoted `false` atom). It keeps its
	// boolean reading; everything else is truthy unless it renders empty.
	switch text {
	case "false", "":
		return false
	default:
		return true
	}
}

// CowSet performs a copy-on-write set on a Store. It creates a new Store
// layer whose prototype is the old Store, sets the key in the new layer,
// and propagates the update up through parent Stores to the ctxStack.
func CowSet(store *StoreInstanceInfo, key string, val Value, r *Registry) {
	// Create new COW layer: only the changed key, prototype = old store.
	newStore := &StoreInstanceInfo{
		TypeName:  store.TypeName,
		Data:      map[string]Value{key: val},
		Prototype: store,
		Parent:    store.Parent,
		ParentKey: store.ParentKey,
	}

	// Track parent for nested Store values.
	if childStore, ok := val.Data.(*StoreInstanceInfo); ok {
		childStore.Parent = newStore
		childStore.ParentKey = key
	}

	// Propagate up the parent chain: each parent Store gets a new COW
	// layer with the updated child reference.
	current := newStore
	parent := store.Parent
	parentKey := store.ParentKey

	for parent != nil {
		newParent := &StoreInstanceInfo{
			TypeName:  parent.TypeName,
			Data:      map[string]Value{parentKey: NewStoreValue(nil, current)},
			Prototype: parent,
			Parent:    parent.Parent,
			ParentKey: parent.ParentKey,
		}
		current.Parent = newParent
		current.ParentKey = parentKey

		current = newParent
		parentKey = parent.ParentKey
		parent = parent.Parent
	}

	// current is the topmost COW'd Store. Update the ctxStack entry that
	// references the original store (either directly or via prototype chain).
	// The topmost COW'd store's prototype is the original root store.
	// Walk each ctxStack entry's prototype chain to see if it passes
	// through the original root, and if so, create a new ctxStack entry
	// that uses the COW'd store.
	// current's prototype is never nil here: current is either the
	// initial COW layer (prototype = the store argument) or the last
	// loop's newParent (prototype = a parent the loop condition proved
	// non-nil).
	r.Contexts.UpdateChain(current.Prototype, current)
}

// IsHostTypeBody reports whether v is a constructed type produced by a
// host Ideal: an ExtensionPayload whose Body embeds eng.HostTypeBody.
// The kernel recognises such a value as a type without inspecting its
// concrete shape (the payload Body being opaque). See
// design/IDEAL.10.md §6.
func IsHostTypeBody(v Value) bool {
	ep, ok := v.Data.(ExtensionPayload)
	if !ok {
		return false
	}
	_, ok = ep.Body.(interface{ hostTypeBody() })
	return ok
}

// IsTypeBody reports whether a value is a valid type definition body
// in the strict, structural sense: it carries explicit type-shape
// information (a type literal, disjunct, record / table / object /
// options type, typed list/map, dependent scalar, fn-shape, or
// predicate function).
//
// AQL also lets every concrete value act as a type — `type Foo 1`
// defines Foo as the singleton type containing only 1 — but that
// "literals as types" admission is checked separately via
// IsLiteralTypeBody at the `type` install site, so paths that need
// to discriminate code-bodies / fn-bodies / data-defs from explicit
// type shapes (e.g. `inspect`) keep using IsTypeBody and stay sharp.
func IsTypeBody(v Value) bool {
	// Type literal (Data==nil): number, string, boolean, any, etc.
	// Excludes the value `none` (Data != nil sentinel).
	if IsBareTypeNode(v) {
		return true
	}
	// Implicit-map record shape (`{x:Integer}`): a Map whose backing
	// OrderedMap is flagged Implicit. Used as a structural Node-type
	// declaration body.
	if IsImplicitMap(v) {
		return true
	}
	// Record type
	if IsRecordType(v) {
		return true
	}
	// Options type
	if IsOptionsType(v) {
		return true
	}
	// Table type
	if IsTableType(v) {
		return true
	}
	// Disjunct
	if IsDisjunct(v) {
		return true
	}
	// Negation (complement) type
	if IsNegation(v) {
		return true
	}
	// Typed list [:type]
	if IsTypedList(v) {
		return true
	}
	// Typed map {:type}
	if IsTypedMap(v) {
		return true
	}
	// Bounded Type (`Type of [B]` / `B/t`)
	if IsBoundedType(v) {
		return true
	}
	// Object type
	if IsClassType(v) {
		return true
	}
	// Surface type (pure operation contract)
	if IsSurfaceType(v) {
		return true
	}
	// Generic type schema (gen [...] + constructor)
	if IsTypeSchema(v) {
		return true
	}
	// Dependent scalar type (Integer gt 10, String lt "z", …)
	if v.IsDepScalar() {
		return true
	}
	// Function-signature type: a FnUndef carrying input + output sig
	// patterns and no body.
	if v.Parent.Equal(TFnUndef) {
		return true
	}
	// Predicate type: a FnDef / Function whose body returns a Boolean.
	if v.Parent.Equal(TFnDef) || v.Parent.Equal(TFunction) {
		return true
	}
	// Micron type body (`refine Micron {fields}`)
	if IsMicronType(v) {
		return true
	}
	// Host-Ideal constructed type (ExtensionPayload + HostTypeBody).
	if IsHostTypeBody(v) {
		return true
	}
	return false
}

// PredicateInputType returns the concrete input type of a
// predicate-shaped fn body (a Function or FnDef whose first sig
// takes exactly one argument with a declared type other than Any).
// Returns nil if v isn't a predicate type or the input type is Any
// or unset — those bodies stay parented at TFnDef / TFunction, the
// pre-existing behavior.
//
// Used by InstallType to mint user-defined predicate types with the
// declared input type as their parent so values rewrapped by the
// typed-bind path participate in the LCA-walk dispatch alongside
// kernel scalars. Without this, `behave compare/q (fn [[Positive
// Positive] …])` would have no dispatch surface — no value's Parent
// is ever Positive.
func PredicateInputType(v Value) *Type {
	if v.Parent == nil {
		return nil
	}
	if !v.Parent.Equal(TFnDef) && !v.Parent.Equal(TFunction) {
		return nil
	}
	info, ok := v.Data.(FnDefInfo)
	if !ok {
		return nil
	}
	sig, ok := info.FirstOwnSig()
	if !ok || len(sig.Params) != 1 {
		return nil
	}
	t := sig.Params[0].Type
	if t == nil || t.Equal(TAny) {
		return nil
	}
	return t
}

// IsLiteralTypeBody reports whether v can be installed as a "value-
// is-a-type" type body — the singleton-type interpretation. Scalar
// literals (Integer / Float / String / Boolean / Atom / Pathon / the
// `none` value), and concrete lists / maps qualify. Used by
// installType to relax the strict IsTypeBody check in a way that
// doesn't pollute the inspect / fn-shape paths.
func IsLiteralTypeBody(v Value) bool {
	if IsNone(v) {
		return true
	}
	switch {
	case v.Parent.ConformsTo(TInteger),
		v.Parent.ConformsTo(TFloat),
		v.Parent.ConformsTo(TNumber),
		v.Parent.ConformsTo(TString),
		v.Parent.ConformsTo(TBoolean),
		v.Parent.ConformsTo(TAtom),
		v.Parent.ConformsTo(TMicron):
		return v.Data != nil
	}
	if v.Parent.Equal(TList) && v.Data != nil {
		return true
	}
	if v.Parent.Equal(TMap) && v.Data != nil {
		return true
	}
	return false
}

// ResolveWordValue converts a word value to its semantic value.
// Words named "true"/"false" become booleans, known type names become type
// literals, and other words become atoms (bare strings).
func ResolveWordValue(v Value) Value {
	if !IsWord(v) {
		return v
	}
	_as1, _ := AsWord(v)
	name := _as1.Name
	switch name {
	case "true":
		return NewBoolean(true)
	case "false":
		return NewBoolean(false)
	default:
		if t, ok := typeNames[name]; ok {
			return NewTypeLiteral(t)
		}
		return NewAtom(name)
	}
}

// SimplifyDisjunctAlts filters Never, dedupes structurally identical
// alternatives, and applies subsumption: a strict subtype drops in
// favour of its supertype, and a concrete value drops if some other
// alternative is a covering type literal. Two concrete values of the
// same type are both kept — each one is a distinct piece of
// information that the type literal couldn't replace.
func SimplifyDisjunctAlts(alts []Value) []Value {
	// First pass: drop Never.
	live := make([]Value, 0, len(alts))
	for _, alt := range alts {
		if ValueType(alt).Equal(TNever) {
			continue
		}
		live = append(live, alt)
	}
	// Second pass: keep an alt only if no other live alt subsumes or
	// duplicates it. "Earlier-wins" for duplicates so source order is
	// preserved among survivors.
	out := make([]Value, 0, len(live))
outer:
	for i, cand := range live {
		candType := ValueType(cand)
		// Drop if structurally equal to an earlier kept alt.
		for j := 0; j < i; j++ {
			if ValueType(live[j]).Equal(candType) && ValuesEqual(live[j], cand) {
				continue outer
			}
		}
		// Drop if subsumed by some other alt:
		//   - cand is a type literal whose type is a strict subtype
		//     of another's (Integer subsumed by Number).
		//   - cand is a concrete value covered by another type literal
		//     (5 subsumed by Integer).
		// Strict subtype only: equal types are handled by dedup above.
		for j, other := range live {
			if i == j {
				continue
			}
			otherType := ValueType(other)
			if candType.Equal(otherType) {
				continue
			}
			if !candType.ConformsTo(otherType) {
				continue
			}
			// cand's type is a strict subtype of other's.
			if IsBareTypeNode(cand) && IsBareTypeNode(other) {
				continue outer
			}
			if IsConcrete(cand) && IsBareTypeNode(other) {
				continue outer
			}
		}
		out = append(out, cand)
	}
	// Canonicalise: `tor` is commutative, so a type union's terms are
	// stored in tcmp order. This is the single boundary every user-facing
	// disjunction passes through (TorHandler / unionType / exclude /
	// extract / the check-mode carrier merge), so `None tor 1` and
	// `1 tor None` reduce to the identical value — equal under `tcmp`,
	// identical when printed.
	sort.SliceStable(out, func(i, j int) bool {
		return disjunctAltLess(out[i], out[j])
	})
	return out
}

// FnDefsOverlap returns true if any signature in a has the same parameter
// types as any signature in b (ignoring param names, return types, and body).
func FnDefsOverlap(a, b FnDefInfo) bool {
	for _, sa := range a.OwnSigs() {
		for _, sb := range b.OwnSigs() {
			if len(sa.Params) != len(sb.Params) {
				continue
			}
			match := true
			for i := range sa.Params {
				if !sa.Params[i].Type.Equal(sb.Params[i].Type) {
					match = false
					break
				}
			}
			if match {
				return true
			}
		}
	}
	return false
}

// BaseValue returns the zero/default value for a given type, similar to Go's
// zero values. Used by both the "base" word and "make" with base:true option.
func BaseValue(t *Type) (Value, error) {
	switch {
	case t.ConformsTo(TInteger):
		return NewInteger(0), nil
	case t.ConformsTo(TFloat):
		return NewFloat(0), nil
	case t.ConformsTo(TNumber):
		return NewInteger(0), nil
	case t.ConformsTo(TString):
		return NewString(""), nil
	case t.ConformsTo(TBoolean):
		return NewBoolean(false), nil
	case t.ConformsTo(TList):
		return NewList([]Value{}), nil
	case t.ConformsTo(TMap):
		return NewMap(NewOrderedMap()), nil
	case t.ConformsTo(TNone):
		return NewTypeLiteral(TNone), nil
	case t.ConformsTo(TAtom):
		return NewAtom(""), nil
	default:
		return Value{}, fmt.Errorf("base: unsupported type %s", t.String())
	}
}

// BaseValueForConstraint returns the base value for a field constraint.
// For type literals, returns the zero value directly.
// For disjunctions (e.g. string|none), returns the base of the first
// non-none alternative.
func BaseValueForConstraint(constraint Value) (Value, error) {
	if IsDisjunct(constraint) {
		di, _ := AsDisjunct(constraint)
		for _, alt := range di.Alternatives {
			if IsTypeLiteral(alt) {
				return BaseValue(ValueType(alt))
			}
		}
		return NewTypeLiteral(TNone), nil
	}
	if IsBareTypeNode(constraint) {
		return BaseValue(ValueType(constraint))
	}
	return Value{}, fmt.Errorf("base: cannot determine base value for %s", constraint.String())
}

// omittedDefaultValue returns the value substituted for an omitted
// optional FnParam. Options-typed params get a Map populated with
// each Field's concrete default (fields whose value is a type body —
// type literals, disjuncts, nested type definitions — carry no
// default and are skipped). Non-Options params fall back to BaseValue
// of the param's declared Type.
func omittedDefaultValue(p FnParam) (Value, error) {
	if p.Pattern != nil && IsOptionsType(*p.Pattern) {
		oi, err := AsOptionsType(*p.Pattern)
		if err == nil && oi.Fields != nil {
			m := NewOrderedMap()
			for _, k := range oi.Fields.Keys() {
				fv, _ := oi.Fields.Get(k)
				if IsTypeBody(fv) {
					continue
				}
				m.Set(k, fv)
			}
			return NewMap(m), nil
		}
	}
	return BaseValue(p.Type)
}

// ExpandOptionalSigs takes a list of fn signatures and expands those with
// optional params into the full set of overloaded signatures. Each
// optional combination becomes its own signature whose body calls the
// original function with the omitted params filled by their type's
// base value.
func ExpandOptionalSigs(name string, sigs []FnSig) []FnSig {
	var expanded []FnSig
	for _, sig := range sigs {
		expanded = append(expanded, sig)

		var optIndices []int
		for i, p := range sig.Params {
			if p.Optional {
				optIndices = append(optIndices, i)
			}
		}
		if len(optIndices) == 0 {
			continue
		}

		numOpt := len(optIndices)
		for mask := 1; mask < (1 << numOpt); mask++ {
			omitted := make(map[int]bool)
			for bit := 0; bit < numOpt; bit++ {
				if mask&(1<<bit) != 0 {
					omitted[optIndices[bit]] = true
				}
			}

			var reducedParams []FnParam
			for i, p := range sig.Params {
				if !omitted[i] {
					reducedParams = append(reducedParams, FnParam{
						Name:    p.Name,
						Type:    p.Type,
						Pattern: p.Pattern,
					})
				}
			}

			var body []Value
			body = append(body, NewWord(name))
			// invalid latches when an omitted param has no usable
			// default: either none can be synthesized, or the synthesized
			// value fails the param's own pattern — the SAME Unify
			// predicate dispatch applies (patternsOk), so the two can't
			// drift. In either case the omission combination is skipped
			// entirely below. The historical bug here `continue`d on a
			// synthesis error, DROPPING the argument from the body: the
			// 0-arg overload then re-dispatched the word with the arg
			// still missing (or with a default the original sig rejects),
			// which matched the same 0-arg overload again — an infinite
			// self-recursion that ground the tape until exhaustion.
			// Skipping the overload makes the omission fail dispatch
			// honestly instead ("matched no signature").
			invalid := false
			presentIdx := 0
			for i, p := range sig.Params {
				if omitted[i] {
					bv, err := omittedDefaultValue(p)
					if err != nil {
						invalid = true
						break
					}
					if p.Pattern != nil {
						if _, ok := Unify(bv, *p.Pattern); !ok {
							invalid = true
							break
						}
					}
					body = append(body, bv)
				} else {
					if p.Name != "" {
						body = append(body, NewWord(p.Name))
					} else {
						body = append(body,
							NewOpenParen(),
							NewWord("args"),
							NewAtom(fmt.Sprintf("%d", presentIdx)),
							// dot, not get: the key is a literal atom index
							// (`args.N`); get no longer accepts an atom key.
							NewWord("dot"),
							NewCloseParen(),
						)
					}
					presentIdx++
				}
			}

			// An omitted param with no valid default: this omission
			// combination cannot be satisfied — generate NO overload for
			// it, so calls relying on the omission fail dispatch cleanly.
			if invalid {
				continue
			}

			// Propagate the parent sig's BarrierPos so an
			// optional-param expansion inherits the same forward/
			// stack convention. -1 (the AQL-source default) stays
			// -1; an explicit barrier carries over but clamps to
			// the reduced param count.
			expandedBarrier := sig.BarrierPos
			if expandedBarrier > len(reducedParams) {
				expandedBarrier = len(reducedParams)
			}
			expanded = append(expanded, FnSig{
				Params:     reducedParams,
				Returns:    sig.Returns,
				Impl:       AQL(body),
				BarrierPos: expandedBarrier,
			})
		}
	}
	return expanded
}
