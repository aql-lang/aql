# GO-TS-PARITY.0 — full functional parity on core, parser and basic

**Status:** IN PROGRESS (started 2026-08-08) · **Instruction:** "full
functionality parity go and ts on modules core, parser and basic, with
100% test coverage on go and ts", and — the steer that shapes the whole
approach — "use shared tsv spec files as much as possible to establish
parity".

Sibling notes: [TS-PARITY-AUDIT.0.md](TS-PARITY-AUDIT.0.md) (the audit
that built the parser stream oracle), [CORE-TS-COVERAGE.0.md](CORE-TS-COVERAGE.0.md),
[BASIC-CHECK-CUT.0.md](BASIC-CHECK-CUT.0.md) (the dependency cut that
preceded this).

## The measurement, and why the obvious one misleads

| module | go | ts | shared corpus |
|---|---|---|---|
| core | 100% | 88.13% | `core/spec`, 84 rows |
| parser | 100% | 93.85% | `parser/spec`, 370 rows, ledger 1 row (a runtime limit) |
| basic | 100% | 100% *of the 15 words ported* | `basic/spec`, 45 rows |

Two numbers that look like progress and are not:

- **`basic/ts` at 100%** is 100% of the 15 words ported (the stack
  vocabulary plus `do`), not of basic's surface. The floor is a ratchet on
  the SURFACE, not on the percentage.
- **Both crossdiffs agree on every row** — `parser-crossdiff` IDENTICAL
  over 1765, `crossdiff` 1808 agree / 0 divergences — and did so on day
  one. That is not evidence of parity. It means the engines do not
  disagree about what the corpora COVER, so **the parity gap is the
  uncovered surface**, and growing the corpora is the instrument rather
  than a formality.

The honest surface measure is the export gap: core/go exposes ~2,989
functions and types, core/ts ~192. parser is the exception — 4,380 Go
source lines against 5,021 TS — which is why parser reached an empty
divergence ledger inside a day and core will not.

## What the corpora are, and the rule that makes them work

Three corpora, each read by TWO runners that share **no code**:

| corpus | go runner | ts runner |
|---|---|---|
| `core/spec` | `core/go/corespec_test.go` | `core/ts/src/corespec.test.ts` |
| `parser/spec` | `parser/go/parserspec_test.go` | `parser/ts/src/parserspec.test.ts` |
| `basic/spec` | `basic/go/basicspec_test.go` | `basic/ts/src/basicspec.test.ts` |

The independence is the point: shared scaffolding hides the same bug from
both engines. Each runner re-implements the row notation and builds its
own fixture registry, and any asymmetry between the two fixtures shows up
as a false divergence — so the fixtures are kept in step deliberately.

`parser/spec/divergent.tsv` is the parity DEBT ledger: one row per
divergence, both columns recorded, each runner asserting its OWN column.
Shrink-only. A fixed divergence MOVES to `parse.tsv` rather than being
deleted, and a row whose two columns are equal FAILS — otherwise the file
stops being an honest debt list. It reached zero on 2026-08-08, then took
one row back when the nesting-depth divergence was measured — a limit the
JS runtime imposes rather than a defect either port can fix.

`scripts/parity-probe.sh` is how a row gets written: it runs a candidate
through both engines and prints AGREE with the shared render, or DIFFER
with both. **Authoring an expected column from one engine's behaviour is
how a divergence gets baselined as a contract**, which is the failure
this whole apparatus exists to prevent.

## What the probe found that the crossdiff could not

Recorded because the pattern generalises: a differential that hard-fails
only when both engines SUCCEED with different values is blind to two
engines that both fail, or both render debug output.

| defect | why crossdiff missed it |
|---|---|
| `TBigInteger`/`TBigDecimal` had types but no constructor and no render arm in core/ts | both engines "succeeded"; the render was wrong on one |
| `1e400` → `inf` in TS, `float_overflow` in Go | a GAP (one errors), which crossdiff permits |
| typed containers dropped the tag (Go) / leaked `word(...)` (TS) | both rendered; neither was source |
| `1 ;` canon'd to a bare `1` in Go, barrier silently gone | both "succeeded" |
| `/q` marker: `word()({false true})` vs `word(undefined)` | both erroring-by-rendering |
| stray `)` dropped to empty in Go | both "succeeded" |
| `1_e5` → 100000 in Go, REJECTED in TS | a GAP, permitted |

In two of five original divergence classes the **`go` column was not the
reference**. The ledger header warned that it was "the reference by
convention, not by proof"; that warning was load-bearing.

## The blocking structure (the finding that scopes the rest)

`basic/ts` cannot advance much further on its own. Every remaining word
is gated on a core/ts capability that does not exist yet. Measured, not
guessed:

