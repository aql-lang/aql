# TYPE-ORDERING.0 — The Value Lattice & Comparison Total Order

This document records the design of AQL's value ordering: the lattice
that places every Value in a total preorder, the cascade
`CompareValues` uses to settle a pair, and the deliberate anomalies
we accepted. It is the canonical reference for `cmp` / `lt` / `gt` /
`lte` / `gte` / `sort` and the `Comparer` capability seam.

## TL;DR

* Every Value has a `Parent` pointing at its lattice node.
  `typeof v == v.Parent`. The lattice has `Any` at the root of the
  main hierarchy; `None` and `Never` are sui-generis roots kept apart
  so the `Parent.Equal(TNone)` shortcut in the dispatch path doesn't
  silently match every value.
* Every type carries a unified `Rank` integer. Kernel types get a
  positional slot in their branch's `n·10¹⁰` band; user-defined
  (`MintType`) and external (`RegisterExternalBuiltin`) types share a
  per-branch external band one increment up (Scalar
  `2·10¹⁰`→`2.1·10¹⁰`, Node `3·10¹⁰`→`3.1·10¹⁰`, Ideal
  `4·10¹⁰`→`4.1·10¹⁰`, Word `5·10¹⁰`→`5.1·10¹⁰`, Type
  `6·10¹⁰`→`6.1·10¹⁰`).
* `CompareValues` first walks the LCA chain looking for a `Comparer`
  capability. If one is found (Number, String, Boolean, Atom, Word,
  Scalar) it owns the result — same-family pairs order by content.
  Only when no Comparer applies does the Rank cascade run.
* `compareTypes` (the Rank fallback) cascades Rank → depth → name → ID.
* Same-Parent concrete values are settled by `compareStructural`:
  Lists by length-then-element-wise, Maps by length-then-keyset-lex-
  then-value-wise.
* **Order is a total preorder, not a strict total order.** Three
  documented anomalies are accepted as deliberate trade-offs (see
  *Anomalies*); they all reduce to "type literals share an equivalence
  class with the family's zero-valued inhabitant."

## The Lattice — full table

`Rank` is the discriminator the comparator keys on for cross-family
pairs. The kernel positional bands step `n·10¹⁰` apart per branch;
external/user bands sit one increment up so they always rank after
the kernel positional subtree in the same branch.

