# AQL Bytecode — staged implementation plan

**Status:** in progress — Stage 0 DONE with a GO result
(`aql-bytecode-baseline.0.md`); Stage 1 recording pass LANDED;
Stage 2 COMPLETE; Stage 3 COMPLETE (user fns, frames, mandatory
TAIL_CALL_USER incl. mutual tails, closures via capture slots, VM
step budget — self AND mutual tail recursion at depth 1M under a
tight ceiling in compiled mode); Stage 4 COMPLETE (the compile-time
meta layer: macro + minilang expansion, generic instantiations, type
operands, multi-overload monomorphization, module dot-access calls —
1412 spec rows compiled, 0 mismatches); Stage 5 GATE MET (the whole
spec corpus — 2607 rows — compiles-or-falls-back with 0 divergences in
BOTH values and error taxonomy; concurrent spec rows race-free in
compiled mode under `-race`; 1494 rows take the compiled path). Reaching
it closed two compiled-mode soundness gaps beyond the span-fallback
islands themselves: the fallback no longer double-executes check-pass
side effects (registry snapshot/restore), and the compiled path now
reproduces the interpreter's runtime guards (declared return type/count
enforced at RET; check-mode-suppressed strict errors — orphan gen,
unpack of a missing key — refuse compilation and fall back). The
native-compilation coverage follow-ons (F4 dynamic dispatch,
sentinel-crossing, multi-threaded islands, query-DSL words) remain —
every such program already runs correctly via fallback. Companion to
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
- *Debugger follow-on (with Stage 5's mixed-mode traces):* a slot →
  name table per CompiledFn so a future debugger can show bindings.
  Locals exist now (params, captures, iterators); the table is cheap
  whenever a consumer appears.

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

**Status: COMPLETE for the v1 scope (June 2026).** Everything this
stage names is lowered: `if` in all value-condition forms PLUS
list-form conditions (the condition body is captured as its own
fragment, emit-gated so plain checks are unchanged, and lowered
inline before JMP_IF_FALSE) and the 2-arg form (a VARIADIC result —
0 or 1 values — absorbed only by the program residual); counted AND
range `for` (FOR_SETUP pops the parseRange start/end/step triple;
literal ranges including negative steps; def-bound concrete ranges
compile, computed elements refuse); and `break`/`continue` as
fragment TERMINATORS — recorded as events, lowered to a JMP to the
loop end (patched holes) or back to FOR_NEXT, with diverging branch
arms contributing no value and skipping the merge jump
("both branches diverge" refuses). Remaining beyond-v1 control
flow (each/fold inlining, case, error) belongs to Stages 4–5. `if` (3-arg, value/paren conditions) lowers to
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
Differential: 568 rows / 0 mismatches, floor 550. The
silent-drop interpreter divergence found during this stage is fixed
(see the engine commit "ceiling-dropped tape edits err loudly"). `eng/go/vm.go` executes the straight-line
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

**Status: COMPLETE (June 2026).** Named, single-declared-return fns
compile as their own code units (`Program.Fns`, params as frame
locals in sig order) via the fn-body analysis hook: `StartFnCompile`
reserves the unit, registers GENERALISED carrier args as param slots
(a call's kept-concrete values must not constant-fold into the
shared unit — found and fixed), arms the body capture, and
`RecordUserCall` records call sites. The VM runs real frames
(per-frame locals, shared operand stack, loop-state bases) with
`CALL_USER`/`RET`, a frame-depth ceiling sharing the tape_exhausted
taxonomy, a step budget sharing the evaluation_limit taxonomy (a
tail spin trips neither ceiling) — and `TAIL_CALL_USER`: tail
positions are marked structurally (body-final calls, and branch arms
whose result is their own trailing call — tail arms reuse the
divergence machinery and skip the merge), and the VM replaces the
frame. The completion pass landed:

- **Mutual tail recursion** — the blocker was a checker FP, not a
  bytecode gap: at the top-level call site every fn is already
  defined, so the nested `StartFnCompile` machinery compiles both
  units and the arm tails lower as cross-unit `TAIL_CALL_USER`. The
  FP (`undefined_word` for a body's forward reference, flagged by
  the install-time analysis that runs before the later `def`) is
  fixed by tagging diagnostics emitted inside `AnalyseFnBody`
  (`CheckDiagnostic.FnBody`) and rescuing tagged undefined_word
  entries at end of pass when the name has a binding by then
  (`RescueForwardRefDiagnostics`) — call-time resolution is the
  documented idiom (recursion.tsv §3). Top-level use-before-def and
  never-defined names keep their diagnostics. (The plan's earlier
  `fnsig` pre-declaration sketch was wrong: `def g fnsig […]` means
  targeted sig REMOVAL — `UninstallFnSigs` — not forward
  declaration.)
- **Closures (capture slots)** — captures ride as hidden trailing
  param slots: the construction site supplies the enclosing frame's
  values (param local / produced value / const), a recursive call
  re-passes the frame's own capture slots — construction-time
  snapshot semantics by construction. `CompiledFn.NParams` counts
  params + captures; the VM pops them uniformly. A capture
  unreachable at a call site, and a closure ESCAPING as a value
  (fn-value call, Stage 4), refuse. Required a provenance sweep:
  when a fn unit's recording closes, its events' producedBy entries
  are dropped (a join inside the body reusing a capture/param ID
  — JoinCarriers keeps the then-side ID — made enclosing call sites
  resolve the capture to a closed unit's event).
- **Unit identity bugfix** — the fn-unit/memo key now includes the
  construction site (`FnAnalysisKey`, first body token's position):
  redefining a same-name same-sig fn used to bind every call to the
  FIRST definition's unit (compiled `2 2` vs interpreted `2 3`;
  no spec row exercised it — now pinned).

**Witnessed: self AND mutual tail-recursion at depth 1,000,000
under a 172-entry ceiling in compiled mode**; equally deep non-tail
recursion (self and mutual) exhausts loudly; a tail SPIN fails with
evaluation_limit in both modes. The install-time synthetic example
evaluation no longer records phantom events (suspended).
Differential: 614 rows / 0 mismatches, floor 610. Deferred to Stage
4 (as planned there): generics, multi-overload selection beyond the
checker-matched sig, fn-value call sites (escaping closures).

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

**Status: COMPLETE (June 2026).** Everything that runs during the
check pass and emits (almost) nothing now compiles or cleanly falls
back. Differential: 1412 spec rows compiled, 0 mismatches; accuracy
ratchets unchanged or tightened (the macro change dropped 10 false
positives). What landed, against the plan's checklist:

- **`RunInCheckMode` words** (`def`/`fn`/`type`/`import`/`module`/
  `var`, and now `macro`) execute during the recording pass; their
  effects are visible to the compiled stream and they emit nothing
  themselves.
- **Macro and minilang expansion.** `macro` constructs in check mode
  (RunInCheckMode, like fn/fnsig), so a macro INSTALLS during the
  pass and its uses expand on the tape (`execMacro`) before the
  recorder sees them — the site lowers to its EXPANSION, never the
  raw-form operand span (R6 #29). Verified by golden
  (`TestEmitMacroExpansionGolden`). The minilang `mini` word is a
  deterministic native that lowers to a bare `CALL_NATIVE`
  (`TestEmitMinilangCompiles`).
- **Generics:** one `CompiledFn` per memoised instantiation — the
  fn-unit key already carries the instantiated arg types and the gen
  bindings are installed around the recorded body analysis, so
  lifting the Stage-3 refusal sufficed. Generic units stay OUT of
  tail marking (the interpreter's `HasGen` exclusion, mirrored).
- **Type operands** (`make`/`is`/`convert`/`of`/…): a new `PUSH_TYPE`
  opcode over a `Program.Types` table of canonical type IDs, resolved
  through the registry's `TypeTable` (then the package `Builtin`
  table) at RUN time — a type node never enters the constant pool,
  where a by-value copy goes stale against the canonical pointer.
  Structural type bodies (record/options/typed-container/disjunct)
  ride the const pool when their interior is carrier-free
  (`typeBodyConstOK`); class/surface bodies refuse (embedded method
  fn-values).
- **Multi-signature fns:** monomorphize per call SHAPE — the checker
  resolves which overload each statically-typed call site selects and
  each becomes its own unit. No runtime `CALL_USER_POLY` is needed
  when the checker can pick; the truly-polymorphic case (a dynamic
  arg reaching several overloads) is deferred to Stage 5's fallback,
  which is exactly where the plan's taxonomy puts it.
- **Module imports / dot-access calls:** the import and the `get`
  name-resolution run during the check pass; the `get` event is
  elided when its result is a statically-known callable/namespace
  (`FnDefInfo` / module-export `ExtensionPayload`), and the resolved
  wrapper's trivial-delegation dispatch records the REAL inner-native
  call — so `MathUtil.sqrt 16.0` lowers to a bare `CALL_NATIVE`
  (`TestEmitModuleCallLowering`).

**Deferred to Stage 5 (by the plan's own taxonomy, not a gap):**
function-value call sites the checker can't resolve (F4 — `m.f 5`
where `get` returns dynamic `Any`) and any dynamic-`get`-result
accessor (`(… mini …).n`). These are "any site the checker widened
to `Any`" → `FALLBACK_INTERP`. They refuse cleanly today and the
program runs correctly through the interpreter
(`TestEmitFnValueCallFallsBack`). A genuine `CALL_USER_POLY` /
checker get-typing-precision pass would compile more of them, but
that is a fallback-boundary concern, which Stage 5 owns.

- **Gate (met):** dual-mode differential over the whole spec corpus
  (1412 rows / 0 mismatches), the macro-expansion and module-call
  goldens, and the multi-overload / minilang / F4-fallback pins in
  `lang/go/bytecode_emit_test.go`.

## Stage 5 — dynamic fallback + concurrency

**Status: IN PROGRESS — span-level FALLBACK_INTERP landed for
self-contained code-body words.** The `OpFallback` opcode +
`Program.Fallbacks` table is the interpreter-island mechanism: a
construct the compiler can't lower re-runs through a sub-engine over a
recorded token span, threading the operand stack (NIn inputs popped
deepest-first and pre-loaded, the island's residual pushed back), with
the compiled code on either side intact. The VM handles NIn>0
threading; the emitter currently emits only fully-baked (NIn=0)
islands.

Words wired: the code-body higher-order data transforms
(`each`/`fold`/`scan`/`for-each`/`select`/`group`, plus
`filter`/`outer`/`inner` — the allow-set widened with the gate green).
A refused dispatch becomes an island iff — allow-listed, single-result,
fully forward-eligible, **core dispatch** (the matched sig is
pointer-identical to the word's main-registry binding, so a
module-qualified inner native through a sub-registry is rejected —
baking its bare name would re-run a different word), every BAKED arg
materialises **deeply concrete** (a data arg with any carrier element
refuses — a stripped def-bound list would bake `[ProperString …]`
instead of the values), and every code-body word is **VM-resolvable**
(a registered native/fn-def or known literal; a value-`def` reference
refuses, because that binding is a check-time carrier at run time). The
island's dynamic result flows to the residual or another fallback; a
downstream TYPED dispatch consuming it still refuses via
`anyDynamicCarrier`, so soundness holds.

**Threaded computed-receiver islands LANDED.** A data arg the check
pass can't materialise — a prior compiled event's result or a loop
local, e.g. `(iota 5) each […]` — is THREADED instead of baked: it must
be the trailing run of sig positions (so the baked args fill the
forward prefix and the one threaded value back-fills the deepest sig
position, positionally faithful by the split rule), capped at one
threaded value for now (multi-threaded layout is a follow-on). The VM
preloads its real runtime value onto the island and re-runs the span;
nested islands compose (`each […] (each […] …)`). Differential: 1438
rows / 0 mismatches, floor 1430. Soundness is gate-proven across the
corpus.

**Concurrency gates LANDED.** `Program` and all its tables are immutable
after compile; every mutable VM scope (operand stack, locals, frames,
loop state) is allocated per `RunProgram` call, and each goroutine runs
against its own `ForkConcurrent` registry. Two `-race` gates: a
synthetic one (`bytecode_concurrency_test.go`) drives one shared
`*Program` from 16 goroutines across the compiled surface (straight-line,
loop, tail-recursive fn, baked AND threaded islands), and the
plan-mandated one (`compiled_concurrent_test.go`) drives the CONCURRENT
SPEC ROWS (`await` parallel bodies, `timeout`/`interval`/`cancel`)
through `RunCompiled` under load — both data-race-free, results matching
the interpreter. `await`/timer branch bodies run as interpreter
fallbacks (v1), forking an isolated registry per branch.

**Whole-corpus gate MET, 0 divergences in values AND error taxonomy**
(`compiled_fullcorpus_test.go`): every row (2607) runs through
`RunCompiled` — compiling what it can, silently falling back otherwise —
and matches the interpreter on both the value and the error code (1494
compiled). Three compiled-mode soundness gaps were closed to get there:

- **Fallback registry-isolation.** `CompileCheck` executes the program
  in check mode, so its RunInCheckMode words (def/import/type/macro, the
  Test harness) leave real side effects; the COMPILED path needs those
  (OpPushType resolves minted IDs, islands re-run over the same
  registry), but the FALLBACK must not re-apply them or it double-mints /
  re-imports / re-runs a Test spec. `RunCompiled` snapshots the mutable
  scopes (`Registry.SnapshotForCompile`: Defs, Types, Contexts, Modules
  load set, builtin-word set, capability slots, check state) and rolls
  them back on the fallback path; the compiled path keeps them. An
  in-place snapshot was the right tool — a `ForkConcurrent` shares the
  parent's `Modules`/`Capabilities` pointers, so a fork's check pass
  still pollutes the real registry's module cache; restoring the SAME
  registry is faithful where a separate one is not.
- **Return type/count check at RET.** The interpreter enforces a fn's
  declared return via a ReturnCheck (`__RC`) token; the compiled fn
  skipped it. The VM now checks the body's result against
  `CompiledFn.Returns` with the same `v.Is(exp)` membership (predicate
  refines run their predicate, bare refines stay nominal, builtins
  unchanged), raising the byte-identical `type_error`. A body whose
  value COUNT differs from the declared returns refuses to compile (the
  single-result lowering would otherwise drop the extras) and falls back.
- **Check-mode-suppressed strict errors.** Some words are lenient in
  check mode but raise at runtime — an orphan `gen [...]`
  (gen_without_constructor), an `unpack` of a missing key (unpack_error).
  The compiled stream IS the check pass, so it would silently succeed.
  Such a word now sets `Check.SuppressedRuntimeError`; `CompileCheck`
  refuses to compile and the interpreter raises the real error on the
  fallback.

**Remaining (native-coverage follow-on; every such program already runs
correctly via fallback):** multi-threaded islands (the trailing run laid
out deepest-first on the operand stack); the general dynamic-dispatch
fallback for `get`-returns-`Any` and fn-value call sites (F4) via
`TYPE_CHECK`-guarded boundaries; `break`/`continue`/`return` sentinels
crossing an island; widening the allow-set into the query-DSL words
(`where`/`having`/`order`/`select`-query) once their pipeline-receiver
semantics are proven island-faithful. The original
Stage-5 scope:

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
