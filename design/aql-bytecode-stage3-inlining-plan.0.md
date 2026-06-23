# Stage-3 module-fn call-site inlining — verified implementation plan

_Status: PLAN. No Stage-3 code landed. This note records the
code-level findings established by tracing `module-test.tsv:38`,
`module-rand.tsv:38`, and `module-parselang.tsv:23` to ground truth,
so the dedicated build starts from certainty._

## Why these 3 rows refuse (and the 2 that must)

Live refusals (5), from the corpus census:

| row | reason | disposition |
| --- | --- | --- |
| `def-node-binding:54` | `fn mk: body result of unknown provenance` | **correct refusal** |
| `macro:45` | `residual value not statically materialisable` | **correct refusal** (error row) |
| `module-parselang:23` | operand provenance at `get` | Stage-3 |
| `module-rand:38` | residual value of unknown provenance | Stage-3 |
| `module-test:38` | `code-body word test-describe (Stage 2)` | Stage-3 |

The two correct refusals are PROVEN unsound to compile by running the
interpreter:

- `def c1 1 def mk fn [[c1:Integer] [List] [[c1]]] mk 9` → `[1]`. The
  returned `[[c1]]` resolves `c1` against the MODULE binding (1) at
  top-level auto-eval, not the param (9). Any compile that binds the
  param in body scope yields `[9]` → differential-gate divergence. The
  `body result of unknown provenance` guard is what keeps it sound.
- `def loopy (macro [[a] [quote [loopy unquote a]]]) macroexpand (loopy 1)`
  → raises `expansion too deep`, exactly the `ERROR:` the spec requires.
  Static divergence is undecidable; refuse-and-raise is the only sound
  behaviour.

So `refusalCeiling=0` is unreachable without unsoundness. The reachable
target is the 3 module rows.

## The single root cause of the 3 module rows

A module-preamble AQL fn (`run-spec`, `run-cases`, `run-case`;
`Rand.list-of`; `ParseLang.parse_calc`) is an `FnFrame` fn. In
check+emit mode its `buildFnBodyReturnsFn` (`eng/go/core_helpers.go:271`)
compiles the body as a **shared, type-memoized unit** against
GENERALISED carrier args:

```go
// core_helpers.go:371-380 — the deliberate invariant
genArgs := make([]Value, len(args))
for i, a := range args {
    genArgs[i] = NewCarrier(a.Parent) // concrete values would const-fold
}                                     // into the shared unit — wrong for
                                      // other call sites.
```

Because the param is a carrier, every field read inside the body
(`(s get "name")`, `(s get "cases")`) is `Any` (the `getNodeReturns`
fold only fires on CONCRETE containers — landed this session, see
below). `Any` then can't dispatch the body's typed code-body word
(`test-describe [String List]`), can't materialise the `for (subs size)`
count to prune the recursion, and can't type the parser/RNG residual.

A unit-body `MarkUncompilable` sets `es.Compilable=false` GLOBALLY
(no per-call isolation), so the body's refusal becomes the whole
program's refusal — that is why `module-test:38`'s program-level reason
is the deep `test-describe` one.

## What must be built (4 interdependent features)

None clears a row alone; only all four together flip `module-test:38`.

1. **Call-site inlining (the new compilation mode).** When a non-generic
   `FnFrame` fn is called with CONCRETE args, record the body's events
   INTO THE CALLER'S STREAM with the concrete args bound (not a shared
   CALL_USER unit), so field reads fold. This mode does not exist:
   `RecordUserCall` only emits `CALL_USER` to a type-memoized unit. The
   inline fragment must be keyed by call site / concrete value, never by
   arg TYPE (else a second call with same types, different values, reuses
   baked constants). Gate it PROBE-FIRST in an isolated `EmitState`
   (`IsolateEmit`) so a body that does not fully inline falls back to
   today's shared-unit path with zero regression.

2. **Zero-count `for` pruning.** A `for` whose count is concretely 0 has
   an unreachable body and yields `[]`. Required so the inlined
   `for (subs size) [ subspec run-spec ]` (subs=`[]`) drops the recursive
   branch instead of analysing it. Sound and bounded, but inert until (1)
   makes `subs` concrete inside the body (prototyped+reverted earlier for
   exactly this reason).

3. **Module closure code-bodies.** `test-describe`/`test-test` bodies
   already declare a `CallableSpec` (`lang/go/modules/test.go:232,279`);
   they need the closure path (`tryRecordClosure`) to admit them once
   their name arg is a concrete String (depends on (1)).

4. **Fn-value dispatch.** `run-case` does `(in subject test-invoke)`,
   where `subject` is the fn-VALUE `double/q`. `test-invoke` invokes it —
   currently `anonymous function dispatch (Stage 3)`. Compiling this is
   the LAST feature; it is the one that finally clears `module-test:38`.

