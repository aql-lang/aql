# Closing `--force-compile` for the client libraries — findings & plan

**Date:** 2026-06-24
**aql:** `main @ 407fedad` (built from source)
**Scope:** make `aql --force-compile` succeed (no refusal, byte-identical to the
interpreter) on every test suite of `voxgig-aql/{trie,decision,bloom-filter}`.
**Status:** investigation + sequenced plan. The baseline gate
(`TestSpecCompiledDifferential|…|TestPropertyDifferential`) is **green** at this
commit. No emitter change is landed in this note — it scopes the work and
**corrects the mental model** the driving brief was written against.

Required reading first: `design/COMPILABLE-SUBSET.md` (the contract + refusal
taxonomy), `design/aql-bytecode-completion.0.md` (the Stage-2 cluster),
`design/module-fn-checkstate-ownership.{5,6}.md` (the dynamic-help artifact),
`design/CLIENT-VERIFICATION-MAIN-2026-06-24.md` (the per-suite refusal ledger).

---

## 1. What I verified (and how the model changes)

Building `main @ 407fedad` and reducing every distinct refusal across the 21
client suites to a minimal repro turned up three facts that **revise** the
brief's Project-A framing. These are reproducible with
`aql --force-compile <file>`:

| Repro | Result | Implication |
|---|---|---|
| `[1 2 3] each [var [[x] x 1 add]]` | ✅ **compiles** → `[2,3,4]` | Simple `var`-block `each`/`fold`/`filter` bodies **already lower** (the closure path handles them). |
| `0 [1 2 3] [var [[x acc] acc x add]] fold` | ✅ **compiles** → `6` | Same — `var`-spliced locals already become frame slots. |
| `[ 1 1 Assert.equal ] "t" Test.test` | ✅ **compiles** | `Test.test` (and `test-check-prop`) are **not** the blocker. Their handlers already branch on `IsCompiledClosure` and drive the body through `InvokeBody` (lang/go/modules/test.go:294). A trivial body compiles. |
| `[ (do {x:[1 1 add]}) {x:2} Assert.equal ] "t" Test.test` | ❌ `code-body word test-test (Stage 2)` | The refusal is the **leaf** `do{}` inside the body, **not** `test-test`. The outer code-body word just reports the generic reason when its closure probe (`tryRecordClosure`) declines because something in the body didn't lower. |
| `{x:(1 add 2)}` (top level) | ✅ **compiles** → `{"x":3}` | Computed map literals lower at top level (`RecordMakeMap`→`OpMakeMap`, emit.go:2198). |
| `fn [[a:Integer][Map][ def m {x:(a 1 add)} m ]]` | ❌ `fn f: body result of unknown provenance` (emit.go:1442) | A computed map built in a fn (value from a **frame-local**), bound to a local and returned as the **body result**, loses provenance. (NB: the *inline-tail* `[… {x:(a 1 add)}]` is not a valid target — a data-context paren doesn't see fn params, so it errors at runtime; the `def m … m` and `do {…}` forms are the real ones.) |
| `fn [[bf:Map][Map][ do {n:[bf "n" get]} ]]` | ❌ `unannotated or opaque word do` (emit.go:2004) | The real client form (`bloom.aql:271`, `decision.aql:159`, `trie.aql:275`): runs `{"n":…}`, but `do` is declared `[Map] Any` (`aql describe do`), so its compiled **output** is a dynamic-Any carrier and the dynamic-output gate refuses it — even though the runtime value is a concrete Map. |
| `[1 2] each [var [[x] {v:[x]}]]` | ❌ `check diagnostics` | The dynamic-help example generator (`module-fn-checkstate-ownership.6.md`) runs the body against a synthetic `{a:1,b:2}` at registration and leaks emit-gating diagnostics — **not** a real check error (`aql check` is clean). |

**Corrected model.** The remaining `--force-compile` refusals across all 21
suites reduce to **three leaf causes**, not the surface words the reasons name:

- **L1 — computed map literals in non-top-level provenance.** A `{k:[expr]}`
  whose values come from fn frame-locals / loop iterators, and especially one
  **returned as a fn body result** or **wrapped in `do {…}`**. The assembly
  opcode exists; the gaps are (a) body-result provenance for an `OpMakeMap`
  result (emit.go:1442) and (b) `do {…}` modelling its output as a concrete Map
  rather than a dynamic carrier (emit.go:2004). This is the dominant blocker —
  it sits inside the `test-test` / `each` / `do` / `unit_spec` refusals.
- **L2 — the dynamic-help `check diagnostics` artifact.** Surfaces on
  `each`/`filter` over a computed-map body and on the `*_smoke` suites. Aligning
  the compile-path check with standalone `aql check` (which is clean) removes
  the *misleading* reason; it does not by itself make a suite green (the L1 leaf
  underneath still refuses).
- **L3 — trie `mk-node` gradual dispatch** (`unmatched dispatch recovered at
  mk-node`): the chained-`set` node builder dispatched over a gradual carrier.
  Narrow; trie-only.

Everything the brief filed under "Project A code-body lowering (each / do /
test-test / test-check-prop)" is **downstream of L1** (plus L2). The
code-body/closure machinery itself is already in place.

---

## 2. Why this is not a quick land (the calibration wall)

