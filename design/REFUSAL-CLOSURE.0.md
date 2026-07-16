# REFUSAL-CLOSURE.0 — compiling the remaining refusal shapes

Status: DESIGN (2026-07-15; **revised 2026-07-16** after an adversarial
review — reason strings, op names, and mechanism preconditions corrected
against HEAD; a §9 inventory added; the closure claim scoped honestly.
§6a **landed** 2026-07-16 with the review's required net-zero gate; §5
**blocked**, reclassified to a check-mode change — see the sections).
The runtime-independence program's ratchets sit at their finish lines
(census 6000/6000 native, refusals 0, bails 0, spec frontier 0
expected-red, public `Run` compiled; the *langspec* frontier compile
ledger separately carries 3 expected-red rows — §9.1, a different
ledger, not a contradiction). What remains is the OFF-CORPUS refusal
envelope — shapes that return `compile_refused` and run on the
interpreter by design ("slow, not wrong"). This note designs the
compile strategy for **each of the eight families below** (§9 inventories
what the eight do NOT cover), so any of them can be landed when its cost
is justified. Every mechanism reuses machinery that already exists and
is proven; none requires a new architectural idea.

The shared soundness rule, unchanged: a shape compiles only when its
compiled execution is BYTE-IDENTICAL to the interpreter (values, error
taxonomy, output, binding state) — otherwise it keeps the refusal. Each
landing must flip the shape's pinned-refusal test to a compile-parity
pin: for §1/§3/§4 those are the `mustRefuseWithParity` calls in
bytecode_edge_findings_test.go (whose §1 header documents the
graduation pattern); §2/§5/§6a are pinned elsewhere — `zzRefusingRow`
(bytecode_effectfence_test.go), the variadic pin in
`TestGlobalBindEnvelope` (bytecode_globalbind_test.go), and the
declining-poly pin (bytecode_flip_divergences_test.go, re-pointed to
the `zpick` fixture at the §6a landing) — which assert
`compile_refused` (and should also pin the reason substring; see §5)
directly. And every landing passes the full battery.

## 1. Wide error-join forward drift — `5 do [7] error ["x"] add 1`

**LANDED 2026-07-16** (`tryRecordDriftWindow`, eng/go/drift_window.go).

Refusal was: "forward operand accounting across a dynamic/island residual
(Stage 3)". The catch result joins to dynamic(Integer|String), so `add`'s
dispatch is value-dependent: the String overload matches all-stack
(consuming the leading `5`), the Integer overload forward-collects `1`.
No static record picks one arm, and the two arms consume DIFFERENT stack
depths — that accounting was the refusal.

**As landed — a STATIC window, no marks.** The doc's from-mark sketch
rested on a stale premise (the §2 lesson again): the window is
fixed-width at record time — [leading residual(s), the ONE catch result,
the word, the forward literal] — because the drift gate itself requires
the matched operands to be a contiguous span directly under the word, and
a fixed window needs no `OpStackMark`. `tryRecordDriftWindow` (hooked
right before `refuseForwardStackDrift`) records one generic evCall event
whose operands are the window laid out top-first — the WORD ITSELF rides
as an inert const (`word(add)`) — lowering to the existing
`OpCallDynamicMixed` with Arg = window width. The VM islands the window
verbatim through `islandRun`: the word token DISPATCHES in the island
with the interpreter's own registry-resolved forward collection over the
LIVE value, so the arm choice AND its residual count are byte-identical
by construction (the Integer pass-through forward-collects → `[5 8]`; the
String handler binds all-stack). The event is variadic-flagged; the
in-order layout machinery promotes the catch result to a frame local and
re-pushes the window in source order.

Gates (each declining shape keeps the refusal, pinned):
- TERMINAL only — nothing after the forward literal but End/DefCleanup
  (a downstream consumer would need a static count the island can't
  promise): `… add 1 drop` still refuses.
- BYSTANDER-FREE — no data values below the window (`1 2 3 do … add 1`):
  the in-order reconciliation cannot interleave the window's const
  re-pushes with untouched values.
- No variadic-event operands (fixed width), contiguous matched span,
  every operand with a compiled home.

