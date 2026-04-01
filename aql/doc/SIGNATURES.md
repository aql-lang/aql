# AQL Word Signatures

All words listed alphabetically with their registered signatures (in match
order — first match wins), return values, and notes on special argument
processing. The **Data Arg** column marks which argument position holds
the primary data being manipulated, where applicable, when that argument
is a List.

Signature arguments are listed deepest-first: `[arg0, arg1, ...]` where
`arg0` is nearest the word (first forward arg or top of stack). Forward
precedence words can collect args from both stack and forward positions
equivalently.

Abbreviations: I=Integer, D=Decimal, N=Number, S=String, A=Atom,
B=Boolean, M=Map, L=List, W=Word, /q=QuoteArgs modifier, /s=stack-only,
/f=FullStack.


## Stack Manipulation

| Word | Signatures (match order) | Returns | Notes | Data Arg |
|------|--------------------------|---------|-------|----------|
| `2drop` /s | `[Any, Any]` | `[]` | Drop top 2 | — |
| `2dup` /s | `[Any, Any]` | `[Any, Any, Any, Any]` | Duplicate top 2 | — |
| `2over` /s | `[Any, Any, Any, Any]` | `[Any, Any, Any, Any, Any, Any]` | Copy 3rd–4th to top | — |
| `2swap` /s | `[Any, Any, Any, Any]` | `[Any, Any, Any, Any]` | Swap top two pairs | — |
| `depth` /s /f | `[]` | `[Integer]` | Returns stack size | — |
| `drop` /s | `[Any]` | `[]` | Remove top | — |
| `dup` /s | `[Any]` | `[Any, Any]` | Duplicate top | — |
| `nip` /s | `[Any, Any]` | `[Any]` | Remove second from top | — |
| `over` /s | `[Any, Any]` | `[Any, Any, Any]` | Copy second to top | — |
| `pick` /s /f | `[I]` | `[Any]` | Copy value at index n | — |
| `roll` /s /f | `[I]` | `[Any]` | Move value at index n to top | — |
| `rot` /s | `[Any, Any, Any]` | `[Any, Any, Any]` | Rotate top 3: `[a,b,c]→[b,c,a]` | — |
| `stack` /s /f | `[I]` | `[L]` | Return entire stack as list | — |
| `swap` /s | `[Any, Any]` | `[Any, Any]` | Exchange top 2 | — |
| `tuck` /s | `[Any, Any]` | `[Any, Any, Any]` | Duplicate top before second | — |


## Math — Arithmetic

| Word | Signatures (match order) | Returns | Notes | Data Arg |
|------|--------------------------|---------|-------|----------|
| `abs` | `[I]` | `[I]` | Absolute value | — |
| | `[D]` | `[D]` | | — |
| `add` | `[I, I]` | `[I]` | Sum | — |
| | `[D, D]` | `[D]` | | — |
| | `[N, D]` | `[D]` | | — |
| | `[D, N]` | `[D]` | | — |
| | `[Scalar, Scalar]` | `[S]` | String concat: `args[1]+args[0]` | — |
| `div` | `[I, I]` | `[I]` | `args[1] / args[0]`; error on zero | — |
| | `[D, D]` | `[D]` | | — |
| | `[N, D]` | `[D]` | | — |
| | `[D, N]` | `[D]` | | — |
| `max` | `[I, I]` | `[I]` | Maximum of two | — |
| | `[D, D]` | `[D]` | | — |
| | `[N, D]` | `[D]` | | — |
| | `[D, N]` | `[D]` | | — |
| `min` | `[I, I]` | `[I]` | Minimum of two | — |
| | `[D, D]` | `[D]` | | — |
| | `[N, D]` | `[D]` | | — |
| | `[D, N]` | `[D]` | | — |
| `mod` | `[I, I]` | `[I]` | `args[1] % args[0]`; error on zero | — |
| | `[D, D]` | `[D]` | | — |
| | `[N, D]` | `[D]` | | — |
| | `[D, N]` | `[D]` | | — |
| `mul` | `[I, I]` | `[I]` | Product | — |
| | `[D, D]` | `[D]` | | — |
| | `[N, D]` | `[D]` | | — |
| | `[D, N]` | `[D]` | | — |
| `negate` | `[I]` | `[I]` | Negation | — |
| | `[D]` | `[D]` | | — |
| `pow` | `[I, I]` | `[I]` | `args[1]^args[0]`; error if exp<0 | — |
| | `[D, D]` | `[D]` | | — |
| | `[N, D]` | `[D]` | | — |
| | `[D, N]` | `[D]` | | — |
| `sign` | `[I]` | `[I]` | Returns -1, 0, or 1 | — |
| | `[D]` | `[I]` | | — |
| `sub` | `[I, I]` | `[I]` | `args[1] - args[0]` | — |
| | `[D, D]` | `[D]` | | — |
| | `[N, D]` | `[D]` | | — |
| | `[D, N]` | `[D]` | | — |


