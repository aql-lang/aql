package eng

// The check piece's state declarations, relocated out of registry.go
// (Stage 2c of the four-piece split): the CheckState aggregate, the
// step-budget default, the severity classification, and the diagnostic
// record. Same package until the physical cut. Stage 4a re-homed the
// file to CORE — the state records are core-owned passive data (the
// analysis algorithms above them stay check) — and Stage 4b moved the
// CheckState methods here beside their type.

import "strings"

// CheckState aggregates the static type-checking state that used to
// live as ten loose fields on Registry. Bundling them serves two
// purposes:
//
//   - **Sandboxing.** A predicate body that runs under unify checks
//     should not mutate enclosing analysis state. With a single
//     struct, snapshot/restore is `saved := r.Check; defer func()
//     { r.Check = saved }()` rather than ten parallel assignments.
//   - **Discoverability.** Anyone reading `Registry` can see the
//     check-mode footprint at a glance instead of scanning ten
//     adjacent declarations.
type CheckState struct {
	// CurCallPos is a TRANSIENT scratch: carrierResults writes the current
	// call's source position here immediately before invoking a sig's
	// ReturnsFn, so a ReturnsFn that needs the call site (e.g. `make Array`
	// stamping a typed-Array carrier's make-site identity) can read it — the
	// ReturnsFunc signature carries no pos. Overwritten on every dispatch; not
	// persistent state. Zero = unknown (synthetic/top-level).
	CurCallPos SrcPos
	// Mode toggles static type-checking execution. When true, the
	// engine runs the same dispatch/matching machinery but carries
	// type-only Carrier values instead of concrete payloads, and
	// replaces signature handlers with carrier-typed return
	// propagation (see Signature.Returns). Diagnostics are
	// accumulated into Diagnostics rather than returned as hard
	// errors.
	Mode        bool
	Diagnostics []CheckDiagnostic

	// ModelEffects is the THIRD MODE, and it is deliberately NOT `Mode`.
	//
	// Check mode does two separable things: it strips concrete values down
	// to type carriers, AND it substitutes a signature's ReturnsFn for its
	// handler. A module body needs the second and cannot survive the first —
	// its export names and map keys are concrete string literals that
	// carrier-stripping destroys — which is why `Mode` is deliberately not
	// propagated into a module sub-registry (see the note in
	// native_module_module.go::runModuleBodyCover). The consequence was
	// that `boru check`, documented as "type-check without running", ran
	// every imported module body's effects with full ambient authority.
	//
	// ModelEffects splits the two: values stay CONCRETE (Mode is false, so
	// dispatch, matching and every handler run exactly as at runtime) while
	// the effect BACKENDS are substituted, so a body still produces real
	// export names while its writes go nowhere.
	//
	// The line it draws is WRITES, and that is what makes it safe: reads are
	// untouched, so no module body that loads today can start failing under
	// check. Concretely — a filesystem mutation lands in a mem-over-host
	// OVERLAY (native.EffectiveFileOps), so a body that writes a file and
	// then reads it back still sees its own bytes; output writes go to
	// io.Discard, which returns the same byte count the real writer would.
	// A modelled effect is invisible to the program, not merely suppressed.
	//
	// Not per-pass state: this is a property of the registry's ROLE (a
	// module sub-registry created while its parent was running a PURE check
	// pass), not of a check pass, so Begin() does not reset it. It propagates
	// transitively — a module imported from a module body inherits it.
	//
	// A COMPILE pass (Compiling) is deliberately excluded at the propagation
	// site: on the compiled path the compile pass's module-body execution is
	// the run's only one, so modelling there would delete the effect rather
	// than deduplicate it.
	//
	// Two classes stay REAL and are known residuals, recorded in
	// design/verse-report-defects-investigation.0.md §D: a network send and
	// stdin (a read, so out of scope by the rule above). A substitutable
	// transport DOES exist for the first (capabilities.HTTPOps), so the
	// reason is not "no seam" — it is that no synthetic response is safe to
	// invent: a fabricated status takes a wrong branch and a fabricated body
	// fails a real parse, so modelling would break bodies that work. The
	// effect ledger
	// (effects.go) is therefore left counting modelled writes too: over-
	// counting only forgoes a safe interpreter fallback, while under-
	// counting a real network send would duplicate it.
	ModelEffects bool

	// FnSummaries caches carrier return-stacks for user-defined fn
	// bodies keyed by (name + "#" + argTypesJoined). Populated by
	// analyseFnBody; re-entrant calls (recursion) consult this
	// cache to break cycles and converge on a fixed point.
	FnSummaries map[string][]Value

	// FnInflight tracks which (name, arg-types) analyses are
	// currently running so that recursive calls can bail out with
	// a placeholder instead of looping.
	FnInflight map[string]bool

	// FnNameInflight counts, per fn NAME, how many of its body analyses
	// are on the stack. A recursive self-call with a DIFFERENT arg shape
	// has a different FnInflight key, so it does not bail — it re-analyses
	// the same body tokens under the narrowed args. Those re-analyses must
	// not RE-EMIT body diagnostics: the first (non-recursive) analysis of
	// the same body already reports any real error, while a call-shape that
	// narrowed a param to a strict Any can spuriously fail dispatch
	// (the trie fuzzy-go recursion's `kid-items`/`get` cascade). When a
	// name is already in-flight, SuppressBodyErrors is raised for the
	// re-entry so only the canonical analysis's diagnostics stand.
	FnNameInflight map[string]int

	// SuppressBodyErrors, when > 0, drops error-level diagnostics emitted
	// during a recursive fn-body RE-ENTRY (see FnNameInflight). Sound: the
	// re-entry re-runs body tokens the outer analysis already checked.
	SuppressBodyErrors int

	// CaughtBodyDepth, when > 0, marks analysis running inside an
	// error-CATCHING body — `do [body]` traps every body error and
	// surfaces it as an Error VALUE, so a guaranteed-runtime-error mirror
	// (a strict-accessor static miss, a provable index OOB, a make
	// construction failure) that fires in the region is NOT a program
	// error and must stay silent (`do [{a:1} !. b] error [dot message]`
	// is a working program). Raised around doListReturnsFn's body run;
	// consulted by CheckAddUniqueDiagnostic and emitIndexOOB.
	CaughtBodyDepth int

	// NestedBodyDepth, when > 0, marks analysis running inside ANY nested
	// body region (RunCarrierBodyWithDefs — if/case branches, loop bodies,
	// quotation/closure bodies). A diagnostic that is only sound for
	// unconditionally-reached code (the top-level unconditional-raise
	// mirror) consults it: a branch or loop body may never execute, so
	// firing there would flag working guard idioms.
	NestedBodyDepth int

	// SpecBaselines is the stack of def-depth snapshots taken at each open
	// SPECULATIVE region entry — a rolled-back nested body (keep=false in
	// runCarrierBodyDefsAdds: branch arms, loop/quotation bodies, handler
	// probes) or a fn-body analysis (AnalyseFnBody). The `undef` handler
	// consults the innermost: popping a binding whose depth does not exceed
	// the region-entry snapshot would delete an ENCLOSING binding from the
	// model for the rest of the pass — a region the runtime may never
	// execute — which flagged `undefined_word` on clean programs (`def x 1
	// do [7] error [undef x 9] x`; the same leak fired from each-bodies and
	// uncalled fn bodies — the wrapped-undef FP class, completeness-review
	// §9.12). In-region bindings (params, body defs — depth above the
	// snapshot) still pop normally, so frame teardown is untouched. `do`
	// bodies (keep=true) push nothing: their defs and undefs leak by
	// design (leak fidelity), matching the runtime.
	SpecBaselines []map[string]int

	// LoopBodyDepth, when > 0, marks analysis running inside a PROVEN
	// counted-for LOOP body (AnalyseLoopBody brackets each round's body run,
	// gated on its provenTrips arg AND a sentinel-free body). Unlike the
	// other NestedBodyDepth contributors (branch arms, quotation bodies), a
	// proven loop body executes UNCONDITIONALLY once per iteration AND at
	// least once — so when NestedBodyDepth == LoopBodyDepth every enclosing
	// body is such a loop body and a per-iteration binding is definitely
	// reached with its residual intact. The S5 first-value loop split
	// (SplitLoopRegionBind) consults exactly that equality for the
	// loop-carried variadic def (REFUSAL-CLOSURE S9.2a); a branch arm keeps
	// the decline (a conditionally-reached split would leak the
	// analysis-only binding — PR #278 review P1-b), and a computed-count or
	// break/continue-bearing loop body never stamps (a zero-trip run leaks
	// the binding; loop control bypasses the site or discards its values —
	// PR #280 review).
	LoopBodyDepth int

	// CondBodyDepth, when > 0, marks analysis running inside a
	// CONDITIONALLY-reached, def-rolled-back body — an if/case branch arm
	// or a loop body (the `keep=false` bodies of runCarrierBodyDefsAdds,
	// whose net def growth is snapshot-restored). It EXCLUDES `do`
	// (keep=true — always executes, leaks its defs by design). The compiler
	// consults it to refuse a fn REDEFINITION that clobbers an enclosing
	// binding in-place (the overlap-removal drops the outer overload without
	// growing depth, so the branch rollback can't restore it): compiled
	// resolution would statically bake the conditional shadow while the
	// interpreter keeps the outer fn when the branch is not taken. Refusing
	// keeps compiled == interpreter (slow, not wrong).
	CondBodyDepth int

	// InflightBails counts Any-placeholder bail-outs taken by
	// recursive calls of UNCHECKED fns (declared returns use the
	// declaration instead and don't count). AnalyseFnBody compares
	// the counter around a body run to know whether its summary was
	// computed under the weakest hypothesis and needs refinement
	// before being cached (design/checker-accuracy-review.10.md A2).
	InflightBails int

	// Emit is the bytecode recorder seam (EmitRecorder). A real
	// *EmitState — installed by the compile entry points after Begin —
	// turns the check pass into the bytecode recording pass (Stage 1 of
	// design/boru-bytecode-plan.0.md): every dispatch through
	// carrierResults records a classified call event and Finalize
	// linearises the trace into a Program. A plain check runs against
	// the inactive no-op recorder (Begin installs it). READ through
	// CheckState.Recorder(), which substitutes the no-op for a nil
	// field; write only from the pass entry points / probe forks.
	Emit EmitRecorder

	// Strict enables the STRICT-MODE advisory surface (`boru check
	// --strict`): every committed dispatch over a dynamic operand emits a
	// non-gating dynamic_dispatch info, making the gradual frontier loud
	// (checker-accuracy-review.10.md "--strict mode"). Persistent config —
	// set by the caller BEFORE Begin (which must not reset it).
	Strict bool

	// Compiling marks a REAL compile pass (CompileCheck / RunCompiled),
	// whose recorded events become an executed Program. A plain `boru check`
	// leaves it false even though fn-body analysis arms transient Emit
	// states. Some check-only precision relaxations (modelling a runtime-
	// arity-variable result as a consumable value) are sound for diagnostics
	// but would feed the recorder an arity the VM cannot honour, so they are
	// gated to !Compiling. Set by the compile entry points after Begin.
	Compiling bool

	// FnAnalysisCounts tracks distinct body analyses (memo misses)
	// per fn DEFINITION SITE (fnQuotaKey: scope + name + body position,
	// NOT bare name — every higher-order closure shares a synthetic
	// "<word>$body" name, so a name-only key pooled unrelated closures
	// across the whole program). Past FnAnalysisQuota the analyser stops
	// re-running the body for new arg shapes — it answers from the
	// declaration or dynamic(Any) — and emits ONE analysis_truncated
	// diagnostic naming the fn, so heavy polymorphic use degrades loudly
	// instead of silently eating the whole step budget
	// (design/checker-accuracy-review.10.md A9).
	FnAnalysisCounts map[string]int

	// StepCount is the running total of engine steps consumed by
	// the current check run, summed across every sub-engine. Used
	// with StepBudget to cap total analysis effort.
	StepCount int

	// StepBudget is the maximum total steps the check run may
	// consume. The "unset" sentinel is -1 — that's what gets
	// substituted with DefaultCheckStepBudget at run time. A real
	// zero is honored as "abort on the first step", which is
	// rarely useful but unambiguous. Once the running count
	// exceeds the resolved budget, the engine emits a
	// step_budget_exceeded diagnostic and returns immediately.
	StepBudget int

	// BudgetTripped is set to true after the first budget overshoot
	// so we emit at most one diagnostic per check run.
	BudgetTripped bool

	// SuppressedRuntimeError is set when a word that is deliberately
	// lenient in check mode skips an error the interpreter raises at
	// runtime — an orphan `gen [...]` (gen_without_constructor), an
	// `unpack` of a missing key (unpack_error). The bytecode compiler
	// reads it after the check pass: the compiled stream elides such
	// words, so it would silently succeed where the interpreter errors —
	// the program is therefore uncompilable and must fall back.
	SuppressedRuntimeError bool

	// AmbiguousGradualSplit is set when matchSignature's forward/stack
	// split for a dispatch depended on whether a GENUINELY MIXED gradual
	// carrier (a Disjunct with some alternatives conforming to a more-
	// specific overload's slot and some not — e.g. `and 0 false` typed
	// Disjunct(Integer,Boolean)) matched that overload. At runtime the
	// concrete value resolves the split one way; the check pass picked the
	// other (a less-specific overload that forward-collects instead of
	// grabbing the carrier from the stack). The two splits produce
	// different result stacks, so the bytecode compiler reads this after
	// the check pass and refuses — the program is uncompilable and must
	// fall back to the interpreter. Dispatch itself is unchanged; this is
	// a compile-time advisory only.
	AmbiguousGradualSplit bool

	// DefsInstalled records the names (and source positions) that
	// the user's program defined during a check run via the def
	// word. Populated by RecordCheckDef; consulted at end of run
	// to emit unused_def warnings.
	DefsInstalled map[string]SrcPos

	// DefsUsed records names looked up via Registry.Lookup or
	// simple-value substitution in check mode. Used to filter out
	// defs that were referenced at least once.
	DefsUsed map[string]bool

	// ContextTypes is a best-effort record of keys that user code
	// wrote to a Store during a check run. The value is the
	// last-seen carrier type for that key, joined via JoinCarriers
	// on repeated writes. Used by get's ReturnsFn so subsequent
	// reads can produce a typed carrier rather than falling back to
	// Any. Shared across the entire check run — not keyed by store
	// identity. It remains the COMPATIBILITY FALLBACK for any store
	// the shape minting misses (design/checker-precision-fronts.0.md
	// §2 stage 3 retires it only when every reader is store-shaped);
	// store-identity-keyed typing lives on StoreShapeInfo carriers
	// (store_shape.go).
	ContextTypes map[string]Value

	// CtxShapes maps a LIVE context-store layer to its abstract
	// StoreShapeInfo carrier, minted lazily by the `context` word's
	// check-mode ReturnsFn (CheckState.ContextShape). Pointer-keyed
	// deliberately: check mode never runs the runtime COW replace, so
	// a layer's pointer is stable for its scope, and holding it as a
	// key keeps the layer reachable (no recycled-allocation aliasing).
	// Per-pass state — reset by Begin, header-cloned by Clone (the
	// shape pointees stay shared; all shape mutation is join-only, so
	// a sandbox leak can only widen — see store_shape.go).
	CtxShapes map[*StoreInstanceInfo]Value

	// MethodShapes maps a dynamic method-read carrier's value ID to the
	// resolved MEMBER value — the trivial-delegation wrapper FnDef a
	// get-family read surfaced from a shape-instance container (a logger /
	// span / instrument / rand handle whose check-mode ReturnsFn instance
	// resolves method SIGNATURES; the runtime instance carries per-call
	// state, so the member itself must never bake — the freeze-gate).
	// Minted by the accessor ReturnsFn via NoteMethodShape (which vets the
	// member: delegation wrapper, named, foreign sub-registry, no genuine
	// 0-arg overload — the miscompile-E auto-dispatch guard's class stays
	// out); consumed by the compile pass's shaped-method model
	// (tryShapedMethodDispatch, method_shape.go). Per-pass state — reset by
	// Begin, header-cloned by Clone (members are immutable values).
	MethodShapes map[string]Value

	// PendingMethodApply threads ONE modelled shaped-method dispatch from
	// tryShapedMethodDispatch into recordDispatchOutcome (set immediately
	// before the model's carrierResults call, consumed by
	// tryRecordMethodApply — the first specialist in the outcome chain — so
	// the member's native never records as a check-time CALL_NATIVE, which
	// would bake the shape instance's state: the freeze-gate). Transient
	// within a single dispatch; reset by Begin.
	PendingMethodApply *PendingMethodApply

	// CodeEffectDepth counts nested code-effect body analyses
	// (AnalyseCodeEffectCarrier — the typed-code-value producer). A
	// stored code body that itself reads stored code (`quote [ops get
	// 0 do]`) would recurse through the element-read producer
	// unboundedly, so the producer declines past depth 1 — nested code
	// stays dynamic(Any), a stage-2/3 precision
	// (design/checker-precision-fronts.0.md §1).
	CodeEffectDepth int

	// FnBodyDepth counts the AnalyseFnBody nesting around the
	// current dispatch. Diagnostics emitted while it is positive
	// come from a fn BODY — code that runs at call time, not at the
	// point of analysis — so an undefined_word there may be a legal
	// forward reference (the documented mutual-recursion idiom:
	// `def isod fn […isev…] def isev fn […isod…]`). Such
	// diagnostics are tagged FnBody and rescued at end of pass when
	// the name has a binding by then (RescueForwardRefDiagnostics).
	FnBodyDepth int

	// ArgsFrameUnnamed reports whether the fn body CURRENTLY under analysis
	// (the one whose args projection is on top of r.Args) has at least one
	// UNNAMED (stack-flowing) parameter. Set and save/restored by
	// runFnBodyOnce in lockstep with the args projection push, so it always
	// reflects the innermost active body. `args` / `args.N` folds soundly to
	// a frame local only when every param is NAMED; with an unnamed param the
	// input stays live on the body stack and folding args.N strands it
	// (a compile≠interpret divergence — design/EDGE-SPEC-FINDINGS.0.md §4), so
	// specialWordResults refuses the program in compile mode when this is set.
	ArgsFrameUnnamed bool

	// FnNameStack is the stack of NAMED fn bodies currently under analysis
	// (one entry per AnalyseFnBody whose fn has a name). Its top is the fn
	// whose body is executing; used to attribute body-local defs and
	// undefined_word diagnostics to their enclosing fn and to record the
	// caller→callee edge for each dispatch. Anonymous bodies (name "") are
	// transparent — not pushed — so attribution lands on the nearest named
	// ancestor. Pushed/popped in lockstep with FnBodyDepth.
	FnNameStack []string

	// FnBinders maps a NAME to the set of fn names that bind it (as a
	// parameter or a body-local def) somewhere in the pass. With FnCallGraph
	// it is the SOUND basis for the dynamic-scope undefined-word rescue: a
	// fn-body reference to a name only ever bound in a per-call frame is
	// rescued iff a binder of that name can actually REACH the reading fn
	// through the call graph (RescueForwardRefDiagnostics). This replaces the
	// reverted "bound anywhere in the pass" rescue, which masked a genuinely-
	// undefined name that merely shared a name with an unrelated fn's param.
	FnBinders map[string]map[string]bool

	// FnCallGraph maps a fn name to the set of fn names its body calls,
	// recorded at each nested AnalyseFnBody entry (including the self-edge of
	// a recursive fn). Its transitive closure answers "can a binder of X
	// reach the fn that reads X" for the dynamic-scope rescue.
	FnCallGraph map[string]map[string]bool
}

