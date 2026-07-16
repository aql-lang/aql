# REFUSAL-CLOSURE.0 — compiling the remaining refusal shapes

Status: DESIGN (2026-07-15). The runtime-independence program's ratchets
sit at their finish lines (census 6000/6000 native, refusals 0, bails 0,
frontier 0 expected-red, public `Run` compiled). What remains is the
OFF-CORPUS refusal envelope — shapes that return `compile_refused` and
run on the interpreter by design ("slow, not wrong"). This note designs
the compile strategy for each, so any of them can be landed when its
cost is justified. Every mechanism below reuses machinery that already
exists and is proven; none requires a new architectural idea.

The shared soundness rule, unchanged: a shape compiles only when its
compiled execution is BYTE-IDENTICAL to the interpreter (values, error
taxonomy, output, binding state) — otherwise it keeps the refusal. Each
landing must fire the corresponding pinned-refusal test's contract (the
mustRefuseWithParity pins in bytecode_edge_findings_test.go and friends
flip to mustCompileWithParity — the edge-findings file documents this
exact graduation pattern in its §1 header) and pass the full battery.

## 1. Wide error-join forward drift — `5 do [7] error ["x"] add 1`

Refusal: "forward operand accounting across a dynamic/island residual
(Stage 3)". The catch result joins to dynamic(Integer|String), so `add`'s
dispatch is value-dependent: the String overload matches all-stack
(consuming the leading `5`), the Integer overload forward-collects `1`.
No static record picks one arm, and the two arms consume DIFFERENT stack
depths — that accounting is the refusal.

**Mechanism: the mark-bounded mixed re-step, generalised.**
`OpStackMark` + `OpCallDynMixedFromMark` already reproduce the
interpreter verbatim over a runtime-variable region (landed for the
do-catch variadic merge): the region `stack[mark:]` plus fixed values
above it re-step through the island machinery, auto-apply hazard
included. The drift site is the same shape one word later: emit the mark
BEFORE the leading residual (`5`), let the do-catch lower as today, then
lower the drifting word (`add` + its forward const `1`) as a
from-mark mixed re-step whose window is [region, forward-const]. The
island re-steps `5 <catch-result> add 1` — the interpreter's own
dispatch resolves the arm, so both consumptions are correct by
construction. Depth accounting disappears because the island returns
the WHOLE region's net (the existing FromMark contract).

Cost: an island (interpreter machinery for one statement), not a native
lowering — but the program compiles, and the island is bounded to the
one statement. Effort: M. The negatives already pinned (mul/sub/String
tokens, no-leading-residual) stay native — the gate fires only where
refuseForwardStackDrift fires today.

## 2. Deferred-token dispatch windows — `def m (flex {a:1}) f m.a`

Refusal: "unmatched dispatch recovered at f". **LANDED 2026-07-16** — and
the mechanism turned out SMALLER than the island this section originally
designed. Investigation showed the premise was stale: by dispatch-recovery
time the check pass has already EVALUATED the reach in place (the recorded
poly-dot event, a product of the Phase-4.2 store-shape typing), so the
failed window holds an event-produced DYNAMIC carrier — not a raw Reach
token. The definiteness screen was declining it at its `v.Dynamic` arm,
one line above the Reach screen the refusal was attributed to.

**Mechanism as landed: route dynamics to the runtime rematch.** The
screen now classifies a Dynamic operand alongside carriers
(`v.Carrier || v.Dynamic → hasCarrier`, engine.go): OpDispatchRematch
re-runs the match over the operand's LIVE runtime value — exactly what
the interpreter's dispatch examines, so the static tag (which is all
"dynamic" means) never enters. A runtime no-match raises the
byte-identical rich signature_error (verified: code, detail, position,
notes and help all match); a match defers to the interpreter. The
existing rematch gates still hold the line: a dynamic with no compiled
home fails operand resolution, and a window under a leading stack
residual fails the written-tuple contiguity bound (that shape — the
cell mutated through an opaque fn param behind a residual — keeps the
sound whole-program refusal, pinned in
TestUnmatchedDispatchTrapNegatives). The zzRefusingRow fence fixture,
the RunCompiledReason pin and the trap-negatives pin were re-pointed to
the §5 variadic-loop-def shape (blocked indefinitely, so stable).

