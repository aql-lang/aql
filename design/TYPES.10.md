# AQL Type Hierarchy

Every AQL value carries a hierarchical type path (e.g.
`Scalar/String/ProperString`). A child type matches its parent:
`Scalar/String/ProperString` matches `Scalar/String` matches
`Scalar`. A parent does NOT match a child. The lattice is **single-
rooted at `Any`** for the main hierarchy; `None` and `Never` are
degenerate roots kept apart so per-value dispatch shortcuts don't
silently match them.

For the comparison total order (`cmp` / `sort` / `lt` / `gt`) and
the per-family `Comparer` cascade, see
[`TYPE-ORDERING.0.md`](TYPE-ORDERING.0.md).

## Type Tree

```
Any                                -- top — matches everything (lattice root)
None                               -- the unit type (its sole inhabitant: `none`)
Never                              -- empty / bottom

Any/Scalar                         -- (`Path()` renders just "Scalar")
  Atom                             -- bare unquoted word used as data (`foo/q`)
  Boolean                          -- true / false
  Number
    Integer                        -- int64; per-value subtypes (e.g. Integer/42)
    Decimal                        -- float64; per-value subtypes
  String
    EmptyString                    -- the empty string ""
    ProperString                   -- non-empty string
  Path                             -- filesystem-style path (parts + abs flag)
  Time                             -- (external, lang/native — Time family)
    Date · DateTime · Instant · TimeOfDay
    Duration
      CalDuration · ClkDuration
    Timezone

Any/Node                           -- (`Path()` renders just "Node")
  List                             -- ordered sequence of values
    Args                           -- argument list (internal — args stack frame)
  Map                              -- ordered key-value pairs
    Inspect                        -- inspection-result map (from `inspect`)

Any/Ideal                          -- structural & domain "kinds"
  Object                           -- typed instances (mutable)
    Resource
      Entity
  Array                            -- mutable ordered array
  Record                           -- typed field schema
  Options                          -- map with defaults/constraints
  Error                            -- error value
  Store                            -- mutable key-value store w/ prototype chain
    System                         -- system configuration store
  Table                            -- list of typed records (SQL-backed)
  Fetch                            -- (external — HTTP fetch family)
    Request · Response
  Timeout · Interval               -- (external — timer handles)
  Tensor                           -- (external — matrix module)
    Matrix · Vector
  [User-defined]                   -- created via `def Foo refine Object {…}`

Any/Word                           -- bare-word values + internal runtime markers
  __FW (Forward)                   -- forward arg-collection marker
  __OP (Paren) / __CP / __ED       -- open/close paren, end
  __PE                             -- ParenExpr in data context
  __IS                             -- interp-string segment
  __FN (Fndef) / __RC / __MK / __MV / __MD / __IN

Any/Type                           -- the "type of types" branch
  Function                         -- callable function reference
  FunctionSignature                -- function-shape value
  Disjunct                         -- tor-union (`Integer tor String`)
    Enum                           -- enumerated atoms (`enum [red green blue]`)
```

## Lattice principles

* **`Any` is the structural root** of the main hierarchy. Every type
  except `None` and `Never` chains to `Any` via its `Parent`
  pointer.
* **Path rendering skips `Any`.** `Scalar.Path()` is `"Scalar"`, not
  `"Any/Scalar"`. Same for the other branch roots. FixedIDs and
  serialised IDs stay the historical short form.
* **`None` and `Never` are degenerate roots** (Rank `12·10⁹` and
  `13·10⁹`). They have no Comparer-bearing ancestor, so
  `CompareValues` settles them via the Rank cascade (`none cmp 5 →
  -1`, `none cmp Any → 1`).
* **A literal IS a type.** `42`, `'hello'`, `true` each have their
  own lattice node (the `Integer/42`, `ProperString/hello`,
  `Boolean/true` paths). `typeof 42 = Integer` is one Parent hop
  up.
