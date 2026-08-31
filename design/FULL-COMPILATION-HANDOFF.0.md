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

As of the 2026-08-31 oracle closure (ten phantom truncated-region entries
removed; the corpus contributes no top-level loop-join entries — both its
loop-body-def rows sit inside fn bodies, where `FnBodyDepth` suppresses them):

	rows with transitions   4291
	transitions total       7443
	  def                    6361   85%
	  type-install           1048
	  undef                    30
	  def-replace               4
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
   **The sandbox now has its first client (2026-08-31), and the flip's
   central mechanism is proven at data level**:
   `TestBindingSandboxRollbackAndReplay`
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
   - **`BindDefReplace` has TWO producers with DIFFERENT replay
     semantics**, and a single twin arm would silently corrupt one of
     them: installDef's redefinition (drop the colliding overload, PUSH
     the captured new entry) versus `UninstallFnSigs`' signature undef
     (REMOVE the most-recent matching entry — possibly mid-stack — and
     push NOTHING; its note-time captured entry is just whatever sits on
     top, not a value to install). The kinds must split, or the spec must
     carry the discriminator, before the def-replace twin is written.
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
loop-join entries in).

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
| `core/go/bind_ledger_note_test.go` | `NoteBindTransition`'s arms directly — suppressions, position precedence, kind/name/depth |
| `core/go/check_state_passend_test.go` | the pass-end cleanup seam (LIFO, exactly once, reset by `Begin`, Clone-isolated) and the `SuppressBindLedger` bracket |
| `check/go/narrow_passend_test.go` | the narrowing pop at pass end (popped on top, left alone when buried) and that `AnalyseLoopBody` ledgers its joined installs |
| `lang/go/narrow_leak_test.go` | the user-visible half of the narrowing leak: a later `Run` reads the bound Lock, not the leaked carrier |
| `eng/go/checkstate_lifecycle_test.go` | that every new `CheckState` field is classified reset-by-`Begin` or persistent |
