# AQL Formal Language Specification

Status: draft formal specification for the current AQL language as implemented by
this repository.

This document is normative for the core language model. Existing reference
manuals, implementation comments, and executable `.tsv` spec files remain useful
explanatory and conformance material, but this document is intended to state the
language rather than redesign it.

## 1. Scope and Notation

### 1.1 Scope

AQL is a concatenative, typed, stack-transforming language. A program is a finite
sequence of tokens evaluated against a registry of words, types, modules, and
capabilities. The core language includes:

- lexical forms for literals, words, grouping, comments, lists, maps, member
  access, template strings, and word modifiers;
- concrete and abstract syntax for programs and data literals;
- static semantics for type names, function signatures, argument collection,
  lexical capture, module visibility, and errors detected before or during word
  dispatch;
- dynamic small-step semantics for stack evaluation, forward collection,
  grouping, quotations, user-defined functions, control values, interpolation,
  maps, lists, and errors;
- a boundary between the language kernel and the standard/native word library;
- conformance expectations grounded in the repository's executable spec files.

The complete behavior of each native word is not restated here. Native word
semantics are part of the standard library boundary defined in Section 9.

### 1.2 Metalanguage

This specification uses the following notation.

- `::=` defines grammar productions.
- `{ X }` means zero or more repetitions of `X`.
- `[ X ]` means optional `X`.
- `X | Y` means alternatives.
- Terminals are written in double quotes or as named lexical classes.
- `Γ` is a static environment: type names, word bindings, module bindings,
  lexical captures, and capability policy.
- `Σ` is an implementation store for resources, modules, caches, and effects.
- `R` is the active word registry.
- `S` is the data stack, written left-to-right with the top at the right:
  `S · v` means stack `S` with top value `v`.
- `T` ranges over types; `τ(v)` is the dynamic type carried by value `v`.
- `T1 ⊑ T2` means type `T1` is a subtype or refinement that matches `T2`.
- `v : T` means `τ(v) ⊑ T`, or, for structural/refinement types, `v` satisfies
  the type predicate.
- `⟨E, S, R, Σ⟩ → ⟨E', S', R', Σ'⟩` is a one-step dynamic transition where
  `E` is the remaining expression/tape.
- `error(c, m)` is an AQL error with code `c` and diagnostic message `m`.

## 2. Lexical Grammar

AQL source is a Unicode character stream. Implementations may preserve exact
source positions for diagnostics. Unless stated otherwise, lexical comparison of
keywords and word names is case-sensitive.

### 2.1 Whitespace and comments

Whitespace separates tokens except inside strings and template literal segments.
Whitespace has no semantic effect except as a separator.

```ebnf
White       ::= " " | "\t" | "\r" | "\n" | other Unicode space
LineComment ::= "#" { not-newline } ("\n" | EOF)
BlockComment ::= "##" { any character not closing the block } "##"
Trivia      ::= { White | LineComment | BlockComment }
```

A `#` begins a line comment only when lexed as a comment delimiter by the host
lexer, not when it appears inside a quoted string or template literal segment.
Block comments do not nest.

### 2.2 Reserved punctuation and keywords

