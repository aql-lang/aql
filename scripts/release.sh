#!/usr/bin/env bash
#
# release.sh — publish the boru modules so `go install …@latest` works.
#
# Releases core/go, check/go, compiler/go, eng/go, basic/go, lang/go and
# cmd/go in DEPENDENCY ORDER (the four-piece kernel split —
# design/ENG-FOUR-PIECE.0.md). For each
# module it:
#   1. auto-bumps the PATCH from the module's latest `<module>/vX.Y.Z` tag
#      (or v0.0.1 if the module has never been tagged);
#   2. strips the local `replace github.com/boru-lang/boru/… => ../…` directives
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

# edit_module runs a go.mod-rewriting function only for a real release; in a
# dry run it prints the intent and leaves the working tree untouched.
edit_module() { # $1 = description, $2 = function name
  if [ "$DRY_RUN" = 1 ]; then echo "  [dry-run] $1"; else "$2"; fi
}

# ---- preflight ----
[ -z "$(git status --porcelain)" ] || { echo "ERROR: working tree not clean"; exit 1; }
[ "$(git rev-parse --abbrev-ref HEAD)" = main ] || { echo "ERROR: releases run from main"; exit 1; }
# Fetch tags so next_patch sees releases cut on another machine / in CI —
# otherwise a fresh or shallow checkout recomputes a stale (or duplicate) patch.
git fetch --tags --quiet origin || true

COREV=$(next_patch core/go); CHECKV=$(next_patch check/go); COMPILERV=$(next_patch compiler/go)
ENGV=$(next_patch eng/go); BASICV=$(next_patch basic/go); LANGV=$(next_patch lang/go); CMDV=$(next_patch cmd/go)
echo "==> Releasing  core/go v$COREV  ·  check/go v$CHECKV  ·  compiler/go v$COMPILERV"
echo "==>            eng/go v$ENGV  ·  basic/go v$BASICV  ·  lang/go v$LANGV  ·  cmd/go v$CMDV"

if [ "$DRY_RUN" = 1 ]; then
  echo "==> [dry-run] skipping the full test gate"
else
  echo "==> Full test suite (release gate)"
  make test
fi

# The actual go.mod surgery, run only for a real release (see edit_module).
# NOTE: `go mod tidy` runs with GOWORK=off, where a DEPENDENCY module's own
# replace directives are ignored — so every sibling a module requires must
# already be pinned to a published version, which is why the kernel pieces
# release before eng.
check_pin() { ( cd check/go
  go mod edit -dropreplace=github.com/boru-lang/boru/core/go
  go mod edit -require="github.com/boru-lang/boru/core/go@v$COREV"
  GOFLAGS=-mod=mod GOWORK=off go mod tidy ); }
compiler_pin() { ( cd compiler/go
  go mod edit -dropreplace=github.com/boru-lang/boru/core/go -dropreplace=github.com/boru-lang/boru/check/go
  go mod edit -require="github.com/boru-lang/boru/core/go@v$COREV" -require="github.com/boru-lang/boru/check/go@v$CHECKV"
  GOFLAGS=-mod=mod GOWORK=off go mod tidy ); }
eng_pin() { ( cd eng/go
  go mod edit -dropreplace=github.com/boru-lang/boru/core/go -dropreplace=github.com/boru-lang/boru/check/go -dropreplace=github.com/boru-lang/boru/compiler/go
  go mod edit -require="github.com/boru-lang/boru/core/go@v$COREV" -require="github.com/boru-lang/boru/check/go@v$CHECKV" -require="github.com/boru-lang/boru/compiler/go@v$COMPILERV"
  GOFLAGS=-mod=mod GOWORK=off go mod tidy ); }
basic_pin() { ( cd basic/go
  go mod edit -dropreplace=github.com/boru-lang/boru/core/go -dropreplace=github.com/boru-lang/boru/check/go -dropreplace=github.com/boru-lang/boru/compiler/go
  go mod edit -dropreplace=github.com/boru-lang/boru/eng/go
  go mod edit -require="github.com/boru-lang/boru/eng/go@v$ENGV"
  GOFLAGS=-mod=mod GOWORK=off go mod tidy ); }
