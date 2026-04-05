# AQL Tutorial

This tutorial walks you through AQL from first principles. By the
end, you will be comfortable writing expressions, defining functions,
and working with the type system.


## Getting Started

Build and run the AQL REPL:

```bash
cd aql
make build
./aql
```

You will see the `aql>` prompt. Type an expression and press Enter
to evaluate it.


## Your First Expression

AQL is a stack machine. Values push onto the stack; words consume
values and push results.

```
aql> 1 2 add
3
```

We pushed `1`, then `2`, then `add` consumed both and pushed `3`.

Try arithmetic:

```
aql> 10 sub 3
7

aql> 4 mul 5
20

aql> 2 pow 10
1024
```

Notice that `sub 3` reads naturally as "subtract 3". The word `sub`
takes the next value (`3`) as its first argument and the top of
stack (`10`) as its second.


## Strings

Strings use double or single quotes. Many words work on strings:

```
aql> "hello" upper
'HELLO'

aql> "hello world" split " "
['hello','world']

aql> "abc" concat "def"
'abcdef'

aql> "hello" contains "ell"
true
```

Template strings use backticks with `${...}` interpolation:

```
aql> def name "world"
aql> `hello ${name}`
'hello world'
```


## The Stack

Values that are not consumed remain on the stack:

```
aql> 1 2 3
1 2 3
```

Stack words let you manipulate values:

```
aql> 5 dup
5 5

aql> 1 2 swap
2 1

aql> 1 2 3 drop
1 2
```


## Lists and Maps

Lists use square brackets:

```
aql> [1, 2, 3]
[1,2,3]
```

Maps use braces with `key:value` pairs:

```
aql> {name: "Alice", age: 30}
{name:'Alice',age:30}
```

Access values with the dot operator:

```
aql> {name: "Alice"} . name
'Alice'

aql> [10, 20, 30] . 1
20
```


## Defining Words

Use `def` to name values or create reusable words:

```
aql> def x 42
aql> x
42

aql> def double [dup add]
aql> 5 double
10

aql> 3 double double
12
```

The list `[dup add]` is a code body. When `double` is called, the
body executes: `dup` copies the top value, then `add` sums both.


## Functions with Types

Use `fn` inside `def` for typed functions:

```
aql> def square fn [Integer Integer [dup mul]]
aql> square 5
25

aql> def greet fn [String String [`hello ${args.0}`]]
aql> greet "world"
'hello world'
```

The three elements define: input type, output type, body.


## Conditionals

The `if` word takes a condition, a then-branch, and an optional
else-branch:

```
aql> 5 gt 3 if ["yes"] ["no"]
'yes'

aql> 0 if ["truthy"] ["falsy"]
'falsy'
```


## Loops

Use `for` for iteration:

```
aql> for 5 [dup mul]
0 1 4 9 16
```

The loop counter starts at 0 and runs to N-1. Each iteration
pushes the counter, then executes the body.

Use a range for more control:

```
aql> for [1, 4] [dup mul]
1 4 9
```


## Evaluation with `do`

The `do` word evaluates a quoted list as a sub-program:

```
aql> do [1 add 2]
3

aql> do {x: [3 add 4], y: 5}
{x:7,y:5}
```


## Error Handling

Errors are values. The `error` word handles them:

```
aql> do [1 div 0] error [drop 42]
42
```

The `do` catches the division-by-zero error and returns it as an
error value. The `error` word then runs the handler list with the
error on the stack. `drop` discards it and `42` is the recovery
value.


## Types

AQL has a hierarchical type system. Check types with `typeof`:

```
aql> typeof 42
Integer

aql> typeof "hello"
String

aql> typeof [1, 2]
List
```

Use `is` to test type compatibility:

```
aql> 42 is Integer
true

aql> 42 is Number
true

aql> 42 is String
false
```


## Parallel Execution

Use `await` to run tasks concurrently:

```
aql> await [[1 add 2] [3 add 4]]
[3,7]
```

Each list element runs in its own goroutine. Results are collected
in order.

Use `sleep` to add delays:

```
aql> sleep 100
```

Schedule deferred work with `timeout` and `interval`:

```
aql> def t timeout 1000 [print "done"]
aql> t cancel
```


## Next Steps

- Read the [How-To Guides](how-to.md) for task-oriented recipes
- Consult the [Reference](reference.md) for complete word signatures
- See the [Explanation](explanation.md) for deeper understanding of
  the concatenative model and type system