Landing tests: `TestEdgeFindingForwardAcrossErrorResidual` (both raise
paths compile with parity; both decline fences pinned). The negatives
already pinned (mul/sub/String tokens, no-leading-residual) stay native —
the window fires only where refuseForwardStackDrift fired.

**Scope (per the adversarial review's qualification):** this is the
review's sanctioned narrow-window variant — the island result is
variadic, so mid-statement drift shapes (`… add 1 mul 2`) and
fragment-context drift sites keep the sound refusal (pinned:
`… add 1 drop`). The statement-end window the review sketched remains
the future widening if those shapes ever matter.

## 2. Deferred-token dispatch windows — `def f fn [[x:List][List][x]] def m (flex {a:1}) f m.a`

(The reproducer is the full `zzRefusingRow` fixture,
bytecode_effectfence_test.go:94 — `f` must be a defined fn for the
dispatch-recovery window to exist; without the preamble the source fails
as check diagnostics, not this refusal.)

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

**Scope note (from the adversarial review):** "unmatched dispatch
recovered at `w`" has several decline causes; the landing above owns
the deferred-token one. The others keep the refusal until designed:
multi-overload user fns over an Any/disjunct operand at the no-match
recovery sites (plausibly §6b's stored-sig-table re-match);
predicate/refinement-typed user-fn params, where the guarded CALL_USER
enforces only nominal types (a candidate for re-running the predicate
via the existing RunTypedBind/OpBindTyped machinery); and
tryRecordPoly's own safety gates (meta / fn-value / mutating /
code-body / multi-result words).

## 3. Member-fn auto-apply mid-expression — `m.double 21 eq 42`

Refusal: "member fn value auto-applies mid-expression". The interpreter
applies the parked fn the moment `21` arrives (a stepLiteral
ARRIVAL-loop event, not a word dispatch); the recorder only sees word
dispatches, so the downstream `eq` stole the operand in the record.

**Mechanism: record the arrival-apply as a dispatch event. LANDED
2026-07-16** — via the M2c shaped-method chassis rather than the
OpCallDynamicTrailing seam this bullet sketched. The memberFnRead tag
now carries the pinpointed member VALUE (readFnMemberValue — a concrete
container + concrete key), and tryMemberFnArrivalDispatch
(method_shape.go), hooked beside tryShapedMethodDispatch where the
check pass steps the member-read carrier, claims the member's SINGLE
plain signature's arity of inert tokens (the ARRIVAL window — the
parked fn fires the moment its args arrive, so the next word never
enters the collection) and records a guarded mid-stream OpCallDynMethod
whose runtime value — the real container read's product — drives the
apply (the island path, byte-identical to the interpreter's forward
auto-dispatch). Two implementation notes: the model runs SUSPENDED (a
plain user fn's ReturnsFn records its own CALL_USER through
core_helpers, not the outcome seam — a live probe leaks a phantom event
the lowering cannot seat), and no body unit is compiled (callDynMethod
islands any plain callable; the declared return contract is
engine-enforced at run time, so the modelled out carrier is sound). The
paren-bounded variant (`(m.double 21) eq 42`, previously "fn-value
application bounded by a paren") graduated with it. Decline fences keep
today's refusals: computed-key reads (no pinpointed member),
multi-overload members (runtime first-match not modelled), captures,
anonymous members, quote/no-eval params, non-inert windows.

## 4. Computed branch bodies — `if (n eq 0) [99] (range 2 4)`

Refusal: "computed branch arm is a spliced list body" (raised from
lang/go/native/native_control.go:694, prefixed "if: …"). The
interpreter's spliceArg EXECUTES a paren-arrived list as a code body;
the compiled value path would push the list as data.

**Mechanism: the dyn-body island, per arm. LANDED 2026-07-16** — as a
BODY SYNTHESIS rather than a bespoke lowering: computedArmDoBody
(native_control.go) rewrites a COMPUTED List-conforming arm to the
equivalent `[do <arm>]` body and routes it through the ORDINARY arm-body
path (RunCarrierBodyWithDefs + the branch fragment capture), where the
dyn-body machinery already owns the computed `do`. The equivalence was
probe-proven on every axis before landing: multi-values, def leaking
(do's keep-defs ≡ the splice's inline leak), and break/continue (the
FlowCtrl escape — the divergence-1 OpFallback translation makes this
hold in compiled loops). Recording pass only, so plain checks keep the
value-arm surface; dead arms, scalar/paren-scalar arms, def-bound
concrete list arms and quoted arms all keep their prior behavior
(pinned). The 2-arg `if`'s computed arm keeps a SOUND refusal ("if:
then-branch not captured" — a follow-on widening if it ever matters);
`case` arms are value-semantics in both engines and stay native.

(The review's filed quoted-break divergence in the adjacent do-body
seam landed independently as the divergence-1 fix + the PR #275
sentinel-scan widenings: valueHasSentinel, the interp/XML/map
recursion, and the transitive callee scan.)

## 5. Variadic loop-collect defs — `def xs (for 3 [1])`

Refusal: "def `xs` consumes loop results (Stage 2 loops only feed the
program residual)" — the VARIADIC-producer arm of lowerDynBind
(eng/go/lower.go:186): the variadic sim slot has no single value to
bind. (The sibling reason "dynamic-scope def `xs` of unpromoted computed
value", lower.go:195, is the NON-variadic unpromoted-producer arm —
including the zero-output-body loop — a DIFFERENT refusal that this
section does not clear.)

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

