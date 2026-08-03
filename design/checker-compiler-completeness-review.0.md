# Type checker + bytecode compiler — completeness review

> **STATUS (2026-08-03): the P0 track landed** — see §9 for the
> implementation record. §2's two divergences are CLOSED (sound refusals
> with parity pins), §3's opaque sentinel now names its blocking
> diagnostic, and the property fuzzer carries the §8.1(3) axes — which
> promptly surfaced a further pre-existing checker false positive, now
> ledgered expected-open (§9.4).

_Reviewed 2026-08-02 at `a727962`. Method: built the CLI from HEAD and
probed shapes directly (`do` under default / `-force-compile` /
`-no-compile`, `check -json`); ran the langspec gates
(`TestCompiledStatus`, `TestCheckAccuracyRatchet`, `TestCheckTypeSoundness`,
`TestCheckAnyFrontier`, `TestCheckRunFalsePositive` — all green on this
tree); audited the refusal raise-sites, the frontier ledgers, and the
status docs. Every divergence below was reproduced on the HEAD binary,
not inherited from a doc._

## 1. Verdict

Both components are **corpus-complete and gate-green**: 6996 spec value
rows, 6630 compilable, **all 6630 compile natively — 0 refusals, 0
interpreter islands — and `refusalGate`/`islandGate` are hard failures
now, not informational ratchets**. The checker pins stand at
`pinnedFalsePositives = 0` and `pinnedTypeSoundnessViolations = 0`
over the same corpus, with the Any-frontier ratio at 6.63% against the
7% ceiling.

The honest frontier lives off-corpus. Most of it refuses soundly
(falls back to the interpreter — slow, not wrong). But this review
found **two live compile≠interpret divergences** that violate the
compiler's one hard guarantee, both in the higher-order/fn-value
family — which is also the answer to "are HOFs and closures fully
compilable": *substantially, but not fully, and the remaining edge is
exactly where the two defects sit.*

## 2. Two live compile≠interpret divergences (new findings)

### 2.1 Chained forward application of Function params miscompiles

```
def compose fn [[f:Function g:Function x:Integer][Integer][f (g x)]]
compose ([a:Integer]=>[a mul 2]) ([b:Integer]=>[b add 3]) 4
```

- `boru check`: **clean** (0 diagnostics, result `Integer`).
- `-no-compile`: **14** (correct).
- default and `-force-compile`: **`[boru/type_error]: compose: expected
  1 return value(s), got 2`** — the program *compiles* (force-compile
  does not refuse) and the compiled RET raises the return-count error,
  i.e. the fn value was committed as data.

Same failure for `f (f x)` (self-composition) — any body whose tail
window holds **two pending forward applies** of Function-typed params.
The single-apply shapes are fine: `f x` at the tail, `f (f x)` split as
`def r (f x) f r`, and `apply1 fn [[fnv:Function][Integer][(fnv 100)]]`
all compile with parity.

Mechanism: the M2 negatives (`bytecode_fnvalue_m2_test.go`,
`TestApplyOverParamFnCompiles`) pin exactly this rule — "a pending
apply that is NOT the whole body-tail window must REFUSE (never compile
the fn+args as unapplied data, never drop an apply the interpreter
performed)" — but only for the **`apply`-word spelling**
(`v c1/r apply c2/r apply` → `fn <n>: apply of a dynamic fn value not
at the body tail`, emit.go:3392). The **forward-call spelling**
`f (g x)` slips past that net: nothing refuses, the recorder falls
through to the symmetric-RET-error assumption, and the interpreter
side is *not* symmetric — it applies both fns. This is the same latent
class the 2026-07-21 body-tail graduation
(`boru-bytecode-next-stages.0.md`) closed for the single-window case.

Disposition per the miscompile-hunt doctrine: the sound near-term fix
is a refusal (extend the pending-apply window check to count pending
*forward* applies of fn-typed locals, not just `apply`-word windows);
the capability fix is chaining `OpCallDynFrame` replays. Either way
this belongs in the `MISCOMPILE-HUNT-FINDINGS.0.md` lineage: it is a
loud wrong error, not a silent wrong value, but `compose` is about as
central as higher-order shapes get.

### 2.2 Atom-param lambda over a computed collection: the two engines
### disagree about whether it is a callback

