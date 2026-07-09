# Why interpreted AQL is slow — investigation & data (2026-07)

## Question

Interpreted AQL (`AQL_NO_COMPILE=1`, the tree-walking engine) runs
"at least two orders of magnitude" slower than compiled AQL (the
bytecode VM), where we expected roughly one. We want the *interpreter*
to land in the same ballpark as CPython / Ruby. This note measures the
gap across languages, profiles the interpreter on the measured shapes,
attributes the cost, and lists prioritized fixes.

Fixtures + harness: `bench/interp/` (three equivalent programs — `fib`,
`loopsum`, `nestloop` — in AQL, Python, Ruby, Node; `run.sh` reports
best-of-N wall-clock and checks the outputs match).

## The gap, in numbers

Best-of-3 wall-clock (ms), 4-core Xeon @ 2.80GHz, Go 1.24. Startup is
~20 ms (aql) / ~17 ms (python) / ~81 ms (ruby) / ~50 ms (node).

| workload | aql-interp | aql-compiled | python | ruby | node |
|---|---:|---:|---:|---:|---:|
| fib(24)          | 20,757 | 418 | 23 | 83 | 51 |
| loopsum (100k)   |  3,429 | 115 | 25 | 80 | 47 |
| nestloop (300²)  |  3,116 | 110 | 22 | 84 | 48 |

Derived ratios:

| workload | interp ÷ compiled | interp ÷ python (execution-only) |
|---|---:|---:|
| fib      | ~50× | ~3,000× |
| loopsum  | ~30× | ~150× |
| nestloop | ~28× | ~140× |

So the user's read is right: the interp↔compiled gap is ~1.5–1.7
orders of magnitude (worse on recursion), and the interp↔Python gap is
2–3.5 orders. The compiled VM itself is only ~15–18× off CPython, most
of which is fixed startup/compile cost — the VM is not the problem, the
**interpreter's per-operation cost** is.

This is consistent with the existing Stage-6 microbenchmark gates
(`design/PERF-BASELINE.10.md`): interp/compiled 21–48× depending on
shape, recursion the worst.

## Where the interpreter spends its time

CPU + allocation profiles of `BenchmarkStage6/*/interp`
(`arith_chain64`, `for_tight`, `map_get`, `recursion_nontail`).

### The headline: the interpreter barely does the user's work

On straight-line arithmetic the actual arithmetic handler is ~3.5% of
execution; the other ~96% is dispatch machinery and the garbage it
generates. Interp allocates **~100 allocations per word dispatch** and
**~340 allocations per fn-call frame**. The measured allocation traffic
is the interpreter's dominant cost — GC alone is ~35–40% of samples.

### CPU attribution (cumulative, interp subset)

| where | ~% samples | what it is |
|---|---:|---|
| `stepWord` | 59% | dispatch entry (everything below) |
| `resolveForwardArgs` | 27% | plan-time forward-argument walk |
| — `evalParenGroupAt` | 23% | sub-evaluating each `( … )` operand |
| `matchSignature` (+`sigArgMatches`/`sigTypeMatches`/`ConformsTo`) | 16% | per-call overload resolution |
| **`runtime.duffcopy`** | **15% (flat)** | **copying the 184-byte `Value` struct** |
| GC (`gcBgMarkWorker`+`mallocgc`+scan) | ~35–40% | driven by the alloc traffic |
| `traceSigStr` | 5% | trace-string build **with tracing off** (now fixed, below) |
| `duffzero`+`memclrNoHeapPointers` | ~7% (flat) | zeroing fresh `Value`s / tapes |

### Allocation attribution (alloc_space, cumulative)

| where | ~% bytes | why it allocates every time |
|---|---:|---|
| `NewTapeWith` (via paren / sub-eval) | 13% | a fresh tape per sub-evaluation; the sub-engine pool does not cover the paren-operand path |
| `Registry.Lookup` → `DefTable.Stack` + `aggregateDispatch` | ~14% | **every** word dispatch re-walks the def-stack and rebuilds the overload table from scratch |
| `effectiveResolved` + `rearrangeForForward` | ~19% | forward collection copies the resolved-stack slice |
| `DefTable.Snapshot` | ~5% | a per-fn-call scope-depth map snapshot |
| `stepMove`/`stepMoveCont` | ~8% | mark/move plumbing for forward args |

## Root causes (ranked)

1. **The `Value` struct is 184 bytes and is copied by value
   everywhere.** `Value` inlines the full type-lattice metadata
   (`Name, FixedID, Rank, Depth, In, Out, Origin, Behavior`, …) that is
   only meaningful on *type nodes* — a tiny fraction of runtime values.
   Every stack slot, tape cell, and argument is a 184-byte copy, which
   is exactly the 15% `duffcopy` + 7% zeroing and a large share of the
   scan cost. A dynamic-language value is normally a tagged word
   (8–16 bytes). This is the single biggest structural cost and it taxes
   the compiled VM too.