| basic word group | needs from core/ts | present? |
|---|---|---|
| stack (14 words) | value constructors, `returnsIdentity`, `fullStack` | **DONE** |
| `do` (runtime half) | a sub-engine run, `newErrorValue` | **DONE** |
| `const` | member types (`MintMemberType`), the type table, `reparentValue`, `canonicalType`, `BoruErrorHint` | no |
| `do` / `if` / `case` / `for` | the CARRIER LATTICE — `joinCarriers`, `runCarrierBody*`, `applyGuardNarrowing` — for their analysis halves | no |
| `def` / `undef` / `var` / `fn` / `afn` / `fnsig` | `installType`, `installFnDef`, the def store's type bindings | no |
| type-generics (`gen`/`extends`/`default`/`of`) | schemas, type params, instantiation | no |
| content types (temporal, micron, bytes, handles, resource) | `makeObject`, class/resource instances, Behaviors, the capability table | no |

So the order is forced: **core/ts first, basic/ts follows.** Each
increment is (1) port a core capability, (2) pin it with `core/spec` rows
in both runners, (3) port the basic words it unblocks, (4) pin those with
`basic/spec` rows. `fullStack` is the worked example — core/ts had no
such dispatch mode, so `depth`/`pick`/`roll` waited for it, and the
corpus carried no rows for them in the meantime. **Writing rows against a
capability that does not exist encodes the gap as a contract**; the
absence of those rows was itself the honest record.

## Deviations that are not bugs

- **BigDecimal scale.** Go's payload is an `apd.Decimal`, TS's was a
  binary64 — so `0d1e400` overflowed to an unparseable `0dInfinity` and
  `0d1e-400` underflowed to **zero**, silently. Closed by giving core/ts
  an exact decimal. Recorded here because the first attempt documented it
  as "trailing-zero scale alone", which was false, and the review caught
  it: a deviation note that understates the deviation is worse than none.
- **Uncoded errors.** `pick`/`roll` out of range raise `fmt.Errorf` in
  Go, so they surface as `non_boru`. The TS port throws a plain `Error`
  to match EXACTLY. Giving TS a coded error would look like an
  improvement and be a divergence. That the rest of the layer is coded
  and these two are not is a non-uniformity in its own right.
- **Value identity.** Go's `ReturnsIdentity` mints a fresh `Value.ID` for
  a duplicated source index so `dup`'s outputs stay distinct for the
  bytecode emitter. core/ts Values carry no ID and there is no TS
  compiler to consume one, so that half is absent rather than stubbed.
- **Nesting depth.** Go guards at 10,000 levels; TS refuses at 500,
  because the tabnas rule engine recurses per level and blows the JS call
  stack near 900 — before any converter counter can fire. The TS bound
  converts an uncontrolled `RangeError` into the promised
  `evaluation_limit`. Measured while recording it: 600 parses, 1,000
  overflows. This is the one divergence that is a property of the RUNTIME
  rather than of either port, and the only remaining row in
  `parser/spec/divergent.tsv`. Closing it means making the TS parser
  iterative, not raising a constant.

## Coverage, and where it must come from

Both Go modules gate at 100% by their OWN suite (`cover-gate-core`,
`cover-gate-parser`), on top of the merged ADR-008 gate. The TS gates
ratchet: `TS_CORE_GATE_LINES` (88), `TS_PARSER_GATE_LINES` (92),
`TS_BASIC_GATE_LINES` (100, a surface ratchet).

The discipline that matters: **coverage comes from corpus rows, not from
per-engine unit tests.** When core/go's canon grew arms for typed
containers, the marker values and the dispatch modifier, `cover-gate-core`
went red — and the fix was `core/spec` rows with new expression kinds
(`bigint`, `bigdec`, `end`, `closeparen`, `typedlist`, `typedmap`,
`dispatchmod`) in both runners, not Go tests. Every such row lifts both
engines at once, which is the whole reason the corpus is the instrument.

One consequence worth stating: a row can legitimately fail on BOTH
engines. `core/spec` and `basic/spec` are SPECS, not differentials — the
expected column is the documented contract — which is exactly the defect
class an agreement-only corpus is structurally blind to.

## Open

- core/ts to 100%: `engine.ts` (~68%) is the bulk.
- parser/ts to 100%: the residue is in `convert.ts` and `grammar.ts`.
- basic/ts: everything past the stack vocabulary, in the order the table
  above forces.
- NUR059: canon still renders sugar tags, `/r` and `/N` word modifiers,
  and paren groups in debug spelling. Both engines agree, so it is render
  quality rather than parity — pinned by corpus rows so a one-sided fix
  fails loudly.
