# The Quiet Colossus and AQL: A Comparative Analysis

## Report: What Ada Got Right, What AQL Can Learn, and a Plan for Convergence

---

## What "The Quiet Colossus" Argues

The essay makes a single sustained argument: Ada, designed under DoD
contract in 1979 and standardized in 1983, anticipated — with unusual
precision — the safety features that every modern language is now trying
to acquire. The industry spent forty years independently rediscovering
Ada's design decisions while describing Ada itself as irrelevant.

The essay traces convergence across seven axes:

1. **Packages** — spec/body separation with compiler-enforced privacy
2. **Range-constrained types** — types that encode domain meaning, not just machine representation
3. **Discriminated unions** — sum types with compiler-enforced field access
4. **Generics** — parametric polymorphism with formal type parameters
5. **Concurrency** — task-level message passing and protected objects (CSP lineage)
6. **Contracts** — preconditions, postconditions, type invariants (Ada 2012)
7. **Null safety** — not-null access types with compile-time enforcement (Ada 2005)

The essay's key insight is not that modern languages copied Ada, but that
they converged on the same solutions because the problems are the same
problems and the good solutions are the good solutions. Ada identified
them first because it was designed for a context where software failures
had already killed people.

---

## How AQL Compares

### Summary matrix

| Ada Principle | Ada Mechanism | AQL Current State | Gap |
|---|---|---|---|
| Spec/body separation | Package spec + body | Module with export maps | No compile-time privacy enforcement |
| Range-constrained types | `type Age is range 0..150` | Type hierarchy (Integer, String, etc.) | No range or value constraints on types |
| Discriminated unions | Discriminated records | Disjunct types (`String\|Integer`) | No pattern matching, no exhaustiveness check |
| Generics | Generic formal parameters | Ad-hoc overloading (multi-signature words) | No type parameterization |
| Concurrency | Tasks + rendezvous + protected objects | `await` with modes (all/first/any) + `timeout` | No shared-state protection, no barriers |
| Contracts | `Pre`, `Post`, type invariants | Type signatures on `fn` definitions | No explicit contract syntax |
| Null safety | `not null` access types (Ada 2005) | `TNone` type, disjunctions with None | No non-null-by-default, no enforcement |
| Exception handling | Declared exceptions, scoped handlers | `error` word, `AqlError` values | No automatic propagation, no exception types |
| Static verification | SPARK formal proof | Planned (carrier-based abstract interpretation) | Not implemented |
| Readability | Verbose, explicit syntax | Concatenative (compact, symmetric mirror) | Argument order is implicit |

### Where AQL already aligns with Ada's philosophy

**Strong typing with semantic hierarchy.** AQL's type system
(`types.go:18-77`) uses hierarchical paths: `Scalar/Number/Integer`,
`Scalar/Time/Duration/CalDuration`, `Object/Record`. This is closer to
Ada's "types encode meaning" philosophy than most dynamic languages.
An Integer is not a String is not a Boolean — the type hierarchy
prevents silent coercion. Signature matching (`sigTypeMatches` in
`signature.go:161-181`) uses this hierarchy for dispatch, and the
subtype relationship means `Integer` matches where `Number` is expected
but not vice versa.

**Module system with exports.** AQL's module system
(`native_module_module.go`) creates isolated sub-registries with
explicit export maps. `export Charops {to-up:to-up to-down:to-down}`
declares exactly what the module provides. This is a form of interface
declaration, even if it lacks Ada's compiler-enforced separation.

**Structured error handling.** AQL's `error` word
(`native_control_error.go`) provides scoped error recovery: the handler
receives the error value on the stack, can inspect it, and return a
replacement. `AqlError` (`aql_error.go`) carries code, detail, source
position, and hints — structured, not just a string.

**Signature-driven dispatch.** AQL's multi-signature words (e.g., `add`
has 6 signatures for Integer, Decimal, String, Date, DateTime, Instant)
are a form of ad-hoc polymorphism where the type system drives behavior.
This is closer to Ada's overloading than to duck typing.

