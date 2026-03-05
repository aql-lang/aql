
# AQL Language Reference

AQL is a concatenative language using function composition. The core
engine is a pure stack machine: each token is interpreted as a function
that modifies the stack. Literals push themselves; words consume
arguments and push results.


## Syntax

### Literals

**Integers** are written as decimal digits with an optional leading minus.

```
42      => 42
-5      => -5
0       => 0
```

**Strings** are enclosed in double or single quotes.

```
"hello world"   => 'hello world'
'single'        => 'single'
""              => ''
```

**Atoms** are bare unquoted words that do not match any defined function,
type name, or boolean. They represent symbolic names.

```
foo             => foo
abc             => abc
```

**Booleans** are the bare words `true` and `false`.

```
true    => true
false   => false
```

**Type literals** name a type without carrying a value.

```
number    string    boolean    atom    scalar    any    none    list    map
```

### Compound Data

**Lists** use square brackets with comma-separated elements.

```
[1,2,3]           => [1,2,3]
["a","b"]         => ['a','b']
[]                => []
```

**Typed lists** constrain every element to a type.

```
[:string]             => [:string]
[:number]             => [:number]
[:{x:number}]         => [:{x:number}]
[:[:number]]          => [:[:number]]
```

**Maps** use braces with `key:value` pairs.

```
{x:1,y:2}        => {x:1,y:2}
{a:"hello"}       => {a:'hello'}
{}                => {}
```

**Typed maps** constrain every value to a type.

```
{:string}             => {:string}
{:number}             => {:number}
{:{x:number}}         => {:{x:number}}
{:[:string]}          => {:[:string]}
```

Structures nest freely.

```
[[1,2],[3,4]]     {a:{b:1}}     [{x:1},{y:2}]     {a:[1,2,3]}
```

### Comments

Line comments start with `#`. Block comments are delimited by `##`.

```
1 add 2         # this is a comment
## multi-line
   comment ## 3 mul 4
```

### Parentheses

Parentheses group sub-expressions and control evaluation order.

```
(1 add 2)                   => 3
2 mul (3 add 4)             => 14
(2 add 3) mul (4 add 1)     => 25
```

### Multiple Values

Tokens that are not consumed remain on the stack. The final stack
contents are the result.

```
1 2 3           => 1 2 3
1 "hello"       => 1 'hello'
```


## Execution Model

AQL is a stack machine. The program is a sequence of tokens read left
to right. Each token either pushes a value onto the stack or names a
word that consumes values and pushes results.

### Prefix evaluation

In the simplest case, arguments sit on the stack before the word
executes. This is the classic stack-machine style:

```
1 2 add         => 3
```

Step by step:

1. `1` — push 1 onto the stack. Stack: `[1]`
2. `2` — push 2. Stack: `[1, 2]`
3. `add` — consume two values, push their sum. Stack: `[3]`

Any values left on the stack at the end are the result.

### Suffix collection

Most words also accept arguments that appear *after* them. When a word
does not yet have enough arguments on the stack, it waits: each
subsequent value is collected as a suffix argument until the word has
everything it needs. Then the word executes.

```
add 1 2         => 3
```

1. `add` — needs two arguments, stack is empty. Wait for suffix values.
2. `1` — collected as first argument to `add`.
3. `2` — collected as second argument. `add` now has both; it executes.

This is why the same word works in prefix, suffix, and infix position:

```
1 2 add         => 3       # prefix: both args already on the stack
add 1 2         => 3       # suffix: both args collected after the word
1 add 2         => 3       # infix: 1 from the stack, 2 collected after
```

### Nested suffix collection

When one word is waiting for arguments and encounters another word,
the inner word collects *its* arguments first. The inner word's result
then becomes an argument to the outer word.

```
def Point record [x:number y:number]
```

1. `def` — needs a name and a body. Waits.
2. `Point` — collected as the name (first argument to `def`).
3. `record` — this is a word, not a literal. It needs its own argument.
4. `[x:number y:number]` — collected by `record`, which executes and
   produces a record type value.
5. That record type value becomes the second argument to `def`. Now
   `def` executes.

### Type-directed collection

Each word declares the types of arguments it accepts. Suffix collection
checks each incoming value against the expected type. If the type does
not match, collection stops — the word executes with whatever prefix
arguments it already had, and the unmatched value stays on the stack.

```
upper "hello" 42
```

1. `upper` — needs one string. Waits.
2. `"hello"` — matches string. Collected. `upper` executes → `'HELLO'`.
3. `42` — pushed onto the stack.

