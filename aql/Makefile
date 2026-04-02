.PHONY: build test clean install run vet fmt wasm

BINARY := bin/aql
CMD := ./cmd/aql
VERSION := 0.1.0-dev
LDFLAGS := -ldflags "-X main.Version=$(VERSION)"
WASM_DIR := ../docs

build:
	go build $(LDFLAGS) -o $(BINARY) $(CMD)

run: build
	./$(BINARY)

test:
	go test ./...

vet:
	go vet ./...

fmt:
	gofmt -w .

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
		cp "$$GOROOT/misc/wasm/wasm_exec.js" $(WASM_DIR)/; \
	elif [ -f "$$GOROOT/lib/wasm/wasm_exec.js" ]; then \
		cp "$$GOROOT/lib/wasm/wasm_exec.js" $(WASM_DIR)/; \
	else \
		echo "error: wasm_exec.js not found in GOROOT"; exit 1; \
	fi
	@echo "Built: $(WASM_DIR)/"
	@echo "Serve with: cd $(WASM_DIR) && python3 -m http.server 8080"