### Where AQL diverges from Ada's philosophy

**Runtime over compile-time.** Ada's central principle is that errors
should be caught before the program runs. AQL currently has no static
checker — all type checking happens at runtime via signature matching.
The carrier-based static type checker
(`doc/CARRIER-STATIC-TYPECHECK-REPORT.md`) is designed but unimplemented.

**Flexibility over constraint.** Ada's type system says "this value MUST
be in range 0..150." AQL's type system says "this value IS an Integer" —
the domain constraint is not expressible. An age of -5 or 999 passes
type checking because `Integer` has no range.

**Concatenative ambiguity.** Ada chose verbosity for readability:
`procedure Sort (A : in out Array_Type)` is self-documenting. AQL's
`3 10 sub` vs `10 sub 3` vs `sub 10 3` all produce different results
depending on the symmetric mirror pattern. The terseness is powerful for
composition but hostile to reading by non-authors — precisely the
failure mode the Steelman document was designed to prevent.

---

## Recommendations

The essay's core lesson is: the problems are old, the good solutions are
known, and ignoring them has costs. AQL should adopt Ada's proven
solutions where they fit a concatenative query language — not by becoming
Ada, but by learning from the forty years of convergence the essay
documents.

### R1: Range-constrained types

**Ada feature:** `type Age is range 0..150`

**AQL design:** Extend the type system with a `constrain` word or
inline syntax on record fields:

```aql
record Person {
  name: String
  age: Integer {min: 0, max: 150}
  email: String {match: ".*@.*"}
}
```

Constraints would be checked at `make` time (when instantiating a
record) and optionally by the static type checker. The constraint
metadata lives on the type definition, not on each value.

**Implementation:** Add a `Constraints` field to record field types in
`native_type_record.go`. The `make` handler (`native_type_make.go`)
already validates field types — extend it to check constraint predicates.
Pattern constraints (`match`) reuse the regex infrastructure recommended
in the batteries-included report.

### R2: Contracts on function signatures

**Ada feature:** `Pre => X > 0, Post => Result > Input`

**AQL design:** Extend `fn` syntax to accept optional contract lists:

```aql
def divide fn [
  [a:Number b:Number] {pre: [b neq 0], post: [is Number]}
  [Number]
  [a b div]
]
```

The `pre` list is evaluated with the named arguments on the stack before
the body runs. The `post` list is evaluated with the result on the stack
after the body runs. Violations produce contract-specific errors:
`[aql/contract_error]: precondition failed: b neq 0`.

**Implementation:** Extend `FnSig` (`value.go:171-177`) with
`Pre []Value` and `Post []Value` fields. The `execFnDefSig` handler
(`engine.go:1399-1529`) already creates sub-engines for fn bodies —
insert pre-check before and post-check after body execution.

### R3: Pattern matching on disjunctions

**Ada feature:** Discriminated records with `case` over discriminant
values, compiler-enforced exhaustiveness.

**AQL design:** Add a `match` word (distinct from string `match`) for
type-based dispatch over disjunct values:

```aql
def describe fn [[shape: Circle|Rectangle] [String] [
  shape match [
    Circle    [shape get "radius" convert String " radius circle" add]
    Rectangle [shape get "width" convert String " wide rectangle" add]
  ]
]]
```

The `match` word takes a value and a list of type-body pairs, dispatches
to the first matching type, and produces a warning (or error in strict
mode) if the cases do not cover all alternatives in the disjunction.

**Implementation:** New `native_control_match.go`. The dispatch logic
reuses `sigTypeMatches` from `signature.go:161-181` to test each case.
Exhaustiveness checking compares the case types against the
`DisjunctInfo.Alternatives` list.

### R4: Static type checking (carrier-based)

**Ada feature:** Comprehensive compile-time verification via SPARK.

