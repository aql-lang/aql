# AQL Vendor — `aql vendor` (Design)

**Status: design proposal — not implemented.** A new top-level CLI
command that fetches source from a *separate, language-agnostic
repository space* and **vendors it in** — copies it into the consuming
project's tree, committed alongside the project — so it can distribute
SDKs and libraries for *any* language platform, not just AQL modules.

## 1. Motivation

AQL is increasingly used to **generate SDKs**: from one API description,
emit a typed client for every target ecosystem (TypeScript, Python, Go,
Rust, …). That produces *many* small packages — potentially one per API,
per version, per language.

Publishing that fan-out into each language's own registry is a poor fit:

- **Registries push back on SDK proliferation.** npm, PyPI, crates.io
  and friends are not keen on hosting hundreds of near-identical
  generated SDK packages under one publisher; it reads as spam, strains
  namespaces, and invites rate-limiting or removal.
- **The lifecycles don't match.** A generated SDK's version tracks the
  *API*, not a hand-maintained library release; publishing each to a
  language registry couples two unrelated release cadences.
- **You still need reproducibility.** Consumers want a pinned, verifiable
  copy — not a `curl | unzip` off a wiki.

`aql vendor` offers a third path: host the generated SDKs in a **separate
vendor space** (an AQL-served vendor registry, or plain git repos), and
let each consumer **vendor** the exact SDK they need directly into their
project. No language-registry gatekeeping, but still pinned, hashed, and
reproducible.

## 2. The model

`aql vendor <spec>` resolves `<spec>` to a source, downloads it, verifies
it, and copies its files into the project's `vendor/` tree, recording the
result in the manifest + lockfile. The vendored tree is **committed** to
the consumer's repo and consumed by the **target language's** toolchain
(tsc, Python, cargo, go, …) — AQL never interprets its contents; it only
places them.

This deliberately mirrors two well-worn patterns:

- **Go vendoring** (`go mod vendor`) — dependencies materialised into a
  committed `vendor/` directory for hermetic, offline-reproducible
  builds.
- **Go's registry-less fetching** (`go get github.com/x/y@v1.2.3`) — no
  central index required; a VCS host + a ref + a content hash is enough.

## 3. `vendor` vs `import` / `install`

The existing `import` (runtime) and `install` (CLI,
`cmd/go/internal/install/install.go`) fetch **AQL modules** from the
**AQL module registry** into `.aql/<name>/` — a build cache that is
gitignored and loaded by the AQL runtime via `import "aql:foo"`. They are
registry-only and AQL-only.

`vendor` is a different axis:

| | `install` / `import` | `vendor` |
|---|---|---|
| Payload | AQL module (`.aql` + `aql.jsonic`) | Any-language source (TS, Py, Go, …) |
| Source | AQL module registry (`GET /module/<name>-x.y.z`) | Separate vendor space: vendor registry **or** GitHub/GitLab/git |
| Lands in | `.aql/<name>/` (build cache, gitignored) | `vendor/<…>/` (committed to the project) |
| Consumed by | the AQL runtime (`import`) | the target language's toolchain |
| Recorded in | `aql.jsonic` `deps` | `aql.jsonic` `vendor` + `vendor.lock` |

The two can coexist in one project: an AQL app `install`s AQL modules
*and* `vendor`s, say, a generated TypeScript SDK it ships to a browser
front-end.

(An AQL SDK could also be vendored — as *source* to embed and modify —
rather than installed as an opaque module; both remain available.)

## 4. Sources and resolution

`<spec>` is resolved by a **pluggable source resolver**, chosen by the
spec's shape (the same way `go get` dispatches on the host):

- **Vendor registry (default).** A bare `name` or `name@version`
  resolves against the configured vendor registry — an `aql serve`-able
  service analogous to the module registry, but serving arbitrary-language
  archives keyed by `name` + semver (`GET /vendor/<name>/<version>`).
- **GitHub / GitLab.** `github.com/org/repo@<ref>` /
  `gitlab.com/org/repo@<ref>` resolve to the host's archive endpoint
  (GitHub `…/archive/refs/tags/<ref>.tar.gz`, GitLab
  `…/-/archive/<ref>/…tar.gz`) — no clone needed for a tag/commit. A
  sub-path (`github.com/org/repo/pkg/client@<ref>`) vendors only that
  subtree.
- **Plain git.** `git+https://host/path.git@<ref>` (and `git+ssh://…`)
  clone-at-ref for hosts without an archive API. Shallow, ref-pinned.
- **Direct archive.** `https://…/foo.tar.gz#sha256=…` for a one-off URL
  with an inline integrity pin.