// DefaultCheckStepBudget caps total check-mode steps across all
// sub-engines. Chosen to comfortably fit typical programs
// (thousands of words) while preventing pathological runaways.
const DefaultCheckStepBudget = 500_000

// CheckSeverity classifies a diagnostic as an error, warning, or info.
// Errors indicate a real type/signature violation that prevents
// successful execution. Warnings flag suspicious patterns that are
// still type-correct. Info is everything else (missing annotation,
// budget overshoot, etc.).
type CheckSeverity string

const (
	SeverityError   CheckSeverity = "error"
	SeverityWarning CheckSeverity = "warning"
	SeverityInfo    CheckSeverity = "info"
)

// checkCodeSeverity maps a diagnostic code to its default severity.
// Unknown codes default to SeverityInfo so new codes don't
// accidentally trip CI gates until they're classified.
var checkCodeSeverity = map[string]CheckSeverity{
	"no_signature":          SeverityError,
	"undefined_word":        SeverityError,
	"incomparable":          SeverityError,
	"fn_body_error":         SeverityError,
	"branch_error":          SeverityError,
	"type_error":            SeverityError,
	"as_error":              SeverityError,
	"uncalled_function":     SeverityError,
	"unreachable_signature": SeverityWarning,
	"partial_dispatch":      SeverityWarning,
	"analysis_truncated":    SeverityInfo,
	// Every emit site (CheckListIndex / CheckAtIndices / the module
	// insert-at/remove-at mirrors) fires only on a PROVABLY out-of-range
	// index over a statically-known length, and every consuming word
	// (getr / at / set / ArrayUtil.*) errors at runtime on that index —
	// there is no lenient consumer, so the diagnostic is a guaranteed
	// runtime failure, not a suspicion. Promoted from SeverityWarning
	// (accessors are REQUIRED reads: a static miss must gate).
	"index_out_of_range":   SeverityError,
	"missing_returns":      SeverityWarning,
	"step_budget_exceeded": SeverityWarning,
	"body_error":           SeverityWarning,
	// The Micron naming rule: a type bound under Scalar/Micron whose
	// name does not end in the "on" suffix (micron.go).
	"micron_name": SeverityError,
	// A strict read (getr/dotr) whose miss is statically decidable — a
	// known Micron kind with a concrete unknown key (native_micron.go), a
	// concrete map / class schema / module export the key provably misses
	// (native_accessor.go getrNodeReturns / getrObjectReturns,
	// native_module_types.go), or a statically-None strict-read parent.
	"not_found": SeverityError,
	// A strict read whose CONTAINER is provably the wrong shape — a
	// concrete list read with a non-integer key ("getr: expected a map").
	"getr_error": SeverityError,
	// `unpack` against a CONCRETE source map that provably lacks a
	// requested key (native_unpack.go's check-mode mirror).
	"unpack_error": SeverityError,
	// A `raise` on the top-level straight line — outside every fn body,
	// branch, loop, and catching `do` — unconditionally errors the
	// program (native_error_raise.go raiseReturns).
	"unconditional_raise": SeverityError,
	// Concrete-operand arithmetic that provably faults at runtime, on the
	// same top-level straight line: an int64 overflow (integer_overflow,
	// the runtime's own code) or an arith_error raise — div/mod by a
	// static zero, pow's negative exponent (native_math.go
	// returnsIntArithChecked / returnsDivMod; all coded since the NUR010
	// fix, so the static code is the runtime's own).
	"integer_overflow": SeverityError,
	"arith_error":      SeverityError,
	// `convert` of a PROVEN-Float source into a Big target — the one
	// type-decidable convert refusal (native_type.go convertScalarReturns).
	"convert_error": SeverityError,
	// `set` of a field outside a class instance's CLOSED schema
	// (native_storage.go setClassInstanceReturns — the runtime's own code).
	"sealed_field": SeverityError,
	// A statically-known value outside the weak container domain
	// (native_storage.go weakValueMirror — the runtime's own code,
	// design/FLEX-ATTRS.1.md §4.4).
	"weak_value_error": SeverityError,
	// Dry-pass mirrors of PURE words over concrete literals
	// (eng/go/drypass.go DryPassReturns): each code is the runtime's own —
	// a failing Assert comparison, a malformed codec/parse literal, an
	// out-of-range module list edit, a reify shape violation, and the
	// outside-a-template macro words.
	"assertion_failure": SeverityError,
	"decode_error":      SeverityError,
	"parse_error":       SeverityError,
	"reify_error":       SeverityError,
	"unquote_error":     SeverityError,
	"splice_error":      SeverityError,
	// Parse.spec's check-mode dry pass (boru:parse): a concrete
	// whole-grammar map with a decidable shape error — unknown section,
	// mistyped token/action/matcher/abnf/rule entries.
	"parse_bad_spec":    SeverityError,
	"parse_bad_action":  SeverityError,
	"parse_bad_matcher": SeverityError,
	"parse_bad_abnf":    SeverityError,
	"parse_bad_rule":    SeverityError,
	// Generics (design/GENERICS.10.md §9.2).
	"constraint_violation": SeverityError,
	"unbound_param":        SeverityError,
	"arity_mismatch":       SeverityError,
	"static_warning":       SeverityWarning,
	// Housekeeping / structural (previously set inline at the emit sites —
	// the table is the single source of truth; TestCheckSeverityTableComplete
	// gates that every emitted code has an entry).
	"unused_def":            SeverityWarning,
	"unreachable_branch":    SeverityWarning,
	"record_shape_mismatch": SeverityError,
	"fold_error":            SeverityError,
	// A typed Patrun (`patrun T`) whose `add` stores a CONCRETE value the
	// checker can prove is not a T (native_patrun.go — the static mirror of
	// the runtime add guard).
	"patrun_error": SeverityError,
	// Advisory (non-gating): a readability nudge, not a defect.
	"forward_strands_operand": SeverityInfo,
	"mixed_form_call":         SeverityInfo,
	// A parked word whose plan filled a slot with a dispatching word
	// committed early at a statement boundary (the else-less-guard
	// shape) — correct, but the source reads ambiguously; an explicit
	// `[]` else or `end` makes the intent loud. See
	// design/FORWARD-COLLECTION-PHASES.10.md.
	"speculative_forward_commit": SeverityInfo,
	// Advisory in CHECK mode by design: a ref-family misuse (`usurp 5`) is
	// lenient under analysis (the value may be a gradual carrier) and raises
	// for real at runtime — the check-mode diagnostic is a hint, not a gate.
	"illegal_ref": SeverityInfo,
	// A8: a macro/DSL expansion the checker cannot run statically degrades
	// to a dynamic value; the advisory makes the precision loss visible.
	"macro_not_expandable": SeverityInfo,
	// Strict-mode advisory: a committed dispatch over a dynamic operand.
	"dynamic_dispatch": SeverityInfo,
	// Advisory (non-gating): an `x is T` guard whose binding's static type
	// already entails T — the check cannot fail, and the dead guard misleads
	// readers about reachable states (completion plan 2.3; the article's
	// "unnecessary defensive check" residue). Emitted by ApplyGuardNarrowing.
	"redundant_guard": SeverityInfo,
	// Case exhaustiveness (design/case-exhaustiveness.0.md). A default-less
	// `case` whose clause matches do not provably cover the scrutinee's
	// static type is an ERROR: the uncovered value silently produces
	// nothing at runtime. Coverage is proven in the sound direction only
	// (opaque predicate matches never count), a gradual dynamic(T)
	// scrutinee skips the check, and the trailing default stays optional
	// exactly when the type disjunction is fully met — so the finding
	// fires only where a genuinely uncoverable value exists. NOT a
	// RuntimeMirror: no-match is not a runtime error, so the compile
	// pipeline refuses on it like any other model-level error.
	"case_not_exhaustive": SeverityError,
	// The advisory duals of the same coverage computation (info,
	// non-gating, per the redundant_guard precedent): a trailing default
	// made unreachable because the clauses already cover every
	// alternative, and a clause subsumed by the clauses before it.
	"case_redundant_default":  SeverityInfo,
	"case_unreachable_clause": SeverityInfo,
	// RESERVED — no emit site yet (completion plan 4.4 / G6). The general
	// "options-looking map literal flows into a slot with no Options schema"
	// lint is BLOCKED ON PRECISION: atom-spelled and string-spelled map keys
	// are indistinguishable post-parse (`{a:1} cmp {'a':1}` → 0 — OrderedMap
	// keys are plain strings), so EVERY concrete map argument would qualify
	// and the rule cannot separate an options idiom from a data map (merge /
	// inject / make inputs) — far below the ~100% on-corpus advisory bar.
	// The per-family remedy shipped instead: option-consuming words declare
	// an Options schema on their opts slot (`convert`'s convertOptsPattern;
	// the emit family's EmitOptsSchema), which turns an unknown key into a
	// hard dispatch rejection at check AND run time. Classified here so a
	// future precise emitter inherits the intended severity.
	"options_key_unchecked": SeverityInfo,
	// `boru check` RUNS module bodies for real: `import`'s signatures are
	// registered RunInCheck (the checker cannot type `Mod.v` without the
	// module's actual exports) and check mode is deliberately NOT
	// propagated into the body (carrier-stripping would destroy the
	// concrete string literals used as export names and map keys). So a
	// module body's effects execute with full ambient authority during a
	// command documented as "type-check without running", and because file
	// modules are never cached, a default run — which pre-flight checks
	// first — executes each body TWICE per importer.
	//
	// Info, not a warning: nothing is wrong with the program, and the
	// behaviour is the composition of two deliberate decisions. What was
	// missing is that it was invisible. One entry per body execution, not
	// deduped — the count IS the finding.
	"module_body_executed_in_check": SeverityInfo,
}

