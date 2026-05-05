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

## 2. Design philosophy: concatenative core, angle-bracket sugar

A type-parameter list is — structurally — an ordered list with one
entry per parameter, where each entry carries a name plus optional
constraint and default. AQL already has lists; AQL already has words
that take quoted lists and do interesting things with them (`def`,
`fn`, `record`, `for`, …). Generics fit the same mould.

**The canonical surface** is fully concatenative: four new engine
words (`gen`, `extends`, `default`, `apply`) extend the type and fn
machinery with parametric polymorphism. **The angle-bracket form**
(`Box<T>`, `<T extends C>`, `Box<Integer>`) is a documented
parser-level sugar that desugars to the canonical form before any
engine code runs.

This split has three concrete benefits over an angle-bracket-native
design:

1. **One core machinery.** Generics are an extension of the existing
   typed-def / record / fn pipeline. The static checker, error
   reporting, and source-position threading work without bespoke
   code paths.
2. **Programmatic generics.** `def myParams [T (U extends Comparable)];
   type Box gen myParams record [...]` — parameter lists can be
   constructed at runtime or assembled by macros. This is impossible
   with a pure-syntax angle-bracket form.
3. **Smaller token surface.** `<` and `>` only need to exist in the
   sugar layer (lexer rewrite). The grammar, AST, and engine never
   see them.

## 3. Goals and non-goals

**Goals.**

1. Add **type parameters** to records, fn-shape types, predicate types,
   typed-def, and fn definitions.
2. Express the feature in a **concatenative core** (`gen`, `extends`,
   `default`, `apply`) so it composes with the rest of the language.
3. Provide a **TypeScript-style angle-bracket sugar** so users
   familiar with mainstream generics syntax can read and write the
   feature without re-learning.
4. Support TypeScript-style **constraints** (`extends`) with semantics
   that integrate naturally with the existing `tand`/`tor`/`Never`/
   `Any` algebra rather than reinventing them.
5. Support **defaults** (`<T = Integer>` / `(T default Integer)`).
6. Be **inferable** wherever the existing signature-matcher already
   has enough information — e.g. `Box<Integer>` should be inferable
   from a value of `{value: 42}` without an explicit annotation.
7. Preserve `aql check` (carrier-based static checking) coverage —
   generics must produce carriers that the checker can refine.

**Non-goals (deferred).**

- Higher-kinded types (parameters that are themselves generic).
- Conditional types (`T extends U ? X : Y`).
- Mapped types (`{[K in keyof T]: …}`).
- Variance annotations richer than the inferred contravariant-input /
  covariant-return rule the fn-shape matcher already implements.
- Generic modules. (Modules can re-export concrete instantiations.)

## 4. Survey of the existing syntactic landscape

What the parser and engine already use, that bears on the design:

- **`<` and `>` are syntactically free.** Comparisons use `lt`, `gt`,
  `lte`, `gte` (`internal/engine/compare.go`). No existing word, sigil,
  or jsonic token consumes `<` or `>`. They are available for the
  sugar layer.
- **Type names start with a capital letter, def names lower-case**
  (`LANGREF.md` §"Type and Def Naming"). The same rule applies to
  type parameters — `gen [T U V]` accepts capitals; `gen [t]` is
  rejected at registration time.
- **Typed-def uses `:`** — `def x:Integer 42`. Reserved as-is. We do
  not introduce a colon-as-extends shorthand in v1.
- **Type algebra uses `tand` / `tor`** — `Integer tor String`. The
  `extends` constraint takes any type expression, so the algebra
  composes for free: `(T extends Number tand Comparable)`.
- **Fn-shape types already encode variance** (contravariant inputs,
  covariant returns; `LANGREF.md` §"Structural Function-Shape Types").
  Generic fn shapes inherit this for free.
- **`Any` and `Never` are the lattice top and bottom.** Unconstrained
  type parameters default to `extends Any`; `Never`-bounded
  parameters are valid but uninhabited.
- **`NoEvalArgs` already exists** for words that take a list as a
  code body (`def`, `fn`, `if`, `for` branches, `each`, etc.) — the
  list arrives quoted instead of being auto-evaluated. `gen` uses
  the same mechanism.

