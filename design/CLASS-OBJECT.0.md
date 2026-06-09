# Class / Object Split — Container Symmetry and the `class` Word

Status: **decisions accepted 2026-06-09** (language owner, via design
review); **implementation pending** — nothing below is built yet.
Promote the decision summary to an ADR entry when Phase 1 lands
(a stub ADR-005 records the decision now; this document owns the
design and plan).

## 1. Decisions

1. **Container symmetry.** Adopt the 2×2: `List : Array :: Map :
   Object`. `Object` becomes a core, *constructible*, *enumerable*,
   *mutable* keyed container — the keyed sibling of `Array`.
   **Store stays a separate surface type** (scoped copy-on-write
   context semantics), even though it shares the prototype-chain
   machinery internally.
2. **`class` word.** The class mechanism splits out of plain Object
   under its own surface: `def Box (class {v:0})`, subclassing
   `def Sub (class Box {extra:0})`, instances via `make Box {…}`.
   `refine Object` becomes a deprecated alias for a release wave.
3. **Sealed instances.** A class instance rejects a write to an
   undeclared field with a loud error. The open-bag use case migrates
   to the mutable Object.
4. **Prototypes stay internal.** The engine's prototype chains
   (instance defaults via `buildBasePrototype`; context layering in
   `contextstack.go`) remain implementation details. No surface
   `proto` exposure, and — emphatically — no prototype *method*
   dispatch: polymorphism stays with signature dispatch through the
   type lattice (one dispatch mechanism, the ADR-004 principle
   applied to dispatch).

## 2. Current state (what the split untangles)

Today `Object` plays three roles at once:

- **Class machinery.** `def Box (refine Object {v:0})` mints a nominal
  lattice type with a field schema and defaults; `make Box {…}`
  constructs tagged instances whose defaults live on an internal
  prototype chain built recursively up the parent types
  (`eng/go/core_make.go::buildBasePrototype`, consulted by
  `GetField`). Methods are free functions dispatched on the nominal
  type (`design/OBJECT-METHODS.0.md` — no `this`/`self`, by design).
- **Mutable bag.** `set` on an instance accepts *undeclared* dynamic
  fields under computed keys — but they are invisible to enumeration
  (`StructUtil.items` returns `[]` for them). Writes succeed,
  iteration lies: the worst of both contracts.
- **Scope machinery.** Store/context layers are `ObjectInstanceInfo`
  values chained by `Prototype` (`eng/go/contextstack.go`).

Adjacent facts the design leans on:

- `set` on Map is **copy-returning** as of 2026-06-09 (Map stays
  immutable; `{} set a 1 set b 2` chains). The mutability rule is now
  explicit: *mutable containers mutate in place and return nothing
  from `set`; immutable values return the updated copy.*
- `Array` — the mutable indexed type — is barely constructible from
  the surface today (`make Array` is unsupported; instances come from
  the host side). The symmetry gives it a story by analogy, but that
  is follow-on work, not part of this plan.
