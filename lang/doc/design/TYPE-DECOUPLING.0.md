# Type-Behaviour Dispatch: Decoupling the Kernel from Domain Types

This proposal replaces the kernel's closed type-dispatch surface
(`Value.IsX` predicates, `Value.AsX` accessors, hardcoded switches in
`Value.String` and the lattice path) with a single behaviour-bearing
type model. The goal is that user modules can register types — and
ordinary user code can introduce types via `type Foo Integer` — that
are dispatched on by the kernel through the same uniform mechanism the
kernel uses for `Integer` itself. No closed enumeration; no kernel
patch needed when a new type is added.

This is a design proposal, not a landed change. Status: **DRAFT (0)**.

---

## 1. The problem

The kernel currently has three parallel mechanisms for type dispatch,
and every one of them is a closed enumeration baked into `eng`:

1. **30 `Value.IsX()` predicates** — `IsWord`, `IsInteger`,
   `IsObjectInstance`, `IsTimeout`, …. Each is a method on `Value` that
   names a specific type. Adding a new type means adding a method or
   accepting that the type is invisible to predicate-style code.

2. **40 `Value.AsX()` payload accessors** — `AsInteger`,
   `AsCalDuration`, `AsMatrix`, …. The kernel both *defines* and
   *exports* payload-extraction methods for types it has no semantic
   reason to know about. `CalDurationData`, `MatrixData`, `TimeoutInfo`
   and `IntervalInfo` are all struct types declared in `eng/go/value.go`
   even though `eng` itself does nothing with them.

3. **Type-symbol dispatch in switches** — `v.VType.Matches(TInteger)`,
   `v.VType.Equal(TList)`, and the ~200-line `case`-tree in
   `Value.String` that hardcodes a render branch for every domain
   type the kernel happens to know about (Date, DateTime, Instant,
   TimeOfDay, CalDuration, ClkDuration, Timezone, Matrix, …).

The core issue: each of these is a *closed set*. `type Foo Integer`
works only because Foo inherits TInteger as its parent in the type
lattice, so `v.VType.Matches(TInteger)` still returns true for a Foo
value. But the kernel can never *natively* see Foo as Foo — and a
plugin module that registers a brand-new type with no kernel parent
(an SDK-provided opaque handle, a domain-specific scalar) can't be
plumbed through the existing dispatch surface at all.

This affects every plugin scenario: user records, user objects, user
predicate types, host-provided foreign types, and even the existing
"domain" types in `eng/types.go` (Date, CalDuration, Matrix, Timeout,
Interval) that morally belong to lang's nativemod packages but live
in the kernel because the kernel needs to dispatch on them.

### Survey of the dispatch surface

Counts taken against the current tree (`eng/go/`):

```
Value.IsX methods      : 31  (eng/go/value.go)
Value.AsX methods      : 40  (eng/go/value.go)
VType.Matches/Equal    : 291 sites in eng/go + 234 in lang/engine
Value.String switch    : ~20 non-primitive arms; lines 1659-1883 of value.go
Hardcoded T* constants : 67  (eng/go/types.go:14-81)
builtinDecls entries   : 80+ (eng/go/typetable.go:325-412)
```

Of the As* methods for the *domain* types — Date, DateTime, Instant,
TimeOfDay, CalDuration, ClkDuration, Timezone, Matrix, Timeout,
Interval — **zero are called from inside `eng/`** except for two
sites in `Value.String` (Timeout at value.go:1815, Interval at
value.go:1818). The kernel *exports* `AsDate`, `AsMatrix`,
`AsCalDuration`, `NewDate`, `NewMatrix`, `NewCalDuration`, … but
every real caller lives in `lang/internal/nativemod/*` (136
references in `time.go`, 76 in `matrix.go`). The kernel pays the
maintenance cost of these methods for symmetry alone.

Two extra coupling points worth naming, because the doc's design
must keep them working:

- **DepScalar override**: `Type.Matches` (eng/go/types.go:194-218)
  carries a special branch that lets `Type/Dependent/Dep<Leaf>`
  satisfy slots typed as the underlying base (e.g. a `DepInteger`
  satisfies `Integer`). The leaf→base map is hardcoded as a switch
  in `DependentLeafBaseType` (eng/go/depscalar.go:176-192).
- **Metatype awareness**: `sigTypeMatches` (eng/go/signature.go:259-279)
  promotes a type literal to its metatype when matched against a
  `Type/*` slot. The metatype assignment (`MetatypeFor`) is itself
  a hardcoded switch on root names — `Scalar/Node/Object` → matching
  `ScalarType/NodeType/ObjectType` (eng/go/types.go:274-287).

Both rules currently sit on `*Type` methods, half on `Value` —
exactly the pattern §6 dissolves.


## 2. The model

A `Type` becomes the dispatch unit. Each Type carries the operations
the kernel needs to perform on values of that type. The kernel never
asks "is this a CalDuration?" — it asks "what does `v.VType.Behavior`
say about `v`?".

