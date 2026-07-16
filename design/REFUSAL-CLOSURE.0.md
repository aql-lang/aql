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
lower the drifting word as a from-mark mixed re-step. Depth accounting
disappears because the island returns the WHOLE region's net (the
existing FromMark contract).

**Preconditions (load-bearing — the naive landing is unsound without
them):**

- **The island window extends from the mark to the STATEMENT END**, not
  the narrower "[region, forward-const]". Two reasons. (a) The island's
  net is runtime-variable, so any recorded dispatch AFTER the window was
  seated over the check pass's single-arm model, which the island's
  per-arm net contradicts — `5 do [7] error ["x"] add 1 mul 2` fires the
  same gate at `add` mid-statement, and a narrow window would mis-seat
  `mul 2`. Re-stepping through the statement end covers every downstream
  consumer of the region verbatim. (b) The gate inspects only ONE
  trailing literal (engine.go:3097), but a BarrierPos≥2 sig forward-
  collects further tokens at runtime; the full trailing span fixes that
  truncation. (The alternative — keep the narrow window but mark the
  island result `variadicResult` so the existing fixed-arity-consumer
  refusal holds downstream — is sound but leaves the mid-statement
  shapes refused; if chosen, qualify the closure claim accordingly.)
- **Mark-arming preconditions inherited from planMarkWindow /
  markWindowShape** (emit.go:6240–6294), restated: no marks already
  planned by the chained-variadic-if machinery (LIFO pairing — the
  TestChainedVariadicIfCompiles constraint); every window entry an
  unpromoted, live frames[0] event result. `refuseForwardStackDrift`
  also fires inside fn-body/branch FRAGMENTS, where the fragment
  lowerers never read markBefore — so either scope §1 to top-level drift
  sites (fragment-context drift keeps its refusal; e.g. the same shape
  inside an fn body or branch arm refuses today with parity [5 8]) or
  design a per-fragment mark-window extension as a separate item.

Cost: an island (interpreter machinery for one statement), not a native
lowering — but the program compiles, and the island is bounded to the
one statement. Effort: M. The negatives already pinned (mul/sub/String
tokens, no-leading-residual) stay native — the gate fires only where
refuseForwardStackDrift fires today. The graduation contract adds
mid-statement pins for both runtime arms (`… add 1 mul 2` → [5 16]; the
raise-arm sibling) alongside the statement-final pin before the
mustRefuseWithParity flip.

## 2. Deferred-token dispatch windows — `def f fn [[x:List][List][x]] def m (flex {a:1}) f m.a`

(The reproducer is the full `zzRefusingRow` fixture,
bytecode_effectfence_test.go:94 — `f` must be a defined fn for the
dispatch-recovery window to exist; without the preamble the source fails
as check diagnostics, not this refusal.)

Refusal: "unmatched dispatch recovered at f". The rematch-trap machinery
(OpDispatchRematch, DispatchSpec.{NWritten,WrittenOff}) declines because
the recovery window holds a DEFERRED token (the flex Reach `m.a`, not
yet evaluated at record time) — the definiteness screen protects the
trap's "guaranteed no-match" claim.

**Mechanism: an island election in the decline arm — with the trap's
terminality discipline kept.** The re-step islands re-step VERBATIM
tokens through interpreter machinery — a Reach token in the window is
evaluated live and the dispatch proceeds or raises exactly as the
interpreter. But the machinery being replaced is deliberately TERMINAL:
OpDispatchRematch, on a runtime MATCH, vmDefers to the interpreter
precisely because the static model was wrong — everything recorded after
the failed dispatch was seated over checkModeAssumeSig's synthesized
recovery carriers. An island that PROCEEDS past a runtime match would
push real results into compiled code the record never modeled. So the
election is sound under one of:

- **(a) terminal-slot semantics, mirroring dispatchRematch**: the island
  is elected only in the trap's own position (top-level, tail-truncated,
  the RecordTrap depth-1 restriction), and on a runtime MATCH it
  vmDefers exactly as the trap does (vm_rematch.go:25). This still
  graduates zzRefusingRow (a terminal error row) and flips the
  effect-fence fixture as planned.
- **(b) proceed-capable islands only under a downstream-consistency
  proof**: single real overload, the assumed sig provably equal to it,
  window equal to the recorded consumption, and declared return
  count/types equal to what the downstream record was seated over.
  Anything else keeps the refusal.

The landing must also carry a negative pin — a multi-overload fn over a
flex-cell Reach whose runtime value MATCHES, with a downstream statement
— asserting refusal or full parity, so the unsound general form cannot
land (today's terminal-only fixture cannot catch it).

**Scope note:** "unmatched dispatch recovered at `w`" has several
decline causes; this section designs the deferred-token one. The others
keep the refusal until designed: multi-overload user fns over an
Any/disjunct operand at the no-match recovery sites (engine.go:8120,
8166 — plausibly §6b's stored-sig-table re-match, cross-referenced
there); predicate/refinement-typed user-fn params, where the guarded
CALL_USER enforces only nominal types (engine.go:8230 — a candidate for
re-running the predicate via the existing RunTypedBind/OpBindTyped
machinery); and tryRecordPoly's own safety gates (meta / fn-value /
mutating / code-body / multi-result words).

