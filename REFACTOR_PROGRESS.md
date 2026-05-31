# PROGRESS — Function-model consolidation refactor

Branch: `claude/bloom-filter-dx-issues-0Gu1V`
Plan (APPROVED): `/root/.claude/plans/for-if-remove-the-moonlit-gadget.md`

## STAGE 0 — equivalence harness — COMPLETE (committed + pushed)

All gates green: eng/go and lang/go both `go vet ./...`=0 and
`go test ./...`=0.

Two golden harnesses (regenerate with the noted flags ONLY after an
intentional change; otherwise they must stay byte-identical):
- `lang/go/native/fnmodel_equivalence_test.go` (+ testdata golden).
  TestFnModelEquivalence. Flag `-update`. Dumps (1) every word's sorted
  compiled `Signatures` (arity/type-paths/barrier/noeval/quote/typearg/
  fullstack/fallback/runcheck/returns/+returnsfn) via `r.Defs.Names()` +
  `r.Lookup(name).Signatures`, and (2) a behavior corpus (native fwd/
  stack/swap, `if` code-bodies, named fns, recursion, multi-overload,
  afn, closures, each/fold, stack words). 0 errors in corpus.
- `lang/go/modules/fnmodel_wrapper_equivalence_test.go` (+ testdata
  golden). TestFnModelWrapperEquivalence. Flag `-update-wrapper`. Pins
  the MODULE-WRAPPER dispatch path (execFnDefLiteral captured-sub-
  registry / CallAQL branch) via mathRegistry + `arg ( math get NAME )`.
  abs=5, sign=-1, negate=-4, ceil=3, floor=2.

## KEY ARCHITECTURAL FINDING (sharpens the plan)
The named AQL fn path ALREADY compiles to a Go handler: `InstallFnDef`
(core_helpers.go:228-458) lowers `def f fn […]` into `RegisterNativeFunc`
with a body-splicing handler closure + a check-mode `returnsFn`. So named
fns already dispatch via `execMatch`. The REMAINING duality is concentrated
in **`execFnDefLiteral`** (engine.go:1800-1989) — Function-VALUE-on-stack
dispatch (afn/`=>`, captured FnDefs, module wrappers) — which builds
handler-less `Signatures` via `fnSigsToSignatures` (engine.go:2204) and
falls back to `execFnDefSigStackMatch` (2054) / `execFnDefSig` (2243) /
`CallAQL` (registry.go:715). THIS is the real target of the Stage-1 spike.

## NEXT: STAGE 1 — compileFnDef + spike
Add `compileFnDef`: attach a Handler to EVERY sig (native passthrough;
AQL closure over the body-runner), resolve BarrierPos, sort. Route
InstallFnDef / upsertFnDef / execFnDefLiteral's afn-lazy path
(fnSigsToSignatures at engine.go:1827-1835) through it. SPIKE: prove
CallAQL-style execution is complete for the execFnDefLiteral cases
(forward collection, captures, returns, def-cleanup, recursion). Fallback:
Handler-sentinel that splices inline — one localized branch in execMatch.
Convert trivial-delegation (engine.go:1958-1976) to a compile-time handler.
After Stage 1, every Signature.Handler is non-nil.

## REMAINING STAGES (from approved plan)
- Stage 2: delete Handler==nil fallbacks + module-closure split
  (engine.go:1882-1883,1923-1924,1947-1977) → dispatch = match → execMatch.
- Stage 3: merge structs. matchSignature/CompareSignatures/sigSlotValue read
  `FnSig.Params[i].Type`; fold Signature-only fields onto FnSig; retire
  `Signature`; delete `FnDefInfo.Signatures` + `fnSigsToSignatures`. Repoint
  readers: canon.go:140, fnsig.go:129-150, registry.go:942-998,
  core_helpers.go:41/232/472-479/840, engine.go:1827/2057, unify.go:120-123,
  modules/type.go:244-248, native_inspect.go:91/206, native_behave.go:132-135,
  native_definition.go:516-518, native_definition_fn.go:23-24.
- Stage 4: NativeFunc/NativeSig become thin shim lowering into FnSig via
  compileFnDef. 348 NativeSig literals UNCHANGED (out of scope).
  RegisterNativeFunc (registry.go:676) already does field-by-field
  NativeSig→Signature copy.