## 5. The canonical concatenative core

Four new engine words. All four are forward-precedence; `gen` and
`apply` use `NoEvalArgs` on their list argument so the parser does
not auto-evaluate it.

### 5.1 `gen` — declare type parameters

```
gen [T  (U extends Comparable)  (V default Integer)  (W extends Comparable default String)]
```

Signature: `gen [List/q] -> [GenSpec]`. Walks the list, collecting one
parameter spec per entry:

- **Bare atom** (e.g. `T`): unconstrained parameter (`extends Any`,
  no default).
- **Paren-expression** (e.g. `(U extends Comparable)`): evaluated
  with `U` bound as a fresh `TypeParam` placeholder in scope, so
  later parameters can refer to earlier ones (`gen [T (U default T)]`)
  and constraints can be F-bounded (`gen [(T extends Container apply [T])]`).

`gen` itself does not install a type. It produces a `GenSpec` value
that the next type-introducing word (`type`, `fn`, `def`) consumes
to build a generic schema.

### 5.2 `extends` — attach a constraint

```
T extends Comparable
```

Signature: `extends [Atom/q TypeExpr] -> [GenParam]`. Forward-collects
the right-hand type expression. Errors with
`[aql/extends_outside_gen]` if invoked outside a `gen` parameter
list.

### 5.3 `default` — attach a default

```
T default Integer
T extends Comparable default String
```

Signature: `default [Atom/q TypeExpr] -> [GenParam]` and
`default [GenParam TypeExpr] -> [GenParam]` (chains after `extends`).
Same context restriction as `extends`.

### 5.4 `apply` — instantiate a schema

```
Box apply [Integer]
Pair apply [String  Integer]
Tree apply [Tree apply [Integer]]
```

Signature: `apply [Schema List] -> [TypeLiteral]`. Looks up the
schema, validates arity and constraints, substitutes each parameter,
and returns a normal type-literal value (`RecordType`, `FnShape`,
`PredicateType`, …) that the rest of the engine consumes without
needing to know it came from a generic.

### 5.5 Worked declarations in the canonical form

```
type Box gen [T] record [value:T]
type Pair gen [K V] record [key:K  value:V]
type Tree gen [T] record [value:T  left:Tree apply [T]  right:Tree apply [T]]
type Mapper gen [T U] fn [[T] [U]]
type Reducer gen [T A] fn [[A T] [A]]
type Predicate gen [T] fn [[T] [Boolean]]
type SortedList gen [(T extends Comparable)] record [items:[:T]]
type Result gen [T (E default Error)] record [ok:T  err:E]

fn identity gen [T] [[T] [T] [/* body */]]
fn pair gen [K V] [[K V] [Pair apply [K V]] [{key:_  value:_}]]
fn map gen [T U] [[fn:Mapper apply [T U]  [:T]] [:U] [/* body */]]
```

### 5.6 Worked applications

```
def intBox:(Box apply [Integer]) {value:42}
def pairs:[:Pair apply [String Integer]] [{key:"x" value:1}]
intBox is (Box apply [Integer])         # → true
intBox is (Box apply [Number])          # → true (Integer extends Number)
```

The parens are needed only because `apply` is forward-precedence and
we want it to bind tightly inside an annotation. In word context
(top level) the parens are unnecessary: `Box apply [Integer]` stands
alone.

## 6. Angle-bracket sugar

A lexer-level rewrite layer recognises two forms and emits the
canonical token stream. The grammar, AST, and engine see no `<` or
`>`.

### 6.1 Reserved tokens

`<` (`#LA`) and `>` (`#RA`) are registered as fixed jsonic tokens so
they tokenize even when adjacent to text (`Box<T>` lexes as `Box`,
`<`, `T`, `>` — same trick as `(`, `)`, `.`, `;`).

### 6.2 Two rewrite rules

| Sugar | Canonical |
|---|---|
| `Name<...>` immediately after a type/fn head (`type`, `fn`, etc.) | `Name gen [...]` |
| `Name<...>` elsewhere (use site) | `Name apply [...]` |

The list contents are themselves rewritten:

| Sugar inside `<…>` | Canonical inside `[…]` |
|---|---|
| `T` (bare) | `T` |
| `T extends C` | `(T extends C)` |
| `T = D` | `(T default D)` |
| `T extends C = D` | `(T extends C default D)` |
| `,` separator | whitespace |

### 6.3 Side-by-side

```
# Sugar
type Box<T> record [value:T]
type Pair<K extends Comparable, V = Any> record [key:K  value:V]
type Tree<T> record [value:T  left:Tree<T>  right:Tree<T>]
type Mapper<T, U> fn [[T] [U]]

def intBox:Box<Integer> {value:42}
intBox is Box<Number>

fn map<T, U> [[fn:Mapper<T, U>  [:T]] [:U] [/* body */]]

# Canonical (what the engine actually sees)
type Box gen [T] record [value:T]
type Pair gen [(K extends Comparable) (V default Any)] record [key:K  value:V]
type Tree gen [T] record [value:T  left:Tree apply [T]  right:Tree apply [T]]
type Mapper gen [T U] fn [[T] [U]]

def intBox:(Box apply [Integer]) {value:42}
intBox is (Box apply [Number])

fn map gen [T U] [[fn:Mapper apply [T U]  [:T]] [:U] [/* body */]]
```

### 6.4 Disambiguation

The sugar layer commits to the rule **`<` is only ever the start of a
generic argument list**. Any `<` not followed by a valid type-param
or type-arg list is a `[aql/syntax_error]`. This is a hard, long-term
commitment: AQL will not later add `<` as a comparison operator
(comparisons stay on `lt`/`gt`/`lte`/`gte`).

Whitespace is irrelevant: `Box<T>`, `Box< T >`, and `Box <T>` all
lex the same.

## 7. Semantics

### 7.1 Schemas vs instantiated types

`gen` followed by `record` / `fn` / predicate body produces a
`TypeSchema` value installed in the type stack. A schema holds:

- the parameter list (names, constraints, defaults)
- the body with parameter references left as `TypeParam(name)`
  placeholders

`apply` substitutes each `TypeParam(name)` with the supplied
argument and runs the existing normalisation (e.g. `tand`
distribution over `tor`). The result is a normal type literal that
downstream code consumes unchanged.

### 7.2 Constraint checking

At each `apply`, for each parameter `T extends C`, run
`isSubtype(arg, C)` — the same predicate used by `is`. Failure
produces `[aql/constraint_violation]` with a hint pointing at the
parameter declaration site (using `WithPos`).

### 7.3 In-scope binding while evaluating constraints

`gen` is **not** a vanilla word. It walks its list with `NoEvalArgs`
on, processes entries left-to-right, and for each entry:

1. Binds the parameter name as a fresh `TypeParam` placeholder in
   the type stack (push).
2. Evaluates the entry's `extends` and `default` expressions with
   that binding visible — this makes both forward references between
   parameters (`gen [T (U default T)]`) and F-bounded constraints
   (`gen [(T extends Container apply [T])]`) work without special
   casing.
3. Records the resulting `GenParam` in the spec.

After the body type is built, the placeholder bindings are popped.
The resulting `TypeSchema` carries the parameter list independently
of the type stack — instantiations re-bind the placeholders fresh
at each `apply`.

### 7.4 Variance

Generic fn-shape types reuse the existing fn-shape variance rules:
contravariant in input parameter positions, covariant in return
positions. No per-parameter variance markers in v1.

### 7.5 Inference

Two inference sites are in scope:

1. **Value-to-type at typed-def sites.** `def x:Box {value:42}` — no
   `apply` written — should infer `Box apply [Integer]` (sugar:
   `Box<Integer>`) by unifying the value against the schema body.
2. **Function-call inference.** `[1 2 3] map (quote double)` should
   infer `T=Integer`, `U=Integer` for `map gen [T U]` from the list
   and the `Mapper apply [T U]` argument shape. The carrier-based
   checker already tracks types through dispatch; inference extends
   this with a substitution-collecting step before subtype checking.

Both forms degrade gracefully — explicit annotation always works.

### 7.6 Interaction with the existing algebra

- `(Box apply [Integer]) tand (Box apply [Number])` reduces to
  `Box apply [Integer]` (per-parameter intersection; record fields
  are read-write so the schema is invariant in `T` by default).
