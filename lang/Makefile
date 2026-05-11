.PHONY: test vet fmt lint-assertions clean

# This module (github.com/metsitaba/voxgig-exp/lang) is the AQL language
# library. The command-line tools (aql, aqlweb, aqlwasm, genhelp,
# solardemo) live in the sibling cmd/go module — build them with
# `make -C ../cmd/go build`.
#
# The targets below also run the standalone eng module (the kernel +
# parser + kernel spec runner) and the cmd/go module so `make test`
# exercises the whole repo.

test:
	go test ./...
	cd ../eng/go && go test ./...
	cd ../cmd/go && go test ./...

vet:
	go vet ./...
	cd ../eng/go && go vet ./...
	cd ../cmd/go && go vet ./...

fmt:
	gofmt -w .
	cd ../eng/go && gofmt -w .
	cd ../cmd/go && gofmt -w .

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