2. **Dispatch re-does per call what never changes.**
   `Registry.Lookup(name)` rebuilds the overload table (`DefTable.Stack`
   copy + `aggregateDispatch`) on *every* dispatch of `add`, `if`,
   `fib`, … even inside a hot loop where the bindings are constant. This
   is ~14% of allocations and feeds `matchSignature`. A per-name
   dispatch cache keyed on the def-stack version would eliminate it for
   the steady state.

3. **Forward-argument collection is allocation-heavy.**
   `resolveForwardArgs` + `rearrangeForForward` + `effectiveResolved`
   copy the resolved stack and run the mark/move machinery for every
   word that collects a forward arg — ~19% of allocations. The bytecode
   VM removes this entirely by baking arg positions at compile time; the
   interpreter re-plans it on every execution.

4. **Paren operands spin up a fresh sub-tape each time.**
   `evalParenGroupAt`/sub-eval reaches `NewTapeWith` (13% of bytes). The
   sub-engine pool (`Registry.enginePool`, added 2026-07) covers
   `InvokeBody`/`autoEvalList` but not the paren-operand path, so
   `(fib (n sub 1))` pays a tape allocation per operand — brutal for
   paren-dense recursion, which is why `fib` is the worst shape.

5. **Fn-call frames cost ~340 allocations each** (scope snapshot +
   args-stack + baseline + tape). TCO capped tail-recursion *space* but
   not per-call *setup*; this is why the interp↔Python gap is ~3,000× on
   recursion vs ~150× on loops.

6. **Dead work on the hot path.** `traceSigStr` (engine.go:2368,2393)
   builds a formatted signature string on every dispatch regardless of
   whether a trace hook is installed — yet `traceNote` is only read when
   `e.trace != nil` (engine.go:909). ~5% CPU plus string allocs, pure
   waste. Prototyped fix: gate both `traceNote` assignments on
   `e.trace != nil`; measured **~10–17% faster** on `arith_chain64`/
   `for_tight` and ~500 fewer allocs/op with trace tests unaffected. Not
   applied in this commit — the coverage allowlist
   (`test/go/covergate/allowlist.tsv`) is line-numbered against
   engine.go, so the edit shifts and must regenerate it; that belongs in
   a focused perf change, not this investigation. Same family:
   `GenerateID`/`NewValueRaw` mint an ID per value (~4%) — lazy IDs would
   help.

## Recommendations (impact × effort)

Cheap, interpreter-local, no compiler needed — these directly move the
interp column:

- **Gate `traceSigStr`/`traceNote` on an active trace hook.**
  ~10–17% on dispatch-hot shapes; prototyped and measured (see cause
  #6). Requires regenerating the line-numbered coverage allowlist.
- **Cache `Registry.Lookup` per name** against a def-stack version
  counter; invalidate on `def`/`undef`. Removes ~14% of allocations and
  the repeated `aggregateDispatch`. Highest value-for-effort.
- **Pool the paren-operand sub-eval tape** the same way `InvokeBody`
  was pooled (side-finding #2 in `design/aql-bytecode-baseline.0.md`,
  still open). Removes the 13% `NewTapeWith` and helps recursion most.
- **Lazy / cheaper value IDs** — don't mint an ID until something needs
  one.

Structural, larger, pays back across interp *and* VM:

- **Shrink `Value`.** Split the type-lattice metadata out of the
  runtime value (a `*Type` already exists — move `Name/FixedID/Rank/
  Depth/In/Out/Origin/Behavior` behind it and keep `Value` to
  `Parent + Data + a few flags`). Targets the 15% `duffcopy` + zeroing +
  scan across the whole engine. Biggest single lever; also the most
  invasive (touches every `Value` literal).
- **Memoize forward-collection plans** per call site, or lower more of
  the mark/move machinery — the interpreter re-plans what the VM bakes
  once.

Strategic:

- The compiled VM is already ~15× of CPython and most of that is
  startup/compile. If the goal is "AQL fast", **widen what compiles**
  and make compile the default for scripts (it already is) rather than
  chasing the interpreter all the way to CPython — the tree-walker's
  ceiling is bounded by the 184-byte value and re-planned dispatch.
  Getting the interpreter into Python's ballpark realistically needs
  #1–#4 *and* the `Value` shrink.

## Reproduce

```bash
# cross-language wall-clock
cd cmd/go && make build
AQL=$PWD/bin/aql bash ../../bench/interp/run.sh 5

# interp vs compiled microbenchmarks + profiles
cd lang/go
go test -bench 'BenchmarkStage6' -benchmem -run '^$' .
go test -bench 'BenchmarkStage6/recursion_nontail/interp' -run '^$' \
  -benchtime 2s -cpuprofile /tmp/cpu.prof -memprofile /tmp/mem.prof .
go tool pprof -top -cum /tmp/cpu.prof
go tool pprof -top -cum -sample_index=alloc_space /tmp/mem.prof
```
