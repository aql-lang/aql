# Runtime-independence completion plan (.0)

Status: **proposal** (2026-07-13, branch `claude/bytecode-compiler-review-skbujj`).
A full status review of the bytecode compiler against the live tree (main
`400b23b`) plus the phased program to reach **full runtime independence**: all
code executes on the VM, on every default path, with the residual interpreter
entries enumerated and ratcheted. Supersedes the row inventories in the
P7-ENDGAME tier table and the `compiled_coverage_test.go` tier-ledger comment,
both of which describe a previous corpus generation (see §Status).

Scope decisions locked with the maintainer:

1. **All three gap rings are in scope** — the corpus refusals, the off-corpus
   library frontier, and the entry points / callback seams that never attempt
   compilation today.
2. **The 9 corpus refusals close via sound runtime re-dispatch** — never
   static best-guess baking (three recovery-site compile attempts were
   differential-proven unsound and reverted; VOXGIG-COMPILE-LEAVES.1.md
   §580-628 stands).
3. **Off-corpus verification is in-repo ported repros now**; the external
   voxgig `--force-compile` sweep re-runs as a follow-on when the client repos
   are reachable (M5 remains blocked-external).

## Status (verified 2026-07-12)

**On-corpus** (`lang/spec`, 6267 rows — COMPILED_STATUS.md): 5934 native,
**0 islands**, 0 tier-2, 0 compute-frontier, **9 refusals** — all "dispatch
recovery (best guess)", root cause soundness, pinned row-exact in
`test/go/langspec/compiled_refusals_test.go::knownRefusals` (2× `apply`
auto-fire, 4× generics wrong-instantiation, 1× variadic-`if`→`each`, 1× local
`add` overload, 1× word-splice). All are ERROR rows: the interpreter raises
unmatched dispatch at runtime; a compiled best guess could diverge. They latch
at the three `MarkUncompilable("unmatched dispatch recovered …")` sites in
`eng/go/engine.go`. Compiled mode is on by default (`CompileTry`).
Bookkeeping debt: `refusalGate = 11` vs actual 9, `computeRefusalCeiling = 17`
vs actual 0, and the tier-ledger comments describe the pre-I-wave row set.

**Off-corpus** (the real frontier): open leaves **L-DO** (fallible multi-value
`do`/`error` under a catch — variable arity; blocks 4 trie suites), **L-EACH**
(`refuseForwardStackDrift`), **L-JOIN** (recursive branch-join operand
provenance — a genuine bug, the only library-code blocker), **L-NP** (VM
runtime bail: `OpLookupDynScope` read miss, masked today by L-JOIN), and
**L-DUP** — the runtime-bail fallback in `RunCompiledReason` re-runs the whole
source after output was already emitted, duplicating every side effect: a hard
`compile==interpret` violation, latent because the differential corpus is
pure-value. Plus: Stage-D 3/4 & 4/4, sort-comparator module-fn `Function`
param slots, radix-msd parameterised-mutable-Array carrier, the aql:test
recursive framework, `template_prop_test` higher-order `each`, net top-level
drivers, and the Project-B construction-check diagnostic parity gap.

**Never-compiled paths (Ring 3):** the REPL, wasm playground, `aql exec`
server, `debug serve`, and the public `(*lang.AQL).Run` all call the
tree-walker unconditionally. Predicates (`Registry.RunPredicate`) and model
actions route through `InvokeCallback` but are never stamped; capturing
runtime fns decline detached stamping; `check-prop` bodies interpret;
concurrent-word bodies (`await`/`timeout`/`interval`) interpret in v1; dyn
do-body sites execute their bodies through the pooled sub-engine (no JIT
cache); `Vm.run` is the designed tier-1 island (cap 3, currently 0 rows).

**Structural caveat the headline hides:** the census is compile-time-only —
"9 refusals / 0 islands" cannot see VM runtime bails
(`vm.go` poly/user-poly/shaped-method/dyn-scope/dyn-frame defer sites) or
off-corpus shapes. Any completion claim must be grounded in an *executed*
census (Phase 10), not the compile-time one.

