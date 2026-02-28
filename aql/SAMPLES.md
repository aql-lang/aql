



## Format

`expression` - purpose

_[relevant stack before] -> [relevant stack after]_ listing entry types


## Types

_typename_ - named type
_parent/child_ - parent and child type (can have more than 2 parts)


## Literals

`1` - number

_[] -> [number/integer]_


`"hello"` - string

_[] -> [string/proper]_


`""` - string

_[] -> [string/empty]_


`a:1,b:c,d:[{e:"f"}]` - jsonic

_[] -> [data/map]_


## Sets


`>2` - numbers greater than 2

_[] -> [uniq/predicate/number/integer]_



`uniq [a,b]` - set of strings "a", "b"

_[] -> [uniq/extant/string]_



# Storage

`set foo 99` - sets store key foo to value 99
set has signature: set [string any]=
where [string any] means expect to find
a string cell follower by an any cell at the top
of the stack. however the = suffix means give precedence to 
future tokens, in reverse order.

examples:

set foo 99
[|] -> [99] - the value is left on the stack

`get foo` - gets value of store key foo




## Seneca REPL inspired

`list foo/bar` - lists all bar entities of API foo.

_[] -> [table/entity]_


`list foo/bar a:1` - lists all bar entities of API foo, where field a == 1.

_[] -> [table/entity]_


