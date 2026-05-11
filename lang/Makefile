.PHONY: build test clean install run vet fmt wasm lint-assertions

BINARY := bin/aql
CMD := ./cmd/aql
VERSION := 0.1.0-dev
LDFLAGS := -ldflags "-X main.Version=$(VERSION)"
WASM_DIR := ../docs
SQLJS_VENDOR := cmd/aqlwasm/vendor

build:
	go build $(LDFLAGS) -o $(BINARY) $(CMD)

run: build
	./$(BINARY)

# Runs the lang test suite plus the eng module's tests (which now
# include the parser and the kernel spec runner — eng is standalone).
test:
	go test ./...
	cd ../eng/go && go test ./...

vet:
	go vet ./...
	cd ../eng/go && go vet ./...

fmt:
	gofmt -w .
	cd ../eng/go && gofmt -w .

# Fail if any non-test runtime handler uses a single-value .Data.(Type)
# assertion (panics on mismatch). Two-value (v, ok := ...) is safe.
lint-assertions:
	@echo "Checking for unsafe .Data.(Type) assertions in runtime handlers..."
	@HITS=$$(find internal/engine -name 'native_*.go' ! -name '*_test.go' \
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
	rm -rf bin/
	go clean -testcache

install:
	go install $(LDFLAGS) $(CMD)

wasm:
	@mkdir -p $(WASM_DIR)
	GOOS=js GOARCH=wasm go build -o $(WASM_DIR)/aql.wasm ./cmd/aqlwasm
	@GOROOT=$$(go env GOROOT); \
	if [ -f "$$GOROOT/misc/wasm/wasm_exec.js" ]; then \
		cp "$$GOROOT/misc/wasm/wasm_exec.js" $(WASM_DIR)/wasm_exec.js; \
	elif [ -f "$$GOROOT/lib/wasm/wasm_exec.js" ]; then \
		cp "$$GOROOT/lib/wasm/wasm_exec.js" $(WASM_DIR)/wasm_exec.js; \
	else \
		echo "error: wasm_exec.js not found in GOROOT"; exit 1; \
	fi
	cp $(SQLJS_VENDOR)/sql-wasm.js $(WASM_DIR)/sql-wasm.js
	cp $(SQLJS_VENDOR)/sql-wasm.wasm $(WASM_DIR)/sql-wasm.wasm
	@echo "Bundling into single HTML file..."
	@SQL_WASM_B64=$$(base64 -w0 $(WASM_DIR)/sql-wasm.wasm); \
	AQL_WASM_B64=$$(base64 -w0 $(WASM_DIR)/aql.wasm); \
	( \
		sed -n '1,/<script src="sql-wasm.js"><\/script>/{ /<script src="sql-wasm.js"><\/script>/!p }' cmd/aqlwasm/index.html; \
		echo '<script>'; \
		cat $(WASM_DIR)/sql-wasm.js; \
		echo '</script>'; \
		echo '<script>'; \
		cat $(WASM_DIR)/wasm_exec.js; \
		echo '</script>'; \
		echo '<script>'; \
		echo "window.__sqlWasmB64 = '$$SQL_WASM_B64';"; \
		echo "window.__aqlWasmB64 = '$$AQL_WASM_B64';"; \
		echo '</script>'; \
		sed -n '/<script src="wasm_exec.js"><\/script>/,$${ /<script src="wasm_exec.js"><\/script>/!p }' cmd/aqlwasm/index.html; \
	) > $(WASM_DIR)/index.html
	@rm -f $(WASM_DIR)/aql.wasm $(WASM_DIR)/wasm_exec.js $(WASM_DIR)/sql-wasm.js $(WASM_DIR)/sql-wasm.wasm
	@echo "Built: $(WASM_DIR)/index.html ($$(du -h $(WASM_DIR)/index.html | cut -f1))"