The raw-Reach/ParenExpr/InterpString token screen REMAINS for windows
that genuinely hold unevaluated tokens; if such a shape resurfaces, the
island election originally designed here (a fully-baked OpFallback span
re-step, arming dynEnv so the island resolves names registry-visibly)
is the ready mechanism.

## 3. Member-fn auto-apply mid-expression — `m.double 21 eq 42`

Refusal: "member fn value auto-applies mid-expression". The interpreter
applies the parked fn the moment `21` arrives (a stepLiteral
ARRIVAL-loop event, not a word dispatch); the recorder only sees word
dispatches, so the downstream `eq` stole the operand in the record.

**Mechanism: record the arrival-apply as a dispatch event.** The check
pass runs the same arrival loop — at the moment execFnDefLiteral fires
the parked fn on an arrival, the recorder is IN SCOPE and knows the fn
operand (provenance: the `m.double` dot-read event) and the arriving
operand. Record it as a call event lowering to the EXISTING
OpCallDynamicTrailing (fn value + trailing arg — the CALL_DYNAMIC
family landed in P4). The statement-tail form already lowers this way;
the mid-expression form differs only in WHERE the event is recorded
(the arrival loop rather than statement close). Effort: M — the
arrival-loop recording seam is new, the lowering is not. The negative
(`m.x eq 5` — non-fn member reads) never enters the arrival-apply path.

## 4. Computed branch bodies — `if (n eq 0) [99] (range 2 4)`

Refusal: "computed branch arm is a spliced list body". The interpreter's
spliceArg EXECUTES a paren-arrived list as a code body; the compiled
value path would push the list as data.

**Mechanism: the dyn-body island, per arm.** tryRecordDynBody /
CompileDynBody already compile a DYNAMIC code body dispatch: the body's
runtime sub-run reads any name (dynEnv widens every def to
registry-visible — the OpBindDynScope/OpBindGlobal machinery), and the
VM brackets the frame so `args` resolves. Lower the REACHABLE computed
arm exactly as a dyn-body dispatch: the branch takes the arm → push the
computed list → the island re-steps it as a body over the live registry.
The interpreter's splice semantics are the island's semantics — parity
by construction. Dead arms (constant condition) and scalar arms keep
today's native lowering. Effort: M.

## 5. Variadic loop-collect defs — `def xs (for 3 [1])`

Refusal: "def `xs` consumes loop results (Stage 2 loops only feed the
program residual)" (lower.go — the variadic sim slot has no single value
to bind).

