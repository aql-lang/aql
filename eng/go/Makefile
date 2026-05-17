.PHONY: test vet fmt lint vuln clean cover

# This module (github.com/aql-lang/aql/eng) is the AQL engine
# kernel + jsonic parser + kernel spec runner. It is standalone — it has
# no dependency on the lang or cmd/go modules.
#
# Code-structure visualisation targets (viz, viz-callgraph, viz-goda,
# viz-gomod, viz-plantuml, viz-list, viz-modgraph, viz-index, viz-clean,
# viz-tools) live in the top-level Makefile (../../Makefile) so the
# whole codebase — every module — can be reviewed from one place. Run
# `make viz` from the repo root to generate them.

test:
	go test ./...

vet:
	go vet ./...

fmt:
	gofmt -w .

# golangci-lint (config: ../../.golangci.yml, found by walking up).
lint:
	golangci-lint run ./...

vuln:
	govulncheck ./...

# Per-module coverage profile. The repo-root `make cover` aggregates
# across all modules.
cover:
	go test -coverprofile=coverage.out ./...
	@go tool cover -func=coverage.out | tail -1

clean:
	go clean -testcache
	rm -f coverage.out
