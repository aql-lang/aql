# CLAUDE.md

## Project

This is **AQL** — a concatenative query language implemented in Go. Each
component sits under a top-level folder; the Go implementation of each
lives in a `<component>/go/` subfolder so that future TS ports can sit
alongside as `<component>/ts/`. The Go modules are:

- `eng/go/` — the engine kernel + jsonic parser + kernel spec runner
  (`github.com/aql-lang/aql/eng/go`).
- `lang/go/` — the language layer: `lang.New()` API and the consolidated
  `native` package (the eng-shim + every built-in word) plus the
  production spec suite (this module,
  `github.com/aql-lang/aql/lang/go`). The pre-May-2026 split between
  `lang/go/engine/` (language-layer primitives + alias shim) and
  `lang/go/native/` (data-manipulation words) was merged into a single
  `lang/go/native/` package — see `Package layout` below.
- `cmd/go/` — the `aql` CLI / REPL
  (`github.com/aql-lang/aql/cmd/go`).
- `calc/go/` — a small calculator built directly on `eng` (learning example,
  `github.com/aql-lang/aql/calc/go`; not published).
- `wpg/` — the wasm web playground: `wpg/wasm` (browser wasm build) and
  `wpg/serve` (standalone HTTP server with embedded HTML)
  (`github.com/aql-lang/aql/wpg`).
- `test/go/` — shared TSV spec-runner scaffolding
  (`github.com/aql-lang/aql/test/go`).
- `test/solardemo/` — standalone HTTP fixture used by API tests
  (`github.com/aql-lang/aql/test/solardemo`).

Language-agnostic content stays at the top of each component:
`eng/spec/`, `lang/spec/`, `lang/doc/`, `calc/doc/`.

## Package layout

`lang/go/` contains:

- `aql.go`, `parse.go` — the public `lang` package (entry points
  `lang.New()`, `(*AQL).Run`, `(*AQL).Check`, `(*AQL).Register`,
  re-exports of `Type`/`Value`/`Signature` for handler authors).
- `native/` — every built-in word and the kernel-shim aliases.
  Sub-files:
  - `aliases.go` — re-exports the eng kernel's exported types/funcs
    as `native.Foo` so the rest of the lang code can stay
    eng-agnostic.
  - `register.go`, `setup.go` — `Register`/`DefaultRegistry` entry
    points.
  - `native_*.go` — language-layer primitives (math, boolean,
    string, stack, control, def, type, accessor, …). Each file
    exposes a `xxxNatives []NativeFunc` slice that `Register`
    iterates.
  - `natives.go` and the per-feature files (`clone.go`, `fetch.go`,
    `filter.go`, …) — data-manipulation words historically owned
    by the `native` sub-package and merged into this consolidated
    package in May 2026.
  - `format.go`, `query.go`, `sqlite.go`, `fileio.go`,
    `conditional.go`, `forloop.go` — feature-specific helpers.
  - `help/` — dynamic help-system implementation (sub-package).
- `formatter/` — code pretty-printer (no engine deps).
- `capabilities/` — file I/O abstraction (`FileOps` interface
  + OS-backed and in-memory implementations).
- `modules/` — loadable modules (`aql:math`, `aql:time`,
  `aql:matrix`, `aql:decision`, `aql:solardemo`).
- `test/` — integration tests and TSV spec runners.

## Build & Test

```bash
make test         # from repo root: fans out across every module
make vet          # vet across every module
make fmt          # gofmt across every module

cd cmd/go
make build        # builds bin/aql

cd ../wpg
make wasm         # builds ../docs/index.html (bundled aql.wasm playground)
make serve        # runs the HTTP playground on :8080
```