```
RANK             TYPE PATH                  REPRESENTATIVE LITERAL          NOTES
─────────────────────────────────────────────────────────────────────────────────────────────
─── root band (1·10¹⁰) ──────────────────────────────────────────────────────────────────────
11_000_000_000   Any                        Any                              top — matches anything
12_000_000_000   None                       none      None                   the unit; type literal
13_000_000_000   Never                      Never                            empty / bottom

─── kernel Scalar band (2·10¹⁰) ─────────────────────────────────────────────────────────────
20_000_000_000   Scalar                     Scalar
20_100_000_000   Scalar/Atom                Atom      red/q                  atom literal via /q
20_200_000_000   Scalar/Boolean             Boolean   false   true           false < true
20_300_000_000   Scalar/Number              Number
20_310_000_000   Scalar/Number/Integer      Integer   -1  0  1  2  42        positional + per-value
20_320_000_000   Scalar/Number/Decimal      Decimal   0.0  3.14              per-value
20_400_000_000   Scalar/String              String
20_410_000_000   Scalar/String/EmptyString  EmptyString   ''                 sole inhabitant
20_420_000_000   Scalar/String/ProperString ProperString  'apple'  'banana'  lex
20_500_000_000   Scalar/Path                Path      a/b/c   /abs/path      length → reverse-lex

─── external/user Scalar band (2.1·10¹⁰) ────────────────────────────────────────────────────
21_000_000_000   Scalar/Time                (make Date '2026-05-23')         external — Time family
21_000_000_000   Scalar/Time/Date           (make Date …)
21_000_000_000   Scalar/Time/DateTime       (make DateTime …)
21_000_000_000   Scalar/Time/Instant        (make Instant …)
21_000_000_000   Scalar/Time/TimeOfDay      (make TimeOfDay …)
21_000_000_000   Scalar/Time/Duration       (make Duration 'PT1H')
21_000_000_000   Scalar/Time/Duration/CalDuration  (make CalDuration 'P1Y')
21_000_000_000   Scalar/Time/Duration/ClkDuration  (make ClkDuration 'PT1H')
21_000_000_000   Scalar/Time/Timezone       (make Timezone 'UTC')
21_000_000_000   Scalar/<user>              def Positive (refine Integer …)  ties: depth → lex name

─── kernel Node band (3·10¹⁰) ───────────────────────────────────────────────────────────────
30_000_000_000   Node                       Node
30_100_000_000   Node/List                  List      []   [1 2 3]           length → element-wise
30_110_000_000   Node/List/Args             (args stack frame)
30_200_000_000   Node/Map                   Map       {}   {a:1, b:2}        length → keys → values
30_210_000_000   Node/Map/Inspect           inspect add                       inspection-result map

─── external/user Node band (3.1·10¹⁰) ──────────────────────────────────────────────────────
31_000_000_000   Node/<user>                def Pair (refine List …)

─── kernel Ideal band (4·10¹⁰) ──────────────────────────────────────────────────────────────
40_000_000_000   Ideal                      Ideal
40_100_000_000   Ideal/Object               Object
40_110_000_000   Ideal/Object/Resource      Resource
40_111_000_000   Ideal/Object/Resource/Entity   make Entity {kind:'api' …}
40_200_000_000   Ideal/Array                make Array [1 2 3]
40_300_000_000   Ideal/Record               refine Record [x:Integer]
40_400_000_000   Ideal/Options              make Options {x:1, y?:String}
40_500_000_000   Ideal/Error                (raised error value)
40_600_000_000   Ideal/Store                (sys-store layer)
40_610_000_000   Ideal/Store/System         __sys
40_700_000_000   Ideal/Table                refine Table Foo

─── external/user Ideal band (4.1·10¹⁰) ─────────────────────────────────────────────────────
41_000_000_000   Ideal/Fetch                make FetchRequest {…}
41_000_000_000   Ideal/Fetch/Request
41_000_000_000   Ideal/Fetch/Response
41_000_000_000   Ideal/Interval             make Interval PT1S
41_000_000_000   Ideal/Tensor               make Tensor [[1 2][3 4]]
41_000_000_000   Ideal/Tensor/Matrix        refine Matrix {rows:2 cols:2}
41_000_000_000   Ideal/Tensor/Vector        refine Vector {n:3}
41_000_000_000   Ideal/Timeout              make Timeout PT5S
41_000_000_000   Ideal/<user>               def Person refine Record […]

─── kernel Word band (5·10¹⁰) ───────────────────────────────────────────────────────────────
50_000_000_000   Word                       add   foo                         bare-word values
50_100_000_000+  Word/__FW … Word/__IN/__DC  (internal runtime markers — forward, paren, end,
                                              fn, mark, move, module, def-cleanup)

─── external/user Word band (5.1·10¹⁰) ──────────────────────────────────────────────────────
51_000_000_000   Word/<user>                (no user path today; reserved)

─── kernel Type band (6·10¹⁰) ───────────────────────────────────────────────────────────────
60_000_000_000   Type                       Type
60_100_000_000   Type/Function              fn [Integer Integer [add 1]]
60_200_000_000   Type/FunctionSignature     fnsig [Integer Integer]
60_300_000_000   Type/Disjunct              Integer tor String tor None
60_310_000_000   Type/Disjunct/Enum         enum [red green blue]

─── external/user Type band (6.1·10¹⁰) ──────────────────────────────────────────────────────
61_000_000_000   Type/<user>                def Positive (Integer gt 0)      DepScalar values
```

## In AQL, literals are types

A concrete value's `Parent` IS its type — there is no separate "value
inhabits type" indirection. `42`, `'hello'`, `true` each have their
own lattice identity (the `Scalar/Number/Integer/42` /
`Scalar/String/ProperString/hello` / `Scalar/Boolean/true` paths).
The Rank is inherited from the leaf type, so values within a leaf
share a Rank and tie-break via the family `Comparer` on numeric /
lex content. `typeof 5 == Integer` is one parent hop up the chain;
`typeof Integer == Number` is one more.

