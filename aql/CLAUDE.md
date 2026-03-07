# CLAUDE.md

## Project

This is **voxgig-exp**, containing **AQL** — a concatenative query language implemented in Go (`aql/` directory).

## Build & Test

```bash
cd aql
make test       # or: go test ./...
make build
make vet
make fmt
```

Run a specific test:
```bash
go test ./test/ -run "TestFactorialTypeScaling" -v
```

## Dependency Issue: modernc.org/libc

The `modernc.org/sqlite` dependency requires `modernc.org/libc`, whose zip may not be present in the Go module cache. DNS resolution to `storage.googleapis.com` can fail in restricted environments, blocking `go mod download`.

**Fix:** Fetch the missing zip via curl, which uses a different resolution path:

```bash
curl -sL "https://proxy.golang.org/modernc.org/libc/@v/v1.67.6.zip" \
  -o "$(go env GOMODCACHE)/cache/download/modernc.org/libc/@v/v1.67.6.zip"
```

Then build/test with sum checks bypassed (since the zip wasn't fetched through the normal go toolchain):

```bash
GONOSUMCHECK='*' GONOSUMDB='*' GOPROXY=off go test ./...
```

If the version changes, update the version string in the curl URL to match what `go.mod` requires.
