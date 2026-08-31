# Full compilation — handoff for the bind-twin line

**Point in time: 2026-08-31; first written 2026-08-30 at `007ac5c`.** This is
a state-of-play note for whoever picks up Stage 4's remaining piece. The
design is [FULL-COMPILATION.0.md](FULL-COMPILATION.0.md); §6.5 is the section
that matters here. This document does not restate the design — it records
where the work stands, what has been measured, and the three or four things
that will waste a day if they are re-derived from scratch.

## Where the work is

Stages 0, 1 and 2 are landed. Stage 4's recorder and apply kernel are
landed. **What remains of Stage 4 is the bind twins**, and they are the
named fix for NUR110, for family L's conditional fn shadow, and for the
unledgered rebind-staleness gates.

The twins are not written yet. What exists is the measurement that has to
precede them: `CheckState.BindLedger`, an **inert** record of every
runtime-visible binding transition the check pass performs, in source
order. Nothing reads it to decide anything, so it cannot regress a program;
it can only size the next increment.

### The population, measured over `lang/spec` (7644 rows)

As of the 2026-08-31 oracle closure and the BindSigUndef split (ten phantom
truncated-region entries removed; two of the four old def-replace entries
were sig-undef NO-OPS — locked matches that remove nothing — and no longer
note at all, taking one row's only transition with them; the corpus
contributes no top-level loop-join entries and no REAL sig-undef removal —
the synthetic rows supply both shapes):

	rows with transitions   4290
	transitions total       7441
	  def                    6361   85%
	  type-install           1048
	  undef                    30
	  def-replace               2
	  sig-undef                 0   (corpus; the synthetic row supplies it)
	deepest single row         36   —  unpack 'boru:math-util' sqrt 16.0

`TestBindLedgerCensus` produces this. Three consequences, and the third
reverses an earlier plan:

- `def` is 85%, so its twin is the one whose cost matters; the other three
  can be the simple, obviously-correct ones.
- `undef` and `def-replace` together are **34 transitions in the entire
  corpus**.
- The deepest single row is **36, not 1672**. The earlier figure counted
  frame-locals. With those excluded no corpus program performs more than a
  few dozen replayable transitions, so **per-transition twin cost is not a
  budget concern** — an earlier note in §6.5 said it was, and that was
  wrong.

## The next increment, in order

1. ~~**Close the live-depth oracle's 9 mismatches**~~ **DONE 2026-08-31** —
   the oracle is landed green with no allowance
   (`test/go/langspec/bind_ledger_live_oracle_test.go`); see the section
   below for what the 9 actually were, because both prior readings of them
   were wrong in instructive ways.
2. **Emit the `def` twin while the check-pass installs are still KEPT**, so
   it is inert, and assert the replay matches. **DONE (2026-08-31), in two
   halves.** The TABLE half: every compiled Program carries
   `Program.BindTwins` — the pass's ledger mirrored entry for entry through
   `NoteBindTransition`'s own funnel (`EmitRecorder.RecordBindTwin`, fired
   after every ledger suppression, table-appended unconditionally — no
   Active/Suspend gate, because a second filter would be a second source of
   truth), and `TestBindTwinsEqualLedger` asserts table == ledger
   elementwise over every compiled corpus row, no allowance (7270 compiled
   rows, 4100 with twins, 0 mismatched). The POSITIONED half: an
   `evBindTwin` event is appended at note time whenever the recorder is
   LIVE, and lowers to an `OpBindTwin` instruction (arg = table index; a VM
   no-op under the keep regime) at the transition's production-order
   position; `TestBindTwinOpsArePlacedOrderedSubset` gates it — every op
   indexes a real entry, indices strictly increase within each unit, and
   placement is a SUBSET of the table by design (7124 of 7154 compiled-row
   entries placed; the ~30 unplaced are the suspended-recorder class —
   each/fold body defs, which have no stream home until the twin is
   arm-resident — plus ops discarded with an island). The FLIP's refusal
   logic is what will tighten subset to equality: a program with an
   unplaced twin must refuse rather than replay incompletely.

   Two lessons from the positioned half, paid for in one corpus row each:
   - `emptyFlexHookOperand`'s "no event recorded since the construction"
     proof enumerates BOOKKEEPING event kinds to skip (evDynBind, and now
     evBindTwin). Any future bookkeeping event kind must join that skip or
     it silently refuses `walk`'s empty-flex accumulator row
     (`corpus-core.tsv:50` went 7270 → 7269 compiled; found by diffing the
     compiled-row LIST against the previous commit, which is faster and
     sharper than re-running the census — keep that probe in the toolbox).
   - Golden churn was 4 tests, all hand-written disasm strings in
     `lang/go/bytecode_emit_test.go` — small because BIND_TWIN emits only
     at def/undef/type/join sites, and worth re-pinning by hand: each new
     BIND_TWIN line in a golden documents where a twin lands.
