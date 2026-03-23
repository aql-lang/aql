
# AQL Arrayification Design

How array-language ideas from APL, J, R, Julia, and similar languages
can be applied to AQL — a concatenative, stack-based query language.


## Motivation

AQL already operates on lists and maps as first-class values. It has
`for` loops, `map`, `reduce`, and `filter` as listed operators, typed
lists `[:type]`, and typed maps `{:type}`. The stack machine naturally
composes operations.

What AQL does not yet have is a systematic treatment of arrays as the
default unit of computation. Array languages differ from traditional
languages not by having more library functions, but by making
whole-array operations primitive, uniform across dimensions, and
composable without explicit loops.

This document explores how to bring those ideas into AQL while
respecting its concatenative nature.


## Design Principles

1. **Words, not methods.** Every array operation is a word with suffix
   precedence, composable on the stack. No dot-method syntax.

2. **Lists are arrays.** AQL lists `[1,2,3]` are the array primitive.
   Nested lists `[[1,2],[3,4]]` represent higher-rank arrays. Shape
   is inferred from structure.

3. **Implicit iteration by default.** Scalar words should lift over
   arrays automatically when a signature does not match but an
   element-wise application would. This is broadcasting.

4. **Shape awareness.** New words should respect and transform shape
   rather than requiring manual indexing.

5. **Concatenative composition.** Pipelines of array transforms should
   read left to right on the stack, as with all AQL code.


## New Data Concepts

### Shape and Rank

Every list value acquires two derived properties:

- **rank**: nesting depth. A scalar has rank 0, a flat list rank 1,
  a list of equal-length lists rank 2, and so on.
- **shape**: list of dimension sizes. `[[1,2,3],[4,5,6]]` has shape
  `[2,3]`.

These are not stored separately — they are computed from the list
structure. Two new words expose them:

```
shape [1,2,3]                 => [3]
shape [[1,2],[3,4],[5,6]]     => [3,2]
rank  [1,2,3]                 => 1
rank  [[1,2],[3,4]]           => 2
```

Ragged lists (sublists of unequal length) have shape `none` beyond
the ragged dimension.


### Typed Arrays

Existing typed list syntax `[:number]` already constrains elements.
This extends naturally:

```
[:[:number]]                  # rank-2 number array
```

A future `array` type constructor could enforce rectangular shape:

```
typedef matrix array [:[:number]]
```


## New Words — Structural Primitives

### reshape

Rearrange elements into a new shape. Total element count must match.

```
reshape [2,3] [1,2,3,4,5,6]      => [[1,2,3],[4,5,6]]
reshape [3,2] [1,2,3,4,5,6]      => [[1,2],[3,4],[5,6]]
reshape [6] [[1,2,3],[4,5,6]]    => [1,2,3,4,5,6]
```

Signature: `[list, list] -> [list]` — first arg is shape, second is data.

### flatten

Collapse all nesting to a single flat list. Equivalent to
`reshape [n]` where n is total element count.

```
flatten [[1,2],[3,4]]             => [1,2,3,4]
flatten [[[1],[2]],[[3],[4]]]     => [1,2,3,4]
```

Signature: `[list] -> [list]`

### transpose

Swap the two outermost dimensions.

```
transpose [[1,2,3],[4,5,6]]      => [[1,4],[2,5],[3,6]]
```

Signature: `[list] -> [list]`

### take / drop

Select or remove leading/trailing elements along the outermost
dimension. Negative counts operate from the end.

```
take 2 [10,20,30,40]             => [10,20]
take -2 [10,20,30,40]            => [30,40]
drop 1 [10,20,30,40]             => [20,30,40]
drop -1 [10,20,30,40]            => [10,20,30]
```

Signatures: `[integer, list] -> [list]`

### reverse

Reverse along the outermost dimension.

```
reverse [1,2,3]                   => [3,2,1]
reverse [[1,2],[3,4]]             => [[3,4],[1,2]]
```