Result: `'HELLO' 42`. The `42` was never consumed because `upper`
was satisfied after one argument.

### Competing words and precedence

When two words are both waiting for suffix arguments, *operator
precedence* determines who gets the value. A higher-precedence word
captures the value before a lower-precedence word, even if the
lower-precedence word started waiting first.

```
2 add 3 mul 4           => 14
```

`add` (precedence 1) is waiting for its second argument. It sees `3`,
but `mul` (precedence 2) appears next — and `mul` binds tighter. So
`mul` captures `3` and `4`, producing 12. That result becomes the
second argument to `add`: 2 + 12 = 14.

```
2 mul 3 add 4           => 10
```

Here `mul` (higher precedence) captures `3` immediately. It executes:
2 * 3 = 6. Then `add` gets 6 from the stack and captures `4`: 6 + 4 = 10.

### The `end` keyword

Sometimes you need to stop suffix collection early — for example, when
a word's argument is followed by more tokens that should not be
consumed.

```
set foo 99 end get foo      => 99
```

Without `end`, the `set` word would try to collect `get` and `foo` as
additional arguments. The `end` keyword forces the nearest waiting word
to stop collecting and execute immediately.

### Lists are quotation

Square brackets create a list of *unevaluated* values. Words inside a
list are stored literally — they are not executed.

```
[1 add 2]               => [1,add,2]       # add is NOT executed
def inc [1 add]
5 inc                    => 6               # now the list body executes
```

When a defined word's body is a list, its elements are spliced into the
token stream on use. This is how `def` creates reusable code fragments.

The `do` word explicitly evaluates a list as if its elements were
tokens in the main program:

```
do [1 add 2]            => 3
```

For maps, `do` evaluates any list values (depth-first), leaving
non-list values unchanged:

```
do {x:[3 add 4], y:[upper "a"]}    => {x:7, y:'A'}
```


## Argument Passing

Words accept arguments in three styles.

**Prefix** — arguments are already on the stack (Forth style).

```
1 2 add         => 3
"hello" upper   => 'HELLO'
```

**Suffix** — arguments follow the word and are collected automatically.

```
add 1 2         => 3
lower "ABC"     => 'abc'
```

**Infix** — some arguments come from the stack, the rest are collected.

```
1 add 2         => 3
10 sub 3        => 7
```

By default most words have *suffix precedence*: when prefix arguments
are available they are tried first; suffix collection is the fallback.
Stack-manipulation words (`dup`, `swap`, `drop`) are prefix-only by
default.


## Word Modifiers

Append a modifier to a word name to override argument behaviour.

| Modifier | Meaning                              |
|----------|--------------------------------------|
| `/p`     | Force prefix-only arguments          |
| `/s`     | Force suffix-only arguments          |
| `/N`     | Expect exactly N arguments           |
| `/Ns`    | Expect N arguments, suffix only      |
| `/Np`    | Expect N arguments, prefix only      |

```
lower/s "E"     => 'e'
"F" lower/p     => 'f'
lower/1 "G"     => 'g'
lower/1s "H"    => 'h'
```


## Operator Precedence

Arithmetic words carry a precedence level. Higher precedence binds
tighter when words compete for suffix arguments.

| Precedence | Words                                          |
|------------|------------------------------------------------|
| 2 (high)   | `mul`, `div`, `mod`, `and`, `nand`             |
| 1 (low)    | `add`, `sub`, `or`, `xor`, `implies`, `lt`, `gt`, `lte`, `gte`, `eq`, `deq` |

```
2 add 3 mul 4               => 14      # (2+3)*4, not 2+(3*4)
2 mul 3 add 4               => 10      # (2*3)+4
1 add 2 mul 3 add 4         => 11
```


## The `end` Keyword

`end` terminates suffix argument collection for the nearest pending
word. Remaining arguments are taken from the stack.

```
set foo 99 end get foo      => 99
unify 1 number end 42       => 1 true 42
```


## Words

### String Words

#### `upper`

Convert a string or atom to uppercase.

*Signatures:* `[string] -> [string]`, `[atom] -> [string]`
*Precedence:* suffix

```
"hello" upper       => 'HELLO'
upper "abc"         => 'ABC'
a upper             => 'A'
```

#### `lower`

Convert a string or atom to lowercase.

*Signatures:* `[string] -> [string]`, `[atom] -> [string]`
*Precedence:* suffix

```
"WORLD" lower       => 'world'
lower "ABC"         => 'abc'
lower B             => 'b'
```

### Arithmetic Words

Arithmetic words take two integers and produce one integer.

*Signature:* `[integer, integer] -> [integer]`

