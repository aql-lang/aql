# FLEX-NODES.0 — FlexMap and FlexList: mutable Node child types

Status: implemented.

## Problem

AQL's Node types (`Map`, `List`) are immutable: the mutation words
(`push`, `pop`, …) return new copies, and `set` deliberately has no
Map/List signatures. Mutable state therefore lives only in the Ideal
types — `Array` (indexed store), `Object` (fields), `Store`
(copy-on-write context) — none of which participate in Node structural
behavior (list/map literals, structural unification, `deq`, the
list/map word vocabulary). Building a map or list incrementally meant
either re-binding a fresh copy on every step or leaving the Node world
for an Ideal and converting back at the end.

## Design

Two new kernel types, children of the immutable Nodes:

```
Node/Map/FlexMap     FixedID 78   Rank 30_220_000_000
Node/List/FlexList   FixedID 79   Rank 30_120_000_000
```

A flex node is **the same data shape as its parent, plus in-place
mutability**. Subtyping does all the inheritance work: signature
dispatch routes through `v.Is(t)` → `ConformsTo`, so a FlexList matches
every `List` slot and a FlexMap every `Map` slot — `get`, `each`,
`sort`, `size`, record/table `make` sources, Options slots, formatting
(Behavior dispatch walks the Parent chain) all work unchanged. The
deeper lattice Rank makes Flex-specific signatures automatically more
specific than the plain ones in `CompareSignatures`, so the mutation
words need no manual ordering.

### Payloads

- **FlexMap reuses `MapPayload`** — its `*OrderedMap` is already
  pointer-backed, so every `Value` copy shares the store and `AsMap` /
  `AsMutableMap` work untouched. (Plain Maps are "immutable" only
  because no word mutates them; the payload was always shareable.)
- **FlexList gets a new pointer payload** `*FlexListData{Elems []Value}`
  (`eng/go/payload.go`). `ListPayload` holds the slice by value, so
  growth (append/push) could never be observed through other copies.
  `AsList`/`AsMutableList` read it; the mutating handlers go through
  `AsFlexList` to reassign `Elems` through the pointer.

### Construction is DEEP COPY — never aliasing

`flex <node>` (sugar) and `make FlexMap m` / `make FlexList l` produce a
deep copy: fresh containers at every level, nested Nodes converted to
flex recursively (`eng/go/core_flex.go::FlexDeepCopy`). Two reasons:

1. **No leaks.** Plain map literals share `*OrderedMap` pointers; a
   shallow flex over one would let `set` mutate "immutable" data that
   other bindings still see.
2. **Mutability at depth.** `flex {a:{b:1}}` must allow
   `set b/q 9 f.a` — so the nested map has to be flex too.

The inverse `node <val>` (= `make Map v` / `make List v`)
deep-converts back (`NodeDeepCopy`), recursing through plain containers
so nested flex is normalised at every depth — the result is
transitively immutable. A container with **no** flex anywhere inside is
returned unchanged (identity), preserving container-identity equality
for `node plainmap`.

Scalars and non-Node payloads (Object, Array, Store, Function, …) pass
through both conversions unchanged: scalars are immutable, and
pointer-backed Ideals already have documented shared-mutation
semantics. A typed list/map that carries concrete elements alongside
its constraint flexes to a plain flex node — the child constraint is
dropped (known limitation). Flexing a bare type body (record, options,
table) is an error.

### Mutation surface (full family) and return convention

| Word | FlexMap | FlexList | Plain Map/List |
|---|---|---|---|
| `set key val n` | in place (String / quoted-Atom keys) | in place (Integer index) | no signature — immutable |
| `append v n` | — | in place; List arg concatenates, others append as one element | no signature |
| `push` / `unshift` | — | in place | new copy (unchanged) |
| `pop` / `shift` | — | in place | new copy (unchanged) |

**Mutators return the flex node itself** so calls chain
(`append 4 fl append 5`); `pop`/`shift` return the node plus the
removed element on top, mirroring the plain forms' `[newList, removed]`
stack shape. (Existing `set` on Array/Object returns nothing; the flex
convention favors chaining — flex nodes are references, the return is
free.)

**`set` bounds: `0..len-1` only.** `idx == len` is an error, not an
append: `set` means "replace an existing slot", growth has a dedicated
word, and silently accepting `len` makes off-by-one growth bugs
invisible. Anything past the end is the same error — **sparse
FlexLists are an error, never padded with None**. The error hints
"use append to grow".

**Storage is by reference.** `set`/`append` store the value as given —
appending a plain map into a FlexList keeps it immutable inside;
`flex` it first if it must be mutable. Containment is the job of the
conversion words, not the storage words (consistent with map values
and closure capture sharing pointer-backed payloads everywhere else).

There is no map-key *deletion* word in core today (no `del`/`delete`);
when one is added it should get an in-place FlexMap signature
(future work).

### Equality, ordering, matching

