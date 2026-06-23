# §7 — Stage C live re-grounding (2026-06): the decouple-and-rebaseline plan

Status: **design / live re-grounding.** Read `.6` first (it is still accurate in
*shape*), then this. The `.0`–`.6` record predates two things that changed the
ground truth: (a) `shareCheckState` landed (cross-registry `Check`/`Emit` sharing
now exists), and (b) the bytecode corpus advanced to `refusalCeiling == 6`. This
note re-measures the entanglement on the live tree and sequences the full Stage C
project the maintainer authorized (relax the "fully-safe" constraint; do the
decouple + corpus re-baseline as one reviewed unit).

## What changed since `.6`

- **`shareCheckState` exists** (`eng/go/engine.go:4408`). In `execFnDefSig`'s
  `capturedReg` branch (~4474) it installs the MAIN registry's active `Check`
  (including `Emit`) onto the module sub-registry for the duration of `CallAQL`,
  then restores. So a module-preamble AQL fn now *records* into the main frame —
  the missing piece `.6`/next-stages §C described ("`capturedReg.Check.Emit.Active()`
  is false") is partly addressed. The remaining gap is **unit isolation**: the
  body records into the MAIN frame, so its internal residual leaks onto the program
  residual rather than being contained in its own `StartFnCompile` unit.

## Confirmed coupling on the live tree (measured this session)

The hermetic-eval change (snapshot+restore `r.Check.Diagnostics` around the
synthetic example eval in `makeDynamicEval`, `lang/go/native/native_help.go`) was
implemented and measured, then reverted to keep the tree green pending the full
unit:

| effect | result |
| --- | --- |
| `decision.aql` false positives (`g fn [[m:Map]…(m get "xs") all]`) | **fixed** — 0 errors (was a synthetic `no_signature` at the example arg) |
| `TestForwardStrandAdvisory_FiresOnGotcha` (`lang/go/forward_strand_advisory_test.go`) | **breaks** — the in-body `forward_strands_operand` advisory was a help-eval side effect |
| `TestCheckUncalledFnBodyTypoStillFlagged` (`lang/go/test/forward_ref_rescue_test.go`) | **breaks** — the uncalled-fn body `undefined_word` (zzyzx) was a help-eval side effect |
| langspec coverage | **shifts** — `reducibleCeiling` trips (2 > 1; a `quote` row reappears) and a NEW `dispatch recovery (best guess)` compute gap of 3 surfaces (rows previously gated as check-errors by synthetic diagnostics now reach the compiler) |

So steps 1–4 below are a **single coupled unit**: the hermetic eval alone both
drops two real capabilities and *increases* the refusal/ceiling counts. It is only
net-positive once the construction-check replaces the side-channel and the
cross-registry unit compilation compiles the exposed + target rows.

## The four coupled steps (the authorized project)

1. **Hermetic help eval** *(implemented, reverted pending the unit)*. Snapshot +
   restore `r.Check.Diagnostics` (and keep `Emit.Suspend()`) around the synthetic
   eval so it contributes NOTHING to diagnostics / compilation gating / coverage.
   File: `lang/go/native/native_help.go::makeDynamicEval`.

2. **First-class construction-time body check.** Replace the eval's accidental
   side-channel with a real post-binding body pass at fn install time, run against
   **carrier** args (`NewCarrier(param-type)` so an abstract `Map`/`List` reads
   `dynamic(Any)` — the §6a-A principle). It must (a) bind the fn name BEFORE the
   pass so recursion resolves, (b) emit the SAME `undefined_word` (uncalled typo)
   and `forward_strands_operand` (in-body strand) diagnostics the two tests pin,
   and (c) snapshot/restore Defs depth, Diagnostics base, Emit (suspended),
   FnBaselines, and the args stack. Insertion point: the `InstallFnDef` registration
   path. This is the load-bearing correctness piece.

3. **Cross-registry module-body unit compilation.** In `execFnDefSig`'s
   `capturedReg` branch, when the main `Check` is active, compile the module-preamble
   fn body as its OWN `StartFnCompile` unit (resolving names in the sub-registry
   scope via the already-shared `Check`/`Emit`) so its internal residual is contained
   and does not leak onto the program residual. Clears the three target rows:
   `module-test.tsv:38` (Test.run-spec / test-describe), `module-parselang.tsv:23`
   (ParseLang.parse_calc get chain), `module-rand.tsv:38` (seeded generator body).

4. **Corpus re-baseline + soundness investigation.** With synthetic errors gone and
   module bodies compiling, re-baseline `test/go/langspec` ceilings and per-row tiers
   in lockstep, and investigate the masked `Assert.throws "did not throw"` / the new
   `dispatch recovery (best guess)` rows — `.6` flagged at least one genuine
   compile-soundness gap the synthetic errors were hiding. The differential (value
   parity, 0 divergences) is the hard gate throughout; the ceiling moves are the
   deliberate, reviewed re-baseline.

## Discipline

Land as ONE reviewed unit (never a partial diagnostic filter — `.6` §3 proved
partial suppression silently reclassifies compilation and changes observable
behavior). `make verify-bytecode` (differential + fuzz + race + aqldebug, 0
divergences) gates soundness; the ceilings are re-baselined with explicit
rationale. Gate-clean-or-revert.
