# Makefile for utils/ — a coreutils subset written in boru.
#
# Everything runs FROM THIS DIRECTORY: boru resolves relative imports against
# the process working directory, so `cd utils && make` is the contract. The
# `boru` binary comes from cmd/go (make -C ../cmd/go build).
#
# This directory is NOT in the repo's Go MODULES list, so nothing here is
# reached by `make test` at the root. That is why cmd/go carries a Go test
# (cmd/go/internal/build/utils_e2e_test.go) that builds and runs two of these
# programs end to end: a top-level boru directory outside the Go fan-out rots
# otherwise, which is the lesson kg/ taught.

BORU ?= ../cmd/go/bin/boru
BIN ?= bin

PROGS := $(patsubst %.boru,%,$(wildcard *.boru))

.PHONY: all build check fmt test clean

all: check test

# Ensure the boru binary exists.
build-boru:
	$(MAKE) -C ../cmd/go build

# Static type-check every program and every test.
#
# `boru build` does NOT check (NUR044), so this target is the only thing
# standing between a typo and a shipped binary.
check:
	@for f in *.boru tests/*.boru; do \
	  echo "check $$f"; \
	  $(BORU) check $$f || exit 1; \
	done

# Canonical formatting, in place — available, but deliberately NOT part of
# `all`, and the sources here are hand-formatted.
#
# NUR046: `boru fmt` is not idempotent. On a `def name fn [[params] [Returns]
# [body]]` whose header does not fit the width — which is most of this
# directory — pass 1 and pass 2 produce different layouts (it converges at
# pass 2). A `fmt` inside `all` would therefore leave a dirty tree on every
# single run, and the intermediate layout runs statements together on one
# line. This is the same posture kg/Makefile held while its own formatter
# blocker was open: keep the target, keep it out of `all`, say why.
#
# Formatting these files does NOT change what they do — all 539 cases pass
# either way, verified — so this is about the tree staying clean and the
# sources staying readable, not about correctness.
fmt:
	@for f in *.boru tests/*.boru; do \
	  $(BORU) fmt $$f || exit 1; \
	done

# Run every suite (each file exits non-zero on failure).
test:
	@for f in tests/*_test.boru; do \
	  echo "== $$f"; \
	  $(BORU) test $$f || exit 1; \
	done

# Build every program into bin/. Each is a standalone executable that needs
# no boru on the target machine.
binaries: $(BIN)
	@for p in $(PROGS); do \
	  echo "build $$p"; \
	  $(BORU) build $$p.boru -o $(BIN)/$$p || exit 1; \
	done

$(BIN):
	mkdir -p $(BIN)

clean:
	rm -rf $(BIN)