tryCompileUserPolyArms declines for MORE than two reasons; the full
reachable set (user_poly.go): zero committed returns, the body-local
fn-baseline gate, non-identical declared returns across arms
(userPolyArmShapeOK — probe-verified reachable:
`def id fn [[x:Any][Any][x]] def g fn [[a:Integer][Integer][a]
[a:String][String][a]] g (id 5)` refuses "gradual-Any arg to
multi-overload user fn `g`"), quote/type-literal/no-eval/raw/form param
slots, anonymous/macro/captured owning defs, deferred-param-list arms,
and variadic-returning arm units — plus defensive gates (<2 same-arity
arms, nil aggregate) that are non-shapes. This section designs the first
two; the others need mechanisms (the differing-returns case could type
the call's residual as the dynamic join of the arms' declared returns —
the §1 machinery) or an explicit designed-keep entry in §9 before the
envelope closes. The gradual-Any probe above should be pinned in
bytecode_edge_findings_test.go.

- **§6a: zero committed returns** (`len(committedReturns) == 0` — the
  zero-return overload set). **LANDED 2026-07-16.** The poly gate's
  `len(committedReturns) == 0` bar is dropped: an empty committed contract
  is admitted, `userPolyArmShapeOK` already matches Returns position-wise
  (0 == 0 keeps the arms consistent), and a new per-arm `unitNetsZero`
  gate requires every arm's body to net exactly zero residual values — so
  the recorded 0-output `OpCallUserPoly` is byte-identical to whichever arm
  the VM's runtime re-match selects. (The gate is load-bearing: a
  declared-`[]` arm pushes no ReturnCheck, so a body that nets values
  flows them verbatim to downstream consumers — `def f fn [[x:Integer]
  [] [x add 1]] f 1 add 1` → 3 — which a fixed 0-out call site cannot
  carry.) `buildFnBodyReturnsFn`'s 0-residual path records the poly call
  and returns nothing; anonymity stays refused by `findOwningFnDef`'s
  `owner.Anonymous` gate. A declared-`[]` arm whose body leaves a
  RESIDUAL (the "residual IS the result" shape) fails `unitNetsZero`, so
  that set keeps its refusal (the `pick`/`zpick` fixture). The `shout`
  fixture (`TestPredicateOverloadDispatchCompiledParity`) now compiles
  with output parity (`"o\n"`, two `CALL_USER_POLY`); the declining
  fixture was re-pointed to `zpick` to keep `planUserPolyDispatch`'s
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

  (The review's filed single-overload analogue — body-local conditional
  shadowing baking the shadow — landed as the CondBodyDepth
  conditional-fn-shadow refusal, PR #275 divergence 2.)

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
- **Multi-overload fn values**: **LANDED 2026-07-16** — and no
  sig-table dispatch mode was needed (a stale-premise reduction again):
  the invoke seam already dispatches through MatchFnSig BEFORE
  InvokeCallback, so the matched sig's own `Impl.Compiled` ref IS the
  sig table. As landed: `storedSigEligible` replaces the
  single-own-sig gate (per-sig: own, AQL body, non-empty,
  sentinel-free); the compile-time store-fn bake loops every stampable
  sig (per-sig unit + ref, `compileStoredFnUnit(fd, sigIdx, pos)`);
  the runtime path gains `StampDetachedSig(r, fd, sigIdx, pos)` with
  per-sig deps and a per-sig §7c restamp box, and
  StampFnValue/StampFnValueInPlace loop all stampable sigs (partial
  success is per-sig fail-safe: a declining sibling stays plain and
  interprets at its own matches). Pins:
  TestStoredFnMultiOverloadStampsPerSig (compile-time, incl. the
  sentinel-declined sibling), TestMultiOverloadHandlerStampsPerSig +
  ...PartialStamp (end-to-end service dispatch: the two-arity handler
  shape that never stamped, both units on the VM, parity), and the eng
  stamp-gate/report pins flipped to per-sig.
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
CONTRACTUALLY not the runtime expansion (MINILANG.5.md §13): the hook
may read state that exists only at runtime. Both compile strategies are
unsound or self-defeating: baking the check-time expansion violates the
contract, and a runtime JIT of the hook + re-step of its expansion is
exactly the interpreter with extra steps. This is a DESIGNED opt-out,
like wasm's pinned RunInterp — recorded, not scheduled. (Go hooks
compile since fa9e844; non-concrete src/opts refusals stay: the record
cannot see the values the runtime expansion would consume.)

