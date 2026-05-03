# AQL Type System Review — Algebraic and Dependent Aspects

This report catalogues weaknesses, semantic gaps, implementation gaps,
and developer-experience issues in the AQL type system as it stands on
the `claude/document-q-modifier-rules-AGp0I` branch (latest commit
`d154b3e`). The scope is the algebraic side (`tor`/`tand`/`tany`/`tall`,
disjunctions), the dependent side (`DepScalar` family, predicate types,
structural fn-shape types), and the surrounding dispatch / planner /
namespace machinery that those features rely on.

Each item cites a file and (where relevant) line so the next person
working on it can find the call site without spelunking. Items are
ordered by category, not severity. A short suggested phase plan
follows the catalogue.


## 1. Algebraic side

### 1.1 `tand` of non-maps falls through to `Unify` — RESOLVED

`internal/engine/native_type_tand.go` now returns `Never` (the bottom
type) when `Unify` fails, instead of erroring. `tall` and the
record-merge path do the same. `tor`/`tany` treat `Never` as the
identity element and filter it out of disjunct alternatives, so
`String tor Never = String` and `[Integer Never] tany = Integer`.
The two operators now share a consistent algebra: `tand`'s zero
case is `Never`, and `tor`'s identity is `Never` (dual to `tand`'s
identity `Any`).

`Never` itself is a registered top-level type (`engine.TNever`,
`types.go:86`); it's uninhabited (only unifies with itself per
`unify.go`'s Never branch), so `v is Never` is `false` for every
concrete value `v`. Tests in `aql/test/type_never_test.go`.

### 1.2 No distribution / De Morgan — RESOLVED

