# Generic Types — Design and Plan

Status: design draft, no implementation.
Branch: `claude/add-nor-xnor-operators-VDZG7` (parking the design here
while the boolean-operator change is in flight; will move to a
dedicated branch when implementation starts).

## 1. Motivation

The AQL type system already has records, typed lists/maps, fn-shape
types, predicate types, dependent scalars, and a `tand`/`tor`/`Never`/
`Any` algebra. What is missing is **parametric polymorphism** — the
ability to write a single type or function shape that abstracts over
one or more type arguments and is instantiated at use sites.

Concrete pain points users hit today:

- `type Box record [value:Any]` loses precision: a `Box` of `Integer`
  is the same type as a `Box` of `String`. There is no way to say
  "a `Box` whose `value` field has type `T`, for the same `T`
  throughout."
- Container fn-shapes have to be re-declared per element type.
  `type IntMapper fn [[Integer] [Integer]]` and
  `type StrMapper fn [[String] [String]]` have identical structure.
- Higher-order list/map words (`map`, `fold`, `outer`, `inner`) accept
  `TList` / `TAny` because we cannot express "a fn from `T` to `U`"
  in a way the static checker can refine across call sites.
- Predicate types currently parameterise by value (`x:Any`) but not by
  type. Useful constructions like "a predicate that accepts any `T`
  and returns `T` if the guard passes" cannot be encoded.

## 2. Goals and non-goals

**Goals.**

1. Add **type parameters** to records, fn-shape types, predicate types,
   typed-def, and fn definitions.
2. Use the **angle-bracket convention** (`<T>`, `<T, U>`) at both
   declaration and application sites.
3. Support TypeScript-style **constraints** (`extends`) with semantics
   that integrate naturally with the existing `tand`/`tor`/`Never`/
   `Any` algebra rather than reinventing them.
4. Support **defaults** (`<T = Integer>`).
5. Be **inferable** wherever the existing signature-matcher already
   has enough information — e.g. `Box<Integer>` should be inferable
   from a value of `{value: 42}` without an explicit annotation.
6. Stay **concatenative-friendly**: a generic application like
   `Box<Integer>` must be a single value-producing expression that
   slots into existing forward / stack argument flow.
7. Preserve `aql check` (carrier-based static checking) coverage —
   generics must produce carriers that the checker can refine.

**Non-goals (deferred).**

- Higher-kinded types (`<F<_>>`).
- Conditional types (`T extends U ? X : Y`).
- Mapped types (`{[K in keyof T]: …}`).
- Variance annotations richer than the inferred contravariant-input /
  covariant-return rule the fn-shape matcher already implements.
- Generic modules. (Modules can re-export concrete instantiations.)

## 3. Survey of the existing syntactic landscape

What the parser and engine already use, that bears on the design:

- **`<` and `>` are syntactically free.** Comparisons use `lt`, `gt`,
  `lte`, `gte` (`internal/engine/compare.go`). No existing word, sigil,
  or jsonic token consumes `<` or `>`.
- **Type names start with a capital letter, def names lower-case**
  (`LANGREF.md` §"Type and Def Naming"). This case discipline is
  already used to disambiguate words during parsing — the same rule
  will apply inside `<…>` for type parameters.