```go
// eng/types.go
type Type struct {
    // existing identity fields (ID, Path, Parent, …) unchanged

    // Behavior is the pluggable per-type operation set. nil means
    // "use the default lattice-based behavior" (which the kernel
    // installs at MintType time for every type that doesn't
    // override). Custom Behaviors cover predicate types, refinement
    // types, dependent scalars, foreign opaque values, and any
    // host-provided type the kernel can't have prior knowledge of.
    Behavior TypeBehavior
}

type TypeBehavior interface {
    // Match reports whether v conforms to this type. Default impl
    // is the existing lattice walk (v.VType == t or descendant).
    // Predicate types override to invoke the predicate body;
    // refinement types override to check the refinement clause.
    Match(v Value, t *Type) bool

    // Format renders v as a string. Used by Value.String, error
    // messages, the canon writer, and the spec runner.
    Format(v Value) string

    // Equal reports semantic equality. Default is reflect.DeepEqual
    // on payloads; structural types override (Map: per-key compare;
    // CalDuration: normalise then compare; etc.).
    Equal(a, b Value) bool
}
```

Most types don't ship a custom Behavior. The `TypeTable` installs a
sensible default at `MintType` time: lattice-based Match, fmt-style
Format, deep-equal. A type provides its own implementation only when
its semantics demand it.

Optional capability sub-interfaces let a type opt into extra
operations without bloating the required interface:

```go
type Hasher interface { Hash(v Value) uint64 }
type Comparer interface { Compare(a, b Value) int }
type Walker interface { Walk(v Value, visit func(Value)) }
// ...
```

Kernel words that need these (`hash`, `sort`, `walk`, …) consult the
Type's Behavior via type assertion: if the Behavior implements
`Comparer`, use it; otherwise the word errors with a clear "type T
does not support compare" message.


## 3. What replaces the three mechanisms

| Today | Tomorrow |
|---|---|
| `v.IsInteger()` | `v.Is(TInteger)` → routes through `TInteger.Behavior.Match` |
| `v.IsCalDuration()` (would need to be added) | `v.Is(time.TCalDuration)` — same path, no kernel change |
| `v.AsInteger()` | `n, _ := v.Data.(int64)` (caller knows the convention) |
| `v.AsCalDuration()` | `time.AsCalDuration(v)` (in the module that owns the payload) |
| `Value.String`'s 200-line switch | one dispatch: `if v.VType.Behavior != nil { return v.VType.Behavior.Format(v) }` |
| `v.VType.Matches(TList)` | `v.Is(TList)` |

**`v.Is(t *Type)` is the single dispatch method.** It's what the
kernel uses, what handlers use, what `is`/`guard` evaluate, what
signature matching consults. Behind it sits `t.Behavior.Match(v, t)`
— pluggable.

**Payload extraction stops being a kernel API for non-primitives.**
The kernel stores `Data any`; callers who know the type also know the
Go form to assert. `AsInteger`-style wrappers become package-local
conveniences in whichever module owns the type. `eng` exports zero
As* methods for non-kernel types.


## 4. How user types fit

### 4.1 `type Foo Integer`

```aql
type Foo Integer
def x:Foo 5
```

What happens:

1. `TypeTable.MintType("Foo", parent=TInteger)` creates a fresh
   `*Type` for Foo.
2. Foo inherits TInteger's Behavior. `Match` passes if `v` is an
   Integer (or Foo). `Format` does `fmt.Sprintf("%d", v.Data)`.
   `Equal` compares int64 payloads.
3. `def x:Foo 5` calls `TFoo.Behavior.Match(NewInteger(5), TFoo)` →
   true → installs.

No kernel code changes when the user types `type Foo Integer`. The
dispatch path is the same as for any other type, because there *is*
only one dispatch path.

### 4.2 Predicate types

```aql
type Even fn [n:Integer Boolean [n 2 mod 0 eq]]
```

The `fn` constructor (already in the codebase) builds a Behavior
whose `Match` invokes the predicate body. No kernel change —
predicate types just have a non-default Behavior on their Type.

### 4.3 Records

```aql
type Point record { x:Decimal y:Decimal }
```

`record` constructs a Behavior whose `Match` checks field-by-field;
`Format` renders the field-name → field-value pairs. Already
structurally what `RecordTypeInfo` does today — just routed through
Behavior instead of hardcoded switches.

### 4.4 Domain types loaded by a module

```go
// lang/internal/nativemod/time/register.go
func Register(r *eng.Registry) {
    TDate = r.Types.MintType("Date", eng.TScalar)
    TDate.Behavior = dateBehavior{}
    // …same for DateTime, CalDuration, Timezone, etc.
}

type dateBehavior struct{}

func (dateBehavior) Match(v eng.Value, t *eng.Type) bool {
    if v.VType != t && !v.VType.IsDescendantOf(t) { return false }
    _, ok := v.Data.(time.Time)
    return ok || v.Data == nil // type-literal Date is also a match
}

func (dateBehavior) Format(v eng.Value) string {
    if t, ok := v.Data.(time.Time); ok {
        return t.Format("2006-01-02")
    }
    return "Date(nil)"
}

func (dateBehavior) Equal(a, b eng.Value) bool {
    aT, _ := a.Data.(time.Time)
    bT, _ := b.Data.(time.Time)
    return aT.Equal(bT)
}
```

