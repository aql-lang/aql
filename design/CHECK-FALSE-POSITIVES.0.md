# CHECK-FALSE-POSITIVES — the checker must not block programs that run correctly

Status: in progress. `aql run` runs a static pre-flight check and ABORTS on any
error-severity diagnostic (`aql.go`), so a FALSE-POSITIVE check error blocks an
otherwise-correct program. The realistic `aql:net` examples
(`bench/networking/apps/`) hit this: they run correctly under `-no-check` but the
default gate rejects them. This note tracks the fix.

## The problem (measured)

`aql run` (default) runs the pre-flight check; error-severity diagnostics abort
execution with `check failed: N error(s)`, exit 1. Warnings do not block. The
apps run correctly (`-no-check`, exit 0) but are refused by the gate:

```
$ cmd/go/bin/aql run -install network bench/networking/apps/echo_redis.aql
check: 13:1: [error] no_signature: no matching signature for def — got (dynamic(Any), List) …
check: 13:5: [error] undefined_word: undefined word: dur
        → check failed, never runs
$ cmd/go/bin/aql run -no-check … echo_redis.aql
mini-redis 10000 ops … 452 req/s      # runs fine
```

## Root cause (isolated)

Minimal repro — two `MiniRedis.cmd` calls in one loop body:

```
for 3 [ MiniRedis.cmd ep "SET k v" drop ]                     → check clean
for 3 [ MiniRedis.cmd ep "SET k v" drop  MiniRedis.cmd ep …]  → check ERROR
   mini-redis.aql:270 no_signature: call — got (Map, __FN, Map); nearest [Map Service Map]
```

**Check-mode carrier types degrade across repeated dispatch in a loop.** On the
second `cmd` analysis, the service endpoint `ep` has widened from `Service` to
`Function`, so `call {…} ep {…}` misses the concrete `[Map Service Map]` sig →
`no_signature`. The full driver's `def dur — got (dynamic(Any), List)` is the
same class (a carrier widened after the loop), and it CASCADES: the failed `def`
leaves `dur` unbound → downstream `undefined_word: dur` → `no_signature for div`.
One root false positive produces four diagnostics.

### Precise mechanism (stack-traced)

The bad diagnostic is emitted at `checkModeAssumeSig` (`engine.go:7382`) while
computing `redis-cmd`'s RETURN type — the path
`execFnDefLiteral → execMatch → carrierResults → declaredReturnCarriers →
buildFnBodyReturnsFn → AnalyseFnBody → runFnBodyOnce → Run(redis-cmd body) →
call fails`. `AnalyseFnBody` re-analyses `redis-cmd`'s body over carrier args in
which `ep` has become a `Function` carrier instead of the connection it is. This
only happens with TWO module-fn dispatches over `ep` inside a `for` body — one
dispatch, or two dispatches WITHOUT a loop, are both clean — so the trigger is
the interaction of the LOOP FIXPOINT analysis (`AnalyseLoopBody`, `carrier.go`)
with the per-dispatch RETURN-TYPE PROBE (`carrierResults`). It is NOT the
lowering/recording arming (a deliberately-uncompilable loop body still errors),
and it PRE-DATES the `Any`-param / `locked_signature` fixes (reverting them does
not clear it). The diagnostic site is DELIBERATELY not suspend-gated
(`engine.go:7329-7368`, the maintainers' anti-over-suppression stance from the
16-row lesson), so the fix must restore precision (keep `ep` a connection), not
just silence the report — silencing risks dropping the genuine errors that gate
guards.

### Confirmed: an implicit-statement-boundary residual (with a workaround)

The trigger is narrower than "loops" in general: it is a **statement boundary
inside a body that is not committed**. Two dispatches in one `for` body separated
by an explicit `;` check CLEAN; without the `;` they trip. Likewise a `def` right
after a `for` trips unless the `for` statement is `;`-terminated:

```
for 3 [ MiniRedis.cmd ep "a" drop   MiniRedis.cmd ep "b" drop ]     → no_signature
for 3 [ MiniRedis.cmd ep "a" drop ; MiniRedis.cmd ep "b" drop ] ;   → clean
```

So the first dispatch leaves a residual — the module fn-VALUE produced by the
`MiniRedis.cmd` dot-access, not fully consumed in check mode — on the analysis
stack, and the next statement's collection mis-grabs it as `ep`. The `;` (an
`end` marker) forces the `commitBarrierForward` that clears it; an IMPLICIT
statement boundary inside a `for`/list body does not. Top-level statements don't
share the residual (each is committed), which is why the bug only shows inside a
body. This only bites the service `call` path (mini-redis); the serve-raw `req`
path (mini-s3) and single-statement loop bodies (todo-api) stay clean.

**Workaround (shipped for the benchmark drivers):** put `;` between statements in
a loop body and after a `for` that a later statement reads across. This lets the
`bench/networking/apps/echo_redis.aql` driver pass the DEFAULT pre-flight gate
(dropping `-no-check`). It is a workaround, not the fix.

### Localised to the ARMED loop-capture path (not "statement boundaries")

