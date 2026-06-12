# AQL Bytecode — staged implementation plan

**Status:** in progress — Stage 0 DONE with a GO result
(`aql-bytecode-baseline.0.md`); Stage 1 recording pass LANDED; Stage
2 VM core + CLI opt-in + `if` and counted-`for` lowering LANDED
(range `for`, list conditions, break/continue pending). Companion to
`aql-bytecode-report.0.md` (the design) and
`aql-bytecode-revisions.0.md` (the June 2026 re-review that this plan
incorporates; read it first — it changes two requirements). Written
against main @ `6fe4b96`.

The plan follows the discipline that worked for TCO
(`TCO-STAGED.0.md`): independently shippable stages, nothing changes
default behaviour until late, every behavioural stage behind a kill
switch with a dual-mode differential gate, and a measurement gate
*before* the build-out so the investment decision is made on current
numbers, not the original report's stale ones.

## Ground rules

- **Two execution modes, one semantics.** The interpreter remains the
  default and the reference. Compiled mode is opt-in (`aql run
  --compile` / `AQL_COMPILE=1`) until the graduation criteria in
  Stage 7 are met.
- **The compiler is the checker.** The emitter is a recording side
  effect on the existing check pass (`eng/go/check.go`,
  `CheckState`); it must not fork the dispatch logic. Any construct
  the emitter can't lower marks its span for interpreter fallback —
  never a third semantics.
- **Differential gate from day one.** The TSV spec suites
  (`lang/spec/*.tsv`) run dual-mode (the `TestSpecProdTCODisabled`
  pattern): identical results, identical error taxonomy, or the stage
  doesn't ship.
