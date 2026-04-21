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

AQL uses `github.com/jsonicjs/jsonic/go` (v0.1.6) for all tokenization and
structural parsing. The custom lexer (`internal/lexer/`) and token types
(`internal/token/`) are stubs — not used in the parse pipeline.

The real parser lives in `internal/parser/parse.go`. Key jsonic integration:

- **Options**: `TextInfo:true` (quoted vs unquoted distinction),
  `ListRef/MapRef:true` (structural metadata), `Pair:true` and `Child:true`
  (typed list/map syntax like `[:String]` and `{:String}`), `Lex:false`
  (raw values for custom processing).
- **Custom tokens**: `(`, `)`, `.`, `;`, `` ` ``, `${` are registered via
  `j.Token()` so jsonic lexes them as separate fixed tokens even when
  adjacent to text.
- **Grammar rules**: The `"val"` rule is extended with `j.Rule()` to handle
  parens, semicolons (aliased to "end"), dot operators, and template strings.
  Parens push to a custom "paren"/"pelem" rule pair that collects items into
  a `parenGroup`. At the top level, paren groups expand to engine markers
  `( ... )`. In data context (map values), they become `ParenExpr` values
  for inline evaluation by `autoEvalMap`. Adjacent dots (`foo.bar`) use
  source position analysis to distinguish from standalone dots
  (`foo . bar`).
- **Template string interpolation**: Backtick is removed from jsonic's
  `StringChars` so it is not consumed by the built-in string matcher.
  Instead, `` ` `` (#BT), `${` (#IS), and template literal text (#TL) are
  handled by custom tokens and grammar rules:
  - A `LexMatcher` (priority 1M) checks `rule.K["aql_tpl"]` to produce
    #TL tokens for literal text segments only inside template strings.
  - `"interp"` rule: opened by #BT in val, sets `K["aql_tpl"]` in BO,
    collects parts into an `interpGroup`.
  - `"ielem"` rule: matches #TL (literal text) or #IS (interpolation start).
  - `"iexpr"/"ieval"` rules: collect expression values between `${` and `}`.
    `iexpr` clears `K["aql_tpl"]` and increments `dlist`/`dmap` so
    expressions parse normally without template literal interference.
  - Nesting works to any depth since each `iexpr` pushes to `val` which
    can match another backtick and open a fresh `interp` rule.
  - `convertInterpGroup` converts the jsonic output to engine
    `InterpString` values (or plain strings if no interpolations found).
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
- `convertTopLevelItems()` — maps the `.` token to the `get` word and
  the `!` `.` pair to `getr`, so `foo.a.b` reaches the engine as
  `foo get a get b` without a separate rewriting pass

## Argument Ordering (CRITICAL)

AQL is a concatenative language where argument order is a **symmetric
mirror around the function word**. The args on each side of the word
are read inward toward it: forward args left-to-right, stack args
nearest-first (top-of-stack). Forward args always fill the lowest sig
indices, then stack args fill the remainder. To move an arg from the
forward side to the stack side while preserving its sig position, it
must go to the **far** (deepest) stack position — creating the mirror.

### The mirror pattern

For a word `f` with N args, the equivalent forms are obtained by
moving the **last** forward arg to the **far left** of the stack,
one at a time. The args on the left of `f` are always the reverse of
the args they replace on the right:

**1 arg:**
```
f a             →  sig[0]=a       (forward)
a f             →  sig[0]=a       (prefix)
```

**2 args** (verified with non-commutative `sub`: 10 sub 3 = 7, 3 sub 10 = -7):
```
f a b           →  sig[0]=a  sig[1]=b     (all forward)
b f a           →  sig[0]=a  sig[1]=b     (1 prefix, 1 forward)
b a f           →  sig[0]=a  sig[1]=b     (all prefix)
```
Note: `a f b` gives sig[0]=b, sig[1]=a — **NOT** equivalent to `f a b`.

**3 args:**
```
f a b c         →  sig[0]=a  sig[1]=b  sig[2]=c   (all forward)
c f a b         →  sig[0]=a  sig[1]=b  sig[2]=c   (1 prefix, 2 forward)
c b f a         →  sig[0]=a  sig[1]=b  sig[2]=c   (2 prefix, 1 forward)
c b a f         →  sig[0]=a  sig[1]=b  sig[2]=c   (all prefix)
```

**4 args:**
```
f a b c d       →  sig[0]=a  sig[1]=b  sig[2]=c  sig[3]=d  (all forward)
d f a b c       →  sig[0]=a  sig[1]=b  sig[2]=c  sig[3]=d  (1 prefix, 3 forward)
d c f a b       →  sig[0]=a  sig[1]=b  sig[2]=c  sig[3]=d  (2 prefix, 2 forward)
d c b f a       →  sig[0]=a  sig[1]=b  sig[2]=c  sig[3]=d  (3 prefix, 1 forward)
d c b a f       →  sig[0]=a  sig[1]=b  sig[2]=c  sig[3]=d  (all prefix)
```

**5 args:**
```
f a b c d e     →  sig[0]=a  sig[1]=b  sig[2]=c  sig[3]=d  sig[4]=e
e f a b c d     →  sig[0]=a  sig[1]=b  sig[2]=c  sig[3]=d  sig[4]=e
e d f a b c     →  sig[0]=a  sig[1]=b  sig[2]=c  sig[3]=d  sig[4]=e
e d c f a b     →  sig[0]=a  sig[1]=b  sig[2]=c  sig[3]=d  sig[4]=e
e d c b f a     →  sig[0]=a  sig[1]=b  sig[2]=c  sig[3]=d  sig[4]=e
e d c b a f     →  sig[0]=a  sig[1]=b  sig[2]=c  sig[3]=d  sig[4]=e
```

### The rule

Reading from the all-forward form `f a b c d e`:
- The forward args `a b c d e` read left-to-right → sig[0..4]
- The all-prefix form reverses them: `e d c b a f` reads top-of-stack
  first → sig[0]=a (top), sig[4]=e (deepest)
- Every mixed form is a split: forward args on the right of `f` keep
  their left-to-right order, stack args on the left are the remaining
  args in reverse

Do NOT assume left-to-right source order maps to sig order — only the
mirror equivalences above are valid. `a f b` is NOT equivalent to
`f a b` (it swaps sig[0] and sig[1]).

### Implementation

`rearrangeForForward()` in `engine.go` reorders collected values so
forward args occupy the deepest positions and stack args (reversed)
follow, then `matchSignature` with `ForceStack` and deepest-first
matching reads them in signature order. For the all-prefix case,
`matchSignature` uses `nearestFirst` mode (`match.go:282`) which maps
top-of-stack → sig[0].

## Quotation System

Lists are **evaluated by default**: `[1 add 2]` → `[3]`. Auto-evaluation
happens in two contexts for parser-created lists (`Eval=true`):

1. **When consumed as a word argument**: `execMatch` (for registered words)
   and `execFnDefSig` (for FnDef auto-invocation) run `autoEvalList` on
   list arguments with `Eval=true`, resolving word elements from DefStacks.
   For example: `def c1 10  def c2 20  [c1 c2] myword` passes `[10, 20]`
   to `myword`, not `[atom(c1), atom(c2)]`.

2. **When unconsumed on the stack at end of Run()**: `autoEvalStack` runs
   `autoEvalList` on remaining lists with `Eval=true && !Quoted`.

The `quote` word (forward precedence) prevents evaluation:
- `quote [1 add 2]` → `[Integer(1), Word(add), Integer(2)]`
- `quote a` → `Atom(a)` (words become atoms)
- `quote 99` → `99` (scalars unchanged)

Quotation is **implicit** for code-body positions via `NoEvalArgs`:
- `def` body: `def double [dup add]` — list is a code body, not data
- `fn` body: function definition bodies
- Control words: `if`, `for` branches/bodies
- Higher-order words: `each`, `fold`, `scan`, `outer`, `inner` code-body args
- `do`, `call`, `module`, `var`: list bodies executed as sub-programs

The `NoEvalArgs` field on `Signature` marks arg positions where list
auto-evaluation is suppressed. Unlike `QuoteArgs`, it does NOT affect
forward collection or word→atom conversion — it only prevents
`autoEvalList` from running in `execMatch`. Map auto-evaluation
(`autoEvalMap`) is NOT affected by `NoEvalArgs`.

Implementation: parser sets `Eval=true` on lists. `execMatch` runs
`autoEvalList` on consumed list args unless `NoEvalArgs[i]` is set.
`quote` sets `Quoted=true` (also suppresses auto-eval). End-of-`Run()`
auto-evaluates only lists with `Eval=true && !Quoted` that were never
consumed.

To add new syntax: register a token with `j.Token()`, extend the `"val"`
rule with `j.Rule()`, and add conversion logic in the appropriate
`convert*` function. For context-sensitive lexing, use `j.AddMatcher()`
with the v0.1.6 rule-aware `LexMatcher` signature
`func(lex *Lex, rule *Rule) *Token` to read `rule.K`/`rule.N` maps.
See the template string interpolation rules for a complete example.

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
- **Engine builtin handlers.** Check `args[N].Data == nil` before calling
  `AsMap()`/`AsList()` on arguments matched via `TMap`/`TList`/`TAny`
  signatures. See `native_accessor_dotr.go` for the canonical pattern.
- **Prefer `val, ok := v.Data.(Type)` over `v.Data.(Type)`.** The
  two-value form never panics; the single-value form panics on mismatch.
- **Write tests that use `recover()`.** For any new word or native
  function, add a test case in `TestTypeLiteralNoPanic` or
  `TestTypeLiteralNoPanicNative` that passes type literals and asserts
  no panic occurs.