Further bisection (read-only, this session) narrows it precisely, and corrects the
earlier "implicit statement boundary" framing:

- `do [ … ]`, `if … [ … ]`, a paren group, and a NON-lowerable `for`
  (`def n dynamic; for n [ … ]`) with the two dispatches are ALL CLEAN. Only a
  **lowerable `for`** (`for 3 [ … ]`) trips.
- The discriminator is `loopCapture` in `AnalyseLoopBody` (`carrier.go:3016`),
  armed by `es.ArmLoopCapture()` in the `for` handler (`native_control.go:980`)
  when the count is statically lowerable. `do`/`if` use `RunCarrierBodyWithDefs`
  UNARMED and are clean; the armed loop re-analysis (fragment capture for bytecode
  lowering, `elemEvalRecordable=true`) leaves a module-fn-VALUE residual on the
  check-mode stack, which the next dispatch mis-collects as `ep` (→ `Function`).
- Confirmed at the emit site: the failing `call` runs with `recordable=false`
  (it is `redis-cmd`'s return-type probe), but its `ep` arg arrived as `Function`
  from the upstream armed dispatch.

So it is NOT a general statement-boundary bug — it is specific to the ARMED
loop-lowering re-analysis. `;` clears it because `stepEnd` commits the residual;
`do`/`if`/non-lowerable-`for` avoid it by never arming.

**The fix (Stage 2), sound + ratchet-gated:** the armed loop-capture analysis
exists to RECORD the loop body for lowering; its diagnostics are a byproduct and
the authoritative ones come from an UNARMED analysis (what `do`/`if` run cleanly).
The precise, sound change is to make `AnalyseLoopBody` source its ERROR
diagnostics from an unarmed round (matching `do`/`if`/non-lowerable-`for`) while
the armed rounds only RECORD the fragment — so a genuine loop-body error (present
in both armed and unarmed) still reports, but an armed-only residual artifact does
not. This is a restructure of a soundness-critical function; it MUST land gated by
`TestCheckAccuracyRatchet` (no genuine-error row dropped) + `verify-bytecode` (the
lowering must still record identically) + the corpus. It is precisely scoped but
not landed in this pass — the residual-leaving line in the armed dispatch was not
pinned, and a restructure without that certainty risks the ratchet. The `;`
workaround remains the shipped resolution for the benchmark drivers.

## The design tension (must respect)

The emit gate (`engine.go:7369`, the `recoverableUnknownType` computation)
already suppresses a NARROW gradual case — a single-overload user fn over an
`Any` carrier — but the maintainers DELIBERATELY keep most `no_signature`
verdicts as errors. The comment at `engine.go:7336-7368` documents why: a broad
`bestMatch>=0` suppression previously DROPPED 16 GENUINE error rows the
interpreter raises on (`TestCheckAccuracyRatchet` 208→192). The fix must be
PRECISE — never a blanket downgrade; every change gated so no real error is lost.

## Approach — staged, precision-first

- **Stage 1 — Corpus + classification (no behavior change).** A regression
  corpus of programs that RUN CORRECTLY (`RunCompiledStrict == Run`, exit 0) yet
  `CompileCheck` emits error-severity diagnostics. Seeded from the 3 apps + the
  reduced repros. Each becomes a pinned "check must not error here" test.
- **Stage 2 — Stop the carrier widening (primary fix, the real root).** Keep a
  value bound OUTSIDE a loop from having its check-mode carrier widened to
  `__FN` / `dynamic(Any)` during loop-body re-analysis. Eliminates the false
  positive at its source without touching the suppression policy.
- **Stage 3 — Downgrade non-guaranteed verdicts (safety net).** Where a
  `no_signature` still rests on a widened/gradual carrier AND recovery declined,
  downgrade error→info, extending `recoverableUnknownType` one class at a time,
  each gated by the corpus AND the 16-row ratchet.
- **Stage 4 — Stop cascades.** Once a statement's dispatch fails and a name never
  binds, suppress the downstream `undefined_word`/`no_signature` consequences.
- **Stage 5 — Re-validate + fold into the benchmark.** The app drivers `aql
  check` clean (or info-only); drop `-no-check` from the committed benchmark
  drivers.

## Verification

- New corpus: run-correct-but-check-errors → assert no error diagnostics AND
  `RunCompiledStrict == Run`.
- `TestCheckAccuracyRatchet` must not lose genuine-error coverage (the primary
  soundness gate — the 16-row guard).
- `verify-bytecode` + crossdiff clean; `cover-gate` 100%.

## Top risk

Trading a false positive for a FALSE NEGATIVE (dropping a genuine error) — the
exact tradeoff the 16-row lesson warns about. Precision-first (Stage 2) avoids
it; every Stage-3 downgrade is ratchet-gated.

## Scope note

This is the checker-precision track. It is orthogonal to — and a prerequisite
for — making the app HANDLERS compile fast (the separate lowering /
materialisation walls, `design/CALLBACK-COMPILATION.0.md`). Solving these makes
`aql run` ACCEPT the apps by default; making them compile is the other frontier.
