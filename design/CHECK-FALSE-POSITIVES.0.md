# CHECK-FALSE-POSITIVES — the checker must not block programs that run correctly

Status: **resolved** for the realistic `aql:net` app drivers. `aql run` runs a
static pre-flight check (plain `Check`) and ABORTS on any error-severity
diagnostic (`cmd/go/internal/check.Preflight` → `a.Check`), so a FALSE-POSITIVE
check error blocks an otherwise-correct program. The `bench/networking/apps/`
drivers hit this; they now pass the DEFAULT gate (no `-no-check`, no `;`
workaround). This note records the measured symptom, the true root cause, and the
fix.

## The symptom (measured)

`bench/networking/apps/echo_redis.aql` ran correctly under `-no-check` but the
default gate rejected it:

```
$ aql run -install network bench/networking/apps/echo_redis.aql
check: [error] no_signature: no matching signature for call —
       got (Map, Function, Map); nearest [Map Service Map]
        → check failed, never runs
```

Minimal reproduction — two `MiniRedis.cmd` calls in one statically-counted loop:

```
for 3 [ MiniRedis.cmd ep "SET k v" drop ]                     → check clean
for 3 [ MiniRedis.cmd ep "SET k v" drop  MiniRedis.cmd ep … ] → check ERROR
```

## Root cause (isolated, stack-traced)

Not a loop-carrier degradation and not an implicit statement boundary (both were
earlier hypotheses, since disproven). The chain is:

1. `mini-redis.aql` declared `def redis-cmd fn [[ep:Any line:String] …]`. The
   connection parameter `ep` was typed **`Any`**.
2. In `MiniRedis.cmd ep "…" drop`, forward-collection assigns sig positions for
   the wrapper. Because `ep:Any` accepts anything, `matchSignature` (on the
   `insertForward` re-entry) collects the FOLLOWING function word **`drop`** into
   the `ep` slot — past the function-word barrier that a concrete slot would stop
   at (`positions=[3 1]` with `drop` at tape index 3, vs the clean `[1 0]`).
3. That non-zero forward count flips the wrapper OFF the clean declared-return
   path (`carrierResults` → `[String]`, one value) and ONTO the inline
   body-splice path, which leaks the body's residual: `reply.line` (String) PLUS
   the unconsumed `call` result (Any). `MiniRedis.cmd ep "X" drop` therefore nets
   `[ProperString Any]` (2 values) instead of `[]`.
4. Inside a statically-counted `for`, the spread-residual model
   (`native_control.go`) repeats that per-iteration residual `count` times, so a
   stray `Function` value lands on the residual; the end-of-run re-eval
   re-dispatches it as the next call's `ep`, and `call` sees `(Map, Function,
   Map)` where it wants `[Map Service Map]` → the spurious `no_signature`.

A plain user fn + `drop`, a native word + `drop`, and a Go-body module fn +
`drop` are ALL clean — the leak is specific to an **AQL-body module wrapper whose
first parameter is `Any`**, called unparenthesised and followed by a token.
`(MiniRedis.cmd ep "X") drop` (parenthesised) and `… ; …` (explicit boundary)
were both clean because each isolates the dispatch — hence the earlier `;`
workaround worked, but it treated the symptom.

## The fix

Type the connection parameter concretely:

```
def redis-cmd fn [[ep:Service line:String] [String] [ … ]]   # was ep:Any
```

An Endpoint returned by `Net.connect` IS a `Service` (a remote Service; see the
`aql:net` docs), and the body's `call {…} ep {…}` requires a `Service`, so
`ep:Service` is the CORRECT, more precise type — not a checker contortion. A
concrete slot no longer forward-collects the trailing `drop`, so dispatch stays
on the declared-return path and nets its single `String`. This mirrors the echo
benchmark's `sock:Any` → `sock:Socket` resolution (design examples favour precise
handler-parameter types for exactly this reason).

The fix restores PRECISION (the parameter's real type), never silences a report,
so no genuine-error coverage is traded away — `TestCheckAccuracyRatchet` is
unchanged.

## Result

- `bench/networking/apps/echo_redis.aql` — the `;` workaround is removed; the
  driver passes the default `aql run` gate and still runs (~480–520 req/s).
- `echo_s3.aql` and `echo_todo.aql` already passed the default gate; all three
  now run without `-no-check`.
- Regression corpus: `lang/go/check_false_positive_test.go` —
  `TestCheckNoFalsePositiveMiniRedisLoop` / `…Driver` (assert the plain-check
  pass — the `aql run` gate — reports no error) plus
  `TestCheckLoopBodyGenuineErrorStillReports` (a genuine `undefined_word` inside
  the same loop body STILL reports — precision, not suppression).
- `TestAppMiniRedis` (the app's functional test) is unchanged and green — `ep` is
  always a `Net.connect` result, so the stricter type is always satisfied.

## Remaining (separate, non-blocking)

The stricter **compile pass** (`a.CompileCheck`, used by the `aql check
--compile` variant, NOT by `aql run` or the default `aql check`) still emits an
`undefined_word: expires` false positive inside mini-redis's SET/GET service
handler lambdas (`[req:Map state:Any] => [ def expires state.expires … ]`). It
does not gate `aql run` and is a distinct facet (compile-pass analysis of a
service-handler closure body with a `state:Any` param), tracked for a follow-up.