### length

Number of elements along the outermost dimension.

```
length [10,20,30]                 => 3
length [[1,2],[3,4],[5,6]]        => 3
```

### iota

Generate a list of integers from 0 to n-1. The fundamental array
constructor.

```
iota 5                            => [0,1,2,3,4]
iota 0                            => []
```

This replaces ad hoc range generation and pairs with `reshape` to
build structured arrays:

```
reshape [3,3] iota 9              => [[0,1,2],[3,4,5],[6,7,8]]
```

### replicate

Repeat elements according to a count vector. Count and data must
have the same length.

```
replicate [2,0,3] [10,20,30]     => [10,10,30,30,30]
```

Boolean mask replication (compress):

```
replicate [1,0,1,0,1] [10,20,30,40,50]  => [10,30,50]
```

### expand

Inverse of compress. Place elements into positions marked true,
filling false positions with a default.

```
expand [1,0,1,0,1] [10,30,50]    => [10,0,30,0,50]
```


## New Words — Iteration and Reduction

### each

Apply a word or quoted list to each element. This is the general
map operation, named `each` to align with array-language convention
and avoid collision with the existing `map` word used for key-value
maps.

```
each [mul 2] [1,2,3]             => [2,4,6]
each [upper] ["a","b","c"]       => ["A","B","C"]
```

Signature: `[list, list] -> [list]` — first arg is body, second is data.

With rank control (see `eachrank` below), `each` can apply to rows,
columns, or any cell structure.

### eachrank

Apply a function at a specific rank within a nested array. Rank 0
means each scalar, rank 1 means each row (innermost list), rank 2
means each matrix, and so on.

```
eachrank 1 [add 10] [[1,2],[3,4]]
# applies [add 10] to each rank-1 cell: each row broadcasts
=> [[11,12],[13,14]]

eachrank 0 [mul 2] [[1,2],[3,4]]
# applies [mul 2] to each scalar
=> [[2,4],[6,8]]
```

Signature: `[integer, list, list] -> [list]`

This is the AQL equivalent of J's rank operator or APL's rank
conjunction. It generalizes "map over rows" and "map over columns"
to arbitrary nesting depths.

### fold

Generalized reduction. Collapse a list using a binary operation.

```
fold [add] [1,2,3,4]             => 10
fold [mul] [1,2,3,4]             => 24
fold [max] [3,1,4,1,5]           => 5
```

With an initial value:

```
fold [add] 0 [1,2,3]             => 6
fold [concat] "" ["a","b","c"]   => "abc"
```

Signature: `[list, list] -> [any]` or `[list, any, list] -> [any]`

### foldaxis

Reduce along a specific axis of a nested array.

```
foldaxis 0 [add] [[1,2],[3,4]]   => [4,6]    # sum columns
foldaxis 1 [add] [[1,2],[3,4]]   => [3,7]    # sum rows
```

Signature: `[integer, list, list] -> [list]`

### scan

Running (prefix) reduction. Produces all intermediate results.

```
scan [add] [1,2,3,4]             => [1,3,6,10]
scan [mul] [1,2,3,4]             => [1,2,6,24]
scan [max] [3,1,4,1,5]           => [3,3,4,4,5]
```

Signature: `[list, list] -> [list]`

Useful for cumulative sums, running maxima, state propagation, and
dynamic programming patterns.


## New Words — Pairing Operations

### outer

Outer product. Apply an operation to every pair drawn from two arrays.

```
outer [mul] [1,2,3] [1,2,3,4]
=> [[1,2,3,4],[2,4,6,8],[3,6,9,12]]

outer [add] [10,20] [1,2,3]
=> [[11,12,13],[21,22,23]]

outer [eq] ["a","b"] ["b","c","a"]
=> [[false,false,true],[true,false,false]]
```

Signature: `[list, list, list] -> [list]` — operation, left array,
right array.

