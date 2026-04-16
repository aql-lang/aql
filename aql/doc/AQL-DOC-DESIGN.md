# Design Document: `aql doc` Command

## Motivation

AQL has 120+ built-in words organized into 14 implicit categories, but users
have no way to discover or browse this structure. The existing `aql help`
command provides a flat word list and per-word detail, but cannot answer
questions like "what string operations are available?" or "what category is
`fold` in?".

The `go doc` command demonstrates the right model: browse packages, look up
symbols, get structured output. `aql doc` adapts this for AQL's flat,
category-based word namespace.

A secondary goal is machine-readable output (`-json`) to support tooling,
IDE integrations, and documentation generation.

---

## CLI Interface

```
aql doc                          List all categories with word counts
aql doc <category>               List words in a category with summaries
aql doc <word>                   Detailed docs for a specific word
aql doc -all                     Full dump: all categories, all words expanded
aql doc -all <category>          All words in a category, fully expanded
aql doc -json [...]              JSON output (combinable with any form above)
aql doc -s <pattern>             Search word names, summaries, descriptions
```

**Name resolution:** categories are checked first, then words. No current
word name collides with a category name. If ambiguity arises in the future,
`aql doc <category>/<word>` could disambiguate.

---

## Data Model

### Current state

Documentation lives in the `help` package (`aql/internal/engine/help/`):

```go
// help.go:14-19
type Entry struct {
    Word        string   // canonical word name
    Summary     string   // one-line description
    Description string   // multi-line explanation
    Notes       []string // caveats, requirements
}

// help.go:28-33
type FuncInfo struct {
    Name              string
    ForwardPrecedence bool
    Sigs              []SigInfo
    Entry             *Entry   // static docs (may be nil)
}
```

Entries are registered in 11 `help_<category>.go` files via `init()` functions.
The category grouping is implicit in the filenames — not accessible at runtime.
There are no help entries for array words (27 words) or temporal words (6 words).

### Proposed changes

**Add `Category` field to `Entry`:**

```go
type Entry struct {
    Word        string
    Category    string   // NEW: "math", "string", "array", etc.
    Summary     string
    Description string
    Notes       []string
}
```

**Add `Category` field to `FuncInfo`:**

```go
type FuncInfo struct {
    Name              string
    Category          string   // NEW: propagated from Entry.Category
    ForwardPrecedence bool
    Sigs              []SigInfo
    Entry             *Entry
}
```

**New `CategoryInfo` type:**

```go
type CategoryInfo struct {
    Name    string   // canonical name: "math", "string", etc.
    Summary string   // one-line description
    Order   int      // display order (not alphabetical)
}
```

**New help package functions:**

```go
func Categories() []CategoryInfo            // all categories in display order
func CategoryLookup(name string) *CategoryInfo
func CategoryWords(category string) []*Entry // entries in a category, sorted by word
func Search(pattern string) []*Entry         // case-insensitive substring match
```

---

## Category Definitions

Derived from `registerCoreWords` groupings in `registry.go:359-511`:

| Order | Category | Summary | Word count | Source |
|---|---|---|---|---|
| 0 | math | Arithmetic and numeric operations | 6 | help_math.go |
| 1 | string | String manipulation and pattern matching | 15 | help_string.go |
| 2 | boolean | Logical operations | 6 | help_boolean.go |
| 3 | compare | Comparison and equality | 7 | help_compare.go |
| 4 | array | List operations and higher-order words | 27 | help_array.go (NEW) |
| 5 | stack | Stack manipulation | 15 | help_stack.go |
| 6 | control | Control flow | 5 | help_control.go |
| 7 | definition | Word and function definition | 8 | (split from help_control.go) |
| 8 | type | Type system operations | 12 | help_type.go |
| 9 | storage | Key-value storage and context | 4 | help_storage.go |
| 10 | io | Input/output and file operations | 9 | help_io.go |
| 11 | temporal | Time, scheduling, and async | 6 | help_temporal.go (NEW) |
| 12 | query | Table queries, modules, and unification | 4+ | help_query.go |
| 13 | help | Self-documentation | 1 | help_help.go |

---

## Output Format

### `aql doc` — list categories

