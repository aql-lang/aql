# Carrier-Based Static Type Checking for AQL

## Executive summary

This report evaluates the feasibility of static type checking for AQL using a **carrier-value abstract interpretation** approach:

- Use non-concrete values ("carriers") that carry only type information.
- Execute programs symbolically over carriers instead of concrete values.
- Choose matching signatures, propagate return carrier types, and merge alternatives via disjunctions.
- Explore branches and function signatures compositionally, with type refinement and error recovery.

### Bottom line

The approach is **feasible and well-aligned with AQL's current runtime architecture**, but it must be implemented as a bounded abstract interpreter (with joining/widening/memoization) to avoid path and union explosion.

---

## Theoretical foundation: abstract interpretation

This approach is **abstract interpretation** — a well-understood formal framework. The mapping between the standard theory and AQL's carrier proposal:

| Abstract Interpretation Concept | AQL Carrier Equivalent |
|---|---|
| Abstract domain | Type lattice (AQL hierarchy + disjunctions) |
| Abstract transfer functions | Signature -> return type mappings |
| Join operation | Disjunction (`A` or `B`) |
| Widening operator | Collapse to common ancestor type |
| Fixed-point iteration | Loop/recursion analysis |

This is good news — the theory is mature, and soundness/termination properties are well-characterized. The checker is not a novel invention but an instance of a proven technique applied to AQL's specific semantics.

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

AQL currently resolves calls by selecting a matching signature from pre-sorted candidates ("first match wins"). `MatchSignature` applies type matching and structural pattern checks (`Unify` for patterns) at call time.

Signature matching is already type-only. The core function `sigTypeMatches(v Value, t Type)` in `signature.go` examines only `VType`, never `Data`. A carrier value (`VType = TInteger`, `Data = nil` or a special `CarrierData`) would match identically to a real integer. The static `MatchSignature` function, `SortSignatures`, `signatureScore`, `Type.Matches()` hierarchy check, and `Unify` are all directly reusable.

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

### 6) Forward collection works over types

Forward matching already uses only type information to decide how many tokens to collect. The abstract interpreter simulates the same "try longest signature first, forward-match by type" algorithm. Since carrier values have types, the same greedy matching logic applies.

### 7) Argument equivalence simplifies analysis

Since `f a b`, `a f b`, and `a b f` all produce identical results (see CLAUDE.md: Argument Ordering), the abstract interpreter can normalize to a canonical form before matching. The result type is position-independent — the checker does not need to track permutations.

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

Flow typing falls out naturally: branch analysis with type narrowing is just abstract interpretation with path sensitivity. `if [x is Integer] [...]` narrows the carrier in the then-branch.

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

### Sources of blow-up

1. **Branch/path explosion**: each conditional multiplies states.
2. **Overload ambiguity**: broad carriers can match many signatures.
3. **Union growth**: repeated joins increase disjunct size.
4. **Optional signature combinations**: AQL already expands optional params combinatorially (`2^N` subsets).

### Why disjunction growth is naturally limited

Looking at actual AQL signatures:

- Arithmetic (`add`, `mul`, etc.) has 4-5 signatures but only 3 distinct return types: `{Integer, Decimal, String}`. Passing `Carry<Number>` through `add` yields at most `Carry<Integer|Decimal>`, not an explosion.
- Most words have 1-2 distinct return types across all their signatures.

Three mitigation strategies prevent explosion:

1. **Width cap**: Limit disjunctions to N alternatives (e.g., 8). Beyond that, widen to the nearest common ancestor. `Integer | Decimal | String | Boolean` -> `Scalar`.

2. **Subsumption**: `Integer | Number` -> `Number` (parent absorbs child). `Integer | Integer` -> `Integer`.

3. **Lazy expansion**: Only split disjunctions when the word actually dispatches differently. If all alternatives map to the same return type, no expansion occurs. Example: `add(Carry<Integer|Decimal>, Carry<Integer>)` matches `[I,I]->I` and `[D,N]->D`. Result: `Carry<Integer|Decimal>` — same width.

### Loop and recursion termination

The type lattice has finite height (bounded hierarchy depth + width-capped disjunctions). Fixed-point iteration:

- **`each`**: Analyze the body once with the element type. Result is a list of whatever the body produces. One pass.
- **`fold`**: Start with accumulator type `T`, run body, get `T'`. If `T' = T`, done. If not, iterate with `T|T'`. Bounded by lattice height.
- **Recursion**: Same fixed-point approach. For factorial, base case gives `Carry<Integer>`, recursive case applies `mul [I,I]->I`, fixed point reached in one step.

### Complexity bound

Worst case is **polynomial, not exponential**, because widening prevents unbounded disjunction growth. The lattice height is `O(depth_of_type_hierarchy * width_cap)`, which is small and constant for AQL's hierarchy.

### Can it scale?

Yes, with standard abstract-interpretation controls:

- **Widening**: cap union cardinality and collapse to ancestor types.
- **Join aggressively**: merge equivalent abstract states at control-flow joins.
- **Memoization/fixpoints**: cache `(pc, abstract stack shape, env)` states.
- **Budgeting**: cap explored signatures/paths/depth per function.
- **Summary-based interprocedural analysis**: avoid re-analyzing same bodies.
- **Timeout/fallback**: degrade to coarse types (`Any`, `Scalar`, etc.) under pressure.

Without these controls, worst-case behavior can be exponential. With them, it is bounded and practical.