- **Typed-def uses `:`** — `def x:Integer 42`. The colon is a type
  annotation operator. `<T:Comparable>` would be visually consistent
  with this; we will use `extends` as the primary form (matching the
  user's TS-style request) and keep `:` reserved for future use as a
  shorthand if usability demands it.
- **Type algebra uses `tand` / `tor`** — `Integer tor String`,
  `Integer tand Number`. Constraints can lean on these directly:
  `<T extends Comparable tand Hashable>` is read by the existing
  algebra without a new intersection operator.
- **Fn-shape types already encode variance** (contravariant inputs,
  covariant returns; `LANGREF.md` §"Structural Function-Shape Types").
  Generic fn shapes inherit this for free.
- **`Any` and `Never` are the lattice top and bottom.** Unconstrained
  type parameters default to `extends Any`; `Never`-bounded
  parameters are valid but uninhabited.
- **Custom jsonic tokens** are registered in `parser/grammar.go`
  alongside `(`, `)`, `.`, `;`, `?`, `!`, `|`. Adding `<` and `>`
  follows the same pattern.

## 4. Syntax

### 4.1 Tokenization

Register `<` (`#LA`, "left angle") and `>` (`#RA`, "right angle") as
fixed jsonic tokens, mirroring `(` and `)`:

```go
LA: j.Token("#LA", "<"),
RA: j.Token("#RA", ">"),
```

Effect: `Box<Integer>` lexes as the four tokens
`Box`, `<`, `Integer`, `>` even though they are written without
whitespace. This matches how `foo.bar` already lexes as `foo`, `.`,
`bar`.

**No comparison-operator collision** — AQL has none. We commit to
keeping `<`/`>` reserved for generic-application syntax (and any
future syntax that fits inside `< … >`); comparison stays on the
named-word `lt`/`gt`/`lte`/`gte` family.

### 4.2 Grammar additions

Extend the `"val"` rule with a new alternate for the application
form. The opener is "an identifier or type expression immediately
followed by `<`"; close is the matching `>`. Inside, the rule
collects a comma-separated list of type expressions (the same grammar
as a fn-shape type body, restricted to type-context conversion).

A new `"tparams"` rule pair handles the **declaration** form
(parameter list with optional `extends` clauses and `=` defaults):

```
tparams := '<' tparam (',' tparam)* '>'
tparam  := TypeName ('extends' typeExpr)? ('=' typeExpr)?
```

Both rules use the existing `convertDataValue` machinery for the
inner type expressions, so `tand` / `tor` / `Never` / `Any` /
nested generic applications all work without further plumbing.

### 4.3 Declaration sites

#### Generic record types

```
type Box<T> record [value:T]
type Pair<K, V> record [key:K  value:V]
type Tree<T> record [value:T  left:Tree<T>  right:Tree<T>]
```

`T`, `K`, `V` are bound only inside the record body. They follow the
type-name capitalisation rule (the parser rejects lowercase
parameters: `type Box<t>` is a hard error).

#### Generic fn-shape types

```
type Mapper<T, U> fn [[T] [U]]
type Reducer<T, A> fn [[A T] [A]]
type Predicate<T> fn [[T] [Boolean]]
```

The contravariance/covariance rules of fn-shape matching apply to the
**instantiated** shape, not to the type parameters themselves —
`Mapper<Integer, Number>` matches a function that accepts `Number`
and returns `Integer`, exactly as `fn [[Integer] [Number]]` does
today.

#### Generic predicate types

```
type NonEmpty<T> fn [x:T T [(x size gt 0) guard x]]
```

#### Generic typed defs

```
def stringBox:Box<String> {value:"hi"}
def pairs:[:Pair<String, Integer>] [{key:"x" value:1} {key:"y" value:2}]
```

#### Generic fn definitions

```
fn identity<T> [[T] [T] [/* body — argument is on the stack */]]
fn pair<K, V> [[K V] [Pair<K, V>] [{key:_  value:_}]]
fn map<T, U> [[fn:Mapper<T, U> [:T]] [:U] [/* body */]]
```

The type parameter list slots between the name and the
`[[inputs] [outputs] [body]]` triple — the same position TS uses
between the function name and the parameter list.

### 4.4 Application sites

```
Box<Integer>                   # type literal
Pair<String, List<Integer>>    # nested
Tree<Tree<Integer>>            # recursive nesting
```

A generic application is a **type expression** — it lives in
data-context (inside maps, type bodies, fn-shape inputs/outputs,
typed-def annotations) just like any other type literal. In word
context the parser converts it to a type-literal `Value` whose
`VType` is the instantiated type and whose `Data` is `nil` (matching
the existing `IsTypeLiteral` predicate).

### 4.5 Constraints (`extends`)

```
type SortedList<T extends Comparable> record [items:[:T]]
type Pair<K extends Hashable, V> record [key:K  value:V]
```

`extends` is parsed inside the `<…>` declaration form only — it is
**not** a top-level word. Inside an `extends` clause the right-hand
side is any type expression, including the algebra:

```
<T extends Number tand Comparable>      # intersection
<T extends String tor Number>           # union
<T extends Container<T>>                # F-bounded — references self
<T extends Any>                         # explicit unconstrained (default)
<T extends Never>                       # uninhabited; allowed but useless
```

**Semantics.** `T extends C` is enforced at instantiation: when a use
site supplies `Foo<X>`, the engine checks `X` is a subtype of `C`
under the existing `Unify` / `tand` machinery. Failure produces a
new error code `[aql/constraint_violation]` with the parameter
name, the supplied type, and the constraint type in the detail.

**No new operator.** We do not add `&` or `|` to the surface — TS
users will read `tand` / `tor` and concatenative users keep their
operator vocabulary. The `extends` keyword is the only addition.

### 4.6 Defaults

```
type Box<T = Integer> record [value:T]
type Pair<K = String, V = Any> record [key:K  value:V]
```

`=` is currently unused at the surface (assignment is `def`, map keys
use `:`). It is safe to introduce inside `<…>` as the default
operator. Defaults follow the standard rule: parameters with defaults
must come after parameters without defaults; partial application
fills missing parameters from defaults.

```
Box                # = Box<Integer>
Pair               # = Pair<String, Any>
Pair<Integer>      # = Pair<Integer, Any>
```

Bare `Box` (no angle brackets) is interpreted as the fully-defaulted
instantiation when every parameter has a default; otherwise it is a
**type schema** value (see §6).

### 4.7 Whitespace

`Box<Integer>`, `Box< Integer >`, and `Box <Integer>` all parse the
same. The opening `<` may appear on the same line as the type name
(no whitespace required to separate them, because `<` is a fixed
token) or after whitespace. Multi-line declarations are allowed:

```
type Pair<
  K extends Comparable,
  V = Any
> record [key:K  value:V]
```

### 4.8 Disambiguation

Because `<` and `>` are now fixed tokens, the only ambiguity is
between **generic application** and **two unrelated tokens that
happen to sit next to a `<`**. In practice this means we forbid `<`
appearing immediately after a non-type-producing word in type
context. The parser's two-pass approach (collect tokens, classify in
the appropriate `convert*` function) makes this enforceable: a `<`
in word context that is not preceded by an identifier or a closing
`>` is a `syntax_error`.

