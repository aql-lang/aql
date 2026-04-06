
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

**Template strings** use backticks and support `${...}` interpolation.
Expressions inside `${...}` are evaluated and their results are
converted to strings and concatenated with the surrounding literal text.

```
`hello`                     => 'hello'
`value is ${1 add 2}`       => 'value is 3'
def x 42; `x = ${x}`       => 'x = 42'
`${a} and ${b}`             => interpolates both a and b
`price: $100`               => 'price: $100'   ($ alone is literal)
```

Template strings nest: an interpolation expression can itself contain
a template string with its own interpolations, to any depth.

```
`a${`inner ${1}`}b`         => 'ainner 1b'
```

Escape sequences in template strings: `\\`, `` \` ``, `\$`, `\n`,
`\t`, `\r`. Use `\$` to include a literal `${` without triggering
interpolation.

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
number    integer    string    boolean    atom    scalar    any    none    list    map
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
subsequent value is collected as a forward argument until the word has
everything it needs. Then the word executes.

```
add 1 2         => 3
```

1. `add` — needs two arguments, stack is empty. Wait for forward values.
2. `1` — collected as first argument to `add`.
3. `2` — collected as second argument. `add` now has both; it executes.

This is why the same word works in prefix, forward, and infix position:

```
1 2 add         => 3       # prefix: both args already on the stack
add 1 2         => 3       # forward: both args collected after the word
1 add 2         => 3       # infix: 1 from the stack, 2 collected after
```

### Nested forward collection

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

### Left-to-right evaluation

When two words are both waiting for forward arguments, evaluation
proceeds strictly left-to-right. Each word collects its arguments
before the next word executes.

```
2 add 3 mul 4           => 20
```

`add` collects `3` as its second argument: 2 + 3 = 5. Then `mul`
collects `4`: 5 * 4 = 20. Use parentheses to control evaluation order:

```
2 add (3 mul 4)         => 14
```

### The `end` keyword

Sometimes you need to stop forward collection early — for example, when
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

By default most words have *forward precedence*: when prefix arguments
are available they are tried first; forward collection is the fallback.
Stack-manipulation words (`dup`, `swap`, `drop`) are stack-only by
default.


## Word Modifiers

Append a modifier to a word name to override argument behaviour.

| Modifier | Meaning                              |
|----------|--------------------------------------|
| `/s`     | Force stack-only arguments          |
| `/f`     | Force forward-only arguments          |
| `/N`     | Expect exactly N arguments           |
| `/Nf`    | Expect N arguments, forward only      |
| `/Ns`    | Expect N arguments, stack only      |

```
lower/f "E"     => 'e'
"F" lower/s     => 'f'
lower/1 "G"     => 'g'
lower/1f "H"    => 'h'
```


## Evaluation Order

All operations evaluate strictly left-to-right. Use parentheses to
control evaluation order when needed.

```
2 add 3 mul 4               => 20      # (2+3)*4, left-to-right
2 mul 3 add 4               => 10      # (2*3)+4
1 add 2 mul 3 add 4         => 13      # ((1+2)*3)+4
2 add (3 mul 4)             => 14      # parens force mul first
```


## The `end` Keyword

`end` terminates forward argument collection for the nearest pending
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
*Precedence:* forward

```
"hello" upper       => 'HELLO'
upper "abc"         => 'ABC'
a upper             => 'A'
```

#### `lower`

Convert a string or atom to lowercase.

*Signatures:* `[string] -> [string]`, `[atom] -> [string]`
*Precedence:* forward

```
"WORLD" lower       => 'world'
lower "ABC"         => 'abc'
lower B             => 'b'
```

#### `concat`

Concatenate list elements into a single string. Each element is
converted to its string representation. Use an options map to set a
separator or to skip empty/nullish parts.

*Signatures:* `[list] -> [string]`, `[list, map] -> [string]`
*Precedence:* forward

```
["a","b","c"] concat                          => 'abc'
["a","b","c"] {sep:", "} concat               => 'a, b, c'
[1,2,3] {sep:"-"} concat                      => '1-2-3'
["a","","c"] {skipEmpty:true} concat           => 'ac'
```

Options: `sep` (string), `skipEmpty` (bool), `skipNullish` (bool).

#### `split`

Split a string into a list of parts by a separator.

*Signatures:* `[string, string] -> [list]`, `[string, string, map] -> [list]`
*Precedence:* forward

```
"a,b,c" "," split                             => ['a','b','c']
"hello world" " " split                       => ['hello','world']
"a,,b" "," {keepEmpty:true} split             => ['a','','b']
"hello" "" split                               => ['h','e','l','l','o']
" a : b : c " ":" {trimParts:true} split       => ['a','b','c']
```

Options: `cs`, `mode`, `lim`, `keepEmpty`, `trimParts`, `u`, `norm`.

#### `trim`

Trim whitespace or specific characters from a string.

*Signatures:*
- `[string] -> [string]`
- `[string, map] -> [string]`
- `[atom] -> [string]`
- `[atom, map] -> [string]`

*Precedence:* forward

```
"  hello  " trim                               => 'hello'
"xxhelloxx" {chars:"x"} trim                   => 'hello'
"  hello  " {side:"left"} trim                 => 'hello  '
"  hello  " {side:"right"} trim                => '  hello'
```

Options: `side` (left/right/both), `chars`, `cs`, `u`, `norm`.

#### `contains`

Test whether a string contains a search term.

*Signatures:* `[string, string] -> [boolean]`, `[string, string, map] -> [boolean]`
*Precedence:* forward

```
"hello world" "world" contains                           => true
"hello world" "xyz" contains                             => false
"Hello" "hello" {cs:"insensitive"} contains              => true
"hello world" "hello" {anchored:"start"} contains        => true
"hello world" "world" {anchored:"end"} contains          => true
```

Options: `cs`, `mode`, `anchored`, `wholeWord`, `u`, `norm`.

#### `indexof`

Find the byte index of a search term in a string. Returns -1 if not
found.

*Signatures:* `[string, string] -> [integer]`, `[string, string, map] -> [integer]`
*Precedence:* forward

```
"hello world" "world" indexof                            => 6
"hello world" "xyz" indexof                              => -1
"abcabc" "abc" {occ:"last"} indexof                      => 3
"HELLO" "hello" {cs:"insensitive"} indexof               => 0
```

Options: `cs`, `mode`, `from`, `occ` (first/last), `u`, `norm`.

#### `replace`

Replace occurrences of a search term in a string.

*Signatures:*
- `[string, string, string] -> [string]`
- `[string, string, string, map] -> [string]`

*Precedence:* forward

```
"hello world" "world" "earth" replace                    => 'hello earth'
"aaa" "a" "b" {scope:"all"} replace                      => 'bbb'
"Hello" "hello" "hi" {cs:"insensitive"} replace          => 'hi'
"aaa" "a" "b" {scope:"all",count:2} replace              => 'bba'
```

Options: `cs`, `mode`, `scope` (first/all), `from`, `count`, `u`, `norm`.

#### `slice`

Extract a substring by numeric position.

*Signatures:*
- `[string, integer] -> [string]`
- `[string, integer, integer] -> [string]`
- `[string, integer, map] -> [string]`
- `[string, integer, integer, map] -> [string]`

*Precedence:* forward

```
"hello" 0 3 slice                                       => 'hel'
"hello" 2 slice                                         => 'llo'
"hello" -3 slice                                        => 'llo'
"hello" 1 -1 slice                                      => 'ell'
```

Negative indices are Python-style: -1 means one before the end.
Options: `unit` (code-unit/code-point), `fromEnd`, `u`, `norm`.

#### `changecase`

Apply a casing transformation to a string. Defaults to `"lower"`.

*Signatures:*
- `[string] -> [string]`
- `[string, map] -> [string]`
- `[atom] -> [string]`
- `[atom, map] -> [string]`

*Precedence:* forward

```
"Hello World" changecase                                => 'hello world'
"hello world" {style:"upper"} changecase                => 'HELLO WORLD'
"hello world" {style:"title"} changecase                => 'Hello World'
"hello world" {style:"capitalize"} changecase           => 'Hello world'
"HELLO WORLD" {style:"sentence"} changecase             => 'Hello world'
```

Styles: `lower`, `upper`, `capitalize`, `title`, `sentence`, `fold`.
Options: `style`, `u`, `norm`, `loc`.

#### `normalize`

Normalize Unicode and optionally clean whitespace and line endings.

*Signatures:* `[string] -> [string]`, `[string, map] -> [string]`
*Precedence:* forward

```
"café" normalize                                        => 'café'
"  hello  " {trim:true} normalize                       => 'hello'
"a  b   c" {collapseWs:true} normalize                  => 'a b c'
"hello" {form:"NFD"} normalize                          => 'hello'
```

Options: `form` (NFC/NFD/NFKC/NFKD), `trim`, `collapseWs`,
`eol` (preserve/lf/crlf).

#### `repeat`

Repeat a string a fixed number of times.

*Signatures:* `[string, integer] -> [string]`, `[string, integer, map] -> [string]`
*Precedence:* forward

```
"ab" 3 repeat                                           => 'ababab'
"ha" 3 {sep:" "} repeat                                 => 'ha ha ha'
"-" 5 repeat                                            => '-----'
"x" 0 repeat                                            => ''
```

Options: `sep`.

#### `pad`

Pad a string to a desired length. Defaults to right-padding with
spaces.

*Signatures:* `[string, integer] -> [string]`, `[string, integer, map] -> [string]`
*Precedence:* forward

```
"hi" 5 pad                                              => 'hi   '
"hi" 5 {side:"left"} pad                                => '   hi'
"hi" 6 {side:"both"} pad                                => '  hi  '
"hi" 5 {fill:"."} pad                                   => 'hi...'
"hi" 5 {side:"left",fill:"0"} pad                       => '000hi'
"hello world" 5 {trunc:true} pad                        => 'hello'
```

Options: `side` (left/right/both), `fill`, `trunc`.

#### `match`

Match a pattern and return a structured result map with fields: `ok`
(boolean), `ms` (list of match maps), `fst` (first match), `lst`
(last match), `n` (count). Each match map has `m` (matched text), `i`
(start index), `e` (end index).

*Signatures:* `[string, string] -> [map]`, `[string, string, map] -> [map]`
*Precedence:* forward

```
"hello world" "world" match .ok                         => true
"hello world" "world" match .fst .m                     => 'world'
"hello world" "xyz" match .ok                           => false
"abab" "ab" {scope:"all"} match .n                      => 2
```

Options: `cs`, `mode`, `scope` (first/all), `u`, `norm`.

#### `escape`

Escape a string for safe use in shells and text tools.

*Signatures:* `[string] -> [string]`, `[string, map] -> [string]`
*Precedence:* forward

```
"hello world" escape                                    => 'hello\ world'
"a.b" {tgt:"sed"} escape                                => 'a\.b'
"a*b" {tgt:"grep"} escape                               => 'a\*b'
```

Options: `tgt` (sh/bash/sed/awk/grep), `quote` (none/single/double).

### Arithmetic Words

Arithmetic words operate on integers and decimals. When both operands
are integers the result is an integer; when either is a decimal the
result is a decimal.

#### `add`

Addition for numbers; string concatenation when at least one argument
is a non-numeric scalar.

*Signatures:*
- `[integer, integer] -> [integer]`
- `[decimal, decimal] -> [decimal]`
- `[scalar, scalar] -> [string]` — string concatenation

```
1 2 add             => 3
2.5 1.5 add         => 4
1 add 2             => 3
"hello" add " world"    => 'hello world'
"count: " add 42        => 'count: 42'
```

#### `sub`

Subtraction. The first argument is the minuend, the
second is the subtrahend.

*Signatures:* `[integer, integer] -> [integer]`, `[decimal, decimal] -> [decimal]`

```
10 3 sub            => 7
10 sub 3            => 7
10.5 3.0 sub        => 7.5
```

#### `mul`

Multiplication.

*Signatures:* `[integer, integer] -> [integer]`, `[decimal, decimal] -> [decimal]`

```
4 5 mul             => 20
4 mul 5             => 20
2.5 4.0 mul         => 10
```

#### `div`

Division. Integer division truncates toward zero.
Division by zero is an error.

*Signatures:* `[integer, integer] -> [integer]`, `[decimal, decimal] -> [decimal]`

```
10 3 div            => 3
10 div 3            => 3
7.0 2.0 div         => 3.5
10 div 0            => ERROR: division_by_zero
```

#### `mod`

Modulo. Modulo by zero is an error.

*Signatures:* `[integer, integer] -> [integer]`, `[decimal, decimal] -> [decimal]`

```
10 3 mod            => 1
10 mod 3            => 1
10 mod 0            => ERROR: modulo_by_zero
```

#### `abs`

Absolute value (unary).

*Signatures:* `[integer] -> [integer]`, `[decimal] -> [decimal]`

```
-5 abs              => 5
5 abs               => 5
abs -3              => 3
-2.5 abs            => 2.5
```

#### `negate`

Negate a number (unary).

*Signatures:* `[integer] -> [integer]`, `[decimal] -> [decimal]`

```
5 negate            => -5
-3 negate           => 3
negate 7            => -7
2.5 negate          => -2.5
```

#### `min`

Return the smaller of two numbers.

*Signatures:* `[integer, integer] -> [integer]`, `[decimal, decimal] -> [decimal]`

```
3 min 5             => 3
5 min 3             => 3
```

#### `max`

Return the larger of two numbers.

*Signatures:* `[integer, integer] -> [integer]`, `[decimal, decimal] -> [decimal]`

```
3 max 5             => 5
5 max 3             => 5
```

#### `pow`

Raise a number to a power.

*Signatures:* `[integer, integer] -> [integer]`, `[decimal, decimal] -> [decimal]`

```
2 10 pow            => 1024
3 3 pow             => 27
5 0 pow             => 1
10 2 pow            => 100
```

Negative exponents produce an error for integer `pow`.

#### `sign`

Return the sign of a number: -1 for negative, 0 for zero, 1 for
positive.

*Signatures:* `[integer] -> [integer]`, `[decimal] -> [decimal]`

```
-7 sign             => -1
0 sign              => 0
42 sign             => 1
-2.5 sign           => -1
```

### Rounding Words

#### `ceil`

Round a decimal up to the nearest integer.

*Signature:* `[decimal] -> [integer]`

```
2.3 ceil            => 3
2.7 ceil            => 3
-2.3 ceil           => -2
-2.7 ceil           => -2
```

#### `floor`

Round a decimal down to the nearest integer.

*Signature:* `[decimal] -> [integer]`

```
2.7 floor           => 2
2.3 floor           => 2
-2.3 floor          => -3
-2.7 floor          => -3
```

#### `round`

Round a decimal to the nearest integer. Ties round away from zero.

*Signature:* `[decimal] -> [integer]`

```
2.7 round           => 3
2.3 round           => 2
2.5 round           => 3
-2.5 round          => -3
```

#### `trunc`

Truncate a decimal toward zero, removing the fractional part.

*Signature:* `[decimal] -> [integer]`

```
2.9 trunc           => 2
-2.9 trunc          => -2
0.5 trunc           => 0
-0.5 trunc          => 0
```

### Roots, Exponentials, and Logarithms

#### `sqrt`

Compute the square root.

*Signatures:* `[integer] -> [decimal]`, `[decimal] -> [decimal]`

```
9 sqrt              => 3
4 sqrt              => 2
2 sqrt              => 1.4142135623730951
0 sqrt              => 0
```

#### `cbrt`

Compute the cube root.

*Signatures:* `[integer] -> [decimal]`, `[decimal] -> [decimal]`

```
27 cbrt             => 3
8 cbrt              => 2
1 cbrt              => 1
0 cbrt              => 0
```

#### `exp`

Compute *e* raised to a power.

*Signatures:* `[integer] -> [decimal]`, `[decimal] -> [decimal]`

```
0 exp               => 1
1 exp               => 2.718281828459045
2 exp               => 7.38905609893065
-1 exp              => 0.36787944117144233
```

#### `log`

Compute the natural logarithm (base *e*).

*Signatures:* `[integer] -> [decimal]`, `[decimal] -> [decimal]`

```
1 log               => 0
math-e log          => 1
10 log              => 2.302585092994046
100 log             => 4.605170185988092
```

#### `log2`

Compute the base-2 logarithm.

*Signatures:* `[integer] -> [decimal]`, `[decimal] -> [decimal]`

```
8 log2              => 3
1 log2              => 0
1024 log2           => 10
2 log2              => 1
```

#### `log10`

Compute the base-10 logarithm.

*Signatures:* `[integer] -> [decimal]`, `[decimal] -> [decimal]`

```
100 log10           => 2
1000 log10          => 3
1 log10             => 0
10 log10            => 1
```

### Trigonometric Words

All trigonometric words work in radians.

#### `sin`

Compute the sine.

*Signatures:* `[integer] -> [decimal]`, `[decimal] -> [decimal]`

```
0 sin               => 0
1 sin               => 0.8414709848078965
```

#### `cos`

Compute the cosine.

*Signatures:* `[integer] -> [decimal]`, `[decimal] -> [decimal]`

```
0 cos               => 1
math-pi cos         => -1
```

#### `tan`

Compute the tangent.

*Signatures:* `[integer] -> [decimal]`, `[decimal] -> [decimal]`

```
0 tan               => 0
1 tan               => 1.557407724654902
```

#### `asin`

Compute the arc sine. Input must be in [-1, 1].

*Signatures:* `[integer] -> [decimal]`, `[decimal] -> [decimal]`

```
0 asin              => 0
1 asin              => 1.5707963267948966
```

#### `acos`

Compute the arc cosine. Input must be in [-1, 1].

*Signatures:* `[integer] -> [decimal]`, `[decimal] -> [decimal]`

```
1 acos              => 0
0 acos              => 1.5707963267948966
-1 acos             => 3.141592653589793
```

#### `atan`

Compute the arc tangent.

*Signatures:* `[integer] -> [decimal]`, `[decimal] -> [decimal]`

```
0 atan              => 0
1 atan              => 0.7853981633974483
```

#### `atan2`

Compute the two-argument arc tangent. Handles quadrant correctly:
`y x atan2`.

*Signature:* `[number, number] -> [decimal]`


```
1 1 atan2           => 0.7853981633974483
1 0 atan2           => 1.5707963267948966
0 1 atan2           => 0
```

#### `hypot`

Compute the hypotenuse length: `sqrt(x*x + y*y)` without overflow.

*Signature:* `[number, number] -> [decimal]`


```
3 4 hypot           => 5
5 12 hypot          => 13
1 1 hypot           => 1.4142135623730951
```

### Math Constants

#### `math-pi`

Push the constant *π* (3.14159...). Stack-only.

*Signature:* `[] -> [decimal]`

```
math-pi             => 3.141592653589793
math-pi 2 mul       => 6.283185307179586
```

#### `math-e`

Push Euler's number *e* (2.71828...). Stack-only.

*Signature:* `[] -> [decimal]`

```
math-e              => 2.718281828459045
math-e log          => 1
```

### Boolean Words

#### `or`

Logical OR for booleans; disjunction (type union) for non-boolean
values.

*Signatures:*
- `[boolean, boolean] -> [boolean]` — logical OR
- `[any, any] -> [disjunct]` — type union

```
true or false           => true
false or false          => false
```

**Disjunction (type union):** When used with non-boolean values, `or`
creates a disjunct type that matches any of its alternatives.

```
string or none                  => string|none
number or string or boolean     => number|string|boolean
```

#### `and`

Logical AND.

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

Exclusive OR.

*Signature:* `[boolean, boolean] -> [boolean]`

```
true xor false          => true
true xor true           => false
```

#### `nand`

Logical NAND (NOT AND).

*Signature:* `[boolean, boolean] -> [boolean]`

```
true nand true          => false
true nand false         => true
```

#### `implies`

Logical implication (a → b). False only when the first argument is
true and the second is false.

*Signature:* `[boolean, boolean] -> [boolean]`

```
true implies false      => false
false implies true      => true
```

### Comparison Words

Comparison words take two arguments with forward precedence.
They use natural type comparisons: integers
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


```
2 gt 1                  => true
1 gt 2                  => false
"def" gt "abc"          => true
```

#### `lte`

Less than or equal.

*Signature:* `[any, any] -> [boolean]`


```
1 lte 2                 => true
1 lte 1                 => true
2 lte 1                 => false
```

#### `gte`

Greater than or equal.

*Signature:* `[any, any] -> [boolean]`


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


```
1 eq 1                  => true
1 eq 2                  => false
"abc" eq "abc"          => true
none eq none            => true
true eq false           => false
```

#### `neq`

Not equal. The negation of `eq`. Returns `true` when the two values
are not exactly equal, `false` when they are.

*Signature:* `[any, any] -> [boolean]`


```
1 neq 2                 => true
1 neq 1                 => false
"abc" neq "xyz"         => true
"abc" neq "abc"         => false
```

#### `deq`

Deep equality. Traverses lists and maps depth-first, comparing all
leaf values. Two lists are deeply equal if they have the same length
and each element is deeply equal. Two maps are deeply equal if they
have the same keys and each value is deeply equal.

*Signature:* `[any, any] -> [boolean]`


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
specifies a variant (string shorthand) or a settings map.

*Signatures:*
- `[any, any] -> [scalar]` — 2-arg form: value and target type
- `[any, any, any] -> [scalar]` — 3-arg form: value, target type, variant or settings map

*Precedence:* forward

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

**Settings map form.** The third argument can be a map with `base`
and/or `size` keys:

| Key    | Default | Values                               |
|--------|---------|--------------------------------------|
| `base` | (none)  | `"hex"`, `"HEX"`, `"bin"`, `"oct"`  |
| `size` | 222     | Max output string length             |

```
convert 10 string {base:hex}              => 'a'
convert "hello" string {size:3}           => 'hel'
convert 255 string {base:hex, size:1}     => 'f'
```

### Stack Words

Stack words are stack-only by default.

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

#### `over`

Copy the second value over the top.

*Signature:* `[any, any] -> [any, any, any]`

```
1 2 over            => 1 2 1
```

#### `rot`

Rotate the third value to the top.

*Signature:* `[any, any, any] -> [any, any, any]`

```
1 2 3 rot           => 2 3 1
```

#### `nip`

Remove the second value, keeping the top.

*Signature:* `[any, any] -> [any]`

```
1 2 nip             => 2
```

#### `tuck`

Copy the top value below the second.

*Signature:* `[any, any] -> [any, any, any]`

```
1 2 tuck            => 2 1 2
```

#### `2dup`

Duplicate the top two values.

*Signature:* `[any, any] -> [any, any, any, any]`

```
1 2 2dup            => 1 2 1 2
```

#### `2swap`

Swap the top two pairs.

*Signature:* `[any, any, any, any] -> [any, any, any, any]`

```
1 2 3 4 2swap       => 3 4 1 2
```

#### `2drop`

Remove the top two values.

*Signature:* `[any, any] -> []`

```
1 2 2drop           =>
```

#### `2over`

Copy the second pair over the top pair.

*Signature:* `[any, any, any, any] -> [any, any, any, any, any, any]`

```
1 2 3 4 2over       => 1 2 3 4 1 2
```

#### `depth`

Push the current stack depth (number of items on the stack).

*Signature:* `[] -> [integer]`

```
1 2 3 depth         => 1 2 3 3
depth               => 0
```

#### `pick`

Copy the nth item from the top of the stack (0-indexed).

*Signature:* `[integer] -> [any]`

```
1 2 3 0 pick        => 1 2 3 3
1 2 3 2 pick        => 1 2 3 1
```

#### `roll`

Rotate the nth item to the top of the stack (0-indexed).

*Signature:* `[integer] -> []`

```
1 2 3 2 roll        => 2 3 1
1 2 3 1 roll        => 1 3 2
```

### Storage Words

#### `context`

Push the current context Store onto the stack. The context is a
mutable Store (Object/Store) with prototype chain resolution for
nested scopes.

*Signature:* `[] -> [Store]`

```
context                     => Store
```

#### `set`

Store a value under a key in an explicit Store. The key may be a bare
word or a string.

*Signatures:*
- `[string, any, Store] -> []`
- `[atom, any, Store] -> []`

*Precedence:* forward

```
context set foo 99
context set bar "hello"
context set "key" 42
```

#### `get` (alias `.`)

Retrieve a value by key from a Store, Map, List, or Object. For Store
lookups, key resolution walks the prototype chain. For Maps and
Objects, returns None if the key is missing. The `.` operator is an
alias. Dot notation `foo.bar` is expanded by the parser to `get bar`.

*Signatures:*
- `[string, Store] -> [any]` — Store lookup
- `[atom, Store] -> [any]`
- `[atom, Node] -> [any]` — Map property / List index
- `[string, Node] -> [any]`
- `[integer, Node] -> [any]`
- `[atom, Object] -> [any]` — Object field access
- `[string, Object] -> [any]`
- `[integer, Object] -> [any]`
- `[any, None] -> [None]` — None propagation

*Precedence:* forward

```
context set foo 99 end context get foo    => 99
{x:1,y:2} get x                          => 1
{x:1,y:2} . x                            => 1
[10,20,30] . 1                            => 20
none . x                                  => none
```

### Type Words

#### `unify`

Attempt to unify two values. Pushes the unified value and a boolean
indicating success.

*Signature:* `[any, any] -> [any, boolean]`
*Precedence:* forward

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

*Precedence:* forward

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

Function definitions support all argument styles (prefix, forward,
infix):

```
def sq fn [[x:number] [number] [x mul x]]
5 sq                        => 25       # prefix
sq 6                        => 36       # forward
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

*Precedence:* forward

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
*Precedence:* forward

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
*Precedence:* forward

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

#### `call`

Evaluate a list as code on the current stack. Similar to `do` but
designed for invoking callback lists in higher-order patterns.

*Signature:* `[list] -> [any...]`
*Precedence:* forward

```
5 [dup mul] call            => 25
2 3 [add] call              => 5
"hello" [upper] call        => 'HELLO'
1 2 [add 10 mul] call       => 30
```

#### `args`

Push the current function's argument list onto the stack. Only
available inside a `fn`-defined function. Stack-only.

*Signature:* `[] -> [list]`

```
def show fn [[x:number] [] [args]]
42 show                     => [42]

def f fn [[a:number,b:number] [] [args]]
1 2 f                       => [1,2]
```

### Record Type Words

#### `record`

Create a record type from a list of field pairs. Each element is a
pair (single-key map) defining a field name and its type constraint.
A record type is a schema that validates maps: it requires exactly the
specified keys, each with a value matching the field's type constraint.

*Signature:* `[list] -> [record-type]`
*Precedence:* forward

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
*Precedence:* forward

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

*Precedence:* forward

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

#### `make`

Create instances of record types, table types, or convert scalar
values. Takes a type and a value (with an optional options map).

*Signatures:*
- `[any, any] -> [any]` — type and value
- `[any, any, map] -> [any]` — type, value, and options

*Precedence:* forward

**Scalar conversion:**

```
make string 42                 => '42'
make number "99"               => 99
make boolean 1                 => true
```

**Record creation (positional):**

```
type Point record [x:number y:number]
make Point [1 2]               => {x:1,y:2}
```

**Record creation (named):**

```
type Point record [x:number y:number]
make Point {x:1 y:2}          => {x:1,y:2}
```

**Table creation:**

```
type Row record [x:integer y:string]
type T table Row
make T [[1 a] [2 b]]          => [{x:1,y:'a'},{x:2,y:'b'}]
```

**Options map** with `base:true` fills missing fields with their
type's zero value:

```
type Item record [name:string qty:number]
make Item {name:"Widget"} {base:true}     => {name:'Widget',qty:0}
```

### Evaluation Words

#### `do`

Evaluate quoted list or map contents. Lists are evaluated as a token
stream; maps have their list values evaluated depth-first.

*Signatures:*
- `[list] -> [results...]`
- `[map] -> [map]`

*Precedence:* forward

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

### Data Access Words

See `get` (alias `.`) under Storage Words above.

#### `getr` (alias `!.`)

Strict variant of `get`. Same access patterns but errors when the
target is `none` or when the key/index is missing. Works on Maps,
Lists, and Objects.

*Signatures:*
- `[map, atom] -> [any]`
- `[map, string] -> [any]`
- `[list, integer] -> [any]`
- `[map, integer] -> [any]`
- `[object, atom] -> [any]`
- `[object, string] -> [any]`
- `[object, integer] -> [any]`
- `[none, any] -> ERROR`

```
{x:1,y:2} x getr           => 1
{x:1,y:2} x !.             => 1
none x !.                   => ERROR
```

### Inspection Words

#### `inspect`

Return an introspection map for a word, containing its name, kind
(`builtin` or `defined`), whether it has forward precedence, and its
list of signatures.

*Signature:* `[word] -> [map]`

```
inspect add    => {name:'add', kind:builtin, forward_precedence:true, signatures:[...]}
```

### Output Words

#### `print`

Print a value to the output writer, followed by a newline. Strings
are printed as-is (no quotes); maps and lists use JSON-like formatting;
tables are printed as aligned text with column headers. The value is
consumed (removed from the stack).

*Signature:* `[any] -> []`
*Precedence:* forward

```
print "hello"               # outputs: hello\n
print 42                    # outputs: 42\n
print {x:1,y:2}            # outputs: {"x": 1, "y": 2}\n
```

#### `printstr`

Same as `print` but does **not** emit a trailing newline. Useful for
building output incrementally or for prompts.

*Signature:* `[any] -> []`
*Precedence:* forward

```
printstr "hello "           # outputs: hello  (no newline)
printstr 42                 # outputs: 42     (no newline)
```


### Conditional Words

#### `if`

Conditional evaluation, analogous to the ternary operator. Evaluates
the condition, then evaluates only the matching branch. Unevaluated
branches produce no side effects.

*Signatures:*
- `[any, any, any] -> [any]` — 3-arg: condition, then-branch, else-branch
- `[any, any] -> [any]` — 2-arg: condition, then-branch (returns nothing if false)

*Precedence:* forward

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

### Iteration Words

#### `for`

Numeric iteration. Takes a range and a body list. The iterator
variable `i` is automatically defined during each iteration and
undefined after the loop completes.

*Signatures:*
- `[integer, list] -> [results...]` — iterate 0 to N-1
- `[list, list] -> [results...]` — iterate with range spec

**Count form** — iterate from 0 to N-1:

```
for 3 [i]                       => 0 1 2
for 5 [i mul i]                 => 0 1 4 9 16
```

**Range spec** — `[end]`, `[start, end]`, or `[start, end, step]`:

```
for [5] [i]                     => 0 1 2 3 4
for [1,4] [i]                   => 1 2 3
for [0,10,3] [i]                => 0 3 6 9
```

The range is exclusive of the end value (like Go's `for i := start; i < end; i += step`).

#### `break`

Exit the current `for` loop immediately. Stack-only.

*Signature:* `[] -> []`

```
for 5 [if [i eq 3] [break] i]  => 0 1 2
```

#### `continue`

Skip the rest of the current iteration and advance to the next.
Stack-only.

*Signature:* `[] -> []`

```
for 5 [if [i eq 2] [continue] i]   => 0 1 3 4
```

### Debugging Words

#### `trace`

Evaluate a list as code (like `do`) with step-by-step tracing output.
Shows the stack state at each step of evaluation, including
resolved vs pending values, pointer position, and annotations for
dispatch decisions (forward/prefix matching,
argument collection). Output is color-coded for terminals.

*Signature:* `[list] -> [any...]`

```
trace [1 add 2]                 # prints step-by-step stack trace, returns 3
trace [3 4 mul]                 # traces multiplication, returns 12
trace ["hello" upper]           # traces string operation, returns 'HELLO'
trace [1 2 3 rot add mul]       # traces stack operations, returns 8
```

### Help Words

#### `help`

Show help for an AQL word. With no argument, prints a summary of the
`help` word itself. Given a word name, prints detailed help including
description, signatures, examples, and notes.

*Signatures:*
- `[] -> []`
- `[word] -> []`
- `[atom] -> []`
- `[string] -> []`

*Precedence:* forward

```
help                           # prints help about help
add help                       # prints help about add
concat help                    # prints help about concat
"split" help                   # prints help about split
```

### File I/O Words

File operations use an internal `FileOps` interface rather than calling
the Go `os` package directly. The default implementation uses the real
file system with the process working directory for relative paths. A
`MemFileOps` implementation is available for testing.

File format handling is dispatched through a pluggable `Format`
interface. Built-in formats are `text`, `json`, `jsonic`, `lines`, `csv`, and `tsv`.
Host applications can register custom formats via `RegisterFormat`.

#### `read`

Read a file and push its contents onto the stack. By default returns
a string with line endings normalized to `\n`.

*Signatures:*
- `[string] -> [string]` — read file at path
- `[string, map] -> [string|list|map]` — read with options

*Precedence:* forward

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
| `fmt`  | `"text"`  | `"text"`, `"json"`, `"jsonic"`, `"lines"`, `"csv"`, `"tsv"` |
| `nl`   | `"lf"`    | `"lf"`, `"crlf"`, `"raw"`                 |

**Format details:**

- `text` — raw string, no parsing
- `json` — parse JSON to AQL map/list
- `jsonic` — parse with jsonic (unquoted keys, trailing commas, etc.)
- `lines` — split on `\n` into a list of strings
- `csv` — parse CSV into a table value with typed schema
- `tsv` — parse TSV (tab-separated) into a table value with typed schema

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

*Precedence:* forward

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
| `fmt`  | `"text"`  | `"text"`, `"json"`, `"jsonic"`, `"lines"`, `"csv"`, `"tsv"` |
| `mode` | `"write"` | `"write"` (truncate), `"append"`           |
| `nl`   | `"lf"`    | `"lf"`, `"crlf"`, `"raw"`                 |

**Note:** With two string arguments of the same type, prefer forward
style (`write "path" "content"`) for clarity. The infix form
`"content" write "path"` is ambiguous because the engine cannot
distinguish path from content when both are strings.

#### `stdin`

Push the special stdin path onto the stack. Use with `read` to read
from standard input.

*Signature:* `[] -> [string]`

```
read stdin                          # read all of stdin as text
read stdin {fmt:"json"}            # parse stdin as JSON
read stdin {fmt:"lines"}           # split stdin into lines
```

#### `stdout`

Push the special stdout path onto the stack. Use with `write` to
write to standard output.

*Signature:* `[] -> [string]`

```
write stdout "hello"               # write to stdout
write stdout (upper "hello")       # write computed value to stdout
```

#### `stderr`

Push the special stderr path onto the stack. Use with `write` to
write to standard error.

*Signature:* `[] -> [string]`

```
write stderr "error message"       # write to stderr
```

### Query Words

Query words filter, sort, and limit table data using SQL-like syntax.
Tables are backed by SQLite when loaded via `read` with a tabular
format (CSV, TSV). Non-SQLite tables are transparently loaded into a
temporary SQLite table for query execution.

#### `select`

Select columns from a table. Use `*` (or `star`) for all columns,
or a list of column names. Column aliases use nested lists.

*Signatures:*
- `[atom("*"), table] -> [table]` — select all columns
- `[list, table] -> [table]` — select named columns



```
select * from people                          # all columns
select [name, age] from people                # named columns
select [[name n], age] from people            # alias: name AS n
select star from people                       # star word = *
```

#### `from`

Look up a named table from the registry store.

*Signature:* `[atom] -> [table]`

```
set people ("file/people.csv" read)
from people                                   # retrieve the table
```

#### `where`

Filter table rows using a condition list. Conditions use the
format `[column op value]` with optional `and`/`or` connectors.

Supported operators: `eq` (=), `neq` (!=), `lt` (<), `gt` (>),
`lte` (<=), `gte` (>=), `like` (LIKE).

*Signature:* `[condition-list, table] -> [table]`



```
from people where [age gt "25"]
from people where [city eq "Paris"]
from people where [age gt "20" and city eq "Paris"]
from people where [name like "A%"]
```

#### `order`

Sort table rows. Accepts a column name (atom) or a list of columns
with optional `asc`/`desc` direction.

*Signatures:*
- `[atom, table] -> [table]` — order by single column
- `[list, table] -> [table]` — order by column list



```
from people order name
from people order [name]
from people order [name desc]
from people order [city asc, name desc]
```

#### `by`

Syntactic sugar for `order by` style expressions. Wraps atom
arguments into a single-element list so `order` always receives a
list. List arguments pass through unchanged.

*Signatures:*
- `[atom] -> [list]`
- `[list] -> [list]`

```
from people order by name
from people order by [name desc]
```

#### `limit`

Restrict the number of rows returned.

*Signature:* `[integer, table] -> [table]`



```
from people limit 2
from people limit 1
```

#### `offset`

Skip a number of rows from the result.

*Signature:* `[integer, table] -> [table]`



```
from people offset 5
from people limit 10 offset 20
```

#### `distinct`

Remove duplicate rows from the result.

*Signature:* `[table] -> [table]`



```
select * (distinct (from people))
```

#### `as`

Add a table alias to a query (useful for joins and subqueries).

*Signature:* `[atom, table] -> [table]`



```
from people as p
```

#### `group`

Group rows by one or more columns. Accepts a column name (atom) or
a list of columns.

*Signatures:*
- `[atom, table] -> [table]` — group by single column
- `[list, table] -> [table]` — group by column list



```
from sales group by [region]
from sales group by [region product]
```

#### `having`

Filter groups after `group by`. Uses the same condition syntax as
`where`.

*Signature:* `[condition-list, table] -> [table]`



```
from sales group by [region] having [count gt 5]
```

#### `join` / `innerjoin` / `leftjoin` / `crossjoin`

Join two tables. `join` and `innerjoin` produce an inner join,
`leftjoin` a left outer join, `crossjoin` a cross join. Use `on` or
`using` to specify the join condition.

*Signature:* `[atom, table] -> [table]`



```
from orders join products on [orders.product_id eq products.id]
from orders leftjoin customers on [orders.cust_id eq customers.id]
from a crossjoin b
```

#### `on`

Set the ON condition for the most recent join.

*Signature:* `[condition-list, table] -> [table]`



```
from orders join products on [orders.pid eq products.id]
```

#### `using`

Set a USING clause for the most recent join (join on columns with
the same name in both tables).

*Signature:* `[column-list, table] -> [table]`



```
from orders join products using [id]
```

#### `union` / `unionall` / `intersect` / `except`

Set operations that combine the results of two queries. `union`
removes duplicates, `unionall` keeps all rows, `intersect` returns
rows in both, `except` returns rows in the left but not the right.

*Signature:* `[table, table] -> [table]`



```
(from employees) union (from contractors)
(from a) intersect (from b)
(from a) except (from b)
(from a) unionall (from b)
```

#### Aggregate Functions in Select

The `select` column list supports aggregate functions and casts as
nested lists. These are not standalone words — they are parsed inside
the column spec.

| Syntax                  | SQL equivalent           |
|-------------------------|--------------------------|
| `[count name cnt]`      | `COUNT("name") AS "cnt"` |
| `[sum amount total]`    | `SUM("amount") AS "total"` |
| `[avg score mean]`      | `AVG("score") AS "mean"` |
| `[min age youngest]`    | `MIN("age") AS "youngest"` |
| `[max age oldest]`      | `MAX("age") AS "oldest"` |
| `[cast age integer]`    | `CAST("age" AS INTEGER)` |

```
select [[count name cnt]] from people
select [[sum amount total], region] from sales group by [region]
select [[cast age integer]] from people
```

#### `star`

Push the wildcard column selector (`*`). Stack-only.

*Signature:* `[] -> [atom]`

```
"users" from star select
"orders" from star select 10 limit
```

### Module Words

#### `module`

Define a module with exported words. The list is evaluated in an
isolated scope and exported words become available under the module
name.

*Signature:* `[atom, list] -> []`
*Precedence:* forward

```
def mymod [def double [2 mul]] module
def utils [def inc [1 add] def dec [1 sub]] module
```

#### `import`

Import a module from a `.aql` file, making its exported words
available. Use a list argument to rename imports.

*Signatures:* `[string] -> []`, `[list, string] -> []`
*Precedence:* forward

```
"utils.aql" import
[Orig Renamed] "utils.aql" import
```

#### Chaining

Query words can be chained. Each operation produces a table that
the next operation consumes.

```
from people where [age gt "20"] order name
from people where [age gt "20"] limit 2
from people order name limit 2
from people where [age gt "20"] order [name] limit 1
```

Use parentheses for column projection with filtering:

```
select [name] (from people where [city eq "Paris"])
```


### Timer and Concurrency Words

#### `sleep`

Pause execution for the specified number of milliseconds.

*Signature:* `[integer] -> []`
*Precedence:* forward

```
sleep 100           # pause for 100ms
500 sleep           # same thing, prefix form
```

#### `timeout`

Schedule a callback to execute once after a delay. The callback is a
quoted list or word, executed with `do` semantics in a new sub-engine.
Returns a `Timeout` object that can be cancelled.

*Signatures:* `[integer, list/q] -> [Timeout]`, `[integer, atom/q] -> [Timeout]`
*Precedence:* forward

```
timeout 1000 [print "done"]       # fires after 1 second
timeout 500 myCallback            # word callback form
def t timeout 200 [print "hi"]    # save handle for cancel
```

#### `interval`

Schedule a callback to repeat at regular intervals. Returns an
`Interval` object that can be cancelled. The interval must be positive.

*Signatures:* `[integer, list/q] -> [Interval]`, `[integer, atom/q] -> [Interval]`
*Precedence:* forward

```
def i interval 1000 [print "tick"]  # fires every second
```

#### `cancel`

Cancel a pending timeout or repeating interval. Idempotent: cancelling
an already-cancelled timer is a no-op.

*Signatures:* `[Timeout] -> []`, `[Interval] -> []`
*Precedence:* forward

```
def t timeout 5000 [print "late"]
t cancel                            # prevent the callback
```

#### `now`

Return the current UTC instant.

*Signature:* `[] -> [Instant]`
*Precedence:* stack-only

```
now       # pushes current time as an Instant value
```

#### `await`

Run a list of parallel branches concurrently using Go goroutines.
Each element of the parallels list is executed with `do` semantics
in its own sub-engine. An optional `Options` argument controls the
mode.

*Signatures:* `[Options, list/q] -> [list]`, `[list/q] -> [list]`
*Precedence:* forward

**Modes** (set via `mode` field in Options):

| Mode | JS equivalent | Behavior |
|------|--------------|----------|
| `'all` (default) | `Promise.all` | Wait for all. First error rejects. Returns `[result, ...]` |
| `'full` | `Promise.allSettled` | Wait for all. Returns `[{status:'ok, value:...}, ...]` |
| `'first` | `Promise.race` | Return first to complete (success or error) |
| `'any` | `Promise.any` | Return first success. All reject → last error |

```
# Default mode (all): wait for all branches
await [[1 add 2] [3 add 4]]                    # → [3, 7]

# Full mode: get status of every branch
await (make Options {mode:'full}) [[1 add 2] [1 div 0]]
# → [{status:ok, value:3}, {status:error, value:error(...)}]

# First mode: race — first to finish wins
await (make Options {mode:'first}) [[sleep 100 1] [42]]
# → 42

# Any mode: first success wins, errors skipped
await (make Options {mode:'any}) [[1 div 0] [42]]
# → 42
```


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
