# AQL Type System Review — Algebraic and Dependent Aspects

This report catalogues weaknesses, semantic gaps, implementation gaps,
and developer-experience issues in the AQL type system. It started on
the `claude/document-q-modifier-rules-AGp0I` branch at commit `d154b3e`
and tracks the type-system work that has landed since.

Each item cites a file and (where relevant) line so the next person
working on it can find the call site without spelunking. Items are
ordered by category, not severity. A short suggested phase plan
follows the catalogue.

**Status legend.** Each subsection ends with a status tag:

- **RESOLVED** — implementation landed, tests cover it.
- **PARTIAL** — partly addressed; the remaining gap is described.
- (no tag) — not yet addressed.

A summary of the resolved items lives in §10.


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

### 1.3 No `DepScalar ↔ DepScalar` unification — RESOLVED

`internal/engine/unify.go` now handles `DepScalar ↔ DepScalar` over
the same leaf type. `DepScalarInfo` carries an optional second
constraint pair (`Kind2`, `Bound2`) so a single value can represent
a closed interval `(gte 10) tand (lte 20)`. Combination rules:

- Same-side bounds tighten: `gte 10 tand gte 5 = gte 10`.
- Opposite-side bounds form an interval: `gt 5 tand lt 10 = (5, 10)`.
- Empty intervals reduce to Never (caught at construction —
  `gt 10 tand lt 5`, `gt 10 tand lt 10`, etc.).
- Cross-base (`DepInteger tand DepString`) fails → Never.

`between` (`internal/engine/compare.go`) is the surface form:
`Integer between 10 20` ≡ `(Integer gte 10) tand (Integer lte 20)`.
The "future extensibility" promise on the `DepKind` bit field is
now redeemed via the dual-storage form.

Tests: `aql/test/type_algebra_test.go` (interval, same-side
tightening, empty interval, strict-touching, singleton, cross-base,
`between` membership-equivalence with the long form).

### 1.4 Disjunct dedup uses structural `valuesEqual`

`internal/engine/carrier.go:344-373` (`mergeAlternatives`) uses
`valuesEqual` to dedup disjunct alternatives in the static-analysis
path. (The runtime tor/tany path now uses `simplifyDisjunctAlts` —
see 1.5 — so this only affects check-mode carrier reasoning.) For
type literals that's fine; for concrete map alternatives, it
compares the underlying ordered-map by structural equality, so two
`record [x:Integer]` values with different `RecordTypeInfo.ID` won't
dedup, but two *structurally identical* concrete maps will collapse
— counter-intuitive in either direction. Rare in practice; worth
tightening with an ID-aware equality predicate.

Status: not addressed. Low impact (static-analysis-only path).

### 1.5 Subsumption + idempotent dedup at construction — RESOLVED

`tor` and `tany` now run `simplifyDisjunctAlts` at construction time
(`internal/engine/native_type_tor.go`,
`internal/engine/native_list_quantifiers.go`). Three reductions fire:

- Filter `Never` (identity for tor).
- Dedup structurally identical alternatives (idempotence:
  `T tor T = T`).
- Subsume strict subtypes: `Integer tor Number = Number`,
  `5 tor Integer = Integer`. Two concrete values of the same type
  are *both* kept (`1 tor 2` ≠ `1`).

`tand`'s distribution dedup uses the same helper, so DNF cross
products canonicalise consistently. Tests in
`aql/test/type_algebra_test.go` (idempotence, subsumption) and
`aql/test/boolean.tsv` (the previously-asserted non-deduping
outputs).

### 1.6 Empty-fold identity — RESOLVED

`[] tall` returns `Any` (identity for `tand`); `[] tany` returns
`Never` (identity for `tor`). Both are now full monoids over lists.
Previously each errored. Tests in `aql/test/type_algebra_test.go`.


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

Status: not addressed. Defer until a new Dep leaf is needed; current
Integer/Decimal/Number/String/Boolean/Atom set covers existing use
cases.

### 2.2 Single-comparison only at the surface — PARTIAL

`gt`/`gte`/`lt`/`lte` each set exactly one bit in `DepKind`. The
common interval case `(Integer gte 10) tand (Integer lte 20)` is now
covered both via tand-of-DepScalars (1.3) and via the `between`
surface form (`Integer between 10 20`). What's still missing: set
membership (`Integer in {1,2,3}`), regex match for strings, length
constraints. Each would need a new `DepKind` family or a different
storage shape; defer until a concrete need emerges.

