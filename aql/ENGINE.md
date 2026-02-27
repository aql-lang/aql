
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
> `lower= E` -> _['e']_ - force suffix args
> `F =lower` -> _['f']_ - force prefix args
> `lower/1= E` -> _['e']_ - force suffix args, fix args
> `F =lower/1` -> _['f']_ - force prefix args, fix args



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



