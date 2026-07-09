# NET-COMPILE-FRONTIER — what blocks the aql:net apps from compiling

Goal: get the realistic `aql:net` example handlers (`design/examples/apps/`) to
compile their bodies to bytecode units (like the echo benchmark handler does),
not just run interpreted. This note maps every remaining blocker precisely so the
work can proceed one capability at a time.

Status: **mapped, not yet closed.** The echo microbenchmark handler compiles
(~55k req/s vs ~620 interpreted). The three realistic apps still fall back to the
interpreter, each for a DISTINCT compiler-frontier reason surfaced by
`aql run -force-compile -install network bench/networking/apps/<app>.aql`.

None of these block `aql run` (the apps run fine interpreted, default gate green,
per design/CHECK-FALSE-POSITIVES.0.md). They block only the compiled speedup.

## The walls (each app, exact blocker)

### mini-redis — TWO walls

1. **Unparenthesised `set … drop` residual (check-mode, a genuine defect).**
   The handlers use the idiom `X set (k) v` newline `drop` (flex `set` returns its
   receiver; the `drop` discards it). Unparenthesised, followed by a `def`, the
   set's dispatch residual corrupts the following `def`'s binding in the COMPILE
   pass (`a.CompileCheck`), so a later read is `undefined_word` (mini-redis
   handlers: `undefined_word: expires`). Same forward-collection-residual class as
   the `redis-cmd ep:Any` bug (design/CHECK-FALSE-POSITIVES.0.md). `(X set (k) v)
   drop` grouping clears it (verified: all 11 sites grouped → 0 check errors).
   Plain `Check` (the `aql run` gate) is unaffected — this is compile-pass only.
   Fixable either by grouping the idiom in the example or by fixing the
   forward-collection residual in the kernel (higher-leverage, higher-risk —
   matchSignature/insertForward).

2. **Stage-3 "branch reads enclosing computation"** (`eng/go/lower.go:431`). After
   wall 1 is cleared, the lowerer refuses because a handler `if`-arm references a
   value computed OUTSIDE the branch (an enclosing body-local — e.g. kv-read's
   `def expired (if …); if expired [ … kv … ] [ 0 ]`, whose arm reads `kv`/`k`).
   `lowerEvents` refuses when a branch-arm operand references an event at
   `op.idx <= scopeFloor`. Closing it needs the lowerer to reference an
   enclosing-scope value from inside an arm (promote the enclosing event to a
   local/slot the arm can PUSH_LOCAL — the `lw.promoted` mechanism exists for the
   main stream; extend it across the scope floor).

### mini-s3 — serve-raw materialization

`operand of unknown provenance or not statically materialisable at serve-raw`. The
streaming HTTP handler threads a MUTABLE/`flex` value that the store-fn bake cannot
materialise as a const. Machinery that already handles the accumulator shape lives
in `moduleScopeMutableCaptures` (`eng/go/callable_words.go`); this handler's shape
is not yet covered. Closing it needs the materialiser to capture (or otherwise
make VM-referenceable) the specific mutable value this handler threads.

### todo-api — Stage-2 "branch leaves extra values"

`fn storedfn$body: branch leaves extra values (Stage 2 lowers single-result
branches)` (`eng/go/lower.go:1600`). One handler `if`-arm nets more than one value
in a non-variadic position; the single-operand arm model records only the top
operand, so the inert/local values below it cannot be reconstructed. The
`allowVariadic`/`fragMulti` path already absorbs a multi-value arm in a
variadic-consuming position; closing this needs that absorption extended to the
handler's arm position (or the specific arm restructured to net one value where
that preserves semantics).

## Assessment

Wall 1 is a genuine residual defect (the recurring forward-collection class). The
other three are UNIMPLEMENTED lowering/materialization CAPABILITIES — the exact
"Stage 2 / Stage 3 lowering + serve-raw materialization" frontier the benchmark
README names. Each is substantial, soundness-critical bytecode-compiler work,
gated by `make verify-bytecode` (differential + property-fuzz + -race) and
`cover-gate` 100%: a wrong lowering that MISCOMPILES is the one outcome the
codebase forbids, so none can be rushed.

## Recommended sequence (one capability per focused, fully-gated change)

1. **Stage-2 multi-value branch** (todo-api) — most self-contained; the
   `fragMulti` machinery is the closest to done.
2. **Stage-3 enclosing-read** (mini-redis wall 2) — extend the `promoted` slot
   mechanism across the scope floor; pair with wall-1 grouping (or the kernel
   residual fix) so mini-redis compiles end-to-end.
3. **serve-raw materialization** (mini-s3) — the deepest; needs the materialiser
   to handle the handler's threaded mutable value.

Each lands only when `verify-bytecode` + `cover-gate` + `TestCheckAccuracyRatchet`
stay green and the app's compiled run matches its interpreted run
(`-force-compile` vs `-no-compile`). The `bench/networking/apps/` drivers make the
before/after measurable.
