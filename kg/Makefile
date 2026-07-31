# Makefile for the kg/ knowledge-graph pipeline.
#
# Everything runs FROM THIS DIRECTORY — BORU imports and fixture paths
# resolve against the working directory. The `boru` binary comes from
# cmd/go (make -C ../cmd/go build).

BORU ?= ../cmd/go/bin/boru

.PHONY: all build check fmt test graph

all: fmt check test graph

# Ensure the boru binary exists.
build:
	$(MAKE) -C ../cmd/go build

# Static type-check every pipeline module and every test.
check:
	@for f in *.boru tests/*.boru; do \
	  echo "check $$f"; \
	  $(BORU) check $$f || exit 1; \
	done

# Canonical formatting, in place.
#
# Part of `all` so the sources cannot drift out of canonical form: the
# formatter is idempotent, so once they are canonical this is a no-op on
# the tree and `make all` leaves nothing to commit.
#
# This target was a guarded no-op until 2026-07-29. It was disabled because
# `boru fmt` corrupted template interpolations (`hi ${name}` re-parsed the
# `${expr}` hole as a map literal) and because formatting these sources took
# ~10 minutes. Both are fixed: the formatter scans backtick literals
# verbatim (lang/go/formatter/format.go scanBacktick / scanInterpHole), and
# emitNode is memoised, which took kg/validate.boru from 53s to 40ms — the
# whole kg tree now formats in well under a second.
fmt:
	@for f in *.boru tests/*.boru; do \
	  $(BORU) fmt $$f || exit 1; \
	done

# Run the whole kg test suite (each file exits non-zero on failure).
test:
	@for f in tests/*_test.boru; do \
	  echo "== $$f"; \
	  $(BORU) $$f || exit 1; \
	done

# Rebuild the committed project knowledge graph (out/graph.json + .sql).
graph:
	$(BORU) main.boru
