# `aql:io` (filesystem surface) — all filesystem functionality

> **Status: design proposal, not implemented (additions).** `aql:io`
> already exists (`lang/go/native/io_module.go`, namespace `IO`); this
> note frames it as the **single home for all filesystem functionality**
> — content I/O, tree mutation, directory listing, metadata (`stat`), and
> the read-only existence/type **predicates** (`exists`, `is-file`,
> `is-dir`, `is-symlink`). `aql:os` keeps only the **non-filesystem**
> remainder of the Go `os` module (env, args, identity, exit — see
> [OS.10.md](OS.10.md)). Read [README.10.md](README.10.md) and
> [`../FILE-ACCESS.10.md`](../FILE-ACCESS.10.md) first.

## 1. The os / io split

Everything that touches the **filesystem** is `aql:io`. `aql:os` is
"whatever is left on the Go `os` module" once the filesystem is removed:

| Go `os` surface | Home |
|---|---|
| `Open`, `ReadFile`, `WriteFile`, `Create`, `Mkdir`, `Remove`, `Rename`, `ReadDir`, `Stat`, `Lstat`, `*os.File`, and the `exists`/`is-*` queries derived from them | **`aql:io`** |
| `Getenv`/`LookupEnv`/`Setenv`/`Environ`, `Args`, `Hostname`, `Getpid`, `Getwd`, `UserHomeDir`, `TempDir`, `Exit` | **`aql:os`** |

`UserHomeDir`/`TempDir`/`Getwd` return path **strings** without touching
the filesystem, so they stay in `aql:os`; the moment a path is *resolved
against the filesystem* (stat, open, list), it is `aql:io`. One module,
one `FileOps` capability, one `fileops` policy scope governs all
filesystem access.

## 2. Import & namespace

```
import "aql:io"          # binds the IO namespace (unchanged)
```

Existing module, unchanged. Words dot-access flat: `IO.read`, `IO.write`,
`IO.exists`, `IO.mkdir`, etc., invoked args-before-dot.

## 3. Existing words (unchanged)

