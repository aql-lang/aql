# Traits — Named Contracts over Open Multimethods

Status: **decisions accepted 2026-06-09** (language owner, via design
review); **implementation scheduled with generics**
(`design/GENERICS.0.md` — whose `T extends Comparable` constraint
examples are waiting for exactly this). ADR-006 records the decision
summary; this document owns the design.

## 1. Decisions

1. **Adopt traits as named contracts, designed now, built with
   generics** (or earlier if a stdlib module needs a contract first).
2. **Explicit conformance with an immediate loud check:**
   `Circle implements Shape` verifies completeness at the declaration
   and registers membership; missing or mismatched overloads error,
   listed by name.
3. **Trait declarations carry full signatures with `Self`** — not
   bare word names — so completeness covers arity and return types,
   and check mode learns real types from trait-typed values.
4. **No default method bodies in v1.** Traits are pure contracts: no
   state, no code. Mixin-style defaults can be added compatibly
   later; shipping them now and removing them later could not be.

## 2. Why traits, and why only this much

The dispatch half of typeclasses already exists (verified on main):

- **Open multimethods.** Separate `def`s of one fn name *merge*
  overloads (`def area fn [[p:Pt]…]` then `def area fn [[p:Pt3]…]` —
  both live, signature dispatch picks correctly). Anyone can extend a
  word over their own type.
- **Set-theoretic types.** `tor`/`tand`/`tnot` close the lattice
  under Boolean algebra, so "a type denoting a set of types" is
  native vocabulary.
- **`behave`** installs shape-validated *kernel* capabilities
  (compare/canon/nodify/unify) on user types — an explicit
  trait-implementation mechanism, but closed to the four
  engine-defined capability names.

What no existing feature provides is the **contract**: a named bundle
of required operations, a completeness check at the site where
conformance is *intended* (rather than a `no matching signature` at
some distant call site), and a type to write in parameter slots and
generics constraints meaning "anything that satisfies this".

Traits therefore **constrain and check; they never dispatch**.
Calling a required word on a trait-typed value is ordinary
multimethod dispatch. One dispatch mechanism — the ADR-004 / ADR-005
principle, applied a third time.

## 3. Surface

```
def Shape trait {
  area:      fn [[Self] [Float]]
  perimeter: fn [[Self] [Float]]
}

def Circle class {r: 0.0}
def area      fn [[c:Circle] [Float] [c.r dup mul 3.14159 mul]]
def perimeter fn [[c:Circle] [Float] [c.r 2.0 mul 3.14159 mul]]

Circle implements Shape
# checks NOW: for every required name, an overload exists whose
# Self positions accept Circle and whose returns conform after
# Self := Circle substitution. On failure:
#   [aql/trait_unsatisfied]: Circle does not satisfy Shape —
#     missing: perimeter (fn [[Circle] [Float]])
#   hint: define the listed overloads before `implements`

def total fn [[s:Shape] [Float] [(s area) add (s perimeter)]]
(make Circle {r:1.0}) total            # ordinary multimethod dispatch

Circle is Shape                        # returns true (after implements)
def Flat (Shape tand (tnot Circle))    # traits join the type algebra
def sort-shapes gen [(T extends Shape)] fn [[ [:T] ] [ [:T] ] [ … ]]
```

- `def Shape trait {…}` is paren-free — the same nested-collection
  shape as `def Foo class {…}` and `def name fn […]`. `trait` and
  `implements` are unclaimed names (verified).
- A trait may be implemented by classes **and builtin/scalar types**
  (`Integer implements Comparable`) — the conformance check is over
  the overload table, which is type-agnostic.
- A type may implement any number of traits; trait membership is
  orthogonal to the single `refine` parent chain. No diamonds: traits
  carry no state and (v1) no bodies.
- Re-declaring the same `implements` is a no-op (idempotent), so
  module load order can't double-register.

### `Self` semantics

`Self` is a placeholder substituted with the implementing type:

- In **parameter** positions: the conformance check requires an
  overload whose corresponding position accepts the implementing
  type (the implementing type or a supertype of it — normal
  signature-compatibility, contravariant reading).
- In **return** positions: the overload's declared return must
  conform to the substituted type (covariant reading) — so
  `clone: fn [[Self] [Self]]` requires Circle's `clone` to return a
  Circle, not merely a Shape.
- Multiple `Self`s are allowed (`cmp: fn [[Self Self] [Integer]]`).

### As a lattice type

`Shape` denotes the set of types that have declared conformance. It
participates everywhere a type can appear: signature slots, `is`,
`unify`, `tor`/`tand`/`tnot` (with the usual identities), and
generics' constraint check (`isSubtype(arg, Shape)` = "arg declared
implements Shape, or arg is a subtype of a type that did").
Subtype propagation: if `Circle implements Shape`, a
`def SmallCircle refine Circle {…}` is a Shape too — its instances
satisfy every Circle-typed overload.

## 4. Static checking

- A `Shape`-typed carrier flowing through check mode lets the
  analyser type calls to required words via the trait sigs with
  `Self := <carrier type>` substituted — slotting into the existing
  carrier inference rather than degrading to `Any`.
- `implements` itself is fully static-checkable; `aql check` runs the
  same completeness check without executing bodies.
- Dead-overload detection composes: an overload required only via a
  trait is *not* dead while the trait has members.

## 5. Errors

| Code | When |
|------|------|
| `trait_unsatisfied` | `implements` finds missing/mismatched overloads — message lists each required name with the expected substituted sig |
| `trait_error` | malformed trait body (non-fn value, missing Self, etc.) |

Both meet the `design/ERRORS.0.md` §7 quality bar: blame at the
declaration site, actionable hint, dispatchable code.

## 6. Non-goals (v1)

- **Default method bodies / mixins** — decided out; revisit only as a
  compatible addition.
- **Implicit structural conformance** (Go-style) — rejected:
  accidental conformance, and membership that shifts with import
  order would make check results unstable.
- **Trait-driven dispatch** of any kind — dispatch stays signature
  matching, full stop.
- **State on traits.**
- **`behave` unification** — `behave` remains the kernel-capability
  hook. A later bridge may define e.g. `Comparable` as the trait the
  compare behavior witnesses, but v1 keeps them separate.

## 7. Open questions

1. **Super-traits** — `def Shape3 trait Shape {volume: …}` (a trait
   requiring another). Natural extension of the `refine`-extends
   reading; decide when generics lands.
2. **Coherence / orphan declarations** — may any module declare
   `SomeoneElsesType implements SomeoneElsesTrait`? v1 leaning:
   allow it (defs are run-scoped, and `implements` is idempotent),
   revisit if module-ecosystem conflicts appear.
3. **Blanket/conditional conformance** — `(List of [T]) implements
   Shape when T implements Shape` — explicitly deferred to the
   generics implementation phase.
4. **Seeded standard traits** — which contracts ship named
   (`Comparable`, `Sizable`, `Showable`?), and where they live
   (core vs `aql:type-util`). Decide alongside the behave bridge.
