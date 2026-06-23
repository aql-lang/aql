# `aql:md5` — Go `crypto/md5`

> **Status: design proposal, not implemented.** A curated, hand-written
> native module wrapping Go's `crypto/md5`. Read
> [README.10.md](README.10.md) first for the shared conventions this note
> assumes, and the sibling hash notes
> ([SHA256](CRYPTO-SHA256.10.md), [SHA512](CRYPTO-SHA512.10.md),
> [SHA1](CRYPTO-SHA1.10.md)) — the four share a digest shape.

## 1. Package & status

Go [`crypto/md5`](https://pkg.go.dev/crypto/md5) computes the MD5 digest
(RFC 1321). `md5.Sum(data []byte) [16]byte` is the one-shot form this
note wraps. Nothing is implemented yet.

## 2. Why curated

The raw `go:` bridge would surface a Bytes input and a `[16]byte` array
— neither holdable nor renderable in AQL. The curated surface adopts the
**shared digest convention** (see README): no Bytes type, so input is a
`String` (UTF-8 bytes) and the digest is a hex `String` (primary) or a
`base64` `String`. MD5 is curated for **checksums and legacy interop**
(content fingerprints, S3 ETags, older protocols), not as a security
primitive — see the legacy note below.

## 3. Import & namespace

```
import "aql:md5"        # binds the Md5 namespace
```

`Md5` does not clash with any builtin type or existing module namespace,
so no `-util` suffix (naming rule in `lang/go/CLAUDE.md` "Package
layout"). Words are dot-accessed: `Md5.hex`, `Md5.base64`.

## 4. API

Signatures are **top-first, sig order** (position 0 is the top of the
stack). All inner native sigs use `BarrierPos: -1` so the swap form
dispatches. The word set matches the other hash notes: `hex` / `base64`.

| Go symbol | aql word | signature (top-first) | one-line doc | aql-ish refinement |
|---|---|---|---|---|
| `md5.Sum(data)` | `hex` | `[String] -> String` | MD5 digest as a lowercase hex string. | Input String → UTF-8 bytes; `[16]byte` → 32-char hex String. Total, no error. |
| `md5.Sum(data)` | `base64` | `[String] -> String` | MD5 digest as standard base64. | Same digest as `hex`, `base64.StdEncoding`. Total, no error. |

## 5. Types

Scalars only — String in, String out. No opaque handle type, no
`RegisterExternalBuiltin` / FixedID. Boundary conversion via
`eng.FromNative` / `eng.ToNative` (`eng/go/gobridge.go`): String arg via
`AsConcreteString`, bytes fed to `md5.Sum`, array hex/base64-encoded back
to a String.

## 6. Errors

None expected — both words are total over any String. The only guard is
`AsConcreteString` on the argument (standard type error for a non-String
/ DepString). No kebab error codes are minted.

## 7. Policy / capabilities

None — pure computation, no side effects, runs under any policy.

## 8. Overlap

The final hex/base64 encoding overlaps conceptually with
[`aql:hex`](ENCODING-HEX.10.md) and
[`aql:base64`](ENCODING-BASE64.10.md); `aql:md5` owns the digest and
bakes the encoding in, exposing no general codec.

## 9. Examples (args-before form)

```
import "aql:md5"

"" Md5.hex           # "d41d8cd98f00b204e9800998ecf8427e"
"abc" Md5.hex        # "900150983cd24fb0d6963f7d28e17f72"
"abc" Md5.base64     # standard-base64 of the same 16 bytes
42 Md5.hex           # ERROR (Integer is not a String)
```

## 10. Open questions / out of scope

- **LEGACY / broken.** MD5 is cryptographically broken: collisions are
  trivially producible. It is fine for non-adversarial **checksums** and
  **legacy interop** (e.g. S3 ETags, file fingerprinting), but MUST NOT
  be used for new security work — prefer
  [`aql:sha256`](CRYPTO-SHA256.10.md) /
  [`aql:sha512`](CRYPTO-SHA512.10.md). The `docs_md5.go` summaries and a
  comment in `md5.go` should state this so the surface is honest. Open
  question: should the module emit a one-time deprecation/advisory note
  on import, or stay silent and rely on docs?
- **Streaming / incremental hashing** — `md5.New()` `hash.Hash` form is
  out of scope until AQL has a Bytes / stream type.
- **Bytes input** — once a Bytes type exists, words should accept it
  directly. Tracked across all four hash notes.

## 11. Implementation sketch

Wiring checklist — no Go code. Reference: `lang/go/modules/math.go`
(`BuildMathModule`).

- `lang/go/modules/md5.go` — `BuildMd5Module(parent *native.Registry)
  (native.ModuleDesc, error)`: isolated `native.DefaultRegistry()`
  sub-registry, register a `Md5Natives []native.NativeFunc` slice (each
  inner sig `BarrierPos: -1`), wrap each word as an `FnDef` export, return
  `ModuleDesc{ID: parent.Modules.NextID(), Exports: {"Md5": …}}`.
- Register `"md5": BuildMd5Module` in the `modules` map in
  `lang/go/modules/modules.go`.
- `lang/go/modules/docs_md5.go` — `registerDocs("aql:md5",
  map[string]string{…})`, one line per export, including the legacy
  caveat (`TestModuleExportDocs`).
- `lang/spec/module-md5.tsv` — rows leading with `import "aql:md5"`;
  every positive row paired with an `ERROR:<substring>` negative sibling
  (non-String arg; unqualified-word `undefined_word` without import).
- No FixedID entry, no policy wiring.
