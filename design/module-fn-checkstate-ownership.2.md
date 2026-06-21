# Module-fn body compilation: the real blocker is checker precision, not `CheckState` ownership

Status: **`.2` — corrected root cause + a landed soundness piece.** This
note supersedes the root-cause analysis in `.0` §4 and the §5b risk
assessment in `.1`. It records what the `.1` plan's commit sequence
actually found once the **decision gate was run** (it is now runnable in
this environment — see below), and re-points the next attempt.

Read `.0` (problem statement) and `.1` (implementation plan) first. This
note changes one load-bearing conclusion in each.

---

## 0. The gate is now runnable (this changes everything about verifying)

`.0` and `.1` were written believing the mandated gate — `diverge.sh`
against the external decision project — could not be run here. It can:
the project clones cleanly over HTTP (`https://github.com/voxgig-aql/decision`)
and its `test/diverge.sh` drives any `aql` that accepts `--force-compile`:

```bash
git clone https://github.com/voxgig-aql/decision /tmp/decision
cd /home/user/aql/cmd/go && go build -o bin/aql ./aql
cd /tmp/decision && BYTECODE_AQL=/home/user/aql/cmd/go/bin/aql bash test/diverge.sh
```

The gate runs the 5 suites under interpreter + checker, and the two
compilable suites (`decision_unit_test`, `decision_smoke_test`) also under
`--force-compile` requiring byte-identical output. **`.1`'s path-A in-repo
fixture is therefore no longer the only option — the real gate is
available, and it is what found the result below.** (The fixture is still
worth landing as standing coverage, but it is no longer on the critical
path.)

---

## 1. What landed (sound, gate-green): §5a registry-discriminated memo keys

`FnAnalysisKey` now carries a per-registry scope id (`Registry.regID`,
minted in `NewRegistry`, read via `AnalysisScopeID`). It prefixes the
memo/recursion/quota key so a module sub-registry's fn cannot alias a
same-named, same-positioned parent fn once a check pass is shared across
registries. This is **inert today** (parent and module never share a memo
map yet) and stays byte-identical under `make test` + the decision gate.
It is the prerequisite `.0`/`.1` identified, and it is correct — but see
§3: it is **not** what was breaking the prior attempts.

Committed; `eng/go/fn_analysis_key_test.go` pins both halves (same scope →
same key; distinct scope → distinct key).

---

## 2. What was implemented, broke the gate, and was reverted: §5b threading

`.1` §5b (the shared-`*CheckState` realization) was implemented in full:

- `Registry.Check` → `*CheckState`; `NewRegistry` allocates it.
- `CheckState.Clone()` deep-copies maps/slices; the two rollback snapshots
  (`predicateSandbox`, `CompileSandbox`) and `ForkConcurrent` use it so a
  shared pointer never leaks a sandbox's mutations.
- `engine.go::execFnDefSig`: when the calling engine is check-active and
  `capturedReg != e.registry`, point `capturedReg.Check` at
  `e.registry.Check` around the `CallAQL`, restored after.