// SeverityFor returns the default severity classification for a
// diagnostic code. Exported so consumers can tag custom codes.
func SeverityFor(code string) CheckSeverity {
	if s, ok := checkCodeSeverity[code]; ok {
		return s
	}
	return SeverityInfo
}

// CheckDiagnostic is a single static type-check finding.
type CheckDiagnostic struct {
	Code     string        `json:"code"`               // short stable code, e.g. "missing_returns", "no_signature"
	Detail   string        `json:"detail"`             // human-readable description
	Word     string        `json:"word,omitempty"`     // word name relevant to the diagnostic, if any
	Row      int           `json:"row,omitempty"`      // 1-based line number, 0 if unknown
	Col      int           `json:"col,omitempty"`      // 1-based column number, 0 if unknown
	Severity CheckSeverity `json:"severity,omitempty"` // default severity from checkCodeSeverity; empty = info
	FnBody   bool          `json:"fnBody,omitempty"`   // emitted during fn-body analysis (call-time code) — see RescueForwardRefDiagnostics
	FnName   string        `json:"fnName,omitempty"`   // enclosing named fn for an FnBody diagnostic — the reader for the dynamic-scope rescue

	// RuntimeMirror marks a diagnostic that mirrors a GUARANTEED runtime
	// error over exactly-known operands (design/CHECKER-COMPLETION.0.md):
	// the finding gates `boru check`, but the recording MODEL underneath it
	// is exact — the program compiles and raises the identical error at
	// runtime (a trap, the VM RET check, the same pure handler) — so the
	// compile pipeline does NOT refuse on it (CompileCheck / Vm.compile
	// skip mirrors in their error-diagnostic refusal). Contrast a
	// model-undermining diagnostic (undefined_word, no_signature), where
	// dispatch did not resolve and the recording is a guess.
	RuntimeMirror bool `json:"runtimeMirror,omitempty"`

	// CaughtAtRuntime marks a would-be-error diagnostic emitted inside an
	// error-TRAPPING region (`do [...]` — CaughtBodyDepth): the runtime
	// traps the error there, so the program is not wrong. AddDiagnostic
	// downgrades such findings to SeverityInfo centrally and stamps this
	// flag — the information (the expression always raises, and where the
	// trap is) survives without a false error verdict.
	CaughtAtRuntime bool `json:"caughtAtRuntime,omitempty"`

	// --- Structured diagnostic payload (design/DIAGNOSTICS.0.md). ---
	// The additive rich layer rendered by RenderCheckDiagnostic beneath
	// the stable one-line `check:` header; every field omitempty so the
	// JSON/LSP wire shape only grows.

	// Src is the offending token's source text — the caret width for
	// the rich source excerpt (falls back to Word when empty).
	Src string `json:"src,omitempty"`
	// Notes are freestanding explanatory lines (`= note: …`).
	Notes []string `json:"notes,omitempty"`
	// Suggestions are actionable fixes (`= help: …`).
	Suggestions []DiagSuggestion `json:"suggestions,omitempty"`
}