## Locked contracts

### C1 — L-DUP: effect fence + designed-bail elimination

A silent interpreter re-run is permitted only when **zero observable effects**
have been emitted since *before the check pass began* (a registry effect
ledger); otherwise the `internal_error` propagates with a "re-run with
`--no-compile` and report" diagnostic. The fence covers **both** fallback arms
of `RunCompiledReason` — the nil-Program path re-runs the source too, and the
check pass executes module imports — plus `InvokeCallback`'s internal-error
retry and (later) `Vm.run`. In parallel, every *designed* bail site converts
to compile-time refusal or sound in-VM handling, so by Stage J the fence on
the runtime-bail arm is provably never crossed. (Propagate-always rejected:
unacceptable interim UX while designed bails exist. Refuse-at-compile-time
alone rejected: cannot cover the defensive assertion belt. Buffering rejected:
non-stdout effects cannot be unwound.)

### C2 — Static-error oracle (permanent, named carve-out)

324 corpus rows are static-check-error rows whose interpreter-identical error
today comes from the fallback re-run; a checker diagnostic is NOT the
interpreter's runtime error (the interpreter partially executes, then raises),
and `compiled_fullcorpus_test.go` gates byte-identical error taxonomy over
those rows. Post-Stage-J the nil-Program branch splits: **refusal** → returned
error after rollback (count is 0 by then); **static check/parse error** → one
bounded interpreter run as the *error oracle*, fenced by C1 and
single-emission by check-pass effect-freedom (O1). Bounded and enumerated —
reachable only from the CompileCheck-err branch, never from refusals or
runtime bails.

### C3 — Rematch soundness rules (for `OpDispatchRematch`)

- **R1 Arm executability.** A recorded arm must run faithfully under the VM.
  `applyHandler` does **not** invoke — it returns the un-quoted Function and
  relies on the interpreter's re-step loop — so such arms are recorded
  **raise-only** with a carrier-based type-unreachability proof, or the site
  refuses. Native arms pass callPoly-style gates (no
  meta/fn-value/re-stepping/mutating/code-body/multi-result); user arms need a
  compiled unit.
- **R2 Completeness.** Every live overload (all arities — error rendering and
  dispatch consider the full sig inventory) is an executable arm, a
  proven-unreachable raise-only arm, or the site refuses. Mixed-arity words
  get per-arity window plans; runtime-ambiguous arity refuses.
- **R3 Predicate-blind matching is forbidden.** Any candidate sig with a
  DepScalar-refined param, `Patterns`, or fn-predicate type needs registry
  access the kernel-pure `MatchSignature` lacks (the exact hazard the
  recovery-site comment documents: a guarded CALL_USER enforces nominal type,
  not value-sensitive predicates) → the site refuses. A registry-aware
  predicate-faithful extension is a separate later design, not assumed.
- **R4 Rebind liveness.** Runtime rebinds are reachable (`OpBindDynScope` →
  `InstallDef`; the local-`add` row runs `def add` at runtime; body-local
  shadows are deliberately exempt from rebind poisoning). The recorder must
  prove no runtime rebind of the word can be live at the site; a runtime
  first-match on an **unrecorded** arm routes through the JIT/callback seam.
  Drift arms are real code paths with defined semantics — no
  `//covergate:allow`-once-proven-unreachable claims.

### C4 — End-state interpreter-entry contract

The fail-safe seams ("decline → interpreter, never the reverse") are permanent
by design, so "carve-outs shrink to check-mode + user opt-out only" is
unattainable and withdrawn. Final invariant: **no interpreter execution of any
program the compiler accepts, on any default path; every residual interpreter
entry belongs to an enumerated, named, per-seam-counted carve-out, and the
counts ratchet down only.** The permanent enumeration: (1) check mode;
(2) explicit user opt-out (`--no-compile`, debug-serve interactive stepping);
(3) C2 error-oracle runs; (4) fail-safe decline seams — stale `depSnap` →
`CallAQL`, JIT-declined bodies → pooled sub-engine,
capture-of-dispatching-binding stamp declines, busy-registry non-nested
callbacks; (5) R4 unrecorded-rematch-arm JIT-decline residue.