#### `add`

Addition for integers; string concatenation when at least one argument
is a string. Non-string scalars are converted to their string
representation before concatenation. Precedence 1.

*Signatures:*
- `[integer, integer] -> [integer]` — numeric addition
- `[scalar, scalar] -> [string]` — string concatenation

```
1 2 add             => 3
1 add 2             => 3
"hello" add " world"    => 'hello world'
"count: " add 42        => 'count: 42'
42 add " items"         => '42 items'
```

#### `sub`

Subtraction. Precedence 1. The first argument is the minuend, the
second is the subtrahend.

```
10 3 sub            => 7
10 sub 3            => 7
```

#### `mul`

Multiplication. Precedence 2.

```
4 5 mul             => 20
4 mul 5             => 20
```

#### `div`

Integer division. Precedence 2. Division by zero is an error.

```
10 3 div            => 3
10 div 3            => 3
10 div 0            => ERROR: division_by_zero
```

#### `mod`

Modulo. Precedence 2. Modulo by zero is an error.

```
10 3 mod            => 1
10 mod 3            => 1
10 mod 0            => ERROR: modulo_by_zero
```

### Boolean Words

#### `or`

Logical OR. Precedence 1.

*Signature:* `[boolean, boolean] -> [boolean]`

```
true or false           => true
false or false          => false
```

#### `and`

Logical AND. Precedence 2 (binds tighter than `or`).

*Signature:* `[boolean, boolean] -> [boolean]`

```
true and false          => false
true and true           => true
true or false and false => true       # and binds first
```

#### `not`

Logical NOT (unary).

*Signature:* `[boolean] -> [boolean]`

```
true not                => false
not false               => true
```

#### `xor`

Exclusive OR. Precedence 1.

*Signature:* `[boolean, boolean] -> [boolean]`

```
true xor false          => true
true xor true           => false
```

#### `nand`

Logical NAND (NOT AND). Precedence 2.

*Signature:* `[boolean, boolean] -> [boolean]`

```
true nand true          => false
true nand false         => true
```

#### `implies`

Logical implication (a → b). False only when the first argument is
true and the second is false. Precedence 1.

*Signature:* `[boolean, boolean] -> [boolean]`

```
true implies false      => false
false implies true      => true
```

### Comparison Words

Comparison words take two arguments with suffix precedence at
precedence level 1. They use natural type comparisons: integers
compare numerically, strings compare lexicographically, booleans
compare as `false < true`, atoms compare lexicographically on
their name.

Cross-type comparisons are an error for ordering words (`lt`, `gt`,
`lte`, `gte`). Equality words (`eq`, `deq`) return `false` for
mismatched types without error.

Non-scalar types (lists, maps) are not orderable — ordering
comparisons produce an error. Use `eq` or `deq` for equality checks
on non-scalars.

#### `lt`

Less than.

*Signature:* `[any, any] -> [boolean]`
*Precedence:* 1

```
1 lt 2                  => true
2 lt 1                  => false
1 lt 1                  => false
"abc" lt "def"          => true
false lt true           => true
```

#### `gt`

Greater than.

*Signature:* `[any, any] -> [boolean]`
*Precedence:* 1

```
2 gt 1                  => true
1 gt 2                  => false
"def" gt "abc"          => true
```

#### `lte`

Less than or equal.

*Signature:* `[any, any] -> [boolean]`
*Precedence:* 1

```
1 lte 2                 => true
1 lte 1                 => true
2 lte 1                 => false
```

#### `gte`

Greater than or equal.

*Signature:* `[any, any] -> [boolean]`
*Precedence:* 1

```
2 gte 1                 => true
1 gte 1                 => true
1 gte 2                 => false
```

#### `eq`

Exact equality. For scalars (integer, string, boolean, atom, none),
compares by value. For non-scalars (list, map), compares by identity
(same in-memory object).

*Signature:* `[any, any] -> [boolean]`
*Precedence:* 1

```
1 eq 1                  => true
1 eq 2                  => false
"abc" eq "abc"          => true
none eq none            => true
true eq false           => false
```

#### `deq`

Deep equality. Traverses lists and maps depth-first, comparing all
leaf values. Two lists are deeply equal if they have the same length
and each element is deeply equal. Two maps are deeply equal if they
have the same keys and each value is deeply equal.

*Signature:* `[any, any] -> [boolean]`
*Precedence:* 1

```
1 deq 1                 => true
[1,2,3] deq [1,2,3]    => true
[1,2] deq [1,2,3]      => false
{x:1,y:2} deq {x:1,y:2}   => true
{x:1,y:2} deq {x:1,y:3}   => false
{x:1} deq {x:1,y:2}        => false
none deq none           => true
```