// NewCheckState builds the registry's initial analysis state: analysis
// off, the step budget at its "unset" sentinel (resolved to the project
// default at run time), and the inactive no-op recorder standing in for
// the emit surface (design/CHECKER-COMPLETION.0.md). Registry
// construction calls this so the check piece owns its own zero state.
func NewCheckState() *CheckState {
	return &CheckState{StepBudget: -1, Emit: TheInactiveEmit}
}

// PendingMethodApply threads ONE modelled shaped-method dispatch from
// tryShapedMethodDispatch into recordDispatchOutcome. Set immediately
// before the model's carrierResults call and consumed by
// tryRecordMethodApply; the model declines (no tape splice) if the
// pending was not consumed (an unexpected special-word short-circuit).
type PendingMethodApply struct {
	Origin Value  // the dynamic method-read carrier; its producing event supplies the runtime fn
	Word   string // the member word name (defensive re-key at the outcome seam)
}

// Clone returns a deep copy of the analysis state: scalar fields are
// copied, the maps (FnSummaries, FnInflight, FnAnalysisCounts,
// DefsInstalled, DefsUsed, ContextTypes) and the Diagnostics slice are
// cloned so a sandboxed run's in-place mutation cannot bleed into the
// snapshot (and a restore cannot bleed back). Emit is copied by pointer
// (the recorder is shared, not snapshotted). Used by the predicate /
// compile sandboxes, which since the Check-pointer conversion
// (design/module-fn-checkstate-ownership.1.md §3.2) must snapshot the
// POINTEE rather than alias it.
func (c *CheckState) Clone() *CheckState {
	if c == nil {
		return nil
	}
	cp := *c // scalars + Emit pointer + map/slice headers
	if c.Diagnostics != nil {
		cp.Diagnostics = append([]CheckDiagnostic(nil), c.Diagnostics...)
	}
	cp.FnSummaries = cloneMap(c.FnSummaries)
	cp.FnInflight = cloneMap(c.FnInflight)
	cp.FnNameInflight = cloneMap(c.FnNameInflight)
	cp.FnAnalysisCounts = cloneMap(c.FnAnalysisCounts)
	cp.DefsInstalled = cloneMap(c.DefsInstalled)
	cp.DefsUsed = cloneMap(c.DefsUsed)
	cp.ContextTypes = cloneMap(c.ContextTypes)
	cp.CtxShapes = cloneMap(c.CtxShapes)
	cp.MethodShapes = cloneMap(c.MethodShapes)
	cp.FnBinders = cloneNestedSet(c.FnBinders)
	cp.FnCallGraph = cloneNestedSet(c.FnCallGraph)
	if c.FnNameStack != nil {
		cp.FnNameStack = append([]string(nil), c.FnNameStack...)
	}
	return &cp
}

