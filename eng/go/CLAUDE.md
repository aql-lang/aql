# eng/go — Kernel CLAUDE.md

The `eng` module is the AQL kernel: types, values, signatures,
matching, the step loop, and the parser bridge. This file
documents conventions specific to this module — anything that's
language-wide rather than lang-specific lives here.

For language-layer conventions (jsonic integration, registry
stacks, helper API discipline, panic prevention) see
`lang/go/CLAUDE.md`.

## Sealed Payload (CRITICAL)

`Value.Data` is a sealed interface: `eng.Payload`. Only types
with the unexported `payloadMarker()` method satisfy it, and the
method is only definable in this package. The seal closes the
historical `Data interface{}` hole — `Value{Parent: TInteger,
Data: "hello"}` is a **compile error**.

Payload variants live in `eng/go/payload.go`. Two flavours:

1. **Wrapper variants** — for Go built-in types we can't add
   methods to: `IntPayload{N: int64}`, `StrPayload{S: string}`,
   `BoolPayload{B: bool}`, `DecPayload{F: float64}`,
   `AtomPayload{Name: string}`, `PathPayload{Info: PathInfo}`,
   `ListPayload{Elems: []Value}`, `MapPayload{M: *OrderedMap}`,
   `ParenExprPayload{Toks: []Value}`,
   `InterpStringPayload{Parts: []InterpPart}`,
   `TimePayload{T: any}`, `DurationPayload{D: any}`,
   `TimezonePayload{Loc: any}`, `MaterializerPayload{M: Materializer}`,
   `NonePayload{}`, `ExtensionPayload{Body: any}`.

2. **Direct variants** — eng-defined struct/pointer types with
   `payloadMarker()` added in payload.go: `WordInfo`,
   `ForwardInfo`, `MarkInfo`, `MoveInfo`, `ReturnCheckInfo`,
   `DefCleanupInfo`, `ModuleDesc`, `FnDefInfo`, `FnUndefInfo`,
   `DisjunctInfo`, `ChildTypeInfo`, `RecordTypeInfo`,
   `OptionsTypeInfo`, `TableTypeInfo`, `TableData`,
   `ObjectTypeInfo`, `ObjectInstanceInfo`, `*StoreInstanceInfo`,
   `*ArrayInstanceInfo`, `*TimeoutInfo`, `*IntervalInfo`,
   `ErrorInfo`, `CalDurationData`, `DepScalarInfo`,
   `PathInfo`, `noneSentinel`.

When adding a new kernel-known payload shape, register the
marker in `payload.go` (either as a wrapper struct or with a
`func (Foo) payloadMarker() {}` line). Without the marker the
compiler refuses to put it in a Value.

For plugin/host-supplied payloads, use `ExtensionPayload` — its
`Body any` is the explicit escape hatch the kernel does NOT
inspect.

## Type Behavior

Every `*Type` carries a `Behavior` field of type
`TypeBehavior`:

```go
type TypeBehavior interface {
    Match(v Value, t *Type) bool
    Format(v Value) string
    Equal(a, b Value) bool
}
```

`DefaultBehavior` is the kernel's no-op: `Match` delegates to
`v.Parent.Matches(t)`, `Format` delegates to `v.String()` (with
the dispatch carefully avoiding re-entry), `Equal` delegates to
`valuesEqualDefault`. Every type registered through the kernel
paths gets `DefaultBehavior` if the caller doesn't supply one.

Types with semantics the kernel can't infer (time formatting,
matrix rendering, predicate-type matching, refinement-type
matching, plugin types) supply a custom Behavior:

- `lang/go/native/native_temporal.go` — Time family Behaviors.
- `lang/go/native/native_misc.go` — Timeout/Interval Behaviors.
- `lang/go/modules/matrix.go` — Tensor/Matrix/Vector Behavior.
- `lang/go/native/fetch.go` — Fetch family (no custom Behavior; uses Default).

The dispatch in `Value.String` walks the Parent chain so
descendants of a type with a custom Behavior inherit it — e.g.
`Node/Map/Inspect` (descendant of `Node/Map`) inherits the
`Node/Map` map-formatting Behavior without per-subtype
registration.

Optional capability interfaces (`Comparer`, `Hasher`, `Walker`)
let a type opt into extra operations without expanding the
required `TypeBehavior` surface.

## Where a Type Lives (kernel/domain boundary)

**Rule**: a type stays kernel-declared (in
`eng/go/typetable.go::builtinDecls` and `eng/go/types.go`'s `T*`
constants) **iff** one of these holds:

1. The parser emits it directly: `Integer`, `Decimal`, `String`,
   `Boolean`, `Atom`, `Path`, `None`, `List`, `Map`.
2. The interpreter loop branches on it structurally: `Word`,
   `Forward`, `Mark`, `Move`, `OpenParen`, `CloseParen`, `End`,
   `ReturnCheck`, `DefCleanup`, `ParenExpr`, `InterpString`,
   `Module`.
3. It is a meta-type used by `is`/`typeof`/`inspect`: `Type`,
   `Function`, `FnDef`, `FnUndef`, `Disjunct`, `Enum`.
