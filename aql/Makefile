.PHONY: build test clean install run vet fmt

BINARY := bin/aql
CMD := ./cmd/aql
VERSION := 0.1.0-dev
LDFLAGS := -ldflags "-X main.Version=$(VERSION)"

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
