# The `import` Word

The `import` word brings external definitions into the current boru engine.
It is registered in `internal/engine/native_module_module.go` alongside the
related `module` and `export` words.

## Signatures

### 1. Import all exports from a module descriptor

```
import module-desc
```

Takes a module descriptor (produced by the `module` word) and installs every
export as a `def` in the current scope.

```boru
def helpers [
  def greet ["hello"]
  export Greet {greet:greet}
] module

import helpers
Greet greet .       # → 'hello'
```

### 2. Import with renaming from a module descriptor

```
import [from to] module-desc            # single rename
import [[from1 to1] [from2 to2]] module-desc  # multiple renames
```

Installs only the listed exports, mapping each `from` name to a `to` name.

### 3. Import from a file

```
import "./utils.boru"
```

File paths must start with `/`, `./`, or `../`. Bare filenames like
`"utils.boru"` are rejected.

For `.boru` files, reads the file, parses it as boru, and runs it in an
**isolated module engine**. All `export`ed names become available as `def`s.

For data files, the content is parsed and pushed directly onto the stack:

| Extension | Behavior |
|---|---|
| `.json` | Parsed as JSON, pushes map or list |
| `.jsonic` | Parsed as jsonic, pushes map or list |
| `.csv` | Loaded as a table (same as `read`) |
| `.tsv` | Loaded as a table (same as `read`) |

```boru
import "./config.boru"       # installs exports as defs
import "./data.json"         # pushes a map/list onto the stack
import "./config.jsonic"     # same — pushes data value
import "./people.csv"        # loads CSV as a table
import "./data.tsv"          # loads TSV as a table
```

### 4. Import from a file with renaming

```
import [Orig Renamed] "./utils.boru"
import [[A AA] [B BB]] "./data.boru"
```

Same as file import, but only the listed exports are installed and each is
renamed. Renaming is **not supported** for data files
(`.json`/`.jsonic`/`.csv`/`.tsv`).

## Isolation

File imports run in a completely fresh engine:

- Internal `def`s inside the imported file do **not** leak into the parent.
- Parent `def`s are **not** visible inside the file's module.
- Only names declared with `export` are accessible after import.

```boru
# lib.boru
def secret 42
export Lib {x:1}

# main session
import "./lib.boru"
Lib x .       # → 1
secret        # → atom 'secret', not 42
```

## Export Resolution

When a module runs `export Foo {val:mydef}`, each value in the export map is
resolved through the module's def stacks **at export time**. So if `mydef`
was defined as `42`, the export map stores the value `42`, not the name.

```boru
# math.boru
def pi 3
def e 2
export Math {pi:pi, e:e}

# usage
import "./math.boru"
Math pi .     # → 3
Math e .      # → 2
```

## Data File Import

Data files are treated as pure data — no module execution:

```boru
import "./data.json"          # pushes parsed map/list
name .                         # access a field

import "./people.csv"         # loads as table
```

CSV/TSV imports use the same `doRead` path as the `read` word, so tables are
stored in SQLite when available.

## Implementation

The implementation lives in `native_module_module.go`:

| Function | Role |
|---|---|
| `registerModule()` | Registers `module`, `export`, and `import` words |
| `runModuleBody()` | Creates isolated engine, runs module body, collects exports |
| `isFilePath()` | Validates file path starts with `/`, `./`, or `../` |
| `isDataFile()` | Checks extension for data files (json, jsonic, csv, tsv) |
| `installExports()` | Installs exports as defs (with optional name filter) |
| `installRenamedExports()` | Handles rename lists (single pair or list of pairs) |
| `loadFileModule()` | Reads file, parses as boru, runs as module |
| `loadDataFile()` | Reads data file via `doRead` (same path as `read` word) |
| `resolveModuleExport()` | Resolves export values through module def stacks |