4. It is a structural type used by `make`/`record`/`object`:
   `Record`, `Options`, `Table`, `ChildType`, `ObjectType`,
   `ObjectInstance`, `Store`, `Array`, `Error`.

Everything else — domain types like `Date`, `DateTime`,
`CalDuration`, `Matrix`, `Timeout`, `Interval`,
`Fetch{Function,Request,Response}`, all user types, all plugin
types — **flows through `RegisterExternalBuiltin`**:

```go
// In the type's owning package:
var TFoo = registerFooType()

func registerFooType() *eng.Type {
    t, err := eng.Builtin.RegisterExternalBuiltin(
        "Object/Foo",  // path
        N,             // stable FixedID from the documented per-module range
        fooBehavior{}, // optional; nil → DefaultBehavior
    )
    if err != nil {
        panic(fmt.Sprintf("foo: register: %v", err))
    }
    return t
}
```

The var-initialiser pattern (rather than `init()`) is important:
package-level vars that reference TFoo (signature slices in
particular) need TFoo non-nil at slice-init time. Go resolves
var-init dependencies before declaration order.

If something would naturally live in `eng/` but the language
layer needs it (e.g. type registration policy, payload-marker
rules), the concern is **language-wide** and belongs here, not
duplicated in lang.

## FixedID Allocation

FixedIDs are baked into serialised Value IDs
(`eng/go/typetable.go::formatFixedID` produces a 14-char ID
embedded in `Value.ID`). Changing an existing type's FixedID is
a wire-compatibility break.

Documented per-module ranges (see
`TypeTable.RegisterExternalBuiltin` doc):

```
   1-99       eng kernel builtins
   100-999    reserved for future eng-internal builtins
   1000-1999  lang/go/native — Scalar/Time family
   2000-2999  lang/go/modules/matrix
   3000-3999  lang/go/native/fetch
   4000-4999  lang/go/native — Timeout, Interval
   5000-9999  reserved for future kernel/language allocations
   10000+     host / third-party plugin types
```

The regression gate is
`lang/go/test/fixedid_stability_test.go::TestFixedIDStability` — it
snapshots every known FixedID and fails on drift. Adding a new
externally-registered type means:

1. Pick a FixedID from your module's reserved range.
2. Add the path → FixedID entry to the snapshot.
3. The test asserts your type registers at the expected ID.

## Type Lattice Fields

`Type` carries two fields populated at registration time that
replace historical hardcoded switches:

- `Metatype *Type` — for the three anchor roots (`TScalar` →
  `TScalarType`, `TNode` → `TNodeType`, `TObject` →
  `TObjectType`). Descendants inherit by Parent-chain walk in
  `MetatypeFor`. Populated via `builtinDecl.MetatypePath` for
  kernel-declared roots.

- `Rank int` — the **unified lattice rank**: one integer giving the
  total order `CompareValues` / `compareTypes` use for every cross-type
  ordering (it replaced the old per-branch `rootBranchRank` /
  `scalarBranchRank` / family-rank ladders). It is positional — a
  type's Rank is its parent's Rank plus a depth-scaled offset, so a
  builtin child always ranks above its parent and siblings run
  least-to-most complex. The scheme (1e10 root bands, +1e8 / +1e7 per
  depth, …) is laid out on `typetable.go::builtinDecls`. Kernel types
  get a positional Rank from `builtinDecl.Rank`; user types
  (`MintType`) and external builtins (`RegisterExternalBuiltin`)
  inherit the parent's Rank, and `compareTypes` breaks the resulting
  ties by depth, then name, then id. `rankOf` (`compare_types.go`)
  walks the parent chain as a fallback for a `*Type` assembled without
  one.

When introducing a new root with its own metatype anchor, add
`MetatypePath` to its `builtinDecl` row.

## Value Has Two Methods

`Value` exposes exactly two methods:

- `Is(t *Type) bool` — canonical dispatch. Routes through
  `t.Behavior.Match(v, t)`.
- `String() string` — `fmt.Stringer` interface.

Every former `IsX` / `AsX` accessor is now a free function in
this package. `Value.AsInteger()` → `eng.AsInteger(v)`. The
lang-layer aliases re-export them so `engine.AsInteger(v)` works
from lang/* packages.

Do NOT add new methods to `Value`. Add free functions instead.
Methods on `Value` accumulate API surface and become coupling
points; free functions are equally callable and can be moved
between packages without affecting the kernel.

## Type installation

A capitalised `def Foo body` installs a type binding (the
TYPE-UNIFORM surface: `def` binds, `make` instantiates, `refine`
constructs — the legacy `type`-binder / `object` / `record` /
`table` / `untype` words were removed in Phase 3, and the `type`
constructor was renamed to `refine`).

The single source of truth is `eng/go/core_type.go::InstallType`. It
validates `body` is a valid type body, mints the lattice identity
via `TypeTable.MintType`, and binds it in the single `DefTable`
(`PushType`, carrying the minted `*Type`). `def`'s handler delegates
here for capitalised names regardless of which surface (eng or lang)
registered `def` — do not fork the logic. `undef` of a capitalised
name pops the binding and retires the minted type
(`TypeTable.Retire`).

If you need to extend the installation policy (a new name shape, an
extra validation rule), modify `InstallType` so every surface picks
it up.