## Math — Rounding

| Word | Signatures (match order) | Returns | Notes | Data Arg |
|------|--------------------------|---------|-------|----------|
| `ceil` | `[D]` | `[I]` | Ceiling | — |
| `floor` | `[D]` | `[I]` | Floor | — |
| `round` | `[D]` | `[I]` | Round to nearest | — |
| `trunc` | `[D]` | `[I]` | Truncate toward zero | — |


## Math — Roots, Exponentials, Logarithms

| Word | Signatures (match order) | Returns | Notes | Data Arg |
|------|--------------------------|---------|-------|----------|
| `cbrt` | `[I]` | `[D]` | Cube root | — |
| | `[D]` | `[D]` | | — |
| `exp` | `[I]` | `[D]` | e^x | — |
| | `[D]` | `[D]` | | — |
| `log` | `[I]` | `[D]` | Natural log | — |
| | `[D]` | `[D]` | | — |
| `log10` | `[I]` | `[D]` | Log base 10 | — |
| | `[D]` | `[D]` | | — |
| `log2` | `[I]` | `[D]` | Log base 2 | — |
| | `[D]` | `[D]` | | — |
| `sqrt` | `[I]` | `[D]` | Square root | — |
| | `[D]` | `[D]` | | — |


## Math — Trigonometry

| Word | Signatures (match order) | Returns | Notes | Data Arg |
|------|--------------------------|---------|-------|----------|
| `acos` | `[I]` | `[D]` | Inverse cosine | — |
| | `[D]` | `[D]` | | — |
| `asin` | `[I]` | `[D]` | Inverse sine | — |
| | `[D]` | `[D]` | | — |
| `atan` | `[I]` | `[D]` | Inverse tangent | — |
| | `[D]` | `[D]` | | — |
| `atan2` | `[I, I]` | `[D]` | `atan2(args[1], args[0])` | — |
| | `[D, D]` | `[D]` | | — |
| | `[N, D]` | `[D]` | | — |
| | `[D, N]` | `[D]` | | — |
| `cos` | `[I]` | `[D]` | Cosine | — |
| | `[D]` | `[D]` | | — |
| `hypot` | `[I, I]` | `[D]` | Hypotenuse | — |
| | `[D, D]` | `[D]` | | — |
| | `[N, D]` | `[D]` | | — |
| | `[D, N]` | `[D]` | | — |
| `sin` | `[I]` | `[D]` | Sine | — |
| | `[D]` | `[D]` | | — |
| `tan` | `[I]` | `[D]` | Tangent | — |
| | `[D]` | `[D]` | | — |


## Math — Constants

| Word | Signatures (match order) | Returns | Notes | Data Arg |
|------|--------------------------|---------|-------|----------|
| `math-e` /s | `[]` | `[D]` | Euler's number | — |
| `math-pi` /s | `[]` | `[D]` | Pi | — |


## String Operations

