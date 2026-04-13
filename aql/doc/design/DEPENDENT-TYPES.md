# Dependent Types for AQL

Design proposal for adding dependent types to AQL, inspired by the
ideas in [A Perfectable Programming Language](https://alok.github.io/lean-pages/perfectable-lean/).
That article argues Lean is uniquely "perfectable" because you can
write properties about Lean, in Lean, using dependent types. This
document explores what that idea looks like translated into a
concatenative, stack-based, dynamically-typed language.


## Core Insight: Types Are Stack Words

AQL already treats types as first-class values — type literals sit on
the stack, `unify` checks type compatibility, `is` tests membership,
`record` builds structured types. The natural extension is: **let
types be computed by the same stack words that compute values.**

A dependent type is just a type that depends on a value. In a stack
language, that means a type that was produced by running code. AQL
already does this:

```
record [x:number y:string]       # type built by a word at runtime
[:number]                        # type built by syntax at parse time
(number or string)               # type built by disjunction
```

The proposal extends this principle to three new capabilities:

1. **Refinement types** — types with value predicates (`where`)
2. **Type functions** — words that produce types from arguments (`tyfn`)
3. **Dependent signatures** — function signatures where output types
   reference input values


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
def abs-pos fn [[x:Pos] [Pos] [x]]
def abs-neg fn [[x:(integer where [lt 0])] [Pos] [x negate]]

5 abs-pos                => 5
-3 abs-neg               => 3
```

The engine tries signatures in order. When a signature slot has a
refined type, the predicate runs as part of matching. If it returns
false, that signature is skipped and the next is tried — exactly
like a type mismatch today.


## 2. Type Functions: `tyfn`

A type function is a word that takes arguments (values or types) and
produces a type. This is how AQL gets generics.

### Syntax

```
tyfn [params] [body]
```

Where `params` is a list of typed parameter declarations (like `fn`
input params) and `body` is a block that computes and leaves a type
on the stack.

### Examples

```
# Parameterized list type (generic container)
def ListOf tyfn [t:type] [[:t]]
type Ints (ListOf integer)
[1,2,3] is Ints             => true
["a"] is Ints                => false

# Fixed-length list
def Vec tyfn [n:integer] [(list where [len n eq])]
type Vec3 (Vec 3)
[1,2,3] is Vec3              => true
[1,2] is Vec3                => false

# Generic pair record
def Pair tyfn [a:type, b:type] [record [fst:a snd:b]]
type IntStr (Pair integer string)
make IntStr [1 "hello"]      => {fst:1, snd:'hello'}

# Bounded number range
def Range tyfn [lo:number, hi:number] [
  (number where [dup gte lo swap lte hi and])
]
type Byte (Range 0 255)
42 is Byte                   => true
300 is Byte                  => false

# Matrix type: list of lists with dimensions
def Matrix tyfn [rows:integer, cols:integer] [
  (Vec rows where [each [len cols eq] fold true [and]])
]
type Mat2x3 (Matrix 2 3)
```

### Semantics

`tyfn` creates a callable word (like `fn`) that:
1. Binds its parameters from the stack / forward collection
2. Evaluates the body in a sub-engine
3. Returns the resulting type value

Type functions participate in forward collection like any word:
```
Vec 5                        # forward: Vec collects 5
5 Vec end                    # prefix: 5 on stack, Vec consumes it
```

### Interaction with `type`

`type` already accepts any type value as its body. Type functions
compose naturally:

```
def Vec tyfn [n:integer] [(list where [len n eq])]
type Pos (integer where [gt 0])
def PosVec tyfn [n:Pos] [(Vec n where [each [is Pos] fold true [and]])]

type PV3 (PosVec 3)
[1,2,3] is PV3              => true
[1,-1,3] is PV3             => false
[1,2] is PV3                => false
```


## 3. Type Variables in Signatures: `@`

For generic functions, we need type variables — placeholders bound
during signature matching.

### Syntax

`@name` in a signature position introduces a type variable. The first
occurrence binds it; subsequent occurrences require the same type.

### Examples

```
# Identity: preserves type
def id fn [[@a] [@a] []]

# Swap: generic over both types
def swap2 fn [[@a, @b] [@b, @a] [swap]]

# Map preserves container, transforms element type
def map-typed fn [
  [xs:[:@a], f:(@a -> @b)]
  [:@b]
  [xs each f]
]
```

### Semantics

During signature matching, when the engine encounters `@a`:
1. If `@a` is unbound: bind it to the concrete type of the argument.
   The match succeeds (any value matches an unbound variable).
2. If `@a` is already bound: check that the argument's type unifies
   with the bound type. If yes, narrow the binding. If no, the
   signature fails.

Type variable bindings are scoped to a single signature match
attempt and discarded afterward. They do not persist.

### Interaction with Refinement Types

Type variables and refinement compose:

```
# A function that takes two values of the same type, both positive
def add-same fn [
  [x:(@a where [gt 0]), y:@a]
  [@a]
  [x add y]
]
```

Here `@a` is bound by the first argument's base type, then the
second argument must match. The `where` clause further constrains
the first argument.


## 4. Dependent Signatures: Output Types Referencing Inputs

The most powerful feature: the output type of a function can
reference named input parameters.

### Syntax

Named parameters from the input signature are visible in the output
type expression:

```
def word fn [[input-params] [output-type-expr] [body]]
```

Where `output-type-expr` may reference names bound in `input-params`.

### Examples

```
# replicate: returns a list of exactly n copies
def Vec tyfn [n:integer] [(list where [len n eq])]

def replicate fn [
  [n:integer, x:@a]
  [(Vec n)]
  [iota n each [drop x]]
]

replicate 3 "x"             => ["x","x","x"]
# return type: (Vec 3), i.e., list of length 3

# take: output length = min(n, input length)
def take fn [
  [n:integer, xs:list]
  [(list where [len (n min (xs len)) eq])]
  [xs slice 0 n]
]

# split-at: returns two lists whose lengths sum to the original
def split-at fn [
  [n:integer, xs:list]
  [(Vec n), (Vec (xs len sub n))]
  [xs slice 0 n  xs slice n (xs len)]
]
```

### Semantics

When a function with a dependent output signature returns:
1. The return values are on the stack.
2. The engine evaluates the output type expression with input
   parameter bindings still in scope.
3. Each return value is checked against its corresponding output type.
4. On mismatch, an error value is produced (not a panic — consistent
   with AQL's error-as-values philosophy).

This is a **runtime contract**, not a compile-time proof. The
predicate executes after the body. Think of it as a postcondition.


## 5. Contracts: `pre` and `post`

For explicit design-by-contract style, two new words provide
preconditions and postconditions without modifying `fn` syntax:

### Syntax

```
def word fn [
  [params]
  [return-types]
  [body]
] pre [precondition] post [postcondition]
```

### Examples

```
# Binary search requires sorted input
def bsearch fn [
  [xs:[:number], target:number]
  [integer]
  [...]
] pre [xs is Sorted] post [
  dup gte 0 if [xs swap get target eq] [true]
]
```

`pre` receives named parameters on the stack/in scope. `post`
receives the return value(s) and named parameters. Both must leave a
boolean. On failure, an error value is produced.

### Semantics

- `pre` runs before the body. If it returns false, the body is not
  executed; an error value is pushed instead.
- `post` runs after the body. If it returns false, the return values
  are replaced with an error value.
- Both run in sub-engines (no mutation of caller state).


## 6. Property Testing: `check`

The article argues Lean's power is writing properties *about* code
*in* the language. AQL can approximate this with property-based
testing using the same language for both properties and code.

### Syntax

```
check [property-block] n
check [property-block] n {options}
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
check [
  var [[n (random (Range 0 100))]
    replicate n 0 len eq n
  ]
] 200
```

### Semantics

`check` evaluates the property block `n` times. Each evaluation
should produce a boolean. If any returns false, `check` pushes an
error value containing the failing inputs (for shrinking/debugging).
If all pass, it pushes `true`.

The `random` word generates random values matching a type —
including refined types. For `(integer where [gt 0])`, it generates
random integers and filters/retries until the predicate passes.


## Does This Provide Generics?

**Yes.** Generics fall out naturally from two features:

### Parametric Polymorphism via Type Functions

```
def ListOf tyfn [t:type] [[:t]]
def MapOf tyfn [k:type, v:type] [{:v}]  # note: map keys are always strings
def Pair tyfn [a:type, b:type] [record [fst:a snd:b]]

type IntList (ListOf integer)
type StrNumMap (MapOf string number)
type IntPair (Pair integer integer)
```

This is exactly what Java/C#/TypeScript call generics — type
constructors parameterized by other types.

### Generic Functions via Type Variables

```
# Works for any element type
def head fn [
  [xs:[:@a]]
  [@a]
  [xs . 0]
]

# Preserves element type
def reverse fn [
  [xs:[:@a]]
  [:@a]
  [xs len iota each [xs len sub 1 sub] each [xs swap get]]
]
```

### Generic Functions via Type Dispatch (Already Exists)

AQL already has a form of generics through type-directed dispatch:

```
def stringify fn [
  [x:integer] [string] [make string x]
  [x:string] [string] [x]
  [x:list] [string] [x each [stringify] " " join]
]
```

The new features complement this with parametric generics where the
*same code* works for *any* type, rather than requiring separate
implementations.


## Implementation Approach

### Phase 1: Refinement Types (`where`)

**Effort: Medium.** Extends the type system, touches signature matching.

1. **Parser**: Recognize `(type where [block])` as a new type form.
   The parser already handles parenthesized expressions and
   `(type or type)` disjunctions. Add `where` as a keyword within
   type expressions that captures the following quoted block.

2. **Type representation**: Add a `Predicate` field to `VType` (or
   create a `RefinedType` wrapper struct) that holds the quoted
   block as a `[]Value`.

3. **Signature matching**: In `MatchSignatureReversed` and forward
   matching, after the base type matches, run the predicate in a
   sub-engine. If it returns false, the match fails.

4. **`unify`**: When one side is refined, check base type first,
   then run predicate if a concrete value is present.

5. **`is`**: Same — base type check, then predicate.

### Phase 2: Type Functions (`tyfn`)

**Effort: Low-Medium.** Very similar to `fn` implementation.

1. **Parser**: Recognize `tyfn` as a word like `fn`. The parameter
   list uses the same syntax as `fn` input params.

2. **Engine**: `tyfn` creates a callable value (like `FnDef`) that,
   when invoked, evaluates the body and returns a type value. The
   body must leave exactly one type value on the stack.

3. **`type` integration**: `type Foo (Bar 3)` already works if
   `Bar` is a word that returns a type. `tyfn` just formalizes this.

### Phase 3: Type Variables (`@`)

**Effort: Medium-High.** Requires changes to signature matching.

1. **Parser**: Recognize `@name` in signature positions as a type
   variable reference. Store as a new value kind (e.g., `TypeVar`).

2. **Signature matching**: Maintain a binding map
   (`map[string]*VType`) during each match attempt. On first
   encounter of `@a`, bind it. On subsequent encounters, unify.

3. **Return type evaluation**: After matching, substitute bound type
   variables into the output type expression.

### Phase 4: Dependent Signatures

**Effort: Medium.** Builds on Phase 3.

1. **Output type evaluation**: Instead of treating output types as
   static annotations, evaluate them as expressions with input
   parameter bindings in scope. This is essentially `do` with a
   pre-populated context.

2. **Postcondition checking**: After the body runs, check each
   return value against the evaluated output type. On mismatch,
   produce an error value.

### Phase 5: `check` and Property Testing

**Effort: Low.** Mostly a new native word.

1. **`random` word**: Generate random values for a given type. For
   base types, straightforward. For refined types, generate-and-test.
   For records, generate each field.

2. **`check` word**: Loop `n` times, evaluate block, collect results.
   On failure, report the failing input values.


## Design Tradeoffs

### Runtime vs. Compile-Time

This proposal is entirely **runtime**. Predicates execute when
values are matched, not when code is parsed. This fits AQL's dynamic
nature but means:

- **Pro**: No separate type-checking phase. Types and code use the
  same language. Refinement predicates can be arbitrarily expressive.
- **Pro**: Works with AQL's existing dynamic dispatch — refined types
  are just more specific dispatchers.
- **Con**: Type errors are caught at execution time, not definition
  time. A function with a wrong return type won't fail until called.
- **Con**: Predicate evaluation has runtime cost. For hot paths,
  predicates could be expensive.

A future `verify` mode could attempt static analysis over refinement
predicates, but this is not required for the initial design.

### Predicate Termination

Refinement predicates could loop forever. The sub-engine should
enforce a timeout (reuse `await` timeout semantics). If a predicate
times out, the match fails.

### Predicate Purity

Predicates run in sub-engines and should not observe or cause side
effects. The sub-engine should have a restricted context (no `write`,
`fetch`, or other I/O words). This is enforceable by stripping I/O
words from the sub-engine's registry.


## Comparison: AQL Dependent Types vs. Lean

| Aspect | Lean | AQL (Proposed) |
|--------|------|----------------|
| When checked | Compile time | Runtime |
| Proof method | Formal logical deduction | Predicate evaluation + property testing |
| Expressiveness | Arbitrary propositions (Prop) | Arbitrary predicates (stack blocks) |
| Guarantees | Sound and complete (if it compiles, it's correct) | Sound but incomplete (runtime only catches executed paths) |
| Syntax overhead | Theorem declarations, tactic proofs | `where` clauses, same syntax as regular code |
| Self-reference | Can prove properties about Lean in Lean | Can test properties about AQL in AQL |
| Generics | Universes and polymorphism | `tyfn` and `@` type variables |
| Learning curve | High (proof tactics, type theory) | Low (predicates are just AQL code) |

Lean gives you certainty: if the proof compiles, the property holds
for all inputs. AQL gives you confidence: if the property passes
1000 random tests with `check`, it probably holds. The tradeoff is
accessibility — AQL programmers don't need to learn proof tactics.


## Syntax Summary

```
# Refinement type
type Pos (integer where [gt 0])

# Type function (generics)
def Vec tyfn [n:integer] [(list where [len n eq])]
def Pair tyfn [a:type, b:type] [record [fst:a snd:b]]

# Type variable in signature
def id fn [[@a] [@a] []]
def head fn [[xs:[:@a]] [@a] [xs . 0]]

# Dependent output type
def replicate fn [
  [n:integer, x:@a]
  [(Vec n)]
  [iota n each [drop x]]
]

# Contract
def bsearch fn [...] pre [xs is Sorted] post [result gte 0]

# Property testing
check [var [[n (random Pos)] replicate n 0 len eq n]] 1000
```

All of these use existing AQL syntax patterns (quoted blocks, pair
syntax, parenthesized type expressions) with minimal new keywords:
`where`, `tyfn`, `@`, `pre`, `post`, `check`, `random`.
