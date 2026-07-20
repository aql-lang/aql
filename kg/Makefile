# Makefile for the kg/ knowledge-graph pipeline.
#
# Everything runs FROM THIS DIRECTORY — AQL imports and fixture paths
# resolve against the working directory. The `aql` binary comes from
# cmd/go (make -C ../cmd/go build).

AQL ?= ../cmd/go/bin/aql

.PHONY: all build check fmt test graph

all: check test graph

# Ensure the aql binary exists.
build:
	$(MAKE) -C ../cmd/go build

# Static type-check every pipeline module and every test.
check:
	@for f in *.aql tests/*.aql; do \
	  echo "check $$f"; \
	  $(AQL) check $$f || exit 1; \
	done

# Canonical formatting, in place.
#
# DISABLED: `aql fmt` in the current build corrupts template-string
# interpolations — it reformats `hello ${name}` into `hello $ {name:name}`
# (the `${expr}` hole is re-parsed as a map literal), changing program
# semantics. The kg sources use templates heavily, so they are
# hand-formatted until the formatter is template-safe. Minimal repro:
#   printf 'def name "w"\nprint `hi ${name}`\n' > /tmp/t.aql
#   aql fmt /tmp/t.aql   # -> print `hi $ {name:name} `
fmt:
	@echo "kg: aql fmt is disabled here — it corrupts \$${...} template interpolations (see comment in kg/Makefile)"

# Run the whole kg test suite (each file exits non-zero on failure).
test:
	@for f in tests/*_test.aql; do \
	  echo "== $$f"; \
	  $(AQL) $$f || exit 1; \
	done

# Rebuild the committed project knowledge graph (out/graph.json + .sql).
graph:
	$(AQL) main.aql
