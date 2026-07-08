# Pending CI change — apply by hand

This folder holds a `.github/workflows/ci.yml` change that could **not** be
pushed from the branch: the OAuth token driving the branch lacks the GitHub
`workflow` scope, so any commit touching `.github/workflows/**` is rejected at
push time. A maintainer (or a token with `workflow` scope) must apply it.

## What it does

Replaces the CI **"Bytecode race gates"** step — which ran the race detector
against three hand-named tests — with a single **`make test-race`** step. The
new lane runs `go test -race -short` over the whole `eng/go` + `lang/go` trees
plus langspec's concurrency rows, so every current and future concurrency test
is race-checked automatically, with no name list to keep in sync. `make
test-race` (the Makefile target and its `-short` calibration) is already on the
branch; this is only the workflow wiring that invokes it.

Verified: `make test-race` runs clean in ~4m17s.

## How to apply

Either apply the patch:

```bash
git apply tmp/ci/ci.yml.patch
```

or copy the ready-made file over the workflow (identical to the current
workflow except for the one step):

```bash
cp tmp/ci/ci.yml .github/workflows/ci.yml
```

Then commit `.github/workflows/ci.yml` and (optionally) delete this folder.

## Files

- `ci.yml`        — the full workflow with the change applied (drop-in replacement)
- `ci.yml.patch`  — unified diff against `.github/workflows/ci.yml`
