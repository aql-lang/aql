# AQL Tutorial

This tutorial teaches AQL from the ground up. It is meant to be read
in order, with the REPL open at your side — type every example and
poke at it. By the end you'll be comfortable with forward calls and
the underlying stack model, the type system, defining typed
functions, working with records and tables, doing concurrent work
with `await`, and packaging code as modules.

If you only want a recipe for a specific task, see the
**[How-To Guides](HOWTO.md)**. If you want the precise behaviour of
a word, see the **[Reference](REFERENCE.md)**.

> **Notation.** In code, a trailing `# returns …` comment shows what an
> expression evaluates to — `square 4  # returns 16` — and in a REPL
> transcript the same appears after the prompt: `aql> square 4  #
> returns 16`. The comment is ordinary documentation (`#` begins a line
> comment), not special syntax; in prose we just say "`square 4`
> returns `16`". (AQL has no result arrow — `=>` is real syntax, the
> anonymous-function word `afn` — which is why results are written as
> comments here.) Note also that the REPL clears the stack after each
> line, so a multi-step computation that relies on leftover stack
> values must go on **one** line (or in a file).


## 1. Install and start the REPL

Until v0.1.0 is tagged, build from a clone (the cmd/go module
carries local `replace` directives, so `go install …@latest` is
not yet supported):

```bash
git clone https://github.com/aql-lang/aql
cd aql/cmd/go && go install ./aql
aql
```

You should see:

```
aql 0.1.0-dev
aql>
```

The prompt accepts AQL expressions. Press `Enter` to evaluate.
`Ctrl-D` (or `exit`) leaves the REPL.

You can also evaluate a one-liner from the shell:

```bash
aql do 'add 1 2'
# 3
```

Or run a file:

```bash
aql script.aql
aql -e '10 sub 3'
```

