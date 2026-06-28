# Module-fn param-slot compilation — focused follow-up plan

Goal: let a CLOSURE inside a MODULE-fn body capture that fn's params (notably a
`[comp:Function]` comparator) so the comparison-sort algorithms force-compile.
Currently they refuse at the outer `each` with "unreachable capture comp". This is
the next sort-cluster leaf after the comparator lowering (`OpCallDynTrailTop`,
landed 1d765fff) and the make-list-in-fn-body leaf (landed f28cc36a).

Soundness invariant (non-negotiable): `compile == interpret`, 0 miscompiles. The
langspec differential is BLIND to off-corpus shapes (it missed two real miscompiles
this session) — every step ships a hand-pinned `RunCompiledStrict==Run` off-corpus
regression + the voxgig `--compile`==interpret sweep, gated by `make verify-bytecode`.

## 1. Root cause (traced to the architectural bottom)

A closure (the outer `each`) capturing a name resolves the capture to a re-pushable
operand via `EmitState.resolveOperand` (emit.go) — an event, a frame-LOCAL SLOT, or
an inert const. For `[comp:Function]`:

- `ComputeCaptures` (fn_capture.go) returns `r.Defs.Top("comp")`, which is the
  **FnDef value** `InstallFnDef` pushed (core_helpers.go:725 `r.Defs.Push(name,
  NewFnDef(entry))`; note `TFnDef` conforms to `TWord` in the lattice, so an
  `AsWord` on it reads empty — the earlier red herring "Word('')").
- `resolveOperand(FnDef)` needs a home. A TOP-LEVEL fn's body is compiled as a
  **unit with param slots** (`buildFnBodyReturnsFn` → `StartFnCompile`,
  core_helpers.go:537; params registered in `emitUnit.localByID` at emit.go:1476),
  so `comp` resolves to its slot — my top-level repros compile. A concrete inert
  comparator (`cmp/r`) also const-bakes. But an ABSTRACT Function carrier with no
  slot has no home → refuse.

**The divergence (confirmed by the comp-install stack trace):** a MODULE fn
(`Sort.insertion`, a FnDef WRAPPER value reached by dot-access) dispatches through
`execFnDefLiteral` (engine.go:3920) → `execFnDefSig` (engine.go:4660) → `CallAQL`
(registry.go:1238), which binds params via `InstallFrameBinding` on the **def-stack
only — NO unit, NO param slots**. A NAMED top-level fn instead dispatches through its
registered sig's `ReturnsFn` = `buildFnBodyReturnsFn`, which `StartFnCompile`s the
body as a unit with param slots. Instrumented proof: at the outer-each capture the
enclosing unit had ZERO named param slots (`unitNames=[]`).

So: **module-fn bodies are never compiled as param-slot units during CompileCheck
re-entry**, so their `Function` params have no slot for a closure to capture.

## 2. The fix — route module-fn compile re-entry through the param-slot unit path

Two candidate approaches; (A) is preferred (reuses the proven path), (B) is the
fallback if (A)'s dispatch surgery proves too broad.

### (A) Compile the module-wrapper's body as a unit (reuse buildFnBodyReturnsFn)
When a module-wrapper FnDef with a real AQL body (not a trivial-delegation wrapper —
`isTrivialDelegationBody` is false; `comp` lives in a real body) is dispatched in
COMPILE mode (`Check.Compiling`), drive its `buildFnBodyReturnsFn` (which already
exists on its sub-registry sig) so the body is `StartFnCompile`'d as a unit with
param slots + `SetUnitParamTypes`, instead of the `execFnDefSig`/`CallAQL`
def-stack run. The each closure then captures `comp` via its param slot.

- Investigate first: WHY does `execFnDefLiteral`/`execFnDefSig` take the CallAQL
  body-run instead of the registered ReturnsFn unit-compile for a module wrapper?
  (The named path uses `execMatch`'s `match.Sig.RunInCheckMode` intercept at
  engine.go:2514 to call the ReturnsFn; the FnDef-value path at 2995 →
  execFnDefLiteral may bypass it.) The fix likely routes the module-wrapper
  dispatch through the same ReturnsFn intercept the named path uses.
- The sub-registry threading must be preserved (the body runs in `fnDef.Registry`
  so module-private words resolve — see lang/go/CLAUDE.md "Module FnDef Wrappers").

