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

Refusal: "unmatched dispatch recovered at f". The rematch-trap machinery
(OpDispatchRematch, DispatchSpec.{NWritten,WrittenOff}) declines because
the recovery window holds a DEFERRED token (the flex Reach `m.a`, not
yet evaluated at record time) — the definiteness screen protects the
trap's "guaranteed no-match" claim.

**Mechanism: stop protecting a claim the island doesn't need.** The
re-step islands (OpCallDynamicMixed / FromMark) re-step VERBATIM tokens
through interpreter machinery — a Reach token in the window is simply
evaluated live (`m.a` → the flex cell) and the dispatch proceeds or
raises exactly as the interpreter. So: when the recovery window contains
deferred tokens, lower the site to a mixed re-step island over the
window instead of an OpDispatchRematch trap. The trap stays for fully
materialisable windows (cheaper: no re-step). Effort: S–M — the decline
arm in the tryRecordUnmatchedDispatchTrap gate becomes an island
election instead of a MarkUncompilable.

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

Refusal: "dynamic-scope def `xs` of unpromoted computed value" (the
OpBindGlobal write-back has no single value to peek: the loop's sim slot
is variadic). PROBED 2026-07-15: the interpreter binds xs to the
region's TOP value (xs = 1) and leaves the REST as residual
(`def xs (for 3 [1]) 99` → residual [1 1 99]) — the def consumes one
value off the collected group, not a list.

**Mechanism: peek-at-mark.** The loop collect is already mark-bounded at
runtime. Add a from-mark mode to the write-back: BIND_GLOBAL_FROM_MARK
peeks the region's TOP (stack[len-1] when the region is non-empty) and
writes it to the recorded slot — the same SetAt discipline, the same
peek-never-pop contract, one new op variant (or an Arg-encoded mode on
OpBindGlobal). The empty-region case is already refused upstream by the
zeroOut consumed-result gate (a 0-value loop result consumed by `def`
refuses today for its own divergence — that refusal stays). Effort: S.

## 6. Poly-decline arms (fn-predicate / gradual-Any overloads)

Two decline reasons in tryCompileUserPolyArms keep sites refusing:

- **Zero committed returns** (`len(committedReturns) == 0` — the
  zero-return overload set). Mechanism: make poly arms count-agnostic
  exactly as closures already are (compileClosureBody's `declared = nil`
  path: the unit RETs its actual residual and the caller takes it
  verbatim). The poly gate required identical declared returns across
  arms for downstream TYPING; a zero-return set has no downstream
  consumers to type, so `declared nil` is sound. Effort: S.
- **Body-local multi-overload fns** (the fn-baseline gate: the runtime
  Lookup cannot resolve a name popped before the VM runs). Mechanism:
  UserPolyRef already stores per-arm units AND impls — the runtime
  re-match (MatchFnSig) can run against the STORED sig table instead of
  a name Lookup, removing the resolve dependency entirely. The stored
  sigs are frozen at record time, which is exactly the interpreter's
  view for a body-local fn (its binding cannot change between
  construction and the call inside the same body). Effort: M.

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

1. §5 peek-at-mark (S) — flips the TestGlobalBindEnvelope variadic pin.
2. §6a zero-return poly arms (S) — flips the declining-poly pin.
3. §2 deferred-token island (S–M) — flips zzRefusingRow, which is the
   effect-fence pins' refusing fixture: those pins then need the NEXT
   off-corpus refusing shape (§1 or §3) as their fixture, or the §1/§3
   landings must come after re-pointing them.
4. §7a capture consts, §7c JIT re-stamp (S–M each).
5. §1 mark-bounded drift island, §3 arrival-apply, §4 dyn-body arms,
   §6b sig-table poly, §7b multi-sig stamps (M each).

After all of §1–§7, the only remaining interpreter execution on any
default path would be: check-mode (the compile front-end itself),
module loads (attributed), const-folds (attributed), explicit RunInterp,
and the §8 designed opt-outs — at which point the refusal envelope is
empty for every expressible shape and the `AQL_COMPILE_FALLBACK` hatch
plus the 51 hatched legacy pins can be retired on schedule.

The external validation for all of this remains the voxgig-aql sweep
(steps 7–9 re-baseline) in a session sourced from that org.