In data context (record bodies, fn input/output lists), the parser
already classifies bare capital words as type names; `<…>` after such
a word is the generic-application alternate.

## 5. Semantics

### 5.1 Type schemas vs. instantiated types

A generic declaration installs a **type schema** in the type stack —
a value of internal kind `TypeSchema` that holds:

- the parameter list (names, constraints, defaults)
- the body (record fields, fn shape, predicate body) with parameter
  references left as `TypeParam(name)` placeholders

A generic application instantiates the schema by substituting each
`TypeParam(name)` with the supplied argument, producing a normal
type-literal value (`RecordType`, `FnShape`, `PredicateType`, …)
that downstream code consumes without needing to know it came from
a generic.

### 5.2 Substitution

Substitution is structural: walk the body, replace each `TypeParam`
node, and run the existing normalisation (e.g. `tand` distribution
over `tor`) on the result. This is the only new piece of engine
logic that generics introduce.

### 5.3 Constraint checking

At instantiation time, for each parameter `T extends C`, run
`isSubtype(arg, C)` — the same predicate used by `is`. Failure
produces `[aql/constraint_violation]` with a hint pointing at the
parameter declaration site (using the existing `Pos` threading via
`WithPos`).

### 5.4 Variance

Generic fn-shape types reuse the existing fn-shape variance rules:
contravariant in input parameter positions, covariant in return
positions. We do **not** add per-parameter variance markers
(`<in T>`, `<out T>`) in the first cut — the inferred variance is
sufficient for the use cases we have. If future use cases need
explicit markers, the syntax slot is reserved.

### 5.5 Inference

Two inference sites are in scope:

1. **Value-to-type inference at typed-def sites.** `def x:Box {value:42}`
   should infer `Box<Integer>` rather than requiring `Box<Integer>`
   explicitly. This is a unification problem: match the value against
   the generic body, collect parameter bindings.

2. **Function-call inference.** `[1 2 3] map (quote double)` should
   infer `T=Integer`, `U=Integer` for `map<T, U>` from the list and
   the `Mapper<T, U>` argument shape. The carrier-based static
   checker already tracks types through dispatch; inference extends
   this with a substitution-collecting step before subtype checking.

Both forms degrade gracefully to "no inference, error, ask user to
annotate" — they are an optimisation over explicit annotation, not a
requirement.

### 5.6 Interaction with the existing algebra

- `Box<Integer> tand Box<Number>` reduces to `Box<Integer>` (the
  intersection of the parameters wins, because the schema is
  invariant in `T` by default — record fields are read-write).
- `Box<Integer> tor Box<String>` stays as a disjunct; it does not
  collapse to `Box<Integer tor String>` because the two types are
  observationally distinct (a value's `value` field is one or the
  other, not the union).
