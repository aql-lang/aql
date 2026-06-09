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

## Status against current `main` (commit `8fdd4e1`, 2026-06-09)

Each row was re-verified with a minimal repro (in `/tmp/aql-dx-check`) against
the freshly-built `cmd/go/bin/aql` from this checkout. The first
verification pass ran against commit `4a8eb2d0`; this revision is a
**second pass against `8fdd4e1`** (134 commits later), which landed
fixes for B1, B6, B7, T4, T8, and `popcount`, plus the `deq` word,
`StructUtil.setpath`, the lazy forward-argument resolution engine, and
three new `aql check` diagnostics (`uncalled_function`,
`forward_strands_operand`, dead-overload detection).

Status legend in the per-theme tables below:
- ✅ **fixed** — the documented symptom no longer reproduces.
- ❌ **open** — symptom reproduces unchanged on current `main`.
- 🟠 **changed/mitigated** — still wrong but with a different symptom, or a
  real improvement landed (e.g. caught by `aql check`) with a residual gap.
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
| B1 | 🔴 | ✅ fixed | Forward `set` (`b set k v`) on a `refine Object` store silently did **not** mutate. **Now:** `set` is an in-place mutator in *both* forms — forward `b set flag 1` writes (read-back returns `1`), and so does the stack form. Note the semantics also changed: `set` returns **nothing**, so `def r (b set k v)` no longer binds anything; read the store back instead. |
| T1 | 🔴 | 🟠 mitigated | When a **namespace word**'s top-of-stack type doesn't match the first signature parameter, runtime dispatch still leaves the function value on the stack as data — no error (re-confirmed: wrong-order call prints `fn my-get(Map, String)`). **But** `aql check` now reports it as an *error*: `uncalled_function: call to 'my-get' matched no signature and was left on the stack as data` (commit `5ddc03c`). Loud at check time, silent at runtime. |
| T3 | 🟠 | 🟠 mitigated | `merge` is still a **deep, index-wise** merge (`{kids:[99]} {kids:[10,20]} StructUtil.merge` → `{kids:[99,20]}`), but it is **no longer a core word** — it moved to `aql:struct-util`, and `StructUtil.setpath` now exists as the one-field update that returns a new structure (`{a:1,b:2} StructUtil.setpath "b" 3` → `{a:1, b:3}`). Remaining work is docs: steer one-field edits to `setpath`. |
| T4 | 🔴 | ✅ fixed | `do {k:[v]}` no longer dispatches a string that names a word: `do {val:["if"]}` → `{"val": "if"}` (commits `99de896`, `3eb461f` — the last string→word promotion was removed and pinned). The box-every-value workaround is obsolete. |
| T5 | 🟡 | 📖 by design | `eq` on lists is **identity** — now documented as intended (commit `0fedfc0`), and the structural form **`deq` exists**: `["a" "b"] ["a" "b"] eq` → `false`, `… deq` → `true`. REFERENCE.md carries the property-body-with-eq-passes-vacuously caveat. |
| T6 | 🟡 | 📖 by design | `xs get i` returning `none` is now *defined* semantics, not a forward-collection accident: a bare word key is a **literal** name (JS `.key`), a parenthesised key is **computed** (JS `[expr]`) — `xs get i` → `None`, `xs get (i)` → `20`. Documented in REFERENCE.md "Maps and access" (commits `ef78e93`, `cb272fc`). |
| T2 | 🟡 | 📖 by design | `fold` body binds **`[element accumulator]`** with the accumulator on top, not `[accumulator element]`. Wrong-order builds silently produce wrong results. (Repro: `t2_fold_order.aql` builds `[1,2,3]` from `[1,2,3]` with body `acc elem push` — confirms current binding order matches the report. Behaviour is intentional under the §1.4 sig-order rule documented in lang/go/CLAUDE.md; reclass as a docs item.) |

**Root causes (my read):** these collapse to three mechanisms — (1) overload
dispatch that falls back to "leave the value as data" instead of erroring on a
type miss (T1; B1 turned out to be separable and was fixed independently by
making `set` an in-place mutator); (2) forward-arg collection grabbing the
next token before evaluating bindings (T6 — since redefined as intentional
literal-key semantics); (3) semantic defaults that are reasonable in
isolation but surprising in combination (`merge` depth, ~~`do` evaluation~~
(fixed), `eq` identity (documented, with `deq` as the structural form)).

