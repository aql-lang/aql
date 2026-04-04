# AQL Implementation Status

Cross-reference of design documents in `aql/doc/` against the
current codebase. Last updated: 2026-04-04.


## Fully Implemented

| Document | Description | Notes |
|----------|-------------|-------|
| PLAN.md | Core execution loop plan | All phases complete. |
| ENGINE.md | Stack machine, forward scanning, argument equivalence | Working. /s /f modifiers, Forth primitives. |
| ENGINE-UNIFIED-ALGO.md | Unified signature matching algorithm | Pending call frames, incremental constraint solving. |
| SIGNATURE-MATCHING-PSEUDOCODE.md | Type matching, scoring, positional/forward/prefix matching | /q modifier, fallback signatures, specificity scoring. |
| TYPES.md | Hierarchical type system (50+ types) | Scalar/Node/Word/Object/Type/Any/None, metatypes, Store. |
| SIGNATURES.md | All 100+ builtin word signatures | Full match-order signatures with returns and notes. |
| LANGREF.md | Language reference for all builtin words | All documented words registered in native_*.go. |
| IMPORTS.md | Module system: descriptors, file imports, renaming | Bare name resolution, `aql:` prefix, isolation. |
| NATIVE-MODULES.md | Native Go modules, `aql:math` | Module loading, dot-notation access, FnDef auto-invocation. |
| FILE-ACCESS.md | read/write words, FileOps interface | CSV/TSV/JSON/jsonic/text, options maps, stdin/stdout/stderr. |
| FOR-LOOP-REVIEW.md | For-loop design review | Sentinel errors for break/continue, mark/move, lazy ForCont. |

**103 native words** across: stack manipulation (15), math (6),
boolean logic (6), string ops (15), comparison (7), type system (11),
control flow (5), definition/scoping (9), array ops (23),
higher-order array (5), storage/context (3), file I/O (5),
printing (2), modules (3), accessor (1), debug (2), unify (1).


## Partially Implemented

### ARRAYIFICATION.md — ~60%

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

### DATAFRAME-WORDS.md — 28+ words

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

### MATRIX-WORDS.md — 83 words + 22 overloads

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

### TEMPORAL-WORDS.md — ~70 words

Date/time types and operations. Planned types: Instant, DateTime,
Date, TimeOfDay, CalDuration, ClkDuration, Timezone.

| Category | Words |
|----------|-------|
| Construction | `date`, `datetime`, `instant`, `time-of-day`, `tz`, `unix`, `unix-ms`, `unix-ns`, `cal-dur`, `duration` |
| Current time | `now`, `now-local`, `today`, `today-utc`, `elapsed` |
| Extraction | `year`, `month`, `day`, `hour`, `minute`, `second`, `nanosecond`, `weekday`, `weekday-name`, `month-name`, `year-day`, `iso-week`, `quarter`, `days-in-month`, `days-in-year`, `leap-year?`, `to-unix`, `to-unix-ms` |
| Duration | `years`, `months`, `weeks`, `days`, `hours`, `minutes`, `seconds`, `ms`, `us`, `ns`, `total-hours`, `total-minutes`, `total-seconds`, `total-ms`, `dur-years`, `dur-months`, `dur-days`, `dur-sign` |
| Arithmetic | `add`/`sub` (extend), `until`, `since`, `diff` |
| Comparison | `before?`, `after?`, `eq` (extend), `compare`, `between?`, `earliest`, `latest` |
| Conversion | `to-date`, `to-time-of-day`, `to-datetime`, `to-instant`, `to-local`, `to-utc`, `to-string`, `format`, `to-iso` |
| Rounding | `round`/`truncate` (extend), `start-of`, `end-of` |
| Timezone | `tz`, `tz-utc`, `tz-local`, `tz-name`, `tz-offset`, `dst?` |
| Parsing | `parse-date`, `parse-datetime`, `auto-date` |

### MINILANG.md — 10+ inline DSLs

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


## Open Design Issues

From AQL-DX-REPORT.md (building `aql:decision`):

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
