#!/usr/bin/env bash
# hash-identity-probe — is `canon` a sound basis for content hashing?
#
# Usage:  scripts/hash-identity-probe.sh
#
# `design/unison-in-boru-report.0.md` proposes deriving a content hash from
# the ADR-015 canon contract and using it as a definition's identity. This
# wrapper runs the in-language battery (scripts/hash-identity-probe.boru)
# and adds the two checks a single boru process cannot make for itself:
#
#   P9  cross-process determinism — the same expression hashed in two
#       separate processes must produce the same digest.
#   P10 cross-port canon parity — Go and TS must render a value to the
#       same bytes, or the two engines disagree about identity.
#
# Findings are written up in `design/unison-hash-identity-probe.0.md`.
# A FAIL is not a bug report against canon: canon meets its own contract.
# It records a property that content hashing needs and ADR-015 does not
# supply.
set -euo pipefail

repo="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
boru="$repo/cmd/go/bin/boru"
[ -x "$boru" ] || { echo "build the binary first: (cd cmd/go && make build)" >&2; exit 1; }

# --- the in-language battery (P1-P8) ------------------------------------
"$boru" --no-compile "$repo/scripts/hash-identity-probe.boru"

# --- P9: cross-process determinism --------------------------------------
echo
echo "== P9: cross-process determinism — one expression, two processes =="
digest() {
  "$boru" do --no-compile "import \"boru:bin-util\" BinUtil.fnv64 (canon $1)" 2>/dev/null | tail -1
}
for expr in '{a:1 b:2}' '[1 2 3]' "'text'" '(context)'; do
  a="$(digest "$expr")"; b="$(digest "$expr")"
  if [ "$a" = "$b" ]; then printf 'PASS  %-12s %s\n' "$expr" "$a"
  else printf 'FAIL  %-12s %s vs %s\n' "$expr" "$a" "$b"; fi
done
echo "   a Store carries live pointer addresses into its canon, so its digest"
echo "   is not stable across processes — it is not stable across runs at all"

# --- P10: cross-port canon parity ---------------------------------------
echo
echo "== P10: cross-port canon parity — Go and TS must agree byte for byte =="
if ! command -v node >/dev/null 2>&1; then
  echo "SKIP  node not on PATH"
  exit 0
fi
tmp="$(mktemp -d)"; trap 'rm -rf "$tmp"' EXIT

# Floats only: they are where shortest-round-trip formatting differs between
# runtimes, and a hash turns a cosmetic difference into a different identity.
# Every case is written with a decimal point so it parses as a Float on
# the Go side: a bare 1e6 is an Integer, and coercing with `add 0.0` would
# silently turn -0.0 into 0.0 and measure the coercion, not canon.
cases='1.0 0.1 1.0e21 1.5e-7 0.30000000000000004 -0.0 1.7976931348623157e308 5.0e-324 100.0 1000000.0 0.5 123456789.123456789'

"$boru" do --no-compile "canon [$cases]" 2>/dev/null \
  | tail -1 | tr -d '[]' | tr ' ' '\n' > "$tmp/go.txt"

cat > "$tmp/ts.ts" <<EOF
import { canonValue } from "$repo/core/ts/src/canon.ts";
import { newFloat } from "$repo/core/ts/src/value.ts";
for (const c of [$(echo "$cases" | tr ' ' ',')]) console.log(canonValue(newFloat(c)));
EOF
node --experimental-strip-types --no-warnings "$tmp/ts.ts" > "$tmp/ts.txt" 2>/dev/null

if diff -q "$tmp/go.txt" "$tmp/ts.txt" >/dev/null; then
  echo "PASS  all $(wc -l < "$tmp/go.txt" | tr -d ' ') float renderings agree across ports"
else
  echo "FAIL  canon diverges across ports:"
  diff "$tmp/go.txt" "$tmp/ts.txt" | sed 's/^/   /'
fi
echo "   NOTE: every case is a decimal literal so both sides hold a Float."
echo "   Comparing unlike types here measures the test setup, not canon."