// IsActive reports whether check mode is currently on. Handlers consult
// this to short-circuit side effects during static analysis.
func (c *CheckState) IsActive() bool {
	return c != nil && c.Mode
}

// SkipsSideEffect reports whether a side-effecting operation should be
// suppressed by check mode. Equivalent to IsActive today — kept
// distinct so the policy can be refined per category later (file write
// vs network vs store mutation) without churning every call site.
func (c *CheckState) SkipsSideEffect() bool {
	return c.IsActive()
}

// ModelsEffects reports whether this registry runs with CONCRETE values but
// SUBSTITUTED effect backends — the third mode a module body executed during
// `boru check` runs in. See CheckState.ModelEffects for the contract and for
// the two classes that stay real.
func (c *CheckState) ModelsEffects() bool {
	return c != nil && c.ModelEffects
}

// PushSpecBaseline / PopSpecBaseline bracket one SPECULATIVE check region
// with its def-depth snapshot (see CheckState.SpecBaselines): pushed by the
// rolled-back nested-body run and the fn-body analysis, consulted by the
// `undef` handler via Registry.SpecUndefBlocked.
func (c *CheckState) PushSpecBaseline(snap map[string]int) {
	c.SpecBaselines = append(c.SpecBaselines, snap)
}

func (c *CheckState) PopSpecBaseline() {
	c.SpecBaselines = c.SpecBaselines[:len(c.SpecBaselines)-1]
}

