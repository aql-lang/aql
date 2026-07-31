# BORU Vendor — `boru vendor` (Design)

**Status: design proposal — not implemented.** A new top-level CLI
command that fetches source from a *separate, language-agnostic
repository space* and **vendors it in** — copies it into the consuming
project's tree, committed alongside the project — so it can distribute
SDKs and libraries for *any* language platform, not just BORU modules.

## 1. Motivation

BORU is increasingly used to **generate SDKs**: from one API description,
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

`boru vendor` offers a third path: host the generated SDKs in a **separate
vendor space** (a BORU-served vendor registry, or plain git repos), and
let each consumer **vendor** the exact SDK they need directly into their
project. No language-registry gatekeeping, but still pinned, hashed, and
reproducible.

## 2. The model

`boru vendor <spec>` resolves `<spec>` to a source, downloads it, verifies
it, and copies its files into the project's `vendor/` tree, recording the
result in the manifest + lockfile. The vendored tree is **committed** to
the consumer's repo and consumed by the **target language's** toolchain
(tsc, Python, cargo, go, …) — BORU never interprets its contents; it only
places them.

This deliberately mirrors two well-worn patterns:

- **Go vendoring** (`go mod vendor`) — dependencies materialised into a
  committed `vendor/` directory for hermetic, offline-reproducible
  builds.
- **Go's registry-less fetching** (`go get github.com/x/y@v1.2.3`) — no
  central index required; a VCS host + a ref + a content hash is enough.

## 3. `vendor` vs `import` / `install`

The existing `import` (runtime) and `install` (CLI,
`cmd/go/internal/install/install.go`) fetch **BORU modules** from the
**BORU module registry** into `.boru/<name>/` — a build cache that is
gitignored and loaded by the BORU runtime via `import "boru:foo"`. They are
registry-only and BORU-only.

`vendor` is a different axis:

| | `install` / `import` | `vendor` |
|---|---|---|
| Payload | BORU module (`.boru` + `boru.jsonic`) | Any-language source (TS, Py, Go, …) |
| Source | BORU module registry (`GET /module/<name>-x.y.z`) | Separate vendor space: vendor registry **or** GitHub/GitLab/git |
| Lands in | `.boru/<name>/` (build cache, gitignored) | `vendor/<…>/` (committed to the project) |
| Consumed by | the BORU runtime (`import`) | the target language's toolchain |
| Recorded in | `boru.jsonic` `deps` | `boru.jsonic` `vendor` + `vendor.lock` |

The two can coexist in one project: a BORU app `install`s BORU modules
*and* `vendor`s, say, a generated TypeScript SDK it ships to a browser
front-end.

(A BORU SDK could also be vendored — as *source* to embed and modify —
rather than installed as an opaque module; both remain available.)

## 4. Sources and resolution

`<spec>` is resolved by a **pluggable source resolver**, chosen by the
spec's shape (the same way `go get` dispatches on the host):

- **Vendor registry (default).** A bare `name` or `name@version`
  resolves against the configured vendor registry — a `boru serve`-able
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

- **`boru.jsonic` gains a `vendor` block** — declared intent, editable by
  hand:

  ```jsonic
  vendor: {
    acme-ts-sdk: { source: "vendor:acme/ts-sdk", version: "^2.1.0", into: "vendor/acme-ts-sdk" }
    widget-py:   { source: "github.com/acme/widget-py", version: "v3.0.1" }
  }
  ```

- **`vendor.lock`** — the resolved, verifiable truth, written by the
  tool. Each entry records:
  - the concrete `source` URL and, for a VCS source, the **resolved
    commit SHA** — which is what is fetched. A tag/semver is kept only as
    metadata + the update constraint: tags can be force-moved or deleted,
    so a later `--frozen` sync of a *tag* could pull different bytes and
    fail the hash check instead of reproducing. Pin the commit, not the
    tag.
  - a **`tree` hash** — a canonical content hash of the *extracted files*
    (a Merkle hash over sorted relative paths + file bytes + modes),
    **required**, because it is what `boru vendor verify` re-derives from
    the working tree. The archive `sha256` cannot be recomputed from
    extracted files (compression, entry order, and archive metadata are
    lost), so it is recorded too but only guards a *re-download*, never
    the on-disk tree.
  - the local `path`.

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
toolchain already looks (a TS `paths` mapping, a Python namespace dir).
Two guards on `into` (enforced at write time, §8):

