# AQL How-To Guides

Short, task-oriented recipes. Each entry assumes you've worked
through the **[Tutorial](TUTORIAL.md)** and just need an answer to
"how do I…?" Use the index below to jump straight in.

## Index

* [Define and use custom words](#define-and-use-custom-words)
* [Write a typed function](#write-a-typed-function)
* [Overload a word with multiple signatures](#overload-a-word-with-multiple-signatures)
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
* [Use scoped variables](#use-scoped-variables)
* [Iterate with `for`](#iterate-with-for)
* [Check types and convert values](#check-types-and-convert-values)
* [Type-check before running](#type-check-before-running)
* [Use modules and imports](#use-modules-and-imports)
* [Build, install, and publish a module](#build-install-and-publish-a-module)
* [Use the built-in `aql:time` module](#use-the-built-in-aqltime-module)
* [Use the built-in `aql:matrix` module](#use-the-built-in-aqlmatrix-module)
* [Use the built-in `aql:decision` module](#use-the-built-in-aqldecision-module)
* [Store secrets in the vault](#store-secrets-in-the-vault)
* [Trace and debug](#trace-and-debug)
* [Use `end` to stop forward collection](#use-end-to-stop-forward-collection)


## Define and use custom words

```
def double [dup add]
5 double                      => 10
```

Custom words compose:

```
def quadruple [double double]
5 quadruple                   => 20
```

Re-bind by calling `def` again; `undef` removes the latest binding:

```
def foo 1
def foo 2
foo                           => 2
undef foo
foo                           => 1
```


## Write a typed function

`fn` constructs a typed function from triples
`[input] [output] [body]`. Pair it with `def` to install it:

```
def square fn [[x:Number] [Number] [x mul x]]
5 square                      => 25
```

Inside the body, named parameters (`x:Number`) bind to stack values
automatically. You can also access them as a list via `args`:

```
def show fn [[a:Number b:Number] [String] [`${a} and ${b}`]]
3 4 show                      => '3 and 4'
```


## Overload a word with multiple signatures

Stack the signatures inside one `fn`. The first matching signature
wins:

```
def add1 fn [
  [Integer] [Integer] [1 add]
  [Decimal] [Decimal] [1.0 add]
  [String]  [String]  [`${args.0}_1`]
]
add1 5                        => 6
add1 2.5                      => 3.5
add1 "abc"                    => 'abc_1'
```


## Work with lists

Create, access, build:

```
[10, 20, 30]                  => [10,20,30]
[10, 20, 30] . 1              => 20
iota 5                        => [0,1,2,3,4]
for 5 [dup mul]               => 0 1 4 9 16
```

Transform with higher-order words:

```
[1, 2, 3] each [dup mul]      => [1,4,9]
[1, 2, 3, 4] where [gt 2]     => [3,4]
[1, 2, 3] fold 0 [add]        => 6
[1, 2, 3] scan 0 [add]        => [1,3,6]
```

Reshape, take, drop, reverse:

```
iota 6 reshape [2, 3]         => [[0,1,2],[3,4,5]]
[1,2,3,4] take 2              => [1,2]
[1,2,3,4] shed 2              => [3,4]
[1,2,3] reverse               => [3,2,1]
[3,1,2] grade                 => [1,2,0]      # sort indices
[1,2,2,3] unique              => [1,2,3]
[1,2,3,4] window 2            => [[1,2],[2,3],[3,4]]
[1,2,3] pairs                 => [[1,2],[2,3]]
[1,2,3,4] group [mod 2]       => {0:[2,4],1:[1,3]}
```

Index and select:

```
[10,20,30] at [2,0]           => [30,10]
[10,20,30] member 20          => true
```

Combinators:

```
[1,2] outer [10,20] [mul]              => [[10,20],[20,40]]
[1,2,3] inner [4,5,6] [mul] [add]      => 32       # 1*4 + 2*5 + 3*6
```


## Work with maps

```
{x:1, y:2}                    => {x:1,y:2}
{x:1} . x                     => 1
{users: ["Ada"]} . users . 0  => 'Ada'
```

`do` evaluates list-valued entries inside a map:

```
do {x: [1 add 2], y: [3 mul 4]}        => {x:3,y:12}
```


## Format strings with interpolation

Backtick strings interpolate `${...}` expressions:

```
def name "world"
`hello ${name}`                       => 'hello world'
`2 + 3 = ${2 add 3}`                  => '2 + 3 = 5'
```

Nest as deep as needed:

```
`a${`inner ${1 add 2}`}b`             => 'ainner 3b'
```

Escapes inside templates: `\\`, `` \` ``, `\$`, `\n`, `\t`, `\r`.


## Format numbers and dates

```
"%.2f" format 3.14159                 => '3.14'
"hello %s" format "world"             => 'hello world'
```

For times, use the `aql:time` module — see
[Use the built-in aql:time module](#use-the-built-in-aqltime-module).


## Handle errors

`do` catches errors raised inside its body and leaves the error
value on the stack. `error` pattern-matches the result:

```
do [1 div 0] error [drop 42]          => 42
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
await [[1 add 2] [3 add 4]]           => [3,7]
```

Choose a mode via an Options map; these mirror JavaScript Promise
combinators:

```
# 'all (default): all must succeed; first error fails the lot
await {mode: 'all}   [[sleep 10 1] [sleep 10 2]]
=> [1,2]

# 'full: always returns all results with status
await {mode: 'full}  [[1] [1 div 0]]
=> [{status:'ok,value:1},{status:'error,value:...}]

# 'first: the first to complete wins
await {mode: 'first} [[sleep 100 1] [sleep 10 2]]
=> 2

# 'any: the first non-error result wins
await {mode: 'any}   [[1 div 0] [sleep 10 42]]
=> 42
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

```
def db sqlite-open "data.db"
db sqlite-exec "CREATE TABLE IF NOT EXISTS users (id INTEGER, name TEXT)"
db sqlite-exec "INSERT INTO users VALUES (?, ?)" [1, "Ada"]
db sqlite-query "SELECT * FROM users WHERE id = ?" [1]
=> [{id:1, name:'Ada'}]
db sqlite-close
```

SQLite operations require the **`sqlite`** capability.


## Define a record type

A record is a struct with named, typed fields. Order is significant.

```
type Point record [x:Number y:Number]
make Point [3 4]                      => {x:3,y:4}
make Point {x:1 y:2}                  => {x:1,y:2}
```

Optional fields with `none`:

```
type Person record [name:String nick:[String tor none]]
make Person {name:"Bob"}              => {name:'Bob',nick:none}
make Person {name:"Ada" nick:"ace"}   => {name:'Ada',nick:'ace'}
```

Fill missing fields with their base value:

```
type Item record [name:String qty:Number active:Boolean]
make Item {name:"Widget"} {base:true}
=> {name:'Widget',qty:0,active:false}
```


## Define a table type

A table is a list-of-rows-conforming-to-a-record:

```
type Row record [name:String qty:Integer]
type Inventory table Row

make Inventory [["Widget" 5] ["Bolt" 12]]
=> [{name:'Widget',qty:5},{name:'Bolt',qty:12}]

make Inventory [[name:"Widget" qty:5] [name:"Bolt" qty:12]]
=> [{name:'Widget',qty:5},{name:'Bolt',qty:12}]
```


## Define an object type with methods

Objects are mutable, inheritable types:

```
type Counter object {
  count: 0
  inc:   [count get 1 add count set]
  value: [count get]
}

def c make Counter
c inc
c inc
c value                                => 2
```

Methods see their containing object's fields via the implicit
`Store`. Inherit by passing a parent object type:

```
type FastCounter object Counter {
  inc: [count get 10 add count set]
}
```


## Use scoped variables

`var` binds local names that are auto-cleared after the block:

```
3 4 var [[a b] (a mul a) add (b mul b) sqrt]      => 5.0
```

Names peel off the top of the stack in order: `a` gets the topmost
value, `b` the next. Inline values are also accepted:

```
var [[[x 2] [y 10]] x add y]          => 12
```

Mix the two:

```
10 var [[[x 2] y] x add y]            => 12       # x=2 inline, y=10 from stack
```


## Iterate with `for`

Numeric loop (0..N-1):

```
for 5 [dup mul]               => 0 1 4 9 16
```

Range:

```
for [1, 4] [dup mul]          => 1 4 9
for [0, 10, 2] [dup mul]      => 0 4 16 36 64
```

Early exit and skip:

```
for 10 [dup gt 5 if [break]]
for 10 [dup mod 2 eq 0 if [continue] else [print dup]]
```


## Check types and convert values

```
typeof 42                     => Integer
typeof "hello"                => String
fulltypeof 42                 => Scalar/Number/Integer
42 is Number                  => true
42 is String                  => false
```

Convert with `make` or `convert`:

```
convert Integer "42"          => 42
convert String 42             => '42'
convert Decimal 5             => 5.0
make string 99                => '99'
```


## Type-check before running

`aql check` runs the static type-checker without executing:

```bash
aql check script.aql
aql check -e '1 add "x"'         # reports a type error
```

To both type-check and then run:

```bash
aql -check script.aql
```

Inside the REPL, the same checker is available; see the `:check`
meta-command (`:help` for the full list).


## Use modules and imports

Define an inline module:

```
import utils [
  def helper [dup add]
  def greet fn [[String] [String] [`hello ${args.0}`]]
]
utils.helper 5                => 10
utils.greet "Ada"             => 'hello Ada'
```

Import from a file:

```
import "lib/utils.aql"
```

Rename on import to avoid collisions:

```
import [helper as h, greet as g] "lib/utils.aql"
```

Import a built-in module:

```
import aql:time
aql:time.now
```


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


## Use the built-in `aql:time` module

```
import aql:time

aql:time.now                          => 2026-05-25T12:00:00Z
aql:time.parse "2026-01-15"           => 2026-01-15
aql:time.date 2026 5 25               => 2026-05-25
aql:time.add (aql:time.now) {days: 7}
aql:time.format (aql:time.now) "yyyy-MM-dd"
aql:time.diff t1 t2                   # Duration
```

The full type set: `Date`, `DateTime`, `Instant`, `TimeOfDay`,
`Duration`, `Timezone`. See the `aql:time` source for the complete
word list.


## Use the built-in `aql:matrix` module

```
import aql:matrix

aql:matrix.make-vector [1, 2, 3]              => Vector(3)
aql:matrix.make-matrix [[1, 2], [3, 4]]        => Matrix(2x2)

(aql:matrix.make-vector [1, 2, 3]) aql:matrix.dot
  (aql:matrix.make-vector [4, 5, 6])           => 32
```

Provides `Tensor`, `Matrix`, `Vector` type-kinds and the standard
linear-algebra operations.


## Use the built-in `aql:decision` module

```
import aql:decision

def policy aql:decision.table [
  [if: [age lt 18], then: 'minor]
  [if: [age gte 18], then: 'adult]
]
policy {age: 21}                      => 'adult
```

Decision tables compile to a single dispatch, type-checked against
the input record.


## Store secrets in the vault

The `aql` binary ships with a local key vault:

```bash
aql vault init
aql vault add github_token 'ghp_xxx'
aql vault list
aql vault get github_token
aql vault grant github_token <process-id>
```

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
`stack` and `depth` inspect the current stack:

```
1 2 3 stack                   => [1,2,3]
1 2 3 depth                   => 3
```

For deeper debugging, `inspect` returns a structured view of a word
or value:

```
inspect [{x:1}]
inspect (quote add)
```


## Use `end` to stop forward collection

When a word would otherwise vacuum up more tokens than you want,
use `end` to stop its collection:

```
set foo 99 end get foo                => 99
def y add x end 5                     # bind y to (add x), with 5 left on stack
```

Read more in
**[Explanation §the end keyword](EXPLANATION.md#the-end-keyword)**.