// Begin enables check mode and resets the per-pass state (diagnostics,
// step count, budget flag, defs-installed/used, context-type tracking).
// Returns a function that switches mode off when called — typically via
// `defer`. Diagnostics gathered during the pass remain accessible on
// Diagnostics for the caller to inspect after the deferred function
// runs.
func (c *CheckState) Begin() func() {
	if c == nil {
		return func() {}
	}
	c.Mode = true
	c.Diagnostics = nil
	c.StepCount = 0
	c.BudgetTripped = false
	c.SuppressedRuntimeError = false
	c.AmbiguousGradualSplit = false
	c.DefsInstalled = nil
	c.DefsUsed = nil
	c.FnNameStack = nil
	c.FnBinders = nil
	c.FnCallGraph = nil
	c.ContextTypes = nil
	c.CtxShapes = nil
	c.MethodShapes = nil
	c.PendingMethodApply = nil
	c.InflightBails = 0
	c.FnNameInflight = nil
	c.SuppressBodyErrors = 0
	c.FnAnalysisCounts = nil
	c.Emit = TheInactiveEmit
	c.CodeEffectDepth = 0
	c.FnBodyDepth = 0
	c.CaughtBodyDepth = 0
	c.NestedBodyDepth = 0
	c.CondBodyDepth = 0
	c.LoopBodyDepth = 0
	c.SpecBaselines = nil
	c.ArgsFrameUnnamed = false
	// Compiling marks a REAL compile pass; the compile entry points set it
	// true AFTER this Begin (via BeginCompilePass). Reset it here so it is
	// a proper per-pass flag — a later plain check on a reused registry
	// must not inherit a prior compile's true.
	c.Compiling = false
	// Arm process-wide ID minting for the pass's lifetime: the emit
	// recorder keys provenance on Value.IDs minted at creation, so every
	// value created while ANY pass is live must carry one (see
	// checkPassDepth in value.go). The decrement is once-guarded — done
	// closures ride defer AND t.Cleanup in places, and a double decrement
	// would drive the counter negative, eliding IDs inside a later
	// legitimate pass.
	checkPassDepth.Add(1)
	ended := false
	return func() {
		c.Mode = false
		if !ended {
			ended = true
			checkPassDepth.Add(-1)
		}
	}
}

// AddDiagnostic appends a diagnostic to the active check run. Safe to
// call outside of check mode — it simply records the finding. If the
// diagnostic's Severity is empty, the default mapping from its Code is
// applied via SeverityFor.
func (c *CheckState) AddDiagnostic(d CheckDiagnostic) {
	if c == nil {
		return
	}
	if d.Severity == "" {
		d.Severity = SeverityFor(d.Code)
	}
	// A recursive fn-body re-entry (a self-call with a different arg shape that
	// re-runs the same body tokens) must not re-emit body errors — the outer,
	// non-recursive analysis of the same body already reports any real defect,
	// whereas the narrowed re-entry can spuriously fail dispatch. Drop only the
	// emergent error-level dispatch diagnostics; warnings/info still flow.
	if c.SuppressBodyErrors > 0 && d.Severity == SeverityError {
		switch d.Code {
		case "no_signature", "undefined_word", "uncalled_function", "branch_error",
			// fn_body_error: an assumed-dispatch analysis runs a body against
			// args the REAL match already rejected (checkModeAssumeSig's
			// post-trap continuation) — a body run that HALTS under those args
			// (a splice operand bound to a typed param) is the same cascade
			// noise as the dispatch codes above; the honest diagnostic is the
			// no_signature / trap at the call site.
			"fn_body_error":
			return
		}
	}
	// Inside an error-TRAPPING region (`do [...]`, CaughtBodyDepth) the
	// runtime catches every body error, so an error-severity finding there
	// is not a program error — re-attribute it centrally: downgrade to
	// info and stamp CaughtAtRuntime, keeping the finding visible without
	// a false verdict. This covers EVERY error family uniformly (the
	// guaranteed-error mirrors, undefined_word, no_signature, …) instead
	// of each emitter special-casing the region.
	if c.CaughtBodyDepth > 0 && d.Severity == SeverityError {
		d.Severity = SeverityInfo
		d.CaughtAtRuntime = true
	}
	if c.FnBodyDepth > 0 {
		d.FnBody = true
		// Attribute the finding to the innermost NAMED fn on the stack — the
		// reader for the dynamic-scope undefined-word rescue. Empty when the
		// only enclosing body is anonymous, which the rescue treats as "no
		// reader" (never dynamic-rescued).
		if n := len(c.FnNameStack); n > 0 {
			d.FnName = c.FnNameStack[n-1]
		}
	}
	c.Diagnostics = append(c.Diagnostics, d)
}

// TruncateDiagnostics drops every diagnostic recorded after position
// n. Used by bounded fixed-point analyses (AnalyseLoopBody, the fold
// accumulator iteration) so that only the FINAL round's diagnostics
// survive — earlier rounds run against not-yet-stable bindings.
func (c *CheckState) TruncateDiagnostics(n int) {
	if c == nil || n < 0 || n >= len(c.Diagnostics) {
		return
	}
	c.Diagnostics = c.Diagnostics[:n]
}

// IsolateBudget snapshots the check-mode step budget (StepCount +
// BudgetTripped) for the duration of a throwaway evaluation, returning a
// restore func. It is the budget-channel complement to IsolateEmit /
// TruncateDiagnostics in the dynamic-help hermetic eval: the synthetic
// example run shares the registry's CheckState, so steps it consumes count
// against the REAL program's shared budget (StepCount is per-registry, and
// the engine's check loop short-circuits every subsequent sub-engine once
// BudgetTripped is set). A documentation example that runs many steps — most
// acutely a RECURSIVE macro, which loops to the step ceiling — would then
// exhaust the program's budget before its own later statements are reached.
// Snapshotting and restoring the counters contains the synthetic eval fully,
// so it can never abort the program's compile. No-op outside check mode.
func (c *CheckState) IsolateBudget() func() {
	if c == nil {
		return func() {}
	}
	savedCount := c.StepCount
	savedTripped := c.BudgetTripped
	return func() {
		c.StepCount = savedCount
		c.BudgetTripped = savedTripped
	}
}

