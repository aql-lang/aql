# AQL Behavior Mechanism — Commentary and Comparison

## What it is

AQL associates per-type capabilities — `compare`, `canon`, `nodify` —
with a type's `Behavior` slot on its `*Type`. The kernel-side free
functions (`eng.CompareValues`, `eng.Value.String`, `eng.NodifyValue`)
dispatch by walking the value's `VType` parent chain looking for a
`Behavior` that implements the corresponding optional capability
interface (`Comparer`, `Format`-via-`TypeBehavior`, `Nodifier`).
Three layers ship today:

1. **Kernel scalars** — `numberCompareBehavior`, `stringCompareBehavior`,
   etc., attached to `TNumber` / `TString` / `TBoolean` / `TAtom` at
   package init time.
2. **Native domain types** — `dateFormatBehavior`, `instantFormatBehavior`,
   `clkDurationFormatBehavior`, each implementing `Compare` alongside
   their existing `Format` method.
3. **User-defined types** — installed via the `behave NAME (fn …)` word:

```aql
type Person object {name:String age:Integer}
behave compare/q (fn [[Person Person] [Integer] [(a 'age' get) (b 'age' get) sub]])
behave canon/q   (fn [[Person]        [String]  [(a 'name' get)]])
behave nodify/q  (fn [[Person]        [Any]     [{n:(a 'name' get) age:(a 'age' get)}]])
```

The `behave` handler validates the fn's first sig against the named
behavior's expected shape, extracts the target type from the input
params, and wraps the type's existing `Behavior` with a `userBehavior`
struct holding the body in the appropriate capability slot. The wrapper
delegates `Match` / `Format` / `Equal` to the previous `Behavior` and
overlays the capability methods on top, so installing `canon` on top of
`compare` (or vice versa) is additive — both slots stay live on one
wrapper.

The kernel keeps a small closed registry of recognised behavior names
(`compare`, `canon`, `nodify`). Plugins extending the system can add
entries at engine-init time, but the closed-by-default posture means
the kernel always knows how to dispatch any installed behavior — there
is no "user-defined capability the kernel can't see".

## Comparison to other languages

The closest cousins are **Common Lisp's CLOS generic functions** and
**Clojure's protocols**. All three share the same shape:

- Capability declared once, kernel-side (CLOS `defgeneric`, Clojure
  `defprotocol`, AQL `behaviors` table).
- Implementations added per-type after the fact (CLOS `defmethod`,
  Clojure `extend-type`, AQL `behave compare/q (fn …)`).
- Dispatch through runtime lookup rather than compile-time resolution.

AQL's behavior-name registry being kernel-closed is more like Clojure's
protocols than CLOS — Clojure also requires a protocol be defined
before extensions can target it.

Where AQL diverges:

- **Open registration in any program**, not just at type-declaration
  time. ML-family languages (Haskell type classes, Rust traits, OCaml
  modules) require all instances declared in the module that owns
  either the type or the class — Rust calls this the orphan rule. AQL
  has no orphan rule; any program can `behave` on any non-builtin type
  in scope. Python's `functools.singledispatch` is the closest
  mainstream parallel; Smalltalk / Ruby reach this through reopened
  classes; Java / C# can't get here without external-dispatch tricks
  like visitor.