It **built clean, passed `make test`, and FAILED the decision gate** with
exactly the `.0` §4 signature: `decision_smoke_test.aql` →
`29 error(s)`, including
`uncalled_function: call to 'decide' … (arguments: Word, Map)` and a
cascade of `no_signature` on the module's own predicate fns. §5a's
scope-keying did **not** reduce the count (29 = the same as `.0`'s "share
the whole `CheckState`" variant) — the first hard evidence that memo
collision was never the cause.

**This was reverted.** Landing code that fails the mandated gate is exactly
the trap `.0` warns against. `make test` + the gate are green on the
§5a-only tree.

---

## 3. The corrected root cause (minimal, reproducible, registry-free)

`.0` §4 attributed the breakage to (a) name-keyed memo collision across
registries and (b) undefined-word leniency resolving in the wrong name
space. **Both are wrong for this failure.** The actual cause, reduced to a
top-level program with **no module, no sub-registry, and none of the §5b
threading**:

```aql
def ep fn [[p:Map] [Boolean] [true]]
def go fn [[el:Any] [Boolean] [ep el]]      # → no_signature: no matching signature for ep
go {a:1}
```

`check` reports `no_signature` for `ep`: **a value statically typed `Any`
does not match a `Map` parameter in check mode.** Swap `el:Any` for
`el:Map` and it checks clean. This holds in forward form (`ep el`), stack
form (`el ep`), and with extra args — it is not swap-, arity-, or
each-specific. It is a plain property of the checker's signature matching:
`Any` is not admitted to a more specific typed parameter.

The decision module hits this pervasively because its evaluators fold over
**untyped** lists:

```aql
def eval-pred-all fn [[children:List input:Map] [Boolean]
  [(children each [input swap eval-pred]) all]]
```

`children:List` is an untyped list, so `each` binds each element at type
`Any`, and `eval-pred` wants `i:Map` — the `Any → Map` mismatch. Every
`no_signature`/`uncalled_function` in the 29-error cascade traces back to
this one gap (the `Word` in `(arguments: Word, Map)` is the residual the
checker leaves after a `no_signature … assuming best-fit` continuation
propagates downstream).

### Why it was invisible until §5b

At baseline the decision module's fn bodies are **never check-analysed**:
the parent's check pass calls module fns through `execFnDefSig →
capturedReg.CallAQL`, and because `capturedReg` (the module sub-registry)
is **not** check-active, the body runs **concretely** (the `.0` §3
concrete-fold path). Concrete execution has real values, so the `Any → Map`
question never arises. §5b's whole point is to stop that concrete fold by
running the body in check mode — which immediately subjects every module
fn body to the checker, surfacing this pre-existing imprecision as parent
errors.

So the decision suites' own `check` being green is **not** evidence the
module type-checks cleanly — it is evidence that imported (and even
in-file `def fn`) bodies are black-boxed by the checker. The imprecision is
real; nobody has been forced to confront it because nobody check-analyses
those bodies.

---

## 4. What this means for the refactor

The `CheckState`-ownership mechanics (`.1` §5a/§5b) are **not the blocker**.
§5b's threading is mechanically sound — it correctly runs the body in check
mode with module-scoped resolution and scope-keyed memos. The blocker is
**orthogonal**: the checker is not precise/optimistic enough to analyse the
decision module's dynamically-typed bodies without error. Until that is
resolved, *any* mechanism that check-analyses module fn bodies — §5b, or
the eventual §6 framework-body compilation — will surface these diagnostics
and fail the gate.

There are two viable directions; they are genuinely different decisions
with different blast radius, and the choice is a maintainer call.

### Direction A — make the checker admit `Any` to typed parameters (optimistic dispatch)

Treat a statically-`Any` argument as matching any typed parameter in check
mode (it provably *might* be that type at runtime; the interpreter
dispatches dynamically and succeeds). There is precedent —
`dadcd5a "Checker: untyped higher-order element access matches
optimistically"` did exactly this for element access; this generalises the
same optimism to fn-param dispatch.

- **Pro:** fixes the real gap; makes module bodies (and ordinary
  `Any`-fed code) check cleanly; unblocks §5b and §6 honestly.
- **Con:** broad blast radius on `signature.go::matchSignature` /
  `dynamic_match`. It weakens a real check (a genuine `Any`-where-`Map`-
  needed bug stops being flagged). Needs its own full gate cycle against
  the whole `lang/spec` corpus + the decision gate, and almost certainly
  new negative spec rows pinning where optimism must *still* reject.
- **Scope:** this is its own task (`design/` note + spec work), independent
  of `CheckState` ownership. It should land and be gated **before** §5b is
  re-attempted.

### Direction B — do not surface imported-module-body diagnostics as parent errors

Keep module bodies black-boxed for *diagnostics* even when §5b runs them in
check mode for *recording*: tag diagnostics emitted while analysing a body
whose registry ≠ the top-level pass (a `ModuleBodyDepth` on the shared
`CheckState`, incremented in the threading block) and drop/downgrade the
error-severity ones at end of pass — the same shape as the existing
`FnBody`/`RescueForwardRefDiagnostics` mechanism, widened from
`undefined_word` to the imported-body case.

- **Pro:** narrowly scoped to the module boundary; matches the de-facto
  contract (an imported module is trusted, validated by its own author's
  `check`); leaves ordinary in-program checking untouched.
- **Con:** suppression must not hide a *compilation* divergence — §5b still
  has to produce a body whose compiled form matches the interpreter, and
  `--force-compile`'s `bytecode == interp` check is the only thing then
  guarding it. If the suppressed-but-imprecise analysis yields a wrong
  carrier that bakes wrong bytecode, the gate catches it as DIVERGES, not
  as a diagnostic — so Direction B trades a loud check failure for a
  subtler divergence risk that must be verified per-suite.

**Recommendation: Direction A**, sequenced as its own gated task before
re-attempting §5b. It fixes the actual defect rather than masking it, and
it is the honest precondition for *any* module-body check-analysis. Keep
§5a (landed) as its prerequisite. Re-attempt §5b (the threading is already
designed and was shown mechanically sound) only once `check` on the
decision suites is **0 errors with module bodies analysed**.

---

## 5. Revised sequence

1. **§5a registry-discriminated memo keys** — *landed*, gate-green, inert.
2. **Checker optimism: `Any` admits to typed params** (Direction A) — its
   own design note + `signature.go`/`dynamic_match` change + `lang/spec`
   negative rows + decision-gate. **New critical-path prerequisite.**
   Validate by: the §3 minimal repro checks clean, AND `check` on every
   decision suite stays 0 errors, AND the full `lang/spec` corpus holds
   (with new rows pinning where `Any` must still be rejected).
3. **§5b pass-global ownership** — re-apply the reverted, already-designed
   threading; now its check-analysis of module bodies produces 0 errors, so
   the gate can be green. Re-verify `CheckState.Clone` rollback + no
   side-effect leak.
4. **§6 framework code-body compilation** — unchanged from `.1` §5.

## 6. Concrete artefacts from this pass

- Landed: `eng/go/registry.go` (`regID` + `AnalysisScopeID`),
  `eng/go/carrier.go` / `core_helpers.go` / `callable_words.go`
  (scope-keyed `FnAnalysisKey`), `eng/go/fn_analysis_key_test.go`.
- Reverted (kept only as the design in `.1` §5b): the `*CheckState`
  pointerisation, `CheckState.Clone`, the two sandbox deep-copies, the
  `ForkConcurrent` clone, and the `execFnDefSig` threading. Revive verbatim
  once step 2 lands.
- Minimal repro for step 2's acceptance test: §3's `ep`/`go` program (and
  its `each`-over-untyped-`List` form) must `check` with 0 errors.