`internal/engine/native_type_tand.go` now distributes `tand` over
`tor`. When either side is a disjunct, `tandValues` computes the
cross product, recursively reduces each pair, drops `Never` results
(via `tor`'s identity rule), and dedupes structurally identical
alternatives. `(A tor B) tand C` reduces to `(A tand C) tor (B tand C)`;
both sides being disjuncts produces the full DNF expansion.

`tall` calls the same helper, so n-ary intersections distribute
through every fold step. The dedup uses `valuesEqual` guarded by
`VType.Equal` (the existing convention — `valuesEqual` shortcuts
to true on Data==nil regardless of VType, so callers that compare
type literals must pre-check VType).

Tests in `aql/test/type_distribute_test.go` cover left- and
right-side disjuncts, both-side cross product, full DNF collapse to
`Never`, dedup, `Never` filtering pre-distribution, value-level
disjuncts, and `tall` folds.

De Morgan rewrites for `not` are not yet in scope — there is no
type-level negation operator, so there is nothing to dualise.

### 1.3 No `DepScalar ↔ DepScalar` unification

`internal/engine/unify.go` Dependent-scalar branch handles
`DepScalar ↔ scalar` only. `(Integer gte 10) tand (Integer lte 20)`
constructs two `DepScalar` values and calls `tand` on them — `tand`
falls through to `Unify`, which has no `DepScalar ↔ DepScalar` rule
and reports failure. The `DepKind` bit field was deliberately sized
to encode combined constraints (`internal/engine/depinteger.go:8-21`)
but no path actually produces one. The "future extensibility" comment
on the bit field is unredeemed.

### 1.4 Disjunct dedup uses structural `valuesEqual`

`internal/engine/carrier.go:344-373` (`mergeAlternatives`) uses
`valuesEqual` to dedup disjunct alternatives. For type literals
that's fine; for concrete map alternatives, it compares the
underlying ordered-map by structural equality, so two `record [x:Integer]`
values with different `RecordTypeInfo.ID`s won't dedup, but two
*structurally identical* concrete maps will collapse — counter-intuitive
in either direction. Rare in practice; worth tightening.


## 2. Dependent side (`DepScalar` family)

### 2.1 Closed family of leaves

`internal/engine/depinteger.go:75-118`. Two parallel hand-maintained
switches:

- `dependentLeafFromBoundType` — bound type → leaf name.
- `dependentLeafBaseType` — leaf name → base type.

Adding `DepDate` / `DepInstant` / `DepCalDuration` / `DepArray`
requires editing both. The "Dependent" lattice branch is hard-rooted
at `Type/Dependent/Dep<Leaf>` rather than living under the base type
itself, so the sub-typing relation is bolted on via a special case
in `Type.Matches` (`internal/engine/types.go:340-396`) — a `DepInteger`
value is *not* a path-prefix subtype of `Scalar/Number/Integer`; the
rule lives in custom Go.

### 2.2 Single-comparison only at the surface

`gt`/`gte`/`lt`/`lte` each set exactly one bit in `DepKind`. There's
no surface syntax for an OR'd combination such as `Integer between 10 20`
or `Integer in {1,2,3}`. The natural workaround (`(Integer gte 10)
tand (Integer lte 20)`) doesn't work because of 1.3.

### 2.3 Bound is a single `Value`

`DepScalarInfo.Bound` is one concrete value. No way to express set
membership, regex match, or length constraints over strings/lists.
Predicate types cover all of these but lose the analyser's ability
to see *what* the constraint is — see 4.

### 2.4 Display lossiness — `VType.Matches` panic risk

`Type.Matches` is overridden so `DepString.Matches(TString)` returns
`true` (`internal/engine/types.go:340-396`). Helpful for sig matching;
hazardous for any code that does `if v.VType.Matches(TString)
{ s, _ := v.AsString() … }`. Two such call sites are guarded with an
explicit `IsDepScalar` check ahead of the switch:

- `internal/engine/value.go:1485-1525` (`Value.String`)
- `internal/engine/registry.go:768-776` (`valToString`)

Other unaudited surfaces: `internal/engine/format.go`, `internal/formatter/`,
the JSON encoder in `internal/engine/print.go`, every `convert` /
`make` handler. A new place that adds `if v.VType.Matches(TInteger)`
and forgets the DepScalar guard is one line away from a panic on a
`DepScalarInfo` payload.


## 3. Predicate types (`type T fn [param Any [body]]`)

### 3.1 Single-arg only

Both `defTypedHandler` (`internal/engine/native_definition_def.go`)
and `is`'s handler (`internal/engine/native_type_is.go`) reject
`len(fnDef.Sigs[0].Params) != 1`. A two-argument predicate
`[x:Any cfg:Map]` is silently rejected, which blocks parameterised
type families like `BoundedInt(min, max)` defined in source.

### 3.2 No CheckMode story

Predicate evaluation goes through `Registry.CallAQL` against a
*concrete* value at runtime. Under `aql check`, the value is a
carrier (`Data=nil`); the predicate's `(x is String)` runs against a
Carrier and falls back to `Unify`, which returns `false`. So the
predicate always says "no" under check mode and every typed binding
errors. No tests exercise this — it's a silent UX hole for static
analysis users.

### 3.3 Predicate has full registry access (sandboxing gap)

`Registry.CallAQL` snapshots `DefStacks` lengths but not `r.Types`,
the context store (`ctxStack`), `ArgsStack`, or `CheckMode` flags.
A predicate body that calls `context set k v` mutates global state
during a unify check; one that calls `def x …` *can* leak via the
DefStacks restore window. Predicate bodies should run in a sandbox;
today they don't.

### 3.4 No predicate-vs-predicate compatibility op

There's no way to ask `Big ⊆ Mid` (where `type Big (Integer gt 100)`
and `type Mid (Integer gt 10)`). Only "does this *value* satisfy
this type". For dependent-type design that's a real loss — you can't
write a fn that takes "any subtype of Mid" without copying the
constraint into every signature.


## 4. Structural fn-shape types (`type T fn [[input] [output]]`, FnUndef)

### 4.1 Exact-match only

