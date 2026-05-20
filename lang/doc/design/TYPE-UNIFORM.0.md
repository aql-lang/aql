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
| `Options` | `type Options {x:Integer}` | `make O {partial}` — defaults filled, fields validated (§7.2) |
| `List` | `type List Integer` (≡ `[:Integer]`) | literal `[1 2 3]` |
| `Map` | `type Map String` (≡ `{:String}`) | literal `{a:1}` |
| `Integer`/`String`/… | `type Integer <bound>` (refinement) | literal `42`; `make` = conversion |
| `Store`, meta/marker types | — | — (not user-constructed) |

The convention holds cleanly for the whole **Object branch** —
`Object`, `Record`, `Table`, `Resource`/`Entity`, `Array`, and every
user type. `Resource`/`Entity` is already convention-shaped today: it
has no constructor word and is used purely via `make`.

**Applicable bases (§7.1).** Only object types *refine*: `type`
accepts the root `Object`, a user object type, or `Resource`/`Entity`
as a base and produces a subtype. For every other kind only the
*root* type (`Record`, `Table`, `Options`, `List`, `Map`) is a valid
`type` base — a user record / table / typed-list is **not**
applicable.

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

## 7. Resolved decisions

The questions left open by the first draft are decided here.

### 7.1 Applying a type — only objects refine

**Decision: only object types are applicable-to-refine. A non-object
user type (a user record, a user table, a typed list) is not a valid
`type` base.**

The valid first arguments to `type` are:

1. **Any object type** — the root `Object`, a user object type, or
   `Resource`/`Entity`. Applying it yields a *subtype*: `type Foo
   {extra}` produces an object type whose `Parent` is `Foo`, so
   `instance is Foo` holds. This is genuine refinement.
2. **A root structural-kind type** — `Record`, `Table`, `Options`,
   `List`, `Map`. Applying it yields a fresh type of that kind.

A *non-root* type of a non-object kind is rejected: `type SomeRecord
{extra:T}` is an error.

Rationale: `RecordTypeInfo` carries only `Fields` — no `Parent`, no
identity — and record unification is exact (same keys, same order). A
"refined" record could only be an *unrelated* record that happens to
share fields; `instance is SomeRecord` would be false. Allowing
`type SomeRecord {…}` would give the *same surface syntax* the
opposite meaning it has for objects (where it is true refinement with
an is-a relation). That trap is worse than the small non-uniformity
of "only objects refine." Objects are the nominal, inheriting kind;
records are the structural, closed kind — the asymmetry is the
design, not an accident. To get a record with more fields, restate
the field set; if you want extend-with-is-a you wanted an object.

### 7.2 `Options` gains a real constructor

**Decision: regularise `Options`. An options type gets a constructor
facet — `make <optionsType> {partial}` produces a concrete map with
the type's defaults filled for absent fields and every present field
validated against its declared field type.**

Today defaulting happens implicitly, only at a `fn`-parameter
boundary; there is no value-producing "apply these option defaults"
operation. The constructor facet makes it explicit and gives
`Options` the same type→instance shape every other structural type
has. The result is a plain map (Options instances, like Record
instances, are plain maps — no distinct payload).

This is a **behaviour addition**, not just a rename: it adds an
operation `Options` did not have. It is additive — the implicit
`fn`-parameter defaulting is unchanged — but it must be a deliberate,
tested change in the implementation phase. The legacy
`make Options {shape}` (which today *builds the options type*) is
replaced by `type Options {shape}`; `make` of the bare `Options` base
is no longer meaningful — you `make` a *specific* options type.

### 7.3 `type` is single-role — no combined binder form

**Decision: `type` is purely a constructor. There is no combined
`type Name Base arg` form. Binding is always `def`; the canonical and
only form is `def Name (type Base arg)`.**

A combined `type Foo Object {…}` would be terser and closer to
today's `type Foo object {…}`, but it is rejected:

- It re-conflates *binding* and *construction* — the exact conflation
  this design exists to separate (§3, §10).
- It would make `type` arity-polymorphic (`type Base arg` vs
  `type Name Base arg`), reviving the ambiguity class that made a
  dedicated constructor word necessary in the first place.
- `make` has no combined "make-and-bind" form — you write
  `def x (make …)`. `type` follows the same symmetry; `def Name (type
  …)` is exactly as verbose as the already-accepted `def x (make …)`.

If, *after* migration, the verbosity proves painful with evidence, a
combined form may be reconsidered strictly as a **lexer-level
rewrite** (`type Foo Base arg` → `def Foo (type Base arg)`,
desugared before the engine sees it, never a distinct semantics).
Default: not done.

### 7.4 How far does `make` go — settled

`make` is **not** pushed further: `Foo {x:1}` does not construct an
instance. A mixed map `{x:Integer y:1}` would be ambiguous, and the
explicit `make` head is the one visible type→value seam. `make`
stays mandatory for instances.

