# The final two refusals — decision note (compile-to-trap vs keep-refusing)

_Status: macro:45 now DONE (refusals 2 → **1**); def-node-binding:54 remains.
After Stage-3 cleared the three module feature rows (5 → 2), `macro.tsv:45` was
landed via compile-to-trap once its true prerequisite was found (see "Row 1"
update). This note records both rows, corrects two earlier diagnoses, and lays
out the remaining option. The only remaining refusal is def-node-binding:54._

## UPDATE — macro:45 LANDED (refusals 2 → 1)

The Option-A prerequisite was found and fixed, and the trap landed (commit
"Stage 3: compile macro:45 recursive-macroexpand to a terminal OpTrap").
**The earlier diagnosis in this note was WRONG twice over** — recorded here so
the mistake isn't repeated:
- It was NOT a recursive-macro paren-capture / FormArgs problem. The
  macro-headed paren `(loopy 1)` IS captured raw and `macroexpand` DOES dispatch.
- The REAL blocker: the dynamic-help example generator (`native_help.go`
  `makeDynamicEval`, fired from `OnRegisterHook` on `def loopy`) runs the
  recursive `loopy` to the step ceiling during compile. `CheckState.StepCount` is
  registry-SHARED and `BudgetTripped` (engine.go) short-circuits every later
  sub-engine — so `macroexpand (loopy 1)` was never reached and the row refused
  on a stale residual. The recursion ran via `execMacro`'s splice path, never the
  depth-guarded `ExpandMacroForm`.
- Fix: `CheckState.IsolateBudget()` (eng/go/check.go) — the 4th hermetic channel
  for the help-eval (beside IsolateEmit / TruncateDiagnostics / def-snapshot),
  snapshot+restore of StepCount/BudgetTripped around the synthetic run. This is a
  genuine latent-bug fix: ANY doc example that runs many steps could otherwise
  burn the real program's shared budget. Then the trap at carrier.go's
  macroexpand branch (`RecordTrap("macroexpand_error", …)` top-level only, via
  `errors.As`) — mirroring the mini/parse/emit `*_unknown_lang` traps.
- Verified: compiles to a single terminal `OpTrap`, byte-identical error parity
  (`[aql/macroexpand_error]: macroexpand: expansion too deep (recursive macro?)`),
  DETERMINISTIC (20× in-process + fresh processes identical), full corpus
  differential clean, whole macro suite green, refusalCeiling 2 → 1.

The determinism observation noted earlier (time-seeded value-ID RNG under deep
expansion) turned out NOT to affect the trap — the fix is deterministic.

## Why this matters

`refusalCeiling` (test/go/langspec/compiled_coverage_test.go) is the gate for
plan **P7**: the interpreter fallback (and the OpFallback island) can only be
DELETED once refusals reach **0**. So these two rows are the entire remaining
distance between "compiler is interpreter-independent for all soundly-compilable
rows" (true today) and "the interpreter dependency can be removed" (needs 0).

Both rows **pass today** — they refuse to compile and fall back to the
interpreter, which produces the correct result. Neither ships a wrong answer.

## Correction to an earlier claim

Earlier status notes (and an earlier version of
`design/aql-bytecode-stage3-inlining-plan.0.md`) described these two as
"correct-by-design refusals that must NOT compile — compiling them ships wrong
answers, proven by the interpreter." **That is inaccurate.** The interpreter
proves the CORRECT results (`[1]` and the depth-limit error); it does not prove
that compilation must be wrong. The accurate statement: both are **conservative
refusals** — the static analysis cannot yet prove the result, so it declines and
falls back. A SOUND compilation exists for each; the open question is whether the
additional compiler work is worth it for these two edge cases.

## Row 1 — `macro.tsv:45` (an ERROR row)

```
def loopy (macro [[a] [quote [loopy unquote a]]])  macroexpand (loopy 1)
```
- **Expected:** `ERROR:expansion too deep`.
- **Interpreter:** raises `[aql/macroexpand_error]: macroexpand: expansion too
  deep (recursive macro?)` — the recursive macro is caught by the depth guard,
  not looped forever.
