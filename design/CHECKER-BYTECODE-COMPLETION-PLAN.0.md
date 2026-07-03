# Checker + Bytecode Compiler — Completeness Review and Completion Plan

Status: **plan** (review verified against the live tree; plan not yet
started). Snapshot date **2026-07-03**, tree = `main` @ `060157b`
(post-PR #224 cross-module element typing). All gate numbers below were
measured by running the ratchet/differential suites on this tree, not
copied from older docs — refusal counts in earlier docs (0, 6, 19, 23,
24, 112…) are snapshots against a growing corpus and are superseded by
the live census here.

Companions: `CARRIER-STATIC-TYPECHECK-REPORT.10.md` (checker
architecture), `checker-precision-fronts.0.md` (the two designed
precision unlocks), `aql-bytecode-finish-line.0.md` +
`aql-bytecode-next-stages.0.md` (compiler stage sequencing),
`aql-bytecode-completion.0.md` (re-scoped P7),
`local-reasoning-for-global-properties-in-aql-report.0.md` (proposals 3,
4, 10 referenced below), `module-fn-checkstate-ownership.7.md` (the
module-body wall).

---

## Part 1 — Where the two subsystems actually stand

They are one system: the compiler is the carrier checker run with a
recording side effect (`CheckState.Emit`), so checker precision is
compiler coverage and checker soundness is compiler correctness. The
review is split only for presentation.

### 1.1 Type checker — live envelope (all four ratchet suites PASS)

| Metric | Pin / ceiling | Live measured | Note |
|---|---|---|---|
| False positives (spec corpus) | `pinnedFalsePositives = 0` | **0 / 3313** value rows | holding |
| Check-silent ERROR rows | `pinnedUnflaggedErrorRows = 172` | **172 / 485** | by policy never reaches 0 (value/state-dependent errors are the runtime's job) |
| Type-soundness violations | `pinnedTypeSoundnessViolations = 8` | **7 / 3306** | improved below pin — the test itself says lower the pin |
| Any-frontier count | 345 (informational) | **381 / 3306** | corpus growth |
| Any-frontier **ratio** | `anyFrontierRatioCeilingPct = 12` | **11.52 %** | **only ~0.48 pt headroom** |
| Check-rejected-but-runs (fuzz) | `pinnedCheckRunDivergent = 104` | **104** | all hand-verified dead-branch `case`/`if` |

What is genuinely landed and solid: single-intercept parity dispatch
(checker runs the real engine loop over carriers); the gradual
`dynamic(T)` modality with narrowing-through-use; strict-disjunct
per-alternative dispatch; guard narrowing for lattice/class types
(incl. paren forms and else-branch complements); branch join + bounded
Kleene loop/recursion fixpoints with assume-guarantee on declared
returns; the loud-diagnostics program (uncalled_function,
unreachable_signature, module-export propagation, `run --check`
preflight, LSP wiring); `--strict` with `dynamic_dispatch` infos;
`forward_strands_operand` at ~100 % measured precision; the severity
table with a completeness gate; and (PR #224) cross-module element
typing.

**Known gaps, in rough severity order** (each verified by live probe on
this tree):

- **G1 — predicate-refinement returns unverified statically.**
  `def Big (Integer gt 10)  def mkbad fn [[] [Big] [5]]  mkbad` checks
  clean (exit 0) and fails only at the runtime RET boundary.
  `checkBodyReturnConformance` flags only provable disjointness. The
  smart-constructor pattern — the reason subset types exist — is
  unchecked exactly where it matters. Also the source of soundness pin
  `user-types.tsv:124`.
- **G2 — guard-blind narrowing for refinement types; validate-then-call
  is a gating false positive.** `extractGuardClauses` skips any guard
  type carrying a predicate body, so `if (x is Big) [g x] [0]` with
  `x:Integer` is a gating `no_signature` (exit 1) while the program
  runs correctly. The FP = 0 pin is corpus-relative; this is a live
  off-corpus FP class, and the named blocker for check-by-default.
- **G3 — no redundant-guard / entailed-dead-branch diagnostics.**
  A provably dead `if (n is Big) …` with `n:Big` produces zero
  diagnostics; `unreachable_branch` fires only on literal `true/false`
  conditions. The checker computes the facts and says nothing.
- **G4 — typed code values (the `do` escape hatch).** `ops get 0 do`
  is `dynamic(Any)`; rated "Severe"; the dominant Any-frontier feeder
  (minilang 47, struct 32, parselang 31 rows…). Design complete
  (`checker-precision-fronts.0.md` §1, `CodeEffectInfo` /
  `Code[in→out]`), **no code**.
- **G5 — store-identity context typing.** `CheckState.ContextTypes` is
  one flat flow-insensitive map; store layering is flattened; same-name
  keys of different stores join. Design complete (§2,
  `StoreShapeInfo`), **no code**.
- **G6 — options maps through `Any` slots.** Words without a declared
  Options schema silently ignore atom-key typos at check *and* runtime
  (`emit json {prety:true}` — clean under `--strict`, wrong output).
- **G7 — `missing_returns` noise / incomplete native `Returns`
  coverage.** Unannotated native sigs warn per call site and feed the
  Any frontier; annotation errors have historically been a soundness
  source (six time-util fixes). No gate asserts full coverage.
- **G8 — residual soundness pins (7).** `forward-barrier.tsv:83`
  (needs concrete-condition folding), `recursion.tsv:53` (for-spread
  modeling), `class.tsv:85`, `corpus-core.tsv:61`, `module-rand.tsv:37`
  and `patrun.tsv:40` (dynamic-dispatch class, possibly irreducible),
  `user-types.tsv:124` (falls to G1).
- **G9 — emit/check entanglement.** `CheckState.Emit` remains a field;
  `emit.go` is 4.3 k lines riding the checker pass; the
  recorder-interface decoupling (comprehensive review Tier-2 item 6) is
  only partially done.
- **G10 — dead-branch attribution.** The 104 fuzz divergences are
  static-checking-by-design (an error in a never-taken arm gates the
  program) but are not labelled as such for users.

### 1.2 Bytecode compiler + VM — live gates (all PASS)

| Gate | Live result |
|---|---|
| Census (`TestCompiledCoverage`) | **3,875** spec value rows → **3,476 compile** (1 island), 248 check-error, **151 refused** → 95.8 % of the 3,627 valid rows produce a Program |
| Accepted-row differential | **3,296 rows, 0 mismatches** (floor 2,224) |
| Whole-corpus compile-or-fallback | 3,875 rows, 3,451 via compiled path, **0 divergences** (values *and* error taxonomy) |
| Re-scoped-P7 partition (`TestOnlyMetaFallsBack`, **enforced**) | tier-1 interpreter-only **0** (cap 3), tier-2 reducible **1** (`flex`; ceiling 1), error-row 83, compute gap **68** (ceiling 86) |
| Property fuzz | 2×1,500 programs, 1,955 compiled path, **0 divergences** |
| Corpus three-mode + combinations parity | green |
| `correct-error == 0` (the one hard coverage gate) | **0** — every known-error row compiles a trap, never silently refuses |
| `COMPILED_STATUS.md` | **stale** (says 3,524 rows / 112 refused; live is 3,875 / 151) |
| External voxgig force-compile corpus | **35 / 48 files** (per PR #224 message; sweep **not re-run** post-merge — "re-gate" outstanding) |

Refusal histogram (151 rows): dispatch recovery (best-guess) **81**,
operand provenance 22, function-value-reaches-word 12, typed-def
dynamic refinement reparent 12, fn-value-call boundary 9,
function-valued operand 5, user-fn call (Stage 3) 4, unconsumed
fn-value carrier 3, code-body word 1, dynamic input 1, paren-bounded
fn-value application 1. Root cause: **125 soundness, 26 coverage,
0 correct-error**. Refusal/island ceilings were deliberately demoted to
informational (`compiled_coverage_test.go:174-180`) because the corpus
grows; the only hard coverage assertion is correct-error = 0.

Fallback semantics: refusal and compiled-mode internal errors roll back
the registry snapshot and silently re-run the interpreter ("slow, not
wrong"); `--force-compile` turns refusal into a loud error. Compiled
mode is **opt-in** (`--compile` / `AQL_COMPILE`), off by default.

**Correctness debt (the part that is *wrong*, not just missing):**

- **M1 — const-pool identity aliasing (Mechanism A, open, deferred by
  design).** Fn-returned container literals can be const-baked such
  that `(mk) eq (mk)` is `true` compiled vs `false` interpreted.
  Narrowed to identity only (no mutation divergence), but it is a live
  compile≠interpret divergence — the one class that violates the hard
  invariant.
- **M2 — Mechanism E remainder.** `/r`-deferred map-field auto-invoke
  (`{f:make42/r}.f`) and nested-factory apply in the main residual
  (`(((mk 1) 2) 3)` leaks a Function value).
- **M3 — latent asymmetries flagged, unconfirmed.**
  `checkParamContract` vs `MatchSignature` guard drift; tail-call
  return checks using static `ConformsTo` instead of the runtime
  membership rule.
- (Closed: PARAM-GUARD-SKIP miscompile fixed with runtime param guards
  at `OpCallUser`; miscompile mechanisms B, C, D fixed as sound
  refusals.)

**The hard wall, named honestly:** sound module-body / cross-registry
compilation (`module-test.tsv:38` and everything shaped like it) is
really **Stage-3 user-fn-call inlining with concrete-argument
propagation** — very high risk, needs a corpus re-baseline, two
attempts already reverted (`module-fn-checkstate-ownership.7.md`
Step 3; `aql-bytecode-stage3-inlining-plan.0.md`). Every completion
path (voxgig 48/48, refusals → 0) runs through it eventually.

---

## Part 2 — Definition of "finished"

Drawn from the projects' own asymptotes; nothing here invents a new
goal.

**Checker done =**
1. FP = 0 held *and made corpus-independent*: the known off-corpus FP
   class (G2 validate-then-call) legalized.
2. Every literal-decidable error shape statically flagged — the named
   remainder is predicate returns (G1). The 172 unflagged rows shrink
   only by class, never chase individual value-dependent rows.
3. Soundness pins reduced to the irreducible dynamic-dispatch core
   (target: pin ≤ 4 of the current 7, each with a written
   irreducibility rationale).
4. Any-frontier **ratio ceiling lowered stepwise** (12 → 10 → 8 %) as
   the two designed precision fronts (G4, G5) land; `do` over genuinely
   runtime-constructed code stays dynamic forever, by design.
5. **Check-by-default**: `aql run` preflights with `--no-check` opt-out
   (gated on 1).

**Compiler done = re-scoped P7:**
1. `compile == interpret` (values + error taxonomy) with **zero known
   divergences** — i.e. M1/M2 fixed; this is currently violated only
   by M1/M2.
2. Tier-2 reducible → 0 (one row: `flex`); compute frontier 68 → 0;
   refusals 151 → 0 **on the live corpus**; islands tier-1-only
   (currently 1 non-tier-1 island).
3. Delete the unbounded whole-program fallback: `RunCompiled` becomes
   `Compile` + `RunProgram`; refusal surfaces as a compile error; a new
   gate asserts every `OpFallback` span classifies tier-1.
4. External acceptance: **48/48** voxgig files under `--force-compile`.
5. Ceilings re-armed: once refusals/islands hit 0, restore them from
   informational to **gating** ratchets (the demotion was a
   growth-phase policy, not the end state).

Out of scope of "finished" (recorded so nobody scope-creeps it in):
bytecode serialization / interpreter-free `aql build` executables (no
doc proposes it; `build` today embeds source), multi-shot islands
(reverted twice, wrong direction), and chasing the 172 unflagged rows
to zero (explicitly never-zero by policy).

---

## Part 3 — The plan

Ordering principle: **correctness before coverage, precision before
frontier burn-down** (the compiler rides the checker, so checker
precision fronts convert refusals for free — the 81-row
dispatch-recovery bucket is mostly checker imprecision seen from the
compiler side), and the one very-high-risk item (Stage 3 inlining) goes
last with everything easier landed first to shrink its blast radius.
Every item lands gate-clean-or-revert under the standing discipline:
`make fmt && make vet && make lint && make test`,
`make verify-bytecode`, ratchets move down only, `make status` refresh,
per-row landing tests, one commit per item with before/after deltas.

### Phase 0 — housekeeping (immediate; hours)

| # | Item | Acceptance |
|---|---|---|
| 0.1 | Lower `pinnedTypeSoundnessViolations` 8 → 7 (test output already instructs it) | ratchet locked at 7 |
| 0.2 | `make status` — regenerate stale `COMPILED_STATUS.md` to the 3,875-row census | `TestCompiledStatus` fresh |
| 0.3 | Re-run the voxgig force-compile sweep + full `make test` (the PR #224 "re-gate before merging" debt) | 35/48 confirmed (or corrected) and recorded |
| 0.4 | Any-frontier headroom decision: with 0.48 pt to the 12 % ceiling, the next corpus-heavy merge trips the gate. Policy: **land Phase 2.1/2.2 before accepting large new corpus batches**; do not raise the ceiling. | written into this doc; enforced by review |

### Phase 1 — correctness debt (compiler miscompiles + latent drift; ~days)

| # | Item | Files | Acceptance |
|---|---|---|---|
| 1.1 | **M1** const-pool identity aliasing: stop const-baking fn-returned container literals whose identity can be observed (bake-refuse under `eq`-reachability, or allocate per call). Prefer the sound-refusal shape used for B/C/D. | `eng/go/emit.go`, `bytecode_constbake_test.go` | the `(mk) eq (mk)` family differentially pinned; 0 known divergences |
| 1.2 | **M2** `/r`-deferred map-field auto-invoke + nested-factory apply residual | `eng/go/emit.go`, `carrier.go` | MISCOMPILE-HUNT rows all sound (compile correctly or refuse) |
| 1.3 | **M3** parity audits: `checkParamContract` ≡ `MatchSignature`; tail-call return check uses the runtime membership rule, not static `ConformsTo` | `eng/go/vm.go` | targeted differential tests for both seams |
| 1.4 | Soundness-pin burn-down: `forward-barrier.tsv:83` (concrete-condition folding), `recursion.tsv:53` (for-spread modeling); write irreducibility rationales for `patrun.tsv:40` / `module-rand.tsv:37` if they are the dynamic-dispatch core; triage `class.tsv:85`, `corpus-core.tsv:61` | `eng/go/carrier.go` | pin 7 → ≤ 4, each residual documented |

### Phase 2 — checker precision, tier 1: refinement types (~days; highest leverage per line)

| # | Item | Acceptance |
|---|---|---|
| 2.1 | **G1** — evaluate predicates on *concrete* carriers at return boundaries (machinery exists for parameters); `unverified_return` info under `--strict` for abstract carriers. Closes the `mkbad` hole; clears pin `user-types.tsv:124`. | probe: `mkbad` check exit 1; symmetric with parameter behaviour |
| 2.2 | **G2** — refinement-aware guard narrowing: `extractGuardClauses` accepts DepScalar-bodied types; then-branch narrows to the refined type, else-branch takes the complement where representable. Kills the validate-then-call FP class. | probe: `if (x is Big) [g x] [0]` checks clean; FP = 0 becomes corpus-independent; **unblocks Phase 5.2** |
| 2.3 | **G3** — `redundant_guard` + entailed `unreachable_branch` advisories (non-gating info): syntactic/interval entailment over DepScalarInfo bounds and carrier-vs-guard subsumption; no SMT. Also label the 104 fuzz dead-branch rejections with the same attribution (G10). | new severity-table codes + completeness gate; advisory precision measured like FORWARD-STRAND (≥ ~99 % on corpus sweep) |

### Phase 3 — compiler frontier, tranche 1 (independent of Phase 2; ~1–2 weeks)

Sequenced from `aql-bytecode-next-stages.0.md` (H, B, I already
landed):

| # | Item (stage) | Rows it moves | Risk |
|---|---|---|---|
| 3.1 | **Stage A** — variadic branch-result modeling (`stageA` doc is design-complete): arms may leave 0-or-N values and merge; unlocks conditional `RecordTrap` (nested/branch error rows), the computed-scrutinee `case` island, `fn m:` extra-values | the 1 island → 0; several error-rows; if-branch lowering | medium (the one structural lowering refactor) |
| 3.2 | **Stage D remainder** — dynamic-receiver `set`/`push`/`raise` (the stated voxgig bottleneck) | operand-provenance + voxgig files | medium |
| 3.3 | **Stage E** — VM reference cell for `flex` | the last tier-2 reducible row → 0 | medium |
| 3.4 | Error-row traps completion: `MathUtil!.nope`, minilang/parselang dynamic-source rows (needs 3.1's conditional traps) | error-row bucket shrinks; `correct-error` stays 0 | low |
| 3.5 | **Stage G remainder + fn-value frontier** — `fn-value.tsv:19`, method-through-map (`method-fnvalue-codebody` design), function-valued operands, unconsumed fn-value carriers | ~20 rows across 5 buckets | medium-high |

### Phase 4 — checker precision, tier 2: the two designed fronts (~2–4 weeks, staged)

| # | Item | Staging | Acceptance |
|---|---|---|---|
| 4.1 | **G4 — typed code values** (`CodeEffectInfo`, `Code[in→out]`): (a) literal producers + `do` consumer; (b) op-table `ChildTypeInfo` element-effect propagation; (c) higher-order consumers (`each`/`fold`). Compile pass **refuses** (never islands) `do` over an effect carrier until the VM grows an invocation path. | per `checker-precision-fronts.0.md` §1 | Any-ratio drops several points → **lower `anyFrontierRatioCeilingPct` 12 → 10**; dispatch-recovery refusal bucket shrinks measurably |
| 4.2 | **G5 — store-identity context typing** (`StoreShapeInfo`): abstract store carriers minted per creation site; ctx/set/get read/write the shape; PushContext copy-on-write layering; retire flat `ContextTypes` last | per §2, precision-only (unknown store → today's behaviour) | module-heavy frontier rows (test/log spans) drop; ceiling 10 → 8 where measured |
| 4.3 | **G7** — native `Returns`/`ReturnsFn` coverage gate + module-fn framework return types (recovers the +4 frontier rows from checkstate-ownership §6) | one sweep + a completeness test | `missing_returns` on the spec corpus → 0 |
| 4.4 | **G6** — options-schema lint: atom-keyed map literal flowing into an `Any` slot of a schema-less word → advisory; schema coverage sweep for the worst offenders (`emit`, …) | non-gating info first | probe `{prety:true}` diagnosed |
| 4.5 | **G9** — finish the emit/check decoupling (recorder interface; drop `CheckState.Emit`). Do this **before** Phase 6 — the module-body work is materially safer against a decoupled recorder. | comprehensive-review Tier-2 item 6 end state | `emit.go` consumes a narrow interface; no behaviour change (differential green) |

### Phase 5 — re-census + adoption flip

| # | Item | Acceptance |
|---|---|---|
| 5.1 | Re-census after Phases 2–4: the 81-row dispatch-recovery bucket and the 12-row typed-def-reparent bucket are expected to collapse substantially from checker precision alone; refresh `COMPILED_STATUS.md`; lower every informational count that moved | census recorded; remaining refusals re-bucketed for Phase 6 |
| 5.2 | **Check-by-default** (`aql run` preflights; `--no-check` opt-out) — now legal because 2.2 removed the known FP class | flip lands with release note; fuzz FP gate extended to the run path |
| 5.3 | Voxgig sweep re-run; ratchet the floor upward from 35/48 (must-not-drop becomes must-reach as Phase 3/6 items land) | recorded per-file with owning stage |

### Phase 6 — the hard wall: Stage 3 / Stage C (module bodies + user-fn inlining; the last big rock)

Sound cross-registry module-body compilation = user-fn-call inlining
with concrete-argument propagation. Two attempts reverted; treat as a
project, not a task:

1. **Design round first** — reconcile `aql-bytecode-stage3-inlining-plan.0.md`
   with `module-fn-checkstate-ownership.7.md`'s closing correction
   (closure-capture provenance is the root cause); decide the
   inlining boundary (per-call summaries vs body splice), the
   provenance model for captured carriers, and the corpus re-baseline
   protocol **before** code.
2. Land behind a flag with the full differential + `-race` +
   combinations matrix as the gate, on the smallest reproducer
   (`module-test.tsv:38`) first, then the fn-value/module buckets.
3. Only then sweep the remaining refusal buckets (operand-provenance
   cascades, dynamic-scope frames — Stage F) that cascade off it.

Exit: refusals → 0 and islands tier-1-only **on the live corpus**.

### Phase 7 — endgame (re-scoped P7 deletion)

1. Delete the unbounded whole-program fallback: `RunCompiled` →
   `Compile` + `RunProgram`; refusal is a compile error; new gate
   asserts every `OpFallback` span classifies tier-1.
2. Re-arm ceilings as **gating** at 0 (refusals, non-tier-1 islands);
   `correct-error == 0` stays.
3. Perf/alloc re-baseline; flip `--compile` default on (interpreter
   remains behind `AQL_NO_COMPILE` until a full release cycle passes).
4. Record the end state in a `.10` doc; open a *separate* decision doc
   if bytecode serialization for `aql build` is ever wanted — it is not
   part of this plan.

### Dependency sketch

```
0.1–0.4 ─┬─ 1.1–1.4 ──────────────┐
         ├─ 2.1 ─ 2.2 ─ 2.3       ├─ 5.1 ─ 5.2/5.3 ─ 6 ─ 7
         └─ 3.1 ─ 3.2/3.3/3.4/3.5 ┘
              4.1 ─ 4.2 ─ 4.3/4.4 ─ 4.5 (before 6)
```

Phases 1, 2, 3 are parallelizable across sessions; 4 follows 2; 5 needs
2+4 (ratio drops) and 3 (island/tier-2 zeros); 6 needs 4.5 and benefits
from everything; 7 is gated on 6's exit criteria.

### Risks

- **Corpus growth vs finish line.** Every new spec row is a compiler
  test; the frontier drifts up while the plan burns it down. Policy:
  finish lines are defined against the live corpus at each phase's
  re-census; the informational-ceiling policy stays until Phase 7
  re-arms them. Phase 0.4 protects the one gate that can trip
  spuriously (Any ratio).
- **Stage 3 is the schedule.** Phases 0–5 are well-understood,
  gate-protected work. Phase 6 has already eaten two reverts; the
  design-round-first structure and the 4.5 decoupling are the
  mitigation. If 6 stalls, everything else still lands and the system
  remains "slow, not wrong" — the fallback doctrine is the safety net.
- **Checker/compiler coupling cuts both ways.** Precision fronts
  (Phase 4) move refusal buckets for free, but any carrier change can
  move compiled behaviour; every Phase 2/4 item must run
  `make verify-bytecode`, not just the checker ratchets.

---

## Execution log

- **2026-07-03 — Phase 0 executed.**
  - 0.1 `pinnedTypeSoundnessViolations` lowered 8 → 7 (generics.tsv:75
    left the violation set; the remaining 7 verified live:
    class.tsv:85, corpus-core.tsv:61, forward-barrier.tsv:83,
    module-rand.tsv:37, patrun.tsv:40, recursion.tsv:53,
    user-types.tsv:124).
  - 0.2 `make status` — `COMPILED_STATUS.md` regenerated to the live
    3,875-row census (3,475 native + 1 island, 151 refused, 248
    check-error).
  - 0.3 **voxgig sweep: blocked-external in this environment** — the
    client corpus is a separate repository not present here; the
    35/48 figure from PR #224's message remains unverified. The
    `make test` half of the re-gate debt is covered (full suite green
    on this tree). The sweep re-run stays owed at the next
    environment that carries the corpus (also gates Phase 5.3).
  - 0.4 headroom policy in force as written (no ceiling raise; Phase
    2.1/2.2 before large corpus batches).
- **2026-07-03 — Phase 1.1 (M1) landed** (`fix(compile): per-call
  identity for fn-body container literals`): OpPushConstFresh +
  ID-pooled compound interning + escaping-multi-read refusal; probe
  matrix 5 divergent shapes → parity, sharing/deq preserved; census
  unchanged (zero coverage cost); VERIFY PASSED + full suite green.
- **2026-07-03 — Phase 1.2 (M2) landed**: both Mechanism-E remainders
  now refuse soundly. Deferred-field auto-invoke: get-family reads
  from containers holding a GENUINELY 0-param fn member refuse at both
  record sites (receiver-keyed; concrete keys inspect only the read
  member; the parked phantom 0-arg sig is discounted, so applied
  member calls `m.b 2` and multi-param member reads keep compiling —
  pinned by TestEmitFnValueFieldCallCompiles). Nested-factory curried
  chain: statically-recovered closure arity vs tail-arg count refuses
  the chain, single-apply factories keep compiling. Census cost:
  2 rows (3474/153 vs 3476/151), both previously divergence-exposed.
  Landing test TestFnValueAutoApplyRefusals (4 refusals with
  fallback-parity + 4 preserved).
- **2026-07-03 — Phase 1.4a landed: comparison concrete folding
  (CompileScalarFold).** eq/neq/deq/cmp/tcmp/lt/lte/gt/gte over
  compile-time-known scalars now fold to their CONCRETE result in
  check mode (tryFoldScalarConst — double-eval agreement guard like
  the module fold; check-mode literals ride as concrete-payload
  carriers, admitted by scalarFoldOperand). A/B-verified: `5 eq 0`
  was a bare Boolean carrier before, concrete `false` after. This is
  the named PREREQUISITE for the forward-barrier.tsv:83 pin
  ("needs comparison ops to fold concretes"); the pin itself stays
  until the `if` analysis COMMITS statically-determined branches —
  probe shows even `if false [1] [2]` joins both arms today — which
  belongs with Phase 3.1 (Stage A branch-result modeling). All
  ratchets hold (FP 0, unflagged 172, soundness 7, frontier 381,
  fuzz FP 104); census cost 4 rows, all one new bucket
  ("if-branch lowering [scheduling]" — branch lowering meeting const
  conds where it expected events), reclaimed by Phase 3.1. Landing
  test TestScalarConstFold (5 folds + param-carrier and
  cross-family negatives). Soundness pin stays 7: recursion.tsv:53
  needs recursion unrolling (Phase 3/6 territory);
  patrun.tsv:40 + module-rand.tsv:37 are the irreducible
  dynamic-dispatch core (a Patrun/seeded-instance member's static
  type is unknowable by design); class.tsv:85 (const-singleton field
  widened by set) and corpus-core.tsv:61 (enum identity) remain
  triage candidates for Phase 2/4.
- **2026-07-03 — Phase 2.1 landed: static return-value membership.**
  `checkBodyReturnConformance` now asks `got.Is(exp)` for a
  compile-time-known SCALAR body residual — the mkbad predicate-return
  hole is closed statically (probe: check exit 1 with the
  byte-identical runtime type_error) and the nominal newtype-return
  hole with it; reparented (`def x:UserId 42  x`) and good-predicate
  bodies stay clean; abstract/value-dependent residuals keep the
  runtime RET check by design. pinnedUnflaggedErrorRows lowered
  172 → 170. ALSO fix-forward for 31913df: the scalar fold now KEEPS
  recording the dispatch (`folded.ID = out[0].ID`) instead of eliding
  the event — event elision stranded every lowering that anchors on a
  condition event (emit goldens, computed-arm if, variadic-else
  claims; the 31913df battery was red). With recording preserved all
  pinned lowering tests pass, the 4-row census dip reversed, and the
  2 newly-flagged rows reclassified compiled → check-error
  (3472 compiled / 153 refused / 250 check-errors).
- **2026-07-03 — Phase 2.2 + 2.3 landed: predicate guard narrowing +
  redundant_guard.** extractGuardClauses now admits DepScalar-bodied
  guard types, narrowing to the MINTED lattice node
  (DefEntry.TypeDef; the constraint value's Parent is only the base
  family), whose depScalarUnifier admits tag-conforming abstract
  carriers nominally. Validate-then-call (`if (x is Big) [g x] [0]`,
  paren AND list guard forms) checks clean — the off-corpus FP class
  and named check-by-default blocker is legalized; unguarded and
  else-branch misuse still gate; runtime parity verified both
  directions. ApplyGuardNarrowing additionally emits the non-gating
  `redundant_guard` info when the guarded binding is a NON-CONCRETE
  strict carrier whose tag already entails the guard (`if (n is Big)`
  with n:Big) — concrete per-shape bindings and dynamic bindings stay
  quiet (a dynamic guard discharges the modality; value-level interval
  entailment is future work). New severity-table entry (Info tier).
  All pins hold (FP 0, unflagged 170, soundness 7, frontier 381, fuzz
  104); census/differential/partition unchanged-green; full lang and
  eng suites ok. Landing test TestPredicateGuardNarrowing (2 clean
  forms, 2 gating negatives, runtime parity, advisory positive + 2
  advisory negatives).
- **2026-07-03 — Phase 3.1 (Stage A) verified ALREADY LANDED; gate
  re-run green; one soundness negative pinned.** The stageA design doc
  and the completeness audit were stale: commit 31f02f3 (2026-06-28,
  ancestor of this branch) implemented variadic branch-result modeling
  — resolveArm + fragMulti multi-value arm accounting, allowVariadic
  fragment propagation, the variadic CALL_USER sentinel with a seeded
  FnSummaries hypothesis for recursive convergence, and
  seatResults/reconcileResults tail-position acceptance (VM unchanged,
  per the doc's own Correction 1). recursion.tsv:53 compiles natively
  and RunCompiled == Run == [6 4 2]; the m 0/1/3/5 convergence and the
  soundness gate are pinned by TestStageAVariadicBranchResult /
  TestStageAVariadicSoundnessGate, to which the missing paren-form
  negative `add (m 3) 1` was added (the only diff). Full battery
  green: VERIFY PASSED, make test exit 0, differential 0 divergences,
  census 3472/153/250 with the "branch leaves extra values" and
  "if-branch lowering [scheduling]" buckets both at 0. Residual noted:
  `def p fn [[n:Integer] [Integer Integer] [n n add 1]]` refuses as
  "result above a literal (Stage 3)" — a Stage-3 seating gap, tracked
  under Phase 3.5/6. recursion.tsv:53 stays among the 7 soundness pins
  as a checker-PRECISION straggler (static spread modeling), distinct
  from the compile-coverage gap Stage A closed.
- **2026-07-03 — Phase 1.3 (M3) verified sound, no change needed.**
  (a) `checkParamContract` already routes through `sigTypeMatches` —
  the interpreter's own runtime param match (deliberately NOT `v.Is`,
  which over-raises on Options/structural params) — plus
  `OpenUnifyMap`/`Unify` for ParamPatterns; the flagged asymmetry was
  closed by the param-guard fix. (b) RET enforcement uses `v.Is` (the
  membership rule); tail marking's static `ConformsTo` in
  `tailCompatibleReturns` is sound because callee→caller conformance
  is checked in the subset direction and nested predicate refines are
  CONJUNCTIVE — probe: `3 is (Big lt 5)` → false (Big's bound still
  applies) — so a lattice child's membership is always a subset of
  its ancestor's, and the eliminated caller check is implied by the
  callee's own RET check.
