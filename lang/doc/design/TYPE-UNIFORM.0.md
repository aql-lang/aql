# Uniform Type Construction — `def` / `make` / `type`

Status: design draft, no implementation.
Branch: `claude/review-architecture-go-practices-SoLNa` (parked here;
move to a dedicated branch when implementation starts).

This document captures a converged design for collapsing AQL's
type-definition vocabulary — `type`, `object`, `record`, `table`,
`untype` — onto **three words**: `def`, `make`, `type`. It records the
reasoning, the surface design, the migration path, and the open
questions. It does **not** change behaviour by itself.

## 1. Motivation

AQL today has more type-construction surface than the underlying model
needs. To define and instantiate types a user reaches for:

- `type Name body` — name a type (binds onto the type stack;
  `eng/go/core_type.go::installType`).
- `object {fields}` / `object {fields} Parent` — build an object type
  (`lang/go/native/native_object_record.go::objectHandler`).
- `record [a:T b:U]` — build a record type (`recordHandler`).
- `table R` — build a table type (`native_type.go`).
- `make Type data` — instantiate (`native_make.go::makeNatives`).
- `untype Name` — pop a type binding.

Several observations, established by working through the model:

1. **`type`-as-a-binder is redundant with `def`.** Both push a
   name→value entry onto a shadowing stack. AQL types are first-class
   values (a type literal is a `Value` with `Data == nil`), so
   `def Foo Integer` already binds a type. The only thing `type` does
   that `def` cannot is *mint a named lattice node* for objects and
   predicate types — and that work can move into the binder rather
   than justify a separate word.

2. **The lowercase constructor words duplicate their own types.**
   `object` builds an `Object`; `record` builds a `Record`; `table`
   builds a `Table`. The type already exists as a first-class value;
   the lowercase word is a second name for "construct one of these."

3. **`untype` duplicates `undef`.** Two stacks (`r.defStacks`,
   `r.types`) with — per `lang/go/CLAUDE.md` — identical shadow/pop
   semantics, kept apart by a naming convention (Capitalised = type,
   lowercase = value) that already prevents collisions. `ResolveTypedName`
   already cross-consults both.

The redundancy is not free: it is five words and two namespaces for
what is, structurally, two operations (construct a type, construct a
value) and one operation (bind a name).

## 2. The model — three words

```
def  name      value     bind a name to a value          (types are values)
make BaseType  data      construct a VALUE of a type
type BaseType  arg       construct a TYPE  from a type
```

- **`def`** is the universal binder. It names values; since types are
  values, it names types. It absorbs `type`-as-binder and `untype`
  (→ `undef`).
- **`make`** is the value constructor — unchanged from today.
- **`type`** is the type constructor. This is its **only** role: it is
  *not* a binder, and it is *not* the bag of `object`/`record`/`table`
  words. It is the single, uniform type-construction operator — the
  exact sibling of `make`, one level up.

### 2.1 The two-facet convention

Every type optionally carries up to two facets:

- a **former**: `type T arg` → a subtype/refinement of `T`
- a **constructor**: `make T arg` → a value of type `T`

`make` is the **sole seam** between the type level and the value
level. A type may have either facet, both, or neither.

### 2.2 Canonical surface

```
def Account  (type Object {balance:Number})
def Savings  (type Account {rate:Decimal})     # inheritance = apply the parent
def People   (type Table (type Record {name:String age:Number}))
def acct     (make Account {balance:0})
```

Bare type names — `Account`, `Savings`, `Object` — are simply
type-values. They are unambiguous *because construction is always
headed by the word `type` or `make`* (see §3).

### 2.3 Before / after

| Today | Proposed |
| --- | --- |
| `type Foo object {x:String}` | `def Foo (type Object {x:String})` |
| `type Bar object {y:Integer} Foo` | `def Bar (type Foo {y:Integer})` |
| `type R record [a:Integer b:String]` | `def R (type Record {a:Integer b:String})` |
| `type People table R` | `def People (type Table R)` |
| `make Foo {x:1}` | `make Foo {x:1}` *(unchanged)* |
| `untype Foo` | `undef Foo` |

Two incidental wins fall out:

- **Inheritance becomes "apply the parent."** There is no privileged
  root constructor and no trailing parent argument: `type Foo {…}`
  refines `Foo`, for any object type `Foo`. `Object` is just the root.
- **Record bodies become maps, not lists.** `type Record {a:Integer
  b:String}` replaces `record [a:Integer b:String]`. AQL maps are
  ordered (`OrderedMap`), so field order — the reason the list form
  existed — is preserved.

## 3. Why `type` is irreducible — the ambiguity argument

It is tempting to drop `type` entirely and let `def Foo Object
{x:Integer}` mean "apply `Object` to `{x:Integer}`." **This does not
work**, for a structural reason.