```
def doc {meta: 7} each [[k:Atom]=>[doc get k]] (keys doc)
```

- `-no-compile`: `[fn (Atom)]` — the interpreter treats the Atom-param
  lambda as **data** (consistent with the literal-collection case
  below).
- default and `-force-compile`: the lambda is compiled as a **callback**
  and applied → `each: element 0: [boru/signature_error]: cannot call
  `` — no signature matches the arguments`.

Control cases: over a *literal* list (`['meta']`) both engines agree
(lambda stays data; the compiled path refuses `code-body word each
(Stage 2)` and falls back — parity preserved); with `k:Any` instead of
`k:Atom` both engines agree (`[7]`). So the trigger is precisely: a
**quote-typed (`Atom`) lambda param** + a **computed** collection —
`tryRecordLambdaClosure` admits the lambda under the callback
convention where the interpreter's runtime rule declines to apply it.
Both behaviours are defensible language-design-wise; the defect is that
they differ. The admission gate needs the same quote-polarity screen
the interpreter uses (or the shape must refuse).

## 3. Compile-pass-only diagnostics (sound, but a DX gap)

```
def mkadd fn [[k:Integer][Function][([y:Integer]=>[k add y])]]
def a (mkadd 4) a 10
```

runs correctly everywhere (14), but `-force-compile` refuses with the
opaque sentinel **`check diagnostics`** while `boru check -json` shows
**zero** diagnostics of any severity. The diagnostic that triggers the
`CompileCheck` gate (lang/go/boru.go:466 — any error-severity or
CaughtAtRuntime finding) exists only in the compile pass. Two costs:
`-force-compile` users get a reason that names nothing, and the
compile-pass checker demonstrably diverges from the plain pass —
`CHECK-FALSE-POSITIVES.0.md` §"compile pass" flagged this class as
unverified; this is a live instance. Worth either surfacing the actual
diagnostic text in the refusal reason, or ledgering the shape.

## 4. The refusal frontier (sound fallbacks — verified live)

All still refusing on HEAD, each with interpreter parity:

- **Full-stack words** `depth`/`pick`/`roll` → `residual value of
  unknown provenance` (designed-keep; the only class that needs a new
  event-producing lowering rather than a gate relaxation —
  `COMPILE-REFUSAL-SURVEY.0.md` §3a).
- **Gradual-Any arg to a multi-overload user fn** whose arms declare
  different returns (and the `Atom/q` quoted-slot variant) → `ambiguous
  dispatch, no poly re-match` (deliberate: the one-output record would
  lie for the unselected arm).
- **`do […] error […]` + trailing expression** → `handler nets no
  value` (deliberate; replaced a leaked `internal_error`).
- **Fn-value edges**: `{f:make42/r}.f` deferred-field auto-invoke →
  `fn value read from a container auto-dispatches (Stage 3)`;
  three-level curried chains `(((mk 1) 2) 3)` → `fn-value apply arity
  mismatch` (emit.go:7180); a closure as the *program* residual →
  `unconsumed fn-value carrier in residual (closure render)`; closures
  built by one `each` and applied by another → `code-body word each
  (Stage 2)` (this last one was miscompile-E's silent `[100]`-vs-`[105]`
  wrong value — now a sound refusal).
- The machine-enforced ledgers hold the rest: 26 frontier TSV rows in 5
  families (`frontier_spec_test.go` — NUR038 twin value-calls ×11,
  NUR031 `$module` provenance ×10, L-DO ×3, namespace capture ×1,
  conditional fn shadow ×1) plus one Go-level expected-red
  (`p6/check-prop-body-on-vm`). The 2026-07-17 raise-site audit
  (`REFUSAL-CLOSURE.0.md` §9.4) stands: of 71 sites, 5 subsumed, 23
  designed-keep, 16 defensive-only, **27 open** — the open cluster is
  the higher-order/fn-value-in-body family plus unknown-provenance
  tails.
- Runtime seams: 15 `vmDefer` bail sites, the `InvokeCallback` stamp
  declines (`restampMaxTries = 3`, then permanently interpreted), and
  the one deliberate semantic divergence — the step budget metering
  per-instruction vs per-token (`COMPILABLE-SUBSET.md` §7).

## 5. Are higher-order functions and closures fully compilable?