### 2.3 Bound is a single `Value` — PARTIAL

`DepScalarInfo` now carries `Kind`/`Bound` plus optional `Kind2`/
`Bound2` (a closed-interval shape — see 1.3). Set membership, regex,
length still need richer payloads — same disposition as 2.2.

### 2.4 Display lossiness — `VType.Matches` panic risk — RESOLVED (hot paths)

`Type.Matches` is overridden so `DepString.Matches(TString)` returns
`true` (`internal/engine/types.go:340-396`). Helpful for sig matching;
hazardous for any code that does `if v.VType.Matches(TString)
{ s, _ := v.AsString() … }` — `AsString` errors but the underscore
swallows it, leaving the caller with a zero-value silent miscompile.

The high-traffic surfaces now have explicit `IsDepScalar` early
exits:

- `valuesEqual` (`internal/engine/unify.go`) — routes via
  `depScalarsEqual` which compares the constraint payload
  structurally (Kind/Bound and Kind2/Bound2).
- `exactEqual` and `deepEqual` (`internal/engine/compare.go`) —
  same early-exit; falls through to `valuesEqual`.
- `compareValues` (`internal/engine/compare.go`) — refuses to
  order DepScalar values, returning a clear "cannot compare
  dependent-type constraint with X" error rather than silently
  returning 0.
- `formatValueJSON` (`internal/engine/print.go`) — renders the
  constraint payload as a quoted JSON string.
- `aql_error.go` stack-rendering — renders the constraint payload
  in the trace label.
- `formatForPrint` (`internal/engine/print.go`) — already had a
  `v.IsDepScalar()` branch.
- `Value.String` and `valToString` — already guarded.

Tests in `aql/test/type_depscalar_safety_test.go` exercise the eq /
lt / print / no-panic paths.

Lower-traffic call sites under `internal/engine/format.go`,
`internal/formatter/`, every `convert`/`make` handler, and a long
tail of arithmetic/string helpers are NOT audited. The recommended
follow-up is a `Value.AsConcreteScalar()` accessor that errors
loudly on DepScalar payloads, used in place of the bare `As*` calls.
Until then, anyone adding a new `Matches(TInteger)` branch needs to
remember the guard.


## 3. Predicate types (`type T fn [param Any [body]]`)

### 3.1 Single-arg only

Both `defTypedHandler` (`internal/engine/native_definition_def.go`)
and `is`'s handler (`internal/engine/native_type_is.go`) reject
`len(fnDef.Sigs[0].Params) != 1`. A two-argument predicate
`[x:Any cfg:Map]` is silently rejected, which blocks parameterised
type families like `BoundedInt(min, max)` defined in source.

Status: not addressed. The cleaner path is via dependent types
(constructors like `between`); parameterised predicate types remain
a research direction.

### 3.2 No CheckMode story — RESOLVED

`RunPredicate` (`internal/engine/util.go`) now short-circuits when
`r.Check.Mode` is true, returning `(candidate, matched=true, nil)`
without invoking the body. Typed bindings flow past the analyser
slot; runtime catches actual violations later. `StripToCarriers`
also preserves type literals (Data already nil) so the
DepScalar-constructor sigs in `gt`/`gte`/`lt`/`lte`/`between`
continue to fire under check mode.

Tests in `aql/test/type_predicate_sandbox_test.go` cover the typed-
predicate accept and the runtime-still-rejects paths.

### 3.3 Predicate has full registry access (sandboxing gap) — RESOLVED

`RunPredicate` snapshots `r.Types`, `r.ctxStack`, and `r.Check`
before the `CallAQL` invocation and restores them on return — so a
predicate body that does `type Foo …` or `context set k v` cannot
leak into the surrounding program. `r.DefStacks` is already
protected by `CallAQL`'s own snapshot.

Tests verify that a predicate body that defines `type Leaked …`
during a `def x:Sneaky v` or `v is Sneaky` invocation does NOT
expose `Leaked` to a subsequent `def n:Leaked …`.

### 3.4 No predicate-vs-predicate compatibility op

There's no way to ask `Big ⊆ Mid` (where `type Big (Integer gt 100)`
and `type Mid (Integer gt 10)`). Only "does this *value* satisfy
this type". For dependent-type design that's a real loss — you can't
write a fn that takes "any subtype of Mid" without copying the
constraint into every signature.

Status: not addressed. Doable for `DepScalar`-shaped predicates via
constraint comparison; arbitrary predicate bodies need symbolic
execution. Defer.


