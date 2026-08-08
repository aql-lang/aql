# CORE-TS-DIVERGENCES.1 — 135 measured core-level divergences, and where they hid

**Status:** 135 MEASURED · 80 CLOSED · 55 PINNED (2026-08-08) · **Ledger:**
[core/spec/divergent.tsv](../core/spec/divergent.tsv) · **Programme:**
[GO-TS-PARITY.0.md](GO-TS-PARITY.0.md)

Sibling: [CORE-GO-TS-DEFECTS.0.md](CORE-GO-TS-DEFECTS.0.md), the 2026-08-06
read-only defect hunt. That one found 22 defects by READING the two cores
against each other. This one found 135 by RUNNING them, and the two sets
barely overlap — which is the first thing worth recording.

## Why none of these were visible

Three instruments were green throughout, and each is blind to this class for
a different reason:

| instrument | rows | why it missed them |
|---|---:|---|
| `crossdiff` (engine) | 1808 | hard-fails only when both engines SUCCEED with different values. Most rows below are error-vs-error or error-vs-success. |
| `parser-crossdiff` | 1765 | parser-level; never reaches the step loop. |
| `core/spec` | 158 | every row was written to a shape the corpus notation could already build, and the notation could not build these. |

The fourth instrument is the one that found them: **the coverage report**.
`core/ts` was at 88.2% and `core/go` at 100%, and the parser half of this
programme had already established the rule —

> **An uncovered branch in one port is where a divergence hides**, because
> nothing has ever compared the two engines there.

Eight agents read each uncovered region of `core/ts` against its Go twin and
proposed candidate expressions; a probe stage ran them through both engines.
139 candidates, 138 well-formed, **135 divergent**. That hit rate is the
finding: the uncovered surface was not merely untested, it was *wrong*.

## The ten classes

Ordered by severity — what a user would actually suffer.

### 1. An empty paren group in the forward window — 25 rows, **ALL CLOSED**

The only class that produces a different **value** rather than a different
error. Go treats `( )` as a no-value operand: `no_value_error`, or a stack
operand supplements it. `core/ts` silently drops the empty group and
**re-associates** the operands.

```
run 7 8 addq ( ) ( 5 )      go: 15 5           ts: 7 13
run 7 negq ( ) ( 5 )        go: -7 5           ts: 7 -5
run addq ( ) ( 5 ) 6        go: no_value_error ts: 11
```

Same source, different arithmetic. Everything else here is a taxonomy or
render difference; this one was a wrong number.

**Closed.** A void group cannot fill the slot it sits in, so it STOPS
forward collection: the word falls back to stack form where a stack operand
can supply the arg, and raises `no_value_error` where none can. `core/ts`
scanned PAST it, letting the next group slide into the empty slot.

The stop then exposed a second defect it had been hiding for as long as it
existed: `preEvalParens` derived a group's result count as
`length - (before - 1)`, arithmetic that only holds for a two-token `( )`
and goes NEGATIVE for every longer group. While the void branch merely
`continue`d that was invisible; the moment it began to `break`, 22 rows in
`core/ts` and 11 in `eng/ts` went red at once. `evalParenAt` now returns the
count instead of the caller re-deriving it.

### 2. The strict forward barrier — 51 rows, **30 CLOSED**

`REFERENCE.md:364` lists "another function word" as a forward-collection
barrier, and `design/STRICT-FORWARD-BARRIER.0.md` makes it uniform: a parked
forward that cannot commit with the args it already holds is **stranded** —
a `signature_error`, not a wait-through. `core/ts` has no such rule, so at an
`Any` slot it waits through the inner dispatch and the outer word **fires**.

```
run boomq negq 5            go: signature_error   ts: fixture_boom
run boomq nosuchword        go: undefined_word    ts: fixture_boom
run boomq ( negq 5 )        both: fixture_boom          <- the grouped form AGREES
```

That last line is the point: the rule exists precisely to make the grouped
and ungrouped spellings behave differently, and in `core/ts` they did not.