### (B) Give the CallAQL body-run param SLOTS (a local param-slot unit)
If routing through the ReturnsFn is too invasive, have the compile-mode CallAQL /
execFnDefSig path open a lightweight `StartFnCompile` unit for the body so named
params get `localByID`/`localByName` slots (and `InstallFnDef` of a Function param
ALSO registers the param's slot id), so `resolveOperand` / the name-fallback
(`resolveCapturedParam` + `emitUnit.localByName`, drafted this session — reverted as
inert without slots) resolves the capture. The drafted name-fallback is the
right shape; it only needs the enclosing unit to actually HAVE the param slots.

## 3. Soundness — the specific hazards

1. **Don't change interpret-mode dispatch.** Gate every new path on
   `Check.Compiling`. The interpreter must keep running module fns exactly as today
   (the repos' own gate is compile==interpret via fallback — never regress the
   interpret 44/48).
2. **Module sub-registry fidelity.** A module fn's body resolves module-private
   words in `fnDef.Registry`. Any unit-compile must analyse/record in that registry,
   not the main one (else a private word becomes undefined → false refuse or, worse,
   a wrong resolution). Mirror `execFnDefSig`'s `capturedReg` threading.
3. **Captured-comparator apply correctness.** Once `comp` captures soundly, the
   inner `(prev key comp)` apply lowers via `OpCallDynTrailTop` (already landed) —
   verify the captured slot value flows to it with the right arg order (the existing
   off-corpus comparator regression covers ordering; add a MODULE-fn variant).
4. **Recursion / re-entry.** A module fn calling itself (or mutual) must not loop the
   unit-compile (memoise on the existing `FnAnalysisKey`, as `buildFnBodyReturnsFn`
   already does at core_helpers.go:482).

## 4. Mandatory regressions (the differential is blind to all of these)
- A module fn with a `[comp:Function]` param + an `each`/nested-`each` body that
  applies `comp` → force-compiles native (no FALLBACK island) AND
  `RunCompiledStrict==Run`, over BOTH a `cmp/r` and a module-defined comparator.
- The SAME shape at TOP LEVEL must still compile (no regression).
- A module fn whose body references a module-PRIVATE word inside the closure →
  compile==interpret (sub-registry fidelity).
- A trivial-delegation module wrapper must stay on its fast `execMatch` path
  (no behaviour change) — `wrapper_dispatch_test.go` stays green.
- The voxgig `--compile`==interpret sweep over all 48 files: still 0 miscompiles.

## 5. Staged steps (each its own gate-clean commit)
0. Reproduce minimally OUTSIDE the sort lib: define a small module inline
   (`module […]`) with a `[comp:Function]` fn whose body nest-`each`-applies comp,
   import + call it. (Top-level repros compile; this must reproduce the refuse, so
   the fix can be validated without the whole sort lib.)
1. Confirm the dispatch divergence (A's investigation): instrument which path
   (ReturnsFn unit-compile vs execFnDefSig/CallAQL) a module-wrapper dispatch takes
   in compile mode, and identify the exact branch (engine.go:2995 / execFnDefLiteral)
   that bypasses the ReturnsFn intercept.
2. Land approach (A) or (B), gated on `Check.Compiling`, sub-registry-faithful.
3. Re-sweep the sort algorithms; comp-capture clears for ALL comparison sorts.

## 6. After comp-capture — the residual sort chain (do NOT forget; flips need ALL)
sort_smoke calls all 16+ algorithms. Even with comp-capture cleared, each still
chains through (root-caused this session): **branch-multi-value** (an `if` arm
running several statements nets >1 value — "branch leaves extra values (Stage 2
lowers single-result branches)"), **swap-at / convert List** over an Array
(surfaced as "check diagnostics"), plus the distribution sorts' own leaves. A
sort-FILE flip lands only when every algorithm's whole chain clears. The comp-capture
fix is the highest-leverage next step (shared by every comparison sort), but it
flips 0 files alone.

## 7. Effort / risk
Approach (A) is module-dispatch surgery — load-bearing for ALL module calls; a wrong
move silently breaks every `pkg.word` dispatch, and the differential is blind to the
off-corpus shapes. Treat as a dedicated multi-step effort with the full gate +
off-corpus regressions at each step, not a quick patch. This is the right scope for a
focused follow-up session.

## 8. ATTEMPTED + VALIDATED the unification — sound, fixes comp-capture, but REGRESSES run-spec (recursion)

Implemented approach (A) end-to-end and ran the full gate. Findings (the unification
ATTEMPT is reverted; the insights stand):

- **The unify point is engine.go:4179** (execFnDefLiteral's sub-registry branch). A
  TRIVIAL-delegation wrapper already routes through `execMatch` (→ carrierResults →
  the matched sig's ReturnsFn = unit-compile); the NON-trivial AQL body routed to
  `execFnDefSig`/`CallAQL` (def-stack, no unit). Routing the non-trivial body through
  `execMatch` IN CHECK MODE — wrapped in `e.shareCheckState(fnDef.Registry)` so the
  body's `buildFnBodyReturnsFn` compiles its unit into the MAIN program and
  `RecordUserCall` references it — is the change.
- **It IS SOUND**: `make verify-bytecode` GREEN (0 miscompiles). Trivial/each/make
  module fns compile correctly; **the sort comp-capture leaf CLEARS** (the inline-
  module repro advances past it). `shareCheckState` is ESSENTIAL — without it the unit
  lands in the sub-registry's emit and every module-fn call refuses "user fn call
  (Stage 3)" (a coverage regression `make test` caught).
- **BUT it REGRESSES `TestRunSpecHarnessCompiles`** (a positive test: `Test.run-spec`
  must compile native). Under the unit path, `test-describe` refuses "unmatched
  dispatch recovered at test-describe" — the RECURSIVE-code-body wall. The inline
  `CallAQL` path AVOIDED this because the recursion terminates DATA-DRIVEN during
  inline analysis (empty `subs`), so test-describe never compiles as a self-
  referential unit. **So the two paths exist partly BECAUSE inline handles recursion
  that unit-compile does not.**

### Revised fix: PROBE-AND-FALLBACK (preferred) or co-implement recursive closures
A naive unify regresses the recursive test framework. Two sound ways to land it:
1. **Probe-and-fallback** (lower risk, preferred): at engine.go:4179 in check mode,
   PROBE the unit-compile (execMatch in a throwaway emit state, like the closure
   probe at callable_words.go:262). If it records cleanly, COMMIT it (sort gets param
   slots). If it refuses (run-spec's recursion), fall back to the inline `CallAQL`
   path (run-spec keeps compiling). Both sound; no regression. The intricacy is the
   throwaway-state + replay at the dispatch level — model it on the existing closure
   probe.
2. **Co-implement recursive-code-body closure compilation** so test-describe compiles
   as a self-referential unit (memoise the in-progress unit on FnAnalysisKey so the
   recursive dispatch resolves to it instead of recovering). Larger; unblocks the
   test framework too — but that is the doc's separate multi-session feature.

Mandatory: `TestRunSpecHarnessCompiles` MUST stay green, AND the sort comp-capture
must clear, in the SAME change. The probe-and-fallback achieves both with the least
risk to the load-bearing module-dispatch path.

## 9. LANDED — fn-dispatch UNIFICATION (commit c5ff7bd9). One code path, all modes.
The design flaw is fixed: every function — named, bare-word, trivial-delegation module
wrapper, AND a real module-fn body — dispatches through `execMatch`, so a fn body is
compiled the SAME way regardless of how the fn was reached (a param-slot unit via the
matched sig's ReturnsFn). The trivial/non-trivial split in execFnDefLiteral is gone
(isTrivialDelegationBody removed). Sub-registry fidelity via match.Reg + shareCheckState;
INTERPRET byte-identical (buildFnBodyHandler runs a FOREIGN-registry body in its home
registry via CallAQL — same execution, one dispatch path). GATE: verify-bytecode GREEN +
crossdiff GREEN (0 interpret divergences) + fmt/vet/lint. The module comp-capture leaf
CLEARS. FOLLOW-UP (handled, soundly falling back): recursive `test-describe` hits the
recursive-closure limit the inline path masked — TestRunSpecHarnessCompiles re-scoped to
compile==interpret, reducibleCeiling 2->3. NEXT: §10 recursive-code-body closures (restore
native run-spec) + the sort chain (comp-capture is cleared; the `fn call operand of unknown
provenance` next leaf = passing a module-fn VALUE as a call arg, then branch-multi-value /
swap-at / convert × 16 algorithms).