This replaces nested loops for generating multiplication tables,
distance matrices, cross-comparisons, and relational joins.

### inner

Generalized inner product. Combine pairs with one operation, then
aggregate with another. Ordinary matrix multiplication is
`inner [mul] [add]`.

```
inner [mul] [add] [[1,2],[3,4]] [[5,6],[7,8]]
=> [[19,22],[43,50]]

inner [eq] [or] [1,2,3] [3,2,1]
=> true    # do any positions match?

inner [min] [max] [3,1,4] [1,5,2]
=> 2       # max of pairwise mins
```

Signature: `[list, list, list, list] -> [list]` — pair-op,
aggregate-op, left, right.

This is a powerful generalization: standard matrix multiply, boolean
matrix product, tropical semiring operations, and fuzzy matching are
all instances.


## New Words — Selection and Membership

### where

Return indices where a boolean list is true. The inverse of
mask-based selection — gives positions rather than values.

```
where [true,false,true,false,true]   => [0,2,4]
```

Pairs naturally with comparison:

```
where each [gt 3] [1,5,2,4,3]       => [1,3]
```

### compress

Select elements where a mask is true. Equivalent to
`replicate mask data` when mask is boolean, but reads more clearly
for the filtering case.

```
compress [true,false,true] [10,20,30]   => [10,30]
```

### member

Test membership of each left element in the right array.

```
member [1,2,3] [2,4,6]               => [false,true,false]
```

### indexof

Find position of each left element in the right array. Returns
length of right array for elements not found.

```
indexof [3,1,4] [1,2,3,4,5]          => [2,0,3]
```

This overloads the existing string `indexof` — the type system
dispatches correctly because the signatures differ (list vs string).

### unique

Remove duplicate elements, preserving first occurrence order.

```
unique [3,1,4,1,5,9,2,6,5]           => [3,1,4,5,9,2,6]
```

### group

Partition elements by key, producing a map of lists.

```
group ["a","b","a","b","a"] [1,2,3,4,5]
=> {a:[1,3,5],b:[2,4]}
```

Signature: `[list, list] -> [map]` — keys, values.

When given a single list, groups by value:

```
group [3,1,4,1,5,9]
=> {3:[0],1:[1,3],4:[2],5:[4],9:[5]}
```

Returns a map from value to list of indices.


## New Words — Ordering

### grade

Return the permutation that would sort the array. Does not sort the
data — returns indices.

```
grade [30,10,40,20]                   => [1,3,0,2]
```

The result is a permutation vector. Apply it to reorder:

```
at grade [30,10,40,20] [30,10,40,20]  => [10,20,30,40]
```

Why grade instead of sort? Because grade lets you sort multiple
arrays in parallel by the same key, rank items, and compose
permutations.

### at

Select elements by index list (gather).

```
at [2,0,1] ["a","b","c"]             => ["c","a","b"]
at [0,0,1,1] [10,20]                 => [10,10,20,20]
```

### sortby

Convenience: sort one array by another. Equivalent to
`at grade keys data`.

```
sortby [3,1,2] ["c","a","b"]         => ["a","b","c"]
```


## New Words — Windows and Neighborhoods

### window

Sliding window of size n over a list.

```
window 2 [1,2,3,4]                   => [[1,2],[2,3],[3,4]]
window 3 [1,2,3,4,5]                 => [[1,2,3],[2,3,4],[3,4,5]]
```

Signature: `[integer, list] -> [list]`

Composes with `each` and `fold` for moving averages, local pattern
detection, convolution-like operations:

```
each [fold [add]] window 3 [1,2,3,4,5]    => [6,9,12]
# moving sum with window size 3
```

### pairs

Adjacent pairs. Shorthand for `window 2`.

```
pairs [1,2,3,4]                       => [[1,2],[2,3],[3,4]]
```


## Broadcasting

### Scalar Extension

When a scalar word like `add` receives a list where it expects a
scalar, it applies element-wise:

