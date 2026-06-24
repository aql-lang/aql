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

## Progress — L1a LANDED; L1b's real blocker isolated

**L1a (computed-map provenance) — landed.** `autoEvalMap` now records an
`OpMakeMap` assembly for **any computed map literal consumed in-frame** (a
word/fn arg), not just `make`'s construction body (the old `dataMap`-only gate).
So a `{k:(expr)}` bound to a value-def local and returned, or read by a
downstream word, compiles instead of refusing "body result of unknown
provenance". Gated on in-frame **consumption** (`consumed` flag threaded from the
arg-evaluation callers): a DEFERRED residual — a bare computed-map fn-body tail,
auto-evaluated by `autoEvalStack` after its frame pops — must still refuse,
because the interpreter evaluates it late (its param bindings gone) and
compiling it in-frame would diverge. Verified: `make verify-bytecode` green
(differential + property + race + aqldebug), full `eng/go` / `lang/go` / `cmd/go`
suites green, neutral on the langspec census. Pinned by
`lang/go/bytecode_computedmap_test.go` (positive: byte-identical to the
interpreter; negative: the deferred-residual tail still refuses, no divergence).
`COMPILABLE-SUBSET.md §3` updated.

> Pre-existing note: `test/go/langspec/TestOnlyMetaFallsBack` (a tier-2 ratchet,
> NOT part of `verify-bytecode`) fails on clean `main @ 407fedad` with identical
> numbers with/without L1a — a stale `reducibleCeiling`, independent of this work.

**L1a already compiles the PAREN form of `do {…}`.** A bonus the widening
delivers directly: `do {n:(bf.n)}` (paren value quotations) now **compiles** —
its values evaluate in-stream (`evalParenExprResults`), `RecordMakeMap` records
the map, and `do`'s `Any` return over a now-resolved concrete map arg falls under
the `dynOutNativeOK` exemption, so no output-type change to `do` is needed.
Verified against `--force-compile`.

**L1b-ii (list quotation form) — LANDED.** The client libs write `do {n:[bf.n]}`
(the **list** form — `bloom.aql:271`, `decision.aql:159`, `trie.aql:275`). A
list-valued map entry keeps its value AS a list (`{n:[3]}`); `do` evaluates each
list later. The value list's elements DO carry provenance (their dispatches
recorded during autoEvalMap), but the list WRAPPER was never recorded —
`RecordMakeList`'s top-frame guard refuses a fn-body list. Fix: record the list
wrapper as a nested `OpMakeList` **inline in autoEvalMap**, right after each
value's dispatches, so the events interleave in stack order
(`get_n, wrap_n, get_m, wrap_m, OpMakeMap`) — wrapping afterward in
`RecordMakeMap` finds the next value's result on top of the stack and fails the
stack-discipline check. The inner assembly (`recordMakeListInner`, extracted from
`RecordMakeList`) skips the top-frame guard because this list is a CONSUMED
operand of the enclosing in-frame `OpMakeMap` — `OpMakeList` re-assembles from its
operands per run, so it never freezes a per-call binding. Gated on the same
in-frame `consumed` flag, so a deferred-residual list still refuses. No `do`
change needed: once the map arg resolves, `do`'s `Any` return falls under
`dynOutNativeOK`. Verified: `make verify-bytecode` green, full `eng/go` /
`lang/go` suites green, neutral on the langspec census; pinned by the list-form
positive + negative rows in `lang/go/bytecode_computedmap_test.go`. The client
`do {…}` blocker is cleared (those suites now advance to their next, unrelated
refusal). `COMPILABLE-SUBSET.md §3` updated.

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

**Step 2 — L1b: `do {…}` over a map body.** Two coupled problems, both real:
(i) `do`'s Map sig declares `Returns: [TAny]` (native_control.go:30), so its
compiled output is a dynamic-Any carrier (emit.go:2004); (ii) `doMapHandler`
evaluates each value code-list in a **sub-engine** (`doEvalDataList`→`New(r)`).
At interpreter runtime the sub-engine shares the registry, so a fn param (`bf`
in `do {n:[bf.n]}`) is visible; in the compiled path that param is a **VM frame
slot**, not a registry binding, so a const-bake-and-rerun would look `bf` up in
`r.Defs`, miss it, and raise `undefined_word`. **That is why the current refusal
is sound** — and why fixing (i) alone (a Map `ReturnsFn` returning the concrete
Map type) is **not sufficient and would be unsound on its own**: each map-value
quotation must be lowered as a **closure with capture** (the same machinery
`each`/`fold` bodies already use), so frame locals resolve through the VM seam,
*and then* the output typed as a concrete Map. Do not attempt the output-type
tweak without the closure lowering. Unblocks `bloom_smoke`, `bloom_unit_spec`,
`tst_unit_test`, `burst_unit_test` and the `do{}` leaves inside `test-test`
bodies.

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

