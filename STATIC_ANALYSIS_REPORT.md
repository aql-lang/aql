# Static analysis — setup and findings

This documents the static-analysis tooling wired into the repo and the
findings from the first run. Three Go modules are covered: `eng/go`
(engine kernel + parser), `lang` (language library), `cmd/go` (CLI tools).

## What was added

| Tool | How it runs | Where |
| --- | --- | --- |
| **golangci-lint** (v2.5.0) | `make lint` — runs `golangci-lint run ./...` in all three modules | new `lint` target in `lang/Makefile`, `cmd/go/Makefile`, and the new `eng/go/Makefile`; new step in `.github/workflows/ci.yml` |
| **govulncheck** | `make vuln` — runs `govulncheck ./...` in all three modules | new `vuln` target in the same Makefiles; new (advisory) step in CI |
| `lint-assertions` (pre-existing grep check) | `make lint-assertions` | unchanged, still in `lang/Makefile` and CI |

`golangci-lint`'s config lives at the repo root (`.golangci.yml`) and is
found by walking up from each module directory. Local install:

```bash
curl -sSfL https://raw.githubusercontent.com/golangci/golangci-lint/HEAD/install.sh \
  | sh -s -- -b "$(go env GOPATH)/bin" v2.5.0
go install golang.org/x/vuln/cmd/govulncheck@latest
```

## golangci-lint configuration (`.golangci.yml`)

Deliberately conservative — meant to gate CI without churn:

- **Enabled:** the `standard` preset (`errcheck`, `govet`, `ineffassign`,
  `staticcheck`, `unused`) plus `bodyclose`, `misspell`, `unconvert`. The
  `gofmt` formatter is checked.
- **Tuned:**
  - `staticcheck` keeps golangci-lint's default check set but additionally
    drops the `QF*` quickfix *suggestions* (De-Morgan rewrites,
    tagged-switch hints) — useful in review, too stylistic to gate on.
  - `errcheck.exclude-functions` skips a few calls where ignoring the
    error is the established convention: `(*database/sql.Tx).Rollback`,
    `(net/http.ResponseWriter).Write`, `(*encoding/json.Encoder).Encode`.
  - `_test.go` files are exempt from `errcheck` and `ineffassign` (test
    setup routinely discards errors; spec-runner fixtures intentionally
    discard the always-nil error from already-type-matched `AsX()`).
  - `unused` is suppressed for a handful of files that carry parked /
    work-in-progress dead symbols — `lang/engine/{native_query,query,
    native_string_helpers,native_temporal_await}.go` (query-builder
    rework, unit-aware string helpers, the await ordering field) — and for
    the `lang/engine/aliases.go` re-export shim, which intentionally
    mirrors the whole `eng` API. **These exclusions are debt; remove them
    once the parked code lands or is deleted.**
- **Not enabled (opt-in / ad-hoc):** `gosec`, `revive`/`stylecheck`,
  `gocritic`, `errorlint`, `nilerr`, `nilaway`. See the gosec section
  below; `nilerr` was evaluated and left out (its three hits are all
  deliberate error-swallowing — verify, then `//nolint` or fix if you
  want it on).

## golangci-lint findings on first run

All findings on the existing code were either fixed or grandfathered via
config. After this change `golangci-lint run ./...` is clean (0 issues)
in all three modules.

### Fixed