## Check-pass stance

`CompileCheck` executes `RunInCheckMode` words (def/import/type/macro/Test)
through interpreter machinery. **Acceptable and declared**: it is the compiler
front-end (static semantics, analogous to comptime/macro expansion), not
user-program execution; value-domain words are symbolically executed as
carriers, and all runtime semantics execute on the VM. Three enforced
obligations: **O1** effect-freedom (`TestCheckPassIsEffectFree`), **O2**
rollback completeness (`TestCheckPassRollbackComplete` — scopes, minted types,
stamp log), **O3** idempotence under re-check (the statement-at-a-time REPL
re-runs the front-end per line).

## Phases (dependency-ordered)

| # | Phase | Size | Risk | Depends on |
|---|-------|------|------|-----------|
| 0 | Bookkeeping ratchets + stale-comment cleanup | S | none | — |
| 1 | C1 effect ledger + fence (both arms) + check-pass effect gate | M | med | — |
| 2 | Entry-point routing: `NewFromRegistry`, `RunAutoValues`, REPL/exec/wasm/debug | M | med | 1 |
| 3 | Dispatch-error parity layer + `OpDispatchRematch` → refusals 9→0; convert VM no-match defers | XL | high | 1 |
| 4 | L-NP then L-JOIN (library-code blockers) | L | high | 1 |
| 5 | Off-corpus leaf cluster: L-DO, L-EACH, Stage-D 3/4 & 4/4, template `each`, net drivers, Project-B | L | med-high | 4 |
| 6 | Stamping extensions: predicates, model actions, captures, JIT cache, concurrent bodies, `Vm.run` runtime compile | L | med | 1 (3 for parity) |
| 7 | Module-fn param-slot compilation (module-fn-checkstate-ownership.{0-7}, re-baselined) | XL | high | 5 |
| 8 | aql:test recursive framework (spec-data inference + recursive code-body closures) | XL | high | 4, 7 |
| 9 | radix-msd parameterised-mutable-Array carrier | XL | high | 7 |
| 10 | Runtime-bail census to zero (`TestRuntimeBailCensus`, `TestNoInterpreterExecution`) | M-L | med | 3, 4, 6 |
| 11 | Stage J: delete the unbounded fallback; flip the public API (`Run`→compiled, `RunInterp` retained) | M | med | refusals=0, bail census=0 |

Orderings honored: the L-DUP fence (1) lands strictly before L-JOIN/L-NP (4,
the proven unmasker); bookkeeping first; Stage J last, gated on refusals=0
AND bail-census=0. Entry points (2) land early for real-world soak under the
fence.

### Phase 0 — Bookkeeping ratchets (S)

Constants and comments only. `refusalGate` 11→9; `computeRefusalCeiling` 17→0
(safe: all 9 refusals classify as `expectErr` error rows before the tier-2
partition, so the compute-gap actual is 0). Rewrite the stale tier-ledger
comments in `compiled_coverage_test.go` and `compiled_metafallback_test.go`
to describe the live `knownRefusals` set. `make status` regen. Never touch
ADR.md.

### Phase 1 — C1 effect ledger + fence (M)

1. `Registry` effect ledger: a generation counter bumped by `NoteEffect()` at
   every observable-effect seam (print/emit writer path, net sends, file
   writes, process spawn).
2. Snapshot **before `CompileCheck`** (the check pass executes module
   imports), fence **both** fallback arms in `RunCompiledReason` and the
   `InvokeCallback` internal-error retry: fall back silently only if the
   ledger is unchanged; otherwise wrap and propagate.
3. Check-pass effect gate (obligation O1).

Tests: `TestRuntimeBailAfterEffectPropagates` (print-then-forced-bail via a
test-only hook: one output, one error — the case the pure-value differential
is blind to) + positive pair `TestRuntimeBailBeforeEffectStillFallsBack`;
`TestRefusalFallbackAfterCheckEffectPropagates`;
`TestCallbackBailAfterEffectPropagates`; `TestCheckPassIsEffectFree`;
`TestCheckPassRollbackComplete`. Risk: a missed seam defeats the fence —
audit against the effect-word inventory; cover-gate forces every branch.