### Conversion Words

#### `convert`

Convert a value to a target scalar type. An optional third argument
specifies a variant (e.g., numeric base).

*Signatures:*
- `[any, any] -> [scalar]` — 2-arg form: value and target type
- `[any, any, string] -> [scalar]` — 3-arg form: value, target type, variant

*Precedence:* suffix

**To string:**

```
convert 99 string              => '99'
convert true string            => 'true'
convert foo string             => 'foo'
```

**To string with variant:**

```
convert 10 string "hex"        => 'a'
convert 255 string "HEX"       => 'FF'
convert 10 string "bin"        => '1010'
convert 8 string "oct"         => '10'
```

**To number:**

```
convert "42" number            => 42
convert "ff" number "hex"      => 255
convert "1010" number "bin"    => 10
convert "10" number "oct"      => 8
```

**To boolean:**

```
convert 1 boolean              => true
convert 0 boolean              => false
convert "true" boolean         => true
convert "" boolean             => false
```

**To atom:**

```
convert 42 atom                => 42
convert "hello" atom           => hello
```

### Stack Words

Stack words are prefix-only by default.

#### `dup`

Duplicate the top value.

*Signature:* `[any] -> [any, any]`

```
1 dup               => 1 1
"a" dup             => 'a' 'a'
```

#### `swap`

Swap the top two values.

*Signature:* `[any, any] -> [any, any]`

```
1 2 swap            => 2 1
```

#### `drop`

Remove the top value.

*Signature:* `[any] -> []`

```
1 drop              =>
99 drop             =>
```

### Storage Words

#### `set`

Store a value under a key. The key may be a bare word or a string.

*Signatures:*
- `[string, any] -> []`
- `[word, any] -> []`
- `[any, any] -> []`

*Precedence:* suffix

```
set foo 99 end
set bar "hello" end
set "key" 42 end
```

#### `get`

Retrieve a previously stored value by key.

*Signature:* `[any] -> [any]`
*Precedence:* suffix

```
set foo 99 end get foo      => 99
set bar "hello" end get bar => 'hello'
```

### Type Words

#### `unify`

Attempt to unify two values. Pushes the unified value and a boolean
indicating success.

*Signature:* `[any, any] -> [any, boolean]`
*Precedence:* suffix

On failure the unified value is the string `'~unify-fail'` and the
boolean is `false`.

**Scalar rules:**

- Identical type and value: succeed, return the value.
- One is a subtype of the other: succeed, return the narrower value.
- `any` unifies with everything (except `none`): return the specific value.
- `none` only unifies with `none`.
- Same type, different values: fail.
- Incompatible types: fail.

```
1 1 unify                   => 1 true
1 2 unify                   => '~unify-fail' false
1 number unify              => 1 true
1 string unify              => '~unify-fail' false
1 any unify                 => 1 true
none none unify             => none true
none 1 unify                => '~unify-fail' false
none any unify              => '~unify-fail' false
```

**List rules:**

- Concrete lists: element-by-element; lengths must match.
- Typed list vs concrete list: every element must match the child type.
- Two typed lists: child types are unified.
- The `list` type literal unifies with any list.

```
[1,"a"] unify [1,"a"]       => [1,'a'] true
[1,2] unify [1,3]           => '~unify-fail' false
[:number] unify [1,2,3]     => [1,2,3] true
[:string] unify [1,2]       => '~unify-fail' false
[:any] unify [:number]      => [:number] true
list unify [1,2]            => [1,2] true
```

**Map rules:**

- Concrete maps: key sets must be identical; values unified pairwise.
- Typed map vs concrete map: every value must match the child type.
- Two typed maps: child types are unified.
- The `map` type literal unifies with any map.

```
{x:1} unify {x:1}           => {x:1} true
{x:1} unify {x:2}           => '~unify-fail' false
{:number} unify {a:1,b:2}   => {a:1,b:2} true
{:string} unify {a:1}       => '~unify-fail' false
{:any} unify {:number}      => {:number} true
map unify {x:1}             => {x:1} true
```

**Nested structures** unify recursively.

```
{x:[1,string]} unify {x:[number,"a"]}     => {x:[1,'a']} true
{a:{b:number}} unify {a:{b:42}}            => {a:{b:42}} true
```

### Definition Words

#### `def`

Define a new word as a literal substitution. The body can be a list
(whose elements are spliced into execution) or a single value.