**Closed for the 30 rows where the parked word has claimed NOTHING** — which
is exactly where Go's `commitBarrierForward` returns false on its first test
("Nothing collected yet — no smaller-arity dispatch to commit"), so the port
matches Go where it fires. Go's commit half, which fires a parked word that
CAN dispatch with its claimed args before declaring the barrier, is not
ported: it needs the tape rearrangement Go performs and a word carrying a
shorter real overload than the parked plan assumed.

Widening it without that half is wrong, and MEASURABLY so — the first
attempt stranded `def h fn […]` at 1-of-2 args and turned 44 `eng/ts` rows
red. The 21 rows that remain here are a different root cause anyway: an
UNDEFINED word in the forward window, where Go raises `undefined_word` and
`core/ts` collects the bare Word as data at an `Any` slot.

**That one was attempted and REVERTED**, and the attempt is worth recording
because the obvious rule is wrong. Refusing any surviving Word at a
non-Word/Atom slot closes 17 rows and breaks 7 `eng/ts` ones: `def inc
fn […]` needs the keyword word to reach its slot, and `null` has no arm in
`resolveForwardToken` (Go plans it as an Atom, `engine.go:8177-8184`, while
`match.ts` has arms for `true`/`false`/`none` and none for `null`) so it
survives as a Word and gets refused. The missing `null` arm is itself a
divergence the sweep found. Closing this class means porting Go's
forward-plan word handling properly, not adding a guard.

### 3. A type-mismatched paren operand — 14 rows, error CODE

Both refuse; they disagree about the code. Go: `signature_error` (the operand
does not fit the slot). `core/ts`: `no_value_error` (it treats the group as
having produced nothing usable). A caller matching on the code behaves
differently.

### 4. The WORD `end` versus the end MARKER — 11 rows, **9 CLOSED**

`;` is the marker and both engines handle it identically (pinned in
`dispatch.tsv`). The bare word `end` is a different thing: `core` has no word
by that name, so Go raised `undefined_word` while `core/ts` resolved the word
to the marker and silently applied barrier semantics — `run end 1` was the
value `1` here.

**Closed.** `core/ts`'s `isOpenParen` / `isCloseParen` / `isEnd` each carried
an extra "…or a Word named `(` / `)` / `end`" fallback that Go's predicates
never had — a leftover from the legacy fixture tokenizer, which produced bare
words where the parser produces markers. All three now test the vType alone.
The 9 rows moved to `dispatch.tsv`; the 2 that remain are class 2 in
disguise.

### 5. The builtin type-name table — 9 rows, **4 CLOSED**

**The crash is closed.** `Word` names a lattice branch, and its type literal
shares that branch's vType. `core/ts`'s `isWord()` tested the vType *alone*,
so `stepWord` resolved the name, wrote the literal back to the same slot, and
the step loop re-entered `stepWord` on it — ending in an uncoded
`AsWord: not a word value` that escaped the BoruError taxonomy entirely.
`isWord()` now also requires word DATA, which is exactly what separates a
word from its type.

**Still open:** `core/ts`'s type table genuinely lacks `Cidron` and `Module`
(it is a subset port of `core/go/typetable.go`), and it has no slash-PATH
form, so `Scalar/String` and `Word/__ED` are `undefined_word` here and
resolve there. A measurement worth keeping: registering every type by its
leaf name — which is literally what Go's `TypeTable.RegisterType` does —
is WRONG. It made `core/ts` resolve `__ED`, and `run __ED` is
`undefined_word` on both engines. So the lookup `stepWord` consults is a
filtered one, and the `!internal` guard in `indexDecl` belongs there.

### 6. A bare marker as a map value — 9 rows

An END or paren marker where a map value goes. Go drops the key (the marker
evaluates to nothing) or refuses the paren markers outright; `core/ts` stores
the marker as **data** and renders it. Reached through the corpus notation
today, but it is what a word handler returning a marker inside a map would do.

### 7. Canon of a bare paren marker — 8 rows, RENDER

Go canons an `OpenParen` as the **empty string** and a `CloseParen` as `)`,
so a list holding one renders with a gap; `core/ts` renders both literally.
Render quality on both sides — NUR059 territory — but they disagree, so it is
debt rather than a shared wart.

