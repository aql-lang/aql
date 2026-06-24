# Client-library fixes — voxgig-aql trie / decision / bloom-filter

**Date:** 2026-06-24
**Upstream branch:** `claude/aql-client-issues-6b8new` (aql-lang/aql)
**Driven by:** the three client verification reports —
[`trie/AQL-MAIN-VERIFICATION.md`](https://github.com/voxgig-aql/trie/blob/main/AQL-MAIN-VERIFICATION.md),
[`decision/aql-main-verification-2026-06-23.md`](https://github.com/voxgig-aql/decision/blob/main/aql-main-verification-2026-06-23.md),
[`bloom-filter/aql-backend-report.md`](https://github.com/voxgig-aql/bloom-filter/blob/main/aql-backend-report.md).

This note tells each client project **what changed upstream, what is now
fixed, what remains, and how to re-pin.**

---

## Upstream fixes landed

| Commit | Fix | Clients it unblocks |
|---|---|---|
| `f247557` | **`None` / type-literal template interpolation.** `` `got ${x}` `` with a `None` (or `typeof`) value rendered `"String"` and corrupted `raise` codes (`raise_error` instead of `bad_input`). | bloom-filter §1 (the `make-validates-input` interpreter failure) |
| `f247557` | **`convert` return-type modeling.** `convert Float x` modeled its result as the bare `Float` *type literal*, so downstream `mul`/`div`/`add` raised a false `no_signature`. Now returns a *value* of the target type (like `make`). | bloom-filter §2, decision (`convert` in `decision.aql`) |
| `1b7b9ae` | **`OpInterp`.** Interpolated strings with runtime-computed holes (`` `got ${x}` `` for a fn param, `` `${1 add 2}` ``, `` `${typeof x}` ``) now compile to bytecode instead of refusing. | all three (smoke/print paths) |
| `a0604d7` | **`fold [push]` false positive.** `push`'s `[Any List]` overload declared no `Returns`, so a fold accumulator widened to `Any` and the next `push` failed `no_signature`. | decision (`no_signature: push`), trie |
| `a0604d7` | **`for` multi-value-body miscompile.** `for 2 ['e' 'f']` compiled to `[f f]` instead of `[e f e f]`; now islands faithfully. | (correctness, all) |
| `fc47452` | **fold body element carrier.** A fold over a `List<Any>` (e.g. `convert List <Array>`) typed its element as a *strict* `Any`, so a body word (`add`, `BinUtil.popcount`) failed. Now uses a dynamic carrier like `each`/`filter`. | bloom-filter smoke (the `popcount` fold) |
| *(gradual-Any)* | **explicitly-`Any` params and returns become gradual (dynamic) carriers.** A value of static type exactly `Any` (a `:Any` param, a `[Any]`-returning helper) is "type unknown", not "the Any root" — it now poly-matches a concrete slot instead of failing `no_signature`. This is the "unify Any with concrete params" wishlist item. | trie (node walkers — `get`/`find-kid`/`trie-insert`), decision |

All land with the full upstream suite green (`make fmt && make vet && make
lint && make test`), and a new verified all-words/structures corpus
(`lang/spec/corpus-*.tsv`, ~407 rows) that gates interpret + check + compile
across the whole language surface.

---

## Per-client status (after the fixes)

Measured from each repo root (so the relative `import "./*.aql"` resolves).

### bloom-filter — ✅ fully clean

| Suite | interpret | check | compile |
|---|---|---|---|
| `bloom_unit_test`  | ✅ | **0** | ✅ |
| `bloom_unit_spec`  | ✅ | **0** | ✅ |
| `bloom_prop_test`  | ✅ | **0** | ✅ |
| `bloom_prop_spec`  | ✅ | **0** | ✅ |
| `bloom_smoke_test` | ✅ | **0** | ✅ |

All five suites now interpret, check (zero errors), and compile cleanly. The
§1 interpreter regression and §2 `no_signature` false positives are gone.

**Action:** adopt this upstream and move the `test/divergence/` pin off
`c44d994` once the branch merges. No library changes needed.

### decision — ⚠️ 4 / 5 clean

| Suite | check |
|---|---|
| `decision_unit_test` / `_spec` | **0** |
| `decision_prop_test` / `_spec` | **0** |
| `decision_smoke_test` | 16 (residual, see below) |

The unit/spec/prop suites are clean (the `push` / `convert` fixes). The smoke
suite's residue is the **dynamic-dispatch + namespace class** (below).

### trie — ⚠️ interpret green; check sharply reduced

Interpreting is fully green (all 11 suites). The gradual-`Any` change cut the
check error counts sharply:

| Suite | before | after |
|---|---:|---:|
| `trie_prop_test`  | — | **0** |
| `trie_unit_test`  | 17 | **5** |
| `trie_unit_spec`  | 25 | **5** |
| `trie_prop_spec`  | — | 6 |
| `burst_unit_test` | 25 | **6** |
| `radix_unit_test` | 66 | **31** |
| `tst_unit_test`   | 70 | ~31 |
| `*_smoke_test`    | high | high |

The residue is two narrower cases the gradual-`Any` change doesn't reach
(emergent in the full transitive analysis, not reproducible in isolation): a
`set` over a dynamic node and a `do {…}` map body referencing a same-named
param (`kids`) — the trie report's "resolve `do{}` quotation params" item — plus
the namespace-exposed-word noise the smoke suites carry at top level.

---

## The big lever — explicitly-`Any` dispatch (now fixed)

The dominant class was **dispatch on an explicitly-`Any`-typed value**: a `:Any`
param or a `[Any]`-returning helper bound a *strict* `Any` carrier, and a word
needing a concrete receiver (`get`, `add`, a user walker like
`find-kid`/`trie-insert`) failed `no_signature` against it — even though it
dispatches fine at runtime.

```aql
def g fn [[v:Any] [Any] [v get "x"]] end   g {x:1}   # was: no_signature: get  →  now: clean
```

This is the trie report's wishlist item **“unify `Any` with concrete params”**.
It is now fixed (the gradual-`Any` change): an explicitly-`Any` param/return
binds a **dynamic (gradual)** carrier — the same treatment `each`/`filter`/`fold`
give untyped list elements — so dispatch poly-matches. The full upstream suite
(check accuracy, type soundness, differential, the new corpus) stays green.

## The remaining residue

Two narrower families are left, both deferred (they need their own focused
pass, and neither reproduces in isolation — they are emergent from the full
transitive analysis):

1. **`do {…}` quotation params and namespace-exposed words.** `undefined_word`
   on a `do {…}` map body that references a same-named param (trie's `kids`),
   and on a type/word only known through an export (decision's `DTable`,
   `eval-table`/`decide` left as `uncalled_function`). The static checker does
   not yet thread these.
2. **`set` over a freshly-dynamic node** inside a deeply nested walker
   expression — a downstream consequence of the gradual carriers that the
   poly-record path does not yet cover for the mutating `set`.

## Checker `unused_def` on reference-exported words (landed)

The client check reports' largest *warning* category — the trie report's
wishlist item **#1, "trace `/r` reference-exports as usages"** — is now fixed.
A word bound into an export map by reference (`export "X" { make: impl/r }`,
the canonical public-API form) or as a bare value (`{ Color: Color }`) was
flagged `unused_def` precisely *because* it was public: the use-tracker counted
only dispatch/`Lookup` uses, not a reference resolution. The fix records the use
at the single resolution chokepoints:

- `ResolveRef` (`eng/go/core_ref.go`) — covers `name/r`, the `ref` word, and
  in-map reference values uniformly.
- The collecting export handler's `resolveModuleExport` and the top-level
  no-op `export` handler (`lang/go/native`) — a standalone module file
  (`aql check trie.aql`) reaches the no-op, which now records each export-map
  value as a use of its def in check mode.

Effect on the clients (modules + suites): `unused_def` warnings fell from **133
to 10** — `trie.aql` 31→0, `burst.aql` 32→0, `decision.aql` 14→3, `bloom.aql`
9→1. The fix is sound by construction (a genuinely unreferenced def is still
flagged — pinned by `lang/go/unused_def_export_test.go`'s negative half) and
leaves the checker's error-frontier ratchet untouched. The residue is body-
local defs reassigned/read inside a `for` body (`found`, `best-pri`, `midkids`)
plus a const-fold ordering quirk on certain 0-arg-fn export combinations —
both emergent (not reproducible in isolation), the same wall the families below
hit.

## Checker error-level false positives — why they stay (the Any frontier)

The error-level diagnostics (`no_signature`, `undefined_word`,
`uncalled_function`) on this generic, dynamically-dispatched code are **not** a
severity bug to demote away. The dominant `no_signature … assuming best-fit
candidate for analysis` is the deliberately-tracked **Any frontier**
(`test/go/langspec/check_accuracy_test.go::pinnedAnyFrontierRows`): a dispatch
whose operand is statically `Any` (a value pulled from `get`, an untyped node
walked recursively). The maintained methodology *shrinks* that frontier by
improving **precision** (flow-narrowing `get`, declaring core-word `Returns`,
unifying `Any` with concrete params — the gradual-`Any` change above was one
such step) while keeping it visible at **error**, not by demoting the severity
(which would hide it). The remaining false positives are emergent from the full
transitive analysis — they do not reproduce in isolation — so they need that
focused precision pass, exactly as the two residue families above. Demoting
severity was considered and rejected on this basis.

### Client-side options until that lands

1. **Keep the current pins** (`c44d994` for the bytecode-capable reference)
   for `trie` and the `decision` smoke suite — interpreting is the supported
   path and is fully green.
2. **Adopt the branch for the now-clean suites.** bloom-filter (all suites)
   and decision's unit/spec/prop suites check clean and can drop their
   `--soft` / `continue-on-error` gates.
3. Where a library function takes an untyped node, a **concrete annotation**
   (`nd:Map` instead of `nd:Any`) checks clean today (see the `find-kid`
   contrast above) — narrowing the public signatures where the shape is
   actually known removes the false positives without waiting on upstream.

---

## Compilation (`--force-compile`) note

`OpInterp` (`1b7b9ae`) removed the interpolated-string refusal, so suites that
previously stopped there now advance. The remaining `--force-compile` refusals
are the test framework's **code-body words** (`each`, `test-test`, `do` at
“Stage 2”) — the tracked emitter-coverage work (decision report §6). `--compile`
(silent interpreter fallback) produces correct output for every suite.

### Residual emitter fix — raise-arm divergence (landed)

A focused emitter false-refusal in this set is now fixed: a fn whose `if`-chain
bottoms out in `raise` (the **`apply-op`** shape — an op-dispatch chain whose
default branch raises) was wrongly flagged **variadic-returning**. The variadic
accounting (`branchVariadicResult` / `eventDivergesDeep`, `eng/go/emit.go`)
counted the `raise` arm as a 0-value merge contributor, so the `if` looked like
a runtime-variable 0-or-1 result; every site consuming the Boolean as a
fixed-arity operand (`Assert.equal`, `print`, `add`) then refused with
“consumes loop results”. A diverging arm never reaches the merge, so the
surviving arm's value is unconditional — `raise` now joins `break`/`continue`/
tail-call in the divergence checks, matching the shallow `fragDiverges`. The
change only **relaxes** an over-marking (a genuine `if c [n] []` 0-or-1 still
refuses fixed-arity consumption), so it is coverage-only and sound by the
differential + property gates.

Effect on the clients: the three `apply-op-*` cases in `decision_unit_test`
now force-compile. The remaining suite refusals are unchanged in kind — `each`
over a `var`-block body (`var` splices onto the tape, a genuine code-body
refusal), `do {…}` map bodies and namespace-exposed words (the deferred checker
family — `check diagnostics`), and `test-check-prop` (the PBT framework). All
still run correctly under `--compile`.

Regression guards: `lang/spec/corpus-structures.tsv` (`classify`, an
op-dispatch chain whose default branch raises, result consumed by `add`) and
`lang/go/bytecode_raisediverge_test.go` (positive + the negative genuine-
variadic that must still refuse).

---

## Re-pin checklist

```bash
# Build the branch tip
REF=<branch-tip-sha>
curl -fsSL "https://codeload.github.com/aql-lang/aql/tar.gz/$REF" | tar -xz -C /tmp/aql --strip-components=1
( cd /tmp/aql/cmd/go && GOFLAGS=-mod=mod go build -o /tmp/aql-bin ./aql )

# From each client repo ROOT (so ./*.aql imports resolve):
/tmp/aql-bin test/<suite>.aql            # interpret
/tmp/aql-bin check test/<suite>.aql      # check
/tmp/aql-bin --compile test/<suite>.aql  # compile (interpreter-fallback ok)
```