### Theme B — Forward-arg collection edges

| Tag | Sev | Status | Issue |
|-----|-----|--------|-------|
| B2a | 🟡 | ❌ open | Chained forward prints reverse: `(1 add 1) print (2 add 2) print` prints **4 then 2**. Re-confirmed on `8fdd4e1`; not flagged by `aql check` either. (Repro: `b2_print_chain.aql`.) Beware: this reordering silently scrambles the apparent results of *any* multi-statement repro that omits `end` separators. |
| B2b | 🟡 | ✅ fixed | Trailing `(expr) print` at EOF no longer raises. (Repro: `b2_print_trailing.aql` runs cleanly and prints `value 1`.) |
| T8 | 🟡 | ✅ fixed | `` `${…}` `` inside a recursive fn body now expands correctly — a genuinely recursive interpolation (`` `step-${n} then ${(n 1 sub) walk}` ``) prints `step-3 then step-2 then step-1 then done`. The raw-template-AST leak from the first verification pass is gone. (Repro: `t8_rec.aql`.) |
| T9.3 | 🟢 | ✅ fixed | The basic shape "user-defined word followed by another word" works (`xs [add-one] each print` → `[11, 21, 31]`, re-confirmed on `8fdd4e1` after the lazy forward-resolution rework). No tighter repro from the trie source has surfaced a remaining failure mode. |
| T9.4 | 🟢 | ❌ open | Mixed-form `if` produces a different result from all-forward: `(x 3 gt) if [...] [...]` evaluates wrongly (`small` when `x=5`, vs `big` from `if (x 3 gt) [...] [...]`). Re-confirmed on `8fdd4e1`; `aql check` reports nothing. All-forward remains the safe form. (Repro: `t9_4_if.aql`.) |

### Theme C — Dispatch residue & overloads

| Tag | Sev | Status | Issue |
|-----|-----|--------|-------|
| B3 | 🟡 | ❌ open | `def _ (void-call)` (where the call has `[]` return) leaves stack residue; the next word mis-dispatches with "no matching signature for print" etc. Re-confirmed on `8fdd4e1`; `aql check` does not flag it. Workaround: give mutators a return value, or call without `def`. (Repro: `b3_void_def.aql` still raises on the next `"done" print`.) |
| B5 | 🟢 | ❌ open | `make Object {}` rejected: "expected a constructed object type, got Object". Error still doesn't suggest `refine Object` subtype or `{…}` literal. (Repro: `b5_make_object.aql` — same message on `8fdd4e1`.) |
| B6 | 🟢 | ✅ fixed | `indexof` is now **haystack-last**, on the data-last grain: it moved to `aql:string-util` as `StringUtil.indexof needle haystack` (`StringUtil.indexof "ll" "hello"` → `2`), with the list form split out as `ArrayUtil.indices` (commits `3b3316c`, `0fedfc0`, `ec5aa25` — the whole string module is now subject-last). The bare core word is gone, so old haystack-first call sites fail loudly rather than answering `-1`. |

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
| B7 | 🟢 | ✅ fixed | A failing `[…] "name" Test.test` case now prints a loud, *named* FAIL line: `FAIL should-fail-named-foo — [aql/assertion_failure]: Assert.equal: expected 2, got 1`, and `Test.describe` paths are included (commit `26ede5c`, pinned by `lang/go/modules/test_failure_naming_test.go`). Passing cases stay quiet. (Repro: `b7_test_name.aql`.) |

### Theme G — Missing primitives / capabilities