```
AQL Reference

Categories:
  math          Arithmetic and numeric operations (6 words)
  string        String manipulation and pattern matching (15 words)
  boolean       Logical operations (6 words)
  compare       Comparison and equality (7 words)
  array         List operations and higher-order words (27 words)
  stack         Stack manipulation (15 words)
  control       Control flow (5 words)
  definition    Word and function definition (8 words)
  type          Type system operations (12 words)
  storage       Key-value storage and context (4 words)
  io            Input/output and file operations (9 words)
  temporal      Time, scheduling, and async (6 words)
  query         Table queries, modules, and unification (4 words)
  help          Self-documentation (1 word)

Use 'aql doc <category>' for words in a category.
Use 'aql doc <word>' for detailed help on a word.
```

### `aql doc math` — category listing

```
math — Arithmetic and numeric operations

  add         Add two numbers, or concatenate two scalars as strings.
  sub         Subtract the top value from the second value.
  mul         Multiply two numbers.
  div         Divide the second value by the top value.
  mod         Compute the remainder of integer division.
  pow         Raise a number to a power.

Use 'aql doc <word>' for details. Use 'aql doc -all math' to expand all.
```

### `aql doc add` — word detail

Reuses existing `FormatDynamic` output with an added "Category:" line:

```
add — Add two numbers, or concatenate two scalars as strings.

Category: math

Precedence: forward — looks ahead for arguments first.
  add x y  <=>  y add x  <=>  y x add

Signatures: (in match order)
  [Integer Integer] → Integer
  [Decimal Number] → Decimal
  [Scalar Scalar] → String
  [CalDuration Date] → Date
  [ClkDuration DateTime] → DateTime
  [ClkDuration Instant] → Instant
  [ClkDuration Date] → DateTime

Description:
  Adds two numeric values. When both are integers the result
  is an integer; if either is a decimal the result is a
  decimal. For non-numeric scalars, concatenates their string
  representations.

Examples:
  add 2 3       → 5
  2 add 3       → 5
  3 2 add       → 5
  add 'a' 'b'   → 'ba'
```

### `aql doc -s fold` — search results

```
Search results for "fold":

  array/fold    Reduce a list with an accumulator and body.
  array/scan    Running reduction (like fold but keeps intermediates).
```

### `aql doc -json` — structured output

Category list:
```json
{"categories": [
  {"name": "math", "summary": "Arithmetic and numeric operations", "word_count": 6},
  ...
]}
```

Single category:
```json
{"category": "math", "summary": "...", "words": [
  {"name": "add", "summary": "Add two numbers, or concatenate..."},
  ...
]}
```

Single word:
```json
{"name": "add", "category": "math", "summary": "...",
 "description": "...", "precedence": "forward",
 "signatures": [{"args": ["Integer","Integer"], "returns": ["Integer"]}, ...],
 "examples": [{"expr": "add 2 3", "result": "5"}, ...],
 "notes": []}
```

---

## Relationship to `aql help`

| Aspect | `aql help` | `aql doc` |
|---|---|---|
| Purpose | Quick reference | Structured reference manual |
| No args | Flat word list | Category listing |
| Word lookup | Detailed docs | Detailed docs + category line |
| Category browse | Not supported | Core feature |
| Search | Not supported | `-s` flag |
| JSON output | Not supported | `-json` flag |
| In-language | `help` word in REPL | CLI only |
| Changes | None (backward compatible) | New command |

Both commands share the same data layer (`help.Entry`, `FuncInfo`,
`FormatDynamic`). `aql help` is not deprecated — it remains the quick
interactive reference. `aql doc` is the comprehensive CLI reference.

---

## Files to Create

| File | Purpose |
|---|---|
| `aql/internal/engine/help/category.go` | `CategoryInfo` type, category registry, `Categories()`, `CategoryLookup()`, `CategoryWords()`, `Search()`, formatting and JSON output functions |
| `aql/internal/engine/help/category_test.go` | Unit tests for category infrastructure |
| `aql/internal/engine/help/help_array.go` | Help entries for 27 array/higher-order words (iota through inner) |
| `aql/internal/engine/help/help_temporal.go` | Help entries for 6 temporal words (now, sleep, timeout, interval, cancel, await) |
| `aql/cmd/aql/doc.go` | `runDoc()` handler for `aql doc` subcommand with flag parsing |

## Files to Modify

