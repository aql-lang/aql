# The paren re-step rule — what actually decides place-vs-apply

**Status:** MEASURED 2026-08-27 · supersedes NUR101's "place uniformly"
ruling of 2026-08-26, which was written against an unmeasured premise.

## 0. Why this document exists

NUR101 asked one question — *does a computed function applied inside an
enclosing group place, or dispatch?* — and was ruled **place uniformly** on
2026-08-26. The first implementation of that ruling deleted
`fnReturnPark`'s survivor-count clause (`closeIdx != idx+2`), on the stated
reasoning that "the survivor count was never the right question".

**It is the right question.** Deleting the clause turned
`(x:Integer => [x mul 2] 5)` from `10` into `fn (Integer) 5` and broke seven
suites with it. The clause was reinstated the same day. What follows is the
rule as MEASURED against `RunInterp`, the tree-walking oracle, rather than
as assumed.

## 1. The rule

> A Function value that a paren **placed** is re-stepped into a **call**
> exactly when it leads **two or more survivors** of an enclosing group that
> closes with a paren rewind.

Both halves are load-bearing, and each is one clause of `fnReturnPark`:

- **survivor count.** `stepCloseParen` sets `e.Pointer = openIdx + park`.
  With exactly one survivor the park returns 1 and the pointer steps PAST
  the Function — it is placed, never re-stepped. With more than one the park
  returns 0, the rewind lands ON the Function, and `stepLiteral` dispatches
  it. Placement is therefore the ONE-survivor case, and always was.
- **which groups rewind.** Only `stepCloseParen` performs the rewind. User
  parens, fn frames and `if` / `for` / `do` bodies close through it; the
  program top level, list literals and map literals do not.

## 2. The measurement

Against `RunInterp` (`lang.Run` is NOT the interpreter post-Stage-J — see
§4), with
`def mk fn [[a:Integer] [Function] [(fn [[b:Integer] [Integer] [a add b]])]]`:

| program | enclosing rewind? | interpreted |
|---|---|---|
| `(mk 1) 2` | no — program residual | `fn (Integer) 2` — **places** |
| `((mk 1) 2)` | yes — the outer paren | `3` — **applies** |
| `[(mk 1) 2]` | no — a list literal | `[fn (Integer) 2]` — **places** |
| `[((mk 1) 2)]` | yes — the inner paren | `[3]` — **applies** |
| `if true [(mk 1) 2]` | yes — the arm's frame | `3` — **applies** |
| `for 2 [(mk 1) 2]` | yes — the body frame | `3 3` — **applies** |
| `do [(mk 1) 2]` | yes — the body frame | `3` — **applies** |
| `case 1 [1] [(mk 1) 2]` | no — the arm is a literal | `[fn (Integer) 2]` — **places** |

The rule predicts every row. NUR101's framing — "placement depends on
enclosing context" — was a correct OBSERVATION with the wrong subject: the
context does not modify a placement decision, it IS a second, separate
decision, taken one paren out.

## 3. What the compiler was doing

Five silent miscompiles, in both directions, all one root cause: **the paren
structure is erased before the residual lowering runs**, so
`resolveDynamicApply` sees the same `[carrier, 2]` residual for `(mk 1) 2`
and for `((mk 1) 2)` and must guess.

| program | interpreted | compiled (before) |
|---|---|---|
| `(mk 1) 2` | `fn (Integer) 2` | `3` — applied what the interpreter placed |
| `(mk2 5) 10` | `fn (Integer) 10` | `11` — same |
| `[((mk 1) 2)]` | `[3]` | `[fn (Integer) 2]` — placed what the interpreter applied |
| `if true [(mk 1) 2]` | `3` | `fn (Integer) 2` — same |
| `if true [((mk 1) 2)]` | `3` | `fn (Integer) 2` — same |

The two directions are not two bugs. `((mk 1) 2)` at the top level compiled
CORRECTLY only by accident: the outer paren collapses, the pair reaches the
program residual, and the carrier arm applies it there — the right answer
by the wrong mechanism, which is why the same paren one level down inside a
list or an arm produced the opposite answer.

## 4. How five miscompiles survived a 100%-covered parity suite

Stage J flipped `lang.Run` from the tree-walking interpreter to the
COMPILED path with an interpreter fallback on refusal. `RunInterp` is the
oracle; `Run` is not one any more.

**96 parity assertions across 5 files still read `…Run(src)`** and compare it
against `RunCompiled`. Every one of them compares the compiled lane against
itself and passes unconditionally. `TestFactoryApplyCompiles` is the clean
example: it asserts `(mk2 5) 10` is `[11]` on "both lanes", and the
interpreter has never answered `11` for that program.

This is the finding that matters most. A parity harness that silently stops
comparing lanes is worse than no harness: it converts a whole class of
divergence into a green check. Any future flip of a Run-like entry point
must be accompanied by a mechanical sweep of its oracle uses.

**And the first pass of that sweep was itself incomplete** — a review bot
caught it, which is the same lesson twice. The pattern used keyed on ONE
naming convention (`gotI`) and missed `errI`, `eI`, `gi`/`ei` and an
effect-parity closure: 21 assertions in the same files kept comparing the
compiled lane to itself. A convention-keyed sweep is not a sweep. The audit
that finds the last one is "every `.Run(` in a file that also calls
`RunCompiled`, read individually". Completing it surfaced two more
divergences (NUR108, NUR109) on top of NUR107.

## 5. What landed here

The interpreter is UNCHANGED — §1 is what it already did. Everything below is
the compiler learning it.

