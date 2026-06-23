# `aql:base64` — Go `encoding/base64`

> **Status: design proposal, not implemented.** A curated, hand-written
> native module wrapping Go's `encoding/base64`. Read
> [README.10.md](README.10.md) first for the shared conventions this note
> assumes.

## 1. Package & status

Go [`encoding/base64`](https://pkg.go.dev/encoding/base64) encodes and
decodes binary data in the Base64 textual alphabets: the standard
RFC-4180/RFC-4648 alphabet (`StdEncoding`), the URL/filename-safe
alphabet (`URLEncoding`, `-`/`_` instead of `+`/`/`), and unpadded
variants (`RawStdEncoding`, `RawURLEncoding`). This note specifies an
idiomatic AQL surface over that package. Nothing is implemented yet.

## 2. Why curated

The raw `go:` reflection bridge would surface `*base64.Encoding`
method sets — `StdEncoding.EncodeToString([]byte) string`,
`StdEncoding.DecodeString(string) ([]byte, error)` — with `[]byte`
boundaries AQL has no type for, plus four separate `*Encoding`
singletons the caller must pick between. The curated surface fixes the
byte model (a String of its UTF-8 bytes — see §5), folds each alphabet
into a named word pair, and collapses every `(value, error)` decode
into value-or-error via `r.AqlError`. Base64 is common enough in data
wrangling (data URIs, tokens, embedded blobs) to earn a first-class,
documented, tested surface.

## 3. Import & namespace

```
import "aql:base64"         # binds the Base64 namespace
```

The bare package name `base64` does not clash with any builtin type
(`String`, `Path`, `Time`, …) or existing module namespace, so no
`-util` suffix is needed (per the naming rule in `lang/go/CLAUDE.md`
"Package layout"). Words are dot-accessed: `Base64.encode`,
`Base64.decode-url`, etc.

## 4. API

Signatures are **top-first, sig order** (position 0 is the top of the
stack). All inner native sigs use `BarrierPos: -1` so the swap form
dispatches.

| Go symbol | aql word | signature (top-first) | one-line doc | aql-ish refinement |
|---|---|---|---|---|
| `StdEncoding.EncodeToString(b)` | `encode` | `[String] -> String` | Encode a string's bytes to standard padded Base64. | `[]byte` arg → String's UTF-8 bytes (§5); total, no error. |
| `StdEncoding.DecodeString(s) ([]byte,err)` | `decode` | `[String] -> String` | Decode standard padded Base64 back to a string. | `([]byte,err)` → value-or-error `decode`; bytes wrapped back as a String (§5). |
| `URLEncoding.EncodeToString(b)` | `encode-url` | `[String] -> String` | Encode using the URL/filename-safe alphabet (padded). | Picks `URLEncoding` (`-`/`_`); total, no error. |
| `URLEncoding.DecodeString(s) ([]byte,err)` | `decode-url` | `[String] -> String` | Decode URL/filename-safe Base64 (padded). | `([]byte,err)` → value-or-error `decode`. |
| `RawStdEncoding.EncodeToString(b)` | `encode-raw` | `[String] -> String` | Encode to standard Base64 with no `=` padding. | Picks `RawStdEncoding`; total, no error. |
| `RawStdEncoding.DecodeString(s) ([]byte,err)` | `decode-raw` | `[String] -> String` | Decode unpadded standard Base64. | `([]byte,err)` → value-or-error `decode`. |

Notes:
- The four Go `*Encoding` singletons are not exposed as values; each
  alphabet/padding combination is its own word pair (`encode` /
  `encode-url` / `encode-raw` and the matching `decode*`). The
  raw-URL combination (`RawURLEncoding`) is left to Open questions —
  the three pairs above cover the overwhelmingly common cases.
- Encoding never fails; only decoding can (corrupt input, wrong
  alphabet, bad padding), so only the `decode*` words carry an error.

## 5. Types

Scalars only — String. **Byte model:** AQL has no Bytes type, so
binary input/output is modelled as a `String` holding the raw bytes
(its UTF-8 bytes for the common text case). `encode` reads the String's
bytes, `decode*` wraps the decoded bytes back into a String. For true
binary that is not valid UTF-8, the alternative is `List[Integer]`
(each 0–255); this note picks the String model as the primary surface
and flags the `List[Integer]` overload in Open questions. No opaque
handle type, no `RegisterExternalBuiltin` / FixedID is needed. Convert
at the boundary with `eng.FromNative` / `eng.ToNative`
(`eng/go/gobridge.go`).

## 6. Errors

Go `error` returns unwrap to an `AqlError` via `r.AqlError(code,
detail, word)` with a kebab-case code:

| code | raised when |
|---|---|
| `decode` | any `DecodeString` fails — `decode`, `decode-url`, or `decode-raw` (corrupt input, wrong alphabet, or bad/absent padding). |

The detail string names which alphabet/word failed so the message is
actionable. Guard the arg with `AsConcreteString` before use; never
panic (`eng/go/CLAUDE.md` "Panic Prevention").

## 7. Policy / capabilities

None — pure value conversion, runs under any policy.

## 8. Overlap

None with an existing module. No AQL-user-facing word does Base64
today; the `parselang`/`emitlang` family handles structured text
formats (JSON, CSV, …), not byte-level transfer encodings.

## 9. Examples (args-before form)

```
import "aql:base64"

"hello" Base64.encode                  # "aGVsbG8="
"aGVsbG8=" Base64.decode               # "hello"
"hi?>" Base64.encode-url               # url-safe alphabet, padded
"hi?>" Base64.encode-raw               # standard alphabet, no "=" padding
"aGVsbG8" Base64.decode-raw            # "hello"   (unpadded input)
"!!!!" Base64.decode                   # ERROR:decode
```

## 10. Open questions / out of scope

- **`List[Integer]` byte overload** — the String byte-model is primary
  (§5); whether `encode`/`decode*` should also accept/return a
  `List[Integer]` (0–255) for true-binary round-tripping is left open
  until a real binary use case appears.
- **`RawURLEncoding`** — the unpadded URL-safe combination is omitted
  for now; add `encode-raw-url` / `decode-raw-url` if needed.
- **Streaming `NewEncoder` / `NewDecoder`** — the `io.Writer`/`io.Reader`
  streaming variants are out of scope; the whole-String words cover the
  in-memory case, and streaming belongs with `aql:io`.
- **Strict decoding** — Go's `Encoding.Strict()` (reject non-canonical
  trailing bits) is not exposed; default permissive decoding is used.

## 11. Implementation sketch

Wiring checklist — no Go code here. Reference: `lang/go/modules/math.go`
(`BuildMathModule`, the pure-module canonical builder).

- `lang/go/modules/base64.go` — `BuildBase64Module(parent *native.Registry)
  (native.ModuleDesc, error)`: make an isolated `native.DefaultRegistry()`
  sub-registry, register a `Base64Natives []native.NativeFunc` slice
  (each inner sig `BarrierPos: -1`), wrap each word as an `FnDef` export
  into an `*OrderedMap`, return `ModuleDesc{ID: parent.Modules.NextID(),
  Exports: {"Base64": …}}`.
- Register `"base64": BuildBase64Module` in the `modules` map in
  `lang/go/modules/modules.go`.
- `lang/go/modules/docs_base64.go` — `registerDocs("aql:base64",
  map[string]string{…})` with a one-line summary per export
  (`TestModuleExportDocs` enforces completeness).
- `lang/spec/module-base64.tsv` — `input⇥expected⇥description` rows,
  each leading with `import "aql:base64"`; every positive row paired
  with an `ERROR:<substring>` negative sibling (Test discipline,
  `lang/go/CLAUDE.md`).
- No FixedID entry (no external type), no policy wiring.
