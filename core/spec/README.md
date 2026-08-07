# core/spec — the core-level parity corpus

The declarative contract `core/go` and `core/ts` are both held to. One set of
files, two runners (`core/go/corespec_test.go`, `core/ts/src/corespec.test.ts`),
no shared code between them.

## Why this is not `eng/spec`

`eng/spec` is source text. Replaying it needs a parser, and neither core has
one — that is the whole point of the core cut. So the rows here are written in
a deliberately tiny, parser-free notation that each runner implements
independently in ~40 lines.

More importantly, `eng/spec` is an **agreement set**: rows are added where the
two engines already agree, so it cannot see a construct one of them never
implemented. design/CORE-GO-TS-DEFECTS.0.md documents 22 confirmed defects it
is green across, and traces every one of them to that property.

These rows are written from the **documented contract** (REFERENCE.md,
design/TYPES.10.md, design/INTEGER-OVERFLOW-STRATEGY.5.md) rather than from
either implementation's behaviour. The `expected` column is the oracle. When
an engine disagrees with a row, the engine is wrong — that is the difference
between a spec and a differential, and it is why a row here can fail on
*both* engines at once.

## Format

Three tab-separated columns, `#` starts a comment line:

```
expr	expected	note
```

`expr` is `<kind> <argument>`:

| kind | argument | builds |
|---|---|---|
| `int` | a decimal integer | an Integer value |
| `str` | the rest of the line, raw | a String value |
| `bool` | `true` / `false` | a Boolean value |
| `none` | — | the None value |
| `typelit` | a builtin type name | that type's literal value |
| `list` | space-separated tokens | a List of them |
| `run` | space-separated tokens | runs them through the step loop |

`run` tokens: a bare decimal is an Integer, `'…'` is a String, a known builtin
type name is that type's literal, and anything else is a Word. The runners
register one fixture word, `addq` (`Integer Integer -> Integer`), so the step
loop, the registry, signature matching and dispatch are all exercised without
pulling in a word library.

`expected` is the canonical rendering (`core.Canon` / `canon()`), or
`ERROR:<code>` for a row that must raise that BoruError taxonomy code.

## What is deliberately absent

Rows for the 22 defects in design/CORE-GO-TS-DEFECTS.0.md are **not** here
yet. They would fail today — several on both engines — and a corpus that
ships red is a corpus people learn to ignore. That document lists each one
with the row text that catches it; the rows land with the fixes.