- Stage 5: remove dead bridges (execFnDefSig, execFnDefSigStackMatch,
  fnDefSigByParamCount, lazy compile). Update eng/go/CLAUDE.md (Signature
  Ordering, trivial-delegation, "two representations"). Add
  lang/doc/design/FUNCTION-MODEL.0.md. Fix USURP.0.md FnDefInfo note.

## INVARIANTS (do not regress)
- Signature ordering: top-first sig order; matchSignature sole source of truth.
- One BarrierPos sentinel: FnSig uses -1 (unset→resolve once at install);
  Signature uses post-resolved (0=no barrier). Pick ONE, resolve at the
  single install boundary AFTER handler-attach (barrier tiebreak in
  CompareSignatures reads resolved BarrierPos).
- Sealed payload: FnDefInfo keeps marker; Handler/Body inside FnSig (never a
  Value) need no marker.
- Check-mode return inference is the one irreducible asymmetry: analyze AQL
  Body (AnalyseFnBody) vs native Returns/ReturnsFn — mode-local read.
- Gate EACH stage: eng/go + lang/go `go vet ./...` + `go test ./...`, plus
  both equivalence harnesses, plus fixedid_stability/data_nil_gate/value_mode.

## HARNESS/REGISTRY FACTS (confirmed)
- Multi-module repo, NO go.work. `cd <mod> && go test ./...`. Modules:
  eng/go, lang/go, cmd/go, calc/go, wpg, test/go, test/solardemo.
  Pre-commit from root: `make fmt && make vet && make lint && make test`.
- Run a program in a test: `r,_ := native.DefaultRegistry(); toks,_ :=
  parser.Parse(src); out,_ := native.NewTop(r).Run(toks)` (native pkg)
  or `native.New(r).Run(toks)` (modules pkg — eng/go/engine is NOT a
  dependency there). Parser import: `github.com/aql-lang/aql/eng/go/parser`.
- Enumerate words: `r.Defs.Names()`. Lookup: `r.Lookup(name) *FnDefInfo`.
- Module wrappers need `modules.InstallResolver(r)` OR build via
  BuildMathModule(r) + push exports as defs (see modules mathRegistry).
- `Signature.TotalArgs()` = len(Args). `Type.Path()` = stable type path.

## CRITICAL ENV GOTCHA
The tool RESULT/display channel intermittently corrupts file-content reads
and stdout (padding with repeated garbage, truncating multi-line output,
even garbling a Write confirmation). The ON-DISK files are correct —
proven by go compiler/vet, go test exit codes, grep -q, git status.
RULES:
1. NEVER trust displayed file contents/stdout for correctness.
2. Verify via EXIT CODES: `cmd >/dev/null 2>&1; echo "X=$?"`, `grep -q`.
3. ONE tool call per message — parallel batches cascade-cancel on the
   first error and waste the whole batch (this bit me repeatedly).
4. If a Write/Edit looks corrupted on readback, rewrite via Bash heredoc
   (`cat > file <<'EOF'`); confirm with `gofmt -l` (empty=ok) + `go vet`.
5. NEVER `git add -A` right after a cancelled batch — a scratch probe
   (zzprobe_test.go) got committed that way and broke the build. Check
   `git status --porcelain` first; remove zz* probes before adding.
6. cwd persists across Bash calls; `cd /home/user/aql && …` for root ops.

---
## STAGE 1 — IN PROGRESS

### 1a DONE (committed, pushed, all gates green)
Extracted `compileFnDef(fnDef) *FnDefInfo` in eng/go/engine.go (after
fnSigsToSignatures, ~line 2240). Wraps fnSigsToSignatures+calcMaxForwardArgs;
routed the inline construction at execFnDefLiteral (was engine.go:1827-1835)
through it. Byte-identical shell (Name/Signatures/MaxForwardArgs/Registry only).
Verified: eng+lang vet+test=0, both equivalence goldens byte-identical.

### 1b SPIKE — the crux (NOT yet started; highest-risk change in the plan)
GOAL: attach a Handler to AQL-bodied compiled Signatures so the
`sig.Handler==nil` fallbacks in execFnDefLiteral (engine.go:1882-1883,
1923-1924) and execFnDefSigStackMatch (2054) become dead, unifying on
execMatch.