From `lang/go/native/io_module.go` (see `lang/go/CLAUDE.md` "Package
layout"): `read`, `write`, `printstr`, `stdin`, `stdout`, `stderr`,
`trace`, `folder`. (`print` stays a core word.) These keep their current
behaviour and `FileOps` backing.

## 4. Filesystem predicates (the "is" functionality)

Read-only existence/type queries — `os.Stat`/`os.Lstat`-backed. Each
takes a path String and returns a Boolean; all are **total** — a missing
path (or any stat failure short of a policy denial) yields `false`, never
an error (the Python `os.path.exists` posture). They gate on `fileops`
**read** (`disk.read`).

| Go source | aql word | signature (top-first) | one-line doc | refinement |
|---|---|---|---|---|
| `os.Stat` (ok) | `exists` | `String → Boolean` | True if a filesystem entry exists at the path. | `(FileInfo,error)` → Boolean; `IsNotExist`/any error → `false`. |
| `os.Stat` + `!IsDir` | `is-file` | `String → Boolean` | True if the path is a regular file. | follows symlinks; else `false`. |
| `os.Stat` + `IsDir` | `is-dir` | `String → Boolean` | True if the path is a directory. | follows symlinks; else `false`. |
| `os.Lstat` + `ModeSymlink` | `is-symlink` | `String → Boolean` | True if the path itself is a symlink. | uses `Lstat` (does not follow). |

## 5. Content, tree, listing, metadata

Paths are Strings; all route through the `FileOps` capability (§6).
Top-first sig order; inner sigs `BarrierPos: -1`. Unlike the §4
predicates, these **operations surface errors** — a failed `mkdir`/`read`
is an `AqlError`, not a silent default.

| Go source | aql word | signature (top-first) | one-line doc | refinement |
|---|---|---|---|---|
| `os.ReadFile` | `read` | `String → String` | *(existing)* read a file's content. | a `Bytes`-returning `read-bytes` is the binary variant ([BYTES.10.md](BYTES.10.md)), out of scope here. |
| `os.WriteFile` | `write` | `path:String content:String →` | *(existing)* write content (truncating). | existing. |
| `os.WriteFile` (append) | `append` | `path:String content:String →` | Append content, creating if absent. | `O_APPEND\|O_CREATE`; error `io-write`. |
| `os.MkdirAll` | `mkdir` | `String →` | Create a directory and parents. | idempotent; error `io-mkdir`. |
| `os.RemoveAll` | `remove` | `String →` | Remove a file or directory tree. | idempotent on missing; error `io-remove`. |
| `os.Rename` | `rename` | `from:String to:String →` | Move/rename a path. | error `io-rename`. |
| `io.Copy`+open | `copy` | `from:String to:String →` | Copy file content to a new path. | error `io-copy`. |
| `os.ReadDir` | `list` | `String → List[String]` | List directory entry names. | `[]DirEntry` → `List[String]`; error `io-list`. |
| `os.Stat` | `stat` | `String → Map` | Full metadata `{size,is-dir,mode,mtime,name}`. | `(FileInfo,error)` → **Map**, value-or-error `io-stat`. (The boolean §4 predicates are the cheap derived form.) |

`read`/`stat`/`list` gate on `disk.read`; `write`/`append`/`mkdir`/
`remove`/`rename`/`copy` gate on `disk.write` (§6).

## 6. Policy / capabilities

All filesystem words use the host **`FileOps`** capability
(`lang/go/capabilities/capabilities.go`, wired via
`permissioned_fileops.go` / `notinstalled_fileops.go`), gated through the
`fileops` scope. Ops/caps from `policy.GlobalsFor`:

| words | `fileops` op | global cap |
|---|---|---|
| `exists`, `is-file`, `is-dir`, `is-symlink`, `read`, `stat`, `list` | `read` | `disk.read` |
| `write`, `append`, `copy`, `remove`, `rename` | `write` | `disk.write` |
| `mkdir` | `mkdir` | `disk.write` |

`HostPolicy(r) == nil` ⇒ ungated (the default opt-in posture); a sandbox
installs a policy to restrict, and `install:false` makes the accessor
return the not-installed stub (no nil deref). Because the predicates and
content words share the one `fileops` scope, a single rule set governs
the whole filesystem surface.

## 7. Errors

`r.AqlError(code, …)` with kebab codes (`io-write`, `io-mkdir`,
`io-remove`, `io-rename`, `io-copy`, `io-list`, `io-stat`); Go `error`
unwrapped; `fileops`-denied → the policy `Denied` code; `install:false` →
`capability_not_installed`. The §4 predicates are **total** (return
`false`, never raise, except an explicit policy denial). Guard path args
with `AsConcreteString`; never panic.

## 8. Overlap

- **`aql:os`** — only the non-filesystem remainder of the Go `os` module
  (env/args/identity/exit, §1). It produces path *strings*
  (`cwd`/`home-dir`/`temp-dir`); `aql:io` resolves them against the
  filesystem. No word overlap.
- **`aql:filepath` / `aql:path-util`** — pure path-string manipulation;
  `aql:io` consumes the paths they build. No overlap.
- **`Bytes` type** — a future `read-bytes` / `Bytes`-accepting `write`
  ([BYTES.10.md](BYTES.10.md)) is the binary path; out of scope here.

## 9. Examples (args-before form)

```
import "aql:io"

"/etc/hosts" IO.exists                           # true   (predicate, total)
"/etc" IO.is-dir                                  # true
"/nope" IO.exists                                 # false  (missing → false)
"out.txt" "hello" IO.write                        # write content
"out.txt" "\nmore" IO.append                      # append
"data" IO.mkdir                                   # create dir (+ parents)
"data" IO.list                                    # ["a.csv" "b.csv"]
"out.txt" IO.stat get "size"                      # 5
"out.txt" "arch/out.txt" IO.rename                # move
"nope.txt" IO.read                                # ERROR:io-read (operations raise)
```

## 10. Open questions / out of scope

- **Binary I/O** — `read-bytes` / `Bytes`-accepting `write` once the
  `Bytes` type lands (BYTES.10.md); deferred.
- **Streaming / large files** — `*os.File` handle words beyond the
  existing stream handles; deferred.
- **Glob / walk** — recursive traversal (`filepath.Walk`/`Glob`); larger
  surface, revisit.
- **Symlink/permission ops** — `symlink`, `chmod`, `chown`; niche,
  deferred.

## 11. Implementation sketch

Reference: `lang/go/native/io_module.go` (existing) + `io.go`
(`BuildIOModule`, the capability-backed builder).

- `lang/go/native/io_module.go` — add the new natives (`exists`,
  `is-file`, `is-dir`, `is-symlink`, `append`, `mkdir`, `remove`,
  `rename`, `copy`, `list`, `stat`), each calling `EffectiveFileOps(r)`;
  predicate handlers return `false` on any non-policy error; inner sigs
  `BarrierPos: -1`.
- `lang/go/capabilities/capabilities.go` — extend the `FileOps`
  interface + OS-backed and in-memory impls with `Stat`/`Lstat`/`Mkdir`/
  `Remove`/`Rename`/`ReadDir` if absent.
- `lang/go/policy` — the `fileops` op→cap bindings above already exist in
  `GlobalsFor` (`read`→`disk.read`, `write`/`mkdir`→`disk.write`); no new
  scope.
- `lang/go/modules/docs_io.go` — one-line `registerDocs` entry per new
  export (`TestModuleExportDocs` enforces).
- `lang/spec/module-io.tsv` — positive rows + `ERROR:` siblings
  (`io-read` on missing, denied-under-policy; predicates returning
  `false` on missing), using the in-memory `MemFileOps` for
  reproducibility (Test discipline, `lang/go/CLAUDE.md`).
