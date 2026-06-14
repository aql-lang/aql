# AQL Bytecode — Completing Runtime Independence (P5–P7)

Status: design. Companion to `design/aql-bytecode-plan.0.md` (which records
the landed P0–P4). This doc specifies the **remaining** work to reach the
goal: a compiled `Program` executes entirely in the VM, with the `OpFallback`
island and the whole-program fallback **deleted**.

## Where we are

P0–P4 landed (all gate-clean: differential + whole-corpus at 0 divergences,
race-clean, alloc ceilings held). The runtime-independence machinery exists:

- **Seam / re-entrancy.** `eng/go/invoke.go::InvokeBody` + `Registry.Invoker`;
  `vmContext.run(startUnit, locals, stack)` is the re-entrant VM loop
  (`eng/go/vm.go`). A code body runs as a compiled closure via
  `vmContext.invokeClosure`, never the interpreter.
- **Closures.** `OpPushClosure` / `ClosurePayload` / `CompiledFn.NCaptures`;
  `callableWords` + `tryRecordClosure` (a throwaway-`EmitState` **probe**
  compiles speculatively, so a refusal falls back to the island untouched).
  Closure capture via `ComputeCaptures`.
- **Runtime dispatch.** `OpCallNativePoly` / `PolyRef` / `tryRecordPoly` /
  `vmContext.callPoly` — the kernel's own `MatchSignature` selects the
  overload at run time. `OpCallDynamic` / `vmContext.callDynamic` /
  `tryNativeFnApply` — the fn-value-call boundary (closures VM-native,
  trivial-delegation methods VM-native, user-fn bodies islanded as a
  backstop, non-callables left untouched).
- **Ratchets.** `test/go/langspec/compiled_coverage_test.go`:
  `refusalCeiling` (rows that produce no `Program`) and `islandCeiling`
  (compiled programs still containing an `OpFallback`). Both are downward;
  P7 is gated on both reaching **0**.

Current ratchets: **598 refused / 26 islanded** (from 651 / 115 at P0;
616 / 29 before P5). Compiled rows 1706 → 1759, 0 divergences throughout.

## The recorder/lowerer model (shared context for the work below)

The emitter (`eng/go/emit.go`) records a linear trace of `emitEvent`s and
lowers it to bytecode. The two load-bearing data structures, and their
**single-value assumptions** that several remaining items must relax:

- `producedBy map[string]int` — value **ID → producing event seq**. One seq
  per ID. `resolveOperand(v)` consults it first (then frame locals, then a
  bare-type-node `OpPushType`, then `materialise`+`isInertConst` const).
- `lowerer.vm []int` — the simulated operand stack: **one entry per event
  result** (the producing seq). `lowerCall` matches an operand's `fromSeq`
  against `lw.vm` positions to emit pushes / a `SWAP`, asserting the result
  is where the stack discipline expects it.

Both assume **each event produces exactly one value, consumed exactly once**.
Multi-result calls and a value referenced more than once break that — see P5
and the stack-discipline item.

---

## P5 — Multi-result lowering (refusals ~44 + dup-body islands)

**Status — partially landed (616/29 → 598/26).** The `(seq, idx)` foundation
(steps 1–3 below) plus **0-result and N-result native calls** landed
gate-clean: `producedBy` is now `map[string]producer{seq,idx}`, the lowerer's
simulated stack is `[]vmSlot{seq,idx}`, an `emitOperand` carries a `resIdx`,
and `RecordCall` records side-effect (`set`/`raise`/`drop`/`printstr`/`sleep`)
and genuine multi-result words instead of refusing `len(outs) != 1`. The
empirical breakdown corrected the plan's framing: the "multi-result" refusal
bucket (34 rows) was dominated by **0-result** side-effect calls, not N-output
calls — those drove most of the −18 refusals.

**Deferred (still refused, soundly):**

