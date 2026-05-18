.PHONY: serve wasm test vet fmt clean

# wpg (github.com/aql-lang/aql/wpg) — the AQL web playground.
#
#   wasm/   AQL engine compiled to WebAssembly (browser-side eval)
#   serve/  Standalone HTTP server (server-side eval; self-contained HTML)
#
# The language library lives at ../lang/go (wired in via go.mod replace).

WASM_DIR := ../docs
SQLJS_VENDOR := wasm/vendor

# Build-info stamps for the playground header (commit / tag / date).
# Fall back gracefully when not in a git checkout.
GIT_COMMIT := $(shell git -C .. rev-parse --short HEAD 2>/dev/null || echo unknown)
GIT_TAG    := $(shell git -C .. describe --tags --abbrev=0 2>/dev/null || echo untagged)
GIT_DATE   := $(shell git -C .. log -1 --format=%cs 2>/dev/null || echo unknown)

serve:
	go run ./serve

wasm:
	@mkdir -p $(WASM_DIR)
	GOOS=js GOARCH=wasm go build -o $(WASM_DIR)/aql.wasm ./wasm
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
	@echo "Bundling into single HTML file (commit $(GIT_COMMIT), tag $(GIT_TAG), date $(GIT_DATE))..."
	@SQL_WASM_B64=$$(base64 -w0 $(WASM_DIR)/sql-wasm.wasm); \
	AQL_WASM_B64=$$(base64 -w0 $(WASM_DIR)/aql.wasm); \
	BUILD_INFO='$(GIT_TAG)<span class="sep">·</span>$(GIT_COMMIT)<span class="sep">·</span>$(GIT_DATE)'; \
	( \
		sed -n '1,/<script src="sql-wasm.js"><\/script>/{ /<script src="sql-wasm.js"><\/script>/!p }' wasm/index.html; \
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
		sed -n '/<script src="wasm_exec.js"><\/script>/,$${ /<script src="wasm_exec.js"><\/script>/!p }' wasm/index.html; \
	) | sed "s|__BUILD_INFO__|$$BUILD_INFO|" > $(WASM_DIR)/index.html
	@rm -f $(WASM_DIR)/aql.wasm $(WASM_DIR)/wasm_exec.js $(WASM_DIR)/sql-wasm.js $(WASM_DIR)/sql-wasm.wasm
	@echo "Built: $(WASM_DIR)/index.html ($$(du -h $(WASM_DIR)/index.html | cut -f1))"

test:
	go test ./...

vet:
	go vet ./...

fmt:
	gofmt -w .

clean:
	go clean -testcache