AQL is space-separated and concatenative: **juxtaposition already
means "a sequence of values"** — list elements, stack values.
`[1 2 3]` is three values; `[Object Integer String]` *must* therefore
be three type-values. So `Object Integer` cannot *also* silently mean
"apply `Object` to `Integer`" — that reading collides head-on with the
sequence reading. There is no whitespace left to carry "application."

It bites concretely:

- `[Object Integer]` — a list of two type-values — versus `Object`
  consuming `Integer`. Unresolvable by juxtaposition.
- `def Foo Object {x:Integer}` — `def` has fixed arity 2. It collects
  `Foo` and `Object`, **stops**, and `{x:Integer}` dangles.

The value level already faced and solved exactly this problem.
`make Foo {x:1}` is unambiguous because `make` is a **dedicated word**
— unambiguously a word, never a value, with known arity. By exact
symmetry, type construction needs its own dedicated marker word. That
word is `type`.

So `type` survives — **reconceived**: not a binder (that role *is*
redundant with `def`), but the type-level constructor operator, the
sibling of `make`. The words `type` and `make` forward-collect like
any other word; the bare type-name *values* do not. No token is
overloaded with two arities.

```
[Object Integer String]          # three bare type-values — no `type` head
(type Object {x:Integer})        # construction — headed by `type`
(make Account {balance:0})       # value construction — headed by `make`
def n:Account …                  # `Account` bare = a type-value
x is Savings                     # `Savings` bare
```

The parenthesised constructor matches AQL's existing idiom — the spec
suite already writes `def opts (make Options {x:Integer})`.

## 4. What is removed, what stays

**Removed words:** `object`, `record`, `table`, `untype`, and
`type`-as-a-binder.

**`type` stays**, as the single type constructor.
**`make` stays**, unchanged.
**`def` stays**, and absorbs the binder role of `type` plus, for
Capitalised targets whose value is a mintable type (object/predicate),
the lattice-minting that `installType` does today.

**Recommended but separable:** collapse the two binding stacks
(`r.defStacks`, `r.types`) into one. The Capitalisation convention
already prevents name collisions and `ResolveTypedName` already
cross-consults; one stack makes resolution trivial and `undef` cover
both. This can land independently of the word changes.

## 5. Per-type behaviour

The convention is uniform as a *frame*; what the argument means is
per-kind ("modulo builtin behaviours").

| Type | former `type T arg` | constructor `make T arg` |
| --- | --- | --- |
| `Object` | `type Object {x:Integer}` — apply parent to inherit | `make Foo {x:1}` |
| `Record` | `type Record {x:Integer}` | `make R {x:1}` |
| `Table` | `type Table R` (arg is a record *type*) | `make T [[…] […]]` |
| `Resource`/`Entity` | `type Entity {…}` | `make Entity {…}` |
| `Array` | `type Array Integer` | `make Array [1 2 3]` |
| `Options` | `type Options {x:Integer}` | — *(none today — see §7.2)* |
| `List` | `type List Integer` (≡ `[:Integer]`) | literal `[1 2 3]` |
| `Map` | `type Map String` (≡ `{:String}`) | literal `{a:1}` |
| `Integer`/`String`/… | `type Integer <bound>` (refinement) | literal `42`; `make` = conversion |
| `Store`, meta/marker types | — | — (not user-constructed) |

The convention holds cleanly for the whole **Object branch** —
`Object`, `Record`, `Table`, `Resource`/`Entity`, `Array`, and every
user type. `Resource`/`Entity` is already convention-shaped today: it
has no constructor word and is used purely via `make`.

## 6. The parser-primitive exception

`List`, `Map`, and the scalars (`Integer`, `String`, `Boolean`,
`Decimal`, `Atom`, `Path`, `None`) are **parser-primitive**: their
instances *must* be expressible as literals — `[1 2 3]`, `{a:1}`, `42`,
`"x"`, `true`. A language that requires `make List [make Integer 1 …]`
to obtain `[1]` is unusable. So for these types the **literal is the
constructor**, and `make` is either an error (`make List 5` already
errors today: *"List is a Node-family type; not a make target"*) or a
**conversion** (`make Integer "42"` parses a string).

This is not a flaw in the convention — it coincides exactly with the
boundary `eng/go/CLAUDE.md` already draws in "Where a Type Lives":

- **Category 1** — *"the parser emits it directly: Integer, Decimal,
  String, Boolean, Atom, Path, None, List, Map."* → literal instances.
- **Category 4** — *"structural type used by make/record/object:
  Record, Options, Table, ChildType, ObjectType, ObjectInstance,
  Store, Array, Error."* → `make` instances.

The `type` *former* still applies to the primitives — `type List
Integer`, `type Integer <bound>` — it is only the *constructor* facet
that the primitives satisfy via literals instead of `make`.

## 7. Open questions

