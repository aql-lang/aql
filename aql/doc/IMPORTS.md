# The `import` Word

The `import` word brings external definitions into the current AQL engine.
It is registered in `internal/engine/builtin_module_module.go` alongside the
related `module` and `export` words.

## Signatures

### 1. Import all exports from a module descriptor

```
module-desc import
```

Takes a module descriptor (produced by the `module` word) and installs every
export as a `def` in the current scope.

```aql
def helpers [
  def greet ["hello"]
  export Greet {greet:greet}
] module

helpers import
Greet greet .       # → 'hello'
```

### 2. Import with renaming from a module descriptor

```
[from to] module-desc import            # single rename
[[from1 to1] [from2 to2]] module-desc import  # multiple renames
```

Installs only the listed exports, mapping each `from` name to a `to` name.

### 3. Import from a file

```
"./utils.aql" import
```

File paths must start with `/`, `./`, or `../`. Bare filenames like
`"utils.aql"` are rejected.

For `.aql` files, reads the file, parses it as AQL, and runs it in an
**isolated module engine**. All `export`ed names become available as `def`s.

For data files, the content is parsed and pushed directly onto the stack:

| Extension | Behavior |
|---|---|
| `.json` | Parsed as JSON, pushes map or list |
| `.jsonic` | Parsed as jsonic, pushes map or list |
| `.csv` | Loaded as a table (same as `read`) |
| `.tsv` | Loaded as a table (same as `read`) |

```aql
"./config.aql" import       # installs exports as defs
"./data.json" import         # pushes a map/list onto the stack
"./config.jsonic" import     # same — pushes data value
"./people.csv" import        # loads CSV as a table
"./data.tsv" import          # loads TSV as a table
```

### 4. Import from a file with renaming

```
[Orig Renamed] "./utils.aql" import
[[A AA] [B BB]] "./data.aql" import
```

Same as file import, but only the listed exports are installed and each is
renamed. Renaming is **not supported** for data files
(`.json`/`.jsonic`/`.csv`/`.tsv`).

## Isolation

File imports run in a completely fresh engine:

- Internal `def`s inside the imported file do **not** leak into the parent.
- Parent `def`s are **not** visible inside the file's module.
- Only names declared with `export` are accessible after import.

```aql
# lib.aql
def secret 42
export Lib {x:1}

# main session
"./lib.aql" import
Lib x .       # → 1
secret        # → atom 'secret', not 42
```

## Export Resolution

When a module runs `export Foo {val:mydef}`, each value in the export map is
resolved through the module's def stacks **at export time**. So if `mydef`
was defined as `42`, the export map stores the value `42`, not the name.

```aql
# math.aql
def pi 3
def e 2
export Math {pi:pi, e:e}

# usage
"./math.aql" import
Math pi .     # → 3
Math e .      # → 2
```

## Data File Import

Data files are treated as pure data — no module execution:

```aql
"./data.json" import          # pushes parsed map/list
name .                         # access a field

"./people.csv" import         # loads as table
```

CSV/TSV imports use the same `doRead` path as the `read` word, so tables are
stored in SQLite when available.

## Implementation

The implementation lives in `builtin_module_module.go`:

| Function | Role |
|---|---|
| `registerModule()` | Registers `module`, `export`, and `import` words |
| `runModuleBody()` | Creates isolated engine, runs module body, collects exports |
| `isFilePath()` | Validates file path starts with `/`, `./`, or `../` |
| `isDataFile()` | Checks extension for data files (json, jsonic, csv, tsv) |
| `installExports()` | Installs exports as defs (with optional name filter) |
| `installRenamedExports()` | Handles rename lists (single pair or list of pairs) |
| `loadFileModule()` | Reads file, parses as AQL, runs as module |
| `loadDataFile()` | Reads data file via `doRead` (same path as `read` word) |
| `resolveModuleExport()` | Resolves export values through module def stacks |
