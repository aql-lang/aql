# Checker ↔ Bytecode-Compiler Architecture Review and Plan

Status: **design / proposal** (`.0` — nothing here is implemented unless a
later commit says so). Date: 2026-06-18.

This note is a router + analysis for the next person (or session) working on the
AQL bytecode compiler. It captures (a) how the checker and the bytecode compiler
actually interact today, (b) the current state of the runtime-independence
ratchet, (c) concrete generalizations / simplifications / data-structure
improvements, and (d) the state-of-the-art algorithms we are reinventing ad-hoc
and could adopt instead. It is meant to be read alongside
`design/aql-bytecode-runtime-independence.0.md` (the P5–P7 work plan, now stale
on numbers — see "Current state" below) and the two module guides
(`eng/go/CLAUDE.md`, `lang/go/CLAUDE.md`).

---

## 0. TL;DR (the verdict)

The core design is **sound and modern — do not rewrite it.** AQL's "the compiler
*is* the carrier checker with a recording side-effect" is exactly the
**single-pass abstract-interpretation baseline-compiler** architecture that every
production WebAssembly engine converged on (Titzer's V8 / Wizard work). The
checker already runs an abstract interpreter over an operand stack; emission
piggybacks on the analysis it already does. That foundation is right.

What has accumulated on top of it is:

1. **Ad-hoc reinventions of well-studied algorithms** — the lowerer hand-codes
   the *stack-scheduling* problem (DAG → stack machine), and the provenance
   layer hand-rolls *value numbering*. There is mature literature for both
   (destination-driven code generation; Koopman stack scheduling; SSA/GVN).
2. **Scattered state and classification tables** that make the system harder to
   extend than it should be — ~7 name-keyed "how does this word compile?"
   tables, ~10 parallel side-maps on `EmitState`, and a strip→remember→
   materialise dance that exists only to undo information the checker threw away.

The recommendation is **incremental cleanup first** (low-risk, gate-clean), then
a **one bigger bet** (reframe the trace as a value graph and lower it with
destination-driven codegen) that is what would actually drive the remaining
refusals toward zero *and* delete the `layoutOperands` special-casing.

---

## 1. How the checker and compiler interact today

### The single channel: `CheckState.Emit`

The entire coupling is one field (`eng/go/registry.go`, `CheckState.Emit
*EmitState`). When it is `nil` you get a plain type-check; when it is set to a
`NewEmitState()`, the *same* dispatch machinery additionally records what it
does. The dispatch funnel is `carrierResults` (`eng/go/carrier.go`), which ends
in a chain:

```
tryFoldStaticIndex → tryFoldModuleConst → tryRecordClosure →
tryRecordPoly → tryRecordFallback → RecordCall
```

and then `EmitState.Finalize(residual)` (`eng/go/emit.go`) linearises the
recorded event trace into a `Program` (`eng/go/bytecode.go`). The host entry
point is `(*AQL).CompileCheck` (`lang/go/aql.go`).

### What the checker hands the compiler

- **The resolved signature** (monomorphic dispatch). The checker selects the
  overload via `matchSignature`; the compiler bakes that as `CALL_NATIVE`
  instead of re-dispatching.
- **Operand provenance**, carried on `Value.ID`. `resolveOperand` (`emit.go`)
  maps a value to (a) an earlier event's output via `producedBy map[string]
  producer`, (b) a frame local, (c) a canonical type node, or (d) an inert
  const. Unknown provenance ⇒ `MarkUncompilable`.
- **Inferred return carriers** — `Signature.Returns` / `ReturnsFn`. The
  `Dynamic` flag means "statically unknown" (opaque ⇒ refuse).
- **Whole-program fn analysis** — `FnSummaries` + `AnalyseFnBody` fixed points.
- **Out-of-band facts** — `SuppressedRuntimeError`, `RecordContextSet`/
  `LookupContextType`, and the `Compilable`/`Reason` latch.

### Key data structures (the "IR")

- The trace: `EmitState.frames [][]emitEvent`; an `emitEvent` is one of
  `evCall / evBranch / evLoop / evCallUser / evFallback` (+ `makeList` /
  `makeMap` flags on `emitCall`).
- The operand: `emitOperand{kind, idx, resIdx, closureUnit, closureCaps}` with
  kinds `opConst / opEvent / opLocal / opType / opClosure`.
- Provenance: `producedBy map[string]producer{seq,idx}` — value-ID → producing
  (event, result-index).
- The lowerer: `lowerer.vm []vmSlot{seq,idx}` — a *simulated operand stack* that
  re-derives stack positions, with `layoutOperands` arranging operands and
  inserting at most one `SWAP`.

**Read these first in a new session:** `eng/go/emit.go` (the recorder + lowerer
data structures, ~2400 lines), `eng/go/lower.go` (`layoutOperands`,
`lowerCall`, `planValueDefLocals`), `eng/go/carrier.go` (`carrierResults` and
the try-chain), `eng/go/callable_words.go` (closure-body compilation).

---

## 2. Current state (numbers, June 2026)

The ratchet is **far** ahead of the prose in
`design/aql-bytecode-runtime-independence.0.md` (which still says 459 / 15). The
live `test/go/langspec/compiled_coverage_test.go` run:

```
2769 rows — 2375 compiled (5 islanded), 312 check-errors, 82 refused
```

Refusal histogram at 82 (the buckets are normalised by `normaliseReason`):

| n  | bucket | nature |
|----|--------|--------|
| 20 | operand provenance | heterogeneous long tail (see below) |
| 12 | dynamic/opaque output | soundness-gated |
| 10 | quoted-operand word | meta words (usurp / ref family) |
| 8  | residual lowering (Stage 1 limit) | stack scheduling |
| 6  | code-body word (NoEvalArgs) | mostly aql:test property words (2-body) |
| 6  | if-branch lowering | variadic-statement-if, computed-else, usurp-if |
| 5  | dynamic input | soundness-gated |
| 5  | suppressed runtime error | *correct refusals* — must emit error path for P7 |
| 3  | multi-return fn | *correct refusals* — count-mismatch error rows |
| 2  | user fn call (Stage 3) | meta |
| 1  | dispatch recovery (best guess) | meta |
| 1  | fn-value-call boundary | meta |
| 1  | function value reaches word | one non-bakeable fn value |
| 1  | operand shape (Stage 1 limit) | stack scheduling |
| 1  | other: "branch leaves extra values" | branch lowering |

The "operand provenance" bucket (20) breaks down into: generic-`make` in fn
bodies (~3), fn-body provenance (capturing lambdas, list-of-param) (~3), dynamic
module outputs (minilang/parselang/rand) (~4), module-type get (~3), macros
(~1), and assorted (`def m {a:g}`, `x/r`, `x/u`) (~3).

**The two downward ratchets** (gates on P7 = deleting the interpreter fallback):
`refusalCeiling` (currently 82) and `islandCeiling` (currently 7, actual 5),
both in `compiled_coverage_test.go`. Both must reach 0 before the `OpFallback`
machinery can be deleted.

### What landed in the June-2026 review session (for continuity)

Seven gate-clean increments took the ceiling 115 → 82 (each a separate commit on
branch `claude/checker-bytecode-compiler-info-pqd2w0`, each verified against the
full gate — differential 0 mismatches, `-race`, alloc, full `make test`):

1. **N-event already-in-layout lowering** (`lower.go::layoutOperands`) — accept
   any N event operands already in sig order on top (was 0/1/2 only).
2. **Residual ordering via local promotion** (`emit.go::Finalize` +
   `planValueDefLocals`) — an out-of-order data residual force-promotes its
   events to frame locals.
3. **`OpMakeMap`** (`bytecode.go`/`vm.go`/`emit.go`/`lower.go`/`engine.go`) —
   computed `make` construction bodies (`make Outer {i:(make Inner …)}`) assemble
   the map at runtime from value operands; keys ride in `Program.MakeMaps`. The
   shared `autoEvalMap` const-fold is split schema-vs-data by `match.Name ==
   "make"` so class defaults still bake while make data values become events.
4. **Make-instance lists via `OpMakeList`** (`emit.go::RecordMakeList`) — dropped
   the `make`-element exclusion (an OpMakeList of make events rebuilds fresh
   instances per run, sound).
5. **0-output closure bodies** (`callable_words.go` + `emit.go::
   RecordClosureCall` + `lang/go/modules/test.go`) — generalised the closure
   path from exactly-1 to 0-or-1 outputs (`callableWord.bodyOut`); registered
   `test-test` so `Test.test "n" [body]` compiles, run via `InvokeBody`.
6. **aql:test describe bodies** — reused the 0-output infra for `test-describe`.
7. **`fnStoreWords`** (`emit.go`) — minilang/parselang `register` *store* a fn
   (never invoke on the VM tape), so the fn bakes as an inert const, exempt from
   the function-valued-operand refusal.

**One attempt was correctly reverted (a real dead end):** extending
`OpMakeList`/`OpMakeMap` into fn-body frames. It is **unsound** — a body word
that shadows a module def resolves to the param local in check mode but to the
module def in the interpreter (`def c1 1 … fn [[c1:Integer]…[[c1]]]` returns
`[9]` compiled vs `[1]` interpreted). The top-frame restriction on
`RecordMakeList`/`RecordMakeMap` is therefore **load-bearing** — do not retry
this without solving the scope-resolution divergence first.

---

## 3. Strengths (validated by the state of the art — do not change)

- **Compiler = abstract interpreter + emission.** This is the WASM
  baseline-compiler design (Titzer). The validator/checker infers operand-stack
  types in one pass and emits as a side-effect. Keep it.
- **Probe-then-commit** for speculative compiles (`tryReturnedClosure`,
  `recordClosureDispatch` run a throwaway `EmitState` first). Sound and reusable.
- **The refusal ratchet** as a regression gate. Excellent — keep it, but see
  §6 for refining it.
- **Soundness-first refusals.** Several refusals are *correct* (mutable-instance
  aliasing, scope-resolution divergence). The discipline of "refuse rather than
  diverge" is right; the differential gate (0 mismatches) is the backstop.

---

## 4. Findings: generalizations & simplifications

Each item: the problem, the evidence, the proposed change, the risk.

### 4.1 The lowerer reinvents stack scheduling

**Problem.** `layoutOperands` (`lower.go`) hand-codes the 0/1/2/N-operand cases,
inserts at most one `SWAP`, and *refuses* anything needing a 3-deep rotate
("operand shape beyond Stage 1", "stack discipline underflow", "call result
above a literal"). This is the classic **stack-scheduling** problem (scheduling
a DAG onto a stack machine, NP-complete in general).

**Evidence.** The `case 0/1/2/default` ladder in `layoutOperands`; the
`reorder`/`shapeBeyond`/`notAdjacent` refusal strings; the residual-ordering
patch (increment 2); the value-def-locals promotion (`OpStoreLocal`, an ad-hoc
"spill to a frame slot").

**Proposed change.** Two options (4.1a is the bigger bet of §5):

- **(4.1a) Destination-driven code generation (DDCG).** Thread a *data
  destination* (stack / frame-local / effect-none / specific slot) and a
  *control destination* **downward** during recording, instead of materialising
  every result onto the simulated stack and shuffling afterward. Your
  `OpStoreLocal` value-def-locals and the new 0-output ("effect/none")
  destination are already *special cases* of destinations; DDCG unifies them.
- **(4.1b) If keeping post-hoc reconciliation:** adopt Koopman's distance-based
  stack scheduling and add **DUP / TUCK / ROT** opcodes (the VM has only `SWAP`
  — that is *why* the 3-deep cases refuse). Limit direct access to the top 3
  stack slots (matches the VM's reach) and rank use/reuse pairs by distance.

**Risk.** 4.1a is a recording-model change (significant); 4.1b is additive
(new opcodes + a heuristic) and lower-risk.

### 4.2 `producedBy` is value numbering — make it first-class

**Problem.** `producedBy map[string]producer` (value-ID → producing event) is
**local value numbering** done by hand, and the carrier-identity hacks (fresh
IDs for impure `make`, the de-collision in `RecordCall`, dup-body distinct-IDs)
are manual value-number management.

**Proposed change.** Frame the trace as an **SSA / GVN value graph** rather than
a linear event list. Multi-result calls, dup bodies, and CSE then fall out
naturally, and lowering becomes a *scheduling* pass over a DAG (where DDCG /
stack-scheduling live) cleanly separated from "what the checker decided."

**Risk.** Large; this is part of the big bet (§5). But even the *framing* (a
`valueNumber` type instead of string IDs) is a useful intermediate step.

### 4.3 Kill the strip → remember → materialise dance

**Problem.** `RecordStrip` / `RememberOriginal` / `origByID` / `materialise`
exist **only** because the checker strips concrete values to carriers (losing
info) and the compiler reconstructs it via ID-preservation + a side table.

**Proposed change.** A carrier should **carry a back-pointer to its concrete
origin** (e.g. a `*Value` on the carrier payload) so the compiler reads it
directly. No ID-keyed recovery, no preservation fragility. This is the single
biggest *simplification* available and removes a class of "operand of unknown
provenance" refusals caused by lost originals.

**Risk.** Medium. Carrier construction is load-bearing for the checker (CSE,
memoization, typing); change `toCarrier` carefully and lean on the differential
gate.

### 4.4 Consolidate `EmitState`'s parallel side-maps

**Problem.** Four per-event-seq boolean maps — `zeroOutSeq`, `typeOut`,
`valueDefs`, `genericSeq` — are all "property of event N" and belong as **flags
on `emitEvent` / `emitCall`**. The string-ID-keyed hot maps (`producedBy`,
`origByID`, `constIdx`) want **integer value handles**, not minted string IDs
(you hash strings on every operand resolution).

**Proposed change.** Move the per-seq booleans onto the event struct; introduce
an integer value-handle table. Mechanical, low-risk, and it speeds up the hot
path.

**Risk.** Low.

### 4.5 Move word compile-classification onto the `Signature`

**Problem.** The checker/compiler consult **seven+ scattered name-keyed tables** —
`callableWords`, `fnStoreWords`, `fnIntrospectionWords`, `moduleConstFoldWords`,
`checkModeLiteralWords`, `fallbackWords`, `islandPureWords` — plus predicates
`dynOutNativeOK`, `quoteOperandInertOK`, `isModuleInnerSig`. They all answer one
question: *how does this word behave for compilation?* Adding a word's compile
behaviour means editing eng-side tables and **coupling eng to module word
names** (the June session added `test-test` and `fnStoreWords` entries this way).

**Proposed change.** Replace them with a declared **`Signature.CompileEffect`**
(an enum: `Pure / SideEffect / Invokes / Stores / Reads / CodeBody / …` plus a
couple of capability bits). The word *declares* its compile-relevant semantics
once; the eng compiler stops hard-coding module names. The `test-test` /
`fnStoreWords` edits would have been one-line module-side declarations. This is
the best **information-sharing** fix and de-couples eng from modules.

**Risk.** Medium (touches sig construction across natives + modules), but
purely additive if defaults preserve current behaviour.

### 4.6 Unify the `Record*` / `tryRecord*` surface

**Problem.** ~13 recorders (`RecordCall`, `RecordClosureCall`, `RecordMakeList`,
`RecordMakeMap`, `RecordUserCall`, `RecordBranch`, `RecordLoop`,
`RecordFallback`, …) share a repeated spine: resolve operands, run the
double-record guard (`producedBy[outs[0].ID]`), append an event, register
outputs, latch refusal.

**Proposed change.** Factor that spine into one core; each recorder supplies only
its event-shape. Once `CompileEffect` (4.5) exists, most of the `carrierResults`
try-chain becomes a single dispatch on the sig's declared effect.

**Risk.** Medium; do it *after* 4.5.

### 4.7 Remove dead / half-wired mechanisms

`SchemaArg` and the `materialiseSchema` counter (`CheckState`) are defined,
propagated (`registry.go:1054`), and **never read** (confirmed during the
`OpMakeMap` work — the schema-vs-data split is keyed on `match.Name` instead).
Either wire them or delete them. Low-risk cleanup.

---

## 5. State of the art we are missing

| Technique | Maps to | Payoff |
|---|---|---|
| **Destination-driven code generation** (Dybvig/Hieb/Butler) | `layoutOperands` shuffles, value-def-locals, residual-ordering refusals | Dissolves the largest refusal class in one model; generalises local/effect destinations |
| **Stack scheduling** (Koopman) | The SWAP-only lowerer, "3-deep rotate" refusals | Add DUP/TUCK/ROT + a distance heuristic; near-total elimination of shuffle refusals |
| **SSA / GVN** (Cocke) | `producedBy` + carrier-identity hacks | Principled multi-result / dup / CSE handling; DAG lowering |
| **Single-pass abstract-interp baseline compilers** (Titzer / WASM) | The whole `CheckState.Emit` model | *Validates the architecture*; their "validator emits control-transfer info for the interpreter in one pass" is a template for emitting **more** from one pass |
| **"Abstract compilation → abstract bytecodes"** | the const-fold path | Framing for *which* deterministic sub-evaluations to bake |
| **Emit a Trap/Raise opcode** | "suppressed runtime error" (5) + "multi-return count-mismatch" (3) | These are not uncompilable — they are *known to error*. Compile the error path instead of refusing. A small, principled addition that clears 8 "correct refusals" for P7 |

**Why DDCG is the headline.** A recursive code generator traditionally
materialises every intermediate value through one result location and then
shuffles. DDCG inverts this: the *caller* of a codegen step knows where it wants
the result, so it passes a destination down. Data destinations include
"accumulator/stack", "a specific frame slot", and "effect/none" (discard — the
0-output side-effect case the June session added by hand). Control destinations
thread where execution resumes, giving one-pass conditionals/short-circuits.
Caveat (Bernstein): stack bytecode loses the tree-lifetime structure DDCG
exploits, so this is a **recording-side** change (push destinations down as the
trace is built), not a lowering-side patch.

---

## 6. Recommended sequencing

**Low-risk, do first (pure simplification, gate-cleanly):**
1. §4.4 — consolidate `EmitState` side-maps onto `emitEvent`; integer value
   handles.
2. §4.7 — delete (or wire) `SchemaArg` / `materialiseSchema`.
3. §4.3 — carrier carries its concrete origin; delete the
   strip/remember/materialise side tables.

**Medium, high-payoff (the information-sharing win):**
4. §4.5 — `Signature.CompileEffect`. Then §4.6 — unify the recorder spine and
   collapse the `carrierResults` try-chain into a dispatch on `CompileEffect`.
5. **Emit a Trap/Raise opcode** (§5 last row) — clears the 8 "correct-refusal"
   rows (`suppressed runtime error`, count-mismatch) that otherwise block P7.

**The big bet (prototype on a branch, behind the differential gate):**
6. §4.2 + §4.1a — reframe the trace as a value graph and lower it with
   destination-driven codegen + a few stack ops (DUP/ROT/TUCK). This is what
   actually drives the remaining shuffle/provenance refusals toward zero *and*
   deletes the `layoutOperands` special-casing. Measure the ceiling delta before
   and after; revert if it cannot stay gate-clean.

**A meta-improvement to the ratchet itself.** The `refusalCeiling` conflates
"genuinely can't compile (soundness)" with "lowering is too weak (scheduling)"
with "missing opcode". Add a second axis to `normaliseReason` — bucket by *root
cause* (soundness / scheduling / opcode / correct-error) — so a future session
knows which investment actually moves the number. The "correct-error" bucket
(8 rows) is a good first proof: it should become *compiled error programs*, not
refusals.

---

## 7. Verification discipline (unchanged contract)

Every increment must stay gate-clean. From the repo root and `test/go/langspec`:

```bash
make fmt && make vet && make lint && make test          # full pre-commit
cd test/go/langspec
go test -run TestCompiledCoverage -v                    # ratchet (refusal + island ceilings)
go test -run TestSpecCompiledDifferential -v            # 0 mismatches REQUIRED
go test -run TestSpecCompiledOrFallback                 # error-taxonomy parity
go test -race -run 'TestCompiledConcurrent'             # race
cd ../../../lang/go && go test ./ -run TestCompiledAllocCeilings   # alloc ceilings
```

Lower `refusalCeiling` (and `islandCeiling`) monotonically; never raise.
Gate-clean-or-revert per increment; if an item cannot stay gate-clean, revert it
and document why (the fn-body-assembly dead end in §2 is the template).

A quick ad-hoc refusal-bucket dump (delete before commit) — a throwaway test in
`test/go/langspec` that iterates the `.tsv` rows, calls `a.CompileCheck`, and
logs `normaliseReason(reason)` filtered by a `BUCKET` env var — is the fastest
way to see what a change cleared. The June session used exactly this.

---

## 8. Key file / symbol index (for navigation)

| Concern | Location |
|---|---|
| The recording side-effect channel | `eng/go/registry.go` — `CheckState.Emit` |
| Dispatch funnel + try-chain | `eng/go/carrier.go` — `carrierResults` (~line 467) |
| Recorder + lowerer data structures | `eng/go/emit.go` — `EmitState`, `emitEvent`, `emitOperand`, `Record*`, `Finalize` |
| Stack-scheduling / operand layout | `eng/go/lower.go` — `layoutOperands`, `lowerCall`, `planValueDefLocals` |
| Closure-body compilation | `eng/go/callable_words.go` — `callableWords`, `compileClosureBody`, `tryRecordClosure` |
| Opcodes + Program | `eng/go/bytecode.go`; VM in `eng/go/vm.go` |
| Word compile-classification tables | `eng/go/emit.go` (`fnIntrospectionWords`, `fnStoreWords`), `eng/go/carrier.go` (`moduleConstFoldWords`, `checkModeLiteralWords`, `fallbackWords`, `islandPureWords`, `dynOutNativeOK`, …) |
| const-fold / schema-vs-data | `eng/go/engine.go` — `autoEvalList`, `autoEvalMap`, `constFoldContainerVal` |
| Host entry point | `lang/go/aql.go` — `(*AQL).CompileCheck` |
| The ratchet | `test/go/langspec/compiled_coverage_test.go` |
| Differential gate | `test/go/langspec/compiled_differential_test.go` |
| The (stale-on-numbers) P5–P7 plan | `design/aql-bytecode-runtime-independence.0.md` |

---

## 9. Sources (state of the art)

- Dybvig, Hieb, Butler — *Destination-Driven Code Generation*:
  https://bernsteinbear.com/assets/img/ddcg.pdf
- Bernstein — *A quick look at destination-driven code generation*:
  https://bernsteinbear.com/blog/ddcg/
- Koopman — *A Preliminary Exploration of Optimized Stack Code Generation*:
  https://users.ece.cmu.edu/~koopman/stack_compiler/stack_co.html
- Titzer — *Whose Baseline Compiler Is It Anyway?* (arXiv 2305.13241):
  https://arxiv.org/abs/2305.13241
- Titzer et al. — *A Fast In-Place Interpreter for WebAssembly* (single-pass
  abstract-interp validator): https://www.cs.tufts.edu/comp/150FP/archive/ben-titzer/wasm-interp.pdf
- *Value numbering*: https://en.wikipedia.org/wiki/Value_numbering
- HN discussion — *type checking is a special case of abstract interpretation*:
  https://news.ycombinator.com/item?id=34088644

---

## 10. How to start a new session from this doc

1. Re-run the ratchet (`§7`) to confirm the current `refusalCeiling` and the
   live histogram — the numbers in `§2` drift as work lands.
2. Pick from `§6` by appetite: a low-risk cleanup (4.4 / 4.7 / 4.3), the
   information-sharing refactor (4.5 → 4.6), the 8-row "compile the error path"
   win (Trap/Raise), or the big DDCG bet.
3. Keep each change a separate gate-clean commit; revert-and-document on any
   divergence. Update the ratchet ceiling (downward only) with a one-line
   rationale in `compiled_coverage_test.go`, as the existing entries do.
4. Do **not** retry fn-body container assembly without first solving the
   scope-resolution divergence (`§2`, the reverted dead end).