`module-rand:38` needs (1)+(2)-style residual typing; `module-parselang:23`
needs (1) plus typing a user-registered fn's return — both narrower than
`module-test:38` but still rooted in (1).

## Landed this session (sound, gate-clean prerequisites)

- **`getNodeReturns` container fold** (`lang/go/native/native_storage.go`):
  a concrete list/map field read folds to the cloned container value
  (fresh ID), not a bare type carrier. 5 rows off the Any frontier
  (254→249). This is the precise prerequisite that makes the inlined
  body's field reads concrete in feature (1).
- Corrected the earlier (wrong) root-cause pin; this note supersedes it.

## REFINED (deeper trace): the get-fold already advanced the concrete pass

A later dispatch-level trace (`EXPDISP` at engine.go:2475 + `MARK` at
MarkUncompilable, both reverted) corrects the picture above. Run-spec's
body is analysed in TWO passes, and the get-fold landed this session
already fixed the concrete one:

- **Concrete pass** (the real call's args): with the get-fold,
  `(s get "name")` folds to `ProperString` over a concrete `Map`, so
  `test-describe` DISPATCHES (`EXPDISP test-describe ProperString/F
  List/T`). `run-cases`/`run-case` also dispatch. The only remaining
  blocker here is that `test-describe`'s code body (a closure) does not
  compile via `tryRecordClosure` → `code-body word test-describe (Stage 2)`.
- **Carrier shared-unit pass** (genArgs = carriers, core_helpers.go:377):
  carrier `s` → `(s get "name")` = `Any` → `test-describe` cannot match
  `[String List]` → `unmatched dispatch recovered at test-describe`, which
  fails the shared unit and poisons the program.

So the two MARKs observed (`unmatched dispatch recovered` THEN `code-body
word`) are the carrier pass and the concrete pass respectively. The
implication SHARPENS feature (1): the shared **carrier** unit must be
BYPASSED for a concrete-arg call (call-site inlining), AND
`test-describe`'s closure body must compile. The closure body captures
concrete `s`, calls `run-cases` (now dispatching), and recurses into
`run-spec` over `subs` — so it also needs feature (2) zero-count `for`
pruning (subs=`[]`) and ultimately feature (4) for `test-invoke`'s
fn-value call. Net: the get-fold moved the concrete pass from "name is
Any, can't dispatch" to "dispatches; closure-body compilation is the next
wall" — measurable progress toward feature (1).

## Earlier prerequisite note (kept for the carrier-pass boundary)

Tracing `args` into `run-spec`'s `buildFnBodyReturnsFn` (temporary
instrumentation, reverted) shows the concrete map is ALREADY LOST before
the body is ever compiled:

```
EXP run-spec arg 0 parent Any concrete false
```

At top level `def s {…} end (s get "name")` compiles (the map is concrete
and the get-fold fires). But passing the same `s` through the module-fn
dispatch boundary `s Test.run-spec` delivers arg 0 as a bare `Any`
carrier — not even `Map`. So specialising the body unit to "concrete
args" is inert (verified: forcing `genArgs[i]=a` when `IsConcrete(a)`
left `module-test:38`'s refusal reason byte-identical) because the arg
that reaches the ReturnsFn is not concrete in the first place.

**Therefore feature (1) has a sub-prerequisite (1a): the module-fn /
dot-access dispatch boundary must PRESERVE the concrete arg** (today it
hands the callee an `Any` carrier in check+emit mode).

Pinpointed (instrumentation in `buildFnBodyReturnsFn`, reverted): for the
`s Test.run-spec` call,

```
EXP run-spec param 0 type Map arg Any/conc=F
```