A foreign module — say one that wraps a database row type — works
identically. The kernel doesn't need to know what a row is; the
module supplies the Behavior and everything dispatches.


## 5. What stays kernel-hardcoded — and why

Some types are *load-bearing for the interpreter loop itself*. The
kernel can't be ignorant of them because the step loop branches on
their shape:

- **List, Map** — auto-eval, splicing, paren handling, interpolation
  all dispatch on these structurally.
- **Word, Forward, Mark, Move, OpenParen, CloseParen, End,
  ReturnCheck, DefCleanup, ParenExpr, InterpString** — control
  tokens. The step loop matches on these to drive dispatch.
- **Integer, Decimal, String, Boolean, Atom, Path, None** — literal
  kinds the parser produces. Removing kernel knowledge here means
  reworking the parser, which is out of scope.
- **Type, Function, FnDef, FnUndef** — type-system meta-types used
  by `is`, `typeof`, `inspect`. Same reasoning.

Everything else — CalDuration, ClkDuration, Date, DateTime, Instant,
TimeOfDay, Timezone, Matrix, Timeout, Interval, all user types, all
record/object instances, all foreign-host opaque values — flows
through the Behavior path.

The boundary is principled: a type stays kernel-hardcoded iff
**the parser produces it directly** or **the interpreter loop
branches on it structurally**. Anything else is data and goes
through Behavior.


## 6. Migration plan — test-green at every step

The cardinal rule of this rollout: **`make test` (the rollup in
`lang/Makefile`) must pass at the end of every numbered step**, with
no temporarily-skipped tests. The plan is structured so each step is
either a strict superset of the old behaviour, or a mechanical
rewrite whose old and new forms can coexist while the rewrite is in
flight. The list below names the entry point in the tree, the
invariant that gates "done", and the tests that prove it.

### Step 0 — Catalogue & freeze

**Goal**: establish a baseline of every call site that today
hardcodes a domain-type identity, before any code moves.

**Work**: grep `eng/go/` and `lang/` for the closed enumerations and
record the inventory in a checked-in note (e.g. `lang/doc/design/
TYPE-DECOUPLING-INVENTORY.md`). The four sets:

1. `Value.IsX` callers outside `eng/go/value.go` (~120 sites).
2. `Value.AsX` callers outside `eng/go/value.go` for non-primitives
   (a few dozen).
3. `Value.String` non-primitive arms (eng/go/value.go:1719-1820).
4. References to domain `T*` constants — `TDate`, `TDateTime`,
   `TInstant`, `TTimeOfDay`, `TCalDuration`, `TClkDuration`,
   `TTimezone`, `TMatrix`, `TTimeout`, `TInterval`, `TFetch*` —
   anywhere outside `eng/go/types.go` and `eng/go/typetable.go`.

**Done when**: the inventory exists and `go vet ./...` /
`make test` still green. No code change yet — this step exists to
make sure subsequent steps know what they touch.

### Step 1 — Add `TypeBehavior` with a default implementation

**Goal**: introduce the seam without changing any observable
behaviour.

**Work**:

1. Define `TypeBehavior` interface (Match / Format / Equal) and the
   optional capability interfaces (Hasher, Comparer, Walker) in
   a new file `eng/go/typebehavior.go`.
2. Add `Behavior TypeBehavior` to the `Type` struct
   (eng/go/typetable.go:48-55).
3. Provide `defaultBehavior` whose `Match` delegates to the
   existing `Type.Matches`, whose `Format` delegates to the
   existing `Value.String`, and whose `Equal` delegates to
   `ValuesEqual` (eng/go/compare.go).
4. In `TypeTable.MintType` and `registerBuiltin` (eng/go/typetable.go:204
   and :434), install `defaultBehavior{}` when the caller did not
   supply one.

**Crucially**: no caller is changed yet. `defaultBehavior.Format`
literally returns `v.String()`, so every existing render arm still
fires. `defaultBehavior.Match` literally returns `t.Matches(...)`,
so every existing dispatch is unchanged.

**Done when**: `make test` (lang + eng + cmd/go) green. Add
`TestTypeBehaviorDefaultsInstalled` to `eng/go/type_names_test.go`
asserting that every builtin `*Type` has a non-nil Behavior after
init.

### Step 2 — Introduce `v.Is(t)` and route the canonical dispatch points

**Goal**: every NEW call site uses `v.Is(t)`; the OLD path keeps
working.

**Work**:

1. Add `func (v Value) Is(t *Type) bool` to `value.go`, defined
   as `t.Behavior.Match(v, t)`. (Names: keep `Is` even though there
   is also a `is` word — they only collide in user-facing text.)
