# Carrier-Based Static Type Checking for AQL

## Executive summary

This report evaluates the feasibility of static type checking for AQL using a **carrier-value abstract interpretation** approach:

- Use non-concrete values (“carriers”) that carry only type information.
- Execute programs symbolically over carriers instead of concrete values.
- Choose matching signatures, propagate return carrier types, and merge alternatives via disjunctions.
- Explore branches and function signatures compositionally, with type refinement and error recovery.

### Bottom line

The approach is **feasible and well-aligned with AQL’s current runtime architecture**, but it must be implemented as a bounded abstract interpreter (with joining/widening/memoization) to avoid path and union explosion.

---

## Scope and question

We evaluate whether the following strategy can provide static typing for AQL:

1. Introduce a dynamic type carrier value.
2. Invoke words/functions by signature only (no concrete execution), pushing return carriers.
3. Use disjunction types for ambiguous return types.
4. Evaluate all branches (`if`, etc.) and join results via disjunction.
5. Analyze each defined function/signature separately.
6. Refine carrier types from checks/guards when possible.
7. Prefer most specific types.
8. On mismatch, log error, continue, and coerce to expected type for forward progress.

---

## Current AQL properties relevant to this design

### 1) Signature-driven dispatch already exists

AQL currently resolves calls by selecting a matching signature from pre-sorted candidates (“first match wins”). `MatchSignature` applies type matching and structural pattern checks (`Unify` for patterns) at call time.  

Implication: a static analyzer can reuse essentially the same matching logic against abstract carrier stacks.

### 2) Unification and disjunction already exist as language concepts

AQL has:

- `Unify(a,b)` rules for scalar/list/map compatibility and narrowing.
- Disjunct handling (`unifyDisjunct`) for alternatives.
- `or` behavior that forms disjunctions for non-boolean operands.

Implication: the language already has the primitives needed to represent and merge uncertain static types.

### 3) Branch runtime semantics are explicit and lazy

`if` currently evaluates condition and executes **only the selected branch** at runtime. A static analysis can intentionally evaluate both branches and join stack effects, which is a standard abstract interpretation dual to lazy runtime branching.

### 4) Function return type declarations are already modeled

Function signatures include declared return types (`FnSig.Returns`), and runtime inserts/checks return markers.

Implication: static checking can enforce/verify return summaries and derive diagnostics before execution.

### 5) Dynamic evaluation constructs exist (`def` splicing, `do`)

- `def` list bodies are spliced into token stream on use.
- `do` evaluates list/map-embedded code through sub-engines.

Implication: full static certainty is impossible in general without conservative assumptions or staged analysis.

---

## Mapping proposed algorithm (a–h) to AQL

### a) Carrier value

**Feasible.** Introduce an abstract `Carrier`:

- `Type`: current abstract type (may be disjunct).
- `Origin`: source location/provenance.
- `Constraints`: optional path constraints/guards.
- `Diagnostics`: accumulated soft errors.

No concrete payload required.

### b) Signature-only invocation and return propagation

**Feasible but with caveats.**

At each call site:

1. Read abstract stack prefix/suffix requirements.
2. Determine candidate signatures that could match carrier types.
3. For each candidate, compute output stack effect as carriers.
4. Join candidate outcomes.

Caveat: uniqueness is not guaranteed when input carriers are broad/disjunctive.

### c) Disjunction for multi-type returns

**Feasible and natural.**

When multiple signatures/branches produce distinct types for same stack position, join with disjunct union. If union grows too large, widen to parent type.

### d) Evaluate all branches and join

**Feasible; expected for static analysis.**

For branch nodes (`if`, loops with conditionals, etc.):

- Evaluate all reachable branches under current abstract state.
- Join resulting abstract stacks.
- Preserve/merge diagnostics.

### e) Analyze each function signature separately

**Strongly recommended.**

Perform interprocedural summary analysis:

- Build summary per signature: input abstract pattern -> output stack effect + diagnostics.
- Memoize summaries and compute fixpoint for recursive cycles.

### f) Prune types via guards

**Feasible and high-value.**