`internal/engine/native_definition_undef.go:71` (`fnSigMatchesSpec`)
uses `Type.Equal` for params and returns. `[Number]→[Number]` does
NOT satisfy `type Mapper fn [[Integer]→[Integer]]`. By the
contravariant-input / covariant-output rules of structural function
subtyping, the `[Number]→[Number]` candidate should satisfy the
constraint — it accepts more inputs and returns at-least-as-narrow
outputs. Variance was flagged as a follow-up in the original
`type+fn: function-shape types via FnUndef` commit and never landed.

### 4.2 `FnParam.Pattern` ignored

The structural matcher only looks at `params[i].Type`. A pattern on
the candidate (e.g. `[p:Point]`) is dropped on the floor.

### 4.3 `Optional` and `BarrierPos` not checked

A candidate with the same arity but different optional flags or a
different barrier position passes the structural matcher; downstream
calls then fail or behave differently than the type promised.


## 5. `r.Types` registry (the post-namespace-split state)

### 5.1 No shadowing

`r.Types[name] = body` overwrites; `def` has stack semantics
(`DefStacks[name] []Value`), `type` doesn't. Defensible if types are
truly singletons (the case rule encourages that) but inconsistent —
and there's no way to scope a temporary type for a sub-program.

### 5.2 Double-write for non-fn types

Non-fn type bodies still pass through `installDef` AND get mirrored
into `r.Types`
(`internal/engine/native_type_typedef.go:65`-ish). The recent
ObjectType-name-rebuild bug was exactly this drift — fixed by
re-fetching from `DefStacks` after `installDef`. Two sources of
truth that desync if anyone forgets the mirror.

### 5.3 No `untype Foo`

No removal analogue to `undef foo`. To re-bind a type you start a
new registry. Tests can't easily isolate.


## 6. Dispatch / planning gaps

### 6.1 No predicate-type CheckMode analysis

Static analyser sees `def n:Bbd v` as "install n with whatever the
predicate returns", carriers it as `Any`, doesn't try to evaluate
the constraint. For `DepScalar` specifically, the constraint is a
known shape (kind + bound) that *could* be checked symbolically
against a carrier-typed input — `Integer/15` carrier vs `gt 10`
constraint — but isn't. So check mode can't tell you ahead of time
that `def n:G10 5` will fail at runtime.

### 6.2 `sigTypeMatches` carrier rule is implicit knowledge

`internal/engine/signature.go:230` excludes Carrier values from the
metatype-matching path. If a contributor writes a native sig with
`[TScalarType, …]` and runs it under check mode, they have to
already know carriers count as "non-metatype" — that knowledge isn't
in `LANGREF.md`, `SIGNATURES.md`, or any code-level doc.

### 6.3 Forward planner accepts `def n:T anything`

The planner type-checks the constraint slot as `TAny`; the actual
unification happens in the handler. So check-mode can't catch
wrong-type bindings before runtime even when the constraint is a
plain `Integer` (where it trivially could).


## 7. Developer experience

### 7.1 No inline disjunct syntax

Every test writes `(Integer tor String)`. There's a `?:` shorthand
for record fields (`{x?:Integer}` = `tor None`) but no general
expression form like `Integer | String`.

### 7.2 `(quote name)` for fn-shape constraints is unidiomatic

`def m:Mapper (quote double)` is the only spelling that works for
fn-shape types. `def m:Mapper double` runs `double` (looking for an
Integer arg) and errors with a confusing "no signature" message
pointing at `double`. The system understands that this is a
typed-binding context but doesn't take the help-the-user step of
suggesting `(quote double)`.

### 7.3 Predicate body boilerplate

Every predicate is `if cond [val] [None]`. A `guard` / `keep-if`
word that takes a Boolean and wraps None-or-value would let the user
write the Bbd predicate as

```
type Bbd fn [x:Any Any [(x is String) and (x gte "b") and (x lte "d") guard x]]
```

