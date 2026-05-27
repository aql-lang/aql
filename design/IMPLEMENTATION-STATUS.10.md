# AQL Implementation Status

Cross-reference of design documents in `lang/doc/design/` against the
current codebase. Last updated: 2026-05-06.

Filenames are suffixed with a 0–10 implementation completeness
indicator (e.g. `PLAN.10.md` is fully implemented, `MINILANG.0.md`
is design-only). The number after the dot is the indicator, not a
version.

## Recent Changes

- **String interpolation**: Backtick template strings with `${...}`
  syntax, parsed natively by jsonic using custom tokens (#BT, #IS, #TL)
  and grammar rules (interp/ielem/iexpr/ieval). Supports nested
  interpolation to any depth. See LANGREF.10.md § Literals.
- **Timer/concurrency words**: `sleep`, `timeout`, `interval`, `cancel`,
  `await` with four parallel execution modes (all, full, first, any).
  New types: `Ideal/Timeout`, `Ideal/Interval`.


## Fully Implemented

| Document | Description | Notes |
|----------|-------------|-------|
| PLAN.10.md | Core execution loop plan | All phases complete. |
| ENGINE.10.md | Stack machine, forward scanning, argument equivalence | Working. /s /f modifiers, Forth primitives. |
| ENGINE-UNIFIED-ALGO.8.md | Unified signature matching algorithm | Pending call frames, incremental constraint solving. |
| SIGNATURE-MATCHING-PSEUDOCODE.10.md | Type matching, scoring, positional/forward/prefix matching | /q modifier, fallback signatures, specificity scoring. |
| TYPES.10.md | Hierarchical type system | Any-root lattice; branches Scalar/Node/Ideal/Word/Type; degenerate roots None/Never; Ideal kinds (Object/Resource/Entity/Array/Record/Options/Error/Store/Table) + external (Time/Tensor/Fetch/Timeout/Interval). Metatypes retired; see TYPE-ORDERING.0.md for ordering. |
| TYPE-ORDERING.0.md | Value lattice, Rank scheme, Comparer cascade, type-literal-first rule | Strict total order over distinct lattice nodes; one deliberate value-level equivalence (1 ≡ 1.0). Verified by lang/spec/compare.tsv (748 rows incl. transitivity + user-defined-type coverage). |
| SIGNATURES.10.md | All 100+ builtin word signatures | Full match-order signatures with returns and notes. |
| LANGREF.10.md | Language reference for all builtin words | All documented words registered in native_*.go. |
| IMPORTS.10.md | Module system: descriptors, file imports, renaming | Bare name resolution, `aql:` prefix, isolation. |
| NATIVE-MODULES.10.md | Native Go modules, `aql:math` | Module loading, dot-notation access, FnDef auto-invocation. |
| FILE-ACCESS.10.md | read/write words, FileOps interface | CSV/TSV/JSON/jsonic/text, options maps, stdin/stdout/stderr. |
| FOR-LOOP-REVIEW.10.md | For-loop design review | Sentinel errors for break/continue, mark/move, lazy ForCont. |

**109 native words** across: stack manipulation (15), math (6),
boolean logic (6), string ops (15), comparison (7), type system (11),
control flow (5), definition/scoping (9), array ops (23),
higher-order array (5), storage/context (3), file I/O (5),
printing (2), modules (3), accessor (1), debug (2), unify (1),
timer/concurrency (6: sleep, timeout, interval, cancel, await, now).


## Partially Implemented

### ARRAYIFICATION.6.md — ~60%

**Done:** `iota`, `reshape`, `flatten`, `transpose`, `take`, `shed`,
`reverse`, `each`, `fold`, `scan`, `outer`, `inner`, `where`,
`unique`, `grade`, `window`, `pairs`, `group`, `replicate`,
`expand`, `at`, `sortby`, `member` (23 words). Object/Array type
with SQLiteStore backing.

**Missing:**
- Broadcasting rules (implicit iteration over mismatched shapes)
- Rank polymorphism (`eachrank`, `foldaxis`)
- `compress` (boolean mask selection, separate from `where`)
- Phases 4-5 from the design doc (broadcasting, advanced composition)


## Not Implemented

### DATAFRAME-WORDS.0.md — 28+ words

SQL-style tabular data manipulation. Planned words:

| Category | Words |
|----------|-------|
| View | `head`, `tail`, `shape`, `cols`, `nrow`, `ncol`, `describe` |
| Columns | `col`, `pick`, `omit` |
| Filter | `sift` (pattern match or predicate per row) |
| Modify | `mutate`, `rename` |
| Sort | `sortby` (with direction) |
| Aggregate | `sum`, `mean`, `count`, `min` (extend), `max` (extend) |
| Group | `groupby` |
| Join | `merge` |
| Combine | `stack` (vertical concatenation) |
| Missing | `dropna`, `fillna` |
| Duplicates | `dedup`, `dupes` |
| Reshape | `melt`, `pivot` |
| Apply | `apply` (per column/element) |
| Row access | `row`, `slice` (extend) |

### MATRIX-WORDS.0.md — 83 words + 22 overloads

Linear algebra operations (gonum dependency). Planned words:

| Category | Words |
|----------|-------|
| Construction | `matrix`, `mat-zeros`, `mat-ones`, `mat-eye`, `mat-diag`, `mat-fill`, `mat-rand`, `mat-randn`, `mat-linspace`, `mat-range`, `mat-from-cols`, `mat-from-table` |
| Shape/info | `mat-rows`, `mat-cols`, `mat-shape`, `mat-size`, `mat-square?`, `mat-symmetric?` |
| Element access | `mat-at`, `mat-set`, `mat-row`, `mat-col`, `mat-diag-of`, `mat-slice`, `mat-set-row`, `mat-set-col` |
| Arithmetic | `add`/`sub`/`mul`/`div` (extend), `mat-emul`, `mat-ediv` |
| Element-wise math | 22 overloads of existing words (`abs`, `sqrt`, `sin`, `cos`, etc.) |
| Transpose/reshape | `mat-t`, `mat-reshape`, `mat-flatten`, `mat-hstack`, `mat-vstack`, `mat-squeeze` |
| Linear algebra | `mat-det`, `mat-inv`, `mat-trace`, `mat-rank`, `mat-norm`, `mat-cond` |
| Decompositions | `mat-lu`, `mat-qr`, `mat-svd`, `mat-svd-vals`, `mat-chol`, `mat-eigen` |
| Solving | `mat-solve`, `mat-lstsq`, `mat-pinv` |
| Aggregation | `mat-sum`, `mat-mean`, `mat-min`, `mat-max`, row/col variants |
| Comparison | `mat-eq?`, `mat-close?`, `mat-gt`, `mat-lt`, `mat-any?`, `mat-all?` |
| Advanced | `mat-dot`, `mat-cross`, `mat-outer`, `mat-kron`, `mat-conv`, `mat-apply`, `mat-map-row`, `mat-map-col` |

### TEMPORAL-WORDS.1.md — ~70 words (partially implemented)

Date/time types and operations. Types implemented: Instant, DateTime,
Date, TimeOfDay, CalDuration, ClkDuration, Timezone, Timeout, Interval.
Core timer words implemented: `now`, `sleep`, `timeout`, `interval`,
`cancel`, `await`. Remaining ~64 temporal module words not yet implemented.

Per the Step 11 resolution of TYPE-DECOUPLING.0.md, free-form
text/ISO parsing was **removed** as a feature rather than reimplemented.
Construction is via numeric (`unix` / `unix-ms` / `unix-ns`) or
wall-clock (`now-local` / `today` / `today-utc`); output-only
formatting is via `to-string` / `to-iso` / `format`. The removed
words appear stricken through below.

| Category | Words |
|----------|-------|
| Construction | ~~`date`~~, ~~`datetime`~~, ~~`instant`~~, ~~`time-of-day`~~, `tz`, `unix`, `unix-ms`, `unix-ns`, `cal-dur`, ~~`duration`~~ (ISO-form) |
| Current time | `now`, `now-local`, `today`, `today-utc`, `elapsed` |
| Extraction | `year`, `month`, `day`, `hour`, `minute`, `second`, `nanosecond`, `weekday`, `weekday-name`, `month-name`, `year-day`, `iso-week`, `quarter`, `days-in-month`, `days-in-year`, `is-leap-year`, `to-unix`, `to-unix-ms` |
| Duration | `years`, `months`, `weeks`, `days`, `hours`, `minutes`, `seconds`, `ms`, `us`, `ns`, `total-hours`, `total-minutes`, `total-seconds`, `total-ms`, `dur-years`, `dur-months`, `dur-days`, `dur-sign` |
| Arithmetic | `add`/`sub` (extend), `until`, `since`, `diff` |
| Comparison | `is-before`, `is-after`, `eq` (extend), `compare`, `is-between`, `earliest`, `latest` |
| Conversion | `to-date`, `to-time-of-day`, `to-datetime`, `to-instant`, `to-local`, `to-utc`, `to-string`, `format`, `to-iso` |
| Rounding | `round`/`truncate` (extend), `start-of`, `end-of` |
| Timezone | `tz`, `tz-utc`, `tz-local`, `tz-name`, `tz-offset`, `is-dst` |
| Parsing | ~~`parse-date`, `parse-datetime`, `auto-date`~~ (removed — see TYPE-DECOUPLING.0.md Step 11) |

### MINILANG.0.md — 10+ inline DSLs

Mini-language literals with `xy/...` two-letter prefix syntax.
Requires lexer integration for prefix detection.

| Prefix | Language | Purpose |
|--------|----------|---------|
| `rm/` | Regexp match | Return match structure |
| `rs/` | Regexp substitute | Backreference replacement |
| `rt/` | Regexp test | Boolean match test |
| `rf/` | Regexp find-all | List of all matches |
| `xp/` | XPath | XML querying |
| `jp/` | JsonPath | Map/list querying |
| `jq/` | jq | Filter expressions |
| `cs/` | CSS selectors | DOM-style selection |
| `tr/` | Transliterate | Perl tr/y semantics |
| `fm/` | Format | String interpolation with `{}` |
| `sh/` | Shell pattern | POSIX glob matching |
| `gl/` | Glob | File glob matching |
| `ur/` | URL pattern | URL template matching |
| `dt/` | Date/time format | Temporal formatting |

### GENERICS.0.md — generic types

Algebraic generics with concatenative core and a sugar layer. No
implementation yet — design draft only.

### XML.0.md — XML format and embedding

Tree-structured XML alternate concrete syntax for AQL programs and
data, with `${...}` interpolation, CSS-selector querying (`cs/`),
and `<aql-embed lang="...">` for embedding foreign syntaxes.


## Reports and Reviews

These documents review existing implementation rather than propose
new features. Their completeness suffix reflects how much of what
they describe or recommend is in the codebase.

| Document | Topic |
|----------|-------|
| AQL-CODE-REVIEW-REPORT.6.md | Architecture, safety, duplication review; most fixes landed, some remain. |
| BATTERIES-INCLUDED-REPORT.5.md | Standard-library coverage analysis; partial uptake. |
| CARRIER-STATIC-TYPECHECK-REPORT.10.md | Carrier-based static type checking — implemented. |
| TYPE-SYSTEM-REVIEW.7.md | Algebraic + dependent type review; majority resolved. |
| AQL-DX-REPORT.5.md | DX feedback from `aql:decision`; several issues open. |
| jsonic-matcher-rule-access-report.10.md | Jsonic LexMatcher rule access; landed in jsonic v0.1.6. |
| aql-boolean-operations-report.10.md | Boolean ops review; ops are implemented. |
| aql-bytecode-outline.0.md | Bytecode AOT compilation outline. |
| aql-bytecode-report.0.md | Full bytecode compilation feasibility report. |
| amop-in-aql-report.0.md | Ambient-Oriented Programming feasibility. |
| fsharp-units-in-aql-report.0.md | F# units of measure feasibility. |


## Reference Documentation

These describe the language itself; their completeness reflects
parity with the codebase, not pending work.

| Document | Topic |
|----------|-------|
| LANGREF.10.md | Language reference — all builtins. |
| SIGNATURES.10.md | Builtin word signatures. |
| TYPES.10.md | Type system — Any-root lattice, branches, kinds. |
| TYPE-ORDERING.0.md | Comparison total order — `cmp`, `sort`, `lt`/`gt`. |
| ENGINE.10.md | Stack machine. |
| IMPORTS.10.md | Module / import system. |
| NATIVE-MODULES.10.md | Native Go modules. |
| FILE-ACCESS.10.md | File I/O API. |
| SAMPLES.10.md | Code samples. |
| tutorial.10.md | Diataxis tutorial. |
| how-to.10.md | Diataxis how-to. |
| reference.10.md | Diataxis reference. |
| explanation.10.md | Diataxis explanation. |


## Open Design Issues

From AQL-DX-REPORT.5.md (building `aql:decision`):

| Priority | Issue | Status |
|----------|-------|--------|
| P0 | List auto-eval strips def references, blocking composition | Open |
| P0 | Def leakage from fn bodies via CallAQL | Partially addressed (undefined word errors help) |
| P1 | Arg ordering confusing across prefix/forward/FnDef contexts | Open |
| P1 | Registered words shadow map keys in dot notation | Open |
| P2 | No ergonomic list-building word | Open |
| P2 | FnDef words cannot forward-collect like builtins | Open |


## Summary

| Area | Status | Word Count |
|------|--------|------------|
| Core engine, types, signatures | Complete | 103 implemented |
| Array processing | ~60% | ~23 of ~40 |
| Dataframe operations | Not started | 0 of ~28 |
| Matrix / linear algebra | Not started | 0 of ~105 |
| Temporal / date-time | Not started | 0 of ~70 |
| Mini-languages | Not started | 0 of ~14 prefixes |
| **Total** | | **~103 of ~360** |
