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

```
Value.IsX methods    : 30
Value.AsX methods    : 40
VType.Matches/Equal  : ~hundreds of call sites across eng + lang
Value.String switch  : 20 arms for non-primitive types
```

Of the As* methods for domain types (Date, DateTime, Instant,
TimeOfDay, CalDuration, ClkDuration, Timezone, Matrix, Timeout,
Interval), **zero are called from inside `eng/`** except for two
sites in `Value.String` (Timeout, Interval). All real callers are in
`lang/internal/nativemod/*`. The kernel only *exports* these methods;
it doesn't *use* them.


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


## 6. Migration shape

A deep change, not a one-week refactor. Sketch:

| Step | Description | Effort |
|---|---|---|
| 1 | Add `Behavior` field to `Type`; supply default `TypeBehavior` impl that replicates today's lattice-walk Match, fmt-style Format, deep-equal Equal. Behavior=nil → use default. Backwards-compatible: nothing else changes yet. | 1-2 days |
| 2 | Introduce `v.Is(t)` and route all `VType.Matches`/`VType.Equal` call sites through it via sed + audit. | 2-3 days |
| 3 | Move 6 domain-type render arms out of `Value.String` — register `Behavior.Format` on each *Type* at runtime. | 1 day |
| 4 | Delete the domain As* methods (`AsCalDuration`, `AsMatrix`, `AsTimeout`, …); move them to free functions in their owning modules (`time.AsCalDuration` etc.). | 1 day mechanical |
| 5 | Delete the domain Is* methods (`IsTimeout`, `IsInterval`); callers use `v.Is(TTimeout)` instead. | 1 day mechanical |
| 6 | Lattice cleanup: today `Match` is implemented half on `*Type` (the lattice walk) and half ad-hoc on `Value` (sig matching, `ChildTypeInfo` special-cases, dep-scalar override). Pull all of that into a single `defaultBehavior` impl that the kernel installs at MintType time. | 2-3 days |
| 7 *(optional)* | Parser hand-off: today the parser bakes TDate, TCalDuration into produced values. To push those types out of `eng` entirely, the parser needs to emit neutral literal tokens (`LiteralWithSyntax{kind: "iso-duration", raw: "P1Y2M3D"}`) that the temporal module post-converts. Real semantic change. | ~1 week + spec rebaseline |

**Total: 2-3 weeks for steps 1-6**, more if step 7 is in scope.
Touches a few hundred files, mostly mechanical, but step 6 has real
complexity around lattice corner cases (ChildTypeInfo, dep-scalar
override, table types).


## 7. Trade-offs

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

If steps 1-3 land but 4-7 don't, **90% of the user-facing benefit is
already there.** Steps 1-3 give the unified dispatch point and the
pluggable formatter, which together mean a temporal-module-defined
CalDuration is indistinguishable from a kernel-defined Integer at the
call site. Steps 4-5 are cosmetic (delete dead method count). Step 6
is the lattice cleanup that can be deferred until it bites. Step 7 is
the only piece with real semantic risk and can be left out if the
parser-coupling is acceptable.

**Recommended landing plan**: do steps 1-3 as a single PR, ship,
observe, then decide whether 4-7 are worth it. The first PR is the
load-bearing one — everything after is mechanical or optional.