KEY INSIGHT (makes it tractable): InstallFnDef's handler (core_helpers.go:
246-339) does NOT return final results — it returns the body as a
PAREN-WRAPPED TOKEN SEQUENCE (`( argN..arg0 body __pa undef.. returncheck )`)
which execMatch's spliceMatchResults splices back onto the stack to be
RE-STEPPED inline. So a result-returning Handler CAN reproduce execFnDefSig's
inline-splice semantics exactly — no sub-engine, no behavior change — by
returning the same token sequence. This means anonymous-fn (afn/=>) dispatch
can move from execFnDefSigStackMatch→execFnDefSig onto the SAME handler shape
InstallFnDef already uses for named fns.

THE SUBTLETIES (must each be handled / tested):
1. Registry: InstallFnDef's handler CLOSES OVER `r` (install-time registry)
   and ignores the passed-in registry arg. A handler attached to a Function
   VALUE dispatched via execFnDefLiteral must instead use the registry the
   engine passes at call time (e.registry / the captured fnDef.Registry),
   because the same Function value can be invoked in different registries.
   => the extracted builder must take the registry from the handler arg, not
   a closure, OR compileFnDef must bind the right registry per dispatch.
2. Check mode: anonymous fns currently route to spliceAnonCheckResult
   (engine.go:2062,2110) which runs AnalyseFnBody. InstallFnDef instead
   supplies a ReturnsFn (core_helpers.go:361-435) consumed by carrierResults
   in execMatch's check-mode intercept. The attached handler path must carry
   an equivalent ReturnsFn so check-mode return inference is preserved for
   afn values (the one irreducible asymmetry).
3. Module closures (capturedReg != nil, execFnDefSig 2293-2334) run via
   CallAQL in the sub-registry and return FINAL results (not tokens). The
   trivial-delegation short-circuit (1958-1976) already uses execMatch.
   Non-trivial module-preamble AQL fns (decision.cond) need the sub-registry
   CallAQL path — their attached handler must close over fnDef.Registry and
   return CallAQL's final results, NOT inline tokens.
4. DefCleanup: InstallFnDef's handler emits a NewDefCleanup marker
   (core_helpers.go:315-318); execFnDefSig does NOT (it relies on __pa +
   undef tail only). Reconcile — the cleanup-marker difference is exactly
   the kind of divergence that causes def-leakage regressions (DX issue 2).

RECOMMENDED 1b SEQUENCING (each its own gated commit):
  1b-i  Extract InstallFnDef's handler closure into a named builder
        `buildFnBodyHandler(r, name, s, fnDef) Handler` + the returnsFn into
        `buildFnBodyReturnsFn(...)`; InstallFnDef calls them. PURE REFACTOR,
        verify via exit codes + goldens.
  1b-ii Make compileFnDef attach buildFnBodyHandler + buildFnBodyReturnsFn to
        each Signature (registry sourced from the handler arg). Anonymous-fn
        Function values now have non-nil Handler.
  1b-iii In execFnDefLiteral, let the `sig.Handler != nil` path own anonymous
        fns (drop the `&& !fnDef.Anonymous` guard at 1882; drop the 1923
        Handler==nil branch). Keep check-mode routing via the ReturnsFn.
        execFnDefSigStackMatch becomes reachable only as dead fallback.
  Gate each against eng+lang test + BOTH equivalence goldens. The goldens are
  the proof: if afn/closure/if-codebody behavior shifts, they fail.

RISK: this is the engine's core dispatch. If 1b-ii/iii destabilizes, revert
to the 1a baseline (commit is clean) and reconsider the Handler-sentinel
fallback (plan's stated alternative: a sentinel the executor recognizes to
splice inline — one localized branch in execMatch, still one dispatch entry).

---
## STAGE 1b — SPIKE SUCCEEDED (committed + pushed)

Done, all gates green (eng+lang vet+test, both equivalence goldens
byte-identical, all lambda/closure/anon/check tests pass):
- 1b-i: extracted buildFnBodyHandler + buildFnBodyReturnsFn from
  InstallFnDef (pure refactors). Body-runner now reusable.
- harness+: added CHECK MODE section to native golden (pins afn/closure
  return inference — all infer Integer).
- 1b-ii: compileFnDef attaches the body-runner handler+returnsFn to the
  compiled Signatures of ANONYMOUS fns. afn/closure Function values now
  dispatch via execMatch (line 1921 sig.Handler==nil now false for them),
  NOT execFnDefSigStackMatch. Goldens byte-identical => behavior-equivalent.