```ebnf
OpenParen   ::= "("
CloseParen  ::= ")"
OpenList    ::= "["
CloseList   ::= "]"
OpenMap     ::= "{"
CloseMap    ::= "}"
Colon       ::= ":"
Comma       ::= ","
Dot         ::= "."
Bang        ::= "!"
Question    ::= "?"
Pipe        ::= "|"
Arrow       ::= "=>"
Backtick    ::= "`"
InterpOpen  ::= "${"
End         ::= "end" | ";"
NoneLit     ::= "none"
BoolLit     ::= "true" | "false"
```

`end` and `;` are equivalent tape barriers. `=>` is lexical sugar for the word
`afn`.

### 2.3 Identifiers and words

A word token is any unquoted text token accepted by the implementation lexer that
is not parsed as a number, string, boolean, `none`, punctuation, or container
syntax.

For portable AQL programs, identifiers SHOULD be restricted to:

```ebnf
IdentStart  ::= UnicodeLetter | "_"
IdentRest   ::= IdentStart | UnicodeDigit | "-"
Ident       ::= IdentStart { IdentRest }
```

Implementations MAY accept a broader JSONic-style unquoted text token space for
backward compatibility. Portable code MUST quote map keys or strings when the
text is not an `Ident`.

### 2.4 Word modifiers

A word may have a trailing modifier suffix.

```ebnf
WordToken   ::= WordName [ WordModifier ]
WordName    ::= nonempty unquoted text not ending in a valid WordModifier
WordModifier ::= "/" ModifierChar { ModifierChar }
ModifierChar ::= Digit | "f" | "s" | "q" | "r"
```

A valid modifier suffix obeys these constraints:

1. at most one contiguous decimal argument-count component appears;
2. `f` and `s` are mutually exclusive;
3. `q` and `r` are mutually exclusive;
4. each letter component appears at most once;
5. the argument count, if present, is a non-negative decimal integer.

Invalid suffixes are not modifiers; the whole token is treated as a plain word.

The modifier meanings are:

| Modifier | Abstract effect |
|----------|-----------------|
| `/q` | produce the atom named by the base word; other modifier components are ignored |
| `/r` | produce a function reference value for the base word without dispatch |
| `/f` | force forward-only argument collection |
| `/s` | force stack-only argument collection |
| `/N` | force exactly `N` arguments |
| combined `/Nf`, `/Ns` | force count and collection side |

### 2.5 Literals

```ebnf
IntegerLit  ::= [ "-" ] Digit { Digit }
DecimalLit  ::= [ "-" ] Digit { Digit } "." Digit { Digit }
Digit       ::= "0" | "1" | "2" | "3" | "4" | "5" | "6" | "7" | "8" | "9"
StringLit   ::= DoubleString | SingleString
DoubleString ::= '"' { StringChar | Escape } '"'
SingleString ::= "'" { StringChar | Escape } "'"
Escape      ::= "\\" ( "\\" | '"' | "'" | "n" | "t" | "r" | "$" | "`" )
TemplateLit ::= "`" { TemplateSegment | Interpolation } "`"
TemplateSegment ::= { character other than unescaped "`" or "${" }
Interpolation ::= "${" Program "}"
```

Template literal escape sequences are `\\`, ``\` ``, `\$`, `\n`, `\t`, and
`\r`. Unknown template escapes preserve the escaped character sequence in the
implementation's source-compatible form.

### 2.6 Type-name tokens

A bare token that resolves to a built-in or in-scope user-defined type name is a
type literal in word context. Built-in short names include `Any`, `None`,
`Never`, `Scalar`, `Atom`, `Boolean`, `Number`, `Integer`, `Decimal`, `String`,
`List`, `Map`, `Record`, `Options`, `Store`, `Table`, `Timeout`, `Interval`, and
`Function`, along with full slash-separated type paths accepted by the engine.

## 3. Concrete Syntax Grammar

The concrete grammar below describes the portable language. The implementation
may accept additional JSONic-compatible surface forms if they lower to the same
abstract syntax.

```ebnf
Program       ::= Trivia { Item Trivia } EOF
Item          ::= Expr | End
Expr          ::= Primary { MemberAccess }
Primary       ::= Literal
                | Word
                | Group
                | List
                | Map
                | TypedList
                | TypedMap

Literal       ::= IntegerLit | DecimalLit | StringLit | TemplateLit | BoolLit | NoneLit
Word          ::= WordToken
Group         ::= "(" Program ")"

List          ::= "[" Trivia [ ListElems ] Trivia "]"
ListElems     ::= Expr { Trivia [ "," ] Trivia Expr }
TypedList     ::= "[" Trivia ":" Trivia TypeExpr
                  { Trivia [ "," ] Trivia Expr } Trivia "]"

