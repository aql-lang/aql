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

15 rows, all **untriaged** — recorded, not yet adjudicated against
`REFERENCE.md`. The `go` column is the reference by convention, not by proof;
either engine may turn out to be the wrong one.

| class | rows | |
|---|---:|---|
| big-decimal canon | 8 | Go keeps the `0d` prefix, TS drops it |
| typed-container canon | 3 | Go drops the tag, TS keeps it |
| marker canon | 1 | `;` → Go renders empty, TS renders `end` |
| error text | 1 | trailing `_` in a numeric literal |
| **behavioural** | **2** | `1e400` → Go raises `float_overflow`, TS returns `inf` |

The last row is the one that matters most: the engines disagree about whether
a program is *valid*, not about how to print it. The first twelve are canon
divergences and belong to `core`, not the parser — they are recorded here
because this is where they were found.