2. Route the four canonical match sites through `v.Is(t)`:
   - `IsValueOfType` (eng/go/core_type.go:371)
   - `sigTypeMatches` (eng/go/signature.go:259)
   - `Unify`'s lattice branch (eng/go/unify.go, the `aType.Matches`
     / `bType.Matches` block)
   - `rejectsTypeLiteral` (eng/go/signature.go:307)
3. Do **not** rewrite the 525+ `VType.Matches` / `VType.Equal` call
   sites yet. Those keep working via `defaultBehavior`. Only the
   four sites above are the canonical entry points the model
   defines; the others are local checks that can move later
   (or never, if `defaultBehavior` covers them).

**Done when**: `make test` green. New tests:
`TestValueIsDelegatesToBehavior` in `eng/go/value_test.go` (or a
new `behavior_test.go`); a regression run of
`lang/test/type_*_test.go` and the full `lang/spec/*.tsv` set
(via the spec runner) to ensure no dispatch regression. The
type-system spec coverage is dense — see § 9 below for the
specific assertions.

### Step 3 — Pluggable Format, draining `Value.String`'s domain arms

**Goal**: pull the six render arms for `Date`, `DateTime`, `Instant`,
`TimeOfDay`, `CalDuration`, `ClkDuration`, `Timezone`, `Matrix`,
`Timeout`, `Interval` out of `value.go` and register them via
`Behavior.Format` instead.

**Work**:

1. Create a per-type `Behavior` whose `Format` returns the exact
   string the old switch produced. Install it at the type's
   registration site (NOT in `value.go`):
   - For Time types: a new `eng/go/coretype_time_behavior.go`
     (or `lang/internal/nativemod/time/behavior.go` once Step 5+
     has moved the constants out of `eng`).
   - For Matrix: `lang/internal/nativemod/matrix/behavior.go`.
   - For Timeout/Interval: stays in `eng` until the entity
     concept moves; currently they live alongside the kernel's
     `timeout`/`interval` words.
2. In `Value.String`, replace the matched arms with a single
   delegation: `if b := v.VType.Behavior; b != nil { return
   b.Format(v) }`. Place this branch high in the switch so the
   primitive arms (Integer / Decimal / String / Boolean / Atom /
   None) keep their fast path — primitives still use
   `defaultBehavior.Format` which inlines to the current code.

**Done when**: `make test` green; the `value.go` switch shrinks by
~100 lines; **all** spec-runner output diffs are byte-identical.
The proof harness here is the spec runner — TSVs in `eng/spec/`,
`lang/spec/`, and `lang/test/check_fixtures` exercise `print` and
error rendering; a single character drift fails them.

### Step 4 — Equality / hash / compare reach the same seam

**Goal**: same dispatch story for `Equal` and (where capability
interfaces are supplied) `Hash`, `Compare`.

**Work**:

1. Route `ValuesEqual` (eng/go/compare.go) through `Behavior.Equal`.
   For everything that defines no override, `defaultBehavior.Equal`
   keeps today's deep-equal semantics — no diff on existing tests.
2. Domain types that need custom equality (CalDuration's
   normalisation, DepScalar's bound comparison) register their
   own `Equal`. The current ad-hoc code (`boundsEqual` /
   `depScalarsEqual` in `depscalar.go`) becomes the body of the
   DepScalar Behavior's `Equal`.

**Done when**: every test that does value comparison still
passes — particularly `lang/test/type_algebra_test.go` and
`lang/engine/compare_test.go`.

### Step 5 — Move domain `T*` constants to their owning modules

**Goal**: `eng/go/types.go` no longer names `TDate`, `TCalDuration`,
`TMatrix`, … nor do `builtinDecls` seed them.

**Work**:

1. Add a registration hook to `Registry` (or extend the existing
   `CapabilityRegistry`): something like
   `r.Types.RegisterExternalBuiltin(path, behavior) *Type`. This
   mints the `*Type`, wires its Behavior, and reserves a stable
   `FixedID` from a range outside the kernel's (e.g. 1000-1999 for
   `time`, 2000-2999 for `matrix`). The hook stores a per-module
   ID range so cross-version stability survives reorderings.
2. For each domain group (time, matrix, fetch, timeout/interval),
   create a `Register(r *Registry)` function in
   `lang/internal/nativemod/<group>/types.go` that calls the hook
   for every type it owns and exports `Time.Date`, `Time.DateTime`,
   etc. as package-level `*Type` variables (or accessor functions).
3. Update lang signatures (`lang/internal/nativemod/time.go` — 136
   references — and `matrix.go` — 76 references) to read these
   variables instead of `engine.TDate`. The `engine` package's
   `aliases.go` keeps re-exports during the transition, but as
   pointers into the nativemod-installed `*Type`s.
4. Delete the domain entries from `builtinDecls` (eng/go/
   typetable.go:325-412) and the `T*` constants from
   `eng/go/types.go`. The kernel keeps only the parser-emitted /
   loop-load-bearing types (Scalar/Integer/Decimal/String/Boolean/
   Atom/Path/None plus all Node/Word/Type kinds — see §5).

