.PHONY: test vet fmt lint vuln lint-assertions clean

# This module (github.com/aql-lang/aql/lang) is the AQL language
# library. The command-line tools (aql, aqlweb, aqlwasm, genhelp,
# solardemo) live in the sibling cmd/go module — build them with
# `make -C ../cmd/go build`.
#
# The targets below also run the eng module (kernel + parser + kernel
# spec runner), the test/go module (shared TSV spec-runner scaffolding,
# renamed from util/go), and the cmd/go module so `make test` exercises
# the whole repo.

test:
	go test ./...
	cd ../eng/go && go test ./...
	cd ../test/go && go test ./...
	cd ../cmd/go && go test ./...

vet:
	go vet ./...
	cd ../eng/go && go vet ./...
	cd ../test/go && go vet ./...
	cd ../cmd/go && go vet ./...

fmt:
	gofmt -w .
	cd ../eng/go && gofmt -w .
	cd ../test/go && gofmt -w .
	cd ../cmd/go && gofmt -w .

# golangci-lint across all four modules (config: ../.golangci.yml,
# found by walking up from each module dir). Install:
#   curl -sSfL https://raw.githubusercontent.com/golangci/golangci-lint/HEAD/install.sh \
#     | sh -s -- -b "$$(go env GOPATH)/bin" v2.5.0
lint:
	golangci-lint run ./...
	cd ../eng/go && golangci-lint run ./...
	cd ../test/go && golangci-lint run ./...
	cd ../cmd/go && golangci-lint run ./...

# Known-vulnerability scan (reachability-aware) across all four modules.
# Install: go install golang.org/x/vuln/cmd/govulncheck@latest
vuln:
	govulncheck ./...
	cd ../eng/go && govulncheck ./...
	cd ../test/go && govulncheck ./...
	cd ../cmd/go && govulncheck ./...

# Fail if any non-test runtime handler uses a single-value .Data.(Type)
# assertion (panics on mismatch). Two-value (v, ok := ...) is safe.
lint-assertions:
	@echo "Checking for unsafe .Data.(Type) assertions in runtime handlers..."
	@HITS=$$(find engine -name 'native_*.go' ! -name '*_test.go' \
		-exec grep -Hn '\.Data\.(' {} \; \
		| grep -v ',.*:=' | grep -v '\.(type)' \
		| grep -v '// lint:allow-assert'); \
	if [ -n "$$HITS" ]; then \
		echo "FAIL: unsafe .Data.(Type) assertions found:"; \
		echo "$$HITS"; \
		echo "Use the two-value form: v, ok := x.Data.(Type)"; \
		exit 1; \
	fi
	@echo "OK: no unsafe assertions found"

clean:
	go clean -testcache