---

## Session-2 status (2026-06-24, branch `claude/aql-client-lib-issues-lev8cs`)

Landed since the note above (all gated: `make verify-bytecode` + full
`eng/go`/`lang/go` suites green, no divergence):

- **L1a** — computed map literals consumed in-frame record `OpMakeMap` (paren
  form `do {k:(expr)}` now compiles). Commit `a730e7b`.
- **L1b-ii** — list-valued map entries (`do {k:[expr]}`) record a nested
  `OpMakeList` inline in `autoEvalMap`. Commit `d9b41a6`. **This closes the
  entire `do {map}` refusal class** (brief refusal class #2): bloom.aql:271 and
  :286 (`do {n:[bf.n] …}`) now compile.
- Ratchet `TestOnlyMetaFallsBack` fixed (soundness-frontier rows attributed to
  the compute frontier, not tier-2). Commit `d633623`.

### Live client-suite refusal map (this branch's build, 21 suites)

| reason | suites | project |
|---|---|---|
| `code-body word each (Stage 2)` | 7 | A |
| `check diagnostics` | 4 (radix_unit, trie_smoke, decision_smoke, tst_unit) | B |
| `code-body word test-test` / `test-check-prop` | 3 | A |
| `unmatched dispatch recovered at mk-node` | 2 (trie_unit_{spec,test}) | A-adjacent |
| `unannotated or opaque word do` | 2 (bloom_smoke, bloom_unit_spec) | A (the `do [body]` form — NOT the map form, which is now compiled) |
| `dynamic input at push` | 1 (burst_unit_test) | ? |
| **OK** | 2 (decision_prop_test, bloom_prop_test) | — |

### Precise decline points found (Project A `each`/code-body)

The `each` refusal is NOT "all code bodies." Empirically:
- `each [dup mul]` (simple), `each [var [[i] … end]]` (var-body, NO capture),
  `each [xs swap get]` (capture, NO var), and nested `each` all **compile**.
- The refusing shape is specifically **a `var`-body that references an
  enclosing binding** (bloom's `iota m each [var [[i] bits bit-test i end]]`,
  where `bits` is a fn-body local). Two distinct decline points:
  1. **Global var-ref** (`def xs … each [var [[i] xs i get end]]`): reaches
     `compileClosureBody` and the body analysis fails with
     `undef: expected fn undef spec, got dynamic(Any)`
     (`native_definition.go:487`) — the `var`-splice cleanup (`undef i`) in a
     closure body analysed with a dynamic ref present mis-resolves the undef
     spec. A var-splice/closure-analysis bug, not a missing feature.
  2. **In-fn capture** (`def f fn [[xs] … [ each [var [[i] xs i get end]] ]]`):
     declines BEFORE `compileClosureBody` runs — i.e. in `tryRecordClosure`/
     `recordClosureDispatch` capture resolution (`callable_words.go:219-226`)
     or the `var`-body capture walk. The capture of an enclosing local through
     a `var`-splice body is not threaded.

  Fixing **the var-splice-body closure with captures** (one capability) is the
  "build it once" core the brief names; `each`/`fold`/`scan`/`filter` share it,
  and `test-test`/`test-check-prop` (the framework drivers) very likely fall out
  with it. Verify (1) and (2) as two sub-fixes of that one unit.

`do [body]` (bloom.aql:310/318, `do [StructUtil.parse text] error […]`) is the
LIST-body code form (with an `error` handler closure) — same code-body cluster,
distinct from the now-compiled `do {map}`.

### Not yet diagnosed (separate units)
- `unmatched dispatch recovered at mk-node` — trie's `mk-node` builds a typed
  node; the checker recovered an unmatched dispatch (a dispatch-recovery, not a
  do-map issue). Needs its own trace.
- `dynamic input at push` (burst_unit_test) — a dynamic operand reaches `push`.
- **Project B** (`check diagnostics`, 4 suites) — unchanged; still the
  hermetic-help + construction-check + corpus-rebaseline unit from
  `module-fn-checkstate-ownership.6.md`. Higher risk; land separately.

**Recommendation:** the next unit is the var-splice-body-with-captures closure
(decline points 1 + 2 above) — highest-frequency single blocker (7 suites),
no corpus calibration. It is genuine Stage-2 emitter work, not a leaf, so it
warrants its own branch + full re-gate rather than being rushed.