**AQL status:** The carrier-based abstract interpreter is fully designed
(`doc/CARRIER-STATIC-TYPECHECK-REPORT.md`) with a phased plan. The
essay's argument — that static verification is not optional for reliable
software — reinforces the urgency.

**Priority change:** Move Phase 0 (return type annotations on all 126
`NativeSig` definitions) from "prerequisite" to "immediate." Every
subsequent recommendation (contracts, range constraints, exhaustiveness)
depends on the static checker. The annotations are mechanical and can be
added incrementally.

### R5: Non-null by default

**Ada feature:** `not null` access types (Ada 2005) with compile-time
enforcement.

**AQL design:** In strict mode (opt-in initially, default eventually),
function parameters that do not include `None` in their type are
non-nullable. A value of `None` passed where `String` is expected
produces a type error rather than silently propagating.

```aql
# Current: None flows through silently
def greet fn [[name:String] [String] ["Hello " name add]]
None greet   # => "Hello None" (silent coercion)

# With non-null enforcement:
None greet   # => [aql/type_error]: expected String, got None
```

**Implementation:** The signature matcher (`sigTypeMatches` in
`signature.go:161-181`) already checks type compatibility. Add a check:
if the signature arg type does not include `TNone` (via disjunction) and
the value is `TNone`, reject the match. This is a one-line addition to
`sigTypeMatches` but requires auditing existing code for intentional
None-passing patterns.

### R6: Module specs, partial patching, and semver via unification

#### R6a: Module spec declarations

**Ada feature:** Package specifications as compiler-enforced contracts.

**AQL design:** Allow modules to declare their interface separately from
implementation:

```aql
module spec MathLib {
  add: fn [[Number Number] [Number]]
  sub: fn [[Number Number] [Number]]
}

module MathLib [
  def add fn [[a:Number b:Number] [Number] [a b add]]
  def sub fn [[a:Number b:Number] [Number] [a b sub]]
  export MathLib {add:add sub:sub}
]
```

The `spec` declaration lists the expected exports with their signatures.
The module body is checked against the spec at load time — if an export
is missing or has an incompatible signature, the import fails with a
clear error. This is not Ada's full spec/body separation (the spec
doesn't replace the body), but it provides the contract benefit:
consumers can depend on the spec without reading the implementation.

**Implementation:** New word `spec` that creates a `ModuleSpecInfo`
value. The `import` handler (`native_module_module.go:187-235`) checks
the imported module's exports against any spec registered for that
module name.

#### R6b: Partial spec patching (the "no-fork" principle)

In the Node.js ecosystem, when a dependency has a bug, the consumer's
options are poor: wait for a fix, fork the entire repository, or use
`patch-package` to apply post-install diffs to `node_modules`. All
three options create maintenance burden. Ada's answer was to make
interfaces so precise that implementations could be swapped without
consumer impact. AQL can go further: allow consumers to **patch
individual exports** of a module without replacing the entire module.

**How AQL's mechanics already support this:**

Module exports are installed via `installExports` (`native_module_module.go:518-532`),
which pushes an `*OrderedMap` onto `DefStacks[exportName]`. Dot notation
(`math.sin`) resolves by looking up `DefStacks["math"]` to get the map,
then calling `map.Get("sin")` on the concrete `*OrderedMap`
(`native_accessor_dotr.go:37-64`). Currently, `def` shadows the entire
export map — you cannot `def math.sin` to override a single field.

**Proposed `patch` word:**

```aql
"aql:math" import

# math.sin has a bug in edge case. Patch it locally:
patch math {
  sin: fn [[x:Number] [Number] [
    # fixed implementation
    x math.sin   # can still call original via explicit path
  ]]
}

# Now math.sin uses the patched version.
# All other math exports (cos, tan, etc.) are unchanged.
# unpatch math  — restores the original.
```

**Mechanics:** `patch` creates a new `*OrderedMap` that is a shallow
copy of the current export map, with the specified keys replaced. It
pushes this new map onto `DefStacks["math"]`, shadowing the original.
`unpatch` calls `uninstallDef` (`native_definition_def.go:235-264`)
which pops the stack, revealing the original. Because DefStacks is a
stack, patches compose: you can patch a patch, and unpatching restores
each layer in order.

