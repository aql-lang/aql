# Forward Collection Is Two Phases

Status: **documents shipped behaviour** (the statement-boundary commit
and template evaluation landed in `00cb7a7`, June 2026). This note is
the map of how forward arguments are actually gathered — written
because the guard-pre-emption bug (bloom-filter report #1) survived as
long as it did on the assumption that there is ONE code path enforcing
the collection barriers. There are two, and they can diverge. Keep
this note current when touching either.

## The question

The argument-order rule (TUTORIAL §3) says forward collection stops at
a barrier: `end`, `)`, another function word, type mismatch. So how
could a parked `if` sit through a following `def` — letting the next
statement run before a guard's raise, and swallowing the next
statement's result as a phantom else? Surely one code path checks the
barrier?

No. Forward-argument gathering is split across two phases that live in
different functions, walk different representations, and — before the
fix — enforced different stop conditions. The rule as documented
described only the first phase.

## Why two phases are forced

A forward argument can be an **expression**: in `mul 2 (add 3 4)` the
value `7` does not exist when `mul` dispatches. The engine is a single
linear pass over the token tape (the stack IS the tape; there is no
AST to pre-bind call shapes), so the only option is to *park* the word
and collect the value when it materialises. Parking splits gathering
into:

1. a **plan-time token walk** over unstepped source tokens, and
2. a **run-time arrival loop** over values reaching the pointer.

Both are load-bearing. Neither can absorb the other: the walk cannot
know an expression's value, and the arrival loop cannot see source
structure that has already been stepped away.

## Phase 1 — the plan-time walk

`stepWord` → `resolveForwardArgs` (engine.go:902, the structure-first
lazy scan of `design/LAZY-ARG-RESOLUTION.0.md`) → `matchSignature`.
The walk looks at the tokens after the word, prunes non-viable
overloads on concrete literals, pre-evaluates paren groups (and, since
`00cb7a7`, interpolated template strings) only at positions some
still-viable overload consumes, and picks a signature.

Phase-1 stop/skip conditions:

- `end`, `)`, engine markers — hard stop.
- A paren group no viable overload consumes — left raw, hard stop.
- A concrete literal that rules out every overload — type prune.
- **A word — counted as one optimistically-filled position and the
  walk continues** (`staticForwardType`, engine.go:1114, classifies it
  `fwdBoundary` but the scan does `pos++`).

That last line is deliberate and load-bearing: `def x (expr)` needs
the planner to look *past* the name `x` — a word — to find and
pre-evaluate the body paren at position 1. The cost: for
`if (cond) [then] def …` the planner counts `def` as a plausible
filler for the else slot, keeps the 3-arg overload viable, and parks
`Forward{ExpectedArgs: 3}` (`insertForward`, engine.go:2103).

When every selected position is resolvable *now*, there is no parking
at all — the word dispatches inside phase 1. This is why
`range 2 6 def x 99 x` was always correct (two literal args, 2-arg
overload complete at the barrier) while the same shape with a paren
condition was not: **same surface syntax, different phase, different
rules.**

## Phase 2 — the arrival loop

Once parked, the word no longer sees tokens. The collection block in
`stepLiteral` routes every VALUE that reaches the pointer into the
next slot (`CollectedArgs++`); on `CollectedArgs == ExpectedArgs` the
completion path (engine.go:2284) drops the marker, force-stacks the
word, and runs `rearrangeForForward` so the first-collected argument
lands on top — sig order for the matcher's top-first read.

Phase-2 stop conditions, before the fix:

- an arriving value that mismatches the next slot's type —
  `implicitEnd` (engine.go:3423);
- an explicit `end` token — `stepEnd` (engine.go:3554);
- end of program — the leftover-forward drain.

And that is the whole list. A function word never *arrives* — it goes
through `stepWord` and dispatches itself — and `stepWord` had no idea
a Forward was pending below it. Nor could type mismatch help when the
waiting slot is `Any` (an `if` else): everything matches `Any`. So
for

```aql
def f fn [[n:Integer] [Integer] [
  if (n eq 0) [raise "zero"]
  def q (10 div n)
  q
]]
```

the parked `if` sat at 2/3 while `def` ran first — the division by
zero pre-empted the guard's raise — and in
`if (n eq 0) [99] add 1 2` the `3` arrived as a perfectly legal else
and was discarded. The failure also hid well: the visible symptom was
the *next statement's* error, which reads as a bug in user code, not
an engine ordering problem.

The same lens explains the sibling template bug fixed in the same
commit: an InterpString is an expression, but phase 1 classified the
raw token by its internal type, so every typed overload was pruned and
`` raise `bad: ${x}` `` mis-dispatched. Expressions must be evaluated
by the phase that meets them, not type-tested as tokens.

## The fix — statement-boundary commit

No third path. `stepWord` is the one chokepoint every dispatching
function word already passes through, so it now asks first: *is there
a parked forward in this paren scope that could already fire?*
(`commitBarrierForward`, engine.go:3474):

1. Nearest pending Forward, scanning down to an open-paren boundary —
   the same scan `stepEnd` performs.
2. **Satisfiability probe** over the forward's CLAIMED args only
   (collected + claimed stack), via `MatchSignature` on the
   collection-order slice. Deliberately narrower than the whole-scope
   match: an implicit commit must not reach below what the word
   claimed.
3. On a match, commit with the **completion path's** mechanics: drop
   the marker, force-stack the word, `rearrangeForForward`, re-step.

Step 3 explicitly does NOT reuse `curryOrStack` (engine.go:4242): its
rearrange is gated on `StackArgs > 0`, so a purely-arrival forward
(`if` with a paren condition: 2 collected, 0 stack) would re-dispatch
with its collected args in reverse — cond and then swapped. The
completion path rearranges unconditionally; the commit mirrors it.
(That gate remains a sharp edge inside `curryOrStack` for any future
caller that reaches it with `StackArgs == 0` and 2+ collected args —
the explicit-`end` paths do not today, because an `end` visible to the
phase-1 walk demotes the arity before parking.)

The satisfiability gate is what preserves deferred-argument flows:

| Form | Behaviour |
|---|---|
| `if (c) [t] def q (…)` | guard fires FIRST, then `def` runs |
| `if (c) [t] add 1 2` | guard fires; `3` stays on the stack (no phantom else) |
| `if (c) [t] (e)` | unchanged — a paren is not a word; its result arrives as the else |
| `if (c) [t] [e]` / `… 42` | unchanged — literals arrive as the else |
| `add 1 def x 5 x` | unchanged — `add` can't fire on one arg, so it keeps waiting |
| `range 2 6 def …` | unchanged — completed in phase 1, never parked |
| explicit `end` / `;` | unchanged — `stepEnd` as before |

One deliberate change beyond the bug: chaining an else-if by letting a
following bare `if`'s *result* arrive as the outer else
(`if c [a] if c2 [b] [e]`) now commits the outer `if` first. The form
appears nowhere in the spec corpus or docs — the documented else-if is
the single-list clause form `if [c1 b1 c2 b2 … else]` — and the old
behaviour ran the inner `if` eagerly even when `c` was true, which is
the same pre-emption disease this fix exists to cure.

## Invariants (keep these true)

- Phase 1 owns *which overload and which token positions*; phase 2
  owns *when values fill them*. Any stop condition added to one phase
  must be reconciled with the other — this divergence is exactly how
  the guard bug happened.
- A token kind that denotes an expression (paren group, template,
  reach) must be EVALUATED by the phase that encounters it, never
  type-tested raw against slots.
- An implicit commit (barrier or mismatch) must re-dispatch through
  the completion layout: `rearrangeForForward` before the top-first
  re-match. First-collected = sig[0].
- The commit probe matches claimed args only; widening it would let a
  statement boundary consume stack values the word never claimed.

Verification: `lang/spec/control.tsv` §6 pins the guard ordering, the
false-guard path, the kept statement result, and the paren-else
non-barrier; `lang/spec/error.tsv` pins the template forms. The
behaviour ships with the full eng + lang suites green.

## Residual

The two phases can still drift — the fix routes the barrier through
the `stepWord` chokepoint, it does not make divergence impossible. If
this bites again, the candidate unifications are: (a) record the
plan-time stop conditions on `ForwardInfo` and have the arrival loop
honour them, or (b) demote overload arity at plan time when the next
token is a function word. (b) was considered and rejected here:
words-as-positions is load-bearing for `def x (expr)` pre-evaluation,
and a plan-time demotion decides sight-unseen what the runtime
satisfiability gate can arbitrate with the actual values in hand.

Related: `design/LAZY-ARG-RESOLUTION.0.md` (phase 1's structure-first
scan), `design/FORWARD-COLLECTION-TRAPS.0.md` (zero-value arrivals and
bare-word keys — other costs of the same architecture),
`design/FORWARD-STRAND-ADVISORY.10.md` (the check-mode advisory for
mixed-form stranding).