**No — but the compiled core is real and broad.** Closures are
first-class in the VM: bodies compile to their own units, captures
resolve to compiled homes and thread through `OpPushClosure` into
`ClosurePayload` slots, and invocation is VM-native (no sub-engine).
Compiling today, with parity: lambdas and token bodies for
`each`/`fold`/`scan`/`filter`/`walk`/`do` (captures included, computed
collections included), closures over loop variables, factories
returning capturing closures (two-level currying, `((mk 5) 10)`),
fn values passed as args / stored in containers with a single call
site, recursion through closure bodies, module-fn `Function` param
slots, `apply` over a param fn, body-tail dynamic apply
(`OpCallDynFrame`+`RetReplay`), and user-poly runtime re-match
(`OpCallUserPoly`) — the aless viewer's 110 runtime-constructed
callbacks all stamp to the VM.

Not compiling (sound refusals): mid-body dynamic apply, three-level
currying / partial-apply arity mismatches, container-fn auto-dispatch,
multi-overload lambdas, gradual collections, captures on the
extras/hook path, capturing handlers at strict store slots, `args`
inside closure bodies, code bodies naming fn-local fns (NUR037),
namespace captures at macro-expanded call sites, and `fn`/`afn`
construction over computed operands (designed-keep). And per §2, two
HOF shapes currently *miscompile* rather than refuse — that is the gap
to close first, because it is the only place the "slow, not wrong"
contract is broken.

## 6. Type checker completeness

The checker is the engine loop run over carriers, so dispatch, arity,
declared returns (type and count), dead overloads, typed-def
constraints, class/record construction, generics, guard narrowing,
case exhaustiveness, and the guaranteed-error mirrors are checked by
construction; every registered native signature carries `Returns`
annotations (the opt-out map is empty). The corpus pins: FP 0, type
soundness 0, 306/1121 known-error rows unflagged (value/state-dependent
by policy), Any-frontier 6.63%/7%.

Open items, most-significant first:

1. **`error [handler]` bodies are wholly unchecked** in the plain pass
   (`native_control.go` gates the seeded run to compile mode) — NUR049,
   Pending. `do [raise bad_input "x"] error [zzz-undefined]` checks
   clean.
2. **Mirrors fire only on the top-level straight line with concrete
   args** — the same `getr 99` that is flagged at top level is silent
   inside a fn body (carrier args erase concreteness).
3. **Any-frontier headroom is 0.37 points** — the next parse-dense
   corpus batch will trip `TestCheckAnyFrontier` and force either
   precision work or a ceiling debate.
4. **144 check-rejects-but-runs fuzz programs** — triaged as sanctioned
   (case exhaustiveness + dead-branch analysis), but under
   check-by-default they gate working programs; the aless dx-report §6
   false-error catalogue is the field report of the same friction (its
   specific shapes — is-narrowing, fold accumulators — now check clean;
   re-verified).
5. **Compile-pass-only diagnostics** (§3) — the two passes can disagree
   with no user-visible explanation.

## 7. Documentation drift (the status docs lag the code)

- `DESIGN-DX-AND-BYTECODE-STATUS-REVIEW.md` still says the ratchets are
  "informational" with 19 live refusals — they are hard gates at 0.
- `checker-inaccuracy-catalog.0.md` / `checker-comprehensive-review.0.md`
  / `CHECKER-BYTECODE-COMPLETION-PLAN.0.md` carry pre-fix numbers (7
  soundness violations, FN-1…FP-1 open) — all resolved or absorbed.
- `COMPILABLE-SUBSET.md` §5 lists classes the survey shows compiling
  (`COMPILE-REFUSAL-SURVEY.0.md` §4) — its own "the code wins" rule
  applies.