Where AQL expressions encode type checks, narrow carrier type on true-path and complement on false-path (if representable).

### g) Prefer most specific types

**Required for precision.**

Use existing subtype hierarchy and specificity ranking. Preserve narrowest carrier type that remains sound.

### h) Error-tolerant continuation

**Feasible and practical for tooling.**

If mismatch occurs:

- Emit diagnostic with expected vs actual abstract type.
- Continue with coerced expected carrier (or joined fallback) to avoid cascade abort.

This supports IDE/lint workflows.

---

## Scalability analysis

## Sources of blow-up

1. **Branch/path explosion**: each conditional multiplies states.
2. **Overload ambiguity**: broad carriers can match many signatures.
3. **Union growth**: repeated joins increase disjunct size.
4. **Optional signature combinations**: AQL already expands optional params combinatorially (`2^N` subsets).

## Can it scale?

Yes, with standard abstract-interpretation controls:

- **Widening**: cap union cardinality and collapse to ancestor types.
- **Join aggressively**: merge equivalent abstract states at control-flow joins.
- **Memoization/fixpoints**: cache `(pc, abstract stack shape, env)` states.
- **Budgeting**: cap explored signatures/paths/depth per function.
- **Summary-based interprocedural analysis**: avoid re-analyzing same bodies.
- **Timeout/fallback**: degrade to coarse types (`Any`, `Scalar`, etc.) under pressure.

Without these controls, worst-case behavior can be exponential.

---

## Comparison with other static typing strategies

## A) Hindley–Milner style global inference

Pros:

- Elegant principal types.
- Powerful parametric inference.

Cons for AQL:

- AQL is stack/concatenative with forward collection, overload dispatch, and dynamic code evaluation; HM core assumptions map imperfectly.

## B) Annotation-heavy checker only

Pros:

- Simpler implementation.
- Predictable performance.

Cons:

- Weaker precision on existing dynamic features.
- Higher user annotation burden.

## C) Carrier-based abstract interpretation (proposed)

Pros:

- Closest to existing engine semantics.
- Leverages current signatures/unification/disjuncts.
- Can provide useful diagnostics without language redesign.

Cons:

- Requires careful state-control engineering.
- Precision/performance trade-offs must be tuned.

**Best fit for AQL today:** carrier-based analysis with bounded abstract interpretation.

---

## Key gotchas and gaps

1. **Dynamic code and late binding** (`def`, `do`, module/runtime-defined words): may force conservative unknowns.
2. **Stack arity uncertainty**: some branches/words can leave varying counts of values; analyzer must model shape uncertainty.
3. **Higher-order/callback words**: need callback summaries or conservative assumptions.
4. **Data-dependent behavior**: type predicates and value-level logic may require path sensitivity for precision.
5. **Error modeling**: should distinguish hard unsoundness from soft recoverable diagnostics.
6. **Determinism under overload ambiguity**: static analyzer must define deterministic join/prioritization policy compatible with runtime dispatch order.

---

## Recommended implementation plan

### Phase 1 — Core abstract engine

- Add `Carrier` abstraction and abstract stack.
- Reuse signature matching logic over abstract types.
- Produce diagnostics on mismatches.

### Phase 2 — Control flow and unions

- Add branch splitting/joining.
- Implement disjunct union normalization + widening caps.

### Phase 3 — Function summaries

- Analyze `def/fn` per signature.
- Memoize summaries and compute fixpoints for recursion.

### Phase 4 — Refinement and precision

- Add guard-based narrowing/pruning.
- Add path-sensitive state where beneficial.

### Phase 5 — Tooling mode

- Non-fatal error recovery (continue with expected type).
- Rich diagnostics with provenance and suggested fixes.

---

## Practical answer to the main question

- **Is it possible?** Yes.
- **Can it scale?** Yes, if implemented with abstract interpretation controls (join/widen/cache/budget).
- **Is exponential explosion possible?** Yes in worst-case; manageable in practice with bounded analysis.
- **Compared to alternatives?** This approach is the most compatible with AQL’s current runtime semantics and type machinery.

