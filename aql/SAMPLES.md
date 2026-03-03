



## Format

`expression` - purpose

_[relevant stack before] -> [relevant stack after]_ listing entry types


## Types

_typename_ - named type
_parent/child_ - parent and child type (can have more than 2 parts)


any - unifies with all types
none - unifies with no types, also a literal value


Type definition:

`type foo string` - type named foo is a string


`type foo {a:string,b:number}` - complex type


`type foo [1 2 add]` - complex type, add is quoted code


`type foo {x:['a' 'b' add]}` - complex type, also quoted code


`type increment word [[integer] [integer]]` - 
  word type, accepting an integer, returning an integer

```
type increment word [
  [integer] [integer]
  [float] [float]
  ]
``` 

- multiple signatures
  

child elements:

`[:string]` - list of strings
`[:{x:number}]` - of of `{x:number}` maps

`{:integer}` - map of integers
`{:[:string]}` - map of string lists


disjunctions


`type foo disjunct [string none]` - optional string 



## Literals

`1` - number

_[] -> [number/integer]_


`"hello"` - string

_[] -> [string/proper]_


`""` - string

_[] -> [string/empty]_


`a:1,b:c,d:[{e:"f"}]` - jsonic

_[] -> [data/map]_



## Control

end - terminates current prefix args and forces remaining 
args to be got from stack


## Sets


`>2` - numbers greater than 2

_[] -> [uniq/predicate/number/integer]_



`uniq [a,b]` - set of strings "a", "b"

_[] -> [uniq/extant/string]_



## Operators

boolean: and or not xor nand implies

numeric: add sub mul div mod pow fact ln upper lower

string: cat slice upper lower repeat pad

list: map reduce filter

map (on values): map reduce filter items


# Constants

numberic: pi euler



## Storage

`set foo 99` - sets store key foo to value 99
set has signature: set [string any]

examples:

`set foo 99`

[|foo 99] -> [99|] - the value is left on the stack


`set foo` - assumes value is already on stack

[99 | foo] -> [99|]


`set` - all args on stack - note order

[99 foo|] -> [99|]


`set foo end 88` - shows usage of end

[99 | foo end 88] -> [99 88|] - and store contains foo:99



`get foo` - gets value of store key foo




## Seneca REPL inspired

`list foo/bar` - lists all bar entities of API foo.

_[] -> [table/entity]_


`list foo/bar a:1` - lists all bar entities of API foo, where field a == 1.

_[] -> [table/entity]_



## Comments

The character # marks a line comment, all characters to the end of the line are ignored
Multi-line, or inline comments use syntax ## commented ##, with both start and end
markers being ##



## Word definitions

Quoted source is directly placed onto future token strea,

The built in def has suffix precedence with signature: 
```
[
  [word any] []
  [string any] []
  [word] []        # takes top of stack for body 
  [string] []      # takes top of stack for body
]
```


```
def increment [[1 add]]
2 increment # result [3|]

def decrement [[1 sub]] 
decrement 3 # result <2|>, works because sub allows suffix args
```

New words defined with `def` can only handle prefix args in general, and have no type checking.
They are literal substitutions.


`def x` means use top of stack as value


`undef foo` - undefines a word, removes its definition
However: definitions can be nested, shadowing each other.

`def foo 1  add foo 2  def foo 3  add foo 4  undef foo  add foo 5  undef foo ## foo invalid ##`
returns <3 7 6>


## Function definitions

`def square [ [number] [number] [dup mul]]`

Words are effectively functions. Thus square takes one argument, suffix precendence. 
The argument can be:

- list with 3n entries

The first two words define the signature, third the implemenation. 
Multiples of three define other signatures


`def square [ number number [dup mul]]` - same as above, 
except function square now has suffix precedence, abbreviate signatures for single args and returns.

Recursion is possible using defined name, or `recur`

```
def square [ 
  [number] [number] [dup mul]
  [float] [float] square
  [integer] [integer] recur
  [string] [string] [convert float dup mul]
]
  ```



## Variables

`var [[x] square x add 1]` # x is taken from the top of stack

var accepts one argument, a list. The first element is a list of variable names.
In this context, unknown words are not errors but register as variable names.

Variables are just defined words. var is a convenience that places an
undef at the end of its clause.

`var [ [x] ... ]` means `def x end ... undef x`


`var [ [[x 2] y] square x add y]` # x is defined in place, y from stack


Use in function definition to name parameters:

```
def square [ 
  [number] [number] [ 
    var [ [x] 
      x mul x] 
    ]
  ]
]

```

And as a convenience:

```
def square [ 
  [x:number] [number] [x mul x] 
]

```

(needs jsonic option where [x:a] -> [{"a":1}])


## Stack jumps

`mark name` - mark a position in stack (past or future)
`unmark` - remove mark at top of stack
`move name` - move stack pointer to mark, remove mark
`jump name` - move stack pointer to mark, leave mark



## Iteration


`for [10] [print i]` - '0\n1\n..9\n' - iterates 0..9, i is convenience var
`for [k:1,10] [print k]` - '1\n2\n..9\n' - [start,end], named iterator
`for [0,10,2] [print args.0]` - '0\n2\n..8\n' - [start,end,step], loop args

implement by repeatedly copying body to future stack 

`break` and `continue` as per js

implement using `move` and `mark`