### Phase 2 — Entry-point routing (M)

Two real prerequisites first: **(a)** `lang.NewFromRegistry` (`lang.New`
builds its own registry; the REPL registry carries Manager/Output/resolver
wiring); **(b)** `RunAutoValues(src) ([]native.Value, error)` — a
Value-returning twin of `RunCompiledReason` (`convertResults` collapses
Integer→int64/String→string, so a `[]any` API cannot back the REPL's
`v.String()` rendering or `/stack` byte-identically). Then: REPL builds one
`*AQL` per session and runs statement-at-a-time `RunAutoValues(line)` (fresh
engine per line over a persistent registry is already the model — check-pass
`def`/`import` effects persist on the compiled path by `SnapshotForCompile`'s
contract); `exec`/wasm route to `RunAuto`; debug-serve interactive stepping
stays a C4 user-opt-out seam. The public `Run` does NOT flip until Phase 11.

### Phase 3 — Dispatch-error parity + `OpDispatchRematch` (XL, the effort center)

**3a. Error-parity layer.** The pure renderer exists
(`diag_msg.go::noMatchDiag`/`runtimeNoMatch`). What keeps `callPoly`'s
no-match *deferring* is the four tape/engine-state layers `sigError` adds:
(1) `voidArgErrorFor` reads live void-group state; (2) the `written`-tuple
selection between unclaimed forward tokens and the marker-bounded stack
prefix (no VM analogue — record the emit-time operand-region boundary as the
window bound; dynamic boundaries refuse); (3) the `reorderHint` tape
fallback (static text — record); (4) `maybeAddFnShapeHint` walks the live
tape (applicability decidable from static tokens — record). All four feed a
`DispatchErrCtx` recorded at compile time; the VM supplies the runtime window
and live sigs and calls the shared builder. Per-layer byte-parity tests;
extend the property fuzzer with no-match/void/reorder shapes.

**3b. `OpDispatchRematch` + `Program.Dispatches []DispatchSpec`**
(generalizing `PolyRef`/`UserPolyRef`), recorded at the three recovery sites
when poly / recovered-user-fn / definite-trap all decline — exactly today's
`MarkUncompilable` residue — subject to C3. Runtime semantics: build the
window per the recorded plan; run the kernel's `MatchSignature` over the
word's **live** sig list (the interpreter's own first-match). Match on a user
arm → `OpCallUser` discipline; on a gated native arm → dispatch, enforce
`NOut`; on a raise-only arm → defensive `internal_error` (proof violated); on
an unrecorded arm (R4 residue) → JIT/callback seam; **no match → shared-
builder raise, only under the raise-proof condition** (no selectable
`Fallback` sig, no 0-arg courtesy dispatch — `fnCourtesyDispatches`). How the
9 rows clear: the 2 `apply` rows record the `[Function]` arm raise-only
(operand statically Integer); the 4 generics rows re-match minted
instantiation types with the sole overload's unit as the executable arm; the
variadic-if→`each` row records `each`'s code-body arms raise-only with the
body operand as the **raw list value** (error-render parity); the local-`add`
row satisfies R4 (the body-local `def add` is frame-scoped and provably
exited); the word-splice row's window over the splice residual is static.
Delete `knownRefusals` entries row-by-row (the stale-entry arm of
`TestRefusalsAreFailures` forces it); `refusalGate` 9→0; the full gate battery
after **every** row (chained-leaf hazard).

**3c.** Flip `callPoly`/`matchUserPoly` no-match defers to faithful raises,
one commit per conversion, each carrying its per-site raise-proof condition
(`callDynamic`'s leave-as-residual-data semantics is preserved, not
converted). Sites failing the condition keep the defer pre-Stage-J and are
resolved in Phase 10.

### Phase 4 — L-NP then L-JOIN (L)

