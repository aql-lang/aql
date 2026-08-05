# CLAUDE.md

The agent guide for this repository is **[AGENTS.md](AGENTS.md)** — read it
first. It is a top-level router: it points you at the right documentation
for the task at hand and explains how to discover the language and the CLI
straight from the tool with **`boru describe`** (words, categories, modules)
and **`boru help`** (the CLI's subcommands).

For a fast, structured orientation, the repository also ships a
**project knowledge graph** — modules, docs, tools, and concepts with
evidence-backed relations — at [kg/out/graph.json](kg/out/graph.json)
(guide: [kg/README.md](kg/README.md)).

Module-specific deep guides — read the relevant one **before** changing
that module:

- Engine kernel (types, values, matching, parser): [eng/go/CLAUDE.md](eng/go/CLAUDE.md)
- Interpreter core (the standalone module cut from eng; kernel conventions apply): [core/go/CLAUDE.md](core/go/CLAUDE.md)
- Base language layer (fundamental words, predefined content types): [basic/go/CLAUDE.md](basic/go/CLAUDE.md)
- Language layer (native words, modules, registry): [lang/go/CLAUDE.md](lang/go/CLAUDE.md)
- Knowledge-graph pipeline (schema, ids, resolution, validation): [kg/README.md](kg/README.md)

Before committing, run the pre-commit checklist from the repo root:

```bash
make fmt && make vet && make lint && make test && make cover-gate
```

`make cover-gate` enforces **ADR-008**: 100% unit-test coverage of every
reachable Go statement (the sole exclusions are provably-unreachable guards
marked with a proof-carrying `//covergate:allow <reason>` comment on the
guard's opening line — see `design/COVERAGE-ALLOWLIST.10.md`).

If your change touches the repository's structure, tooling, or
documentation set, also keep the project knowledge graph current:
update [kg/project/boru-project.jsonic](kg/project/boru-project.jsonic)
and rebuild the committed bundle with `make -C kg graph` (see
[kg/README.md](kg/README.md)); `make -C kg check test` verifies the
pipeline itself.