- **Single dispatch on the LCA**, not on individual operand types.
  CLOS does true multimethods (dispatch on every arg's class); AQL
  collapses to "find the lowest common ancestor of the operand types
  and ask its `Behavior`" — strictly less powerful but easier to
  reason about. Cross-type compare (Integer + Decimal → Number) falls
  out naturally; cross-type addition (Date + CalDuration) would need
  either a multimethod-style extension or the user attaching the impl
  to the LCA themselves.

- **Capability split between projection and encoding**: `nodify` is
  the projection step (returns a Node or Scalar — data-shape, not
  JSON-encoded); `jsonify` composes nodification with voxgig/struct's
  JSON encoder. Most languages collapse these — Python's `json.dumps`
  calls `default()` and serialises in one step, Rust's
  `serde::Serialize` produces wire bytes directly, Go's `MarshalJSON`
  returns `[]byte`. AQL exposing the intermediate Node as a
  first-class result lets users compose with downstream AQL
  transforms before any encoder runs — a fit for a query language.

- **Capability methods are Go interfaces** detected via type
  assertion (`t.Behavior.(Comparer)`). Structurally identical to how
  Go's stdlib finds `error`, `Stringer`, `MarshalJSON`. The novelty
  is that the `Behavior` pointer can be swapped at runtime — Go's
  interface dispatch is otherwise locked at compile time.

**Quick taxonomy:**

| Language | Add capability to existing type? | When? | Dispatch on |
|----------|----------------------------------|-------|-------------|
| Java / C# | No (must edit class) | Compile time | Receiver only |
| Go | Via interface (read-only fit) | Compile time | Receiver only |
| Rust | `impl Trait for T` (orphan-checked) | Compile time | Receiver only |
| Haskell | `instance` (orphan-checked) | Compile time | One type per class param |
| Python | Yes (reopen / `singledispatch`) | Runtime | Receiver (or first arg) |
| Ruby / Smalltalk | Yes (reopen class) | Runtime | Receiver only |
| Clojure | Yes (`extend-type` after `defprotocol`) | Runtime | Receiver only |
| Common Lisp / CLOS | Yes (`defmethod`) | Runtime | All args (multimethods) |
| **AQL** | **Yes (`behave`)** | **Runtime** | **LCA of operand types** |

## Type considerations

### Structural typing for validation, nominal typing for dispatch

AQL splits the role types play depending on whether they validate or
dispatch — the subtlest gotcha for users coming from nominal languages.

**Validation is structural.** `def x:Person {name:'A' age:30}` validates
that the map literal has fields matching Person's shape. The map is
accepted because its structure fits — but its `VType` stays as `TMap`,
not `Person`. The Person name was a *gate* the value had to pass; once
through, the value forgets it ever heard of Person.

**Dispatch is nominal.** `value lt other` walks `value.VType`'s parent
chain looking for a `Comparer`. If `value.VType` is `TMap`, it'll find
whatever Map has (nothing today) — it'll *never* find Person's
Comparer, even though that value was just validated as a Person.

The way to *carry* nominal identity is `make`:

```aql
type Person object {name:String age:Integer}
behave compare/q (fn [[Person Person] [Integer] [(a 'age' get) (b 'age' get) sub]])

def x:Person {name:'A' age:30}            ; x.VType = TMap     — compare misses
def y      (make Person {name:'B' age:25}) ; y.VType = Person   — compare hits
```

Same input, two different runtime identities — one carrying Person, one
not. The compiler-of-record (the kernel) won't warn about either.

How other languages handle this:

- **Java / C# / Rust**: nominal throughout. `Person p = …` means p IS a
  Person; dispatch works. The construction site is where the type is
  asserted, and that assertion sticks.
- **Go**: nominal but with explicit conversions (`Person(x)`). After
  conversion the value carries Person; before, it doesn't.
- **TypeScript**: structural throughout. There is no per-type dispatch
  to begin with — methods live on classes, types are just shapes.
- **OCaml**: structural for objects, nominal for variants. The dispatch
  question mostly doesn't arise because OCaml objects aren't where
  variant-style dispatch happens.

AQL splits the difference deliberately. Structural validation keeps
typed bindings flexible (any old map fits if its shape is right), while
nominal dispatch keeps behavior lookup predictable (only `make` produces
a value that behaves as the named type). The cost is the gotcha above:
declaring `def x:Person …` does not make x dispatch as a Person.

Practical advice: if you want dispatch, use `make`. If you only want
validation, typed `def` is enough.

A possible future refinement: have typed `def` wrap the value with a
"carrier" that records the asserted type, so dispatch can find Person's
behaviors on a typed-bound map. This would unify the two roles at the
cost of one indirection per typed binding — worth measuring before
landing.

### Lattice walk is purely covariant

Descendants inherit ancestor capabilities (Integer inherits Number's
Compare); ancestors do not inherit descendant capabilities; sibling
branches never share. No contravariance handling. If you `behave
compare/q` on `Vehicle`, every Car / Truck instance picks it up
automatically; if you then add one specifically for Car, Car wins via
the walk-from-LCA-upward order. This is the Liskov-friendly direction
and the right default; the Eiffel-style covariant override of input
types isn't expressible.

### Predicate-type dispatch (implemented)

A predicate type `type Positive fn [n:Integer Integer [(n gt 0) guard
n]]` was historically dispatch-invisible: it had no `*Type` of its
own (just an FnDef value on the type stack), so `behave compare/q (fn
[[Positive Positive] …])` had no values to dispatch on — no value's
`VType` was ever `Positive`. The lattice walk never reached it.

The current kernel resolves this for predicate types whose input is
concrete (`[n:Integer …]` rather than `[x:Any …]`):

1. **`InstallType` detects predicate shape via `PredicateInputType`**
   and mints the `*Type` parented at the predicate's input type (not
   at `TFnDef`). Positive's parent becomes Integer; the lattice walk
   from Positive reaches Integer → Number → Scalar, picking up
   numeric Comparers along the way.

