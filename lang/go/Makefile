.PHONY: test vet fmt lint vuln lint-assertions clean publish tags

# This module (github.com/aql-lang/aql/lang/go) is the AQL language
# library. The aql CLI lives in the sibling cmd/go module — build it
# with `make -C ../../cmd/go build`. The web playground (wasm + serve)
# lives in ../../wpg.
#
# This Makefile runs targets for this module only. Use the top-level
# Makefile at ../../Makefile to fan out across all modules.

test:
	go test ./...

vet:
	go vet ./...

fmt:
	gofmt -w .

# golangci-lint (config: ../../.golangci.yml, found by walking up).
lint:
	golangci-lint run ./...

# Known-vulnerability scan (reachability-aware).
# Install: go install golang.org/x/vuln/cmd/govulncheck@latest
vuln:
	govulncheck ./...

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

# Publish a new tagged release.
#   make publish V=0.1.0           # eng/go pinned to v0.1.0 as well
#   make publish V=0.1.0 ENG=0.1.0 # explicit eng version
#
# Updates the `require github.com/aql-lang/aql/eng/go` line to the
# pinned eng version (so consumers resolve a real tag), commits, tags
# `lang/go/v$(V)`, pushes. The wildcard `replace` directive keeps local
# development resolving to ../../eng/go regardless of the require version.
publish: test
	@test -n "$(V)" || (echo "Usage: make publish V=x.y.z [ENG=x.y.z]" && exit 1)
	@ENG_VER=$${ENG:-$(V)}; \
	  perl -i -pe 's{(github\.com/aql-lang/aql/eng/go) v\S+}{$$1 v'$$ENG_VER'}' go.mod; \
	  cd ../.. && git add lang/go/go.mod && \
	  (git diff --cached --quiet || git commit -m "lang/go: v$(V) (eng/go v'$$ENG_VER')") && \
	  git tag lang/go/v$(V) && \
	  git push origin main lang/go/v$(V)
	@if command -v gh >/dev/null 2>&1; then \
		cd ../.. && gh release create lang/go/v$(V) --title "lang/go v$(V)" --notes "AQL language layer v$(V)"; \
	fi

# List recent lang/go tags (newest first).
tags:
	@git -C ../.. tag -l 'lang/go/v*' --sort=-version:refname
