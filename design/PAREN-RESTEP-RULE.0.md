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

**75 parity assertions across 5 files still read `gotI, _ := …Run(src)`** and
compare it against `RunCompiled`. Every one of them compares the compiled
lane against itself and passes unconditionally. `TestFactoryApplyCompiles`
is the clean example: it asserts `(mk2 5) 10` is `[11]` on "both lanes",
and the interpreter has never answered `11` for that program.

This is the finding that matters most. A parity harness that silently stops
comparing lanes is worse than no harness: it converts a whole class of
divergence into a green check. Any future flip of a Run-like entry point
must be accompanied by a mechanical sweep of its oracle uses.

## 5. What landed here

The interpreter is UNCHANGED — §1 is what it already did. Everything below
is the compiler learning it.

1. **`fnReturnPark`'s survivor clause is restored** and its header rewritten
   to say why the count is the question.
2. **`stepCloseParen`'s window scan admits a Function-typed CARRIER lead**,
   not only a DYNAMIC one. A paren whose lead the windows cannot model now
   refuses (sharing `recordParenLeadingApply`'s existing refusal site)
   instead of falling through to a placement the interpreter never performs.
   Concrete fn values are excluded by `IsFnTypedCarrier`'s `Carrier` bit,
   which keeps `((inc/v) 7)` → `8` compiling.
3. **`RecordBranch.resolveArm` refuses an arm whose residual LEADS with a
   re-stepped fn** (`armResidualReStepped`) — narrower than
   `closureResidualHasUnappliedFn`, because an arm's SOLE carrier is
   legitimately placed data on both lanes.

Result on the 16-shape probe: **0 value divergences**, down from 5.

## 6. Graduation

The refusals are the honest signal, not the destination. Applying a
paren-bounded carrier lead needs `RecordDynApply` to admit an EVENT lead,
which `DynApplyLeadEligible` declines today — that is Stage 3's universal fn
values plus the Apply kernel. When it lands, rows 2 and 3 above become
recorded applies rather than refusals, and `((mk 1) 2)` compiles natively in
every position rather than only as the program residual.