| File | Lines | Change |
|---|---|---|
| `help/help.go` | 14 | Add `Category string` to `Entry` |
| `help/help.go` | 28 | Add `Category string` to `FuncInfo` |
| `help/help.go` | ~159 | Add "Category:" line to `FormatDynamic` after header |
| `help/help_math.go` | all entries | Add `Category: "math"` |
| `help/help_string.go` | all entries | Add `Category: "string"` |
| `help/help_boolean.go` | all entries | Add `Category: "boolean"` |
| `help/help_compare.go` | all entries | Add `Category: "compare"` |
| `help/help_stack.go` | all entries | Add `Category: "stack"` |
| `help/help_control.go` | all entries | Add `Category: "control"` or `"definition"` |
| `help/help_type.go` | all entries | Add `Category: "type"` |
| `help/help_storage.go` | all entries | Add `Category: "storage"` |
| `help/help_io.go` | all entries | Add `Category: "io"` |
| `help/help_query.go` | all entries | Add `Category: "query"` |
| `help/help_help.go` | all entries | Add `Category: "help"` |
| `native_help.go` | ~74 | Propagate `entry.Category` to `FuncInfo.Category` in `BuildFuncInfo` |
| `cmd/aql/main.go` | 41 | Add `aql doc` to Usage string |
| `cmd/aql/main.go` | ~63 | Add `doc` subcommand dispatch after `help` |
| `cmd/aql/main_test.go` | append | Add `TestExecuteDoc*` integration tests |

---

## Implementation Phases

### Phase 1 — Data model and categories

1. Add `Category` field to `Entry` and `FuncInfo` in `help.go`
2. Add `Category: "<name>"` to every entry in all 11 existing `help_*.go` files
3. Create `help_array.go` with entries for all 27 array/higher-order words
4. Create `help_temporal.go` with entries for all 6 temporal words
5. Propagate Category in `BuildFuncInfo` in `native_help.go`
6. Create `category.go` with `CategoryInfo`, registry, and lookup functions

### Phase 2 — Formatting and CLI

1. Implement `FormatCategoryList()`, `FormatCategory()`, `FormatWordDoc()`
2. Implement JSON output variants
3. Implement `Search()` with case-insensitive substring matching
4. Create `cmd/aql/doc.go` with `runDoc()` and flag parsing
5. Wire `doc` subcommand into `main.go` dispatch and Usage string
6. Add "Category:" line to `FormatDynamic` in `help.go`

### Phase 3 — Testing and polish

1. Write `category_test.go` unit tests
2. Write `main_test.go` integration tests for all `aql doc` forms
3. Re-run `go generate` for genhelp (picks up new array/temporal entries)
4. Verify `aql help` is unchanged (backward compatibility)
5. Run full test suite

---

## Verification

| Command | Expected behavior |
|---|---|
| `aql doc` | Lists 14 categories with correct word counts |
| `aql doc math` | Lists 6 math words with summaries |
| `aql doc array` | Lists 27 array words with summaries |
| `aql doc add` | Full docs with "Category: math" line |
| `aql doc fold` | Full docs with "Category: array" line |
| `aql doc -all math` | All 6 math words fully expanded |
| `aql doc -json` | Valid JSON with all categories |
| `aql doc -json add` | Valid JSON with full word detail |
| `aql doc -s concat` | Finds concat and related words |
| `aql doc nonexistent` | Error: "no documentation for..." |
| `aql help` | Unchanged flat word list (regression) |
| `aql help add` | Unchanged output plus new Category line |
| `go test ./...` | Full suite passes |
| `go vet ./...` | No issues |

---

## Design Decisions

**Why add Category to Entry rather than a separate mapping?**
The category is intrinsic metadata about a word. Keeping it on Entry makes it
available everywhere help data is used (REPL, CLI, in-language `help` word)
without a secondary lookup. It also makes the `help_<category>.go` files
self-documenting.

**Why not derive categories from filenames?**
Runtime filename derivation (`runtime.Caller` tricks) is fragile and
non-portable. An explicit field is simpler, testable, and works across
build modes.

**Why keep `aql help` separate?**
The `help` command and `help` AQL word serve interactive quick-lookup. The
`doc` command serves reference browsing. They share data and formatting,
so there is no code duplication. Users who only need quick help do not
need to learn doc flags.

**Why a separate `doc.go` file?**
`main.go` already dispatches 10+ subcommands. Following the existing pattern
of `auth.go`, `install.go`, `registry.go` in `cmd/aql/`, a separate file
keeps the subcommand self-contained.

**Why split `definition` from `control`?**
`def`, `fn`, `var`, `undef`, `call`, `dblcall`, `args`, `popargs` are
conceptually distinct from `if`, `for`, `do`, `quote`, `error`. The current
`help_control.go` mixes both. Splitting them into separate categories makes
browsing more focused. The file can remain `help_control.go` with entries
using either `Category: "control"` or `Category: "definition"`.