- **Containment.** `into` must resolve *inside* the project root — no
  absolute path and no `../` escape. The archive-entry `filepath.IsLocal`
  check guards paths *inside* the archive; it does not guard where the
  tree lands, so the destination is validated separately.
- **Ownership.** The tool only swaps into a directory it *owns* — an
  empty dir, or one carrying an `.boru-vendor` marker from a prior vendor
  of the same entry. It refuses to overwrite a non-empty, unmanaged
  directory (so `into: "src/acme"` can never delete hand-written code),
  and `into` paths must be unique across lock entries.

BORU writes files and nothing else; wiring the tree into a build is the
target ecosystem's concern (§9) — note the Go caveat there: a module
root's `vendor/` is special to the Go toolchain, so Go SDKs default
*outside* `vendor/`.

## 7. Command surface

```
boru vendor <spec> [--version=<v>] [--into=<dir>] [--source=<url>] [--frozen]
boru vendor                 # reconcile: lock+fetch new/changed manifest entries, then materialise
boru vendor update [<name>] # re-resolve to the latest allowed version, relock
boru vendor list            # show vendored packages, versions, sources
boru vendor rm <name>       # drop from vendor tree + manifest + lock
boru vendor verify          # re-derive the tree hash and check it against vendor.lock
```

- `boru vendor github.com/acme/widget-py@v3.0.1` — add + fetch + record.
- `boru vendor` — **reconcile the manifest with the tree**: a `vendor:`
  entry added or bumped by hand is resolved, locked, and materialised;
  unchanged entries reproduce from the lock. (Plain sync must read the
  manifest — syncing only the already-locked entries would silently
  ignore a hand-added dependency.)
- `--frozen` — the CI / fresh-clone path: never re-resolve; fail if the
  manifest and lock disagree, or the lock is missing/stale.

## 8. Fetch and safety plumbing

Reuse the machinery `install` already proved
(`cmd/go/internal/install/install.go`): a bounded download
(`io.LimitReader` with a size cap so a hostile source can't stream an
unbounded body) and an HTTP timeout. Extraction then hardens on three
fronts:

- **Path containment (archive entries).** Every entry name is checked
  with `filepath.IsLocal` (rejects `..`, absolute, and volume-relative
  paths) before writing.
- **Link-aware extraction (tar).** `filepath.IsLocal` on *names* is not
  enough once we accept `tar.gz` (git hosts serve tarballs): a tar can
  carry a symlink/hardlink whose target escapes the temp dir and then a
  later "local-looking" entry *under* that link. The extractor therefore
  **rejects link entries outright** (or, if links must ever be preserved,
  resolves each final path with symlink-aware containment). Zip has no
  link problem; tar does — so the expansion from zip to tar must add this
  explicitly.
- **Destination containment (`into`).** The `into` target is resolved and
  checked to lie within the project root and to be an owned/empty
  directory (§6) *before* the swap — the archive-entry check guards paths
  inside the archive, not where the tree lands.

Everything lands atomically (extract to a temp dir, verify the `tree`
hash, then swap into place) so a failed or tampered fetch never leaves a
half-written tree, and a hash mismatch aborts *before* the swap.

## 9. Language integration

BORU stays language-agnostic: it delivers files and records provenance.
Consuming a vendored tree is per-ecosystem:

- **TypeScript / Node** — map the vendored dir via `tsconfig.json`
  `compilerOptions.paths`, or a `package.json` `file:` dependency.
- **Python** — a namespace package under a vendored `src/` on the path,
  or an editable `pip install -e <dir>`.
