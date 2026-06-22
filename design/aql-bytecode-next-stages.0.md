# AQL Bytecode — Next Stages (detailed work to refusalCeiling 0 + P7)

Status: design / work-breakdown. Written after the session that drove
`refusalCeiling` 20 → 16 and landed the variadic-stack VM mechanism. This is the
execution spec for the remaining 16 refusals and the final P7 deletion. Companion
to `aql-bytecode-cluster5-residual-lowering.0.md` (Gap A/B detail),
`aql-bytecode-finish-line.0.md` (roadmap), and `module-fn-checkstate-ownership.{0..6}.md`
(the module-body-compilation project).

Read those first for context; this doc is the *forward* plan, organised by the
**foundation** each remaining row needs rather than row-by-row, because the rows
cluster onto a small number of foundational changes.

---

## 0. Current state (measured, tree clean, all gates green)

| ratchet | value | target |
| --- | ---: | --- |
| `refusalCeiling` (compiled_coverage_test.go) | **16** | 0 |
| `islandCeiling` (compiled_coverage_test.go) | **0** ✓ | 0 (done) |
| `reducibleCeiling` (compiled_metafallback_test.go) | **2** | 0 |
| `interpreterOnlyCeiling` | 0 / cap 3 | capped (permanent) |
| `computeRefusalCeiling` | 18 / 86 | 0 |

Corpus: 2830 value rows; 2502 compile (0 islanded), 16 refuse.

### Infrastructure already in place (reuse these)

- **Variadic stack region** — `OpStackMark` / `OpDropToMark` / `OpPopMark`
  (bytecode.go), a per-run `marks []int` in the VM (`vm.go::run`), the
  `planVariadicClaims` pre-pass + `lw.markBefore`/`lw.variadicElse` lowering
  (lower.go). Built for the chained variadic-if; the primitive (truncate the stack
  to a saved depth) is the building block for **all** runtime-variable-count work.
- **Terminal-trap infrastructure** — `EmitState.RecordTrap` (top-frame only) +
  `MarkUncompilable` is a no-op once `trapAt` is set (emit.go), so an operation
  that *is* the trap no longer self-refuses. Reuse for any remaining error-row.
- **Container const-fold** — `constFoldContainerVal` + the `autoEvalMap` bare-value
  fold (engine.go), gated by `exprRefsCarrier` + `containsSharedMutable`.
- **Residual reconciliation** — `forceOrder` + `planValueDefLocals` +
  `seatResults` (emit.go/lower.go); promotes out-of-order producers to frame
  locals.
- **fn-value trailing apply** — `resolveDynamicApply` + `OpCallDynamic` /
  `OpCallDynamicTrailing` (emit.go:2710 / vm.go:329) for a residual fn value
  applied to its neighbours.

---

## 1. The 16 remaining rows by foundation

| # | foundation (stage) | rows | risk |
| --- | --- | --- | --- |
| A | variadic branch/return modeling | `recursion.tsv:53` | high |
| B | conditional dynamic apply in a branch | `forward-barrier.tsv:83` | high |
| C | sound module-body compilation (cross-registry EmitState) | `module-test.tsv:38`, `module-parselang.tsv:23`, `module-rand.tsv:38` | very high (corpus re-baseline) |
| D | dynamic dispatch to module/sub-registry words | `reach.tsv:38`, `module-io.tsv:29`, `module-io.tsv:30` | high |
| E | VM reference cells (by-reference containers) | `flex.tsv:138` | high |
| F | dynamic-scope frames | `recursion.tsv:72` | high (soundness) |
| G | closure-return / fn-value-call boundary | `bytecode-combinations.tsv:74`, `def-node-binding.tsv:54`, `fn-value.tsv:19`, `patrun.tsv:40` | high (soundness) |
| H | dispatch-recovery operand order | `bytecode-combinations.tsv:113` | medium |
| I | divergent macro (permanent fallback / spec decision) | `macro.tsv:45` | n/a |

Stages C–G also clear the 2 reducible rows (Test/Assert, quote-macro) and chip at
`computeRefusalCeiling`.

