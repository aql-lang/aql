# AQL File Access API Design


## Words

### `read` — Read a file

Push file contents onto the stack as a string.

```
read "data.txt"                          # => "file contents..."
read "data.txt" {enc:"utf8"}             # => explicit encoding
read "config.json" {fmt:"json"}          # => parsed to map/list
"data.txt" read                          # => prefix style
```

Signatures:
- `[string] -> [string]` — read file at path
- `[string, map] -> [string|list|map]` — read with options


### `write` — Write a file

Write content to a file. Returns the path written.

```
"hello world" write "out.txt"            # => 'out.txt'
write "out.txt" "hello world"            # => 'out.txt'
"hello" write "out.txt" {mode:"append"}  # => append mode
[1,2,3] write "out.json" {fmt:"json"}   # => serialize list to JSON
```

Signatures:
- `[string, string] -> [string]` — path, content -> path
- `[string, string, map] -> [string]` — path, content, options -> path
- `[string, list|map, map] -> [string]` — path, data, options (requires fmt)


## Options Map

| Key    | Default   | Values                                   |
|--------|-----------|------------------------------------------|
| `enc`  | `"utf8"`  | `"utf8"`, `"binary"`, `"latin1"`         |
| `fmt`  | `"text"`  | `"text"`, `"json"`, `"lines"`, `"csv"`   |
| `mode` | `"write"` | `"write"` (truncate), `"append"`         |
| `nl`   | `"os"`    | `"os"`, `"lf"`, `"crlf"`, `"raw"`       |

### Format Details

- `text` — raw string, no parsing
- `json` — on read: parse JSON to AQL map/list; on write: serialize to JSON
- `lines` — on read: split into list of strings; on write: join list with newline
- `csv` — on read: parse to table (list of maps); on write: serialize table


## Line Endings

The `nl` option controls line ending behavior:

- **On read with `"raw"` or `"os"`**: return content as-is, no normalization
- **On read with `"lf"`**: normalize all `\r\n` and `\r` to `\n`
- **On read with `"crlf"`**: normalize all to `\r\n`
- **On write with `"os"`**: use platform line ending
- **On write with `"lf"`**: force `\n`
- **On write with `"crlf"`**: force `\r\n`
- **With `{fmt:"lines"}`**: split on any of `\n`, `\r\n`, `\r` regardless of `nl`

Principle: do not silently mutate content unless the user asks for it.


## Sandbox and Path Security

File paths are resolved relative to a **root directory** set on the engine
instance by the host application:

- All paths resolved relative to root
- Absolute paths are rejected
- `..` traversal beyond root is rejected
- The host application controls the sandbox boundary

```go
engine.SetRoot("/safe/directory")
```


## Error Handling

File operations follow AQL's existing error conventions:

- File not found: `ERROR:file not found: data.txt`
- Permission denied: `ERROR:permission denied: secret.txt`
- Path outside sandbox: `ERROR:path not allowed: ../../../etc/passwd`
- Invalid UTF-8: `ERROR:invalid utf8: data.bin`
- Invalid JSON: `ERROR:invalid json: data.json`

By default, errors halt execution. For soft behavior:
```
read "maybe.txt" {missing:"none"}        # => none if file absent
```


## Gotchas and Design Notes

### Encoding
- Default UTF-8; error on invalid bytes (don't silently mangle)
- `{enc:"binary"}` returns base64-encoded string (future: native bytes type)

### Large Files
- Entire file loaded to memory (stack machine model)
- Consider a configurable size limit (e.g., 10MB default)
- `{fmt:"lines"}` on large files creates large lists

### Write Atomicity
- Implementation should write to temp file, then rename
- Prevents partial writes on crash or error

### Return Values
- `read` returns content (or parsed structure with fmt)
- `write` returns the path — enables chaining: `"data" write "out.txt" read`

### Concurrent Access
- Not an issue for single-threaded stack machine
- API design does not preclude future safety mechanisms

### Path Resolution
- Relative to engine root (set by host), falling back to cwd
- Script-relative paths could be added later via `{rel:"script"}`


## Not in Scope (v1)

- Directory operations (`ls`, `mkdir`, `rm`)
- Streaming or chunked reads
- File metadata (`stat`, `exists`)
- Network I/O (`fetch`, `http`)
- Glob patterns
- File watching


## Examples

```
# Read a config file
read "config.json" {fmt:"json"}

# Process CSV data
read "users.csv" {fmt:"csv"}

# Write results with unix line endings
"line1\nline2\n" write "out.txt" {nl:"lf"}

# Append to a log
"new entry\n" write "log.txt" {mode:"append"}

# Copy a file
read "source.txt" write "dest.txt"

# Read lines, process, write back
read "data.txt" {fmt:"lines"}
# ... process list on stack ...
write "data.txt" {fmt:"lines"}
```