**The spec as a safety net:** If a module spec exists (R6a), the `patch`
word checks that the replacement value has a compatible type with the
spec's declaration for that key. You can fix the implementation but you
cannot change the interface. This is Ada's principle applied to
dependency management: the spec is the contract, and patches must honor
the contract.

**The mutable export problem:** If a module exports a mutable reference
(a Store value or a `var`-bound value), patching the export map does
not patch the underlying mutable state. The export map is immutable —
it holds Values, which may be references to mutable structures, but
the map itself is a snapshot. This means:

- **Exporting a function** (`sin: fn [...]`): safe to patch. The patched
  function replaces the original in the export map. Callers get the new
  function.
- **Exporting a store reference** (`cache: context get "cache"`): patching
  the export map replaces the reference, not the store. Code that already
  holds a direct reference to the old store is unaffected. This is
  correct behavior — you cannot retroactively patch references that have
  escaped — but it means mutable exports should be avoided in patchable
  modules. The module spec can enforce this by declaring which exports
  are functions (patchable) vs values (frozen at import time).

**Comparison to other ecosystems:**

| Ecosystem | Patching mechanism | Granularity | Safety |
|---|---|---|---|
| Node.js | `patch-package` (post-install file diff) | File-level | None — raw text patches |
| Python | Monkey-patching (`module.fn = new_fn`) | Attribute-level | None — no type checking |
| Ruby | `Module#prepend`, refinements | Method-level | Scoped refinements are safe-ish |
| Ada | Replace package body, keep spec | Entire body | Spec-enforced contract |
| **AQL (proposed)** | `patch` word with spec checking | Export-field-level | Spec-enforced type compatibility |

AQL's approach is more granular than Ada's (field-level vs body-level)
and safer than Node.js/Python (spec-checked vs unchecked). The key
insight is that AQL's DefStacks shadowing mechanism already provides
the layered override semantics — `patch` just needs to make them
ergonomic and spec-aware.

#### R6c: Semver enforcement via unification

Ada's annex certification model — where a compiler's conformance to
each annex is independently testable against a formal standard — suggests
a question: can AQL automatically determine whether a new version of a
module is backward-compatible with the previous version?

**The answer is yes, using AQL's existing unification machinery, with
one extension for function signature comparison.**

**How it works:**

A module's exported `*OrderedMap` IS its spec. Given two versions of a
module, backward compatibility reduces to: does the new export map
contain everything the old export map contained, with compatible types?

This is exactly what `openUnifyMap` (`unify.go:708-725`) checks:

```go
func openUnifyMap(pattern, candidate Value) bool {
    pMap := pattern.AsMap()   // v1 exports (the "contract")
    cMap := candidate.AsMap() // v2 exports (the "candidate")
    for _, key := range pMap.Keys() {
        pVal, _ := pMap.Get(key)
        cVal, ok := cMap.Get(key)
        if !ok { return false }             // v2 removed an export
        if _, uOk := Unify(pVal, cVal); !uOk {
            return false                     // values incompatible
        }
    }
    return true  // v2 has everything v1 has, with compatible values
}
```

- **v2 adds new exports:** `openUnifyMap` allows extra keys in the
  candidate → minor version bump (backward compatible).
- **v2 removes an export:** key missing → major version bump (breaking).
- **v2 changes an export's value:** `Unify(v1val, v2val)` fails →
  major version bump (breaking).

**The function signature gap:** Currently, `Unify` on two `FnDefInfo`
values falls through to string comparison (`unify.go:639`), which is
not semantically meaningful. Two functions with identical signatures but
different implementations would fail to unify.

**Proposed extension — `unifyFnSig`:** Add a unification rule for
`TFnDef`/`TFunction` values that compares signature compatibility
rather than identity:

```go
// In Unify(), before the default fallback:
if a.VType.Equal(TFnDef) && b.VType.Equal(TFnDef) {
    return unifyFnSig(a, b)
}
```

`unifyFnSig` checks:
1. **Same number of overloads** (or v2 has more — additional overloads
   are additive, not breaking).
2. **For each v1 overload, a compatible v2 overload exists:**
   - Same number of parameters.
   - Each v2 parameter type is a supertype of (or equal to) the
     corresponding v1 parameter type. Widening parameters
     (`Integer` → `Number`) is backward-compatible: v2 accepts
     everything v1 accepted plus more. This uses the existing
     `Type.IsSubtypeOf()` check (`unify.go:108-116`).
   - The v2 return type is a subtype of (or equal to) the v1 return
     type. Narrowing returns (`Number` → `Integer`) is backward-
     compatible: v2 guarantees more than v1 promised. (This is
     Liskov's covariance/contravariance principle.)
3. **Forward precedence unchanged** — changing a word from forward to
   stack-only is a breaking change.

**Semver determination:**

| Change | Compatible? | Semver |
|---|---|---|
| New export added | Yes | Minor |
| Export removed | No | Major |
| Function: parameter type widened (`Integer` → `Number`) | Yes | Minor |
| Function: parameter type narrowed (`Number` → `Integer`) | No | Major |
| Function: return type narrowed (`Number` → `Integer`) | Yes | Minor |
| Function: return type widened (`Integer` → `Number`) | No | Major |
| Function: new overload added | Yes | Minor |
| Function: overload removed | No | Major |
| Function: forward precedence changed | No | Major |
| Value export: same value | Yes | Patch |
| Value export: different value, same type | Yes | Minor |
| Value export: different type | No | Major |

**CLI integration:**

```
aql compat v1-exports.aql v2-exports.aql
# Output:
#   Compatible: yes
#   Semver: minor (2 exports added, 0 removed, 0 changed)
#   Added: sqrt, cbrt
#   Unchanged: add, sub, mul, div, mod, pow
```

Or as an AQL word within the `aql:ai` or `aql:pkg` module:

```aql
"aql:pkg" import
v1 pkg.compat v2   # => {compatible: true, semver: "minor", added: [...]}
```

**Why this matters:** The npm ecosystem's semver violations are a
major source of breakage. Developers must manually assess whether a
new version is compatible, and they frequently get it wrong. AQL can
make semver a **computed property** of the export diff, not a human
judgment call. Combined with the spec system (R6a), this creates a
chain: the spec defines the contract, the patch system (R6b) allows
safe local fixes, and the compat checker (R6c) ensures that upstream
changes are correctly versioned.

**Implementation:** Add `unifyFnSig` to `unify.go`. Create a
`compat` word in a new `aql:pkg` native module that loads two export
maps, runs `openUnifyMap` with the extended function unification, and
reports the semver classification. The `Returns []Type` annotations
on `NativeSig` (Phase 1 prerequisite from the static checker plan)
are required for return type comparison.

### R7: Explicit argument naming in signatures

**Ada feature:** Named parameters: `Sort(A => My_Array, Order => Ascending)`

**AQL design:** AQL's concatenative syntax makes positional arguments
inherent. However, for words with 3+ arguments, the symmetric mirror
pattern makes intent unclear. Options maps already provide named
arguments for complex words (`split "," "hello,world" {cs: "sensitive"}`).

**Recommendation:** For user-defined functions, encourage the named-parameter
pattern that `fn` already supports:

```aql
def transfer fn [[from:String to:String amount:Number] [Map] [
  {from: from, to: to, amount: amount, status: "ok"}
]]
```

No language change needed — this is a documentation and convention
recommendation. The `aql doc` command (designed in `doc/AQL-DOC-DESIGN.md`)
should display parameter names prominently.

---

## Implementation Plan

### Phase 1: Foundation (enables everything else)

| Task | Files | Effort | Dependency |
|---|---|---|---|
| Add `Returns []Type` to `NativeSig` | `nativefunc.go`, all 76 `native_*.go` files | Medium | None |
| Annotate return types on all 126 signatures | All `native_*.go` files | Medium (mechanical) | NativeSig change |
| Non-null checking in `sigTypeMatches` | `signature.go:161-181` | Small | None |
| Add `Constraints` map to record field types | `native_type_record.go`, `native_type_make.go` | Small | None |

### Phase 2: Contracts and matching

| Task | Files | Effort | Dependency |
|---|---|---|---|
| Add `Pre`/`Post` fields to `FnSig` | `value.go:171-177` | Small | None |
| Contract checking in `execFnDefSig` | `engine.go:1399-1529` | Medium | FnSig change |
| `match` word for type dispatch | New `native_control_match.go` | Medium | None |
| Exhaustiveness warnings | `native_control_match.go` | Small | match word |
| Range constraints on record fields | `native_type_make.go` | Medium | Constraints field |

### Phase 3: Static checker (carrier-based)

| Task | Files | Effort | Dependency |
|---|---|---|---|
| Core carrier abstraction + abstract stack | New `typecheck/` package | Large | Return type annotations |
| Signature matching over carriers | Reuse `signature.go` | Medium | Carrier abstraction |
| Branch splitting/joining (`if`) | `typecheck/` | Medium | Core carrier |
| Disjunction normalization + widening | `typecheck/` | Medium | Branch handling |
| Function summaries + fixpoints | `typecheck/` | Large | Widening |

### Phase 4: Module specs and tooling

| Task | Files | Effort | Dependency |
|---|---|---|---|
| Module `spec` declarations | New word + `ModuleSpecInfo` type | Medium | None |
| Spec-vs-export checking in `import` | `native_module_module.go` | Medium | Spec declarations |
| `aql doc` command | `cmd/aql/doc.go`, `help/category.go` | Medium | None |
| Integrate static checker into `aql check` CLI | `cmd/aql/check.go` | Medium | Static checker |

---

## The Deeper Lesson

The essay's most powerful observation is not about any specific feature.
It is this: *Ada's successes are invisible because they are successes.
The languages that failed visibly generated the discourse. Reliable
software does not generate conference talks.*

AQL is a young language. It has the opportunity to learn from Ada's
forty years of proven design without carrying Ada's syntactic baggage or
government-procurement reputation. The specific recommendations above —
range constraints, contracts, pattern matching, non-null defaults,
static checking — are not Ada-specific ideas. They are the ideas that
every modern language is converging toward. The essay simply documents
that convergence with unusual clarity and traces its origins with unusual
honesty.

The goal is not to make AQL into Ada. The goal is to make AQL into a
language whose successes are invisible — because the programs it runs
are correct, and correct programs do not generate incident reports.

---

## Key files referenced

| File | Relevance |
|---|---|
| `aql/internal/engine/types.go` | Type hierarchy, subtype relationships |
| `aql/internal/engine/signature.go:161-181` | `sigTypeMatches` — where null checking would go |
| `aql/internal/engine/value.go:171-177` | `FnSig` — where contracts would go |
| `aql/internal/engine/engine.go:1399-1529` | `execFnDefSig` — where contract checking would run |
| `aql/internal/engine/nativefunc.go:12-37` | `NativeSig` — needs `Returns []Type` field |
| `aql/internal/engine/native_type_record.go` | Record types — needs constraint support |
| `aql/internal/engine/native_type_make.go` | Record instantiation — needs constraint validation |
| `aql/internal/engine/unify.go:32-38` | Disjunct unification — relevant to pattern matching |
| `aql/internal/engine/native_module_module.go` | Module system — needs spec checking |
| `aql/doc/CARRIER-STATIC-TYPECHECK-REPORT.md` | Static type checker design |
| `aql/doc/BATTERIES-INCLUDED-REPORT.md` | Standard library recommendations (regex needed for constraints) |