| Tag | Sev | Status | Issue |
|-----|-----|--------|-------|
| T9.1 | 🟡 | ❌ open | No way to build a map with **computed keys**: `set` on a `Map` literal still raises `no matching signature for set` (signatures cover Store/Object/Array only — re-confirmed on `8fdd4e1`). (Repro: `t9_1_map_set.aql`.) `refine Object` dynamic fields aren't enumerable. The trie code still has to use association-list workarounds. |
| T9.2 | 🟡 | ❌ open | `filter` still rejects `[…]` quotation — "expected: filter (Function, Any) or (Reach, Any)" (the new Reach lens form `$.field` is accepted, but a plain quotation still isn't). Inconsistent with `each`/`fold` which accept quotations. (Repro: `t9_2_filter.aql`.) |
| T9.6 | 🟡 | ❌ missing | `raise` is still undefined on `8fdd4e1`. (Repro: `t9_6_raise.aql` → `undefined word: raise`.) |
| T9.7 | 🟡 | ❌ missing | Still no in-memory parser on `8fdd4e1`. (Repro: `t9_7_parse.aql` → `undefined word: parse`.) |
| — | 🟡 | 🟠 covered | `with` / `assoc` remain undefined as words, but the *capability* landed: `StructUtil.setpath` is a copy-returning single-key (and deep-path) update — `{a:1,b:2} StructUtil.setpath "b" 3` → `{a:1, b:3}`. Remaining gap is naming/discoverability, i.e. docs. |

### Theme H — HAMT case study (capability ceiling)

The trie report explicitly declined a HAMT and documented why. Two levels:

**Level A — to express a *correct, persistent* HAMT.** Almost everything is
present (full bitwise suite, `bin.fnv32/64`, O(1) list indexing, structural
sharing via copy-returning ops). Gaps (status against current main in
parentheses):
1. **`popcount`** — the one genuinely absent primitive, central to HAMT slot
   indexing. (✅ **landed** — `aql:bin-util` now exports it, alongside
   `clz`, `ctz`, `bitlen`, `mask`, `reverse`, `swap`:
   `255 BinUtil.popcount` → `8`.)
2. **`insert-at` / `remove-at` for lists** — composable today from
   `take`/`concat`/`shed`; a primitive is cleaner and avoids O(n) rebuilds.
   (❌ still missing on `8fdd4e1` — `insert-at` undefined.)
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

## Verification summary (second pass, `8fdd4e1`, 2026-06-09)

Confirmed fixed since the source reports (no further action needed):

- **B1** forward `set` now mutates in place (both forms; returns nothing).
- **B2b** trailing `(expr) print` at EOF no longer raises.
- **B4** `export`-using library now runs directly with exit 0.
- **B6** `indexof` is haystack-last (`StringUtil.indexof needle haystack`;
  list form is `ArrayUtil.indices`).
- **B7** failing `Test.test` cases print a named `FAIL` line.
- **T4** `do {k:["if"]}` stores the string; the string→word promotion
  is gone.
- **T7** `node`, `eq`, single uppercase letters (`L`) — all usable as `def`
  bindings; only `end` remains reserved (by design as a statement
  terminator).
- **T8** string interpolation inside a recursive fn body expands
  correctly (the raw-template-AST leak from the first pass is gone).
- **T9.3** user-defined word followed by another word collects cleanly.
- **T9.5** print order matches source order.
- **popcount** landed as `BinUtil.popcount` (with `clz`/`ctz`/`bitlen`/…).

Mitigated (real change landed; residual gap remains):

- **T1** — runtime dispatch still silently leaves the fn value as data,
  but `aql check` now reports it as an `uncalled_function` **error**.
- **T3** — `merge` moved out of core into `StructUtil` (still deep,
  index-wise); `StructUtil.setpath` is the one-field-update alternative.
  Residual work is docs.
- **with/assoc** — capability exists as `StructUtil.setpath`; only the
  idiomatic name/docs are missing.

Confirmed still open:

- **B2a** chained-print reversal, **B3** void-call residue, **T9.4**
  mixed-form `if` — none flagged by `aql check` either.
- **B5** `make Object {}` error still lacks a hint.
- **T9.1, T9.2** map computed keys / filter quotation.
- **T9.6 `raise`, T9.7 in-memory `parse`, insert-at/remove-at** — still
  missing.

Reclassified as docs / by design:

- **T2** `fold` `[elem acc]` binding order is intentional under the
  unified §1.4 sig-order rule — documentation work, not a code fix.
- **T5** `eq` identity on compounds is intended; structural `deq` now
  exists and the distinction is documented.
- **T6** bare vs parenthesised `get` keys are now *defined* JS-equivalent
  semantics (literal `.key` vs computed `[expr]`), documented in
  REFERENCE.md.

Unverified (need a tighter repro):

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

Most of the lost hours in both reports trace to **two** mechanisms.
Status after the second pass:

1. **Namespace dispatch type-mismatch must error, not no-op.** (T1 —
   🟠 **half done.** `aql check` now errors with `uncalled_function`;
   the runtime still leaves the fn value as data. B1, which looked like
   the same mechanism, turned out separable and is ✅ fixed: `set` now
   mutates in place in both forms.) Remaining: make the *runtime* loud,
   or accept check-time-only and promote `aql check` hard in the docs.
2. **`get` on a bare undefined word should error, not return `none`**
   (T6 — 📖 **resolved by design instead**: bare keys are literal
   (JS `.key`), parenthesised keys are computed (JS `[expr]`), and the
   `None`-for-missing behaviour is documented as intentional.)
3. **A "loud diagnostics" pass for the common silent shapes** —
   🟠 **partially landed**: `aql check` gained `uncalled_function`,
   the `forward_strands_operand` advisory (catches `1 2 add 3 mul`),
   and dead-overload detection; T4 was fixed outright. Still silent at
   check time and runtime: B3 void-`def` residue, B2a print reversal,
   T9.4 mixed-form `if`.

### P1 — A small set of new primitives that retire workarounds

1. ~~**Shallow field update word `with` / `assoc`**~~ (T3) —
   ✅ capability landed as `StructUtil.setpath`; remaining is a docs
   pointer (and optionally an `assoc`/`with` alias).
2. **`raise` / `throw` with a message** (T9.6). ❌ still missing.
3. **In-memory jsonic `parse` / `decode`** to complement `jsonify`
   (T9.7). ❌ still missing.
4. **`filter` accepts a `[…]` quotation** like `each`/`fold` (T9.2).
   ❌ still open (filter now also takes a Reach lens, but not a
   quotation).
5. ~~**Native `popcount`**~~ ✅ landed (`BinUtil.popcount`) +
   **`insert-at` / `remove-at`** for lists (HAMT Level A) ❌ still
   missing.

### P2 — Docs (cheapest single batch)

A one-page "Gotchas" / "Idioms" reference covering the items that are
**still load-bearing** after the second verification pass:

- Argument order = **reverse** of call order (T1) — partially covered:
  `aql describe <word>` now prints precedence + equivalence forms, and
  REFERENCE.md documents the `/s` / `/f` / `/N` modifiers; a
  consolidated user-facing page is still missing.
- `fold` body binding order (T2 — still not stated in `describe fold`
  or REFERENCE.md; note the natural `[push]` body now works under lazy
  resolution, so re-derive the doc from current behaviour).
- `merge` is deep, index-wise — point one-field edits at
  `StructUtil.setpath` (T3).
- ~~`do {k:[v]}` evaluates value quotations~~ (T4) — fixed, drop it.
- ~~`eq` on lists is identity~~ (T5) — documented, `deq` exists; done.
- ~~`xs get i` vs `xs get (i)`~~ (T6) — documented (literal vs
  computed keys); done.
- `make Object {}` requires a `refine Object` subtype or `{…}` literal
  (B5) — still needed (or fix the error hint, which is cheaper).
- ~~`indexof` is haystack-first~~ (B6) — fixed; done.
- Promote the declarative spec test API (`Test.spec` / `Test.run-spec`)
  into the user docs — still missing from REFERENCE/TUTORIAL/HOWTO.
- **Upgrade note** for downstream voxgig-style libraries: namespaces are
  CamelCase (`Test.`, `Assert.`, `MathUtil.`), `aql:array` is now
  `aql:array-util` (binds `ArrayUtil.`), string words live in
  `aql:string-util`, `merge`/`setpath` in `aql:struct-util`, and module
  fn exports use the `name/r` referent form.

Items dropped from the original gotchas list because they're now fixed:
T4 do-evaluates-words, T5 eq identity (documented + `deq`), T6 get-key
forms (documented), B6 indexof direction, T7 reserved-name false
positives, B4 export-only-import surprise.

### P3 — Test-framework UX

- ~~**Name the failing case** in `test.test` output~~ (B7) — ✅ done:
  failing cases print `FAIL <name> — <assertion detail>`, with
  `Test.describe` path included.

### P4 — Runtime/capability (HAMT Level B — explicit, not urgent)

If/when a HAMT-class persistent map is on the roadmap: mutable unboxed
fixed-width arrays with transients, or ship a native HAMT/CHAMP map type. As
a bonus, the native map would retire AQL's dynamic-key-map limitation (T9.1)
and turn association-list workarounds back into idiomatic maps.

---

## Cross-cutting suggestion

Both reports independently arrive at the same diagnosis: **forward-arg
collection** is responsible for an outsized share of confusion (B2a, B3, T6,
T8, T9.3, T9.4). The structure-first **lazy forward-argument resolution**
engine (`6687638`) plus the literal-key `get` semantics have since
retired most of this group: T8 and T9.3 are fixed and T6 is now defined
behaviour. What survives is B2a (chained-print reversal), B3
(void-`def` residue), and T9.4 (mixed-form `if`). Two follow-ups still
worth doing:

1. Make **`print` stack-first by default** — the bloom report explicitly
   suggests this. The chained `(expr) print (expr) print` reversal (B2a)
   remains the one item in this group fixable without touching the
   dispatch model — `print` is the only word where forward collection
   actively hurts readability. (The `/s` modifier exists, so this may
   now be a one-line default change: `print` ≈ `print/s`.)
2. A short user-facing **forward-vs-stack arity reference**. Partially
   done: REFERENCE.md documents the `/s` / `/f` / `/N` modifiers and
   `aql describe` prints per-word precedence with equivalence forms.
   The lang/go CLAUDE.md "Argument Ordering (CRITICAL)" section remains
   the authoritative source; a one-page distillation in the user docs
   would close the remaining discoverability gap.

## Remaining work, classified (added 2026-06-09)

**Quick wins — small, contained code changes:**

| Item | Why it's small |
|------|----------------|
| B2a print reversal | Make `print` stack-first by default; the `/s` machinery already exists |
| B5 `make Object {}` hint | Error-message text only — append "use a `refine Object` subtype or a `{…}` literal" |
| T9.6 `raise` | One new word: build the error value and return it as an `*Error` |
| T9.2 filter quotation | Alignment with `each`/`fold`, which already accept quotations — add the `(List, Any)` overload |
| insert-at / remove-at | Two small list primitives over existing copy-returning list ops |
| `assoc`/`with` alias | Alias (or re-export) of the existing `StructUtil.setpath` |
| T9.7 in-memory `parse` | jsonic is already linked (jsonify exists); expose the decode direction |
| B3 void-`def` residue | Loud-error path: `def x (expr)` where expr yields no value should raise "def: expression produced no value" instead of letting the next word mis-dispatch |

**Documentation gaps — no engine change needed:**

- T2 — state `fold`'s body binding order in `describe fold` + REFERENCE.md.
- T3 — gotcha note: `merge` is deep/index-wise; one-field edits should
  use `StructUtil.setpath`.
- T1 (residual) — document that `aql check` catches silent namespace
  dispatch misses (`uncalled_function`), and recommend running it.
- B1 (residual) — document `set`'s in-place, no-return-value contract.
- Promote `Test.spec` / `Test.run-spec` into user docs.
- Upgrade note: CamelCase namespaces, `-util` module renames,
  `name/r` referent exports.
- A consolidated forward-vs-stack one-pager (distil CLAUDE.md
  "Argument Ordering").

**Real engine work (neither quick nor docs):**

- T1 at runtime — making namespace-word dispatch misses loud at run
  time without breaking higher-order use of fn values as data.
- T9.4 mixed-form `if` — forward/stack mixing in one call.
- T9.1 computed map keys — properly retired by a native persistent map
  (P4 / HAMT Level B).