- **Live compile result:** REFUSES, reason `residual value not statically
  materialisable`. Falls back; the interpreter raises the error. Row passes.
- **Why it refuses:** the lenient check pass does not run the recursive
  expansion to the depth limit, so it produces a residual it cannot lower.

### Option A (compile-to-trap) — ATTEMPTED, has an unanticipated PREREQUISITE
The plan was: emit a terminal `OpTrap` raising the byte-identical
`macroexpand_error`, mirroring the sibling expansion-time traps (`mini`/`parse`
`*_unknown_lang` in native_macro.go, `illegal_ref`, getr `not_found`). The trap
site is right in principle — `eng/go/carrier.go` `carrierResults` runs
`ExpandMacroForm` during the check pass, the depth guard at
`eng/go/macro_expand.go:411-413` raises a bare `macroexpand_error`, and an `else`
branch there could `RecordTrap` it.

**A build attempt (worktree, reverted clean) found this does NOT work yet.** The
recursive row's `macroexpand (loopy 1)` does **not reliably dispatch
`macroexpand` under check mode** in the full corpus census: the `macroexpand`
word and its raw-captured `(loopy 1)` paren (head = the self-recursive macro) are
left UNCONSUMED on the residual and refuse with "residual value not statically
materialisable" BEFORE the trap site is ever reached (instrumented: zero hits in
`carrierResults` for this row in the gate). So a trap there never fires; coverage
stayed at 2. The blocker is in the engine's FormArgs paren-capture of a paren
whose head is a self-recursive macro, not in the trap pattern itself.

PREREQUISITE for Option A: make `macroexpand (<recursive-macro> …)` dispatch
deterministically under check mode (so it reaches `carrierResults` and the trap).
That is deep engine work on recursive-macro paren capture, it risks the
interpreter's macro dispatch path, and — see the determinism note below — it
interacts with global mutable state. Not the "low-risk" change first assumed.

### Option B (keep refusing) — zero cost, faithful
Leave it. The fallback already raises the exact error. The only thing lost is
P7 (cannot delete the interpreter while any refusal remains).

## Row 2 — `def-node-binding.tsv:54` (a VALUE row)

```
def c1 1  def mk fn [[c1:Integer] [List] [[c1]]]  mk 9
```
- **Expected:** `[1]`.
- **Interpreter:** returns `[1]` — NOT `[9]`. The subtlety: the body returns the
  list `[[c1]]`; its inner `[c1]` is auto-evaluated AFTER `mk` returns (deferred
  list evaluation at end of run), in MODULE scope, where `c1` is the module
  binding `1` — the param `c1 = 9` is already out of scope. So the returned
  list resolves `c1` to the module binding, not the param.
- **Live compile result:** REFUSES, reason `fn mk: body result of unknown
  provenance`. Falls back; the interpreter returns `[1]`. Row passes.
- **Why it refuses:** the compiler cannot statically prove which `c1` the
  deferred-evaluated returned list binds, so it conservatively declines rather
  than risk baking the WRONG one (`[9]`).

### Option A (compile correctly) — ATTEMPTED, blocked on a missing VM feature
A build attempt (worktree, reverted clean) confirmed a sound compilation does NOT
fit without a substantial new VM feature. Precise root cause:
- **Interpreter:** splices mk's body INLINE (`eng/go/core_helpers.go:162`
  `buildFnBodyHandler`) and runs a SINGLE end-of-run `autoEvalStack`
  (`eng/go/engine.go:991`). The returned `[[c1]]` is auto-evaluated AFTER the
  param frame is torn down, so `c1` resolves in MODULE scope → `[1]`.
