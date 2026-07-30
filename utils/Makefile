# Makefile for utils/ — a coreutils subset written in AQL.
#
# Everything runs FROM THIS DIRECTORY: AQL resolves relative imports against
# the process working directory, so `cd utils && make` is the contract. The
# `aql` binary comes from cmd/go (make -C ../cmd/go build).
#
# This directory is NOT in the repo's Go MODULES list, so nothing here is
# reached by `make test` at the root. That is why cmd/go carries a Go test
# (cmd/go/internal/build/utils_e2e_test.go) that builds and runs two of these
# programs end to end: a top-level AQL directory outside the Go fan-out rots
# otherwise, which is the lesson kg/ taught.

AQL ?= ../cmd/go/bin/aql
BIN ?= bin

PROGS := $(patsubst %.aql,%,$(wildcard *.aql))

.PHONY: all build check fmt test clean

all: fmt check test

# Ensure the aql binary exists.
build-aql:
	$(MAKE) -C ../cmd/go build

# Static type-check every program and every test.
#
# `aql build` does NOT check (NUR044), so this target is the only thing
# standing between a typo and a shipped binary.
check:
	@for f in *.aql tests/*.aql; do \
	  echo "check $$f"; \
	  $(AQL) check $$f || exit 1; \
	done

# Canonical formatting, in place. Part of `all` so the sources cannot drift:
# the formatter is idempotent, so once canonical this leaves the tree clean.
fmt:
	@for f in *.aql tests/*.aql; do \
	  $(AQL) fmt $$f || exit 1; \
	done

# Run every suite (each file exits non-zero on failure).
test:
	@for f in tests/*_test.aql; do \
	  echo "== $$f"; \
	  $(AQL) test $$f || exit 1; \
	done

# Build every program into bin/. Each is a standalone executable that needs
# no aql on the target machine.
binaries: $(BIN)
	@for p in $(PROGS); do \
	  echo "build $$p"; \
	  $(AQL) build $$p.aql -o $(BIN)/$$p || exit 1; \
	done

$(BIN):
	mkdir -p $(BIN)

clean:
	rm -rf $(BIN)
