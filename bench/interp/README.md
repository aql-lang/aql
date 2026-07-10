# Interpreter-speed measurement fixtures

Cross-language fixtures that quantify how slow the **AQL interpreter**
(`AQL_NO_COMPILE=1`) is relative to the **bytecode VM** (default) and to
other dynamic-language interpreters (CPython, Ruby, Node). They exist to
turn the "interpreted AQL is orders of magnitude too slow" claim into a
reproducible number and to anchor before/after comparisons for any
interpreter optimization.

## Workloads

Three equivalent programs, one per dispatch character, each producing an
identical result in every language (checked by `run.sh`):

| Fixture | Shape | Stresses |
|---|---|---|
| `fib` | naive recursive `fib(24)` | fn-call frames + paren operands |
| `loopsum` | sum `0..99999` in a `for` loop | dispatch + arithmetic + loop machinery |
| `nestloop` | `300 × 300` nested `for` counting | nested loop / body re-evaluation |

Files: `fixtures/<name>.{aql,py,rb,js}`.

## Running

```bash
AQL=/path/to/aql bench/interp/run.sh [reps]     # default reps=3, best-of-N
```

`run.sh` reports best-of-N wall-clock (ms) per language. Wall-clock
includes process start + parse; startup is ~20 ms for `aql`, ~17 ms for
Python, so subtract it before quoting execution-only ratios (fixtures
are sized so execution dominates on the AQL-interp column).

## Reference snapshot (2026-07, 4-core Xeon 2.80GHz)

Original baseline (before the interpreter-perf work):

| workload | aql-interp | aql-compiled | python | ruby | node |
|---|---:|---:|---:|---:|---:|
| fib      | ~22,000 ms | 418 ms | 23 ms | 83 ms | 51 ms |
| loopsum  |  ~3,950 ms | 115 ms | 25 ms | 80 ms | 47 ms |
| nestloop |  ~3,330 ms | 110 ms | 22 ms | 84 ms | 48 ms |

After the six-cause fix series (`design/INTERPRETER-SPEED-PLAN.10.md`):

| workload | aql-interp | aql-compiled | python | ruby | node |
|---|---:|---:|---:|---:|---:|
| fib      | ~9,100 ms | 380 ms | 20 ms | 68 ms | 38 ms |
| loopsum  | ~2,430 ms |  93 ms | 21 ms | 66 ms | 36 ms |
| nestloop | ~2,160 ms |  86 ms | 18 ms | 65 ms | 37 ms |

After the second-pass series (`design/INTERPRETER-PYTHON-PARITY.10.md` —
frame-skeleton memoization, mode-gated ID elision, scratch/buffer reuse,
trace-gating completion, Equal fast path):

| workload | aql-interp | aql-compiled | python | ruby | node |
|---|---:|---:|---:|---:|---:|
| fib      | ~5,080 ms | 232 ms | 22 ms | 79 ms | 46 ms |
| loopsum  | ~1,510 ms |  65 ms | 23 ms | 77 ms | 44 ms |
| nestloop | ~1,310 ms |  59 ms | 21 ms | 78 ms | 42 ms |

Cumulative: the interpreter is ~4.3× faster than the original baseline on
recursion and ~2.5× on loops; the interp↔compiled execution gap is
~14–23× on most shapes, and allocations per op are down 50–75 % from the
second-pass start. The compiled VM also gained (~15–20 %) from the
ID-elision and type-equality work. Analysis, per-fix notes, and the
closing assessment of what remains (and why the VM is the performance
story): `design/INTERPRETER-PYTHON-PARITY.10.md`.
