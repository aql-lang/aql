# core/go — Interpreter core CLAUDE.md

The `core` module is the interpreter core cut out of the kernel
(design/ENG-FOUR-PIECE.0.md, Stage 4): values, types, signatures,
matching, the registry, and the step loop, standalone on
cockroachdb/apd alone. The kernel conventions in
[eng/go/CLAUDE.md](../../eng/go/CLAUDE.md) apply to this module
verbatim — the payload seal, signature ordering, no zero-value
overload, canonical `*Type` pointers, the payload-presence
predicates — and that guide remains their single home while the
check and compiler pieces are still being carved out of `eng/go`.

Two rules specific to this module:

- **No upward imports.** core requires no boru sibling. The check /
  compiler / eng behaviors reach core ONLY through the seam slots
  (`AnalysisImpl`, `CheckBraid`, the `EmitRecorder` hooks,
  `DriftWindowRecorder`, the compiled-runtime hooks); every slot has
  a NAMED inactive default pinned by a core-side test
  (`TestInactiveAnalysisImpl`, `TestInactiveCheckBraid`). A new slot
  follows the same pattern — an anonymous default replaced at init is
  unreachable and fails the merged ADR-008 gate.

  `EmitRecorder` (`emit_recorder.go`) is the widest of these, and it
  is one of two seams with a consumer OUTSIDE this module: `basic`'s
  `if` / `case` / `for` record control flow through it. That is why
  the interface carries the branch/loop group (`TakeFragment`,
  `RecordBranch`, `RecordLoop`) and why `BranchRecord`,
  `CodeEffectInfo` and the deliberately opaque `EmitFragmentRef`
  (`any`) are declared here rather than in compiler — a word library
  must be able to record a fragment without naming
  `*compiler.EmitState`. core never inspects a fragment; it hands the
  same reference back untouched. If a caller outside compiler ever has
  to type-assert the recorder, the interface is short a method: widen
  it rather than let the downcast stand (ADR-013's 2026-08-07 second
  amendment).

  The other outside-consumer seam is `AnalysisImpl.AnalyseFnBody` /
  `.AnalyseLoopBody`, exported as `RunFnBodyAnalysis` /
  `RunLoopBodyAnalysis` (`analysis_hooks.go`): `basic`'s `fn` and
  `for` need to re-enter the analysis pass over a body, but the pass
  itself is the checker's. Same discipline — every parameter and
  result is a core type, so the seam names no check symbol.

- **The carrier lattice is core's, not the checker's** (ADR-013's
  2026-08-08 amendment). `carrier_new.go`, `carrier_join.go`,
  `carrier_body.go`, `carrier_spread.go`, `guard_narrow.go`,
  `guard_predicate.go`, `deadsig.go` and `record_typed_def.go` hold
  what used to live in `check/go/carrier.go`. The test is ownership of
  the *types*, not of the phase: a function over `Value`, `*Type`,
  `r.Defs` and `CheckState` belongs here even though only an analysis
  pass ever calls it, because that is what lets a word library carry
  an analysis half without depending on the checker. What stays above
  is the pass — memoisation, recursion bailing, the per-shape quota,
  the Kleene fixed point. Two slots died to this move
  (`JoinCarriersHook`, `AnalysisImpl.AddUnique`): with their subjects
  core-resident there was nothing left to indirect. Prefer deleting a
  slot that way over keeping it for symmetry.
- **The eng facade mirrors this surface.** `eng/go/aliases_core.go`
  is GENERATED (`piecetool -facade`); after changing core's exported
  surface, regenerate it (generic functions go in the hand-written
  `eng/go/aliases_core_generic.go`) and keep `eng/go/piece_map.tsv`
  current. Facade wrappers must remain direct calls so they inline —
  the alloc-ceiling tests gate the module boundary's cost.

Coverage: `make cover-gate-core` gates core/go by its own suite
(floor ratcheting to 100, design/ENG-FOUR-PIECE.0.md Stage 5), on
top of the repo-wide merged ADR-008 gate.