— the SIG PARAM is correctly typed `Map`, but the ARG VALUE delivered to
the ReturnsFn is a bare `Any` carrier (not concrete, not even `Map`). And
the same map read DIRECTLY (`def s {…full module-test shape…} end
(s get "name")`) compiles — the value IS concrete until it crosses the
module call. So the erasure is neither at def-time nor in matchSignature
(which matched `Map`); it is in the **cross-registry module-dispatch
arg-passing** (the path that resolves `Test.run-spec` and invokes
run-spec's ReturnsFn — execFnDefLiteral/execFnDefSig + shareCheckState).
That path substitutes an `Any` carrier for the concrete operand. Fix:
thread the concrete operand through to the callee's check-mode analysis
(it sits in the hot, shared cross-registry dispatch — gate behind the
full differential before landing). Only then can (1) fold anything.

## module-rand:38 is the CLOSEST row — a different (narrower) root than module-test

Deep tracing (dispatch + mark + residual-provenance, all reverted) shows
`"aql:rand" import end def r (Rand.with-seed 2) r.list-of [Rand.int 0 10] 3`
is NOT blocked by the inlining stack at all:

- NO `MarkUncompilable` fires — the body records cleanly. `rand-with-seed`
  and `rand-int` both reach `carrierResults` and record (`CARRES rand-int …
  rc false` → generic CALL_NATIVE). The `[Rand.int 0 10]` generator and the
  count compile.
- The SOLE failure is at lowering (emit.go:3226): the program's final
  residual — the concrete `List` returned by `list-of` — has NO `producedBy`
  entry (`RESIDUAL-UNKNOWN rv.ID N_… nEvents 2`, and the 2 events are
  `rand-with-seed` + `rand-int`, not `list-of`). So `list-of`'s own dispatch
  recorded NOTHING.

Root cause: `r.list-of` is a NoEvalArgs / code-body module wrapper
(`wrapRandFnDefNoEval`, BodyPos 0, BodyOut 1). Unlike the plain wrappers
(`rand-int`, `rand-with-seed`) which short-circuit to `execMatch` →
`carrierResults` → record, the NoEvalArgs/code-body wrapper dispatch does
NOT pass through `carrierResults` (no `CARRES rand-list-of` line), so its
result carrier is produced but never recorded as a closure call. The
residual is then untraceable → refuse.

**Fix (narrower than module-test's 4-feature stack): make `list-of`'s
dispatch RECORD a closure call** so its `[Rand.int 0 10]` body → a closure
unit and its result → a tracked operand.

CORRECTION (further tracing): `r.list-of`'s dispatch goes through NEITHER
the `execFnDefSig`/`CallAQL` path (a `FNDEFSIG` probe never fired) NOR
`carrierResults` (no `CARRES rand-list-of`/`list-of` line). So the result
`N_…` (the final `List`) is produced by some OTHER path — most likely the
Rand-instance `get`/method dispatch for `r.list-of` (where `r =
(Rand.with-seed 2)` is a Rand instance, not the module export). That exact
dispatch function is NOT yet pinned; pin it FIRST (instrument the Rand
instance's method/get handling), then route it through the
closure-recording machinery (`tryRecordClosure` / `RecordClosureCall`).
The fix does NOT need call-site inlining, zero-count-for, or fn-value
dispatch, and MUST be gated behind the full `make verify-bytecode` (the
path is shared). If it lands clean, `module-rand:38` clears
(refusalCeiling 5→4) and likely helps `map-from`. Recommended NEXT row —
but the dispatch site needs pinning before the one-line claim holds.

## REJECTED shortcut (tested empirically): get-returns-concrete-FnDef

Hypothesis: `r` (the `Rand.with-seed` result) is a plain Map, so
`r get list-of` runs `getNodeReturns`, which returns `dyn`(Any) for an
FnDef field — so list-of never dispatches and its gen body `[Rand.int 0
10]` is left as the untracked residual. Hoped fix: return the CONCRETE
FnDef for a (module-wrapper) FnDef field so it resolves like a module word
and dispatches.

Tested (reverted): forcing `getNodeReturns` to return `CloneValue(val)`
for FnDef fields did NOT compile `module-rand:38` — it still refuses
`residual value of unknown provenance`. (And `m.f 2 3` from fn-value.tsv
compiles independently either way.) So the get-typing is NOT the blocker:
even with the concrete FnDef in hand, list-of's dispatch records no event.

Deeper confirmation (FDL trace, reverted): even with `getNodeReturns`
returning the concrete list-of FnDef, `execFnDefLiteral` is called for
`rand-with-seed` and `rand-int` but NEVER for `rand-list-of`/`list-of` —
the concrete FnDef value sits on the check stack as DATA and never
dispatches, so `[Rand.int 0 10]` and `3` are left unconsumed and the gen
body list becomes the untracked residual. So module-rand:38 needs THREE
interlocking pieces, each revealing the next: (a) get resolves the
concrete module-wrapper FnDef, (b) that value actually DISPATCHES in
check+emit mode (it currently does not — the fn-value-on-stack-with-args
dispatch is not driven here), and (c) the dispatch RECORDS (closure /
OpCallDynamic) so the result is tracked. This is the fn-value-dispatch
feature module-test:38 also needs for `test-invoke`. Do not re-attempt the
get-typing shortcut alone; the work is the full dispatch path.

## Discipline for the build

Probe-first per feature (isolate, commit only on full success, fall back
otherwise). `make verify-bytecode` (differential + fuzz + race + aqldebug,
0 divergences) gates every step; ceilings re-baseline only when a row
actually clears, with rationale. Expect features (1)-(3) to clear NO row
on their own — the ceiling moves only when (4) lands. Gate-clean-or-revert
throughout.
