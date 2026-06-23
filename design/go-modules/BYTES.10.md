# The `Bytes` type — a first-class binary leaf

> **Status: design proposal, not implemented.** This note specifies a new
> AQL value type, `Bytes`, the foundation for all binary-adjacent
> functionality. The words that construct, inspect, encode, hash, and
> sign byte data live in the expanded `aql:bin-util` module — see
> [BIN-UTIL.10.md](BIN-UTIL.10.md). Read [README.10.md](README.10.md)
> first for the shared conventions.

## 1. Why a type (not List[Integer] or String)

AQL has no binary representation today: file reads degrade to `String`
(UTF-8), and `eng.FromNative`/`ToNative` (`eng/go/gobridge.go`) have **no
`[]byte` case** — a Go byte slice falls through to a best-effort
`fmt.Sprintf` string. Every binary operation (hashing, base64/hex,
HMAC, CRC, UUID bytes, secure random) is defined over `[]byte`, so a
coherent surface needs one shared, lossless, type-safe carrier.

The two cheaper models were rejected:

- **`List[Integer]`** — workable but verbose, allocation-heavy (one
  `IntPayload` per byte), and ambiguous (any list of small ints *looks*
  like bytes). It cannot carry a Comparer/Format that reads as binary.
- **`String`** — composes with file reads but is lossy for non-UTF-8
  data and conflates text with bytes.

A real `Bytes` leaf makes binary **first-class and aql-native**: `x is
Bytes` works, values render as hex, comparison is byte-lexicographic,
and `[]byte` round-trips through the Go bridge unchanged.

## 2. Registration

`Bytes` is a **global external builtin**, registered in a new
`lang/go/native/native_bytes.go`, mirroring the established external-type
pattern used for `Time`, the fetch result, `Timeout`, and `Interval`
(`RegisterExternalBuiltin` with a stable `FixedID`):

- **FixedID** — allocate the next free value in the `lang/go/native`
  scalar band **1000–1999** (`eng/go/CLAUDE.md` "FixedID Allocation";
  document the exact number chosen) and pin it in
  `lang/go/test/fixedid_stability_test.go`. (Not the 10000+ host range —
  this is a language builtin, not a host plugin type.)
- **No-panic init** — registration records any error via the
  `TypeInitError` path checked at `DefaultRegistryWithPolicy`, per
  ADR-005 (`eng/go/CLAUDE.md` "Panic Prevention" — no init-time panic).
- Path/name: `Ideal/Bytes` (a concrete data leaf), exported so
  `BinUtil`-built values type as `Bytes` and `x is Bytes` resolves.

## 3. Payload & behavior

- **Payload** — `ExtensionPayload{Body: []byte}` via `eng.NewExtension`
  (`eng/go/payload.go`), so **no kernel struct change** is required; the
  slice is the whole state.
- **`TypeBehavior`** (the seam external types implement):
  - `Format` → hex, length-capped, e.g. `Bytes(5) 68656c6c6f` (short) and
    `Bytes(1024) 0001020304…` (truncated with the length shown). The full
    value is obtained with `BinUtil.hex-encode` / `base64-encode`.
  - `Comparer` → byte-lexicographic (`bytes.Compare`), opening with the
    `litVsConcreteOrder` rule so the bare `Bytes` literal sorts below every
    concrete value (`design/TYPE-ORDERING.10.md`, `eng/go/compare.go`).
  - `Equal` → bytewise (`bytes.Equal`). (HMAC verification uses a separate
    *constant-time* compare — see BIN-UTIL.10.md — not this `Equal`.)
  - `DeepCloner` → copy the backing slice (`eng/go/clone.go`) so a cloned
    `Bytes` shares no storage with the original.

## 4. The value bridge (the one `eng/` change)

Extend `eng/go/gobridge.go`:

- `FromNative([]byte)` → a `Bytes` value (wrapping the slice).
- `ToNative(Bytes)` → the underlying `[]byte`.

This is the only change outside `lang/`. It lets every wrapped Go call in
`bin-util` (`sha256.Sum256`, `base64.DecodeString`, `uuid.New`, …) pass
and receive bytes without per-word boilerplate, and makes `Bytes` the
natural target for a future binary file read.

## 5. Construction, inspection, conversion (words live in `bin-util`)

