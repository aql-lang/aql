# AQL Bytecode — Finish-Line Plan (to re-scoped P7)

Status: design + live tracker. Companion to `aql-bytecode-completion.0.md`
(the cluster-by-cluster roadmap) and `aql-bytecode-runtime-independence.0.md`
(the P5–P7 machinery). This doc reconciles those with the **measured live
state** and sequences the remaining work to the goal.

## Goal (re-scoped P7)

Runtime independence: every supported program runs entirely in the VM, so the
interpreter dependencies can be retired. Per `completion.0.md` §3/§5 the target
is **re-scoped**, not literal:

- Keep the `OpFallback` island machinery, but **confined to tier-1** words that
  execute runtime-computed code (`Vm.run`). That category is genuinely
  irreducible (there is no ahead-of-time instruction sequence for code computed
  at runtime).
- Delete only the **unbounded whole-program fallback** in `RunCompiled` once the
  reducible + compute ratchets reach 0, so the island is provably confined to
  tier-1 spans.
- **Error rows compile to error programs**: a spec ERROR row that the checker is
  lenient about must lower to an `OpTrap`/`RAISE` program raising the
  byte-identical taxonomy, so every row yields a `Program`.

## Live state

> **Update (2026-06): refusalCeiling is now 6, islandCeiling 0.** The numbers in
> the table below are the ORIGINAL baseline; the authoritative live state is
> `test/go/langspec/COMPILED_STATUS.md` (`make status`). Current refusals (6):
> `def-node-binding.tsv:54` (fn-body `[[c1]]` list — verified hazard),
> `macro.tsv:45` (divergent macro — correct-error, Stage I),
> `module-parselang.tsv:23` + `module-rand.tsv:38` + `module-test.tsv:38` (the
> cross-registry module-body project, Stage C), and `recursion.tsv:72` (dynamic
> scope, Stage F). Most recently `bytecode-combinations.tsv:74` (returned
> capturing closure + per-iteration dynamic apply) landed (Stage G), 7 → 6 —
> see `aql-bytecode-next-stages.0.md` Stage G.

Measured by `go test ./test/go/langspec -run TestCompiledCoverage|TestOnlyMetaFallsBack`
and generated into `test/go/langspec/COMPILED_STATUS.md` (`make status`).

| ratchet (const, file) | current | finish |
| --- | ---: | --- |
| `refusalCeiling` (compiled_coverage_test.go) | 24 | → 0 |
| `islandCeiling` (compiled_coverage_test.go) | 2 | → 0 (or tier-1 only) |
| `interpreterOnlyCeiling` (compiled_metafallback_test.go) | 0 used / cap 3 | capped (permanent) |
| `reducibleCeiling` (compiled_metafallback_test.go) | 3 | → 0 |
| `computeRefusalCeiling` (compiled_metafallback_test.go) | 18 used / 86 | → 0 |

Of the 26 not-fully-native rows: **0** tier-1, **3** tier-2 reducible, **5**
error-rows, **18** compute-gap (one of which is the 2nd island).

## Remaining rows (the concrete inventory)

The full enumeration of every non-native row (refused/islanded), as of this
writing — the work list the clusters below address:

- **Reducible (3):** `quote` (`macro.tsv` macroexpand-with-quote residual),
  `word` (`word-splice.tsv` splice whose inlined body reaches an
  un-compilable word), `Test/Assert` (`module-test.tsv` run-spec residual).
- **Error rows (5):** `MathUtil!.nope` (module-instance, not_found),
  `+re/[a-z]+/` & `mini re 'a'` (module-minilang), `parse calc 'x'`
  (module-parselang), and the case-island error row
  `case [f 1] [...]` where `f` returns 0 values.
- **Compute gap (18):** operand-provenance cascades (7 — class typed-instance
  field defaults, user-fn-result-in-map, factory closures, rand/struct module
  receivers), dynamic input (4 — flex mutation, IO context set, struct getpath,
  reach setpath), if-branch lowering (1), fn-value-call boundary (1 —
  `m.f` method-through-map), function-value-reaches-word (1 — patrun dispatch),
  dispatch recovery (1 — `(3 and "x") add 1`), residual lowering Stage-1 (1 —
  parselang register), the `fn m:` extra-values branch (1), and the computed-
  scrutinee `case [1 add 1] [...]` island (1).

