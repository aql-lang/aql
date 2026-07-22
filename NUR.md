# Non-Uniformity Register (NUR)

A running register of every place where AQL — the language or its
implementation — deviates from one of its own uniform rules. A
non-uniformity is any special case: a type treated differently from its
siblings, one member of a word family with an exception, a path that
bypasses a single-source-of-truth mechanism. Uniformity is a core design
value of AQL (one parser, one argument-positioning convention, one
binding store, one total order, one truthiness rule); this register is
where every deviation from that value is made visible, argued, and
either eliminated or explicitly accepted.

Records are short, numbered (`NUR000`, `NUR001`, …), and dated, in the
style of [ADR.md](ADR.md). Numbers are never reused; resolved records
are kept (with a pointer to the resolving change) rather than deleted,
so the history of *why* stays legible.

> **A newly-encountered non-uniformity is a PR blocker.** When a
> non-uniformity surfaces — in code review, in a design note, or during
> coding and debugging — it is recorded here immediately with status
> **Pending**, and the PR that surfaced it must not merge until the
> entry is either **Resolved** (the divergence is removed) or marked
> **Allowed** (an explicit, argued acceptance). Unlike ADRs, *recording*
> is mandatory on discovery, not on maintainer instruction; what
> requires the maintainer is the **Allowed** verdict — the same reviewed
> discipline as a `//covergate:allow` entry
> (`design/COVERAGE-ALLOWLIST.10.md`).

**Statuses:**

- **Pending** — recorded but not yet argued to a verdict. Blocks the PR
  that surfaced it. Every Pending record must also appear in the
  pending list below.
- **Allowed** — a deliberate divergence, kept. The record states the
  uniform rule, the divergence, the rationale, and the evidence that
  pins it (docs and tests), so the acceptance cannot silently rot.
- **Resolved** — the divergence was removed. The record is kept and
  points at the resolving change.

---

## Pending non-uniformities (the blocking list)

The live list of records whose status is **Pending**. A PR that
surfaced (or contains) one of these must not merge while it is listed
here. An entry leaves this list only by becoming **Resolved** or
**Allowed** in its record below — keep the two in sync in the same
commit.

| # | Title | Surfaced by |
|---|-------|-------------|
| *(none)* | | |

---

## NUR000 — Boolean arithmetic is a defined error {#nur000}

**Status:** Allowed · **Date:** 2026-07-22

### The uniform rule

The six arithmetic words (`add`/`sub`/`mul`/`div`/`mod`/`pow`) are
**total within every scalar type and every Micron kind**
(REFERENCE.md §"Within-type operations"): numbers compute, `String` and
`Atom` carry the occurrence package, `Bytes` mirrors it over byte
subsequences, Microns fall to the field-wise default.

### The divergence

`Boolean` is the single scalar family excluded: `add true false` raises
`[aql/type_error]: add: arithmetic is not defined on Boolean` — for all
six ops.

### Why allowed

Boolean deliberately carries the logical words (`and`/`or`/`xor`/`not`)
instead of arithmetic; every candidate arithmetic semantics (C-style
integer promotion, GF(2)) is arbitrary, and an arbitrary choice would
be silently accepted where a loud error teaches the logical vocabulary.
The exception is implemented as a **registered** `[Boolean Boolean]`
signature that raises with a pinned message (the `setMicron` precedent)
rather than by signature absence, so the failure is specific instead of
an opaque dispatch error; a check-mode mirror (`booleanArithReturns`)
flags concrete Boolean arithmetic statically; and the signatures are
CoreDefault, so a user's `refine Boolean` overload can still extend an
arithmetic word by specificity (the refinement escape).

### Evidence

- `lang/go/native/native_scalar_ops.go` — `booleanArithHandler` /
  `booleanArithError` / `booleanArithReturns`; the six erroring
  `[Boolean Boolean]` signatures.
- REFERENCE.md §"Within-type operations" — "**`Boolean`** arithmetic is
  a **defined error**".
- `lang/spec/scalar-micron-ops.tsv` (all six ops pinned as errors);
  `lang/spec/open-words.tsv` (the refine-extension escape and its
  negative twins).

---

## NUR001 — `convert Boolean` coerces by presence, not content {#nur001}

**Status:** Allowed · **Date:** 2026-07-22

### The uniform rule

`convert <ScalarType> <String>` parses the string's **content**:
`convert Integer "42"` → `42`, `convert Float "1.5"` → `1.5`.

### The divergence

`convert Boolean "false"` → `true`. Boolean conversion applies the
truthiness rule — only `false`, `0`/`0.0`, `none`, `""`, `[]`, `{}` are
false; a String's characters are never inspected.

### Why allowed

`convert Boolean` shares **one** coercion rule with `if`-condition
truthiness and `make Boolean` (presence, not content); making the
conversion path parse content would fork truthiness into two rules —
a worse non-uniformity than the one it fixes. Content parsing exists as
an explicit opt-in: `convert Boolean {truthy: true}` parses the YAML
tokens (`yes`/`no`/`true`/`false`/`on`/`off`, case-insensitive) and
falls back to presence for anything else; the option is inert for
non-Boolean targets.

