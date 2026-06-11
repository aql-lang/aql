# AQL How-To Guides

Short, task-oriented recipes. Each entry assumes you've worked
through the **[Tutorial](TUTORIAL.md)** and just need an answer to
"how do I…?" Use the index below to jump straight in.

> **Notation.** In code, a `# returns …` comment shows what an
> expression evaluates to (`5 double  # returns 10`); in prose we say
> "`5 double` returns `10`". The comment is ordinary documentation, not
> special syntax. (AQL has no result arrow — `=>` is the
> anonymous-function word `afn`.)

## Index

* [Define and use custom words](#define-and-use-custom-words)
* [Write a typed function](#write-a-typed-function)
* [Overload a word with multiple signatures](#overload-a-word-with-multiple-signatures)
* [Define a macro (add new syntax)](#define-a-macro-add-new-syntax)
* [Work with lists](#work-with-lists)
* [Work with maps](#work-with-maps)
* [Format strings with interpolation](#format-strings-with-interpolation)
* [Format numbers and dates](#format-numbers-and-dates)
* [Handle errors](#handle-errors)
* [Run code in parallel](#run-code-in-parallel)
* [Use timers and delays](#use-timers-and-delays)
* [Read and write files](#read-and-write-files)
* [Read from stdin and write to stdout](#read-from-stdin-and-write-to-stdout)
* [Make HTTP requests with `fetch`](#make-http-requests-with-fetch)
* [Use a SQLite database](#use-a-sqlite-database)
* [Define a record type](#define-a-record-type)
* [Define a table type](#define-a-table-type)
* [Define an object type with methods](#define-an-object-type-with-methods)
* [Define a generic type](#define-a-generic-type)
* [Use scoped variables](#use-scoped-variables)
* [Iterate with `for`](#iterate-with-for)
* [Check types and convert values](#check-types-and-convert-values)
* [Type-check before running](#type-check-before-running)
* [Use modules and imports](#use-modules-and-imports)
* [Build, install, and publish a module](#build-install-and-publish-a-module)
* [Use the built-in `aql:time-util` module](#use-the-built-in-aqltime-module)
* [Use the built-in `aql:matrix-util` module](#use-the-built-in-aqlmatrix-module)
* [Store secrets in the vault](#store-secrets-in-the-vault)
* [Trace and debug](#trace-and-debug)
* [Use `end` to stop forward collection](#use-end-to-stop-forward-collection)


## Define and use custom words

```
def double word [dup add]
5 double                      # returns 10
```

Custom words compose:

<!-- aql-test: skip -->
```
def quadruple word [double double]
5 quadruple                   # returns 20
```

Re-bind by calling `def` again; `undef` removes the latest binding:

```
def foo 1
def foo 2
foo                           # returns 2
undef foo
foo                           # returns 1
```


## Write a typed function

`fn` constructs a typed function from triples
`[input] [output] [body]`. Pair it with `def` to install it:

```
def square fn [[x:Number] [Number] [mul x x]]
square 5                      # returns 25
```

Inside the body, named parameters (`x:Number`) bind to argument
slots automatically. The first listed parameter is `args[0]`, the
second is `args[1]`, etc. You can also access the full slot list via
`args`:

```
def show fn [[a:Number b:Number] [String] [`${a} and ${b}`]]
show 1 2                      # returns '1 and 2'
2 show 1                      # returns '1 and 2'
2 1 show                      # returns '1 and 2'
```

All three calls compute the same thing: AQL fills argument slots by
walking forward tokens first (left to right) and then taking from
the stack top-first. For details see
**[Tutorial §3](TUTORIAL.md#3-three-ways-to-call-a-word)**.


## Overload a word with multiple signatures

Stack the signatures inside one `fn`. The first matching signature
wins:

```
def add1 fn [
  [Integer] [Integer] [add 1]
  [Float] [Float] [add 1.0]
  [String]  [String]  [`${args.0}_1`]
]
add1 5                        # returns 6
add1 2.5                      # returns 3.5
add1 "abc"                    # returns 'abc_1'
```


## Define a macro (add new syntax)

When you need a new **control form** — something that must see its
arguments as *code*, not as already-computed values — reach for `macro`
instead of `fn`. A macro receives its operands raw, runs its body at
expansion time to build a template, and the template is spliced in place
of the call. Use `quote [ … ]` for the template and `unquote` / `splice`
for the holes:

```
def unless (macro [[cond body] [
  quote [ if unquote cond [] unquote body ]
]])
def x 5
unless (x gt 10) [99]         # returns 99 — body runs only when the condition is false
```

Macros are **hygienic**: a name the template introduces with a literal
`def` is auto-renamed, so it can't clash with a caller's variable — you
don't need to manage temporaries by hand:

```
def myor (macro [[a b] [ quote [ def tmp unquote a  if tmp [tmp] [unquote b] ] ]])
def tmp 42
myor false tmp                # returns 42 — the template's `tmp` is renamed; your `tmp` is safe
```

To bind a name the *caller* should see, take it through `unquote` (`def
unquote name …`). Use `macroexpand (mac args…)` to see what a call
expands to without running it, and `gensym` for unique names in
hand-written cases. See **[Reference: Macros](REFERENCE.md#macros)**.


## Work with lists

Create, access, build:

```
[10, 20, 30]                  # returns [10 20 30]
[10, 20, 30] . 1              # returns 20
iota 5                        # returns [0 1 2 3 4]
range 2 6                     # returns [2 3 4 5] — start/stop
range 0 10 3                  # returns [0 3 6 9] — start/stop/step
for 5 [42]                    # returns 42 42 42 42 42 — body runs 5 times
```

Transform with higher-order words. Argument order follows the rule
from **[Tutorial §3](TUTORIAL.md#the-argument-order-rule)** — `fold`
takes `body data init` in all-forward form:

```
[1, 2, 3] each [dup mul]      # returns [1 4 9]
fold [add] [1, 2, 3] 0        # returns 6 — all-forward
0 [1, 2, 3] [add] fold        # returns 6 — all-stack, same result
```

Take, drop, reverse, flatten (built-in):

```
[1,2,3,4] take 2              # returns [1 2]
[1,2,3,4] shed 2              # returns [3 4]
[1,2,3] reverse               # returns [3 2 1]
[[1,2],[3]] flatten           # returns [1 2 3] — one level
flatten -1 [1,[2,[3]]]        # returns [1 2 3] — fully flatten
```

The richer array vocabulary — reshaping, ordering, grouping,
neighborhoods, indexing — lives in the `aql:array-util` module:

```
"aql:array-util" import end
iota 6 ArrayUtil.reshape [2, 3]   # returns [[0 1 2] [3 4 5]]
[3,1,2] ArrayUtil.grade           # returns [1 2 0] — sort indices
[1,2,2,3] ArrayUtil.unique        # returns [1 2 3]
ArrayUtil.indices [20,99,10] [10,20,30]   # returns [1 -1 0] — index of each needle (-1 = absent)
[1,2,3,4] ArrayUtil.window 2      # returns [[1 2] [2 3] [3 4]]
[1,2,3] ArrayUtil.pairs           # returns [[1 2] [2 3]]
[10,20,30] ArrayUtil.at [2,0]     # returns [30 10]
```


## Work with maps

```
{x:1, y:2}                    # returns {x:1 y:2}
{x:1} . x                     # returns 1
{users: ["Ada"]} . users . 0  # returns 'Ada'
```

A bare name with no `: value` is **field shorthand** — `{foo}` means
`{foo: foo}`, binding the name to itself just like JavaScript:

```
def x 1  def y 2
{x y}                         # returns {x:1 y:2}
{x z:3 y}                     # returns {x:1 y:2 z:3} — mix with explicit pairs
```

The key is the base name and the value is the whole token, so word
modifiers ride along on the value: `{f/r}` ≡ `{f: f/r}` (hold a function
as data) and `{f?}` ≡ `{f?: f}` (an optional field). Only unquoted
identifiers qualify; quoted keys like `{'foo'}` stay errors. See
[Reference: Map field shorthand](REFERENCE.md#map-field-shorthand).

`do` evaluates list-valued entries inside a map:

```
do {x: [add 1 2], y: [mul 3 4]}        # returns {x:3 y:12}
```

A function stored in a map is callable through the dotted accessor when
you store it with the `/r` ref modifier, which keeps it as a data value:

```
def inc fn [[n:Integer] [Integer] [add n 1]]
def m {inc: inc/r}
m.inc 5                                # returns 6
```

Stored bare (`{inc: inc}`), the map value is auto-evaluated and `inc` is
invoked with no argument — which fails its signature, so `def m {inc: inc}`
is a build error (bare words never degrade to data). Store it with `/r`,
or call it by resolving the name at call time with bare `m get inc 5`.


## Format strings with interpolation

Backtick strings interpolate `${...}` expressions:

```
def name "world"
`hello ${name}`                       # returns 'hello world'
`2 + 3 = ${add 2 3}`                  # returns '2 + 3 = 5'
```

Nest as deep as needed:

```
`a${`inner ${add 1 2}`}b`             # returns 'ainner 3b'
```

Escapes inside templates: `\\`, `` \` ``, `\$`, `\n`, `\t`, `\r`.


## Format numbers and dates

Use template strings for general value-to-string formatting:

```
`pi = ${3.14159}`                     # returns 'pi = 3.14159'
`hello ${"world"}`                    # returns 'hello world'
```

For controlled rounding, import the math module:

<!-- aql-test: skip -->
```
"aql:math-util" import end
`${3.14159 100 mul MathUtil.round 100 div}`  # returns '3.14'
```

For times, use the `aql:time-util` module — see
[Use the built-in aql:time-util module](#use-the-built-in-aqltime-module).


## Handle errors

`do` catches errors raised inside its body and leaves the error
value on the stack. `error` pattern-matches the result:

```
do [1 div 0] error [drop 42]          # returns 42
```

Pattern: `do [risky] error [handler]`. Inside the handler the error
value is on the stack — inspect it with `.` or `drop` it and supply
a default:

```
do [read "missing.json"] error [
  drop
  print "file missing, using default"
  {x:0, y:0}
]
```

If there is no error, the `error` word is a no-op.


## Run code in parallel

`await` runs a list of code blocks in their own goroutines and
gathers the results:

```
"aql:time-util" import end TimeUtil.await [[add 1 2] [add 3 4]]           # returns [3 7]
```

Choose a mode via an Options map; these mirror JavaScript Promise
combinators:

<!-- aql-test: skip -->
```
# 'all (default): all must succeed; first error fails the lot
await {mode: 'all}   [[sleep 10 1] [sleep 10 2]]
  # returns [1 2]

# 'full: always returns all results with status
await {mode: 'full}  [[1] [1 div 0]]
  # returns [{status:'ok,value:1},{status:'error value:...}]

# 'first: the first to complete wins
await {mode: 'first} [[sleep 100 1] [sleep 10 2]]
  # returns 2

# 'any: the first non-error result wins
await {mode: 'any}   [[1 div 0] [sleep 10 42]]
  # returns 42
```

Each branch runs in a sub-engine, so writes to mutable objects
inside one branch do not bleed into the others.


## Use timers and delays

Pause the current branch:

```
sleep 100                              # 100ms
```

Schedule a single deferred callback:

```
def t timeout 1000 [print "fired"]
t cancel                               # cancel before it fires
```

Schedule a repeating callback:

```
def i interval 500 [print "tick"]
i cancel                               # stop the loop
```


## Read and write files

```
read "data.json"                       # auto-detects JSON
read "data.csv" {fmt: 'csv}            # explicit format
read "data.csv" {fmt: 'csv, header: true}

write "out.txt" "hello"
write "out.json" {x:1}
write "out.tsv" [[1,2],[3,4]] {fmt: 'tsv}
```

Supported formats: `json`, `csv`, `tsv`, `jsonic`, `text`.


## Read from stdin and write to stdout

Special paths `stdin`, `stdout`, `stderr` work with `read` and
`write`:

```
read stdin                             # read once until EOF
write stdout "hello\n"
write stderr "error\n"
```


## Make HTTP requests with `fetch`

```
fetch "https://api.example.com/v1/things"
fetch "https://api.example.com/v1/things" {method: 'post, body: {x:1}}
fetch "https://api.example.com/v1/things" {
  method: 'get,
  headers: {Authorization: "Bearer ..."}
}
```

The result is a `Response` value with `.status`, `.body`, `.headers`
fields. `fetch` requires the **`fetch`** capability.


## Use a SQLite database

<!-- aql-test: skip -->
```
def db sqlite-open "data.db"
db sqlite-exec "CREATE TABLE IF NOT EXISTS users (id INTEGER, name TEXT)"
db sqlite-exec "INSERT INTO users VALUES (?, ?)" [1, "Ada"]
db sqlite-query "SELECT * FROM users WHERE id = ?" [1]
  # returns [{id:1 name:'Ada'}]
db sqlite-close
```

SQLite operations require the **`sqlite`** capability.


## Define a record type

A record is a struct with named, typed fields. Order is significant.

```
def Point refine Record [x:Number y:Number]
make Point [3 4]                      # returns {x:3 y:4}
make Point {x:1 y:2}                  # returns {x:1 y:2}
```


## Define a table type

A table is a list-of-rows-conforming-to-a-record:

```
def Row refine Record [name:String qty:Integer]
def Inventory refine Table Row

make Inventory [["Widget" 5] ["Bolt" 12]]
  # returns [{name:'Widget' qty:5} {name:'Bolt' qty:12}]
```


## Define an object type with methods

Objects are mutable, inheritable types. Declare one with `refine
Object` and a field map (`name: defaultValue`); construct instances
with `make`. Read fields with the dotted accessor (`.field`) and
mutate them with `set` (note the arg order — `obj value key set` —
see [Tutorial §3](TUTORIAL.md#the-argument-order-rule) for why):

<!-- aql-test: skip -->
```
def Counter (class {count: 0})

def c (make Counter {})
c 1 "count" set                       # c.count := 1
c 2 "count" set                       # c.count := 2
c.count                               # returns 2
```

Wrap `make` in `(…)` so `def` binds the *result* to `c` (rather than
binding `c` to the literal word `make`); the same grouping around
`refine` keeps the type expression bound to `Counter`. See
[Argument order](TUTORIAL.md#the-argument-order-rule).

The same parentheses are needed to read a field **straight off a fresh
construct** — dotted access binds tightly to its immediate receiver:

```
def Counter (class {count: 0})
(make Counter {}).count               # returns 0 — parenthesise the make
make Counter {} .count                # returns error — parses as make Counter ({}.count)
```

Binding to `c` first (as above) sidesteps this; otherwise wrap the
construct. See [Reference: Maps and access](REFERENCE.md#maps-and-access).

### Nested object fields need an explicit constructor default

A field whose type is another object type must default to a **constructed
instance**, not the bare type literal. Writing `field: NestedType` stores
the *type* `NestedType` as the field value — `inst.field` is then the type
literal, not a usable instance. Construct the default with `(make
NestedType {})`:

<!-- aql-test: skip -->
```
def Bits (class {})
def Foo  (class {bits: (make Bits {})})   # construct the default

def inst (make Foo {})
inst.bits 1 "0" set                               # mutate the nested object
inst.bits                             # returns Object/Bits{0:1}
```

The `field: NestedType` form looks reasonable (it reads like a type
declaration) but only names the type; `inst.bits` is then the bare type
literal (`object<Object/Bits>{}`), not a usable instance. Reach for
`(make NestedType {})` whenever a field should default to a fresh nested
object.

### Methods are free functions over the instance

AQL objects hold **fields, not methods**: the field map has no method
slot and there is no inline dispatch. Putting a body in the map
(`class {count: 0, inc: [count 1 add]}`) does **not** create a
callable — that just stores a list under the field `inc`, and `c inc`
raises `undefined_word`. Model a method as an ordinary typed `fn`
whose first parameter is the instance, then invoke it in stack form
(`instance method`) or forward form (`method instance`).

A read-only accessor returns a value derived from the instance:

<!-- aql-test: skip -->
```
def Counter (class {count: 0})
def doubled fn [[c:Counter] [Integer] [c.count 2 mul]]

def c (make Counter {})
c 5 "count" set
c doubled                             # returns 10
```

A mutator changes the instance in place. `set` returns nothing, so the
mutator's output signature is empty (`[]`); re-push the instance at the
end instead if you want to chain calls:

<!-- aql-test: skip -->
```
def Counter (class {count: 0})
def bump fn [[c:Counter] [] [c (c.count 1 add) "count" set]]

def c (make Counter {})
c bump
c bump
c.count                               # returns 2
```

Because methods are just typed functions, they overload, type-check,
and compose like any other word.


## Define a generic type

Put type parameters in angle brackets after the name; instantiate
by filling them in. Each instantiation is a distinct nominal type
with the full class contract (strict fields, sealing, `deq`):

```
def Box<T> class {value:T}

def bi (make Box<Integer> {value:42})
typeof bi                             # returns Box of [Integer]
bi is Box                             # returns true
bi is Box<String>                     # returns false
```

Bound a parameter with `extends` (any type works as a bound —
including predicates and surfaces) and default one with `=`:

```
def Sorted<T extends Number> class {items:[:T]}
def Result<T, E = Error> refine Record [ok:T err:E]
(Result of [Integer])                 # returns record{ok:Integer err:Error}
```

A bare schema as a construction target infers its arguments from
the body (`make Box {value:42}` is a `Box of [Integer]`), and a
generic **function** uses the spelled-out `gen` form, binding its
parameters from each call's arguments:

```
def Box<T> class {value:T}
def boxit gen [T] fn [[x:T] [Any] [make (Box of [T]) {value:x}]]
typeof (boxit "hi")                   # returns Box of [ProperString]
```

For recursion use `Self of [...]` (the schema's own name is unbound
while its body builds). See **[Reference: Generic
types](REFERENCE.md#generic-types)**.


## Use scoped variables

`var` binds local names that are auto-cleared after the block.
Bare-word declarations pop from the stack:

```
"aql:math-util" import end
3 4 var [[a b] (a mul a) add (b mul b) MathUtil.sqrt]    # returns 5.0
```

`a` gets the topmost value (4) and `b` gets the next (3), matching
the argument-order rule. Inline values are also accepted:

```
var [[[x 2] [y 10]] add x y]          # returns 12
```

Mix the two:

```
10 var [[[x 2] y] add x y]            # returns 12 — x=2 inline, y=10 from stack
```


## Iterate with `for`

Numeric loop runs the body N times (the body sees an empty stack —
`for` does not push the iteration index):

```
for 5 [42]                    # returns 42 42 42 42 42
```

Range form (start, stop) and (start, stop, step):

```
for [1, 4] [99]               # returns 99 99 99
for [0, 10, 2] [99]           # returns 99 99 99 99 99
```

If you want the index inside the body, use `iota N each [...]`
instead — `each` does pass the element to the body:

```
iota 5 each [dup mul]         # returns [0 1 4 9 16]
```

For a **side-effecting loop that collects nothing** — mutating an object,
accumulating into state — use `for-each`. Unlike `each`, its body may leave
the stack empty (no throwaway sentinel needed), and it produces no result:

<!-- aql-test: skip -->
```
def Box (class {sum: 0})
def b (make Box {})
[1 2 3] for-each [var [[x] b (b.sum x add) "sum" set]]
b.sum                         # returns 6
```


## Check types and convert values

```
typeof 42                     # returns Integer
typeof "hello"                # returns ProperString
pathof Integer                # returns [Scalar Number Integer]
42 is Number                  # returns true
42 is String                  # returns false
```

Convert with `convert`:

```
convert Integer "42"          # returns 42
convert String 42             # returns '42'
convert Float 5             # returns 5.0
```


## Type-check before running

`aql check` runs the static type-checker without executing:

```bash
aql check script.aql
aql check -e '1 add "x"'         # reports a type error
```

It also raises **advisories** (info level, non-gating) for likely
mistakes that still run. For example, the forward-greediness gotcha —
`1 2 add 3 mul` returns `5`, not `9`, because `add` reaches forward for
`3` and strands the `1`:

```bash
aql check -e '1 2 add 3 mul'
# check: [info] forward_strands_operand: add collected a forward argument
#   while a Number operand was left unconsumed on the stack — it may be
#   stranded; group the intended operands, e.g. (… add …)
```

Group the operands you mean to combine — `(1 2 add) 3 mul` returns `9`
and the advisory clears. Advisories never fail the check (only errors do).

The checker also catches the one silent failure mode the runtime still
permits: a **namespace word whose arguments match no signature**. At
runtime the function value is left on the stack as data (it might be a
deliberate higher-order use); `aql check` flags it as an **error**:

```bash
aql check script.aql
# check: [error] uncalled_function: call to 'my-get' matched no
#   signature and was left on the stack as data (arguments: Map, String)
```

This is almost always an argument-order or arity bug. Because it is
silent at runtime, **run `aql check` as a matter of course** — in CI,
and before committing — the same way you'd run a linter.

To both type-check and then run:

```bash
aql -check script.aql
```

Inside the REPL, the same checker is available; see the `:check`
meta-command (`:help` for the full list).


## Use modules and imports

Define an inline module with the `module` form. The body must call
`export "namespace" {...}` to publish bindings. Export **functions**
with the `/r` ref modifier — the export map auto-evaluates, so a bare
`greet` would be dispatched there (0-arg) rather than exported as the
function. Values and types export bare:

<!-- aql-test: skip -->
```
import module [
  def base 10
  def greet fn [[name:String] [String] [`hello ${name}`]]
  export "utils" {base: base, greet: greet/r}
]
"Ada" utils.greet                     # returns 'hello Ada'
```

Here `base` (a value) exports bare, while `greet` (a function) exports
with `/r`.

Import from a file (relative paths must start with `./`, `../`, or
`/`):

```
"./lib/utils.aql" import
```

Import a built-in native module (registers words under a namespace
prefix):

```
"aql:math-util" import end
5 MathUtil.log                            # returns 1.6094379124341003
```

Native module words are reached via the namespace prefix
(`MathUtil.log`, `MathUtil.ceil`, …). The trailing `end` after `import`
is **only needed when the next token could itself be a module path** —
without it, `"aql:math-util" import "foo" print` would try to import a
module named `"foo"`. You do **not** need `end` before ordinary use of
the namespace; `import` takes its path and stops, leaving the rest to run:

```
import "aql:math-util"
5 MathUtil.log                            # returns 1.6094379124341003
(MathUtil.ceil 4.2)                       # the paren runs on its own → 5
```

The string-hash and char-code words live in `aql:bin`, handy for
building bloom filters and other sketches:

```
"aql:bin-util" import end
"A" BinUtil.ord                           # returns 65
65 BinUtil.chr                            # returns 'A'
"hello" BinUtil.fnv32                     # returns 1335831723 — 32-bit FNV-1a
"hello" BinUtil.fnv64                     # 64-bit FNV-1a, non-negative
```

### One file, two modes: `export` at the top level

`export "Name" {...}` collects a namespace when a file is **imported**.
At the **top level** — running the file directly (`aql foo.aql`) or in
the REPL — `export` is a no-op that simply discards its arguments. So a
single file can both run standalone and export when imported: put the
`export` last, after whatever standalone code the file runs.


## Build, install, and publish a module

A module on disk is a directory with an `aql.jsonic` manifest plus
`.aql` source files. Workflow:

```bash
aql prep                     # parse aql.jsonic, write .aql/aql.json
aql pack                     # build a publishable .zip
aql register                 # register an account on a registry
aql login                    # log in
aql publish                  # upload the current module
aql install acme/widgets-1.2.3
aql clean                    # delete .aql/* except dotfiles
```

By default operations target the public registry; override with
`-r <url>`. See [CLI Reference](CLI.md) for full flags.


## Use the built-in `aql:time-util` module

The native module name is `"aql:time-util"`; words register under the
`time.` namespace prefix.

```
"aql:time-util" import end
TimeUtil.parse "2026-01-15"               # Date value
```

Provides `Date`, `DateTime`, `Instant`, `TimeOfDay`, `Duration`,
`Timezone` types. See the module source for the complete word list.


## Use the built-in `aql:matrix-util` module

```
"aql:matrix-util" import end
MatrixUtil.make-vector [1, 2, 3]          # Vector(3)
```

Provides `Tensor`, `Matrix`, `Vector` type-kinds and the standard
linear-algebra operations under the `matrix.` namespace.


## Write property tests

The `aql:test` module includes a property-based tester. A *property* is a
predicate that should hold for every generated input; the framework draws
random inputs, runs the property against each, and reports the first
failing case.

<!-- aql-test: skip -->
```
"aql:test" import end

# Test.check-prop  name  [gen]  [property]  runs  seed  max-shrinks
Test.check-prop "non-negative"
  [r.int 0 100]                       # gen: leave ONE value on the stack
  [0 gte]                             # property: takes it, leaves a Boolean
  50 1 0
end
```

The two bodies follow a fixed convention:

- **gen body** must leave **exactly one** value on the stack — the
  generated input. It may use the per-iteration random source bound as
  `r`: `r.int lo hi`, `r.float lo hi`, `r.bool`, `r.string alphabet len`,
  `r.one-of [a b c]`.
- **property body** receives that value on the stack (and via `args.0`)
  and must leave a **Boolean**. `false` — or an error — in any iteration
  is a failure.

For a **compound input** (e.g. a pair of strings), don't leave two values
on the stack — the gen body must produce one. Pack them with `r.list-of`
(a fixed-length list) or `r.map-from` (a map), then destructure in the
property body:

<!-- aql-test: skip -->
```
"aql:test" import end
Test.check-prop "pair-of-strings"
  [r.list-of [r.string "abc" 6] 2]    # ONE List of two strings
  [var [[pair]
    def a (pair.0)                    # destructure the compound input
    def b (pair.1)
    (pair size) 2 eq                  # property over the pair
  ]]
  20 1 0
end
```

Report results with `Test.report` (one readable line per property)
rather than the verbose `Test.results` table, and park a
work-in-progress property with `Test.skip` (a drop-in for
`Test.check-prop` that records it as skipped without running it):

<!-- aql-test: skip -->
```
"aql:test" import end
Test.check-prop "ready"   [r.int 0 9] [0 gte] 10 1 0 end
Test.skip       "flaky"   [r.int 0 9] [false] 10 1 0 end   # parked
Test.report end print
#   pass: ready
#   skip: flaky
# 1 passed, 0 failed, 1 skipped
Test.fail-count end print             # returns 0
```

A property body may `import` a native module (e.g. `"aql:math-util" import`)
and use it across every run.


## Write unit tests and declarative specs

For example-based tests, `aql:test` provides `Test.test` (run a body,
catch assertion failures) and the `Assert` namespace. A failing case is
reported **loudly and by name**, and later cases still run:

<!-- aql-test: skip -->
```
"aql:test" import end
[ 1 1 Assert.equal ] "identity holds" Test.test end
[ 1 2 Assert.equal ] "this one fails" Test.test end
# FAIL this one fails — [aql/assertion_failure]: Assert.equal: expected 2, got 1
0 Test.fail-count end Assert.equal end       # gate: no failures allowed
```

For table-driven suites, the **declarative spec form** separates the
cases (data) from the runner. A spec names a subject (the word under
test, passed as `subject/q`) and a list of `{name in out}` cases;
`Test.run-spec` invokes the subject on each case's `in` list and
checks the result against `out`:

<!-- aql-test: skip -->
```
"aql:test" import end
def double fn [[n:Integer] [Integer] [n 2 mul]] end

def s (Test.spec [
  {name: "d3", in: [3], out: 6}
  {name: "d0", in: [0], out: 0}
] double/q "doubling") end

s Test.run-spec end
Test.summary end print                # returns {total:2 passed:2 failed:0}
```

`Test.spec-with-subs` nests sub-specs for grouped suites, and
`Test.describe "group" [ … ]` prefixes every failure inside the body
with the group path. Prefer the spec form whenever the cases are
naturally a table — the data reads at a glance and new cases are
one-line edits.


## Store secrets in the vault

The `aql` binary ships with a local key vault:

```bash
aql vault init

# Paste a token you copied from a SaaS console: the value is read
# from the OS clipboard and the clipboard is wiped immediately after.
aql vault add --from-clipboard --provider=github github_token

# ...or, in scripts/CI, pipe it in without it touching shell history:
printf %s "$TOKEN" | aql vault add --from-stdin --provider=github github_token

aql vault list
aql vault get github_token                       # redacted
aql vault grant --agent=ci --ttl=2h github_token # scoped capability
aql vault exec github_token=GITHUB_TOKEN -- gh repo list

# Track when a key needs rotating: attach an optional expiry, then
# review what is coming due (or overdue).
aql vault add --expiry=90d --from-stdin --provider=github github_token
aql vault expiry                                 # pending expiries, soonest first
aql vault expiry --within=14d                    # only what's due soon
```

The secret is never passed as a command-line argument (that would
leak it into your shell history and the process listing). Use
`--from-clipboard`, `--from-stdin`, `--from-env=VAR`, or the
interactive no-echo prompt you get when you pass none of them.
`--from-clipboard` works on macOS, Linux (Wayland or X11), and
Windows; if you run a clipboard manager, clear its history too,
since it may keep a copy the wipe cannot reach.

Aliases can be namespaced — `proj1:github_token` and
`proj2:github_token` are different keys. Set a default with
`aql vault config --set namespace.default=proj1` and bare names
resolve into it (`vault add key` stores `proj1:key`; `vault get key`
reads it back); `:key` forces the root namespace. Filter reports with
`vault list --namespace=proj1` or `vault audit --namespace=proj1`
(`:` = root only). Reorganise with `vault mv`: `vault mv key proj1:key`
moves one key, `vault mv proj1: team:` renames a whole namespace —
capabilities follow the key, values are copy-verified before the old
entry is removed, and `--dry-run` previews.

Keys can carry an optional expiry so you remember to rotate before the
upstream credential lapses. Attach one when adding (`--expiry`), when
rotating (`rotate --expiry`), or later (`vault expiry set key 2026-12-31`),
and remove it with `vault expiry clear key`. The value is a date
(`2026-12-31`), an RFC3339 timestamp, or a duration from now with day
support (`90d`, `720h`). It is a reminder only — never enforced, so an
expired alias still resolves. `vault list` shows an `EXPIRES` column,
and `vault expiry` lists the keys that have one, soonest (and most
overdue) first, filterable with `--namespace` and `--within`.

By default the vault sits in `~/.aql`; point it elsewhere with
`--folder`/`AQL_VAULT_FOLDER`, and give the files an inner suffix
(`vault.work.jsonic`) with `--suffix`/`AQL_VAULT_SUFFIX` so a project or
team vault can live beside the default one. The flags go before the
mode (`aql vault --folder=./vault --suffix=team init`); supply the same
folder and suffix on every command for that vault, or export the env
vars for the session.

Passphrases work the same way: `vault init` and every command that
opens the file keyring prompt for the vault passphrase with echo
suppressed, and `vault export`/`import` prompt for the bundle
passphrase — just run the command and type when asked. The
environment variables `AQL_VAULT_PASSPHRASE` and
`AQL_VAULT_EXPORT_PASSPHRASE` exist for contexts that cannot prompt:
services (`vault proxy`, `vault mcp`), CI, and pipelines where stdin
already carries the secret (`--from-stdin`, bundle import from
stdin). Avoid `export`-ing them in an interactive shell — that puts
the passphrase in your shell history and into the environment of
every child process; set them per-invocation from a secrets source
instead.

`aql vault exec <alias[=ENV][,...]> -- <cmd> [args...]` runs an
external command with vault secrets injected as environment
variables — the secret never appears on the command line. Use
`--upper` to uppercase derived names, `--prefix=PFX` to prepend a
fixed prefix, or `--clear-env` for a sanitized environment.

Inside AQL, secrets are surfaced via the `vault` capability:

```
vault get "github_token"
```

The backend is OS-specific (macOS Keychain, Linux Secret Service,
Windows Credential Manager, 1Password, or a file fallback).


## Trace and debug

`trace` evaluates a list and records each step:

```
trace [1 add 2 mul 3]
```

In the REPL, `:trace on` toggles tracing for every expression.
`depth` reports the current stack size:

```
1 2 3 depth                   # returns 1 2 3 3 — depth pushes the count; the values stay
```

For deeper debugging, `inspect` returns a structured view of a word
or value:

```
inspect [{x:1}]
inspect (quote add)
```


## Use `end` to stop forward collection

A word only forward-collects a token it could actually take as an
argument — a paren or an incompatible value is left to run on its own
(`import "aql:string-util"` followed by `(StringUtil.upper "hi")` works
with no terminator). You reach for `end` in the remaining case: when the
next token *would* be a valid argument but you mean it for the next word.
The classic example is two adjacent module paths:

<!-- aql-test: skip -->
```
"aql:math-util" import end "foo" print     # returns 'foo'
```

Without `end`, `import` would treat `"foo"` as a second module path and
try to load it. The same idiom helps any time a forward-collecting word
sits next to a value of a type it would accept.

`;` is a synonym for `end`, handy between statements:

```
add 1 2 ; add 3 4                          # returns 3 7 — two statements
```

Read more in
**[Explanation §the end keyword](EXPLANATION.md#the-end-keyword)**.


## Sandbox untrusted code

AQL has an opt-in permissions model that can restrict what a program
is allowed to do — useful for running submitted code (`aql exec`),
embedded scripts, or untrusted plugins. By default there are no
restrictions; permissions activate only when you pass a `--perms`
flag or `Options.Policy` to `lang.New`.

### Built-in profiles

```bash
aql policy list
```

Common profiles, most-permissive first:

| Profile | What it permits |
|---|---|
| `full` | Everything (default; equivalent to no policy). |
| `trusted` | Everything; same as `full` but explicit. |
| `client` | Read disk, outbound network (host-configured); no writes. |
| `read-only` | Read disk + read safe env vars; no writes, no network. |
| `sandbox` | Engine + math/time modules; no disk write, no network, no process. |
| `compute` | Pure computation; no I/O capabilities installed at all. |

### Run code under a profile

```bash
aql do --perms=sandbox add 1 2                   # 3
aql -e 'add 1 2' --perms=read-only               # 3
aql script.aql --perms-file=./prod-policy.jsonic
aql exec -p 8091 --perms=sandbox                 # bound at startup
```

### Incremental overrides

`--allow` / `--deny` accumulate rules on top of the base profile.
`--no-install` removes a capability slot entirely (the wrapped
FileOps / SQLite / etc. is never constructed).

```bash
aql do --perms=sandbox --allow=engine.shell true
aql exec --perms=trusted --no-install=network --no-install=sqlite
aql do --perms-inline='{ scopes: { engine: { words: { default: "deny", rules: [{ allow: ["add"] }] } } } }' add 1 2
```

For where-bearing rules (paths, hosts), use `--perms-inline` or a
`--perms-file`:

```bash
aql exec --perms-inline='{
  scopes: {
    fileops: { words: {
      default: "deny"
      rules: [{ allow: ["read"], where: { path: ["/srv/data/**"] } }]
    } }
  }
}'
```

### Debug a denial

`aql policy explain` prints why a check would be allowed or denied:

```bash
aql policy explain sandbox fileops.write path=/etc/passwd
# profile:  sandbox
# scope:    fileops
# op:       write
# decision: DENY
# blame:    global.disk.write (rule #1)
```

### Author a custom profile

Profiles are jsonic documents. Drop one in
`~/.config/aql/policies/<name>.jsonic` to make it loadable by short
name. See `aql policy show sandbox --json` for a starting point and
**[design/PERMISSIONS.10.md](design/PERMISSIONS.10.md)**
for the full schema.

### What the model gates

- **`engine` scope** — kernel words (add, dup, def, …).
- **`modules` scope** — which `aql:*` modules can be imported and
  which exports are callable.
- **Capability scopes** — `fileops`, `network`, `sqlite`, `formats`,
  `env`, `process`, `clock`. Each can be uninstalled with
  `install: false` or have its operations gated by rules.
- **`global` scope** — hard caps like `disk.write`, `network`,
  `process`. A global denial overrides any allow rule below it.

The HTTP `aql exec` service deliberately does NOT accept policy in
the request body — the policy is bound at server startup. Run
multiple `aql exec` instances on different ports for different
policies.

For running AQL-from-AQL with stricter permissions (test harnesses,
plugin sandboxes), the `aql:vm` native module exposes
`Vm.run`/`Vm.run-with` with capability attenuation.