- `(Box apply [Integer]) tor (Box apply [String])` stays as a
  disjunct — does not auto-collapse to `Box apply [Integer tor String]`,
  because the two are observationally distinct.
- `Box apply [Never]` is type-inhabited but value-uninhabited; the
  engine emits a `static_warning` at instantiation.

### 7.7 Recursion and F-bounds

`type Tree gen [T] record [...  left:Tree apply [T] ...]` is
permitted. Substitution memoises on `(schema, normalised args)` to
avoid loops. F-bounds work because of §7.3: the placeholder for `T`
is in scope while the constraint is evaluated.

## 8. Case study: the `aql:decision` module

`internal/nativemod/decision.go` is a DMN-style decision module
(decision tables and decision trees) implemented in pure AQL. It is
a good case study because it has three independent shapes of
`Any`-punt that generics resolve in distinct ways.

### 8.1 The result-type punt

Every record that carries a decision result types it as `Any` (or
`Map`):

```aql
type Rule       record [when:Map  then:Map]
type DTable     record [kind:String  rules:List  hit-policy:String]
type DTree      record [kind:String  root:Atom  nodes:List]
type LeafNode   record [id:Atom  kind:String  result:Any]
def decide fn [[model:Map  input:Map] [Any] [...]]
```

A table that returns `{premium: 1.5}` records and a table that
returns `Integer` codes have the same static type. The carrier
checker cannot refine the result of `decide` past `Any`, so every
caller has to dynamic-check.

Threading a single result parameter `R` through the schema fixes it:

```aql
type Rule<R>     record [when:Pred  then:R]
type DTable<R>   record [kind:String  rules:[:Rule<R>]  hit-policy:HitPolicy]
type LeafNode<R> record [id:Atom  kind:String  result:R]
type DTree<R>    record [kind:String  root:Atom  nodes:[:(BranchNode tor LeafNode<R>)]]

# Combined with the Result<T, E> shape from §5.5:
def decide fn [[model:(DTable<R> tor DTree<R>)  input:Map]
               [Result<R, DecisionError>] [...]]
```

Or, in the canonical form:

```aql
type Rule gen [R] record [when:Pred  then:R]
def decide gen [R] fn [
  [model:((DTable apply [R]) tor (DTree apply [R]))  input:Map]
  [Result apply [R DecisionError]]
  [...]
]
```

This is the highest-leverage change in the module — it propagates
precision into every call site of `decide`.

### 8.2 The comparison-operand punt

`apply-op` is fully untyped:

```aql
def apply-op fn [[rhs:Any  op:String  lhs:Any] [Boolean] [...]]
```

`"hello" lt 5` passes the static check today because both operands
satisfy `Any`. A bounded type parameter rejects it:

```aql
type Comparable Integer tor Decimal tor String

def apply-op<T extends Comparable> fn [
  [rhs:T  op:String  lhs:T] [Boolean] [...]
]
```

The constraint reuses the existing type algebra — no new mechanism.
This is the cheapest cleanup in the module: one signature change,
one new type alias.

### 8.3 The recursive-shape punt

`Pred` flattens three structurally distinct cases into one record
with `children:Any`:

```aql
type Pred record [kind:String  op:String  children:Any]
```

`children` is a list of sub-predicates for `all`/`any` and a single
sub-predicate for `not`. Generics don't directly fix this — the
right shape is a tagged union — but they unblock the cleaner
formulation:

```aql
type AllPred  record [kind:String  op:String  children:[:Pred]]
type AnyPred  record [kind:String  op:String  children:[:Pred]]
type NotPred  record [kind:String  op:String  children:Pred]
type CondPred record [field:Atom    op:String  value:Any]
type Pred AllPred tor AnyPred tor NotPred tor CondPred
```

Builder functions then return the precise variant:

```aql
def all-of fn [[children:[:Pred]] [AllPred] [
  make AllPred {kind:"group" op:"all" children:children}
]]
```

Generics participate here for `Pred` carrying a phantom result
parameter only if the predicate body branches on the same `R` as the
enclosing rule — not the case here, so this part of the module
benefits from the disjunct refactor more than from generics per se.

