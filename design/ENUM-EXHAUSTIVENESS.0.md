# ENUM-EXHAUSTIVENESS — static coverage checking for closed disjuncts

**Status: living reference.** Describes the `case`-exhaustiveness advisory
shipped in `lang/go/native/conditional.go` plus the design frame for the two
deferred sites (`if`, signatures).

## Why

`enum [red green blue]` builds a `Type/Disjunct/Enum` — a *closed* set of
named constants. `tor` builds a `Type/Disjunct` — a union that is open in
spirit (`Integer tor String` admits unboundedly many inhabitants). The two
are behaviourally identical at dispatch (membership routes through the same
`DisjunctInfo` machinery; `NewEnum` and `NewDisjunct` differ only by tag —
`eng/go/value.go`), which historically left the distinct `Enum` tag doing
nothing but relabelling `typeof`/`inspect`.

**Exhaustiveness is the functional payoff that makes closedness pay for
itself.** If a value's static type is a closed set `{red, green, blue}`, a
construct that dispatches on it can be checked to cover every member (or
carry an explicit default). An open union carries no such obligation. That
asymmetry — check the closed set, never the open union — is the whole design.

## The shape it keys on (not the tag)

A deliberate, empirically-grounded choice: exhaustiveness keys on the
**"disjunct whose alternatives are all concrete values"** shape, NOT on the
`Enum` type tag.

In check mode an enum-typed binding (`c:Color`) does not reach a construct as
an `Enum`. Param carriers widen the tag to `Disjunct`, but the concrete
alternatives survive and are readable via `AsDisjunct(v).Alternatives`. So:

- Keying on `Enum` would miss the primary case (the widened carrier presents
  as `Disjunct`) and would need a deep carrier-typing repair to fix.
- Keying on "all alternatives `IsConcrete`" works with the carrier as it is
  today, and is *more* correct: it also covers a hand-written closed union
  `red/q tor green/q tor blue/q` (equally closed), while correctly skipping a
  half-open union like `red/q tor Integer` (one alternative is a bare type
  literal → unbounded → not checkable).

`enum` remains the ergonomic, intent-documenting constructor for exactly this
shape; the checker simply recognises the shape however it was built.

Rule: **analyse iff the scrutinee is a `Disjunct` with ≥1 alternative and
every alternative is concrete.** Any bare-type-literal alternative → skip.

## Site 1 — `case` (shipped)

`case`'s check-mode return computer `caseReturnsFn`
(`lang/go/native/conditional.go`) already walks the clause list `elems`
(even index = match arm, trailing odd element = default) and knows the
scrutinee `v`. The exhaustiveness pass hangs off that walk as a pure
side-effect (it emits diagnostics; it does not change the computed return
type, and the runtime `caseHandler` is untouched):

```
def Color enum [red/q green/q blue/q]
def name c:Color [ case c [ red/q "R"  green/q "G" ] ]   # advisory: 'blue' unhandled, no default
```

Two advisories, both `SeverityInfo` (non-gating), modelled on the existing
`redundant_guard` lint (`eng/go/carrier.go::ApplyGuardNarrowing`):

- **`case_nonexhaustive`** — the scrutinee is a closed set, there is no
  trailing default, no arm is a predicate, and ≥1 member is matched by no
  arm. Detail lists the uncovered members.
- **`case_unreachable_clause`** — a concrete match arm unifies with *no*
  member of the closed set (a typo'd or out-of-set constant, e.g. a `yellow/q`
  arm on a `Color` scrutinee). The dual of coverage, nearly free once the
  member/arm cross-product is computed.

### Decisions

- **A trailing default defeats exhaustiveness.** `case c [red/q "R" "other"]`
  is exhaustive by construction. This is the explicit escape hatch, and it
  means the check only bites code that is *trying* to enumerate.
- **A predicate arm (`[gt 3]`, `isCodeBody`) suppresses the coverage
  check.** A predicate can match any subset of the remaining members and is
  statically undecidable, so a predicate arm makes non-exhaustiveness
  unprovable — we do not warn. Unreachable-clause detection still runs for
  the concrete arms (independent of predicates).
- **Word arms resolve like `caseClauses`.** A bare-word match resolves
  through `ResolveTypedName` then `ResolveWordValue` (→ `Atom`) before the
  coverage test, so `red` and `red/q` behave identically.
- **Coverage test is `UnifyR(arm, member, r)`** — the same relation
  `caseClauses` dispatches on at runtime, so "covered" means exactly "would
  match at runtime."
- **Dedup** by (code, detail), matching the `redundant_guard` pattern, so a
  fn body re-analysed per call-shape emits each finding once.

### Severity: info now, warning later

`case_nonexhaustive` is conceptually a partial-coverage finding, the same
family as `partial_dispatch`/`unreachable_signature` (both `SeverityWarning`).
It is deliberately landed at `SeverityInfo` first so it cannot retroactively
gate `aql check` on the existing spec corpus. Promotion to `SeverityWarning`
is a follow-up once the corpus is known clean. A non-exhaustive `case` is
NOT a `RuntimeMirror`: at runtime an unmatched `case` returns no value, it
does not trap, so the finding is advisory, not a mirror of a guaranteed error.

## Site 2 — `if` (deferred, folded into narrowing)

The `if [c1 b1 …]` clause-list form is a boolean-guard chain, not a value
dispatch, so exhaustiveness only applies when the guards are recognisably
member tests (`(c teq red/q)`), which is brittle to pattern-match. The
higher-value, reusable primitive is **`is`-narrowing**: after
`if (x is Color) [ … ]` the then-branch should see `x` at the narrowed type,
which then feeds a `case`. `if`-specific exhaustiveness is a low-yield special
case of narrowing and is intentionally not implemented as its own check.

## Site 3 — function signatures (deferred, the mirror win)

If a fn param's type is a closed set and the value-pattern overloads omit a
member, an uncovered call raises `no_signature` at runtime:

```
def render fn [ [red/q] [String] ["R"]  [green/q] [String] ["G"] ]   # 'blue' → no_signature
```

Unlike Site 1 this one legitimately *is* a `RuntimeMirror` — an uncovered
call is a guaranteed trap — so it fits the existing gating-mirror machinery
and would upgrade a runtime failure to a compile-time error. It is the most
valuable site and the hardest (it reasons over the sig set, not a linear
clause list); deferred to a later phase. Hook: the fn-analysis/install path
(`AnalyseFnBody` / `native_definition.go`).

## Precondition note

Everything here is bounded by the static type flowing into the construct.
`enum` members are atoms, which widen to `Atom`/`Any` easily; once a scrutinee
has lost its disjunct carrier there is nothing to check. Tightening enum-typed
value provenance (so `Color` survives more transformations) would widen the
reach of all three sites and is the natural companion work.
