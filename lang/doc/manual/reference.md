# AQL Reference

Information-oriented reference for AQL syntax, types, and the
built-in word library. For learning AQL, start with the
**[Tutorial](tutorial.md)**. For task-oriented recipes, see the
**[How-To Guides](how-to.md)**. For *why* AQL is shaped the way it
is, see the **[Explanation](explanation.md)**.

## Contents

* [Syntax](#syntax)
* [Evaluation model](#evaluation-model)
* [Type system](#type-system)
* [Word reference](#word-reference)
  * [Stack manipulation](#stack-manipulation)
  * [Arithmetic](#arithmetic)
  * [Rounding](#rounding)
  * [Roots, exponentials, logarithms](#roots-exponentials-logarithms)
  * [Trigonometry](#trigonometry)
  * [Constants](#constants)
  * [Strings](#strings)
  * [Boolean](#boolean)
  * [Comparison](#comparison)
  * [Definition and scoping](#definition-and-scoping)
  * [Control flow](#control-flow)
  * [List and array words](#list-and-array-words)
  * [Higher-order array words](#higher-order-array-words)
  * [Maps and access](#maps-and-access)
  * [Type words](#type-words)
  * [Inspection](#inspection)
  * [I/O](#io)
  * [Networking — `fetch`](#networking--fetch)
  * [SQLite](#sqlite)
  * [Modules](#modules)
  * [Concurrency](#concurrency)
  * [Unification](#unification)
  * [Help](#help)
* [Built-in modules](#built-in-modules)
* [Error codes](#error-codes)
* [Capabilities](#capabilities)


## Syntax

### Literals

| Syntax | Type | Example |
|--------|------|---------|
| Digits with optional `-` | `Integer` | `42`, `-5`, `0` |
| Digits with `.` | `Decimal` | `3.14`, `-0.5` |
| Double or single quotes | `String` | `"hello"`, `'world'` |
| Backticks with `${...}` | `String` (template) | `` `x = ${x}` `` |
| `true`, `false` | `Boolean` | `true` |
| `none` | `None` | `none` |
| Bare unquoted word | atom, only inside a `/q`-quoted slot | `foo` |
| `quote foo` | `Atom` | `foo` |

Type literals: `number`, `integer`, `decimal`, `string`, `boolean`,
`atom`, `scalar`, `any`, `none`, `list`, `map`, plus every named
type from `type`.

### Compound data

| Syntax | Meaning |
|--------|---------|
| `[a, b, c]` | List literal |
| `[:Type]` | Typed list (every element must match `Type`) |
| `{k:v, ...}` | Map literal |
| `{:Type}` | Typed map (every value must match `Type`) |

Commas are optional inside list and map literals — `[1 2 3]` and
`[1, 2, 3]` are equivalent.

### Comments

| Syntax | Scope |
|--------|-------|
| `# text` | Line — to end of line |
| `## text ##` | Block — delimited |

### Grouping

`(expr)` evaluates a sub-expression eagerly, regardless of forward
collection:

```
2 mul (3 add 4)               => 14
```

### Template-string escapes

`\\`, `` \` ``, `\$`, `\n`, `\t`, `\r`. Use `\$` for a literal `${`.

### Word modifiers

A trailing `/...` suffix overrides a word's default argument shape:

| Modifier | Meaning |
|----------|---------|
| `/s` | Stack-only — never forward-collect |
| `/f` | Forward-only — never read the stack |
| `/N` | Force exactly N arguments |
| `/Nf` | N arguments, forward only |
| `/Ns` | N arguments, stack only |

```
lower/f "ABC"                 => 'abc'
"DEF" lower/s                 => 'def'
lower/1 "GHI"                 => 'ghi'
```


## Evaluation model

* **Stack machine.** Each token either pushes a value or invokes a
  word. The final stack is the result.
* **Forward collection.** A word can collect arguments from after
  itself as well as before, with stack-first then forward-fill into
  signature slots.
* **Type-directed collection.** A forward token is only consumed if
  it matches the next expected type; mismatches stop collection and
  the word executes with what it has (or fails if it doesn't have
  enough).
* **Left-to-right.** Words that are still waiting evaluate strictly
  in source order. Use `(...)` to override.
* **Quotation.** Lists are *unevaluated* by default. `do` evaluates
  one; `call` splices one onto the stack; `quote` prevents
  evaluation of the next token.
* **`end`.** Forces the nearest waiting word to stop forward
  collection.

See **[Explanation §The stack model](explanation.md#the-stack-model)**
for a longer treatment.


## Type system

### Hierarchy

```
Any
├── None                            -- unit; sole inhabitant: `none`
├── Never                           -- empty / bottom
├── Scalar
│   ├── Atom
│   ├── Boolean                     -- false | true
│   ├── Number
│   │   ├── Integer
│   │   └── Decimal
│   ├── String
│   │   ├── EmptyString
│   │   └── ProperString
│   ├── Path
│   └── Time
│       ├── Date, DateTime, Instant, TimeOfDay
│       ├── Duration (CalDuration | ClkDuration)
│       └── Timezone
├── Node
│   ├── List
│   └── Map
├── Ideal
│   ├── Object (Resource (Entity))
│   ├── Array, Record, Options, Error
│   ├── Store, Table
│   ├── Fetch (Request | Response)
│   ├── Timeout, Interval
│   └── Tensor (Matrix | Vector)
├── Word
│   └── (internal control words)
└── Type
    ├── Function, FunctionSignature
    └── Disjunct (Enum)
```

A child matches its parent (`Integer` is a `Number` is a `Scalar`
is an `Any`); the converse is false. Types are written with
slash-separated paths in `fulltypeof`; short names like `Number`
or `Integer` are accepted in signatures.

### Short names

| Short | Full path |
|-------|-----------|
| `String` | `Scalar/String` |
| `Number` | `Scalar/Number` |
| `Integer` | `Scalar/Number/Integer` |
| `Decimal` | `Scalar/Number/Decimal` |
| `Boolean` | `Scalar/Boolean` |
| `Atom` | `Scalar/Atom` |
| `List` | `Node/List` |
| `Map` | `Node/Map` |
| `Store` | `Ideal/Store` |
| `Table` | `Ideal/Table` |
| `Record` | `Ideal/Record` |
| `Options` | `Ideal/Options` |
| `Timeout` | `Ideal/Timeout` |
| `Interval` | `Ideal/Interval` |
| `Function` | `Word/Function` |

### Disjunctions

`A tor B` produces a disjunctive type — values match if they match
either side:

```
type OptInt (Integer tor none)
OptInt unify 5                => 5 true
OptInt unify none             => none true
OptInt unify "x"              => '~unify-fail' false
```

### Type ordering

Every type has a unified integer rank. `cmp` / `lt` / `gt` / `sort`
all run a single LCA-Comparer-then-Rank cascade, so cross-type
comparisons are well-defined and total. Type literals sort strictly
below their concrete inhabitants of the same family:

```
Integer lt 0                  => true
[1,2] cmp [1,3]               => -1
```


## Word reference

Each entry lists the word, its signature(s), and one or more
examples. Where multiple signatures exist, they're tried in
declaration order — first match wins.

### Stack manipulation

All stack words are stack-only (modifier `/s`).

| Word | Effect | Description |
|------|--------|-------------|
| `dup` | `a → a a` | Duplicate top |
| `drop` | `a →` | Remove top |
| `swap` | `a b → b a` | Exchange top two |
| `over` | `a b → a b a` | Copy second to top |
| `rot` | `a b c → b c a` | Rotate top three |
| `nip` | `a b → b` | Remove second |
| `tuck` | `a b → b a b` | Copy top below second |
| `2dup` | `a b → a b a b` | Duplicate top pair |
| `2drop` | `a b →` | Remove top pair |
| `2swap` | `a b c d → c d a b` | Swap top two pairs |
| `2over` | `a b c d → a b c d a b` | Copy third pair to top |
| `pick` | `n → v` | Copy value at depth n |
| `roll` | `n → v` | Move value at depth n to top |
| `depth` | `→ n` | Current stack size |
| `stack` | `→ [...]` | Entire stack as a list |

### Arithmetic

Forward-collecting, Integer/Decimal with auto-promotion.

| Word | Operation | Example |
|------|-----------|---------|
| `add` | `a + b` | `1 add 2 => 3` |
| `sub` | `a - b` | `10 sub 3 => 7` |
| `mul` | `a * b` | `4 mul 5 => 20` |
| `div` | `a / b` | `10 div 2 => 5` |
| `mod` | `a % b` | `10 mod 3 => 1` |
| `pow` | `a ^ b` | `2 pow 10 => 1024` |
| `abs` | `|a|` | `abs -5 => 5` |
| `negate` | `-a` | `negate 5 => -5` |
| `sign` | `-1/0/1` | `sign -5 => -1` |
| `min` | minimum | `3 min 5 => 3` |
| `max` | maximum | `3 max 5 => 5` |

`add` on non-numeric scalars performs string concatenation:
`"a" add "b" => 'ab'`.

### Rounding

| Word | Description | Example |
|------|-------------|---------|
| `floor` | Round down | `floor 3.7 => 3` |
| `ceil` | Round up | `ceil 3.2 => 4` |
| `round` | Round nearest | `round 3.5 => 4` |
| `trunc` | Truncate toward zero | `trunc 3.9 => 3` |

### Roots, exponentials, logarithms

| Word | Description |
|------|-------------|
| `sqrt` | Square root |
| `cbrt` | Cube root |
| `exp` | e^x |
| `log` | Natural logarithm |
| `log2` | Log base 2 |
| `log10` | Log base 10 |

### Trigonometry

| Word | Description |
|------|-------------|
| `sin`, `cos`, `tan` | Standard trig functions |
| `asin`, `acos`, `atan` | Inverse trig functions |
| `atan2` | Two-argument arctangent |
| `hypot` | Hypotenuse, `hypot a b = sqrt(a^2 + b^2)` |

### Constants

| Word | Value |
|------|-------|
| `math-pi` | 3.141592653589793 |
| `math-e` | 2.718281828459045 |

### Strings

All forward-collecting. The "options" form takes a trailing map
with named flags (see each word's docs in
`lang/doc/design/LANGREF.10.md` for the full set).

| Word | Description | Example |
|------|-------------|---------|
| `upper` | Uppercase | `"hello" upper => 'HELLO'` |
| `lower` | Lowercase | `"ABC" lower => 'abc'` |
| `concat` | Join list elements into a string | `["a","b"] concat => 'ab'` |
| `split` | Split string by separator | `"a,b" split "," => ['a','b']` |
| `contains` | Substring test | `"hello" contains "ell" => true` |
| `indexof` | Find position (–1 if absent) | `"hello" indexof "ll" => 2` |
| `slice` | Substring; negative indices ok | `"hello" slice 1 3 => 'el'` |
| `replace` | Replace pattern | `"hello" replace "l" "r" => 'herro'` |
| `repeat` | Repeat string | `"ab" repeat 3 => 'ababab'` |
| `trim` | Trim whitespace or chars | `"  hi  " trim => 'hi'` |
| `pad` | Pad to width | `"hi" pad 5 => 'hi   '` |
| `match` | Regex match | `"abc" match "b(c)" => {0:'bc',1:'c'}` |
| `escape` | Escape special chars | `"a&b" {mode:'html} escape` |
| `normalize` | Unicode normalize / cleanup | `"a  b" {collapseWs:true} normalize` |
| `changecase` | Casing transform | `"fooBar" {style:'snake} changecase` |
| `format` | printf-style format | `"%.2f" format 3.14159 => '3.14'` |

#### Options examples

```
"a,,b" "," {keepEmpty:true} split                       => ['a','','b']
" a : b " ":" {trimParts:true} split                    => ['a','b']
"Hello" "hello" {cs:"insensitive"} contains             => true
"aaa" "a" "b" {scope:"all"} replace                     => 'bbb'
"hi" 5 {side:"left"} pad                                => '   hi'
"hello world" {style:"snake"} changecase                => 'hello_world'
```

### Boolean

| Word | Description | Example |
|------|-------------|---------|
| `and` | Logical AND | `true and false => false` |
| `or` | Logical OR | `true or false => true` |
| `not` | Logical NOT | `not true => false` |
| `xor` | Exclusive OR | `true xor true => false` |
| `nand` | NOT AND | `true nand true => false` |
| `implies` | Implication | `true implies false => false` |

### Comparison

All comparison words route through one total order — see
**[Explanation §Type ordering](explanation.md#type-ordering)**.

| Word | Description | Example |
|------|-------------|---------|
| `eq` | Equal (cross-leaf magnitude allowed) | `1 eq 1.0 => true` |
| `neq` | Not equal | `1 neq 2 => true` |
| `deq` | Deep / strict-identity equality | `[1,2] deq [1,2] => true` |
| `lt` | Less than | `1 lt 2 => true` |
| `gt` | Greater than | `2 gt 1 => true` |
| `lte` | Less or equal | `1 lte 1 => true` |
| `gte` | Greater or equal | `2 gte 1 => true` |
| `cmp` | Three-way: `-1` / `0` / `1` | `5 cmp 10 => -1` |
| `between` | Build closed-interval refinement | `Integer between 10 20` |

### Definition and scoping

| Word | Description | Example |
|------|-------------|---------|
| `def` | Define a word | `def x 42` |
| `undef` | Remove the latest definition | `undef x` |
| `fn` | Create typed function | `fn [[Integer] [Integer] [dup mul]]` |
| `var` | Scoped variable block | `var [[x] x mul x]` |
| `args` | Current `fn` args list (inside body) | `args . 0` |
| `call` | Splice list onto stack | `call [1 2 3]` |
| `quote` | Prevent evaluation of next token | `quote [1 add 2]` |

#### `fn` shape

A `fn` body is a flat list of `[input-sig] [output-sig] [body]`
triples. Inputs may be plain types or `name:Type` pairs (the names
become local bindings during the body):

```
def hyp fn [
  [a:Number b:Number] [Number]
  [(a mul a) add (b mul b) sqrt]
]
3 4 hyp                       => 5.0
```

### Control flow

| Word | Description | Example |
|------|-------------|---------|
| `if` | Conditional; else branch optional | `5 gt 3 if ["y"] ["n"]` |
| `for` | Numeric loop (counter or range) | `for 5 [dup mul]` |
| `do` | Evaluate list as program | `do [1 add 2] => 3` |
| `error` | Pattern-match an error value | `do [1 div 0] error [drop 42]` |
| `break` | Exit `for` loop early | `for 10 [dup gt 5 if [break]]` |
| `continue` | Skip to next iteration | `for 10 [dup mod 2 if [continue]]` |

#### `for` forms

```
for N [body]              # 0 .. N-1
for [a, b] [body]         # a .. b-1
for [a, b, step] [body]   # arithmetic progression
```

### List and array words

| Word | Description | Example |
|------|-------------|---------|
| `iota` | Generate `[0..N-1]` | `iota 5 => [0,1,2,3,4]` |
| `reshape` | Change dimensions | `iota 6 reshape [2,3]` |
| `flatten` | Remove one level of nesting | `[[1,2],[3]] flatten => [1,2,3]` |
| `transpose` | Swap rows/columns | `[[1,2],[3,4]] transpose` |
| `take` | First N elements | `[1,2,3,4] take 2 => [1,2]` |
| `shed` | Drop first N | `[1,2,3,4] shed 2 => [3,4]` |
| `reverse` | Reverse order | `[1,2,3] reverse => [3,2,1]` |
| `unique` | Remove duplicates | `[1,2,2,3] unique => [1,2,3]` |
| `grade` | Indices that would sort | `[3,1,2] grade => [1,2,0]` |
| `window` | Sliding window of size N | `[1,2,3,4] window 2` |
| `pairs` | Adjacent pairs | `[1,2,3] pairs => [[1,2],[2,3]]` |
| `group` | Group by key function | `[1,2,3] group [mod 2]` |
| `replicate` | Repeat each element N times | `[1,2,3] replicate [2,1,3]` |
| `expand` | Expand by Boolean mask | `[1,2,3] expand [true,false,true]` |
| `at` | Select by index list | `[10,20,30] at [2,0]` |
| `sortby` | Sort by predicate | `[3,1,2] sortby []` |
| `member` | Membership test | `[1,2,3] member 2 => true` |

### Higher-order array words

| Word | Description | Example |
|------|-------------|---------|
| `each` | Map a function | `[1,2,3] each [dup mul]` |
| `fold` | Reduce with accumulator | `[1,2,3] fold 0 [add] => 6` |
| `scan` | Running fold | `[1,2,3] scan 0 [add] => [1,3,6]` |
| `where` | Filter by predicate | `[1,2,3,4] where [gt 2]` |
| `outer` | Outer product | `[1,2] outer [3,4] [mul]` |
| `inner` | Inner product | `[1,2] inner [3,4] [mul] [add]` |

### Maps and access

| Word | Description | Example |
|------|-------------|---------|
| `get` / `.` | Lookup field/key | `{x:1} . x => 1` |
| `getr` / `!.` | Strict lookup (errors if missing) | `{x:1} !. y => error` |
| `set` | Set a key in a Store | `context set foo 99` |
| `context` | Push the current context Store | `context` |

### Type words

| Word | Description | Example |
|------|-------------|---------|
| `typeof` | Short type name | `typeof 42 => Integer` |
| `fulltypeof` | Full slash path | `fulltypeof 42 => Scalar/Number/Integer` |
| `is` | Type-compatibility test | `42 is Number => true` |
| `convert` | Convert scalar between types | `convert Integer "42"` |
| `base` | Zero / base value for a type | `base Integer => 0` |
| `type` | Register a named type | `type Point record [x:Number y:Number]` |
| `record` | Define a record type | `record [x:Number y:Number]` |
| `object` | Define an object type | `object {count: 0}` |
| `table` | Define a table type | `table Row` |
| `make` | Construct typed value or instance | `make Point [1 2]` |
| `untype` | Remove a type binding | `untype Point` |

### Inspection

| Word | Description |
|------|-------------|
| `inspect` | Structured view of a value, word, or type |
| `trace` | Evaluate a list with step-by-step tracing |

### I/O

| Word | Description | Example |
|------|-------------|---------|
| `print` | Print with newline | `print "hello"` |
| `printstr` | Print without newline | `printstr "hi"` |
| `read` | Read a file or stdin | `read "data.json"` |
| `write` | Write a file or std stream | `write "out.txt" "hi"` |
| `stdin`, `stdout`, `stderr` | Pseudo-paths | `read stdin` |

`read` and `write` accept an Options map with `fmt: 'json | 'csv |
'tsv | 'jsonic | 'text` to force a format; otherwise the extension
decides.

### Networking — `fetch`

| Word | Description |
|------|-------------|
| `fetch` | HTTP request, returns a `Response` |

```
fetch "https://api.example.com/v1"
fetch "https://api.example.com/v1" {
  method:  'post,
  headers: {Authorization: "Bearer x"},
  body:    {x: 1}
}
```

`Response` is an Ideal with `status`, `headers`, `body` fields.
Requires the `fetch` capability.

### SQLite

| Word | Description |
|------|-------------|
| `sqlite-open` | Open or create a database |
| `sqlite-close` | Close a database |
| `sqlite-exec` | Execute statement(s) (no rows expected) |
| `sqlite-query` | Run a query, return rows as a list of maps |
| `sqlite-tx` | Run a list inside a transaction |

```
def db sqlite-open "data.db"
db sqlite-exec "CREATE TABLE t (id INTEGER, n TEXT)"
db sqlite-query "SELECT * FROM t"
```

Requires the `sqlite` capability.

### Modules

| Word | Description | Example |
|------|-------------|---------|
| `module` | Define a module inline | `module [def x 1]` |
| `import` | Import a module by name or file | `import "lib.aql"` |

```
import utils [def f [dup add]]
utils.f 3                     => 6

import aql:time

import [helper as h] "lib/utils.aql"
```

### Concurrency

| Word | Signature | Description |
|------|-----------|-------------|
| `now` | `[] → [Instant]` | Current UTC instant |
| `sleep` | `[Integer] → []` | Pause for N milliseconds |
| `timeout` | `[Integer, List] → [Timeout]` | Schedule one-shot callback |
| `interval` | `[Integer, List] → [Interval]` | Schedule repeating callback |
| `cancel` | `[Timeout|Interval] → []` | Cancel a timer |
| `await` | `[Options?, List] → [List|Any]` | Run blocks in parallel |

Await modes (passed as `{mode: 'atom}` in the Options map):

| Mode | Behaviour | JS equivalent |
|------|-----------|---------------|
| `'all` (default) | All must succeed; first error fails | `Promise.all` |
| `'full` | All complete, results carry `{status,value}` | `Promise.allSettled` |
| `'first` | First to complete wins | `Promise.race` |
| `'any` | First non-error wins | `Promise.any` |

### Unification

| Word | Description |
|------|-------------|
| `unify` | Unify two values; returns result and Boolean |

```
1 unify Number                => 1 true
1 unify "x"                   => '~unify-fail' false
record [x:Number] unify {x:1} => '~unify-fail' false   # records ≠ maps
```

### Help

| Word | Description |
|------|-------------|
| `help` | Print help for a topic or list everything |


## Built-in modules

Built-in modules ship with the binary but are not auto-loaded —
`import aql:xxx` to enable.

| Module | What's inside |
|--------|---------------|
| `aql:math` | Extended numerics: complex math, statistics, special functions. |
| `aql:time` | `now`, `parse`, `format`, `add`, `diff`, `date`, `datetime`, `instant`, `timeofday`, `duration`, `timezone`. |
| `aql:matrix` | Tensor / Matrix / Vector types and linear algebra. |
| `aql:decision` | Decision tables (rules engine). |
| `aql:solardemo` | Example host module backing the API tests. |


## Error codes

Errors are values of type `Ideal/Error` with a `code` atom and a
`message` string. Common codes:

| Code | Meaning |
|------|---------|
| `undefined_word` | A bare name was used outside a quoted slot. |
| `type_mismatch` | A value didn't match an expected signature slot. |
| `arity_mismatch` | Wrong number of arguments. |
| `div_zero` | Division by zero. |
| `out_of_range` | Index or numeric value outside the legal range. |
| `unify_fail` | Two values cannot unify. |
| `not_found` | Strict lookup (`!.`, `getr`) found no key. |
| `io_error` | File I/O failed. |
| `cap_denied` | Operation needed a capability that wasn't enabled. |
| `cancelled` | Operation cancelled (timer, await branch). |

Use `do [...] error [...]` to catch them; dispatch on `.code`.


## Capabilities

A capability is a runtime feature flag that gates side-effecting
words. The defaults match the CLI; embeddings (Wasm, library) may
disable any of them.

| Capability | Words it gates |
|------------|----------------|
| `fileio` | `read`, `write` |
| `fetch` | `fetch` |
| `sqlite` | `sqlite-*` |
| `timers` | `timeout`, `interval`, `sleep` |
| `subprocess` | (reserved) |
| `vault` | secret lookup via vault |

Words attempting to use a disabled capability raise an
`Error{code:'cap_denied}`.