## The `CompareValues` cascade

```
CompareValues(a, b):
  1. aType, bType = ValueType(a), ValueType(b)
       - Data == nil && !Carrier → &v (the value IS the lattice node)
       - otherwise              → v.Parent
  2. lca = lowestCommonAncestor(aType, bType)
  3. Walk lca up the parent chain looking for a Comparer:
        ┌─────────────────────┬───────────────────────────────────┐
        │ numberComparer      │ on TNumber → float64 magnitude    │
        │ stringComparer      │ on TString → UTF-8 lex            │
        │ booleanComparer     │ on TBoolean → false < true        │
        │ atomComparer        │ on TAtom → name lex               │
        │ wordComparer        │ on TWord → rendered-form lex      │
        │ scalarComparer      │ on TScalar → comparePaths fallback│
        └─────────────────────┴───────────────────────────────────┘
       Each returns ErrNoComparer when its payload-extraction
       doesn't apply (DepScalar in a numeric Comparer, etc.) so the
       walk can resume.
  4. No Comparer found → compareTypes(aType, bType)
       Rank → typeDepth → lex Name → lattice ID
  5. Types identical and concrete → compareStructural:
        List → compareListElems (length, then element-wise CompareValues)
        Map  → compareMapEntries (length, then sorted-keys lex,
                                  then per-key CompareValues)
        else → strings.Compare(CanonValue(a), CanonValue(b))
```

The Comparer cascade firing *before* the Rank cascade is what
preserves traditional numeric semantics across the Integer / Decimal
positional split: even though `Decimal` (Rank `20.32·10⁹`) ranks
above `Integer` (Rank `20.31·10⁹`), `1.1 lt 2` is `true` because the
LCA `Number` has the numeric `Comparer` and it owns the comparison —
`AsNumber(1.1) = 1.1`, `AsNumber(2) = 2.0`, `1.1 < 2.0` → `-1`.

## Node ordering

Two concrete nodes of the same Parent are settled by
`compareStructural`:

### Lists

```
[1 2 3] cmp [1 2 4]     → -1   (lengths 3 = 3; (3 cmp 4) = -1)
[1 'a'] cmp [1 'b']     → -1   (per-position Comparer)
[1.1 2] cmp [1 2.1]     →  1   ((1.1 cmp 1) =  1, stops the walk)
[1 2 3] cmp [1 2 3]     →  0   (all elements equal)
[1 2 3] cmp [1 2 4 5]   → -1   (length 3 < length 4 — element walk skipped)
[]      cmp [1]         → -1
[9]     cmp [1 2 99]    → -1   (length wins)
```

### Maps

```
{a:1 b:2} cmp {a:1 b:3}  → -1   (same length, sorted keys [a,b] match;
                                  (2 cmp 3) = -1 stops the walk)
{a:1 b:2} cmp {b:2 a:1}  →  0   (declaration order doesn't matter)
{a:1 b:2} cmp {a:1 c:2}  → -1   (same length, sorted keys differ:
                                  'b' < 'c')
{a:1 b:2} cmp {a:1}      →  1   (length 2 > length 1)
{}        cmp {a:1}      → -1
```

### Nested

`CompareValues` recurses, so `[[1] [2]] cmp [[1] [3]] = -1` resolves
to `CompareValues([1], [2])` at position 1 → `-1` via the list rule
→ `1 vs 2` via the Number Comparer.

## Cross-family ordering — Rank decides

When no Comparer applies (the LCA walk reaches Any without finding
one), `compareTypes` settles by Rank, then depth, then name, then
ID. This produces the macro ordering:

```
Booleans < Numbers < Strings < Paths < Times          (Scalar band)
                <
Lists < Maps                                          (Node band)
                <
Objects < Records < Options < Errors < Stores < Tables (Ideal band)
                <
Words                                                 (Word band)
                <
Functions < FunctionSignatures < Disjuncts < Enums    (Type band)
```

