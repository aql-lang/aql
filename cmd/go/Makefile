.PHONY: build run install vet fmt test lint vuln clean wasm

# This module (github.com/aql-lang/aql/cmd/go) holds the AQL
# command-line tools. The language library lives in ../../lang and the
# kernel in ../../eng/go (both wired in via replace directives in go.mod).

BINARY := bin/aql
CMD := ./aql
VERSION := 0.1.0-dev
LDFLAGS := -ldflags "-X main.Version=$(VERSION)"
WASM_DIR := ../../docs
SQLJS_VENDOR := aqlwasm/vendor

build:
	go build $(LDFLAGS) -o $(BINARY) $(CMD)

run: build
	./$(BINARY)

install:
	go install $(LDFLAGS) $(CMD)

test:
	go test ./...

vet:
	go vet ./...

fmt:
	gofmt -w .

# golangci-lint (config: ../../.golangci.yml) and govulncheck.
lint:
	golangci-lint run ./...

vuln:
	govulncheck ./...

clean:
	rm -rf bin/
	go clean -testcache

wasm:
	@mkdir -p $(WASM_DIR)
	GOOS=js GOARCH=wasm go build -o $(WASM_DIR)/aql.wasm ./aqlwasm
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
		sed -n '1,/<script src="sql-wasm.js"><\/script>/{ /<script src="sql-wasm.js"><\/script>/!p }' aqlwasm/index.html; \
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
		sed -n '/<script src="wasm_exec.js"><\/script>/,$${ /<script src="wasm_exec.js"><\/script>/!p }' aqlwasm/index.html; \
	) > $(WASM_DIR)/index.html
	@rm -f $(WASM_DIR)/aql.wasm $(WASM_DIR)/wasm_exec.js $(WASM_DIR)/sql-wasm.js $(WASM_DIR)/sql-wasm.wasm
	@echo "Built: $(WASM_DIR)/index.html ($$(du -h $(WASM_DIR)/index.html | cut -f1))"