| Word | Signatures (match order) | Returns | Notes | Data Arg |
|------|--------------------------|---------|-------|----------|
| `changecase` | `[S, M]` | `[S]` | Opts: {style:A} (camel, snake, etc.) | — |
| | `[S]` | `[S]` | | — |
| | `[A, M]` | `[S]` | | — |
| | `[A]` | `[S]` | | — |
| `concat` | `[L, M]` | `[S]` | Opts: {sep:S}; joins list elements | `arg0: L` |
| | `[L]` | `[S]` | | `arg0: L` |
| `contains` | `[S, S, M]` | `[B]` | Opts: {cs:B}; `args[0]`=needle, `args[1]`=haystack | — |
| | `[S, S]` | `[B]` | | — |
| `escape` | `[S, M]` | `[S]` | Opts: {mode:A} (json/uri/html/csv) | — |
| | `[S]` | `[S]` | | — |
| `indexof` | `[S, S, M]` | `[I]` | Opts: {cs:B}; returns -1 if not found | — |
| | `[S, S]` | `[I]` | | — |
| `lower` | `[S]` | `[S]` | Lowercase | — |
| | `[A]` | `[S]` | | — |
| `match` | `[S, S, M]` | `[M]` | Regex/pattern match with opts | — |
| | `[S, S]` | `[M]` | | — |
| `normalize` | `[S, M]` | `[S]` | Opts: {form:A} (nfc/nfd/nfkc/nfkd) | — |
| | `[S]` | `[S]` | | — |
| `pad` | `[I, M, S]` | `[S]` | Opts: {side:A, char:S}; `args[2]`=input string | — |
| | `[I, S]` | `[S]` | `args[1]`=input string, `args[0]`=width | — |
| `repeat` | `[S, I, M]` | `[S]` | Opts map in 3-arg form | — |
| | `[S, I]` | `[S]` | `args[0]`=string, `args[1]`=count | — |
| `replace` | `[S, S, S, M]` | `[S]` | Opts: {cs:B, mode:A} | — |
| | `[S, S, S]` | `[S]` | `args[0]`=pattern, `args[1]`=replacement, `args[2]`=input | — |
| `slice` | `[I, I, S]` | `[S]` | `args[0]`=start, `args[1]`=end, `args[2]`=data | — |
| | `[I, I, L]` | `[L]` | Same for lists | `arg2: L` |
| | `[I, S]` | `[S]` | Start only | — |
| | `[I, L]` | `[L]` | | `arg1: L` |
| | `[S]` | `[S]` | Copy (identity) | — |
| | `[L]` | `[L]` | | `arg0: L` |
| `split` | `[S, S, M]` | `[L]` | Opts: {cs:B, mode:A, norm:A, trim:B, lim:I} | — |
| | `[S, S]` | `[L]` | `args[0]`=separator, `args[1]`=input | — |
| `trim` | `[S, M]` | `[S]` | Opts: {chars:S, side:A} | — |
| | `[S]` | `[S]` | Default whitespace trim | — |
| | `[A, M]` | `[S]` | | — |
| | `[A]` | `[S]` | | — |
| `upper` | `[S]` | `[S]` | Uppercase | — |
| | `[A]` | `[S]` | | — |


## Boolean Operations

| Word | Signatures (match order) | Returns | Notes | Data Arg |
|------|--------------------------|---------|-------|----------|
| `and` | `[B, B]` | `[B]` | Logical AND | — |
| `implies` | `[B, B]` | `[B]` | `!args[1] \|\| args[0]` (reversed) | — |
| `nand` | `[B, B]` | `[B]` | Logical NAND | — |
| `not` | `[B]` | `[B]` | Logical NOT | — |
| `or` | `[B, B]` | `[B]` | Logical OR | — |
| | `[Any, Any]` | `[Disjunct]` | Creates/flattens disjunction union | — |
| `xor` | `[B, B]` | `[B]` | Logical XOR | — |


## Comparison

| Word | Signatures (match order) | Returns | Notes | Data Arg |
|------|--------------------------|---------|-------|----------|
| `deq` | `[Any, Any]` | `[B]` | Deep equality (traverses lists/maps) | — |
| `eq` | `[Any, Any]` | `[B]` | Exact equality (identity for non-scalars) | — |
| `gt` | `[Any, Any]` | `[B]` | `args[1] > args[0]` (reversed) | — |
| `gte` | `[Any, Any]` | `[B]` | `args[1] >= args[0]` (reversed) | — |
| `lt` | `[Any, Any]` | `[B]` | `args[1] < args[0]` (reversed) | — |
| `lte` | `[Any, Any]` | `[B]` | `args[1] <= args[0]` (reversed) | — |
| `neq` | `[Any, Any]` | `[B]` | Not equal (negation of eq) | — |


## Accessors

| Word | Signatures (match order) | Returns | Notes | Data Arg |
|------|--------------------------|---------|-------|----------|
| `dot` / `.` | `[A, Node]` | `[Any]` | Property access; returns None if missing | — |
| | `[S, Node]` | `[Any]` | | — |
| | `[I, Node]` | `[Any]` | List index or map key | — |
| | `[A, Object]` | `[Any]` | Object field access | — |
| | `[S, Object]` | `[Any]` | | — |
| | `[I, Object]` | `[Any]` | | — |
| | `[Any, None]` | `[None]` | None propagation | — |
| `dotr` / `!.` | `[M, A]` | `[Any]` | Strict: error if key not found | — |
| | `[M, S]` | `[Any]` | | — |
| | `[L, I]` | `[Any]` | Strict: error if index OOB | `arg0: L` |
| | `[M, I]` | `[Any]` | | — |
| | `[None, Any]` | error | Error if parent is None | — |