---

## Stage A — variadic branch/return modeling (Gap A remainder)

> **Superseded VM analysis — see
> [`aql-bytecode-stageA-branch-result-modeling.0.md`](aql-bytecode-stageA-branch-result-modeling.0.md).**
> The deep-dive validated this stage against the live tree (June 2026) and found
> the VM side is **already done**: `OpRet` never truncates the stack
> (`vm.go:812-849`) and `checkReturnContract` no-ops when `len(Returns)==0`
> (`vm.go:1030-1032`), so a `[]`-declared variadic return needs **no opcode
> change** — points 2 below ("OpRet leaves a fixed count") and the "OpRet must
> leave all values" design bullet are stale. The load-bearing change is the
> recorder's **single-operand arm model** (`emitBranch.thenOut/elsOut`,
> `resolveArm` at `emit.go:922` takes only `stk[len-1]`), not the VM. Read the
> deep-dive for the corrected, staged plan; the rest of this section is the
> original framing.

**Row.** `def m fn [[n:Integer] [] [if (n lte 0) [] [n mul 2 m (n sub 1)]]] m 3`
→ `6 4 2`. Reason: `fn m: branch leaves extra values (Stage 2 lowers single-result
branches)` (`lower.go:864`, `lowerFragment`).

**Root cause.** Three coupled gaps:
1. **Multi-value branch arm.** The else body `[n mul 2 m (n sub 1)]` leaves
   `n*2` (1 value) **plus** the recursive `m (n sub 1)` result. `lowerFragment`
   (lower.go:840) requires an arm to net exactly `[out]` or 0 — it rejects >1.
2. **Variadic fn return.** `m` declares `[]` returns but produces N values at
   run time (N grows per frame). `reconcileResults` (lower.go:760) rejects a
   variadic fn-body residual (`rejectVariadic=true`), and `OpRet` leaves a fixed
   count.
3. **Variadic `CALL_USER`.** The recursive `m (n sub 1)` returns a runtime-
   variable count; `evCallUser` carries a fixed `nout` (`emit.go:1411`,
   `RecordUserCall`), and `AnalyseFnBody`'s residual is a fixed stack.

**Design.**
- Model a branch/fn result as a **count range** `[lo,hi]` (extend `lw.variadic`
  from a bool to carry a range, or add `lw.variadicN map[int]struct{lo,hi int}`).
  The then=0/else=(1+variadic) arm merge yields a variadic the residual absorbs —
  `lowerArms` (lower.go:1116) already marks a merge variadic on count mismatch;
  extend it to allow an arm that is *itself* multi/variadic.