## 4. Structural fn-shape types (`type T fn [[input] [output]]`, FnUndef)

### 4.1 Exact-match only — RESOLVED

`internal/engine/native_definition_undef.go` now provides
`fnSigSatisfiesSpec` alongside the original `fnSigMatchesSpec`. The
new function applies the standard structural subtyping rules:

- **Inputs are contravariant.** Each spec param type must be a
  subtype of the candidate's. `(Integer)→X` satisfies a constraint
  declared as `(Integer)→X`, and so does `(Number)→X` — the
  candidate accepts more.
- **Returns are covariant.** Each candidate return must be a
  subtype of the spec's. A function returning Integer satisfies a
  spec promising Number.

`fnDefHasSig` (in `fnsig_unify.go`) now calls the variance-aware
helper. The exact-match `fnSigMatchesSpec` is retained for `undef
name fn [spec]` — there the user is naming a specific shape to
discard, and exact matching is the right rule.

Pattern (FnParam.Pattern), Optional, and BarrierPos differences are
still not checked — see 4.2 and 4.3.

Tests in `aql/test/type_fnvariance_test.go` cover both the
contravariant and covariant directions, exact match (regression),
and Any/concrete edge cases.

### 4.2 `FnParam.Pattern` ignored — RESOLVED

`fnSigSatisfiesSpec` (`internal/engine/fnsig.go`) now compares
patterns: when the spec declares one, the candidate's pattern (if
any) must unify with the spec's. A candidate without a pattern still
satisfies a spec with one — the candidate's broader contract still
admits everything the spec demands.

### 4.3 `Optional` and `BarrierPos` not checked — PARTIAL (Optional resolved)

`Optional` is now part of the variance check. `spec.Optional[i]`
implies `sig.Optional[i]` (covariant on optional flags). A candidate
that's required where the spec is optional fails — the candidate
can't accept the omission the spec is allowed to make.

`BarrierPos` is intentionally NOT checked: `FnSigSpec` doesn't carry
a BarrierPos field (it's a body-level collection setting on FnSig,
not part of the structural shape), so the type system can't declare
a barrier requirement. To enable BarrierPos in fn-shape types,
`FnSigSpec` would first need the field plus a parser-side syntax —
a deferrable extension.


## 5. `r.Types` registry (the post-namespace-split state)

### 5.1 No shadowing

`r.Types[name] = body` overwrites; `def` has stack semantics
(`DefStacks[name] []Value`), `type` doesn't. Defensible if types are
truly singletons (the case rule encourages that) but inconsistent —
and there's no way to scope a temporary type for a sub-program.

Status: not addressed. Defer until sub-program-scoped types are a
real use case.

### 5.2 Double-write for non-fn types

Non-fn type bodies still pass through `installDef` AND get mirrored
into `r.Types`
(`internal/engine/native_type_typedef.go:65`-ish). The recent
ObjectType-name-rebuild bug was exactly this drift — fixed by
re-fetching from `DefStacks` after `installDef`. Two sources of
truth that desync if anyone forgets the mirror.

Status: not addressed. Medium-invasive structural fix; touches many
call sites. Worth doing but not on the immediate slice.

### 5.3 No `untype Foo`

No removal analogue to `undef foo`. To re-bind a type you start a
new registry. Tests can't easily isolate.

Status: not addressed. Trivial fix; low immediate value.


## 6. Dispatch / planning gaps

### 6.1 No predicate-type CheckMode analysis — PARTIAL (DepScalar resolved)

`defTypedHandler` now has a `r.Check.Mode && constraint.IsDepScalar()`
short-circuit: when the body's VType matches the dependent's base
type, the binding is accepted symbolically (the per-value check
stays runtime-only). Cross-base mismatches still reject. This makes
`def x:G10 15` flow through `aql check` cleanly — runtime catches
genuine value-level violations later.

Predicate-fn constraints route through `RunPredicate`'s CheckMode
short-circuit (see §3.2): under check mode the body isn't run, so
the typed binding is accepted. Symbolic execution against the body
remains a research problem (§3.4 territory).

### 6.2 `sigTypeMatches` carrier rule is implicit knowledge

`internal/engine/signature.go:230` excludes Carrier values from the
metatype-matching path. If a contributor writes a native sig with
`[TScalarType, …]` and runs it under check mode, they have to
already know carriers count as "non-metatype" — that knowledge isn't
in `LANGREF.md`, `SIGNATURES.md`, or any code-level doc.

