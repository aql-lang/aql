# `hash/crc32` → `aql:crc32`

> **Status: design proposal — not implemented.** A curated, hand-written
> native module wrapping Go's `hash/crc32` with an idiomatic ("aql-ish")
> surface. Read [`README.10.md`](README.10.md) first for the shared
> conventions this note assumes.

## 1. Package & status

Go package: [`hash/crc32`](https://pkg.go.dev/hash/crc32) — the 32-bit
cyclic redundancy check, in three standard polynomials (IEEE, the
zlib/Ethernet default; Castagnoli, used by iSCSI/SSE4.2; Koopman). This
note specifies `aql:crc32` (namespace `Crc32`), a value-oriented wrapper
that reduces a String to its checksum Integer. Design proposal; no Go
code exists yet.

## 2. Why curated

The raw `go:hash/crc32` bridge would surface the stdlib's hash-builder
machinery: `crc32.NewIEEE()` returns a `hash.Hash32`, the caller drives
`Write([]byte)` then `Sum32()`, and `MakeTable` returns a `*crc32.Table`
pointer that is meaningless as AQL data. That is a stateful, multi-call,
opaque-handle dance for what is conceptually one pure function:
`bytes → checksum`. The curated surface collapses each polynomial to a
single total word (`ieee` / `castagnoli` / `koopman`) that takes a String
and returns its checksum directly — no handle, no `Write`/`Sum` protocol,
no `*Table` pointer in user space.

## 3. Import & namespace

```
import "aql:crc32"        # binds the Crc32 namespace
```

The bare capitalized package name `Crc32` is free (not a builtin type,
not an existing module namespace), so **no `-util` suffix** is needed
(per the naming rule in `lang/go/CLAUDE.md` "Package layout"). Words are
dot-accessed: `Crc32.ieee`, `Crc32.castagnoli`, …

## 4. API

Signatures are **top-first, sig order** (position 0 = top of stack), per
the README "Argument order & dispatch" rule. All inner natives use
`BarrierPos: -1` so the swap form `a Crc32.word b` dispatches (these are
all single-arg, so the relevant forms are `s Crc32.ieee` stack and the
plain word; `BarrierPos: -1` keeps dispatch uniform).

| Go symbol | aql word | signature (top-first) | one-line doc | aql-ish refinement |
|---|---|---|---|---|
| `crc32.ChecksumIEEE` | `ieee` | `[String] -> Integer` | CRC-32 checksum, IEEE polynomial (zlib/Ethernet default). | The whole `NewIEEE`/`Write`/`Sum32` protocol collapses to one pure `String → Integer`. Input bytes are the String's UTF-8 bytes. |
| `crc32.MakeTable(Castagnoli)` + `crc32.Checksum` | `castagnoli` | `[String] -> Integer` | CRC-32 checksum, Castagnoli polynomial (iSCSI / SSE4.2). | The `*crc32.Table` is built once internally (`crc32.MakeTable(crc32.Castagnoli)`, a package-level cached value) and never surfaces; the word is a pure `String → Integer`. |
| `crc32.MakeTable(Koopman)` + `crc32.Checksum` | `koopman` | `[String] -> Integer` | CRC-32 checksum, Koopman polynomial. | Same shape as `castagnoli` with `crc32.Koopman`; pure `String → Integer`. |
| `crc32.ChecksumIEEE` (hex render) | `ieee-hex` | `[String] -> String` | IEEE checksum as a zero-padded 8-char lowercase hex String. | *Optional convenience.* Same computation as `ieee`, rendered `%08x` so it concatenates / compares as a fixed-width digest String. |
| `crc32.MakeTable(Castagnoli)` (hex render) | `castagnoli-hex` | `[String] -> String` | Castagnoli checksum as an 8-char hex String. | *Optional convenience.* `%08x` render of `castagnoli`. |
| `crc32.MakeTable(Koopman)` (hex render) | `koopman-hex` | `[String] -> String` | Koopman checksum as an 8-char hex String. | *Optional convenience.* `%08x` render of `koopman`. |

### Input modeling

Input is a **String**, and the bytes hashed are its UTF-8 byte encoding —
the same convention every checksum/digest module in this roster uses
(`aql:sha256`, etc.). AQL has no Bytes type (see
[BYTES.10.md](BYTES.10.md)); a String is the canonical carrier for "some
bytes", and `eng.FromNative` already hands back a Go `string` whose
`[]byte(s)` is fed straight to `crc32.Checksum`.

### uint32 → Integer fits cleanly

`crc32.Checksum` returns a Go `uint32` (0 .. 4,294,967,295). AQL's
`Integer` is an `int64`, whose positive range (up to 9.2e18) **comfortably
contains the full uint32 range**, so the checksum is returned as a plain
non-negative `Integer` with no overflow, no sign games, and no
information loss. This is the key difference from
[CRC64](HASH-CRC64.10.md), whose uint64 result does *not* fit int64 and
is therefore returned as a hex String by default.

## 5. Types

Scalars only — String in, Integer (or hex String for the `-hex` variants)
out. **No opaque external handle:** the `*crc32.Table` values are an
internal implementation detail held in package-level Go variables, never
a `RegisterExternalBuiltin` / FixedID type. Convert at the boundary with
`eng.FromNative` / `eng.ToNative` (`eng/go/gobridge.go`):
String↔`string`, Integer↔int kinds.

## 6. Errors

No panics (`eng/go/CLAUDE.md` "Panic Prevention"). The only failure mode
is a non-String argument, guarded with `AsConcreteString` before use and
signalled via `r.AqlError(code, detail, word)`:

| code | raised when |
|---|---|
| `bad-arg` | the argument is not a concrete String (type literal / wrong type). |

The CRC computation itself is total — every byte sequence has a checksum
— so there is no Go `error` to unwrap.

## 7. Policy / capabilities

**None — pure.** Purely in-memory arithmetic over the input bytes;
nothing touches disk, network, env, or clock. Runs under any policy.

## 8. Overlap

None with an existing module. `aql:bin-util` (`BinUtil`) owns
`hash/fnv` (per the README roster) — a *different* hash family for a
different purpose (fast hash-table keys, not error-detection
checksums) — and the bitwise word group; it exposes no CRC words, so
`aql:crc32` is a genuinely new surface and does not move or change
anything in `aql:bin-util`. The crypto-hash modules
(`aql:sha256` / `aql:md5` / …) share the "String → digest" shape but
cover cryptographic digests; CRC is a non-cryptographic checksum and is
deliberately a separate module.

## 9. Examples (args-before form)

All args-before form (`s Crc32.word`); never `Crc32.word s` as pure
forward.

```
import "aql:crc32"

"hello" Crc32.ieee                 # 907060870
"hello" Crc32.castagnoli           # 2591144780
"hello" Crc32.koopman              # (Koopman polynomial checksum)
"" Crc32.ieee                      # 0          (empty input)
"hello" Crc32.ieee-hex             # "360a2f6a" (8-char zero-padded hex)
42 Crc32.ieee                      # ERROR:bad-arg   (Integer, not String)
```

## 10. Open questions / out of scope

- **The `-hex` variants** are marked optional: ship them only if a
  fixed-width hex digest String is actually wanted at call sites; the
  base Integer words plus `Strconv.format-int-base 16` already cover the
  need (see [STRCONV.10.md](STRCONV.10.md)). Decide before writing the
  spec rows.
- **Custom polynomials** — `crc32.MakeTable(poly)` for an arbitrary user
  polynomial is out of scope; the three named standards cover the
  realistic cases. Add a `table` + `checksum` pair later only if a
  concrete need appears.
- **Incremental / streaming hashing** (the `hash.Hash32` `Write`/`Sum32`
  protocol over a stream) is out of scope — this module is the
  one-shot `bytes → checksum` surface. Streaming, if ever needed, belongs
  alongside `aql:io`'s Stream handles, not here.
- **Hashing file contents** — read the file with `IO.read` (see
  [README.10.md](README.10.md) roster) and pipe the String into
  `Crc32.ieee`; this module does no I/O of its own.

## 11. Implementation sketch

Wiring checklist — no Go code here. Reference: `lang/go/modules/math.go`
(`BuildMathModule`, the pure-module canonical builder).

- `lang/go/modules/crc32.go` — `BuildCrc32Module(parent *native.Registry)
  (native.ModuleDesc, error)`: make an isolated `native.DefaultRegistry()`
  sub-registry, register a `Crc32Natives []native.NativeFunc` slice (each
  inner sig `BarrierPos: -1`), build the three `*crc32.Table` values once
  as package-level vars, wrap each word as an `FnDef` export into an
  `*OrderedMap`, return `ModuleDesc{ID: parent.Modules.NextID(),
  Exports: {"Crc32": …}}`.
- Register `"crc32": BuildCrc32Module` in the `modules` map in
  `lang/go/modules/modules.go`.
- `lang/go/modules/docs_crc32.go` — `registerDocs("aql:crc32",
  map[string]string{…})` with a one-line summary per export
  (`TestModuleExportDocs` enforces completeness).
- `lang/spec/module-crc32.tsv` — `input⇥expected⇥description` rows, each
  leading with `import "aql:crc32"`; every positive row paired with an
  `ERROR:<substring>` negative sibling (Test discipline,
  `lang/go/CLAUDE.md`).
- Boundary conversion via `eng.FromNative` / `eng.ToNative`
  (String↔`string`, Integer↔int kinds). No FixedID entry (no external
  type), no policy wiring.
