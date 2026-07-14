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

### Phase 3 — Dispatch-error parity + `OpDispatchRematch` (XL → L, de-risked 2026-07-13)

**Re-sizing note (frontier-suite exploration):** all 9 `knownRefusals` rows
are ERROR rows in the corpus (signature/signature_error — apply.tsv:37/38,
edge-types-3.tsv:35/119, generics-sugar.tsv:37, generics.tsv:60,
forward-barrier.tsv:80, open-words.tsv:32, word-splice.tsv:115). For them
`OpDispatchRematch` degenerates to a RUNTIME-EVALUATED TRAP: re-run
`MatchSignature` over the concrete window at VM time, expect no-match, raise
via the shared builder (`diag_msg.go` noMatchDiag/runtimeNoMatch — already
shared by the interpreter's sigError and the VM param-contract guards). No
dispatch arm is ever selected for these rows, so the matched-arm execution
half of 3b can land later with 3c. The blocker each row hits today is
`tryRecordUnmatchedDispatchTrap`'s carrier/dynamic/splice decline
(engine.go:8194-8233) — every row carries a non-concrete operand, so the
rich diagnostic must be built at runtime over the concrete window rather
than baked.

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

**Do-unit registry replay (added 2026-07-13, found by the variation
sweep).** Root cause: `RunCarrierBodyWithDefs` (carrier.go) rolls back only
`r.Defs` after the check-time do-body run — the body's `def Big`/`import`
also mutate `r.Types.parts` + the minted lattice + `Modules.loaded`, which
are NOT rolled back and ARE kept on the compiled path (by design:
OpPushType resolves minted IDs). The VM replays the baked const-list body
via RunResolved over the same registry → `InstallType` parts conflict for
typed defs; the import half re-binds a PLAIN MAP instead of a ModuleExport
(`ensureExportsBound`) so mini/parse kind lookups fail. Near-term fix
(landed with the frontier suite follow-up): refuse to bake replay-hazard
bodies (import / capitalised def) — "slow, not wrong" — plus the
ensureExportsBound value-kind fix. Full graduation belongs HERE: the JIT
detached-unit cache compiles dyn do-bodies as units (no token replay), and
the unit's RunInCheckMode-word semantics become idempotent by construction
(the check-time install is the only install).

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

**Re-scoping (2026-07-13, frontier bootstrap):** several Phase 7–9 target
shapes ALREADY COMPILE and are pinned green in lang/spec/frontier/: the
aql:test recursion rows ×2 (frontier-aql-test.tsv), the module-fn-param
inline-lambda comparator (frontier-module-fn-param.tsv), template-each
(frontier-template-each.tsv), and stage-d ×2. Re-baseline each phase
against the live census before executing — the remaining work is narrower
than the phase descriptions above (e.g. Phase 7's comparator seam may only
need the /r-parked named-fn form; Phase 8's remainder is spec-data
inference breadth). The radix-msd repro is unconstructible in-repo
(frontier-array-carrier.tsv, dated note) — Phase 9 is driven by the
external voxgig sweep.

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

## TSV migration (WS4 priority-1 batch, landed 2026-07-13)

41 off-corpus Go parity regressions migrated into the main shared corpus as
`lang/spec/bytecode-migrated.tsv` (expected values interpreter-derived;
every row verified native — no refusal, no island — before entry):
computed-map fn-body/do-map shapes (bytecode_computedmap_test.go, whose
positive loops were removed — the negatives, deferred-residual divergence
guards, stay in Go), infallible multi-value do bodies
(bytecode_do_catch_test.go native list removed; the fallible fallback list
stays, coupled to frontier-do-catch.tsv), OpCallUserPoly dispatch
directions, and the EDGE-SPEC-FINDINGS over-widening guards
(bytecode_edge_findings_test.go — the Go file keeps the refusal/reason
pairing). Census: 5934 → 5975 fallback-free native rows;
`minCompiledRows` 2224 → 2265; refusals unchanged at 9;
`COMPILED_STATUS.md` regenerated.

Follow-on batches (recorded, not yet executed): mergedword/fnvalue-m2/
stage2 value halves already mirror existing corpus rows (open-words.tsv,
path-modifier.tsv, recursion.tsv, module-minilang.tsv) — their Go files
keep the native-lowering/no-island halves and stay put; priority-2 legacy
corpus unification (lang/go/test/*.tsv, 12 files, older schema) and
priority-3 lang/go/test value tests remain as scoped in the plan. The
p2 entry-point frontier cases (repl/exec runner copies) also remain as
the WS1b follow-on.

## Phase 2 landed + a security discovery (2026-07-13)

Entry-point routing landed: `lang.NewFromRegistry` + `RunAutoValues` (the
Value-returning twin of RunCompiledReason — one shared core, the []any
variant is now a converting wrapper); the REPL runs one `*AQL` per session
with per-line `RunAutoValues` (parse-probe preserved for the historical
"parse error:" prefix; state persistence unchanged via the
keep-on-compile contract); `aql exec` routes through `RunCompiledReason`.
p2 pins live in cmd/go/internal/{repl,exec} (compiled line + zero
unattributed interp entries + refused-line fallback).

**SECURITY DISCOVERY (new Phase 10 item): compiled dispatch bypassed the
engine word policy.** `policyGateWord` runs per interpreter stepWord
dispatch (skipped in check mode by design) and the VM never consulted it —
so `aql run` (default CompileTry) with a "deny add" policy RAN `1 add 2`
compiled to 3 where the interpreter raises permission-denied; exec
inherited the hole the moment it moved to the compiled entry (caught by
its policy test). Closed conservatively in CompileCheck: a registry with
an installed word checker refuses compilation ("policy-gated registry"),
so the interpreter — where the gate lives — owns every dispatch. Pinned by
lang/go/policy_compiled_gate_test.go (deny parity + strict-mode surfacing
+ the no-policy negative). LIFTING the refusal is a Phase 10 item: a
VM-side policy gate at CALL_NATIVE / CALL_USER / poly / dynamic dispatch
(arg-conditional rules need the runtime window, so the gate must live at
dispatch, not compile time), then delete the CompileCheck refusal and the
reason string.

## L-DO implementation map (Step 3 analysis, 2026-07-13 — execute next)

The closure path already compiles do bodies COUNT-AGNOSTIC (tryRecordClosureCall
callable_words.go:200-205: BodyOutResidual multi-out; the VM's frameless RET
returns the full residual). The exact-N seat is the only unsound piece for a
fallible body (catch → 1 Error vs N): the gate at callable_words.go:508-516
(closureResidualExact) admits the exact-N unit and the dispatch seats N.
Two-part fix:

1. **Variadic mark at the record sites.** Move the fallibility scan
   (doBodyMayRaise/tokensMayRaise/wordMayRaise/fnDefMayRaise,
   native_control.go:324-398) into eng (ModuleExport detected by type PATH —
   the isModuleFamilyValue precedent, carrier.go:1089-1104). In
   recordClosureDispatch (callable_words.go:412-541): when
   sig.CompileEffect.Has(CompileFallbackBody) && len(outs) > 1 &&
   tokensMayRaise(bodyToks, r), set f.variadicResult on the event
   RecordClosureCall appends (add a flag param). Same condition at the
   generic RecordCall fall-through (emit.go:3570) and keep
   tryRecordDynBody's existing unconditional variadic mark. Then DELETE
   doListReturnsFn's MarkUncompilable arm (native_control.go:318-320) — the
   ReturnsFn keeps returning the full residual for check precision.

2. **Region-top consumption for `error`** (the catch merge). `error` is
   StripsUnconsumedInput (stripResidualShapeOK callable_words.go:559-587):
   its runtime read is EXACTLY the top of stack — depth-agnostic — so a
   variadic region below it is consumable: teach layoutOperands/the lowerer
   that a strip-input dispatch over a variadic region pops 1 at runtime and
   yields region' = variadic (region minus top plus handler result). The
   program residual absorbs region'; any OTHER fixed-arity consumer of the
   region keeps the refusal ("unseatable residuals keep the refusal").
   OpStackMark/OpDropToMark (bytecode.go:190-192, vm.go:1784-1808) are
   available if the lowering needs an explicit region boundary; the
   variadic-if claiming lowering (lower.go:424/2198/2206) is the model.

Verification: the 8 frontier-do-catch.tsv rows graduate row-by-row (ledger
stale arms fire); TestDoCatchMultiValueArity's fallback list flips to
native (update wantCompiled + the comment); the migrated infallible rows
must stay native; whole-suite differential + verify-bytecode after EVERY
sub-change (the chained-leaf hazard); watch the vary gate's do-catch bucket
graduate.

Net drivers (for:) reuse part 1's variadic mark at RecordLoop
(emit.go:3267-3274) + per-iteration mark/collect; L-EACH and Stage-D as
scoped in the Step 3 plan above.

### L-DO part 1 LANDED (2026-07-14)

The SetCatchVariadic latch is in (EmitRecorder + EmitState; consumed by
RecordClosureCall, the generic RecordCall, and tryRecordDynBody, keyed to
the CompileFallbackBody sig); doListReturnsFn latches instead of refusing.
First graduation: `def y 5 do [(10 div y) 2]` compiles natively (moved to
the main corpus); the vary gate's do-catch bucket graduated. The remaining
6 frontier do-catch rows drifted to the DOWNSTREAM refusals ("residual
shape beyond Stage 1", "dynamic value precedes residual args", "residual
value not statically materialisable") — part 2 (region-top consumption for
the strip-input `error` dispatch over a variadic region) is the next
execution item, per the implementation map above.

### L-DO part 2 — next probe (continuation note)

The 6 remaining do-catch rows' drifted refusals come from the PROGRAM
residual layout (emit.go ~5985-6000: "dynamic value precedes residual
args" fires in the unhandled-dynamic loop; "residual shape beyond Stage 1"
from seatResults; "residual value not statically materialisable" from
resolveOperand/materialise). Per-row check-time shapes differ (an
always-raising body nets a single Error carrier at check time — no latch —
while the runtime also nets 1: consistent; the def-msg rows net 2). Next:
probe each row's residual shape at Finalize (which carriers, which flags),
then extend the strip-input consumption per the L-DO map. Row 6-family may
need the `error` handler result de-dynamified (dot code over a concrete
Error field read) rather than region work.

### L-DO part 2 — refined design after the row-8 probe (2026-07-14)

Row-8 check-time shape: the body nets [Any-carrier(dynamic), 2] → latch →
variadic do event; `error` strips the top and returns a dynamic result;
the PROGRAM residual then holds a leading Dynamic that sigTypeMatches
Function → the emit.go:5994 guard fires. The fix is VARIADIC PROPAGATION
through strip-input dispatches: in RecordClosureCall, after resolving ops,
if any operand's producedBy event carries variadicResult AND the sig is
StripsUnconsumedInput (`error`), mark THIS event variadicResult too — the
region stays a region (top consumed, handler result pushed; runtime pops
are depth-agnostic). The program residual then absorbs it exactly as it
absorbs `do [for 3 [1]]`. Fixed-arity consumers of the propagated region
keep refusing. Verify each row against the interpreter before ledger
graduation; expect the def-msg rows to need one further hop (def consuming
the propagated region top).

### L-DO part 2b — absorption seams located (2026-07-14)

Two absorption seams exist already: fn-unit finish (emit.go ~2991:
variadic TAIL event → rec.variadic, with the dynBodyResult/declared-tuple
exception) and the program-residual layout (the 5994 fn-boundary guard
runs BEFORE seating; single-entry variadic residuals pass because the
guard loop needs i+1<len). The 6 rows fail because their residuals hold
REGION entries in non-tail positions ([region..., handler-result]) — the
layout must treat a contiguous variadic-region prefix/tail as one
absorbable unit (OpStackMark region or force-locals promotion like the
out-of-order mirror at ~2996) instead of walking its entries into the
Dynamic-matches-Function guard. Next: trace seatResults' handling of
`do [for 3 [1]]` (the working single-entry case) and generalise.

### L-DO part 2b — corrected target (2026-07-14, final for this pass)

seatResults ALREADY absorbs contiguous same-event variadic runs to the end
(lower.go:1382 sameEventRunToEnd); the 6 rows refuse EARLIER at the
fn-boundary guard, and the guard is CORRECT: an Any-typed region entry
could be a parked Function, and the interpreter's stepLiteral auto-applies
values landing above a parked fn — a verbatim push would be unsound. The
sound lowering is the VERBATIM WINDOW family (OpCallDynamicMixed /
trailingWindowApplyShape, emit.go ~5960-5985): island the region + its
consumers as one window the VM re-steps exactly as the interpreter,
generalised from the single-dynamic-entry gate to a VARIADIC REGION
(bounded by OpStackMark). Alternatively per-row: narrow the do body's
static types so Function is excluded (the guard's existing not-disjoint
carve-out then admits them) — worth checking whether typed fn returns
([Any] on f) can narrow through the catch merge. Both routes preserve the
auto-apply hazard soundly.

### L-DO END-TO-END CONFIRMED for typed returns (2026-07-14)

`def f fn [[x:Any] [Integer] [raise …]] do [(f 5) 2] error [dot code]`
COMPILES NATIVELY with byte-identical parity on the CAUGHT path — the
variadic latch + strip-input propagation carry the full catch merge when
the body's static types exclude Function. Pinned in the main corpus
(bytecode-migrated.tsv). The 6 remaining frontier rows are Any-typed
(module exports / [Any] returns) — held by the CORRECT parked-fn
auto-apply guard; they graduate via the verbatim-window generalisation or
by future return-type narrowing through the catch merge.

### Net drivers — first attempt REVERTED (2026-07-14)

The naive route (multi-out loop body: out=nil + allowVariadic through
lowerFragment, per-iteration values accumulating on the VM stack) compiled
but DIVERGED: `for 3 [1 2]` → compiled [] vs interp [1 2 1 2 1 2] — the
fragment reconciliation with no out operand DISCARDS the per-iteration
residual rather than leaving it. Reverted on the spot (the differential
probe caught it pre-commit). The correct route is the plan's per-iteration
mark/collect: an OpStackMark region per iteration with the fragment's
values retained above it (or an explicit per-iteration collect op),
mirroring how lowerFragment's out-operand path STORES the single value.
Inspect lowerFragment's out=nil path (what pops the sim values) before the
next attempt.

## Phase 5 state after the 2026-07-14 push (Step 3 consolidation)

CLEARED: L-DO machinery end-to-end (typed-return catch merges compile the
caught path natively; the variadic latch + strip-input propagation are
in); net drivers part 1 (computed multi-value loop bodies accumulate
per-iteration via residualN reconciliation, parked-fn screened). Already
green from the WS2 discoveries: Stage-D rows ×2, template-each,
module-fn-param (lambda form), aql-test recursion ×2 — i.e. the Phase 5
"Stage-D 3/4 & 4/4, template each" items and the Phase 7/8 lambda-form
and recursion shapes were narrower than planned or already done.

REMAINING in Phase 5: L-DO part 2b (Any-typed regions — the verbatim-
window generalisation, mapped); L-EACH (the forward-plan recording:
redirect the recorded dispatch to the INTERPRETER's forward/stack split
[forward-literal → sig0, concrete-stack → sig1] where the check pass's
Dynamic-blocked all-stack match diverges — deep matcher work, needs
FORWARD-COLLECTION-PHASES.10.md study first); net-driver residues
(inert-const tails need const re-push in the reconciliation; Function
regions stay refused by design).

## L-JOIN converge-then-record — concrete flow (2026-07-14)

Seam: carrier.go ~3979-4004 (AnalyseFnBody's refinement block). The armed
case currently SKIPS refinement because each extra runOnce re-records into
the same open fragment — and the FIRST armed runOnce records under the
weakest (Any-bail) hypothesis, which is exactly the iteration-mismatch
L-JOIN divergence (operand and producedBy from different rounds). The fix
uses the EXISTING recorder primitives, in this order:

    cp := rec.Checkpoint()          // before the first runOnce (armed only)
    result := runOnce()             // records under the weak hypothesis
    if InflightBails grew && armed && !stackHasVariadic(result) {
        rec.Rollback(cp)            // discard the weak-hypothesis recording
        resume := rec.Suspend()
        result = refineRecursiveSummary(...)   // converge, recording OFF
        r.Check.FnSummaries[key] = result      // seed the converged memo
        resume()
        result = runOnce()          // ONE recording pass, stable IDs/types
    }

Open question to verify first: Rollback's blast radius across an open
StartFnCompile unit (does it restore units/fragments or only events?) —
read emitCheckpoint's fields; if units are outside it, take the checkpoint
inside the unit's fragment instead. Gate: the L-JOIN frontier row
(frontier-join.tsv) + the whole-program multi-section
--compile==--no-compile sweep (the trie_smoke shape).

### L-JOIN open question RESOLVED (2026-07-14): Rollback is not units-aware

emitCheckpoint records {seq, consts, types, fallbacks, fnRecs,
siteCounts}; Rollback GUARDS on len(fnRecs) != cp.fnRecs and silently
NO-OPS — a recursive body's runOnce mints units via StartFnCompile, so the
converge-then-record flow requires extending Rollback to unwind fnRecs >
cp.fnRecs first: truncate es.fnRecs, delete their unitKey memo entries and
any localByID/promoted state keyed to the dropped units — StartFnCompile's
own fn-unit cleanup ("mirroring" comment inside Rollback's provenance
loop) is the model. THEN the checkpoint/rollback/suspend/re-record
sequence from the flow above applies unchanged. Verify with the L-JOIN
frontier row + the fnRecs-guard unit test (a rollback across a minted
unit must fully unwind, not no-op).

### L-JOIN converge-then-record — first attempt REVERTED (2026-07-14)

The naive wiring (units-aware Rollback + suspend-refine-reseed-rerecord in
AnalyseFnBody) did NOT clear the L-JOIN row (still refuses with the pinned
provenance reason — the final recording pass still misses the operand
home) and REGRESSED the aql-test recursion green pins ("fn
test-describe$body: body leaves extra values (Stage 3 lowers in-order
results)") — the re-record pass leaves different residuals for
already-working recursive shapes whose first-pass recording was correct.
Reverted on the spot (the frontier must-COMPILE arm caught it
immediately). Learnings for the next attempt: (1) the flow must apply
ONLY when the first pass's recording is actually provenance-broken (gate
on the resolveOperand failure signal, not on every recursive bail); (2)
the Rollback units-extension changes the loop-analysis caller's semantics
too — land it separately with its own unit test; (3) the aql-test pins
double as the regression canary for ANY recording-pass restructure.

### L-EACH mechanics (2026-07-14 close-out read)

refuseForwardStackDrift (engine.go:2857) fires when: active recording, a
forward-eligible non-NoEvalArgs sig matched ALL-STACK with the top tape
operand Dynamic + a deeper concrete operand consumed, and the next tape
token is a forwardLiteralOperand. The divergence: the checker's all-stack
split consumed [Dynamic-top, 5] while the runtime's concrete top lets the
word forward-collect the literal → [1(fwd), 7(stack)] — DIFFERENT
OPERANDS, different consumed set. The fix must RE-DISPATCH at check time
under the runtime's split: re-run matchSignature with the forward token
offered, adopt its operand assignment for the recorded call, consume the
forward token from the tape walk (pointer coordination —
FORWARD-COLLECTION-PHASES.10.md's two-phase contract), and leave `5`
unconsumed. Multi-session item: touches the dispatch commit path, not
just the recorder. The 3 frontier rows + the edge-findings negatives
(mul/sub/String-token forms that already compile) are the gates.

### Phase 6 progress — predicate + model-action stamps landed (2026-07-14)

Two of the six stamping extensions are in, both via the established
detached-unit primitives (no new machinery):

- **Predicates** (`p6/predicate-stamps-and-runs-vm` graduated):
  `InstallType`'s predicate arm stamps the body at construction —
  `StampDetachedFn` over a NAMED copy (the type name labels the stamp
  event; anonymous predicate bodies otherwise record unfindable
  empty-name events) + `stampCompiledRef` onto the original shared impl,
  gated on `RuntimeStampingEnabled`. `RunPredicate`'s existing
  `InvokeCallback` then runs refine/is/typed-def predicate bodies on the
  VM.
- **Model actions** (`p6/model-action-stamps` graduated): `buildActions`
  stamps each action at model build via `stampActionFn` —
  `eng.StampFnValue` (the net-codec/service-handler CLONE precedent: the
  user's spec value stays plain) over the model's private copy, which
  takes the ACTION name when the spec lambda is anonymous (event label +
  action-error attribution, applied on compiled and interpreted paths
  alike so parity holds). `makeAction`'s `InvokeCallback` runs the unit;
  captures decline to CallAQL unchanged.

Discovery while pinning the negatives: a lambda written directly as a
map VALUE (`actions:{gen:([mod:Any] => [flag])}` inside a fn body) runs
NO capture analysis — data-context ParenExpr construction never sees the
enclosing frame, so the body's `flag` read fails at action-invoke time
with undefined_word on the interpreter TODAY (pre-existing, independent
of stamping; pinned in `TestModelActionDataContextLambdaDeclineIsRenamed`).
The capturing form that works is word-context construction (`def act
([mod:Any] => [flag])` then `actions:{gen: act/r}`), which captures and
declines the stamp with the lexical-captures reason. Whether data-context
lambda construction SHOULD capture is a language-semantics question for
the maintainer, not a compiler gap — the compiled path faithfully
reproduces the interpreter either way.

Remaining Phase 6 items: capturing closures via OpPushClosure capture
slots, the JIT detached-unit cache (also graduates the
do-registry-replay rows natively), concurrent fork bodies under -race,
`Vm.run` runtime compile.

### Coverage-gate repair riding the model-action commit (2026-07-14)

The model-action battery surfaced a LATENT cover-gate failure that predates
this branch — CI runs `make test` but never `make cover-gate`, so the gate
only holds when it is actually run locally, and two regressions had slipped
through:

- `freshenFnUnitConsts`'s refusal return became DEAD on main (b2e3989,
  2026-07-11: the multi-read escaping case seats a per-call local instead
  of refusing) — the caller's `return nil, reason, false` arm in Finalize
  was unreachable ever since. Removed: the pass returns nothing now, and
  the stale "Otherwise refuse" doc text names the local seat instead.
- `promoteLateDynBind`'s promotion tail (lower.go, 11 statements) went
  corpus-invisible: plan-time value-def promotion now seats every
  dyn-bound source the corpus produces before Finalize, so the late pass
  finds each srcSeq already in rec.promoted. The pass remains the sound
  backstop for late DynEnv arming; its contract is pinned directly by
  `TestPromoteLateDynBind` (seat + rewrite, all four skip gates, the
  disarmed no-ops).
- The net-drivers multi-value arm's unknown-provenance refusal
  (emit.go RecordLoop) had no deterministic pin. A Module instance atop
  a multi-value loop body is the minimal in-repo shape;
  `TestForBodyUnknownProvenanceRefuses` pins reason + fallback parity
  for both net arms.
- The vm.go runUnitNested cross-program covergate pragma GRADUATED: a
  detached-stamped ref (model action, predicate) invoked mid-VM-run is a
  normal interpreter-fallback path now, hit by the p6 frontier cases.

Follow-up recorded for the maintainer: CI should run `make cover-gate`
(the gate is only as strong as its least-run invocation).

### Phase 6 progress — concurrent fork bodies landed (2026-07-14)

`p6/concurrent-fork-bodies-on-vm` graduated: await's parallels arg gets a
new CompileEffect, `CompileStoresBodyList` — the spawn store-body pattern
applied PER ELEMENT. The recorder compiles each parallels element to its
own 0-param unit (`compileStoredBody`) and rebuilds the list with
synthetic fn-value carriers; `runParallelBranch` recognises a carrier and
runs it via `RunUnit` on the branch's ForkConcurrent fork (exactly
`spawnHandler`'s run side), with the C1 effect fence on an internal_error
(re-run the raw tokens on the interpreter only when the unit emitted no
observable effect — `eng.IsInternalError` is the exported face of the
seam's taxonomy check). An element that refuses to compile keeps its raw
list and THAT branch interprets — per-element and sound (pinned:
a Module construction in one branch interprets while the sibling runs
compiled, with parity). Modes all/full/first/any share one outcome
mapping (`branchOutcome`) so compiled and interpreted branches agree
exactly. The nested-concurrency shape (program forks per goroutine ×
branch forks per run, all sharing one Program) rides the CI -race gate as
a new TestCompiledConcurrencyRaceFree case; two corpus rows carry the
shape into the shared TSV suite.

Remaining Phase 6 items: JIT detached-unit cache (graduates the
do-registry-replay rows + p6/check-prop-body-on-vm), capturing closures
via OpPushClosure capture slots, Vm.run runtime compile (needs the
Phase 10 VM-side policy gate — sub-engines are always policy-composed).

### Phase 6 JIT detached-unit cache — scoping (2026-07-14 explorer read)

Two re-interpretation seams feed the cache, plus the auto-eval seam:
(a) code-body: InvokeBody (invoke.go:21) → vmContext.invokeClosureOn
(vm.go:382) whose RAW-LIST arm (vm.go:388) RunResolved-s into the pooled
sub-engine — fed by `do`'s baked CALL_NATIVE (doListHandler
native_control.go:224; noEvalBodiesInert emit.go:5024 / tryRecordDynBody
carrier.go:1520); (b) check-prop: runCheckProp (modules/test.go:734) runs
gen/property bodies PER ITERATION via parent.CallAQL (test.go:782/:809,
throwaway FnSig per call) — the "CallAQL" unattributed entries;
(c) runPooledSub auto-eval sites (engine.go:3866/3956/4119/4213/4229/
4266/4391/4623) are the Phase-10 census seam, not this cache's target.

Key machinery: CompiledFnRef.depSnap/depsFresh (bytecode.go:591/608,
per-name Defs.Gen deftable.go:61); body identity should key on the
STRUCTURAL FnAnalysisKey precedent (callable_words.go:57), not Value.ID
(recorder re-mints IDs; not a content hash). runUnitNested refuses
foreign-program refs (vm.go:359) — a detached unit invoked mid-run takes
the interpreter, so units that must run inside the live program compile
INLINE into its Fns, not as detached Programs.

Decisions from the read:
1. **check-prop bodies graduate via compile-time closure units, not a
   runtime cache**: the Test.check-prop dispatch site can compile its
   MODULE-SCOPE gen/property bodies like any code-body word
   (compileStoredBody-style carriers or closure units at the record
   site), and runCheckProp runs a carrier via RunUnit/InvokeCallback per
   iteration instead of the throwaway-FnSig CallAQL. The fn-scope
   ${frame-local} interpolation case MUST keep refusing
   (TestCheckPropInterpStringFnScopeRefuses stays green forever).
2. **do-registry-replay rows are NOT graduated by the cache alone**: the
   body-unit's `def Big Integer` must lower idempotently against the
   KEPT check-pass state (Types.parts / minted lattice / Modules.loaded
   survive by design — compile_sandbox.go:9; RunCarrierBodyWithDefs
   rolls back only r.Defs, carrier.go:3043). The unit must resolve the
   check-time mint via OpPushType/OpBindTyped (the top-level typed-def
   path, emit.go:3575 / lower.go:1587) rather than re-running
   InstallType (which trips IsKnownPart → "conflicts with an existing
   type name", types.go:357). And it must be compiled INLINE against the
   live registry's mint — a detached fork compile mints on the fork
   (fork.go:41) and its OpPushType IDs mean nothing to the live
   registry. Sequence: land the check-prop half first; the typed-def
   do-body half is its own item (InstallType-idempotent body lowering).

Refusal-path map for the do rows (for the later half): tryRecordClosure
declines → tryRecordDynBody declines on bodyHasReplayHazard
(carrier.go:1540; emit.go:5068) → the code-body refusal at emit.go:3830
fires "code-body word do" (emit.go:3855) — pinned at
frontier_spec_test.go:185, bytecode_replayhazard_test.go:20, TSV rows
frontier-do-registry-replay.tsv:13-14.

### Phase 6 progress — check-prop bodies as stored-param units (2026-07-14)

The JIT-cache scoping's check-prop half landed WITHOUT a runtime cache:
`Signature.StoredBodies []StoredBodySpec{Pos, Params}` declares a word's
param-carrying stored code-body positions (registration folds it like
Callable), and the recorder's new STORED-PARAM-BODY edge —
`compileStoredParamBody`, MODULE SCOPE ONLY — compiles each declared
position to a closure unit whose param slots bind the declared params,
riding as a carrier whose single sig mirrors the handler's own CallAQL
sig (same Params, same raw Body) plus the CompiledFnRef. `runCheckProp`
(storedBodyArg) dispatches a carrier through InvokeCallback: the unit
runs NESTED on the VM (same-program ref) with the identical CallAQL
frame as its per-invoke fallback. Declines everywhere leave the raw list
and the interpreter path byte-identical.

Ledger motion: p6/check-prop-body-on-vm DRIFTED, not graduated — the
per-iteration CallAQL entries are GONE and iteration count adds ZERO
entries (TestCheckPropIterationsAddNoInterpEntries pins invariance
2 vs 60 runs), but the case still sees Engine.Run×68 + runPooledSub×65:
`import "aql:test"` MODULE-LOAD AQL (BuildTestModule preambles),
identical for an import-only program. Re-pinned as a Phase 10 item —
the module-load C4 attribution seam, not body compilation. The fn-scope
guard held only after gating the new edge to module scope (the first
attempt compiled the ${frame-local} body and
TestCheckPropInterpStringFnScopeRefuses caught the miscompile risk
immediately — the module-scope gate is load-bearing). The replay-hazard
gen body falls through to the standing NoEvalArgs gates and refuses the
program (parity via fallback, pinned).

Remaining Phase 6: capturing closures via OpPushClosure capture slots;
Vm.run runtime compile (Phase 10 policy gate); do-registry-replay rows
(idempotent type lowering — see the scoping section above).

### Phase 3 OpDispatchRematch — implementation map (2026-07-14 read)

The 9 knownRefusals' actual refusal reason IS "unmatched dispatch
recovered at <word>" — latched at engine.go:8025 (and the sibling
single-overload path at :7960) after BOTH recoveries decline:
tryRecordUnmatchedDispatchTrap (engine.go:7992 → :8154) and the
imprecise-carrier poly re-match (:8013-8021). The shared rich-diagnostic
builders are DONE (diag_msg.go): `runtimeNoMatch(r, name, written)`
builds the interpreter-identical error from runtime values alone —
exactly the rematch's no-match arm.

Insertion point: NOT a new site — restructure the trap's per-position
screen loop (engine.go:8198-8234). Today a CARRIER position declines the
whole trap immediately; instead classify the full window first:
- any DYNAMIC / UNDEFINED / Reach / ParenExpr / InterpString / Splice
  position, in-window open paren, or a 0-arg real sig → decline (today's
  arms, unchanged);
- zero carriers → today's definite OpTrap (byte-identical serialised
  error);
- ≥1 carrier → attempt the RUNTIME REMATCH record: resolve EVERY window
  value via es.resolveOperand (a make-result carrier is event-produced →
  resolvable), mirror RecordTrap's depth-1/top-level guard, and record a
  terminal OpDispatchRematch {word, operand list in the written-tuple
  order sigError uses, pos} into Program.Dispatches. VM semantics: gather
  the operand values, fn := r.Lookup(word) (LIVE binding — matches the
  local-add row where the body-scoped overload is gone by the time the
  top-level dispatch runs), value-level re-match; NO MATCH → raise
  runtimeNoMatch(r, word, written) with the recorded pos stamped —
  byte-identical to the interpreter; MATCH (static model was wrong — a
  refined tag / predicate satisfied at run time) → vmDefer-class internal
  error → RunCompiled's fenced interpreter fallback (slow, not wrong; the
  tail was truncated at the terminal op so it cannot continue).
- the diagnostic parity: the recovery's no_signature diagnostic is
  emitted only on !Compiling passes (engine.go:8028-8035), so the compile
  pass needs no RuntimeMirror plumbing — the rematch record simply
  replaces the MarkUncompilable at :8025 (and Finalize truncation follows
  the trap precedent).

Open per-row questions for the implementation session (probe each):
1. WINDOW vs STACK operands: checkModeFallbackPositions covers forward
   TAPE positions; rows whose failing operand was already ON THE STACK
   at dispatch (`5 inc apply` — inc fired, apply sees a non-fn) need the
   written-tuple reconstruction to include stack operands — find what
   sigError passes as `written` at these sites and mirror it exactly.
2. The word-splice row (`f p`, p = `word [1 add 2]`): IsSplice declines
   today; the plan note says the window rides the STATIC splice residual
   — needs its own record shape; probe whether the splice fires before
   f's match in check mode.
3. The variadic-if each-row: the raw list operand must be recorded for
   render parity (the plan's per-row note).
4. Arity: some candidates examine fewer positions than maxN — confirm
   noMatchDiag's written tuple for these rows equals the interpreter's
   (run each row interpreted, capture the error, then assert the trap's
   runtime build matches — the differential's Detail-equality gate does
   this per row).
5. RecordDispatchRematch emit/lower plumbing: mirror emitTrap
   (evTrap sibling or a new evRematch), terminal/diverging; lowering
   pushes the operands then OpDispatchRematch(specIdx).

Sequencing: land generics rows first (single event-carrier window, the
cleanest shape), then local-add (window bounding), apply/each, splice
last. Full battery + census + refusalGate ratchet per row; knownRefusals
rows delete via the stale arm as each compiles.

### Phase 3 progress — OpDispatchRematch LANDED, refusals 9 → 3 (2026-07-14)

The runtime-evaluated trap is in, on the implementation map's exact
insertion point. tryRecordUnmatchedDispatchTrap's screen loop now
CLASSIFIES the window: hard declines unchanged (dynamic / undefined /
reach / paren / interp-string / splice, in-window open paren, 0-arg
courtesy screen); zero carriers → today's definite serialised OpTrap;
≥1 carrier → the RUNTIME REMATCH record, under three byte-identity
guards — (1) the window must EQUAL the written tuple sigError renders
(rematchWritten, the carrier-aware twin of the forward-else-stack
derivation; known literals true/false/none resolve as the match's
forward walk resolved them), (2) the tape reorder probe must not apply,
(3) the fn-shape typed-binding hint must not apply. The record resolves
every window value to an operand (RecordDispatchRematchValues →
RecordDispatchRematch, an emitTrap variant sharing the trap's terminal
truncation), lowering seats the operands via layoutOperands (the
lowerUserCall discipline — the first draft's pushOperand of an event
operand baked Consts[seq] and the differential caught the wrong-value
window immediately) and emits OpDispatchRematch over
Program.Dispatches[{Word, NArgs, Pos}]. The VM arm rebuilds the window
(callPoly's layout), re-matches via MatchSignature over the word's LIVE
registry binding, and on NO MATCH raises runtimeNoMatch (diag_msg.go)
stamped at the recorded pos — byte-identical to the interpreter's
sigError, full-error compare pinned per row; on a MATCH (the static
model was wrong) defers via vm:rematch-matched — pinned by the
refined-return shape compiling, deferring, and producing the
interpreter's value.

Graduations: all six single-carrier-window knownRefusals rows (the
apply.tsv pair + four generics rows) — stale arms fired, entries
deleted; refusalGate ratcheted 9 → 3; census 5981 → 5987 fully-native
(refused 3). The M4-era carrier-hazard negatives graduated WITH their
soundness pin: the refinement-escape and predicate-param shapes now
compile and their guard moved from "must refuse" to "the rematch must
DEFER with parity" (TestUnmatchedDispatchTrapNegatives rematches
section, TestDispatchRematchMatchDefers). TestTrapKeepsPriorCallEffects
upgraded: the printing-body row now runs compiled — effects once, in
order, then the identical raise, no fallback duplicate.

Remaining 3 rows: local-add (match window 3 positions, written tuple 1 —
needs the DispatchErrCtx window-bound, plan 3a), the each variadic-if
row (0-arg courtesy screen — a different recovery path), the word-splice
row (deferred-expression screen). Then 3c: the vm:poly-no-match /
vm:user-poly-no-match defers can convert to faithful runtimeNoMatch
raises the same way the rematch's no-match arm does.

### do-rows idempotent type lowering — the exact conflict (2026-07-14 probe)

For `do [def Big Integer 15 is Big]`, tryRecordClosure's probe declines
with: `fn body analysis error in do$body: type: name part "Big" in "Big"
conflicts with an existing type name`. Mechanism confirmed: do's
ReturnsFn pass (RunCarrierBodyWithDefs) ran the body once — InstallType
minted "Big" into the KEPT state (Types.parts + lattice; Defs rolled
back) — and the closure compile's AnalyseFnBody re-runs the SAME def
site, whose InstallType now trips IsKnownPart (registry.go:1674 →
types.go:357). Top-level `def Big Integer 15 is Big` compiles fine
(prog=true): the single check-pass install is the only install and the
lowering resolves the mint (OpPushType/OpBindTyped).

Fix directions for the implementation session (choose one):
1. CHECK-MODE re-analysis idempotence in InstallType: a re-install of
   the same name at the SAME def site with a unifying body resolves to
   the existing mint instead of erroring — needs site provenance
   (SrcPos on the minted type or the parts entry) so genuine duplicate
   defs (`def Big Integer def Big Integer` — the interpreter errors)
   still mirror the runtime error. Distinguishing signal: the re-run of
   ONE source position vs two positions.
2. Sandbox the ReturnsFn body pass's TYPE state (parts + lattice) the
   way snapshotPredicateState does, now that the replay-hazard REFUSAL
   means no baked capitalized-def body survives to need the kept mint —
   the closure compile then mints fresh with no conflict, and the
   unit's kept mint serves OpPushType. Risk: other consumers of the
   mint between the ReturnsFn pass and the closure compile; audit
   compile_sandbox.go's keep-on-compile contract before choosing this.
Whichever lands, the frontier-do-registry-replay TSV rows and the
"code-body word (NoEvalArgs)" vary bucket graduate together, and the
unit's runtime InstallType execution must ALSO be idempotent against
the check-time mint (the unit re-runs the def per invocation — the
same OpPushType-resolution discipline, not a re-install).

### do-rows — the fix IS do-def leak fidelity (2026-07-14 follow-up probe)

Interpreter semantics established: **do-body defs LEAK to the enclosing
scope** (`do [def x 5] end x add 1` → 6; `do [def Big Integer] end 15
is Big` → true; a REPEATED fn-scoped do-typed-def conflicts in the
interpreter too — the fn cleanup pops the binding but not the part, a
pre-existing language asymmetry the compiled path must reproduce, not
fix). The check pass's RunCarrierBodyWithDefs rollback of do-body defs
is therefore an INFIDELITY: it produces the standing post-do
undefined_word-class diagnostics (both probe rows check with 1-2
diagnostics and fall back today — the "check diagnostics
(wrapped-context false positive)" vary bucket) AND the parts conflict
(with the binding rolled back, validateTypeName's Defs.IsType skip
does not fire, so the closure re-analysis trips IsKnownPart).

The aligned fix: the DO body's check-mode run keeps its def bindings
(matching the leak), scoped to `do` only — branch/loop/fn bodies stay
rolled back (conditionally executed). Consequences to verify in the
implementation session: (1) the closure re-analysis then shadows
(IsType true → parts check skipped) and the do rows compile as closure
units, with the unit's runtime def lowering registry-visible
(evDynBind/OpBindDynScope — the leak); (2) the post-do
false-positive diagnostics disappear — the checker-accuracy ratchets
(TestCheckAccuracyRatchet) and the vary bucket
"check diagnostics (wrapped-context false positive)" will MOVE and
must be re-baselined through their stale/drift arms, never hand-edited;
(3) do-defs visible to the post-do tail also changes what the RECORDER
sees for the tail (previously unresolved reads become bound) — the
census may move in both directions; run the full differential.

### Phase 3 tail progress — 3c part 1: the native-poly no-match raise (2026-07-14)

`vm:poly-no-match` converts to a FAITHFUL runtime raise for the record-gated
subset (PolyNoMatchSpec on PolyRef). Mechanism: sigError's diagnostic has
exactly two tape dependencies — the WRITTEN tuple its notes render
(forward-else-stack derivation) and the SECONDARY reorder-probe tuple (the
stack prefix); everything else (candidate verdicts, the value-based reorder
probe, suggestions) is a pure function of the word, its live table, and those
tuples (noMatchDiag / reorderHintFor). The recovery sites
(checkModeAssumeSig's three tryRecordPoly calls) run AT the failed-dispatch
tape state — the same state the runtime interpreter raises from — so the
record probes both tuples there (polyNoMatchProbe, snapshotted BEFORE the
operand-resolution loop's eval-map tape.Set) and resolves them onto the
operand window by Value.ID (mapTupleToWindow, the rematch's identity gate).
The VM's no-match arm rebuilds the tuples from the live window and raises the
byte-identical error (vm_poly_nomatch.go); no spec → the sound defer stands.

Gates. Record-time: the two tape-only diagnostic layers must not apply
(void-arg group, fn-shape typed-binding hint); both tuples must map onto the
window (the deeper-stack `9 1 x add` shape declines — the local-add lesson);
every other-arity overload must be excluded — NARROWER arity declines
outright (its runtime collection could match where the raise claims failure),
WIDER arity needs the structural reach bound below it (polyReachBound — an
over-estimate of collectable operands; the IsConcrete trap: markers carry
payloads, so the stop-set screens them explicitly). Runtime drift guards:
table length pinned (NSigs), no narrower-arity live sig, indices in-window.

This CLOSES a live user-facing bug: an effect before a poly no-match tripped
the C1 fence — fallback blocked, the user saw internal_error + "report this
as a compiler bug" instead of the signature_error (pinned by
TestPolyNoMatchAfterEffectRaisesCanonical: effects once, canonical raise,
compiled). Byte-identity battery: TestPolyNoMatchRaisesByteIdentical (the
three written-derivation modes: one forward token, full stack prefix, stack
fallback after a word breaks the forward walk).

Remaining for 3c: the vm:user-poly-no-match twin (matchUserPoly — same spec,
recorded at the RecordUserPolyCall site; needs the subset==full-table screen
on top), and the non-recovery poly record sites (carrier.go dispatch paths —
no Engine tape in scope; they pass nil and keep the defer).

### 3c part 2 (bounded): the fence-blocked alt (2026-07-14)

The FULL user-poly spec is a multi-session item: the record site
(core_helpers.go buildFnBodyReturnsFn -> RecordUserPolyCall) runs at a
SUCCESSFUL ambiguous dispatch with no Engine in scope, and the
failure-equivalent tape state exists only at the dispatch's FIRST
(pre-collection) entry — the park/splice/retry cycle re-enters the dispatch
with the args moved BEFORE the word, so a probe taken at the retry or at the
ReturnsFn would derive the WRONG written tuple (a spliced [x] where the
runtime interpreter renders "none were supplied"). Threading needs a
dispatch-identity latch that survives park/retry and nests LIFO across
same-name dispatches; mapped, not built.

What landed instead — the bounded sound half, fixing the SAME live bug for
user-polys (probe-confirmed: an effect before a user-poly no-match produced
internal_error + "report this as a compiler bug"): the two no-match defer
sites attach a best-effort rich raise (AqlError.DeferAlt via vmDeferAlt)
that lang's fenceBlockedFallback surfaces INSTEAD of the internal error when
the effect fence blocks the re-run. Soundness screen (bestEffortNoMatch):
every non-fallback live overload takes exactly the window's arity (no
other-arity collection, no 0-arg courtesy could succeed), and for user-polys
the recorded subset must COVER the live table's non-fallback arms (an
appended same-arity arm slips past the index drift guard). Best-effort by
design: Detail + candidate verdicts are canonical over the live values; the
rendered tuple is the full window, which may be wider than the interpreter's
tape-derived tuple. The OPEN-fallback arm is untouched (byte-identity by
re-running); ungated shapes (mixed-arity add) keep the honest internal error
(TestPolyNoMatchUngatedAfterEffectKeepsInternal pins the edge).

### L-JOIN pinpoint — the refusal is a JOIN-ID identity drift, not
### refinement instability (2026-07-14 probe)

The Phase 4 L-JOIN design ("converge-then-record": suspend recording through
refineRecursiveSummary's rounds, record once with the converged environment)
targets the WRONG layer as stated: the `fn call operand of unknown
provenance` refusal reproduces on the FIRST armed body run, where refinement
is already skipped (`armed` gate at the AnalyseFnBody call site). Probe
evidence on the lJoinRepro family:

- The failing RecordUserCall operand is the recursive call's IF-JOIN
  argument (`best2`, sig position 3 / the bisected twins' position 2): a
  fresh `S_…` join-minted carrier ID that was NEVER setProduced.
- The trigger is the JOIN SHAPE, not arity (the arity correlation in the
  first bisect round was an artifact of which OUTER argument was `none`):
  a SAME-PARENT arm join (`[7]` vs an Integer-typed hypothesis) compiles at
  every arity; an ALTERNATIVES-UNION join — a None arm (`[7]` vs a
  None-typed param) or distant cousins (`[7]` vs a String `pc`) — refuses
  at the recursive call. Forward vs stack call form is irrelevant.
- RECURSION is load-bearing for the in-body variant: the NON-recursive twin
  of the same union-join shape (`def b2 (if (n "e" get) [7] ["s"]) b2 g`
  inside a plain fn body) COMPILES — only the recursive fn's union-join
  operand loses provenance. Separately, a TOP-LEVEL twin
  (`def m {e:true} def b2 (if (m "e" get) [7] ["s"]) (b2 g)`) ALSO refuses
  with the same reason — likely a DIFFERENT gap (the concrete map folds the
  condition constant → if3ReturnsFn's LiteralCondValue arm, and/or the
  DISJUNCT operand routes the call through disjunctPartitionReturns, whose
  joinReturnRows mints appear in the traces) — triage the two shapes
  separately.
- if3ReturnsFn itself registers and returns ONE value (out := joined top;
  RecordBranch(Out: out) — no drift inside a single invocation). The drift
  is across RUNS: the failing RecordUserCall's args carry types from a
  DIFFERENT hypothesis than the outer call site (an Integer `key` in a
  program with no Integer key anywhere), so identify WHICH AnalyseFnBody
  run (memo key + armed state) records the failing call, and where ITS
  binding's join ID was minted. Next probe: tag AnalyseFnBody entries
  (key, armed, suspended) and instrument ALL joinCarriersInner exits (the
  union arm, the subtype-widen NewCarrier arms, JoinCarrierStacks' gradual
  flip) plus joinReturnRows — the same-parent collapse arm was the only one
  logged this round. Fix directions unchanged: (a) a produced-ID alias for
  the ReturnsFn's join value onto the branch event, or (b) one shared join
  value between the record and the binding.

Converge-then-record remains needed for the REFINEMENT half (non-armed
rounds feeding stale IDs into memoised summaries), but it lands after the
identity drift above is fixed — the drift owns the current refusal.

### L-JOIN LANDED — the per-alternative recording leak (2026-07-14)

The instrument round (AnalyseFnBody run tags + partition entry/site logs)
closed the diagnosis in one step: the "join-ID identity drift" was the
check-mode DISJUNCT DISTRIBUTION recording per-alternative. At a user-fn
dispatch over a strict-disjunct arg, carrierResults' partition arm
(disjunctPartitionReturns) enumerated per-alternative combos —
alternativeCarriers mints a FRESH-ID carrier copy per alternative — and ran
the fn's ReturnsFn per combo. Under an ARMED recording each combo run hit
RecordUserCall with the fresh-ID copies ("fn call operand of unknown
provenance") and would have compiled one unit per combo; the union-shape
correlation (None arm / distant cousins) was simply WHICH joins produce a
strict Disjunct that triggers distribution, and the recursion correlation
was WHICH dispatches route through carrierResults.

The fix (carrier.go): (1) disjunctPartitionReturns runs its combo transfer
loop SUSPENDED — the combos are type probes; diagnostics (partial_dispatch)
are not gated by suspension; (2) carrierResults' partition arm, for a USER
FN under an armed recording, falls THROUGH to the ordinary dispatch — one
recorded CALL_USER with the ORIGINAL (provenance-carrying) args — and
re-IDs the partition-joined carriers onto the recorded results, keeping the
per-alternative type precision; (3) the fall-through is gated by
disjunctCombosTakeSig: every combo must first-match the committed sig BY
Signature.Impl identity (Lookup mints fresh aggregates in check mode, so
pointer identity never holds) — a combo taking a SIBLING overload (a narrow
arm ahead of the committed wide one) keeps the refusal, because one baked
CALL_USER would miscompile that alternative. Multi-overload straddles were
probed to already route through the OpCallUserPoly machinery (sound by
runtime re-match; TestLJoinSiblingOverloadStaysSound pins both directions).

Graduations: p4/l-np-no-runtime-bail-after-join fired its stale arm and is
DELETED from the frontier ledger — both stages at once: the repro compiles
AND runs with ZERO runtime bails (the staged L-NP vm:dyn-scope-miss handoff
never fired; L-NP has no known repro now and waits for one). The "separate
top-level shape" from the pinpoint note was the SAME leak reached from the
top-level dispatch — graduated with the family
(TestLJoinTopLevelDisjunctCallCompiles). The earlier converge-then-record
design is retired unless a refinement-round instability repro appears.

### each variadic-if row — the "0-arg courtesy screen" claim is stale (2026-07-14 probe)

`each` carries FOUR 2-arg non-fallback sigs and NO 0-arg overload, so
tryRecordUnmatchedDispatchTrap's `n == 0` courtesy screen cannot be what
declines `def n 0 if (n eq 0) [99] [1 2] each [dup mul]` — the refusal
("unmatched dispatch recovered at each") comes from somewhere else. Probe
facts: BOTH const-condition directions raise the SAME
"cannot call `each`" signature_error on both engines (the then arm leaves a
lone 99, the else arm's [1 2] splices to two loose Integers; neither is the
collection each's sigs want), so the row is a terminal-raise candidate for
the rematch/trap machinery. Next: trace WHERE the each dispatch failure
recovers in check mode — a higher-order code-body word's failed dispatch may
take a recovery path that never reaches tryRecordUnmatchedDispatchTrap, or
the trap's window/written gates decline on the branch-event operand.
Correct the knownRefusals comment when the real mechanism is pinned.