## Clusters (sequenced by leverage ÷ risk)

Each lands gate-clean (see Verification), lowers its ceiling monotonically, and
is committed with before/after numbers. Gate-clean-or-revert.

1. **Error-row trap programs.** ✅ **PARTIALLY LANDED (26 → 24).** The two
   `illegal_ref` rows (`def x 5  x/r`, `def x 5  x/u` — a ref-family modifier
   on a non-fn binding) now compile to a terminal `OpTrap` raising the
   byte-identical illegal_ref, via a top-level `RecordTrap` at the two
   `!IsFunctionRef(v)` check-active branches (`eng/go/engine.go` stepWord `/r`
   + stepWordUsurp `/u`). The x/u row also left tier-2 usurp (reducible 4 → 3).
   **Remaining (4 error rows):** `MathUtil!.nope` (not_found), the
   minilang/parselang rows (a dynamic-source parse the checker is lenient on),
   and the `case [f 1]` 0-value-scrutinee island — each needs its own
   compile-time error detection + `RecordTrap`. `RecordTrap` only fires
   top-level today; a nested error row needs the conditional-trap modeling that
   also unblocks branch error rows.

2. **Tier-2 reducible (3 → 0).** `quote`/`word`/`Test/Assert` residuals — each
   a named residual of an already-mostly-compiled feature. Files:
   `eng/go/carrier.go`, `eng/go/emit.go`, `lang/go/modules/test.go`.

3. **Operand-provenance cascade (7, soundness).** Dominated by `make` class
   typed-instance field defaults (`completion.0.md` item 3, deferred — needs a
   "suppress event recording inside a const-baked schema construction"
   primitive); the rest cascade when their producer compiles. Files:
   `eng/go/emit.go`, `eng/go/carrier.go`.

4. **Dynamic-input (4, soundness).** flex reference cells, IO-context set
   receiver, struct getpath, reach setpath — poly-extension to non-core safe
   builtins + a VM value-model reference cell for flex. Files:
   `eng/go/carrier.go`, `eng/go/vm.go`.

5. **Stage-2 branch-result lowering (4+1+1).** The variadic-merge /
   branch-result-modeling refactor so an arm can leave 0-or-N values and merge
   (computed-else, `fn m:` extra values). The one structural refactor. Files:
   `eng/go/lower.go`, `eng/go/emit.go`, `eng/go/bytecode.go`/`vm.go`.

6. **Fn-value frontier (3).** `m.f` method-through-map call boundary, patrun
   dispatch, dispatch-recovery — follow-ons to the apply/introspection work.
   Files: `eng/go/emit.go`, `eng/go/carrier.go`.

7. **Code-body-scrutinee `case` islands (2 → islands 0).** `case [1 add 1] […]`
   — inline scrutinee recording seated as a value-def local across the
   desugared if-chain. Files: `eng/go/emit.go`, `eng/go/lower.go`,
   `lang/go/native/native_control.go`.

8. **Re-scoped P7 deletion** — gated on 1–7 reaching 0. `RunCompiled` becomes
   `Compile` + `RunProgram` (a refusal surfaces as a compile error); the
   `OpFallback` island stays but a new gate asserts every span classifies
   tier-1; re-baseline perf/alloc.

## Verification (per item)

1. `make fmt && make vet && make lint && make test`.
2. `make verify-bytecode` — differential + whole-corpus compile-or-fallback
   (0 value + taxonomy divergences) + combinations + property fuzz + `-race` +
   alloc ceilings + `-tags aqldebug`.
3. Ratchets only move DOWN (lower the ceiling const with a one-line rationale);
   `correct-error` stays 0.
4. `make status` to refresh `COMPILED_STATUS.md` (`TestCompiledStatus` fails if
   stale).
5. A per-row landing test in `lang/go/bytecode_findings_test.go` (positive +
   negative).
6. Commit each landed item separately with its before/after ratchet delta.
