# `ci/` — staged CI workflow update

This folder holds a CI workflow change that must be applied by hand.

## Why it lives here instead of in `.github/workflows/`

The change was authored by an integration whose GitHub OAuth token lacks
the `workflow` scope, so a push touching `.github/workflows/*` is rejected
by GitHub. Rather than drop the change, it is staged here for a maintainer
(whose credentials carry the `workflow` scope) to move into place.

## What the change is

[`ci/ci.yml`](./ci.yml) is the **full** intended contents of
`.github/workflows/ci.yml`. The differences from the version currently
on `main` are two new steps, added immediately after **Test (all
modules)**:

```yaml
      - name: Cover gate (ADR-008)
        run: make cover-gate

      - name: Knowledge graph (check, test, freshness)
        run: |
          make -C kg check
          make -C kg test
          make -C kg graph
          git diff --exit-code kg/out
```

The second step keeps the committed project knowledge graph
([`kg/out/graph.json`](../kg/out/graph.json)) honest: it type-checks the
boru pipeline, runs its test suite, rebuilds the bundle, and fails the
build if the committed bundle drifts from the deterministic rebuild —
mechanically enforcing the "update the graph with each PR" rule in
[kg/README.md](../kg/README.md).

This enforces ADR-008 (100% coverage across the merged cross-suite profile,
minus the guards marked `//covergate:allow <reason>`) in CI. Those pragmas
live on the guard's own line, so they no longer drift as refactors shift
line numbers — the class of failure that went unnoticed across the #236–#248
merge batch, when the exclusions were a separate line-keyed file and nothing
ran `make cover-gate` on a PR.

## How to apply

```bash
cp ci/ci.yml .github/workflows/ci.yml
git rm -r ci
git add .github/workflows/ci.yml
git commit -m "ci: enforce make cover-gate (ADR-008)"
git push
```

(Or just copy the single `Cover gate (ADR-008)` step into the existing file
if it has diverged in the meantime, and delete this folder.)

## Recommended follow-up

Make the `build-and-test` job a **required status check** on `main` (branch
protection). Right now these PRs have no required checks, so a red CI does
not block a merge — which is how the cover-gate and langspec-ratchet
failures reached `main` in the first place.