## Storage

| Word | Signatures (match order) | Returns | Notes | Data Arg |
|------|--------------------------|---------|-------|----------|
| `context` | `[W]` | `[W]` | Dispatcher: converts set/get to context-set/context-get | — |
| `context-get` | `[S]` | `[Any]` | Returns None if key not in context | — |
| | `[A]` /q | `[Any]` | | — |
| `context-set` | `[S, Any]` | `[]` | Store in scoped context | — |
| | `[A, Any]` /q | `[]` | | — |
| `get` | `[Any]` | `[Any]` | Retrieve from store (or return as-is) | — |
| `set` | `[S, Any]` | `[]` | Store key=value globally | — |
| | `[A, Any]` /q | `[]` | | — |


## Definition

| Word | Signatures (match order) | Returns | Notes | Data Arg |
|------|--------------------------|---------|-------|----------|
| `__pa` /s | `[]` | `[]` | Internal: pops args stack | — |
| `args` | `[]` | `[L]` | Returns current fn args from argsStack | — |
| `call` | `[L]` | `[Any...]` | Splices list contents onto stack | `arg0: L` |
| `dblcall` | `[I, L]` | `[Any...]` | Doubles integer, then calls body | `arg1: L` |
| `def` | `[S, Any]` | `[]` | Define word (literal or fn body) | — |
| | `[A, Any]` /q | `[]` | | — |
| `fn` | `[L]` | `[Function]` | Parse signature triples from list | `arg0: L` |
| `undef` | `[S]` | `[]` | Remove word definition | — |
| | `[A]` /q | `[]` | | — |
| | `[S, FnUndef]` | `[]` | Targeted undef with signature info | — |
| | `[A, FnUndef]` /q | `[]` | | — |
| `var` | `[L]` | `[Any]` | Define scoped variable; returns body result | `arg0: L` |


## Control Flow

| Word | Signatures (match order) | Returns | Notes | Data Arg |
|------|--------------------------|---------|-------|----------|
| `break` /s | `[]` | `[]` | Throws break sentinel to exit for loop | — |
| `continue` /s | `[]` | `[]` | Throws continue sentinel; skip to next iteration | — |
| `do` | `[L]` | `[Any...]` | Evaluate list as sub-program | `arg0: L` |
| | `[M]` | `[M]` | Evaluate map values depth-first | — |
| `error` | `[Error, L]` | `[Any...]` | Handle error with handler list | `arg1: L` |
| | `[Error]` | `[]` | Consume and print error | — |
| `for` | `[I, L]` | `[Any...]` | Numeric loop; body in `args[1]` | `arg1: L` |
| | `[L, L]` | `[Any...]` | Range loop `[start,end,step]`; body in `args[1]` | `arg0: L` (range), `arg1: L` (body) |
| `if` | `[Any, Any, Any]` | `[Any...]` | `args[0]`=cond, `args[1]`=then, `args[2]`=else | — |
| | `[Any, Any]` | `[Any...]` | `args[0]`=cond, `args[1]`=then | — |
| `quote` | `[W]` | `[A]` | Converts word to atom (prevents evaluation) | — |
| | `[Any]` | `[Any]` | Sets Quoted=true (prevents auto-eval of lists) | — |


## Type Operations