### 7.1 Applying a record

`RecordTypeInfo` carries only `Fields` — no `Name`, no `Parent`, no
identity (unlike `ObjectTypeInfo`). Record unification is closed and
order-strict. So `type SomeRecord {extra:T}` — refining a record — has
no inheritance semantics to fall back on. **Decision needed:** either
(a) disallow applying a record type, or (b) define it as "a fresh
structural record with the merged field set" (not a nominal subtype).
Option (b) is more uniform; option (a) is more honest about records
being structural.

### 7.2 Regularising `Options`

`Options` is the lone misfit among the structural types: today
`make Options {x:Integer}` produces the options *type*, and conforming
plain maps are the only "instances." Under the uniform convention
`make` of an options *type* should produce a real instance — a map
with the options' defaults applied and validated. Adopting the
convention is the opportunity to fix this asymmetry (the same
asymmetry noted for records in `TYPE-SYSTEM-REVIEW`).

### 7.3 Combined `type Name Base arg` sugar

The canonical form is `def Foo (type Object {…})` — `type` is a pure
constructor, `def` binds. A combined `type Foo Object {…}` form
(construct + bind in one word, ≈ what AQL has today) is terser.
**Decision needed:** keep `type` single-role (pure constructor) and
require `def (…)`, or admit the combined form as arity-overloaded
sugar that desugars to `def Foo (type …)`. This document leans toward
single-role for conceptual cleanliness; the combined form can be added
later as pure sugar if the verbosity proves annoying.

### 7.4 How far does `make` go

`make` could be pushed further — `Foo {x:1}` (value-shaped arg) could
construct an instance while `type Foo {x:Integer}` constructs a type.
This is **deliberately rejected**: a mixed map `{x:Integer y:1}` is
ambiguous, and the explicit `make` head is worth keeping as the one
visible type→value seam. `make` stays mandatory for instances.

## 8. Migration plan

The change is additive-first so the test suite stays green throughout.

- **Phase 0** — this document.
- **Phase 1 (additive).** Introduce `type` as a constructor word that
  works *alongside* `object`/`record`/`table` and the current
  `type`-as-binder. Both syntaxes valid. New `.tsv` spec rows and Go
  tests exercise the new form. Nothing breaks.
- **Phase 2 (migrate).** Rewrite `lang/spec/*.tsv`, `eng/spec/*.tsv`,
  and the Go test suites to the new syntax. The spec files *are* the
  language specification — this phase is the bulk of the work and
  must be a single reviewed pass.
- **Phase 3 (remove).** Delete `object`/`record`/`table`/`untype`;
  fold `type`-as-binder into `def`; move the object/predicate
  lattice-minting into `def`.
- **Phase 4 (optional).** Collapse the two binding stacks into one.

Each phase is independently shippable and leaves `make test` green.

## 9. Non-goals and honest limits

- **This is surface unification.** The *kinds* — `Object`, `Record`,
  `Table`, `Array`, … — remain hard-coded Go: their payload structs
  (`eng/go/value.go`), constructors, unification, and behaviour are
  unchanged, merely reached via "`type` applied" instead of via a
  registered lowercase word. A genuinely new *kind* still requires
  Go. This proposal does not deliver user-definable kinds.
- **Not generics, not HKT.** Parametric polymorphism is a separate
  proposal (`GENERICS.0.md`). However, this design is *consistent
  with* and *friendly to* that direction: treating `type` as a
  type-level constructor word is the same "type constructors are
  ordinary applicable things" stance that higher-kinded types need.
  A future reconciliation should align this `type` operator with
  `GENERICS.0.md`'s `apply`.
- **`make` stays the sole type→value seam.** No implicit coercion.

## 10. Decision summary

- **Three words.** `def` binds; `make` constructs values; `type`
  constructs types. `object`, `record`, `table`, `untype`, and
  `type`-as-binder are removed.
- **`type` is a constructor, not a binder.** It is the sibling of
  `make`, one level up. Both are ordinary forward-collecting words of
  fixed arity; bare type names remain plain values.
- **Construction is always headed.** `type`/`make` mark construction;
  unmarked juxtaposition stays value-sequencing. This resolves the
  ambiguity that makes a keyword necessary — `type` is irreducible for
  the same reason `make` is.
- **Two-facet convention.** Every type optionally has a former
  (`type T arg`) and a constructor (`make T arg`). Uniform across the
  Object branch.
- **Parser-primitive exception.** `List`/`Map`/scalars take literal
  instances, not `make` — coinciding with the existing kernel
  category-1/category-4 boundary.
- **Open items before implementation:** applying-a-record semantics
  (§7.1), regularising `Options` (§7.2), the combined-sugar question
  (§7.3).
- **Rollout:** additive first (Phase 1), then migrate the spec/test
  suites (Phase 2), then remove the old words (Phase 3).
