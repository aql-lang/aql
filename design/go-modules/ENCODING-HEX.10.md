# `aql:hex` — Go `encoding/hex`

> **Status: design proposal, not implemented.** A curated, hand-written
> native module wrapping Go's `encoding/hex`. Read
> [README.10.md](README.10.md) first for the shared conventions this note
> assumes.

## 1. Package & status

Go [`encoding/hex`](https://pkg.go.dev/encoding/hex) encodes and decodes
binary data as lowercase hexadecimal text (`EncodeToString` /
`DecodeString`) and renders a `hexdump -C`-style annotated dump
(`Dump`). This note specifies an idiomatic AQL surface over that
package. Nothing is implemented yet.

## 2. Why curated

The raw `go:` reflection bridge would surface `hex.EncodeToString([]byte)
string` and `hex.DecodeString(string) ([]byte, error)` with `[]byte`
boundaries AQL has no type for, plus length-prefixed `Encode`/`Decode`
buffer variants that make no sense without a Bytes type. The curated
surface fixes the byte model (a String of its bytes — see §5), keeps
only the whole-value words, and collapses the decode `(value, error)`
into value-or-error via `r.AqlError`. Hex round-tripping and hexdumps
are common in data/debug work and earn a first-class, tested surface.

## 3. Import & namespace

```
import "aql:hex"            # binds the Hex namespace
```

The bare package name `hex` does not clash with any builtin type
(`String`, `Path`, `Time`, …) or existing module namespace, so no
`-util` suffix is needed (per the naming rule in `lang/go/CLAUDE.md`
"Package layout"). Words are dot-accessed: `Hex.encode`, `Hex.decode`,
`Hex.dump`.

## 4. API

Signatures are **top-first, sig order** (position 0 is the top of the
stack). All inner native sigs use `BarrierPos: -1` so the swap form
dispatches.

| Go symbol | aql word | signature (top-first) | one-line doc | aql-ish refinement |
|---|---|---|---|---|
| `EncodeToString(b)` | `encode` | `[String] -> String` | Encode a string's bytes to lowercase hex. | `[]byte` arg → String's bytes (§5); total, no error. |
| `DecodeString(s) ([]byte,err)` | `decode` | `[String] -> String` | Decode a hex string back to its bytes. | `([]byte,err)` → value-or-error `decode`; bytes wrapped back as a String (§5). |
| `Dump(b) string` | `dump` | `[String] -> String` | Render a `hexdump -C`-style annotated dump (offsets + ASCII gutter), for debugging. | `[]byte` arg → String's bytes; total, no error. |

Notes:
- `EncodeToString` always produces lowercase; there is no separate
  uppercase word (Go's package has none). Callers who want uppercase
  can `StringUtil`-uppercase the result.
- `dump` is a one-way debugging renderer (multi-line text with offsets
  and an ASCII gutter); there is no inverse word.
- The length-prefixed `Encode`/`Decode`/`DecodedLen`/`EncodedLen`
  buffer helpers are folded away — the String words cover the
  whole-value case.

## 5. Types

Scalars only — String. **Byte model:** AQL has no Bytes type, so
binary input/output is modelled as a `String` holding the raw bytes
(its UTF-8 bytes for the common text case). `encode`/`dump` read the
String's bytes; `decode` wraps the decoded bytes back into a String.
For true binary that is not valid UTF-8, the alternative is
`List[Integer]` (each 0–255); this note picks the String model as the
primary surface and flags the `List[Integer]` overload in Open
questions. No opaque handle type, no `RegisterExternalBuiltin` /
FixedID is needed. Convert at the boundary with `eng.FromNative` /
`eng.ToNative` (`eng/go/gobridge.go`).

## 6. Errors

Go `error` returns unwrap to an `AqlError` via `r.AqlError(code,
detail, word)` with a kebab-case code:

| code | raised when |
|---|---|
| `decode` | `DecodeString` fails — odd-length input or a non-hex digit. |

Guard the arg with `AsConcreteString` before use; never panic
(`eng/go/CLAUDE.md` "Panic Prevention").

## 7. Policy / capabilities

None — pure value conversion, runs under any policy.

## 8. Overlap

None with an existing module. No AQL-user-facing word does hex
byte-encoding today. (`BinUtil` in `aql:bin-util` handles integer
*bit* ops and FNV hashing, not byte/text hex transfer encoding — a
different domain.)

## 9. Examples (args-before form)

```
import "aql:hex"

"hi" Hex.encode                        # "6869"
"6869" Hex.decode                      # "hi"
"hello world" Hex.dump                 # "00000000  68 65 6c 6c …  |hello world|\n"
"zz" Hex.decode                        # ERROR:decode   (non-hex digit)
"abc" Hex.decode                       # ERROR:decode   (odd length)
```

## 10. Open questions / out of scope

- **`List[Integer]` byte overload** — the String byte-model is primary
  (§5); whether `encode`/`decode` should also accept/return a
  `List[Integer]` (0–255) for true-binary round-tripping is left open
  until a real binary use case appears.
- **Streaming `NewEncoder` / `NewDecoder` / `Dumper`** — the
  `io.Writer`/`io.Reader` streaming variants are out of scope; the
  whole-String words cover the in-memory case.
- **Uppercase hex** — not provided (Go has no uppercase encoder);
  `StringUtil`-uppercase the `encode` output if needed.

## 11. Implementation sketch

Wiring checklist — no Go code here. Reference: `lang/go/modules/math.go`
(`BuildMathModule`, the pure-module canonical builder).

- `lang/go/modules/hex.go` — `BuildHexModule(parent *native.Registry)
  (native.ModuleDesc, error)`: make an isolated `native.DefaultRegistry()`
  sub-registry, register a `HexNatives []native.NativeFunc` slice (each
  inner sig `BarrierPos: -1`), wrap each word as an `FnDef` export into
  an `*OrderedMap`, return `ModuleDesc{ID: parent.Modules.NextID(),
  Exports: {"Hex": …}}`.
- Register `"hex": BuildHexModule` in the `modules` map in
  `lang/go/modules/modules.go`.
- `lang/go/modules/docs_hex.go` — `registerDocs("aql:hex",
  map[string]string{…})` with a one-line summary per export
  (`TestModuleExportDocs` enforces completeness).
- `lang/spec/module-hex.tsv` — `input⇥expected⇥description` rows, each
  leading with `import "aql:hex"`; every positive row paired with an
  `ERROR:<substring>` negative sibling (Test discipline,
  `lang/go/CLAUDE.md`).
- No FixedID entry (no external type), no policy wiring.