* **User-defined types use a per-branch external Rank band.** Kernel
  positional ranks step `n·10¹⁰` per branch; user/external types
  share a band one increment up (Scalar `2·10¹⁰`→`2.1·10¹⁰`, Ideal
  `4·10¹⁰`→`4.1·10¹⁰`, etc.) so they always sort after the kernel
  positional subtree in the same branch.

## Mutability

| Branch | Mutability |
|---|---|
| `Scalar` (numbers, strings, atoms, paths, time) | immutable |
| `Node` (List, Map) | immutable values |
| `Ideal/Object` and descendants | mutable instances |
| `Ideal/Store`, `Ideal/Array`, `Ideal/Table` | mutable containers |
| `Ideal/Record`, `Ideal/Options` | type-shape values (immutable bodies) |

## ID Prefixes

Every value carries a unique ID: `<prefix>_<12 hex chars>`. The
prefix is derived from the **topmost ancestor that is NOT `Any`** —
`IDPrefixForType` walks the parent chain to the branch root.

| Prefix | Branch |
|---|---|
| `S_` | Scalar (numbers, strings, booleans, atoms, paths, time) |
| `N_` | Node (lists, maps) |
| `W_` | Word (function references, internal markers) |
| `T_` | Type / Ideal / Any / None / Never |

## Type Matching

* `Any` matches everything (a deliberate fast-path in `Type.Matches`).
* A child matches a parent via the ancestor walk.
* A parent does NOT match a child.
* DepScalar values (`Integer gt 0`) inherit their base scalar as
  `Parent`, so `(Integer gt 0).Parent.Matches(Integer)` is true via
  the regular ancestor walk; no special override needed.
* Signature matching routes through `Type.Behavior.Match` so per-
  type custom matchers participate (predicate types, refinement
  types, plugin types).

## Short Name Expansion

Type names auto-expand via the kernel `Builtin` table:

| Short name | Full path |
|---|---|
| `String`, `EmptyString`, `ProperString` | `Scalar/String[/…]` |
| `Number`, `Integer`, `Decimal` | `Scalar/Number[/…]` |
| `Boolean`, `Atom`, `Path` | `Scalar/[…]` |
| `Date`, `DateTime`, `Instant`, `TimeOfDay`, `Duration`, `CalDuration`, `ClkDuration`, `Timezone` | `Scalar/Time/[…]` (external) |
| `List`, `Map` | `Node/[…]` |
| `Object`, `Resource`, `Entity`, `Array`, `Record`, `Options`, `Error`, `Store`, `Table` | `Ideal/[…]` |
| `Tensor`, `Matrix`, `Vector` | `Ideal/Tensor/[…]` (external) |
| `Timeout`, `Interval` | `Ideal/[…]` (external) |
| `Function`, `FunctionSignature`, `Disjunct`, `Enum` | `Type/[…]` |

## Comparison and Ordering

`cmp` / `lt` / `gt` / `sort` use a unified total order over the
lattice. The discriminator cascade is:

1. **LCA Comparer walk.** Per-family `Comparer` capabilities live
   on `TNumber`, `TString`, `TBoolean`, `TAtom`, `TWord`, `TScalar`.
   Same-family pairs settle here.
2. **Lattice Rank fallback.** Cross-family pairs (no Comparer
   applies) settle by Rank → depth → name → lattice ID via
   `compareTypes`.
3. **Structural compare.** Two values of the same Parent: lists by
   length-then-element-wise, maps by length-then-sorted-keys-then-
   values, others by canonical-form lex.

**Type-literal-first rule.** Within a family, a bare type literal
sorts strictly below every concrete inhabitant — including the
family's zero-valued inhabitant. `Integer cmp 0 → -1`, `String cmp
'' → -1`, `Boolean cmp false → -1`, `Path cmp <any path> → -1`.

The result is a **strict total order over distinct lattice nodes**,
with one deliberate value-level equivalence: cross-leaf numeric
magnitude (`1 cmp 1.0 → 0`), preserved so traditional arithmetic
semantics survive the Integer/Decimal split.

Full design + verification at
[`TYPE-ORDERING.0.md`](TYPE-ORDERING.0.md). Test matrix at
`lang/spec/compare.tsv`.
