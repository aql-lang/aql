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


## 1. Install and start the REPL

Install the binary, then run it with no arguments:

```bash
go install github.com/aql-lang/aql/cmd/go/aql@latest
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
aql do '1 add 2'
# 3
```

Or run a file:

```bash
aql script.aql
aql -e '"hello" upper'
```


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
aql> 1 2 add        # prefix — both args from stack
3
aql> add 1 2        # forward — both args after the word
3
aql> 1 add 2        # infix — one from stack, one forward
3
```

This is why `10 sub 3` reads naturally as "10 minus 3":

```
aql> 10 sub 3
7
```

You'll mix all three styles as your code grows. See
**[Explanation §Forward Collection](EXPLANATION.md#forward-collection-beyond-reverse-polish)**
for what's happening underneath.


## 4. Numbers, rounding, math

Integers and decimals are different leaves of the same `Number`
type. Arithmetic auto-promotes:

```
aql> 4 mul 5            => 20
aql> 2 pow 10           => 1024
aql> 7 div 2            => 3        # integer division
aql> 7.0 div 2          => 3.5      # decimal: real division
aql> 10 mod 3           => 1
aql> abs -5             => 5
aql> 3 min 5            => 3
aql> 3 max 5            => 5
```

Rounding:

```
aql> floor 3.7          => 3
aql> ceil 3.2           => 4
aql> round 3.5          => 4
aql> trunc 3.9          => 3
```

Roots, logs, trig:

```
aql> sqrt 16            => 4
aql> log 2.718281828    => 1.0
aql> sin 0              => 0
aql> hypot 3 4          => 5
```

Constants:

```
aql> math-pi            => 3.141592653589793
aql> math-e             => 2.718281828459045
```


## 5. Strings

Strings use single or double quotes (they're interchangeable):

```
aql> "hello" upper                 => 'HELLO'
aql> "HELLO" lower                 => 'hello'
aql> "hello,world" split ","       => ['hello','world']
aql> ["a","b","c"] concat          => 'abc'
aql> "hello" contains "ell"        => true
aql> "hello" indexof "ll"          => 2
aql> "hello" slice 1 3             => 'el'
aql> "hello" replace "l" "r"       => 'herro'
aql> "  hi  " trim                 => 'hi'
aql> "ab" repeat 3                 => 'ababab'
aql> "hi" pad 5                    => 'hi   '
```

Backtick template strings interpolate `${...}` expressions:

```
aql> def name "world"
aql> `hello ${name}`               => 'hello world'
aql> `2 + 3 = ${2 add 3}`         => '2 + 3 = 5'
```

Templates nest:

```
aql> `a${`inner ${1 add 2}`}b`     => 'ainner 3b'
```


## 6. Manipulating the stack

When the stack model isn't quite enough, these words rearrange it:

```
aql> 5 dup              => 5 5             # duplicate top
aql> 1 2 swap           => 2 1             # exchange top two
aql> 1 2 3 drop         => 1 2             # discard top
aql> 1 2 over           => 1 2 1           # copy second to top
aql> 1 2 3 rot          => 2 3 1           # rotate top three
aql> 1 2 nip            => 2               # remove second
aql> 1 2 tuck           => 2 1 2           # copy top below second
aql> depth              => 0               # current stack size
```

Most of the time you won't need these — forward collection covers
the common cases. They're a tool for when the shape of the stack
fights you.


## 7. Lists and maps

Lists use square brackets, maps use braces:

```
aql> [1, 2, 3]                       => [1,2,3]
aql> {name: "Alice", age: 30}        => {name:'Alice',age:30}
```

Commas are optional inside literals — both `[1 2 3]` and `[1, 2, 3]`
parse the same.

The dot operator accesses fields by name or by index:

```
aql> {name: "Alice"} . name          => 'Alice'
aql> [10, 20, 30] . 1                => 20
aql> {a: {b: 99}} . a . b            => 99
```

Use `!.` (also called `getr`) when the key *must* exist — it raises
an error instead of returning `none`:

```
aql> {x:1} !. y                      => error: missing key 'y'
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
aql> x                               => 42
```

When the body is a list, calling the word *runs* the list:

```
aql> def double [dup add]
aql> 5 double                        => 10
aql> 3 double double                 => 12
```

Composition is concatenation:

```
aql> def quadruple [double double]
aql> 5 quadruple                     => 20
```

To remove a definition use `undef`:

```
aql> undef x
```


## 9. Typed functions with `fn`

`fn` builds a typed function. The shape is a list of
`[input-sig] [output-sig] [body]` triples:

```
aql> def square fn [[x:Number] [Number] [x mul x]]
aql> 5 square                        => 25
aql> 2.5 square                      => 6.25
```

Named parameters (like `x:Number`) bind to stack values automatically
inside the body. You can also use the implicit `args` list:

```
aql> def greet fn [[String] [String] [`hello ${args.0}`]]
aql> greet "world"                   => 'hello world'
```

Multiple signatures give you ad-hoc polymorphism — first match wins:

```
aql> def inc fn [
  [Integer] [Integer] [1 add]
  [Decimal] [Decimal] [1.0 add]
]
aql> inc 5                           => 6
aql> inc 2.5                         => 3.5
```


## 10. Conditionals and loops

`if` takes a condition, a then-branch, and an optional else-branch.
The branches are lists (which is why they're not evaluated up-front):

```
aql> 5 gt 3 if ["yes"] ["no"]        => 'yes'
aql> 0 if ["truthy"] ["falsy"]       => 'falsy'
```

`for` iterates over a numeric range, pushing the counter into the
body each step:

```
aql> for 5 [dup mul]                 => 0 1 4 9 16
aql> for [1, 4] [dup mul]            => 1 4 9
aql> for [0, 10, 2] [dup mul]        => 0 4 16 36 64
```

`break` and `continue` work inside the body:

```
aql> for 10 [dup gt 5 if [break]]
```


## 11. Higher-order list words

These are the bread-and-butter of array programming in AQL:

```
aql> [1, 2, 3] each [dup mul]        => [1,4,9]
aql> [1, 2, 3, 4] where [gt 2]       => [3,4]
aql> [1, 2, 3, 4, 5] fold 0 [add]    => 15
aql> [1, 2, 3] scan 0 [add]          => [1,3,6]
```

Sequence-building:

```
aql> iota 5                          => [0,1,2,3,4]
aql> iota 6 reshape [2, 3]           => [[0,1,2],[3,4,5]]
aql> [1, 2, 3] reverse                => [3,2,1]
aql> [1, 2, 2, 3] unique              => [1,2,3]
aql> [3, 1, 2] grade                  => [1,2,0]
```

`outer` and `inner` are APL-style array combinators:

```
aql> [1, 2] outer [10, 20] [mul]     => [[10,20],[20,40]]
aql> [1, 2] inner [3, 4] [mul] [add] => 11      # 1*3 + 2*4
```


## 12. Types and `is`

Every value has a type, organised into a hierarchy. Inspect with
`typeof` or `fulltypeof`:

```
aql> typeof 42                       => Integer
aql> typeof "hello"                  => String
aql> typeof [1, 2]                   => List
aql> fulltypeof 42                   => Scalar/Number/Integer
```

Use `is` to test membership against any ancestor in the hierarchy:

```
aql> 42 is Integer                   => true
aql> 42 is Number                    => true
aql> 42 is Scalar                    => true
aql> 42 is String                    => false
```

Convert with `make` (for scalars) or `convert`:

```
aql> convert Integer "42"            => 42
aql> convert String 42               => '42'
aql> make string 99                  => '99'
```


## 13. Records and tables

`record` defines a struct-like type with named typed fields. `table`
turns one into a list-of-rows type. `make` instantiates them:

```
aql> type Point record [x:Number y:Number]
aql> make Point [3 4]                => {x:3,y:4}
aql> make Point {x:1 y:2}            => {x:1,y:2}

