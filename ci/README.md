# ci/

Workflow definitions staged outside `.github/workflows/` because the
automation that authored them cannot write to the workflow path (the
GitHub token lacks the `workflow` scope). Move a file from here into
`.github/workflows/` from a clone with workflow permissions to activate it.

## `pages.yml` — deploy the wasm playground to GitHub Pages

Rebuilds the web playground (`wpg` → `docs/index.html`) on every push to
`main` and publishes it to GitHub Pages via the official Pages deploy
actions, so the live playground tracks the latest engine instead of the
stale committed bundle.

To activate:

1. `git mv ci/pages.yml .github/workflows/pages.yml` (or copy it there)
   and commit.
2. In the repository settings, set **Pages → Build and deployment →
   Source → "GitHub Actions"**. Until then the workflow builds and
   uploads the artifact but the deploy step is a no-op.