**L-NP first** — clearing L-JOIN unmasks L-NP's runtime bail; even fenced, no
new bail classes. Fix: make the `OpBindDynScope` write side bind the
fold/var-body local into the unit's dyn-scope frame before the read
(bind/read unit-ownership disagreement); if not reachable in one session,
land the sound half — a compile-time predicate refusing the shape. Either
way the runtime bail dies. **L-JOIN**: converge-then-record — run the
recursive fixpoint to type convergence with recording off, then one final
recording pass with the converged environment (stable-ID keying as the
fallback design). Gate: a whole-program multi-section
`--compile`==`--no-compile` byte-identical sweep (the trie_smoke shape that
caught L-DUP), not a row diff. Ported repros land as `lang/go` Go
regressions + new `lang/spec` rows.

### Phase 5 — Off-corpus leaf cluster (L, independent M items by leverage)

L-DO via `OpStackMark`/`OpDropToMark` extended to the catch merge (a variadic
dynamic-region carrier downstream; unseatable residuals keep the refusal);
L-EACH records the forward-collection plan from static token text; Stage-D
3/4 anchored to the `autoEvalList` gate **symbol** (the
`runPooledSub(…, e.isTop || consumed || e.elemEvalRecordable)` call — the
`[6,6]`-vs-`undefined word` hazard pinned by a negative test); Stage-D 4/4 as
a divergence variant (`NOut=-1`, `CompileDiverges` contract); net drivers via
per-iteration mark/collect in `for:` lowering (same primitive as L-DO);
Project-B via result-count-polymorphic dispatch through `recordPoly` with a
corpus-calibrated rebaseline (`TestCompileCheckMatchesCheckDiagnostics`);
template `template_prop_test` `each` via closure-unit capture over the
Stage-C `OpLookupDynScope` route.

### Phase 6 — Stamping extensions (L)

Predicates stamp at refine/`is`/typed-def construction and model actions at
model build (the `StampFnValueInPlace` pattern); capturing bodies compile as
closure units (reuse the `OpPushClosure` capture-slot machinery;
`InvokeCallback` already receives captures — seat into trailing slots);
**one** JIT detached-unit cache (keyed by body identity + def-table
generation) serving dyn do-bodies, check-prop bodies, and R4 unrecorded arms
— frame-local-interpolating bodies still decline
(`TestCheckPropInterpStringFnScopeRefuses` stays green); concurrent fork
bodies lower to closure units with VM re-entry under `-race`; **`Vm.run`
compiles at runtime** via fork-isolated compile (fresh `CheckState`, per the
`StampDetachedFn` precedent — never a mid-run `CompileCheck` on the executing
registry) + `SnapshotForCompile` rollback; a refused runtime string
interprets from the start (pre-effect — no L-DUP hazard).
`interpreterOnlyCeiling = 3` is retained with a rewritten rationale.

### Phases 7–9 — the multi-session features

**7** Module-fn param-slot compilation: execute the
`module-fn-checkstate-ownership.{0-7}` design after re-baselining `.7`
(grounded on a stale `refusalCeiling == 6`): re-baseline → hermetic help-eval
→ cross-registry `StartFnCompile` for module-fn bodies with real param slots
→ comparator capture (`comp:Function` feeding `OpCallDynTrailTop`) → sort
residuals. Highest-blast-radius seam in the tree; whole-suite sweep per step.
**8** aql:test recursive framework: spec-data type inference + recursive
code-body closure compilation; `OpDispatchRematch` is the relief valve where
dispatch still cannot statically resolve — subject to C3, never static
baking. **9** radix-msd: the parameterised-mutable-Array carrier (gradual
element types + set-widening) — a genuine type-system feature, deliberately
last; nothing depends on it.

### Phase 10 — Runtime-bail census to zero (M-L)