// RecordDef remembers a name the user bound during a check run so
// end-of-run analysis can flag defs that were never referenced. Names
// starting with "_" (engine internals) are ignored.
func (c *CheckState) RecordDef(name string, pos SrcPos) {
	if !c.IsActive() || name == "" || strings.HasPrefix(name, "_") {
		return
	}
	if c.DefsInstalled == nil {
		c.DefsInstalled = map[string]SrcPos{}
	}
	c.DefsInstalled[name] = pos
	// "Ever-used during the run" semantics: a use recorded against this name
	// at ANY point counts. The tracker is flat (name-keyed, no scope id), so
	// resetting the use on every rebind produced false unused_def warnings
	// wherever a name is legitimately read and then re-bound — loop-carried
	// flags re-bound each iteration (decision's found/best-pri/done) and a
	// name reused as an independent local across sibling quotations where only
	// one reads it (sort's var-destructure counters). The one use the reset
	// was really protecting against — a fn's construction-time self-reference,
	// which must NOT count as a use — is now suppressed precisely at its
	// source (checkFnBodyAtConstruction snapshots/restores this name's use
	// flag), so dropping the blanket reset keeps uncalled-fn detection
	// (TestCheckUnusedDefFn) while clearing the read-then-rebind FPs. The only
	// residual is a benign FN: a genuinely dead VALUE rebind
	// (`def x 1  x print  def x 2`) is no longer flagged — acceptable, and in
	// line with the tracker's already-documented forward-ref FN.
}

// RecordUse is the exported wrapper over recordUse for callers outside the
// eng package — notably module export resolution (lang/native), which records
// each reference-exported public word as a use so unused_def does not falsely
// flag the entire public API.
func (c *CheckState) RecordUse(name string) { c.recordUse(name) }

// recordUse marks a name as referenced during check mode. Safe to call
// unconditionally; outside check mode it is a no-op. Used by
// Registry.Lookup and stepWord's simple-value path.
func (c *CheckState) recordUse(name string) {
	if !c.IsActive() || name == "" {
		return
	}
	if c.DefsUsed == nil {
		c.DefsUsed = map[string]bool{}
	}
	c.DefsUsed[name] = true
}

// EmitUnusedDefDiagnostics walks the set of defs installed during a
// check run and emits an unused_def warning for any name that was
// never referenced. Call this at the end of a check pass, before
// returning the CheckResult.
func (c *CheckState) EmitUnusedDefDiagnostics() {
	if c == nil {
		return
	}
	for name, pos := range c.DefsInstalled {
		if c.DefsUsed[name] {
			continue
		}
		c.AddDiagnostic(CheckDiagnostic{
			Code:   "unused_def",
			Detail: "def " + name + " is never used",
			Word:   name,
			Row:    pos.Row,
			Col:    pos.Col,
		})
	}
}

// recordCallEdge notes that fn `caller` dispatches fn `callee` (both named).
// The self-edge of a recursive fn is recorded too — it is what makes a
// body-local binding visible to a same-fn read on a sibling branch across a
// recursive frame. No-op for the top-level caller (empty name).
func (c *CheckState) RecordCallEdge(caller, callee string) {
	if caller == "" || callee == "" {
		return
	}
	if c.FnCallGraph == nil {
		c.FnCallGraph = map[string]map[string]bool{}
	}
	m := c.FnCallGraph[caller]
	if m == nil {
		m = map[string]bool{}
		c.FnCallGraph[caller] = m
	}
	m[callee] = true
}

// RecordFnBinder attributes a binding of `name` to the innermost named fn
// currently under analysis (top of FnNameStack) — a body-local def, or (via
// the AnalyseFnBody param loop) a parameter. No-op outside check mode, at the
// top level (empty stack), or for engine-internal ($-/_-prefixed) names.
func (c *CheckState) RecordFnBinder(name string) {
	if !c.IsActive() || name == "" || len(c.FnNameStack) == 0 {
		return
	}
	if name[0] == '_' || name[0] == '$' {
		return
	}
	fn := c.FnNameStack[len(c.FnNameStack)-1]
	if fn == "" {
		return
	}
	if c.FnBinders == nil {
		c.FnBinders = map[string]map[string]bool{}
	}
	m := c.FnBinders[name]
	if m == nil {
		m = map[string]bool{}
		c.FnBinders[name] = m
	}
	m[fn] = true
}

// dynamicScopeReachable reports whether some fn that binds `name` can reach
// `reader` (the fn whose body referenced it) through the recorded call graph
// — i.e. a runtime call stack exists where `reader` executes while a binder
// frame of `name` is live. Parameters/captures are frame-lifetime, so the
// answer is sound for them; a body-local binder is sound for the recursion
// idiom (the def precedes the reaching call) with a documented narrow residual
// when a local is bound only AFTER the call that reaches the reader.
func (c *CheckState) DynamicScopeReachable(name, reader string) bool {
	if reader == "" {
		return false
	}
	binders := c.FnBinders[name]
	if len(binders) == 0 {
		return false
	}
	for b := range binders {
		if c.callReaches(b, reader) {
			return true
		}
	}
	return false
}

// callReaches reports whether fn `from` transitively calls `to` in the
// recorded call graph. A fn reaches itself ONLY via an actual recursion edge
// (from→…→from), never trivially — so a non-recursive fn's own body-local
// binding is not treated as visible to a same-fn read on a sibling branch.
func (c *CheckState) callReaches(from, to string) bool {
	if len(c.FnCallGraph) == 0 {
		return false
	}
	seen := map[string]bool{}
	var walk func(n string) bool
	walk = func(n string) bool {
		for callee := range c.FnCallGraph[n] {
			if callee == to {
				return true
			}
			if !seen[callee] {
				seen[callee] = true
				if walk(callee) {
					return true
				}
			}
		}
		return false
	}
	return walk(from)
}

// RecordContextSet records (key → carrier) for the given store-set
// call. Called from `set`'s ReturnsFn. Repeated writes to the same key
// join their carrier types via JoinCarriers so the recorded type
// reflects every write. Safe to call outside check mode — it becomes a
// no-op.
func (c *CheckState) RecordContextSet(key string, carrier Value) {
	if !c.IsActive() || key == "" {
		return
	}
	if c.ContextTypes == nil {
		c.ContextTypes = map[string]Value{}
	}
	if existing, ok := c.ContextTypes[key]; ok {
		c.ContextTypes[key] = JoinCarriers(existing, carrier)
		return
	}
	c.ContextTypes[key] = carrier
}

