# `aql:sha256` — Go `crypto/sha256`

> **Status: design proposal, not implemented.** A curated, hand-written
> native module wrapping Go's `crypto/sha256`. Read
> [README.10.md](README.10.md) first for the shared conventions this note
> assumes, and the other three hash notes
> ([SHA512](CRYPTO-SHA512.10.md), [SHA1](CRYPTO-SHA1.10.md),
> [MD5](CRYPTO-MD5.10.md)) — the four share a digest shape.

## 1. Package & status

Go [`crypto/sha256`](https://pkg.go.dev/crypto/sha256) computes the
SHA-224 and SHA-256 digests (FIPS 180-4). `sha256.Sum256(data []byte)
[32]byte` and `sha256.Sum224(data []byte) [28]byte` are the one-shot
forms this note wraps. Nothing is implemented yet.

## 2. Why curated

The raw `go:` bridge would surface `Sum256([]byte) [32]byte` — a Bytes
input AQL has no type for and a fixed-size byte array as output, neither
of which an AQL program can hold or render. The curated surface adopts
the **shared digest convention** (see README): AQL has no Bytes type, so
input is modelled as a `String` (UTF-8 bytes) and the digest is returned
as a hex `String` (primary) or a `base64` `String`. Hashing a string is
common enough (etags, content addressing, cache keys, integrity checks)
to earn a first-class, documented, tested surface.

## 3. Import & namespace

```
import "aql:sha256"        # binds the Sha256 namespace
```

`Sha256` does not clash with any builtin type or existing module
namespace, so no `-util` suffix is needed (naming rule in
`lang/go/CLAUDE.md` "Package layout"). Words are dot-accessed:
`Sha256.hex`, `Sha256.base64`.

## 4. API

Signatures are **top-first, sig order** (position 0 is the top of the
stack). All inner native sigs use `BarrierPos: -1` so the swap form
dispatches. The shared word set is `hex` / `base64`; the 224-bit
variants append the bit width.

| Go symbol | aql word | signature (top-first) | one-line doc | aql-ish refinement |
|---|---|---|---|---|
| `sha256.Sum256(data)` | `hex` | `[String] -> String` | SHA-256 digest as a lowercase hex string. | Input String → UTF-8 bytes; `[32]byte` → 64-char hex String. Total, no error. |
| `sha256.Sum256(data)` | `base64` | `[String] -> String` | SHA-256 digest as standard base64. | Same digest as `hex`, encoded with `base64.StdEncoding`. Total, no error. |
| `sha256.Sum224(data)` | `hex-224` | `[String] -> String` | SHA-224 digest as a lowercase hex string. | `[28]byte` → 56-char hex String. Total, no error. |
| `sha256.Sum224(data)` | `base64-224` | `[String] -> String` | SHA-224 digest as standard base64. | Same digest as `hex-224`, base64-encoded. Total, no error. |

The word set deliberately matches the `hex`/`base64` pair used by the
other hash notes so the four modules read identically; only the
underlying `SumNNN` differs.

## 5. Types

Scalars only — String in, String out. No opaque handle type, no
`RegisterExternalBuiltin` / FixedID. Convert at the boundary with
`eng.FromNative` / `eng.ToNative` (`eng/go/gobridge.go`): the String arg
is taken via `AsConcreteString`, its bytes fed to `Sum256`/`Sum224`, and
the array hex/base64-encoded back to a String.

## 6. Errors

None expected at runtime — both words are total over any String. The
only guard is `AsConcreteString` on the argument, which yields the
standard type error if a non-String (or a DepString constraint) reaches
the handler. No kebab error codes are minted.

## 7. Policy / capabilities

None — pure computation, no side effects, runs under any policy.

## 8. Overlap

The hex/base64 *encoding* steps overlap conceptually with
[`aql:hex`](ENCODING-HEX.10.md) (`encoding/hex`) and
[`aql:base64`](ENCODING-BASE64.10.md) (`encoding/base64`), which encode
arbitrary String↔encoded-String. The dividing line: `aql:sha256` owns
the *digest* (the SHA computation) and bakes the final encoding in as a
convenience; it does not expose general-purpose codecs. A caller wanting
a different encoding of raw digest bytes is out of scope (AQL has no
Bytes type to hand back).

## 9. Examples (args-before form)

```
import "aql:sha256"

"" Sha256.hex            # "e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855"
"abc" Sha256.hex         # "ba7816bf8f01cfea414140de5dae2223b00361a396177a9cb410ff61f20015ad"
"abc" Sha256.base64      # "ungWv48Bz+pBQUDeXa4iI7ADYaOWF3qctBD/YfIAFa0="
"abc" Sha256.hex-224     # "23097d223405d8228642a477bda255b32aadbce4bda0b3f7e36c9da7"
42 Sha256.hex            # ERROR (Integer is not a String)
```

## 10. Open questions / out of scope

- **Streaming / incremental hashing** — `sha256.New()` returns a
  `hash.Hash` for chunked input. Out of scope until AQL grows a Bytes /
  stream type; the one-shot `Sum*` forms cover string hashing.
- **Bytes input** — once a Bytes type exists, `hex`/`base64` should
  accept it directly rather than only UTF-8 String. Tracked across all
  four hash notes.
- **HMAC** — keyed hashing lives in [`aql:hmac`](CRYPTO-HMAC.10.md),
  which selects `sha256` (and the others) by algo atom; it is not
  duplicated here.

## 11. Implementation sketch

Wiring checklist — no Go code. Reference: `lang/go/modules/math.go`
(`BuildMathModule`, the pure-module canonical builder).

- `lang/go/modules/sha256.go` — `BuildSha256Module(parent
  *native.Registry) (native.ModuleDesc, error)`: make an isolated
  `native.DefaultRegistry()` sub-registry, register a `Sha256Natives
  []native.NativeFunc` slice (each inner sig `BarrierPos: -1`), wrap each
  word as an `FnDef` export into an `*OrderedMap`, return
  `ModuleDesc{ID: parent.Modules.NextID(), Exports: {"Sha256": …}}`.
- Register `"sha256": BuildSha256Module` in the `modules` map in
  `lang/go/modules/modules.go`.
- `lang/go/modules/docs_sha256.go` — `registerDocs("aql:sha256",
  map[string]string{…})` with a one-line summary per export
  (`TestModuleExportDocs` enforces completeness).
- `lang/spec/module-sha256.tsv` — `input⇥expected⇥description` rows, each
  leading with `import "aql:sha256"`; every positive row paired with an
  `ERROR:<substring>` negative sibling (Test discipline,
  `lang/go/CLAUDE.md`) — e.g. a non-String argument, and the
  unqualified-word `undefined_word` case without import.
- No FixedID entry (no external type), no policy wiring.
