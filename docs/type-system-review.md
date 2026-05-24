# Go Unifier Robustness Review (AQL)

This revision focuses only on the **Go implementation** of the unifier (`eng/go/unify.go`) and practical ways to make it more robust.

## Current strengths (Go unifier)

The current algorithm already has a sensible staged flow:
- deep word resolution first,
- early handling of disjuncts / `Never` / `None` / `Any`,
- structural branches (list/map/table/record/typed-list),
- dependent scalars,
- function-signature matching,
- literal-vs-concrete and subtype fallback.

This ordering is readable and aligns well with AQL’s runtime type model.

## Robustness risks in the current shape

1. **Order-dependent semantics are implicit**
   - `Unify` is a long branch chain where behavior depends on check order.
   - Adding a new special case can silently change existing outcomes.

2. **Type/category detection is distributed**
   - Logic for “what kind of value/type is this?” is repeated inline (`Data == nil`, table detection, dep-scalar checks, etc.).
   - This increases the chance of inconsistent handling across branches.

3. **Low observability for failed unifications**
   - `Unify` returns only `(Value, bool)`.
   - For regressions or edge cases, there is no structured reason path.

4. **Law drift risk**
   - Core properties (idempotence, expected commutativity for symmetric cases, monotonic narrowing) are not enforced as first-class invariants.

5. **Branch interaction complexity**
   - Disjunct × dep-scalar × typed-list × table interactions are hard to reason about by inspection.
   - Hardest bugs here are usually “valid alone, wrong in composition.”

## Concrete improvements to make the Go unifier more robust

### 1) Add a rule-engine front layer (classification + dispatch)

Refactor `Unify` into:
- `classify(Value) -> Kind` (TypeLiteral, ConcreteScalar, DepScalar, ListLiteral, TypedList, TableType, MapLiteral, FnSigConstraint, etc.)
- a dispatch matrix keyed by `(KindA, KindB)`
- small handler functions per pair.

Why this helps:
- makes precedence explicit,
- prevents accidental branch shadowing,
- lets reviewers reason locally about each pairwise rule.

### 2) Introduce `UnifyDetailed` alongside existing API

Keep `Unify(a,b) (Value,bool)` for compatibility, but implement:

- `UnifyDetailed(a,b) (Value, UnifyResult)`
- where `UnifyResult` contains
  - `OK bool`
  - `Reason enum` (e.g., `ReasonNoneMismatch`, `ReasonRecordFieldConflict`, `ReasonDepScalarUnsat`)
  - `Path []string` (e.g., map/list position trail)
  - optional `LeftType`, `RightType` snapshot.

Then have `Unify` call `UnifyDetailed` and drop details.

Why this helps:
- better debugging and test diagnostics,
- easier CI triage when a law/property test fails.

### 3) Encode unification laws as property tests

Add property tests (fuzz + deterministic corpus) for invariants such as:
- **Idempotence**: `unify(x,x) == x` when valid.
- **Symmetry** on symmetric domains: if both succeed and no directional preference rule applies, `unify(a,b)` equivalent to `unify(b,a)`.
- **Narrowing monotonicity**: result type should be <= both inputs in lattice terms (except explicit widening rules).
- **None/Never laws**: `Never` only with `Never`; `None` only with `None`.
- **Disjunct soundness**: result must satisfy both original constraints.

Why this helps:
- catches subtle order regressions that example tests miss.

### 4) Add pairwise interaction test matrix

Create a table-driven suite that crosses major categories:
- {Type literal, Concrete, DepScalar, Disjunct, List, TypedList, Table, Map, FnSig} × same set.

For each pair, assert one of:
- success + expected result class,
- failure + expected reason code.

Why this helps:
- makes unsupported/forbidden combinations explicit and reviewable.

### 5) Normalize helper predicates and denoted-type extraction

Centralize recurring logic into helpers:
- `isTypeLiteral(v)`
- `denotedType(v)`
- `isConcrete(v)`
- `kindOfList(v)` (plain/typed/table)
- `constraintKind(v)` (dep-scalar/disjunct/fnsig)

Why this helps:
- single source of truth for sensitive checks,
- reduces inconsistent edge handling.

### 6) Add recursion/size guards

For deep structural values (large nested lists/maps/records), add:
- depth counter,
- node budget,
- deterministic fail reason (`ReasonBudgetExceeded`) when limits hit.

Why this helps:
- protects against pathological inputs and accidental worst-case blowups.

### 7) Canonicalize before structural unification where safe

For map/record-like unification, canonicalize key ordering and shape before value-level recursion.

Why this helps:
- avoids incidental ordering bugs,
- stabilizes behavior across equivalent representations.

### 8) Make disjunct selection deterministic and optimal

Current “first match wins” can be brittle if alternative ordering changes. Improve by:
- evaluating all satisfiable alternatives,
- selecting most-specific successful result (specificity score / lattice depth / tie-break rules),
- documenting deterministic tie-break.

Why this helps:
- removes accidental dependency on branch order in disjunct construction.

### 9) Isolate function-signature unification policy

Move function-signature unification into a dedicated policy object/module with explicit variance knobs and tests:
- argument variance policy,
- return variance policy,
- overload coverage policy.

Why this helps:
- keeps `Unify` core simpler,
- makes future function-type evolution safer.

### 10) Add differential regression harness for unifier changes

Before/after refactors, run golden fixtures containing:
- representative successful unifications,
- known-failure cases,
- historically buggy compositions.

Require unchanged outcome (or intentional reviewed delta) for merge.

## Suggested implementation sequence (low-risk)

1. Add `UnifyDetailed` + reason codes without changing semantics.
2. Add helper predicate/type-extraction layer.
3. Add property tests + pairwise matrix tests.
4. Refactor branch chain into classified dispatch while keeping behavior identical.
5. Upgrade disjunct selection and function-signature policy with explicit test-gated deltas.

## Bottom line

The Go unifier is already well-structured for a dynamic runtime, but robustness will improve significantly by making precedence explicit, emitting machine-readable failure reasons, and enforcing algebraic invariants with property-based tests. Those changes reduce regression risk while preserving the existing runtime model.