- `make Object {}` is currently an error ("expected a constructed
  object type") — the voxgig DX report B5 item.

## 3. Design

### 3.1 The 2×2

|            | immutable value | mutable container |
|------------|-----------------|-------------------|
| indexed    | `List`          | `Array`           |
| keyed      | `Map`           | `Object`          |

- `set` contract column-wise: immutable → copy-returning (Map today;
  List is an open question, §6); mutable → in-place, returns nothing
  (Array, Object, Store).
- **Plain Object** (`make Object {}` or `make Object {a:1}`):
  - open: any string/atom key may be set, computed keys via parens;
  - fully enumerable: `StructUtil.items` / `size` see every field;
  - in-place `set` (already the Object behaviour), `get` with
    literal/computed key semantics unchanged;
  - `eq` identity / `deq` structural, like the other compounds.
- **Store** keeps its surface (context, copy-on-write layering,
  strict `get`). Documented relationship: "a Store is an Object with
  scope-chain semantics" — but no type merger.

### 3.2 The `class` word

```
def Box (class {v:0})              # mint nominal type, schema, defaults
def Sub (class Box {extra:0})      # subclass: parent chain, inherited defaults
def b (make Box {v:1})             # constructor (unchanged)
b set v 2                          # in-place field write (declared field)
b set w 9                          # ERROR [aql/sealed_field] — see 3.3
```

- `class {schema}` is a type expression, fitting the existing minting
  idiom (`def Name (refine Base …)`, `def Big (Integer gt 10)`).
- `class Parent {schema}` extends; defaults inherit via the existing
  internal prototype chain (no new machinery).
- `refine Object {…}` parses as an alias for `class {…}` during the
  deprecation wave, then warns, then is removed. Bare `refine` on
  scalar types (newtypes, predicate subsets) is untouched.
- Methods are unchanged: free functions over the instance, signature
  dispatch on the nominal type. If implicit-receiver ergonomics are
  ever wanted, the compatible route is OBJECT-METHODS Option A
  (implicit `self` as first sig param — still signature dispatch).

### 3.3 Sealing

Writing an undeclared field on a class instance raises:

```
[aql/sealed_field]: set: 'w' is not a field of Box (fields: v)
  = hint: class instances are sealed — declare the field in the class
    schema, or use a plain Object (make Object {…}) for open data
```

- Loud at the `set`, pointing at the causing site, with an actionable
  hint — per the error-quality bar in `design/ERRORS.0.md` §7.
- This *removes* today's silently-accepted dynamic fields on
  instances. That behaviour is currently non-enumerable and therefore
  almost certainly unrelied-upon, but the change is breaking in
  principle → upgrade note.

### 3.4 Prototypes (internal only — decided)

No `proto` word, no constructor option, no chain walking from AQL.
Rationale recorded for posterity: the chain exists twice internally
(instance defaults, context scoping) and both stay; exposing
delegation invites prototype method dispatch, which would be a second
dispatch mechanism alongside signature matching — the same
"two conventions" failure mode ADR-004 closed for argument
collection. Revisit only with a concrete use case that the class
defaults + module system cannot express.

## 4. Interactions with open proposals

- **B5 (`make Object {}` hint), `design/ERRORS.0.md` §4** — superseded
  by Phase 2: `make Object {}` becomes *valid* (an empty mutable
  Object) instead of erroring with a better message. The hint
  proposal should not be implemented ahead of this.
- **T9.1 residual (voxgig DX)** — plain-Object enumerability removes
  the "dynamic fields aren't enumerable" side gap; the perf residual
  (copy-per-insert on Map) still parks with the P4 native persistent
  map.
- **`StructUtil.items` / `size` / `walk`** gain Object coverage as
  part of Phase 2 (spec rows per ADR-003 discipline where words are
  module exports; core spec rows otherwise).

## 5. Implementation phases

| Phase | Content | Breaking? |
|-------|---------|-----------|
| 1 | `class` word as alias surface over the existing refine-Object machinery; docs lead with `class`; `refine Object` still works silently | no |
| 2 | `make Object {}` / `make Object {a:1}` construct plain open Objects; full enumeration (`items`, `size`); docs + REFERENCE 2×2 table | no (turns an error into a value) |
| 3 | Seal class instances (`sealed_field` error); upgrade note | **yes** (removes silent dynamic fields on instances) |
| 4 | Deprecation warning on `refine Object`, then removal; README upgrade-notes entry | **yes** (rename wave) |

Each phase carries the standard test discipline: positive + negative
spec rows (notably: sealed write errors, undeclared-field enumeration,
`make Object` with non-map argument, type-literal guards), fnmodel
golden regeneration where signatures change, and `describe` /
REFERENCE / TUTORIAL updates in the same change.

## 6. Open questions (not blocking Phase 1)

1. **List `set` symmetry.** Map now has copy-returning `set`; List
   does not (`[10 20] set 0 99` is a signature error). The 2×2 argues
   List should gain the same copy-returning form. Decide before
   documenting the column rule as universal.
2. **Object literal syntax.** Is `make Object {…}` enough, or does a
   distinct literal ever pay for itself? (Recommendation: `make` is
   enough; a literal would collide with Map braces.)
3. **Array constructibility.** `make Array [..]` by symmetry — same
   shape as Phase 2, separate decision.
4. **`class` introspection.** What does `typeof` / `describe` show for
   a class type and its schema? (Likely: reuse the existing
   ObjectTypeInfo rendering; confirm during Phase 1.)