```
add 10 [1,2,3]                        => [11,12,13]
mul [1,2,3] [10,20,30]                => [10,40,90]
```

This is broadcasting at rank 0: scalar operations lift automatically
over list structure.

### Rules

1. **Scalar + list**: apply scalar op to each element.
2. **List + list (same length)**: apply element-wise (zip-with).
3. **List + list (different length)**: error. No implicit padding.
4. **Nested + flat**: align outermost dimension, broadcast inward.

Broadcasting is not magic — it follows the same rules as `each` but
is applied implicitly when the type signature demands a scalar and
receives an array. The type system already handles dispatch; this
extends it with an automatic lifting rule.


## Integration with Existing AQL

### Relationship to `for`

`for` remains the explicit loop for side effects and imperative
iteration. Array words handle the declarative, functional case.
Most code that currently uses `for` to build lists can be replaced
with `each`, `fold`, or `scan`.

### Relationship to `map`, `reduce`, `filter`

The existing `map`, `reduce`, `filter` words (listed in SAMPLES.md)
are the precursors:

| Existing   | Array word   | Difference                          |
|------------|-------------|--------------------------------------|
| map        | each        | `each` also supports rank control    |
| reduce     | fold        | `fold` generalizes across axes       |
| filter     | compress    | `compress` uses a precomputed mask   |

The existing words remain as aliases or for map-specific behavior
(operating on map values). The array words are the generalized forms.

### Relationship to Stack Operations

Array words consume and produce stack values like any other word.
They compose naturally:

```
iota 10                     # [0,1,2,...,9]
each [mul 2]                # [0,2,4,...,18]
fold [add]                  # 90
```

The stack-based pipeline replaces explicit loop accumulators.

### Relationship to `def` and `fn`

Users define custom array operations using existing definition words:

```
def movingavg [
  [integer, list] [list]
  [var [[n xs]
    each [fold [add] div n] window n xs
  ]]
]
```

Tacit (point-free) composition is a future consideration. The
concatenative model already supports implicit pipelining.


## Type System Implications

### New Types

```
Scalar/Number/Integer
Scalar/Number/Decimal
Scalar/String/Proper
Node/List                   # existing
Node/List/Array             # rectangular list (all sublists same length)
Node/Map                    # existing
```

`Array` is a subtype of `List` that guarantees rectangular shape.
Functions like `transpose`, `reshape`, and `inner` require `Array`
and produce `Array`.

### Type Signatures for Array Words

```
each:      [list, list] -> [list]
fold:      [list, list] -> [any]
scan:      [list, list] -> [list]
outer:     [list, list, list] -> [list]
inner:     [list, list, list, list] -> [list]
reshape:   [list, list] -> [list]
transpose: [list] -> [list]
take:      [integer, list] -> [list]
drop:      [integer, list] -> [list]
shape:     [list] -> [list]
rank:      [list] -> [integer]
iota:      [integer] -> [list]
window:    [integer, list] -> [list]
grade:     [list] -> [list]
at:        [list, list] -> [list]
where:     [list] -> [list]
compress:  [list, list] -> [list]
unique:    [list] -> [list]
member:    [list, list] -> [list]
group:     [list, list] -> [map]
flatten:   [list] -> [list]
reverse:   [list] -> [list]
length:    [list] -> [integer]
replicate: [list, list] -> [list]
expand:    [list, list] -> [list]
```

All use suffix precedence, consistent with AQL convention.


## Implementation Priority

### Phase 1 — Foundation

Core structural words that everything else builds on:

- `iota` — array construction
- `shape`, `rank`, `length` — inspection
- `reshape`, `flatten`, `transpose` — structure transforms
- `take`, `drop`, `reverse` — basic selection

### Phase 2 — Iteration

The generalized higher-order array operations:

- `each` — element-wise application
- `fold` — reduction
- `scan` — prefix reduction
- `compress`, `where` — mask-based selection

### Phase 3 — Pairing