- `lowerFragment`: when `allowVariadic` and the arm's residual is `[fixed…,
  variadic-tail]`, lower all of it and propagate a variadic merge slot instead of
  refusing.
- Fn return: let a `[]`-declared fn return its full residual. `reconcileResults`
  accepts a variadic tail; `OpRet` must leave **all** values above the frame base
  (it already pops the frame — change the result-count handling to "everything
  above `stackBase`" for a variadic-return unit). `checkReturnContract`
  (vm.go:943) must skip the count check for a variadic-return unit.
- Variadic `CALL_USER`: `AnalyseFnBody` returns a residual whose tail is marked
  variadic; `RecordUserCall` records `nout = -1` (sentinel: "all results");
  `lowerUserCall` (lower.go) pushes a variadic sim slot. The VM's `OpCallUser`
  already appends whatever the callee left, so no opcode change — only the
  sim-side count model.

**Hardest part.** The recursion must converge: `AnalyseFnBody`'s memo
(`FnSummaries`) keys on arg types; `m`'s summary is "variadic", and the recursive
call inside reads that same summary — a fixed point reached in one pass because
the variadic marker is count-agnostic. Verify the memo does not loop.

**Soundness hazard.** A variadic fn result must only ever flow to the **program
residual** or another variadic-absorbing position — never to a fixed-arity call
operand (the count is unknown at the call site). Gate: refuse a variadic
`CALL_USER` result consumed as a sig-position operand.

**Files.** `eng/go/lower.go` (`lowerFragment`, `lowerArms`, `reconcileResults`,
`lowerUserCall`, the `variadic` model), `eng/go/emit.go` (`RecordUserCall` nout
sentinel, `AnalyseFnBody` residual marking), `eng/go/vm.go` (`OpRet` /
`checkReturnContract` variadic-return), `eng/go/carrier.go` (`AnalyseFnBody`).

**Verification.** `recursion.tsv` rows; a fn returning a *fixed* multi-count must
still count-check (no regression). New `bytecode_findings_test.go` row:
`m 3 → [6 4 2]`, plus a negative (a variadic result fed to `add` must refuse).
Full `verify-bytecode`; lower `refusalCeiling` 16 → 15.

---

## Stage B — conditional dynamic apply in a branch

**Row.** `"aql:math-util" import end def n 5 if (n eq 0) [99] MathUtil.sqrt 16`
→ `4.0`. Reason: `if: else value of unknown provenance` (`emit.go:1004`).

**Root cause.** The 3-arg if's **else** is the module-export fn value
`MathUtil.sqrt` (a `get sqrt MathUtil` result), and the trailing `16` auto-applies
to the if **result** — but *only* on the false branch (where the result is the
fn). On the true branch the result is `99` and `16` is a separate residual value.
So the auto-apply is **conditional on the runtime branch**:
- `n=5` (false) → else → `sqrt` → apply `16` → `4.0`.
- `n=0` (true) → then → `99`; `16` stays → `[99, 16]`.

Two sub-problems: (1) resolve a module-export fn value as a branch-else operand
(today `resolveOperand` declines it → "unknown provenance"); (2) lower a trailing
apply that fires only when the branch produced a callable.

**Design.**
- Resolve `MathUtil.sqrt` (an immutable module-export fn) as a baked **const fn
  value** operand — the "no-capture fn value bakes as a const" path already
  exists; extend it to a module-export `get` of a fn field (the get folds to the
  concrete FnDef, which bakes).
- The if result is then a value that is *sometimes* a fn. The trailing `16` must
  lower to a **runtime-conditional apply**: push 16, then `OpCallDynamic`/
  `OpCallDynamicTrailing` over the if result — `callDynamic` (vm.go:329) **already**
  checks callability at run time and leaves the value + arg as data when not
  callable. So the existing dynamic-apply opcode handles "apply if callable, else
  leave both" — exactly the `[99,16]` vs `[4.0]` contract.
- The work is connecting the branch result (variadic-or-value) to the trailing
  `resolveDynamicApply` path, which today only triggers for a residual whose
  *lead/tail* is a fn value, not for a branch result that *may* be a fn.

**Soundness hazard.** Verify `callDynamic`'s non-callable fall-through produces
exactly `[result, 16]` in the same order the interpreter leaves them (the prior
`(3 and "x") add 1` revert was an operand-order divergence — re-test order).

**Files.** `eng/go/carrier.go` (`resolveOperand` for a module-export fn get →
const fn), `eng/go/emit.go` (`resolveDynamicApply` extension to a branch result),
`eng/go/vm.go` (`callDynamic` — already handles the fall-through; verify order).

**Verification.** `forward-barrier.tsv:83` (n=5 → 4.0) AND a both-polarity battery
(n=0 → [99 16]); 0 divergences. `refusalCeiling` -1.

---

## Stage C — sound module-body compilation (cross-registry EmitState) — LARGEST

**Rows.** `module-test.tsv:38` (Test harness), `module-parselang.tsv:23`
(sublanguage parse), `module-rand.tsv:38` (seeded generator) — plus the 2
**reducible** rows (Test/Assert, quote-macro) and several `computeRefusalCeiling`
rows. This stage has the widest payoff and the widest blast radius.

**Root cause (verified this session).** A module-preamble AQL fn (`run-spec`,
`run-cases`, `parse_calc`, generator bodies) dispatched as a value runs its body
via `CallAQL` (`registry.go:1103`) → `sub.Run`, which records into whatever frame
is active. Attempting to unit-compile it via `buildFnBodyReturnsFn` returned a
bare `[Any]` because **the module sub-registry has a separate, inactive
EmitState**: `e.registry.Check.Emit != capturedReg.Check.Emit` and
`capturedReg.Check.Emit.Active()` is false (confirmed with a debug probe). So
`StartFnCompile` declines and no unit records — the body's internal calls (e.g.
the test subject `double`) leak onto the program residual.

This is the **shelved §5b/§6** "sound module-body compilation" project
(`module-fn-checkstate-ownership.6.md`). §6 documents why it is not a localized
fix: the dynamic-help example eval's diagnostics are load-bearing for both
construction-time checking AND the compilation corpus, so the change requires a
**corpus re-baseline** plus investigating a masked "did not throw" soundness gap.

**Design (the project, in order).**
1. **Cross-registry EmitState sharing.** When the compiling program dispatches a
   module-preamble AQL fn, the fn body must unit-compile **against the main
   program's EmitState** while resolving names in its **own sub-registry scope**.
   Concretely: at the `execFnDefSig` capturedReg branch (engine.go ~4411), when
   `e.registry.Check.IsActive()`, temporarily install the main EmitState +
   check-mode onto `capturedReg.Check` (Emit is a `*EmitState`, registry.go:291)
   for the duration of `StartFnCompile` + `AnalyseFnBody` + `RecordUserCall`, then
   restore. Watch the coupled state: `FnSummaries` memo (must be scoped by
   `AnalysisScopeID`/`regID` so a module fn's summary doesn't collide — already
   prefixed, registry.go:134), `Diagnostics` (must not double-collect), and
   `suspended`/fragment depth.
2. **Hermetic dynamic-help eval** (§6 step 1). Make `GenerateDynamicExamples`
   (native_help.go) run in a fully isolated check state (snapshot+restore
   diagnostics, not just `Suspend()`), so synthetic-example diagnostics never gate
   compilation or pollute the corpus.
3. **First-class construction-time body check** (§6 step 2). Replace the help
   eval's accidental side-channel with a real post-binding, carrier-arg body check
   (the §6a-A principle: an abstract `Map`/`List` param reads `dynamic(Any)`), so
   `TestCheckUncalledFnBodyTypoStillFlagged` / `TestForwardStrandAdvisory` stay
   sound.
4. **Corpus re-baseline** (§6 step 3). With synthetic errors gone, re-baseline the
   `test/go/langspec` ceilings and per-row tiers, and investigate the masked
   `Assert.throws "did not throw"` row (a likely latent compile-soundness gap the
   eval was hiding).

**Soundness hazard.** This stage can change *what compiles and how it runs*
(§6 found a behavioral change behind partial suppression). Do steps 1–4 as ONE
reviewed change, never a partial diagnostic filter.

**Files.** `eng/go/engine.go` (`execFnDefSig`), `eng/go/registry.go`
(`CallAQL`, `CheckState` sharing helper — add a `BorrowCheck(parent)`/restore that
shares `Emit`+`Mode` and snapshots `Diagnostics`), `lang/go/native/native_help.go`
(hermetic eval), `eng/go/carrier.go` (construction-time body check), and
`test/go/langspec/*` (re-baseline).

**Verification.** `module-test.tsv:38`, `module-parselang.tsv:23`,
`module-rand.tsv:38` compile with parity; `reducibleCeiling` 2 → 0; the two
construction-check tests stay green; the corpus re-baseline is a deliberate,
reviewed ceiling change with the "did not throw" row explained. This is the one
stage that is a *project*, not a commit.

---

## Stage D — dynamic dispatch to module / sub-registry words

**Rows.** `reach.tsv:38` (`StructUtil.getpath $.a.b (StructUtil.setpath …)`),
`module-io.tsv:29/30` (`IO.write`/`read` over a dynamic context receiver). Reason:
`dynamic input at getpath` / `at set`.

**Root cause.** `tryRecordPoly` (carrier.go:847) re-matches a word's signatures at
run time via `OpCallNativePoly`, but it requires a **main-registry builtin**
(`r.IsBuiltinWord(word)`, carrier.go:860) and the matched sig to be the word's
main-registry binding (the CORE-dispatch guard, carrier.go:898–915). `getpath`,
`set`, etc. live in **module sub-registries**, so poly declines and the row
refuses — the VM's `callPoly` (vm.go:257) re-matches over `r.Lookup(word)` (main
registry), which would run the wrong word for a module-qualified name.

**Design.** Teach `OpCallNativePoly` (and `PolyRef`) to carry the **owning
sub-registry** so the VM re-matches over the correct registry. `PolyRef` gains a
registry/module handle; `callPoly` resolves the word in that registry; the
recorder admits a module word when its sub-registry sigs are safe (no meta /
fn-value / code-body / side-effect-on-dynamic-shape). This is the cluster-4
"poly-extension to non-core safe builtins" generalised to sub-registries.

**Soundness hazard.** A module word's overloads must be **value-faithful** under
runtime re-match (same first-match the interpreter takes). Exclude any word whose
dispatch mutates registry/context state in a shape-dependent way until proven.

**Files.** `eng/go/bytecode.go` (`PolyRef` + a registry handle in the Program
pools), `eng/go/vm.go` (`callPoly` registry-aware re-match), `eng/go/carrier.go`
(`tryRecordPoly` sub-registry admission).

**Verification.** `reach.tsv:38`, `module-io.tsv:29/30` compile with parity; a
negative (a genuinely runtime-shape-dependent module word) still refuses.

---

## Stage E — VM reference cells (by-reference containers)

**Row.** `flex.tsv:138`: `def l [1 2] push 3 l drop l` → `[1 2]`. Reason:
`dynamic input at drop`. Also the **reducible** `flex` entry.

**Root cause.** `flex` is a reference-semantics container; mutations (`push`,
`drop`) must be visible through aliases. The VM value model is **by-value**, so a
mutation through one binding is not seen through another — the compiled program
would diverge. The carrier refuses rather than bake an unsound by-value copy.

**Design.** Add a **reference cell** to the VM value model — a heap-boxed
`*RefCell{ Value }` payload (eng/go/payload.go marker) that `flex` instances carry,
so `push`/`drop`/index through any alias mutate the shared cell. This mirrors the
interpreter's pointer-backed `Map`/`Store`/`Array` (which already share state
across capture, per lang CLAUDE.md "Capture semantics"). The compiled `push`/`drop`
operate on the cell, not a copy.

**Soundness hazard.** Reference cells interact with const-baking
(`isInertConst` must NOT bake a mutable ref cell as a shared const — same
`containsSharedMutable` screen used by the map-field fold) and with the args-
aliasing invariant (the `-tags aqldebug` lane). Couple with the tier-2 `flex`
entry.

**Files.** `eng/go/payload.go` (RefCell payload), `eng/go/value.go` (flex value
construction), `eng/go/vm.go` (push/drop on a cell), `eng/go/emit.go`
(`isInertConst` excludes ref cells), `lang/go/native` flex handlers.

**Verification.** `flex.tsv` rows incl. alias-mutation; `reducibleCeiling` -1 (the
flex entry); the `-race` and `aqldebug` lanes stay green.

---

## Stage F — dynamic-scope frames

**Row.** `def g fn [[] [Integer] [n]] def f2 fn [[n:Integer] [Integer] [g]] f2 42`
→ `42`. Reason: `fn g: body result of unknown provenance`. `g` reads `n`, which is
**not** its param or capture — it resolves dynamically to the *caller* `f2`'s `n`
(dynamic scoping).

**Root cause.** The compiler models lexical scope (params + lexical captures,
fn_capture.go). `g`'s `n` is neither — it is the dynamic call-stack's nearest `n`.
Compiling it requires modeling the **dynamic def-stack** at run time, which the
static unit model deliberately does not (a unit is reused across call sites with
different dynamic environments).

**Design (per-case soundness — this is the riskiest semantically).** Two options:
1. **Compile a dynamic read** — `g`'s body emits a runtime def-stack lookup for
   `n` (a new `OpDynLookup name` reading the VM's def-stack), faithful to the
   interpreter. Bounded but adds a dynamic-scope opcode and a VM def-stack mirror.
2. **Classify as permanent fallback** — dynamic-scope reads are rare and arguably
   should not be in the compilable subset; document `g`-style dynamic reads as a
   permanent tier (like tier-1 Vm.run) and exclude from `refusalCeiling`.

Recommend (2) unless dynamic scope is a required compiled feature — it is a
semantic corner (the plan calls it "correctly refused").

**Files (option 1).** `eng/go/bytecode.go` (`OpDynLookup`), `eng/go/vm.go`
(def-stack mirror), `eng/go/carrier.go` (emit a dynamic read for an unresolved
body word that the interpreter resolves dynamically).

**Verification.** `recursion.tsv:72` parity; a typo (genuinely undefined) still
errors `undefined_word`, not a silent dynamic read.

---

## Stage G — closure-return / fn-value-call boundary

**Rows.** `bytecode-combinations.tsv:74` (`def mk2 fn […[Function][([y]=>[x add
y])]] for 3 [(mk2 i) 10]`), `def-node-binding.tsv:54` (`[[c1]]` fn-body list
literal — VERIFIED HAZARD, reverted), `fn-value.tsv:19` (`m.f` a map-stored fn
applied), `patrun.tsv:40` (a dispatch-table fn reaches `add`).

**Root cause.** Each is a fn VALUE whose identity/provenance the residual model
can't track:
- `mk2` returns a **closure capturing a param** (`x`); the returned-closure unit
  exists (`tryReturnedClosure`, emit.go:1296) but the *factory return through a
  fn body residual* refuses ("body result of unknown provenance").
- `def-node-binding:54` `[[c1]]` evaluates the inner list in a scope where the
  param is bound differently than a naive `OpMakeList` over locals — verified to
  diverge (`[[9]]` vs `[[1]]`), reverted. Needs the interpreter's per-call list
  assembly scope.
- `fn-value:19` / `patrun:40` — a fn value retrieved at run time (map get /
  patrun find) applied to args: `dynamic value precedes residual args` /
  `function value reaches add`. The fn value's arity/identity is dynamic.

**Design.** Build on `tryReturnedClosure` + `resolveDynamicApply`:
- Closure return: let a fn-body residual that is a returned closure value flow as
  an `OpPushClosure` operand (the unit exists); the blocker is the residual
  reconciliation declining it — admit a closure operand in `StartFnCompile.finish`
  (emit.go:1296 already tries it; extend to the factory-in-a-loop shape).
- fn-value application: route `m.f`/`find …` results through `OpCallDynamic` (the
  trailing-apply path) — the value is dynamic but the apply opcode handles it; the
  blocker is the "dynamic value precedes residual args" gate refusing a *leading*
  dynamic fn with following args. Extend `resolveDynamicApply` to the leading-fn-
  then-args shape (it currently favours trailing).
- `def-node-binding:54`: requires fn-body container assembly matching the
  interpreter's binding scope — defer; it is the documented hazard.

**Soundness hazard.** Operand ORDER on dynamic apply (the `(3 and "x") add 1`
revert); the `[[c1]]` scope divergence. Every change here re-tests the differential
on the exact reverted shapes.

**Files.** `eng/go/emit.go` (`tryReturnedClosure`, `resolveDynamicApply`,
`StartFnCompile.finish`), `eng/go/carrier.go`, `eng/go/vm.go` (`callDynamic`).

---

## Stage H — dispatch-recovery operand order

**Row.** `bytecode-combinations.tsv:113`: `(3 and "x") add 1` → `'x1'`. Reason:
`unmatched dispatch recovered at add`. `3 and "x"` doesn't match a numeric `and`;
the interpreter *recovers* by treating the mismatch as a value, then `add` does
string concat → `'x1'`.

**Root cause.** The recovery path (`engine.go` ~6199) produces a result the poly
lowering mis-orders: a prior attempt poly-lowered to `[1x]` vs the interpreter's
`[x1]` (operand order) — verified & reverted.

**Design.** Record the recovered dispatch with the **interpreter's operand order**
preserved. The recovery yields a concrete value; lower it as a normal
`CALL_NATIVE` (not poly) with the operands in the order the recovered handler
consumed them. The fix is purely operand-order bookkeeping at the recovery seam.

**Files.** `eng/go/engine.go` (recovery seam), `eng/go/emit.go` (record the
recovered call with correct operand order).

**Verification.** `bytecode-combinations.tsv:113` → `'x1'` (exact order); the
prior `[1x]` divergence is the regression guard.

---

## Stage I — divergent macro (`macro.tsv:45`)

**Row.** `def loopy (macro [[a] [quote [loopy unquote a]]]) macroexpand (loopy 1)`
→ `ERROR:expansion too deep`. **Not statically compilable.** At check time `loopy`
resolves to a carrier, so `expandAllMacros` (macro_expand.go:410, bound 256) never
re-expands to the depth limit it raises at *run* time; the compiler cannot know
the expansion diverges without running it (and running it to the limit at compile
time is itself the divergence).

**Decision (spec/scope — yours).** Either:
- **Permanent fallback tier** — classify a recursive/divergent `macroexpand` as
  interpreter-only (like tier-1 `Vm.run`), excluded from `refusalCeiling`. This is
  the honest classification: it is genuinely runtime-only.
- **Spec reclassification** — mark the row as a documented non-compilable error
  row.

No bytecode change makes this compile soundly; it needs a tiering decision so it
stops counting against the ceiling.

---

## Stage J — re-scoped P7 deletion (the endpoint)

Gated on `refusalCeiling == 0` (or every residual refusal placed in a documented
permanent-fallback tier per Stages F/I) and `reducibleCeiling == 0`.

1. `lang/go/aql.go::RunCompiled` — replace the `a.Run(src)` whole-program fallback
   with `Compile` + `RunProgram`; a refusal becomes a surfaced compile error.
2. Keep the `OpFallback` island (`vm.go::runFallback`, `bytecode.go::OpFallback`,
   `emit.go::RecordFallback`) but add a gate asserting **every** `OpFallback` span
   classifies tier-1 (`interpreterOnlyWords`). With `islandCeiling == 0` today,
   the only tier-1 fallback is a `Vm.run` of genuinely runtime-constructed source.
3. Re-baseline perf/alloc (`bytecode_*_bench_test.go`,
   `bytecode_allocguard_test.go`).

---

## Recommended sequencing (leverage ÷ risk)

1. **H** (dispatch-recovery order) — smallest, well-understood, 1 row.
2. **B** (conditional dynamic apply) — reuses `callDynamic`; 1 row.
3. **A** (variadic branch/return) — reuses the variadic-stack primitive; 1 row +
   unblocks future variadic shapes.
4. **D** (sub-registry poly) — 3 rows; medium.
5. **G** (closure/fn-value) — 3–4 rows; soundness-careful.
6. **E** (reference cells) — 1 row + reducible; VM value-model.
7. **F** / **I** — tiering decisions (dynamic scope, divergent macro).
8. **C** (module-body compilation) — the project; do as ONE reviewed change with
   the corpus re-baseline. Highest payoff (3 rows + 2 reducible + computeGap), but
   schedule deliberately.
9. **J** — P7 once the ceilings are 0 (or tiered).

Each non-C stage lands as its own gate-clean commit with a per-row landing test
and a one-line ratchet-rationale, exactly like the four wins this session. C is a
reviewed project with an explicit corpus re-baseline.

## Discipline (unchanged)

`make fmt && make vet && make lint && make test`, then `make verify-bytecode`
(differential + whole-corpus parity + combination + property fuzz + `-race` +
`aqldebug`, **0** divergences) and `make status`. Ratchets move down only, with a
rationale. Gate-clean-or-revert: every unsound attempt this session and prior was
caught by the differential and reverted — keep that the backstop.