- Stale code comments: `bytecode.go` `OpPushClosure` ("capture-free
  bodies only for now" — captures compile today);
  `interp_entry.go` lists a `vm:shaped-method` bail site that no longer
  exists.

## 8. Recommendations — the path to full compilation

### 8.1 Restore the parity contract first (P0)

1. **Refuse the chained forward-apply window.** In the fn-unit finish,
   count the pending dynamic applies in the residual window: more than
   one fn-typed local/event awaiting application — or any apply whose
   consumer is itself inside the window (the `f (g x)` nesting) —
   must `MarkUncompilable`, exactly as the `apply`-word spelling
   already does (emit.go:3392). Pin both spellings side by side, with
   the `def r (g x) f r` split as the keeps-compiling control, and add
   ledgered frontier rows so the later graduation is measured.
2. **Screen lambda-callback admission by quote polarity.**
   `tryRecordLambdaClosure` must decline a lambda whose param pattern
   is quote-typed (`Atom`, the `/q` family) unless the interpreter's
   own application rule would fire — cheapest is to consult the same
   predicate the interpreter uses (the `sigTypeMatches` single-funnel
   precedent). Pin the literal-vs-computed collection pair.
3. **Widen the property fuzzer's generator axes.** Both defects lived
   in generator blind spots: it spells apply/usurp and closures, but
   not *chained* fn-param application inside fn bodies, nor quote-typed
   lambda params over computed collections. Add axes: apply spelling
   (forward vs `apply` vs `/r`), apply depth (1–3), lambda param
   polarity, collection provenance (literal / computed / gradual).
   This is the highest-leverage single change — every divergence this
   review and the miscompile hunt found was off-corpus, and the
   differential gates are blind by construction to shapes the
   generator cannot spell.
4. **Name the diagnostic in the `check diagnostics` refusal.**
   `CompileCheck` holds the diagnostics list at the refusal site
   (boru.go:466); format the first code+position into the reason. It is
   the only refusal string that currently names nothing.

### 8.2 Convert the HOF refusals into capability (P1, by leverage ÷ risk)

1. **Stage G proper — general dynamic apply inside fn units.** The one
   mechanism that natively compiles `compose`, mid-body applies, and
   curried chains: route the fn-unit body residual through
   `resolveDynamicApply` (today wired only into the program-residual
   `Finalize`) and extend `trailingApply` to accept a
   `resolveOperand`-able fn (event OR frame local), lowering chained
   applies to consecutive `OpCallDynamic`s in token order. This also
   retires the §8.1(1) refusal. Known two-coordinated-lowering-changes
   project (`boru-bytecode-next-stages.0.md`); high soundness risk —
   land it *behind* the §8.1(3) fuzzer widening.
2. **Full-stack words** — an event-producing lowering
   (`OpDepth`/`OpPick`/`OpRoll` minting runtime provenance), per the
   survey's ranking: the only class that adds capability rather than
   relaxing a soundness gate.
3. **Multi-overload user fn over gradual-Any with differing arm
   returns** — record the join of the arms' returns and dispatch via
   the existing `OpCallUserPoly` re-match; census before/after, since
   the dynamic output may re-refuse downstream. The `Atom/q`-slot
   variant falls out if the re-match window learns to bind quoted
   values.
4. **Container-fn auto-dispatch** — a member-read op carrying the
   interpreter's own 0-arg-satisfiable auto-invoke rule.
5. **The ledgered families** (NUR038 twin value-calls, NUR031
   `$module` provenance, L-DO variadic regions, namespace capture) —
   ledger-first, one mechanism per family.
6. **`do…error` island arity** — lift the single-output island model
   with the existing mark/variadic-region machinery
   (`OpStackMark`/`OpDropToMark`) so a handler netting no value
   threads correctly.

### 8.3 Define the finish line honestly (P2)

Literal 100% is not the target and should not be: compile-time words
execute during check by design, the step-budget metering divergence is
deliberate, and the designed-keeps exist because compiling them would
change meaning. Recommend stating the finish line as: (a) **zero
divergences** — differential + fuzzer green over a generator that can
spell every language construct; (b) every runtime-reachable construct
either **compiles natively or carries a designed-keep entry** with
rationale (the REFUSAL-CLOSURE audit format), driving the 27 open
raise-sites to zero *open*; (c) every refusal **loud and
self-explanatory**. Under that definition, Stage G kills the
fn-value-in-body cluster, branch/loop residual modeling kills the
unknown-provenance tails, and the recovery/poly funnels go last.

### 8.4 Checker (parallel track)

1. **NUR049** — check `error [handler]` bodies in the plain pass
   (unify with the compile-mode seeded run).
2. **One diagnostic surface for both passes** — no diagnostic should
   exist that `CompileCheck` can see and `check -json` cannot.
3. **Any-frontier headroom** — spend the 0.37 points deliberately:
   precision work on the top feeders (module-io, module-sift) or an
   argued ceiling bump, before a corpus batch trips the gate.
4. **In-body mirrors** — bounded concrete re-analysis at monomorphic
   all-concrete call sites, mirrors-only, so the dynamic-help FP class
   that killed concrete-example-arg analysis stays dead.

### 8.5 Docs

Refresh or header-annotate the stale status docs (§7) and fix the two
stale code comments. Cheap, and they actively mislead — this review
initially chased the "19 refusals / informational ratchets" claim.

## 9. Implementation record (2026-08-03)

### 9.1 §2.1 CLOSED — chained forward apply refuses

`noteDynFrameReplay` (emit.go) arms the whole-frame replay only when the
window holds exactly ONE applicable value (`replayApplicables`): the flat
re-push loses the source's paren structure, so a two-applicable window —
`compose`'s `f (g x)`, `f (f x)` — compiled the RET count error where the
interpreter applies both fns. The chained shapes now refuse (`unapplied
fn-value in body residual`) with faithful fallback: default mode returns
14/7. Pins: `eng/go/emit_dynapply_fnunit_test.go` (white-box decline
arms), `lang/go/bytecode_chained_apply_test.go` (both spellings + the
still-compiling single-apply controls), `lang/spec/frontier/
frontier-chained-apply.tsv` + its `frontierCompileLedger` entries
(graduation = Stage G). The single-applicable stylesheet shapes and the
`apply`-word spelling keep compiling — full suite + census green.

### 9.2 §2.2 CLOSED — quote-polarity screen on lambda callbacks

An Atom-typed lambda param is a /q quote-capture slot the runtime never
binds from a delivered value, so the interpreter leaves such a lambda as
data in the callback position. Two screens restore parity:
`lambdaHookCompatible`'s quote arm (lambda-value bodies) and
`quoteParamCarrierBind` consulted at the user-fn dispatch record inside
closure units only (token bodies) — `each`/`fold` over `(keys m)` now
refuse to the interpreter's own result, and the `filter` sibling
compiles with the identical error. Top-level /q no-match parity was
already exact and is untouched. Pins:
`eng/go/quote_lambda_screen_test.go`,
`lang/go/bytecode_quote_lambda_test.go`.

### 9.3 §3 / §8.1(4) CLOSED — the sentinel names its diagnostic

`RunCompiledStrict` appends the first blocking diagnostic to the
`check diagnostics` refusal (`checkDiagnosticsDetail`, lang/go/boru.go)
— `force-compile: check diagnostics: [undefined_word] undefined word: a`
— using the same predicate the `CompileCheck` gate refused on. The
sentinel string itself is unchanged (the fallback classifier compares it
by equality), as is the `Vm.compile` programmatic mirror. Pin:
`TestRunCompiledStrict/check-diagnostics refusal names the blocking
diagnostic`.

### 9.4 §8.1(3) LANDED — fuzzer axes; a new FP surfaced and ledgered

`genHofProg` (compiled_property_test.go) adds the four axes: apply
spelling (forward / `apply`-word / `/r`), apply depth 1–3, lambda param
quote polarity, collection provenance (literal / computed / gradual).
A cranked run (8 seeds × 3000; 16,312 compiled programs) reports **0
divergences**. The axes immediately surfaced a pre-existing PLAIN-pass
checker false positive the old grammar could not spell: `def zr (f x)`
— a def bound to a Function-param application — flags
`undefined_word: zr` on a clean-running program (the strict Function
carrier's un-collapsed group residual stalls the pending def; the
`tryDynamicFnValueDispatch` collapse admits only DYNAMIC bounds and its
evaluation-fixed window scanner rejects param reads — a first widening
attempt confirmed both facts and was reverted as out-of-scope for a
bounded landing). Triage: `pinnedCheckRunDivergent` 144 → 218 with the
class recorded (`check_run_fp_test.go`), expected-open ledger pin
`lang/go/check_fn_param_apply_def_fp_test.go`, and the
`frontier-chained-apply.tsv` def-split row keyed to it. Graduation =
the strict-Function-carrier collapse on the plain-check surface.

### 9.5 §8.4.1 CLOSED — NUR049 resolved (handler bodies checked)

`errorReturnsFn`'s seeded handler-body run covers the PLAIN pass
(native_control.go, un-gated 2026-08-03): a handler typo flags
`undefined_word` statically, the sealed-group `(dot message)` idiom is
PROVEN `no_signature` at check time, the working idioms stay clean, and
the plain-pass `error` bound narrows to the compile pass's join
(`do [7] error [drop 9]` → dynamic(Integer)). Corpus-safe by
construction — every corpus row already passed this analysis in the
compile pass. The record's full fix scope landed: the misleading help
text now offers the sequential spelling for barrier-receiver boundary
words (`strandedForwardError` + `forwardParensSuggestion`,
`barrierReceiverWord`), and both shipped examples are repaired
(`todo-tui-client.boru` ×4 arms; `todo/audit.boru`'s handler +
its `record` sink, whose `[None] [ None ]` spelling netted two values).
NUR049 is retired from NUR.md per the register protocol. Pins:
`lang/go/check_error_handler_test.go`,
`eng/go/barrier_receiver_hint_test.go`. This also advances §8.4.2: the
`error`-handler surface is now identical across the two passes.

### 9.6 P1 groundwork LANDED; the capability mechanisms stay staged

The §8.2 targets are now LEDGERED (measurable, per the ledger-first
discipline) where they weren't: `frontier-chained-apply.tsv` (Stage G's
graduation rows, §9.1), `frontier-full-stack.tsv` (depth/pick/roll —
previously off-corpus entirely), `frontier-poly-join.tsv` (the
differing-arm-returns gate; its Atom-slot sibling turned out already
GRADUATED — the survey's §3c refusal compiles natively today and rides
as the control row), and `frontier-do-error-arity.tsv` (the
zero-netting handler). The capability mechanisms themselves — Stage G's
two coordinated lowering changes, the OpDepth/OpPick/OpRoll family, the
return-join user-poly recording, the container auto-invoke op, the
variable-arity island — remain the staged projects §8.2 sequences: each
is census-gated soundness-frontier work the repo's own discipline
forbids half-landing (DESIGN-DX-AND-BYTECODE-STATUS-REVIEW's closing
rule), now with the §8.1(3) fuzzer in place as the safety net the
sequencing required.

