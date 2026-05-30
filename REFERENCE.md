# AQL Reference

Information-oriented reference for AQL syntax, types, and the
built-in word library. For learning AQL, start with the
**[Tutorial](TUTORIAL.md)**. For task-oriented recipes, see the
**[How-To Guides](HOWTO.md)**. For *why* AQL is shaped the way it
is, see the **[Explanation](EXPLANATION.md)**.

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

Type literals: `Number`, `Integer`, `Decimal`, `String`, `Boolean`,
`Atom`, `Scalar`, `Any`, `None`, `List`, `Map`, plus every named
type you define with `def`.

### Compound data

| Syntax | Meaning |
|--------|---------|
| `[a, b, c]` | List literal |
| `[:Type]` | Typed list (every element must match `Type`) |
| `{k:v, ...}` | Map literal |
| `{foo}` | Field shorthand — `{foo}` ≡ `{foo: foo}` (see [Map field shorthand](#map-field-shorthand)) |
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

### Map field shorthand

A map entry written as a bare name — with no `: value` — is shorthand
for binding that name to itself, mirroring JavaScript's `{ foo }`:

| Shorthand | Expands to | Notes |
|-----------|------------|-------|
| `{foo}` | `{foo: foo}` | value is the same auto-evaluated word |
| `{foo/r}` | `{foo: foo/r}` | a word modifier stays on the **value**; the key is the base name |
| `{foo?}` | `{foo?: foo}` | a trailing `?` keeps the field **optional**; the value is the bare word |

The rule in one line: **the key is the base name and the value is the
whole token.** So `{foo}` looks up the binding `foo` and stores it under
key `foo`; `{foo/r}` stores it under key `foo` but keeps the `/r` on the
value.

```
def x 1
{x}                           => {x:1}
def a 10  def b 20
{a b}                         => {a:10 b:20}        # keys sort
{a c:3 b}                     => {a:10 b:20 c:3}    # mixes with explicit pairs
{outer: {a}}                  => {outer:{a:10}}     # nests
```

Because a shorthand value is auto-evaluated exactly like any bare map
value, the same rules apply (see
[Maps and access](#maps-and-access)): a plain binding resolves, a 0-arg
function dispatches, and a function that needs arguments must be held as
data with `/r` (or stored as an atom with `/q`):

```
def inc fn [[n:Integer] [Integer] [n add 1]]
{inc}                         => build error    # inc dispatched 0-arg, fails its signature
{inc/r} . inc 5               => 6              # /r holds the function as data
{inc/q} . inc is Atom         => true           # /q stores the bare name as an atom
```

The optional form composes with the `?:` optional-field rule: `{foo?}`
desugars to `{foo?: foo}`, i.e. the value becomes
`disjunct(foo, None, Absent)` — present, explicitly `none`, or absent.

**Only unquoted identifiers trigger the shorthand.** A quoted key
(`{'foo'}`, `{"foo"}`) or a non-identifier (`{123}`) is a parse error —
write the explicit `key: value` form for those. The pretty-printer
(`aql fmt`) normalises every shorthand back to its explicit form
(`{foo}` → `{foo:foo}`, `{foo/r}` → `{foo:foo/r}`, `{foo?}` →
`{foo?:foo}`).

**A word modifier belongs on a value, never on a bare key.** It is
legal on a shorthand entry (`{foo/r}` — the token is the value) but an
error on an explicit pair: `{foo/r: 1}` raises `[aql/illegal_key]`,
because the `/r` could only attach to the key `foo`, which is just a
name. If you genuinely need a `/` in a key, make it a literal with a
quoted key (`{'a/b': 1}`) or a computed key (`{[a/b]: 1}`).


## Evaluation model

* **Stack machine.** Each token either pushes a value or invokes a
  word. The final stack is the result.
* **Argument-order rule.** When a word runs, its parameter slots
  are filled **forward-first, then stack**. Tokens after the word
  are taken in source order into `args[0]`, `args[1]`, … until a
  barrier (`end`, `)`, another function word, type mismatch). Any
  remaining slots are filled from the stack, top of stack into the
  next-to-fill slot first. See
  **[Tutorial §3](TUTORIAL.md#the-argument-order-rule)**.
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

See **[Explanation §The stack model](EXPLANATION.md#the-stack-model)**
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
slash-separated paths in `pathof`; short names like `Number` or
`Integer` are accepted in signatures.

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
def OptInt (Integer tor none)
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

Forward-collecting, Integer/Decimal with auto-promotion. The
asymmetric ops (`sub`, `div`, `mod`, `pow`) follow the
**argument-order rule** — see
[Tutorial §3](TUTORIAL.md#the-argument-order-rule). All three call
forms `a b sub`, `a sub b`, and `sub b a` compute `a - b`.

| Word | Operation | Example |
|------|-----------|---------|
| `add` | `a + b` (commutative) | `1 add 2 => 3` |
| `sub` | `a - b` | `10 sub 3 => 7` |
| `mul` | `a * b` (commutative) | `4 mul 5 => 20` |
| `div` | `a / b` | `10 div 2 => 5` |
| `mod` | `a % b` | `10 mod 3 => 1` |
| `pow` | `a ^ b` | `2 pow 10 => 1024` |

`add` on non-numeric scalars performs string concatenation:
`"a" add "b" => 'ab'`.

Additional numeric words (`abs`, `negate`, `sign`, `min`, `max`,
`floor`, `ceil`, `round`, `trunc`, `sqrt`, `cbrt`, `exp`, `log`,
`log2`, `log10`, `sin`, `cos`, `tan`, `asin`, `acos`, `atan`,
`atan2`, `hypot`, constants `math.pi`, `math.e`) live in the
**`aql:math`** native module. Import to use:

```
"aql:math" import end
math.abs -5                   => 5
math.floor 3.7                => 3
math.sqrt 16                  => 4.0
```

### Strings

All forward-collecting. The "options" form takes a trailing map
with named flags (see each word's docs in
`lang/doc/design/LANGREF.10.md` for the full set).

**Argument-order note:** for binary/ternary string words like
`contains`, `indexof`, `slice`, `replace`, `split`, the
all-forward form `WORD input arg…` is the clearest reading per the
[argument-order rule](TUTORIAL.md#the-argument-order-rule). Infix
forms work too but require placing the search/needle on the *left*
of the word, with the haystack as the forward arg.

| Word | Description | Example |
|------|-------------|---------|
| `upper` | Uppercase | `upper "hello" => 'HELLO'` |
| `lower` | Lowercase | `lower "ABC" => 'abc'` |
| `concat` | Join list elements into a string | `concat ["a","b"] => 'ab'` |
| `split` | Split string by separator | `split "a,b" "," => ['a','b']` |
| `contains` | Substring test | `contains "hello" "ell" => true` |
| `indexof` | Find position (–1 if absent) | `indexof "hello" "ll" => 2` |
| `slice` | Substring; negative indices ok | `slice "hello" 1 3 => 'el'` |
| `replace` | Replace pattern | `replace "hello" "l" "r" => 'herlo'` |
| `repeat` | Repeat string | `repeat "ab" 3 => 'ababab'` |
| `trim` | Trim whitespace or chars | `trim "  hi  " => 'hi'` |
| `pad` | Pad to width | `"hi" pad 5 => 'hi   '` |
| `match` | Regex match (returns a struct) | `match "abc" "b(c)"` |

#### Options examples

Pass an Options map as the *last* forward argument:

```
split   "a,,b"      ","    {keepEmpty: true}            => ['a' '' 'b']
contains "hello"    "Ell"  {cs: "insensitive"}          => true
replace "aaa"       "a" "b" {scope: "all"}              => 'bbb'
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
**[Explanation §Type ordering](EXPLANATION.md#type-ordering)**.

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
| `var` | Scoped variable block | `5 var [[x] x mul x] => 25` |
| `args` | Current `fn` args list (inside body) | `args . 0` |
| `call` | Splice list onto stack | `call [1 2 3]` |
| `quote` | Prevent evaluation of next token | `quote [1 add 2]` |

#### `fn` shape

A `fn` body is a flat list of `[input-sig] [output-sig] [body]`
triples. Inputs may be plain types or `name:Type` pairs (the names
become local bindings during the body); the output-sig declares the
return type(s):

```
def inc fn [[n:Integer] [Integer] [n add 1]]
inc 5                         => 6

def avg fn [[a:Number b:Number] [Decimal] [(a add b) div 2.0]]
avg 3 4                       => 3.5
```

**Return types are checked.** When the body finishes, each declared
output type must accept the corresponding result value, by the same
rule the parameters use (see [fn type semantics](#fn-type-semantics)
below). A mismatch is an error, not a silent pass:

```
def bad fn [[] [Integer] ['hi']]
bad                           => [aql/type_error] return value 1: expected Integer got ProperString
```

Multiple triples declare overloads (the engine tries each in order);
multiple output types declare multiple return values.

#### `fn` type semantics

Parameter and return annotations may name **any** type — builtins,
and user-defined types introduced with `def NAME refine …`. A value
is accepted at a slot when it is a *member* of the declared type, and
the membership rule is **the same at parameters, returns, and the
`is` word** (one question: `v is T`). How membership is decided
depends on what kind of type `T` is:

**Builtins and structural types — nominal subtyping.** A value
matches when its own type is the declared type or a descendant.

```
def first fn [[xs:List] [Any] [xs get 0]]
first [10 20 30]              => 10
```

**Object / Record / Table types — nominal, by construction.** An
instance built with `make` carries the type's tag, so it satisfies
both parameter and return slots of that type (and of any supertype):

```
def Box (refine Object {v:0})
def wrap fn [[n:Integer] [Box] [make Box {v:n}]]
typeof (wrap 5)               => Box
(wrap 5) get 'v'              => 5
```

**Bare refinement — a *newtype*.** `def Pos (refine Integer)` adds no
predicate; it is a distinct nominal type. A plain `Integer` is **not**
a `Pos` — you construct one explicitly with a typed `def`. The same
strict rule holds at parameters and returns:

```
def Pos (refine Integer)
42 is Pos                                          => false
def g fn [[n:Pos] [Integer] [n]]
42 g                                               => [aql/signature_error] no matching signature for g
def x:Pos 42   x g                                 => 42

def mk fn [[] [Pos] [7]]
mk                                                 => [aql/type_error] return value 1: expected Pos got Integer
def mk2 fn [[] [Pos] [def x:Pos 7 x]]
mk2                                                => 7
```

**Predicate refinement — a *subset type*.** `def Big (Integer gt 10)`
carves out a subset by a predicate. Any base-type value that
satisfies the predicate is a member — no explicit construction
needed — and the predicate is enforced at parameters **and** returns
alike:

```
def Big (Integer gt 10)
50 is Big                                          => true
5  is Big                                          => false
def g fn [[n:Big] [Integer] [n]]
50 g                                               => 50
5  g                                               => [aql/signature_error] no matching signature for g

def mk fn [[] [Big] [50]]
mk                                                 => 50
def mkbad fn [[] [Big] [5]]
mkbad                                              => [aql/type_error] return value 1: expected Big got Integer
```

The newtype-vs-subset distinction and its cross-language rationale are
explained in **[Explanation: Function signatures](EXPLANATION.md#function-signatures-and-refinement-types)**
and pinned in `design/REFINE-NEWTYPE-VS-SUBSET.0.md`.

### Control flow

| Word | Description | Example |
|------|-------------|---------|
| `if` | Conditional; else branch optional | `if (5 gt 3) ["y"] ["n"]` |
| `for` | Numeric loop (counter or range) | `for 5 [42]` |
| `do` | Evaluate list as program | `do [1 add 2] => 3` |
| `error` | Pattern-match an error value | `do [1 div 0] error [drop 42]` |
| `break` | Exit `for` loop early | `for 10 [break]` |
| `continue` | Skip to next iteration | `for 10 [continue]` |

For `if`, the canonical form is all-forward `if cond [then] [else]`
— this is the form where the argument-order rule places cond into
`args[0]`, then into `args[1]`, else into `args[2]` as the handler
expects. See
**[Tutorial §3](TUTORIAL.md#the-argument-order-rule)**.

#### `for` forms

```
for N [body]              # body runs N times (no iteration index pushed)
for [a, b] [body]         # body runs b-a times
for [a, b, step] [body]   # body runs (b-a)/step times
```

`for` does not push the iteration index onto the body's stack. To
process a sequence with the index/element, use `iota N each [body]`
(each pushes the element before running the body):

```
iota 5 each [dup mul]     => [0 1 4 9 16]
```

### List and array words

| Word | Description | Example |
|------|-------------|---------|
| `iota` | Generate `[0..N-1]` | `iota 5 => [0,1,2,3,4]` |
| `reshape` | Change dimensions | `iota 6 reshape [2,3]` |
| `flatten` | Remove one level of nesting | `[[1,2],[3]] flatten => [1,2,3]` |
| `take` | First N elements | `[1,2,3,4] take 2 => [1,2]` |
| `shed` | Drop first N | `[1,2,3,4] shed 2 => [3,4]` |
| `reverse` | Reverse order | `[1,2,3] reverse => [3,2,1]` |
| `unique` | Remove duplicates | `[1,2,2,3] unique => [1,2,3]` |
| `grade` | Indices that would sort | `[3,1,2] grade => [1,2,0]` |
| `window` | Sliding window of size N | `[1,2,3,4] window 2` |
| `pairs` | Adjacent pairs | `[1,2,3] pairs => [[1,2],[2,3]]` |
| `group` | Pair parallel keys/values into a multi-map | `["a","b","a"] [1,2,3] group` |
| `replicate` | Repeat each element N times | `[1,2,3] replicate [2,1,3]` |
| `expand` | Expand by Boolean mask | `[1,2,3] expand [true,false,true]` |
| `at` | Select by index list | `[10,20,30] at [2,0]` |
| `sortby` | Sort by parallel key list | `["b","a","c"] [2,1,3] sortby` |
| `member` | Per-element membership test | `[1,2,3] [2,3,4] member => [true,true,false]` |
| `size` | Element / key count of a collection — works on any value (see [Size](#size)) | `[1,2,3] size => 3` |

### Higher-order array words

| Word | Description | Example |
|------|-------------|---------|
| `each` | Map a function | `[1,2,3] each [dup mul]` |
| `fold` | Reduce with accumulator | `fold [add] [1,2,3] 0 => 6` |
| `scan` | Running fold | `scan [add] [1,2,3]` |
| `outer` | Outer product | `outer [mul] [3,4] [1,2]` |
| `inner` | Inner product | `inner [add] [mul] [3,4] [1,2]` |

These higher-order words follow the **argument-order rule**
(see [Tutorial §3](TUTORIAL.md#the-argument-order-rule)). The
all-forward form shown above maps each list argument left-to-right
into the signature: `fold` takes `body data init`, `scan` takes
`body data`, `outer` takes `body listB listA`, `inner` takes
`combineBody productBody listB listA`.

### Size

`size` reports the **natural size** of *any* value as an `Integer`.
Unlike the collection words above — which accept only a concrete
list — `size` has signature `[Any]` and is a **total** function:
every value has a size and `size` never errors. For a list it returns
the element count, so it is the canonical way to ask "how long is this
list?"; it also generalises to maps, strings, numbers, and user types.

```
[10,20,30] size          => 3
```

The size of a value is the size of the collection it stands for, by
type:

| Value | Size | Example |
|-------|------|---------|
| List | element count | `[10,20,30] size => 3` |
| Map | key count | `{a:1, b:2} size => 2` |
| String | length in bytes | `"hello" size => 5` |
| Atom | length of the name | `foo/q size => 3` |
| Integer / Decimal | floored magnitude | `42 size => 42`, `7.9 size => 7` |
| Boolean | `1` for `true`, `0` for `false` | `true size => 1` |
| Path | segment count | `(make Path "a/b/c") size => 3` |
| Object / Array / Store / Table | field / element / entry / row count | `(make Pt {x:1 y:2}) size => 2` |
| `None`, a Date, a bare scalar, or any non-concrete value (e.g. a bare type literal) | `0` (never errors) | `None size => 0`, `List size => 0` |

Dispatch is type-driven: each type contributes its own size rule via
the kernel's `Sizer` capability (`eng.SizeOf`), and a type with no
`Sizer` in its lattice sizes to `0`. There is no separate `length`
word — `size` subsumes it.

### Maps and access

| Word | Description | Example |
|------|-------------|---------|
| `get` / `.` | Lookup field/key | `{x:1} . x => 1` |
| `getr` / `!.` | Strict lookup (errors if missing) | `{x:1} !. y => error` |
| `set` | Set a key in a Store | `context set foo 99` |
| `context` | Push the current context Store | `context` |

**Dotted access binds tightly.** A `.`/`!.` chain groups to a single
`( … )` so it binds to its immediate receiver, not to a surrounding call:
`size m.x` means `size (m.x)`, and `a.b.c` is `( a get b get c )`. Two
consequences:

- **Access the result of a call** by parenthesising the call:
  `(make Point {x:1 y:2}) .x`, `(import "data.json") . name` — bare
  `make … {} .x` would feed `.x`'s result *into* `make`.
- **`/r` is a pure reference to a *function word*: it resolves the name to
  the bound function and *advances the pointer* — it never calls the
  function in place.** `/r` is legal **only** for function words; a name
  bound to a non-function value (a plain value, a type) raises
  `[aql/illegal_ref]`, because a bare value name already pushes its value
  — there is no call/value asymmetry for `/r` to break. The same rule
  applies to the `ref` word. The reference holds at any arity and in any
  position (top level, list element, paren, `do`-block, map value): `g/r`
  yields the function as data, `add/r 2 3` leaves `[Function, 2, 3]` on
  the stack (the args are *not* consumed), and `[zero/r]` is
  `[<function>]` even for a 0-arg `zero` (it is **not** fired). To
  **call** a referenced function, use the bare word (`add 2 3`), `apply`
  it (`2 3 (quote (f/r)) apply`), or access it as a member (below), where
  `get` brings the value live.

- **A function stored in a plain map** is callable via dot. Store it with
  `/r` — `{fn: myfn/r}` — which holds the function as data; then
  `m.fn arg` retrieves it (via `get`) and the arg calls it. Stored *bare*
  (`{fn: myfn}`) the map value is auto-evaluated: `myfn` is dispatched
  0-arg, which fails if it needs arguments — so a bare entry like
  `{fn: myfn}` is a **build error** (bare words never degrade to data;
  use `/r` for a callable data value, or `/q` for an atom). **Module
  functions are exported the same way** — `export "m" {fn: fn/r}` (a
  bare `{fn: fn}` export errors for the same reason). The distinction is
  whether the value is *brought live*: `/r` itself holds; member access
  (`get`) and bare words dispatch.

### Type words

| Word | Description | Example |
|------|-------------|---------|
| `typeof` | Type of a value (single Parent hop) | `typeof 42 => Integer` |
| `pathof` | Ancestry path (root first, leaf last) | `pathof Integer` |
| `is` | Type-compatibility test | `42 is Number => true` |
| `convert` | Convert scalar between types | `convert Integer "42" => 42` |
| `base` | Zero / base value for a type | `base Integer => 0` |
| `refine` | Build a refinement of a base type | `refine Object {count:0}` |
| `make` | Construct typed value or instance | `make Point [1 2]` |

Named types are introduced by pairing `def` with a `refine`
expression: `def Point refine Record [x:Number y:Number]`,
`def Counter refine Object {count: 0}`, `def Inventory refine Table
Row`. See **[HOWTO: Define a record/table/object type](HOWTO.md#define-a-record-type)**.

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

<!-- aql-test: skip -->
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
1 unify Number                       => 1 true
1 unify "x"                          => '~unify-fail' false
refine Record [x:Number] unify {x:1} => '~unify-fail' false   # records ≠ maps
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
