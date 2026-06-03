# Voxgig DX Report — Consolidated

Source reports:
- `voxgig-aql/bloom-filter/dx-report.md` (2026-06-01, aql `5b983b6`) — building
  a bloom filter library, five test suites, docs.
- `voxgig-aql/trie/DX-REPORT.md` (2026-06-01, aql `b6617dd`) — building four
  trie variants, eight namespaces, ~2000 lines + tests, plus a HAMT
  feasibility study.

Both authors shipped working libraries; their friction was almost entirely in
*discovering the idioms*, and nearly every hour lost went to behaviour that
failed **quietly** rather than loudly.

Severity legend (carried over from the source reports):
**🔴 high** = silent wrong results / blocks a use case ·
**🟡 medium** = friction with a clear workaround ·
**🟢 low** = papercut.

> Identifiers: each row is tagged `B#` (bloom report) or `T#` (trie report) so
> you can trace back. `T9.x` are items from the trie report's "Smaller
> papercuts" subsection.

## Status against current `main` (commit `4a8eb2d0`)

Each row was re-verified with a minimal repro (in `/tmp/aql-dx-check`) against
the freshly-built `cmd/go/bin/aql` from this checkout.

Status legend in the per-theme tables below:
- ✅ **fixed** — the documented symptom no longer reproduces.
- ❌ **open** — symptom reproduces unchanged on current `main`.
- 🟠 **changed** — still wrong, but the symptom is different from the report.
- 📖 **by design** — behaviour is intentional; reclassified as a docs item.

**Verification deltas worth flagging up front:**
- **Namespace export names are now CamelCase.** Commit `a3d701b1`
  (2026-06-01) made module exports enforce capital-initial names (`Test.test`,
  `Assert.equal`, `MathUtil.sqrt`) — the source reports used the lowercase
  `test.`/`assert.`/`math.` forms, so most of the voxgig test files now need
  a global rename before they will load.
- **`aql:array` was renamed to `aql:array-util`** (and binds the `ArrayUtil`
  namespace), per the lang/go CLAUDE.md. Three other modules took similar
  `-util` suffixes (`time-util`, `type-util`, `matrix-util`) to avoid
  colliding with builtin type names. The voxgig bloom-filter still imports
  `"aql:array"` and will not load against current `main` for that reason —
  unrelated to any of the issues raised in the reports, but blocks
  end-to-end re-running of the voxgig suites until they are updated.

These two breaking changes are **not** themselves DX issues in the sense the
reports describe (both are deliberate), but they are the reason a naive
`aql test/…` against current `main` will look catastrophic. Worth a short
upgrade note in the changelog if there isn't one already.

---

## Issues by theme

### Theme A — Silent dispatch / silent wrong results

This is the dominant category. Both authors identify it as the single biggest
cost; both lost time to bugs that produced wrong values with no error.

| Tag | Sev | Status | Issue |
|-----|-----|--------|-------|
| B1 | 🔴 | ❌ open | Forward `set` (`b set k v`) on a `refine Object` store silently does **not** mutate and the return value also lacks the write. Stack form (`b v k set`) works. Bisected from a saturated bloom filter. (Repro: `b1_set_fwd.aql` → forward both prints `None`, stack both print `1`.) |
| T1 | 🔴 | ❌ open | When a **namespace word**'s top-of-stack type doesn't match the first signature parameter, dispatch leaves the function value on the stack as data — no error. (A plain non-namespace word *does* error in the same case.) (Repro: `t1_ns_dispatch.aql` → wrong-order call prints `a fn my-get(Map, String)` instead of erroring.) |
| T3 | 🔴 | ❌ open | `merge` is a **deep, index-wise** merge: `{kids:[99]} {kids:[10,20]} merge` → `{kids:[99,20]}`. Used as a one-field update it fused sibling subtrees together — the corruption appeared in a branch the edit never touched. |
| T4 | 🔴 | ❌ open | `do {k:[v]}` evaluates each value quotation; if the value is a string that happens to name a word (`"if"`, `"do"`, `"get"`), the word is **dispatched** instead of stored. Workaround in use: box every stored value in a one-element list. (Repro: `t4_do_word.aql` → `do {val:["if"]}` raises `no matching signature for if`.) |
| T5 | 🟡 | ❌ open | `eq` on lists is **identity**, not structure (`["a" "b"] ["a" "b"] eq` → `false`). `assert.equal` is deep, so unit tests passed; a property body using `eq` silently passed vacuously. |
| T6 | 🟡 | ❌ open | `xs get i` with `i` a binding returns `none` — forward `get` grabs the bare word, not its value. `xs get (i)` works. Same root cause as B2 (forward-collection) but the symptom is silent. (Repro: `t6_get_bare.aql` confirms `xs get i` → `None`.) |
| T2 | 🟡 | 📖 by design | `fold` body binds **`[element accumulator]`** with the accumulator on top, not `[accumulator element]`. Wrong-order builds silently produce wrong results. (Repro: `t2_fold_order.aql` builds `[1,2,3]` from `[1,2,3]` with body `acc elem push` — confirms current binding order matches the report. Behaviour is intentional under the §1.4 sig-order rule documented in lang/go/CLAUDE.md; reclass as a docs item.) |

