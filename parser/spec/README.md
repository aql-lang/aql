# parser/spec — the parser-level parity corpus

The declarative contract `parser/go` and `parser/ts` are both held to. One set
of files, two runners (`parser/go/parserspec_test.go`,
`parser/ts/src/parserspec.test.ts`), no shared code between them.

## Why it exists

`parse-battery.test.ts` opened by declaring "The Go parser is the REFERENCE:
every construct converts to the same value stream." Nothing checked that.
When it was checked for the first time, **15 of 254 rows encoded a render
`parser/go` does not produce** (design/TS-PARITY-AUDIT.0.md) — the battery
had been pinning TypeScript's own behaviour, and the twins could drift with
no gate noticing.

The other twin pairs already had a shared corpus — `eng/spec` for the
engines, `core/spec` for the cores. The parser was the only pair without one.

A parser-level oracle *was* designed: `parser/go/streamdump_test.go` and
`parser/ts/src/streamdump.ts` both dump the value stream for every `eng/spec`
row. Nothing ever ran the comparison — no target, no CI step — so with
`STREAMDUMP_FILE` unset the Go side dumps to a temp dir and discards it.
`make crossdiff` does compare the two *engines*, but hard-fails only when
both produce a value and the values differ, so a parser-level difference that
still evaluates alike is invisible to it.

## Two runners, no shared code

Each runner reads the files and implements the escape decoding and the render
independently, in about forty lines. That is the same discipline `core/spec`
uses, for the same reason: shared scaffolding can hide one bug from both
engines (design/CORE-GO-TS-DEFECTS.0.md, blind spot 9), and a shared reader
would hide exactly the class of defect this corpus was built to catch.

## Format

Tab-separated. A line is a **comment** only when it starts with `#` *and*
contains no tab — `#` is boru's own comment marker, so sources begin with it,
and treating every `#` line as a comment silently drops those rows. Blank
lines are ignored; a whitespace-only source is a real row, not a blank one.

Every column is escaped (`\n`, `\t`, `\\`) so one row is exactly one line. A
render can itself contain a newline — XML text spanning lines does — and an
unescaped one splits the row and truncates it silently. Both failure modes
above were live bugs during this corpus's construction, which is why both are
called out here.

### `parse.tsv` — the contract

```
src	expected
```

`expected` is the canon of the parsed value stream (each value rendered,
space-joined), or `ERR ` and the first line of the error text. **Both engines
must produce it.**

### `divergent.tsv` — the parity debt

```
src	go	ts	note
```

Sources the two engines render differently. Each runner asserts its **own**
column, so both suites stay green while the difference stays pinned: if either
engine moves, its column fails and the row must be re-examined rather than
silently re-baselined. Each runner also **fails a row whose `go` and `ts`
columns are equal** — a fixed divergence must move to `parse.tsv`, or this
file stops being an honest debt list.

The file is **shrink-only**. Adding a row needs the justification a
`//covergate:allow` does (design/COVERAGE-ALLOWLIST.10.md): a reviewed reason
the difference is not simply a bug to fix.

## The current debt

**Zero.** Both engines agree on every row of `parse.tsv`, and
`divergent.tsv` is empty.

That is the goal state, not a broken corpus: both runners assert it
explicitly, so a NEW divergence has to be added here deliberately, with the
justification the header demands, rather than quietly landing as a changed
expectation somewhere else.

## History

The ledger held **15 of 254 rows** when it was first checked, and was driven
to zero on 2026-08-08. Measuring what had never been measured then added
three BigDecimal rows, and those were closed too — by giving `core/ts` a
real arbitrary-precision decimal (`core/ts/src/decimal.ts`, a scaled bigint
on apd's model) in place of the binary64 payload that could not represent
what Go represented. What the original fifteen turned out to be:

| class | rows | resolution |
|---|---:|---|
| big-decimal canon | 8 | `core/ts` had `TBigInteger`/`TBigDecimal` as types with **no constructor and no render arm** — every big number lost its `0d` marker |
| typed-container canon | 3 | **both** engines wrong: Go dropped the element-type tag, TS leaked `word(...)`. `REFERENCE.md:228`/`:1224` settle it as `[:Integer]` |
| marker canon | 1 | **TS was right**: `REFERENCE.md:415` makes `end` the word and `;` its synonym; Go rendered it empty, so `1 ;` reparsed as a bare `1` |
| error text | 1 | the two jsonic ports disagree about whether `1_` lexes as a number; TS's fallback now reproduces Go's classification |
| behavioural | 2 | `1e400` → TS had the `float_overflow` refusal but only on a path `1e400` never took |

| BigDecimal range/scale | 3 | found later by measurement; `core/ts` had a binary64 payload, so `0d1e400` overflowed to Infinity, `0d1e-400` underflowed to **zero**, and `0d0.30` lost its scale |

Worth keeping in view: the `go` column was **not** the reference in two of
the five original classes. The header's warning that it is "the reference by
convention, not by proof" was load-bearing.

Two of the eighteen were invisible to the 1765-row `parser-crossdiff`,
because both engines were erroring-by-rendering rather than disagreeing on a
value. `scripts/parity-probe.sh` is what found them.
