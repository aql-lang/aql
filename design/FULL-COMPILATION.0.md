# Full compilation — the total lowering design

**Status:** proposal, no code. **Recorded:** 2026-08-25.
**Provenance:** the directive that closes the question
`design/COMPILE-DECLARATION-MODEL.0.md` left open: interpreter islands are
not acceptable, the interpreter is not an escape hatch, failure to compile
is a hard error, and compiled code must behave exactly as interpreted code
with the checker aligned with both. This note designs the compiler that
satisfies those four sentences.

> Authority: this is a proposal. `design/COMPILABLE-SUBSET.md` remains the
> statement of the current subset; `design/RUNTIME-INDEPENDENCE-COMPLETION-PLAN.0.md`
> remains the record of the doctrine this note extends. Where this note and
> the code disagree, the code wins. Code citations were verified against the
> tree at the recording date and drift as the tree does.

---

## 0. The claim, in one paragraph

The compiler becomes **total** by changing what its worst verdict is. Today
the recorder's worst verdict is *refuse* — `MarkUncompilable`, a one-way
latch (`compiler/go/emit.go:1366`), after which the interpreter owns the
whole program. Under this design the worst verdict is *lower generically*:
every dispatch the checker cannot resolve statically compiles into opcodes
that make the interpreter's own decisions at runtime — collection, signature
matching, application, name resolution — by calling the same kernel routines
the interpreter calls, over runtime values instead of tape tokens. Where the
checker has proof, today's typed lowering is kept unchanged. Nothing ever
re-enters `Engine.Run`; nothing ever re-steps a token; the only executors
are the VM and the Go handlers it calls. Parity is not an aspiration bolted
on afterwards: the generic lane is *defined* as "the interpreter's decision
procedure, compiled", sharing its code, so the two lanes can only diverge
where they fail to share — and the differential gates police exactly that
seam. This is the architecture Factor shipped when it retired its
interpreter, the one Chez Scheme and SBCL and Julia embody, and the one the
partial-evaluation literature says is forced: for a language with dynamic
semantics, the worst binding-time verdict is *dynamic*, and the answer to
dynamic is **residual code, never refusal**.

---

## 1. Terms of the mission — precise definitions

The four sentences of the directive, made checkable:

- **T1 — Totality.** For every source program the interpreter accepts,
  `CompileCheck` returns a `*Program`. `compile_refused` (Stage J,
  `lang/go/boru.go:1079-1099`) becomes an internal invariant violation, not
  a result. The `BORU_COMPILE_FALLBACK` hatch retires at the end state.
- **T2 — No islands.** At runtime, a compiled program never re-enters the
  interpreter's stepping machinery over tokens — neither recorded *source*
  tokens (`OpFallback`, `eng/go/vm.go:1166-1199`; drift-window
  `OpCallDynamicMixed`, `compiler/go/drift_window.go:31`) nor runtime
  *value* windows (`OpCallDynFrame`/RetReplay, `eng/go/vm.go:1071`;
  the `callDynamic` non-closure arms, `eng/go/vm.go:702-715`;
  raw-token bodies through `RunResolved`), and never bails to a
  whole-program interpreter re-run (`vmDefer`, `eng/go/vm_defer.go:16`).
  What remains allowed is what the runtime-independence doctrine already
  blesses (§3): kernel routines over data — matching, application, typed
  bind, name lookup — and **runtime compilation** producing Programs the VM
  runs.
- **T3 — Behavioral parity.** For every program, the compiled run and the
  interpreter produce the same observables: residual values, effects and
  their order, error identity — code, message text, blame position, and
  *which* of two possible raise texts (§6.2). The one standing exception
  stays the step-budget metering divergence (`design/COMPILABLE-SUBSET.md`
  §7), which needs a ruling either way (§11, O2).
- **T4 — Checker alignment.** The checker never blocks compilation (its
  worst answer is a dynamic carrier plus advisory diagnostics), its
  diagnostics are identical whether or not a program will be compiled, and
  every fact it feeds the typed lane is sound. Statically-definite errors
  compile into code that raises the identical error at the identical moment
  (the `OpTrap` discipline), so "the program is wrong" is expressed by the
  program raising, not by the toolchain refusing.

Non-goals, explicitly: no language-semantics changes in service of
compilation (Factor changed its language to get there — mandatory effect
declarations, `call(` — boru does not; §4.1); no serialized-artifact story
(blocked independently — a Program pins sub-registries by reference,
`design/STAGE3-INLINING-DESIGN-ROUND.0.md`); no speculative typed lowering
that would need deoptimization (§6.9).

---

## 2. Where the system actually is — the measured gap

Three measurements define the distance to T1/T2. The first two are known;
the third is this note's correction to the frame.

**2.1 The ledgered frontier is 153 rows, 127 refusing.** The
`lang/spec/frontier/*.tsv` ledger pins 127 interpreter-works/compiler-refuses
rows across 38 `failsWith` reasons plus 26 must-compile witnesses
(`test/go/langspec/frontier_spec_test.go:700-738`; counts measured from the
`frontierCompileLedger` map itself). Grouped by missing capability (family
letters used throughout this note; the A–D/I split below carries one fewer
row than the ledger — a one-row re-audit of that pool is owed):

| Family | Rows | Essence |
|---|---:|---|
| A. Computed-fn / closure provenance | 45 | fn values whose shape the checker cannot know: Church/SKI/CPS chains, `FnUtil` results, computed closures at argument slots, curried chains |
| B. Fn value as data operand (Stage 3) | 22 | `is`/`eq`/`deq`/`canon`/`for-each` receiving a fn value the gate assumes would be re-stepped |
| C. Multi-dynamic-result residual | 11 | two dynamic results live at once; the static seat model cannot address them |
| D. Provenance totalization | 13 | operands with no producing event: `$module`/namespace synthetics, cross-registry captures |
| E. Check-diagnostics sentinel | 8 | the checker rejects programs the interpreter runs (`lang/go/boru.go:459-472`) |
| F. Dispatch recovery | 5 | `unmatched dispatch recovered at apply/…` — recovered windows have no trap lowering yet |
| G. Working islands | 7 | compile and run correctly; ledgered only because the program embeds `OpFallback` |
| H. `while` | 6 | no structured lowering; body words splice tape-coupled tokens |
| I. Variadic no-static-seat | 5 | `await first/any` winner residuals: 0..N results exceed any static seat (NUR067) |
| J. Pinned miscompiles | 2 | bare `Function`-param read — both lanes wrong vs the 2026-08-15 ruling |
| K/L. Context layer; conditional fn shadow | 2 | `context` needs a frame the inline stream lacks; `installDef` overlap-removal defeats rollback |

**2.2 The ledger is a sample, not the universe.** The recorder holds ~96
`MarkUncompilable` sites plus lowering declines. Measured at the Stage-1
baseline (2026-08-25): **96 recorder call sites** (compiler/go 64, check/go
11, basic/go 10, core/go 7, lang/go 2, plus 2 test fixtures) and **78
lowerer/`Finalize` declines**, normalizing to **161 distinct reason
templates** — a third more than this note first estimated, and a majority
of them built at run time from a word name rather than written as literals.
The frontier ledger pins 38 `failsWith` substrings, of which only 33 are
source reason strings at all (the other five are harness verdicts) — so the
ledger exercises about a fifth of the surface. Whole gate
families have zero ledgered rows: `args`/`__pa` context words, anonymous
dispatch, quoted-operand words (`usurp`/force-arity/ref), `dynamic input` /
`unannotated or opaque word` (`compiler/go/emit.go:4880/:4889`),
`for:` multi-value bodies (`emit.go:4101`), mid-body dynamic apply
(`emit.go:3481`), multi-arg chained apply, splice/interp-string/XML
runtime-computed parts, the `mini` hooks, the typed-def construction family
(`basic/go/native_definition.go:801-1732`). And the recorder is not the
only latch: whole-program refusal also latches in `CompileCheck` itself —
`SuppressedRuntimeError` (a word deliberately lenient in check mode whose
runtime raise the pass could not model: an orphan `gen`, an `unpack` of a
missing key — `lang/go/boru.go:474-482`), `AmbiguousGradualSplit` (set
inside `Engine.MatchSignature`, `core/go/engine.go:8393-8413`; latched at
`boru.go:483-491`), and `Finalize`'s own post-recording declines
(`boru.go:503-506`; §5). Totality must be proven against the **whole gate
inventory**, with generated differential sweeps as the oracle (the
690-program sweep that found 24 divergences the ~30 hand-picked rows
missed — `design/HIGHER-ORDER-FUNCTIONS.0.md` §9g), not against the 153
rows.

**2.3 "Islands are at zero" is true only of `OpFallback`.** The live system
still re-enters the interpreter at runtime on the compiled path:

- `OpCallDynFrame` + RetReplay re-steps a runtime **value window** through a
  pooled Engine so `execFnDefLiteral`'s landing rule decides
  (`eng/go/vm.go:1071, 2326`) — sanctioned as "not an island" because the
  window holds values, not source, but it is `Engine.Run` mid-program.
- The drift window (`OpCallDynamicMixed`) steps a window **containing the
  word token itself** live, with interpreter forward collection
  (`compiler/go/drift_window.go:31`) — literal source re-stepping.
- The `callDynamic` family's non-closure arms island `[fn, args…]` windows
  (`eng/go/vm.go:702-715`); `OpCallDynamicTrailing` is hard-bounded to
  arity 1 (`vm.go:720`) precisely because the general case has no compiled
  form.
- Raw-token code bodies fall through `InvokeBody` to `RunResolved`
  (`core/go/invoke.go:21`; `eng/go/vm_defer.go:43`).
- Predicate and membership types run boru code **inside signature matching
  itself**: `v.Is(t)` on a predicate type routes through `PredicateUnifier`
  (which holds a `*Registry` — `core/go/unify_predicate.go:20-26`) to
  `Registry.RunPredicate` (`core/go/registry.go:1917`) → `CallBoru`, a
  sub-engine tape run for any unstamped predicate body — reachable from
  every "kernel" matching site, the VM's included.
- A callback arriving while the main registry is mid-run routes to the
  interpreter by *structure*, not analysis: `CanHostVM` declines
  (`core/go/registry.go:677-686`) and `InvokeCallback` falls to `CallBoru`
  (`core/go/invoke.go:49-52`).
- ~15 named `vmDefer` sites resolve runtime surprises by **re-running the
  whole program on the interpreter**, fenced by the C1 effect fence
  (`lang/go/boru.go:1264`) — dyn-scope misses, poly no-match/NOut drift,
  user-poly table drift, rematch-matched, shaped methods, splice-active,
  dyn-frame count mismatches.

T2 therefore has a second, unledgered debt: the **live island residue** and
the **defer valve**. This note treats both as first-class targets with their
own ratchets (§9), because a total compiler whose runtime quietly re-runs
programs on the interpreter has not left the status quo.

---

## 3. Why the two-lane shape is forced, and why it is already legal