**Done when**: `make test` green; `grep -nE "TDate|TFetch|TMatrix|TTimeout|TInterval|TCalDuration|TClkDuration|TInstant|TTimeOfDay|TTimezone" eng/go/` returns zero hits. `eng_test` + `lang/test` + `lang/internal/nativemod/*_test.go` all pass. The full spec suite (`eng/spec/*.tsv`, `lang/spec/*.tsv`) is byte-identical.

**Risk to be aware of**: `FixedID` numbers are baked into serialised
value IDs (eng/go/typetable.go:496-511 — `formatFixedID` produces a
14-char ID embedded in `Value.ID`). If a serialised ID escapes via
the SDK cache or a stored module artifact, changing where a Type's
FixedID is assigned (kernel → external) can break round-tripping.
The migration must allocate the external ID range explicitly and,
where stability matters, preserve the old numbers exactly.

### Step 6 — Drain domain `IsX`/`AsX` methods from `eng/go/value.go`

**Goal**: the kernel's `Value` no longer carries methods named for
types it doesn't define.

**Work** (mechanical, but high call-site count):

1. For each `IsTimeout`, `IsInterval`, `IsArray`, `IsObjectInstance`,
   `IsRecordType`, `IsTableType`, `IsOptionsType`, `IsTypedList`,
   `IsTypedMap`, `IsDepScalar`, `IsDisjunct`, … rewrite callers
   to `v.Is(TFoo)` (or, where Behavior knows it, a
   capability-interface assertion). Then delete the method.
2. Same for the `AsX` accessors that are domain-payload getters.
   Move them as free functions to their owning packages:
   `time.AsDate(v) (time.Time, bool)`,
   `matrix.AsMatrix(v) (MatrixData, bool)`, etc.
   The kernel keeps `AsList`, `AsMap`, `AsString`, `AsInteger`,
   `AsDecimal`, `AsBoolean`, `AsAtom`, `AsPath`, `AsWord`,
   `AsForward` — and the structural accessors used by the
   interpreter itself.
