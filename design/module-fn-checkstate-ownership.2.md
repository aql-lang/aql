# §6 follow-on — compiling the test framework: run-spec blocker investigation

Status: investigation on top of the landed §5 refactor (commits `865a84d`,
`2816e8f`) and a tangential §6 refinement (`aada2c1`, see below). Records what
§5 unblocked and what — by hands-on tracing — actually blocks `module-test.tsv:38`
(`run-spec`). Two earlier guesses (element typing; the `Quoted` flag) were tested
and DISPROVEN; this note states the verified facts and the precise open target.

## What §5 already unblocked (measured)

`test-describe`'s closure path now compiles for-loop tails in a `BodyOut:0`
closure, value-defs of computed values, computed-count `for (n) [...]`, AND the
dynamic subject dispatch (`Test.invoke`). The closure infra is not the blocker;
`run-spec` falls back faithfully.

## What it is NOT (each tested, then reverted)

- **NOT element typing** (the original `.2` guess). `def cs (s get "cases")
  def c (cs 0 get) c get "k"` — a generic-Map field read, then element read,
  then field read — COMPILES. List/map element field-access already works.
- **NOT the `/q` type-match phase.** `quote (s get "cases")` first refused with
  "quoted-operand word quote": the `get`-on-a-generic-Map result is a
  non-concrete carrier that optimistically conforms to `TAtom`, so quote's
  word-capture sig (`[TAtom]`, QuoteArgs, tried first) claimed it. Commit
  `aada2c1` guards `positionalMatch` so a `/q` position rejects a non-concrete
  carrier (routing it to the value sig). That guard is CORRECT and gate-clean,
  but **tangential**: it only changed the refusal *reason*, not the outcome —
  corpus-neutral, does not unblock `run-spec`. Kept as a small refinement, not a
  step on the critical path.
- **NOT the `Quoted` flag.** Hypothesis: quote sets `Quoted=true`, breaking
  downstream `get`-matching. Tested by skipping `Quoted` in check mode — the
  chain STILL refused. Reverted.

## What it IS (verified)

Instrumenting `quoteAnyHandler`'s input: `quote (s get "cases")` hands the
handler a **`None` carrier** (`parent=None data=<nil> carrier=true`). So inside
the `quote`, the inner `(s get "cases")` evaluates to **None** in check mode —
whereas the SAME `(s get "cases")` standalone (the noquote control above) yields
a matchable carrier the downstream `get`s accept. The downstream `cs 0 get` /
`c get …` then run on a None-derived carrier, match no signature, and the
recovery emits a `no_signature` ERROR (`@…`), which `CompileCheck` refuses on.

So the quote changes the inner expression's check-mode RESULT (matchable carrier
-> None). The remaining unknown is the exact mechanism: quote's forward-collection
evaluates the paren `(s get "cases")` at a point/mode where the check-mode
`get`-on-generic-Map returns None (missing-key result) instead of the
dynamic/typed carrier it returns when evaluated inline. The forward-collection
phase (`engine.go` ~`1205`/`2927`/`5856`, `resolveForwardArgs`) and `get`'s
check-mode ReturnsFn for a generic-Map receiver are the two suspects.

### Next target

Pin why `(s get "cases")` yields None when forward-collected by `quote` but a
matchable carrier inline, and make the two agree (the inline behaviour is the
correct one — a generic-Map field read should yield a dynamic carrier, not
None). That is the actual §6-unblocking step. Guard tightly and watch the corpus
(refusalCeiling / any-frontier); /q forward-collection is language-wide.

## Do NOT broaden the dispatch recovery (verified regression)

A tempting shortcut — record `OpCallNativePoly` in the non-disjunct recovery for
non-concrete args, so a `get`-on-None/Any compiles — pushes `refusalCeiling`
10 -> 23 (0 divergences): the poly result is a dynamic Any carrier that POISONS
downstream typed consumers corpus-wide. The fix belongs upstream (the None
production), not in the recovery.

## After this blocker

The cascade continues: `test-record`'s 7-arg call with `[]` + `(c get "name")`
operands, and the recursive `run-spec` over `subs` (`for (subs size)
[..run-spec..]`) — re-attempt the reverted `evCallUser` value-def-local
promotion (`.0` §7) there. Gate: `module-test.tsv:38` ->
`{total:2 passed:2 failed:0}` compiled == interpreter, 0 divergences,
`TestModuleFnCheckPathGate` green.
