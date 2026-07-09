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
# Every command's exit status and output are checked: a fixture that
# exits non-zero (a runtime error, a missing runtime) aborts the run
# rather than being silently recorded as a tiny "best" time, and the
# five languages' outputs for each workload must match (the fixtures are
# equivalent programs — a divergent result means a broken fixture, not a
# valid measurement).
#
# Usage:  AQL=/path/to/aql bench/interp/run.sh [reps]
set -u
here="$(cd "$(dirname "$0")" && pwd)"
fix="$here/fixtures"
AQL="${AQL:-aql}"
reps="${1:-3}"
workloads=(fib loopsum nestloop)

die() { echo "run.sh: $*" >&2; exit 1; }

# run once, capturing stdout; abort on non-zero exit. Prints trimmed stdout.
run_capture() {
  local label="$1"; shift
  local out status
  out=$("$@" 2>&1)
  status=$?
  if (( status != 0 )); then
    die "[$label] command failed (exit $status): $*
$out"
  fi
  # trim trailing whitespace/newlines for a stable comparison
  printf '%s' "${out%$'\n'}"
}

# best-of-N wall time in milliseconds for a command (already verified to
# succeed by a preceding run_capture; still aborts if a rep errors).
best_ms() {
  local label="$1"; shift
  local best=99999999 t start end status
  for _ in $(seq 1 "$reps"); do
    start=$(date +%s.%N)
    "$@" >/dev/null 2>&1
    status=$?
    end=$(date +%s.%N)
    (( status != 0 )) && die "[$label] command failed mid-timing (exit $status): $*"
    t=$(awk "BEGIN{printf \"%.0f\", ($end-$start)*1000}")
    (( t < best )) && best=$t
  done
  echo "$best"
}

printf "%-10s %12s %12s %12s %12s %12s\n" workload aql-interp aql-compiled python ruby node
printf "%-10s %12s %12s %12s %12s %12s\n" -------- ---------- ------------ ------ ---- ----
for w in "${workloads[@]}"; do
  # Verify each command succeeds and all five outputs agree before timing.
  ref=$(run_capture "$w/aql-interp" env AQL_NO_COMPILE=1 "$AQL" run "$fix/$w.aql")
  declare -A got=(
    [aql-compiled]="$(run_capture "$w/aql-compiled" "$AQL" run "$fix/$w.aql")"
    [python]="$(run_capture "$w/python" python3 "$fix/$w.py")"
    [ruby]="$(run_capture "$w/ruby" ruby "$fix/$w.rb")"
    [node]="$(run_capture "$w/node" node "$fix/$w.js")"
  )
  for lang in aql-compiled python ruby node; do
    [[ "${got[$lang]}" == "$ref" ]] || \
      die "[$w] $lang output ${got[$lang]@Q} != aql-interp ${ref@Q} — fixtures diverge"
  done

  ai=$(best_ms "$w/aql-interp" env AQL_NO_COMPILE=1 "$AQL" run "$fix/$w.aql")
  ac=$(best_ms "$w/aql-compiled" "$AQL" run "$fix/$w.aql")
  py=$(best_ms "$w/python" python3 "$fix/$w.py")
  rb=$(best_ms "$w/ruby" ruby "$fix/$w.rb")
  nd=$(best_ms "$w/node" node "$fix/$w.js")
  printf "%-10s %10sms %10sms %10sms %10sms %10sms\n" "$w" "$ai" "$ac" "$py" "$rb" "$nd"
done