---

## Comparison with other static typing strategies

| Approach | Fit for AQL | Why |
|---|---|---|
| Hindley-Milner | Poor | Assumes parametric polymorphism, no subtyping, single principal type. AQL has ad-hoc overloading, subtype hierarchy, first-match dispatch. |
| Abstract interpretation (this proposal) | Excellent | Follows AQL's own execution model. Same signature matching, same dispatch order. |
| Flow typing | Falls out naturally | Branch analysis with type narrowing is just abstract interpretation with path sensitivity. `if [x is Integer] [...]` narrows the carrier in the then-branch. |
| Gradual typing | Partial overlap | AQL's approach is stronger — it tracks precise types where known and degrades to `Any` only at escape hatches, rather than using `?` throughout. |
| Factor's stack checker | AQL goes further | Factor checks stack arity (`( a b -- c )`) but not value types. AQL's carriers track concrete types AND handle dispatch. Factor lacks ad-hoc overloading. |
| Annotation-heavy checker only | Moderate | Simpler implementation and predictable performance, but weaker precision on existing dynamic features and higher user annotation burden. |

**Best fit for AQL today:** carrier-based analysis with bounded abstract interpretation.

---

## Key gotchas and gaps

### Severe (fundamentally hard)

1. **`do` on dynamically-constructed lists.** `do` evaluates a list as code. If the list is a literal (`do [dup add]`), the checker can analyze it. If it's computed at runtime (`ops get 0 do`), the checker sees `Carry<Any>` and cannot know what code runs. Must conservatively return `Carry<Any>` or flag as uncheckable.

2. **`context get` / `context set`.** The context store is dynamically keyed. `context get x` could return anything. Without whole-program tracking of all `set` calls on all paths, the result is `Carry<Any>`. This is a fundamental escape hatch.

3. **Runtime `def` rebinding.** `def x 1` then later `def x "hello"` changes the type of `x`. Conditional defs create disjunctions. The abstract interpreter must track `def` bindings as part of its state:
   ```
   if [cond] [def x 1] [def x "hello"]
   # x : Carry<Integer|String>
   ```

### Moderate (tractable with effort)

4. **Higher-order words with non-literal bodies.** `each`, `fold`, `outer` take code bodies as lists. For literal bodies (`each [dup add]`), analysis is straightforward — analyze the body with the element type. For bodies passed from elsewhere (a function parameter that happens to be a list), the checker cannot know the stack effect.

5. **Forward collection arg-count ambiguity.** When a word has both `[Integer]` and `[Integer, String]` signatures, the engine tries longer signatures first, looks forward for a `String` match, falls back if not found. The abstract interpreter must simulate this — tractable but requires reimplementing the forward scanning logic.

6. **Module imports.** Cross-file analysis requires parsing and analyzing imported files, building module-level type environments, resolving `utils.helper` to exported types. Standard for static analyzers but adds implementation scope.

7. **Unbounded stack.** Loops can push unbounded values. Mitigation: compute the stack effect (net push/pop per iteration) rather than tracking every iteration's values.

### Low (manageable)

8. **List auto-evaluation.** The checker must track `Eval`/`Quoted` flags to know which lists will be auto-evaluated. Mirrors existing engine logic directly.

9. **Barrier positions in signatures.** The `|` syntax in forward collection creates barriers. The abstract interpreter must respect these, but the logic is purely structural.

10. **Data-dependent behavior**: type predicates and value-level logic may require path sensitivity for precision.

11. **Error modeling**: should distinguish hard unsoundness from soft recoverable diagnostics.

12. **Determinism under overload ambiguity**: static analyzer must define deterministic join/prioritization policy compatible with runtime dispatch order.

---

## The checker as a modified engine

What makes this approach particularly natural for AQL is that **the checker IS an AQL engine with a different value representation**. You're not building a separate formal system — you're running the same dispatch, same matching, same signature priority, just with carrier values instead of concrete values. The engine's existing `MatchSignature`, `sigTypeMatches`, `SortSignatures`, `Type.Matches()`, and `Unify` are directly reusable.

The abstract interpreter could be structured as a modified `engine.Run()` where:

- **Literals** push `Carry<their_type>` instead of concrete values.
- **Word execution** calls the signature matcher, reads the return type, pushes `Carry<return_type>` instead of calling the handler.
- **`if`** evaluates both branches and joins.
- **`def`** updates an abstract def-binding map.

This means the checker **automatically stays in sync with the language** as words are added — the same `NativeSig` definitions that drive runtime dispatch drive static checking.

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

## Verdict

- **Is it possible?** Yes.
- **Can it scale?** Yes, if implemented with abstract interpretation controls (join/widen/cache/budget).
- **Is exponential explosion possible?** Yes in worst-case; manageable in practice with bounded analysis. Worst case is polynomial with widening, not exponential.
- **Compared to alternatives?** This approach is the most compatible with AQL's current runtime semantics and type machinery.
- **Why is it a natural fit?** The approach scales because AQL's type lattice is bounded and shallow (unlike, say, structural types in TypeScript which can nest arbitrarily). The main escape hatches (`do` on computed lists, `context get`, dynamic `def`) are exactly the constructs you'd expect to be hard to type-check in any dynamic language — they don't invalidate the approach, they just define its boundary. For straight-line code, function calls, branching, and loops over typed data, the carrier approach gives precise results with no explosion.