`--force-compile` is the carrier checker with a recording side effect; the
langspec corpus (`test/go/langspec`, ~2830 rows) is **calibrated to the exact
set of rows that currently compile, island, or refuse**, with hard ceilings and
per-row tier expectations (`TestCompiledCoverage`, `TestOnlyMetaFallsBack`,
`TestCompiledStatus`). Any change that makes a new construct lower **reclassifies
rows** and trips those pins even when the new lowering is correct and
byte-identical — exactly the failure mode `module-fn-checkstate-ownership.6.md`
measured for three separate partial fixes. So each leaf below is a *widening +
deliberate re-baseline*, landed as its own reviewed unit with
`COMPILABLE-SUBSET.md` updated in lockstep — not a one-line gate flip.

The soundness floor is non-negotiable: const-pool mutation safety
(`isInertConst` must never admit a mutable `Array`/`Object`/`Store` — §4 of the
contract, pinned by `bytecode_constbake_test.go`) and `compile == interpreter`
(the differential + property gates). A computed-map widening (L1) touches an
assembled-Map result that must **not** enter the const pool.

---

## 3. Sequenced plan

Ordered by leverage and ascending risk. Each step ends with `make
verify-bytecode` green + a reviewed corpus re-baseline + a
`COMPILABLE-SUBSET.md` update.

**Step 1 — L1a: computed-map body-result provenance.** Thread the `OpMakeMap`
result's provenance so a fn body returning `{k:[local]}` resolves (emit.go:1442
sees a known operand instead of "unknown provenance"). Pin the assembled map's
mutation-safety: it is a fresh per-run instance (like the existing top-level
case), so it must stay out of `isInertConst`. Adds the most suites — it sits
inside `unit_spec` bodies and the `each`/`fold` transforms.

**Step 2 — L1b: `do {…}` concrete-Map output.** Make `do` over a map body model
its output as the body's concrete Map type rather than a dynamic carrier, so the
emit.go:2004 dynamic-output gate passes (the `dynOutNativeOK` exemption is the
template). Unblocks `bloom_smoke`, `bloom_unit_spec`, `tst_unit_test`,
`burst_unit_test` and the `do{}` leaves inside `test-test` bodies.

**Step 3 — re-verify the code-body words.** With L1 closed, re-run the suites:
`test-test` / `test-check-prop` / `each`-bodies that only refused because of an
L1 leaf should now lower (their handlers are already closure-ready). Expect a
large batch of suites to flip green here with **no new emitter code** — only the
corpus re-baseline.

**Step 4 — L2: dynamic-help hermeticity (the `module-fn-checkstate-ownership`
project).** Make the help-example eval hermetic (snapshot+restore diagnostics so
its synthetic-arg dispatch failures never reach the emit-gating set) **and** add
the first-class construction-check pass that currently rides on the eval's
side-channel (retains `TestCheckUncalledFnBodyTypoStillFlagged` /
`TestForwardStrandAdvisory`), then re-baseline. Per `.6.md` this is coverage-safe
as isolation (option b) but needs the construction-check to keep the two pinned
behaviours — land them together. Clears the `*_smoke` and `each`/`filter`
`check diagnostics`.

**Step 5 — L3: trie `mk-node` gradual dispatch.** The narrowest, trie-only;
do last. Lower the chained-`set` builder dispatched over a gradual carrier (or,
client-side, the trie report's §7 already shows the `do {…}` form is
interchangeable — once L1b lands, the idiomatic `do{}` `mk-node` compiles, so L3
may dissolve into L1b).

**Client repos:** no source change expected. As each step lands and a suite
reaches zero refusals, promote that suite's advisory `--force-compile` CI step
to a hard gate (the divergence harnesses already auto-detect the compilable
subset).

---

## 4. Recommendation

This is real multi-step compiler work on a deliberately-calibrated subset, and
the brief's own ordering (Project A before the higher-risk help-eval project) is
right — but the actual Project-A surface is **L1 (computed-map provenance)**, not
the code-body words, which are already wired. The responsible path is to land
L1a → L1b → re-baseline as the first reviewed unit (it should flip the majority
of suites), then L2, then L3 — each gated by `make verify-bytecode` and a
lockstep `COMPILABLE-SUBSET.md` update, rather than a single sweeping change. I
did **not** ship a speculative widening here: a partial emitter change that
reclassifies the corpus without the paired re-baseline is exactly the regression
shape the design record warns against, and "all gates green + zero new
divergence" must be demonstrated per step, not assumed.

## 5. Reproduce the inventory

```bash
REF=407fedad2ea2b30c3dde2f29cfbe60e55f94db4e
mkdir -p /tmp/aql && curl -fsSL "https://codeload.github.com/aql-lang/aql/tar.gz/$REF" \
  | tar -xz -C /tmp/aql --strip-components=1
( cd /tmp/aql/cmd/go && GOFLAGS=-mod=mod go build -o /tmp/aql-bin ./aql )
p(){ printf '%s\n' "$2" >/tmp/r.aql; printf '%-40s -> ' "$1"
     /tmp/aql-bin --force-compile /tmp/r.aql 2>&1 | grep -o 'force-compile:.*' || echo COMPILES; }
p "each var-block"  '[1 2 3] each [var [[x] x 1 add]] print end'
p "Test.test trivial" 'import "aql:test" end ([ 1 1 Assert.equal ] "t" Test.test) end'
p "do computed map"  'def f fn [[a:Integer] [Map] [ do {x:[a]} ]] (f 5) print end'
p "map body result"  'def f fn [[a:Integer] [Map] [ {x:[a]} ]] (f 5) print end'
```