The trap stays for fully materialisable windows (cheaper: no re-step).
Effort: S–M for the deferred-token island itself.

## 3. Member-fn auto-apply mid-expression — `m.double 21 eq 42`

Refusal: "member fn value auto-applies mid-expression". The interpreter
applies the parked fn the moment `21` arrives (a stepLiteral
ARRIVAL-loop event, not a word dispatch); the recorder only sees word
dispatches, so the downstream `eq` stole the operand in the record.

**Mechanism (revised): a modelled-dispatch rule on the carrier, not an
arrival recording.** The original text proposed recording "at the moment
execFnDefLiteral fires the parked fn" — but in the compile pass the
member read surfaces as a checker-typed dynamic(Any) carrier with
memberFnRead provenance (engine.go:3146, carrier.go:1049), NOT a
concrete FnDef, so **execFnDefLiteral never fires during recording**:
there is no arrival event to record. That is exactly why the recorder
"only sees word dispatches" today. The landing is instead a NEW modelled
rule in the tryShapedMethodDispatch / tryDynamicFnValueDispatch family:
when a memberFnRead-provenance carrier is at the pointer with arriving
operands, model the apply on the compile pass — **gated on a CONCRETE
receiver**, so the recorder can recover the member FnDefInfo and its
ARITY at the dot-read (readsFnMember already walks the container).
Arity is load-bearing: without it the lowering bakes an operand count
that a runtime arity-2 member (`m.add 1 2 …`) violates, mis-seating
every downstream operand.

Lowering: by recovered arity, the same split the statement-tail forms
already use (trailingApply vs trailingWindowApplyShape,
emit.go:6355–6395) — the simple case lowers to the EXISTING
**OpCallDynamic** (fn value below Arg trailing args — the CALL_DYNAMIC
family landed in P4; the statement-tail member apply disassembles to
exactly this op), multi-arg mid-expression to the OpCallDynamicMixed
window island. (**Not** OpCallDynamicTrailing — that is the OPPOSITE
source shape, the fn TRAILING its argument as in `5 m.f`, with a
rotate-fn-on-top non-callable residual and an Arg==1 bound.)

Effort: M — a new speculative checker rule plus arity gating, not just a
recording seam. Negatives: `m.x eq 5` (non-fn member reads) never enters
the path; NON-CONCRETE receivers keep the refusal, pinned alongside the
graduation flip; and the flip adds an arity-2 mid-expression pin
(`m.add 1 2 eq 3` → true).

## 4. Computed branch bodies — `if (n eq 0) [99] (range 2 4)`

Refusal: "computed branch arm is a spliced list body" (raised from
lang/go/native/native_control.go:694, prefixed "if: …"). The
interpreter's spliceArg EXECUTES a paren-arrived list as a code body;
the compiled value path would push the list as data.

**Mechanism: the dyn-body island, per arm — under three gates the
do-reuse actually needs.** tryRecordDynBody / CompileDynBody already
compile a DYNAMIC code body dispatch: the body's runtime sub-run reads
any name (dynEnv widens every def to registry-visible), and the VM
brackets the frame so `args` resolves. Lower the REACHABLE computed arm
as a dyn-body dispatch. But `do`'s dyn-body semantics are NOT `if`'s
spliceArg semantics; parity needs:

1. **A runtime twin of spliceArg's exact predicate** — splice-as-code
   only a PLAIN list (`Parent==TList && concrete && !IsTypedList &&
   !IsTableType`); typed lists, Tables, and scalars push as DATA. The
   split is decided by the RUNTIME value (range is statically
   [:Integer] yet returns a plain list), so `do`'s overload dispatch —
   which would route a runtime typed list to the List code-body sig and
   execute it — diverges from the interpreter's data push.
2. **Flow-control translation.** The interpreter splices `if` arms ON
   THE MAIN TAPE, so `break`/`continue` in a computed arm reaches the
   enclosing loop (verified: `def b (quote [break]) for 5 [if (i gte 2)
   (b) [i]]` → `0 1`). The arm island therefore needs escapedFlow
   propagation (resolveEscapedFlow after the apply, as the
   OpCallDynamic family already does) — the plain dyn-body CALL_NATIVE
   has none, and the concrete-body sentinel cannot screen a computed
   arm's unknowable tokens.