## 9. Inventory — what §1–§8 do NOT cover

The eight families above were derived from the raise-site inventory
(grep MarkUncompilable / refusal-reason strings across eng/go/emit.go,
lower.go, engine.go, carrier.go, core_helpers.go and the lang natives).
The following LIVE shapes are outside them; each needs a mechanism, a
designed-keep entry, or an unreachability argument before the envelope
can be called closed.

### 9.1 The langspec frontier's three expected-red rows (L-DO part 2)

The langspec frontier compile ledger (frontierCompileLedger,
test/go/langspec/frontier_spec_test.go) pins three live refusing rows:
two def-msg do-catch rows refusing **"residual shape beyond Stage 1"**
(the seatResults raise family, emit.go:6808 — call-result-above-a-
literal / results-reordered / unconsumed-call-results) and one
module-export-in-variadic-region row refusing **"residual value not
statically materialisable"** (emit.go:6569). Their design belongs to the
completion plan's Phase-5 "L-DO part 2b" (see
RUNTIME-INDEPENDENCE-COMPLETION-PLAN.0.md); those landings flip the
frontierCompileLedger rows. Until then the closure claim excludes them.

### 9.2 Probe-verified expressible shapes with no section

- **Splice over a computed payload** (engine.go:3852) — `def xs (range
  1 3) def d word xs d` (pinned in multiout_closure_test.go). Candidate:
  the §4 dyn-body / mixed re-step island generalised beyond branch arms.
- **Interpolated XML with a runtime-computed part** (engine.go:4225) —
  the string sibling already compiles via RecordInterp/OpInterp; mirror
  it (an OpInterpXml rebuilding the tree per run, or a re-step island).
- **Loop-carried store of a variadic result** (lower.go:246) —
  `for 3 [ def acc (for 2 [1]) ]`. §5 covers only the top-level
  def-of-loop-collect; this needs a STORE_LOCAL_FROM_MARK sibling — and
  inherits §5's check-mode blocker.