(`-e` evaluates one expression and exits. Uppercasing a string needs
the string-util module — `aql -e 'import "aql:string-util"
StringUtil.upper "hello"'` — see [§5: Strings](#5-strings).)


## 2. Your first expressions

A program is a sequence of *words* and *values*. A word takes its
arguments where you write them, so a call reads left to right, and a
value by itself is just itself — whatever your line leaves behind is
what the REPL prints:

```
aql> add 1 2                         # returns 3
aql> mul 4 5                         # returns 20
aql> 42                              # returns 42
```

Two pieces of punctuation you'll use from the start:

**Parens group a sub-expression.** The group is evaluated first and
its result feeds the surrounding call — exactly what you'd expect
from a conventional language:

```
aql> add 1 (mul 2 3)                 # returns 7
aql> mul (add 1 2) (10 sub 3)        # returns 21
```

(`10 sub 3` is `10 - 3` written infix — more on that in §3.)

**`;` ends a statement.** Several statements can share a line; each
runs in order, and the line's combined results are printed. `;` is
just punctuation for the word `end` — the two are interchangeable:

```
aql> add 1 2; mul 3 4
3 12
aql> def x 5; mul x x                # returns 25
```

You'll see `;` (or `end`) constantly after `def` and `import` lines,
separating setup from the expression that uses it.


## 3. Three ways to call a word

You've been writing the **forward** form — arguments after the word —
which is the recommended style for new code. The same `add` also
works **infix** and, because AQL is concatenative under the hood,
**all-stack**:

```
aql> add 1 2        # returns 3 — forward: both args after the word
aql> 1 add 2        # returns 3 — infix: one before, one after
aql> 1 2 add        # returns 3 — all-stack: both args before the word
```

The all-stack form is the *stack machine* showing through: a literal
pushes itself onto a value stack, and a word pops what it needs.
Step by step in `1 2 add`: `1` is pushed, `2` is pushed, `add` pops
both and pushes their sum. Values nothing consumes stay on the
stack — that's what the REPL prints:

```
aql> 1 2 3
1 2 3
```

Pipelines lean on this (a value left by one word is picked up by the
next), and §6 covers the words that rearrange the stack directly.
Until then, forward form does everything you need.

### The argument-order rule

When a word runs, AQL fills its parameter slots `args[0]`, `args[1]`,
… in this order:

1. **Take tokens after the word, in source order**, into `args[0]`,
   `args[1]`, …, until either the signature is full or a barrier is
   hit (`end`, `)`, another function word, type mismatch).
2. **Fill any slots still empty from the stack, top-first** — the
   top of stack goes into the next-to-fill slot.

So for an asymmetric operation like `sub` (whose handler computes
`args[1] - args[0]` — deeper minus top — to read naturally as
"a minus b"), all three call forms compute the same thing:

```
aql> 10 sub 3       # returns 7 — infix: args[0]=3 (forward), args[1]=10 (stack) → 10 - 3
aql> sub 3 10       # returns 7 — all-forward: args[0]=3 (first), args[1]=10 (second) → 10 - 3
aql> 10 3 sub       # returns 7 — all-stack: args[0]=top=3, args[1]=10 → 10 - 3
```

The pattern: `a sub b` always means `a - b`, no matter where `a` and
`b` are written. The three forms above all encode `a=10, b=3`. Note
that `sub 10 3` is *not* the same expression — it encodes `a=3,
b=10`, giving `-7`. For non-commutative operations, the infix form
(`10 sub 3`) is the one that reads the way it computes.

User-defined functions follow the same rule. For
`def show fn [[a:Number b:Number] [String] [`${a} and ${b}`]]`:

```
aql> show 1 2       # forward: args[0]=1=a, args[1]=2=b
'1 and 2'
aql> 2 show 1       # mixed: args[0]=1 (forward)=a, args[1]=2 (stack)=b
'1 and 2'
aql> 2 1 show       # all-stack: args[0]=top=1=a, args[1]=2=b
'1 and 2'
```

See **[Explanation §Forward Collection](EXPLANATION.md#forward-collection-beyond-reverse-polish)**
for the underlying mechanics.


## 4. Numbers, rounding, math

Integers and floats are different leaves of the same `Number`
type. Arithmetic auto-promotes. The core arithmetic words live at
the top level:

```
aql> mul 4 5            # returns 20
aql> 2 pow 10           # returns 1024
aql> 7 div 2            # returns 3 — integer division
aql> 7.0 div 2          # returns 3.5 — float: real division
aql> 10 mod 3           # returns 1
aql> 10 sub 3           # returns 7 — a sub b ≡ a - b (see §3)
```

For absolute value, rounding, roots, logs, trig, and the standard
constants, import the `aql:math-util` module — its words register
under the `MathUtil.` namespace (the pattern for every built-in
module: the `aql:NAME-util` id binds the `NameUtil` namespace; see
the [module table in §20](#20-modules--namespaces-and-imports)):

```
aql> import "aql:math-util"
aql> MathUtil.abs -5        # returns 5
aql> MathUtil.min 3 5       # returns 3
aql> MathUtil.max 3 5       # returns 5
aql> MathUtil.floor 3.7     # returns 3
aql> MathUtil.ceil 3.2      # returns 4
aql> MathUtil.round 3.5     # returns 4
aql> MathUtil.trunc 3.9     # returns 3
aql> MathUtil.sqrt 16       # returns 4.0
aql> MathUtil.log MathUtil.e    # returns 1.0
aql> MathUtil.sin 0         # returns 0.0
aql> MathUtil.hypot 3 4     # returns 5.0
aql> MathUtil.pi            # returns 3.141592653589793
aql> MathUtil.e             # returns 2.718281828459045
```


## 5. Strings

Strings use single or double quotes (they're interchangeable):

The `aql:string-util` words put the **subject string last** (the data-last
grain), so the clearest all-forward form is `WORD arg… subject` — e.g.
`split sep input`, `contains needle haystack`, `indexof needle haystack`,
`replace search repl input`. This also lets the subject flow from a pipeline
(`input WORD arg`). An options map, when present, trails the subject. See
[§3: the argument-order rule](#the-argument-order-rule).

```
aql> import "aql:string-util" "hello" StringUtil.upper                 # returns 'HELLO'
aql> import "aql:string-util" "HELLO" StringUtil.lower                 # returns 'hello'
aql> import "aql:string-util" StringUtil.split "," "hello,world"       # returns ['hello' 'world'] — subject (input) LAST
aql> import "aql:string-util" ["a","b","c"] StringUtil.concat          # returns 'abc' — joins list elements
aql> import "aql:string-util" StringUtil.contains "ell" "hello"        # returns true — haystack LAST
aql> import "aql:string-util" StringUtil.indexof "ll" "hello"          # returns 2 — haystack LAST: `indexof needle haystack`
aql> "hello" slice 1 3             # returns 'el'
aql> import "aql:string-util" StringUtil.replace "l" "r" "hello"       # returns 'herlo' — subject (input) LAST
aql> import "aql:string-util" "  hi  " StringUtil.trim                 # returns 'hi'
aql> import "aql:string-util" "hi" StringUtil.pad 5                    # returns 'hi   '
```

Backtick template strings interpolate `${...}` expressions:

```
aql> def name "world"
aql> `hello ${name}`               # returns 'hello world'
aql> `2 + 3 = ${add 2 3}`         # returns '2 + 3 = 5'
```

Templates nest:

```
aql> `a${`inner ${add 1 2}`}b`     # returns 'ainner 3b'
```


## 6. Manipulating the stack

When the stack model isn't quite enough, these words rearrange it:

```
aql> 5 dup              # returns 5 5 — duplicate top
aql> 1 2 swap           # returns 2 1 — exchange top two
aql> 1 2 3 drop         # returns 1 2 — discard top
aql> 1 2 over           # returns 1 2 1 — copy second to top
aql> 1 2 3 rot          # returns 2 3 1 — rotate top three
aql> 1 2 nip            # returns 2 — remove second
aql> 1 2 tuck           # returns 2 1 2 — copy top below second
aql> depth              # returns 0 — current stack size
```

Most of the time you won't need these — forward collection covers
the common cases. They're a tool for when the shape of the stack
fights you.


## 7. Lists and maps

Lists use square brackets, maps use braces:

```
aql> [1, 2, 3]                       # returns [1 2 3]
aql> {name: "Alice", age: 30}        # returns {age:30 name:'Alice'} — keys sort
```

Commas are optional inside literals — both `[1 2 3]` and `[1, 2, 3]`
parse the same.

A map entry can be just a bare name — `{foo}` is shorthand for
`{foo: foo}`, the same as in JavaScript:

```
aql> def x 1
aql> def y 2
aql> {x y}                           # returns {x:1 y:2}
```

(See [Reference: Map field shorthand](REFERENCE.md#map-field-shorthand)
for the `/r` and `?` variants.)

The dot operator accesses fields by name or by index:

```
aql> {name: "Alice"} . name          # returns 'Alice'
aql> [10, 20, 30] . 1                # returns 20
aql> {a: {b: 99}} . a . b            # returns 99
```

Use `!.` (also called `getr`) when the key *must* exist — it raises
an error instead of returning `none`:

```
aql> {x:1} !. y                      # returns error: key "y" not found
```

Lists and maps nest freely:

```
aql> [{x:1, y:2}, {x:3, y:4}]
aql> {users: ["Alice", "Bob"], count: 2}
```


## 8. Defining words

Use `def` to give a value (or a code block) a name:

```
aql> def x 42
aql> x                               # returns 42
```

When the body is a list, calling the word *runs* the list:

```
aql> def double word [dup add]
aql> 5 double                        # returns 10
aql> 3 double double                 # returns 12
```

Composition is concatenation:

<!-- aql-test: skip -->
```
aql> def quadruple word [double double]
aql> 5 quadruple                     # returns 20
```

To remove a definition use `undef`:

```
aql> undef x
```


## 9. Typed functions with `fn`

`fn` builds a typed function. The shape is a list of
`[input-sig] [output-sig] [body]` triples:

```
aql> def square fn [[x:Number] [Number] [mul x x]]
aql> square 5                        # returns 25
aql> square 2.5                      # returns 6.25
```

For the common one-signature, one-parameter case there is also a
3-arg form — the triple without the wrapping list:

```
aql> def square fn x:Number [Number] [mul x x]
aql> square 5                        # returns 25
```

The 3-arg form's input must not be a list (a list after `fn` always
means the list-of-triples shape above), so multi-parameter and
multi-signature functions use the list form.

Named parameters (like `x:Number`) bind to stack values automatically
inside the body. You can also use the implicit `args` list:

```
aql> def greet fn [[String] [String] [`hello ${args.0}`]]
aql> greet "world"                   # returns 'hello world'
```

Multiple signatures give you ad-hoc polymorphism — first match wins:

```
aql> def inc fn [
  [Integer] [Integer] [add 1]
  [Float] [Float] [add 1.0]
]
aql> inc 5                           # returns 6
aql> inc 2.5                         # returns 3.5
```

From here on, make **`aql check`** part of your loop: it type-checks
a file (or a one-liner via `aql check -e '…'`) without running it,
and it catches exactly the mistakes typed functions introduce —
no-matching-signature calls, a function value left uncalled on the
stack, a body that can't produce the declared return type. Some of
its diagnostics are *more* informative than a run: where running
leaves a named function value sitting on the stack, `aql check`
reports `uncalled_function` and points at the call site. Check
first, then run.


## 10. Conditionals and loops

`if` takes a condition, a then-branch, and an optional else-branch.
The branches are lists (which is why they're not evaluated up-front):

```
aql> if (5 gt 3) ["yes"] ["no"]      # returns 'yes'
aql> 0 if ["truthy"] ["falsy"]       # returns 'falsy'
```

The condition is coerced to a boolean by **presence, not content**.
Falsey values are `false`, `0`, `none`, and the empty list/map/string.
Every non-empty string is **true**, so `"false"`, `"FALSE"`, `"0"`,
and `"no"` all take the then-branch — a String's characters are never
inspected. When in doubt, compare explicitly (`x eq 0`, `s eq ""`)
rather than relying on string truthiness.

`for` iterates over a numeric range, pushing the counter into the
body each step:

<!-- aql-test: skip -->
```
aql> for 5 [dup mul]                 # returns 0 1 4 9 16
aql> for [1, 4] [dup mul]            # returns 1 4 9
aql> for [0, 10, 2] [dup mul]        # returns 0 4 16 36 64
```

`break` and `continue` work inside the body:

```
aql> for 10 [dup gt 5 if [break]]
```


## 11. Higher-order list words

These are the bread-and-butter of array programming in AQL. Note
how the multi-list combinators use the all-forward call shape so
each list argument lands in a predictable slot — see
[§3: the argument-order rule](#the-argument-order-rule).

```
aql> [1, 2, 3] each [dup mul]        # returns [1 4 9]
aql> fold [add] [1, 2, 3, 4, 5] 0    # returns 15 — body, data, init
aql> scan [add] [1, 2, 3]            # returns [1 3 6]
```

Sequence-building:

```
aql> iota 5                          # returns [0 1 2 3 4]
aql> range 2 6                       # returns [2 3 4 5]
aql> [1, 2, 3] reverse                # returns [3 2 1]
```

Reshaping, ordering, and grouping live in the `aql:array-util` module
(reached via the `ArrayUtil.` prefix after importing):

```
aql> import "aql:array-util"
aql> iota 6 ArrayUtil.reshape [2, 3]     # returns [[0 1 2] [3 4 5]]
aql> [1, 2, 2, 3] ArrayUtil.unique       # returns [1 2 3]
aql> [3, 1, 2] ArrayUtil.grade           # returns [1 2 0]
```

`outer` and `inner` are APL-style array combinators (built-in):

```
aql> outer [mul] [10, 20] [1, 2]     # returns [[10 20] [20 40]]
aql> inner [add] [mul] [3, 4] [1, 2] # body order: combine, product
```


## 12. Types and `is`

Every value has a type, organised into a hierarchy. Inspect a
value's type with `typeof`, or walk its ancestry with `pathof`:

```
aql> typeof 42                       # returns Integer
aql> typeof "hello"                  # returns ProperString
aql> typeof [1, 2]                   # returns List
aql> pathof Integer                  # returns [Scalar Number Integer]
```

Use `is` to test membership against any ancestor in the hierarchy:

```
aql> 42 is Integer                   # returns true
aql> 42 is Number                    # returns true
aql> 42 is Scalar                    # returns true
aql> 42 is String                    # returns false
```

Convert with `convert`:

```
aql> convert Integer "42"            # returns 42
aql> convert String 42               # returns '42'
```


## 13. Records and tables

A record is a struct-like type with named typed fields; a table is a
list-of-rows-conforming-to-a-record. Define both with
`def NAME refine Record …` / `def NAME refine Table …`, and use
`make` to instantiate:

```
aql> def Point refine Record [x:Number y:Number]
aql> make Point [3 4]                # returns {x:3 y:4}
aql> make Point {x:1 y:2}            # returns {x:1 y:2}

aql> def Row refine Record [name:String qty:Integer]
aql> def Inventory refine Table Row
aql> make Inventory [["Widget" 5] ["Bolt" 12]]
  # returns [{name:'Widget' qty:5} {name:'Bolt' qty:12}]
```

Field constraints can be disjunctive — `(String tor none)` means
"string or absent":

```
aql> def Person refine Record [name:String nick:(String tor none)]
aql> make Person {name:"Alice" nick:"ace"}     # returns {name:'Alice' nick:'ace'}
aql> make Person {name:"Bob"}                  # returns {name:'Bob' nick:None}
```

The omitted field holds the absence marker, which canonical rendering
shows as capital-`N` `None`. In source you write (and test with) the
lowercase value: `(make Person {name:"Bob"}) . nick eq none` returns
`true`. See **[Reference: Absence — `none` and
`None`](REFERENCE.md#absence--none-and-none)** for the full story.

Types can also be **generic** — declared over type parameters in
angle brackets and instantiated with concrete arguments. Each
instantiation is a real, distinct type:

```
aql> def Box<T> class {value:T}
aql> def b:Box<Integer> {value:42}
aql> typeof b                        # returns Box of [Integer]
aql> b is Box                        # returns true
aql> b is Box<String>                # returns false
```

Generic *functions* use the spelled-out form — `def first gen [T]
fn [[xs:[:T]] [T] [xs get 0]]` works over a list of any element
type. See **[Reference: Generic types](REFERENCE.md#generic-types)**
for bounds (`T extends Number`), defaults, recursion via `Self`,
and inference.


## 14. Scoped variables with `var`

`var` introduces local names that are automatically un-defined at
the end of the block. Bare-word declarations pop from the stack
(top into the first listed name, etc. — matching the argument-order
rule):

```
aql> import "aql:math-util"
aql> 3 4 var [[a b] (a mul a) add (b mul b) MathUtil.sqrt]   # returns 5.0
```

The first element of the list is the binding list. The remaining
elements are the body. `a` here binds to `4` (top of stack), `b` to
`3`. Inline values:

```
aql> var [[[x 2] [y 10]] add x y]               # returns 12
```


## 15. Evaluation with `do` and `quote`

A list literal evaluates its contents by default and keeps the
results *as a list* — `[add 1 2]` becomes `[3]`, not `3`:

```
aql> [add 1 2]                       # returns [3]
```

Use `quote` to hold a list as unevaluated data instead (see below).
`do` runs a list as a sub-program, leaving its results on the stack
rather than in a list:

```
aql> do [add 1 2]                    # returns 3
aql> do {x: [add 3 4], y: 5}        # returns {x:7 y:5}
```

`quote` prevents a single token from being interpreted:

```
aql> quote foo                       # returns foo/q
```


## 16. Macros

`quote` lets you *hold* code as data. A **macro** lets you *transform*
it: a macro runs at expansion time on its arguments **as code** and
splices the result into the call site. Where `fn` receives values, a
macro receives unevaluated forms — so you can build new syntax in AQL
itself.

You write a macro with `macro [[params] [body]]`. The body produces a
template — a `quote [ … ]` list — with `unquote` marking the holes
where the operands go:

```
aql> def twice (macro [[e] [ quote [ unquote e add unquote e ] ]])
aql> twice 5                         # returns 10
```

`twice 5` isn't a function call — it *rewrites itself* into the code
`5 add 5`, which then runs. See exactly what a macro produces with
`macroexpand` (it doesn't run the result):

```
aql> def twice (macro [[e] [ quote [ unquote e add unquote e ] ]])
aql> macroexpand (twice 5)           # returns [5 word(add) 5]
```

(It's a *token list* — `add` shows as `word(add)` because it's an
unevaluated word in the expansion, not a call yet.)

Because a macro sees its arguments as code, it can make new control
forms. Here is an `unless` — `if`, but inverted — that takes its
condition and body unevaluated:

```
aql> def unless (macro [[cond body] [ quote [ if unquote cond [] unquote body ] ]])
aql> unless false [42]               # returns 42
```

`splice` is `unquote`'s sibling: it spreads a *list* operand's elements
into the surrounding code, instead of inserting one node.

**Macros are hygienic.** A name a macro introduces with a literal `def`
is automatically renamed, so it can never clash with one of your
variables — even if the names match:

```
aql> def myor (macro [[a b] [ quote [ def tmp unquote a  if tmp [tmp] [unquote b] ] ]])
aql> def tmp 42
aql> myor false tmp                  # returns 42
```

The template's `tmp` and your `tmp` stay separate; `myor` returns your
`42` untouched. (To bind a name the *caller* should see, pass it
through `unquote` — `def unquote name …`.)

Macros are the deep end — see **[Reference: Macros](REFERENCE.md#macros)**
for the full set (`macro`, `unquote`, `splice`, `gensym`, `macroexpand`).


## 17. Error handling

Errors are values, not exceptions. `do` catches them and the
`error` word pattern-matches:

```
aql> do [1 div 0]
Error(div: division by zero)

aql> do [1 div 0] error [drop 42]    # returns 42
```

The pattern is `do [risky] error [handler]`. Inside the handler the
error value is on the stack — `drop` it and push a recovery value,
or inspect its fields with `.`.


## 18. Concurrency with `await`

`await` (in the `aql:time-util` module) runs a list of code blocks
in parallel and collects the results:

```
aql> import "aql:time-util" TimeUtil.await [[add 1 2] [add 3 4]]     # returns [3 7]
```

Pick a mode via an options map — these mirror JavaScript Promise
combinators:

<!-- aql-test: skip -->
```
aql> await {mode: 'all}   [[sleep 10 1] [sleep 10 2]]
  # returns [1 2] — all must succeed

aql> await {mode: 'first} [[sleep 100 1] [sleep 10 2]]
  # returns 2 — race winner

aql> await {mode: 'any}   [[1 div 0] [sleep 10 42]]
  # returns 42 — first non-error

aql> await {mode: 'full}  [[1] [1 div 0]]
  # returns [{status:'ok,value:1},{status:'error value:...}]
```

Schedule deferred work with `timeout` and `interval`, cancel with
`cancel`:

```
aql> def t timeout 1000 [print "fired"]
aql> t cancel
```


## 19. Reading and writing files

File I/O lives in the `aql:io` module, and every filesystem target is a
`Pathon` micron built with `make Pathon "…"` — a bare string is not
accepted, so a file path is type-distinct from ordinary text.

```
aql> import "aql:io"
aql> IO.read (make Pathon "data.json")
aql> IO.read (make Pathon "data.csv") {fmt: 'csv}
aql> IO.write (make Pathon "out.txt") "hello"
aql> IO.write (make Pathon "out.json") {x:1, y:2}
```

Supported formats: `json`, `csv`, `tsv`, `jsonic`, `text`. By
default the format is inferred from the extension. `IO.read IO.stdin`
and `IO.write IO.stdout "..."` work too (the stream handles are not
Pathons — they are `StreamKind` atoms).

Importing `aql:io` also extends the core `list` and `remove` words with a
Pathon overload, so `list (make Pathon "dir")` enumerates a directory and
`remove (make Pathon "f")` deletes a path — the same bare words that list a
table or remove a record, now polymorphic over a filesystem path.

File access requires the **`fileio`** capability to be enabled.
The CLI enables it by default; embeddings may disable it.


## 20. Modules — namespaces and imports

A *module* is a fresh evaluation context. Define one inline with
the `module` form, calling `export "namespace" {…}` to publish
bindings:

<!-- aql-test: skip -->
```
aql> import module [
       def helper [dup add]
       def greet fn [[name:String] [String] [`hello ${name}`]]
       export "utils" {helper: helper, greet: greet}
     ]
aql> "Ada" utils.greet               # returns 'hello Ada'
```

Import from a file (path must start with `./`, `../`, or `/`):

```
aql> import "./lib/utils.aql"
```

Built-in native modules are imported as quoted `aql:` ids; each one
binds a single capital-initial namespace. The rule of thumb: a
`-util` id binds the matching `NameUtil` namespace; capability and
framework modules keep plain names:

| Import id | Namespace | | Import id | Namespace |
|-----------|-----------|-|-----------|-----------|
| `aql:math-util` | `MathUtil` | | `aql:io` | `IO` |
| `aql:array-util` | `ArrayUtil` | | `aql:net` | `Net` |
| `aql:string-util` | `StringUtil` | | `aql:test` | `Test`, `Assert` |
| `aql:struct-util` | `StructUtil` | | `aql:rand` | `Rand` |
| `aql:time-util` | `TimeUtil` | | `aql:query` | `Query` |
| `aql:type-util` | `TypeUtil` | | `aql:report` | `Report` |
| `aql:matrix-util` | `MatrixUtil` | | `aql:vm` | `Vm` |
| `aql:bin-util` | `BinUtil` | | `aql:logic-util` | `LogicUtil` |

(The full per-module word lists are in
**[Reference: Built-in modules](REFERENCE.md#built-in-modules)**.)

```
aql> import "aql:math-util"
aql> MathUtil.log 5                      # returns 1.6094379124341003
```

The trailing `end` is needed only when the token after `import`
*could* be one of its arguments. Collection is type-directed: here
`5` fits none of `import`'s argument shapes (a path string, a rename
list), so `import` takes its path and stops with no `end`:

```
aql> import "aql:math-util"
aql> 5 MathUtil.log                      # returns 1.6094379124341003
```

But a word (`MathUtil.…`) or a string right after `import` *would* be
collected — `import "aql:math-util" "foo" print` needs the `end`,
or `import` would try to load `"foo"`. Note that in a script file a
line break does **not** stop collection (the REPL evaluates line by
line, a file is one program) — so in files, the robust habit is
`import end`.


## 21. Manage secrets with the vault

`aql` ships a local secrets vault for third-party API keys and tokens.
This walkthrough creates a vault — under `~/.aql` or a folder you
choose — adds keys by typing or pasting them, tags them with optional
expiry reminders, moves the vault to another machine, and injects the
secrets into other programs — all without a secret, a passphrase, or a
key value ever touching your shell history, the process command line,
or an environment variable you set by hand. Every `(hidden)` line below
is a no-echo prompt.

Two passphrases appear: the **vault passphrase** unlocks the local
keyring, and a separate **export passphrase** protects a bundle in
transit. Both are always prompted; the `AQL_VAULT_PASSPHRASE` /
`AQL_VAULT_EXPORT_PASSPHRASE` environment variables exist only for
non-interactive use (services, CI) and are intentionally avoided here.

### Create the vault

The `file` backend stores everything under `~/.aql`, which is what
makes a vault portable. The passphrase is typed, confirmed, and may
not be empty.

```bash
aql vault init --backend=file
#   Set vault passphrase:        (hidden)
#   Confirm passphrase:          (hidden)
```

Prefer a different location? Point the vault at a folder of your choice
with `--folder`, and give its files an inner suffix with `--suffix`
(`vault.work.jsonic`, `vault.work.keyring`, …) so several vaults can sit
side by side in one folder:

```bash
aql vault --folder=./team-vault --suffix=work init --backend=file
#   Set vault passphrase:        (hidden)
#   Confirm passphrase:          (hidden)
#   vault initialized: backend=file store=team-vault/vault.work.jsonic
```

The `--folder`/`--suffix` flags are global, so they go *before* the
mode, and they describe *where the vault is* — pass the same pair to
every command that touches it (or export `AQL_VAULT_FOLDER` /
`AQL_VAULT_SUFFIX` for the session). A flag wins over the matching
environment variable. The rest of this walkthrough uses the default
`~/.aql` vault.

### Add keys — paste or type

Paste a token you just copied from a SaaS console. The value is read
straight from the OS clipboard and the clipboard is wiped afterwards:

```bash
aql vault add --from-clipboard --provider=github github_token
#   Vault passphrase:            (hidden)
#   stored github_token (backend=file, 40 bytes)
#   clipboard cleared (pbpaste/pbcopy)
#   note: a clipboard manager, if you run one, may still hold a copy — clear its history too
```

Clipboard support works on macOS (pbpaste/pbcopy), Linux (wl-clipboard
on Wayland, or xclip / xsel on X11), and Windows (PowerShell). Or type
the value directly — the input is not echoed. Add `--expiry` to record
when the key should be rotated (a date, an RFC3339 timestamp, or a
duration from now like `90d` or `720h`):

```bash
aql vault add --provider=openai --expiry=2026-12-31 openai_key
#   Secret value:                (hidden)
#   Vault passphrase:            (hidden)
#   stored openai_key (backend=file, 51 bytes)
#   expires 2026-12-31T00:00:00Z
```

Namespacing lets two projects share a key name: a `ns:` prefix becomes
part of the stored name. (`aql vault config --set namespace.default=proj`
then makes bare names resolve into `proj` automatically.)

```bash
aql vault add --from-clipboard proj:deploy_key
```

Inspect what's stored — names and metadata only, never values. The
listing includes an `EXPIRES` column (a dash when no expiry is set):

```bash
aql vault list
aql vault get github_token            # redacted; add --reveal to spot-check
```

> The secret is never a command-line argument, so don't write
> `aql vault add github_token 'ghp_...'` — that would leak it to your
> shell history. Use `--from-clipboard`, the prompt, or (for scripts)
> `--from-stdin`.

### Track key expiries

An expiry is a rotation reminder — it is **never enforced**, so an
expired key still works; it just shows up as overdue. Set or replace one
any time without touching the secret, and clear it when the key is
rotated:

```bash
aql vault expiry set github_token 90d     # also accepts a date or RFC3339 stamp
aql vault expiry clear github_token       # drop the reminder
```

Review what's pending with `vault expiry`. It lists only keys that carry
an expiry, soonest (and most overdue) first, with a human status; narrow
it by namespace or to a due-soon window:

```bash
aql vault expiry
#   ALIAS                    NAMESPACE        EXPIRES               STATUS
#   openai_key               (root)           2026-12-31T00:00:00Z  in 203d

aql vault expiry --within=30d             # only keys due within 30 days (or overdue)
aql vault expiry --namespace=proj         # only the proj namespace (':' = root)
```

You can also pin an expiry while rotating a key — `aql vault rotate
--expiry=90d openai_key` — and a rotation without `--expiry` keeps the
existing reminder.

### Export the vault to a portable bundle

`export` re-encrypts the keys and metadata under the separate export
passphrase, independent of the host keystore, into one file:

```bash
aql vault export --out=vault.aqlx
#   Vault passphrase:            (hidden)   # unlock the source vault
#   Set export passphrase:       (hidden)   # protects the bundle in transit
#   Confirm export passphrase:   (hidden)
#   exported 3 secret(s) to vault.aqlx
```

Copy `vault.aqlx` to the other machine by any means (scp, USB, …).

### Import on a different machine

The destination vault can use its own vault passphrase, and even a
different backend (say, the macOS Keychain) — only the export
passphrase has to match what you set above. `import` auto-detects a
bundle versus a `.env` file.

```bash
aql vault init --backend=file
#   Set vault passphrase:        (hidden)
#   Confirm passphrase:          (hidden)

aql vault import vault.aqlx
#   Export passphrase:           (hidden)   # the bundle passphrase from export
#   Vault passphrase:            (hidden)   # this machine's vault passphrase
#   imported github_token
#   imported openai_key
#   imported proj:deploy_key
#   imported 3 secret(s)

aql vault list                            # confirm the keys arrived
```

The destination vault can also live wherever you like. Supply the same
`--folder`/`--suffix` (before the mode) on each command, and they target
that one vault throughout:

```bash
aql vault --folder=/srv/vault --suffix=prod init --backend=file
aql vault --folder=/srv/vault --suffix=prod import vault.aqlx
aql vault --folder=/srv/vault --suffix=prod list
```

Existing aliases are skipped unless you pass `--overwrite`. Once
imported, securely delete the transit file (`shred -u vault.aqlx`).
Expiry reminders are local to a vault and aren't carried in the bundle,
so re-set them on the destination (`vault expiry set …`) if you want
them there too.

### Inject secrets into other commands

`vault exec` resolves the named aliases, runs the command with each
value placed in its **environment block** — never on the command line,
never logged — and propagates the child's exit code.

```bash
# alias `github_token` becomes $github_token in the child process:
aql vault exec github_token -- gh repo list
#   Vault passphrase:            (hidden)

# Remap to the env-var name a tool expects:
aql vault exec github_token=GITHUB_TOKEN -- gh auth status

# Inject several at once; --upper derives UPPERCASE names:
aql vault exec --upper github_token,openai_key -- ./deploy.sh
#   → $GITHUB_TOKEN and $OPENAI_KEY in the child

# A namespaced alias surfaces under its base name ($deploy_key):
aql vault exec proj:deploy_key -- terraform apply

# --clear-env runs in a sanitized environment (keeps only
# PATH/HOME/USER/SHELL/TERM/LANG/LC_ALL/TMPDIR plus the injected keys):
aql vault exec --clear-env openai_key=OPENAI_API_KEY -- ./untrusted-tool
```

To run many commands without retyping the vault passphrase each time,
start the loopback broker (`aql vault proxy`) and hand tools scoped,
expiring capability tokens instead of the secrets themselves — see the
**[CLI Reference](CLI.md#aql-vault)** for the proxy, capabilities,
`vault mv`, and `vault verify`.

### Publish with a recipe

Package publishers each read their token from a different place. A
**recipe** (`--for=<tool>`) injects the secret in exactly the form one
tool expects — no `~/.npmrc` edit, nothing on the command line:

```bash
aql vault exec --for=npm npm_token -- npm publish
```

`--for` is repeatable, and each entry can name its own secret, so one
command can credential several tools at once — publish to npm *and*
push a GitHub release tag, each from its own secret:

```bash
aql vault exec --for=npm=npm_token --for=github=gh_pat -- make publish
```

### Scan for leaked secrets

`vault scan` finds secrets that escaped into files — and, with
`--home`, into the credential dotfiles tools leave lying around:

```bash
aql vault scan .          # secret-like tokens in this directory tree
aql vault scan --home     # plaintext creds in ~/.npmrc, ~/.netrc, ~/.aws/credentials, …
```

It masks every value and exits non-zero when it finds something, so it
drops straight into a pre-commit hook or CI. Move a finding into the
vault, then delete the plaintext.

Prefer a menu to flags? `aql vault -i` opens an interactive TUI over
everything above.


## 22. Build and publish a module

A reusable AQL module is a directory with an `aql.jsonic` manifest. The
build-and-publish path is a short pipeline:

```bash
aql prep                 # parse aql.jsonic → .aql/aql.json
aql pack                 # build a publishable zip under .aql/_pack/
aql register             # one-time: create an account on a registry
aql login                # authenticate; store the token (add --vault to keep it in the vault)
aql publish              # pack + upload the current module
aql clean                # remove .aql/* build artifacts
```

On another machine, pull a published module into a project and import
it:

```bash
aql install mymod-1.0.0  # download into .aql/ and record the dependency
```

The registry URL defaults to a local server; pass `-r <url>` to target
another, and run your own with `aql registry -r ~/registry -p 8080`.


## 23. Where to next

- **[How-To Guides](HOWTO.md)** — practical recipes by task.
- **[Reference](REFERENCE.md)** — every word, every type.
- **[Explanation](EXPLANATION.md)** — the design choices behind AQL.
- **[CLI Reference](CLI.md)** — `aql do`, `aql check`, `aql fmt`,
  `aql serve`, and the rest of the binary.

Common next steps:

* Run `aql help` for an in-REPL word list, then `aql help <word>`
  for a specific signature.
* Try `aql check script.aql` to type-check before running.
* Run `aql fmt script.aql` to canonicalise indentation.
* Build a small module, package it with `aql pack`, and publish it
  with `aql publish`.
* Serve AQL over HTTP or to an editor: `aql registry`, `aql exec`,
  `aql lsp`, or several at once under `aql serve` — see
  [How-To → Run AQL as a service](HOWTO.md#run-aql-as-a-service).

Welcome to AQL.