// LookupContextType returns the carrier recorded for the given key via
// a prior set, or (Any-carrier, false) when the key has not been
// observed in this check run.
func (c *CheckState) LookupContextType(key string) (Value, bool) {
	if c == nil {
		return NewCarrier(TAny), false
	}
	if v, ok := c.ContextTypes[key]; ok {
		return v, true
	}
	return NewCarrier(TAny), false
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
//   - a member with a GENUINE 0-arg overload — the miscompile-E
//     auto-dispatch family (Span.finish, Rand.bool) — is now ANNOTATED
//     rather than excluded: its landing is modelled as an arity-0
//     OpCallDynMethod (shapedMethodApplyWindow's all-0-arg path), the
//     read-guard refusal is skipped for the annotated read
//     (EmitState.noteShapedRead), and a landing the model cannot claim
//     REFUSES outright (tryShapedMethodDispatch's guard-owned decline)
//     so the guard is re-homed, never weakened;
//   - macros stay data (applied only by name).
func (c *CheckState) NoteMethodShape(out, member Value) {
	if !c.IsActive() || out.ID == "" {
		return
	}
	fd, ok := member.Data.(FnDefInfo)
	if !ok || fd.Registry == nil || fd.Name == "" || fd.Macro {
		return
	}
	if !IsDelegationFnDef(fd) {
		return
	}
	if c.MethodShapes == nil {
		c.MethodShapes = map[string]Value{}
	}
	c.MethodShapes[out.ID] = member
	// Mirror the annotation into the recorder so the get-family read
	// guards (recordCallRefusal / RecordPolyCall) can skip their
	// auto-dispatch refusal for a read the landing model owns.
	c.Emit.noteShapedRead(out.ID)
}

// methodShapeMember returns the annotated member for a carrier ID.
func (c *CheckState) MethodShapeMember(id string) (Value, bool) {
	if c == nil || id == "" || c.MethodShapes == nil {
		return Value{}, false
	}
	m, ok := c.MethodShapes[id]
	return m, ok
}

// ContextShape returns the abstract shape carrier for a LIVE context
// layer, minting it on first sight. Keyed by the layer's identity (the
// *StoreInstanceInfo pointer): in check mode the runtime COW replace
// never runs (set's Impl is intercepted), so the top-of-stack pointer
// is stable for the lifetime of its scope — every `context` call in one
// scope shares one shape, and each engine Run's pushed layer (a `do`
// body, a module load) gets its own. Holding the pointer as a map key
// also keeps the layer reachable, so a recycled allocation can never
// alias two scopes' shapes. The returned Value is a fresh-ID copy
// sharing the shape pointer.
func (c *CheckState) ContextShape(store *StoreInstanceInfo, scope int) Value {
	if c == nil || store == nil {
		return NewCarrier(TStore)
	}
	if c.CtxShapes == nil {
		c.CtxShapes = map[*StoreInstanceInfo]Value{}
	}
	v, ok := c.CtxShapes[store]
	if !ok {
		v = NewStoreShapeCarrier(TStore, scope)
		c.CtxShapes[store] = v
	}
	v.ID = GenerateID(IDPrefixForType(v.Parent))
	return v
}

// BeginCompilePass is Begin() plus the compile-pass arming ritual shared
// by every bytecode-recording entry point (lang's CompileCheck, boru:vm's
// Vm.compile): install a fresh EmitState, mark the pass as Compiling, and
// drop the fn-body memos so bodies re-analyse — and re-record — under
// THIS pass (a summary cached by an earlier plain check would leave its
// compiled unit empty). One shared helper is what keeps the ritual's
// pieces from going missing in a hand-rolled copy: Vm.compile shipped
// without the Compiling flag for exactly that reason.
func (c *CheckState) BeginCompilePass() func() {
	done := c.Begin()
	if c == nil {
		return done
	}
	c.Emit = NewEmitStateHook()
	c.Compiling = true
	c.FnSummaries = nil
	c.FnInflight = nil
	return done
}

// IsolateEmit swaps in a FRESH EmitState (sharing the registry) for the
// duration of a throwaway evaluation, returning a restore func. It is the
// hermetic complement to Suspend: Suspend keeps the SAME EmitState (only
// stopping recording), so a nested eval's interned consts and RememberOriginal
// entries still pollute the live EmitState's pool. The dynamic-help example
// eval fires from OnRegisterHook on EVERY fn registration — including the
// program's own `def f fn […]` DURING compilation — so without a swap its
// example run leaks compile-time state (e.g. a generated `['a' 'b']` sample
// list) into the program's own EmitState, corrupting a later operand's compile.
// Swapping to a throwaway pool, discarded on restore, contains it fully.
func (c *CheckState) IsolateEmit() func() {
	if c == nil {
		return func() {}
	}
	saved := c.Emit
	c.Emit = NewIsolatedEmitHook(c.Recorder())
	return func() { c.Emit = saved }
}

// SpecUndefBlocked reports whether an `undef` of name inside the CURRENT
// speculative check region would pop a binding that PREDATES the region —
// the deletion the model must not commit: the region may never execute at
// run time, so leaking the pop flagged `undefined_word` on clean programs
// (the wrapped-undef FP class — an `undef` in a skipped error handler, an
// each body over an empty list, an uncalled fn body). False outside check
// mode, outside any speculative region, and for bindings pushed INSIDE the
// region (frame params, body defs — their depth exceeds the entry
// snapshot), so teardown and body-local undef are untouched.
func (r *Registry) SpecUndefBlocked(name string) bool {
	if !r.Check.IsActive() || len(r.Check.SpecBaselines) == 0 {
		return false
	}
	base := r.Check.SpecBaselines[len(r.Check.SpecBaselines)-1]
	return r.Defs.Depth(name) <= base[name]
}

// RescueForwardRefDiagnostics drops undefined_word diagnostics that
// were emitted INSIDE a fn-body analysis (FnBody tag) for names that
// have a binding by the end of the pass. A fn body runs at CALL
// time, when the whole program's defs exist — so a body reference to
// a later definition is the documented forward-reference idiom
// (recursion via forward ref, mutual recursion: lang/spec/
// recursion.tsv §3), not a defect; the install-time body analysis
// just runs too early to see it. Names still unbound at end of pass
// keep their diagnostic (a genuine typo). Top-level (non-FnBody)
// uses before a def keep theirs too — those genuinely error at run
// time.
//
// Known limitation: a top-level CALL placed before the dependent
// def (`def f fn […g…] f 1 def g …`) errors at run time but is
// rescued here — the checker doesn't order call sites against defs.
//
// Call at end of a check pass, before reading Diagnostics.
func (r *Registry) RescueForwardRefDiagnostics() {
	if r == nil || r.Check.Diagnostics == nil {
		return
	}
	kept := r.Check.Diagnostics[:0]
	for _, d := range r.Check.Diagnostics {
		if d.Code == "undefined_word" && d.FnBody && d.Word != "" {
			// Module-scope forward reference: the name has a binding by end of
			// pass (recursion, mutual recursion, a later top-level def).
			if _, bound := r.Defs.Top(d.Word); bound || r.Lookup(d.Word) != nil {
				continue
			}
			// Dynamic-scope reference: the name lives only in a per-call frame
			// (a fn parameter or a body-local def), popped before end of pass,
			// but boru's dynamic scoping makes it visible to a fn REACHED from
			// the binder's frame. Rescue iff some fn that binds the name can
			// actually reach the reading fn through the call graph — the SOUND
			// condition. A name merely bound by an unrelated fn that never
			// calls the reader (`def f fn [[] [x]] def g fn [[x:Integer] [1]] f`)
			// stays flagged: it genuinely errors at run time.
			if r.Check.DynamicScopeReachable(d.Word, d.FnName) {
				continue
			}
		}
		kept = append(kept, d)
	}
	r.Check.Diagnostics = kept
}
