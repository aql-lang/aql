# P7 Endgame — record (.10)

Status: **landed** (2026-07-04, branch
`claude/aql-local-reasoning-design-rb7elj`). The closing record of the
checker/bytecode completion program executed against
[`CHECKER-BYTECODE-COMPLETION-PLAN.0.md`](CHECKER-BYTECODE-COMPLETION-PLAN.0.md)
(whose execution log carries the per-landing detail) and the Phase-6
staged sweep specified by
[`STAGE3-INLINING-DESIGN-ROUND.0.md`](STAGE3-INLINING-DESIGN-ROUND.0.md).

## What the program achieved (start → finish, one session-week)

| Metric | Start (main @ 060157b) | Finish |
|---|---:|---:|
| Whole-program refusals | 151 | **11** |
| Native rows / islands | 3,475 / 1 | **3,613 / 1** (99.7 % of compilable) |
| Error-row fallbacks | 83 | **3** (each sound-by-proof) |
| Tier-1 / tier-2 partitions | 0 / 1 | **0 / 0** |
| Known miscompiles | mechanisms A + E open | **0** |
| Checker Any-frontier | 381 rows / 11.52 % | **195 / 5.90 %** (ceiling 12 → 7) |
| `missing_returns` | 87 | **0** (registry-wide gate, empty allowlist) |
| Soundness pins | 8 | **7** (each with a rationale) |
| Check adoption | opt-in `--check` | **check-by-default** (quiet gate) |
| Compiled-mode adoption | opt-in `--compile` | **compiled-by-default** (this record) |

Throughout: false positives 0/3313, differential and
compile-or-fallback **0 divergences** at every landing, `VERIFY
PASSED` (including `-race`, combinations, property-fuzz, and
`aqldebug` lanes) and full `make test` before every commit.

> **Addendum (2026-07-04, maintainer direction):** the default flip in
> action 1 was landed and then **reverted to opt-in** — compiled mode
> is OFF by default, controlled by the checker-style flag family
> `--compile` / `--force-compile` / `--no-compile` (+ `AQL_COMPILE` /
> `AQL_FORCE_COMPILE` / `AQL_NO_COMPILE`, the `--no` twin winning over
> everything). The safety case below still holds and the flip remains
> a one-line change whenever the maintainer chooses to take it; the
> other two endgame actions (the gated frontier, the standing
> perf/alloc baseline) are unaffected.

## The three endgame actions

1. **Compiled mode is the default.** `ResolveCompileMode` returns
   `CompileTry` with no flags — the Stage-7 flip the rollout contract
   reserved (`AQL_NO_COMPILE` is the kill switch; `--force-compile`
   still upgrades refusal to a loud error; the legacy `--compile` /
   `AQL_COMPILE` opt-ins are accepted no-ops). Safety case: the
   fallback is silent and sound ("slow, not wrong"), and the
   differential gates hold byte-identical values *and* error taxonomy
   across the 3,875-row corpus, the combination matrix, and the fuzz
   lanes.
2. **The coverage frontier is gated again.** The growth-phase
   informational policy on refusals/islands is over:
   `TestCompiledCoverage` now gates at the documented-tier floor
   (`refusalGate = 11`, `islandGate = 1`). A new refusal must be
   classified into a named tier or fixed — never drift in silently.
   The gate only moves down as tiers close; up only with a new named
   tier recorded here.
3. **Perf/alloc baseline stands as gated.** The alloc ceilings inside
   `make verify-bytecode` ran green at every landing; the compiled
   default makes them the de-facto runtime baseline. No re-baseline
   was needed — no landing regressed them.

## The remaining 11 + 1, by tier (the honest residue)

| Tier | Rows | Disposition |
|---|---|---|
| Container auto-dispatch guard (miscompile-E) | module-log 72/73, module-rand 14/15 | **Sound refusals.** Compiling them requires modeling the interpreter's landing-auto-dispatch semantics; the guard must never be weakened for coverage. |
| G5 flex path-shape typing | flex 88/95 | Named non-Phase-6 track (StoreShapeInfo stage 3+). |
| M6 dynamic-scope tier | recursion 71/72 | Maintainer tiering decision — dynamic-scope frames are the one semantics the unit model cannot honestly claim. |
| Sound non-definite error rows | convert-ideal 30, forward-barrier 80, word-splice 115 | The entire error allowlist: each has a written feasibility proof that the dispatch may succeed at runtime. |
| Compute-frontier island | error.tsv 25 | The single `OpFallback` span. |

**The unbounded whole-program fallback in `RunCompiled` stays**, by
design: with 11 documented-tier refusals live it is the mechanism
that keeps them sound, and its deletion (`Compile` + `RunProgram`,
refusal = compile error) is gated on the tiers above reaching zero.
That deletion is the one remaining P7 line item, owned by the tier
owners — not a hidden default.

## Out of scope, recorded

The voxgig force-compile sweep (M5) remains blocked-external — the
client corpus is not in this environment; the 35/48 figure from PR
#224 is the last measured value and the sweep re-run is owed at the
next environment that carries it. Bytecode serialization for
`aql build` remains explicitly unproposed.