| Where | Linter | Fix |
| --- | --- | --- |
| `eng/go/engine.go` | `unused` | deleted dead `(*Engine).peekForwardValue`, `(*Engine).resolvedStackBefore` |
| `eng/go/parser/parse.go` | `unused` | deleted dead `isWhitespace` |
| `eng/go/carrier.go:442` | `ineffassign` | removed dead `bestHasFn = true` (followed by `break`) |
| `eng/go/engine.go` (stepCloseParen) | `ineffassign` | removed dead `closeIdx--` (overwritten by `findCloseParenAfter` two lines down) |
| `eng/go/engine.go:612` | `errcheck` | `_ = e.stepOpenParen()` — it never returns a non-nil error |
| `eng/go/nativefunc.go:72` | `staticcheck` S1016 | `//nolint:staticcheck` — the explicit `NativeSig`→`Signature` field copy is intentional |
| `lang/engine/native_array.go:1164`, `lang/native/natives.go:554`, `lang/engine/mutability_test.go:227` | `staticcheck` ST1023 | dropped the redundant explicit type in `var x T = …` |
| `lang/native/listops.go:37,73` | `staticcheck` S1009 | dropped the `nil` check before `len()` |
| `lang/internal/nativemod/time.go:233` | `errcheck` | `_, _ = fmt.Sscanf(…)` — best-effort parse of an already-validated digit run |
| `lang/engine/mutability_test.go:42`, `lang/test/object_type_test.go:1522` | `staticcheck` SA4006 | dropped / `_`-ed the dead intermediate `result` assignment |
| `cmd/go/aql/install.go:91` | `errcheck` | check `os.MkdirAll` for the directory-entry case and bail on error |
| `cmd/go/aql/auth.go:588` | `errcheck` | `_ = json.Unmarshal(…)` with a comment — best-effort parse of a 201 body |
| `cmd/go/solardemo/main.go` (×4) | `staticcheck` ST1013 | `405` → `http.StatusMethodNotAllowed` |

### Grandfathered via config (debt to triage)

- **`unused` (~31 symbols)** in `lang/engine/native_query.go` and
  `lang/engine/query.go` — a query-builder feature that's only partly
  wired up (`offsetHandler`, `distinctHandler`, `groupListHandler`,
  `groupAtomHandler`, `havingHandler`, `onHandler`, `usingHandler`,
  `joinWordNative`, `setOpWordNative`, `QueryBuilder.clone`,
  `toQueryBuilder`, `doSelect`, `resolveSelectSubExprs`,
  `resolveWhereSubExprs`, …). Either finish the feature or delete the
  dead handlers, then drop the exclusion.
- **`unused`** in `lang/engine/native_string_helpers.go` (`toGraphemes`,
  `strLen`, `strSlice`) and `lang/engine/native_temporal_await.go`
  (`order` field) — small orphans, same treatment.
- **`unused`** in `lang/engine/aliases.go` (6 re-export aliases:
  `lookupDefType`, `resolveDefType`, `parseFnUndefSpec`,
  `outputSigIsConcreteReturns`, `isSigTypeValue`, `outputSigValues`) —
  expected for a "mirror the whole API" shim; leave as-is or prune.
- **`QF1001`/`QF1003`** (~8 spots in `word_name.go`, `native_boolean.go`,
  `native/setpath.go`, `engine/query.go`) — De-Morgan / tagged-switch
  refactor suggestions; suppressed globally, not bugs.

### Looked at but left alone

`nilerr` (not enabled) flags three deliberate error-swallows — worth a
second look but defensible as designed:

- `lang/engine/native_module_module.go` (`loadModuleResources`) — a
  malformed `.aql/aql.json` is treated as "no resources" rather than an
  error. A typo would silently disable a module's resources.
- `lang/engine/native_type.go:321` — a predicate-evaluation error inside
  `x is SomePredicate` yields `false` rather than propagating.

## govulncheck

`make vuln` currently **fails** (exit 3). All reachable findings are in
the **Go standard library** at `go1.24.7` (the version pinned by the `go`
directive); none are in this repo's first-party code or in a reachable
path through a third-party dependency:

- **~19 reachable stdlib advisories** (`GO-2025-401x` … `GO-2026-49xx`) in
  `crypto/x509`, `crypto/tls`, `net/http`, `net/url`, `net`, `os`,
  `encoding/asn1`. Reached via the HTTP-fetch native word, the decision
  module's TLS use, etc. Fixed in patch releases ranging from `go1.24.8`
  through `go1.25.10`.
- **~14 more** in imported packages / required modules that are *not* on a
  reachable call path (govulncheck does not fail on these).

**Recommendation:** bump the `go` directive (and keep it current) — that
clears the ones fixed on the 1.24.x line; the rest need 1.25.x. Until
then the CI `vuln` step is `continue-on-error: true` (advisory). The scan
is still worth keeping: it's the signal for when to update Go and the
guard against a future *reachable* dependency CVE.

## gosec (ad-hoc — not wired into CI/Makefiles)