`<ref>` is a tag, a semver (resolved against the source's tags), or a
commit SHA. Absent a ref, resolution takes the latest tag that satisfies
the recorded constraint. Source resolvers are additive, so `bitbucket`,
`sourcehut`, an internal Artifactory, etc. can be added without touching
callers — the "alternate downloads" requirement.

## 5. Manifest and lockfile

Two files, keeping human intent and machine reproducibility separate
(package.json/package-lock, go.mod/go.sum):

- **`aql.jsonic` gains a `vendor` block** — declared intent, editable by
  hand:

  ```jsonic
  vendor: {
    acme-ts-sdk: { source: "vendor:acme/ts-sdk", version: "^2.1.0", into: "vendor/acme-ts-sdk" }
    widget-py:   { source: "github.com/acme/widget-py", version: "v3.0.1" }
  }
  ```

- **`vendor.lock`** — the resolved, verifiable truth, written by the
  tool: for each entry the exact resolved `version`/commit, the concrete
  `source` URL it came from, a `sha256` of the fetched archive (and,
  optionally, a per-tree Merkle hash), and the local `path`. `vendor`
  with no args reproduces exactly from this file; CI can `--frozen` to
  fail on drift.

## 6. On-disk layout

Vendored trees live under a single committed root, namespaced by source
so two SDKs never collide:

```
vendor/
  acme-ts-sdk/            # from vendor registry, `into:` override honoured
  github.com/acme/widget-py/
  gitlab.com/org/thing/
```

The default path is derived from the source (host + path), overridable
per entry with `into:` so it can drop straight into a place the target
toolchain already looks (e.g. a TS `paths` mapping, a Python namespace
dir, a Go module `replace` target). AQL writes files and nothing else;
how the tree is wired into a build is the target ecosystem's concern
(§9).

## 7. Command surface

```
aql vendor <spec> [--version=<v>] [--into=<dir>] [--source=<url>] [--frozen]
aql vendor                 # sync every entry from vendor.lock (reproduce)
aql vendor update [<name>] # re-resolve to the latest allowed version, relock
aql vendor list            # show vendored packages, versions, sources
aql vendor rm <name>       # drop from vendor/ + manifest + lock
aql vendor verify          # re-hash vendor/ against vendor.lock (tamper check)
```

- `aql vendor github.com/acme/widget-py@v3.0.1` — add + fetch + record.
- `aql vendor` — hermetic reproduce (the CI / fresh-clone path).
- `--frozen` — never re-resolve; fail if the lock is missing/stale.

## 8. Fetch and safety plumbing

Reuse the machinery `install` already proved
(`cmd/go/internal/install/install.go`): a bounded download
(`io.LimitReader` with a size cap so a hostile source can't stream an
unbounded body), an HTTP timeout, and — critically — **archive-slip
protection** on every entry via `filepath.IsLocal` (rejects `..`,
absolute, and volume-relative paths) before writing under the
destination. Extend the extractor from zip-only to **zip + tar.gz** (git
hosts serve tarballs). Everything lands atomically (extract to a temp dir,
verify the hash, then swap into `vendor/…`) so a failed fetch never leaves
a half-written tree.

## 9. Language integration

AQL stays language-agnostic: it delivers files and records provenance.
Consuming a vendored tree is per-ecosystem, and the design leaves a
**post-vendor hook** seam for it (a declared command run in the vendored
dir after placement, recorded but sandboxed under the CLI permission
model):

- **TypeScript / Node** — map `vendor/<name>` via `tsconfig.json`
  `compilerOptions.paths`, or a `package.json` `file:` dependency.
- **Python** — a namespace package under a vendored `src/` on the path,
  or an editable `pip install -e vendor/<name>`.
- **Go** — a `replace <module> => ./vendor/<name>` in `go.mod`.
- **Rust** — a `[patch]` / path dependency in `Cargo.toml`.

The hook is optional; the default is "files are placed, you wire them"
(the honest, minimal contract). Auto-wiring per ecosystem is a follow-up.

## 10. The vendor repository space

The default source is a **vendor registry** — deliberately *separate*
from the AQL module registry so SDK fan-out never pollutes the module
namespace (the whole point of §1). It is another service composable under
`aql serve` (alongside `registry`), exposing:

- `GET /vendor/<name>/<version>` → the archive (tar.gz), plus a
  `GET /vendor/<name>` index of available versions;
- `POST /api/vendor/publish` → upload an archive for `name`+`version`
  (auth as with module publish);
- content addressed + hash-pinned, so a mirror or a plain object store
  (S3/GCS static hosting) can serve it read-only.

Because §4 also accepts git hosts directly, the registry is a
convenience, not a requirement: an org can host SDKs as tagged GitHub
repos and skip running a service at all.

## 11. Security model

Vendored code is committed and executed by the target toolchain, so treat
it as a supply-chain dependency:

- **Integrity.** Every fetch is hashed (`sha256` of the archive) and
  checked against `vendor.lock`; a mismatch aborts. `aql vendor verify`
  re-hashes the on-disk tree to catch post-vendor tampering.
- **Pinning.** The lock records a concrete commit/tag, never a floating
  branch; `--frozen` enforces it in CI.
- **Path safety.** `filepath.IsLocal` guard on extraction (§8).
- **Provenance.** The lock records the exact resolved source URL + ref +
  hash, so an audit can answer "where did this come from."
- **Untrusted hooks.** A §9 post-vendor hook runs under the existing
  policy/permission model (`--perms`), off by default, never implicitly.
- **Future.** Optional signature verification (sigstore-style) and a
  `--require-signed` gate.

## 12. Entry-point wiring

`aql vendor` is a new one-shot subcommand beside `install` / `publish` in
the CLI dispatch, in its own `cmd/go/internal/vendor/` package. It reuses
`pathutil` (tilde/path handling), the bounded-fetch + archive-slip
helpers factored out of `install`, and the `command` interface every
subcommand implements. `aql help` / `CLI.md` / the Diátaxis reference
gain an entry; no runtime or engine change is required — this is purely
CLI + a new optional service.

## 13. Open questions / non-goals

- **Transitive vendoring.** Does a vendored package declare its *own*
  `vendor` deps that we recurse into, or is vendoring flat by design?
  (Lean flat first; the generated-SDK use case rarely nests.)
- **Version constraints.** Full semver ranges vs exact pins only.
  (Start with exact + `^`/`~`; resolve against source tags.)
- **Workspaces / monorepos.** A shared vendor root across sub-projects.
- **Update ergonomics.** `aql vendor update` policy — latest tag, latest
  within range, or interactive.
- **Non-goal:** AQL does **not** build, type-check, or run the vendored
  code. It fetches, verifies, places, and records. The target
  ecosystem's toolchain owns the rest.
- **Non-goal (v1):** signature verification and mirror federation —
  designed for (§11) but not required to ship the core fetch/vendor loop.