**Root causes (my read):** these collapse to three mechanisms — (1) overload
dispatch that falls back to "leave the value as data" instead of erroring on a
type miss (T1, and very likely B1); (2) forward-arg collection grabbing the
next token before evaluating bindings (T6, and adjacent to B2); (3) semantic
defaults that are reasonable in isolation but surprising in combination
(`merge` depth, `do` evaluation, `eq` identity).

### Theme B — Forward-arg collection edges

| Tag | Sev | Status | Issue |
|-----|-----|--------|-------|
| B2a | 🟡 | ❌ open | Chained forward prints reverse: `(1 add 1) print (2 add 2) print` prints **4 then 2**. (Repro: `b2_print_chain.aql`.) |
| B2b | 🟡 | ✅ fixed | Trailing `(expr) print` at EOF no longer raises. (Repro: `b2_print_trailing.aql` runs cleanly and prints `value 1`.) |
| T8 | 🟡 | 🟠 changed | `` `${…}` `` inside a recursive fn body — symptom is **different** now: instead of an "undefined word" error, the interpolation surfaces as a raw template AST (`word()({[{step- []} { [word(n)]}]})`) printed in place of the expanded string. Still wrong, no longer noisy. (Repro: `t8_simple.aql`.) |
| T9.3 | 🟢 | ❓ not reproduced | The basic shape "user-defined word followed by another word" no longer triggers the failure mode the report describes (`xs [add-one] each print end` → `[11, 21, 31]`). May have been fixed by the recent `force-arity` / forward-args work; needs a tighter repro from the trie source to confirm. |
| T9.4 | 🟢 | ❌ open | Mixed-form `if` produces a different result from all-forward: `(x 3 gt) if [...] [...]` evaluated wrongly (`small` when `x=5`, vs `big` from `if (x 3 gt) [...] [...]`). All-forward remains the safe form. (Repro: `t9_4_if.aql`.) |

### Theme C — Dispatch residue & overloads

| Tag | Sev | Status | Issue |
|-----|-----|--------|-------|
| B3 | 🟡 | ❌ open | `def _ (void-call)` (where the call has `[]` return) leaves stack residue; the next word mis-dispatches with "no matching signature for print" etc. Workaround: give mutators a return value, or call without `def`. (Repro: `b3_void_def.aql` still raises on the next `"done" print`.) |
| B5 | 🟢 | ❌ open | `make Object {}` rejected: "expected a constructed object type, got Object". Error doesn't suggest `refine Object` subtype or `{…}` literal. (Repro: `b5_make_object.aql` — same message.) |
| B6 | 🟢 | ❌ open | `indexof` is **haystack-first** (`indexof haystack needle`), cutting against the data-last grain elsewhere in the language. `("ZZBZZ" indexof "B")` → `-1`. (Repro: `b6_indexof.aql`.) |

### Theme D — Reserved identifiers

