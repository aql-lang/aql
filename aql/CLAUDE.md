# CLAUDE.md

## Project

This is **voxgig-exp**, containing **AQL** — a concatenative query language implemented in Go (`aql/` directory).

## Build & Test

```bash
cd aql
make test       # or: go test ./...
make build
make vet
make fmt
```

Run a specific test:
```bash
go test ./test/ -run "TestFactorialTypeScaling" -v
```

## Dependencies

The `github.com/voxgig/struct` module is published as a Go submodule at
`github.com/voxgig/struct/go`. The `go.mod` replace directive handles this:

```
replace github.com/voxgig/struct v0.1.0 => github.com/voxgig/struct/go v0.1.0
```

If `go build` or `go test` fails downloading `modernc.org/sqlite` (or
other large modules) with a timeout from `storage.googleapis.com`, run:

```bash
GOPROXY=direct go mod download
```

This bypasses the Go module proxy and downloads directly from the source
repositories. After that, `go build ./...` and `go test ./...` will work
normally using the cached modules.

## Jsonic Token Usage

AQL uses `github.com/jsonicjs/jsonic/go` (v0.1.4) for all tokenization and
structural parsing. The custom lexer (`internal/lexer/`) and token types
(`internal/token/`) are stubs — not used in the parse pipeline.

The real parser lives in `internal/parser/parse.go`. Key jsonic integration:

- **Options**: `TextInfo:true` (quoted vs unquoted distinction),
  `ListRef/MapRef:true` (structural metadata), `Pair:true` and `Child:true`
  (typed list/map syntax like `[:String]` and `{:String}`), `Lex:false`
  (raw values for custom processing).
- **Custom tokens**: `(`, `)`, `.`, `;` are registered via `j.Token()` so
  jsonic lexes them as separate fixed tokens even when adjacent to text.
- **Grammar rule**: The `"val"` rule is extended with `j.Rule()` to handle
  parens, semicolons (aliased to "end"), and dot operators. Parens push to
  a custom "paren"/"pelem" rule pair that collects items into a `parenGroup`.
  At the top level, paren groups expand to engine markers `( ... )`. In data
  context (map values), they become `ParenExpr` values for inline evaluation
  by `autoEvalMap`. Adjacent dots (`foo.bar`) use source position analysis
  to distinguish from standalone dots (`foo . bar`).
- **Number wrapping**: A `j.Sub()` callback wraps floats containing `.` in a
  `numberVal` struct to distinguish integers from decimals at parse time.

## Parser Customization

The parser converts jsonic output to engine values through two semantic contexts:

- **Word context** (top level, lists): unquoted text → words (callable),
  quoted → strings. Lists created in word context are marked `Eval=true`
  for auto-evaluation at end of execution.
- **Data context** (inside maps): unquoted text → words (executable),
  quoted text → strings, `true`/`false` → booleans, type names → type literals,
  paren groups → `ParenExpr` (inline evaluation).

Key conversion functions in `parse.go`:
- `convertTopLevel()` / `convertTopLevelValue()` — word context
- `convertDataValue()` / `convertMapData()` — data context (atoms, not strings)
- `convertWordList()` / `convertDataList()` — lists (word context, Eval=true)
- `expandDottedWord()` — transforms `foo.a.b` into `( foo a dot b dot )`

## Quotation System

Lists are **evaluated by default**: `[1 add 2]` → `[3]`. Auto-evaluation
happens at the end of `Run()` for parser-created lists (`Eval=true`) that
were not consumed by any word.

The `quote` word (forward precedence) prevents evaluation:
- `quote [1 add 2]` → `[Integer(1), Word(add), Integer(2)]`
- `quote a` → `Atom(a)` (words become atoms)
- `quote 99` → `99` (scalars unchanged)

Quotation is **implicit** in well-defined contexts:
- `def` body: `def double [dup add]` — list consumed by def, not auto-evaluated
- `fn` body: function definition bodies
- Control words: `if`, `for` branches/bodies
- Any word that consumes a list via forward/prefix argument collection

Implementation: parser sets `Eval=true` on lists. `execMatch` strips `Eval`
from consumed arguments. `quote` sets `Quoted=true`. End-of-`Run()` only
auto-evaluates lists with `Eval=true && !Quoted`.

To add new syntax: register a token with `j.Token()`, extend the `"val"`
rule with `j.Rule()`, and add conversion logic in the appropriate
`convert*` function.

## Panic Prevention (CRITICAL)

**Panics must never occur in this codebase.** All code must be defensive
against unexpected input. Return errors instead of panicking — user
errors must be reported as error return values that are printed to the
user, never as panics. This is a hard rule — no exceptions (the only
permitted panic is `mustType()` in `types.go` which runs at init time
on hardcoded type paths).

Key patterns to follow:

- **Always nil-check before dereferencing.** `Value.AsMap()` and
  `Value.AsList()` return `nil` when `Data` is nil (type literals like
  bare `Map` or `List`) **or when `Data` is a non-concrete subtype**
  (e.g. `RecordTypeInfo`, `OptionsTypeInfo`, `ChildTypeInfo`). Never
  call `.Len()`, `.Keys()`, `.Get()` etc. on a potentially nil result
  without checking first.
- **Type literals have nil Data.** `NewTypeLiteral(TMap)` creates a Value
  with `VType=TMap, Data=nil`. Any code that receives a Value matching
  `TMap`/`TList`/`TAny` must handle the `Data==nil` case. This includes
  signature-matched arguments — `TAny` matches type literals.
- **Map subtypes share VType=TMap.** RecordTypeInfo, OptionsTypeInfo,
  ChildTypeInfo, and *OrderedMap all have `VType=TMap`. Code that checks
  `VType.Equal(TMap)` matches all of them. Use `IsRecordType()`,
  `IsOptionsType()`, `IsTypedMap()` to discriminate, and guard
  `AsMap()` calls — it returns nil for non-OrderedMap data.
- **Guard conversion functions.** `valueToAny()` and `valueToMap()` in
  `internal/native/transform.go` have nil-Data guards. If you add new
  conversion helpers, include the same guard.
- **Native function safety.** `makeFullStackHandler()` in
  `internal/native/native.go` rejects type literals and Options types
  centrally before any native handler runs. If you bypass this wrapper,
  add your own guard.
- **Engine builtin handlers.** Check `args[N].Data == nil` before calling
  `AsMap()`/`AsList()` on arguments matched via `TMap`/`TList`/`TAny`
  signatures. See `builtin_accessor_dot.go` for the canonical pattern.
- **Prefer `val, ok := v.Data.(Type)` over `v.Data.(Type)`.** The
  two-value form never panics; the single-value form panics on mismatch.
- **Write tests that use `recover()`.** For any new word or native
  function, add a test case in `TestTypeLiteralNoPanic` or
  `TestTypeLiteralNoPanicNative` that passes type literals and asserts
  no panic occurs.
