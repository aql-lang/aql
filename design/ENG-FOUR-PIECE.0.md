# ENG-FOUR-PIECE.0 — splitting the kernel into eng → compiler → check → core

**Status:** In progress · **Started:** 2026-08-05 (maintainer instruction:
"refactor eng into the following pieces: core - pure interpreter, check -
type checker, compiler - compiler, eng - bytecode runner. Dependency tree
should be: eng -> compiler -> check -> core, and core is standalone.
Refactor go first, and get core to 100% coverage.")

Recon evidence (file manifest, entanglement families F1–F10, seam designs
S1–S10, risks): [ENG-FOUR-PIECE-RECON.0.md](ENG-FOUR-PIECE-RECON.0.md).

## Decisions

1. **Real sub-packages inside the eng module.** `eng/go/core`,
   `eng/go/check`, `eng/go/compiler`; the existing `eng/go` (package eng)
   remains the TOP piece: the bytecode runner (vm*.go, user_poly) plus a
   FACADE of type aliases and re-exports (`type Value = core.Value`,
   `type Type = core.Value`, `type Registry = core.Registry`, …) so
   lang/, basic/, test/, specfix/ compile unchanged. Import direction is
   compiler-enforced: core imports neither sibling; check imports core
   only; compiler imports check+core; eng imports all three.
2. **parser re-points to core.** `eng/go/parser` already uses only core
   surface (recon-verified); post-split it imports core directly, so
   core+parser IS the standalone pure interpreter.
3. **specfix stays above the facade** (it exercises check+compiler
   surface by design). A core-only corpus lane variant proves the
   standalone interpreter without check/compile rows.
4. **Seams before moves.** The wrong-direction entanglements are broken
   IN-PACKAGE first (hooks with no-op defaults — the `theInactiveEmit`
   precedent), so every stage keeps `make test` + the merged ADR-008
   gate green; the physical package cut is the LAST step per boundary.
   - S1 `AnalysisHooks` on Registry (+ cached `analysisActive` bool for
     the hot loop) replaces direct `Check.` consultation in core.
   - S2 re-cut `EmitRecorder` (home: check; implementor: compiler;
     opaque fragment/checkpoint handles; spec types live in check).
   - S3 `DispatchRecorder` (compiler registers on check).
   - S4 `CompiledRuntime` (eng registers on core; generalizes Invoker).
   - S5 declarative metadata (CompileEffect, CallableSpec, ReturnsFn
     slots) stays core-declared extension vocabulary.
   - S6 `PayloadBase` embeddable marker extends the payload seal
     across packages.
5. **Coverage.** New `cover-gate-core` target: core by its own suite
   (plus parser + the core corpus lane), floor ratcheting to **100**
   with the established `//covergate:allow` proof discipline.
   Check-mode-only arms move OUT of core rather than being allowlisted
   (recon §5). The merged ADR-008 gate stays 100% throughout.
6. **Stages** (each committed green: fmt/vet/lint/test/cover-gate):
   - **Stage 0** — in-package hygiene: regroup misfiled functions into
     piece-named files; eliminate the 13 concrete `.(*EmitState)`
     asserts outside the emit cluster; add the piece-map lint
     (files→pieces manifest; wrong-direction identifier references
     fail CI) — the "virtual packages" that make the later cut
     mechanical.
   - **Stage 1** — invert core→eng (F5): S4 CompiledRuntime, effects
     split (S7), stamping side-table (S8).
   - **Stage 2** — invert core→check (F1/F7/F3-hard): S1 hooks, move
     NewCarrier/carrierOfLiteral down, CheckState decl out of
     registry.go.
   - **Stage 3** — invert check→compiler (F2/F6): S2 recorder re-cut +
     S3 dispatch recorder; move recordDispatchOutcome family,
     user_poly, drift_window to compiler-side files.
   - **Stage 4** — the physical package split, bottom-up (core, check,
     compiler; facade aliases in eng); parser re-point; internal tests
     move with their code; external tests untouched via the facade.
   - **Stage 5** — gates: cover-gate-core at 100; per-piece profiles;
     the TS mirror follows in a later program ("go first").
7. **Amendment (2026-08-05, maintainer): the pieces are TOP-LEVEL
   modules, not sub-packages.** "Core, basic, eng and lang are all to
   be at the same level, not under internal in eng." Decision 1's
   sub-package layout is superseded: the cut core lives at `core/go`
   (module `github.com/boru-lang/boru/core/go`, package `core`), a
   sibling of `eng/go`, and the module chain becomes
   `core ← eng ← basic ← lang ← cmd`. check and compiler remain
   VIRTUAL pieces inside eng/go (tracked by `eng/go/piece_map.tsv`)
   until their own cuts; the parser likewise stays an eng/go
   sub-package on the facade until the Stage-5 re-point.

## Stage 4 record (2026-08-05)

- `core/go` cut: the interpreter-core production files (package
  `core`) moved with their white-box tests; the module requires only
  cockroachdb/apd.
- `eng/go` keeps check + compiler + vm + parser and gains the
  GENERATED facade `aliases_core.go` (`piecetool -facade`: type
  aliases, const re-exports, set-once var mirrors, thin inlinable
  wrapper funcs) plus the hand-written `aliases_core_generic.go`
  (the generic `Cap[T]`, which a facade wrapper cannot express).
  Mutable slot tables are NOT mirrored — installers write `core.X`
  qualified. Regenerate the facade after any core surface change.
  Wrapper funcs must stay direct calls (never var-of-func) so they
  inline; the alloc ceilings (`TestInterpAllocCeilings`) gate this
  across the module boundary.
- Every seam slot carries a NAMED, test-pinned inactive default
  (`TestInactiveAnalysisImpl`, `TestInactiveCheckBraid`,
  `TestInactiveEmitMethods`): an anonymous default replaced at init
  is unreachable and fails the merged ADR-008 gate.
- `make cover-gate-core` established: core/go by its own suite,
  floor 80 (measured 80.5%); Stage 5 ratchets it to 100 by moving
  the remaining check/compile-only arms out of core files or
  covering them from the core suite.
- `make cover-gate-eng` RE-BASED: the cut moved the interpreter
  core's statements and ~120 kernel test files to core/go, so the
  pre-cut floor (89, measured over the undivided kernel) is not
  comparable. The gate now spans eng/go only, restarting at the
  post-cut measured 84.6% (floor 84); the (cover-gate-eng,
  cover-gate-core) pair supersedes the old single gate and both
  ratchet independently to 100.
- Facade funcs no suite calls (66 at the cut) are emitted as
  `var Name = core.Name` func-value re-exports — same call syntax
  at every reference site, no wrapper body to leave permanently
  uncovered (`coldFuncs` in the generator).