lang_pin() { ( cd lang/go
  go mod edit -dropreplace=github.com/boru-lang/boru/core/go -dropreplace=github.com/boru-lang/boru/check/go -dropreplace=github.com/boru-lang/boru/compiler/go
  go mod edit -dropreplace=github.com/boru-lang/boru/eng/go -dropreplace=github.com/boru-lang/boru/basic/go
  go mod edit -require="github.com/boru-lang/boru/eng/go@v$ENGV" -require="github.com/boru-lang/boru/basic/go@v$BASICV"
  GOFLAGS=-mod=mod GOWORK=off go mod tidy ); }
cmd_pin() { ( cd cmd/go
  go mod edit -dropreplace=github.com/boru-lang/boru/core/go -dropreplace=github.com/boru-lang/boru/check/go -dropreplace=github.com/boru-lang/boru/compiler/go
  go mod edit -dropreplace=github.com/boru-lang/boru/eng/go -dropreplace=github.com/boru-lang/boru/basic/go -dropreplace=github.com/boru-lang/boru/lang/go
  go mod edit -require="github.com/boru-lang/boru/eng/go@v$ENGV" -require="github.com/boru-lang/boru/lang/go@v$LANGV"
  # Version lives in cmd/go/main.go (boru/main.go is a thin entrypoint).
  perl -i -pe 's{(^var Version = )"[^"]*"}{$1"'"$CMDV"'"}' main.go
  GOFLAGS=-mod=mod GOWORK=off go mod tidy ); }

# ---- core/go (leaf: no sibling deps, no go.mod change) ----
echo "==> core/go v$COREV"
run "git tag core/go/v$COREV"
run "git push origin core/go/v$COREV"

# ---- check/go (requires core/go) ----
echo "==> check/go v$CHECKV"
edit_module "cd check/go: drop core replace, pin core@v$COREV, go mod tidy" check_pin
run "git add check/go/go.mod check/go/go.sum"
run "git commit -m 'check/go: v$CHECKV (core/go v$COREV)'"
run "git tag check/go/v$CHECKV"
run "git push origin main check/go/v$CHECKV"

# ---- compiler/go (requires core/go + check/go) ----
echo "==> compiler/go v$COMPILERV"
edit_module "cd compiler/go: drop core+check replaces, pin versions, go mod tidy" compiler_pin
run "git add compiler/go/go.mod compiler/go/go.sum"
run "git commit -m 'compiler/go: v$COMPILERV (core/go v$COREV, check/go v$CHECKV)'"
run "git tag compiler/go/v$COMPILERV"
run "git push origin main compiler/go/v$COMPILERV"

# ---- eng/go (requires core/go + check/go + compiler/go) ----
echo "==> eng/go v$ENGV"
edit_module "cd eng/go: drop kernel replaces, pin core/check/compiler, go mod tidy" eng_pin
run "git add eng/go/go.mod eng/go/go.sum"
run "git commit -m 'eng/go: v$ENGV (core/go v$COREV, check/go v$CHECKV, compiler/go v$COMPILERV)'"
run "git tag eng/go/v$ENGV"
run "git push origin main eng/go/v$ENGV"

# ---- basic/go (requires eng/go) ----
echo "==> basic/go v$BASICV"
edit_module "cd basic/go: drop eng replace, pin eng@v$ENGV, go mod tidy" basic_pin
run "git add basic/go/go.mod basic/go/go.sum"
run "git commit -m 'basic/go: v$BASICV (eng/go v$ENGV)'"
run "git tag basic/go/v$BASICV"
run "git push origin main basic/go/v$BASICV"

# ---- lang/go (requires eng/go + basic/go) ----
echo "==> lang/go v$LANGV"
edit_module "cd lang/go: drop eng replace, pin eng@v$ENGV, go mod tidy" lang_pin
run "git add lang/go/go.mod lang/go/go.sum"
run "git commit -m 'lang/go: v$LANGV (eng/go v$ENGV, basic/go v$BASICV)'"
run "git tag lang/go/v$LANGV"
run "git push origin main lang/go/v$LANGV"

# ---- cmd/go (requires eng/go + lang/go) ----
echo "==> cmd/go v$CMDV"
edit_module "cd cmd/go: drop eng+lang replaces, pin versions, stamp Version=$CMDV, go mod tidy" cmd_pin
run "git add cmd/go/go.mod cmd/go/go.sum cmd/go/main.go"
run "git commit -m 'cmd/go: v$CMDV (eng/go v$ENGV, lang/go v$LANGV)'"
run "git tag cmd/go/v$CMDV"
run "git push origin main cmd/go/v$CMDV"

echo "==> Done. Install with:"
echo "    go install github.com/boru-lang/boru/cmd/go/boru@v$CMDV"