*Signatures:*
- `[word, any] -> []`
- `[string, any] -> []`

*Precedence:* suffix

```
def increment [1 add]
2 increment                 => 3

def double [dup add]
5 double                    => 10

def myval 42
myval                       => 42

def "quoted" [1 add]
10 quoted                   => 11

[1 add] def inc2
10 inc2                     => 11
```

Defined words may be used multiple times.

```
def inc [1 add]
1 inc inc inc               => 4
```

**Partial application via `def ... end`.** When a word inside a `def`
body does not receive all of its arguments, the word and its collected
arguments are packaged together. The resulting definition acts as a
partially applied function — supply the remaining arguments on use.

```
def add5 add 5 end
10 add5                     => 15

def mul3 mul 3 end
4 mul3                      => 12

def sub1 sub 1 end
10 sub1                     => 9
```

This works for all words, not just arithmetic:

```
def greet add "hello " end
greet "world"               => 'hello world'

def lt10 lt 10 end
5 lt10                      => true

def and_true and true end
false and_true              => false
```

Curried words compose naturally:

```
def add5 add 5 end
def mul2 mul 2 end
3 add5 mul2                 => 16

def add5_twice [add5 add5]
10 add5_twice               => 20
```

Definitions stack: a second `def` for the same name shadows the
previous one.

```
def foo 1
def foo 2
foo                         => 2
```

#### Function Signatures with `fn`

The `fn` word parses a list of signature triples into a typed function
definition. Use it with `def` to create functions with typed
parameters:

```
def name fn [[input-params] [output-types] [body]]
```

Each triple consists of an input signature, an output signature
(informational), and a body.

**Unnamed parameters** list the expected types positionally:

```
def double fn [[number] [number] [dup add]]
7 double                    => 14
```

**Named parameters** use pair syntax (`name:type`) and are bound as
scoped variables during execution:

```
def square fn [[x:number] [number] [x mul x]]
5 square                    => 25
```

Multiple parameters are comma-separated:

```
def add2 fn [[x:number,y:number] [number] [x add y]]
3 5 add2                    => 8
```

Named parameters are automatically undefined after the body executes,
so they do not leak:

```
def sq fn [[x:number] [number] [x mul x]]
4 sq x                      => 16 x
```

Function definitions support all argument styles (prefix, suffix,
infix):

```
def sq fn [[x:number] [number] [x mul x]]
5 sq                        => 25       # prefix
sq 6                        => 36       # suffix
```

Multiple overloaded signatures can be specified as consecutive triples:

```
def op fn [[number] [number] [dup mul] [string] [string] [dup]]
```

Supported type names in signatures: `any`, `none`, `scalar`, `number`,
`integer`, `string`, `boolean`, `atom`, `list`, `map`.

#### `undef`

Remove the most recent definition of a word. If definitions were
stacked, the previous one is revealed.

*Signatures:*
- `[word] -> []`
- `[string] -> []`

*Precedence:* suffix

```
def foo 1 foo undef foo foo             => 1 foo

def foo 1 def foo 2 foo undef foo foo   => 2 1
```

### Variable Words

#### `var`

Define scoped variables. `var` takes one list argument whose first
element is a list of variable declarations and whose remaining
elements form the body. After the body executes, all variables are
automatically undefined.

*Signature:* `[list] -> [results...]`
*Precedence:* suffix

Each declaration is one of:

| Form       | Meaning                             |
|------------|-------------------------------------|
| `x`        | Bare word — takes value from stack  |
| `[x 2]`    | List — defines x with value 2       |

The expansion `var [[x] body...]` is equivalent to
`def x end body... undef x`.

**Variable from stack:**

```
5 var [[x] x mul x]                    => 25
```

Here `x` is bound to 5 (the top of the stack). The body `x mul x`
computes 5 * 5 = 25. After execution, `x` is undefined.

**Inline value:**

```
var [[[x 2]] x mul x]                  => 4
```

`x` is bound to 2 directly inside the declaration.

**Multiple variables:**

```
3 5 var [[x y] x add y]               => 8
```

`x` binds to the top of the stack (5), `y` to the next (3).
Wait — each `def name end` in the expansion peels the topmost value:
first `x` gets 5, then `y` gets 3.

**Mixed inline and stack:**

```
10 var [[[x 2] y] x add y]            => 12
```

`x` = 2 (inline), `y` = 10 (from stack).

**Variables do not leak:**

```
5 var [[x] x mul x] x                 => 25 x
```

After `var` completes, `x` reverts to an unknown word (atom `x`).

**Preserves existing definitions:**

