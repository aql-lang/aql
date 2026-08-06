# compiler/go — Compiler CLAUDE.md

The `compiler` module is the recorder and lowerer cut out of the
kernel (design/ENG-FOUR-PIECE.0.md): the EmitState recording pass
that rides the check run (`emit.go`), the trace lowerer that
linearises it (`lower.go`), the bytecode Program model and its
emitter (`bytecode.go`), the dispatch-outcome recording family —
const folds, poly events, dyn-body closures, fallback islands
(`compiler_dispatch_record.go`) — plus user-poly bake and planning,
code-body closure compilation, typed code stack effects, the
forward-drift window, and detached fn-unit stamping. It PRODUCES
bytecode; the VM that executes it lives in `eng`. The kernel
conventions in [eng/go/CLAUDE.md](../../eng/go/CLAUDE.md) apply to
this module verbatim — the payload seal, signature ordering, no
zero-value overload, canonical `*Type` pointers, the payload-presence
predicates — and that guide remains their single home. For the
POSITIVE statement of what compiles and why (the rule each refusal
gate defends), design/COMPILABLE-SUBSET.md stays the reference.

Three rules specific to this module:

- **No upward imports.** The chain is core -> check -> compiler ->
  eng, and compiler requires check and core ONLY. Never name an eng
  symbol here: eng reaches the compiler through its generated facade
  and through core's compiled-runtime hooks, never the other way.
- **The compiler is the ACTIVE half of its seams.** It declares no
  slot table of its own; it installs into the ones below it, all at
  init: `check.DispatchBraid` (S3) from `dispatch_hooks_install.go`,
  `core.DriftWindowRecorder` from `drift_window.go`, and
  `core.NewEmitStateHook` / `core.NewIsolatedEmitHook` from
  `emit.go`. Note the `EmitRecorder` INTERFACE lives in core
  (`core/go/emit_recorder.go`) — `*EmitState` implements it, and
  check names only `core.EmitRecorder`. Every default on the far side
  is named and pinned (`TestInactiveDispatchBraid` in check,
  `TestInactiveEmitMethods` here in `zz_settled_test.go`,
  `TestInactiveEmitMethodArms` / `TestInactiveConstructorSlots` in
  core); a new slot follows the same pattern — an anonymous default
  replaced at init is unreachable and fails the merged ADR-008 gate.
- **The eng facade mirrors this surface.**
  `eng/go/aliases_compiler.go` is GENERATED (`piecetool -facade`);
  after changing the compiler's exported surface, regenerate it and
  keep `eng/go/piece_map.tsv` current. Facade wrappers must remain
  direct calls so they inline — the alloc-ceiling tests gate the
  module boundary's cost.

**Installing S3 (`installDispatchBraid`).** The five check-side slots
take `recordDispatchOutcome`, `tryFoldScalarConst`, `tryRecordPoly`,
and — for the two plan-returning slots — CLOSURES that wrap
`tryCompileUserPolyArms` / `planUserPolyDispatch`. The wrapping is
load-bearing, not ceremony: check holds the plan as the opaque
`check.UserPolyPlan` interface and tests `plan != nil`, so a declined
bake must surface as a NIL INTERFACE; returning the concrete
`*userPolyPlan` directly would hand check a typed nil that reads as
present. Keep any new plan-returning installer in that shape.

Tests: `cd compiler/go && go test ./...` — a flat single package, no
sub-packages and no module Makefile. From the repo root `make test`
and `make test-race` fan out over compiler/go with the rest of
`MODULES` / `RACE_MODULES`. Emitted bytecode is only PROVED by
running it, so any change to recording or lowering also wants
`make verify-bytecode` (the interpreter/compiler differential, the
emitter and alloc pins, the -race concurrency gates) and
`cd eng/go && go test ./...` for the VM side. Coverage: compiler/go
has no standalone gate — it rides the repo-wide merged ADR-008
`make cover-gate`.