3. **Only then** flip to rollback-and-replay onto
   `core/go/binding_sandbox.go`, which is already built with the exact
   mints-retained / retirements-rolled-back partition §6.5 requires.
   **FLIP v1 LANDED (2026-08-31), staged behind `BORU_TWIN_REGIME=1` —
   default OFF stays byte-identical.** The machinery, end to end: the
   recorder reads the flag once per pass (`compiler.TwinRegimeEnabled` →
   `EmitState.twinRegime`), stamps `Program.TwinRegime` at Finalize, and
   under the regime refuses any program whose twin table is not FULLY
   stream-placed (the subset-to-equality tightening this step always
   promised: an unplaced twin is a transition the rollback would lose).
   lang's compiled entries (`RunAutoValues`, `RunCompiledStrict`) take a
   `SnapshotBindings` before the pass under the same switch and, for a
   stamped Program, roll back via the new
   `core.RestoreBindingsForReplay` at the one safe between-phases point —
   RestoreBindings minus the module-ledger rollback, because imports ran
   once on the pass and must NOT re-run on the next request; the
   namespace BINDING is what the def twin replays. At run time each
   placed `OpBindTwin` calls `core.ApplyBindTwin` (bind_twin_apply.go):
   push kinds re-install the captured entry (Push / PushType /
   PushTypeAdopted by Minted), an undef pops-then-retires-if-minted, a
   sig-undef removes the captured entry by identity (ID, else value
   equality — never by position), a def-replace pops then pushes. The
   COMPUTED-def pairing landed exactly as the needGlobal-coincidence
   fact below predicted: `ApplyBindTwin` skips a captured entry matching
   `TypeDef == nil && !IsConcrete && !IsBareTypeNode`, and that def's
   `OpBindGlobal` (its `GlobalBindSpec.Push` stamped by lowerDynBind
   under the regime) PUSHES the runtime value instead of SetAt — twin
   before bind in stream order, so replace-twins net zero with the push.

   **Measured on landing, whole corpus
   (`test/go/langspec/bind_twin_regime_test.go`, the regime lane —
   regime-compiled vs fresh interpreter): 6411 rows compiled, 0
   divergences, no allowance.** The default recorder compiled 6416 the
   same day: the full-placement gate costs exactly 5 rows (classified in
   the lane by the `twin regime:` refusal reason). The flip-era analysis
   called four of them "island-discarded" — **that mechanism label was
   WRONG, and the 2026-08-31 recovery instrumented rather than assumed**:
   all five rows record through the CLOSURE path (`tryRecordClosure` —
   `RecordFallback` never fires for any of them), so the do bodies
   compile to closure units whose defs are frame-local, and the
   interpreter's leak is what the rollback loses. Island-discard
   accounting in Finalize would have recovered NOTHING — and worse, an
   island-re-execution account is unsound for a capitalised def (the
   regime retains minted type IDs, so a VM-time re-run of
   `def Big Integer` re-mints against the surviving name-part — the
   bodyHasReplayHazard class). The landed recovery is §6.5-faithful
   instead (replay, never re-execution): **do-body twin ADOPTION** —
   `CallableSpec.BodyOnceKeepsDefs` (set on `do` alone: handler runs
   the body exactly once, defs leak; check-mode twin is
   `RunCarrierBodyKeepDefs`) licenses `EmitState.AdoptBodyTwins` after
   the closure record to PLACE each suspended twin whose noted position
   is a token site in that body's tree as a real `evBindTwin` after the
   call event, so the lowered `OpBindTwin` replays the captured
   identical entry once the unit returns. Adoption is FENCED (the
   Codex P1 round on #421, each fence verified by repro): only twins
   noted inside the dispatch's own outermost keep-defs body run
   (KeepDefsBodyGuard's bracket — an aliased quotation's earlier
   multi-run twins stay out), excluding sub-ranges noted under a
   nested non-keep body run (a nested each's per-element transitions
   — analyseHigherOrderBodyVals now suspends through
   BodyAnalysisGuard so the taint sees it), and only at the root
   stream (a do nested in a callback's compiled unit keeps its sound
   refusal). Each fenced-out shape refuses to the interpreter; the
   fences cost zero corpus rows. That recovered the four
   do-body rows (`do [def Big Integer …]`, the predicate variant,
   `do [def x 5 raise …]`, the quoted `[def zz 5 …] do`). The ONE
   remaining regime-only refusal is the suspended-recorder each-body
   leaking def (`bytecode-migrated.tsv:41`) — deliberately unflagged:
   a multi-run body's runtime re-runs the body per element where the
   ledger noted one generalized transition with carrier-valued
   captures, so a single replay would be wrong in count and value; it
   waits for arm-residency.
   Cross-request persistence — keep-on-compile's contract, now delivered
   by replay — is pinned by `TestTwinRegimeSmoke` (a replayed binding is
   readable by the next request's check pass on the same instance), and
   holds for adopted twins too (`do [def x 5] x add 1` then `x add 10`
   → 15 on the same instance).
   `Program.TwinRegime` also drives the disasm mode tags
   (`(inert)`/`(replay)`, `(push)` on global binds).

   What remains for the DEFAULT flip, in order: (a) run the regime lane
   long enough to trust it (it is committed and green — every push of
   this PR now exercises the flip corpus-wide); (b) arm-resident twins
   for the each-body row (which also closes NUR110 by replacing join
   twins with per-arm twins); (c) flip the default, delete the
   keep-regime latches the payoff list names (frozen-read/
   NotifyNameRebound gates, emit.go's rebind latches, family L's
   CondBodyDepth refusal, NUR037), and collapse `GlobalBindSpec.Push`/
   the twin-regime branches into the only path.

   The sandbox's first client and the data-level proof that preceded the
   flip: `TestBindingSandboxRollbackAndReplay`
   (`test/go/langspec/bind_replay_sandbox_test.go`) runs snapshot →
   compile pass → `RestoreBindings` → replay **from the Program's own
   recorded captures**: each note captures the `DefEntry` it installed
   (`RecordBindTwin(tr, entry)`; `Program.BindTwinEntries`, 1:1 with the
   table), and the harness re-installs those captures in table order —
   the exact contract a twin op executes. Whole corpus: 7644 rows cycled,
   0 failures — after every replayed transition the live depth equals the
   ledger's recorded depth, and the final stacks match the pass-left
   stacks in entry identity (TypeDef pointer, Minted; compared via the
   new `DefTable.Entries`). The skipped rows (undef / def-replace) are by
   design: an undef twin captures NOTHING — at VM time it pops whatever
   is then live and retires a minted type from the popped entry itself —
   and a def-replace twin must reproduce the overlap-drop; both are
   VM-op work, not data-replay work. One operand decision is thereby
   already made and proven: PUSH-kind twins carry their entry, captured
   at the note (the only moment the identical object is knowably on
   top). The remaining operand decision is the COMPUTED value-def class,
   whose captured body is a check-pass carrier, not the runtime value —
   those twins must ride the evDynBind/OpBindGlobal seat machinery in
   push mode instead of replaying the capture.

   Facts established for the flip, each paid for:
   - `OpBindGlobal` today covers only ROOT defs of NON-concrete values
     (lowerDynBind's needGlobal — a concrete `def x 1` emits NO op at
     all), so under rollback every value-def twin needs an operand story,
     not just the write-back class. The def twin's value resolution wants
     to ride the evDynBind machinery (src/srcSeq/val + the promotion
     seats), which already exists per value def — not a second resolver.
   - A JOINED binding's twin replayed unconditionally at the join re-creates
     NUR110's leak at the VM level, but it also exactly REPRODUCES today's
     keep-regime behavior — so flip v1 may replay joins unconditionally
     with no regression, and arm-residency then closes NUR110 as its own
     increment.
   - The ledger's absolute depths are exactly reproducible over the
     rolled-back registry (the harness's per-transition assertion), so a
     twin op can trust its recorded depth at VM time.
   - A SetAt-based interim (widening OpBindGlobal to concrete defs under
     the keep regime) was considered and rejected: for a loop-body def the
     runtime value changes per iteration while the interpreter PUSHES per
     iteration, so the write-back is not behavior-neutral there — the flip
     changes semantics coherently or not at all.
   - The COMPUTED-value twin class coincides exactly with lowerDynBind's
     `needGlobal` predicate (root && !IsConcrete && !IsBareTypeNode): a
     const-folded `def x (1 add 2)` records a concrete 3 (verbatim class),
     and a bare-node body is self-representing — so at the flip, "skip the
     BIND_TWIN whose captured entry matches needGlobal's predicate, and
     flip that def's OpBindGlobal from SetAt to Push" partitions the def
     population with no third case.
   - **`BindDefReplace` HAD two producers with different replay semantics
     — split DONE (2026-08-31, `BindSigUndef`).** The conflation was not
     theoretical: probed live, a plain two-overload fn's signature undef
     removed an entry (depth 2 → 1) while recording delta 0, and the
     composition gate read clean because the corpus lacks the shape
     entirely (the blind-spot class again; the synthetic rows now supply
     both it and the locked no-op counterpart). `UninstallFnSigs` now
     commits and notes each removal individually as `BindSigUndef`
     (delta -1, the note carrying the REMOVED entry via
     `NoteBindTransitionEntry` so a twin can remove that identical entry
     mid-stack), and notes NOTHING when every match is locked — a no-op
     is not a transition. `BindDefReplace` keeps exactly one producer:
     installDef's drop-then-push redefinition.
   - **Islands re-execute; twins must not double-apply there.** A do-body
     compiled as a FALLBACK island re-runs its tokens in a sub-engine at
     VM time, executing any `def` inside it for real — so a twin event
     discarded with the island's events is CORRECT, not a placement gap.
     The unplaced-twin refusal the flip needs must therefore distinguish
     "discarded into an island that re-executes the transition" (sound —
     do not refuse, do not re-apply) from "noted under a suspended
     recorder with nothing to apply it" (refuse). Conflating them either
     double-installs do-body defs or refuses every islanded program.

**Do not invert that staging.** Rollback-first breaks every program until
the last transition has a twin, and offers no intermediate state where the
differential means anything. This is the same reasoning that made Stage 2
land its three re-seats separately.

## The strong oracle: landed, GREEN, no allowance (2026-08-31)

`Boru.NativeRegistry()` exposes the live registry, so the ledger's final
depth per name is compared against the depth the check pass **actually
left**. That is a strictly stronger check than `TestBindLedgerDepthsCompose`'s
self-composition, and it is the assertion step 2 above needs. It is landed as
`TestBindLedgerLiveDepths` (`test/go/langspec/bind_ledger_live_oracle_test.go`),
over the corpus AND the synthetic rows, at **0 mismatches with no allowance**.

The 9 mismatches closed as THREE fixes, and both prior readings of them were
wrong in ways worth keeping:

- **Gensym temps were not "macro construction"** — they were two
  snapshot/restore-truncated eval regions whose installs the ledger recorded
  while their own `Restore` tore them down: `expandMacroWith`'s template run,
  and the DYNAMIC-HELP example eval (`makeDynamicEval`), which fires mid-pass
  from the fn-registration hook and was the phantom "construction-time
  expansion" (found by stack-tracing the note, not by reading). Both now
  bracket with `CheckState.SuppressBindLedger` — the truncation rule the body
  runner already carried, applied at two more truncation sites.
- **The module-io multi-pass hypothesis was tested first, and it was wrong**
  — the doubling reproduces in a single pass and at plain runtime. The real
  cause was a genuine leak: `narrowDynamicUses`' analysis push had no
  top-level popper, so the leaked carrier SHADOWED the real binding for later
  Runs on the same instance (`typeof l` answered `dynamic(Ideal)`, not
  `Lock`). Fixed with a guarded pass-end pop (`CheckState.PassEndCleanups`);
  pinned by `lang/go/narrow_leak_test.go` and `check/go/narrow_passend_test.go`.
  So the warning above ("the oracle may be the fault") had the right posture
  and the wrong mechanism: the oracle was RIGHT, and what it found was a
  user-visible bug, not a bookkeeping gap.
- **The synthetic while row was itself malformed** — `while (n gt 0) […]`
  hands `while` a Boolean its `(List, List)` signature refuses, so the row
  never exercised a loop and "passed" composition while measuring nothing
  (the same lesson as the corpus blind spot, one level up: a gate's OWN
  synthetic input can be the thing that cannot contain the case). Corrected
  to `[n gt 0]`, the oracle immediately showed `AnalyseLoopBody`'s final
  joined post-loop pushes bypassing the ledger (raw `r.Defs.Push`, no note).
  They are now ledgered — the loop join is `InstallJoinedDefs`' one-branch
  rule and records like it.

Census after: 7453 → 7443 (ten phantom truncated-region entries out,
loop-join entries in); → 7441 after the BindSigUndef split (two locked-match
no-op notes gone).

**STRENGTHENED 2026-08-31 (Codex review round on PR #418), and the stronger
form found four more real defects.** The oracle now checks BOTH directions:
ledgered names against live depth (as before), and — for rows that COMPILED
— every name whose live depth changed across the pass with NO ledger entry
(the direction the first cut could not see; a wholly-missed transition
reported as success). Two documented scopings keep it honest rather than
tolerant: the unledgered direction runs only where a Program exists (a
refused or erroring row legitimately abandons partial state no twin will
replay — `gen [T] gen [U] …` raises with the outer binder still pushed, in
both engines), and `__`-prefixed interner bindings (`__const:`, `__gen:`)
are §6.5's own compile-time products. What the stronger form caught, each
fixed rather than excluded:

- **The both-arms branch join never noted.** `InstallJoinedDefs`' both-arms
  push (`if c [def op 1] [def op 2]`) had no `NoteBindTransition` at all —
  the function's own doc claimed every push noted — so a live binding with
  no ledger entry, invisible to the ledgered-names-only walk.
- **Narrowing-only "adds" recorded phantom defs at BOTH joins.** An arm or
  loop body that merely CONSUMED a dynamic name through a typed slot grew
  the def stack via `narrowDynamicUses`; the join re-pushed it and noted a
  `def` for a name the source never defines — and the joined carrier leaked
  past the pass (the staleness bug's join-level sibling). The discriminator
  is exact because narrowing preserves the value's ID: an add whose ID
  equals the pre-binding's ID is the same runtime value under a tighter
  bound — skipped at both `InstallJoinedDefs` and `AnalyseLoopBody`, no
  push, no note (`narrowedSameBinding`).
- **The truncated-region restores could not undo pops.** `Defs.Snapshot` /
  `Restore` is depth-based — restore can only TRUNCATE — so a macro
  template or help-eval that ran `undef` on a pre-existing name (or an
  overlapping redefinition) escaped the region while `SuppressBindLedger`
  suppressed its note. Fixed with the new CONTENT-preserving, IN-PLACE
  `DefTable.SnapshotEntries` / `RestoreEntriesSnapshot` (gen-guarded, so
  untouched names see zero cache churn and restored gens move forward).
  **A hazard paid for on the way: `RestoreBindings` — the table SWAP — is
  only safe at the between-phases point.** Used inside these nested
  regions it broke every reference the enclosing pass holds across the
  region, measured as ~200 corpus rows' ledgered defs vanishing from the
  post-pass registry. The flip's runtime client uses it exactly once,
  between CompileCheck and RunProgram, never nested.
- **The narrowing cleanup's pop guard needed the push-time DEPTH in its
  identity token** (Codex's P1): a later check-mode rebind can bind a
  carrier `ValuesEqual` cannot tell from the narrowing's own value, and
  value equality alone would pop that real binding; the depth rules the
  impostor out.

The loop-join POSITION limitation stands as recorded (every joined name
gets the note-time CurWordPos floor — a real site inside the body, not each
def's own): §6.5 already names the join position as the one to revisit, and
the flip's arm-resident twins replace join twins wholesale, at which point
each arm def carries its own position.

## What the ledger excludes, and why each exclusion was measured

Each of these was arrived at by instrumenting and counting, not by reading.

| exclusion | reason | effect |
|---|---|---|
| `FnBodyDepth > 0` | a fn body's `def` is a FRAME-LOCAL, pushed per call and popped by the frame teardown, which never reaches `UninstallDef`; the compiled lane gives it a slot, not a registry binding | 69254 → 6110 |
| `!shadow` in `installDef` | `InstallFrameBinding` shadows rather than removes, so a macro or fn PARAMETER does not outlive the pass | 6110 → 5705, incoherence 274 → 2 |
| `BindDefReplace` as its own kind | `installDef`'s overlap filter DROPS the colliding entry before pushing, so net depth is unchanged; a twin must replace, not push | incoherence 2 → 0 |
| `RolledBackBodyDepth > 0` | an install inside any `keep=false` body is undone by that body's own truncation | 7455 → 7453 |

`RolledBackBodyDepth` is deliberately **wider** than `CondBodyDepth`: a
condition fragment is truncated too, even though it is not conditional.
What makes an install unrecordable is the truncation, not the
conditionality.

## Two lessons this line has already paid for

### A green gate over a corpus that cannot contain the case is not evidence

`TestBindLedgerDepthsCompose` reported a clean **0 incoherent over 7644
rows** while every top-level branch-arm `def` was being recorded TWICE at
the same depth — the arm's own speculative `installDef` plus the joined
binding `InstallJoinedDefs` leaves behind, straddling a truncation in
`runCarrierBodyDefsAdds` that is not itself a transition. A twin built
against that ledger would have pushed the binding twice, and the
differential would have reported it much later as a mysterious divergence.

The gate was silent because `lang/spec` contains exactly three
`if …[def …]` rows and **all three are inside fn bodies**, where
`FnBodyDepth` suppresses them. The one shape the census exists to size —
NUR110's own — was the one shape the corpus could not report on.

`TestBindLedgerBranchArmDepthsCompose` now supplies synthetic top-level
rows (both arms, one arm, a pre-existing outer binding, two `if`s in
sequence, a `while` body) and was verified to FAIL without the exclusion:
8 incoherent transitions across 6 rows. **When the population being
measured is a shape the corpus lacks, the gate has to supply it.**

### A count of call sites is not a count of transitions

This section of §6.5 already carried that lesson about `modules.go`
(eleven export-install sites, ZERO recorded transitions — they are
test-setup helpers). It then had to be applied to its own author: a review
found **five instrumentation sites missing, 1750 transitions, 23% of the
population** (5705 → 7455), and one row of the §6.5 site table was not
merely incomplete but false — it asserted `word_extend.go` has "no direct
`Defs` mutation, it reaches the table through `installDef`". Each missed
site bypassed the function the reading had assumed was the funnel:
`InstallJoinedDefs`, `word_extend.go`'s two direct pushes, capitalised
`undef` → `Defs.PopEntry`, signature undef → `UninstallFnSigs`, and
`installDef`'s module-wrapper rebind early return.

## Positions: three attempts, all three wrong for different reasons

A twin is emitted at its source position, so every ledger entry needs one.
Both obvious sources are wrong, and the next author will reach for them:

- The **value's own `Pos`** is the VALUE token. `def x 1` gives 1:7, the `1`.
- **`CurWordPos`** has already MOVED by install time whenever the body was
  analysed first: `def f fn [[x:Integer] [Integer] [def y x y]]` gives 1:34,
  the INNER `def`.
- Staging the site in `CheckState.PendingBindPos` and **clearing it on each
  note** lets a suppressed body-local note steal the position its enclosing
  `def` had already staged.

What works is **save/restore** around each install, in
`InstallAndRecordDef` — the only frame that knows the def site. Precedence
is staged-site → value position → `CurWordPos` as the floor for values that
carry none (an `undef`, a word extension, a fn body).

One limit, stated rather than papered over: a JOINED branch binding lands
on the `if`, because the join runs after that dispatch and the arm's own
`def` token is already gone. Revisit when the twin op needs a finer
position than the construct that produced the binding.

## Process rules this line earned the hard way

- **For anything touching `installDef` / `carrier_join` / `carrier_body` /
  `resolveOperand`, wait for the FULL `make test` before committing.** A
  revert (`4c06f5a`) came from pushing on a single passing test while the
  `lang/go` suite was still running.
- **Run `make -C kg graph` after editing any tracked design doc.** Two CI
  failures came from forgetting.
- **Never run a second coverage job alongside `make cover-gate`.** Doing so
  starved `TestServeStepShutdownDrains` (a 10s wall-clock bound on a
  signal-driven shutdown) and cost a full re-run to disprove. The clean
  re-run passed; the failure was self-inflicted load, and it is recorded
  here so the next person does not spend the same hour on it.
- CI runs `cover-gate-core` per PR, not the merged `cover-gate`. Both need
  to be green locally: a statement covered in the merged profile can be
  uncovered in the core-standalone one. `cover-gate-core` was red for two
  commits because core/go's own suite reaches `NoteBindTransition` only
  with check mode off, so every statement past its first guard was
  unreachable from that suite alone.

## Constraints still in force

- **No third NUR110 refusal attempt.** Three are recorded in `NUR.md` as
  measured and rejected. The rule all three miss is that a branch-arm `def`
  LEAKS and SHADOWS and exists iff the body ran — which needs a third
  binding state, which is what the twins are for. NUR110 stays open until
  they land.
- Do not rebuild the broad region routing (breaks
  `TestEmitSplitFormsIdentical`).
- Do not relax `!fd.ArgsReversed` in `vmNativeApplicable` (a measured -7).
- The interpreter stays the reference oracle. Islanding interpretation
  inside compiled code is not acceptable, and the interpreter is not an
  escape hatch.

## Where the tests live

| gate | what it pins |
|---|---|
| `test/go/langspec/bind_ledger_census_test.go` | the census, and that every entry names a real source site |
| `test/go/langspec/bind_replay_equivalence_test.go` | depth composition over the corpus, and over the synthetic branch-arm rows the corpus lacks |
| `test/go/langspec/bind_ledger_live_oracle_test.go` | the STRONG oracle: final ledger depth per name == the depth the pass left in the live registry, corpus + synthetics, no allowance |
| `test/go/langspec/bind_twin_emission_test.go` | the emission gate: every compiled Program's `BindTwins` table == the pass's ledger elementwise — what a recorder-lifecycle hole would break |
| `compiler/go/bind_twin_test.go` | `RecordBindTwin`'s contract directly: unconditional append (suspension must not filter), nil-receiver safe, the finalized Program carries a copy |
| `test/go/langspec/bind_twin_emission_test.go` (`TestBindTwinOpsArePlacedOrderedSubset`) | the placement gate: every OpBindTwin indexes a real entry, strictly increasing per unit; placement ⊆ table until the flip's refusal tightens it |
| `test/go/langspec/bind_replay_sandbox_test.go` | the sandbox's first client: rollback + ledger replay reproduces the pass-left registry over every push-only corpus row (depths per transition, entry identity per name) |
| `test/go/langspec/bind_twin_regime_test.go` | THE FLIP's lane (`BORU_TWIN_REGIME=1` via t.Setenv): regime-compiled vs fresh interpreter over the corpus, divergences gated at 0, refusals classified, compiled floor 6400; plus the hand-checkable smoke (concrete def, computed def, cross-request persistence, type install) |
| `core/go/bind_twin_apply_test.go` | `ApplyBindTwin`'s arms directly — each kind, the carrier-class skip, sig-undef identity match, `RestoreBindingsForReplay`'s pass-final module ledger |
| `compiler/go/bind_twin_test.go` (`TestFinalizeTwinRegimeStampAndPlacementGate`) | the regime stamp and the full-placement refusal (a table-only twin refuses; flag off tolerates) |
| `core/go/bind_ledger_note_test.go` | `NoteBindTransition`'s arms directly — suppressions, position precedence, kind/name/depth |
| `core/go/check_state_passend_test.go` | the pass-end cleanup seam (LIFO, exactly once, reset by `Begin`, Clone-isolated) and the `SuppressBindLedger` bracket |
| `check/go/narrow_passend_test.go` | the narrowing pop at pass end (popped on top, left alone when buried) and that `AnalyseLoopBody` ledgers its joined installs |
| `lang/go/narrow_leak_test.go` | the user-visible half of the narrowing leak: a later `Run` reads the bound Lock, not the leaked carrier |
| `eng/go/checkstate_lifecycle_test.go` | that every new `CheckState` field is classified reset-by-`Begin` or persistent |
