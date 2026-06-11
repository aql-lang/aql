# AQL Explanation

This document explains the ideas behind AQL — the *why* behind the
syntax, the type system, and the runtime. It complements the
**[Tutorial](TUTORIAL.md)** (learning), **[How-To Guides](HOWTO.md)**
(tasks), and **[Reference](REFERENCE.md)** (precise behaviour).

> **Notation.** In code, a `# returns …` comment shows what an
> expression evaluates to (`4 square  # returns 16`); in prose we say
> "`4 square` returns `16`". The comment is ordinary documentation, not
> special syntax. (AQL has no result arrow — `=>` is the
> anonymous-function word `afn` — so results are written as comments.)

## Contents

* [What is a concatenative language?](#what-is-a-concatenative-language)
* [The stack model](#the-stack-model)
* [Forward collection: beyond reverse Polish](#forward-collection-beyond-reverse-polish)
* [The `end` keyword](#the-end-keyword)
* [Type-directed dispatch](#type-directed-dispatch)
* [Function signatures and refinement types](#function-signatures-and-refinement-types)
* [Type ordering](#type-ordering)
* [Immutability and mutability](#immutability-and-mutability)
* [Quotation and evaluation](#quotation-and-evaluation)
* [Macros](#macros)
* [The Options pattern](#the-options-pattern)
* [Parallel execution model](#parallel-execution-model)
* [Errors as values](#errors-as-values)
* [Store and context](#store-and-context)
* [Module system](#module-system)
* [Ideals and type-kinds](#ideals-and-type-kinds)
* [Generics as memoised type construction](#generics-as-memoised-type-construction)
* [Capabilities](#capabilities)
* [Design influences](#design-influences)


## What is a concatenative language?

In most languages you compose functions by *nesting* them — `f(g(x))`.
In a concatenative language you compose by *placing them next to each
other* — `x g f`. Every word is a function on the stack, and the
output of one is the input of the next. Composition is concatenation.

```
def double word [dup add]
def quadruple word [double double]
5 quadruple                       # returns 20
```

`quadruple` is not "calling double twice" in the usual sense. It is
the textual juxtaposition of `double` with itself: a new program
that *is* the composition. There is no syntactic overhead for
combining operations — no parentheses, no `compose(...)`, no `.then`.


## The stack model

AQL has a single data stack. Every literal pushes; every word pops
its arguments and pushes its results. The stack is the implicit
data flow.

```
3 4 add                           # returns 7
7 2 mul                           # returns 14
```

Step by step: push 3, push 4, `add` consumes both and pushes 7; then
push 2, `mul` consumes both and pushes 14. There is no `tmp = a + b`
intermediate — the stack *is* the intermediate.

Written as one line the two steps compose by parenthesising the first
— `(3 4 add) 2 mul` returns `14`. The bare `3 4 add 2 mul` does **not** give
14: because `add` can also *collect forward* (the next section), it
grabs the following `2` as a second argument — computing `4 add 2 =
6` — which leaves `3` and `6` on the stack for `mul`, giving `18`.
When a trailing word would otherwise reach forward past the value you
mean to leave on the stack, insert a barrier: parenthesise the first
step — `(3 4 add) 2 mul` — or stop the collection with `end` or `;`
(`3 4 add ; 2 mul`). All three give `14`. A newline is **not** a
barrier: in a file `3 4 add` followed by `2 mul` on the next line still
evaluates to `18`, because the stack and forward collection both carry
across line breaks. (The REPL is the exception — it evaluates and
clears the stack line by line, so `3 4 add` then `2 mul` there prints
`7` and then errors for want of a second operand.)

This eliminates the need for variable binding in simple cases. When
naming actually helps readability, `def`, `var`, and named-parameter
`fn` are there. The default is *point-free composition*.


## Forward collection: beyond reverse Polish

Traditional concatenative languages (Forth, Factor) use strict
reverse Polish notation: arguments always *precede* the word. AQL
extends this with **forward collection**: a word can gather
arguments that appear *after* it.

```
1 2 add                           # returns 3 — classic prefix
add 1 2                           # returns 3 — fully forward
1 add 2                           # returns 3 — infix: one stack, one forward
```

All three are equivalent. The word `add` needs two arguments. If
fewer are on the stack when it runs, it enters a forward-collecting
mode and consumes following tokens until its signature is filled.

This lets AQL read naturally in infix position. `10 sub 3` reads
"ten minus three"; `not true` reads "not true"; and with the string
module imported, `StringUtil.upper "hello"` reads "uppercase hello".
You never have to mentally reverse-engineer `10 3 -`. (String words
like `upper`/`lower`/`split` are not built in — they live in
`aql:string-util`; see the [Reference](REFERENCE.md). Only words such
as `add`, `sub`, `mul`, `not`, `dup` are available without an import.)

### Forward by default — a cultural rule

Forward collection is not just available, it is the **language
default**, pinned as [ADR-004](ADR.md#adr-004): every word — core,
module, and your own `fn` definitions — collects forward unless its
semantics are intrinsically about the stack. The only standing
exception is the traditional Forth stack vocabulary (`dup`, `swap`,
`drop`, `over`, `rot`, …), which is stack-only because manipulating
the stack *is* what those words mean.

Two practical consequences:

- **Write the forward form first.** `word arg1 arg2` is the canonical
  call shape in code, docs, and examples; the pipeline form
  (`arg2 arg1 word`) is an equivalent you reach for when a value is
  already flowing on the stack.
- **Don't ask for a word to be flipped.** When forward collection is
  awkward at one call site, the per-call levers are grouping
  (`(…)`, `end`, `;`) and the modifiers `/s` (stack-only here),
  `/f` (forward-only here), and `/N` (exactly N args) — not a change
  to the word's default. `(1 add 1) print/s`, for instance, prints a
  value that is already on the stack.

### How collection works

When a word executes, AQL fills its argument slots in this order:

1. **Forward first.** Walk the tokens after the word in source
   order, left to right. Each token is evaluated and its type
   checked against the next-to-fill slot. If the type matches, the
   value goes into `args[0]`, then `args[1]`, …, and the walk
   continues. If the type doesn't match, or the walk hits a barrier
   (`end`, `)`, another function word), forward collection stops.
2. **Stack second.** Any slots still empty are filled from the
   stack, top of stack into the next-to-fill slot first.

So `args[0]` is whichever argument is closest to the word in source
position (or the deepest forward arg if all came from the right);
`args[N-1]` is the furthest. Handlers can rely on a single
positional contract regardless of how the user wrote the call.

For an asymmetric operation like `sub` (handler returns
`args[1] - args[0]` — deeper minus top), this single rule produces
the same answer for every call form:

```
10 3 sub        # all-stack:   args[0]=top=3, args[1]=10  →  7
10 sub 3        # mixed:       args[0]=3, args[1]=10       →  7
sub 3 10        # all-forward: args[0]=3, args[1]=10       →  7
```

After rearrangement, the word always sees arguments in signature
order. This is why the body doesn't have to care which side they
came from.

### Type-directed collection

Forward collection respects types. Consider:

```
not true 42
```

1. `not` needs one `Boolean`. Stack is empty. Enter forward mode.
2. `true` matches `Boolean`. Collected. `not` runs → `false`.
3. `42` is not consumed (no waiting word). It is pushed.

Result: `false 42`. Type matching governs *how far* a word reaches,
but it does **not** make a word reject an argument of a type one of
its signatures accepts. `add`, for instance, has both a numeric
signature and a `Scalar`-concatenation signature, so a string is a
*valid* second argument: `add 1 "x"` does not stop and fail — it
collects `"x"` and concatenates, giving `'x1'`. Forward collection
narrows where the boundary falls; it is not a guarantee that a
"wrong-looking" value will be refused.

### Forward greediness and stranded operands

Forward collection reaches *past* values already on the stack, which can
surprise you when a binary word sits between its operands:

```
1 2 add 3 mul                     # returns 5 — not the 9 you might expect
```

Reading left to right you might expect `(1 + 2) * 3 = 9`. Instead `add`
reaches forward for `3` and takes `2` from the stack — computing `3 + 2 = 5`
and leaving the `1` stranded underneath. `mul` then multiplies `5 * 1 = 5`.
Group the operands you actually mean to combine:

```
(1 2 add) 3 mul                   # returns 9
```

`aql check` surfaces the suspicious case as a non-gating advisory:

```text
$ aql check -e '1 2 add 3 mul'
check: [info] forward_strands_operand: add collected a forward argument
  while a Number operand was left unconsumed on the stack — it may be
  stranded; group the intended operands, e.g. (… add …)
```

It fires only when a word both reaches forward *and* takes a stack argument
while leaving a **sibling** operand (a value of the same type it just
consumed) behind — so the idiomatic swap form `10 sub 3` and a deliberately
deeper stack such as `1 2 3 add` stay quiet.


## The `end` keyword

`end` is the escape hatch for the rare case where forward collection
goes too far. It stops the nearest waiting word, forcing any
remaining arguments to come from the stack:

```
set foo 99 end get foo            # returns 99
```

Without `end`, `set` would try to collect `get` and `foo` as
additional arguments. With `end`, `set` stops collecting after
two arguments and `get foo` is free to run.

It's needed less often than you might think — type-directed
collection cuts most of the cases — but when the type system can't
disambiguate (e.g., two adjacent words that both happen to accept
the same type), `end` is the simple, explicit fix.

### A word only reaches for what it can use

Collection is *structure-first*: a word forward-collects a following token
only when one of its signatures could actually take it. Anything else — a
parenthesised expression, or a value whose type fits none of the word's
remaining argument slots — is left alone to run as its own expression. This
is why an unterminated `import` no longer swallows the code that uses it:

```
import "aql:string-util"          # no `end` needed…
(StringUtil.upper "hi")           # …this paren runs on its own → 'HI'
```

`import` takes its module path and stops, because the following
parenthesised expression matches none of its argument shapes. (Earlier
builds eagerly evaluated that paren *before* `import` ran, so it failed with
`undefined word: StringUtil` — see `design/LAZY-ARG-RESOLUTION.0.md`.) You
still reach for `end` when the next token *could* legitimately be the word's
argument — most commonly a second string path right after `import`:

```
"aql:math-util" import end "foo" print    # without `end`, import would load "foo"
```

An empty paren `()` is the empty expression: it produces no value, so it
contributes nothing where it appears (`5 () add 3` → `8`; the `()` is
simply skipped). Empty parens nest freely: `(())`, `( () )`, `(add 1 ())`
all parse.


## Type-directed dispatch

Every value in AQL carries a hierarchical type. The type
`Scalar/Number/Integer` is a child of `Scalar/Number`, which is a
child of `Scalar`, which is a child of `Any`. A child matches its
parent; the reverse is false.

Words declare *signatures* — patterns over types. When a word has
multiple signatures, the engine tries each in order and uses the
first that matches. This produces natural overloading without a
separate dispatch construct:

```
add 1 2                           # returns 3 — Integer + Integer
add 1.0 2                         # returns 3.0 — Float + Number, promotes
"a" "b" add                       # returns 'ab' — Scalar + Scalar, concatenates
```

The same `add` covers numeric addition and string concatenation —
not because it has an `if isString` inside, but because two of its
signatures match different argument shapes.

This makes the type system *active*: it isn't just for verification,
it drives behaviour.


## Function signatures and refinement types

A `fn` declares types for both its inputs and its outputs:

```
def avg fn [[a:Number b:Number] [Float] [(a add b) div 2.0]]
```

Inputs are checked when the function is called; outputs are checked
when its body finishes. The important principle is that these are not
two different checks — **a value is accepted at a parameter slot, a
return slot, or by the `is` word using one and the same membership
rule**: *is this value a member of that type?* Asking the same
question at every boundary is what keeps the language honest — a
function can't accept a value its own signature would reject, and a
value that flows out of one function flows into the next without
surprise.

What "member of that type" *means* depends on the kind of type, and
user types built with `refine` come in two kinds that answer it
differently. AQL keeps both, because they correspond to two genuinely
different intentions — and the rest of the world splits the same way.

**A bare refinement is a newtype.** `def UserId (refine Integer)`
gives you a distinct name with no new constraint. The point is
*identity*, not validation: a `UserId` and a `ProductId` are both
integers underneath, but mixing them is a bug. So a bare `Integer` is
**not** a `UserId` — you have to say so explicitly (`def id:UserId
42`). This is exactly how `newtype` works in Haskell, tuple structs in
Rust, defined types in Go, and opaque types in Scala: the wrapper is
deliberate, required at every boundary, symmetric.

**A predicate refinement is a subset type.** `def Big (Integer gt
10)` carves a subset out of the integers by a *predicate*. Here the
point is *validation*, not identity: any integer over ten already
qualifies, with nothing to construct. Membership is the predicate, and
it is checked the same way going in (a parameter) and coming out (a
return). This is how subset types work in F\*, Liquid Haskell, Dafny,
and Ada's range subtypes: value-sensitive and symmetric.

```
def UserId (refine Integer)               # newtype — identity
42 is UserId                  # returns false — a raw Integer is not a UserId
def id:UserId 42   id is UserId  # returns true — constructed explicitly

def Big (Integer gt 10)                   # subset — validation
50 is Big                     # returns true — 50 qualifies, no construction
5  is Big                     # returns false — 5 does not
```

The trap AQL avoids is treating these asymmetrically — lenient on the
way in, strict on the way out (or vice-versa). No mainstream language
does that on purpose; each picks one discipline per kind and applies
it at every boundary. AQL does the same: newtypes are nominal and
symmetric, subset types are value-sensitive and symmetric. The full
rationale is in `design/REFINE-NEWTYPE-VS-SUBSET.0.md`.


## Type ordering

AQL has a single total order over every value, computed in two stages:

1. **LCA-Comparer.** Find the least common ancestor of the two
   types. If the ancestor declares a comparer, use it (so
   `Integer`↔`Float` runs the numeric comparer at the `Number`
   level, and the instant-bearing `Time` leaves —
   `Date`/`DateTime`/`Instant` — compare chronologically at the
   `Time` level).
2. **Rank fallback.** Otherwise compare the integer `Rank` each
   type carries, giving cross-family pairs a defined position
   (roughly `Boolean < Number < String < … < List < Map < …`).

That total order is what **`sort`** and the collection words use, and
it is surfaced directly by **`tcmp`** (`1 tcmp "a"` returns `-1`).

But ordering values of *unrelated* families is almost always a
mistake, so the everyday ordering words — `cmp`, `lt`, `lte`, `gt`,
`gte` — are **family-restricted**. They compare only same-type values
or values stage 1 can place with a real family comparer; a pair that
would only order by the stage-2 Rank fallback (`1 lt "a"`,
`List lt Map`) raises `[aql/incomparable]` and points you at `tcmp`:

```
1 lt 2.0                          # returns true        — Integer and Float share Number
1 lt "a"                          # returns error       — different families; use tcmp
1 tcmp "a"                        # returns -1          — the total order, on demand
```

**Equality is *not* restricted.** `eq`/`neq`/`deq` compare across types
freely — values of different types are simply not equal (`1 eq "1"`
returns `false`), which is safe and needs no escape hatch.

A bare type literal sorts strictly below every concrete inhabitant
of its family (same-family, so the restricted words allow it):

```
Integer lt 0                      # returns true
```

Lists are length-first then element-wise; maps are key-set then
value-wise. The end effect: everything is sortable through `tcmp` /
`sort`, while a stray cross-type `lt`/`cmp` is caught rather than
silently answered.


## Immutability and mutability

AQL draws a deliberate line between immutable values and mutable
objects:

* **Scalars** (numbers, strings, booleans, atoms, times) are
  immutable. Every operation returns a new value.
* **Nodes** (lists, maps) are immutable values. List/map operations
  return new copies.
* **Ideals** (Store, Array, Record-instance, Table-instance,
  Object-instance, Tensor) are mutable. Their methods modify in
  place.

This distinction matters for concurrency. When `await` runs
parallel branches in separate sub-engines, immutable values are
safe to share, mutable Ideals are not — changes inside a branch
don't propagate to the parent.

Mutable instances are deliberately rare in idiomatic AQL: prefer
returning a new value to mutating, until a benchmark says otherwise.


## Quotation and evaluation

Lists are *dual-purpose*: data structures and code bodies. By
default a list literal is **evaluated** — its elements run and the
list holds the results:

```
[1 add 2]                         # returns [3] — evaluated by default
[1 2 3]                           # returns [1 2 3] — plain data, nothing to run
```

`quote` is the opt-out. It keeps the list (or the next token)
unevaluated, as data — so the elements stay as written (words become
atoms) and can be run later with `do`:

```
[1 add 2] size                    # returns 1 — already evaluated: one element, 3
quote [1 add 2] do                # returns 3 — held as data, then run
```

Some positions are *implicitly* quoted — they take a list as code to
run later, not a value to evaluate now: all branches of `if`, the
body of `for`, the function passed to `each` / `fold`, and the body
list of a `fn` definition. That is why `fn [[n:Integer] [Integer] [n
add 1]]` stores `[n add 1]` as the function body instead of trying to
run it at definition time. (A plain `def name [body]` is *not* one of
these positions — it evaluates the list and binds the result, so a
Forth-style splice uses the explicit `word` form: `def double word
[dup add]`.)

To evaluate a held list at the point of use, use `do`:

* `do [1 add 2]` — runs it as a sub-program, leaving results on the
  stack.

The duality — lists as both data and code — is the homoiconic core
that lets AQL do metaprogramming with no separate AST type.


## Macros

Quotation lets you *hold* code as data; **macros** let you *transform*
it. A macro is a function the engine runs at **expansion time**, on its
operands **as unevaluated code**, whose returned tokens are spliced into
the call site in place of the call. Where a normal word receives
*values*, a macro receives *forms* — so it can build new control
structures and syntax in AQL itself, not in Go.

```
def twice (macro [[e] [ quote [ unquote e add unquote e ] ]])
twice 5                           # returns 10
```

`twice 5` does not pass `5` to a function; it rewrites the call to the
code `5 add 5`, which then runs. The template is an ordinary `quote
[ … ]` region — default-data, the polarity flip from AQL's default-eval
— and `unquote` / `splice` are the holes where operands flow back in:
`unquote x` inserts one node, `splice xs` spreads a list's elements.

This is the classic LISP dividend, and AQL builds it from parts it
already had — `quote`, the splice marker behind `word`, raw-form
argument capture, and closure capture — rather than a new engine. Two
LISP problems come along for free:

* **Hygiene.** A name a macro introduces (a literal `def tmp` in a
  template) is auto-renamed to a fresh `gensym`, so it can never clash
  with a same-named variable at the call site. The `unquote`/`splice`
  boundary doubles as the provenance marker — names that came *from* the
  caller (through an escape) are left alone, which is exactly the
  distinction hygienic macro systems must otherwise synthesize.
* **Staging.** Macros are define-before-use and expand left-to-right, so
  generated code is just more source that re-enters the same evaluator.

Reference details and the full operator set are in
**[Reference: Macros](REFERENCE.md#macros)**; the design and its LISP
lineage are in `design/MACROS.0.md`.


## The Options pattern

Most non-trivial words accept an optional trailing `Map` to carry
named flags. This keeps the main signature small while leaving
room for growth:

<!-- aql-test: skip -->
```
# split/replace live in aql:string-util; shown unqualified here for
# brevity (in real code: "aql:string-util" import end StringUtil.split …).
"hello world" split " "                              # basic
"hello world" split " " {trim: true}                 # with options
"aaa" "a" "b" {scope:'all, count:2} replace          # returns 'bba'
await {mode: 'first} [[sleep 100 1] [sleep 10 2]]    # returns 2
```

By convention, every word that takes options declares the option
form as a separate, last-resort signature — so the options map is
always optional and the option-less call still works. Options keys
are atoms (`'all`, `'insensitive`, `'last`), not strings, so
typos surface at type-check time.


## Parallel execution model

The `await` word bridges AQL's sequential stack model with Go's
goroutines. Each element of the parallel list runs in its own
goroutine with an independent sub-engine:

<!-- aql-test: skip -->
```
await [[sleep 100 1] [sleep 100 2]]   # returns [1 2]
```

The four modes mirror JavaScript's Promise combinators, providing
familiar semantics:

* **`'all`** (default) — every branch must succeed; the first
  error fails the whole `await`.
* **`'full`** — every branch completes; results carry
  `{status:'ok|'error, value:...}` (like `Promise.allSettled`).
* **`'first`** — the first branch to complete wins (race).
* **`'any`** — the first non-error result wins.

Each branch runs under `do` semantics: the list is evaluated as a
sub-program, and the final stack value becomes the result. Mutable
side effects within a branch are local to that branch's sub-engine.


## Errors as values

AQL lets you treat errors as values rather than as exceptions — but
this happens at a `do [...]` boundary, not automatically. When a word
fails *in the open*, it unwinds: `1 div 0` on its own aborts the
program (and `1 div 0 dup` never reaches `dup`). Wrap the failing code
in a `do [...]` block and the failure is instead *reified* — the block
produces an `Error` value that sits on the stack like any other:

```
do [1 div 0]                      # returns error(division by zero)
```

So `do [...]` is the construct that converts an unwinding failure into
a first-class value; outside it, errors propagate.

`error` is a pattern-match: if the top of the stack is an `Error`,
run the handler; otherwise no-op. Handlers see the error value on
the stack and choose what to do with it:

```
do [1 div 0] error [drop 42]      # returns 42

do [read "missing.json"] error [
  dup .code eq 'io_error if [
    drop {}
  ] [
    "fatal: " printstr print
  ]
]
```

This makes error handling *compositional* — errors flow through
pipelines exactly like ordinary values, and you handle them at the
boundary where it makes sense.


## Store and context

The execution context is a `Store` — a mutable key-value map with
prototype-chain lookup. `set` writes to the current store, `get`
walks the prototype chain (parent first), and sub-engines (created
by `do`, `for`, `each`, `await`) inherit from the parent's store.

```
context set x 42
context get x                     # returns 42
```

This is functionally JavaScript-style prototype inheritance: child
contexts can read parent bindings, but writes are local. It gives
you lexical scoping with copy-on-write semantics, without any
explicit closure construct.


## Module system

A module is a fresh evaluation context. You build one by
evaluating a list in a new store, then expose the resulting
bindings under a namespace:

```
import utils [
  def helper [dup add]
  def greet fn [[String] [String] [`hello ${args.0}`]]
]
```

After import, `utils.helper` and `utils.greet` are available — the
dot is just field access on the module's exported map.

File imports load source from disk; renaming on import (`import
[helper as h] "..."`) prevents collisions; built-in modules
(`aql:math`, `aql:time-util`, `aql:matrix-util`, `aql:decision`) are
host-provided and follow the same shape.

There is no global namespace flattening: every imported binding
lives under the module's prefix until you alias it explicitly.


## Ideals and type-kinds

AQL has a system for *type-kinds* called **Ideals**. An Ideal is
the type-constructor turned into data — `Object`, `Record`, `Table`,
`Array` are all instances. Each Ideal carries:

* a name and a lattice anchor (so the kind has a place in the
  hierarchy),
* func fields for `Construct`, `Instantiate`, `Match`, `Format`,
  `Field`, `Equal`, `Unify`,
* metadata flags (`Inherits`, `OrderStrict`) that let shared
  helpers stay generic.

The practical consequence: a host program can register a *new*
type-kind (e.g. `Graph`, `Tensor`, `Stream`) at runtime, and
the kernel routes `refine`, `make`, `is`, and unification through
it the same way it does for the built-ins. The `aql:matrix-util`
module does exactly this for `Matrix` and `Vector`.

You usually don't write Ideals — you use them via `refine`
(Record / Table / Object) and `make`. The framework matters only if you're
extending the language with a new kind of typed container.


## Generics as memoised type construction

Generic types follow the same "types are values" doctrine as the
rest of the system. A schema (`def Box<T> class {value:T}`) is a
value holding a type body with placeholder nodes embedded;
instantiation (`Box of [Integer]`, or the sugar `Box<Integer>`) is
ordinary execution that substitutes the placeholders and **interns
the result** — one lattice node per (schema, arguments), minted as
a child of the schema. That one decision buys most of the
semantics for free: `typeof` names the instantiation because it IS
a type; `is Box` works by plain ancestry; two mentions of
`Box<Integer>` are `teq` because they're the same node; and the
checker gets monomorphization without new machinery, because its
per-call memo keys on argument types that now include the
instantiation's identity. Bounds reuse the membership question the
whole system already answers — `extends C` admits exactly what
`is C` admits, so predicates, disjunctions, and surfaces are all
valid bounds with zero special cases. The trade-off chosen for v1
is **invariance**: `Box<Integer>` is not a `Box<Number>`, the same
conservative default the nominal class system uses.


## Capabilities

Side-effecting words (`read`, `write`, `fetch`, `sqlite-*`,
`timeout`, `interval`, `sleep`, vault lookups) are gated by
*capabilities* — runtime feature flags on the Registry. The CLI
turns them all on by default; embeddings (Wasm playground, an
LLM tool host) can disable any of them.

When a disabled word runs, it raises `Error{code:'cap_denied}`.
This is the same shape as any other error: the calling code can
catch it with `do ... error [...]` and react appropriately.

Capabilities are deliberately coarse — one flag per system —
because the per-call enforcement happens *inside* the words. Finer
sandboxing (e.g., a path whitelist for `read`) is layered on top
via the `capabilities.FileOps` interface, which an embedder
provides directly.


## Design influences

AQL draws from several traditions:

* **Forth, Factor** — stack-based execution, word definitions,
  quotations, the basic "code is a sequence of words" feel.
* **APL, J, K** — array operations (`iota`, `reshape`, `grade`,
  `outer`, `inner`), the array-everywhere intuition.
* **JavaScript** — Promise combinators (`all`, `allSettled`, `race`,
  `any`), prototype-chain stores, template literals.
* **SQL** — relational thinking behind tables and records.
* **Prolog** — unification-based type matching (`unify`).
* **Haskell** — type-directed dispatch, immutable-by-default values,
  total comparison.
* **Lisp** — homoiconicity, lists as data and code, REPL-driven
  development.

The result is intended to feel like a stack language that doesn't
fight readability, a query language that doesn't fight composition,
and an array language that doesn't require you to leave the rest
of programming behind.