- **Multi-RETURN / 0-return fns** (step 4 — the "multi-return fn" bucket, ~10
  rows): `StartFnCompile` still requires exactly one declared return; the fn
  unit's single `outOp` and `RecordUserCall`'s single `out` need generalising
  to N results, and `Finalize`'s fn-unit RET check to N (`OpRet` already loops
  over `Returns`). Mechanical follow-on on the same `(seq, idx)` base.
- **dup-body islands** (`each [dup add]`): `dup` returns `[args[0], args[0]]` —
  the SAME `Value.ID` twice (`spliceMatchResults` does not re-mint), so the two
  outputs collapse in the ID-keyed `producedBy` and the operand layout refuses
  the consume as "not adjacent." This is the **carrier-identity** item's
  territory (the next section): a multiply-emitted identical value needs
  distinct ids (or a value-def local / `DUP`), not the `(seq, idx)` machinery.

**Symptom.** `X returns N values (Stage 1 lowers single-result calls)`
(`emit.go::RecordCall`, `len(outs) != 1`), `fn … without exactly one declared
return` (`StartFnCompile`), and the `dup`-bodied higher-order islands
(`each [dup add]` — `dup` is 1-in-2-out, so its closure body refuses).

**Root cause.** The single-value model: `producedBy[id] = seq` cannot say
*which* of an event's N results an id is; `lw.vm` has one slot per event.

**Plan.**

1. **Track the result index.** Change the producer registration to
   `producedBy[id] = (seq, idx)` (a small struct, or a parallel
   `producedIdx map[string]int`). `setProduced` records every `outs[i]` with
   `idx=i`. `resolveOperand` returns the index in the `emitOperand`
   (`fromSeq` plus a new `fromIdx`).