**3.1 Refusal is not an available verdict.** Rice's theorem (Rice 1953,
*Trans. AMS* 74(2):358–366) makes every non-trivial semantic property of
boru programs undecidable — "this lead is always the same fn", "this slot
never sees a fn value", "this body's result count is fixed". A compiler
that must accept every program therefore cannot demand static resolution
everywhere. Partial-evaluation theory says what it must do instead: a
compiler is the interpreter specialized to the program (Futamura 1971,
*Systems, Computers, Controls* 2(5):45–50), and binding-time analysis is
**congruent** — its worst verdict is *dynamic*, whose answer is **residual
code that makes the decision at runtime**, never rejection (Jones, Gomard
& Sestoft, *Partial Evaluation and Automatic Program Generation*, 1993).
boru's recorder is a hand-written first Futamura projection whose dynamic
verdict currently aborts. The fix is not more static power — control-flow
analysis for higher-order programs is EXPTIME-complete at k≥1 and still
incomplete (Van Horn & Mairson, ICFP 2008) — the fix is giving the dynamic
verdict its residual code. That is the whole design.

**3.2 The doctrine already blesses the runtime kernel.** The
runtime-independence invariant C4 bans *interpreter execution*, not runtime
decisions: "no interpreter execution of any program the compiler accepts,
on any default path" with enumerated carve-outs
(`design/RUNTIME-INDEPENDENCE-COMPLETION-PLAN.0.md:123-135`), and its
stated method is "sound runtime re-dispatch — never static best-guess
baking" (`:16-19`). Kernel `MatchSignature` at VM time is shipped doctrine
(`OpCallNativePoly`, `eng/go/vm.go:485-590`; `OpCallUserPoly`, `:604`;
`OpDispatchRematch` re-matching "over the word's LIVE registry binding");
runtime name resolution is shipped (`OpLookupDynScope`/`OpBindDynScope`);
runtime **compilation** is shipped (`StampDetachedSig`,
`compiler/go/stamp_runtime.go:60`; the bounded JIT restamp, `:192`;
`Vm.run` compiling runtime-supplied source under fork isolation). And
`COMPILE-REFUSAL-SURVEY.0.md` already measured the corollary: after those
opcodes drove islands 102→0, **not one live refusal is a
dispatch-resolution problem**. What refuses today is everything *around*
dispatch: putting operands on the stack (collection), naming their homes
(provenance), typing their results, and applying fn values with the right
application model. §6 gives each of those its mechanism.

Two of the doctrine's own rulings this note must **supersede rather than
inherit**, and says so where it does. C4 records the fail-safe decline
seams ("decline → interpreter, never the reverse") as *permanent by
design* — §6.10 retires them anyway, a deliberate overturn T2 forces, with
a named compiled replacement per seam: the seams were permanent because
their only landing pad was the interpreter, and T2 removes the landing
pad. And C3's R3 defers registry-aware (predicate-faithful) matching in
compiled dispatch to "a separate later design, not assumed"
(`RUNTIME-INDEPENDENCE-COMPLETION-PLAN.0.md:109-117`) — §6.3's
predicate-unit inventory and §6.10's retirement row are that design.

**3.3 The precedents all have this shape.** Full compilation of a dynamic
language without an interpreter escape hatch has been shipped repeatedly,
always with the same three components — a *total baseline lowering*, a
*shared runtime kernel*, and a *resident compiler for runtime-supplied
code*:

- **Factor** is the closest precedent and the only dynamic concatenative
  language to reach total compilation: interpreter removed in 0.91 (2007);
  a non-optimizing compiler that lowers every quotation by a single
  left-to-right pass (totality is structural — every element has a
  template); an optimizing compiler whose refusal is *soft* (falls to the
  base compiler, never to an interpreter); one universal callable protocol
  (`call` generic over quotation/curried/composed — `curry`/`compose` are
  2-cell data, no codegen); eval as the resident compiler ("the parser
  always compiles code into quotations"); redefinition honored by
  compilation units + dependency-indexed repatching (Pestov et al., DLS
  2010, 43–56). Notably, pre-2008 Factor had boru's exact anti-pattern —
  redefinition decompiled dependents back to the interpreter — and the fix
  was making the cheap compiler total.
- **Chez Scheme / SBCL**: compile-only Lisps. `eval` invokes the compiler
  (Chez's `current-eval` is `compile`; SBCL "essentially a compiler-only
  implementation"); top-level redefinition honored from compiled code by
  one indirection — symbol value/fdefn cells (Dybvig, ICFP 2006, §3).
- **Julia**: the checker-gates-tiers model whole: Kleene-iterated inference
  whose ⊤ is `Any`; inference failure emits a *compiled* generic-dispatch
  call (`jl_apply_generic` — the same routine from every tier), never
  refusal, never interpretation (Bezanson et al., OOPSLA 2018). Its
  world-age rule is the caution, not the template: it *changed the
  language's* redefinition visibility to license inlining (Belyakova et
  al., OOPSLA 2020). boru's interpreter sees rebinds at the next dispatch,
  so boru compiles the live def-stack read instead (§6.5).
- **Baseline compilers** are the totality mechanism in the JIT world: a
  translator "derived very simply from an interpreter by having the
  interpreter save its action-routine code in a buffer rather than
  executing it" is total by construction (Deutsch & Schiffman, POPL 1984)
  — the modern forms compile bytecode 1:1 to code that calls shared
  builtins (V8 Sparkplug; JSC Baseline). Measured value of exactly this
  transformation: ~1.5–3× over interpretation (D&S: 0.686 vs 1.0; JSC:
  3.97→1.71 ns/op), bought purely by deleting fetch-decode, doing identical
  semantic work.
- **Self** supplies the missing piece for a no-interpreter world:
  deoptimization lands in *unoptimized compiled* code (Hölzle, Chambers &
  Ungar, PLDI 1992) — proof that no interpreter is needed as a landing pad.
  boru's analogue: every `vmDefer` site retires onto the generic lane
  (§6.10).
- The **counterexamples** transfer nothing: Stalin/MLton achieve totality
  by closed-world bans (no `eval`, no reload) boru cannot adopt;
  LuaJIT/HotSpot keep interpreters for native-code economics (startup,
  memory, deopt simplicity) that mostly vanish for an AOT-to-bytecode VM.

**3.4 Engagement with the prior note.** `COMPILE-DECLARATION-MODEL.0.md`
proposed two things. Its §4.1 declaration triple
(`tapeBound` / `needs` / `env`, constraints C1–C4) is *adopted* here as the
handler-contract vocabulary (§6.8) — the fn-util lesson stands: what a Go
handler does with an operand is an irreducible declaration. Its §4.2
(typed islands — give `OpFallback` a result contract, re-tier
`islandCeiling` into a budget) is **rejected by the directive**, and this
note argues it is also unnecessary: the seven working island rows it aimed
at are ordinary `each`/`fold`/`scan` calls that the universal fn-value lane
(§6.3–6.4) compiles natively, and the island cascade it aimed to soften
disappears when nothing islands. Its §4.3 warning — two agreeing lanes are
a bug detector, and collapsing them loses the gate — is answered in §8:
the interpreter remains the reference oracle in the harness (a sanctioned
carve-out), the lanes share *kernel routines* but not *drivers*, and the
differential corpus moves to generated sweeps, which is where its power
already was.

---

## 4. The architecture: one kernel, two lanes

```
                      ┌────────────────────────────────────────────┐
   source ──parse──►  │  check pass (carrier abstract interpreter)  │
                      │  = compiler front end (comptime; sanctioned)│
                      └──────────────┬─────────────────────────────┘
                                     │ per dispatch event
                     proof available │              no proof
                          ┌──────────▼───────┐   ┌──────▼───────────────┐
                          │   T-lane (typed) │   │  G-lane (generic)    │
                          │  today's lowering│   │  compiled decision   │
                          │  CALL_NATIVE,    │   │  procedure: collect, │
                          │  CALL_USER, folds│   │  match, apply, bind  │
                          └──────────┬───────┘   └──────┬───────────────┘
                                     └───────┬──────────┘
                                             ▼
                          one Program; one VM; one shared kernel
                     (MatchSignature, Apply, Collect, TypedBind, Lookup,
                      DefTable ops, error builders — the same functions
                      the interpreter calls)
```

- **The decision is per event, not per program.** Today one unprovable site
  latches the whole program to the interpreter. Under totality the recorder
  cascade (`recordDispatchOutcome`,
  `compiler/go/compiler_dispatch_record.go:53`) keeps its existing
  preference order — method apply, folds, closures, dyn-body, poly — and
  its terminal arm becomes *emit generic*, not `MarkUncompilable`. A
  program is a mosaic of T-lane and G-lane events; a G-lane event's
  successors return to the T-lane the moment the checker has proof again
  (guard narrowing already re-enters typed knowledge mid-block — the
  occurrence-typing pattern, Tobin-Hochstadt & Felleisen, ICFP 2010).
- **The lanes share kernel code literally, not by re-implementation.** The
  repo has already paid for this lesson twice: the `applyHandler`
  mark-not-call episode (a second dispatch path produced three lane
  divergences; the fix was removing the fork) and the stage-3 rule "a
  dispatch seam may not have two recording paths"
  (`design/STAGE3-INLINING-DESIGN-ROUND.0.md:162-164`). A generic opcode
  that *re-implements* matching or application would reintroduce drift one
  level down. The obligation is structural: G-lane opcodes call the same
  `core` functions the Engine calls (`MatchSignature`, `signature.go:175`;
  `SigTypeMatches` incl. the dynamic-carrier rule, `:297-341`;
  `CallBoruFn`/`InvokeCallbackFn`; `RunTypedBind`; `Registry.Lookup` +
  `aggregateDispatch`; the `RuntimeNoMatch` diagnostic builders), and the
  Engine is refactored to call the newly extracted routines (§6.2) so
  there is exactly one implementation of every decision.
- **No speculation, no deopt — which demands a sharper eligibility rule
  than "checker-proven".** A fact proven against a *mutable binding* is a
  bet under §6.5's live-rebinding semantics: the recorded `NOut` and Impl
  identity are claims a rebind can falsify (`eng/go/vm.go:575-585`,
  `:646-650`), and no site-local replacement can repair the
  **continuation** — the downstream T-lane code laid out for the stale
  result count — in an in-flight frame: a restamp serves *future* entries
  only, and a site-local re-dispatch executes the new arm but leaves the
  stale layout. So T-lane eligibility is *binding-stable* proof: facts
  rooted in sealed natives, module-sealed words, unit-locals, and
  structure the checker can prove no reachable rebind touches (the
  soundness classification, §6.9). A binding-*sensitive* dispatch — and
  the width-consuming region downstream of it — lowers to the
  G-lane/mark-region form, which is count-generic by construction and
  therefore valid across any rebind. Runtime staleness guards then never
  need an interpreter landing pad: future entries recompile (§6.7's
  restamp policy); in-flight frames were never speculated on. This is the
  Self stance minus speculation: the generic lane is the landing pad, and
  it is compiled code.

The formal frame for T4 (§6.9): the checker stays an *abstracted
interpreter* over carriers — the pattern of Van Horn & Might, "Abstracting
Abstract Machines" (ICFP 2010) — with ⊤ = dynamic carrier always a sound
answer (Cousot & Cousot, POPL 1977), so checker totality is free; typing is
**erased** (zero runtime footprint; Bracha, OOPSLA 2004 workshop), and lane
boundaries are Matthews–Findler *lump* boundaries — representation-
identical, conversion-free, check-free (POPL 2007). Every published cast
discipline (natural/transient/concrete) observably diverges from an erased
reference semantics (Chung et al., ECOOP 2018; Vitousek et al., POPL 2017),
so erasure is not a choice among equals: it is the only point compatible
with T3. What gradual-typing performance folklore indicts is guarded casts
(Takikawa et al., POPL 2016), which this design never introduces; the cost
model is Julia's — dynamic-dispatch cost on unproven sites, recovered by
caching (§7).

---

## 5. The total lowering algorithm (recorder side)

The per-event decision procedure that replaces refusal. `E` is the dispatch
event the check pass just resolved (or failed to resolve) over carriers.

```
LowerEvent(E):
  1  if E is a check-mode word (def/import/type/macro/Test):
       run at compile time as today (front-end carve-out, §3.2);
       emit binding TWINS for anything the runtime must re-see (§6.5):
       def → OpBindDef, undef → OpUndef, so VM-time def order is real.
  2  if E folds (const/scalar/module/full-stack under exactness proofs):
       fold as today (folds still record events — anchors stay).
  3  if E is statically DEFINITE-ERROR (checker proves the raise):
       emit OpTrap carrying the interpreter's full structured diagnostic
       (existing discipline: TryRecordUnmatchedDispatchTrap, PolyNoMatchSpec).
  4  if every operand resolves and the checker proved ONE signature:
       T-lane: CALL_NATIVE / CALL_USER / closure unit, as today.
  5  if operands resolve but the signature choice is runtime-only:
       T-lane poly: OpCallNativePoly / OpCallUserPoly, as today —
       EXTENDED to variable pop-windows via the statement descriptor (§6.2),
       closing the smaller-arity-overload unsoundness
       (compiler_dispatch_record.go:544).
  6  otherwise — the former MarkUncompilable arm:
       G-lane: emit the statement's compiled collection form (§6.2):
         PUSH operand descriptors in written order
         (literals as consts; paren groups as compiled sub-fragments;
          word references as name tokens; fn values as universal closures)
         then OpCollect+OpDispatchGeneric / OpApply per §6.2–6.4.
       Refusal is no longer reachable from this arm.
```

Structural constraints inherited from the recorder and kept: one recording
path per seam (strictly additive probes only); the probe-then-real closure
compile pattern and its drift hazards; `Rollback` cannot unwind compiled fn
units (`emit.go:3269`) — G-lane emission must be as latch-free as T-lane
recording. Step 6 is total by construction (every token class has a
descriptor form — the Deutsch–Schiffman argument).

**The same rule must bind the lowering phase, or the procedure is not
total.** Recording is only half the pipeline: `Finalize` and the lowerer
decline today *after* recording succeeded (`emit.go:7545-7592` —
`resolveDynamicApply`, residual operand resolution, mark-window
verification, `seatResults`; ~31 decline strings in `lower.go`, e.g.
`spillSeat`/`layoutOperands` at `:386`/`:1512` and the fragment-depth
ceiling at `:2041`; `stamp_runtime.go:139-153` records the
clean-probe-then-Finalize-declines class as production-reachable). And two
latch classes are not dispatch events at all: the fn-literal capture latch
during body recording (`emit.go:1830`) and the *retroactive* stored-handler
invalidation, where a later event invalidates an earlier bake
(`emit.go:2460`). Under totality every one of these becomes a **demotion,
never a refusal**: the recorder keeps a statement descriptor for *every*
statement — T-lane ones included — so a statement whose typed lowering
declines at Finalize re-lowers in descriptor (G-lane) form; a retroactive
latch demotes the earlier statement the same way (its descriptor is still
held); typed-def construction shapes take their §6.8 lowering. The
totality claim is therefore a **pipeline** property — "no refusal string
reachable from `Finalize`", not merely "no `MarkUncompilable` in the
recorder" — and §9's census is scoped accordingly.

The rest of §6 specifies the runtime mechanisms step 5–6 rely on, in
dependency order.

---

## 6. The mechanisms

### 6.1 What the interpreter actually decides at runtime — the split

Binding-time analysis of the Engine loop (`core/go/engine.go:1375-1490`,
`stepWord` `:2658`, `Engine.MatchSignature` `:8381`) yields a clean split.
**Static, per token, always:** the token *region* of every statement (its
hard syntactic delimiters — `end`, parens, the statement commit); dispatch
modifiers (`/v`, `/u`, `/q`); sugar/reach/paren lowering; macro expansion.
Barrier geometry and token word-vs-value class are static *per binding
set* — they follow the live signatures, so under rebinding they are
re-derived, not re-recorded (§6.2's revalidation rule).
**Runtime-irreducible:** (1) overload selection over unknown value types —
including predicate params that run boru code inside matching; (2) the
forward/stack **split itself** under dynamic operands (forward drift);
(3) zero/multi-value expression widths; (4) fn-value claims, arities, and
result counts; (5) def/undef and dynamic scope; (6) partial application
(`curryOrStack`'s curry lists, `engine.go:8159`); (7) runtime-supplied
code. The G-lane is exactly the compilation of (1)–(7); everything static
is decided at record time and baked into the descriptors below. This is
the compile-time/run-time line the whole design draws, and it is the
literature's line: the *domain* of every dispatch is static even when the
verdict is not.

### 6.2 The compiled collection machine — statement descriptors

The single deepest finding of the code review: signature matching is
already runtime-shared, but **argument collection is not**, and collection
is value-dependent. The pinned §9d case
(`test/go/langspec/frontier_spec_test.go:562-573`): a function-valued
argument reaching a leading collection must RAISE where a trailing-window
model would apply — and which of two raise texts fires is, in the ledger's
own words, "selected by collection state the window does not carry". The
ledger calls it "not repairable at run time". That verdict is about the
*window* model — an arity-N pop with no history. The repair is at compile
time: carry the history.

**Mechanism.** For every G-lane statement the recorder emits a **statement
descriptor** — a static table entry (a new `Program` side table, peer to
`Dispatches`) recording what the interpreter would have read off the tape:

```
StmtDesc {
  lead:      word name | apply | fn-value slot     // what dispatches
  slots:     []SlotDesc                            // written order
    SlotDesc { class: const | local | event | group(fragment idx)
               | wordRef(name) | typeNode(id)
               quote: none | /q | /v | /u          // static modifiers
             }
  barrier:   per-candidate-sig BarrierPos geometry
             // static only for a record-time-resolved word lead;
             // derived live for value leads and after rebinding (§6.5)
  seal/mods: dispatch-control facts the tape carried
}
```

and a matching opcode pair:

- `OpCollect(desc)` — runs the **extracted collection routine** over the
  descriptor: walks the slots in written order exactly as
  `resolveForwardArgs` + the arrival loop walk the tape
  (`engine.go:1876-2232`, `:3992-4062`), evaluating `group` slots by
  *calling their compiled fragments* only where a viable overload consumes
  the position (preserving the interpreter's conditional-evaluation order),
  applying `/q` Word→Atom in place, honoring the strict forward barrier
  (a bare function word acts as a forward-collection barrier —
  `fnWordBarrierAt`, `core/go/engine.go:2234-2242`), and producing either
  a claimed window + parked signature, or the interpreter's own stranded-
  forward / no-match raise — with the same *text selection*, because the
  state that selects the text is the collection machine's register state,
  fed from the descriptor. That state is wider than a pair of scalars:
  `strandedForwardError` (`core/go/engine.go:6878-6916`) draws on the
  forward's identity and blame position, the missing-count arithmetic, the
  paren-scope scan boundary, and a live `barrierReceiverWord` probe that
  can add a third text variant — or *commit* the forward instead
  (`:6825-6866`). All of it is register state of the one machine, which is
  why the mechanism is "extract the machine", never "enumerate the texts".
- `OpDispatchGeneric(desc)` — completes the statement:
  the word-policy gate first (the same `WordChecker` every named VM
  dispatch already consults — `vmContext.gateWord`, `lang/go/boru.go:418-427`
  — raising the identical permission error before any matching or entry),
  then live `Registry.Lookup` + `aggregateDispatch` on the lead (honoring
  rebinding, §6.5), kernel `MatchSignature` over the claimed window
  (variable window — the arity is an output of collection, not an input,
  closing the fixed-pop unsoundness), then handler call / unit entry /
  curry-list construction (`curryOrStack`'s compiled twin) / the
  interpreter's exact no-match raise.

**Live revalidation, because boundaries are binding-dependent.** The
forward scan's extent is derived from the *live* binding's signatures
(per-signature `forwardLimit` = `BarrierPos`), and even a token's
word-vs-value class can change when a name is rebound. A descriptor that
froze record-time classification would therefore collect wrongly after a
rebind (§6.5). So slots carry the underlying **token identity** alongside
their record-time class, the descriptor spans the statement's full
syntactic region (hard delimiters — `end`, close-paren, statement commit —
bound it regardless of bindings), and `OpCollect` re-derives class and
scan extent against the live binding set using the shared routine — the
same re-derivation the interpreter performs on every execution. Paren
groups stay pre-compiled fragments (their boundaries are syntactic and
cannot shift); what revalidation changes is only which slots a given live
signature set claims.

**Why this escapes the recorded "DO NOT RETRY".** Three attempts to
compile at the concrete-mismatch recovery site were differential-reverted
because "the recovery fires for reasons (forward-collection state, arity,
coercion) the param guard does not replicate"
(`design/VOXGIG-COMPILE-LEAVES.1.md:610-616`). Those attempts *committed a
bet* at compile time and guarded it with a check that lacked the
collection state; the descriptor mechanism exists to *carry* that state
and re-run the same routine — a replayed decision, not a guarded guess.
F2 is the falsifier if that distinction fails in practice.

**The extraction obligation.** The collection algorithm today reads *and
mutates* the tape (sugar expansion mid-scan, `engine.go:8731`;
`evalParenGroupAt` runs code in place). The routine must be factored to
operate over an abstract slot sequence — values, unevaluated fragments,
name tokens — with the Engine supplying a tape-backed adapter and the VM a
descriptor-backed one. This is the note's largest single work item and its
riskiest (§10 stages it first; §11 F1 falsifies on it). The prize is
double: the G-lane gains faithful collection, and the Engine's own loop
becomes a client of the shared routine — after which collection *cannot*
drift between lanes. The functional-derivation literature is explicit that
this factoring is where parity is cheapest (Ager, Biernacki, Danvy &
Midtgaard, BRICS RS-03-14, 2003): the VM's dynamic kernel should *be* the
interpreter's routines factored out, not a re-implementation.

Two consequences worth naming. First, the drift-window island
(`OpCallDynamicMixed`) and the mid-body/multi-arg chained-apply refusals
are all instances of "collection state missing at runtime" — they lower to
`OpCollect` forms and their islands are deleted. Second, `args`/`__pa` and
`context` (families K and the unledgered context-word gates) stop being
"context-dependent words": the descriptor's frame arms the existing DynEnv
args bracket (`eng/go/vm.go:450-483`) **per region** instead of
program-wide, and a context-frame opcode pair gives auto-evaluated bodies
their own layer exactly where the interpreter's sub-engine push did.

### 6.3 Universal fn values — open closure conversion

Every fn value the program can create carries, from creation, everything
application needs — so no apply site can lack a compiled target (Reynolds
1972's global-apply totality, realized as *open* closure conversion, not a
closed tag table, because runtime-compiled code must slot in; Minamide,
Morrisett & Harper, POPL 1996, is the formal warrant that one uniform
`∃env.(code × env)` convention covers every closure):

- **Executable ref per signature**: a `CompiledFn` unit, a native `Impl`,
  or a *lazily-compiled* body — creation stamps the body's tokens and
  defining registry; first application compiles via the detached-stamp path
  (`StampDetachedSig`) and patches, Factor's lazy-JIT-entry precedent. The
  dual-representation hook already exists (`SigImpl.BoruImpl.Compiled`,
  `core/go/sigimpl.go:56-61`).
- **Captures with homes**, including the currently-unsolved cross-registry
  case. Today a capture is a bare `{Name, Value}` snapshot
  (`CapturedBinding`, `core/go/value.go:882-885`; the capture rule is
  `ComputeCaptures`, `core/go/fn_capture.go:312`; closure operands resolve
  in `compiler/go/callable_words.go:470-480`). The *proposed* extension —
  nothing shipped does this yet — tags capture operands with their defining
  registry, completing the shipped halves (`CompiledFn.Reg` + `enterUnit`
  curReg swap, `compiler/go/bytecode.go:970`, `eng/go/vm.go:1403-1414`)
  and retiring the foreign-module island (family D's cross-module rows,
  the `filter A.big` working island).
- **Quote state and dispatch-control state** (`/q` polarity, sealed/applied
  bits) — so the quote-lambda screen (`check/go/check_fnbody.go:388`)
  becomes routing, not refusal: a `/q`-slot lambda delivered as a callback
  stays DATA, because the shared collection routine applies the same
  non-binding rule the runtime applies. The screen's *refusal* retires; its
  *knowledge* moves into the value.
- **Identity per the interpreter's semantics** — `eq` is identity token,
  `deq`/`canon` are content, module fn values box pointers (family B's
  entire premise is that these words only *read*; with a first-class
  compiled representation the Stage-3 gates delete). The byte-identity
  mandate makes the interpreter's current allocation/identity behavior the
  de-facto spec: compiled closures must reproduce it, and any future
  closure caching/sharing (Keep, Hearn & Dybvig, Scheme Workshop 2012)
  requires a Lua-5.2-style spec loosening *first* (§11, O3).
- **Partial applications as data**: `curryOrStack`'s curry list gets a
  compiled twin — a curried value is `(base fn, held args)`, applied by the
  shared apply; no per-composition codegen (Factor's `curried`/`composed`
  cells).
- **Predicate and membership-type bodies join the same inventory.** Today
  `v.Is(t)` on a predicate/DepScalar type runs the predicate's boru body
  through `Registry.RunPredicate` → `CallBoru` — a tape run *inside the
  matcher*, reachable from every matching site including the VM's
  (`core/go/unify_predicate.go:21`, `core/go/membership.go:3-6`). Under
  the universal representation a predicate body carries a lazily-stamped
  unit like any other fn value, and `RunPredicate` executes it through the
  VM invoker, mid-match raise semantics preserved. This is the
  registry-aware matching that doctrine item R3 deferred to "a separate
  later design, not assumed" — this section is that design (§3.2).

With this representation, family A's "unknown provenance / closure shape
unknown" dissolves for the *value* half: an unknown fn value is not a hole
in the trace, it is a runtime value with a universal calling convention.
The checker's carrier for it is the typed existential — dynamic as to
shape, concrete as to "applicable".

### 6.4 The Apply operation and the NUR101 application model

One dedicated, arity-carrying **Apply** kernel routine, seamed where
`execFnDefLiteral` sits today (`core/go/engine.go:5092`), used by both
lanes (the maintainer-ruled "dedicated Apply Op" — NUR077, `NUR.md:80`).
Its contract implements the application model that NUR101 established:

- **Name-read lead** (`def k (FnUtil.const 7)` … `k 99`, `(k 99)`): WORD
  dispatch — the runtime binding always applies. Lowering: `OpDispatchGeneric`
  through the live def stack (§6.5). Interp answer `7`; the compiled lane
  must produce `7` by *doing the same lookup*, not by baking the value.
- **Anonymous/event value in value position**: data unless applied by the
  placement rules; a named Go-impl fn value stays data. Lowering: push; the
  descriptor's placement facts decide.
- **Computed groups** (`(mk 1) 2`): the spec (ADR BROAD, `ADR.md:235-248`)
  says the group *places* its collapsed Function — and here the compiled
  lane is already the specified one; the **interpreter** is wrong
  (`NUR.md:285-286` — "fixing the placement rule fixes the divergence with
  it, and no compiler change is wanted"). Graduating family J (and parts of A) therefore requires
  interpreter fixes to the ruled semantics; the parity oracle for those
  rows is *the ruling*, not the current interpreter output. The doc-level
  consequence: T3's oracle is "the interpreter once NUR101/NUR078 land",
  and those NURs sequence *before* the affected G-lane admissions (§10).
- **Curried chains stage**: each application step is its own Apply event
  (the intermediate closure is a first-class value), eliminating
  "miscompile mechanism E" (`emit.go:7067-7076`) structurally.
- **Under-application and count mismatches** follow the interpreter's own
  rules (`(1 2 (mk 4))` → `1 6`; return-count trims and their exact error
  taxonomy) because Apply calls the same `CallBoru` return-enforcement
  helpers (`core/go/registry.go:1585-1752`) — with `CallBoru`'s internal
  tape run replaced by unit entry for compiled bodies (it is already
  tape-free at its *interface*; the inside migrates per §6.8). Fidelity
  cuts both ways: Apply reproduces `execFnDefLiteral`'s landing rule *as
  it is*, asymmetries included — the foreign-frame path is trim-only,
  count never enforced (the documented frame-path asymmetry,
  `eng/go/vm.go:2316-2344`) — so the replacement for RetReplay never
  raises where the interpreter trims.
- **Tail position is part of the contract.** The interpreter runs
  constant-space loops through dynamically-dispatched callees; a G-lane
  Apply that entered every unit by nested VM activation (`runUnitNested`,
  `eng/go/vm.go:397-410`, Go-stack re-entrant) would grow a frame per
  iteration — an unbounded-loop divergence in exactly the programs
  totality admits. The descriptor marks tail position; Apply in tail
  context enters units by frame replacement (the `OpTailCallUser`
  discipline) or by trampolining through the dispatch loop; a
  deep-tail-loop compiled-vs-interpreted gate joins §9's parity suite.

`RetReplay`, `OpCallDynFrame`, and the `callDynamic` island arms retire
onto this routine: the body-tail window becomes an ordinary Apply on a
universal value; the "runtime decides applicability" rule is Apply's first
branch (a non-callable value stays data — faithful either way, as
`OpCallDynamic` already is).

### 6.5 Generic word dispatch, rebinding, and the def twins

`OpDispatchGeneric`'s lookup half honors hot rebinding *by construction*:
it reads the same `DefTable` stacks the interpreter reads
(`core/go/registry.go:1095-1243`), at dispatch time. No world-age epoch, no
semantic change — boru's interpreter sees a rebind at the next dispatch,
so the compiled lane does too, because it performs the lookup. The
per-name `DefTable.Gen` dispatch cache (`registry.go:1070-1088`) is the
inline-cache seed (§7).

For this to be sound inside compiled programs, VM-time definition order
must be real: the **bind twins** already named in the recorder's interim
comments (`emit.go:2460/2474` — "until the §5.6 bind twins make VM-time
def order real"). Every check-mode `def`/`undef`/module install that
affects runtime-visible bindings emits a twin op so the VM's registry state
at instruction *i* equals the interpreter's tape state at the corresponding
token. This deletes the unledgered **rebind-staleness gates** — the frozen-read
refusals (`NoteFrozenRead`/`NotifyNameRebound`) and the interim
stored-handler latches (`emit.go:2460-2478`) — family L (conditional fn
shadow: the refusal site is `core/go/core_helpers.go:165`, and the
ledger's graduation note asks for exactly this — "a runtime dispatch
respecting the conditional binding",
`test/go/langspec/frontier_spec_test.go:206-207`), and the NUR037
fn-local-fn refusal (the name is looked up live, so the never-executed-def
hazard disappears). Family F — dispatch *recovery* — is §6.9(1)'s to
close, not the twins'.

Three consistency obligations come with the twins, and they interlock.

**Replay, never re-execution.** Today the compiled path deliberately
*keeps* the check pass's `RunInCheckMode` installs and runs the Program on
that registry (`lang/go/boru.go:1200-1202`); replaying the same
transitions through twins would double-apply them (duplicate `DefTable`
entries, so a later `undef` exposes the duplicate instead of the prior
binding). The twin regime therefore rolls the *runtime-visible binding
transitions* back to the pre-check snapshot before `RunProgram`, and each
twin re-installs the **identical binding object** the check pass produced
— the same `FnDefInfo`, the same module instance — at its source position;
nothing runs twice, and compile-time-only products (minted type IDs, macro
expansions, const folds) stay baked under the front-end carve-out. A
module import in particular executes **once**, in the front end; its twin
re-binds the already-produced instance, so module fn-value identity
(§6.3's pointer-based `eq`) has a single referent and a Program's pinned
sub-registries stay the only instance.

**Stamp freshness is taken against the post-replay world.** Dependency
snapshots recorded during the check pass (`DepSnap` over `(Depth, Gen)`,
`compiler/go/stamp_runtime.go:155-165`) would read stale against a
rolled-back-and-replayed `DefTable`; under the twin regime dep snapshots
are taken — or re-based — at twin-execution time, so the first invoke does
not mass-restamp.

**Effect order.** The check pass currently executes module imports,
effects included, before any VM instruction (`lang/go/boru.go:1017-1023` —
the C1 fence is armed before the check pass for exactly this reason),
which violates T3's ordering for a program with a runtime effect *before*
an effectful import. Under totality the front-end pass must be
**observationally silent** — the doctrine's O1 effect-freedom obligation
enforced rather than assumed — with every effectful half of a check-mode
word given a runtime twin at its source position. The one effect the
twins cannot carry is module-body execution itself: an effect-dependent
*export* must exist before the checker can type against it, so the body
runs in the front end. Until Stage 7's compile-module-bodies work moves
that execution to the twin position, import-time effect ordering remains
a **named front-end carve-out with a documented T3 exception** — which is
today's behavior, made explicit (O4). Hot code loading
then stops being "an interpreter-path feature"
(`design/HOT-CODE-LOADING.0.md:94-99`): a swap's cost is a G-lane lookup or
a JIT restamp — de-optimization, not decompilation.

### 6.6 Production-order results, mark regions, and provenance

Families C, D, I share one cause: the static seat model — every value must
have a compile-time address (event/const/local) — cannot address values
whose existence or count is runtime-only. The G-lane drops the requirement:
**within a G-lane region, the stack is the address**. Values live where the
statement machine put them, in production order; `planValueDefLocals`-style
promotion applies only at the region's typed borders.

- Family C (two dynamic results live at once, the NUR038 seal twins)
  dissolves: both results simply sit on the stack.
- Family I (0..N winner residuals, NUR067) gets the **generalized mark
  region**: `OpStackMark`/`OpDropToMark` exist in two narrow shapes; the
  general form is a mark with *no static count*, closed by ops that consume
  "everything since the mark" (variadic collect, splice, residual merge).
  This is the interpreter's own model — the tape region before the pointer
  *is* the value stack — finally given a compiled twin.
- Family D's synthetics (`$module`, namespace reads) become `wordRef`
  slots resolved by the same runtime lookup as any name; "operand of
  unknown provenance" is not an answerable question in the G-lane because
  it is never asked. Provenance remains the *T-lane's* discipline — where
  it is the soundness story for folds and seats and stays exactly as
  `COMPILABLE-SUBSET.md` §2 states it.

### 6.7 Runtime-supplied code: the resident compiler

Every no-interpreter system in the survey ships its compiler in the
runtime; islands are the only alternative ever tried, and they are banned.
boru is unusually well-positioned: **runtime compilation is already
shipped** — fork-isolated (`ForkConcurrent` + fresh `CheckState` +
`BeginCompilePass`), dependency-stamped (`DepSnap` over `(Depth, Gen)`),
invalidation-bounded (`RestampMaxTries=3`), and exercised by `Vm.run` and
the JIT restamp. The eng module already imports the compiler (the module
chain permits it — "compiler → eng" is dataflow, not import direction), so
the VM may invoke compilation without any layering change.

The G-lane extends this from "detached fn" to every runtime-code shape:
computed `do` bodies (the maintainer directive is on record: "`do` must
ALWAYS compile — natively … correctness via interpreter fallback is not
enough", `design/DO-STRUCTURE-COMPILATION.0.md:252-254`), computed splices
and interp-string/XML code parts, `mini` hook expansions, module bodies at
import (retiring the "module-load" attributed interpreter entry, or
re-declaring it as a front-end carve-out — §11, O4). Two rules keep it
sound and total:

- **Isolation**: never a mid-run `CompileCheck` on the executing registry
  (the shipped rule); compiled-at-runtime units enter by the same universal
  convention as AOT units.
- **Induction**: runtime compilation must itself never refuse — and
  "never refuse" is a property of the whole pipeline, not the recorder
  alone. The recursive call is to the *same* total procedure, so what the
  induction actually establishes is this: each compile of a finite body
  terminates and, given §5's demotion rule at `Finalize`, produces a
  runnable Program. A computed body is *not* structurally smaller than its
  producer — eval chains can construct larger bodies — so total compile
  work is bounded by **execution**, not program size, which matches
  interpreter cost semantics: the interpreter also pays per execution of
  computed code. Its preconditions are explicit and carried by F5: §5's
  Finalize totality, an answer for the front end's own ceilings (O5's
  check-budget exhaustion; the fragment-depth ceiling, `lower.go:2041`),
  and the eligibility/policy gates around detached stamping. Cost is the
  known trade — compile-on-eval is ~interpretation cost per op for
  run-once code (§7) — bounded by the **planned** Phase 6 JIT
  detached-unit cache: named as the graduation at `emit.go:6569`, scoped
  in `RUNTIME-INDEPENDENCE-COMPLETION-PLAN.0.md`'s Phase 6 (body identity
  keys on the structural `FnAnalysisKey` precedent, never `Value.ID`), and
  **not yet built** — an explicit Stage 7 dependency.
- **Staleness policy at the end state**: the bounded restamp
  (`RestampMaxTries = 3`) exists because its exhaustion arm lands on
  `CallBoru` — tape interpretation — fine under today's valve, banned
  under T2, and "G-lane re-dispatch" is no answer for a boru body (it
  selects the live arm; *executing* the arm still needs the unit the
  budget just refused). Under totality the restamp is **unbounded but
  memoised**, keyed structurally per `(body key, dep Gen)` via the unit
  cache: a rebind costs one compile per *world*, not per invoke, and the
  rebind-between-every-invoke pathology degrades to compile-per-invoke ≈
  interpreter cost (§7 prices it) — never to a wrong answer or an
  interpreter run. The budget deletes with the valve.
- **Registry mutation inside runtime-compiled code.** A computed body may
  itself contain `def`/`import`/`type`/macro use. The rule is §6.5's twin
  regime applied to the *executing* registry: the fork's check pass is
  observationally silent and its runtime-visible transitions twin into
  the unit, applied to the live registry at unit-run time in source
  order; compile-time identities the fork minted (types, macro
  expansions) are re-minted/reconciled onto the live registry under the
  same single-instance rule, with the interpreter's own behavior for the
  same eval-time definitions as the parity oracle. Macro bindings visible
  to the fork's expansion are the live registry's at fork time — a
  mid-run rebind after the fork is the same race the interpreter has.
- **Re-entrant hosting**: the busy-registry callback route (`CanHostVM`
  declining → `CallBoru`, `core/go/registry.go:677-686`,
  `core/go/invoke.go:49-52`) is a *structural* decline that no recorder
  totality touches; its retirement is nested VM hosting on the running
  registry — the mid-run `NestedRunner` seam already exists — named in
  §6.10's table.

One genuine pressure point from the survey: systems that refuse islands
also refuse *lexical* eval (Chez/SBCL/Hermes evaluate against top-level
environments only). boru's eval-class words that see local scope are
covered because the compiled lane can **reify the environment** — the
DynEnv machinery exists precisely to make frame bindings registry-visible —
armed per-region by the descriptor (§6.2), not program-wide.

### 6.8 Handler contracts: retiring the tape-coupled handler class

The VM refuses handler results that carry tape tokens (`screenResults`,
`eng/go/vm.go:217-223`, over the `tapeCoupled` predicate at `:171`) —
correctly: "return tokens for the engine to re-step"
is the one interpreter capability with no compiled meaning. Totality
requires that no reachable handler *needs* it. The instrument is the
adopted declaration triple (§3.4): every signature declares, per operand,
`tapeBound` (tri-state, unset = refuse at registration — constraint C1),
`needs` (a set: `FnDefInfo | RawTokens | CompiledUnit | Any`), and `env`
(`None | Captured | Live`). The migration:

- Handlers with `needs: CompiledUnit`-capable slots receive **units**
  (compiled at record time when the body is written down; at runtime via
  §6.7 when computed). `InvokeBody`'s raw-token fall-through arm retires;
  bodies run through `enterBodyUnit` under the VM invoker — the modelled
  callback frame the HOF audit names as the `InvokeBody` island's
  graduation criterion (`design/HIGHER-ORDER-FUNCTIONS.0.md:921`),
  generalized here from the list-Function rows to every code-body word.
- `env: Live` handlers (dyn-scope, `parselang`-class) get the per-region
  DynEnv arming.
- Structured-lowering words that still splice (`while` — family H) get
  structured lowerings (`WHILE_SETUP`/`WHILE_NEXT` on the `FOR_SETUP`
  precedent: per-iteration condition fragment, accumulating body values on
  a mark region, break/continue as jumps, truthiness via the kernel rule).
- Any handler that genuinely re-steps its operand on the tape
  (`tapeBound: Yes` — today's honest `var`-class answer) must be
  **rewritten to a structured or unit-taking form**. That is a finite,
  enumerable worklist (the triple's registration assert produces it), and
  it is the part of totality that is handler work rather than compiler
  work. Nothing in the survey suggests a shortcut exists.
- The **check-lenient words** behind the `SuppressedRuntimeError` latch
  (`lang/go/boru.go:474-482` — an orphan `gen`, an `unpack` of a missing
  key: lenient in check mode, strict at runtime) are their own per-word
  migration item, because neither of §6.9's mechanisms covers them as-is —
  the pass could not even *model* the runtime raise, so a trap has nothing
  proven to carry and the recorded stream was built on the lenient answer.
  Each such word either gets a strict check-mode twin (model the raise;
  then the trap/builder discipline applies) or a generic lowering of the
  affected statement. The latch's own census enumerates the list.

### 6.9 Checker totality and lane alignment

Four changes, all conservative in the abstract-interpretation frame:

1. **Delete the whole-program "check diagnostics" sentinel**
   (`lang/go/boru.go:459-472`). A statically-definite error compiles to
   code that raises the interpreter's error at the identical moment,
   catchably (family E's `FnUtil.flip 5` rows, where *the error is the
   specified result*) — in two forms, because diagnostic *content* is
   value-dependent even when failure is not: `OpTrap` with the baked
   structured diagnostic only where every operand is concrete, and the
   runtime error-builder over the live window otherwise. The trap path
   already declines carrier operands for exactly this reason — the rich
   diagnostic (received-argument note, per-candidate verdicts) must be
   built over the *concrete* runtime value or the lanes diverge
   (`core/go/engine.go:9182-9197`), and the runtime rematch is the shipped
   mechanism that builds it live. Could-match dynamics compile to G-lane
   dispatch that raises or succeeds as the runtime decides. Recovered
   windows (family F) become traps or G-lane rematch — `OpDispatchRematch`
   is the in-tree template, and §6.2's descriptor state is what
   distinguishes this from the three differential-reverted recovery-site
   attempts the "DO NOT RETRY" record pins. The fabricated `assume-sig`
   recovery windows, which corrupt the tape model, are replaced by honest
   G-lane regions, not admitted. With the sentinel gone, the C2 error
   oracle's check-error trigger is unreachable: C2 narrows to parse-error
   programs — which never reach either executor — and retires as a
   production path at Stage 8, surviving only inside the harness (§8).
2. **Classify every checker shortcut** as `advisory-only` vs
   `sound-for-lowering` (the soundiness rule — Livshits et al., CACM 2015).
   The quota/recursion bails are widenings and must widen only to
   interpreter-enforced facts or to dynamic (the precedent is already in
   the tree: the accuracy quota is disabled while compiling,
   `check/go/carrier.go:2751-2767`). Success typings are the published
   model of this stance (Lindahl & Sagonas, PPDP 2006); the difference —
   boru's typed lane *trusts* its facts — is exactly why the
   classification must be explicit.
3. **Collapse or prove diagnostic-neutral every `!Compiling` fork**
   (static-if reduction, loop spread, closure surfacing, `no_signature`
   emission itself, `check_recovery.go:1018,1126`). T4 requires one
   checker behavior regardless of the consumer. The open budget question —
   step-budget exhaustion during check has no emit-anyway story
   (`core/go/check_state.go:250`) — is O5 in §11.
4. **State the alignment property formally** so it can be tested: (a)
   *erasure* — the checker's **inferred** facts (carriers, joins,
   narrowings) have zero runtime footprint: discarding them changes
   nothing, in either lane, with `=` rather than "up to cast errors"
   (the degenerate gradual guarantee — Siek et al., SNAPL 2015).
   Deliberately out of scope: **source annotations**, which in boru are
   runtime dispatch inputs — a signature's declared types feed
   `MatchSignature`, and a typed def validates at bind time — so they are
   program semantics both lanes must honor identically, not checker
   metadata to erase; (b)
   *abstraction soundness* — every carrier fact over-approximates every
   run (both lanes — one execution semantics as far as the checker is
   concerned); (c) *lane coherence* — `obs(VM(compile p)) = obs(Engine p)`
   over the declared observable alphabet, checked per-program by the
   differential harness, which is translation validation in the Pnueli
   sense (TACAS 1998) with the interpreter as reference oracle.

### 6.10 Retiring the escape valves

The end-state runtime has **no interpreter landing pad**. The worklist is
the ~15 `vmDefer` sites plus the C1 fence, each with a named replacement:

| Escape site | Replacement |
|---|---|
| dyn-scope miss / dispatching / active-token | G-lane lookup raises the interpreter's own error |
| poly no-match (canonical error cases) | kernel no-match raise (`PolyNoMatchSpec` already proves window rebuildability — make it total via the descriptor) |
| NOut drift / user-poly table drift | never speculated on under §4's binding-stability rule — the site and its continuation are G-lane; future entries restamp (§6.7) |
| rematch-**matched** ("the doctrine's landing pad") | execute the matched arm natively — the R1 item the plan already names |
| shaped-method claims | `OpCallDynMethod` completes natively via Apply |
| RetReplay count mismatch | Apply reproduces `execFnDefLiteral`'s landing rule as-is — trim-only on the foreign-frame path (`eng/go/vm.go:2316-2344`), never a new enforcement the interpreter lacks |
| splice-active | generalized mark regions |
| callback `internal_error` degrade (`core/go/invoke.go:56-64`) | propagate — with no second lane there is nothing to degrade *to* |
| predicate body inside matching (`RunPredicate` → `CallBoru`) | lazily-stamped predicate units run via the VM invoker (§6.3) |
| busy-registry callback (`CanHostVM` decline → `CallBoru`) | re-entrant VM hosting on the running registry (the `NestedRunner` seam, §6.7) |
| JIT-declined / restamp-exhausted bodies → pooled sub-engine | unbounded memoised restamp (§6.7); the analysis-decline class dissolves with §5's Finalize totality |

This retirement list deliberately **overturns C4's recorded permanence**
of the fail-safe decline seams ("permanent by design",
`RUNTIME-INDEPENDENCE-COMPLETION-PLAN.0.md:123-135`): those seams were
permanent because their only landing pad was the interpreter, and
refusing to land there meant refusing the program. T2 removes the landing
pad; the table is the argument that each seam can land compiled instead.

The C1 effect fence exists to make re-runs safe; when there are no re-runs
it reduces to error propagation. `runtimeShouldFallback`
(`lang/go/boru.go:1298`) and the whole `RunCompiled`-vs-`RunCompiledStrict`
split collapse into one semantics — strict *is* the semantics.
`OpFallback`, its lowerer, `islandRun`, `runIslandResolved`, the drift
window, and the P7 machinery delete (the long-promised deletion), each
removal pinned by the engine-entry census (§9).

---

## 7. Performance

The G-lane must beat the interpreter for the design to be an upgrade rather
than a shuffle. The evidence says it will, for the classical reason:
per-op semantic work is identical, and the fetch/decode/collection-planning
overhead is paid once at compile time.

- Baseline-vs-interpreter, same semantic work: ~1.5–3× across four
  independent systems (Deutsch–Schiffman 1984: 1.46×; JSC baseline: 2.3×
  per-op; SpiderMonkey and Sparkplug corroborate at whole-workload scale).
  The G-lane is *better* placed than those baselines: collection planning
  (phase-1 overload pruning over the token window) is precomputed into the
  descriptor, whereas the interpreter re-derives it per execution.
- boru's own T-lane measures 21–48× over the interpreter
  (`design/PERF-BASELINE.10.md`, Stage 6). The mosaic program keeps that
  wherever the checker has proof; G-lane events pay live lookup + kernel
  match — the components the interpreter also pays.
- Caching, staged after correctness: the per-name `Gen`-keyed dispatch
  cache is the monomorphic inline cache; a PIC (Hölzle, Chambers & Ungar,
  ECOOP 1991) is the polymorphic extension. Factor's measured caution
  applies — its PIC bought only 5–10% at runtime (the big win was
  elsewhere) — so measure before building (§11, F4).
- The known costs, priced: per-region DynEnv arming (bounded by region, not
  program); lazy first-apply compilation (Factor's trade — compile cost
  ~interpretation cost for run-once bodies, amortized by the planned
  structural unit cache, §6.7); the rebind-per-invoke pathology — a
  dependency rebound between every invoke forces a compile per invoke,
  which memoised restamp floors at ≈ interpreter cost for the same
  program, the correct floor; alloc ceilings stay gated
  (`lang/go/bytecode_allocguard_test.go`) and the facade-inlining
  discipline holds.

The step-budget divergence (`COMPILABLE-SUBSET.md` §7) remains the one
place the lanes are observably different programs at the ceiling; O2 asks
for the ruling totality forces.

---

## 8. Two lanes as a bug detector — keeping the gate while closing the gap

The prior note's warning stands: NUR101 was *found* by lane disagreement,
with the interpreter wrong. Collapsing to one semantics would lose the
detector. This design keeps it, on three legs:

1. **The interpreter survives as the reference oracle** — a sanctioned
   carve-out (check-mode front end, the C2 error oracle, the differential
   harness), just no longer a runtime lane. `RunInterp` remains callable
   under the harness and the explicit opt-out.
2. **Shared kernel ≠ shared driver.** The Engine's tape walk and the VM's
   instruction walk remain independent drivers of the shared routines;
   drift can still arise in the drivers and descriptor construction, and
   that is precisely what the differential corpus exercises. What sharing
   removes is the *silent re-implementation* class of drift — the class
   that produced the applyHandler episode.
3. **The corpus shifts from hand-picked to generated**, which is where its
   power already was (the 690-program sweep; the property fuzzer). The
   frontier ledger's rows graduate into ordinary spec rows; the ledger
   mechanism itself remains for any future regression, and the EMI-style
   move (mutate dynamically-dead code; re-phrase the same binding across
   call forms) extends it (Le, Afshari & Su, PLDI 2014; McKeeman 1998).

---

## 9. Ratchets and gates — how progress is measured

New ratchets, alongside the existing ones (all monotone, all in-tree):

- **`engineEntryCeiling`** — runtime `Engine` entries observed during
  compiled corpus runs, built on the shipped interpreter-entry attribution
  seam (`ArmInterpEntryHook`, `core/go/interp_entry.go:78`, and the
  unattributed-entry assertions in `lang/go/frontier_cases_test.go:120-140`)
  extended from attribution to census — the gate the completion plan names
  `TestNoInterpreterExecution` is planned there, not yet in the tree.
  Start = current live residue (§2.3); end = 0 outside carve-outs. This is
  T2's number. **Measured 2026-08-25 and now in the tree**
  (`test/go/langspec/engine_entry_census_test.go`, riding the
  compile-or-fallback walk): **505** unattributed interpreter runs across
  7,568 corpus rows, by route `Engine.Run×505` (the ground truth — every
  tree-walk emits it exactly once) with `CallBoru×282`,
  `vm:island×63`, `runPooledSub×37`, `InvokeCallback:callboru×34`,
  `RunResolved×31`, `vm:island-resolved×21` naming how it was reached.
  The 84 VM islands are the point: `islandCeiling=0` passed the whole time,
  because it counts only `OpFallback` spans in a disassembly. The two island
  chokepoints had to be given their own seams to be nameable at all —
  `runIslandResolved` was a hand-rolled copy of `core.RunResolved` that had
  dropped its seam emit, so every island reported the generic `Engine.Run`.
- **`deferCeiling`** — `vmDefer` activations on the corpus; end = 0, then
  the mechanism deletes. Measured 2026-08-25: **5** — `vm:poly-nout-drift×3`,
  `vm:poly-no-match×2`, both named in §6.10's retirement table.
- **Refusal-site census** — the recorder's `MarkUncompilable` call sites,
  counted by source scan (`test/go/langspec/refusal_site_census_test.go`):
  **96** at the Stage-1 baseline. This is the STATIC half — machinery that
  exists rather than machinery that fired — so it keeps falling while the
  corpus's runtime refusal count sits at zero. The lowerer/`Finalize` and
  `CompileCheck` layers surface as reasons in the corpus census and as
  pinned frontier rows; end = no refusal string reachable from `Finalize`
  (§5), not merely an empty recorder.
- **Frontier ledger** — rows graduate by the stale-arm mechanism, never
  hand-edits; family counts per §2.1 are the per-stage scorecard.
- **Parity gates** — `make verify-bytecode` (byte-identical differential
  incl. error taxonomy), the property fuzzer, whole-suite
  `--compile`==`--no-compile`, executed censuses (compile-time censuses
  are blind), and hand-pinned off-corpus regressions with no-`FALLBACK`
  disassembly asserts — all existing, all kept green throughout; the
  server-concurrency corpus (§13) joins them as it lands.
- **The performance register** (§14) — the permanent, host-keyed record
  every stage writes its before/after measurements into.

---

## 10. Staging

Ordered by dependency; each stage is separately shippable, gated, and
reversible. Rows-moved counts are the ledgered minimum (the unmeasured
universe closes alongside).

| Stage | Work | Closes | Risk |
|---|---|---|---|
| **0** | Adopt the declaration triple (COMPILE-DECLARATION-MODEL Stages 0–2: delete the dead flag, introduce `{tapeBound, needs, env}` under C1–C4, assert over every signature) | 0 rows; produces the §6.8 handler worklist | low |
| **1** | Instrument: engine-entry census + defer census + refusal-reason census; declare the observable alphabet for T3 | 0 rows; makes T2 measurable | low |
| **2** | **Extract the collection kernel** (§6.2): factor `resolveForwardArgs`/arrival/`MatchSignature`-forward-scan over abstract slots; re-seat the Engine on it; prove no interpreter regression (perf + full differential) | 0 rows; unblocks everything | **high** — F1 |
| **3** | Universal fn values (§6.3, predicate units included) + the Apply kernel (§6.4, tail discipline included) + interpreter-side NUR101/NUR078 fixes to the ruled semantics; retire `OpCallDynFrame`/`callDynamic` islands onto Apply | A (45), B (22), J (2), and five of G's seven (the fn-value island rows; `filter A.big` lands with §6.3's registry-tagged captures here, the full-stack-in-body row with Stage 4's descriptor folds) | medium |
| **4** | Statement descriptors + `OpCollect`/`OpDispatchGeneric` (§6.2, §6.5) + bind twins; recorder step-6 flips from refuse to generic for word dispatch; delete drift-window islanding | F (5), L (1), K (1), most unledgered dispatch gates, §9d | medium |
| **5** | Production-order regions + generalized marks (§6.6) | C (11), D (13), I (5) | medium |
| **6** | Handler migration per the triple (§6.8): units-not-tokens, `while` lowering, per-region DynEnv, `args`/`__pa`/`context` frames | H (6), context/tape-bound gate families | medium — wide but enumerable |
| **7** | Runtime compilation everywhere (§6.7): computed bodies, splices, module bodies; the structural unit cache built here if not before (a hard dependency); unbounded memoised restamp; induction preconditions documented and fuzzed | eval-class gates | medium |
| **8** | Checker totality (§6.9): sentinel deletion, traps for definite errors, `!Compiling`-fork collapse, soundiness classification | E (8) | medium |
| **9** | Retire the valves (§6.10): defer sites → native answers; delete `OpFallback`/P7 machinery, the fence's re-run half, the fallback hatch; flip `CompileCheck` to total; `compile_refused` becomes a structured `internal_error` return (panics stay forbidden outside init-time registration) | T1, T2 complete | low by then |

The dependency spine is 2 → {3,4} → 5 → 9; stages 6–8 are parallel tracks
off it. Nothing lands without its differential gate; every stage's admission
sweep is generated, not hand-picked (§8.3). Until a statement's enabling
mechanism lands, step 6's arm remains **partial** for it: such statements
keep refusing, loudly — a Stage-4 G-lane statement whose runtime-only
result width feeds a static seat still declines until Stage 5's regions
land. T1 is a Stage-9 property, not a rolling one.

---

## 11. Open questions (O) and what would falsify this (F)

**O1 — NUR101/NUR078 interpreter fixes.** Stage 3 depends on interpreter
changes to ruled-but-unimplemented semantics. If those rulings are
re-litigated, the affected rows' oracle is undefined and Stage 3 stalls.

**O2 — the step budget.** Totality makes the per-instruction metering the
only live metering. Ruling needed: keep the documented one-directional
divergence against the oracle, or re-meter to match. Same for the
lane-divergent tape/stack ceilings the fn-value work measured.

**O3 — closure identity latitude.** If future optimization wants closure
caching/sharing, the spec must first grant Lua-5.2-style latitude;
until then the interpreter's allocation behavior binds.

**O4 — module-body execution at import.** Compile module bodies (Stage 7)
or declare import-time execution a front-end carve-out. This note leans
compile; the carve-out is coherent but must be stated.

**O5 — check-time budget exhaustion.** A program whose *check* pass
exhausts the budget has no emit-anyway story; totality needs one (likely:
widen-to-dynamic at the frontier of the exhausted region — but that must
be designed, not assumed).

**O6 — concurrency.** The runtime is one-registry-per-goroutine; detached
stamping runs under `ForkConcurrent`'s owning-goroutine contract
(`compiler/go/stamp_runtime.go:57-59`), and a `RestampBox` serialises
concurrent invokers of a shared signature across registries
(`:189-198`). Under "runtime compilation everywhere" plus
restamp-as-the-staleness-answer, the design owes rules for: registry
affinity of G-lane lookup and restamp under `spawn`/`await`, restamp-box
sharing when different callers' registries disagree, `DefTable.Gen` cache
reads as §7's inline-cache seed, and which `-race` gates each stage keeps
green.

**F1 — the collection kernel cannot be extracted.** If
`resolveForwardArgs`'s interleaving of evaluation with planning cannot be
factored over abstract slots without behavior change (the mid-scan sugar
expansion, `engine.go:8731`, is the hard case), §6.2 fails and with it the
faithful G-lane — the design would degrade to window dispatch, which §9d
proves unfaithful. This is why Stage 2 is first and why its gate is the
full differential on the *interpreter* side alone.

**F1b — the extracted routine slows the interpreter.** Stage 2's gate
includes interpreter performance, and unlike F4 there is no fallback: the
Engine re-seat cannot be reverted without abandoning the sharing
obligation §6.2 calls the parity foundation. If the abstraction layer
over the interpreter's hottest loop cannot match the tape-specialized
walk (mitigation to try first: a specialized, inlinable tape adapter),
the sharing obligation needs a weaker form — one implementation generated
into two specializations — and the drift-defense argument re-examination.

**F2 — error identity needs state outside the collection machine.** §6.2
claims the raise-selection state is exactly the machine's register state —
parked signature, claimed count and missing-count arithmetic, the
forward's identity and blame position, the paren-scope scan boundary, and
the commit-vs-strand outcome including the live `barrierReceiverWord`
probe (`core/go/engine.go:6825-6916`). If a case surfaces whose error
selection depends on state outside that register set — state the
descriptor cannot carry statically-bounded — the parity claim narrows and
the case needs a ruling (spec the error) or a descriptor extension.

**F3 — a `tapeBound: Yes` handler that cannot be rewritten.** If some
handler's semantics genuinely require re-stepping its operand on the live
tape (not a unit, not a reified env), T2 and totality collide on that word
and the directive itself must arbitrate. No such word is currently known;
`var` — the historical example — has a structured lowering.

**F4 — G-lane slower than the interpreter.** If a shipped Stage-4 generic
region measures slower than the tree-walker on like-for-like programs, the
premise "descriptor precomputation ≥ interpreter's re-derivation" is wrong
in this VM; the design still holds semantically but the performance story
reverts to T-lane coverage pressure. Benchmark at Stage 4, not at the end
— into the register (§14), so the verdict is host-keyed and durable.

**F5 — runtime compilation breaks the induction.** §6.7's induction rests
on named preconditions: §5's Finalize totality, an answer for O5's
check-budget exhaustion, the fragment-depth ceiling (`lower.go:2041`), and
the eligibility/policy gates around detached stamping. If any
runtime-compile path can still reach a shape that refuses — or if the
memoised-restamp policy cannot hold its cost floor — totality has a hole;
the fix is extending the G-lane (or the demotion rule) to that shape, and
the fuzzer must hunt for it explicitly.

---

## 12. Worked example — a callback server, statement by statement

The sharpest acceptance test for this design is the workload boru's own
benchmarks already care about: a Node-style callback server. The tree
contains its two poles today. `bench/networking/echo_boru.boru` — a
`Net.serve-raw` handler whose body the callback-compilation seam runs on
the VM — measures **~65,100 req/s compiled vs ~884 interpreted (~74×),
within ~1.7× of hand-written Go** (`bench/networking/README.md`). And
`design/examples/apps/mini-redis.boru` is a full protocol server (custom
codec, patrun-routed commands) with the same architecture. Both compile
**because they avoid the Node idioms**. Add the two most Node-shaped moves
— composed middleware and a `while` read loop — and today the *whole
program* refuses, landing every request back at ~884 req/s. That cliff is
the design's target.

The program (real surface syntax; `auth`/`route` bodies elided):

```
import "boru:net"
import "boru:fn-util"

def auth  fn line:String String [ … pass or reject … ]        # S1
def route fn line:String String [ … dispatch on the verb … ]  # S1
def handle (FnUtil.compose route/v auth/v)                    # S2

def ln
  (Net.serve-raw {tcp:8080}                                   # S3
      (fn sock:Socket Any                                     # S4
          [def nl (convert Bytes "\n")
            while [ … more input … ]                          # S5
              [def line (convert String (Net.recv-until sock nl))  # S6
                def reply (handle line)                       # S7
                Net.send-bytes (convert Bytes reply) sock     # S8
              ]
          ]
      ))
```

Statement by statement:

| Stmt | Interpreter semantics | Today | Under this design |
|---|---|---|---|
| S1 | typed fns; bodies checked, units built | compiles (T-lane `CALL_USER` units) | unchanged |
| S2 | `compose` receives two fn *values* (`/v`), stores them, returns a composed fn value — invoked later from Go, never on the tape | **refuses**: `function-valued operand at FnUtil.compose (Stage 3)` — the missing `CompileStoresFn`-class declaration, the fn-util defect | declaration triple says `tapeBound: No` (§6.8, Stage 0); the composed value is data over two units — Factor's `composed` cell (§6.3, Stage 3) |
| S3 | `serve-raw` stores the handler fn; the Go accept loop owns it | compiles; store-site callback stamping is shipped (`RuntimeStampingEnabled`) | unchanged, minus the `CallBoru` fallback arm |
| S4 | the connection callback: a closure, invoked per connection **after the program ends** | compiles; `InvokeCallback` → `RunUnit` — this seam *is* the measured ~74× | unchanged; the busy-registry decline arm retires (§6.7) |
| S5 | `while [cond] [body]`: per-iteration condition re-eval, body values accumulate, `break`/`continue` | **refuses**: `code-body word while (Stage 2)` — family H | structured `WHILE` lowering (§6.8, Stage 6) |
| S6, S8 | socket words, typed operands | compile (T-lane `CALL_NATIVE` — the echo bench proves it) | unchanged |
| S7 | `(handle line)`: a **name-read lead** — WORD dispatch; the live binding of `handle` applies, every request | **refuses**: `def-bound computed fn apply (closure shape unknown — Stage 1)` — the NUR101 wall; this is the Express-middleware idiom, 100% refused today | §6.4 Apply + §6.5 generic dispatch: live def-stack lookup, then unit entry — G-lane, cacheable (§7) — Stage 3/4 |
| S9 (hot swap) | a reload re-runs `def handle (FnUtil.compose route2/v auth/v)`; the next request sees v2 because S7 re-resolves the name | rebind gates refuse or the program was interpreted anyway | bind twins + live lookup make it native; in-flight connections finish on the old unit (frame-boundary cutover, §4), the next S7 dispatch sees v2; staleness costs one memoised restamp per world (§6.7) |

And the runtime path, end to end: the callback stamps at its store site
(shipped); steady-state dispatch invokes the unit with the registry idle
between events (shipped — the echo number); a handler that synchronously
triggers *another* callback hits the busy-registry route, which is the
re-entrant-hosting item (§6.7/§6.10) — the one genuinely new runtime
mechanism this program needs; a recv timeout raises the same
`[boru/timeout]` error with the same blame, propagated, never re-run
(§6.10); and the read loop runs constant-space under Apply's tail
discipline (§6.4). Under the design the program compiles **whole**: S7 is
the only G-lane event in it, paying one live lookup per request against
an otherwise fully typed lowering.

---

## 13. The server-concurrency test corpus

**The goal is that every callback case compiles — all of them, fully.**
Each case below is a graduation gate, not an aspiration: it passes only
when it runs with zero runtime `Engine` entries (the §9 census armed for
the whole run), byte-identical protocol transcripts against the
interpreter oracle, and green under `-race`. Echo and mini-redis prove
the *sequential* callback path; nothing in the tree pins the concurrent
and lifecycle cases — and the corpus's absence is already visible as CI
noise (`TestTuiServeAllViewersGoneQuits`, a viewer-lifecycle race, flaked
on this very PR's doc-only diff). Build the corpus on the existing apps
(`bench/networking/`: echo, `echo_redis`, `echo_s3`;
`design/examples/apps/`: mini-redis, mini-s3, todo-api), one case per
concurrency shape:

1. **Steady-state sequential dispatch** — echo (exists; the baseline).
2. **Protocol framing / codec re-entry** — mini-redis (exists); extend
   with partial frames (the codec's `{need:1}` path re-entered
   mid-message) and pipelined commands.
3. **Concurrent connections** — N simultaneous clients, handlers on
   multiple goroutines: the one-registry-per-goroutine rule under load;
   the O6 design item's test bed.
4. **Nested synchronous callback** — a handler that calls a service whose
   reply invokes another boru callback before the first returns: the
   busy-registry route (`CanHostVM` decline), i.e. §6.10's re-entrant
   hosting row, exercised deliberately.
5. **Handler re-entrancy** — a handler whose body applies itself (or its
   own composed chain) recursively.
6. **Hot swap under load** — rebind the routed handler mid-traffic:
   in-flight connections complete on the old unit, the next dispatch sees
   the new one, and restamp cost is one compile per world (§6.7) —
   asserted by counting compiles, not just answers.
7. **Fan-out** — `spawn`/`await` inside a handler; `await first`/`any`
   winner residuals (family I's mark regions) driving a reply.
8. **Deadlines and timers** — recv timeouts and timer callbacks: the
   `[boru/timeout]` error identity and blame position, compiled vs
   interpreted.
9. **Error paths** — a raising handler per connection: same error, same
   catchability, connection teardown identical, and no whole-program
   re-run behind it (the C1 fence's retirement made observable).
10. **Long-lived connection / soak** — hours-scale run: constant-space
    read loops (tail discipline, §6.4), no drift-triggered degrade to
    interpretation ever (`engineEntryCeiling` stays 0 for the whole
    soak), GC stability.
11. **Session/viewer lifecycle** — the TUI-serve class: all clients
    disconnecting, reconnecting, half-closed sockets — pinned
    deterministically, replacing today's timing-sensitive test.

Every case doubles as a **register workload** (§14): the corpus is both
the correctness gate and the performance instrument, so a stage that
graduates a case also records what that graduation cost or bought.

---

## 14. The performance register

The measurement discipline today is good at *relative* answers
(`benchstat` before/after, the alloc-ceiling gates in `make test` —
`design/PERF-BASELINE.10.md`) and bad at *longitudinal* ones: snapshots
live as prose in design notes, and `bench/networking/README.md` already
carries both warnings this section exists to fix — "absolute req/s track
the box", and a superseded row that had silently measured the wrong lane.
A ten-stage compiler rebuild needs a permanent, host-honest record.

**The register.** `bench/register/` holds two committed, append-only
JSONL files:

- `hosts.jsonl` — one record per distinct host: `host` (the id — a short
  stable hash over the normalized identity tuple: CPU model, physical
  core count, memory, OS name and major version, architecture,
  virtualization class), plus the full spec the id was derived from —
  CPU model string, cores/threads, RAM, storage class, OS and kernel
  versions, bare-metal/VM/container, CPU governor where known, and a
  free-text label.
- `measurements.jsonl` — one line per measurement:
  `{ts, commit, host, surface, workload, metric, value, unit, n,
  spread, benchtime, flags, go, os_version}`. `os_version` and `go` ride
  on every row because a host drifts under a stable id (patch levels,
  toolchains). `surface` is one of **`check`** (the static pass —
  `BenchmarkPerfCheck`), **`compile`** (emit + lower cost —
  `BenchmarkPerfCompile`), **`interp`** (interpreted execution),
  **`exec`** (compiled execution — the Stage-6 suite), and **`e2e`** (the
  §13 server workloads: req/s, µs/round-trip). All three implementation
  surfaces — interpreter, checker, compiler — are first-class, so a
  change that buys execution speed by spending check time is *visible*.

**Mechanics.** A `make bench-register` target runs the suites and the
§13 workloads, derives the host id, and appends rows; a verify step
asserts the files are append-only (existing lines byte-identical — the
kg digest discipline applied to measurements). The register **records,
never gates**: execution time is too noisy to fail CI on, so the
deterministic alloc ceilings remain the only perf *gates*, and the
register is the memory. A pinned CI runner class may contribute rows
under its own host id; developer boxes contribute under theirs.

**Reading it.** Absolute comparisons are valid only within one host id;
across hosts, only ratios travel (compiled/interpreted, boru/Go,
check-cost/exec-cost) — so rows record the absolutes and reports derive
the ratios. A small boru tool (`bench/register/report.boru`, in the kg
pipeline's dogfooding tradition) renders per-`(host, workload)` time
series over commits and flags a value outside the trailing window's
spread. Permanence is the point: rows are never edited or deleted — a
measurement discovered to be wrong is *superseded by a new row* naming
it, the same discipline the frontier ledger uses — so "did Stage 4's
descriptors slow the interpreter?" (F1b) and "what did Stage 3 buy on
mini-redis?" stay answerable years later, on the hosts that measured
them.

Every stage in §10 lands with register rows on at least one host: the
before/after pair is part of the stage's deliverable, and F4's Stage-4
benchmark is simply the first mandated pair.

---

## 15. Related work

**In-tree:** `COMPILABLE-SUBSET.md` (the subset this note totalizes);
`COMPILE-DECLARATION-MODEL.0.md` (the declaration triple, adopted; typed
islands, rejected); `COMPILE-REFUSAL-SURVEY.0.md` (dispatch opcodes
necessary but insufficient — the finding §6.2/§6.6 answer);
`RUNTIME-INDEPENDENCE-COMPLETION-PLAN.0.md` (the doctrine and the defer
worklist); `HIGHER-ORDER-FUNCTIONS.0.md` (§9d and the §9g generated-sweep law);
`FUNCTION-VALUE-SCOPE.0.md` (the env axis); `NUR.md` NUR101/NUR078
(NUR067 and NUR037 survive outside `NUR.md` — the ledger notes at
`test/go/langspec/frontier_spec_test.go:439-451` and
`design/HIGHER-ORDER-FUNCTIONS.0.md:1197`);
`DO-STRUCTURE-COMPILATION.0.md` (the "always compile" directive);
`HOT-CODE-LOADING.0.md`; `STAGE3-INLINING-DESIGN-ROUND.0.md` (one
recording path; the third architecture); `VOXGIG-COMPILE-LEAVES.1.md`
(the differential-reverted recovery-site attempts — the "DO NOT RETRY"
record §6.2 answers); `AOT-COMPILE.0.md` and
`INTERPRETER-TIERED-EXECUTION.0.md` (adjacent, unimplemented tiers).

**Systems:** Pestov, Ehrenberg & Groff, "Factor: A Dynamic Stack-based
Programming Language", DLS 2010, 43–56. Dybvig, "The Development of Chez
Scheme", ICFP 2006. MacLachlan, "The Python Compiler for CMU Common Lisp",
LFP 1992. Bezanson et al., "Julia: Dynamism and Performance Reconciled by
Design", PACMPL 2(OOPSLA):120, 2018. Belyakova et al., "World Age in
Julia", PACMPL 4(OOPSLA):207, 2020. V8 Sparkplug (v8.dev/blog/sparkplug);
JSC "Speedometer 2 and the JavaScriptCore baseline" (webkit.org/blog/10308);
Hermes (facebook/hermes). Siskind, Stalin; MLton
(mlton.org/WholeProgramOptimization) — the closed-world contrast.

**Theory:** Futamura 1971 (repr. HOSC 12(4):381–391, 1999). Jones, Gomard
& Sestoft 1993. Reynolds, "Definitional Interpreters for Higher-Order
Programming Languages", 1972 (repr. HOSC 11(4):363–397). Danvy & Nielsen,
"Defunctionalization at Work", PPDP 2001. Johnsson, FPCA 1985. Appel,
*Compiling with Continuations*, 1992; Shao & Appel, LFP 1994. Minamide,
Morrisett & Harper, "Typed Closure Conversion", POPL 1996. Shivers,
CMU-CS-91-145, 1991. Van Horn & Might, "Abstracting Abstract Machines",
ICFP 2010; Van Horn & Mairson, ICFP 2008. Deutsch & Schiffman, POPL 1984.
Chambers & Ungar, PLDI 1989. Hölzle, Chambers & Ungar, ECOOP 1991; PLDI
1992. Kleffner, MS thesis, Northeastern, 2017. Diggins, "Typing Functional
Stack-Based Languages", 2008. Pöial, EuroFORML 1990. Rice, Trans. AMS
74(2):358–366, 1953. Ager, Biernacki, Danvy & Midtgaard, BRICS RS-03-14 /
PPDP 2003. Leroy, POPL 2006. Pnueli, Siegel & Singerman, TACAS 1998;
Necula, PLDI 2000. McKeeman, DTJ 10(1), 1998; Yang et al. (Csmith), PLDI
2011; Le, Afshari & Su (EMI), PLDI 2014.

**Checker totality:** Cousot & Cousot, POPL 1977; PLILP 1992. Cartwright &
Fagan, PLDI 1991; Wright & Cartwright, TOPLAS 1997. Lindahl & Sagonas,
PPDP 2006. Siek & Taha, Scheme Workshop 2006; Siek et al., SNAPL 2015.
Wadler & Findler, ESOP 2009. Takikawa et al., POPL 2016. Vitousek, Swords
& Siek, POPL 2017. Chung et al., ECOOP 2018. Herman, Tomb & Flanagan,
HOSC 2010. Tobin-Hochstadt & Felleisen, ICFP 2010. Matthews & Findler,
POPL 2007. Bracha, OOPSLA 2004 workshop. Hackett & Guo, PLDI 2012.
Castagna et al. (Elixir set-theoretic gradual types), ⟨Programming⟩ 8(2),
2024. Livshits et al., "In Defense of Soundiness", CACM 58(2), 2015.
Darais et al., "Abstracting Definitional Interpreters", ICFP 2017.
