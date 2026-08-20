# design/ — design notes index

This folder holds two kinds of document:

- **Living references** — notes that track the current implementation and
  are kept correct as the code evolves. Read these as authority.
- **Legacy / historical** — point-in-time reports, completed plans, and
  accepted proposals, retained **purely for project history**. They record
  how the system got to where it is, and may describe designs, function
  names, or behaviours that no longer exist. Do **not** treat them as a
  description of the current engine.

The numeric suffix in a filename (`.10`, `.8`, `.0`, …) is a document
revision/era marker, **not** a status flag — a high number does not mean
"current," and several `.10` docs are historical. Use the classification
below, not the suffix.

This index was produced by a forward-args-focused documentation audit; the
argument-ordering / dispatch cluster is classified exhaustively, and the
wider historical material is listed so its status is unambiguous. Topic
specs not listed here (e.g. domain-word specs) were not re-audited in this
pass and carry no implied status either way.

> **Canonical source of truth for argument ordering / forward args** is the
> code-adjacent guide `eng/go/CLAUDE.md` ("Signature Ordering") and
> `lang/go/CLAUDE.md` ("Argument Ordering"), backed by the living references
> below. The single rule: each signature's `BarrierPos` marks the `|`;
> positions before it are forward-eligible (filled from forward tokens in
> source order, else the stack), positions after it are stack-only; the
> stack is consumed top-down (sig[0] = top). This is ONE rule at every
> arity — two-arg words are **not** a special case and there is no
> "swap form"; a call form only chooses where the split falls, so
> `a f b` binds a different pair than `f a b` for exactly the reason
> `c f a b` and `f c a b` bind differently at three args. Surface style: **infix** for the two-arg words
> convention reads as operators (`1 add 2`, `10 sub 3`), **forward
> form `f a b c`** for everything else (`STYLE-GUIDE.md` §S2).

## Living references — argument ordering & dispatch

- `ADR-004-REFINEMENT.0.md` — the four argument-handling categories
  (forward-eligible, mixed-barrier, stack-only, quoting slots),
  `BarrierPos` semantics and its single resolution boundary, the
  stack-only closed list and its admission test, and the composition
  rationale for the forward default. **Draft material for a refined
  ADR-004** (NUR023): its description of current behaviour is accurate
  and current, but it is *discovery*, not an ADR entry — promotion into
  `ADR.md` is a separate maintainer decision.
