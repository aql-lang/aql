# AQL Tutorial

This tutorial teaches AQL from the ground up. It is meant to be read
in order, with the REPL open at your side — type every example and
poke at it. By the end you'll be comfortable with the stack model,
the type system, defining typed functions, working with records and
tables, doing concurrent work with `await`, and packaging code as
modules.

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
the string-util module — `aql -e '"aql:string-util" import end
StringUtil.upper "hello"'` — see [§5: Strings](#5-strings).)


## 2. The stack — your first expression

AQL is a *stack machine*. Each token does one of two things:

* a **literal** pushes itself onto the stack,
* a **word** pops arguments off the stack and pushes results.

Try it:

```
aql> 1 2 add
3
```

Step by step: `1` is pushed, `2` is pushed, `add` pops both and
pushes their sum.

Values not consumed are left on the stack:

```
aql> 1 2 3
1 2 3
```

The final stack is the result you see printed.


## 3. Three ways to call a word

Unlike Forth, AQL words can collect arguments from after themselves
as well as from the stack. The same `add` works in three positions:

```
aql> 1 2 add        # all-stack — both args from stack
3
aql> add 1 2        # all-forward — both args after the word
3
aql> 1 add 2        # mixed — one from stack, one forward
3
```

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
aql> 10 3 sub       # all-stack: args[0]=top=3, args[1]=10  →  10 - 3
7
aql> 10 sub 3       # mixed:    args[0]=3 (forward), args[1]=10 (stack)  →  10 - 3
7
aql> sub 3 10       # all-forward: args[0]=3 (first), args[1]=10 (second)  →  10 - 3
7
```

The pattern: `a sub b` always means `a - b`, no matter where `a` and
`b` are written. The three forms above all encode `a=10, b=3`. Note
that `sub 10 3` is *not* the same expression — it encodes `a=3,
b=10`, giving `-7`.

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
aql> "aql:math-util" import end
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
aql> "aql:string-util" import end "hello" StringUtil.upper                 # returns 'HELLO'
aql> "aql:string-util" import end "HELLO" StringUtil.lower                 # returns 'hello'
aql> "aql:string-util" import end StringUtil.split "," "hello,world"       # returns ['hello' 'world'] — subject (input) LAST
aql> "aql:string-util" import end ["a","b","c"] StringUtil.concat          # returns 'abc' — joins list elements
aql> "aql:string-util" import end StringUtil.contains "ell" "hello"        # returns true — haystack LAST
aql> "aql:string-util" import end StringUtil.indexof "ll" "hello"          # returns 2 — haystack LAST: `indexof needle haystack`
aql> "hello" slice 1 3             # returns 'el'
aql> "aql:string-util" import end StringUtil.replace "l" "r" "hello"       # returns 'herlo' — subject (input) LAST
aql> "aql:string-util" import end "  hi  " StringUtil.trim                 # returns 'hi'
aql> "aql:string-util" import end "hi" StringUtil.pad 5                    # returns 'hi   '
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

The condition is coerced to a boolean. Falsey values are `false`,
`0`, `none`, the empty list/map/string, and — watch out — the *exact*
string `"false"`. Every other non-empty string is **true**, so
`"FALSE"`, `"0"`, and `"no"` all take the then-branch. When in doubt,
compare explicitly (`x eq 0`, `s eq ""`) rather than relying on string
truthiness.

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
aql> "aql:array-util" import end
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
aql> "aql:math-util" import end
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
aql> "aql:time-util" import end TimeUtil.await [[add 1 2] [add 3 4]]     # returns [3 7]
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

```
aql> read "data.json"
aql> read "data.csv" {fmt: 'csv}
aql> write "out.txt" "hello"
aql> write "out.json" {x:1, y:2}
```

Supported formats: `json`, `csv`, `tsv`, `jsonic`, `text`. By
default the format is inferred from the extension. `read stdin`
and `write stdout "..."` work too.

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
aql> "./lib/utils.aql" import
```

Built-in native modules are imported as quoted `aql:` ids; each one
binds a single capital-initial namespace. The rule of thumb: a
`-util` id binds the matching `NameUtil` namespace; capability and
framework modules keep plain names:

| Import id | Namespace | | Import id | Namespace |
|-----------|-----------|-|-----------|-----------|
| `aql:math-util` | `MathUtil` | | `aql:io` | `IO` |
| `aql:array-util` | `ArrayUtil` | | `aql:net` | `Net` |
| `aql:string-util` | `StringUtil` | | `aql:decision` | `Decision` |
| `aql:struct-util` | `StructUtil` | | `aql:test` | `Test`, `Assert` |
| `aql:time-util` | `TimeUtil` | | `aql:rand` | `Rand` |
| `aql:type-util` | `TypeUtil` | | `aql:query` | `Query` |
| `aql:matrix-util` | `MatrixUtil` | | `aql:report` | `Report` |
| `aql:bin-util` | `BinUtil` | | `aql:vm` | `Vm` |
| `aql:logic-util` | `LogicUtil` | | | |

(The full per-module word lists are in
**[Reference: Built-in modules](REFERENCE.md#built-in-modules)**.)

```
aql> "aql:math-util" import end
aql> MathUtil.log 5                      # returns 1.6094379124341003
```

The trailing `end` is needed only when the token after `import`
*could* be one of its arguments. Collection is type-directed: here
`5` fits none of `import`'s argument shapes (a path string, a rename
list), so `import` takes its path and stops with no `end`:

```
aql> "aql:math-util" import
aql> 5 MathUtil.log                      # returns 1.6094379124341003
```

But a word (`MathUtil.…`) or a string right after `import` *would* be
collected — `"aql:math-util" import end "foo" print` needs the `end`,
or `import` would try to load `"foo"`. Note that in a script file a
line break does **not** stop collection (the REPL evaluates line by
line, a file is one program) — so in files, the robust habit is
`import end`.


## 21. Manage secrets with the vault

`aql` ships a local secrets vault for third-party API keys and tokens.
This walkthrough creates a vault, adds keys by typing or pasting them,
moves the vault to another machine, and injects the secrets into other
programs — all without a secret, a passphrase, or a key value ever
touching your shell history, the process command line, or an
environment variable you set by hand. Every `(hidden)` line below is a
no-echo prompt.

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
the value directly — the input is not echoed:

```bash
aql vault add --provider=openai openai_key
#   Secret value:                (hidden)
#   Vault passphrase:            (hidden)
```

Namespacing lets two projects share a key name: a `ns:` prefix becomes
part of the stored name. (`aql vault config --set namespace.default=proj`
then makes bare names resolve into `proj` automatically.)

```bash
aql vault add --from-clipboard proj:deploy_key
```

Inspect what's stored — names and metadata only, never values:

```bash
aql vault list
aql vault get github_token            # redacted; add --reveal to spot-check
```

> The secret is never a command-line argument, so don't write
> `aql vault add github_token 'ghp_...'` — that would leak it to your
> shell history. Use `--from-clipboard`, the prompt, or (for scripts)
> `--from-stdin`.

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

Existing aliases are skipped unless you pass `--overwrite`. Once
imported, securely delete the transit file (`shred -u vault.aqlx`).

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


## 22. Where to next

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

Welcome to AQL.