CRITICAL SCOPING LESSON: handler attachment MUST be gated on
fnDef.Anonymous. A first attempt attached handlers to ALL compiled sigs
and broke TestTypeFnPredicate_NotIndependentlyCallable: a non-anonymous
predicate-type FnDef sitting bare on the stack (`def Bbd …; Bbd "c"`)
relies on Handler==nil to stay INERT DATA (fall through to
execFnDefSigStackMatch which leaves it on the stack). Giving it a handler
made it auto-dispatch the predicate. So: anonymous => handler (active
dispatch); non-anonymous bare FnDef => no handler (inert). The
execFnDefLiteral comment at ~1881 ("predicate-type FnDefs landing bare
are intentionally inert") is the spec for this.

### REMAINING in Stage 1 / boundary into Stage 2
The `sig.Handler==nil` fallbacks at engine.go:~1876 and ~1921 are now
only reached by NON-anonymous bare FnDefs (predicates) + module wrappers
(which have their own branch at ~1947). execFnDefSigStackMatch is still
needed for those. So we CANNOT yet delete the fallbacks (that was the
naive Stage-2 plan). Reassess: the duality that remains is
  (a) predicate FnDefs — intentionally inert, must stay handler-less;
  (b) module-preamble AQL fns (decision.cond) — run via CallAQL in a
      sub-registry (execFnDefSig capturedReg path), returns final results.
Neither is an afn. The Stage-3 STRUCT MERGE (Sigs+Signatures→one,
Params[i].Type in matchSignature) is independent of these and is likely
the higher-value next move. Consider whether fully deleting
execFnDefSigStackMatch is worth it vs. leaving it for the predicate-inert
+ module-CallAQL cases.

NEXT DECISION POINT (ask user): proceed to Stage 3 struct merge, OR keep
pushing Stage 2 to also route module-preamble fns through handlers
(harder — needs the capturedReg CallAQL path as a handler that returns
final results, not inline tokens).

---
## STAGE 3 — struct merge (STARTED; lint baseline + dead-code done)

DONE:
- make lint baseline: eng+lang both golangci-lint clean (0 issues) after
  removing the now-unused fnSigsToSignatures (compileFnDef inlined it).
- Confirmed BarrierPos sentinel already consistent (BarrierAllForward
  named constant used everywhere; registry.go:327 resolves once).

RECON for the merge (validated):
- Signature{} CONSTRUCTION sites = only 3: registry.go:683 (the native
  shim — all 348 NativeSig literals funnel here), engine.go:2250
  (compileFnDef), core_helpers.go:69 (clearSigs/undef rebuild). Tiny.
- Signature.Args READERS = ~31 (mostly signature.go matchSignature region
  + engine.go forward-planner: lines signature.go 183/236/334/336/397/433,
  engine.go 192/337-338/1535/3292/3375/3411/3624/3636/3675/3785). The cost
  is here, not construction.
- The shape mismatch is the crux: Signature.Args is []*Type; FnSig.Params
  is []FnParam (name+type+pattern+optional). Merging means readers go from
  sig.Args[i] (a *Type) to sig.Params[i].Type.

MERGE PLAN (smallest compiling increments, gate each):
  3a. Add `Params []FnParam` to Signature (additive, compiles green).
      Populate it at all 3 construction sites alongside Args (Params[i]
      = FnParam{Type: Args[i], Pattern: from Patterns map, ...}).
  3b. Introduce sigArgType(sig,i) *Type helper returning sig.Args[i];
      repoint the ~31 readers to it. Gate.
  3c. Flip sigArgType to read sig.Params[i].Type; gate. Now Args is
      write-only.
  3d. Delete Signature.Args + the Patterns map (fold into Params);
      delete the field from the 3 constructors. Gate.
  3e. Rename the merged struct / alias FnSig=Signature OR fold remaining
      Signature-only fields (Handler/FullStack/Fallback/RunInCheckMode/
      ReturnsFn/CheckFullStackFn/QuoteArgs/TypeArgs) onto FnSig; make
      FnDefInfo carry ONE slice. This is the big one — do last, isolated.

RISK/RECOMMENDATION: 3a-3d are mechanical and individually gateable.
3e (collapse to one struct + delete FnDefInfo.Signatures, repoint all
.Sigs/.Signatures readers) is the largest single step — consider a git
worktree so a half-done state never touches the main tree. The anonymous-
fn dispatch unification (Stage 1b) is already the headline win and is
committed; Stage 3 is the structural cleanup on top.
