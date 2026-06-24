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

### trie — ⚠️ interpret green; check improved, residual dynamic-dispatch

Interpreting is fully green (all 11 suites). The library's own `check` error
counts dropped (the `push`/`convert`/`fold` fixes), but the imperative unit
suites still report the **dynamic-dispatch class** because the trie library
is built almost entirely on `Any`-typed node walking.

---

## The remaining gap (decision smoke, trie) — root cause

The residual errors are all one class: **dispatch on an explicitly-`Any`-typed
value.** A fn parameter declared `:Any` (or a value read from an untyped
structure) becomes a *strict* `Any` carrier in check mode, and a word that
needs a concrete receiver (`get`, `add`, a user fn like `find-kid` /
`trie-insert` / `all` / `any`) fails `no_signature` against it — even though it
dispatches fine at runtime. Minimal reproduction:

```aql
def g fn [[v:Any] [Any] [v get "x"]] end   g {x:1}   # interp: 1   check: no_signature: get
```

Contrast a *typed* param, which checks clean:

```aql
def f fn [[m:Map] [Any] [m get "x"]] end   f {x:1}   # interp: 1   check: clean
```

This is the trie report's wishlist item **“unify `Any` with concrete params”**
and the decision/`dx-report.md` finding. The principled fix is to bind an
explicitly-`Any` parameter to a **dynamic (gradual)** carrier — the same
treatment `each`/`filter`/`fold` already give untyped list elements
(`NewElementCarrier`) — so dispatch on it poly-matches instead of failing. That
is a deliberately-scoped, broad checker change (it touches every fn-param
carrier construction site and the type-soundness profile), so it is **left for a
dedicated pass** rather than bundled here, where it could destabilise the
heavily-pinned checker.

The related `undefined_word` cases (`DTable` in decision smoke; `do {…}`
quotation params like `kids` in trie) are the same family: a name that is only
known through a namespace-exposed export or a quotation parameter, which the
static checker does not yet thread.

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
“Stage 2”) — the tracked emitter-coverage work (decision report §6), unchanged
by this branch. `--compile` (silent interpreter fallback) produces correct
output for every suite.

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
