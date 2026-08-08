# CORE-TS-DIVERGENCES.1 — 135 measured core-level divergences, and where they hid

**Status:** MEASURED and PINNED, not fixed (2026-08-08) · **Ledger:**
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

### 1. An empty paren group in the forward window — 25 rows, WRONG ANSWER

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
render difference; this one is a wrong number.

### 2. The strict forward barrier — 51 rows

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
and ungrouped spellings behave differently, and in `core/ts` they do not.
This is the largest class and the one with a written design note behind it,
so it is the clearest "TS is wrong".

### 3. A type-mismatched paren operand — 14 rows, error CODE

Both refuse; they disagree about the code. Go: `signature_error` (the operand
does not fit the slot). `core/ts`: `no_value_error` (it treats the group as
having produced nothing usable). A caller matching on the code behaves
differently.

### 4. The WORD `end` versus the end MARKER — 11 rows

`;` is the marker and both engines handle it identically (pinned in
`dispatch.tsv`). The bare word `end` is a different thing: `core` has no word
by that name, so Go raises `undefined_word` while `core/ts` resolves the word
to the marker and silently applies barrier semantics.

```
run end 1                   go: undefined_word    ts: 1
```

### 5. The builtin type-name table — 9 rows

`core/ts`'s leaf name table is narrower than `core/go`'s (`Cidron`, `Module`
resolve there and not here) and it has no slash-path form (`Scalar/String`,
`Word/__ED`). **`Word` is worse than a miss**: `core/ts` throws an uncoded
`AsWord: not a word value`, a `non_boru` failure escaping the error taxonomy
entirely. That is the one row here that is a defect on any reading.

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

### 9. BigDecimal sign and scale — 2 rows

```
bigdec -0.0                 go: -0d0.0     ts: 0d0.0
bigdec 0e5                  go: 0d000000   ts: 0d0
```

`core/ts/src/decimal.ts` loses the sign of negative zero and normalises away
a zero's exponent, where Go's `apd` payload keeps both. Small, and both are
the decimal type's whole reason for existing: an exact representation that
renders what it was given.

### 10. Error ORDER inside containers

Which of two failing map values surfaces first differs between the engines.
Folded into the ledger's map sections rather than given its own; it is a
consequence of class 2 rather than a separate rule.

## What is deliberately NOT done here

**None of these are fixed.** Classes 1, 2 and 5 are real feature work in
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