- **Compiler refusal is SOUND:** `AnalyseFnBody` (`eng/go/carrier.go:2308`) runs
  the body in an ISOLATED `sub.Run` (`carrier.go:2434`) whose end-of-run
  `autoEvalStack` fires WHILE the param `c1` is still bound, so the inner `[c1]`
  resolves to the PARAM carrier (`Integer`) → residual `[[Integer]]` →
  `materialise` fails on the carrier → refuse at `eng/go/emit.go:1426`. The param
  carrier POISONS the residual, so the compiler can never bake `[9]`. (Confirmed:
  the shadowing matrix — deeper `[[[c1]]]`, later-rebind `… mk 9 def c1 2` → `[2]`,
  the no-module-c1 `undefined_word` case — all fall back faithfully today.)
- **The blocker:** to compile, mk's residual must stay the RAW deferred list
  `[[word(c1)]]` and be folded by the existing top-level deferred-list machinery
  (`autoEvalList` → const-fold, `engine.go:3051`) — which ALREADY handles
  module-scope deferred words correctly (`def c1 1 [[c1]] def c1 2` compiles to
  `[[2]]`). BUT the bytecode VM has **NO deferred auto-eval**: `runProgram`
  (`vm.go:161-217`) has no final `autoEvalStack`, `OpCallNative` (`vm.go:693-735`)
  calls handlers on raw stack args with no `autoEvalList`, and the VM fails loudly
  on raw Words in results (`tapeCoupled`, `vm.go:123`). So baking the raw list is
  sound ONLY when mk's result lands at the TOP-LEVEL residual (folded at compile
  time); the moment it is CONSUMED downstream (`mk 9 0 get`, `mk 9 size`) the VM
  would operate on the unresolved `word(c1)` while the interpreter auto-evals the
  consumed arg in module scope — a SILENT WRONG ANSWER. Suppressing
  `AnalyseFnBody`'s auto-eval to keep the residual raw has corpus-wide blast
  radius and opens exactly that hazard.

PREREQUISITE for Option A: give the bytecode VM a deferred-auto-eval pass (both
end-of-run AND consumed-arg `autoEvalList`, with the interpreter's module-scope
resolution) — a substantial new VM feature, not an emit-side tweak. Matches this
note's original "subtle, higher risk" assessment.

### Option B (keep refusing) — RECOMMENDED, in place
Leave it. The fallback returns `[1]`. Same P7 caveat.

## Determinism observation (flagged for investigation, NOT an active bug)

The Option-A attempt observed that, in ISOLATED repeated `CompileCheck` of the
exact recursive source within one process, the FIRST call refuses while later
calls behave differently — correlated with global mutable state advanced during
the ~256-deep recursive expansion: the process-global, time-seeded value-ID RNG
(`eng/go/value.go:1005-1008`) feeds `CanonValue` → the emit state's `producedBy`
provenance map. **In the actual gate this is NOT observed** — `TestCompiled
Coverage` deterministically reports refused=2, this row consistently refuses.
So there is no evidence of a non-deterministic SHIPPING path; the observation is
a probe-scenario artifact worth a look only IF Option A's prerequisite is later
pursued (deep recursive expansion stressing the value-ID/provenance coupling). A
deterministic, non-time-seeded value-ID scheme would be the clean fix if it ever
proves real. Do not chase it speculatively.

## Recommendation (final)

- **`macro.tsv:45`:** **DONE** — compiled to a terminal trap (see the UPDATE
  above). refusals 2 → 1.
- **`def-node-binding.tsv:54`:** **keep refusing (Option B)** — the LAST refusal,
  and its refusal is PROVABLY SOUND (the param carrier poisons mk's residual, so
  the compiler can never bake the wrong `[9]`). Compiling it requires a new VM
  deferred-auto-eval pass (Option A above) — a substantial feature with
  corpus-wide blast radius, not worth it for one row that already falls back
  faithfully (`[1]`).

Net (final): refusals = **1**, and it is a SOUND refusal that falls back
faithfully. Every spec row produces the correct result in both engines. Reaching
refusals=0 (the gate to delete the interpreter fallback, P7) is blocked on a
single substantial prerequisite — a VM deferred-auto-eval pass — which is a
deliberate future project, not a quick win. This is the natural stopping point:
the bytecode compiler now compiles every spec row that can be compiled soundly.
