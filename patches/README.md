# `patches/` — changes that cannot be pushed, staged for a maintainer

A change under `.github/workflows/` cannot be pushed by an integration whose
GitHub OAuth token lacks the `workflow` scope — GitHub rejects the push
outright:

```
! [remote rejected] refusing to allow an OAuth App to create or update
  workflow `.github/workflows/ci.yml` without `workflow` scope
```

Rather than drop such a change, or land half of it, it is staged here as a
**git patch** that a maintainer (whose credentials carry the scope) applies.

## Why a patch and not a copy of the file

`ci/` already stages a workflow change the older way — a full copy of the
intended `.github/workflows/ci.yml`. That file has since gone **stale**, and it
is the argument for this folder existing:

- it predates the `no-binaries` job and the Node setup now on `main`, so
  copying it into place would silently **delete** both;
- its two staged steps were superseded by narrower ones that already landed
  (`make -C kg verify`, and `make cover-gate-core` rather than the full
  `make cover-gate` — a deliberate CI-budget call recorded in the workflow's
  own comments).

A full-file copy captures a whole file at one instant and rots against every
later edit, silently. A patch names only the lines it changes, so `git apply`
**fails loudly** if the surrounding code moved — which is the behaviour you
want from a change that may sit here for a while.

## How to apply

```bash
git apply patches/0001-ci-go-install-retry.patch     # check first: --check
# or, to keep the authorship and message:
git am  patches/0001-ci-go-install-retry.patch
```

Then delete the patch file — a staged change that has landed is the same dead
weight `ci/` became. `git apply --check <file>` verifies applicability without
touching the tree.

## Contents

| patch | what it does | why it is here |
| --- | --- | --- |
| `0001-ci-go-install-retry.patch` | Wraps CI's two `go install` toolchain steps in a bounded retry (`scripts/go-install-retry.sh`), for the `sum.golang.org` outages that have cost four `build-and-test` runs across #409 and #410. | Touches `.github/workflows/ci.yml`. |

`0001` carries **both** halves — the script and the `ci.yml` lines that invoke
it. That is deliberate: `be4f9b0` once landed the script alone (its workflow
half having been refused for this same reason), and `a372519` then deleted it,
correctly — a script nothing invokes reads as a solved problem, which is worse
than an honest gap.