`gosec` was run once for this report. It is **not** in `make lint` or CI
(too noisy at default settings — G104 unhandled-error and permission-bit
checks dominate), but a periodic manual `gosec ./...` per module is
worthwhile. Findings by module:

### `cmd/go` — the most security-relevant module

- **G305 (zip-slip)** — `cmd/go/aql/install.go:89`, the module-zip
  extractor. There *is* a guard (`if strings.Contains(f.Name, "..")`),
  and `filepath.Join` cleans the path, so the practical risk is low — but
  the canonical pattern (`filepath.Clean`, then require the result to be
  under `destDir`) is more robust. **Worth tightening.**
- **G703 / G304 (path traversal via taint)** — `install.go`,
  `registry.go`, and `auth.go`/`main.go`: module names / file names from
  HTTP requests and CLI args flow into filesystem paths. Same root cause
  as the zip-slip item; review the path-construction sites together.
- **G114 (no server timeouts)** — `cmd/go/aqlweb/main.go:75`,
  `cmd/go/solardemo/main.go:204`, `cmd/go/aql/registry.go:273` use
  `http.ListenAndServe` with no `ReadTimeout`/`WriteTimeout`. Fine for a
  dev/test server; harden (use a configured `http.Server`) if any of
  these is ever exposed.
- **G107 (variable URL in HTTP request)** — `install.go:53` fetches from
  the user-supplied `-r <registry-url>`. Intended behavior.
- **G705 (XSS via taint)** — `registry.go:47` writes a module zip's bytes
  to the response. False positive for binary content; set
  `Content-Type: application/zip` for clarity.
- **G104 (×18), G301/G306 (perm bits ×12)** — unhandled `w.Write` /
  `json.Encode` / `rc.Close` returns, and `0755` dir / `0644` file modes
  (gosec wants `0750`/`0600`). Conventional; not acted on.

### `eng/go`

- **G404 (weak RNG ×2)** — `eng/go/value.go:513,523`: `math/rand` to
  generate object-type IDs (`"T_" + 12 hex chars`). Not a security token;
  fine as a uniqueness source.
- **G115 (integer-overflow conversion ×7)** — `value.go:523,531–536`: the
  `int64`/`uint64`→`byte` truncations that turn random bytes into hex
  digits. Intentional masking.
- **G602 (×2)** — `core_helpers.go:404,411`: gosec's bounds analysis
  being conservative; false positives.

### `lang`

- **G201 (SQL string formatting ×1)** — `lang/engine/sqlite.go:122`:
  `fmt.Sprintf("INSERT INTO %s (%s) VALUES (%s)", quoteIdent(name),
  joinQuoted(columns), …)`. Identifiers are quoted via `quoteIdent` /
  `joinQuoted` and the values use `?` placeholders, so this is not an
  injection vector — but it's the kind of pattern worth a comment.
- **G304 (×1)** — `lang/internal/fileops/fileops.go:33`: `os.ReadFile`
  on a resolved module path. There is resolution logic upstream; gosec
  flags any variable path. Consider `os.Root`-scoped access (Go 1.24).
- **G104 (×2)** — the `tx.Rollback()` calls in `sqlite.go` (now excluded
  in the golangci-lint config too).

## Suggested next steps

1. **Bump the `go` directive** (and keep current) to clear the
   1.24.x-fixed stdlib advisories; flip the CI `vuln` step to blocking
   once it's clean.
2. **Triage the parked dead code** in `lang/engine/native_query.go` /
   `query.go` (finish or delete) and drop the matching `unused`
   exclusions from `.golangci.yml`.
3. **Review the `cmd/go/aql` path-construction sites** (zip extraction,
   registry file serving, module install) — the gosec G305/G703 cluster.
4. **Consider rewriting `lint-assertions`** (the grep for unsafe
   `.Data.(Type)` assertions in `lang/engine/native_*.go`) as a small
   `go/analysis` analyzer so it runs under `golangci-lint` and matches the
   AST instead of source text.
5. Optionally tighten `.golangci.yml` over time — `errorlint`,
   `bodyclose` settings, a curated `gosec` profile, `gofumpt` instead of
   `gofmt`.