**The mechanism, and the reason it has to exist.** The paren structure is
erased before the residual lowering, so `resolveDynamicApply` sees the same
`[carrier, 2]` for `(mk 1) 2` and for `((mk 1) 2)` — one placed, one applied,
byte-identical at that point. Nothing downstream can recover the difference,
so the fact is recorded where it is still known: at the collapse.

`CheckState` now carries a matched pair, and together they ARE §1:

- `ParenPlacedFnIDs` — the carriers a paren PLACED (one survivor; the park
  returns 1 and the pointer steps past). Already existed for member reads;
  the park now records every carrier it places.
- `ParenReSteppedFnIDs` — the carriers an enclosing paren's rewind LANDED ON
  and will re-step into a call (more than one survivor; the park returns 0).
  New, written by `recordParenReStep` immediately after the park is read,
  from the same post-removal indices.

`leadPlacedNotRead` then reads "placed, not arrived through a read that
dispatches, **and not re-stepped one level out**". Three facts, all recorded,
none inferred.

The individual changes:

1. **`fnReturnPark`'s survivor clause is restored** and its header rewritten to
   say why the count is the question. No behaviour change from `main`.
2. **`recordParenReStep`** records the park's negative twin (above), under the
   park's own exclusions — a reach-lowered group's re-step is its dispatch, not
   a user paren's — and the same "might be callable" test the auto-dispatch
   guard uses, so both ends agree on what the rewind would have called.
3. **`resolveDynamicApply`'s carrier arm** applies a placed lead only when the
   re-step record says an enclosing paren claimed it. `((mk 1) 2)` and
   `((mk2 5) 10)` keep compiling natively; `(mk 1) 2` and `(mk2 5) 10` — which
   compiled to the WRONG answer — now refuse.
4. **Its dynamic arm** was over-refusing a def-bound read (`def h (find …)  h
   {…}`): a bare name always calls, whatever mark its value carries from
   wherever it was built. It now shares `leadPlacedNotRead`'s conjunction
   instead of testing placement alone.
5. **`RecordBranch.resolveArm` refuses an arm whose residual LEADS with a
   re-stepped fn** (`residualLeadReStepped`) — narrower than
   `closureResidualHasUnappliedFn`, because an arm's SOLE carrier is
   legitimately placed data on both lanes (`if c [(mk 1)]`).
6. **`RecordMakeListInner` refuses a list whose LEAD element the re-step record
   claims.** This is where the record earns its keep: `[(mk 1) 2]` arrives with
   byte-identical elements and must keep compiling, so only the recorded fact
   can separate them.

Result on the 16-shape probe: **0 value divergences**, down from 5, with
`((mk 1) 2)`, `((mk2 5) 10)`, `[(mk 1) 2]`, `for`/`do` bodies and
`((inc/v) 7)` all still compiling natively.

The standing measurement is `lang/go/nur101_paren_restep_test.go`
(`TestParenReStepRule`): for every shape the rule classifies, the compiled lane
either agrees with `RunInterp` or refuses — it never answers differently.

## 6. Graduation

**Two graduated 2026-08-27 with Stage 3's first increment, and the mechanism
is not the one this section predicted.** The refusals were called "the same
shape in four positions". They are two different questions, and only one of
them needed the Apply kernel:

- **A LAYOUT question** — will anything re-step this value where it now
  sits? At a program residual, no, and the records already prove it.
  `placedNotReStepped` (`compiler/go/emit.go`) lets the residual refusal loop
  skip such a value and simply lay it out, where before it refused, reading
  the ABSENCE of a record as evidence of a hazard. `(inc/v) 7` and
  `(tbl get k) 5` compile natively on that alone, and
  `frontier-nur038-seal.tsv`'s explicit-paren row graduated into
  `lang/spec/fn-value.tsv` §6 with them. A CONCRETE fn value joined the
  record as its fourth placement shape for this reader; the two apply arms
  gate on `Carrier` and `Dynamic` respectively and never see one, so the
  widening was layout-only.
- **A RENDERING question** — `(mk 1) 2` and `(mk2 5) 10` still refuse, now
  saying so precisely: "unconsumed fn-value carrier in residual (closure
  render)". A VM closure does not render like the interpreter's `FnDefInfo`.
  That is §6.3's universal fn value, and it is Stage 3's real dependency.

The two remaining arm/list shapes (`if true [(mk 1) 2]`, `[((mk 1) 2)]`) need
the apply RECORDED rather than skipped, which needs `RecordDynApply` to admit
an EVENT lead — `DynApplyLeadEligible` declines it today. When that lands,
`TestParenReStepListElementRefusal` graduates to a parity row and
`ParenReSteppedFnIDs` can retire in favour of the recorded event.

The ratchet for what already graduated is
`TestParenReStepPlacedLayoutCompiles`, pinned POSITIVE on purpose: the rule
test tolerates refusals by design, which is what makes it safe to extend and
useless for catching a regression back to one.

## 7. Four records this opened

- **NUR106** — the vacuous parity harness (§4). The sweep landed; the guard
  that stops it recurring has not.
- **NUR107** — a callee no overload of which matches raises `signature_error`
  interpreted and returns data compiled. Surfaced by the sweep, on the very
  test that had pinned that claim as non-reproducing. VM-side fix, its own
  increment.
- **NUR108** — the compiled lane's diagnostics point somewhere else: a
  `BIND_TYPED` validate failure carries no position at all, and a
  statically-failing typed-def trap blames the value where the interpreter
  blames the `def`. Same code, same message.
- **NUR109** — `parse` over an unbound def-scoped parser name is `parse_error`
  compiled and `parse_unknown_lang` interpreted. The compiled answer is the
  specified one; the interpreter still falls back to the kind-name miss.

All three divergences are error-lane, and all three were behind NUR106. That
is the pattern worth carrying forward: the oracle hole was in the files most
specifically about the lane it hid.
