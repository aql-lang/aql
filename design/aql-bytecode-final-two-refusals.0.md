# The final two refusals — decision note (compile-to-trap vs keep-refusing)

_Status: DISCOVERY / decision-needed. After Stage-3 cleared the three module
feature rows, the compiled-coverage refusal count sits at **2** (down from 5
this effort). This note records exactly what those two rows are, corrects an
earlier overstatement about them, and lays out the options. No behaviour change
is proposed here — that needs an explicit decision._

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

### Option A (compile correctly) — subtle, higher risk
Model the deferred-evaluation scope: a returned list literal whose words resolve
at post-return (module) scope, not the fn's param scope. Done correctly the
compiled program returns `[1]`, matching. This is a genuine feature (deferred-
eval provenance), and the failure mode of getting it wrong is a SILENT wrong
answer (`[9]`) — so it must be driven entirely by the differential gate.
- Risk: higher than Row 1. The whole point of the current refusal is that
  distinguishing the two `c1` bindings is exactly what's hard; a naive
  compilation returns `[9]`. Worth doing only with careful differential
  coverage of the param-vs-module-binding shadowing cases.

### Option B (keep refusing) — zero cost, faithful
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

## Recommendation (revised after the Option-A attempt)

- **`macro.tsv:45`:** **keep refusing (Option B).** Option A is blocked on a
  prerequisite that was not low-risk as first thought — recursive-macro paren
  dispatch under check mode — which is deep engine work touching the macro path.
  Not worth it for one error row that already falls back faithfully.
- **`def-node-binding.tsv:54`:** **keep refusing (Option B).** Unchanged: a
  correct compilation needs deferred-eval-scope provenance modeling, subtle with
  a silent-wrong-answer failure mode.

Net (revised): **both** remaining rows have an identified deeper prerequisite and
both fall back faithfully today. refusalCeiling stays at **2**. Reaching P7
(refusals 0, delete the interpreter fallback) requires two separate engine
features (recursive-macro check-mode dispatch; deferred-eval-scope provenance) —
each a deliberate project, not a quick trap. Until then every spec row produces
the correct result in both engines; the only cost of the 2 refusals is that the
interpreter fallback cannot yet be deleted.