`TestRuntimeBailCensus` *executes* every compilable corpus row + the
combinations matrix via `RunProgram` with a test-only `Registry.onRuntimeBail`
hook counting every internal_error-class defer; ratchet pinned at the
measured count, must be 0 before Stage J. Burn down the remaining designed
defers (poly NOut drift, user-poly unresolved/drift, shaped-method,
dyn-scope dispatching/active-token, dyn-frame replay, and any 3c sites that
failed their raise-proof) — each becomes compile-time refusal, sound in-VM
handling, or an argued defensive arm. `TestNoInterpreterExecution`: assertion
hooks at `Engine.Run` / `RunResolved` / `CallAQL` / `runPooledSub` recording
any armed-mode entry with seam attribution; zero unattributed entries;
per-seam counters ratchet down only.

### Phase 11 — Stage J (M)

Gated on refusals=0, bail census=0, all off-corpus regressions green. Split
the nil-Program branch per C2 (refusal → returned error after rollback;
static error → the bounded oracle run). Delete the `runtimeShouldFallback`
re-run entirely — with the census at 0, any surviving `internal_error` is a
compiler bug and propagates; one-release escape hatch `AQL_COMPILE_FALLBACK=1`,
then deleted. Flip the public `Run` to the compiled path; the tree-walker
remains public as `RunInterp` (check-mode front-end, error oracle,
differential/specgen oracle — the retention that keeps `verify-bytecode`
meaningful). End-state ratchets: `refusalGate=0`, `islandGate=0`,
`knownRefusals={}`, bail census=0, `interpreterOnlyCeiling=3`, C4 per-seam
counters pinned. Gates: `TestNoUnboundedFallback` (the only interpreter call
in `RunCompiledReason` is the CompileCheck-err branch), updated
`TestRunCompiledReason` contract rows, pinned REPL/exec refusal behavior,
`compiled_fullcorpus` unchanged (the 324 static-error rows stay
byte-identical via the oracle).

## Verification doctrine

Every phase, non-negotiable: `make verify-bytecode` (byte-identical
differential incl. error taxonomy); the whole-suite
`--compile`==`--no-compile` sweep (never just the changed construct — the
chained-leaf hazard); `compiled_fullcorpus` / `compiled_property` (crank
`AQL_FUZZ_SEEDS`/`AQL_FUZZ_ITERS` on dispatch-touching phases) /
`compiled_combinations` (PATH rows per new opcode) / `compiled_concurrent`
(`-race`); hand-pinned off-corpus `RunCompiledStrict==Run` regressions per
leaf; `make fmt && make vet && make lint && make test && make cover-gate`;
positive+negative pairing on every gate change; no panics; never touch
ADR.md; `make status` on census moves; gates ratchet down only.

Standing end-state gate set: `TestNoInterpreterExecution` (C4 seams only,
attribution required), `TestRuntimeBailCensus == 0`, `TestNoUnboundedFallback`,
`TestRefusalsAreFailures` with empty `knownRefusals` + `refusalGate = 0`,
`compiled_fullcorpus` error-taxonomy parity, and the external voxgig
`--force-compile` sweep as a follow-on when the client repos are reachable.

## Test-first frontier suite (landed 2026-07-13)

Every remaining gap above now has a FAILING-BY-DESIGN test pinned under an
expected-red ledger (the knownRefusals stale/drift/bootstrap contract,
generalized). Three layers:

- **Go frontier cases** — `lang/go/frontier_ledger_test.go` (runner) +
  `frontier_cases_test.go` (cases): stamping (p6), hook-based interp-entry /
  bail censuses (p10), end-state Run (p11), built on the WS1 observability
  seams (`eng/go/interp_entry.go`: InterpEntry + BailEvent hooks).
- **Shared frontier TSV corpus** — `lang/spec/frontier/*.tsv` (join, do-catch,
  forward-drift, stage-d, for-multi, module-fn-param, aql-test,
  template-each): standard `input⇥expected` rows, interpreter-verified
  (`TestFrontierSpecInterp` — what a TS port runs), compile status pinned by
  `frontierCompileLedger` (`TestFrontierSpecCompiled`,
  `test/go/langspec/frontier_spec_test.go`). Graduation = row compiles →
  delete the entry, usually move the row into the main corpus.
