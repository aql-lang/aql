# Type Operations

Proposal for filling gaps in AQL's type-operation vocabulary, drawing
on TypeScript utility types, Python `typing`, Haskell type classes,
and SQL's type/cast surface.

## Naming convention

All NEW type-level operations are prefixed with `t`. This separates
them from the value-level surface and makes the type-language
discoverable. The existing `tor` / `tand` / `tany` / `tall` already
follow this rule; the older `typeof` / `pathof` / `refine` / `is` /
`guard` / `base` / `enum` / `convert` are kept as-is for backward
compatibility (they're the well-known surface that predates the
convention).

Naming pattern for new ops: `t<verb>` where `<verb>` says what the
operation produces or asks. `teq` (type equality), `tpartial` (make
fields optional), `texclude` (remove an alternative), `tpick` (select
fields), and so on.

## Current type operations

| Word | Signature | Notes |
|---|---|---|
| `refine` | `Any Node -> Type`, `Any -> Type` | uniform type constructor |
| `pathof` | `Any -> [:Type]` | ancestry path, root first |
| `typeof` | `Any -> Type` | single Parent hop |
| `enum` | `List -> Enum` | fixed enumeration |
| `is` | `Any Any -> Boolean` | subtype / membership test |
| `guard` | `Any Boolean -> Any` | value-or-None on predicate |
| `base` | `Any -> Any` | zero/default for a type |
| `convert` | `Scalar Scalar -> Scalar` (2-arg)<br/>`Scalar Map Scalar -> Scalar` (3-arg) | scalar coercion |
| `tor` | `Any Any -> Any` | type-level disjunct union |
| `tand` | `Any Any -> Any` | type-level intersection |
| `tany` | `List -> Any` | list-reduction `tor` |
| `tall` | `List -> Any` | list-reduction `tand` |

## Proposed additions

### Type-set algebra

| Word | Signature | Semantics | Inspiration |
|---|---|---|---|
| `teq` | `Any Any -> Boolean` | strict type equality (lattice node identity, NOT subtype). `Integer teq Integer -> true`, `Integer teq Number -> false`, `Integer is Number -> true` | Haskell `type Eq`, Python `type(a) is type(b)` |
| `texclude` | `Any Any -> Any` | set difference: remove alternatives from a disjunct. `(String tor None) texclude None -> String` | TypeScript `Exclude<T,U>` |
| `textract` | `Any Any -> Any` | retain only the alternatives that intersect. `(String tor Number tor Boolean) textract Number -> Number` | TypeScript `Extract<T,U>` |

### Record / Object surgery

| Word | Signature | Semantics | Inspiration |
|---|---|---|---|
| `tpartial` | `Any -> Type` | wrap every field of a Record / Object in `T \| None` (idempotent — fields already containing None are unchanged) | TypeScript `Partial<T>` |
| `tpick` | `Any List -> Type` | retain only the named fields. `tpick Person [name age]` -> a record with just those two fields | TypeScript `Pick<T,K>` |
| `tomit` | `Any List -> Type` | drop the named fields. `tomit Person [secret]` | TypeScript `Omit<T,K>` |
| `tmerge` | `Any Any -> Type` | combine two record/object types by field-union; overlap unifies via `tand` rules | TypeScript intersection of records |
| `trequired` | `Any -> Type` | strip `\| None` from every field. Inverse of `tpartial` | TypeScript `Required<T>` |

### Function-type introspection

| Word | Signature | Semantics | Inspiration |
|---|---|---|---|
| `tparamsof` | `Any -> [:Type]` | the parameter types of a Function value or FunctionSignature | TypeScript `Parameters<T>` |
| `treturnsof` | `Any -> Any` | the declared return type | TypeScript `ReturnType<T>` |
| `tarityof` | `Any -> Integer` | number of required (non-optional) parameters | — |

### Hierarchy navigation

| Word | Signature | Semantics | Inspiration |
|---|---|---|---|
| `tparent` | `Any -> Type` | direct lattice parent. `tparent Integer -> Number` | Python `type.__bases__`, Ruby `Class#superclass` |
| `troot` | `Any -> Type` | top of the branch (Scalar / Node / Type / Ideal / Any / None / Never) | — |
| `tlca` | `Any Any -> Type` | least common ancestor of two types | LCA in compiler type inference |

### Disjunct introspection

| Word | Signature | Semantics | Inspiration |
|---|---|---|---|
| `talts` | `Any -> [:Type]` | the alternatives of a Disjunct or Enum, as a typed list | Haskell `Data.Generic`'s constructors |

### Refinement primitives

| Word | Signature | Semantics | Inspiration |
|---|---|---|---|
| `tnominal` | `Any -> Type` | discoverable alias for the 1-arg `refine BaseType` form, when nominal newtype is the explicit intent | Haskell `newtype`, Rust `struct Wrapper(T)` |
| `tbrand` | `Any Atom -> Type` | attach a phantom tag for static disambiguation. `tbrand Integer userid/q` and `tbrand Integer orderid/q` are distinct types from the same base | TypeScript brand types |

## Implementation status

| Word | Status |
|---|---|
| `teq` | implemented (this proposal) |
| `tpartial` | implemented (this proposal) — Record + Object |
| `texclude` | proposed |
| `textract` | proposed |
| `tpick` | proposed |
| `tomit` | proposed |
| `tmerge` | proposed |
| `trequired` | proposed |
| `tparamsof` | proposed |
| `treturnsof` | proposed |
| `tarityof` | proposed |
| `tparent` | proposed |
| `troot` | proposed |
| `tlca` | proposed |
| `talts` | proposed |
| `tnominal` | proposed |
| `tbrand` | proposed |

## Notes on `teq` vs `is`

`is` and `teq` answer different questions:

- `Integer is Number` -> **true** (Integer is a subtype of Number)
- `Integer teq Number` -> **false** (they are distinct lattice nodes)
- `5 is Integer` -> **true** (5 is a value of type Integer)
- `5 teq Integer` -> **false** (5 is not a type at all)
- `Integer teq Integer` -> **true** (same lattice node)

`teq` requires both sides to be type bodies (`IsTypeBody` true).
Bare type literals compare by `*Type.Equal` (ID identity); structural
type bodies (record types, disjuncts, etc.) compare via `ValuesEqual`.

## Notes on `tpartial` semantics

`tpartial` is idempotent: a field whose value type already includes
`None` is left unchanged. So `tpartial (tpartial T) teq tpartial T`
holds.

For Record types, the result is another Record type with the same
field order and each value type wrapped as `T | None`.

For Object types, all fields (including inherited) are flattened
into the result's own field map, and the result is registered as a
fresh anonymous Object type (lattice parent: `Object`). This means
`tpartial Person` is NOT a subtype of `Person` — the lattice
relationship is intentionally broken because every Person is a valid
PartialPerson but not vice versa, and AQL's lattice runs the other
way (a child requires more, not less).

Future work: support `tpartial` on typed-map shapes (`{:T}`) and on
disjunct types where the partial-ization should distribute over each
alternative.