### 9.6a §8.2(2) LANDED — full-stack words graduate for exact stacks

`EmitState.FoldFullStack` (emit.go, wired at the check-mode full-stack
dispatch in engine.go) statically folds `depth`/`pick`/`roll` when the
simulated stack is provably exact — the dispatch ELIDES: `depth` bakes
its count as a concrete const, `pick` re-pushes the picked entry (an
event target is promoted to a value-def local, riding the same
double-reference machinery as `dup`), and `roll` re-seats the true
permutation. No new opcode and no event — which is a *stronger* result
than the §8.2(2) sketch (an event-producing lowering): the elision has
zero runtime surface to get wrong, and the whole risk concentrates in
the exactness gate (top frame of the top unit, no open mark window,
every entry a known operand home, no variadic producer, concrete
in-range `n`). Inexact contexts decline to the historical refusal —
including out-of-range `n`, where the interpreter's raise stays
byte-identical via fallback. Graduated rows moved to
`lang/spec/corpus-core.tsv`; the one remaining sub-frontier (a roll
permuting two EVENT results — the program-residual re-push order
exceeds Stage 1) is ledgered in `frontier-full-stack.tsv`. Pins:
`eng/go/fold_fullstack_test.go` (every gate arm), the corpus rows
(differential-owned).

A bookkeeping correction from this landing: the committed
`COMPILED_STATUS.md` census this review's §1 quoted (6996 rows) had
been STALE — the staleness check is deliberately informational
(`compiled_status_test.go`), and the corpus had grown to 7241 rows
without a refresh. The binding truth was always the ratchet gates
(refusals 0 / islands 0), which held at both counts and still hold at
the regenerated 7246 (including the five graduated rows).

### 9.7 Still open

Stage G proper and the rest of P1's mechanisms (§8.2/§9.6), the
finish-line statement's enforcement (§8.3), the remaining
one-diagnostic-surface classes (§8.4.2 — the closure-render
compile-pass diagnostics, now at least visible via §9.3), the
Any-frontier headroom decision (§8.4.3), the in-body mirrors widening
(§8.4.4), and the §9.4 def-split checker FP's graduation.