3. The lang/engine `aliases.go` re-exports stay (so `lang/`
   callers don't break). The owning packages export the moved
   functions.

**Done when**: `make test` green; `wc -l eng/go/value.go` drops
substantially; `grep -nE "^func \(v Value\) (Is|As)[A-Z]"
eng/go/value.go` excludes every domain-only method.

### Step 7 — Lattice cleanup (consolidate `*Type` lattice walk and `Value` overrides)

**Goal**: `defaultBehavior.Match` carries the full
type-membership semantics, including DepScalar override, metatype
promotion, ChildType handling — anything that today is split
between `Type.Matches` and ad-hoc `Value`-side switches.

**Work**:

1. Pull `DependentLeafBaseType`'s hardcoded switch
   (eng/go/depscalar.go:176-192) into a per-Type registration: when
   a `DepInteger` `*Type` is registered, it learns its base
   (`TInteger`) via its Behavior. The leaf→base table goes away.
2. Pull `MetatypeFor`'s root-name switch (eng/go/types.go:274-287)
   into the root types' Behavior: `TScalar`'s Behavior knows it's
   the metatype-anchor for `ScalarType`. Adding a new root no
   longer requires editing `MetatypeFor`.
3. Move `ChildTypeInfo` handling out of the `Value.String` `TList`
   and `TMap` arms (eng/go/value.go:1798, 1855) into the List /
   Map Behaviors. List Behavior's `Format` checks for
   `ChildTypeInfo`, `TableTypeInfo`, `TableData`, `Materializer`
   payloads — same code path, lives on List's Behavior now.

**Done when**: `make test` green; `Type.Matches` becomes a thin
wrapper that delegates to `Behavior.Match`. `Type.Matches` may
remain as a convenience name, or be deleted in favour of
`v.Is(t)`.

### Step 8 *(optional)* — Parser hand-off

**Goal**: push `Time` and other parser-coupled types out of the
kernel entirely.

**Work**: as the prior draft described — the parser emits neutral
literal tokens (`LiteralWithSyntax{kind: "iso-duration", raw:
"P1Y2M3D"}`) that the temporal module post-converts on first use.
This affects every spec that hand-writes a Date / Duration value
and requires a spec rebaseline. Out of scope for the first PR
series.

### Effort summary

| Step | Work | Risk | Test surface |
|---|---|---|---|
| 0 | Inventory, no code change | None | none — green by definition |
| 1 | Add seam + defaults | Very low | type_names_test, behavior_test |
| 2 | Route 4 canonical sites through `v.Is` | Low | type_*_test.go, all spec TSVs |
| 3 | Pluggable Format for domain types | Low — byte-equal output gate | spec TSV diff |
| 4 | Pluggable Equal / Hash / Compare | Low | compare_test, type_algebra_test |
| 5 | Move domain T* out of eng | **Medium** — FixedID stability | nativemod tests, full rollup |
| 6 | Drain domain IsX/AsX | Low (mechanical) | full rollup |
| 7 | Consolidate lattice into Behavior | Medium — corner cases | depscalar_test, type_predicate_*, fnvariance_test |
| 8 | Parser hand-off | High — spec rebaseline | every TSV that names a Time / Matrix literal |

Steps 1-4 are a single PR. Steps 5-6 are a second PR. Step 7 is a
third PR. Step 8 is a separate proposal.


## 7. Worked examples

### 7.1 Adding a new type from a native module

Before — today a native module wanting to ship a `Color` type
cannot. The kernel must be patched: a row goes into
`builtinDecls`, a `TColor = mustType(...)` line lands in
`types.go`, an `IsColor` method may grow on `Value`, render arms
appear in the `Value.String` switch, and lang code references
`engine.TColor`. The kernel ends up knowing about Color even
though Color is purely a module concern.

After — every step happens inside the module:

```go
// lang/native/color/register.go (no kernel patch)
package color

import "github.com/aql-lang/aql/eng"

var TColor *eng.Type   // exported for downstream signature use

// behavior is the per-Type ops bundle for Color.
type behavior struct{}

func (behavior) Match(v eng.Value, t *eng.Type) bool {
    if v.VType != t { return false }
    _, ok := v.Data.(RGB)
    return ok
}
func (behavior) Format(v eng.Value) string {
    rgb, _ := v.Data.(RGB)
    return fmt.Sprintf("#%02x%02x%02x", rgb.R, rgb.G, rgb.B)
}
func (behavior) Equal(a, b eng.Value) bool {
    ar, _ := a.Data.(RGB); br, _ := b.Data.(RGB)
    return ar == br
}

// Optional capability: Comparer. Equipping the type with this
// makes `sort` work on Color values without any kernel change.
func (behavior) Compare(a, b eng.Value) int { ... }

func Register(r *eng.Registry) {
    TColor = r.Types.RegisterExternalBuiltin(
        "Object/Color",         // path under Object root
        behavior{},             // the ops bundle
    )
    // Now register words that produce / consume Color.
    r.RegisterNativeFunc(eng.NativeFunc{
        Name: "rgb",
        Signatures: []eng.NativeSig{{
            Args:    []*eng.Type{eng.TInteger, eng.TInteger, eng.TInteger},
            Returns: []*eng.Type{TColor},
            Handler: rgbHandler,
        }},
    })
}

func rgbHandler(args []eng.Value, ...) ([]eng.Value, error) {
    r, _ := args[0].AsConcreteInteger()
    g, _ := args[1].AsConcreteInteger()
    b, _ := args[2].AsConcreteInteger()
    return []eng.Value{eng.NewValueRaw(TColor, RGB{byte(r), byte(g), byte(b)})}, nil
}

// AsColor is the kernel-free payload accessor — owned by this
// package, not by eng/.
func AsColor(v eng.Value) (RGB, bool) {
    if v.VType != TColor { return RGB{}, false }
    rgb, ok := v.Data.(RGB)
    return rgb, ok
}
```

The host wires it up the same way the math / time modules wire
today — `color.Register(r)` from `DefaultRegistry()` (or from a
`module 'aql:color'` import-resolver entry, if the module is
opt-in). The kernel learns nothing about Color. `v is Color`,
`print someColor`, `someColor eq someColor`, and `sort
listOfColors` all work, because the dispatch path consults
`TColor.Behavior` and the optional Comparer capability.

### 7.2 Adding a new type from an aql source module

Three flavours, all syntactically already supported today —
post-decoupling they go through the same Behavior pipeline as
any kernel type, no special-casing.

**Refinement of a kernel type** — `type Foo Integer`:

```aql
type Foo Integer
def x:Foo 5
def y:Foo 'hello'    # error: 'hello' is not a Foo
```

What `installType` (eng/go/core_type.go:476-530) does:
- `r.Types.MintType("Foo", parent=TInteger)` mints a fresh
  `*Type` whose Parent is TInteger.
- The minted Type inherits TInteger's Behavior (the kernel's
  default Integer Behavior). `Match` for a Foo value passes when
  the value is an Integer (parent walk) AND `v.VType == TFoo` (or
  a descendant) — i.e. only values *declared* as Foo, not any
  Integer, satisfy `is Foo`. (Today this nuance is handled by the
  predicate-type path; Step 2 routes it through Behavior.)
- `def x:Foo 5` calls `TFoo.Behavior.Match(NewInteger(5), TFoo)`.
  Default Behavior returns true (5 satisfies Integer ⊆ Foo's
  parent), installs `x` as Integer carrying the Foo declaration.

**Predicate type** — `type Even fn [n:Integer Boolean [n 2 mod 0 eq]]`:

What `fn` (the constructor word) does:
- Builds an `FnDefInfo` carrying the body.
- `type Even <fnDef>` calls `MintType("Even", parent=TInteger)`
  and installs a `predicateBehavior{fn: fnDef}` on the minted
  Type. The Behavior's `Match` invokes the predicate body via
  `RunPredicate` (registry.go:822) — already the path predicate
  types use today.