- `Box<Never>` is **inhabited** at the type level (it is a record
  type with a `Never`-typed field), but no concrete value satisfies
  it because no value satisfies `Never`. The engine reports this as
  a `static_warning` at instantiation — useful for catching dead code
  but not a hard error.

### 5.7 Recursion

`type Tree<T> record [value:T  left:Tree<T>  right:Tree<T>]` is
permitted. The schema body refers to its own name; substitution is
performed lazily at field access (or eagerly with cycle detection,
TBD — see §8).

### 5.8 F-bounds

`<T extends Container<T>>` is permitted. The constraint is not
checked until instantiation, so the self-reference does not need a
fixpoint at declaration time. At instantiation, the engine
substitutes the supplied `T` into both occurrences and then runs
the subtype check.

## 6. Worked examples

### 6.1 Generic container

```
type Box<T> record [value:T]

# Application produces a concrete type literal:
def intBox:Box<Integer> {value:42}
def strBox:Box<String>  {value:"hi"}

intBox is Box<Integer>      # → true
intBox is Box<String>       # → false
intBox is Box<Number>       # → true   (Integer extends Number)
```

### 6.2 Generic fn shape

```
type Mapper<T, U> fn [[T] [U]]

fn double [[Integer] [Integer] [2 mul]]
(quote double) is Mapper<Integer, Integer>      # → true
(quote double) is Mapper<Number, Integer>       # → false (Integer ≠ Number for input)
(quote double) is Mapper<Integer, Number>       # → true  (Integer is a subtype of Number for return)
```

### 6.3 Generic fn definition with inference

```
fn map<T, U> [[fn:Mapper<T, U>  [:T]] [:U] [
  # body iterates the list and calls fn on each element
]]

[1 2 3] map (quote double)
# Inference: the list is [:Integer], so T=Integer.
# (quote double) matches Mapper<T=Integer, U=?>; double's return is Integer, so U=Integer.
# Result type: [:Integer]
```

### 6.4 Constrained parameter

```
type Comparable tor [Integer Decimal String]
type SortedList<T extends Comparable> record [items:[:T]]

def names:SortedList<String> {items:["amy" "bob"]}    # OK
def boxes:SortedList<Box<Integer>>                    # constraint_violation:
                                                       #   Box<Integer> does not extend Comparable
```

### 6.5 Default

```
type Result<T, E = Error> record [ok:T  err:E]
def r:Result<Integer> {ok:1  err:none}        # E defaults to Error
```

## 7. Implementation plan

### Phase 0 — design lock-down

This document, plus a short follow-up RFC review with the team. Pin
the syntax (especially the `extends` vs `:` decision and the `=`
default-operator decision).

### Phase 1 — tokenization and grammar

- Add `LA`/`RA` jsonic tokens in `parser/grammar.go`.
- Extend the `"val"` rule with the application alternate.
- Add `"tparams"` rule for declarations.
- Add `convertGenericApp` and `convertTParams` helpers in
  `parser/parse.go`.
- Tests: parse-only round-trip tests for every form in §4.

### Phase 2 — schema values and substitution

- New `Value` kind: `TypeSchema` (held in the type stack, not the
  def stack).
- New placeholder: `TypeParam{name}` — a marker that flows through
  the engine until substitution.
- `instantiateSchema(schema, args)` performs constraint-checking and
  substitution, returning a normal type literal.
- `RegisterType` recognises the `<…>` declaration form and installs
  a `TypeSchema` instead of a concrete type.
- Tests: instantiation, substitution, normalisation interplay with
  `tand`/`tor`.

### Phase 3 — typed-def, `is`, and pattern dispatch

- Typed-def sites accept `Foo<…>` annotations.
- `is` accepts a generic application on the right.
- Signature matching learns `TypeParam` is "matches anything, binds
  to whatever it sees" — this is the inference path for fn-defs.
- Carrier values for generic record types preserve parameter
  bindings so `aql check` can report precise types.

### Phase 4 — defaults and constraints

- Default substitution at the parser level (or at instantiation,
  TBD).
- `extends` clause checked against `Unify` — emit
  `[aql/constraint_violation]` on failure.
- Tests: every constraint shape from §4.5.

### Phase 5 — generic fn definitions

- Extend `fn` registration to accept a parameter list.
- Per-call inference using carrier types.
- Tests: `map`, `fold`, `pair`, `identity`, plus error cases where
  inference fails (unannotated uses).

### Phase 6 — docs

