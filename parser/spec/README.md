# parser/spec — the parser-level parity corpus

The declarative contract `parser/go` and `parser/ts` are both held to. One set
of files, two independent runner pairs (`parser/go/parserspec_test.go` and
`parser/ts/src/parserspec.test.ts` for parsed values;
`parser/go/lexspec_test.go` and `parser/ts/src/lexspec.test.ts` for raw
tokens), no shared reader or renderer code between ports.

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

### `lex.tsv` — the formatter-facing token contract

```
src	OK|ERR	canonical token JSON
```

Exactly three columns are required. `OK` means the lexer reached clean EOF;
`ERR` means it stopped at a lexical error and its returned prefix is
untrustworthy. Each token records its kind, raw source, and `si` as a UTF-8
byte offset. The canonical JSON field order is `name`, `src`, `si`.

This deliberately has separate Go and TypeScript readers and renderers rather
than extending the parsed-value harness: raw tokens retain whitespace,
comments, newlines, and boundaries which `parse.tsv` necessarily discards.
Its 27-row exact ratchet covers empty and bad EOF, an unterminated backtick's
clean lexical EOF, trivia, astral-character byte offsets, decimal and dotted
exponents, number/member and colon/arrow suffix boundaries, malformed
separators, hexadecimal/octal/binary overflow boundaries, and adjacent quote
boundaries after plain words and completed receivers.

### `parse.tsv` — the contract

```
src	expected	optional note
```

Exactly three columns are required; the note column is always present and `-`
means no note. This makes an unescaped tab an extra field that fails loudly
instead of silently truncating `expected`. `expected` is the canon of the parsed value
stream (each value rendered, space-joined), or `ERR ` and the first line of
the error text. **Both engines must produce it.**

### `divergent.tsv` — the parity debt

```
src	go	ts	note
```

Sources the two engines render differently. The four-column reader remains so
a probe result records both sides and a nonblank justification before review,
but **the committed row-count ratchet is zero**: either runner fails if any
data row exists. Full parser parity is a hard invariant, so a measured
difference must be fixed and moved to `parse.tsv`, not accepted as green debt.

The file is therefore **zero-only** in a passing tree. The justification field
is still mandatory when inspecting a probe-produced row, so even a deliberately
red diagnostic change cannot lose the reason it exists.

### `nesting.tsv` — the generated-depth contract

```
kind	depth	expected outcome
```

Exactly three columns are required. Rather than committing enormous source
and render strings, each runner independently expands `kind` (`list`, `map`,
`paren`, `typed-list`, `typed-map`, or a mixed list/map/paren spine) to the
requested source-container `depth`. `expected outcome` is `OK` or
`ERR evaluation_limit`.

Successful deep values are deliberately not canonically rendered: recursive
rendering has its own host-stack limit and is outside this parser contract.
Instead, each runner iteratively walks all 501 or 10,000 containers, checking
the exact list/map/paren/typed-container kind, singleton edge, map key, and
terminal value. A parser that accepts the source but truncates or flattens the
result therefore fails. The rows pin both the depth-501 acceptance that the old
TypeScript-only guard violated and the exact shared 10,000-frame boundary.
Root parentheses accept only 9,999 source groups because their implicit
top-level item conversion is also a frame.

This contract replaces the former nesting row in `divergent.tsv`: the
TypeScript converter now walks nested parser nodes on an explicit work stack,
so it reaches the same boundary as Go without depending on the JavaScript host
call stack.

### `shape.tsv` — the semantic-shape contract

```
case	src	expected semantic shape
```

Exactly three columns are required, case names are unique, and `src` and the
expected shape use the same escape notation as `parse.tsv`. Each runner owns a
separate strict reader and recursive renderer.

This is the deliberately non-canonical half of the parser oracle. Canonical
source is retained as a compact payload label, but every rendered value also
records its source position, `Eval` and `Quoted` flags, and word payload
(`ArgCount`, `/s`, `/f`, `/r`, and `/u`). Lists, maps, parentheses, and reach
nodes are traversed so nested and operator-generated positions cannot hide
inside an equal outer canon. Map rendering also records the implicit-pair bit
and computed-key set; typed containers expose their child constraint plus
concrete elements/entries; sugar exposes every `SugarInfo` field, including
the angle head and deferred head error. The modifier rows cover `/N`, `/f`,
`/s`, `/r`, `/u`, `/ur`, and `/q`; the last is represented by the Atom payload
it is specified to emit. Parser-authored values do not set `Quoted` — quoted
source becomes a String or Atom — so the corpus pins that flag false while
pinning both false and true states of `Eval`. Unicode rows pin columns after a
plain value and after the minilang, template, and XML custom matchers. Columns
are 1-based Unicode code points, never UTF-8 bytes or JavaScript UTF-16 units.

Error shapes record the structured `BoruError` fields rather than its rendered
first line: code, detail, row, column, offending source, full source, hint,
notes, and help suggestions. This catches diagnostic parity drift even when
the two user-facing first lines still compare equal in `parse.tsv`.

## The current debt

**None.** `divergent.tsv` is header-only, and both runners keep the empty
ledger as a hard zero-row ratchet. The eleven rows found after the ledger first
reached zero were closed rather than accepted:

- Eight rule-step-limit shapes now retain the exact offending `]`/`}` token
  and its position from the TS rule subscriber; before the guard, TS silently
  accepted several of them.
- Two decimal/underscore lexer-boundary rows (`1.2_`, `1_.2`) now reach one
  whole numeric token in both ports through matching, narrowly scoped
  boundary matchers.
- The depth-501 row became the stronger generated `nesting.tsv` matrix after
  TypeScript conversion was made stack-safe through the shared 10,000 limit.

The independent numeric sweep also exposed a shared language-contract bug:
both ports admitted `_` next to a prefix/exponent marker even though
REFERENCE.md permits it only between digits. Both validators now check actual
digits in the literal's base, and the final 66-case matrix lives in
`parse.tsv`.

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

The main corpus grew from 370 rows to 648 unique sources while driving
`parser/ts` from 93.97% to 100% line coverage; the final gate also requires
100% branches and functions. Both runners machine-pin the current corpus sizes:
648 parse rows, 27 raw-lexer rows, 18 generated-depth rows, 26 semantic-shape
rows, and zero divergence rows. An intentional corpus change updates both
independent constants and this prose together. The growth found further defects the
crossdiff could not:

- an EMPTY `${}` interpolation in an XML **attribute** folded to `""` in Go
  and stayed a runtime hole in TS — Go's nil/empty conflation, mirrored
  anyway because parity with the shipped language is the contract;
- the eight rule-step-cap shapes above, where TS silently accepted programs
  Go rejects;
- an error raised while CONVERTING `${…}` rendered its two halves in the
  opposite order (`interpolation expression error:` before or after
  `[boru/float_overflow]:`).

Each was found by sweeping the regions the coverage report called
uncovered, which is the general lesson: **an uncovered branch in one port is
where a divergence hides**, because nothing has ever compared the two there.
