# Dependent Types for AQL

Design proposal for adding dependent types to AQL, inspired by the
ideas in [A Perfectable Programming Language](https://alok.github.io/lean-pages/perfectable-lean/).
That article argues Lean is uniquely "perfectable" because you can
write properties about Lean, in Lean, using dependent types. This
document explores what that idea looks like translated into a
concatenative, stack-based, dynamically-typed language.


## The Foundational Observation: Values Are Types

Most languages draw a hard line between values and types. Lean
bridges that line with dependent types. AQL has no line to bridge.

In AQL, every value already IS a type — the most specific type in the
hierarchy. `42` has type `Scalar/Number/Integer` (and the engine
already supports literal subtypes like `Integer/42`). A type literal
like `number` is a value that sits on the stack. `record` is an
ordinary word that takes a list and returns a type value. `type`
just binds a name to whatever value you hand it:

```
record [x:number y:number]       # word returns a type value
type Point record [x:number y:number]   # binds the name Point
[:number]                        # typed list — a type value
(number or string)               # disjunction — a type value
```

There is no "type level" vs "value level." The stack is the only
level. Type-level computation is just computation.

This means AQL does not need a special `tyfn` keyword or any
separate machinery for "type functions." A function that returns a
type is just a function:

```
# A function that takes a number and returns a type
def Vec fn [[n:integer] [type] [
  (list where [len n eq])
]]

# Use it — Vec is a normal word, called normally
type Vec3 (Vec 3)
[1,2,3] is Vec3              => true
[1,2] is Vec3                => false

# A function that takes two types and returns a type
def Pair fn [[a:type, b:type] [type] [
  record [fst:a snd:b]
]]

type IntStr (Pair integer string)
make IntStr [1 "hello"]      => {fst:1, snd:'hello'}
```

No new syntax. No new concept. `def`, `fn`, `type`, `record` — all
existing words. The "generic type constructor" is just a function
that happens to return a type. This is the consequence of having no
value/type boundary.


## What AQL Actually Needs

Given that type-returning functions already work, only three things
are genuinely missing to get dependent types:

1. **Refinement types** — narrowing a type with a predicate (`where`)
2. **Type variables** — binding a type during signature matching (`@`)
3. **Property testing** — checking properties about AQL in AQL (`check`)

Everything else — type functions, generic containers, dependent
return types — falls out of these three plus what already exists.


## 1. Refinement Types: `where`

A refinement type narrows an existing type with a predicate — a
quoted block that receives the value and must leave a boolean.

### Syntax

```
(base-type where [predicate])
```

The predicate block receives the candidate value on the stack and
must push `true` or `false`.

### Examples

```
# Positive integer
type Pos (integer where [gt 0])

# Non-empty string
type NonEmpty (string where [len gt 0])

# Percentage: number in [0, 100]
type Pct (number where [dup gte 0 swap lte 100 and])

# Fixed-length list
type Triple (list where [len eq 3])

# Sorted list (each element <= next)
type Sorted ([:number] where [
  var [[xs]
    xs len lt 2 if [true] [
      xs each-pair [lte] fold true [and]
    ]
  ]
])
```

### Why `where` Is the Only New Keyword Needed

Refinement is the one thing AQL's type system genuinely can't
express today. Disjunctions (`or`) combine types horizontally.
The hierarchical tree narrows types vertically. But neither can
say "an integer, but only if it's positive." That requires
attaching a predicate to a type — which is what `where` does.

Everything else people associate with dependent types is
expressible once `where` exists:

```
# "Vector of length n" — dependent type
def Vec fn [[n:integer] [type] [(list where [len n eq])]]

# "Number in range" — bounded type
def Range fn [[lo:number, hi:number] [type] [
  (number where [dup gte lo swap lte hi and])
]]
type Byte (Range 0 255)

# "Non-empty list of T" — constrained generic
def NonEmptyList fn [[t:type] [type] [
  ([:t] where [len gt 0])
]]
```

These are all normal functions returning refined types. No special
forms.

### Semantics

A refined type `(T where [P])` matches a value `v` when:
1. `v` matches base type `T` (standard hierarchical match)
2. Evaluating `v P` produces `true`

Step 2 runs the predicate in a sub-engine (like `do`), so it
cannot mutate the caller's state.

### Interaction with Existing Words

**`is`** checks refinement:
```
5 is Pos                 => true
-3 is Pos                => false
"hello" is Pos           => false
```

**`unify`** checks and narrows:
```
Pos unify 5              => 5 true
Pos unify -3             => '~unify-fail' false
Pos unify number         => Pos true        # type-vs-type: narrower wins
```

**Signature matching** evaluates predicates during dispatch:
```
def abs fn [
  [x:Pos] [Pos] [x]
  [x:(integer where [lt 0])] [Pos] [x negate]
]

5 abs                    => 5
-3 abs                   => 3
```

The engine tries signatures in order. When a signature slot has a
refined type, the predicate runs as part of matching. If it returns
false, that signature is skipped — exactly like a type mismatch today.

### Dependent Return Types

Because named parameters from `fn` input signatures are in scope
when output types are evaluated, dependent return types work
automatically:

```
def Vec fn [[n:integer] [type] [(list where [len n eq])]]

def replicate fn [
  [n:integer, x:any]
  [(Vec n)]                  # output type references input n
  [iota n each [drop x]]
]

replicate 3 "x"             => ["x","x","x"]
# return type contract: (Vec 3), i.e., list of length 3
```

The output type `(Vec n)` evaluates with `n` bound to the actual
argument. If the body violates the contract, an error value is
produced. This is a runtime postcondition, not a compile-time proof.

### Pre/Post Contracts

For explicit contract style, `where` on the output type covers
postconditions. For preconditions beyond what the input types
express, use a refined input type:

```
# Binary search requires sorted input
type Sorted ([:number] where [
  var [[xs]
    xs len lt 2 if [true] [
      xs each-pair [lte] fold true [and]
    ]
  ]
])

def bsearch fn [
  [xs:Sorted, target:number]
  [(integer where [dup gte 0 if [xs swap get target eq] [true]])]
  [...]
]
```

The precondition (sorted input) is the input type. The postcondition
(valid index pointing to target) is the output type. Both are just
types with `where` clauses. No `pre`/`post` keywords needed.


## 2. Type Variables: `@`

For generic functions that preserve type relationships across
arguments and return values, we need one mechanism that doesn't
fall out of existing features: type variables.

### The Problem

A normal function can take `any` and return `any`:

```
def id fn [[any] [any] []]
```

But this loses the constraint "output type = input type." If you
pass an integer, the return type should be integer, not any. The
existing type system can't express this — `any` matches everything
but remembers nothing.

### Syntax

`@name` in a signature position introduces a type variable. The
first occurrence binds it; subsequent occurrences must unify.

```
# Identity preserves type
def id fn [[@a] [@a] []]

# Swap is generic over both types
def swap2 fn [[@a, @b] [@b, @a] [swap]]

# Head extracts the element type from a typed list
def head fn [[xs:[:@a]] [@a] [xs . 0]]

# Two args must have the same type
def add-same fn [[x:@a, y:@a] [@a] [x add y]]
```

### Why This Can't Be Expressed With `where`

You might think: just use a `where` clause to check type equality.
But `where` constrains a single value. Type variables constrain
relationships *between* signature positions. "The return type is
the same as the input type" requires binding information from one
position and using it in another. That's what `@` does.

### Semantics

During signature matching, when the engine encounters `@a`:
1. If `@a` is unbound: bind it to the concrete type of the argument.
   Match succeeds (any value matches an unbound variable).
2. If `@a` is already bound: check that the argument's type unifies
   with the bound type. If yes, narrow the binding. If no, the
   signature fails.

Type variable bindings are scoped to a single signature match
attempt and discarded afterward.

### Interaction with Refinement

Type variables and `where` compose:

```
# Both args same type, first must be positive
def add-pos fn [
  [x:(@a where [gt 0]), y:@a]
  [@a]
  [x add y]
]
```

`@a` is bound by the first argument's base type, then the second
argument must match. The `where` clause further constrains the
first argument's value.

### Interaction with Type-Returning Functions

Type variables can appear inside computed types:

```
def PairOf fn [[t:type] [type] [record [fst:t snd:t]]]

def make-pair fn [
  [x:@a, y:@a]
  [(PairOf @a)]
  [make (PairOf @a) [x y]]
]
```

`@a` binds during input matching, then the return type expression
`(PairOf @a)` evaluates with the bound type substituted in.


## 3. Property Testing: `check`

The article's deepest claim is that Lean is "perfectable" because
you can reason about code in the same language. AQL can't prove
properties (that requires a fundamentally different execution
model). But it can *test* properties — and because types are values
and predicates are code, the same language expresses both the code
and the properties about the code.

### Syntax

```
check [property-block] n
```

### Examples

```
# Addition is commutative
check [
  var [[x (random integer) y (random integer)]
    x add y eq (y add x)
  ]
] 1000

# Map preserves length
check [
  var [[xs (random [:integer])]
    xs each [dup add] len eq (xs len)
  ]
] 500

# Replicate produces correct length
def Vec fn [[n:integer] [type] [(list where [len n eq])]]

check [
  var [[n (random (integer where [gte 0 lte 100]))]
    replicate n 0 is (Vec n)
  ]
] 200

# Reverse is its own inverse
check [
  var [[xs (random [:integer])]
    xs reverse reverse eq xs
  ]
] 500
```

### Semantics

`check` evaluates the property block `n` times. Each evaluation
must produce a boolean. If any returns false, `check` pushes an
error value containing the failing inputs. If all pass, it pushes
`true`.

The `random` word generates random values matching a type —
including refined types. For `(integer where [gt 0])`, it generates
random integers and filters until the predicate passes.

### Why `check` Is Sufficient

Lean's formal proofs give certainty: if it compiles, the property
holds for all inputs, forever. AQL's `check` gives confidence: if
1000 random tests pass, the property probably holds. The tradeoff:

- AQL programmers never need to learn proof tactics. Properties are
  just AQL code they already know how to write.
- The same `where` predicates used in types are reusable as test
  generators — `random Pos` generates positive integers because
  `Pos` already knows what "positive integer" means.
- Properties compose the same way functions compose — because they
  ARE functions.


## Generics: Already Solved

With just `where` and `@`, generics fall out of what AQL already has.
No additional features needed.

### Generic Type Constructors

These are just functions that return types:

```
def ListOf fn [[t:type] [type] [[:t]]]
def Pair fn [[a:type, b:type] [type] [record [fst:a snd:b]]]

type IntList (ListOf integer)
type IntPair (Pair integer integer)
```

Already works today (modulo `where` for constrained variants). The
function *is* the generic. Calling it with different arguments
produces different types.

### Generic Functions

Type variables provide parametric polymorphism:

```
def head fn [[xs:[:@a]] [@a] [xs . 0]]
def reverse fn [[xs:[:@a]] [:@a] [xs len iota each [xs len sub 1 sub] each [xs swap get]]]
def zip fn [[xs:[:@a], ys:[:@b]] [:(Pair @a @b)] [
  iota (xs len min (ys len)) each [
    var [[i] make (Pair @a @b) [(xs get i) (ys get i)]]
  ]
]]
```

### Bounded Generics

Combine `@` with `where`:

```
# Works for any numeric type
def sum fn [[xs:[:(@a where [is number])]] [@a] [xs fold 0 [add]]]
```

### Ad-Hoc Polymorphism (Already Exists)

AQL already has this via multiple `fn` signatures:

```
def show fn [
  [x:integer] [string] [make string x]
  [x:string] [string] [x]
  [x:list] [string] [x each [show] ", " join]
]
```

Parametric generics (`@`) complement this: use `@` when the same
code works for any type; use multiple signatures when different
types need different code.


## Implementation

### Phase 1: `where` (Refinement Types)

The only new syntax. Extends the parser and type system.

1. **Parser**: Recognize `where` inside parenthesized type
   expressions. The parser already handles `(type or type)`.
   Add `where` as a keyword that captures the next quoted block.
   Result: a refined type value carrying base type + predicate.

2. **Type representation**: Add a `Predicate []Value` field to
   the type representation (or wrap in a `RefinedType` struct).

3. **Signature matching**: After the base type matches, run the
   predicate in a sub-engine. False = match fails.

4. **`is`**: Base type check, then predicate.

5. **`unify`**: Base type unification first, then predicate if a
   concrete value is present.

### Phase 2: `@` (Type Variables)

New token in signatures. Changes to signature matching.

1. **Parser**: Recognize `@name` in signature positions. Store as
   a `TypeVar` value.

2. **Signature matching**: Maintain a `map[string]*VType` binding
   map per match attempt. First `@a` binds; later `@a` unifies.

3. **Return type evaluation**: Substitute bound type variables into
   output type expressions after matching.

### Phase 3: `check` and `random`

Two new native words. No parser changes.

1. **`random`**: Generate values for a type. Base types are
   straightforward. Refined types use generate-and-filter.
   Records generate per-field. Lists generate elements.

2. **`check`**: Loop, evaluate, collect. Report failures with
   the failing inputs for debugging.


## Design Tradeoffs

### Runtime, Not Compile-Time

All checking is runtime. Predicates execute when values are
matched. This fits AQL's dynamic nature:

- **Pro**: Types and code use the same language. No separate
  type-checking phase. Predicates can be arbitrarily expressive.
- **Pro**: Works with existing dynamic dispatch — refined types are
  just more specific dispatchers.
- **Con**: Errors caught at call time, not definition time.
- **Con**: Predicate evaluation has runtime cost.

### Predicate Termination

Predicates could loop. Sub-engines should enforce a timeout (reuse
`await` timeout mechanics). If a predicate times out, the match
fails with an error.

### Predicate Purity

Predicates run in sub-engines with a restricted context — no I/O
words (`write`, `fetch`, etc.). This is enforceable by filtering
the sub-engine's registry.


## Comparison: AQL vs. Lean

| Aspect | Lean | AQL (Proposed) |
|--------|------|----------------|
| Value/type boundary | Bridged by dependent types | No boundary exists |
| Type-level computation | Universe hierarchy | Ordinary functions |
| When checked | Compile time | Runtime |
| Proof method | Formal logical deduction | Predicate evaluation + property testing |
| Generics | Universes and polymorphism | Functions returning types + `@` |
| New concepts needed | Dependent types, tactics, universes | `where`, `@` |
| Self-reference | Prove properties about Lean in Lean | Test properties about AQL in AQL |
| Learning curve | High (type theory) | Low (predicates are stack code) |

Lean needs dependent types to bridge the value/type gap. AQL has no
gap to bridge. The only missing piece is `where` — the ability to
say "this type, but only when this predicate holds." Everything else
is already expressible because types are already values.


## Syntax Summary

```
# Refinement type — the one new concept
type Pos (integer where [gt 0])
type Sorted ([:number] where [each-pair [lte] fold true [and]])

# Type-returning function — already works, no new syntax
def Vec fn [[n:integer] [type] [(list where [len n eq])]]
def Pair fn [[a:type, b:type] [type] [record [fst:a snd:b]]]
def Range fn [[lo:number, hi:number] [type] [
  (number where [dup gte lo swap lte hi and])
]]

# Generic function with type variables
def head fn [[xs:[:@a]] [@a] [xs . 0]]
def zip fn [[xs:[:@a], ys:[:@b]] [:(Pair @a @b)] [...]]

# Dependent return type — falls out of named params + where
def replicate fn [
  [n:integer, x:any]
  [(Vec n)]
  [iota n each [drop x]]
]

# Property testing
check [var [[x (random Pos)] x gt 0]] 1000
```

New keywords: `where`, `@`, `check`, `random`. That's it.
Everything else reuses existing AQL.
