# Case exhaustiveness — static coverage checking for `case`

Status: landed (July 2026). Maintainer decision: hard errors.

## The rule

`aql check` requires a `case`'s clauses to be **exhaustive** over the
scrutinee's static type. A default-less `case` whose clause matches do
not provably cover the scrutinee is an **error-severity** finding,
`case_not_exhaustive`, which gates `aql check`, refuses `aql run` at
preflight, and refuses compilation (it is deliberately NOT a
`RuntimeMirror`: no-match is not a runtime error — the runtime keeps the
spec-pinned produce-nothing behaviour — so the finding is a model-level
judgement like any other type error).

The trailing default is **not required when the type disjunctions are
met**: the clauses cover

- every alternative of a declared union (`def IS (Integer tor String)`,
  `x:IS`) — including through nested unions and a union type used
  directly in match position;
- every member of an enum (`enum`-style atom unions);
- Boolean's `true` **and** `false` (the opaque `Scalar/Boolean` leaf is
  synthetically decomposed — it is not a `true tor false` disjunction in
  the lattice);
- the `none` alternative of an optional (`Integer tor none`) via a
  `none` clause;
- a plain type via a clause naming it or an ancestor (`Integer` under
  `Number`);
- a **concrete** scrutinee via a clause that provably matches the value
  (`case 2 [1 'a' 2 'b']` is exhaustive; `case 9 [1 'a' 2 'b']` is the
  error).

## Soundness direction

Coverage is proven in the sound direction only — a clause is credited
with covering an alternative only when every runtime inhabitant of the
alternative provably unifies with the match (the same `UnifyR` relation
`caseClauses` applies at runtime):

- a type-literal match covers an alternative by **lattice containment**
  (`alt.ConformsTo(match)`), with named unions/enums expanded through
  `UnionCarrierForType`;
- a concrete match covers a concrete alternative by `UnifyR` itself, so
  numeric-leaf exactness (`2.0` vs `2`) is decided identically to the
  runtime;
- **opaque matches prove nothing**: code-body predicates (`[gt 3]`),
  paren expressions (runtime-evaluated generic instantiations),
  unresolvable words, and predicate-refinement types in the covering
  position (a `DepScalar` body only covers via the concrete-value
  membership check). A total predicate pair (`[lt 3]`/`[gte 3]`) still
  demands a default — the Rust match-guard rule. The closed-form
  DepScalar complement (`NegateType`) is a compatible later precision
  upgrade.
- a bare-refine newtype match does NOT statically cover its base
  scrutinee, even though `UnifyR` admits base values at runtime
  (case.tsv §2) — crediting it would be sound, but the conservative
  choice keeps the coverage relation purely lattice-shaped; revisit if
  it bites.

Consequently "cannot prove exhaustive" findings can be conservative
(they demand a default the runtime might never need), but the checker
never wrongly *proves* a case exhaustive.

## Opt-outs and stability

- A **gradual `dynamic(T)` scrutinee skips the check** — `x:Any` params
  opt out of static typing, so the spec-pinned no-match/no-default
  produce-nothing behaviour remains reachable (and spec-pinned) through
  a dynamically-typed scrutinee (case.tsv §1).
- A **concrete scrutinee inside a fn body is skipped**
  (`CheckState.FnBodyDepth > 0`): `AnalyseFnBody` re-runs bodies per
  call shape with the actual argument bound (`f 9` re-analyses with
  `x=9` even for an `:Any` param), so a value-level finding there
  describes one call, not the code. Declared type domains (carriers)
  are checked everywhere; under-coverage is caught by the
  construction-time generalized analysis, and a narrowed call-shape
  domain can only shrink the uncovered set (coverage is monotone).
- Findings dedupe by **code+position** — per-call-shape re-analysis
  would otherwise re-emit with narrowed details; the first
  (generalized) finding is authoritative.
- The pass runs in `caseReturnsFn` BEFORE the plain-check /
  compile-desugar paths diverge, so both passes report identical
  findings (the same-diagnostics contract, eng/go/CLAUDE.md).

## The advisory duals (info, non-gating)

The same coverage computation yields two advisories, per the
`redundant_guard` precedent ("gate on wrongness, advise on smell"):

- `case_unreachable_clause` — a clause an earlier clause fully subsumes
  (`Integer` after `Number`, `5` after `Integer`, a duplicate literal).
  Deliberately clause-vs-clause only — a domain-based rule would be
  unstable under call-shape narrowing.
- `case_redundant_default` — a trailing default made dead by full
  coverage. Emitted only over a DECLARED union domain
  (`DisjunctInfo.Declared`), the one alternative set that is stable and
  author-intent-backed.

## What changed where

- `lang/go/native/case_exhaustive.go` — the coverage pass.
- `lang/go/native/conditional.go` — the `caseReturnsFn` hook (and a
  stale header fix: the compile desugar never required a trailing
  default; only the code-body-scrutinee sub-path does).
- `eng/go/registry.go` — severity classifications.
- `lang/spec/case.tsv` — the no-match/no-default row now dispatches on
  an `:Any` param (the statically-typed spelling is a check error); new
  §6 pins the check-clean exhaustive shapes as accuracy canaries.
- `test/go/langspec/check_run_fp_test.go` — pin 104 → 182: every
  generated default-less `case` that runs clean is now a SANCTIONED
  check-vs-run divergence (178 of 182 divergent programs are this
  class; the other 4 are the pre-existing `if` dead-branch residue).
- `lang/go/test/case_exhaustive_check_test.go` — positive/negative
  pins for every domain shape, the opt-outs, severities, and dedupe.

## Result-typing note

`caseBranchJoin` (plain check) has always omitted a None contribution
for a missing default — optimistic against the runtime's produce-nothing
no-match. Under exhaustiveness checking that optimism is retroactively
justified: a default-less case either proves exhaustive (the no-match
path is dead) or errors. The compile desugar's 0-or-1 variadic model is
unchanged.