- **Mandatory from v1** (per the revisions note): `TAIL_CALL_USER`
  with the interpreter's eligibility conditions (R2), the
  dynamic-resolution soundness condition on locals promotion (R3),
  and budget/taxonomy parity (`evaluation_limit` /
  resource-exhaustion errors) (R6 #27).
- **Placement.** VM + emitter live in the engine kernel:
  `eng/go/bytecode.go` (Program/Instr/opcodes), `eng/go/emit*.go`
  (the recording pass), `eng/go/vm*.go` (the loop), with the CLI flag
  in `cmd/go` and dual-mode spec wiring in `lang/go`. Follow
  `eng/go/CLAUDE.md` before touching the kernel.

## Developer experience

Bytecode is an **execution mode, not a build artifact** — the DX
model is a JIT's, not a compiler's. Decisions (several restate
report §7.7 and the readiness note; collected here as the
user-facing contract):

**Trigger.** Eager compile-at-load, riding the check pre-flight
`aql run` already performs; no build step, no persisted `.aqlc`
(which eliminates the staleness gotcha class). Rollout:

1. *Now (Stage 1):* tooling-only — `lang.(*AQL).CompileCheck` and
   the planned `aql check --emit` disassembly. Execution unchanged.
2. *Opt-in (Stages 2–6):* `aql run --compile` / `AQL_COMPILE=1`.
   Uncompilable programs run on the interpreter SILENTLY — same
   results either way, by the differential gate.
3. *Default-on (Stage 7):* the flag inverts into a long-lived kill
   switch (`AQL_NO_COMPILE`), the `Registry.TCO.Disable` pattern.

Embedding hosts get the same via `lang.Options`; modules compile
once per import and cache in-process. The REPL stays interpreted
(one-shot lines; cold-start cost exceeds the win; mode consistency
within a session beats µs).

**Explainability.** Because fallback is silent, the load-bearing DX
is answering "why didn't my hot loop compile?". The emitter's
four-class site taxonomy and first-offender refusal reasons
("code-body word if (Stage 2)", "polymorphic dispatch at add") are
that answer; surface them via `aql check --emit` (disassembly +
site-class counts) and a compile report (info-severity diagnostics
naming each fallback span and its reason) so "make this compile"
becomes an actionable refactor.

**Debugging and source maps.** Required, and structural:
`Program.Debug` is the source map — a 1:1 pc → SrcPos table; every
instruction carries the dispatching word's position; it lives inside
the in-memory Program (no external map files, nothing persisted).
Per stage:

- *Stage 2 (hard requirement):* on a handler error the VM wraps
  `Debug[pc]` and routes through the SAME error-format path as the
  interpreter — error text byte-identical, so error-scraping tooling
  never learns which mode ran.
- *Stage 5:* mixed-mode stack traces — each frame (compiled or
  fallback span) renders from its own map.
- *Trace mode:* compiled mode disables itself under `--trace` until
  a PC-level trace that renders spans exists (semantics are
  identical, so tracing the interpreter is tracing the program).
- *Stage 3 follow-on:* a slot → name table per CompiledFn once
  locals exist, so a future debugger can show bindings. Pointless
  before locals; cheap after.

## Stage 0 — re-baseline and go/no-go (measurement only)

**Status: DONE (June 2026) — gate result GO.** Corpus:
`lang/go/bytecode_baseline_bench_test.go`; results and attribution:
`aql-bytecode-baseline.0.md`. Dispatch machinery measured at ~96% of
engine execution on the straight-line arithmetic shape (~3.5% actual
arithmetic), ~22µs and ~100 allocs per dispatch, ~340 allocs per fn
call frame — far above the 40% gate below.

No engine changes. Build the benchmark corpus and measure the
*current* interpreter, because the tape rewrite (`9903045`) and TCO
(`92a5931`…`5b1537c`) invalidated the report's §7 numbers.

- Microbenchmarks, one per dispatch shape (report §7.5): arithmetic
  chain, comparison chain, `if` scalar/list cond, counted `for`,
  `each`/`fold` over typed lists, record field access, string ops,
  deep and tail recursion, a `do`-heavy orchestration script.
- `pprof` the interpreter on the corpus; record the share of runtime
  in `matchSignature`, forward collection, splice bookkeeping, and
  small per-call allocations (`-benchmem`). That share is the
  theoretical ceiling of the whole project.
- **Gate (go/no-go):** dispatch + collection + splice ≥ ~40% of
  runtime on the compute-shaped benchmarks. Below that, v1's realistic
  win is <2× and the maintenance cost of a second execution mode is
  not justified — stop here and record the numbers.

Deliverable: `design/aql-bytecode-baseline.0.md` with the corpus, the
profile, and the decision. *~1 week.*

## Stage 1 — Program model + recording pass, straight-line only

**Status: RECORDING PASS LANDED (June 2026).** `eng/go/bytecode.go`
(Program/Instr/opcodes/disassembler) and `eng/go/emit.go` (EmitState:
ID-based operand provenance, the four-class site taxonomy, Stage-1
linearizer with simulated-stack discipline checks) ship with
`lang.(*AQL).CompileCheck` as the entry point and golden tests
(`lang/go/bytecode_emit_test.go`): the mirror forms `add 1 2` /
`2 1 add` lower identically, paren results chain through the
simulated stack, literal-substitution defs inline via provenance,
top-level stripped literals intern via RecordStrip, and every
beyond-Stage-1 construct (code-body words, user fns, polymorphic and
dynamic sites, recovered dispatches) is refused with a precise
reason. Remaining Stage-1 items: the CLI surface (`aql check
--emit`) and broadening the accepted operand shapes.

Introduce `Program` (code, constants, sig table, debug spans,
max-stack) and the emitter as a side effect of the check pass:
literal pushes (`PUSH_CONST` with interning), monomorphic
`CALL_NATIVE` at sites where `execMatch` resolved a single signature,
and nothing else — any other construct flags the program
"uncompilable" and `--compile` silently runs the interpreter
(whole-program fallback; span-level fallback comes in Stage 5).

- Operand sourcing comes from the checker's `MatchResult`
  positions; prefer emit-order choices that make
  `rearrangeForForward` a no-op, else emit `SWAP`/`ROLL`.
- Label/constant resolution as a final pass; `MaxStack` computed
  during emission.
- **Prerequisite (added June 2026): LANDED.** The checker's
  per-alternative disjunct dispatch fix — finding A1 of
  `checker-accuracy-review.0.md` — shipped on this branch
  (`disjunctPartitionReturns`). A strict disjunct input used to
  first-match as a whole, selecting a signature the runtime may not
  take; baking that `sig_id` into `CALL_NATIVE` would have called
  the wrong handler. The emitter consumes the partition directly:
  monomorphic where all alternatives agree on the signature,
  `CALL_NATIVE_POLY` exactly where their choices diverge, and the
  `partial_dispatch` warning marks paths needing a runtime guard.
- **Gate:** a `Program` disassembler (`aql check --emit` or similar,
  debug-only) + golden tests for the lowering of each spec row the
  emitter accepts. No VM yet — emission correctness is checked
  structurally.

*~1–2 weeks.*

## Stage 2 — VM core + control flow

**Status: VM CORE + `if` + counted `for` LOWERING LANDED (June
2026).** `if` (3-arg, value/paren conditions) lowers to
JMP_IF_FALSE / JMP via fragment-scoped recording: the branch
ReturnsFn arms a capture so each branch body's events record into an
EmitFragment instead of suspending, and RecordBranch composes them
into a branch event the lowerer emits with patched forward jumps.
Closed-branch rule: a fragment may consume only its own productions
and constants and must net exactly one value (reads of enclosing
computation refuse — Stage 3, with locals); list-form conditions
refuse (no checker provenance — follow-on); 2-arg `if` refuses.
Conditions use the engine's CoerceBoolean truthiness. All jumps are
forward and the VM enforces that, so the structural step bound
still holds; the budget ships with the first back-edge (loops).
Counted loops (`for n [body]`) lower to FOR_SETUP / FOR_NEXT with
the iterator as a VM local (PUSH_LOCAL; AnalyseLoopBody registers
the binding and captures the FINAL fixed-point round as the body
fragment) and the body's trailing JMP as the program's only
back-edge — the VM enforces that back-edges target FOR_NEXT, so
termination rides the loop counter. Loop results are VARIADIC (one
value per iteration accumulates, matching the interpreter); the
emitter refuses downstream consumption and only the program residual
absorbs them. Resource parity: the VM stack has the tape's
bounded-growth ceiling (same TapeConfig arithmetic) and overflowing
raises tape_exhausted. Known PRE-EXISTING interpreter divergence
discovered here: under a tiny ceiling the interpreter SILENTLY DROPS
a huge loop's results (exit 0, no output — the "ceiling-dropped
splice" misdiagnosis genre pinned in TCO Stage 0) where the VM errs
loudly; the VM keeps the loud behaviour. Range-form `for`, bodies
netting ≠1 value, and `break`/`continue` are follow-ons.
Differential: 549 rows / 0 mismatches, floor 500. `eng/go/vm.go` executes the straight-line
instruction set (handler errors stamped with `Debug[pc]` + source; a
belt-and-braces guard refuses tape-coupled handler results). CLI
opt-in shipped: `aql run --compile` / `aql do --compile`
(`lang.(*AQL).RunCompiled`, silent interpreter fallback) and `aql
check --emit` (disassembly or the refusal reason). **The compiled
path is NEVER the default** — flag-only, per the ground rules.

The differential gate is live
(`test/go/langspec/compiled_differential_test.go`): every spec value
row the emitter accepts runs through BOTH engines — 537 rows
compiled, 0 mismatches, with a 150-row floor so a regression to
refusing the corpus is caught. Getting to 0 hardened the emitter
with guards the gate itself discovered: per-occurrence (non-pooled)
compound constants (`eq` identity — the report's gotcha #13, caught
empirically), a plain-data whitelist for the constant pool (carriers,
type bodies, reaches, splices refused — canonical-pointer staleness),
refusals for function-valued operands, quoted-operand and
type-operand words, anonymous fn-value dispatch, dynamic outputs,
and residual-stack reconciliation (bare-literal programs compile to
`PUSH_CONST`).

The `for { switch op }` loop over `Program`, with the operand stack,
and the mark/move lowerings: `if` → `JMP_IF_FALSE`/`JMP`, `for` →
`FOR_SETUP`/`FOR_NEXT` (hidden iterator + accumulator slots), `break`
/`continue` → static jumps. Errors wrap `DebugInfo[pc]` and must
render identically to interpreter errors (report §10.5: shared
error-format path).

- **Budget parity from this stage:** the VM counts steps against the
  same `TapeConfig`/`lang.Options`-derived limits and raises the same
  `evaluation_limit` taxonomy (R6 #27). Pre-size the operand stack
  from `MaxStack`; frame-depth and growth ceilings mirror the tape's
  bounded-growth behaviour (loud failure, never unbounded
  allocation).
- **Gate:** dual-mode run of the control-flow and arithmetic spec
  TSVs — identical values *and* identical error taxonomy, including
  the runaway cases. Kill switch: `--compile` off by default.

*~2 weeks.*

## Stage 3 — user fns, frames, and mandatory tail calls

`CALL_USER`/`RET` with call frames (return PC, locals base), param
*and capture* slots (`fn_capture.go` computes captures at
construction), recursion by `fn_id` — and `TAIL_CALL_USER` in the
same stage, because tail-call elimination is a language guarantee
(`TCO.10.md`), not an optimisation to defer.

- **Eligibility at emit time** reuses the interpreter's conditions
  (`fn_frame_probe.go` / `fn_frame_elide.go`): tail position by
  structural analysis, identity via `FnFrameMeta`, generic frames
  excluded (`HasGen`), name-coverage over torn-down bindings, and the
  Stage 4b `returnsConform` split — FULL tail call (drop the caller's
  return check) when callee returns conform, else a checked variant
  that preserves the caller's `__RC` semantics.
- **Locals promotion under the R3 condition:** a `def`/param/capture
  becomes a frame slot only when the checker proves no dynamically
  reached callee resolves the name (`DefsUsed` analysis); otherwise
  emit `REG_DEF_PUSH`/`REG_DEF_POP` registry ops, which preserve the
  interpreter's innermost-binding-wins visibility exactly. Benchmark
  both paths; expect registry ops to be common at first.
- Stage-5 residual boundaries (module pins, foreign frames,
  same-registry `CallAQL`) decline tail treatment, same as the
  interpreter (R6 #30).
- **Gate:** dual-mode `lang/spec/recursion.tsv` (the TCO Stage 0
  pins) including the taxonomy rows: tail runaway →
  `evaluation_limit`, non-tail → resource exhaustion, in *both*
  modes. Depth-10000 tail chains run in O(1) frames under a small
  ceiling, matching the interpreter's Stage 4 results.

*~2–3 weeks. This is the stage with real correctness risk; do not
compress it.*

## Stage 4 — the compile-time meta layer

Everything that runs during the check pass and emits (almost)
nothing: `RunInCheckMode` words (`def`/`fn`/`type`/`import`/
`module`/`var`), **macro and minilang expansion** (run
`execMacro`-equivalent expansion during the recording pass — it is
deterministic and memoised by operand canon — and lower the expanded
stream; raw-form operand spans are never lowered pre-expansion, R6
#29), **generics** (one `CompiledFn` per memoised instantiation,
bounded by the existing `of` interning; generic fns stay excluded
from tail elision), multi-signature fns (`CompiledFnSet`,
`CALL_USER_POLY`), function-value call sites (F4: emit poly dispatch
or fallback where the checker couldn't resolve the callee), and
module imports (per-module compile cache; expansion and import inputs
hashed into the program identity, R6 #25).

- **Gate:** dual-mode runs of the fn-model, macro, minilang,
  generics, and module spec suites. Golden tests that a macro/mini
  site lowers to its expansion and an import compiles once.

*~3–4 weeks.*

## Stage 5 — dynamic fallback + concurrency

Span-level `FALLBACK_INTERP`: `do` on computed lists, unresolved
`context get`, leaky runtime `def`, and any site the checker widened
to `Any`. The VM hands the relevant values to an engine instance over
the recorded token span and resumes; `TYPE_CHECK` ops guard every
fallback→compiled boundary with the checker's expected carrier
(report §9.1). `break`/`continue`/`return` sentinels crossing the
boundary translate to the enclosing loop labels / frame unwind
(report §10.6).

Concurrency: `Program` and all its tables are immutable after
compile — `ForkConcurrent` branches share them; all mutable VM state
(stacks, frames, any caches) lives per-registry/per-fork (R6 #28).
v1 runs `await`/timer branch bodies as interpreter fallbacks;
compiled per-fork entry points are a later optimisation.

- **Gate:** dual-mode runs of the full spec corpus — at this point
  every program either compiles or falls back, so the whole suite
  must pass in compiled mode. Race detector (`go test -race`) over
  the concurrent spec rows in compiled mode.

*~2 weeks.*

## Stage 6 — specialisation (benchmark-driven, each independent)

Only after Stage 5 is green, and only as the Stage-0 corpus directs:

- **Sig splitting for `ReturnsFn`** (report §9.4) — the single
  biggest lever: auto-generate monomorphic sig_ids (`add_i_i`, …) so
  integer chains stay monomorphic. Regression-test mono propagation
  on chains explicitly.
- `CALL_NATIVE1_1`/`CALL_NATIVE2_1` fast paths; compact encoding.
- Inline caches at `CALL_NATIVE_POLY`/`CALL_USER_POLY` sites —
  per-fork cache storage only (R6 #28).
- Typed opcodes / unboxed cells remain v2+; do not start them here.
- **Gate:** each item lands with before/after numbers against the
  Stage-0 baseline on the same corpus.

*~1–2 weeks per item taken.*

## Stage 7 — graduation criteria (compiled mode by default)

Flip the default only when all of:

1. Differential gate (full spec corpus, dual-mode, values + error
   taxonomy) clean in CI for a sustained period across language
   changes — not just at a point in time.
2. Compiled ≥ the Stage-0 go/no-go multiplier on the compute
   benchmarks; ≤10% regression on the fallback-heavy benchmarks
   (compile cost included).
3. Tooling parity for the supported surface: PC→span error rendering
   byte-identical, `aql check` diagnostics unchanged, trace mode has
   a PC-level equivalent or compiled mode disables itself under
   `--trace`.
4. A documented kill switch (env var) that survives graduation, the
   way `Registry.TCO.Disable` did — demoted to diagnostic later, not
   removed early.

## What is explicitly out of scope

`.aqlc` persistence (compile in memory per run; staleness class
eliminated per report §9.2), partial recompilation, JIT/tracing,
typed opcodes and unboxed stack cells (v2+ per report §7.6), and
compiled `await` branch bodies.

## Risk register (delta to report §12.4)

1. **Stage 0 says no.** Entirely possible post-tape/TCO — that is
   the point of the gate. The fallback position is to keep the
   baseline doc and revisit when workloads change.
2. **R3 makes locals promotion rare**, dragging fn-heavy benchmarks
   toward registry-op cost. Mitigation: measure in Stage 3; invest
   in the `DefsUsed` proof precision before reaching for Stage 6
   toys.
3. **TCO parity bugs** (R6 #23) — the only stage that can silently
   change documented resource semantics. Mitigation: Stage 3's
   taxonomy gate runs the interpreter's own pin suite in both modes.
4. **Two-mode maintenance drag** — every language change must keep
   the differential gate green. Mitigation: the emitter rides the
   checker, so changes that bypass the checker can't ship anyway;
   budget the gate into CI from Stage 2.

## Rough total

~12–15 weeks of focused work to Stage 5 (feature parity, opt-in),
plus benchmark-driven Stage 6 items. Stage 0 is ~1 week and can be
done immediately; nothing else should start until its numbers are in.
