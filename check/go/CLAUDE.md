# check/go — Type checker CLAUDE.md

The `check` module is the static-analysis pass cut out of the kernel
(design/ENG-FOUR-PIECE.0.md): carrier typing and check mode, the
fn-body / construction / ordering mirrors, the dispatch-recovery
braid, and the standalone diagnostic passes — dead overloads, the
pure-word dry pass, guard-predicate narrowing, static index and size
checking, the shaped-method dispatch model, store-identity shapes.
It drives the SAME dispatch machinery core drives, over carriers
instead of payloads. The kernel conventions in
[eng/go/CLAUDE.md](../../eng/go/CLAUDE.md) apply to this module
verbatim — the payload seal, signature ordering, no zero-value
overload, canonical `*Type` pointers, the payload-presence predicates
— and that guide remains their single home.

Three rules specific to this module:

- **No upward imports.** The chain is core -> check -> compiler ->
  eng, and check requires core ALONE (`go.mod` names no other boru
  sibling). Never write a compiler or eng symbol here: the compiler
  reaches check only by installing into the S3 slot table below, and
  eng sees check only through its generated facade.
- **Downward into core, only through slots.** check's analysis and
  check-mode behaviors are handed to core at init — `AnalysisImpl`
  (S1, 9 slots) and `CheckBraid` (S9, 21 slots), both installed from
  `check_recovery.go` (`installAnalysisImpl` / `installCheckBraid`),
  plus `core.JoinCarriersHook` from `carrier.go`. Core-side tests pin
  each NAMED inactive default (`TestInactiveAnalysisImpl`,
  `TestInactiveCheckBraid`); a new slot follows the same pattern — an
  anonymous default replaced at init is unreachable and fails the
  merged ADR-008 gate.
- **The eng facade mirrors this surface.** `eng/go/aliases_check.go`
  is GENERATED (`piecetool -facade`); after changing check's exported
  surface, regenerate it and keep `eng/go/piece_map.tsv` current.
  Facade wrappers must remain direct calls so they inline — the
  alloc-ceiling tests gate the module boundary's cost.

**The S3 seam (check <- compiler).** Everything check asks of the
recording / folding / poly-bake machinery routes through
`DispatchBraid` in `dispatch_hooks.go`, so no check file carries a
named compiler symbol. Five slots — `RecordOutcome`,
`TryFoldScalarConst`, `TryRecordPoly`, `CompileUserPolyArms`,
`PlanUserPoly` — each backed by a NAMED inactive default
(`inactiveDispatch*`, the exact decline an unarmed recording pass
produces) and reached through a private forwarder
(`dispatchRecordOutcome`, …) so call sites read as they did before
the cut. A compiler-less build simply runs the defaults. The
compiler's arm table crosses the seam as `UserPolyPlan`, an OPAQUE
interface (`SubstituteJoinedOuts`, `SigIdx`, `Units`, `Impls`,
`Sigs`): the record site substitutes the return-join carriers and
unpacks the parallel slices without naming the compiler's concrete
plan type — which is why an installer must return a NIL INTERFACE,
never a typed nil, when the plan declines (check tests `plan != nil`).
The compiler installs the active half at init in
`compiler/go/dispatch_hooks_install.go`; `zz_finish_test.go`'s
`TestInactiveDispatchBraid` pins the inactive half.

Tests: `cd check/go && go test ./...` — a flat single package, no
sub-packages and no module Makefile. From the repo root `make test`
and `make test-race` fan out over check/go with the rest of
`MODULES` / `RACE_MODULES`. Because check's behavior is only observed
end-to-end through the pipeline above it, a semantic change wants
`cd eng/go && go test ./...` and the lang/test differentials too.
Coverage: check/go has no standalone gate — it rides the repo-wide
merged ADR-008 `make cover-gate`.