Run a specific test:
```bash
cd lang/go && go test ./test/ -run "TestFactorialTypeScaling" -v
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
structural parsing. There is exactly one parser — it lives in the
standalone **eng** module at `eng/go/parser/parse.go`
(`github.com/aql-lang/aql/eng/go/parser`); `lang.Parse` re-exports
it. (The old hand-rolled lexer / token / AST / tree-walking evaluator
were removed — jsonic is the sole parsing path.) Key jsonic integration:

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
- `expandDottedWord()` — transforms `foo.a.b` into `( foo get a get b )`

## Argument Ordering (CRITICAL)

Post §1.4 unification, dispatch is governed by **one** rule applied
to every signature, regardless of whether the word was historically
"forward-collecting" or "stack-only":

> Each `Signature` declares a boundary `BarrierPos` (the position of
> the `|` marker). Args at sig positions `[0..BarrierPos-1]` may be
> collected from forward tokens (in source order) or fall back to
> the stack. Args at sig positions `[BarrierPos..N-1]` always come
> from the stack. Stack consumption is always **top-down**: sig[i]
> reads the top of the stack, sig[i+1] reads the next-deeper, etc.

`BarrierPos == 0` means "all stack" (legacy stack-only).
`BarrierPos == N` means "all forward-eligible" (legacy forward-prec).
`0 < B < N` mixes the two: forward fills the leading B positions then
stack fills the rest.

### The unified algorithm

For each candidate `sig`:

1. **Forward phase** — walk sig[0..forwardLimit-1] in order. At each
   step, take the next future token and check it against the next
   sig type. Stop on a structural boundary (open paren, end, function
   word) or on type mismatch — the remaining sig positions then come
   from the stack.

2. **Stack phase** — walk the remaining sig positions in order, top
   of stack first. sig[fwd] = stack top, sig[fwd+1] = next-deeper, etc.

The handler always sees args in sig order: `args[0]` is whatever
matched sig[0], regardless of whether it came from a future token or
the top of the stack.

### Examples

For `def f fn [[A B C] Any [...]]` (no boundary, all forward-eligible),
all of these forms call `Fimpl(a, b, c)`:

```
f a b c     → forward [a,b,c]                        → sig=[a,b,c]
c f a b     → forward [a,b], stack top=c             → sig=[a,b,c]
c b f a     → forward [a],   stack top=b, deeper=c   → sig=[a,b,c]
c b a f     → forward [],    stack top=a, …, c       → sig=[a,b,c]
```

For `def g fn [[A B | C] Any [...]]` (boundary at position 2), only
forms that put `c` on the stack are valid:

```
c g a b     → forward [a,b], stack top=c   → sig=[a,b,c]
c b g a     → forward [a],   stack top=b…c → sig=[a,b,c]
c b a g     → forward [],    stack top=a…c → sig=[a,b,c]
```

For `def h fn [[| A B C] Any [...]]` (boundary at 0, legacy stack-only),
only the all-prefix form matches:

```
c b a h     → forward [], stack top=a, deeper=b, deepest=c → sig=[a,b,c]
```

### Non-commutative two-arg sanity check

With `sub` declared as a 2-arg forward-eligible word — handler computes
`args[1] - args[0]` (post-§1.4 phase 4: every binary math handler
computes `b op a` so the swap form reads naturally):

```
sub 10 3    → forward [10,3]                  → sig=[10,3] → 3-10 = -7
3 sub 10    → forward [10], stack top=3       → sig=[10,3] → 3-10 = -7
3 10 sub    → forward [],   stack top=10…3    → sig=[10,3] → 3-10 = -7
10 sub 3    → forward [3],  stack top=10      → sig=[3,10] → 10-3 =  7  (swap form)
```

`a f b` is the **swap form**: it binds sig[0] from the forward side
(b) and sig[1] from the prefix (a). The mirror equivalence
`f a b ≡ b f a ≡ b a f` holds; `a f b` is the only non-equivalent
two-arg arrangement. The phase-4 handler convention picks the swap
form as the canonical surface syntax: `10 sub 3 = 7` matches how
a reader scans left-to-right.

### Implementation

The matcher in `eng/go/match.go::matchSignature` runs a single
loop over sig positions. Forward limit comes from `sig.BarrierPos`,
overridable per call site by `/s` (force stack: limit=0) or `/f`
(force forward: limit=N). When forward args have to be collected
across paren / nested-forward boundaries, `rearrangeForForward` lays
the collected values out so the post-collection retry sees them
top-down in sig order.

## Quotation System

Lists are **evaluated by default**: `[1 add 2]` → `[3]`. Auto-evaluation
happens in two contexts for parser-created lists (`Eval=true`):

1. **When consumed as a word argument**: `execMatch` (for registered words)
   and `execFnDefSig` (for FnDef auto-invocation) run `autoEvalList` on
   list arguments with `Eval=true`, resolving word elements via the def
   stack (`r.TopOfDefStack`).
   For example: `def c1 10  def c2 20  [c1 c2] myword` passes `[10, 20]`
   to `myword`, not `[atom(c1), atom(c2)]`.

2. **When unconsumed on the stack at end of Run()**: `autoEvalStack` runs
   `autoEvalList` on remaining lists with `Eval=true && !Quoted`.

The `quote` word (forward arg collection) prevents evaluation:
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

## Registry Bindings (CRITICAL)

Post the TYPE-UNIFORM Phase 4 collapse the `Registry` holds **one**
per-name binding store, `r.Defs` (a `*DefTable`). It carries both
kinds of binding:

- **value bindings** — `def x 1`, fn bodies, fn-body parameters,
  carrier-merge join points, module imports.
- **type bindings** — `def Foo Integer` (a capitalised name). The
  `DefEntry` additionally carries the minted lattice `*Type` in
  `DefEntry.TypeDef`.

The capitalisation convention keeps the two kinds of name disjoint
(`Foo` is a type, `foo` a value), so one map suffices. Each name maps
to a shadowing stack; `def x 1; def x 2` → `x` is 2; `undef x` → `x`
is 1.

`r.Types` (a `*TypeTable`) is **not** a binding store — it is the type
*lattice*: it mints type identities (`MintType`), indexes them by ID
(`LookupByID`), retires them (`Retire`), and holds the static builtin
name index (`LookupBuiltinByName`).

**DefTable methods** (`r.Defs.*`):
- reads — `Top(name) (Value, bool)`, `TopEntry(name) (DefEntry, bool)`,
  `Has(name)`, `IsType(name)`, `Depth(name)`, `Stack(name) []Value`,
  `Names()`.
- writes — `Push(name, v)` (value binding),
  `PushType(name, def, body)` (type binding), `Pop(name) bool`,
  `PopEntry(name) (DefEntry, bool)`, `Replace`, `Truncate`, `Delete`,
  `Set`.
- snapshot/restore — `Snapshot() map[string]int` / `Restore(snap)`,
  depth-based; pair around fn-body sandboxes and carrier-merge joins.
  (The predicate sandbox additionally clones `r.Types` to roll back
  lattice mints — see `snapshotPredicateState`.)

**Registry resolution helpers**:
- `r.ResolveTypedName(name) (Value, bool)` — single-store lookup; the
  canonical way to resolve a type-context name.
- `r.TopTypeBody(name) (Value, bool)` — the body when name's active
  binding is a *type* binding (false otherwise).
- `r.LookupTypeName(name) *Type` — the active lattice type for name:
  a dynamic binding's minted def, or an external builtin.

`undef` is the universal unbinder: a capitalised name pops the type
binding and retires its minted type from the lattice; a lowercase
name pops a value binding.

## Helper API discipline

The engine consolidates several distributed implicit contracts behind
helper APIs in `eng/util.go` (re-exported through `native/aliases.go`).
Use the helpers rather than
the underlying state. Adding direct field access regresses the
consolidation and will be flagged in code review.

**Concrete-value guards** (panic-prevention, type-literal vs concrete):
- `IsTypeLiteral(v)` — true if `Data == nil` and not a carrier and not None.
- `IsConcrete(v)` — true if `Data != nil` and not a carrier.
- `RequireConcreteList(v, op) (ReadList, error)` — unwraps a list-typed
  Value or returns an error when the value is a type literal/carrier.
- `RequireConcreteMap(v, op) (ReadMap, error)` — same for maps.

Handlers that take `TList`/`TMap`/`TAny` args should guard with
`!IsConcrete(args[i])` (or use the `RequireConcreteX` helpers) before
calling `AsList(v)`/`AsMap(v)` — otherwise carriers and type literals
panic on `.Len()`. (`AsList` / `AsMap` are free functions in eng, not
methods on `Value`; only `Is(t)` and `String()` remain as methods.)

**DepScalar-rejecting accessors**:
- `v.AsConcreteString()`, `v.AsConcreteInteger()`, `v.AsConcreteDecimal()`,
  `v.AsConcreteBoolean()`, `v.AsConcreteAtom()` — reject DepScalar
  payloads with a clear error rather than silently returning the zero
  value. Always prefer these over the bare free-function accessors
  (`AsString(v)`, `AsInteger(v)`, …) when the arg comes from a
  sig-matched value (a `TString` slot can secretly hold a `DepString`
  constraint). These DepScalar-rejecting variants remain methods on
  `Value` since they're the canonical handler-side error path; the
  low-level accessors were drained to free functions in `eng/` as part
  of the type-decoupling work — see
  `lang/doc/design/TYPE-DECOUPLING.0.md`.

**Check mode**:
- `r.IsCheckMode()` — read-side helper. Replaces `r.Check.Mode` and
  the `r != nil && r.Check.Mode` nil-guarded variants.
- `r.BeginCheckMode() func()` — entry-side helper; resets per-pass
  state and returns a deferred-cleanup function. Use as
  `defer r.BeginCheckMode()()` in the analyser entry point.

**Error construction**:
- `r.AqlError(code, detail, word)` — handler-side error constructor;
  picks up `r.Source` automatically. Replaces the recurring
  `makeAqlError(code, detail, name, r.Source, "")` pattern.
- `r.AqlErrorHint(code, detail, word, hint)` — same with an explicit
  hint string. The engine-internal helpers (`signatureError`,
  `insufficientArgsError`, etc. in `engine.go`) layer above these
  with engine-specific source resolution.

**Args stack** (per-fn-call args list, used by the `args` word):
- `r.PushArgs(list)` / `r.PopArgs()` / `r.TopArgs() (Value, bool)`.
  The underlying `argsStack` field is unexported.

**Context stack** (scoped Store layers for `ctx-set`/`ctx-get`/etc.):
- `r.PushContext(parent)` — push a new copy-on-write child layer.
- `r.PushExistingContext(ctx)` — push an existing Store without
  wrapping (rare; used by module loading to inherit the parent's ctx).
- `r.PopContext()`, `r.Context()` (returns the data map),
  `r.ContextStore()` (returns the StoreInstanceInfo),
  `r.UpdateCtxStoreChain(orig, new)`.

**Pos threading**:
- `WithPos(v, src)` — return v with Pos copied from src. Use when a
  handler constructs a new Value from an input — error reporting
  downstream then has the source location.

## Undefined Words (CRITICAL)

An undefined word reaching the pointer is an **error**, not a value.
A word that is not registered, not in the def stack, and not a known
literal (`true`/`false`/a type name) raises `[aql/undefined_word]` at
`stepWord`. There is no implicit `Word → Atom` fallback.

Names that are meant as data must be quoted:

- `foo/q` — word-suffix form: parses `foo` directly as `Atom(foo)`.
  This is the preferred short form and the canonical render output
  (canon.go emits `name/q` rather than `(quote name)`). Like the
  other suffix modifiers, `/q` stacks with `/s`, `/f`, and the
  argCount digit in any order (`foo/sq` ≡ `foo/qs`); when `q` is
  present the result is an Atom and the other modifiers are
  accepted syntactically but ignored.
- `quote foo` — function form: captures the upcoming Word as
  `Atom(foo)`.
- `(quote foo)` — same as `quote foo`, inside a paren so forward
  collection can pick up the resulting Atom.
- A `/q`-marked sig position — captures the upcoming Word as an Atom
  during forward collection (`def name body`, `get key map`,
  `set key val store`, etc.).

CheckMode is the single exception: `stepWord` keeps the lenient
"undefined → `Atom{Undefined:true}`" path so static analysis can
continue past a typo. Each dangling Undefined Atom is converted to a
diagnostic + `Any` carrier in the end-of-`Run()` drain in
`engine.go`.

When adding a sig that should accept a bare-word name as data, add `/q`
to the corresponding Atom position. Without `/q`, callers will see an
`undefined_word` error and must wrap the name in `quote` themselves.

## Value Comparison & Ordering

`cmp` / `lt` / `gt` / `lte` / `gte` / `sort` route through one total
order — see `lang/doc/design/TYPE-ORDERING.0.md` for the canonical
design. The kernel-side implementation lives in `eng/go/compare.go`
and `eng/go/compare_scalar_behaviors.go`; this section captures
what handler authors and word implementers need to know.

**The cascade** (per `CompareValues`):

1. **LCA Comparer walk** — per-family Comparers on `TNumber`,
   `TString`, `TBoolean`, `TAtom`, `TWord`, `TScalar` own same-
   family pairs. A Comparer can return `ErrNoComparer` to opt out
   (DepScalar values do this so numeric Comparers don't read
   `DepScalarInfo` as a zero float).
2. **Rank fallback** — `compareTypes` (Rank → depth → name → ID)
   settles cross-family pairs.
3. **Structural compare** — when types match, lists go length-then-
   element-wise, maps go length-then-sorted-keys-then-values,
   everything else falls to `CanonValue` lex.

**Type-literal-first rule.** Every family Comparer opens with
`litVsConcreteOrder(a, b)`: a bare type literal sorts strictly
below every concrete inhabitant in the same family. So `Integer
cmp 0 → -1`, `String cmp '' → -1`, `Boolean cmp false → -1`,
`Path cmp <any path> → -1`. Two type literals delegate to
`litVsLitOrder` → `compareTypes` so they order by lattice Rank
(`Number cmp Integer → -1`). The rule lives in per-family
Comparers and `comparePaths`; `scalarCompareBehavior` deliberately
does NOT apply it (cross-family pairs must be Rank-only — otherwise
`true cmp Integer` flips wrong).

**Cross-leaf numeric equivalence.** `1 cmp 1.0 → 0` is preserved:
the Number Comparer projects both Integer and Decimal to `float64`,
so magnitude equality across leaves stays consistent with
arithmetic (`1 + 0 == 1`, `1.0 == 1`).

**Order property.** Strict total order over distinct lattice nodes,
total preorder over values (one deliberate equivalence: cross-leaf
magnitude). Verified by `lang/spec/compare.tsv` (748 rows incl.
transitivity battery + user-defined-type coverage).

**Adding a Comparer to a user/external type.** Implement the
`Comparer` interface on your `TypeBehavior`:

```go
type fooBehavior struct{ defaultBehavior }
func (fooBehavior) Compare(a, b Value) (int, error) {
    // 1. Opt out for shapes you don't own (DepScalar, carriers).
    // 2. Apply litVsConcreteOrder if your type's literal should
    //    sort below concrete instances.
    // 3. Otherwise, project a and b into your domain and return
    //    -1/0/1 (or ErrNoComparer to bubble up to Rank).
}
```

The Comparer participates automatically in `cmp`/`sort`/etc. via
the LCA walk — no separate registration.

## Panic Prevention (CRITICAL)

**Panics must never occur in this codebase.** All code must be defensive
against unexpected input. Return errors instead of panicking — user
errors must be reported as error return values that are printed to the
user, never as panics. This is a hard rule.

The only permitted panics are at **init time**, on hardcoded type-registration
paths — they signal a build-time programmer error (FixedID collision or
malformed type path), not a runtime condition. Each such call site carries
a `// lint:allow-panic` comment. The current set:

- `eng/types.go::mustType` — eng's hardcoded built-in types.
- `native/native_misc.go::registerTimerType` — TTimeout, TInterval.
- `native/native_temporal.go::registerTemporalType` — TDate, TDateTime, …
- `native/fetch.go::registerFetchType` — TFetchFunction, TFetchRequest, …
- `modules/matrix.go::registerTensorTypes` — TTensor, TMatrix, TVector.

Do not add new init-time panics without also annotating them
`// lint:allow-panic` and listing them here.

Key patterns to follow:

- **Always nil-check before dereferencing.** `AsMap(v)` and `AsList(v)`
  return `nil` when `Data` is nil (type literals like bare `Map` or
  `List`) **or when `Data` is a non-concrete subtype** (e.g.
  `RecordTypeInfo`, `OptionsTypeInfo`, `ChildTypeInfo`). Never call
  `.Len()`, `.Keys()`, `.Get()` etc. on a potentially nil result
  without checking first.
- **Type literals have nil Data.** `NewTypeLiteral(TMap)` creates a Value
  with `Parent=TMap, Data=nil`. Any code that receives a Value matching
  `TMap`/`TList`/`TAny` must handle the `Data==nil` case. This includes
  signature-matched arguments — `TAny` matches type literals.
- **Map subtypes share Parent=TMap.** RecordTypeInfo, OptionsTypeInfo,
  ChildTypeInfo, and *OrderedMap all have `Parent=TMap`. Code that checks
  `Parent.Equal(TMap)` matches all of them. Use `IsRecordType(v)`,
  `IsOptionsType(v)`, `IsTypedMap(v)` to discriminate, and guard
  `AsMap(v)` calls — it returns nil for non-OrderedMap data.
- **Guard conversion functions.** `valueToAny()` and `valueToMap()` in
  `native/transform.go` have nil-Data guards. If you add new
  conversion helpers, include the same guard.
- **Engine builtin handlers.** Check `args[N].Data == nil` before calling
  `AsMap(args[N])`/`AsList(args[N])` on arguments matched via
  `TMap`/`TList`/`TAny` signatures. See `native_accessor_dotr.go`
  for the canonical pattern.
- **Prefer `val, ok := v.Data.(Type)` over `v.Data.(Type)`.** The
  two-value form never panics; the single-value form panics on mismatch.
- **Write tests that use `recover()`.** For any new word or native
  function, add a test case in `TestTypeLiteralNoPanic` or
  `TestTypeLiteralNoPanicNative` that passes type literals and asserts
  no panic occurs.