### 8. An unclosed paren in the value stream — 6 rows, error CODE

Go completes the pending dispatch and reports `signature_error`; `core/ts`
reports `syntax_error`. The core engine receives **values, not text**, so
calling this a syntax error is arguably the wrong layer — but which code is
right is a `REFERENCE.md` question, so it is recorded rather than "fixed".

### 9. BigDecimal sign and scale — 2 rows, **BOTH CLOSED**

```
bigdec -0.0                 go: -0d0.0     ts: 0d0.0    (was)
bigdec 0e5                  go: 0d000000   ts: 0d0      (was)
```

A bigint has no `-0`, so the sign of `-0.0` was lost the moment the
significand was built. `Decimal` now carries an explicit `negZero` flag
beside the coefficient, exactly as `apd` carries `Negative`. And a zero
significand short-circuited its exponent away; it now grows its trailing-zero
run like any other value, because the scale is part of the identity rather
than noise to normalise. Seven rows in `canon.tsv` pin both edges.

### 10. Error ORDER inside containers

Which of two failing map values surfaces first differs between the engines.
Folded into the ledger's map sections rather than given its own; it is a
consequence of class 2 rather than a separate rule.

## What is closed, and what is deliberately not

**80 of the 135 are closed** — the whole of classes 1 and 9, the crash in
class 5, 9 of the 11 in class 4, and 30 of the 51 in class 2. Each was a small, local defect with an
unambiguous Go twin to read against, and each moved its rows OUT of the
ledger into the spec file they belong in, which is the mechanism working as
designed.

**The remaining 55 are not fixed.** Classes 1, 2 and 5 are real feature work in
`core/ts` — the barrier is a whole rule with its own design note, the empty-
paren handling is a rewrite of the forward window's operand planning, and the
type-name table is a data gap plus a path resolver. Fixing them piecemeal
while the ledger was still growing would have meant re-measuring after every
step and losing the shape of the finding.

What IS done is the thing that makes them impossible to lose: every one is a
row in `core/spec/divergent.tsv`, both columns recorded, **each runner
asserting its own column**, and a row whose two columns become EQUAL fails.
So a fix cannot land silently, and a regression cannot either.

## The `go` column is not the reference by proof

It is the reference by convention. On the parser side, two of the five
original divergence classes turned out to have **Go** wrong. Class 7 here
(Go rendering an `OpenParen` as the empty string) and class 8 (a *syntax*
error from an engine that never sees syntax) both deserve adjudication
against `REFERENCE.md` before anyone assumes which side moves.

## What this did to coverage

Pinning the 135 rows took `core/ts` from 88.20% to **90.71%**, and
`engine.ts` from 69.08% to **76.80%** — without a line of new engine code.
That is the rule restated from the other end: the uncovered surface and the
divergent surface were the same surface.

Closing 80 of them kept it there (90.35%): a row that moves from
`divergent.tsv` to a spec file still runs, so the coverage it bought does
not come back. The small dip from 90.73% is the barrier short-circuiting
paths those rows used to walk — the rows still pass, they just reach the
refusal sooner.

## What the closures cost, and the rule they taught

Two of the four fixes broke a DOWNSTREAM module, and both for the same
reason: `eng/ts` has hand-rolled fixtures that build values the parser never
builds — a map with no `Eval` flag, words named `(` and `)`. Tightening
`core/ts` to match Go exposed them.

- The map eval-gate (previous commit) broke 3 `eng/ts` rows. **Correct fix:**
  the fixture now builds an EVAL map, which is what the parser builds.
- Dropping the word-shaped `(`/`)` fallback broke 39. **Correct fix: none
  yet** — the fallback is restored for those two and kept dropped for `end`,
  which had no such user. The asymmetry is ugly and is recorded as such,
  because the alternative was either leaving a module red or reverting a
  real parity fix.

The generalisable part: **a fixture that constructs values by hand is a
second, unversioned parser**, and every divergence it hides is invisible
until the engine stops being lenient. `eng/ts`'s fixtures are the last
users of the legacy tokenizer, and they are why `core/ts` still accepts two
spellings of a paren marker.
