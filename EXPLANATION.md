# AQL Explanation

This document explains the ideas behind AQL — the *why* behind the
syntax, the type system, and the runtime. It complements the
**[Tutorial](TUTORIAL.md)** (learning), **[How-To Guides](HOWTO.md)**
(tasks), and **[Reference](REFERENCE.md)** (precise behaviour).

## Contents

* [What is a concatenative language?](#what-is-a-concatenative-language)
* [The stack model](#the-stack-model)
* [Forward collection: beyond reverse Polish](#forward-collection-beyond-reverse-polish)
* [The `end` keyword](#the-end-keyword)
* [Type-directed dispatch](#type-directed-dispatch)
* [Type ordering](#type-ordering)
* [Immutability and mutability](#immutability-and-mutability)
* [Quotation and evaluation](#quotation-and-evaluation)
* [The Options pattern](#the-options-pattern)
* [Parallel execution model](#parallel-execution-model)
* [Errors as values](#errors-as-values)
* [Store and context](#store-and-context)
* [Module system](#module-system)
* [Ideals and type-kinds](#ideals-and-type-kinds)
* [Capabilities](#capabilities)
* [Design influences](#design-influences)


## What is a concatenative language?

In most languages you compose functions by *nesting* them — `f(g(x))`.
In a concatenative language you compose by *placing them next to each
other* — `x g f`. Every word is a function on the stack, and the
output of one is the input of the next. Composition is concatenation.

```
def double [dup add]
def quadruple [double double]
5 quadruple                       => 20
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
3 4 add 2 mul                     => 14
```

Step by step: push 3, push 4, `add` consumes both and pushes 7,
push 2, `mul` consumes both and pushes 14. There is no `tmp = a + b`
intermediate — the stack *is* the intermediate.

This eliminates the need for variable binding in simple cases. When
naming actually helps readability, `def`, `var`, and named-parameter
`fn` are there. The default is *point-free composition*.


## Forward collection: beyond reverse Polish

Traditional concatenative languages (Forth, Factor) use strict
reverse Polish notation: arguments always *precede* the word. AQL
extends this with **forward collection**: a word can gather
arguments that appear *after* it.

```
1 2 add                           => 3    # classic prefix
add 1 2                           => 3    # fully forward
1 add 2                           => 3    # infix: one stack, one forward
```

All three are equivalent. The word `add` needs two arguments. If
fewer are on the stack when it runs, it enters a forward-collecting
mode and consumes following tokens until its signature is filled.

This lets AQL read naturally in infix position. `10 sub 3` reads
"ten minus three"; `"hello" upper` reads "uppercase hello"; you
never have to mentally reverse-engineer `10 3 -`.

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
upper "hello" 42
```

1. `upper` needs one `String`. Stack is empty. Enter forward mode.
2. `"hello"` matches `String`. Collected. `upper` runs → `'HELLO'`.
3. `42` is not consumed (no waiting word). It is pushed.

Result: `'HELLO' 42`. The same logic prevents `add 1 "x"` from
silently doing the wrong thing: after `1` is collected as
`add`'s first argument, `"x"` won't match the second `Number`
slot, so collection stops. `add` then fails for arity reasons
(rather than computing nonsense).


## The `end` keyword

`end` is the escape hatch for the rare case where forward collection
goes too far. It stops the nearest waiting word, forcing any
remaining arguments to come from the stack:

```
set foo 99 end get foo            => 99
```

Without `end`, `set` would try to collect `get` and `foo` as
additional arguments. With `end`, `set` stops collecting after
two arguments and `get foo` is free to run.

It's needed less often than you might think — type-directed
collection cuts most of the cases — but when the type system can't
disambiguate (e.g., two adjacent words that both happen to accept
the same type), `end` is the simple, explicit fix.


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
add 1 2                           => 3      # Integer + Integer
add 1.0 2                         => 3.0    # Decimal + Number, promotes
add "a" "b"                       => 'ab'   # Scalar + Scalar, concatenates
```

The same `add` covers numeric addition and string concatenation —
not because it has an `if isString` inside, but because two of its
signatures match different argument shapes.

This makes the type system *active*: it isn't just for verification,
it drives behaviour.


## Type ordering

AQL exposes a single total order over every value. `cmp`, `lt`,
`gt`, `lte`, `gte`, and `sort` all consult it. The order is:

1. **LCA-Comparer.** Find the least common ancestor of the two
   types. If the ancestor declares a comparer, use it (so
   `Integer cmp Decimal` runs the numeric comparer at the
   `Number` level).
2. **Rank fallback.** Otherwise compare the integer `Rank` each
   type carries (cross-family comparisons are *defined*, not an
   error).

A bare type literal sorts strictly below every concrete inhabitant
of its family:

```
Integer lt 0                      => true
```

Lists are length-first then element-wise; maps are key-set then
value-wise. The end effect is that everything is sortable and the
order is well-defined.


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
default, list literals are quotations — they store their elements
unevaluated:

```
[1 add 2]                         => [1,add,2]    # NOT evaluated
```

Code-body positions — the second argument to `def`, all branches
of `if`, the body of `for`, the function passed to `each`, etc. —
implicitly take a quotation. This is why `def double [dup add]`
works: the list is stored, not run.

To evaluate a list at the point of use, three options:

* `do [1 add 2]` — evaluates as a sub-program, leaves results on
  the stack.
* `[1 add 2] call` — splices the list onto the current stack
  (designed for callback patterns).
* `quote foo` — opposite direction: stop the *next* token from
  being interpreted, keep it as data.

The duality — lists as both data and code — is the homoiconic core
that lets AQL do metaprogramming with no separate AST type.


## The Options pattern

Most non-trivial words accept an optional trailing `Map` to carry
named flags. This keeps the main signature small while leaving
room for growth:

```
"hello world" split " "                              # basic
"hello world" split " " {trim: true}                 # with options
"aaa" "a" "b" {scope:'all, count:2} replace          => 'bba'
await {mode: 'first} [[sleep 100 1] [sleep 10 2]]    => 2
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

```
await [[sleep 100 1] [sleep 100 2]]   => [1,2]
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

AQL treats errors as values, not exceptions. When `1 div 0`
"fails", it doesn't unwind the stack — it produces an `Error` value
that sits on the stack like any other:

```
do [1 div 0]                      => Error(div: division by zero)
```

`error` is a pattern-match: if the top of the stack is an `Error`,
run the handler; otherwise no-op. Handlers see the error value on
the stack and choose what to do with it:

```
do [1 div 0] error [drop 42]      => 42

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
context get x                     => 42
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
(`aql:math`, `aql:time`, `aql:matrix`, `aql:decision`) are
host-provided and follow the same surface.

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
the kernel routes `type`, `make`, `is`, and unification through
it the same way it does for the built-ins. The `aql:matrix`
module does exactly this for `Matrix` and `Vector`.

You usually don't write Ideals — you use them via `record`,
`table`, `object`, `make`. The framework matters only if you're
extending the language with a new kind of typed container.


## Capabilities

Side-effecting words (`read`, `write`, `fetch`, `sqlite-*`,
`timeout`, `interval`, `sleep`, vault lookups) are gated by
*capabilities* — runtime feature flags on the Registry. The CLI
turns them all on by default; embeddings (Wasm playground, an
LLM tool host) can disable any of them.

When a disabled word runs, it raises `Error{code:'cap_denied}`.
This is the same surface as any other error: the calling code can
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
