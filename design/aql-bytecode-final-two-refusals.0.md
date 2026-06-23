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

### Option A (compile-to-trap) — viable, moderate effort
Emit a terminal `OpTrap` that raises the byte-identical `macroexpand_error`.
This is the SAME pattern already used for other expansion-time errors —
`mini`/`parse` `*_unknown_lang`, `illegal_ref`, getr `not_found` (see the
`refusalCeiling` history in compiled_coverage_test.go: "a top-level RecordTrap …
compiles a TERMINAL OpTrap raising the byte-identical error"). The checker would
need to detect the runaway expansion during the check pass (run macroexpand
under the same depth guard the interpreter uses) and `RecordTrap` when it trips.
Sound by construction — the compiled program raises the identical error, the
differential gate stays green. Takes refusals **2 → 1**.
- Risk: low. The trap pattern is established. The work is making the check-pass
  macroexpand hit + report the depth limit rather than degrading to an
  unmaterialisable residual.

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

## Recommendation

- **`macro.tsv:45`:** pursue **Option A (compile-to-trap)** if/when 0 refusals
  (P7) is the goal — it is tractable, matches an established sound pattern, and
  carries low risk. This is the better of the two to attempt next.
- **`def-node-binding.tsv:54`:** **defer** (keep refusing) unless P7 is being
  actively closed out. The fallback is faithful, and a correct compilation is
  subtle with a silent-wrong-answer failure mode; only attempt it with thorough
  differential coverage of deferred-eval scope shadowing.

Net: refusals can soundly reach **1** via Row 1's trap with modest effort;
reaching **0** additionally requires Row 2's deferred-eval-scope feature. Until
then, both rows fall back faithfully and every spec row produces the correct
result in both engines.
