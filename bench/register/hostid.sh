#!/usr/bin/env bash
# Host fingerprint + id for the performance register
# (design/FULL-COMPILATION.0.md section 14).
#
# Prints one hosts.jsonl record on stdout. The id is
#   "h:" + decimal(fnv64a(canonical-tuple))
# using the SAME hash the knowledge graph uses for identity
# (kg/digest.boru's BinUtil.fnv64, sign bit cleared), so there is one
# hash in this repository rather than two.
#
# The canonical tuple deliberately omits anything that drifts on one
# physical machine: memory is rounded to whole GiB because MemTotal
# moves by a few KB across kernels and containers, which would fork the
# id for the same box. Kernel and OS patch levels ride in the RECORD but
# not in the ID, and every measurement row carries os_version and go
# separately, so drift under a stable id stays visible.
#
# Usage: BORU=cmd/go/bin/boru bench/register/hostid.sh
set -u
BORU="${BORU:-cmd/go/bin/boru}"
[ -x "$BORU" ] || { echo "hostid.sh: no boru binary at $BORU (build with: cd cmd/go && make build)" >&2; exit 1; }

j() { printf '%s' "$1" | sed 's/\\/\\\\/g; s/"/\\"/g'; }

case "$(uname -s)" in
  Linux)
    cpu_model=$(sed -n 's/^model name[[:space:]]*: //p' /proc/cpuinfo | head -1)
    [ -n "$cpu_model" ] || cpu_model=$(sed -n 's/^Model[[:space:]]*: //p' /proc/cpuinfo | head -1)
    cpu_threads=$(grep -c '^processor' /proc/cpuinfo || echo 0)
    cpu_cores=$(sed -n 's/^cpu cores[[:space:]]*: //p' /proc/cpuinfo | head -1)
    [ -n "$cpu_cores" ] || cpu_cores=$cpu_threads
    mem_kb=$(sed -n 's/^MemTotal:[[:space:]]*\([0-9]*\) kB/\1/p' /proc/meminfo)
    kernel=$(uname -r)
    os_name=$(sed -n 's/^NAME="\{0,1\}\([^"]*\)"\{0,1\}$/\1/p' /etc/os-release | head -1)
    os_major=$(sed -n 's/^VERSION_ID="\{0,1\}\([^"]*\)"\{0,1\}$/\1/p' /etc/os-release | head -1)
    if command -v systemd-detect-virt >/dev/null 2>&1; then
      virt=$(systemd-detect-virt 2>/dev/null || echo none)
      [ "$virt" = none ] && virt=bare-metal
    elif grep -q '^flags.*hypervisor' /proc/cpuinfo 2>/dev/null; then
      virt=vm
    else
      virt=unknown
    fi
    [ -f /.dockerenv ] && virt=container
    governor=$(cat /sys/devices/system/cpu/cpu0/cpufreq/scaling_governor 2>/dev/null || echo "")
    ;;
  Darwin)
    cpu_model=$(sysctl -n machdep.cpu.brand_string 2>/dev/null || echo unknown)
    cpu_threads=$(sysctl -n hw.logicalcpu 2>/dev/null || echo 0)
    cpu_cores=$(sysctl -n hw.physicalcpu 2>/dev/null || echo "$cpu_threads")
    mem_kb=$(( $(sysctl -n hw.memsize 2>/dev/null || echo 0) / 1024 ))
    kernel=$(uname -r)
    os_name=macOS
    os_major=$(sw_vers -productVersion 2>/dev/null | cut -d. -f1-2)
    virt=bare-metal
    governor=""
    ;;
  *)
    echo "hostid.sh: unsupported platform $(uname -s)" >&2; exit 1 ;;
esac

arch=$(uname -m)
case "$arch" in x86_64) arch=amd64 ;; aarch64) arch=arm64 ;; esac
mem_gib=$(( mem_kb / 1048576 ))

# The tuple the id hashes. Recorded in the row as id_fields so a future
# change of the tuple is detectable rather than silently re-keying hosts.
id_fields="cpu_model|cpu_cores|mem_gib|os_name|os_major|arch|virt"
canon="${cpu_model}|${cpu_cores}|${mem_gib}|${os_name}|${os_major}|${arch}|${virt}"

hash=$("$BORU" run -e "import \"boru:bin-util\" print (convert String (BinUtil.fnv64 \"$(j "$canon")\"))" 2>/dev/null | tr -d '[:space:]')
[ -n "$hash" ] || { echo "hostid.sh: fnv64 failed" >&2; exit 1; }

printf '{"host":"h:%s","label":"%s","cpu_model":"%s","cpu_cores":%s,"cpu_threads":%s,"mem_kb":%s,"arch":"%s","os_name":"%s","os_major":"%s","kernel":"%s","virt":"%s","governor":"%s","first_seen":"%s","id_fields":"%s"}\n' \
  "$hash" "${REGISTER_LABEL:-$(hostname 2>/dev/null || echo unknown)}" "$(j "$cpu_model")" "$cpu_cores" "$cpu_threads" \
  "$mem_kb" "$arch" "$(j "$os_name")" "$(j "$os_major")" "$(j "$kernel")" "$virt" "$(j "$governor")" \
  "$(date -u +%Y-%m-%dT%H:%M:%SZ)" "$id_fields"
