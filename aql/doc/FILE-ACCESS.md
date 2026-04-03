# AQL File Access API Design

## Status: Implemented (v2 — CSV/TSV added)


## Architecture

File operations use an internal `FileOps` interface (`internal/fileops`)
rather than calling the Go `os` package directly. This allows:

- **Testing**: Swap in `MemFileOps` for in-memory tests (no real files)
- **Sandboxing**: Replace with a custom implementation for restricted environments
- **Abstraction**: Dangerous operations are isolated behind a clear interface

```go
type FileOps interface {
    ReadFile(path string) ([]byte, error)
    WriteFile(path string, data []byte, perm os.FileMode) error
    ResolvePath(path string) (string, error)
}
```

Set via `aql.SetFileOps(ops)` or `registry.SetFileOps(ops)`.
Default implementation uses the real file system with process cwd for relative paths.

### Format Registry

File format handling (json, jsonic, lines, etc.) is dispatched through a
`Format` interface on the Registry:

```go
type Format interface {
    Decode(content string) ([]Value, error)
    Encode(v Value) (string, error)
}
```

Built-in formats are registered at startup via `DefaultFormats()`.
The host application can add custom formats:

```go
a := aql.New()
a.RegisterFormat("yaml", &MyYAMLFormat{})
// Now: read "config.yaml" {fmt:"yaml"}
```

The `read` and `write` words look up the format by name from `Registry.Formats`
and call `Decode`/`Encode` respectively. Line ending normalization is applied
before format decoding and after encoding, so formats operate on normalized content.


## Words

### `read` — Read a file

Push file contents onto the stack as a string (or parsed structure with fmt).

```
read "data.txt"                          # => "file contents..."
read "data.txt" {enc:"utf8"}             # => explicit encoding
read "config.json" {fmt:"json"}          # => parsed to map/list
read "config.jsonic" {fmt:"jsonic"}      # => parsed with jsonic
"data.txt" read                          # => prefix style
```

Signatures:
- `[string] -> [string]` — read file at path
- `[string, map] -> [string|list|map]` — read with options


### `write` — Write a file

Write content to a file. Returns the path written.

```
write "out.txt" "hello world"            # => 'out.txt'
write "out.txt" (upper "hello")          # => 'out.txt' (content from expression)
write "log.txt" "entry\n" {mode:"append"} # => append mode
```

**Note**: With two string arguments of the same type, prefer forward style
(`write "path" "content"`) for clarity. The infix form `"content" write "path"`
is ambiguous because the engine cannot distinguish path from content when both
are strings.

Signatures:
- `[string, string] -> [string]` — path, content -> path
- `[string, string, map] -> [string]` — path, content, options -> path
- `[string, any, map] -> [string]` — path, data, options (auto-serializes with jsonic)


## Options Map

| Key    | Default   | Values                                     |
|--------|-----------|--------------------------------------------|
| `enc`  | `"utf8"`  | `"utf8"`, `"binary"`, `"latin1"`           |
| `fmt`  | `"text"`  | `"text"`, `"json"`, `"jsonic"`, `"lines"`, `"csv"`, `"tsv"` |
| `mode` | `"write"` | `"write"` (truncate), `"append"`           |
| `nl`   | `"lf"`    | `"lf"`, `"crlf"`, `"raw"`                 |

### Format Details

- `text` — raw string, no parsing
- `json` — on read: parse JSON to AQL map/list; on write: serialize to JSON
- `jsonic` — on read: parse with jsonic (relaxed JSON: unquoted keys, etc.)
- `lines` — on read: split into list of strings; on write: join list with newline
- `csv` — on read: parse CSV into a table value with schema; on write: serialize table to CSV
- `tsv` — on read: parse TSV into a table value with schema; on write: serialize table to TSV


## Line Endings

**Default behavior**: All line endings are normalized to `\n` on read (`nl:"lf"`).

The `nl` option controls line ending behavior:

- **`"lf"` (default)**: Normalize all `\r\n` and `\r` to `\n` on read; write `\n`
- **`"crlf"`**: Normalize to `\r\n` on read and write
- **`"raw"`**: No normalization — content preserved exactly as-is


## Path Resolution

Relative paths are resolved against the **process working directory** by default.
The `FileOps` interface controls resolution — custom implementations can change this.


## Error Handling

File operations follow AQL's existing error conventions:

- File not found: `ERROR:read: open nope.txt: file does not exist`
- Write failure: `ERROR:write: ...`
- Invalid JSON: `ERROR:read: invalid json: ...`
- Invalid jsonic: `ERROR:read: invalid jsonic: ...`
- Unknown format: `ERROR:read: unknown format: xyz`


## Testing

Use `MemFileOps` for tests without touching the file system:

```go
mem := fileops.NewMem()
mem.Files["data.txt"] = []byte("hello")

reg := engine.DefaultRegistry()
reg.SetFileOps(mem)

// Or via public API:
a := aql.New()
a.SetFileOps(aql.NewMemFileOps())
```


## Gotchas and Design Notes

### Same-Type Argument Ambiguity
`write` takes `[string, string]` — with two strings on the stack, flexible
matching cannot distinguish path from content. Use forward form or parens:
```
write "path" "content"               # clear: both forward
write "path" (read "source.txt")     # clear: content from expression
```

### Encoding
- Default UTF-8; binary files need `{enc:"binary"}` (future enhancement)

### Large Files
- Entire file loaded to memory (stack machine model)
- `{fmt:"lines"}` on large files creates large lists

### Append Mode
- `{mode:"append"}` reads existing content and prepends it
- If the file doesn't exist, creates it with just the new content

### Return Values
- `read` returns content (or parsed structure with fmt)
- `write` returns the path


## Not in Scope

- Sandbox/path traversal protection (use custom FileOps)
- Directory operations (`ls`, `mkdir`, `rm`)
- Streaming or chunked reads
- File metadata (`stat`, `exists`)
- Network I/O (`fetch`, `http`)
- Glob patterns
- File watching
- Write atomicity (temp+rename)


## Examples

```
# Read a config file as JSON
read "config.json" {fmt:"json"}

# Read relaxed config with jsonic
read "config.jsonic" {fmt:"jsonic"}

# Read with raw line endings (no normalization)
read "data.bin" {nl:"raw"}

# Write results
write "out.txt" "hello world"

# Append to a log
write "log.txt" "new entry\n" {mode:"append"}

# Copy a file (forward form)
write "dst.txt" (read "src.txt")

# Read lines into a list
read "data.txt" {fmt:"lines"}

# Write with Windows line endings
write "out.txt" "a\nb\n" {nl:"crlf"}
```
