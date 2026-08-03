# Type checker + bytecode compiler — completeness review

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