- **Flexness is a mutability mode, not value identity.**
  `valuesEqualDefault`, `listsEqual`, `mapsEqual`, `DeepEqual`, and
  `compareValuesClassified` normalise via `nodeFamily(t)`
  (TFlexMap→TMap, TFlexList→TList — exact, never `ConformsTo`, so
  Inspect/Args keep their identity). Hence `(flex {a:1}) deq {a:1}` →
  true (deep), and `cmp` orders flex/plain pairs by content. Bare type
  literals are *not* normalised: `List tcmp FlexList` stays a Rank
  ordering.
- **`eq` (ExactEqual) stays container identity**: a flex copy is a
  fresh container, so `(flex m) eq m` is false; `dup`'d/aliased flex
  nodes are `eq`.
- **`is`/unify are nominal-subtype**: a `FlexMap` literal matches only
  FlexMap-tagged values (`{} is FlexMap` → false); the supertype
  literals `Map`/`List` accept flex values (`(flex {}) is Map` → true)
  and unify returns the flex value (narrowing keeps the more specific
  side). Concrete-vs-concrete unification *outputs* are plain nodes.
- **Canon** renders flex nodes round-trippable — `(flex {a:1})` /
  `(flex [1 2])`; `Value.String` stays the inherited plain rendering.
- **Shape()** classifies concrete flex nodes as ShapeMap/ShapeList via
  explicit `TFlexMap`/`TFlexList` checks.

### `make` dispatch

One new signature: `{Args: [Node, Any], TypeArgs: {0}, Handler:
MakeNodeHandler}` (`native_make.go`), sorting between `[Ideal, Map]`
and `[Scalar, Any]`. The handler (`core_flex.go::MakeNodeHandler`):

1. **Defers structural type bodies** (`!IsBareTypeNode`) to
   `MakeHandler` — RecordTypeInfo etc. have `Parent=TMap`, which
   conforms to Node, and must keep their Ideal instantiation path.
2. **Defers to host-registered Ideal kinds** (registry-first dispatch,
   like `MakeHandler`/`MakeObjHandler`) so a host claiming a
   Node-family target still owns it.
3. Owns the four Node targets: FlexMap/FlexList → family-check +
   `FlexDeepCopy` (`make FlexMap [1 2]` errors); exactly Map/List →
   family-check + `NodeDeepCopy`. This also fixes the trap where
   `make Map flexmap` would have returned the flex node unconverted
   through MakeHandler's ConformsTo-identity path.
4. Anything else (Node itself, Inspect, Args) → `MakeHandler`.

`make Map {}` / `make List []` therefore work symmetrically (deep-
normalising identity on a plain literal).

## FlexList vs Array — purpose and abilities

| | `FlexList` (Node) | `Array` (Ideal) |
|---|---|---|
| World | Node — full structural list behavior | Ideal — opaque to Node structure |
| Dispatch | matches every `List` sig slot | only `Array` slots |
| Vocabulary | all list words (each/sort/take/reverse/…) + append/push/pop/unshift/shift in place | `set`/`get` by index + APL vocabulary (iota, reshape, transpose, where, …) |
| Growth | `append`/`push`/`unshift` | fixed length per instance (`set` is in-bounds only) |
| Equality | structural (`deq` by content, family-normalised) | reference semantics |
| Conversion | `node`/`flex` round-trip with List | `make Array list` / `convert List arr` |
| Use for | incrementally building structural data that flows through list words and unification | bulk numeric/indexed workloads, APL-style transforms |

## Record and Table mutability

`Record` and `Table` are Ideal type *descriptors*; `make R {…}` /
`make T [rows]` instances are plain immutable Map / List-of-Map shapes.
Flex nodes give a sanctioned mutable escape with an explicit validation
boundary:

```
def r (make R {a:1})      # validated, immutable
def f (flex r)            # mutable copy — NOT validated while flex
set a/q 2 f               # unchecked mutation
make R (node f)           # back through validation
```

A future option is dedicated FlexRecord/FlexTable kinds with validated
in-place mutation; the flex/node round-trip covers the need without new
machinery for now.

## Known limitations (v1, by design unless noted)

- Non-mutating words (`sort`, `filter`, `reverse`, `each`, unify
  outputs, struct-module words, `clone`, `jsonify`) return **plain**
  List/Map from flex input.
- A flex map does not satisfy a concrete-map *value pattern* in fn
  dispatch (engine map-pattern params use exact `TMap`); candidate
  follow-up if it bites.
- Typed-list/map child constraints are dropped by `flex` (no typed
  flex nodes).
- No map-key deletion word yet (none exists for any container in core).
- Flex nodes are runtime-only values: the parser never produces them,
  `Eval` is never set on them, and `resolveAtomReferents` (program
  tapes) deliberately ignores `FlexListData`.

## Verification

`lang/spec/flex.tsv` (11 sections, positive+negative paired: aliasing,
no-leak both directions, bounds, plain-immutability regression rows,
equality/ordering, nominal `is`, inherited words);
`lang/go/native/flex_test.go` (container identity across bindings and
returns, growth visibility through Value copies, `node` identity on
plain, no-panic battery for type literals, bounds-hint);
FixedID snapshot entries in `lang/go/test/fixedid_stability_test.go`.
