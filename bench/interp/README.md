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

| workload | aql-interp | aql-compiled | python | ruby | node |
|---|---:|---:|---:|---:|---:|
| fib      | 20,757 ms | 418 ms | 23 ms | 83 ms | 51 ms |
| loopsum  |  3,429 ms | 115 ms | 25 ms | 80 ms | 47 ms |
| nestloop |  3,116 ms | 110 ms | 22 ms | 84 ms | 48 ms |

Interpreter is ~28–50× slower than the compiled VM and ~140–900× slower
than CPython (execution-only, after subtracting startup, fib is ~3,000×).
Root-cause analysis and recommendations:
`design/INTERPRETER-SPEED-INVESTIGATION.10.md`.
