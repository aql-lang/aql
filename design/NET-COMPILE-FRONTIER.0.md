# NET-COMPILE-FRONTIER — what blocks the aql:net apps from compiling

Goal: get the realistic `aql:net` example handlers (`design/examples/apps/`) to
compile their bodies to bytecode units (like the echo benchmark handler does),
not just run interpreted. This note maps every remaining blocker precisely so the
work can proceed one capability at a time.

Status: **2 of 3 closed.** The echo microbenchmark handler compiles (~55k req/s
vs ~620 interpreted). **todo-api** (~12x) and **mini-redis** now compile their
handlers (see the per-app sections below); **mini-s3** remains, blocked on two
deliberate-frontier capabilities (`do`/`error` trap-region lowering + higher-order
`filter`) for an I/O-bound app. Each blocker was surfaced by
`aql run -force-compile -install network bench/networking/apps/<app>.aql`.

None of these block `aql run` (the apps run fine interpreted, default gate green,
per design/CHECK-FALSE-POSITIVES.0.md). They block only the compiled speedup.

## The walls (each app, exact blocker)

### mini-redis — TWO walls — **CLOSED**, compiles (~680 vs ~442 req/s)

1. **Unparenthesised `set … drop` residual (check-mode).** The handlers use the
   idiom `X set (k) v` newline `drop` (flex `set` returns its receiver; the `drop`
   discards it). Unparenthesised, followed by a `def`, the set's dispatch residual
   corrupts the following `def`'s binding in the compile pass (`undefined_word:
   expires`). **Fixed** by grouping all 11 sites as `(X set (k) v) drop`
   (semantics-preserving — verified by TestAppMiniRedis).

2. **Stage-3 "branch reads enclosing computation"** (was `eng/go/lower.go:431`).
   A handler `if`-arm references a value-def computed OUTSIDE the branch (LRANGE:
   `def start …; if … [ … if (start gte n) [ … ] [ slice start n cur ] ]`). The
   enclosing value-defs ARE promoted to frame locals, and their consuming
   references rewrite to local pushes — but an arm-OUT designation referencing a
   promoted producer is left for `lowerFragment` to re-resolve, and the scopeFloor
   guard treated that residual `opEvent` as a forbidden enclosing read. **Fixed**:
   the guard now skips operands whose producer is in `lw.promoted` (delivered via
   the frame, not the enclosing sim). Regression:
   `TestBranchArmReadsEnclosingValueDef`.

### mini-s3 — MULTIPLE walls, not yet closed

The top-level `operand of unknown provenance … at serve-raw` is a CONSEQUENCE:
the `serve-raw` handler `([conn] => [ s3-conn conn store ])` fails its compile
probe because `s3-conn`'s body refuses. Attributing every refusal to its fn
(instrument `MarkUncompilable` with `es.unitNames`) shows FOUR distinct blockers,
two of them deliberate frontier islands:

- **`s3-conn`: `code-body word do (Stage 2)`.** The per-connection actor is
  `def ok (do [ s3-handle-one conn store drop true ] error [ drop false ]); if ok
  [ s3-conn conn store ] [ 0 ]`. Both `do` and `error` carry
  `CompileEffect: CompileFallbackBody` (`lang/go/native/native_control.go`) — they
  DELIBERATELY island to the interpreter. Compiling this means lowering a
  `do`/`error` trap region (bytecode exception handling) — a large, deliberate
  VM feature, not a bug.
- **store `list` handler: `function-valued operand at filter (Stage 3)`.** The
  LIST handler runs `filter ([e:Any] => […]) (keys objects)` — higher-order
  compilation of a lambda operand. A separate large frontier.
- **`s3-handle-one`: `unmatched dispatch recovered at convert`** and the store
  handlers' `unmatched dispatch recovered at drop` (×2). The drop ones are the
  set/drop residual class — clearable by grouping `(objects set …) drop` (as
  mini-redis/todo-api). The `convert` one sits inside the deeply-nested method
  `if` chain and needs its own look.

So mini-s3 requires TWO deliberate-frontier capabilities (`do`/`error` body
lowering + higher-order `filter`) on top of the residual cleanups. Each is a
substantial, soundness-critical VM feature the compiler team has intentionally
deferred (the CompileFallbackBody islands). mini-s3 is also I/O-bound —
`-force-compile` vs `-no-compile` are within noise (~100 req/s both) — so the
payoff is small. Left for dedicated follow-ups; NOT closed.

### todo-api — Stage-2 "branch leaves extra values" — **CLOSED**, compiles (~12x)

`fn storedfn$body: branch leaves extra values (Stage 2 …)`. Two parts. (a) Several
handlers left their flex `set` receivers un-dropped (unlike mini-redis) — grouped
+ dropped, semantics-preserving. (b) The PUT arm `def t2 {…}; (todos set (id) t2)
drop; t2` has t2 as BOTH the `set` argument and the arm result; the intra-arm use
popped its single sim slot, leaving nothing to seat as the result. **Fixed**:
`planValueDefLocals` now promotes a fragment-internal arm-result value-def that is
also consumed as an operand within its arm (the `fragResultStaysOnSim` helper).
Regression: `TestBranchArmResultSelfConsumed`.

## Remaining — mini-s3 (`do`/`error` body + higher-order `filter`)

See the mini-s3 section above for the full attribution. Two deliberate-frontier
capabilities block it — `do`/`error` trap-region lowering (the per-connection
actor) and higher-order `filter` compilation (the LIST handler) — plus set/drop
grouping + a `convert` residual. Each capability is a substantial, soundness-
critical VM feature the compiler deliberately islands today; mini-s3 is also
I/O-bound (compiled ≈ interpreted), so the payoff is small. Not yet done — these
are dedicated follow-ups, not a quick fix.

## What landed (each a focused, fully-gated change)

- **todo-api** (Stage-2 self-consumed arm result) — `fragResultStaysOnSim`
  promotion + example set-drop cleanup. Compiles ~12x.
- **mini-redis** (Stage-3 promoted-operand scopeFloor skip) — the guard skips
  operands delivered via a frame local + example set-drop grouping. Compiles.

Each landed only after `verify-bytecode` (differential + property-fuzz + -race),
`cover-gate` 100%, `TestCheckAccuracyRatchet` unchanged, and the app's compiled
run matched its interpreted run (`-force-compile` vs `-no-compile` / the app
functional tests). A wrong lowering that MISCOMPILES is the one outcome the
codebase forbids, so none was rushed. The `bench/networking/apps/` drivers make
the before/after measurable.