Cross-array operations:

- `outer` — outer product
- `inner` — generalized inner product
- `window`, `pairs` — neighborhood operations

### Phase 4 — Selection and Ordering

Structural algebra:

- `grade`, `at`, `sortby` — permutation-based ordering
- `member`, `indexof` — membership testing
- `unique`, `group` — structural partitioning
- `replicate`, `expand` — count-based reshaping

### Phase 5 — Broadcasting

Automatic scalar extension:

- Implicit lifting of scalar words over lists
- Element-wise zip for matching-length lists
- Rank-controlled application with `eachrank`
- `foldaxis` for axis-specific reduction


## Examples

### Moving Average

```
def movingavg [
  [integer, list] [list]
  [var [[n xs]
    each [fold [add] div n] window n xs
  ]]
]

movingavg 3 [1,2,3,4,5,6]            => [2,3,4,5]
```

### Matrix Multiply

```
inner [mul] [add] [[1,0],[0,1]] [[5,6],[7,8]]
=> [[5,6],[7,8]]
```

### Histogram

```
group [3,1,4,1,5,9,2,6,5,3,5]
# => {3:[0,9],1:[1,3],4:[2],5:[4,8,10],9:[5],2:[6],6:[7]}

each [length] group [3,1,4,1,5,9,2,6,5,3,5]
# counts per value
```

### Coordinate Pairs

```
outer [concat " "] ["a","b","c"] ["1","2"]
=> [["a 1","a 2"],["b 1","b 2"],["c 1","c 2"]]
```

### Running Maximum

```
scan [max] [3,1,4,1,5,9,2,6]         => [3,3,4,4,5,9,9,9]
```

### Sort by Length

```
sortby each [length] ["cat","a","dogs","be"]
=> ["a","be","cat","dogs"]
```

### Boolean Selection

```
var [[xs] [3,1,4,1,5,9,2,6]]
compress each [gt 3] xs xs
=> [4,5,9,6]
```

### Identity Matrix

```
reshape [3,3] outer [eq] iota 3 iota 3
# outer [eq] [0,1,2] [0,1,2]
# => [[true,false,false],[false,true,false],[false,false,true]]
# with numeric conversion: [[1,0,0],[0,1,0],[0,0,1]]
```


## Contrast: Traditional vs Array Style in AQL

### Sum of squares (traditional)

```
var [[total] 0]
for 5 [def total [add total mul i i]]
total
```

### Sum of squares (array style)

```
fold [add] each [dup mul] iota 5
```

### Filter and transform (traditional)

```
# manual loop building a result list
```

### Filter and transform (array style)

```
var [[xs] [3,1,4,1,5,9,2,6]]
each [mul 10] compress each [gt 3] xs xs
=> [40,50,90,60]
```

The array style eliminates temporary variables, explicit
accumulators, and loop counters. The concatenative pipeline reads as
a sequence of transforms: generate, select, apply, aggregate.


## Summary

The core insight from array languages is not any single operation but
the discipline of expressing computation as shape-aware transforms
composed without explicit loops. AQL's concatenative model is
naturally suited to this: the stack is already a pipeline, words
already compose, and the type system already dispatches by structure.

Arrayification adds:
- **Shape vocabulary**: `shape`, `rank`, `reshape`, `transpose`, `flatten`
- **Generalized iteration**: `each`, `eachrank`, `fold`, `foldaxis`, `scan`
- **Cross-array pairing**: `outer`, `inner`
- **Structural selection**: `compress`, `where`, `replicate`, `expand`
- **Ordering algebra**: `grade`, `at`, `sortby`
- **Membership and grouping**: `member`, `indexof`, `unique`, `group`
- **Neighborhoods**: `window`, `pairs`
- **Construction**: `iota`
- **Broadcasting**: implicit scalar extension over arrays

Together these give AQL a systematic, compositional approach to array
programming while staying true to its concatenative stack-machine
identity.