2. **Multi-slot `lw.vm`.** Make `lw.vm` entries `(seq, idx)`. A multi-result
   event pushes N entries (`idx = 0..N-1`, deepest-first matching the
   handler's result order). `lowerCall`'s position checks compare `(seq,
   idx)` pairs.
3. **Record multi-result calls.** `RecordCall` stops refusing `len(outs) !=
   1`; it records the event and registers all outs. `OpCallNative` already
   pushes every handler result, so the VM side needs no change for natives.
4. **Multi-return fns.** `StartFnCompile` accepts `len(declared) != 1`;
   `CompiledFn.Returns` already a slice; the `OpRet` check loops over it
   (it already does); the fn-unit residual reconciliation
   (`Finalize`/`lowerFragment`) generalises from one result to N.
5. **Residual.** The program-residual reconciliation in `Finalize` already
   walks a `residual []Value`; extend it to consume N results of one event.

**Files.** `eng/go/emit.go` (the bulk — `emitOperand`, `setProduced`,
`resolveOperand`, `lowerCall`, `Finalize`, `lowerFragment`, `StartFnCompile`),
`eng/go/carrier.go` (`RecordCall` refusal). No VM change for native
multi-result; CALL_USER multi-return already returns a slice.

**Risk (the plan's top hotspot).** `lowerCall`'s stack-discipline logic is
deeply single-result; every `case 0/1/2/default` and the `SWAP` insertion
must be re-derived for `(seq, idx)` operands. Land behind the full gate;
revert if any divergence. **Recommended first** of the remaining items — it
is foundational (value-def locals and dup bodies build on it) and purely
mechanical (no fn-value/type subtlety).

---

## Carrier-identity for `make` + value-def locals (stack-discipline ~24)

**Symptom.** `stack discipline underflow at eq/cmp/deq/is` for
`def a (make Array [1 2]) a eq a`, `(make P {}) (make P {}) eq`,
`(make Foo 1) is Foo`.

**Root cause (two distinct problems, found this session).**

1. **`make` carrier dedup.** Two identical `make P {}` calls produce carriers
   the check pass treats as the **same value id**, so `producedBy` collapses
   both eq-operands onto the LAST make's seq — the first instance is lost.
   `make` is impure (each call is a fresh instance), so its result carrier
   must be **uniquely identified per call**, unlike a pure const.
   (`NewValueRaw` already mints a fresh `ID` at run time; the dedup is on the
   check-pass CARRIER, upstream — investigate the make `ReturnsFn` /
   carrierResults memoization.)
2. **Value-def locals.** A `def`-bound COMPUTED value (an event result, not a
   const) referenced **more than once** is on the VM stack only once.
   `def a (make…) a eq a` needs `a` computed once, stored, and re-pushed.

**Plan.**

1. **Unique make carriers.** Ensure the check-pass result carrier of `make`
   (and any impure constructor) gets a fresh id per call so two identical
   calls are distinct events with distinct `producedBy` seqs. This alone
   fixes `(make P {}) (make P {}) eq` (two genuine operands → normal `case 2`).
2. **`OpStoreLocal` + value-def locals.** A multiply-referenced event result
   (count its operand references during a lowering pre-pass) is assigned a VM
   local: emit `OpStoreLocal slot` right after its event, and
   `resolveOperand` returns that `localSlot` for every reference. Re-pushes
   become `PUSH_LOCAL`. (This is the plan's deferred "F4 value-def receivers"
   generalised to any computed def value, not just const-materialisable ones.)
3. **`OpDup` (optional fast path).** For the adjacent same-value-twice shape
   (`a eq a`), once identity is correct, a `DUP` before a binary op is sound
   and cheaper than a local. NOTE: a `DUP` is **unsound before step 1** —
   it was tried this session and diverged on `(make P {}) (make P {})`
   precisely because the two distinct makes shared a carrier id.

**Files.** `eng/go/carrier.go` / the make `ReturnsFn` (carrier identity),
`eng/go/emit.go` (`OpStoreLocal`, the reference-count pre-pass,
`resolveOperand`), `eng/go/bytecode.go` + `eng/go/vm.go` (`OpStoreLocal`,
optional `OpDup`).

**Risk.** Carrier identity is load-bearing for the checker (CSE, memoization,
typing); changing it could shift diagnostics. Do step 1 in isolation, measure
the differential + check-accuracy gates, before step 2. **Recommended after
P5** (the reference-count/locals machinery is cleaner once `lw.vm` already
carries `(seq, idx)`).

---

## Fn-values-on-the-stack (`apply` + higher-order fn args, refusals ~148)

**Symptom.** `unannotated or opaque word apply` for `5 inc/r apply`,
`z/r apply`, `f/r apply`; and any higher-order word handed a fn VALUE
(`each [idg] …`). The fn value can neither bake (it is code, not
`isInertConst`) nor resolve provenance.

**Root cause.** There is no way to put a runtime `FnDefInfo` value on the VM
stack. The closure machinery exists (`OpPushClosure`) but is only emitted for
code BODIES, not for fn VALUES referenced by name.

**Plan.**

1. **FnDefInfo operand → closure.** When `resolveOperand` (or a dedicated
   hook before `RecordCall`) sees an `FnDefInfo` value used as an operand,
   compile its body to a `CompiledFn` unit via the existing
   `StartFnCompile`+`AnalyseFnBody` path (probe-guarded, exactly like
   `tryRecordClosure`), and return a closure operand. A user fn's params are
   NAMED (bound to frame locals); the unit reads them via `PUSH_LOCAL` and is
   invoked like `CALL_USER`. Anonymous afn values compile the same way. A fn
   that does not compile leaves the operand unresolved → the program falls
   back, exactly as today.
2. **`apply` elision.** `apply`'s handler just unquotes the fn for the engine
   to re-step (`native_ref.go::applyHandler`). In compiled form the fn IS a
   closure on the stack, so `apply` is an identity — elide it at record time
   (like the get/getr module-resolution elision in `RecordCall`), leaving the
   closure for the application to consume.
3. **Stack-form application.** `apply`'s convention is STACK-form: `args… fn
   apply` (fn on TOP, args BELOW). The existing `OpCallDynamic` is residual-
   form (fn first, args after). Add a stack-form variant (or a flag on
   `OpCallDynamic`) that pops the fn from the top and applies it to the N
   values below — `callDynamic` already routes closures to `vc.run` VM-native
   and FnDefs to `tryNativeFnApply` / island.
4. **Value-vs-call.** Respect the interpreter's "0-arg anonymous value stays
   data" rule (eng CLAUDE.md "Sharp edge"): only elide+apply where the
   interpreter would apply; a fn value bound by `def` is captured at check
   time (compile-time) and never reaches runtime as an apply.

**Files.** `eng/go/emit.go` (FnDefInfo-operand → closure compile; reuse
`compileClosureBody`/probe), `eng/go/carrier.go` (`apply` elision + the
fn-value-operand hook), `eng/go/bytecode.go`+`vm.go` (stack-form CALL_DYNAMIC),
`lang/go/native/native_ref.go` (apply already returns the fn unchanged — no
handler change, the recorder elides it).

**Benefit beyond apply.** This is what lets `callDynamic` stop islanding
user-fn applies (it can compile the body to a unit and run VM-native), and it
de-islands higher-order words handed a named/anonymous fn value. **The biggest
single refusal unlock.**

**Risk.** The value-vs-call ambiguity; probe-compiling arbitrary fn bodies
(captures, recursion, generics — generics already compile per-instantiation);
the stack-form calling convention.

---

## Predicate-type operands (refusals ~157, "operand provenance")

**Symptom.** `operand of unknown provenance … at tcmp/teq/lt` for
`(Integer gt 10) tcmp (Integer gt 10)`, `(Integer gt 10) lt (Integer gt 20)`
— type-algebra and comparison over predicate (refinement) types.

**Root cause.** `(Integer gt 10)` is a `DepScalarInfo` value (a self-contained
subset type: base family + predicate, no registry per eng CLAUDE.md). At the
operand site it is a check-mode **carrier** with no recovered original, so
`resolveOperand` → `materialise` fails.

**Plan.**

1. **Const-bake concrete predicate types.** Add `DepScalarInfo` to
   `typeBodyConstOK` / `isInertConst` (`eng/go/emit.go`) so a CONCRETE
   predicate-type value bakes into the const pool (the payload is self-
   contained — base + predicate, no registry, no canonical-pointer hazard).
2. **Carrier provenance for predicate types.** Where the operand is a CARRIER
   (the common case), record the concrete predicate value in `origByID`
   (the `RecordStrip` mechanism) when `(Integer gt 10)` evaluates at check
   time, so `materialise` recovers it. Investigate whether the bounded-type /
   predicate construction path can carry its concrete value through the
   strip the way top-level literals do.

**Files.** `eng/go/emit.go` (`isInertConst`/`typeBodyConstOK`,
`materialise`/`origByID`), the predicate-type construction site
(`core_boundedtype.go` / `native_type.go`) for the strip hook.

**Risk.** Predicate types carry a `*Type` (the minted node) plus the
`DepScalarInfo` body; ensure baking by value stays canonical for matching
(`v.Is(t)`). Lower-leverage per line of effort than fn-values; sequence late.

---

## `case` clause compilation (islands ~16)

**Symptom.** `code-body word case (Stage 2)` — `case v [m1 b1 m2 b2 … default]`
islands (a structured match/block clause list, not a single body).

**Root cause.** `case` is a multi-way dispatcher (`native_control.go::
caseClauses`), not a one-body higher-order word, so it fits neither the
closure model nor a single CALL.

**Plan — desugar to a branch chain.** `case` is an n-way `if`: for each
`(match, block)` pair, test `v` against `match` and, on success, run `block`;
the trailing lone clause is the default. The branch-lowering machinery already
exists (`ArmBranchCapture` / `RecordBranch` / the `JMP_IF_FALSE`/`JMP`
lowering). Add a `RecordCase` that:

1. Resolves `v` to an operand (computed once → a value-def local per the
   stack-discipline item, since `v` is tested against every clause).
2. For each clause, records a guard `v is match` (a type match) or `v eq
   match` (a value match — `caseClauses` decides which) and the block as a
   captured fragment, chaining `JMP_IF_FALSE` to the next clause.
3. Records the default block (or `None`) as the final else.

It lowers exactly like a nested `if`, producing pure control flow + native
calls — no island.

**Files.** `eng/go/emit.go` (`RecordCase`/`lowerCase`, reusing branch
lowering), `eng/go/carrier.go` (route `case` to `RecordCase`),
`lang/go/native/native_control.go` (expose the match-kind per clause so the
recorder emits `is` vs `eq`).

**Risk.** Matching a clause `match` faithfully (type vs value vs predicate —
mirror `caseClauses` exactly); requires the value-def-local for `v`. Sequence
after the stack-discipline item.

---

## P7 — delete the fallback (the goal)

**Hard precondition:** both ratchets at **0** — a new
`TestEveryRowCompiles` (every `.tsv` value row yields a non-nil `Program`)
green, AND `islandCeiling == 0`. Until then deletion would turn refusals into
errors.

Note the island backstop survives in two non-obvious places that the items
above must each retire: the higher-order **island** (P5 dup + the
closure-probe refusals), the `callDynamic` user-fn **island** (fn-values-on-
the-stack lets it compile the body to a unit), and the whole-program
**fallback** (every row compiles).

**Deletions:**

- `vm.go` — `OpFallback` case + `runFallback` + `islandEng`; the `callDynamic`
  island branch (after fn-values-on-the-stack makes it VM-native).
- `bytecode.go` — `OpFallback`, `FallbackSpan`, `Program.Fallbacks`, disasm.
- `emit.go` — `RecordFallback`, `evFallback`, `lowerFallback`.
- `carrier.go` — `tryRecordFallback`, `islandPureWords`, `fallbackWords`,
  `bodyFreeForFallback`, and the `carrierResults` island call (closures + poly
  now cover their cases).
- `lang/go/aql.go` — `RunCompiled`'s `a.Run(src)` whole-program fallback
  becomes `Compile` + `RunProgram`, surfacing a compile error as an error.

Re-baseline perf/alloc afterwards (the fallback path's snapshot machinery may
simplify).

---

## Recommended sequencing

Each lands gate-clean (differential + full-corpus 0 divergences, race, alloc,
lint) and lowers a ratchet monotonically.

1. **Multi-result lowering (P5).** Foundational, mechanical; unlocks dup-body
   islands + 44 refusals; the `(seq, idx)` operand model the next items reuse.
2. **Carrier-identity for `make` + value-def locals.** Unblocks the stack-
   discipline bucket; makes `OpDup`/`OpStoreLocal` sound.
3. **Fn-values-on-the-stack.** Biggest refusal unlock (~148); de-islands the
   `callDynamic` user-fn apply.
4. **`case` clause compilation.** Reuses value-def locals + branch lowering.
5. **Predicate-type operands.** Lowest leverage/effort; sequence late.
6. **P7 deletion** — once both ratchets hit 0.

## Verification discipline (unchanged contract)

Run after EACH item (the existing `make verify-bytecode` sequence):

1. `make fmt && make vet && make lint && make test`.
2. `TestCompiledCoverage` — `refusalCeiling` and `islandCeiling` only ever
   move DOWN; `TestEveryRowCompiles` is the P7 gate.
3. `TestSpecCompiledDifferential` (raise `minCompiledRows`),
   `TestSpecCompiledOrFallback` — **0 divergences** (values + error taxonomy).
4. `-race` on both concurrency gates.
5. `bytecode_allocguard_test.go` — the monomorphic CALL_NATIVE hot path
   unchanged; no new hot-loop allocation.

**Gate-clean-or-defer per item:** if an item can't stay gate-clean, revert it
and re-document — the ratchets record what still refuses/islands. Commit each
landed item separately with its before/after ratchet numbers.