- LANGREF.md: new "Generic Types" section after "Predicate Types"
  and before "Type and Def Naming".
- SIGNATURES.md: add `extends` and the angle-bracket syntax to the
  type-expression grammar.
- TYPES.md: cover schemas, substitution, constraint checking.
- A new `GENERICS.md` user-facing how-to in `aql/doc/`.

## 8. Open questions

1. **Eager vs. lazy substitution for recursive types.** Eager is
   simpler but loops on `type Tree<T> record […  left:Tree<T> …]`.
   Lazy avoids the loop but complicates equality. Probable answer:
   memoise on `(schema, args)` pairs — check identity before
   substituting deeper.

2. **`extends` vs `:` shorthand.** TS uses `extends`. AQL's existing
   `def x:Type` and record field `name:Type` both use `:`. Should
   `<T:Comparable>` be a sugar for `<T extends Comparable>`? My
   recommendation: ship only `extends` first, add `:` later if
   users find it natural. Cost of adding it later is low; cost of
   removing it is high.

3. **Variance markers.** Defer. The fn-shape variance rules cover
   the cases we have. If we add explicit markers later, `<in T>` /
   `<out T>` (TS 4.7+) is the obvious choice and slots into the
   existing tparam grammar.

4. **Generic word resolution order.** The type stack already
   resolves before defs and natives. Schemas live in the type stack.
   But an instantiated `Box<Integer>` must resolve `Box` first
   (find the schema), then read `<Integer>` as the application.
   This is straightforward but worth a test for the case where
   `Box` is also shadowed by a non-generic type.

5. **Error messages for failed inference.** When inference can't
   solve, we want the error to point at the call site and list
   the parameters that could not be bound, not just say "no
   matching signature". This needs new error infrastructure
   parallel to `signatureError`.

6. **Module exports.** A module that defines `type Box<T>` exports
   the schema, not an instantiation. Users of the module write
   `module:Box<Integer>` at the call site. This Just Works under
   the design above, but worth a test.

7. **Generic predicate types — what does "static" mean?** A
   predicate body that branches on `T` is genuinely generic, but
   constraint checking the body across all possible `T` is
   undecidable in general. We should document that predicate
   bodies are typed at instantiation time, not at declaration.

## 9. Risk register

- **Parser ambiguity creep.** Reserving `<`/`>` exclusively for
  generics is a long-term commitment. We should not later add `<`
  as a comparison operator. Document this in `LANGREF.md`'s syntax
  section as a hard rule.
- **Carrier-checker complexity.** Substitution must thread through
  the carrier path or `aql check` regresses. Plan to write
  carrier-specific tests in Phase 3 alongside the dispatch work.
- **Performance.** Repeated instantiations with the same args (e.g.
  `Box<Integer>` mentioned 50 times in a program) should hit a
  cache. The implementation should memoise on `(schema, normalised
  args)` keys from the start.
- **Documentation drift.** Five doc files mention the type system.
  Phase 6 must touch all of them in one PR or readers will see
  inconsistent stories.

## 10. A concatenative-friendly alternative (for the record)

The user asked for angle brackets, and the analysis above commits to
that direction. For completeness, a fully word-based alternative
would look like:

```
type Box generic [T] record [value:T]
type Pair generic [K V] record [key:K  value:V]
Box of [Integer]
Pair of [String Integer]
```

It is more in keeping with the rest of AQL's surface (no new
brackets, no new infix-style keyword), and reuses the existing list
syntax for parameter lists. The cost is that newcomers from
TypeScript / Java / C# / Rust would not recognise it, and the
declaration site is more verbose. We do not recommend this path,
but it is a reasonable fallback if `<…>` proves to clash with
something we have not anticipated.

## 11. Decision summary

- **Syntax:** angle brackets, TS-style. `Box<T>`, `<T extends C>`,
  `<T = D>`.
- **Tokenization:** `<` and `>` become reserved fixed tokens.
- **Constraints:** `extends` keyword inside `<…>`; right-hand side is
  any type expression including `tand`/`tor`.
- **Defaults:** `=` inside `<…>`.
- **Variance:** inferred from fn-shape rules; no explicit markers in
  v1.
- **Inference:** at typed-def and fn-call sites, via the existing
  carrier/unify machinery.
- **Algebra:** generic instantiations participate in `tand`/`tor` as
  ordinary types (invariant per parameter; no auto-distribution
  through type constructors).
- **Phased rollout:** six phases, each with its own test surface.
