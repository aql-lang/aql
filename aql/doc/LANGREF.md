
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

**Strings** are enclosed in double or single quotes. Unrecognised bare
words are coerced to strings.

```
"hello world"   => 'hello world'
'single'        => 'single'
""              => ''
foo             => 'foo'
```

**Booleans** are the bare words `true` and `false`.

```
true    => true
false   => false
```

**Type literals** name a type without carrying a value.

```
number    string    boolean    any    none    list    map
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

| Precedence | Words              |
|------------|--------------------|
| 2 (high)   | `mul`, `div`, `mod`|
| 1 (low)    | `add`, `sub`       |

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

Convert a string to uppercase.

*Signature:* `[string] -> [string]`
*Precedence:* suffix

```
"hello" upper       => 'HELLO'
upper "abc"         => 'ABC'
a upper             => 'A'
```

#### `lower`

Convert a string to lowercase.

*Signature:* `[string] -> [string]`
*Precedence:* suffix

```
"WORLD" lower       => 'world'
lower "ABC"         => 'abc'
lower B             => 'b'
```

### Arithmetic Words

All arithmetic words take two integers and produce one integer.

*Signature:* `[integer, integer] -> [integer]`

#### `add`

Addition. Precedence 1.

```
1 2 add             => 3
1 add 2             => 3
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
4 sq x                      => 16 'x'
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

Supported type names in signatures: `any`, `none`, `number`,
`integer`, `string`, `boolean`, `list`, `map`.

#### `undef`

Remove the most recent definition of a word. If definitions were
stacked, the previous one is revealed.

*Signatures:*
- `[word] -> []`
- `[string] -> []`

*Precedence:* suffix

```
def foo 1 foo undef foo foo             => 1 'foo'

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
5 var [[x] x mul x] x                 => 25 'x'
```

After `var` completes, `x` reverts to an unknown word (string `'x'`).

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


## Type System

Types form a slash-separated hierarchy. A child type matches a parent
pattern; a parent does not match a child pattern.

```
string
  string/proper         non-empty strings
  string/empty          the empty string ""

number
  number/integer        integer values

boolean
  boolean/true          the value true
  boolean/false         the value false

list                    lists
map                     maps
any                     matches all data types
none                    matches only itself
```

The `any` type matches all data types but not internal types (`word`,
`forward`, `paren`).

When matching function signatures, the most specific match wins:
longest argument list with narrowest types.


## Error Codes

| Code              | Meaning                                |
|-------------------|----------------------------------------|
| `syntax_error`    | Unmatched parenthesis or malformed input |
| `signature_error` | No matching signature for the given arguments |
| `division_by_zero`| Division by zero in `div`              |
| `modulo_by_zero`  | Modulo by zero in `mod`                |