2. **The typed-bind path rewraps the validated value with the
   predicate's `*Type`.** `def x:Positive 5` runs the predicate body
   (`(n gt 0) guard n`), gets back `5` if it satisfies, then sets
   `out.VType = Positive` before installing. The underlying `Data`
   is unchanged — `AsInteger(x)` still works — but the VType change
   lets `CompareValues` find behaviors registered on Positive.

3. **`CanonValue` walks the parent chain for user-defined types**
   looking for an installed canon body. Built-in Behaviors (List,
   Map, Date, …) are deliberately skipped — their Format methods
   produce `Value.String`'s debug shape rather than Canon's
   source-shape conventions — but user `behave canon/q` bodies on
   non-builtin types win.

Predicate types declared with `Any` input — the historical `fn [x:Any
Any […]]` shape used by `guard`-style validators that check the type
inside the body — remain pure validation gates. Their `*Type` stays
parented at `TFnDef` and rewrapping is skipped; rewrapping would
break rendering and downstream type tests because the value would
appear as a Function instance with the underlying scalar tucked into
`Data`. This split is intentional: `Any`-input predicates have no
meaningful parent on the dispatch path, so giving them one would be
worse than the current behavior.

### Earlier options considered

For reference, the design space considered during implementation:

1. **Stay as-is, document the distinction.** Predicate types remain
   gates, nominal types remain categories. Users who want
   predicate-driven behavior wrap their concern in a nominal type
   (`type PositiveNum object {n:Integer}` plus a predicate guarding
   construction). Cleanest from a kernel perspective; pushes the work
   onto users. The current state.

2. **Predicate-aware dispatch.** Extend `CompareValues` /
   `NodifyValue` to fall through to a predicate-walk when the lattice
   walk yields no Comparer. For each predicate type in scope,
   evaluate the predicate against both operands; if it accepts both
   AND has a Compare attached (through a new `behave compare/q (fn
   [[Positive Positive] …])` path that special-cases predicate
   targets), use that. No change to `Value`; arbitrary scope. Two
   downsides: predicate evaluation per `lt`/`gt` call costs an
   engine run, and ordering across in-scope predicates is awkward
   ("first match" depends on type-stack push order, which is fragile
   for users).

3. **Tagged values.** Extend `Value` with an optional set of
   "satisfied predicate-type names" populated by the kernel during
   typed bind. The dispatch consults the tag instead of re-evaluating
   predicates. Sidesteps the runtime cost of option (2) at the price
   of widening the `Value` payload — touches every value flowing
   through the engine, including ones that never see a predicate
   type.