Map           ::= "{" Trivia [ MapElems ] Trivia "}"
MapElems      ::= MapElem { Trivia [ "," ] Trivia MapElem }
MapElem       ::= MapPair | OptionalMapPair | ComputedMapPair | MapShorthand
MapPair       ::= MapKey Trivia ":" Trivia Expr
OptionalMapPair ::= MapKey Trivia "?" Trivia ":" Trivia Expr
ComputedMapPair ::= "[" Program "]" Trivia ":" Trivia Expr
MapShorthand  ::= Ident [ WordModifier ] [ "?" ]
MapKey        ::= Ident | StringLit
TypedMap      ::= "{" Trivia ":" Trivia TypeExpr
                  { Trivia [ "," ] Trivia MapElem } Trivia "}"

MemberAccess  ::= "." Trivia MemberKey
                | "!" Trivia "." Trivia MemberKey
MemberKey     ::= Ident | StringLit | Group

TypeExpr      ::= Expr
```

### 3.1 Commas

Commas are optional separators in lists and maps. `[1, 2, 3]` and `[1 2 3]`
are equivalent. `{a:1, b:2}` and `{a:1 b:2}` are equivalent.

### 3.2 Dot access lowering

A member-access chain lowers before evaluation.

```text
x . k        ≡  ( x get k )
x ! . k      ≡  ( x getr k )
x . a . b    ≡  ( x get a get b )
```

A lone `.` token lowers to the word `get`. A `!.` sequence lowers to the word
`getr`.

### 3.3 Map shorthand lowering

Map shorthand lowers structurally before map evaluation.

```text
{foo}       ≡ {foo: foo}
{foo/r}     ≡ {foo: foo/r}
{foo/q}     ≡ {foo: foo/q}
{foo?}      ≡ {foo?: foo}
{foo/r?}    ≡ {foo?: foo/r}
```

The key is the base word name. The value is the whole word token, including a
valid modifier suffix. A modifier suffix on an explicit bare key is illegal:
`{foo/r: 1}` MUST raise `aql/illegal_key`. To use `/` as key data, quote the key
or compute it.

### 3.4 Optional-field lowering

A field written `k?: T` lowers its value type to:

```text
disjunct(T, None, Absent)
```

`Absent` is an internal type used by structural unification to represent a
missing field. The value may be present with a value matching `T`, present as
`none`, or absent.

### 3.5 Parentheses

At top level and list word-context positions, parentheses are tape markers that
force an inner expression to evaluate before surrounding forward collection can
consume past the closing marker. In data contexts, a parenthesized expression is
stored as a parenthesized expression node and evaluated when its containing value
auto-evaluates.

Unmatched opening parentheses are syntax errors. Unmatched closing parentheses
are syntax errors or runtime syntax errors, depending on when they are detected.

## 4. Abstract Syntax

AQL implementations parse source into a sequence of abstract values and control
markers. The following abstract syntax is normative; concrete parser details are
not.

```ebnf
Tape       ::= Item*
Item       ::= Value | WordCall | WordRef | EndMark | OpenMark | CloseMark
Value      ::= Scalar | TypeLiteral | ListValue | MapValue | InterpString
Scalar     ::= Integer | Decimal | String | Boolean | None | Atom
WordCall   ::= word(name, modifier?)
WordRef    ::= ref(name)
TypeLiteral ::= type(T)
ListValue  ::= list(items: Tape, eval: Bool, child?: Value)
MapValue   ::= map(entries: ordered KeyValue*, eval: Bool, child?: Value)
KeyValue   ::= key: String, value: Value | ExprValue, metadata?
InterpString ::= interp(parts: (StringSegment | Tape)*)
OpenMark   ::= "("
CloseMark  ::= ")"
EndMark    ::= "end"
ExprValue  ::= paren(Tape)
```

Important abstract distinctions:

1. `word(x)` dispatches a binding named `x` unless it resolves as a data binding
   that pushes a value.
2. `ref(x)` resolves `x` to a function value and pushes that value without
   dispatch; non-function references are runtime errors.
3. `Atom("x")` is scalar data; it does not resolve.
4. A `ListValue` created by source list syntax is evaluable code by default in
   word context, but it may be protected by quoting or by native/user function
   signatures that mark the argument as no-evaluation.
5. A `MapValue` created by explicit source map syntax auto-evaluates its values
   unless it is quoted or consumed by a no-evaluation argument slot.
6. Ordering of map entries is deterministic by key for synthesized/parsed maps
   unless an operation explicitly preserves insertion order.

## 5. Type System and Static Semantics

### 5.1 Type universe

AQL values carry hierarchical types. The root is `Any`; `Never` is empty;
`None` is the singleton type inhabited only by `none`; scalar, node, ideal, word,
and type families are children of `Any`.

The core hierarchy is:

```text
Any
├── None
├── Never
├── Scalar
│   ├── Atom
│   ├── Boolean
│   ├── Number
│   │   ├── Integer
│   │   └── Decimal
│   ├── String
│   │   ├── EmptyString
│   │   └── ProperString
│   └── Path
├── Node
│   ├── List
│   └── Map
├── Ideal
│   ├── Object
│   │   └── Resource
│   │       └── Entity
│   ├── Array
│   ├── Record
│   ├── Options
│   ├── Error
│   ├── Store
│   ├── Table
│   ├── Fetch
│   │   ├── Request
│   │   └── Response
│   ├── Timeout
│   ├── Interval
│   └── Tensor
│       ├── Matrix
│       └── Vector
├── Word
└── Type
    ├── Function
    ├── FunctionSignature
    └── Disjunct