PROBED 2026-07-16 (RunInterp, the authoritative runtime): the interpreter
binds `xs` to the region's FIRST value (the stack-deepest — `def i 0 def
xs (for 3 [def i (add i 1) i]) xs` → `[2 3 1]`: `xs = 1`, residual `[2
3]`) and spills the remaining N−1 as residual. The empty-region case
diverges: `def xs (for 0 [1]) 99` → `[]` (the interpreter FORWARD-COLLECTS
the next token `99` as `xs`, so nothing spills), and `def xs (for 0 [1])
xs` raises `undefined_word` — hence a static-trip≥1 gate is mandatory.

**Mechanism designed and PROTOTYPED 2026-07-16 (splice-at-mark).** An
`OpStackMark` opens the region before the loop; a from-mark `OpBindGlobal`
binds `stack[mark]` (the first value) and SPLICES it out of the region
bottom (`copy(stack[mark:], stack[mark+1:])`), leaving the rest as the
residual; a `loopStaticallyNonEmpty` gate (all bounds concrete, the
FOR_NEXT trip test true on the first iteration) guarantees a non-empty
region so the splice never underflows. The VM/lowering side is correct.

**BLOCKED — reclassified S → check-mode change.** A probe of the compile
front-end shows the CHECKER does not model this shape as the interpreter
does. It collapses the loop into a single variadic carrier and has `def`
consume the WHOLE region: `def xs (for 3 [1])` records a check residual of
LENGTH 0 (region fully consumed), and `def xs (for 3 [1]) xs` binds
`xs = [:Integer]` — the ENTIRE region carrier, producer = the loop event —
so the read re-surfaces the whole region. The runtime splice removes ONE
value, so the read case would ship `[1 1]` where the interpreter yields
`[1 1 1]` — a SILENT MISCOMPILE. There is no seam to detect the mismatch
at lower time (the read "matches" the variadic sim slot spuriously).

Making §5 sound therefore requires a CHECK-MODE forward-collection change:
a forward-collecting word consuming a variadic loop region must take ONE
element (bind the first-value carrier) and leave N−1 as a variadic
residual carrier, mirroring the interpreter's per-value collection. That
is a deep, broad engine change (stepLiteral / autoEval / the def handler's
value source), well beyond the original "S" lowering estimate, and it must
be validated against the whole `compiled_fullcorpus` oracle before it can
land. Until then the refusal STAYS — slow, not wrong. The splice-at-mark
mechanism above is the ready lowering half for that future landing.

## 6. Poly-decline arms (fn-predicate / gradual-Any overloads)

Two decline reasons in tryCompileUserPolyArms keep sites refusing:

- **Zero committed returns** (`len(committedReturns) == 0` — the
  zero-return overload set). **LANDED 2026-07-16.** The poly gate's
  `len(committedReturns) == 0` bar is dropped: an empty committed contract
  is admitted, `userPolyArmShapeOK` already matches Returns position-wise
  (0 == 0 keeps the arms consistent), and a new per-arm `unitNetsZero`
  gate requires every arm's body to net exactly zero residual values — so
  the recorded 0-output `OpCallUserPoly` is byte-identical to whichever arm
  the VM's runtime re-match selects. `buildFnBodyReturnsFn`'s 0-residual
  path records the poly call and returns nothing; anonymity stays refused
  by `findOwningFnDef`'s `owner.Anonymous` gate. A declared-`[]` arm whose
  body leaves a RESIDUAL (the "residual IS the result" shape) fails
  `unitNetsZero`, so that set keeps its refusal (the `pick`/`zpick`
  fixture). The `shout` fixture (`TestPredicateOverloadDispatchCompiledParity`)
  now compiles with output parity (`"o\n"`, two `CALL_USER_POLY`); the
  declining fixture was re-pointed to `zpick` to keep `planUserPolyDispatch`'s
  refusal arm covered.
- **Body-local multi-overload fns** (the fn-baseline gate: the runtime
  Lookup cannot resolve a name popped before the VM runs). **LANDED
  2026-07-16.** UserPolyRef gains a stored dispatch table
  (UserPolyRef.Sigs — the arm signatures frozen at record time);
  matchUserPoly's stored mode re-matches over the frozen subset with no
  live Lookup and no index/Impl drift guard (there is no live table to
  drift from). The doc's "binding cannot change" premise needed one
  repair, found by probing: DYNAMIC-SCOPE mutation — a callee whose own
  body rebinds the same name overlap-replaces the local in place, and
  the replacement SURVIVES the callee's teardown (interpreter
  semantics, pinned at [141] in TestBodyLocalMultiOverloadPolyStored's
  mutator negative) — so the freeze gates on Check.FnBinders: when any
  OTHER fn binds the word as a body-local, the refusal stays and the
  interpreter owns the shape. Pins: both arms dispatch by runtime value
  through one compiled program, a no-match defers to the interpreter's
  canonical signature_error, and the module-scope live-Lookup mode is
  untouched.

## 7. Per-callback stamp declines (not whole-program refusals)

- **Runtime-minted capture values** ("closure captures a runtime-minted
  value (no compile identity)"): the identity gate exists so the
  freshen/share machinery can't misclassify. **LANDED 2026-07-16** — and
  simpler than the const-bake this bullet designed: for a DETACHED unit
  the capture is per-ref and frozen, so StampDetachedFn mints a fresh
  identity on a CLONE of the captured slice (confined to the fork's
  compile; the published value stays untouched) and the existing capture
  slot path keys it like any ID-bearing capture — no emit change at all.
  The whole-program compile keeps the identity gate (there the
  freshen/share concern is live). Pinned in TestStampDetachedFnShapeGates
  (eng) and TestComputedCaptureStampsAndRunsWithCaptures (lang,
  end-to-end with per-value refs + interpreter parity).
- **Multi-overload fn values**: stamp EVERY own sig to a unit and give
  CompiledFnRef a sig-table dispatch mode — MatchFnSig at invoke picks
  the unit, the user-poly re-match precedent applied to the callback
  seam. Effort: M.
- **Stale-dep refs degrade permanently to CallAQL**: **LANDED
  2026-07-16.** InvokeCallback, on depsFresh failure, re-runs
  StampDetachedFn against the live bindings via a per-ref restampBox
  (CompiledFnRef.restamp — allocated by StampDetachedFn only, so
  compile-time refs keep the interpreter path): the box carries the
  stamp inputs (the §7a-cloned fd + pos) and the current re-stamped twin
  under a mutex serialising concurrent invokers of one shared sig; each
  re-stamp snapshots the new generations, a stable rebind pays ONE
  compile then runs the VM again, and restampMaxTries (3) bounds a hot
  rebinding loop — after the budget the seam stays on CallAQL (slow,
  not wrong). Pinned in TestInvokeCallbackJITRestamp (freshen-to-live-
  value parity, twin reuse without recompile, budget exhaustion,
  disarmed decline). This is also the mechanism for the plan's Phase-6
  "JIT detached-unit cache" item.

## 8. AQL-written mini compile hooks — keep the opt-out

An AQL compile hook is a macro whose check-time expansion is
CONTRACTUALLY not the runtime expansion (§13): the hook may read state
that exists only at runtime. Both compile strategies are unsound or
self-defeating: baking the check-time expansion violates the contract,
and a runtime JIT of the hook + re-step of its expansion is exactly the
interpreter with extra steps. This is a DESIGNED opt-out, like wasm's
pinned RunInterp — recorded, not scheduled. (Go hooks compile since
fa9e844; non-concrete src/opts refusals stay: the record cannot see the
values the runtime expansion would consume.)

## Sequencing and gates

Cheapest-first, each with the standard battery + fullcorpus
0-divergence + census ratchets, one landing per commit:

1. §5 peek-at-mark — **RECLASSIFIED S → check-mode change, BLOCKED** (see
   §5: the checker binds the whole variadic region where the interpreter
   binds one value; the lowering half is prototyped and ready, but the
   sound landing needs a forward-collection change validated against the
   full corpus). The TestGlobalBindEnvelope variadic pin STAYS red.
2. §6a zero-return poly arms — **LANDED 2026-07-16** (unitNetsZero gate);
   the declining-poly pin flipped, re-pointed to the `zpick` fixture.
3. §2 deferred-token windows — **LANDED 2026-07-16** (the dynamic-operand
   rematch; see §2 for why the island design was not needed). The
   effect-fence pins, RunCompiledReason and the trap-negatives refusal
   were re-pointed to the §5 variadic-loop-def fixture (stable — §5 is
   blocked indefinitely).
4. §7a capture identities — **LANDED 2026-07-16** (the detached-stamp
   capture-clone mint; see §7). §7c JIT re-stamp — **LANDED 2026-07-16**
   (the per-ref restampBox; see §7).
5. §6b sig-table poly — **LANDED 2026-07-16** (UserPolyRef.Sigs stored
   mode + the FnBinders dynamic-scope gate; see §6). §1 mark-bounded
   drift island, §3 arrival-apply, §4 dyn-body arms, §7b multi-sig
   stamps (M each) remain.

After all of §1–§7, the only remaining interpreter execution on any
default path would be: check-mode (the compile front-end itself),
module loads (attributed), const-folds (attributed), explicit RunInterp,
and the §8 designed opt-outs — at which point the refusal envelope is
empty for every expressible shape and the `AQL_COMPILE_FALLBACK` hatch
plus the 51 hatched legacy pins can be retired on schedule.

The external validation for all of this remains the voxgig-aql sweep
(steps 7–9 re-baseline) in a session sourced from that org.