### 8.4 Where generics don't help

- **`Cond.value:Any`** is genuinely heterogeneous per condition: each
  `Cond` compares a different input field, so the value type varies
  row-by-row. This is a path-dependent / dependent-record problem,
  not a parametric one. Best left as `Any` until AQL grows a
  dependent-record story.
- **The `collect` hit policy returns `[:R]`, not `R`.** Different
  hit policies have different return-type variants, which
  TypeScript expresses with conditional types — explicitly
  out-of-scope (§3 non-goals). Workaround: split `decide` into
  `decide-first<R>`, `decide-collect<R>`, etc., each with its own
  return type. Each is parametric in `R`; the dispatch on
  hit-policy moves from runtime to the type level.
- **Stringly-typed field reads.** Most accesses go through
  `(map get "field")` rather than typed-record dot access. Refining
  types end-to-end requires also tightening those reads to dot
  accessors against the now-precise record types. This is a
  co-requisite refactor, not an extra cost — the current dynamic
  accesses are a symptom of not having generics.

### 8.5 Order of impact

If only one piece landed, **§8.1 (`decide<R>` returning
`Result<R, DecisionError>`)** is the highest-leverage change because
it propagates precision into every caller. **§8.2 (bounded
`apply-op`)** is the cheapest cleanup. **§8.3 (Pred disjunct)** is
nice-to-have and largely about disjuncts rather than generics.

This case study suggests a useful diagnostic for adopting generics
elsewhere in the codebase: look for fields, parameters, or returns
typed `Any` or `Map` that are *the same shape across all call
sites of the surrounding function* — those are the parametric
ones. `Any`s that genuinely vary per call site need a different
tool (disjuncts, dependent records, or just leaving them as `Any`).

## 9. Implementation plan

### Phase 0 — design lock-down

This document, plus a short follow-up RFC review with the team. Pin
the four core word names (`gen`, `extends`, `default`, `apply`) and
the sugar rewrite rules.

### Phase 1 — schemas, substitution, and the four core words

- New `Value` kinds: `TypeSchema`, `GenSpec`, `GenParam`, and the
  `TypeParam{name}` placeholder.
- `RegisterGen`, `RegisterExtends`, `RegisterDefault`, `RegisterApply`
  in `internal/engine/native_type_*.go` files (one per word, matching
  the existing layout).
- `instantiateSchema(schema, args)` performs constraint-checking and
  substitution; memoises on `(schema, normalised args)`.
- `type` and `fn` registrations recognise a `GenSpec` argument and
  install a `TypeSchema` instead of a concrete type.
- Tests: every form in §5.5 and §5.6 in canonical syntax only.

### Phase 2 — typed-def, `is`, and pattern dispatch

- Typed-def sites accept schema instantiations (`Box apply [...]`)
  in annotations.
- `is` accepts an instantiation on the right.
- Signature matching learns `TypeParam` is "matches anything, binds
  to whatever it sees" — the inference path for fn-defs.
- Carrier values for generic record types preserve parameter
  bindings so `aql check` reports precise types.

### Phase 3 — angle-bracket sugar

- Add `LA`/`RA` jsonic tokens in `parser/grammar.go`.
- Lexer-level rewrite producing the canonical token stream:
  - `Name<...>` after `type`/`fn` → `Name gen [...]`.
  - `Name<...>` elsewhere → `(Name apply [...])`.
  - `T extends C` inside `<…>` → `(T extends C)`.
  - `T = D` inside `<…>` → `(T default D)`.
  - `,` inside `<…>` → whitespace.
- Tests: every example in §6.3 produces the same engine behaviour
  as its canonical twin.

### Phase 4 — inference

- Value-to-type inference at typed-def sites (§7.5.1).
- Carrier-based call-site inference (§7.5.2).
- Tests: cases that succeed without annotation; cases that fail
  with helpful error messages.

### Phase 5 — generic fn definitions and higher-order word retrofit

- Extend `fn` registration to accept a `GenSpec`.
- Retrofit `map`, `fold`, `outer`, `inner` to use generic fn-shape
  types so the static checker can refine result types.

### Phase 6 — docs