```

External modules may register additional children. A child type matches its
ancestors.

```text
T ⊑ T                                  (T-Refl)
T1 ⊑ T2 ∧ T2 ⊑ T3 ⇒ T1 ⊑ T3           (T-Trans)
τ(v) ⊑ T ⇒ v : T                       (T-Value)
```

### 5.2 Refinement and structural types

AQL supports nominal refinements, predicate refinements, disjunctions, typed
lists, typed maps, records, options, objects, and tables.

- `refine Base` creates a nominal newtype. Plain `Base` values do not match the
  newtype unless constructed or rebound through a type boundary.
- `Base op constraint`-style dependent scalar refinements are value-sensitive
  subset types. A base value matches if it has the correct base family and
  satisfies the predicate.
- `A tor B` creates a disjunctive type. A value matches if it matches at least
  one alternative.
- `[:T]` is the type of lists whose elements all match `T`.
- `{:T}` is the type of maps whose values all match `T`.
- `refine Record [k:T ...]` creates a record type requiring exactly the declared
  field shape unless the field type contains `Absent` through optional-field
  lowering.
- `refine Options {k:default ...}` creates an options type; concrete defaults
  are inserted or accepted when omitted, while type literals require callers to
  provide a matching value.
- `refine Table R` creates a table type whose rows match record type `R`.

Disjunction rule:

```text
v : T_i for some i ∈ 1..n
-------------------------
v : disjunct(T_1, ..., T_n)
```

Typed list rule:

```text
∀ e ∈ elems. e : T
------------------
list(elems) : List[T]
```

Typed map rule:

```text
∀ (k ↦ v) ∈ entries. v : T
--------------------------
map(entries) : Map[T]
```

Record rule, where `fields(R)` maps keys to required field types:

```text
keys(m) = keys(fields(R)) modulo fields admitting Absent
∀ k present in m. m[k] : fields(R)[k]
∀ k missing from m. Absent : fields(R)[k]
-----------------------------------------
m : R
```

### 5.3 Static environment

`Γ` contains at least:

- type-name bindings, including user-defined names introduced by `def Name ...`
  when the right-hand side is a type;
- value bindings introduced by `def`, `var`, module import, lexical function
  parameters, and implementation-defined host bindings;
- function bindings with one or more signatures;
- module bindings and exported names;
- capability bindings for effectful operations.

Bindings are resolved from the innermost lexical/function scope outward, then
through the active module and registry. A local binding shadows an outer binding
with the same name. Function values may capture lexical bindings present when the
function is defined.

### 5.4 Function signatures

A function signature is a pair of an input parameter list and an output type
list, with a body for defined functions.

```ebnf
FnDefSpec   ::= "fn" ListValue
FnSigTriple ::= InputSig OutputSig Body
InputSig    ::= "[" { Param | Barrier } "]"
Param       ::= [ Name ":" ] TypeExpr [ "?" ]
Barrier     ::= "|"
OutputSig   ::= TypeExpr | "[" { TypeExpr } "]"
Body        ::= ListValue | Expr
```

A function definition list contains one or more triples:

```text
fn [[in1] [out1] [body1] [in2] [out2] [body2] ...]
```

A function-shape type contains pairs without bodies:

```text
fn [[in1] [out1] [in2] [out2] ...]
```

Named parameters are bound by name while evaluating a function body. Unnamed
parameters are provided positionally. If an output signature consists entirely
of concrete non-type values, it is return-by-value sugar: those values are
appended to the body after an `end`, and the static return types are `Any`.

### 5.5 Forward/stack barrier in signatures

The optional `|` marker in an input signature defines a collection barrier.

- No `|`: all parameters are forward-eligible by default.
- `[| a:T b:U]`: all parameters are stack-only.
- `[a:T | b:U]`: `a` is forward-eligible; `b` and later parameters must come
  from the stack.

Let `barrier(sig)` be the number of forward-eligible parameters.

### 5.6 Signature matching and dispatch

Given a candidate signature `σ = ([p_0:T_0, ..., p_{n-1}:T_{n-1}], returns)`, an
argument vector `args` matches `σ` iff:

```text
len(args) = n
∀ i ∈ 0..n-1. args[i] : T_i
```

For overloaded words, signatures are tried in registry order. The first matching
signature is selected. Failure to find a matching signature is a signature error.
Implementations SHOULD prefer signatures that avoid accidentally consuming a
following function word when a non-consuming signature can match, preserving the
left-to-right word boundary behavior of the reference engine.

### 5.7 Return checking

After a user-defined function body runs, the produced return segment must match
the output signature.

```text
∀ i. ret_i : Out_i
------------------
Γ ⊢ return(ret_0..ret_n) ok
```

A return mismatch raises a return-value error. Functions that declare zero
returns must leave no new return values, after accounting for the caller's
pre-call stack prefix.

### 5.8 Static errors

A conforming implementation MUST diagnose the following as errors no later than
the point at which the relevant construct is parsed, checked, or dispatched:

- unmatched opening or closing parenthesis;
- illegal map-key modifier on explicit bare keys;
- malformed function signatures or output signatures;
- unresolved type names in type/signature context;
- duplicate or contradictory function parameter forms where rejected by the
  canonical signature parser;
- arity or signature mismatches when no overload can accept collected arguments;
- return-value count or type mismatches;
- illegal `/r` reference to a non-function binding;
- capability denial for effectful words;
- module import or visibility failures.

## 6. Dynamic Semantics

### 6.1 Values and stacks

Evaluation starts with an initial state:

```text
⟨program, [], R0, Σ0⟩
```

The final result of a successful program is its final stack. If evaluation
raises `error(c, m)`, execution stops and reports that error.

### 6.2 Literal and type-literal rules

```text
⟨v E, S, R, Σ⟩ → ⟨E, S · v, R, Σ⟩
```

This applies to scalar values, type literals, quoted atom values, and function
references after reference resolution.

### 6.3 Word resolution

A word token `w` resolves in `R` and `Γ` as follows:

1. If `w` names a value binding, push the bound value, unless the binding is a
   function and the token is a normal call.
2. If `w` names a function binding and the token is a normal call, dispatch it
   using the collection rules below.
3. If `w` names a type, push the type literal.
4. If `w` is unresolved in a context accepting atoms, it may be treated as an
   atom only when explicitly quoted by `/q` or by a quoting word; otherwise an
   unresolved-word error is raised when dispatch/resolution requires a binding.

The `none`, `true`, and `false` literals do not resolve as words.

### 6.4 Argument collection

When a function word `f` is encountered, the evaluator chooses an overload by
attempting to collect arguments for its signatures. Collection has two sources:
future tokens to the right of `f`, and already-evaluated values on the stack.

For a signature with `n` parameters and barrier `b`, forward collection may fill
only slots `0..b-1`.

Forward collection scans future tokens left-to-right. It stops at:

- `end`;
- `)` matching the current group;
- a token that evaluates to a value not matching the next forward-eligible
  parameter;
- a function word or syntactic control token that the dispatcher chooses not to
  pre-evaluate for collection;
- end of tape.

Collected forward values fill `args[0]`, then `args[1]`, and so on. Remaining
slots are filled from the stack, top first, into the next unfilled slot.

For `f` with selected signature `σ`:

```text
collect(E, S, σ) = (args, E', S')
args matches σ
native_or_user_apply(f, args, S', R, Σ) = (S'', R', Σ')
-------------------------------------------------------
⟨f E, S, R, Σ⟩ → ⟨E', S'', R', Σ'⟩
```

For asymmetric operations, handlers receive the same argument vector regardless
of source-side spelling. Example with two arguments:

```text
10 3 sub        args[0] = 3, args[1] = 10
10 sub 3        args[0] = 3, args[1] = 10
sub 3 10        args[0] = 3, args[1] = 10
```

### 6.5 Word modifier dynamic rules

- `/q`: `x/q` steps as `Atom("x")` and never dispatches.
- `/r`: `x/r` resolves `x` to a function value and pushes it. If `x` does not
  resolve to a function, raise `illegal_ref`.
- `/s`: no future tokens may be used during collection.
- `/f`: no stack values may be used during collection.
- `/N`: dispatch uses exactly `N` arguments if a compatible signature can be
  selected or adapted by the native word's argument-shape protocol.

### 6.6 End barrier

```text
⟨end E, S, R, Σ⟩ → ⟨E, S, R, Σ⟩
```

`end` is inert when reached directly. Its primary effect is as a barrier that
prevents the nearest waiting word from consuming later tokens as forward
arguments.

### 6.7 Parentheses

A parenthesized group evaluates to completion before the enclosing tape proceeds.
If the group produces stack suffix `G`, that suffix is spliced into the outer
stack.

```text
⟨G, [], R, Σ⟩ →* ⟨[], SG, R', Σ'⟩
------------------------------------------------
⟨( G ) E, S, R, Σ⟩ → ⟨E, S · SG, R', Σ'⟩
```

The group boundary also stops forward collection from words outside the group.

### 6.8 Lists and quotation

A source list in word context denotes a `List` value whose elements are AQL tape
items. Lists are evaluated by default only at specified auto-evaluation points:
when run by `do`, spliced by `call`, consumed by a word that evaluates code, or
when the engine's list auto-evaluation contract for parser-created lists applies.

The `quote` word and no-evaluation argument slots prevent auto-evaluation and
preserve list contents as data.

Representative rules:

```text
⟨[G] E, S, R, Σ⟩ → ⟨E, S · list(G, eval=true), R, Σ⟩

quote v  ⇒ push v with eval=false where applicable

do [G]  ⇒ evaluate G in the current registry and stack discipline
call [G] ⇒ splice G into the current tape
```

### 6.9 Maps

An explicit source map denotes a `Map` value. Its values auto-evaluate unless the
map is quoted or consumed by a no-evaluation slot. A computed key `[K]: V` first
evaluates `K`; the resulting scalar key is converted to a string key by the
standard key conversion rule. Values are evaluated according to ordinary AQL
rules in map value context.

```text
∀ entries k_i ↦ expr_i. eval_map_value(expr_i) = v_i
-----------------------------------------------------
{ k_1: expr_1 ... k_n: expr_n } ⇓ map(k_1 ↦ v_1, ..., k_n ↦ v_n)
```

### 6.10 Template strings

A template string evaluates each interpolation expression in source order. Each
expression must produce a value convertible to string by the standard string
conversion used by interpolation. Literal segments are concatenated with the
converted interpolation values.

```text
eval(E_i) = v_i
s = seg_0 ++ string(v_1) ++ seg_1 ++ ... ++ string(v_n) ++ seg_n
----------------------------------------------------------------
`seg_0 ${E_1} seg_1 ... ${E_n} seg_n` ⇓ s
```

A template with no interpolation is equivalent to a plain string literal after
escape processing.

### 6.11 User-defined functions

Applying a user-defined function signature performs:

1. bind named parameters in a fresh function scope;
2. push or bind unnamed positional parameters according to the function calling
   convention;
3. evaluate the function body in the function registry/scope, using captured
   lexical bindings if the function was defined in a module or closure context;
4. check returns against the output signature;
5. splice return values onto the caller stack.

```text
bind(params, args, Γ) = Γ_f
eval(body, Γ_f, R_f, Σ, S_body) = (ret, R_f', Σ')
ret matches outputs
--------------------------------------------------
apply_user(f, args, S, R, Σ) = (S · ret, R, Σ')
```

Definitions created inside a function body affect the function-local environment
unless the defining word explicitly mutates an outer or module scope.

### 6.12 Definitions and mutation

`def` introduces an immutable binding. If the left-hand side carries a type
annotation, the right-hand side is unified with that type before binding.

```text
Γ ⊢ e ⇓ v     v : T
-------------------
def x:T e  ⇒ Γ[x ↦ v:T]
```

`var` introduces a mutable binding whose update rules are defined by the native
word library and store model. Mutation preserves type invariants declared at the
binding boundary.

### 6.13 Errors

Errors are values in parts of the library and control states in the evaluator.
When an evaluator-level error is raised, normal stepping stops. Libraries may
also return error-typed values where a word's signature declares such behavior.
Error codes are namespaced strings such as `syntax_error`, `signature`,
`illegal_key`, `illegal_ref`, or capability/module-specific codes.

## 7. Memory, Effects, Capabilities, and Concurrency

### 7.1 Store model

Pure AQL expressions transform only the stack and registry. Effectful native
words may also transform `Σ`, which includes external resources, filesystem
state, network state, SQLite handles, module caches, vault state, and host
capabilities.

A conforming implementation MUST preserve the observable order of effects for
left-to-right evaluation except where a concurrency word explicitly permits
parallel or interleaved execution.

### 7.2 Capability model

Effectful words that access files, network, subprocesses, secrets, or other host
resources are guarded by capabilities. A capability check is part of the word's
precondition:

```text
capability_allowed(Γ, Σ, op, resource)
args match σ
--------------------------------------
apply_effectful(op, args, Σ) succeeds
```

If the capability predicate is false, the word raises a capability error and
MUST NOT perform the denied effect.

### 7.3 Mutable bindings and resources

Mutable bindings and resource/object values are references into `Σ` or
implementation-managed cells. Implementations MUST NOT expose data races through
safe AQL constructs. If host resources are shared, their native word definitions
must specify serialization, atomicity, or error behavior.

### 7.4 Concurrency

Concurrency words, including `await`, evaluate multiple code values or effectful
computations and collect their results. Unless a word's documentation states
otherwise:

- result order is source/input order, not completion order;
- each branch receives the lexical environment of the call site;
- branch-local stack effects are isolated and the parent receives only declared
  results;
- capability checks apply independently in each branch;
- if any branch raises an evaluator-level error, the concurrency word raises an
  error and may cancel unfinished branches.

## 8. Module Semantics

A module is an AQL source unit with a module registry, lexical scope, and export
set. Module import performs:

1. resolve a module name/path according to the module search and registry rules;
2. parse and evaluate the module at most once per cache key unless cache policy
   requires reload;
3. expose only exported bindings to the importer, unless an internal import form
   explicitly permits private access;
4. preserve lexical captures for functions exported from the module.

Module-local definitions shadow imported definitions in that module. Importing a
name into another module or script may shadow an outer binding according to the
normal name-resolution rules.

## 9. Standard Library Boundary

The language kernel defines syntax, abstract values, stack evaluation,
collection, type matching, function definition/application, module loading
protocol, and the capability/error framework.

The standard/native library defines concrete words for:

- stack manipulation;
- arithmetic, rounding, numeric functions, constants, and comparison;
- string and boolean operations;
- definition, scoping, and control-flow words;
- list, array, map, object, record, table, and unification operations;
- type construction and inspection;
- I/O, filesystem, networking, SQLite, vault, and other effectful modules;
- concurrency words;
- help, formatting, LSP, registry, and CLI-facing services.

A conforming AQL implementation may implement only the kernel, but it MUST state
which standard/native library profiles it supports. A conforming full
implementation of this repository's language MUST pass the conformance suite in
Section 10 for the profiles it claims.

## 10. Conformance Suite

The executable conformance suite consists of repository spec files and ordinary
unit tests. These files are normative examples: an implementation that claims
compatibility with the current AQL language must produce the same successful
results and the same error classes for the same profile.

Core engine conformance:

- `eng/spec/*.tsv` covers core stack evaluation, literals, numbers, strings,
  lists, type inspection, dispatch, forward collection, barriers, parentheses,
  quoting, resolution, functions, multivalue behavior, patterns, structures,
  shadowing, errors, and make/refinement behavior.

Language/native conformance:

- `lang/spec/*.tsv` covers list/map operations, resources, objects, records,
  storage, user types, comparison, unpacking, and word splicing.
- `lang/go/test/*.tsv` covers native-language behavior such as booleans,
  binary operations, options parameters, signature matching, syntax,
  unification, for-loops, behavior-level examples, and file items.
- `lang/go/test/**/*.aql` contains module and checker fixtures.

CLI and service conformance:

- `cmd/go/internal/**/*_test.go` covers CLI subcommands, REPL behavior, LSP,
  serving, registry, vault, publish/install flows, and API surfaces.

A conformance test has four parts: source program, expected stack or error
class, optional fixture/profile requirements, and human-readable description.
New language behavior MUST add or update positive tests, negative tests, and edge
case tests. Mechanized semantics in Lean, Redex, K, Coq, Isabelle/HOL, or another
formal tool SHOULD be generated from or checked against this document and these
spec files for the core evaluator.

## 11. Open Questions for Spec Completion

The current implementation is ahead of this first formal document in several
areas. The following questions should be resolved before declaring this document
stable:

1. Which JSONic extensions beyond the portable grammar are intentionally part of
   the language rather than parser compatibility?
2. Which standard library profiles should be named for partial conformance
   (`kernel`, `native-core`, `io`, `network`, `sqlite`, `vault`, `cli`)?
3. Should map entry order be specified as sorted-by-key for all constructed maps,
   insertion order, or operation-specific?
4. Which exact error-code strings are normative versus implementation details?
5. What is the smallest Lean model that should be maintained as the mechanized
   core: parser-free abstract machine only, or parser plus abstract machine?
6. Which concurrency cancellation and resource-cleanup guarantees are required
   across all host runtimes?