| Tag | Sev | Status | Issue |
|-----|-----|--------|-------|
| T7 | 🟡 | ✅ fixed (mostly) | `def node 1`, `def eq 42`, and `def L 5` all now work and print their values correctly — none of the three reported reserved-name failures reproduce. `end` is still a parser terminator (not a binding name) by design. Worth a docs note that all word names except the call-terminator `end` are usable bindings. (Repros: `t7_reserved*.aql`.) |

### Theme E — Modules / entry points

| Tag | Sev | Status | Issue |
|-----|-----|--------|-------|
| B4 | 🟡 | ✅ fixed | A library that uses `export` now runs directly with exit 0. (Repro: `b4_run_export.aql` — a minimal `def …; export "Mod" { … }` file runs cleanly.) The "library is import-only" surprise the bloom report describes is gone; an entry-point file using `export` is now a no-op rather than an error. |

### Theme F — Test framework UX

| Tag | Sev | Status | Issue |
|-----|-----|--------|-------|
| B7 | 🟢 | ❌ open | When an assertion inside a `[…] "name" Test.test` block fails, the error message still points at the *summary* line (`0 Test.fail-count end Assert.equal end`), not the failing case's name. (Repro: `b7_test_name.aql` — error span lands on line 11, the summary line, with no reference to `"should-fail-named-foo"`.) |

### Theme G — Missing primitives / capabilities

| Tag | Sev | Status | Issue |
|-----|-----|--------|-------|
| T9.1 | 🟡 | ❌ open | No way to build a map with **computed keys**: `set` on a `Map` literal still raises `no matching signature for set` (signatures cover Store/Object/Array only). (Repro: `t9_1_map_set.aql`.) `refine Object` dynamic fields aren't enumerable. The trie code still has to use association-list workarounds. |
| T9.2 | 🟡 | ❌ open | `filter` still rejects `[…]` quotation — "expected: filter (Function, Any)". (Repro: `t9_2_filter.aql`.) Inconsistent with `each`/`fold` which accept quotations. |
| T9.6 | 🟡 | ❌ missing | `raise` is still undefined. (Repro: `t9_6_raise.aql` → `undefined word: raise`.) |
| T9.7 | 🟡 | ❌ missing | No in-memory parser. (Repro: `t9_7_parse.aql` → `undefined word: parse`.) |
| — | 🟡 | ❌ missing | `with` / `assoc` still undefined. (Repro: `with_assoc.aql` → `undefined word: with`.) |

### Theme H — HAMT case study (capability ceiling)

The trie report explicitly declined a HAMT and documented why. Two levels:

**Level A — to express a *correct, persistent* HAMT.** Almost everything is
present (full bitwise suite, `bin.fnv32/64`, O(1) list indexing, structural
sharing via copy-returning ops). Gaps (status against current main in
parentheses):
1. **`popcount`** — the one genuinely absent primitive, central to HAMT slot
   indexing. Implementable in user code (SWAR or ≤64-step loop), but a native
   primitive is the highest-leverage single addition. (❌ still missing — `255
   popcount` → `undefined word: popcount`.)
2. **`insert-at` / `remove-at` for lists** — composable today from
   `take`/`concat`/`shed`; a primitive is cleaner and avoids O(n) rebuilds.
   (❌ still missing — `insert-at` undefined.)
3. **Defined fixed-width unsigned integer semantics** (`u32`/`u64`, or
   documented shift/wrap behaviour at bit 31/63). (Not directly probed; no
   evidence of change either way.)

**Level B — to make a HAMT actually *pay off*.** This is a runtime decision,
not surface syntax:
1. **Mutable, fixed-width, unboxed arrays** with an O(1) in-place
   `set`/`insert` contract (transient fast path for bulk construction). AQL
   has indexed `set` on `Array` but not on `List`, and the mutation contract
   isn't exposed.
2. **Layout guarantees** — contiguous packed storage, unboxed small ints — for
   the cache locality that *is* the HAMT's edge.
3. Realistically, **a native persistent-map type in the runtime** (HAMT/CHAMP)
   the way Clojure/Scala/Erlang ship one. As a bonus, this would also retire
   AQL's dynamic-key-map limitation (T9.1).

### Theme I — Minor papercuts