Example: `true cmp 5 = -1` (Boolean Rank `20.2·10⁹` < Integer Rank
`20.31·10⁹`); `5 cmp 'a' = -1` (Integer < String); `'a' cmp [1] =
-1` (String band < List band).

## Anomalies — type literal ≡ family zero

The Comparer-first design has a deliberate consequence: when a type
literal is compared *within its own family*, the family Comparer
reads the type literal as `Data == nil` and pulls a zero-valued
payload. This places the type literal in the same equivalence class
as the family's zero-valued inhabitant:

```
Integer cmp 0      → 0          # type literal Integer ≡ value 0
Number  cmp 0      → 0
Decimal cmp 0      → 0
Decimal cmp 0.0    → 0
Integer cmp Number → 0          # type literals also tie among themselves
Number  cmp Decimal → 0
Boolean cmp false  → 0          # Boolean ≡ false
String  cmp ''     → 0          # String ≡ ''
EmptyString cmp ProperString → 0
EmptyString cmp ''           → 0

1 cmp 1.0         → 0          # cross-leaf numeric magnitude
```

These collapse violates **antisymmetry**: `Integer cmp 0 = 0` and
`0 cmp Number = 0` and `Integer cmp Number = 0`, but the three values
are not identical lattice nodes. **They are equivalent under `cmp`,
not equal in identity.**

The order is therefore a **total preorder**, not a strict total
order. It is:

* Reflexive: `a cmp a = 0` for every value.
* Transitive: `a ≤ b ∧ b ≤ c ⇒ a ≤ c` (verified by the test suite).
* **Not antisymmetric**: distinct values can compare equal (the
  cases above).

### Why we accept the anomaly

Two alternative designs were considered and rejected:

1. **Disambiguate type literals from values in the comparer.** The
   numeric Comparer could check `IsTypeLiteral` and route type
   literals through Rank. This restores antisymmetry but breaks the
   single-source-of-truth property: the Comparer would have to know
   about lattice mechanics, and the consistent "values within a
   family use the family Comparer" rule would have an exception.
2. **Cross-leaf numeric distinction (`1 cmp 1.0 = -1`).** This
   would mean numeric equality across Integer/Decimal is no longer
   transitive with arithmetic (`1.0 + 0 == 1.0` but `1 cmp 1.0 ≠
   0`). Worse user-facing surprise than the current collapse.

The accepted trade-off: comparator stays simple, type literals
sort as their family zero, callers that need lattice identity use
`(&a).Equal(&b)` or `typeof`.

### `none` and `Never` are not affected

`none` and `Never` are kept as their own degenerate roots (Rank
`12·10⁹`, `13·10⁹`). They have no Comparer-bearing ancestor, so
cross-pair comparisons fall through to Rank and order cleanly:

```
none cmp 5     → -1     (None Rank 12·10⁹ < Integer band)
none cmp Any   →  1     (None Rank 12·10⁹ > Any Rank 11·10⁹)
Never cmp Any  →  1     (Never Rank 13·10⁹ > Any Rank 11·10⁹)
```

`none` and `none` compare equal; `none` and any other value compare
strictly by Rank.

## Implementation pointers

| Concern | File | Key symbol |
|---|---|---|
| Lattice declaration | `eng/go/typetable.go` | `builtinDecls`, `Rank` |
| External/user Rank band | `eng/go/typetable.go` | `externalBandFor` |
| Total-order tiebreak | `eng/go/compare_types.go` | `compareTypes`, `rankOf` |
| LCA + Comparer walk | `eng/go/compare.go` | `CompareValues`, `lowestCommonAncestor` |
| Per-family Comparers | `eng/go/compare_scalar_behaviors.go` | `numberCompareBehavior`, `stringCompareBehavior`, … |
| Structural compare | `eng/go/compare_types.go` | `compareStructural`, `compareListElems`, `compareMapEntries` |
| Spec coverage | `lang/spec/compare.tsv` | tests for every cell above |

## Verification

`lang/spec/compare.tsv` carries the test matrix: every scalar cross
product, the node ordering rules, the cross-numeric chain, the
anomalies, and an explicit transitivity battery. The spec runner
asserts each row; any drift fails CI.
