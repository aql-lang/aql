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

A native module (`aql:math`, `aql:array-util`, `aql:matrix-util`, …) must **never
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
only by an `aql:array-util`-style prefix. That is confusing in exactly the
case it matters most: when both apply to the *same* value type but mean
different things.

The motivating case was the array vocabulary. Three array operations had
been given `arr-`-prefixed built-in names (`arr-flatten`,
`arr-transpose`, `arr-indexof`) purely to dodge collisions with the core
`flatten` and `indexof`, and the first cut of the `aql:array-util` module
re-exported them as `ArrayUtil.flatten`/`ArrayUtil.indexof`. That meant
`flatten` (core, one level) and `ArrayUtil.flatten` (deep) did *different
things to the same list* — a foot-gun, and a symptom that the boundary
was drawn in the wrong place.

### Consequences

For `aql:array-util` specifically:

- **Deep flatten** is now `flatten -1` — a negative depth on the core
  `flatten` word (which removes one level by default, or `N` levels with
  `flatten N`). There is no `ArrayUtil.flatten`.
- **List lookup** is `ArrayUtil.indices` — a distinctly-named array word
  (for each needle, its index in the haystack, or `-1` when absent). There
  is no `ArrayUtil.indexof`.

  > **Amendment (2026-06-07).** This was originally folded into the core
  > `indexof` word as a `[List, List]` overload. Two later changes undid
  > that: `indexof` itself moved out of core into `aql:string-util`
  > (`StringUtil.indexof`, string-only), and overloading one word across
  > two unrelated domains proved a smell — the string form returns a
  > scalar with `-1`-when-absent, while the list form returns a vector
  > with a *different* absent sentinel. The list form is now its own word,
  > `ArrayUtil.indices`, in `aql:array-util`, with `-1` for an absent
  > needle (consistent with the string form's not-found value). This still
  > honours the ADR: `indices` shadows no core word.
- **`transpose`** has no core counterpart, so it keeps its plain name and
  remains `ArrayUtil.transpose`. The `arr-` workaround names are gone.

After this, the `aql:array-util` export set shares no name with any core word.

### Applied to `aql:matrix-util`

The `aql:matrix-util` module predated this record and exported `size`,
`flatten`, and `transpose`. These have been reconciled:

- **`size`** — dropped. The core `size` word already reports a tensor's
  entry count via the Sizer behavior (`TensorData`), so a `MatrixUtil.size`
  export only shadowed it.
- **`flatten`** — renamed to **`MatrixUtil.values`** (the row-major list of
  entries). The core `flatten` word remains the only `flatten`.
- **`transpose`** — kept. `transpose` is *not* a core word; it lives in
  the `aql:array-util` module. `MatrixUtil.transpose` and `ArrayUtil.transpose` are
  two namespaced module words, which this rule permits — the rule is
  about shadowing *core* words, not other module words.

After this, no module export shadows a core word.

---

## ADR-002 — No implicit broadcasting {#adr-002}

**Status:** Accepted · **Date:** 2026-05-30

### Decision

AQL will **not** implement broadcasting — the implicit lifting of a
scalar word over an array. Applying an operation across an array is
always **explicit**, via a combinator (`each`, `eachrank`, `fold`, …).
A scalar word applied to a list where it expects a scalar is a **type
error**, not a silent element-wise map.

```
add 10 [1,2,3]            # type error — no matching signature
each [add 10] [1,2,3]     #  # returns [11,12,13]   (the supported form)
```

### Context

An earlier draft of `design/ARRAYIFICATION.6.md` proposed broadcasting:
`add 10 [1,2,3]` returns `[11,12,13]`, with rules for scalar+list, equal-length
list+list zip, and nested alignment. It is attractive (it reads like
NumPy/APL) but a poor fit for AQL:

1. **It cannot be a word.** It would have to be a fallback wedged into
   the signature matcher (`eng/go/match.go`) — the most load-bearing
   code in the kernel — affecting *every* scalar word at once. A subtle
   bug there regresses the whole language, not one word.
2. **It defeats the static checker.** Result rank depends on the runtime
   shape of the operands, so `Check` mode could no longer infer result
   types without modelling unknown-depth lifting — undermining the
   typed-list carrier inference the codebase already relies on.
3. **It is ambiguous.** Words that legitimately take list arguments
   (`reshape`, `at`, the `group`/`fold` overloads, …) collide with the
   "scalar op lifted over a list" reading. The matcher would need a
   fragile precedence rule between "a real `[List, …]` signature exists"
   and "no scalar match → broadcast".
4. **It buys ergonomics, not power.** `add 10 [1,2,3]` is already
   `each [add 10] [1,2,3]`. The implicit form saves keystrokes at the
   cost of making dispatch — and reading — less predictable.

### Consequences

- Design principle 3 is "explicit iteration", not "implicit iteration".
- The `## Broadcasting` section of the arrayification design is marked
  rejected; Phase 5 is "rank polymorphism" (`eachrank`, `foldaxis`),
  which is explicit depth-targeting, not broadcasting.
- `eachrank`/`foldaxis` bodies must themselves iterate (e.g.
  `eachrank 1 [each [add 10]] …`); there is no implicit lift at the cell.
- This is a decision about the *language*. Type-specific element-wise
  behaviour can still be offered by a word with an explicit `[List, …]`
  signature (as `add` does for string concatenation, or `indexof` for
  lists) — that is normal signature dispatch, not broadcasting.
