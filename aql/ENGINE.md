
# AQL Engine

Concatenative language using function composition.

The core engine is a pure stack machine: take the next token,
interpret as a function, which modifies the stack in some way.
For example, literals add themselves to the stack by default.

Future tokens are considered to be on the same stack as previous data,
but perhaps not yet resolved.

The language is typed so that functions can have signatures.
Function are polymorphic, and the most specific signature matches.
Most specific being longest with narrowest types.

The usual forth style primitives are provided by the system: dup, swap, etc.

Suffix arguments are supported as follows. Some function signatures
may specify that they are forward matches. If such a signature has
precedence, sufficent future tokens must be resolved to check for a
match.  In this case a new primitive, forward, is placed on the stack,
which stores internal accounting to manipulate the stack so that the
primary function will ultimately find its expected arguments in order
at the top of the stack. To make this work, ALL functions have an
implicit signature that checks for a forward value (very low
precedence) and performs appropriate stack manipulation.

Function call syntax allows for explicit selection of signature by
specifying the number of arguments needed, and whether they are prefix
or suffix arguments.







## Syntax Examples

Template:
__name__: _[before stack | future stack] -> [after stack | remaining future stack]_
> `example code` -> _[literal after stack | literal future stack]_

The future stack indication is optional.


__upper__: [string] -> [string]
> `a upper` -> _['A']_


__lower__: 
* [string] -> [string]
* [|string] -> [string]
> `lower B` -> _['b']_
> `C lower` -> _['c']_
> `99 lower` -> _[99 lower]_ SIGNATURE ERROR
> `lower/1 D` -> _['d']_ - disambiguate expected arg count
> `lower/s E` -> _['e']_ - force suffix args
> `F lower/p` -> _['f']_ - force prefix args
> `lower/1s E` -> _['e']_ - force suffix args, fix args
> `F lower/1p` -> _['f']_ - force prefix args, fix args



## Engine Examples


### upper

__upper__: [string] -> [string]
> `a upper` -> _['A']_

1. [| ^'a' upper] - start, ^ indicates stack pointer
2. ['a' | ^upper] - 'a' self inserts
3. [(string) | ^upper] - upper signature match
4. upper('a') -> 'A' - execute upper (primitive)
5. ['A' | ^] - insert result 'A'
6. no future tokens, output top of stack: 'A'


### lower

__lower__: 
* [string] -> [string]
* [|string] -> [string]
> `lower B` -> _['c']_
> `C lower` -> _['c']_
> `99 lower` -> _[99 lower]_ SIGNATURE ERROR

1. [| ^lower 'B'] - start
2. [lower forward{args:1,arg:0} | ^'b'] - insert forward primitive, tracking expected arg count
3. [... (forward) | ^'B'] - implicit forward signature match by 'b'
4. ['B' ^lower |] - forward primitive stack manipulation
5. [(string) ^lower |] - signature match
6. ['B' ^lower|]
7. ['b'| ^]




## Word Signatures and Arguments

All words can accept arguments from the stack. These are called prefix arguments.
Tokens from the future stack are called suffix arguments.

A word can accept one or more type signatures. These are specified as
lists in reverse stack order.

By default all words have suffix precedence. When invoked, to force
prefix only, append /p to the word name. To force suffix only, use /s.

This can also be used when defining the word, to indicate default behaviour. But this can
always be overridden in situ.


For example:

`def square [ [integer] [integer] [dup mul] ]`

Define word with name "square" accepting one integer type argument from the top of the stack, 
and replacing the top of the stack with another integer. Prefix args only.
Thus: [|] -> `2 square` -> [4|]


`def area [ [float integer] [float] [mul] ]`

Define word with name "area" accepting two float type arguments from the top of the stack, 
and replacing the top of the stack with another float. Prefix args only.
Thus: [|] -> `3 2.5 area` -> [7.5|]

Note that `1.5 4 area` would be an error as the types do not match. Remember type signatures
are in reverse stack order.


Words can have multiple signatures:

```
def area [ 
  [float integer] [float] [mul 'foo'] # ends with [<result> 'foo'|] 
  [integer float] [float] [mul 'bar'] # ends with [<result> 'bar'|] 
]
```

The list defining the word must have size divisible by three. When a word is invoked, the 
matching signature should be chosen. Thus:

* [|] -> `1.5 2 area` -> [3.0 'foo'|]
* [|] -> `3 4.5 area` -> [15.5 'bar'|]

Some conveniences that are equivalent:

```
def foo [[string] [string] [upper]]
def foo [string string upper]

def bar [[] [] [2 mul]]
def bar [[ 2 mul ]]
```

If a word has suffix precedence (the default), the arguments to match
against the signature can be constructed using the stack and future
tokens as follows: match each type in the signature in order against
future tokens, until a mismatch, then continue matching against the
stack in reverse order.

For example:


`def square [ [integer] [integer] [dup mul swap drop] ]`

Define word with name "square" accepting one integer type argument from the top of the stack, 
and replacing the top of the stack with another integer. However it can also accept suffix
arguments.
Thus: [|] -> `2 square` -> [4|] but also [|] -> `square 2` -> [4|]


`def area [ [float integer] [float] [mul] ]`

* [|] -> `area 1.5 2` -> [3|]
* [|] -> `1.5 area 2` -> [3|]
* [|] -> `1.5 2 area` -> [3|]


Implementation: words themselves shoudl never deal with suffix precedence. Instead the 
interpreter should move any matched values from the future stack onto the main stack, and the
word can then proceed normally, as if the values had been prefix values all along.



The default argument precedence can be specified using a /p or /s suffix in the definition:

* `def foo/p ...` - only use prefix args.


## Traditional Forth-style Builtins

Words such as dup, swap, drop, etc, taken directly form forth, are considered defined with /p, an d only operate on the stack be default. 

* [1|] -> `dup` -> [1 1|]
* [2 3|] -> `swap` -> [3 2|]
* [4|] -> `drop` -> [|]

But they can be forced to use suffix args with /s:

* [|] -> `dup/s 1` -> [1 1|]  # engine translates to [1|] -> `dup` -> [1 1|]