```
def foo 99
5 var [[x] x add foo] foo             => 104 99
```

`foo` remains defined after `var` completes.

### Function Words

#### `fn`

Parse a list of signature triples into a typed function definition
value. Used with `def` to create functions with typed and/or named
parameters.

*Signature:* `[list] -> [fndef]`
*Precedence:* suffix

The list argument must contain one or more triples of
`[input-sig] [output-sig] [body]`. The `fn` word returns an internal
function definition value which `def` recognizes and installs as typed
signatures.

```
def square fn [[x:number] [number] [x mul x]]
5 square                    => 25
```

See [Function Signatures with `fn`](#function-signatures-with-fn)
above for full details.

### Record Type Words

#### `record`

Create a record type from a list of field pairs. Each element is a
pair (single-key map) defining a field name and its type constraint.
A record type is a schema that validates maps: it requires exactly the
specified keys, each with a value matching the field's type constraint.

*Signature:* `[list] -> [record-type]`
*Precedence:* suffix

```
record [x:number y:string]                => record{x:number,y:string}
record [{x:{z:boolean}} "y":1]            => record{x:{z:boolean},y:1}
```

Use with `type` to create named record types:

```
type Point record [x:number y:number]
Point                                      => record{x:number,y:number}
```

Records only unify with other records, never with maps or lists.
Field order is significant — two records with the same fields in
different order do not unify.

```
record [x:any] unify record [x:number]       => record{x:number} true
record [x:number] unify record [y:number]     => '~unify-fail' false
record [x:number y:string] unify
  record [y:string x:number]                  => '~unify-fail' false  # order differs
```

Records do not unify with maps:

```
{x:1} unify record [x:number]                => '~unify-fail' false
map unify record [x:number]                   => '~unify-fail' false
```

**Optional fields with inline disjunctions.** A field value written
as a list `[...]` is evaluated as code. This lets you write
disjunctions directly inside the record definition:

```
record [x:number y:[string or none]]
                                => record{x:number,y:string|none}
```

The disjunction narrows on unification:

```
record [x:number y:[string or none]] unify record [x:number y:string]
                                => record{x:number,y:string} true
```

`make` accepts `none` for optional fields:

```
type Person record [name:string nick:[string or none]]
make Person ["Alice" "ace"]     => {name:'Alice',nick:'ace'}
make Person ["Bob" none]        => {name:'Bob',nick:none}
```

**Map form.** `make` also accepts a map, matching field names by key.
Missing fields are filled with `none` when the field type allows it:

```
type Person record [name:string nick:[string or none]]
make Person {name:"Alice" nick:"ace"}  => {name:'Alice',nick:'ace'}
make Person {name:"Bob"}               => {name:'Bob',nick:none}
```

Unknown keys and missing required fields are errors:

```
make Person {nick:"ace"}               => error: missing field "name"
make Person {name:"A" extra:1}         => error: unknown field "extra"
```

**Options map.** `make` accepts a trailing options map. With
`base:true`, missing fields are filled with their type's base value
(zero value) instead of `none`:

```
type Item record [name:string qty:number active:boolean]
make Item {name:"Widget"} {base:true}  => {name:'Widget',qty:0,active:false}
make Item {name:"Bolt" qty:5} {base:true}
                                       => {name:'Bolt',qty:5,active:false}
```

For disjunction fields, the base of the first non-none alternative
is used:

```
type Rec record [x:number y:[string or none]]
make Rec {x:1} {base:true}            => {x:1,y:''}
make Rec {x:1}                        => {x:1,y:none}
```

**User-defined types as field constraints.** Alternatively, define
a disjunction separately and reference it by name:

```
type OptStr (string or none)
type Person record [name:string nick:OptStr]
Person                          => record{name:string,nick:string|none}
```

**Nested record types.** Define inner records separately and
reference them by name:

```
type Inner record [z:string]
type Outer record [x:number y:Inner]
Outer                          => record{x:number,y:record{z:string}}
```

#### `table`

Create a table type from a record type. A table represents a list of
record instances — each row is a map conforming to the record schema.

*Signature:* `[record-type] -> [table-type]`
*Precedence:* suffix

```
table record [x:number y:string]          => table{x:number,y:string}
```

Use with `type` to create named table types:

```
type foo record [x:integer y:string]
type bar table foo
bar                                        => table{x:number/integer,y:string}
```

Tables only unify with other tables, never with plain lists. Two table
types unify by unifying their underlying record schemas.

```
type A record [x:any]
type B record [x:number]
(table A) unify (table B)                  => table{x:number} true
```

Tables do not unify with `list`:

```
list unify table record [x:number]         => '~unify-fail' false
```

Use `make` to create table instances from a list of row lists. Each
inner list provides the values for one row, either positionally or by
name:

```
type foo record [x:integer y:string]
type bar table foo
make bar [[1 a] [2 b]]                    => [{x:1,y:'a'},{x:2,y:'b'}]
make bar [[x:1 y:a] [x:2 y:b]]           => [{x:1,y:'a'},{x:2,y:'b'}]
```

#### `type`

Define a named type. The body must be a type value: a record type,
table type, disjunct, type literal, typed list, or typed map. Unlike
`def`, `type` validates that the body is actually a type.

*Signatures:*
- `[word, any] -> []`
- `[string, any] -> []`

*Precedence:* suffix

```
type Point record [x:number y:number]
make Point [1 2]                           => {x:1,y:2}

type OptNum (number or none)
OptNum unify 5                             => 5 true
OptNum unify none                          => none true

type Nums [:number]
Nums unify [1,2,3]                         => [1,2,3] true

type Num number
Num unify 42                               => 42 true
```

### Evaluation Words

#### `do`

Evaluate quoted list or map contents. Lists are evaluated as a token
stream; maps have their list values evaluated depth-first.

*Signatures:*
- `[list] -> [results...]`
- `[map] -> [map]`

*Precedence:* suffix

**List evaluation** — elements are executed as if they were tokens in
the main program:

```
do [1 add 2]                               => 3
do [upper "hello"]                         => 'HELLO'
do [1 2 3]                                 => 1 2 3
```

**Map evaluation** — list values are evaluated, non-list values pass
through unchanged. Nested maps are processed recursively:

```
do {x:[3 add 4], y:[upper "a"]}           => {x:7, y:'A'}
do {x:1, y:"hello"}                        => {x:1, y:'hello'}
do {m:{x:[5 mul 2]}}                       => {m:{x:10}}
```


#### `typeof`

Return the base type of a value as an atom.

*Signature:* `[any] -> [atom]`

```
typeof 42              => number
typeof "hello"         => string
typeof true            => boolean
typeof [1 2 3]         => list
typeof {x:1}           => map
typeof none            => none
```

#### `base`

Return the zero/default value of a type, similar to Go's zero values.

*Signature:* `[type-literal] -> [value]`

```
base integer           => 0
base number            => 0
base string            => ''
base boolean           => false
base list              => []
base map               => {}
base none              => none
```


### Conditional Words

#### `if`

Conditional evaluation, analogous to the ternary operator. Evaluates
the condition, then evaluates only the matching branch. Unevaluated
branches produce no side effects.

*Signatures:*
- `[any, any, any] -> [any]` — 3-arg: condition, then-branch, else-branch
- `[any, any] -> [any]` — 2-arg: condition, then-branch (returns nothing if false)

*Precedence:* suffix

**Condition evaluation:** If the condition is a list, it is evaluated
as code (like `do`). The result is then tested for truthiness.

**Truthiness rules** (same as `convert boolean`):
- `false`, `0`, `""`, `none`, empty list, empty map → **falsy**
- `true`, non-zero numbers, non-empty strings → **truthy**

**Branch evaluation:** If a branch is a list, it is evaluated as code.
Scalar branch values are returned as-is. Only the matching branch is
evaluated — the other is never executed.

**3-arg form** (if-then-else):

```
if true 1 2                     => 1
if false 1 2                    => 2
if true "yes" "no"              => 'yes'
if false "yes" "no"             => 'no'
```

**2-arg form** (if-then, no else):

```
if true 42                      => 42
if false 42                     =>        # empty stack
```

**List conditions** — evaluated as code:

```
if [1 lt 2] [3 add 4] [5 add 6]    => 7
if [2 lt 1] [3 add 4] [5 add 6]    => 11
```

**List branches** — evaluated as code:

```
if true [1 add 2] [3 add 4]    => 3
if false [1 add 2] [3 add 4]   => 7
```

**Lazy evaluation** — only the matching branch is evaluated:

```
if true 1 [10 div 0]           => 1       # no division error
if false [10 div 0] 2          => 2       # no division error
```

**Falsy values:**

```
if 0 1 2                       => 2
if "" 1 2                      => 2
if none 1 2                    => 2
```

**Truthy values:**

```
if 1 10 20                     => 10
if "yes" 10 20                 => 10
if 42 10 20                    => 10
```

**Nested:**

```
if true (if false 1 2) 3       => 2
if false 1 (if true 2 3)       => 2
```


### File I/O Words

File operations use an internal `FileOps` interface rather than calling
the Go `os` package directly. The default implementation uses the real
file system with the process working directory for relative paths. A
`MemFileOps` implementation is available for testing.

File format handling is dispatched through a pluggable `Format`
interface. Built-in formats are `text`, `json`, `jsonic`, and `lines`.
Host applications can register custom formats via `RegisterFormat`.

#### `read`

Read a file and push its contents onto the stack. By default returns
a string with line endings normalized to `\n`.

*Signatures:*
- `[string] -> [string]` — read file at path
- `[string, map] -> [string|list|map]` — read with options

*Precedence:* suffix

```
read "data.txt"                         # read as text
"data.txt" read                         # prefix style
read "config.json" {fmt:"json"}         # parse JSON to map/list
read "config.jsonic" {fmt:"jsonic"}     # parse with jsonic (relaxed JSON)
read "data.txt" {fmt:"lines"}           # split into list of strings
read "raw.bin" {nl:"raw"}              # no line ending normalization
```

**Options map:**

| Key    | Default   | Values                                     |
|--------|-----------|--------------------------------------------|
| `enc`  | `"utf8"`  | `"utf8"`, `"binary"`, `"latin1"`           |
| `fmt`  | `"text"`  | `"text"`, `"json"`, `"jsonic"`, `"lines"`  |
| `nl`   | `"lf"`    | `"lf"`, `"crlf"`, `"raw"`                 |

**Format details:**

- `text` — raw string, no parsing
- `json` — parse JSON to AQL map/list
- `jsonic` — parse with jsonic (unquoted keys, trailing commas, etc.)
- `lines` — split on `\n` into a list of strings

**Line ending normalization:**

- `"lf"` (default) — normalize all `\r\n` and `\r` to `\n`
- `"crlf"` — normalize all to `\r\n`
- `"raw"` — no normalization, content preserved as-is

#### `write`

Write content to a file. Returns the path written.

*Signatures:*
- `[string, string] -> [string]` — path, content -> path
- `[string, string, map] -> [string]` — path, content, options -> path
- `[string, any, map] -> [string]` — path, data, options (auto-serializes)

*Precedence:* suffix

```
write "out.txt" "hello world"
write "out.txt" (upper "hello")
write "log.txt" "entry\n" {mode:"append"}
write "out.txt" "a\nb\n" {nl:"crlf"}
```

**Options map:**

| Key    | Default   | Values                                     |
|--------|-----------|--------------------------------------------|
| `enc`  | `"utf8"`  | `"utf8"`, `"binary"`, `"latin1"`           |
| `fmt`  | `"text"`  | `"text"`, `"json"`, `"jsonic"`, `"lines"`  |
| `mode` | `"write"` | `"write"` (truncate), `"append"`           |
| `nl`   | `"lf"`    | `"lf"`, `"crlf"`, `"raw"`                 |

**Note:** With two string arguments of the same type, prefer suffix
style (`write "path" "content"`) for clarity. The infix form
`"content" write "path"` is ambiguous because the engine cannot
distinguish path from content when both are strings.


## Type System

Types form a slash-separated hierarchy. A child type matches a parent
pattern; a parent does not match a child pattern.

```
scalar                  virtual supertype of all scalar types
  string
    string/proper       non-empty strings
    string/empty        the empty string ""
  number
    number/integer      integer values
  boolean
    boolean/true        the value true
    boolean/false       the value false
  atom                  bare unquoted words (no function signature)

list                    lists
map                     maps
any                     matches all data types
none                    matches only itself
```

The `scalar` type matches `string`, `number`, `boolean`, and `atom`
(and all their subtypes). It is useful in function signatures that
accept any scalar value.

The `atom` type represents bare words that do not resolve to a
registered function, type name, or boolean. Previously such words were
coerced to strings; now they retain their identity as atoms. Atoms
display without quotes.

The `any` type matches all data types but not internal types (`word`,
`forward`, `paren`).

When matching function signatures, the most specific match wins:
longest argument list with narrowest types.


## Error Codes

| Code              | Meaning                                          |
|-------------------|--------------------------------------------------|
| `syntax_error`    | Unmatched parenthesis or malformed input          |
| `signature_error` | No matching signature for the given arguments     |
| `division_by_zero`| Division by zero in `div`                         |
| `modulo_by_zero`  | Modulo by zero in `mod`                           |
| `cannot_compare`  | Ordering comparison on incompatible or non-orderable types |
| `read`            | File read error (not found, invalid format, etc.) |
| `write`           | File write error                                  |