instead of

```
type Bbd fn [x:Any Any [if ((x is String) and (x gte "b") and (x lte "d")) [x] [None]]]
```

### 7.4 Error messages don't name the type

`def n:Bbd "e"` errors with `def n: value does not satisfy predicate
type` — the user has to remember which type was at the colon. The
handler has the resolved constraint `Value` but not its registered
name (the map's key was `n`, not the type's name `Bbd`).

### 7.5 `inspect` for fn-shape types is sparse

`inspect Mapper` (where Mapper is a `FnUndef`) returns the
type-inspection map but `signatures` is `[]`. `buildTypeInspection`
has no case for the structural-fn shape, so the user can't see what
sigs Mapper requires without re-reading source.

### 7.6 Two ways to express the same thing, with no nudge

`type T (Integer gt 10)` and `type T fn [n:Any Any [if (n is Integer)
and (n gt 10) [n] [None]]]` are runtime-equivalent but use different
machinery. The `DepScalar` form is checkable in principle (6.1), the
predicate form isn't. There's no lint that says "this predicate is
expressible as a DepScalar" so users gravitate toward the predicate
form because it's the more general spelling.

### 7.7 No documentation

`LANGREF.md` has a brief atom rule and the `/q` block, nothing on:

- DepScalar (the shape, the unification rule, the gt/gte/lt/lte
  shorthand, the `Type/Dependent/Dep<Leaf>` paths)
- Predicate types (the None/value contract, the typed-def
  invocation rule, the `is` invocation rule, the
  not-independently-callable rule)
- Structural fn-shape types (FnUndef, the typed-def `(quote name)`
  idiom)
- The type/def case rule
- `r.Types` vs `DefStacks` separation (relevant for anyone writing a
  new word that consults named types)

Discoverability is "read the source".


## 8. Suggested phase plan

If the goal is biggest UX gain per LOC, I'd rank the items as:

1. **Variance in `fnSigMatchesSpec`** (4.1). One function. Unblocks
   real polymorphism over fn-shape types. Low risk because it
   strictly widens what was previously accepted.

2. **`DepScalar ↔ DepScalar` `Unify` plus a `between` surface form**
   (1.3 + 2.2). Closes the bit-field promise and makes `tand` over
   dependent integers consistent. Mid risk — needs care that the
   combined constraint reports correctly in `Value.String` /
   `valToString`.

3. **Sandbox predicate evaluation** (3.3). Wrap `CallAQL` with
   snapshot/restore for `r.Types`, context store, `ArgsStack`, and
   `CheckMode`. Removes a class of nondeterministic bugs and a real
   security concern for any future "evaluate user-supplied AQL"
   surface.

4. **Name the type in def/is errors** (7.4). Pass the constraint's
   source name through `defTypedHandler` and `is`'s handler closure.
   Pure UX, ~20 lines.

5. **`LANGREF.md` section on dependent + algebraic types** (7.7).
   No code change. Closes the worst of the discoverability gap.

6. **Predicate `guard` word** (7.3). Tiny addition, big readability
   win for the predicate-type idiom.

7. **`untype Foo`** (5.3). For test isolation.

Items 4.2, 4.3, 6.1, 6.2, 6.3 are real but each pulls in a wider
audit; I'd defer those until the items above are done.

The structural rework around 5.1 and 5.2 (single source of truth for
type values) is the most invasive and could be left for a v2 of the
type-system surface once the smaller items shake out.


## 9. Items not in scope of this report

- The bytecode AOT plan (`docs/reports/aql-bytecode-report.md`)
  intersects every type-system decision; weaknesses there are
  cataloged in that report.
- The check-mode step-budget and recursion handling are orthogonal
  and not reviewed here.
- Object-type inheritance has its own subtleties (the
  `Object/Foo/Bar` re-naming dance in `installDef`); only mentioned
  here as the immediate cause of 5.2.
