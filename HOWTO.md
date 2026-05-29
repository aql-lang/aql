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

Inside the body, named parameters (`x:Number`) bind to argument
slots automatically. The first listed parameter is `args[0]`, the
second is `args[1]`, etc. You can also access the full slot list via
`args`:

```
def show fn [[a:Number b:Number] [String] [`${a} and ${b}`]]
show 1 2                      => '1 and 2'
2 show 1                      => '1 and 2'
2 1 show                      => '1 and 2'
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
for 5 [42]                    => 42 42 42 42 42   # body runs 5 times
```

Transform with higher-order words. Argument order follows the rule
from **[Tutorial §3](TUTORIAL.md#the-argument-order-rule)** — `fold`
takes `body data init` in all-forward form:

```
[1, 2, 3] each [dup mul]      => [1,4,9]
fold [add] [1, 2, 3] 0        => 6              # all-forward
0 [1, 2, 3] [add] fold        => 6              # all-stack, same result
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
```

Index:

```
[10,20,30] at [2,0]           => [30,10]
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

A function stored in a map is callable through the dotted accessor when
you store it with the `/r` ref modifier, which keeps it as a data value:

```
def inc fn [[n:Integer] [Integer] [n add 1]]
def m {inc: inc/r}
m.inc 5                                => 6
```

Stored bare (`{inc: inc}`), the map value is auto-evaluated and `inc` is
invoked with no argument — which fails its signature, so `def m {inc: inc}`
is a build error (bare words never degrade to data). Store it with `/r`,
or call it by resolving the name at call time with bare `m get inc 5`.


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

Use template strings for general value-to-string formatting:

```
`pi = ${3.14159}`                     => 'pi = 3.14159'
`hello ${"world"}`                    => 'hello world'
```

For controlled rounding, import the math module:

```
"aql:math" import end
`${3.14159 100 mul math.round 100 div}` => '3.14'
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
def Point refine Record [x:Number y:Number]
make Point [3 4]                      => {x:3,y:4}
make Point {x:1 y:2}                  => {x:1,y:2}
```


## Define a table type

A table is a list-of-rows-conforming-to-a-record:

```
def Row refine Record [name:String qty:Integer]
def Inventory refine Table Row

make Inventory [["Widget" 5] ["Bolt" 12]]
=> [{name:'Widget',qty:5},{name:'Bolt',qty:12}]
```


## Define an object type with methods

Objects are mutable, inheritable types. Declare one with `refine
Object` and a field map (`name: defaultValue`); construct instances
with `make`. Read fields with the dotted accessor (`.field`) and
mutate them with `set` (note the arg order — `obj value key set` —
see [Tutorial §3](TUTORIAL.md#the-argument-order-rule) for why):

```
def Counter (refine Object {count: 0})

def c (make Counter {})
c 1 "count" set                       # c.count := 1
c 2 "count" set                       # c.count := 2
c.count                               => 2
```

Wrap `make` in `(…)` so `def` binds the *result* to `c` (rather than
binding `c` to the literal word `make`); the same grouping around
`refine` keeps the type expression bound to `Counter`. See
[Argument order](TUTORIAL.md#the-argument-order-rule).

The same parentheses are needed to read a field **straight off a fresh
construct** — dotted access binds tightly to its immediate receiver:

```
def Counter (refine Object {count: 0})
(make Counter {}).count               => 0       # parenthesise the make
make Counter {} .count                => error   # parses as make Counter ({}.count)
```

Binding to `c` first (as above) sidesteps this; otherwise wrap the
construct. See [Reference: Maps and access](REFERENCE.md#maps-and-access).

### Methods are free functions over the instance

AQL objects hold **fields, not methods**: the field map has no method
slot and there is no inline dispatch. Putting a body in the map
(`refine Object {count: 0, inc: [count 1 add]}`) does **not** create a
callable — that just stores a list under the field `inc`, and `c inc`
raises `undefined_word`. Model a method as an ordinary typed `fn`
whose first parameter is the instance, then invoke it in stack form
(`instance method`) or forward form (`method instance`).

A read-only accessor returns a value derived from the instance:

```
def Counter (refine Object {count: 0})
def doubled fn [[c:Counter] [Integer] [c.count 2 mul]]

def c (make Counter {})
c 5 "count" set
c doubled                             => 10
```

A mutator changes the instance in place. `set` returns nothing, so the
mutator's output signature is empty (`[]`); re-push the instance at the
end instead if you want to chain calls:

```
def Counter (refine Object {count: 0})
def bump fn [[c:Counter] [] [c (c.count 1 add) "count" set]]

def c (make Counter {})
c bump
c bump
c.count                               => 2
```

Because methods are just typed functions, they overload, type-check,
and compose like any other word.


## Use scoped variables

`var` binds local names that are auto-cleared after the block.
Bare-word declarations pop from the stack:

```
"aql:math" import end
3 4 var [[a b] (a mul a) add (b mul b) math.sqrt]    => 5.0
```

`a` gets the topmost value (4) and `b` gets the next (3), matching
the argument-order rule. Inline values are also accepted:

```
var [[[x 2] [y 10]] x add y]          => 12
```

Mix the two:

```
10 var [[[x 2] y] x add y]            => 12       # x=2 inline, y=10 from stack
```


## Iterate with `for`

Numeric loop runs the body N times (the body sees an empty stack —
`for` does not push the iteration index):

```
for 5 [42]                    => 42 42 42 42 42
```

Range form (start, stop) and (start, stop, step):

```
for [1, 4] [99]               => 99 99 99
for [0, 10, 2] [99]           => 99 99 99 99 99
```

If you want the index inside the body, use `iota N each [...]`
instead — `each` does pass the element to the body:

```
iota 5 each [dup mul]         => [0,1,4,9,16]
```


## Check types and convert values

```
typeof 42                     => Integer
typeof "hello"                => ProperString
pathof Integer                => [Scalar Number Integer]
42 is Number                  => true
42 is String                  => false
```

Convert with `convert`:

```
convert Integer "42"          => 42
convert String 42             => '42'
convert Decimal 5             => 5.0
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

Define an inline module with the `module` form. The body must call
`export "namespace" {...}` to publish bindings. Export **functions**
with the `/r` ref modifier — the export map auto-evaluates, so a bare
`greet` would be dispatched there (0-arg) rather than exported as the
function. Values and types export bare:

```
import module [
  def base 10
  def greet fn [[name:String] [String] [`hello ${name}`]]
  export "utils" {base: base, greet: greet/r}
]
"Ada" utils.greet                     => 'hello Ada'
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
"aql:math" import end
5 math.log                            => 1.6094379124341003
```

Native module words are reached via the namespace prefix
(`math.log`, `math.ceil`, …). The `end` after `import` prevents
forward collection from grabbing the next token as another path —
without it, `"aql:math" import "foo" print` would try to import a
module named `"foo"`.


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

The native module name is `"aql:time"`; words register under the
`time.` namespace prefix.

```
"aql:time" import end
time.parse "2026-01-15"               # Date value
```

Provides `Date`, `DateTime`, `Instant`, `TimeOfDay`, `Duration`,
`Timezone` types. See the module source for the complete word list.


## Use the built-in `aql:matrix` module

```
"aql:matrix" import end
matrix.make-vector [1, 2, 3]          # Vector(3)
```

Provides `Tensor`, `Matrix`, `Vector` type-kinds and the standard
linear-algebra operations under the `matrix.` namespace.


## Use the built-in `aql:decision` module

```
"aql:decision" import end
```

Decision tables compile to a single dispatch, type-checked against
the input record. See the module source for the full API.


## Store secrets in the vault

The `aql` binary ships with a local key vault:

```bash
aql vault init
aql vault add github_token 'ghp_xxx'
aql vault list
aql vault get github_token
aql vault grant github_token <process-id>
aql vault exec github_token=GITHUB_TOKEN -- gh repo list
```

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
use `end` to stop its collection. For example, `import` is
forward-precedence and will try to consume the next string as a
second module name:

```
"aql:math" import end "foo" print     => 'foo'
```

Without `end`, `import` would attempt to import a module named
`"foo"`. The same idiom is useful any time a forward-precedence
word sits next to a value the next word actually needs.

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
aql do --perms=sandbox 1 add 2                   # 3
aql -e '1 add 2' --perms=read-only               # 3
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
aql do --perms-inline='{ scopes: { engine: { words: { default: "deny", rules: [{ allow: ["add"] }] } } } }' 1 add 2
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
**[lang/doc/design/PERMISSIONS.0.md](lang/doc/design/PERMISSIONS.0.md)**
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
`vm.run`/`vm.run-with` with capability attenuation.
