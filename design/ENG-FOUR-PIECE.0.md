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