- **knownRefusals target inventory** — `TestFrontierRefusalRowsCompile`:
  ledger DERIVED from knownRefusals (auto-coupled graduation), asserting the
  Phase-3 target (compile + byte-identical parity) per row.

### Bootstrap census (2026-07-13)

13 frontier TSV rows red: L-DO ×8 (7 × "fallible multi-value body under a
catch" + 1 chained leaf, below), net-driver ×1 ("for: body nets multiple
values per iteration"), L-EACH ×3 ("forward operand accounting across a
dynamic/island residual"), L-JOIN ×1 ("fn call operand of unknown
provenance"). Plus the 9 knownRefusals rows ("unmatched dispatch recovered").

### Discoveries (unexpected passes — green pins, noted per plan §Verification)

- **Constant dry-pass beats fnDefMayRaise**: `import "aql:struct-util" def g
  StructUtil.parse/r do [(g "x") 2] error [dot code]` COMPILES natively —
  parse's pure ReturnsFn dry-pass proves the constant call cannot raise, so
  the exact arity is sound. The raising twin (`(g "")`) refuses on a
  DIFFERENT leaf ("dynamic value precedes residual args (fn-value-call
  boundary)") — pinned as a chained-leaf ledger entry. The
  `bytecode_do_catch_test.go` fallback-list comment predates this
  (requireEngineParity's wantCompiled=false never asserted non-compilation).
- **aql-test recursion rows ×2, template-each, module-fn-param
  (inline-lambda comparator), stage-d ×2** already compile natively — kept
  as green must-COMPILE pins in the frontier TSVs.
- **radix-msd Array-carrier repro unconstructible** in-repo (no Array
  constructor reachable from spec wiring) — dropped with a dated comment in
  `frontier-array-carrier.tsv`; the external voxgig sweep owns it.

## Variation sweep (WS3, landed 2026-07-13)

`test/go/vary` re-embeds passing corpus rows in 14 compile contexts
(paren/fn/lambda/do/do-catch/if-arms/for/each/module bodies, statement
mixing, dirty-stack prefixes, splice) and classifies every variant through
the dual pipeline — the interpreter recomputes every expectation, so no
generated corpus is checked in. Consumers: `specgen -vary` (full-breadth
triage; outputs are artifacts, not committed) and langspec's standing
`TestVariationDifferential` gate (deterministic nested sampling —
`vary.Sample` orders by input hash so `AQL_VARY_SEEDS` cranking strictly
ADDS variants; default 32 seeds).

### First triage run (default breadth: 32 seeds × 14 transforms)

pass=384, refused=42 (9 buckets, all pinned in `varyRefusalLedger`),
islanded=0, interp/check-reject=12 (discards), **diverged=10 — a genuine
MISCOMPILE class found on the first run**:

**do-unit registry replay.** A compiled `do [...]` body containing
registry-mutating statements replays them against the check pass's surviving
state: typed defs re-install and raise "conflicts with an existing type
name" at VM time (minimal repro `do [def Big Integer 15 is Big]` — compiled
yields an Error value, interpreter yields `true`); the mirror half is that
CHECK-TIME registrations (ParseLang.register, minilang kinds) are INVISIBLE
to the compiled run (`parse_unknown_lang` vs `9`). Value defs are
unaffected. This ships today for any user writing `do [def T … …]` — it is
not a variation-only artifact. Pinned: 10 variants (5 seeds × do-body/
do-catch) in `varyKnownMiscompiles` (stale-armed) + 2 minimal frontier rows
in `lang/spec/frontier/frontier-do-registry-replay.tsv` ledgered on the
parity-failure mode. Fix belongs to the do-unit RunInCheckMode-word
semantics (Phase 5/6): the unit's type-def/registration path must be
idempotent the way top-level `OpPushType` already is.

Refusal buckets (count @ default breadth): dynamic/opaque output ×12, check
diagnostics (wrapped-context false positive) ×11, runtime bail ×7, for-multi
×3, dynamic input ×2, L-DO do-catch ×2, residual lowering ×2, stack
discipline ×2, operand provenance ×1.