Status: not addressed. Documentation-mostly fix.

### 6.3 Forward planner accepts `def n:T anything`

The planner type-checks the constraint slot as `TAny`; the actual
unification happens in the handler. So check-mode can't catch
wrong-type bindings before runtime even when the constraint is a
plain `Integer` (where it trivially could).

Status: not addressed. Mid effort; meaningful UX win for
`aql check` users.


## 7. Developer experience

### 7.1 No inline disjunct syntax

Every test writes `(Integer tor String)`. There's a `?:` shorthand
for record fields (`{x?:Integer}` = `tor None`) but no general
expression form like `Integer | String`.

Status: not addressed. Parser change — needs a new lexer token plus
grammar rule. Medium effort.

### 7.2 `(quote name)` for fn-shape constraints is unidiomatic

`def m:Mapper (quote double)` is the only spelling that works for
fn-shape types. `def m:Mapper double` runs `double` (looking for an
Integer arg) and errors with a confusing "no signature" message
pointing at `double`. The system understands that this is a
typed-binding context but doesn't take the help-the-user step of
suggesting `(quote double)`.

Status: not addressed. Auto-quote is a design choice (changes
semantics); a better error message is ~10 lines.

### 7.3 Predicate body boilerplate — RESOLVED

`guard` (`internal/engine/native_type_guard.go`) is the predicate-
body workhorse: `cond guard val` returns `val` when `cond` is true,
`None` otherwise. Sig is `[Any, Boolean]` in mirror order with
`BarrierPos=1` so it composes with `and`/`or` chains without
greedily consuming a chained second forward arg.

Predicate bodies shorten from

```
if cond [val] [None]
```

to

```
cond guard val
```

A transforming predicate stays just as terse:
`(x is String) guard (x upper)` returns the upper-cased string when
the input is a String, None otherwise.

Tests in `aql/test/type_guard_test.go` cover the bare cases, the
predicate-type idiom, the BarrierPos behaviour, and the typed-def
transforming-predicate path.

### 7.4 Error messages don't name the type — RESOLVED

`defTypedHandler` (`internal/engine/native_definition_def.go`) now
captures the source name when the constraint resolves through a
word lookup, and surfaces it in both error paths:

- Predicate-type failure:
  `def n: value 'e' does not satisfy predicate type Bbd`.
- Unification failure:
  `def n: value 5 does not unify with declared type G10`.

When the constraint is a built-in type used inline (no user `type`
alias), the message falls back to the rendered type form
(`Scalar/Number/Integer`). `is` returns a boolean, so it has no
error path of its own.

Tests in `aql/test/type_error_messages_test.go`.

### 7.5 `inspect` for fn-shape types is sparse — RESOLVED

`buildTypeInspection` (`internal/engine/native_type_inspect.go`) now
has cases for both fn-shape types (FnUndef) and dependent scalars
(DepScalar):

- **Function shape**: `inspect Mapper` returns
  `{kind: function_shape, signatures: [{params: […], returns: […]}, …]}`.
- **Dependent scalar**: `inspect G10` returns
  `{kind: dependent_scalar, leaf: 'Integer', lo: {kind: 'gt', value: 10}}`,
  with `hi` populated for the upper-bound or interval cases.

Tests in `aql/test/type_inspect_test.go`.

### 7.6 Two ways to express the same thing, with no nudge

`type T (Integer gt 10)` and `type T fn [n:Any Any [if (n is Integer)
and (n gt 10) [n] [None]]]` are runtime-equivalent but use different
machinery. The `DepScalar` form is checkable in principle (6.1), the
predicate form isn't. There's no lint that says "this predicate is
expressible as a DepScalar" so users gravitate toward the predicate
form because it's the more general spelling.

Status: not addressed. Needs a normaliser that recognises DepScalar
shapes inside predicate bodies — non-trivial.

### 7.7 No documentation — RESOLVED

`doc/LANGREF.md` now has dedicated sections for:

- **Type Algebra** — `tand`, `tor`, `tall`, `tany`, the laws table,
  `Never` (bottom type).
- **Dependent Types** — DepScalar shape, gt/gte/lt/lte, intervals,
  `between`, the `Type/Dependent/Dep<Leaf>` paths, unification rule.
- **Predicate Types** — None/value contract, `guard` shorthand,
  coercive predicates, the not-independently-callable rule.
- **Structural Function-Shape Types** — variance (contravariant
  inputs, covariant returns), the `(quote name)` idiom.