- LANGREF.md: new "Generic Types" section after "Predicate Types"
  and before "Type and Def Naming". Lead with sugar (the form most
  users will write); cross-reference the canonical form.
- SIGNATURES.md: add `gen`, `extends`, `default`, `apply` with their
  signatures.
- TYPES.md: cover schemas, substitution, constraint checking, and
  the sugar/canonical correspondence.
- A new `GENERICS.md` user-facing how-to in `aql/doc/`.

## 10. Open questions

1. **Default substitution timing.** Eagerly at parse time (simpler,
   no late binding) or lazily at `apply` time (allows defaults to
   reference parameters bound later in the schema). My
   recommendation: lazy, because §7.3's binding mechanism makes it
   nearly free.

2. **Sugar for `extends` outside `gen`.** Should we allow
   `extends` as a standalone word for ad-hoc subtype assertions
   (`x extends Comparable` ↔ `x is Comparable tand Comparable`)?
   No — keep `extends` strictly bound to the `gen` parameter list
   to avoid muddying its meaning.

3. **`apply` arity inference for defaulted schemas.** Bare `Box`
   (no `apply`) where every parameter has a default — does it
   auto-instantiate to `Box apply []`? Probably yes, with a clear
   error when not all parameters have defaults.

4. **Generic word resolution order.** Schemas live in the type
   stack; `apply` resolves the head against the type stack first,
   def stack second. Worth a test for the case where `Box` is also
   shadowed by a non-generic type.

5. **Failed-inference error messages.** When inference can't solve,
   the error should point at the call site and list the parameters
   that could not be bound, not just say "no matching signature".
   Needs new error infrastructure parallel to `signatureError`.

6. **Module exports.** A module that defines `type Box gen [T] …`
   exports the schema, not an instantiation. Users of the module
   write `module:Box apply [Integer]` (or `module:Box<Integer>`)
   at the call site. Should Just Work but worth a test.

7. **Generic predicate types — what does "static" mean?** A
   predicate body that branches on `T` is genuinely generic, but
   constraint-checking the body across all possible `T` is
   undecidable in general. Document that predicate bodies are
   typed at instantiation time, not at declaration.

## 11. Risk register

- **Sugar-canonical drift.** The two surfaces must stay in lockstep.
  Mitigation: every sugar test in Phase 3 is a pair of programs (one
  in each surface) that must produce identical engine output.
- **`<`/`>` reservation is permanent.** Once we ship the sugar, we
  cannot use `<` for comparisons or as an operator anywhere. Document
  this in `LANGREF.md`'s syntax section as a hard rule.
- **Carrier-checker complexity.** Substitution must thread through
  the carrier path or `aql check` regresses. Plan to write
  carrier-specific tests in Phase 2 alongside the dispatch work.
- **Performance.** Repeated instantiations with the same args (e.g.
  `Box apply [Integer]` mentioned 50 times) hit the
  `instantiateSchema` memo. Implement the memo from the start.
- **Documentation drift.** Five doc files mention the type system.
  Phase 6 must touch all of them in one PR.

## 12. Decision summary

- **Canonical form:** four engine words — `gen` (declare params),
  `extends` (constrain), `default` (default value), `apply`
  (instantiate). All ordinary forward-precedence words; `gen` and
  `apply` use `NoEvalArgs` on their list.
- **Sugar:** angle brackets, TS-style. `Box<T>`, `<T extends C>`,
  `<T = D>`, `Box<Integer>`. Pure lexer rewrite to the canonical
  form; nothing downstream sees `<` or `>`.
- **Constraints:** `extends` clause inside the parameter list;
  right-hand side is any type expression including `tand`/`tor`.
- **Defaults:** `default` word in the canonical form; `=` in the
  sugar.
- **Variance:** inferred from fn-shape rules; no explicit markers
  in v1.
- **Inference:** at typed-def and fn-call sites, via the existing
  carrier/unify machinery.
- **Algebra:** generic instantiations participate in `tand`/`tor` as
  ordinary types (invariant per parameter; no auto-distribution
  through type constructors).
- **Phased rollout:** six phases, with the canonical core landing
  before the sugar so the engine is exercised independently of the
  parser changes.