- `SIGNATURES.10.md` — signature shape, top-first positions, `/q` rules.
- `SIG-ORDER-REFACTOR.10.md` — the §1.4 top-first unification (end state).
- `FORWARD-COLLECTION-PHASES.10.md` — the two-phase collection model.
- `FORWARD-COLLECTION-TRAPS.0.md` — collection edge cases.
- `FORWARD-STRAND-ADVISORY.10.md` — mixed-dispatch / infix-split advisory.
- `FUNCTION-MODEL.10.md` — unified `Signature`/`FnSig`, single dispatch path.
- `USURP.10.md` — `/u` wrapper and `BarrierPos` interplay.
- `REACH.10.md` — dot-access lowering to `get`/`getr` chains.
- `ENGINE.10.md` — core engine model (argument-equivalence principle).
  *Corrected in the latest audit (removed the stale "prefix tried first,
  forward as fallback" framing; added the split-classes note).*
- `LANGREF.10.md` — language reference. *Corrected in the latest audit
  (same forward-args phrasing; anchored `sub` to `args[1] - args[0]`).*

## Legacy / historical — argument ordering & dispatch

These are banner-marked in-file; they name a multi-path dispatcher that no
longer exists (the engine unified onto one `BarrierPos`-driven rule):

- `ENGINE-UNIFIED-ALGO.8.md` — pre-unification *proposal*; its "current
  behaviour" review (incl. the claim that `flexibleMatch` permutes args) is
  obsolete — the shipped matcher never permutes.
- `SIGNATURE-MATCHING-PSEUDOCODE.10.md` — pseudocode for the removed
  prefix-vs-forward two-mode dispatcher, `MatchSignatureReversed`, and the
  `SequentialPlanner` feature flag.
- `LAZY-ARG-RESOLUTION.10.md` — accepted proposal; its eager-probe "current
  behaviour" sections predate the shipped structure-first model.

> Note: `PORT_OBSERVATIONS.5.md` §1.4 quotes the old two-category
> ("forward-collecting vs stack-only") model, but as a *resolved* problem
> statement immediately followed by the current `BarrierPos` resolution —
> it is historical narrative, not a stale live claim.

## Legacy / historical — point-in-time reports, reviews & analyses

- `BORU-CODE-REVIEW-REPORT.6.md`, `REVIEW-NOTES.10.md`,
  `FOR-LOOP-REVIEW.10.md`, `TYPE-SYSTEM-REVIEW.7.md`,
  `checker-accuracy-review.10.md`,
  `checker-compiler-architecture-review.0.md` — dated review snapshots.
- `BORU-DX-REPORT.5.md`, `BORU-DX-REPORT-LOG.md`, `BORU-DX-REPORT-DEBUG.0.md`,
  `VOXGIG-BORU-REPORTS.5.md`, `VOXGIG-DX-REPORT.5.md`,
  `DESIGN-DX-AND-BYTECODE-STATUS-REVIEW.md` — DX experience reports /
  status snapshots (the last consolidates and closes out the DX reports).
- `BATTERIES-INCLUDED-REPORT.5.md`, `CARRIER-STATIC-TYPECHECK-REPORT.10.md`,
  `STATIC_ANALYSIS_REPORT.10.md`, `checker-loud-diagnostics-report.10.md`,
  `boru-boolean-operations-report.10.md`,
  `boru_property_based_reduction_report.10.md`,
  `jsonic-matcher-rule-access-report.10.md`, `data-last-audit.10.md`,
  `WAT-AUDIT.5.md`, `FORMAL-FINDINGS.0.md` — feature/feasibility/audit
  reports pinned to a point in time.
- `TYPE-REPRESENTATION.0.md` — why a type NAME does not always denote its
  type. One lattice (`*Type`), but `IsTypeBody` unions 18 value shapes and
  `InstallType` has 11 branches using 3 strategies to bind a name to one.
  Two strategies work everywhere; the catch-all branch leaves the name
  usable in some positions and not others. Carries the branch table, the
  measured surface consequences, two corrections this audit had to make to
  its own earlier claims, and a staged fix. NUR090 / NUR091, issue #392.
  **Superseded 2026-08-20** by `TYPE-REPRESENTATION.1.md` (landed): the
  kind-split it measures no longer exists on the current tree.
- `TYPE-REPRESENTATION.1.md` — the type-node fusion: every named type
  evaluates to its minted lattice node; structure is recorded content
  (`TypeContentOf`), membership is Match + Unify on node Behaviors.
  **LANDED 2026-08-20 (PR #394)** — all five stages implemented; §9 is
  the implementation record, §6 the pinned semantic deltas. Resolved
  NUR090/093/094, issues #391/#392. Sections 0-8 are the design as
  proposed and measure the pre-flip tree.
- `FUNCTION-TYPES.0.md` — proposal + working prototype for a *declared*
  function type (`fnsig Integer String`, `fn` minus its body), answering
  the audit's "`Function` is opaque" finding. Measures what the change
  buys (a wrong-shaped argument moves from a run-time refusal to a
  check-time error) and what it does not (NUR089 is orthogonal and
  survives it). **Proposal, not merged** — §7 lists what production
  would still need. *(§5 item 3 — the pair form rejecting structural
  named types — retired 2026-08-20 by the type-node fusion.)*
- `HIGHER-ORDER-FUNCTIONS.0.md` — an empirical audit of higher-order
  support and combinator expressibility (2026-08-19): what was built and
  run, a comparison against Haskell/Scheme/JS/Factor, and the
  call-vs-value gotchas, plus §4.2 on the six spellings of one signature
  and which of them `boru fmt` collapses. **Point-in-time**, and
  deliberately so — it records behaviour that NUR073, NUR078, NUR086,
  NUR087 and NUR088 are each expected to change (NUR085 already landed:
  §5.2 is closed by the `/v` totality rule), so read its §4.2
  and §5 against the register rather than as current truth. The house
  rules it applies live in [`STYLE-GUIDE.md`](../STYLE-GUIDE.md).
- `CLIENT-FIXES-2026-06-24.md`, `CLIENT-VERIFICATION-MAIN-2026-06-24.md`,
  `FORCE-COMPILE-CLIENT-COVERAGE.0.md` — dated client-library verification
  reports.
- `STDLIB-COVERAGE.10.md`, `IMPLEMENTATION-STATUS.10.md` — coverage/status
  snapshots (inherently point-in-time).
- Comparative "X-in-boru" applicability studies (idea evaluations, not
  specs): `amop-in-boru-report.0.md`,
  `effect-oriented-programming-in-boru-report.0.md`,
  `elixir-types-in-boru-report.10.md`, `fsharp-units-in-boru-report.0.md`,
  `dynamic-modality-report.10.md`, `LISP-ANALYSIS.5.md`,
  `RACKET-ANALYSIS.5.md`, `RACKET-FEATURES-EXAMPLES.5.md`,
  `PORT_OBSERVATIONS.5.md`,
  `rust-zig-roc-faber-in-boru-report.0.md`,
  `roc-in-boru-report.0.md` (the deep Roc pass; supersedes the
  four-language report's §4, which the 2026 Roc rewrite made stale — and
  whose verification pass recorded NUR079 and NUR080),
  `unison-in-boru-report.0.md` (with
  `unison-hash-identity-probe.0.md`, the proof-of-concept pass that
  measured its central proposal and **corrected** it — the probe is
  reproducible via `scripts/hash-identity-probe.sh`; both are
  consolidated into the design note `CONTENT-ADDRESSING.0.md`, which
  is the one to read first),
  `verse-in-boru-report.0.md` (with
  `verse-report-defects-investigation.0.md`, the root-cause follow-up on
  the defects that report's verification pass turned up).

- **`COMPILE-REFUSAL-SURVEY.0.md`** — which compile refusals are still
  reachable, measured by running them rather than by reading the refusal
  strings. Answers "would an interpreter-identical signature-matching
  opcode remove the remaining refusals?" (no: that opcode exists, and none
  of the live refusals is a dispatch-resolution problem), and records
  specimens that now compile though `COMPILABLE-SUBSET.md` §5 lists their
  class as refusing.

## Legacy / historical — completed plans & phase docs

- `PLAN.10.md` (marked complete), `PERMISSIONS-PLAN.10.md`,
  `PBT-PLAN.10.md` (shipped), `MACROS-PHASE1.10.md` (complete),
  `MACROS-PHASE5.5.md`, `TCO-STAGED.10.md` (TCO now a language guarantee;
  see `TCO.10.md`).

## Legacy / historical — boru-bytecode project series

Point-in-time plan/status/deep-dive notes for the bytecode-compiler effort
(each pinned to a refusal-count/commit snapshot):

- `boru-bytecode-outline.0.md`, `boru-bytecode-plan.0.md`,
  `boru-bytecode-report.0.md`, `boru-bytecode-revisions.0.md`,
  `boru-bytecode-baseline.0.md`, `boru-bytecode-readiness.0.md`,
  `boru-bytecode-runtime-independence.0.md`, `boru-bytecode-completion.0.md`,
  `boru-bytecode-finish-line.0.md`, `boru-bytecode-next-stages.0.md`,
  `boru-bytecode-cluster5-residual-lowering.0.md`,
  `boru-bytecode-stage3-inlining-plan.0.md`,
  `boru-bytecode-stageA-branch-result-modeling.0.md`,
  `boru-bytecode-method-fnvalue-codebody.0.md`,
  `boru-bytecode-final-two-refusals.0.md`.

## Legacy / historical — superseded version series & handoff notes

- `module-fn-checkstate-ownership.0.md` … `.6.md` — superseded by `.7.md`
  (the current head of that series).
- `ACCESSOR-SPLIT-AND-CLEANUP-BUG.md` — session handoff / bug note.
