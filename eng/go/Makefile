.PHONY: test vet fmt lint vuln clean

# This module (github.com/aql-lang/aql/eng) is the AQL engine
# kernel + jsonic parser + kernel spec runner. It is standalone — it has
# no dependency on the lang or cmd/go modules.

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
