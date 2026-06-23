# `hash/crc64` → `aql:crc64`

> **Status: design proposal — not implemented.** A curated, hand-written
> native module wrapping Go's `hash/crc64` with an idiomatic ("aql-ish")
> surface. Read [`README.10.md`](README.10.md) first for the shared
> conventions this note assumes.

## 1. Package & status

Go package: [`hash/crc64`](https://pkg.go.dev/hash/crc64) — the 64-bit
cyclic redundancy check, in two standard polynomials (ISO 3309, used by
HDLC / Go's own gzip member-trailer lineage; ECMA-182, used by tape and
many storage formats). This note specifies `aql:crc64` (namespace
`Crc64`), a value-oriented wrapper that reduces a String to its checksum.
Design proposal; no Go code exists yet.

## 2. Why curated

The raw `go:hash/crc64` bridge would surface the stateful builder
machinery — `crc64.New(table)` returns a `hash.Hash64`, the caller drives
`Write([]byte)` then `Sum64()`, and `crc64.MakeTable(crc64.ISO)` returns
a `*crc64.Table` pointer with no meaning as AQL data. The curated surface
collapses each polynomial to one total word (`iso` / `ecma`) that takes a
String and returns its checksum directly — and, crucially, **chooses a
return representation that does not silently corrupt the value** (see
§4), which a mechanical bridge of `Sum64() uint64` would not.

## 3. Import & namespace

```
import "aql:crc64"        # binds the Crc64 namespace
```

The bare capitalized package name `Crc64` is free (not a builtin type,
not an existing module namespace), so **no `-util` suffix** is needed
(per the naming rule in `lang/go/CLAUDE.md` "Package layout"). Words are
dot-accessed: `Crc64.iso`, `Crc64.ecma`, …

## 4. API

Signatures are **top-first, sig order** (position 0 = top of stack), per
the README "Argument order & dispatch" rule. All inner natives use
`BarrierPos: -1` so dispatch is uniform (these are single-arg words).

| Go symbol | aql word | signature (top-first) | one-line doc | aql-ish refinement |
|---|---|---|---|---|
| `crc64.MakeTable(ISO)` + `crc64.Checksum` | `iso` | `[String] -> String` | CRC-64 checksum (ISO 3309 polynomial) as a 16-char hex String. | The `NewISO`/`Write`/`Sum64` protocol and the `*crc64.Table` pointer collapse to one pure word. **Returns hex** (`%016x`), not Integer — see "uint64 overflows int64" below. |
| `crc64.MakeTable(ECMA)` + `crc64.Checksum` | `ecma` | `[String] -> String` | CRC-64 checksum (ECMA-182 polynomial) as a 16-char hex String. | Same shape as `iso` with `crc64.ECMA`; pure `String → String` (hex). |
| `crc64.MakeTable(ISO)` (signed Integer render) | `iso-int` | `[String] -> Integer` | ISO checksum reinterpreted as a signed int64 Integer. | *Optional, caveated.* The uint64 bits reinterpreted as `int64` — round-trips losslessly but values with the top bit set read as **negative** Integers. |
| `crc64.MakeTable(ECMA)` (signed Integer render) | `ecma-int` | `[String] -> Integer` | ECMA checksum reinterpreted as a signed int64 Integer. | *Optional, caveated.* Same `int64(sum)` reinterpretation as `iso-int`. |

### uint64 overflows int64 — why hex is the default

`crc64.Checksum` returns a Go `uint64` (0 .. 18,446,744,073,709,551,615).
AQL's `Integer` is a signed `int64`, whose maximum is
9,223,372,036,854,775,807 — **roughly half** the uint64 range. Any
checksum with the high bit set therefore **does not fit** in an AQL
Integer: a naive `NewInteger(int64(sum))` would wrap such values to
*negative* numbers, which is surprising, breaks ordering/comparison
expectations, and silently differs from the canonical unsigned checksum
that every other CRC-64 tool prints.

The default words (`iso` / `ecma`) therefore return the checksum as a
**16-char zero-padded lowercase hex String** (`%016x`). Hex is:

- **lossless** — all 64 bits survive, unlike a truncated/wrapped Integer;
- **canonical** — it matches how CRC-64 digests are conventionally
  written and compared across tools;
- **composable** — a fixed-width String concatenates and compares
  cleanly, the same choice the crypto-digest modules make for their
  longer hashes.

(Contrast [CRC32](HASH-CRC32.10.md), whose uint32 result fits int64
exactly, so it returns a plain Integer.)

### The signed-Integer variant (opt-in, with a caveat)

For callers who genuinely want a numeric value (arithmetic, bucketing),
`iso-int` / `ecma-int` return `int64(sum)` — a **bit-reinterpretation**,
not a magnitude conversion. It round-trips the full 64 bits losslessly
(`uint64(int64(sum)) == sum`), but **values with the top bit set appear
negative**. That is documented as the explicit trade-off of these words;
the hex default avoids the surprise for everyone who does not opt in.

### Input modeling

Input is a **String**; the bytes hashed are its UTF-8 byte encoding —
the same convention as [CRC32](HASH-CRC32.10.md) and the crypto-digest
modules. AQL has no Bytes type (see [BYTES.10.md](BYTES.10.md)).

## 5. Types

Scalars only — String in; String (hex) or Integer (the `-int` variants)
out. **No opaque external handle:** the `*crc64.Table` values are
internal package-level Go vars, never a `RegisterExternalBuiltin` /
FixedID type. Convert at the boundary with `eng.FromNative` /
`eng.ToNative` (`eng/go/gobridge.go`).

## 6. Errors

No panics (`eng/go/CLAUDE.md` "Panic Prevention"). The only failure mode
is a non-String argument, guarded with `AsConcreteString` before use and
signalled via `r.AqlError(code, detail, word)`:

| code | raised when |
|---|---|
| `bad-arg` | the argument is not a concrete String (type literal / wrong type). |

The CRC computation itself is total, so there is no Go `error` to unwrap.

## 7. Policy / capabilities

**None — pure.** Purely in-memory arithmetic over the input bytes. Runs
under any policy.

## 8. Overlap

None with an existing module. `aql:bin-util` (`BinUtil`) owns `hash/fnv`
and the bitwise words (per the README roster) but exposes no CRC words,
so `aql:crc64` is a genuinely new surface and changes nothing in
`aql:bin-util`. Sibling [CRC32](HASH-CRC32.10.md) is the 32-bit member of
the same family; the two share shape but differ in the return-type
decision (Integer vs hex String) precisely because of the int64 range
boundary.

## 9. Examples (args-before form)

All args-before form (`s Crc64.word`); never pure forward `Crc64.word s`.

```
import "aql:crc64"

"hello" Crc64.iso                  # "31b0a35b4 baf...".  16-char hex String
"hello" Crc64.ecma                 # 16-char hex String (ECMA polynomial)
"" Crc64.iso                       # "0000000000000000"  (empty input)
"hello" Crc64.iso-int              # signed int64 reinterpretation (may be negative)
42 Crc64.iso                       # ERROR:bad-arg   (Integer, not String)
```

## 10. Open questions / out of scope

- **Whether to ship `iso-int` / `ecma-int` at all.** They add a sharp
  edge (negative checksums) for a niche need. Defaulting to hex and
  *omitting* the signed variants until someone asks is the conservative
  option; this note proposes them as opt-in but flags the decision.
- **A uint64-as-`math/big` path** — returning the full unsigned value as
  a big Integer (via `aql:math-util`'s big lineage) is an alternative to
  hex that keeps a *number*. Deferred: hex is simpler, canonical, and
  enough for the error-detection use case CRC-64 actually serves.
- **Custom polynomials / streaming** — out of scope, same reasoning as
  [CRC32](HASH-CRC32.10.md): the two named standards and the one-shot
  surface cover the realistic cases.

## 11. Implementation sketch

Wiring checklist — no Go code here. Reference: `lang/go/modules/math.go`
(`BuildMathModule`, the pure-module canonical builder).

- `lang/go/modules/crc64.go` — `BuildCrc64Module(parent *native.Registry)
  (native.ModuleDesc, error)`: isolated `native.DefaultRegistry()`
  sub-registry, register a `Crc64Natives []native.NativeFunc` slice (each
  inner sig `BarrierPos: -1`), build the two `*crc64.Table` values once as
  package-level vars, wrap each word as an `FnDef` export into an
  `*OrderedMap`, return `ModuleDesc{ID: parent.Modules.NextID(),
  Exports: {"Crc64": …}}`. The default words render `%016x`; the `-int`
  words return `native.NewInteger(int64(sum))`.
- Register `"crc64": BuildCrc64Module` in the `modules` map in
  `lang/go/modules/modules.go`.
- `lang/go/modules/docs_crc64.go` — `registerDocs("aql:crc64",
  map[string]string{…})` with a one-line summary per export
  (`TestModuleExportDocs` enforces completeness).
- `lang/spec/module-crc64.tsv` — `input⇥expected⇥description` rows, each
  leading with `import "aql:crc64"`; pin the hex width and the empty-input
  case, and pair every positive row with an `ERROR:<substring>` negative
  sibling (Test discipline, `lang/go/CLAUDE.md`). If `-int` ships, pin a
  top-bit-set input that demonstrates the negative result so the caveat
  is part of the contract.
- Boundary conversion via `eng.FromNative` / `eng.ToNative`. No FixedID
  entry (no external type), no policy wiring.