| Tag | Sev | Status | Issue |
|-----|-----|--------|-------|
| T9.5 | 🟢 | ✅ fixed | Print order now matches source order. (Repro: `t9_5_print_order.aql` prints `first`/`second`/`third` in source order.) The demo-script "leading blank line" workaround is no longer needed. |
| T9.8 | 🟢 | ❓ not retested | Property-generator order/charset sensitivity wasn't reproduced here — needs the full `test.prop`/`rand` driver to probe and the source repro from the trie report. Leave as documented but unverified. |

---

## What worked well (consolidated)

Both reports flagged genuine strengths:

- **Static dispatch & types.** `refine Object` subtypes, typed `fn`
  signatures, forward-precedence dispatch — predictable once internalised.
- **Recursion** (direct + mutual) with pattern-matched overloads is
  ceremony-free; the natural way to write trie traversal.
- **Persistent structures fall out naturally.** `push`/`merge` return new
  structures; immutable, path-copying tries were the path of least resistance.
- **Multiple namespaces per module** (`export "A" {…}` twice) cleanly split
  a `…Set` from a `…Map` over one engine.
- **Property testing** (`test.prop` / `test.check-prop` with `aql:rand`) was
  good enough to cross-check four variants against each other and find real
  bugs. The declarative spec form (`test.spec` / `test.run-spec`) works well
  but isn't promoted in the user docs — bloom-filter author found it in
  `design/NATIVE-MODULES.10.md`.
- **Error messages** — *where they fire* — are specific and well-pointed.
  The "forward args may have run into the next word; group the call with
  parens" hint is praised in both reports. The gap is silent cases, not loud
  ones.
- **`fold` / `each` / `iota` / `array.where`** and the math module read
  naturally in data-piped form.

---

## Verification summary

Confirmed fixed since the source reports (no further action needed):

- **B2b** trailing `(expr) print` at EOF no longer raises.
- **B4** `export`-using library now runs directly with exit 0.
- **T7** `node`, `eq`, single uppercase letters (`L`) — all usable as `def`
  bindings; only `end` remains reserved (by design as a statement
  terminator).
- **T9.5** print order matches source order.

Confirmed still open (priorities below remain valid):

- **B1, T1** — the two silent-dispatch bugs that dominate Theme A.
- **B2a** chained-print reversal, **B3** void-call residue.
- **B5** `make Object {}` hint, **B6** `indexof` direction, **B7**
  `Test.test` failing-case naming.
- **T3** deep `merge`, **T4** `do`-evaluates-words, **T5** list `eq`
  identity, **T6** forward-`get` swallows variable.
- **T9.1, T9.2, T9.4** map/filter/if shape limits.
- **T9.6, T9.7, popcount, insert-at, with/assoc** — all still missing.

Changed (still wrong, different symptom):

- **T8** string interp in a recursive fn body now leaks a raw template
  AST into the output instead of erroring with "undefined word".

Reclassified as docs:

- **T2** `fold` `[elem acc]` binding order is intentional under the
  unified §1.4 sig-order rule — documentation work, not a code fix.

Unverified (need a tighter repro):

- **T9.3** end-of-block user-fn collection — my probe came up clean;
  the actual trie source case may probe a different shape.
- **T9.8** generator order/charset sensitivity — needs the full
  `test.prop`/`rand` driver.

Collateral breaking changes (not DX bugs, but explain why a naive
re-run of the voxgig suites looks broken): namespaces CamelCase
(`Test.test`, `MathUtil.sqrt`); `aql:array` → `aql:array-util`.

---

## Prioritised recommendations

Ordered by leverage (impact × number of authors affected ÷ implementation
effort). Each item maps back to the tags above.

### P0 — Make silent failures loud (largest single leverage)

Most of the lost hours in both reports trace to **two** mechanisms. Address
these and most of Theme A collapses.

1. **Namespace dispatch type-mismatch must error, not no-op.** Today, a
   namespace word whose first signature param doesn't match TOS leaves the
   function value on the stack as data (T1) — almost always an arity/order
   bug. Plain words error in the same case. Align the behaviours, or at
   minimum emit a warning when a namespace member resolves to a bare function
   value. Likely also fixes B1 (the bloom `set`-forward silent no-op behaves
   exactly like a missed overload).
