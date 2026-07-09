.PHONY: build test vet fmt lint vuln clean

# This module (github.com/aql-lang/aql/test/solardemo) is a standalone
# HTTP fixture used by the lang/go API tests. It has no sibling deps.

build:
	go build ./...

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

clean:
	go clean -testcache