## 8. Migration plan

The change is additive-first so the test suite stays green throughout.

- **Phase 0** — this document.
- **Phase 1 (additive). — IMPLEMENTED.** Introduce the type
  constructor as a word that works *alongside* `object`/`record`/
  `table` and the current `type`. Both syntaxes valid; nothing breaks.

  Implementation notes (as landed):

  - The word `type` already exists as the binder and **quotes its
    first argument as a name**; the new constructor must *evaluate*
    its first argument (the base type). The two cannot share the word
    during an additive phase. The constructor therefore landed under
    the transitional name **`maketype`** (`lang/go/native/native_type.go`),
    to be renamed to `type` at the Phase-3 cutover.
  - `maketype BaseType arg` constructs: `maketype Object {fields}`,
    `maketype <objtype> {fields}` (inheritance — apply the parent),
    `maketype Record {fields}` (field map, not the list form),
    `maketype Table <recordtype>`. It reuses the existing
    object/record/table construction logic and does not bind — pair
    it with `def`.
  - `def` still rejects capitalised names, so the prototype binds
    type names with lowercase identifiers (`def pt (maketype Record
    {…})`). Folding the capitalised-name binder role into `def` —
    so the canonical `def Account (type Object {…})` works — is
    Phase 2 (see the corrected migration plan in §8).
  - `Options` is not yet a `maketype` base (deferred — §7.2);
    applying a *user* record type is not yet supported (deferred —
    §7.1).
  - Tests: `lang/go/test/maketype_test.go`.
  - Incidental fix found via the prototype: nine `r.AqlError` call
    sites in `native_definition.go` / `native_type.go` /
    `native_behave.go` had a literal `%s` in the error *code* and
    *word* (e.g. `"def %s_error"`) — a defect from the May-2026
    error-helper refactor. Corrected to `"def_error"` etc.
- **Phase 2 — `def` becomes the universal binder. — IMPLEMENTED.**
  `def` with a capitalised name is now a TYPE binding; a lowercase
  name remains a VALUE binding.

  Implementation notes (as landed):

  - `defHandler` (`lang/go/native/native_definition.go`): when the
    name is capitalised, `def` delegates to **`eng.InstallType`** —
    the exact path the `type` word uses. So `def Foo body` ≡
    `type Foo body`: object/predicate lattice-minting, literal /
    singleton type bodies (`def Foo 1`), and all type-installation
    validation are reused verbatim — no logic duplication, no
    divergence risk. The canonical `def Account (maketype Object
    {…})` now works and `typeof` reports `Account`.
  - `defTypedHandler` (`def name:T value`) still rejects capitalised
    names: a typed-def is a *value* binding with a type constraint;
    type-annotating a type binding is contradictory.
  - Tests updated for the relaxed rule: `name_case_test.go` (the two
    `def`-capital-rejected negatives became positives — a capitalised
    `def` is a type binding, and `def`-bound object types mint a
    name); `type_namespace_test.go` (`TypeThenDef` now asserts
    `def Foo` shadows an existing `type Foo`); `maketype_test.go`
    moved to the canonical capitalised-name form.
  - `make test` / `make lint` clean repo-wide.

  This corrects the first draft's Phase-2-before-Phase-3 ordering
  bug — the spec migration could not target the final syntax until
  `def` could bind capitalised type names; now it can.
- **Phase 3 — cutover (one atomic breaking pass).** Remove
  `object`/`record`/`table`/`untype` and `type`-as-binder; rename
  `maketype` → `type`; migrate `lang/spec/*.tsv`, `eng/spec/*.tsv`,
  and the Go test suites to the new syntax — all in the same change.
  These cannot be separated: removing `type`-the-binder breaks every
  spec that still uses it. The suite is red only *within* this single
  reviewed change. §7.1–7.3 (now resolved) fix the target syntax.
- **Phase 4 (optional).** Collapse the two binding stacks into one.

Phases 1–2 are additive and leave `make test` green. Phase 3 is the
single breaking change; it is green at its start and end.

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
- **Resolved decisions (§7):** only **object** types are
  applicable-to-refine — a user record/table/typed-list is not a
  valid `type` base (§7.1); **`Options` gains a real constructor** —
  `make <optionsType> {partial}` produces a defaulted, validated map
  (§7.2); **`type` is single-role** — pure constructor, binding is
  always `def`, no combined `type Name Base arg` form (§7.3).
- **Rollout:** Phase 1 (additive `maketype`) — done. Phase 2 — `def`
  becomes the universal binder (must precede migration). Phase 3 —
  one atomic breaking pass: remove the old words, rename
  `maketype` → `type`, migrate every spec and test. Phase 4
  (optional) — collapse the two binding stacks.
