#!/usr/bin/env bash
# Cross-language interpreter-speed harness.
#
# Runs three equivalent workloads (fib, loopsum, nestloop) under:
#   - AQL interpreter   (AQL_NO_COMPILE=1)
#   - AQL bytecode VM    (default)
#   - CPython, Ruby, Node
# and reports the best-of-N wall-clock time per cell, plus the
# interp/compiled and interp/python ratios.
#
# Usage:  AQL=/path/to/aql bench/interp/run.sh [reps]
set -u
here="$(cd "$(dirname "$0")" && pwd)"
fix="$here/fixtures"
AQL="${AQL:-aql}"
reps="${1:-3}"
workloads=(fib loopsum nestloop)

# best-of-N wall time in milliseconds for a command
best_ms() {
  local best=99999999 t
  for _ in $(seq 1 "$reps"); do
    local start end
    start=$(date +%s.%N)
    "$@" >/dev/null 2>&1
    end=$(date +%s.%N)
    t=$(awk "BEGIN{printf \"%.0f\", ($end-$start)*1000}")
    (( t < best )) && best=$t
  done
  echo "$best"
}

printf "%-10s %12s %12s %12s %12s %12s\n" workload aql-interp aql-compiled python ruby node
printf "%-10s %12s %12s %12s %12s %12s\n" -------- ---------- ------------ ------ ---- ----
for w in "${workloads[@]}"; do
  ai=$(AQL_NO_COMPILE=1 best_ms "$AQL" run "$fix/$w.aql")
  ac=$(best_ms "$AQL" run "$fix/$w.aql")
  py=$(best_ms python3 "$fix/$w.py")
  rb=$(best_ms ruby "$fix/$w.rb")
  nd=$(best_ms node "$fix/$w.js")
  printf "%-10s %10sms %10sms %10sms %10sms %10sms\n" "$w" "$ai" "$ac" "$py" "$rb" "$nd"
done