### Evidence

- `lang/go/native/native_type.go` — `coerceBooleanTruthy` and the
  `truthy` option plumbing on `convert`.
- REFERENCE.md — "**`convert Boolean` is presence coercion; `{truthy:
  true}` opts into YAML parsing**" and §`if` ("coerces its condition …
  the exact same rule as `convert Boolean` and `make Boolean`").
- `lang/go/native/native_type_convert_seam9_test.go` (both modes,
  positive and negative); `lang/go/native/integration_coverage_test.go`
  (`'false' convert Boolean` → `true` pinned explicitly).

---

## NUR002 — Boolean alone is case-exhaustible by value enumeration {#nur002}

**Status:** Allowed · **Date:** 2026-07-22

### The uniform rule

For a scalar scrutinee, a default-less `case` proves exhaustiveness
through type clauses, `[is T]` predicates, or comparison-predicate /
refinement interval unions — never by listing scalar literals
(`case n [1 … 2 …]` can not cover `Integer`).

### The divergence

`true` + `false` cover a `Boolean` scrutinee like enum members: `case b
[true 1 false 0]` is statically exhaustive with no default, and `case b
[true 1]` is a `case_not_exhaustive` check error (`uncovered: false`).
No other builtin scalar gets literal-enumeration coverage.

### Why allowed

The divergence follows from cardinality, not special pleading: Boolean
is the only **finite** builtin scalar, so value enumeration is actually
sound for it — the checker's coverage proof stays in the sound
direction. The mechanism is the same value/type coverage channel enums
use (`def Color (red/q tor …)` covered member by member), so Boolean is
being admitted to an existing uniform mechanism that other scalars
cannot soundly enter, rather than getting a private code path.

### Evidence

- REFERENCE.md §"`case` — dispatch and exhaustiveness" — "**Boolean, by
  `true` and `false`** (or the `Boolean` literal)", including the
  negative example.
- `lang/spec/case.tsv` §6 ("true+false cover Boolean", alongside the
  union and enum coverage rows that show the shared mechanism).

---

## NUR003 — `and`/`or` select an operand; the rest of the boolean family returns strict Boolean {#nur003}

**Status:** Allowed · **Date:** 2026-07-22

### The uniform rule

The boolean word family returns strict `Boolean`: `not`, `xor`, `any`,
`all`, and the `aql:logic-util` gates (`nand`/`nor`/`xnor`/`iff`/
`implies`) all coerce their inputs by truthiness and yield `true` or
`false`.

### The divergence

`and` and `or` are value-selecting short-circuit connectives: they
return whichever **operand** decided the result, of whatever type —
`1 and 2` → `2`, `false 5 and` → `false`, `0 9 or` → `9`.

### Why allowed

Deliberate Lisp/Python semantics: the operand form composes directly
(`x or default`, with `otherwise` as the None-aware variant), and a
strict Boolean is one `not not` (or a comparison) away. The divergence
is loudly documented at the word table itself, and check mode types the
result precisely — `foldOrJoin` concrete-folds statically-decided
selections and otherwise narrows to the join of the operand types, so
the non-uniform return type never degrades static analysis to `Any`.

### Evidence

- `lang/go/native/native_boolean.go` — `andHandler`/`orHandler` (operand
  return) vs `notHandler`/`boolBinaryNative`/`anyHandler`/`allHandler`
  (strict Boolean); `foldOrJoin` for the check-mode typing.
- REFERENCE.md §Boolean — "**`and` / `or` return an operand, not a
  coerced boolean.**"

---

## NUR004 — Boolean and Atom have no lattice subtypes {#nur004}

**Status:** Allowed · **Date:** 2026-07-22

### The uniform rule

The scalar branch families carry structural leaves: `String` has
`EmptyString`/`ProperString`, `Number` has `Integer`/`Float`/
`BigInteger`/`BigDecimal`, `Micron` has its twelve kinds.

### The divergence

`Scalar/Boolean` and `Scalar/Atom` are leaf-less — direct children of
`Scalar` with no builtin subtypes (no `True`/`False` lattice nodes).

### Why allowed

Vacuous rather than divergent: no kernel mechanism requires a scalar
family to have leaves, and nothing dispatches on their presence. There
is no useful structural split of Boolean — `True`/`False` subtypes would
duplicate what value-level machinery already provides uniformly (`case`
literal coverage per NUR002, and DepScalar refinements: `(Boolean gte
true)` *is* the true-only subset, since Boolean is a supported
refinement base like every other well-known scalar). Users who want a
nominal split can mint it (`refine Boolean`), which participates in
dispatch by specificity like any refinement.

### Evidence

- `eng/go/typetable.go::builtinDecls` — the Scalar branch layout.
- `eng/go/depscalar.go::canonicalBaseType` — Boolean listed among the
  supported DepScalar bases; `lang/spec/compare.tsv` and
  `lang/spec/case.tsv` exercise the value-level machinery that stands in
  for subtypes.