3. **Runtime-variable arm count.** A spliced arm nets N values (arm
   `[99]` = 1 vs spliced `(range 2 4)` = 2), so the branch merge and
   downstream seating ride the existing armOutVariadic →
   branchVariadicResult disposition; a fixed-arity downstream consumer
   keeps the refusal.

Dead arms (constant condition) and scalar arms keep today's native
lowering. Effort: M.

> **Filed during review — a pre-existing, on-by-default divergence in
> the adjacent seam:** `def b (quote [break]) for 5 [do b i]` —
> interpreter: empty (the break escapes do's sub-engine to the enclosing
> shared-tape loop); default compiled: five caught "flow signal with no
> enclosing loop" errors plus 0..4. tryRecordDynBody's sentinel
> rationale (carrier.go:1560) is false whenever the ENCLOSING loop is
> compiled. Fix independently of §4: translate escaped flow on the
> dyn-body CALL_NATIVE seam, or refuse computed dyn-bodies under a
> compiled enclosing loop; pin the shape in
> bytecode_edge_findings_test.go.

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
- **§6b: body-local multi-overload fns** (the fn-baseline gate: the
  runtime Lookup cannot resolve a name popped before the VM runs).
  Mechanism: UserPolyRef already stores per-arm units AND impls — the
  runtime re-match (MatchFnSig) can run against the STORED sig table
  instead of a name Lookup. **Corrected parity argument:** a body-local
  binding CAN change between construction and call (conditional
  redefinition `if c [def g fn-B]`, per-iteration re-defs) — the
  interpreter resolves the LIVE table at the call, so a frozen table
  needs a **stability gate**: either a static screen (refuse when any
  conditional/looped/intervening def/undef of the name occurs between
  the recorded construction and the dispatch) or a runtime guard in the
  existing style (snapshot the name's def-table generation at record,
  check depsFresh-style at dispatch, defer to the interpreter on
  mismatch — the role the live-Lookup drift guard plays today, which
  "remove the resolve dependency entirely" must not silently discard).
  The userPolyArmShapeOK screen and the owner gates (capture-free,
  named, non-macro) are retained: per-iteration reconstructions with
  differing captures must not share one frozen unit. Effort: M.

> **Filed during review — a live divergence in the single-overload
> analogue:** body-local conditional shadowing (`if c [def g fn …]`
> inside a body) compiles under force-compile printing 101/101 vs the
> interpreter's 101/2 — the CALL_USER path lacks exactly the
> generation-stability gate above. The §6b gate should cover it; worth
> pinning independently.

## 7. Per-callback stamp declines (not whole-program refusals)

- **Runtime-minted capture values** ("closure captures a runtime-minted
  value (no compile identity)"): the identity gate exists so the
  freshen/share machinery can't misclassify. For a DETACHED unit the
  capture is per-ref and frozen — bake it as an UNPOOLED const operand
  (internUnpooled) instead of resolving by ID; the ref's Captures slot
  path (landed today) already delivers per-value semantics, so the
  const is only the COMPILE-time home for body reads. Effort: S–M.
- **Multi-overload fn values**: stamp EVERY own sig to a unit and give
  CompiledFnRef a sig-table dispatch mode — MatchFnSig at invoke picks
  the unit, the user-poly re-match precedent applied to the callback
  seam. Effort: M.
- **Stale-dep refs degrade permanently to CallAQL**: add a JIT
  re-stamp — InvokeCallback, on depsFresh failure, re-runs
  StampDetachedFn against the live bindings (bounded retries; each
  re-stamp snapshots the new generations). This is also the mechanism
  for the plan's Phase-6 "JIT detached-unit cache" item (key: body
  identity + def-table generation). Effort: M.

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

1. §5 splice-at-mark — **RECLASSIFIED S → check-mode change, BLOCKED**
   (see §5: the checker binds the whole variadic region where the
   interpreter binds one value and spills N−1; the lowering half is
   prototyped and ready, but the sound landing needs a
   forward-collection change validated against the full corpus). The
   TestGlobalBindEnvelope variadic pin STAYS red.
2. §6a zero-return poly arms — **LANDED 2026-07-16** (`unitNetsZero`
   gate — every arm's body must provably net zero); the declining-poly
   pin flipped, re-pointed to the `zpick` fixture.
3. §2 deferred-token island (S–M, terminal-slot semantics) — flips
   zzRefusingRow, which is the effect-fence pins' refusing fixture:
   those pins then need the NEXT off-corpus refusing shape (§1 or §3)
   as their fixture, or the §1/§3 landings must come after re-pointing
   them. Adds the runtime-MATCH negative pin.
4. §7a capture consts, §7c JIT re-stamp (S–M each).
5. §1 mark-bounded drift island (statement-end window), §3
   modelled member apply (concrete-receiver gate), §4 dyn-body arms
   (three gates), §6b sig-table poly (stability gate), §7b multi-sig
   stamps (M each).

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