4. **Mint a `*Type` for each predicate type, plus a typed carrier on
   binding.** This is the cleanest extension because it reuses the
   existing dispatch wholesale.

   When `type Positive fn […]` is declared, also mint a fresh
   `*Type` named `Positive`, parented at the predicate's input type
   (`Integer` in this case). The predicate body becomes the
   `Match` method on this Type's Behavior (which already exists in
   the kernel for predicate types).

   When the kernel performs a typed bind `def x:Positive 5`, it
   wraps the value `5` in a lightweight `TypedCarrier{Type:
   Positive, Value: 5}` — a `Value` whose `VType` is `Positive`
   and whose `Data` is the underlying `5`. Accessors
   (`AsInteger`, etc.) unwrap transparently; the only thing that
   changes is the `VType` reported by `typeof` and consulted by
   dispatch.

   With this in place, `behave compare/q (fn [[Positive Positive]
   …])` Just Works through the existing LCA walk — the carrier's
   `VType` is `Positive`, the lattice finds the registered
   Comparer, and the body runs. The same mechanism subsumes the
   structural-vs-nominal gotcha above: `def x:Person {…}` would
   wrap the map in a `Person`-typed carrier so Person's behaviors
   dispatch on it.

   The cost is one `Value` allocation per typed bind, plus a small
   amount of kernel logic to unwrap carriers in accessors. The
   model becomes uniform: a value's runtime identity is what its
   `VType` says it is, regardless of whether the type is nominal
   (record), structural-with-assertion (predicate), or kernel
   (scalar). Worth measuring before landing — typed binds are not
   universal but they're not rare either.

Option (4) was the one landed (a thinner version than originally
described — no separate carrier struct; the rewrap just sets `out.VType
= def` since the payload shape is already compatible with the
predicate's input type). Options (2) and (3) are narrower fixes that
would have left the structural-vs-nominal split intact.

The same idea is what would close the structural-vs-nominal gotcha
for *object-shape* user types — a typed bind would wrap the map with
a Person-typed carrier. Object types are not addressed yet: a Map
rewrapped as a Person would have `VType.Parent = TObjectType`, which
breaks the lattice walk for things like `is Map` (Person doesn't
descend from Map). A proper fix probably means a Carrier value that
remembers both the asserted type AND the underlying VType, with
accessors that consult both. Out of scope for the predicate-type
work.

### No method overloading on the same type

One behavior name → one capability slot per type. Can't have
`compare-by-age` versus `compare-by-name` on the same type — users
emulate via distinct nominal types (`type PersonByAge object {…}`) or
by writing an ad-hoc comparator at the call site. CLOS has `:before` /
`:after` / `:around` and qualifier-keyed methods; Clojure has
multimethods with arbitrary dispatch fns. AQL stays minimal here,
which keeps the dispatch path predictable and the kernel surface
small.

### Behavior name registry as contract

The closed kernel table (compare / canon / nodify today) is a design
decision, not a limitation: new capabilities need kernel awareness
because the kernel needs to know how to dispatch them. CLOS lets users
define new generic functions freely; AQL's narrower surface trades
that flexibility for guaranteed dispatch. The natural extension point
is allowing plugins to register new behavior names at engine-init time
(parallel to `RegisterExternalBuiltin` for types) — a small future
change.

## Summary

The overall posture: AQL sits closer to **Clojure protocols + Lisp-style
runtime extension** than to ML-family type classes. The kernel keeps a
small list of dispatchable capabilities; the world extends the impls.
Two interesting bets:

- **Projection / encoding split** (`nodify` decomposed from `jsonify`).
- **AQL-body-as-Go-interface bridge** (the `userBehavior` wrapper) —
  user-supplied AQL fn bodies become first-class implementations of
  Go-side capability interfaces, dispatched alongside the kernel and
  native implementations through the same mechanism.

Both are individually uncontroversial; the combination is what gives
AQL behavior dispatch its distinctive character.