- `v is Even` → `TEven.Behavior.Match(v, TEven)` → predicate
  body runs against v, returns Boolean. No new dispatch logic.

**Record type** — `type Point record { x:Decimal y:Decimal }`:

What `record` builds:
- A `RecordTypeInfo` describing the field shape.
- `type Point <record>` mints a Type whose Behavior is a
  `recordBehavior{ shape: recordTypeInfo }`. `Match` checks
  field-by-field conformance (today's `IsValueOfType` map
  recurse, eng/go/core_type.go:411-431). `Format` prints
  `Point{x:1.0 y:2.0}`. `Equal` does per-key compare.
- Nominal vs structural is preserved: a bare `{x:1.0 y:2.0}` map
  has VType=TMap, so `m is Point` returns true only via the
  recordBehavior's Match (structural conformance), but `typeof m`
  remains `Map` — `Point` is a TYPE name, not the value's
  VType. Today's `IsRecordShape` distinction (eng/go/core_type.go:299)
  is the same; only the dispatch path changes.

In all three cases the answer to "what changed in the kernel?" is
"nothing" — `installType` now just installs a Behavior on the
minted Type. The kernel doesn't need to know about Foo / Even /
Point because the kernel doesn't dispatch on them by name — it
dispatches via the Behavior.

### 7.3 Exporting a type from one module to another

```aql
# in colors.aql
type Color record { r:Integer g:Integer b:Integer }
type Palette { primary:Color secondary:Color }

# in main.aql
'colors.aql' module
def p:Palette { primary:{r:255 g:0 b:0} secondary:{r:0 g:0 b:255} }
```

The existing module mechanism (lang/engine/native_module_module.go)
already exports type bindings — `r.Types.PopType` returns the
body, the export map carries it, the importing registry installs
it via `r.Types.PushType` again. Because the type's Behavior
travels with the `*Type` (the Behavior is a field on the Type, not
on the registry), the exported type works identically in the
importing scope without any additional plumbing.


## 8. Test gating per step

Each step lands behind a specific set of tests; the rollout breaks
no green tests at any point. The mapping (alphabetised under each
step):

**Step 1 — defaults installed**
- `eng/go/type_names_test.go::TestTypeBehaviorDefaultsInstalled` (new)
- `make test` rollup green

**Step 2 — `v.Is(t)` canonical dispatch**
- `eng/go/type_names_test.go` (unchanged — pass through delegation)
- `lang/test/istype_test.go`
- `lang/test/typed_def_test.go`
- `lang/test/type_algebra_test.go`
- `lang/test/type_depscalar_safety_test.go`
- `lang/test/type_distribute_test.go`
- `lang/test/type_error_messages_test.go`
- `lang/test/type_fnsig_test.go`
- `lang/test/type_fnvariance_test.go`
- `lang/test/type_guard_test.go`
- `lang/test/type_inspect_test.go`
- `lang/test/type_namespace_test.go`
- `lang/test/type_never_test.go`
- `lang/test/type_predicate_arity_test.go`
- `lang/test/type_predicate_sandbox_test.go`
- `lang/test/type_shadow_test.go`
- spec runner: `eng/spec/dispatch.tsv`, `eng/spec/mirror.tsv`,
  `eng/spec/pattern.tsv`, `eng/spec/record.tsv`,
  `lang/spec/list.tsv`, `lang/spec/map.tsv`

**Step 3 — pluggable Format**
- `lang/test/error_format_test.go` (renders types in errors)
- `lang/test/type_error_messages_test.go`
- `lang/test/check_fixtures_test.go` (golden text)
- spec rollup byte-identical diff: `eng/spec/*.tsv`,
  `lang/spec/*.tsv`, `lang/test/check_fixtures/*`
- `lang/internal/nativemod/time_test.go`,
  `lang/internal/nativemod/matrix_test.go`

**Step 4 — pluggable Equal**
- `lang/engine/compare_test.go`
- `lang/test/type_algebra_test.go`
- `lang/test/type_depscalar_safety_test.go`

**Step 5 — domain `T*` move**
- All Step-2/3/4 tests (regression set)
- `lang/internal/nativemod/time_test.go` (now uses the
  module-owned `*Type`)
- `lang/internal/nativemod/matrix_test.go`
- `lang/test/factorial_type_scaling_test.go` (touches type
  IDs in its assertions; verify nothing references a hardcoded
  FixedID range that moved)
- Verify FixedID stability: a unit test that snapshots
  `Builtin.byID` plus the per-module externally-registered IDs.

**Step 6 — drain domain IsX/AsX**
- Whole `make test` rollup — every Value-method-using call site
  in `lang/` (mechanical)
- `lang/test/object_type_test.go`
- `lang/test/resource_type_test.go`
- `lang/test/typecheck_test.go`

**Step 7 — lattice cleanup**
- `lang/test/type_depscalar_safety_test.go` (dep-scalar override
  is the highest-risk corner)
- `lang/test/type_predicate_arity_test.go`,
  `lang/test/type_predicate_sandbox_test.go`
