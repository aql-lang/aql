# AQL Import Structure

This document explains how Go imports are organized across the AQL codebase.

## Module Identity

The Go module is `github.com/metsitaba/voxgig-exp/aql` (defined in `go.mod`).
All internal packages live under `aql/internal/` and are only visible within
the module.

## External Dependencies

| Dependency | Purpose |
|---|---|
| `github.com/jsonicjs/jsonic/go` | Tokenization and structural parsing (the real lexer) |
| `github.com/jsonicjs/csv/go` | CSV format support |
| `github.com/chzyer/readline` | Interactive REPL line editing |
| `modernc.org/sqlite` | SQLite database access (pure Go) |
| `voxgiguniversalsdk` | Voxgig Universal SDK for API operations |
| `github.com/voxgig/struct` | Deep cloning and structural utilities |
| `golang.org/x/text` | Unicode/text processing |

### Replace Directives

Two `replace` directives in `go.mod` remap import paths:

```
replace github.com/voxgig/struct v0.1.0 => github.com/voxgig/struct/go v0.1.0
replace voxgiguniversalsdk v0.1.1 => github.com/voxgig/udk/go v0.1.1
```

These exist because the upstream repos publish Go modules in a `/go`
subdirectory. The replace directives let the code import the shorter path
while Go fetches from the actual submodule location.

## Internal Package Graph

```
cmd/aql/main.go
├── aql (public API)
│   ├── internal/engine
│   ├── internal/parser
│   ├── internal/native
│   └── internal/fileops
└── internal/repl

internal/repl
├── internal/engine
├── internal/engine/help
├── internal/parser
└── internal/native

internal/parser
├── internal/engine        (converts tokens → engine.Value)
├── internal/ast           (stub, legacy)
├── internal/lexer         (stub, legacy)
└── internal/token         (stub, legacy)

internal/native
└── internal/engine        (only dependency — defines all builtin words)

internal/engine
└── internal/fileops       (file system abstraction)

internal/fileops           (leaf — no internal imports)
internal/object            (leaf — legacy, unused in main flow)
internal/evaluator         (legacy tree-walker, imports ast + object)
```

## Package Roles

### Active packages (used in the main execution path)

**`aql` (root package, `aql.go`)** — Public API surface. Re-exports types
(`Type`, `Value`, `Signature`, `Format`, `FileOps`) from internal packages so
consumers don't need to reach into `internal/`. Provides `New()`, `Run()`,
`Register()`, and `RegisterPrefixOnly()`.

**`internal/engine`** — Core stack machine: the registry of words, type
system, value representation, signature matching, and execution loop. This is
the hub that most other packages depend on. Contains ~70 builtin words
organized in subdirectory files (math, string, boolean, control flow, etc.)
and a `help/` subdirectory for documentation text.

**`internal/parser`** — Converts AQL source text into `[]engine.Value` using
the jsonic library. Handles two semantic contexts: *word context* (top level,
unquoted text = callable words) and *data context* (inside maps/lists,
unquoted text = strings). See `parse.go` for the main logic.

**`internal/native`** — Registers additional builtin words that need external
dependencies (HTTP fetch, deep clone via voxgig/struct, jsonic operations).
Every file in this package imports only `internal/engine`.

**`internal/repl`** — Interactive shell. Orchestrates engine, parser, and
native packages. Uses `readline` for line editing and `engine/help` for the
help system.

**`internal/fileops`** — Minimal interface (`FileOps`) abstracting file
read/write. Has zero internal dependencies. The engine uses this so tests can
inject an in-memory filesystem (`MemFileOps`).

### Legacy/stub packages (not in the active execution path)

**`internal/lexer`**, **`internal/token`**, **`internal/ast`** — Stubs from an
earlier hand-written lexer/parser. The real parsing uses jsonic. The parser
still imports these for compatibility but they are not doing meaningful work.

**`internal/evaluator`**, **`internal/object`** — A classical tree-walking
interpreter pattern. Not integrated into the stack machine engine. These are
unused in the actual AQL execution flow.

## Key Import Patterns

1. **Hub-and-spoke**: `engine` is the central hub. Almost every active package
   imports it. Nothing imports `native` or `repl` except the entry points.

2. **No circular imports**: The dependency graph is a DAG. `engine` never
   imports `parser` or `native` — instead, the top-level wiring in `aql.go`
   calls `parser.Parse` and `native.Register` to connect them.

3. **Public API re-exports**: `aql.go` uses Go type aliases (`type Type =
   engine.Type`) and package-level `var` assignments (`var NewType =
   engine.NewType`) to expose internal types without breaking encapsulation.

4. **Replace directives for submodules**: External Voxgig dependencies use
   `replace` in `go.mod` to map short import paths to their actual `/go`
   submodule locations.