- **Type and Def Naming** — the case rule.
- **Type-Registry Internals** — `r.Types` vs `DefStacks` split,
  callability rules.

Discoverability gap closed for the algebraic and dependent surface.


## 8. Recommended next slice

The post-survey work has cleared the high-correctness items
(predicate sandboxing, CheckMode handling, fn-shape variance, panic
audit, fn-shape inspect). The remaining open items, in priority
order:

1. **Forward planner narrowing** (6.3). When a typed-def constraint
   resolves to a static type literal at plan time, narrow the body's
   expected type so check-mode catches mismatches before runtime.
   Mid-effort; meaningful UX win for `aql check` users.

2. **Better error for missing `(quote name)`** (7.2). ~10 lines to
   detect the typed-binding context and suggest the quote idiom.

3. **Single source of truth for type values** (5.2). Drop the
   double-write of non-fn types between `installDef` and `r.Types`.
   Touches many call sites; safest done after the §3.x and §4.x
   work has shaken out (which it now has).

4. **Inline disjunct syntax** (7.1). `Integer | String` shorthand
   for `Integer tor String`. Parser change; medium effort, big DX.

5. **Closed family of leaves** (2.1). Move the hand-maintained
   leaf-name switches in `depscalar.go` to a registry-driven table.
   Defer until adding a new Dep leaf is needed.

After those, what remains is mostly research-level (predicate-vs-
predicate compatibility, full predicate symbolic execution) or
genuinely deferable until a concrete trigger arrives.

After these, the most invasive remaining item is §5.2 (single source
of truth for type values).

Defer indefinitely without a concrete trigger: §1.4, §2.1, §2.2/§2.3
beyond `between`, §3.1, §3.4, §4.2, §4.3, §5.1, §5.3, §7.1, §7.6.


## 9. Items not in scope of this report

- The bytecode AOT plan (`docs/reports/aql-bytecode-report.md`)
  intersects every type-system decision; weaknesses there are
  cataloged in that report.
- The check-mode step-budget and recursion handling are orthogonal
  and not reviewed here.
- Object-type inheritance has its own subtleties (the
  `Object/Foo/Bar` re-naming dance in `installDef`); only mentioned
  here as the immediate cause of 5.2.


## 10. Resolved-items summary

For at-a-glance status:

| Item  | Topic                                | Status   |
|-------|--------------------------------------|----------|
| §1.1  | `tand` of non-maps → Never           | RESOLVED |
| §1.2  | Distribution of `tand` over `tor`    | RESOLVED |
| §1.3  | `DepScalar ↔ DepScalar` (intervals)  | RESOLVED |
| §1.4  | Carrier-path disjunct dedup          | open     |
| §1.5  | Subsumption + dedup at construction  | RESOLVED |
| §1.6  | Empty-fold identity                  | RESOLVED |
| §2.1  | Closed family of leaves              | open     |
| §2.2  | Single-comparison surface (`between`)| PARTIAL  |
| §2.3  | Single-`Value` bound                 | PARTIAL  |
| §2.4  | Display lossiness panic risk         | PARTIAL  |
| §3.1  | Single-arg predicate                 | open     |
| §3.2  | Predicate CheckMode story            | RESOLVED |
| §3.3  | Predicate sandboxing                 | RESOLVED |
| §3.4  | Predicate-vs-predicate compatibility | open     |
| §4.1  | Variance in `fnSigMatchesSpec`       | RESOLVED |
| §4.2  | `FnParam.Pattern` ignored            | RESOLVED |
| §4.3  | `Optional`/`BarrierPos` not checked  | PARTIAL  |
| §5.1  | Type shadowing                       | open     |
| §5.2  | Double-write for non-fn types        | open     |
| §5.3  | `untype Foo`                         | open     |
| §6.1  | Predicate-type CheckMode analysis    | PARTIAL  |
| §6.2  | `sigTypeMatches` carrier rule docs   | open     |
| §6.3  | Forward planner narrowing            | open     |
| §7.1  | Inline disjunct syntax (`|`)         | open     |
| §7.2  | `(quote name)` ergonomics            | open     |
| §7.3  | Predicate `guard` word               | RESOLVED |
| §7.4  | Name the type in errors              | RESOLVED |
| §7.5  | `inspect` for fn-shape types         | RESOLVED |
| §7.6  | DepScalar-vs-predicate nudge         | open     |
| §7.7  | LANGREF docs                         | RESOLVED |
