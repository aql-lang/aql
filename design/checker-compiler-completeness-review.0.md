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