The type ships no words of its own; the operations are concentrated in
`aql:bin-util` (the user's explicit consolidation). The core set
(detailed in [BIN-UTIL.10.md](BIN-UTIL.10.md)):

| word | signature (top-first) | meaning |
|---|---|---|
| `bytes` | `[String] -> Bytes` / `[List sub] -> Bytes` | UTF-8 encode a String, or pack a List of Integers (0–255). |
| `text` | `[Bytes] -> String` | decode UTF-8 (error `invalid-utf8`). |
| `ints` | `[Bytes] -> List` | unpack to a List of Integers (0–255). |
| `length` | `[Bytes] -> Integer` | byte count. |
| `slice` | `[Integer hi, Integer lo, Bytes b] -> Bytes` | sub-slice `[lo, hi)`. |
| `concat` | `[List of Bytes] -> Bytes` | join byte sequences. |
| `byte-at` | `[Integer i, Bytes b] -> Integer` | the byte at index `i` (error `index-range`). |
| `equal` | `[Bytes b, Bytes a] -> Boolean` | bytewise equality. |

Words that take "data to hash/encode" accept **`String` or `Bytes`** (a
String is UTF-8 encoded first), so the common text case stays terse.

## 6. Errors

| code | raised when |
|---|---|
| `expected-byte` | a List element passed to `bytes` is not an Integer in 0–255. |
| `invalid-utf8` | `text` is given bytes that are not valid UTF-8. |
| `index-range` | `byte-at` / `slice` index is out of range. |

Guard with `RequireConcreteList` / a `Bytes` unwrap helper and
range-check before converting; never panic (`eng/go/CLAUDE.md`).

## 7. Policy / capabilities

None — the type and its in-memory operations are pure. (The *entropy*
words that produce `Bytes` — `random-bytes`, `uuid` — are gated; that
gating lives with those words in BIN-UTIL.10.md, not on the type.)

## 8. Overlap

Net-new — there is no binary type today. It *complements* `String`:
`bytes`/`text` are the explicit, lossless bridge between the two, where
previously binary silently became a (possibly lossy) String at the I/O
boundary.

## 9. Examples (args-before form)

```
import "aql:bin-util"

"hello" BinUtil.bytes                       # Bytes(5) 68656c6c6f
"hello" BinUtil.bytes BinUtil.length        # 5
[104 105] BinUtil.bytes BinUtil.text        # "hi"
"hi" BinUtil.bytes BinUtil.ints             # [104 105]
"café" BinUtil.bytes BinUtil.length         # 5  (é is two UTF-8 bytes)
[256] BinUtil.bytes                          # ERROR:expected-byte
```

## 10. Open questions / out of scope

- **Binary file I/O** — an `IO.read-bytes` (and `IO.write` accepting
  `Bytes`) yielding/consuming `Bytes` instead of String is the natural
  follow-on, but lives in `aql:io`; out of scope here. Cross-reference
  once Bytes lands.
- **Bytes literal syntax** — none proposed; construct via `BinUtil.bytes`
  / decode words. A `0x…` literal could be added later.
- **Mutability** — `Bytes` is treated as immutable (slice operations
  return fresh values); the shared-pointer capture caveat
  (`lang/go/CLAUDE.md` "Capture semantics") does not apply because no word
  mutates in place.

## 11. Implementation sketch

- `eng/go/gobridge.go` — add the `[]byte` ↔ `Bytes` cases (§4).
- `lang/go/native/native_bytes.go` — `registerBytesType()` via
  `RegisterExternalBuiltin("Ideal/Bytes", <FixedID 1000–1999>,
  bytesBehavior{})`; behavior implements `Format`/`Comparer`/`Equal`/
  `DeepCloner` (§3); funnel init errors through `TypeInitError`.
- `lang/go/test/fixedid_stability_test.go` — pin the new FixedID.
- Construction/inspection words are added in `lang/go/modules/binary.go`
  (see BIN-UTIL.10.md), not here.
- Tests: a `Bytes` round-trip (`bytes`→`text`, `bytes`→`ints`), ordering
  (`cmp`, type-literal-first), `Equal`, deep clone, and the negative
  `expected-byte` / `invalid-utf8` / `index-range` cases (Test discipline,
  `lang/go/CLAUDE.md`).
