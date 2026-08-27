#!/usr/bin/env bash
# Install a Go tool, retrying transient module-fetch failures.
#
# `go install <pkg>@<version>` verifies every module in the tool's own
# dependency graph against sum.golang.org. That is one network round trip per
# checksum tile, none of them retried, and any single one of them failing
# fails the whole CI job before a line of this repository is built.
#
# It happens. On 2026-08-27 the golangci-lint install failed twice in three
# minutes with
#
#   <module>: verifying module: reading https://sum.golang.org/tile/8/…:
#   stream error: stream ID …; INTERNAL_ERROR; received from peer
#
# on DIFFERENT modules and different tiles each time — the signature of a
# degraded service rather than of anything wrong in the tree, since a real
# problem (a bad go.sum, a wrong pin, a deleted module) fails on the same
# module every time. Both runs died in the toolchain install, having never
# reached `make build`.
#
# The fix is a bounded retry, and deliberately nothing more. Suppressing the
# error instead — GONOSUMDB, GONOSUMCHECK, GOPRIVATE widening, GOFLAGS
# -insecure — buys a green run by not verifying checksums, which trades a
# transient CI failure for a supply-chain hole.
#
# Usage:
#   scripts/go-install-retry.sh <package@version> [attempts]
#
# attempts defaults to 3: two retried, then a final unguarded one.
set -euo pipefail

pkg=${1:?usage: scripts/go-install-retry.sh <package@version> [attempts]}
attempts=${2:-3}

for (( attempt = 1; attempt < attempts; attempt++ )); do
  if go install "$pkg"; then
    exit 0
  fi
  delay=$(( attempt * 15 ))
  printf 'go install %s failed (attempt %d/%d) — retrying in %ds\n' \
    "$pkg" "$attempt" "$attempts" "$delay" >&2
  sleep "$delay"
done

# The LAST attempt runs UNGUARDED on purpose. A genuine failure — a version
# that does not exist, a module that was withdrawn, a real checksum mismatch —
# must still fail the job with go's own diagnostic, not with a summary from
# this script that buries what actually went wrong.
go install "$pkg"