- `lang/test/type_fnvariance_test.go`
- `lang/engine/path_subtype_test.go`
- spec rollup: full pass — corner cases live in TSVs

After each step, the gating commands are:
```
cd lang && make test    # rolls up lang + eng + cmd/go
cd lang && make vet
```
The CI gate is `make test`; the local fast-feedback loop is
`go test ./test/ -run TestX -v` against any individual test in the
list above.


## 9. Open questions before implementation

1. **`FixedID` allocation policy for external types.** Step 5
   needs a clear answer to "if module M defines type T at version
   v1, then at v2 splits T into T1 and T2, what FixedID do T1 and
   T2 get?" Proposal: the host declares an explicit FixedID per
   external type so reorderings inside the source file don't
   reshuffle IDs. The hook signature becomes `RegisterExternalBuiltin(
   path string, fixedID int, behavior TypeBehavior) *Type`.

2. **Compatibility surface for `Type.Matches` after Step 7.** Many
   `VType.Matches(TInteger)` call sites in non-eng code (234 in
   `lang/engine/`) read naturally. Proposal: keep `Type.Matches`
   as a method delegating to `Behavior.Match`. No mechanical
   rewrite needed in lang/; the cost is one extra interface call,
   already covered by the §7 cost analysis.

3. **Carrier values and Behavior.** Carriers (Value.Carrier =
   true) have nil Data but represent abstract values, not types
   (eng/go/signature.go:236-258). The default Behavior must
   short-circuit Carrier values to a known result so check-mode
   continues to work. Suggestion: a Carrier value matches t iff
   the carrier's declared VType is a subtype of t — i.e. exactly
   today's logic, formalised inside `defaultBehavior.Match`.

4. **`lang/engine/native_type.go::validateAndInstallType` vs
   `eng/go/core_type.go::installType`.** Two near-identical
   implementations exist (compare line 221-263 vs 476-530). The
   lang version accepts `TString` for the name (the `type 'Foo' …`
   variant); the eng version accepts only `TAtom`. Consolidate at
   Step 1 or Step 2 — they're both doing the same MintType + Bind
   dance, and the duplication will become an active hazard once
   Behavior wiring lands.

5. **What happens to the `eng/CLAUDE.md` documentation block on
   the kernel/domain split?** The "registry stacks" section
   already calls out `r.Types` as the canonical type-storage
   surface. The decoupling adds a Behavior layer; the docs need
   one new section explaining when to provide a custom Behavior
   versus accepting the default.


## 10. Trade-offs

### What you gain

- **User modules become first-class.** A temporal-module-defined
  CalDuration is indistinguishable from a kernel-defined Integer at
  the dispatch site. Same for user records, user objects, predicate
  types — all behave uniformly under `v.Is(t)`.
- **One unified mechanism.** `v.Is(t)` replaces 70 ad-hoc methods.
- **Smaller kernel surface.** `Value.String`'s switch shrinks from
  ~200 lines to a 4-line delegation. The 50+ domain accessor methods
  vanish from `eng/`.
- **Clearer policy.** "Why is TInteger in the kernel but TDate is
  not?" has a real answer: the parser produces Integer literals; it
  doesn't produce Date literals. Defensible boundary.

### What it costs

- **Pointer indirection on hot paths.** Today's
  `v.VType.Matches(TInteger)` inlines easily; the new
  `v.VType.Behavior.Match(v, TInteger)` is an interface call. Likely
  measurable on the matching path (called once per arg per dispatch).
  The kernel-primitive Match path can keep its fast path: default
  Behavior's `Match` is itself a small function the compiler may
  inline; for the loop-critical case it might be worth a specialised
  "ints fast" check before the interface call.
- **Interface evolution cost.** Adding a new operation to
  `TypeBehavior` is a breaking change for every implementor. Mitigate
  with capability sub-interfaces (`Hasher`, `Comparer`, `Walker`,
  ...) — required `TypeBehavior` stays minimal.
- **The kernel/domain boundary becomes policy.** Today it's
  visible: `TInteger` is in `eng/types.go`. After the refactor, it's
  still in `eng/types.go` but the *reason* is "the parser emits it
  directly" — a more nuanced justification that someone could erode
  by adding a "small convenience" kernel type. Worth a documented
  rule in `eng/CLAUDE.md`.

### What's actually delivered by each step

If steps 1-4 land but 5-8 don't, **90% of the user-facing benefit is
already there.** Steps 1-4 give the unified dispatch point, the
pluggable formatter, and pluggable equality — together meaning that
a temporal-module-defined CalDuration is indistinguishable from a
kernel-defined Integer at the call site. Steps 5-6 move and delete
the now-redundant T* constants and IsX/AsX methods. Step 7 is the
lattice cleanup that can be deferred until it bites. Step 8 is the
only piece with real semantic risk and can be left out if the
parser-coupling is acceptable.

**Recommended landing plan**: steps 1-4 as a single PR, ship,
observe, then decide whether 5-8 are worth it. The first PR is the
load-bearing one — everything after is mechanical or optional.
