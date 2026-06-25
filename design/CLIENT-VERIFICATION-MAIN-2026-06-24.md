# Client-library verification against `aql` main — and the fixes to apply

**Date:** 2026-06-24
**aql verified:** `main` @ `0b010ae1b7123f4fbb72758dc0b65f32d2aadae0` (built from
source, `GOFLAGS=-mod=mod go build ./aql`)
**Libraries verified:** `voxgig-aql/trie`, `voxgig-aql/decision`,
`voxgig-aql/bloom-filter` (downloaded as source tarballs from
`codeload.github.com` and run from each repo root so `import "./*.aql"`
resolves).
**Driven by** the three client verification reports:
[trie/AQL-MAIN-VERIFICATION.md](https://github.com/voxgig-aql/trie/blob/main/AQL-MAIN-VERIFICATION.md),
[decision/aql-main-verification-2026-06-23.md](https://github.com/voxgig-aql/decision/blob/main/aql-main-verification-2026-06-23.md),
[bloom-filter/aql-backend-report.md](https://github.com/voxgig-aql/bloom-filter/blob/main/aql-backend-report.md).

This is the follow-up to
[`CLIENT-FIXES-2026-06-24.md`](CLIENT-FIXES-2026-06-24.md): it **re-verifies the
landed upstream fixes against the current `main` HEAD** (the client reports
tested `14036b41`; `main` is now several checker commits further on) and tells
each client project exactly what to change.

---

## TL;DR — every issue in the three reports is resolved on `main`

Across **all 21 test suites of all three libraries**, on `main @ 0b010ae`:

| Mode | Result |
|---|---|
| **Interpreting** (`aql suite.aql`) | ✅ all 21 suites pass |
| **Checking** (`aql check suite.aql`) | ✅ all 21 suites **0 errors** |
| **`aql check module.aql`** (direct, all 6 modules) | ✅ **0 errors** each |
| **`--compile` == interpreter** | ✅ byte-identical on all 21 suites |
| **`--force-compile`** (strict bytecode, no fallback) | ⚠️ partial — code-body words still refuse (enumerated below), all fall back correctly under `--compile` |

The checker false-positive classes the reports documented (`no_signature` on
`Any`-typed dynamic dispatch, `undefined_word` on `do{}` params, `unused_def`
on `/r` reference-exports, the `all`/`any` and `DTable`/`eval-table` namespace
residue, the trie `mk-node`/`put-kid` cascades) are **gone** — including in the
direct module checks (`aql check trie.aql` etc. now report **0 errors**, down
from the 150–300 in the original `AQL-CHECK-REPORT.md`). The bloom-filter
interpreter `None`-interpolation regression is fixed. Nothing in any of the
three libraries needs a source change to pass interpret + check.

**The one remaining limitation** is `--force-compile` (the strict bytecode path
that refuses rather than falling back): it still cannot lower a handful of
**code-body words** (`each` over a `var`/lambda body, `do {…}` map bodies, the
test-framework words `test-test` / `test-check-prop`). Those are tracked
upstream emitter work, **deferred by design** (see "Remaining `--force-compile`
gaps" below). `--compile` (silent interpreter fallback) produces correct output
for every suite, and the **compile==interpreter invariant holds everywhere** —
so this is a coverage gap, not a correctness bug.

---

## Full result matrix (`main @ 0b010ae`)

`interp` = `aql X`; `check` = `aql check X` error count; `==interp` = `aql
--compile X` output equals `aql X`; `force-compile` = `aql --force-compile X`
(the strict path).

### bloom-filter

| Suite | interp | check | ==interp | force-compile |
|---|:--:|:--:|:--:|---|
| `bloom_unit_test`  | ✅ | 0 | ✅ | refuse: `test-test` (Stage 2) |
| `bloom_unit_spec`  | ✅ | 0 | ✅ | refuse: `do` |
| `bloom_prop_test`  | ✅ | 0 | ✅ | ✅ **compiles** |
| `bloom_prop_spec`  | ✅ | 0 | ✅ | refuse: `each` (Stage 2) |
| `bloom_smoke_test` | ✅ | 0 | ✅ | refuse: `do` |

### decision

| Suite | interp | check | ==interp | force-compile |
|---|:--:|:--:|:--:|---|
| `decision_unit_test`  | ✅ | 0 | ✅ | refuse: `test-test` (Stage 2) |
| `decision_unit_spec`  | ✅ | 0 | ✅ | refuse: `each` (Stage 2) |
| `decision_prop_test`  | ✅ | 0 | ✅ | ✅ **compiles** |
| `decision_prop_spec`  | ✅ | 0 | ✅ | refuse: `each` (Stage 2) |
| `decision_smoke_test` | ✅ | 0 | ✅ | refuse: `check diagnostics`† |

### trie (trie / radix / tst / burst)

| Suite | interp | check | ==interp | force-compile |
|---|:--:|:--:|:--:|---|
| `trie_unit_test`  | ✅ | 0 | ✅ | refuse: `unmatched dispatch recovered at mk-node` |
| `trie_unit_spec`  | ✅ | 0 | ✅ | refuse: `unmatched dispatch recovered at mk-node` |
| `trie_prop_test`  | ✅ | 0 | ✅ | refuse: `test-check-prop` (Stage 2) |
| `trie_prop_spec`  | ✅ | 0 | ✅ | refuse: `each` (Stage 2) |
| `trie_smoke_test` | ✅ | 0 | ✅ | refuse: `check diagnostics`† |
| `radix_unit_test` | ✅ | 0 | ✅ | refuse: `check diagnostics`† |
| `radix_prop_spec` | ✅ | 0 | ✅ | refuse: `each` (Stage 2) |
| `tst_unit_test`   | ✅ | 0 | ✅ | refuse: `do` |
| `tst_prop_spec`   | ✅ | 0 | ✅ | refuse: `each` (Stage 2) |
| `burst_unit_test` | ✅ | 0 | ✅ | refuse: `do` |
| `burst_prop_spec` | ✅ | 0 | ✅ | refuse: `each` (Stage 2) |

† `check diagnostics` here is **not** a check-mode error you can see with `aql
check` — those three suites check **0 errors**. It is the compile path's
internal check pass tripping the *dynamic-help example generator* (synthetic
`{a:1,b:2}` args run through fn bodies at registration). See the gaps section.

---

## What changed since each report

### trie — the entire §3–§7 story is resolved

The report's core finding was that `main`'s transitive `aql check` inherited the
library's `no_signature`/`undefined_word` cascades, gating both check and
`--force-compile` for every consumer, and that even the client-side restructure
(`mk-node` chained `set`, the `kids-of` accessor) could not drive the unit
suites to zero because of "emergent whole-program cascades."

On `main @ 0b010ae` those cascades are gone. Every one of the 11 suites checks
**0 errors**, and the four modules check **0 errors** directly
(`aql check trie.aql / radix.aql / tst.aql / burst.aql`). The wishlist items
1–4 from `AQL-CHECK-REPORT.md` all landed: `/r` reference-exports are traced as
uses (`unused_def` gone), `Any` unifies with concrete params (gradual carriers),
the `do{}`-param and dynamic-`set` residue cleared, and the body re-parser no
longer mis-reports `build-row`.

### decision — the residual 2 and the smoke 16 are both gone

The report had `decision.aql` at 2 advisory false positives (`all`/`any`) and
`decision_smoke_test` at 16 (the `DTable` / `eval-table` / `decide` namespace
class). On `main @ 0b010ae`: `aql check decision.aql` = **0 errors**, and
`aql check decision_smoke_test.aql` = **0 errors** (2 harmless `unused_def`
*warnings* on body-local `best-pri`/`done` remain — warnings, not errors, and
do not gate).

### bloom-filter — already clean, still clean

The report already verified `14036b4` fully clean. Re-confirmed on `0b010ae`:
all five suites interpret, check 0, and `--compile`-match. `bloom_prop_test`
fully `--force-compile`s.

---

## Fixes to apply — per client

No library **source** change is required for interpret + check to pass. The
fixes are **re-pin + CI gate promotion**, so each project benefits from the
landed checker work. Locations below are from the current `main` of each repo.

### bloom-filter

1. **Re-pin (optional but recommended).** Bump `AQL_REF` from
   `14036b4125a9ccbd9655503a1a4171c008d93d06` to
   `0b010ae1b7123f4fbb72758dc0b65f32d2aadae0` in:
   - `ci/test.yml` (`env.AQL_REF`)
   - `.claude/hooks/session-start.sh` (`AQL_REF=`)
   - `test/divergence/run.sh` (`AQL_BYTECODE_REF=`)
   - `api.json` (`"aql_ref": "0b010ae"`)
   - and the prose refs in `AGENTS.md`, `docs/how-to.md`, `README.md`,
     `plugins/.../SKILL.md`, `.claude/skills/.../SKILL.md`.

   bloom is already clean on `14036b4`, so this is housekeeping; the
   `ci/test.yml` cross-file ref-consistency job already enforces the four
   machine refs agree.
2. **No gate change needed.** The divergence job already encodes the two hard
   invariants (interpreter green; every suite the compiler accepts matches the
   interpreter), and both hold.

### decision

1. **Promote `aql check` to a hard gate.** The existing "Static check
   (advisory, non-gating)" step in `.github/workflows/test.yml` runs
   `aql check --soft decision.aql` with `continue-on-error: true`. `decision.aql`
   now checks **0 errors**, so drop both `--soft` and `continue-on-error` to make
   it a real gate. All five `test/*.aql` suites also check 0 (including
   `decision_smoke_test`, was 16), so optionally extend the step to a loop over
   `test/*.aql` to guard the suites against regressions too.
2. **Re-pin the interpreter baseline (optional).** `AQL_REF` is `958c379b`
   (the interpreter baseline) in `.github/workflows/test.yml` +
   `.claude/hooks/session-start.sh` + `api.json`. It can move to `0b010ae`; the
   module only *requires* `≥ 958c379b` (`surface`/`exposes`, generics,
   `refine Record`, `fnsig`), so bumping is safe but not required. `diverge.sh`
   already tracks `main` HEAD independently, so leave it.
3. **`--force-compile`: keep advisory.** `decision_prop_test` compiles;
   `unit`/`spec` refuse on `test-test`/`each`; `smoke` refuses on the
   dynamic-help `check diagnostics` artifact. All run correctly under
   `--compile`.

### trie

1. **Re-pin `14036b41` → `0b010ae`** in the five machine/prose locations the
   `ci/test.yml` consistency job checks and documents:
   - `ci/test.yml` (`env.AQL_REF`)
   - `.claude/hooks/session-start.sh` (`AQL_REF=`)
   - `api.json` (`"aql_ref": "0b010ae"`)
   - `AGENTS.md` ("Verified against `aql` commit …")
   - `docs/how-to.md` (the `git checkout …` line and the prose ref)
2. **Promote the unit-suite `aql check` from advisory to a hard gate.** The
   report set the `aql check` and `--force-compile` steps over the unit suites
   to `continue-on-error: true` because each unit file inherited 5–31 transitive
   `no_signature` errors. Those are now **0**. Drop `continue-on-error` on the
   **`aql check`** step (and the module-direct
   `aql check trie.aql radix.aql tst.aql burst.aql` step — also 0 now). This
   restores the three-way intent the report wanted to keep but couldn't.
3. **Keep `--force-compile` advisory.** It still refuses (see gaps), so leave
   that one step `continue-on-error`.
4. **Optional cleanup — the client-side workarounds can be revisited.** §7 of
   the report restructured `trie.aql`'s `mk-node` to chained `set` and added a
   typed `kids-of` accessor to dodge the checker. The checker no longer needs
   either (dynamic dispatch and `do{}` params both check clean now). You *may*
   revert to the more idiomatic `do {end:[fin] val:[val] kids:[kids]}` form for
   readability — **but test `--force-compile` after**: the current
   chained-`set` `mk-node` is what makes `trie_unit_test`/`_spec` refuse with
   `unmatched dispatch recovered at mk-node`; the `do{}` form will refuse
   differently (`do`, like the other modules). Either way it interprets and
   checks clean, so this is purely a style/coverage choice. Recommendation:
   **leave it as-is** unless you are actively chasing `--force-compile`
   coverage, since neither form fully compiles yet.

---

## Remaining `--force-compile` gaps (upstream, deferred — context for clients)

These refusals are why no suite except the two `*_prop_test` files fully
compiles. They are **sound** (`design/COMPILABLE-SUBSET.md`: "refusal is always
sound; the worst failure mode is slow, not wrong") and every one falls back to a
correct interpreter run under `--compile`.

| Refusal | Where | What it is |
|---|---|---|
| `code-body word each (Stage 2)` | every `*_spec`, several units | `each` whose body is a `var`/lambda block — `var` splices onto the tape; lowering it is the tracked Stage-2 code-body work |
| `code-body word test-test (Stage 2)` | `*_unit_test` | the PBT/unit framework's `test-test` driver word |
| `code-body word test-check-prop (Stage 2)` | `trie_prop_test` | the property-test framework's check word |
| `unannotated or opaque word do` | `bloom_smoke`, `bloom_unit_spec`, `tst_unit_test`, `burst_unit_test` | a `do {key: [expr]}` computed-value **map body** |
| `unmatched dispatch recovered at mk-node` | `trie_unit_test`/`_spec` | the chained-`set` `mk-node` builder dispatched over a gradual node — the emitter recovers but refuses to lower |
| `check diagnostics` | `decision_smoke`, `radix_unit`, `trie_smoke` | **not** a real check error (these suites `aql check` clean). The compile path's internal check runs the **dynamic-help example generator** — each registered fn body is evaluated in check mode against a synthetic `{a:1,b:2}`, and those synthetic dispatch failures become error diagnostics that gate the emit. Diagnosed in `design/module-fn-checkstate-ownership.{5,6}.md` |

**Why these are deferred, not quick fixes.** The test-framework / `each` / `do`
code-body lowering is the named Stage-2 emitter cluster
(`design/aql-bytecode-completion.0.md`, `COMPILABLE-SUBSET.md`). The
`check diagnostics` artifact is worse than a filter: the dynamic-help eval is
*load-bearing twice over* — it doubles as the only construction-time check of a
defined-but-never-called fn body, and the langspec compilation corpus (2830
rows) is calibrated to the exact diagnostic set it emits. `module-fn-checkstate-
ownership.6.md` measured three partial fixes; each either regressed the
coverage corpus or changed observable runtime behavior. The sound fix
(hermetic help eval + a first-class construction-check pass + a corpus
re-baseline) is a scoped project, not a localized change. **Recommendation:
keep `--force-compile` advisory in all three clients until that lands**; the
interpreter (and `--compile`, which matches it) remain the supported,
fully-green paths.

---

## aql-side status

The report-listed issues are **already fixed on `main`** — the checker
precision work (gradual-`Any` carriers, `/r` reference-export use-tracing,
recursive re-analysis suppression, dynamic-carrier / Options-Record / branch-
merge / fold-seed matching, param narrowing) merged ahead of `0b010ae` and is
captured in [`CLIENT-FIXES-2026-06-24.md`](CLIENT-FIXES-2026-06-24.md). This
document is the independent re-verification that those fixes hold against the
current HEAD across all three real client libraries, plus the precise,
reproducible ledger of what `--force-compile` still defers. No further aql
source change is proposed here: the remaining items are the deferred emitter /
dynamic-help-eval projects above, where a partial change is known to regress the
calibrated corpus.

---

## Reproduce

```bash
REF=0b010ae1b7123f4fbb72758dc0b65f32d2aadae0
mkdir -p /tmp/aql && curl -fsSL \
  "https://codeload.github.com/aql-lang/aql/tar.gz/$REF" \
  | tar -xz -C /tmp/aql --strip-components=1
( cd /tmp/aql/cmd/go && GOFLAGS=-mod=mod go build -o /tmp/aql-bin ./aql )

# From each client repo ROOT (so ./*.aql imports resolve):
for f in test/*.aql; do
  /tmp/aql-bin "$f" >/dev/null            && echo "interp ok   $f"
  /tmp/aql-bin check "$f"                 # 0 errors
  /tmp/aql-bin --compile "$f" >/dev/null  # output matches interpreter
  /tmp/aql-bin --force-compile "$f"       # strict: see the gaps table
done
```
