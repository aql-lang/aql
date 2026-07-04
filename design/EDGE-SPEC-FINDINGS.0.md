# Edge-spec expansion — compiler divergences found

Status: **findings** (2026-07-04). While adding ~2,000 edge-case spec
rows (the `lang/spec/edge-*.tsv` families), three genuine
compile≠interpret divergences surfaced — the exact class of latent
miscompile the expansion was meant to flush out. Each is a shape the
compiler lowers to a WRONG result where it should either compile
correctly or REFUSE and fall back (the "slow, not wrong" contract).
The rows are dropped from the corpus (they cannot be pinned under the
0-divergence gate) and recorded here as reproducers for a sound-refusal
or correct-lowering fix.

## 1. Forward collection reaches across an error-handler residual

```
5 do [raise aa "x"] error [drop 9] add 1
```

- Interpreter: `5 10` — the `error [drop 9]` handler leaves `5 9` on the
  stack, then `add 1` forward-collects `1` and takes `9`, computing
  `9 + 1 = 10` and stranding `5`.
- Compiled: `14 1` — the compiled path mis-schedules the `add`
  operands across the `do…error` boundary (appears to compute `5 + 9`).

Root-cause family: the residual-window operand accounting across a
reified-error boundary — adjacent to the statement-boundary absorption
class the M2c landing already closed for method dispatch
(`design/CHECKER-BYTECODE-COMPLETION-PLAN.0.md`, Phase 3.5 log). The
sound fix is either to model the handler residual as a barrier the
`add` window cannot cross, or to refuse the shape.

## 2. Applied member-fn boundary leaves the function unapplied

```
def d fn [[n:Integer] [Integer] [n mul 2]] def m {double: d/r} m.double 21 eq 42
```

- Interpreter: `true` — `m.double 21` auto-applies the parked `d/r` to
  `21` → `42`, then `eq 42` → `true`.
- Compiled: `fn d(Integer) false` — the compiled path leaves the fn
  value unapplied on the stack, so `eq 42` compares the function to
  `42` (false) with the fn stranded.

Root-cause family: the fn-value-call boundary (plan P4 / M2). The M2c
method-shape work models statement-position *0-return* method calls;
this is a *value-returning* member fn (`m.double 21` feeding `eq`) —
the shape-annotated dispatch must extend to a mid-expression member
apply, or refuse.

## 3. Paren-arrived value run as an else body (from edge-forward)

```
def n 5 if (n eq 0) [99] (range 2 4)
```

- Interpreter: `2 3` — the parenthesised `(range 2 4)` arrives as the
  else *body* and is executed, leaving its elements.
- Compiled: `[2 3]` — the compiled path treats the arrived value as a
  list result rather than an executed else body.

Root-cause family: the computed-else branch modeling for a paren group
whose value is itself a runnable body — the boundary between "else
value" and "else body" under forward arrival. Sound fix: refuse the
ambiguous computed-else-body shape (fall back), or model the
body-execution semantics.

## Disposition

All three are **soundness** items: the compiler produces a wrong value
where it must refuse. They are lower-frequency than the corpus rows
(none appears in the 3,875-row base corpus), and each has a clear
sound-refusal fix that costs at most the row itself. Recommended: add
each as a refusal (fall back) first — restoring the invariant — then
model the correct lowering as a follow-up, mirroring the Phase-6
"refuse-then-model" discipline. Landing tests should pin each
reproducer's fallback parity.

## Checker-precision rows removed to hold the sacred pins

Six edge rows were dropped from the corpus not because they fail, but
because pinning them would move a **sacred checker pin** that the
check-by-default safety case depends on. Each runs correctly; each is
a candidate for a checker-precision follow-up.

- **`[None] get 0` → None** (edge-containers-4): the checker flags this
  correct row — a genuine value-row **false positive** (the FP pin is
  0). A stored/read `None` at a concrete list index is the trigger.
- **Type-algebra precision gaps** (would raise the type-soundness pin
  from 7): `if/1 [true 7]`; `P tor Q` (union of two newtypes);
  `tnot Never` → Any; `tnot Any` → Never; a predicate-fn-as-type
  return admitting a passing base value. In each the checker's static
  stack type differs from the runtime value's type — the same
  "checker-more-precise-than-runtime / type-algebra" class the seven
  standing soundness pins already document, not a wrong-acceptance.

These belong to the same checker-precision track as
`checker-precision-fronts.0.md`; folding them in is a pin-raise
decision for a maintainer, kept out of this test-expansion landing.

## 4. `args.N` accessor diverges inside a compiled fn body

```
def f fn [[Integer String] [String] [args.1]] f 1 "hi"
```

- Interpreter: `'hi'` — `args.1` reads the call's second argument.
- Compiled: divergent — the VM's CALL_USER frame binds params to
  frame locals and does not maintain the interpreter's per-call args
  stack, so `args.N` inside a compiled body reads wrong. The emitter
  refuses a bare `args` word (documented boundary); the dotted
  `args.N` form slipped past that guard and compiled to a divergent
  result rather than refusing.

Sound fix: extend the `args`/`__pa` compile refusal to the `args.N`
reach form so the body falls back. Reproducer dropped from the corpus.