| Word | Signatures (match order) | Returns | Notes | Data Arg |
|------|--------------------------|---------|-------|----------|
| `base` | `[Any]` | `[Any]` | Returns zero/base value for a type | — |
| `convert` | `[ScalarType, M, Scalar]` | `[Scalar]` | Opts: {base:S}; convert with options | — |
| | `[ScalarType, Scalar]` | `[Scalar]` | Convert to target scalar type | — |
| `fulltypeof` | `[Any]` | `[A]` | Full type path (e.g. `Scalar/String`) | — |
| `inspect` | `[W]` | `[M]` | Word/type inspection | — |
| | `[A]` | `[M]` | | — |
| | `[Node]` | `[M]` | | — |
| | `[Scalar]` | `[M]` | | — |
| `is` | `[Any, Any]` | `[B]` | Unification-based type/value check | — |
| `make` | `[Object, Any, Object]` | `[Object]` | Object from source with prototype | — |
| | `[Any, Any, M]` | `[Any]` | Type + source + options | — |
| | `[Any, Any]` | `[Any]` | Type + source | — |
| `object` | `[M]` | `[Object]` | Define object type from field map | — |
| | `[M, Any]` | `[Object]` | Object type with parent | — |
| `record` | `[L]` | `[Record]` | Define record type from field list | `arg0: L` |
| `table` | `[Any]` | `[Table]` | Define table from record type | — |
| `type` | `[S, Any]` | `[]` | Register named type | — |
| | `[A, Any]` /q | `[]` | | — |
| `typeof` | `[Any]` | `[A]` | Short type name (e.g. `String`) | — |


## I/O

| Word | Signatures (match order) | Returns | Notes | Data Arg |
|------|--------------------------|---------|-------|----------|
| `print` | `[Any]` | `[]` | Print value to stdout with newline | — |
| `printstr` | `[Any]` | `[]` | Print value to stdout without newline | — |
| `read` | `[S, M]` | `[Any]` | Opts: {enc, fmt, nl}; read file | — |
| | `[S]` | `[Any]` | Read file at path | — |
| `stderr` | `[]` | `[S]` | Push stderr path string | — |
| `stdin` | `[]` | `[S]` | Push stdin path string | — |
| `stdout` | `[]` | `[S]` | Push stdout path string | — |
| `trace` | `[L]` | `[Any...]` | Evaluate list with step-by-step trace output | `arg0: L` |
| `write` | `[S, S, M]` | `[S]` | Opts: {enc, fmt, mode, nl}; write string to file | — |
| | `[S, Any, M]` | `[S]` | Write non-string data (auto-format as jsonic) | — |
| | `[S, S]` | `[S]` | Write string to file at path | — |


## Module

| Word | Signatures (match order) | Returns | Notes | Data Arg |
|------|--------------------------|---------|-------|----------|
| `import` | `[Module]` | `[]` | Import all exports as defs | — |
| | `[L, Module]` | `[]` | Rename imports via list | `arg0: L` |
| | `[A, Module]` | `[]` | Rename single export | — |
| | `[S]` | `[]` | Import from file path | — |
| | `[L, S]` | `[]` | Import from file with renames | `arg0: L` |
| | `[A, L]` /q(0) | `[]` | Inline module: `import name [body]` | `arg1: L` |
| | `[L, A, L]` /q(1) | `[]` | Inline module with renames | `arg0: L` (renames), `arg2: L` (body) |
| | `[A, A, L]` /q(1) | `[]` | Inline module single rename | `arg2: L` |
| `module` | `[L]` | `[Module]` | Define module from body list | `arg0: L` |


## Unification

| Word | Signatures (match order) | Returns | Notes | Data Arg |
|------|--------------------------|---------|-------|----------|
| `unify` | `[Any, Any]` | `[S, B]` | Returns unified result string + success boolean | — |


## Help

| Word | Signatures (match order) | Returns | Notes | Data Arg |
|------|--------------------------|---------|-------|----------|
| `help` | `[S]` | `[]` | Print help for named topic | — |
| | `[A]` | `[]` | | — |
| | `[A]` /q | `[]` | | — |
| | `[]` | `[]` | Print general help (no args) | — |


## Notes on Argument Ordering

Several words reverse their handler arguments relative to the signature
position. This is because with forward-first collection, `a b word` results
in `args[0]=a, args[1]=b`, but the mathematical operation needs
`args[1] op args[0]` to produce the natural `a op b` result:

- **sub, div, mod, pow**: compute `args[1] op args[0]`
- **lt, gt, lte, gte**: compare `args[1] vs args[0]`
- **atan2**: `atan2(args[1], args[0])`
- **add** (scalar concat): `args[1] + args[0]`
- **implies**: `!args[1] || args[0]`
- **contains, indexof**: `args[0]`=needle, `args[1]`=haystack
- **split**: `args[0]`=separator, `args[1]`=input
- **replace**: `args[0]`=pattern, `args[1]`=replacement, `args[2]`=input
- **pad**: `args[2]`=input string in 3-arg form