2. **`get` on a bare undefined word should error, not return `none`** (T6).
   This is the same forward-collection issue as B2 but the symptom is silent.
3. **Add a `popcount` style "loud diagnostics" pass** for the common silent
   shapes: `def _ (void-call)` residue (B3), `do {k:[v]}` evaluating a stored
   word-named string (T4) — at least emit a warning when `do` evaluates a
   value to a word reference.

### P1 — A small set of new primitives that retire workarounds

1. **Shallow field update word `with` / `assoc`** so `merge` isn't the only
   "replace one key" option (T3). Removes a 🔴 footgun.
2. **`raise` / `throw` with a message** (T9.6).
3. **In-memory jsonic `parse` / `decode`** to complement `jsonify` (T9.7).
4. **`filter` accepts a `[…]` quotation** like `each`/`fold` (T9.2).
5. **Native `popcount`** + **`insert-at` / `remove-at`** for lists (HAMT
   Level A).

### P2 — Docs (cheapest single batch)

A one-page "Gotchas" / "Idioms" reference covering the items that are
**still load-bearing** after the verification pass:

- Argument order = **reverse** of call order (T1).
- `fold` binds **`[element accumulator]`** with accumulator on top (T2 — now
  formally docs-only, intentional under the §1.4 sig-order rule).
- `merge` is deep, index-wise — use a shallow update / explicit constructor
  for one-field edits (T3).
- `do {k:[v]}` evaluates value quotations; box values whose runtime type is
  unknown (T4).
- `eq` on lists is identity; use `Assert.equal` or a structural equality
  word for structural compare (T5).
- `xs get i` vs `xs get (i)` — forward collection grabs the bare word (T6).
- `make Object {}` requires a `refine Object` subtype or `{…}` literal (B5).
- `indexof` is haystack-first; `("hay" indexof "needle")` is the wrong way
  round (B6).
- Promote the declarative spec test API (`Test.spec` / `Test.run-spec`) into
  the user docs.
- **Upgrade note** for downstream voxgig-style libraries: namespaces are
  CamelCase (`Test.`, `Assert.`, `MathUtil.`), and `aql:array` is now
  `aql:array-util` (binds `ArrayUtil.`).

Items dropped from the original gotchas list because they're now fixed:
T7 reserved-name false positives, B4 export-only-import surprise.

### P3 — Test-framework UX

- **Name the failing case** in `test.test` output (current message points at
  the summary line, not the failing case) (B7). Property drivers already
  surface `failing-input` — bring the example-based drivers up to parity.

### P4 — Runtime/capability (HAMT Level B — explicit, not urgent)

If/when a HAMT-class persistent map is on the roadmap: mutable unboxed
fixed-width arrays with transients, or ship a native HAMT/CHAMP map type. As
a bonus, the native map would retire AQL's dynamic-key-map limitation (T9.1)
and turn association-list workarounds back into idiomatic maps.

---

## Cross-cutting suggestion

Both reports independently arrive at the same diagnosis: **forward-arg
collection** is responsible for an outsized share of confusion (B2a, B3, T6,
T8, T9.3, T9.4). Today the rules are: collect forward greedily until a
delimiter; collide with the next token if the call wasn't grouped. The
recent `force-arity` / `forward-args` / `stack-args` words (commits since
the source reports) give callers explicit control over which side each
arg comes from — that's the foundation for fixing the symptoms while
preserving the model. Two follow-ups worth considering:

1. Make **`print` stack-first by default** — the bloom report explicitly
   suggests this. Today the chained `(expr) print (expr) print` reversal
   (B2a) is the one item from this group that is *most easily fixed without
   touching the dispatch model* — `print` is the only word where forward
   collection actively hurts readability.
2. A short **forward-vs-stack arity reference** in `lang/doc/` that names
   the rules and the new `/s` / `/f` / `/N` modifiers as the explicit
   levers. The lang/go CLAUDE.md "Argument Ordering" section is the
   authoritative source — a user-facing distillation of it would close the
   discoverability gap even without further semantics changes.