- **Go** — **not** under `vendor/`. The Go toolchain treats a module
  root's `vendor/` specially and rejects `replace <mod> => ./vendor/…`
  ("replacement module directory path … is inside vendor directory"), so
  a Go SDK placed there breaks normal module commands. Go SDKs default to
  a non-`vendor/` dir (e.g. `third_party/<name>`, or a `go.work use`),
  with the `replace`/`use` pointing there. This is exactly why the
  default path is source-derived and per-ecosystem overridable (§6)
  rather than hard-wired to `vendor/`.
- **Rust** — a `[patch]` / path dependency in `Cargo.toml`.

A **post-vendor hook** (a declared command run in the vendored dir after
placement — `npm install`, `pip install -e`, …) is an optional seam, off
by default. Its trust boundary must be stated honestly: the CLI
permission model (`--perms`) gates *BORU words* (the fileops / network /
process capability wrappers), it does **not** sandbox the syscalls of a
spawned `npm` / `pip` / shell process once launched. A hook therefore
runs with the user's full privileges and is **explicitly unsandboxed** —
enable it only for trusted vendored code. A real OS-level sandbox
(namespaces / seccomp / a container) for hooks is a future option, not a
guarantee the design makes today. The safe default stays "files are
placed, you wire them."

## 10. The vendor repository space

The default source is a **vendor registry** — deliberately *separate*
from the BORU module registry so SDK fan-out never pollutes the module
namespace (the whole point of §1). It is another service composable under
`boru serve` (alongside `registry`), exposing:

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

- **Integrity.** The fetched archive is hashed (`sha256`) to guard
  re-downloads, and — the load-bearing check — the extracted tree gets a
  canonical `tree` hash pinned in the lock. `boru vendor verify`
  re-derives the tree hash from the working files to catch post-vendor
  tampering; it does not (cannot) re-hash the archive from the extracted
  files (§5). A mismatch aborts before any swap (§8).
- **Pinning.** For VCS sources the lock records and fetches the resolved
  **commit SHA**, never a tag or branch — tags move, commits don't — so
  `--frozen` is genuinely reproducible (§5).
- **Path safety.** Archive-entry containment (`filepath.IsLocal`),
  link-entry rejection for tar, and destination (`into`) containment +
  ownership — all before the atomic swap (§6, §8).
- **Provenance.** The lock records the exact resolved source URL + commit
  + tree hash, so an audit can answer "where did this come from."
- **Hooks are unsandboxed.** A §9 post-vendor hook is off by default and,
  when enabled, runs with full user privileges — `--perms` gates BORU
  words, not a spawned process's syscalls. It is a trusted-code seam, not
  a security boundary; enable only for code you trust.
- **Future.** An OS-level sandbox for hooks, and optional signature
  verification (sigstore-style) with a `--require-signed` gate.

## 12. Entry-point wiring

`boru vendor` is a new one-shot subcommand beside `install` / `publish` in
the CLI dispatch, in its own `cmd/go/internal/vendor/` package. It reuses
`pathutil` (tilde/path handling), the bounded-fetch + archive-slip
helpers factored out of `install`, and the `command` interface every
subcommand implements. `boru help` / `CLI.md` / the Diátaxis reference
gain an entry; no runtime or engine change is required — this is purely
CLI + a new optional service.

## 13. Open questions / non-goals

- **Transitive vendoring.** Does a vendored package declare its *own*
  `vendor` deps that we recurse into, or is vendoring flat by design?
  (Lean flat first; the generated-SDK use case rarely nests.)
- **Version constraints.** Full semver ranges vs exact pins only.
  (Start with exact + `^`/`~`; resolve against source tags.)
- **Workspaces / monorepos.** A shared vendor root across sub-projects.
- **Update ergonomics.** `boru vendor update` policy — latest tag, latest
  within range, or interactive.
- **Non-goal:** BORU does **not** build, type-check, or run the vendored
  code. It fetches, verifies, places, and records. The target
  ecosystem's toolchain owns the rest.
- **Non-goal (v1):** signature verification and mirror federation —
  designed for (§11) but not required to ship the core fetch/vendor loop.