- **Curried-factory body provenance** (emit.go:2959, "fn mk: body result
  of unknown provenance") — `((mk 1) 2) 3`. Candidate: extend
  tryReturnedClosure to capturing/nested closures via §7a's
  unpooled-const capture mechanism. Note emit.go:6343 ("fn-value apply
  arity mismatch — curried chain or partial apply", currently ZERO test
  pins) is shadowed upstream by :2959 for this shape and remains the
  backstop once :2959 graduates — pin both.
- **Paren-bounded fn-value application** (engine.go:6882, "fn-value
  application bounded by a paren (dynamic value precedes args)") —
  `add 1 ((m get "f") 5)`, the stamp suite's canonical refusing fixture.
  Distinct from §3 (a paren-close guard on a LEADING dynamic carrier,
  not the arrival-loop member event; the trailing arm already compiles
  via RecordDynApply). Candidates: extend RecordDynApply to the leading
  case with the paren as the event boundary, or a §1-style mark-bounded
  island over the paren window. Retiring the AQL_COMPILE_FALLBACK hatch
  (Sequencing) requires re-pointing or graduating the two stamp-suite
  pins whose fixture this is — the same bookkeeping as zzRefusingRow.
- **Surface-shape typed dispatch** (engine.go:7857) — the S2 generic
  surface call (`g (make Circle {})` over `gen [(T extends Shape)]`).
  Candidate: runtime re-match over the exposer's registered op (the
  §2/§6b precedent), or a designed opt-out.
- **fn/afn construction over a computed operand**
  (native_definition.go:1282/:1446) — `fn Integer (mk [String])
  ['five']`. Either a §8-style designed opt-out (the site's rationale:
  the compiled unit would bake the check-time placeholder) or a runtime
  re-construction op in the OpBindTyped style.

### 9.3 Residual guard-owned declines

The typed-def RecordTypedBind decline arms (native_definition.go:646
dynamic-refinement reparent, :717 fn-predicate bind, :998 DepScalar
validation — the first two pinned) and the shaped-method guards
(method_shape.go:213 zero-arg landing, :482 operand of unknown
provenance — both pinned) stay interpreter-owned; each should be marked
expressible-keep or defensive-only as it is audited.
(callable_words.go:250's gradual-Any collection ambiguity is
DEFENSIVE-ONLY — every shipping Callable word carries CompileDynBody or
CrossCollectionTokenShape; pinned white-box in dynbody_unit_test.go.)

### 9.4 The Stage-2/3 raise-site tail

~50 further MarkUncompilable sites (clusters in emit.go 2294–4360 and
lower.go) carry neither a section nor an unreachability argument.
Several are PINNED reachable: "branch reads enclosing computation"
(lower.go:528), the fn-body residual family ("body leaves extra values"
/ "result is a variadic loop value" / "result above a literal",
lower.go:1677), "apply of a dynamic fn value not at the body tail"
(emit.go:3015), "anonymous function dispatch" (emit.go:4002). Where one
of these is subsumed by a §-mechanism (e.g. dynamic-apply-not-at-tail
under a generalised §1 body-window re-step), the subsumption must be
stated when that mechanism lands and the corresponding pins flipped;
where it is a designed keep (as lower.go:1108's consumed side-effect
loop already is, per §5), say so; the remainder need the audit.

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
   mode + the FnBinders dynamic-scope gate; see §6). §3 arrival-apply —
   **LANDED 2026-07-16** (tryMemberFnArrivalDispatch on the M2c chassis;
   see §3). §4 computed branch arms — **LANDED 2026-07-16**
   (computedArmDoBody synthesis; see §4). §1 drift window — **LANDED
   2026-07-16** (tryRecordDriftWindow — a STATIC OpCallDynamicMixed
   window, no marks needed; see §1). §7b multi-sig stamps — **LANDED
   2026-07-16** (per-sig refs; the matched sig's Impl IS the sig
   table; see §7). Every schedulable item of §1–§7 is now landed;
   only §5 (blocked on the check-mode forward-collection change) and
   the §8 designed opt-out remain open.

After all of §1–§7, **the enumerated refusal families are closed** —
the remaining interpreter execution on any default path would be:
check-mode (the compile front-end itself), module loads (attributed),
const-folds (attributed), explicit RunInterp, the §8 designed opt-outs,
and the **§9 inventory** (the L-DO part-2 residues, the seven
probe-verified shapes, the guard-owned declines, and whatever the §9.4
tail audit does not retire). The envelope is empty for every expressible
shape only once §9 is also worked off; at that point the
`AQL_COMPILE_FALLBACK` hatch plus the hatched legacy pins (49 at this
writing — the authoritative count is
`git grep -c 'Setenv("AQL_COMPILE_FALLBACK"' -- '*_test.go'`; each
landing shifts it) can be retired on schedule, after re-pointing the
stamp-suite pins per §9.2.

The external validation for all of this remains the voxgig-aql sweep
(steps 7–9 re-baseline) in a session sourced from that org.
