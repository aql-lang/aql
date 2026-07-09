#!/usr/bin/env bash
#
# release.sh — publish the aql modules so `go install …@latest` works.
#
# Releases eng/go, lang/go and cmd/go in DEPENDENCY ORDER. For each module it:
#   1. auto-bumps the PATCH from the module's latest `<module>/vX.Y.Z` tag
#      (or v0.0.1 if the module has never been tagged);
#   2. strips the local `replace github.com/aql-lang/aql/… => ../…` directives
#      (a published go.mod MUST NOT contain replace directives, or
#      `go install …@version` refuses it);
#   3. pins the sibling `require` lines to the versions just released;
#   4. `go mod tidy` + commits the module's go.mod/go.sum (and cmd/go's
#      main.go version stamp);
#   5. tags `<module>/vX.Y.Z` and pushes main + the tag.
#
# The whole run is gated by a full `make test` up front — nothing is tagged
# unless the entire suite passes. Local development keeps building via the
# committed `go.work` (which supplies in-tree sibling source on top of the
# now-replace-free, real-version go.mods).
#
# Usage:  make release        # or: scripts/release.sh
#         DRY_RUN=1 make release   # print what would happen, tag/push nothing
#
set -euo pipefail
cd "$(dirname "$0")/.."
ROOT=$(pwd)
DRY_RUN=${DRY_RUN:-0}

run() { if [ "$DRY_RUN" = 1 ]; then echo "  [dry-run] $*"; else eval "$*"; fi; }

# next_patch <module-path> — echo the next vMAJOR.MINOR.(PATCH+1) for the
# module's tag namespace, or 0.0.1 when it has never been released.
next_patch() {
  local m=$1 latest v
  latest=$(git tag -l "$m/v*" --sort=-version:refname | head -1)
  if [ -z "$latest" ]; then echo "0.0.1"; return; fi
  v=${latest#"$m"/v}
  echo "${v%%.*}.$(echo "$v" | cut -d. -f2).$(( $(echo "$v" | cut -d. -f3) + 1 ))"
}

# ---- preflight ----
[ -z "$(git status --porcelain)" ] || { echo "ERROR: working tree not clean"; exit 1; }
[ "$(git rev-parse --abbrev-ref HEAD)" = main ] || { echo "ERROR: releases run from main"; exit 1; }

ENGV=$(next_patch eng/go); LANGV=$(next_patch lang/go); CMDV=$(next_patch cmd/go)
echo "==> Releasing  eng/go v$ENGV  ·  lang/go v$LANGV  ·  cmd/go v$CMDV"

echo "==> Full test suite (release gate)"
make test

# ---- eng/go (leaf: no sibling deps, no go.mod change) ----
echo "==> eng/go v$ENGV"
run "git tag eng/go/v$ENGV"
run "git push origin eng/go/v$ENGV"

# ---- lang/go (requires eng/go) ----
echo "==> lang/go v$LANGV"
( cd lang/go
  go mod edit -dropreplace=github.com/aql-lang/aql/eng/go
  go mod edit -require="github.com/aql-lang/aql/eng/go@v$ENGV"
  GOFLAGS=-mod=mod GOWORK=off go mod tidy )
run "git add lang/go/go.mod lang/go/go.sum"
run "git commit -m 'lang/go: v$LANGV (eng/go v$ENGV)'"
run "git tag lang/go/v$LANGV"
run "git push origin main lang/go/v$LANGV"

# ---- cmd/go (requires eng/go + lang/go) ----
echo "==> cmd/go v$CMDV"
( cd cmd/go
  go mod edit -dropreplace=github.com/aql-lang/aql/eng/go -dropreplace=github.com/aql-lang/aql/lang/go
  go mod edit -require="github.com/aql-lang/aql/eng/go@v$ENGV" -require="github.com/aql-lang/aql/lang/go@v$LANGV"
  perl -i -pe 's{(^var Version = )"[^"]*"}{$1"'"$CMDV"'"}' aql/main.go 2>/dev/null || true
  GOFLAGS=-mod=mod GOWORK=off go mod tidy )
run "git add cmd/go/go.mod cmd/go/go.sum cmd/go/aql/main.go"
run "git commit -m 'cmd/go: v$CMDV (eng/go v$ENGV, lang/go v$LANGV)'"
run "git tag cmd/go/v$CMDV"
run "git push origin main cmd/go/v$CMDV"

echo "==> Done. Install with:"
echo "    go install github.com/aql-lang/aql/cmd/go/aql@v$CMDV"
