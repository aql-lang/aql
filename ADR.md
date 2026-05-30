# Architecture Design Record (ADR)

A running list of the key architectural decisions behind AQL — the ones
that shape the language and its implementation, with the reasoning that
led to them. Each record is short, numbered, and dated. Newer decisions
may supersede older ones; superseded records are kept (struck through in
status) rather than deleted, so the history of *why* stays legible.

When you make a decision that future contributors would otherwise have
to reverse-engineer from the code, add a record here.

---

## ADR-001 — Native modules must not shadow core words {#adr-001}

**Status:** Accepted · **Date:** 2026-05-30

### Decision

A native module (`aql:math`, `aql:array`, `aql:matrix`, …) must **never
export a name that collides with a core (built-in) word**. If an
operation would naturally share a core word's name, do one of the
following instead:

1. **Extend the core word** with an additional type-dispatched signature,
   when the operation is a genuine variant of it; or
2. **Choose a different export name** for the module word.

### Context

AQL resolves words by signature and has no implicit `Word → Atom`
fallback. When a module exports a name that also exists as a core word,
two different operations end up wearing the "same" name, distinguished
only by an `aql:array`-style prefix. That is confusing in exactly the
case it matters most: when both apply to the *same* value type but mean
different things.

The motivating case was the array vocabulary. Three array operations had
been given `arr-`-prefixed built-in names (`arr-flatten`,
`arr-transpose`, `arr-indexof`) purely to dodge collisions with the core
`flatten` and `indexof`, and the first cut of the `aql:array` module
re-exported them as `array.flatten`/`array.indexof`. That meant
`flatten` (core, one level) and `array.flatten` (deep) did *different
things to the same list* — a foot-gun, and a symptom that the boundary
was drawn in the wrong place.

### Consequences

For `aql:array` specifically:

- **Deep flatten** is now `flatten -1` — a negative depth on the core
  `flatten` word (which removes one level by default, or `N` levels with
  `flatten N`). There is no `array.flatten`.
- **List lookup** is now a `[List, List]` overload of the core `indexof`
  word (its string form returns a scalar position; the list form returns
  a vector of indices). There is no `array.indexof`.
- **`transpose`** has no core counterpart, so it keeps its plain name and
  remains `array.transpose`. The `arr-` workaround names are gone.

After this, the `aql:array` export set shares no name with any core word.

### Known deviations

The `aql:matrix` module predates this record and still exports `flatten`,
`size`, and `transpose` (as `matrix.flatten` etc.). These operate on a
distinct `Tensor`/`Matrix` type rather than on plain lists, so the
overlap is nominal rather than behavioural, but they do not yet satisfy
the rule as stated. Reconciling them (fold into the core words by type,
or rename) is tracked as follow-up work, not done here.
