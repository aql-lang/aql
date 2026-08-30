# Full compilation — handoff for the bind-twin line

**Point in time: 2026-08-30, `007ac5c`.** This is a state-of-play note for
whoever picks up Stage 4's remaining piece. The design is
[FULL-COMPILATION.0.md](FULL-COMPILATION.0.md); §6.5 is the section that
matters here. This document does not restate the design — it records where
the work stands, what has been measured, and the three or four things that
will waste a day if they are re-derived from scratch.

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

	rows with transitions   4291
	transitions total       7453
	  def                    6371   85%
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

1. **Close the live-depth oracle's 9 mismatches** (below). §6.5's staging
   requires asserting that the replayed binding state matches what the pass
   left, and that assertion is the oracle.
2. **Emit the `def` twin while the check-pass installs are still KEPT**, so
   it is inert, and assert the replay matches.
3. **Only then** flip to rollback-and-replay onto
   `core/go/binding_sandbox.go`, which is already built with the exact
   mints-retained / retirements-rolled-back partition §6.5 requires, and
   whose header says "NO CLIENT YET" for precisely this reason.

**Do not invert that staging.** Rollback-first breaks every program until
the last transition has a twin, and offers no intermediate state where the
differential means anything. This is the same reasoning that made Stage 2
land its three re-seats separately.

## The strong oracle: built, run, and NOT green

`Boru.NativeRegistry()` exposes the live registry, so the ledger's final
depth per name can be compared against the depth the check pass **actually
left**. That is a strictly stronger check than `TestBindLedgerDepthsCompose`'s
self-composition, and it is the assertion step 2 above needs.

Run at `007ac5c`: **9 mismatches over 4291 rows**, in two classes.

| class | rows | shape |
|---|---|---|
| macro-expansion gensym temps | 8 | `edge-quote-3.tsv:39-42`, `macro.tsv:24-25`. A macro's CONSTRUCTION-time expansion installs a hygiene temp (`tmp$g1`, `t`, `g`) that is later torn down; the install is recorded, the teardown is not. The REAL expansion's temp (`tmp$g2`) is live and correctly recorded |
| a doubled module-io install | 1 | `module-io.tsv:223`, `def l (IO.lock …)` — ledger 1, live 2, so a second install is unaccounted |

It is **deliberately not landed as a gate with an allowance**. A gate that
reports a number it tolerates teaches the next author to tolerate it.

Two warnings for whoever closes these:

- For the gensym class, find the teardown site and decide record-the-pop
  versus suppress-the-install **by measuring which one the runtime leaves**.
  A first probe of this class produced a reading that contradicted the
  per-row dump; the dump was right. Reading the call graph is a hypothesis.
- For the module-io row, **test the multi-pass hypothesis first**:
  `CheckState.Begin` resets the ledger per pass while the registry
  accumulates across passes. If that is the cause then the ORACLE is wrong,
  not the ledger, and "fixing" the ledger to match would be a fabricated
  result.

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
| `core/go/bind_ledger_note_test.go` | `NoteBindTransition`'s arms directly — suppressions, position precedence, kind/name/depth |
| `eng/go/checkstate_lifecycle_test.go` | that every new `CheckState` field is classified reset-by-`Begin` or persistent |
