# `aql:sha512` — Go `crypto/sha512`

> **Status: design proposal, not implemented.** A curated, hand-written
> native module wrapping Go's `crypto/sha512`. Read
> [README.10.md](README.10.md) first for the shared conventions this note
> assumes, and the sibling hash notes
> ([SHA256](CRYPTO-SHA256.10.md), [SHA1](CRYPTO-SHA1.10.md),
> [MD5](CRYPTO-MD5.10.md)) — the four share a digest shape.

## 1. Package & status

Go [`crypto/sha512`](https://pkg.go.dev/crypto/sha512) computes the
SHA-512 family (FIPS 180-4): `sha512.Sum512(data) [64]byte`,
`sha512.Sum384(data) [48]byte`, plus the truncated
`sha512.Sum512_256(data) [32]byte` and `sha512.Sum512_224(data)
[28]byte`. This note wraps those one-shot forms. Nothing is implemented
yet.

## 2. Why curated

As with the other hash notes, the raw `go:` bridge would surface a Bytes
input and a fixed-size byte array — neither holdable nor renderable in
AQL. The curated surface adopts the **shared digest convention** (see
README): no Bytes type, so input is a `String` (UTF-8 bytes) and the
digest is a hex `String` (primary) or a `base64` `String`. SHA-512 is the
preferred modern digest on 64-bit hosts (faster than SHA-256 there) and
the truncated variants give a 256/224-bit digest from the 512-bit core.

## 3. Import & namespace

```
import "aql:sha512"        # binds the Sha512 namespace
```

`Sha512` does not clash with any builtin type or existing module
namespace, so no `-util` suffix (naming rule in `lang/go/CLAUDE.md`
"Package layout"). Words are dot-accessed: `Sha512.hex`, `Sha512.base64`.

## 4. API

Signatures are **top-first, sig order** (position 0 is the top of the
stack). All inner native sigs use `BarrierPos: -1` so the swap form
dispatches. The shared word set is `hex` / `base64` (the 512-bit
default); variants append the digest width.

| Go symbol | aql word | signature (top-first) | one-line doc | aql-ish refinement |
|---|---|---|---|---|
| `sha512.Sum512(data)` | `hex` | `[String] -> String` | SHA-512 digest as a lowercase hex string. | Input String → UTF-8 bytes; `[64]byte` → 128-char hex String. Total, no error. |
| `sha512.Sum512(data)` | `base64` | `[String] -> String` | SHA-512 digest as standard base64. | Same digest as `hex`, `base64.StdEncoding`. Total, no error. |
| `sha512.Sum384(data)` | `hex-384` | `[String] -> String` | SHA-384 digest as a lowercase hex string. | `[48]byte` → 96-char hex String. Total, no error. |
| `sha512.Sum384(data)` | `base64-384` | `[String] -> String` | SHA-384 digest as standard base64. | Same digest as `hex-384`, base64-encoded. Total, no error. |
| `sha512.Sum512_256(data)` | `hex-256` | `[String] -> String` | SHA-512/256 digest as a lowercase hex string. | Truncated 512-bit core; `[32]byte` → 64-char hex String. Distinct from `aql:sha256` (different IV). Total, no error. |
| `sha512.Sum512_224(data)` | `hex-224` | `[String] -> String` | SHA-512/224 digest as a lowercase hex string. | `[28]byte` → 56-char hex String. Total, no error. |

The base `hex`/`base64` pair matches the other three hash notes
verbatim; only the underlying `SumNNN` differs. `base64-256` / `base64-224`
are deliberately omitted (the truncated variants are niche; add later if
a need appears) — an Open question.

## 5. Types

Scalars only — String in, String out. No opaque handle type, no
`RegisterExternalBuiltin` / FixedID. Boundary conversion via
`eng.FromNative` / `eng.ToNative` (`eng/go/gobridge.go`): String arg via
`AsConcreteString`, bytes fed to the chosen `SumNNN`, array
hex/base64-encoded back to a String.

## 6. Errors

None expected — every word is total over any String. The only guard is
`AsConcreteString` on the argument (standard type error for a non-String
/ DepString). No kebab error codes are minted.

## 7. Policy / capabilities

None — pure computation, no side effects, runs under any policy.

## 8. Overlap

The final hex/base64 encoding overlaps conceptually with
[`aql:hex`](ENCODING-HEX.10.md) and
[`aql:base64`](ENCODING-BASE64.10.md); `aql:sha512` owns the digest and
bakes the encoding in, exposing no general codec. The `hex-256` digest is
**not** the same as [`aql:sha256`](CRYPTO-SHA256.10.md)'s `hex` —
SHA-512/256 uses a different initial value, so the two disagree on the
same input; the note flags this so callers do not assume equivalence.

## 9. Examples (args-before form)

```
import "aql:sha512"

"abc" Sha512.hex
# "ddaf35a193617abacc417349ae20413112e6fa4e89a97ea20a9eeee64b55d39a2192992a274fc1a836ba3c23a3feebbd454d4423643ce80e2a9ac94fa54ca49f"
"abc" Sha512.base64      # standard-base64 of the same 64 bytes
"abc" Sha512.hex-384
# "cb00753f45a35e8bb5a03d699ac65007272c32ab0eded1631a8b605a43ff5bed8086072ba1e7cc2358baeca134c825a7"
"abc" Sha512.hex-256     # 64-char hex; NOT equal to (Sha256.hex "abc")
42 Sha512.hex            # ERROR (Integer is not a String)
```

## 10. Open questions / out of scope

- **`base64-256` / `base64-224`** — base64 forms of the truncated
  variants are omitted for now (niche). Add if a real need appears.
- **Streaming / incremental hashing** — `sha512.New*()` `hash.Hash`
  forms are out of scope until AQL has a Bytes / stream type.
- **Bytes input** — once a Bytes type exists, words should accept it
  directly rather than only UTF-8 String. Tracked across all four hash
  notes.

## 11. Implementation sketch

Wiring checklist — no Go code. Reference: `lang/go/modules/math.go`
(`BuildMathModule`).

- `lang/go/modules/sha512.go` — `BuildSha512Module(parent
  *native.Registry) (native.ModuleDesc, error)`: isolated
  `native.DefaultRegistry()` sub-registry, register a `Sha512Natives
  []native.NativeFunc` slice (each inner sig `BarrierPos: -1`), wrap each
  word as an `FnDef` export, return `ModuleDesc{ID:
  parent.Modules.NextID(), Exports: {"Sha512": …}}`.
- Register `"sha512": BuildSha512Module` in the `modules` map in
  `lang/go/modules/modules.go`.
- `lang/go/modules/docs_sha512.go` — `registerDocs("aql:sha512",
  map[string]string{…})`, one line per export (`TestModuleExportDocs`).
- `lang/spec/module-sha512.tsv` — rows leading with `import "aql:sha512"`;
  every positive row paired with an `ERROR:<substring>` negative sibling
  (non-String arg; unqualified-word `undefined_word` without import).
- No FixedID entry, no policy wiring.
