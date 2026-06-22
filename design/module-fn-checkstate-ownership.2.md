# §6 follow-on — compiling the test framework's code-body words: a blocker map

Status: investigation, on top of the landed §5 refactor
(`module-fn-checkstate-ownership.1.md` §5a/§5b/§5c, commits `865a84d`,
`2816e8f`). Records what §5 unblocked, the concrete cascade that still blocks
`module-test.tsv:38` (`run-spec`), and why the obvious first fix regresses — so
§6 is taken on with the real scope in view, not the plan's one-line billing.

## What §5 already unblocked (measured)

After §5, `test-describe`'s closure path (`tryRecordClosure` -> the
`CallableSpec` `BodyOut:0` body unit) compiles a broad range of bodies that
previously refused:

| body | now |
|---|---|
| `[1 2 add] "g" Test.describe` | compiles |
| `[for 3 [i] end] ...` (variadic for-loop tail in a `BodyOut:0` closure) | compiles |
| `[def x (3 4 add) x] ...` (value-def of a computed value) | compiles |
| `[def n 3 for (n) [i] end] ...` (computed-count for) | compiles |
| `[def r ([3] double/q Test.invoke) r] ...` (the subject dynamic dispatch) | compiles |

So the closure infrastructure, value-def-locals, computed-count loops, and even
the dynamic subject dispatch (`test-invoke`) are NOT the blockers. The check
path is correct: `run-spec` falls back faithfully, and its refusal reason moved
from the §5b leak artifact "residual lowering (call results reordered)" to the
genuine "code-body word test-describe".

## The remaining cascade that blocks run-spec (module-test.tsv:38)

`run-spec`'s body is `[ ... cases subject run-cases  for (subs size)
[...recurse...] ] name test-describe`. Compiling `test-describe`'s closure
recurses into the module-private `run-cases` -> `run-case` bodies. Tracing the
closure-body probe's first decline reason at each step (instrument
`recordClosureDispatch`'s throwaway `EmitState.Reason`):

1. **`get` on an Any-typed list element** — `def c (cases i get)` types `c` as
   `Any` (list elements are untyped), so `c get "in"` is `get(String, Any)`,
   matching no `get` overload -> the NON-DISJUNCT dispatch recovery
   (`engine.go`, ~6360) latches `MarkUncompilable("unmatched dispatch recovered
   at get")`. The DISJUNCT recovery just above already rescues a straddle via
   `tryRecordPoly`; the non-disjunct path never tries poly.

2. **`quote` of a computed operand** — `def cases quote (s get "cases")`.
   `quote` declares `CompileQuoteInert`, so it bakes only an INERT operand; a
   computed `(s get ...)` result trips `hasUncoveredQuoteArg && !quoteInertOK`
   -> `MarkUncompilable("quoted-operand word quote")`.

3. (unreached) `test-record`'s 7-arg call with `[]` + `(c get "name")` operands,
   and the recursive `run-spec` over `subs` in the `for` body.

## Why the obvious fix for (1) regresses — the trap

Extending the non-disjunct recovery to `tryRecordPoly` when an arg is
non-concrete (so `get` on an Any element records `OpCallNativePoly`, re-matched
on the real value at run time — sound by `tryRecordPoly`'s gate) CLEARS blocker
(1) but REGRESSES the corpus: refusalCeiling 10 -> 23, reducible tier-2 2 -> 15,
0 divergences but a large coverage loss.

Root cause: the poly-recovered result is a dynamic `Any` carrier (the overload,
hence the return, is value-determined). That dynamic result then POISONS
downstream consumers — a typed word fed the now-dynamic value refuses via the
dynamic-input guard. The recovery fires broadly (every non-concrete-arg no-match
across the corpus), so many rows that compiled with a precise recovered result
now carry a dynamic one and refuse a step later. Narrowing to `get`/`getr` does
not help: `get` on an `Any` receiver is inherently `Any`-returning, so the poison
is intrinsic to the result type, not the breadth.

The real lever is UPSTREAM PRECISION, not the recovery: the blocker is that
`cases`/`subs` elements are typed `Any`. With typed-list element typing (a
`cases` element known to be the case-record shape `{name,in,out}`), `c get "in"`
matches a real `get` overload, no recovery fires, and the result is precise — no
downstream poison. So §6 is gated on ELEMENT TYPING (or a dynamic-access
compilation that preserves a precise enough result), not on the dispatch-recovery
path.

## Recommendation

§6 is a multi-step dedicated effort, larger than the plan's three-word list
implies. Sequence:

1. Precise list/map element typing for the `get`-on-element case (the run-case
   field reads) — the prerequisite that makes (1) compile WITHOUT a dynamic-Any
   poison. Likely the largest piece; may overlap the typed-container work.
2. `quote` of a computed operand (2): allow `CompileQuoteInert` words to bake a
   `CALL_NATIVE` over a computed operand that RESOLVES to an operand, since
   `quote` of a runtime value is a deterministic handler — provided the QuoteArgs
   capture semantics for a paren-result operand are preserved.
3. Then the `test-record` multi-arg call and the recursive `for (subs size)
   [run-spec]` — re-attempt the reverted `evCallUser` value-def-local promotion
   (`.0` §7) here, per the plan.
4. Land on its own branch; the gate is `module-test.tsv:38` ->
   `{total:2 passed:2 failed:0}` compiled == interpreter, 0 divergences, and the
   `TestModuleFnCheckPathGate` collision fixture staying green.

Each step gated independently; do not let a refusal-count change from a later
step mask a regression from an earlier one (the §5/§6 separation discipline).