aql> type Row record [name:String qty:Integer]
aql> type Inventory table Row
aql> make Inventory [["Widget" 5] ["Bolt" 12]]
=> [{name:'Widget',qty:5},{name:'Bolt',qty:12}]
```

Field constraints can be disjunctive — `[String tor none]` means
"string or absent":

```
aql> type Person record [name:String nick:[String tor none]]
aql> make Person {name:"Alice" nick:"ace"}     => {name:'Alice',nick:'ace'}
aql> make Person {name:"Bob"}                  => {name:'Bob',nick:none}
```


## 14. Scoped variables with `var`

`var` introduces local names that are automatically un-defined at
the end of the block:

```
aql> 3 4 var [[a b] (a mul a) add (b mul b) sqrt]      => 5.0
```

The first element of the list is the binding list (each name is
peeled off the top of the stack). The remaining elements are the
body. Inline values:

```
aql> var [[[x 2] [y 10]] x add y]               => 12
```


## 15. Evaluation with `do`, `call`, and `quote`

Lists are quotations by default — `[1 add 2]` is *data*, not code:

```
aql> [1 add 2]                       => [1,add,2]
```

`do` evaluates a list as a sub-program:

```
aql> do [1 add 2]                    => 3
aql> do {x: [3 add 4], y: 5}        => {x:7,y:5}
```

`call` splices a list onto the current stack:

```
aql> 1 2 [add 10 mul] call           => 30
```

`quote` prevents a single token from being interpreted:

```
aql> quote foo                       => foo
```


## 16. Error handling

Errors are values, not exceptions. `do` catches them and the
`error` word pattern-matches:

```
aql> do [1 div 0]
Error(div: division by zero)

aql> do [1 div 0] error [drop 42]    => 42
```

The pattern is `do [risky] error [handler]`. Inside the handler the
error value is on the stack — `drop` it and push a recovery value,
or inspect its fields with `.`.


## 17. Concurrency with `await`

`await` runs a list of code blocks in parallel and collects the
results:

```
aql> await [[1 add 2] [3 add 4]]     => [3,7]
```

Pick a mode via an options map — these mirror JavaScript Promise
combinators:

```
aql> await {mode: 'all}   [[sleep 10 1] [sleep 10 2]]
=> [1,2]                                # all must succeed

aql> await {mode: 'first} [[sleep 100 1] [sleep 10 2]]
=> 2                                    # race winner

aql> await {mode: 'any}   [[1 div 0] [sleep 10 42]]
=> 42                                   # first non-error

aql> await {mode: 'full}  [[1] [1 div 0]]
=> [{status:'ok,value:1},{status:'error,value:...}]
```

Schedule deferred work with `timeout` and `interval`, cancel with
`cancel`:

```
aql> def t timeout 1000 [print "fired"]
aql> t cancel
```


## 18. Reading and writing files

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


## 19. Modules — namespaces and imports

A *module* is a fresh evaluation context. Define one inline:

```
aql> import utils [
  def helper [dup add]
  def greet fn [[String] [String] [`hello ${args.0}`]]
]
aql> utils.helper 5                  => 10
aql> utils.greet "Ada"               => 'hello Ada'
```

Import from a file:

```
aql> import "lib/utils.aql"
```

Rename to avoid collisions:

```
aql> import [helper as h] "lib/utils.aql"
```

Built-in modules: `aql:math`, `aql:time`, `aql:matrix`,
`aql:decision`. Import them by name:

```
aql> import aql:time
aql> aql:time.now
```


## 20. Where to next

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
