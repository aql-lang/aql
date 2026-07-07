# CLAUDE.md

The agent guide for this repository is **[AGENTS.md](AGENTS.md)** — read it
first. It is a top-level router: it points you at the right documentation
for the task at hand and explains how to discover the language and the CLI
straight from the tool with **`aql describe`** (words, categories, modules)
and **`aql help`** (the CLI's subcommands).

Module-specific deep guides — read the relevant one **before** changing
that module:

- Engine kernel (types, values, matching, parser): [eng/go/CLAUDE.md](eng/go/CLAUDE.md)
- Language layer (native words, modules, registry): [lang/go/CLAUDE.md](lang/go/CLAUDE.md)

Before committing, run the pre-commit checklist from the repo root:

```bash
make fmt && make vet && make lint && make test && make cover-gate
```

`make cover-gate` enforces **ADR-008**: 100% unit-test coverage of every
reachable Go statement (the sole exclusion is the reviewed, proof-carrying
`test/go/covergate/allowlist.tsv` — see `design/COVERAGE-ALLOWLIST.10.md`).
