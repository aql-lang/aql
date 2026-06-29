# The `Bytes` type — a first-class binary leaf + bit-syntax

> **Status: design proposal, not implemented.** This note specifies a new
> AQL value type, `Bytes`, the foundation for all binary-adjacent
> functionality, together with its **storage & memory model** (§4) and the
> **`pack`/`unpack` bit-syntax** (§7) that binary wire protocols are written
> against. The heavier binary words (cryptographic hashes, HMAC,
> CRC, base/hex encodings, secure random, UUIDs) live in the expanded
> `aql:bin-util` module — see [BIN-UTIL.10.md](BIN-UTIL.10.md). The type, its
> literal, its interop overloads, and the bit-syntax are **core** (no import).
> Read [README.10.md](README.10.md) for the shared conventions.
>
> **Decisions taken at design-review time:** (1) this doc is the **authoritative
> full `Bytes` design** (the earlier type-only draft is folded in); (2) the
> word surface is **signature overloads of existing words** — `convert`
> (text/ints ⇄ Bytes, compact), `slice`, `add`, `make Bytes` (pack), `unpack`
> (decode) — plus the one new `unpack-prefix`; crypto/encoding/hash/uuid stay
> in `aql:bin-util`; (3) hex/binary byte constants are the **`+hb/…/` and
> `+bb/…/` minilang kinds** (`import "aql:minilang"`), **not** a core lexer
> literal — there is no `0x"…"` or `b"…"` form (see §6); (4) the memory model is
> **zero-copy share + copy-on-ingest** (this supersedes the earlier
> `DeepCloner` decision — see §4).
>
> Used by `NETWORK-SERVERS.0.md` §3, `NETWORK-CLIENTS.0.md` §3, and
> `STREAM-WORDS.0.md` (`from-bytes`/`to-bytes`), which now reference this doc
> rather than sketching the type inline.

## 1. Why a type (not List[Integer] or String)

AQL has no binary representation today: file reads degrade to `String`
(UTF-8), and `eng.FromNative`/`ToNative` (`eng/go/gobridge.go`) have **no
`[]byte` case** — a Go byte slice falls through to a best-effort
`fmt.Sprintf` string. Every binary operation (hashing, base64/hex, HMAC, CRC,
UUID bytes, secure random) is defined over `[]byte`, and — newly — **network
framing** (`NETWORK-SERVERS.0.md`) reads and writes raw octets off a socket.
A coherent surface needs one shared, lossless, type-safe carrier.

The two cheaper models were rejected:

- **`List[Integer]`** — workable but verbose, allocation-heavy (one
  `IntPayload` per byte — 100s of bytes of overhead per KB of data), and
  ambiguous (any list of small ints *looks* like bytes). It cannot carry a
  Comparer/Format that reads as binary, and it defeats the zero-copy messaging
  (§4) that the actor/service layers depend on.
- **`String`** — composes with file reads but is lossy for non-UTF-8 data and
  conflates text with bytes.

A real `Bytes` leaf makes binary **first-class and aql-native**: `x is Bytes`
works, values render as hex, comparison is byte-lexicographic, `[]byte`
round-trips through the Go bridge unchanged, and — because the backing slice is
never mutated in place (§4) — a `Bytes` is **shared zero-copy** across forks,
sends, and slices, exactly like a `String`. That last property is what lets a
parsed frame flow from a socket through a handler and into a reply with no
intermediate copy (`SERVICES.0.md` §7.1).

## 2. Registration

`Bytes` is a **global external builtin**, registered in a new
`lang/go/native/native_bytes.go`, mirroring the established external-type
pattern used for `Time`, the fetch result, `Timeout`, and `Interval`
(`RegisterExternalBuiltin` with a stable `FixedID`):

- **FixedID** — allocate the next free value in the `lang/go/native` scalar
  band **1000–1999** (`eng/go/CLAUDE.md` "FixedID Allocation"; document the
  exact number chosen) and pin it in `lang/go/test/fixedid_stability_test.go`.
  (Not the 10000+ host range — this is a language builtin, not a host plugin
  type.)
- **No-panic init** — registration records any error via the `TypeInitError`
  path checked at `DefaultRegistryWithPolicy`, per ADR-005 (`eng/go/CLAUDE.md`
  "Panic Prevention" — no init-time panic).
- Path/name: `Scalar/Bytes` — a concrete **Scalar** data leaf (sibling of
  `String` and the `Scalar/Time` family, so `x is Scalar` holds, and the
  `Scalar/…` prefix agrees with the 1000–1999 band above), exported so core and
  `BinUtil`-built values type as `Bytes` and `x is Bytes` resolves.

## 3. Payload & behavior

- **Payload** — `ExtensionPayload{Body: bytesBody{data []byte}}` via
  `eng.NewExtension` (`eng/go/payload.go`), so **no kernel struct change** is
  required; the slice is the whole state.
- **`TypeBehavior`** (the seam external types implement):
  - `Format` → hex, length-capped, e.g. `Bytes<68 65 6c 6c 6f>` (short) and
    `Bytes<00 01 02 03 04 … (1024)>` (truncated with the length shown). The full
    value is obtained with `BinUtil.hex-encode` / `base64-encode`.
  - `Comparer` → byte-lexicographic (`bytes.Compare`), opening with the
    `litVsConcreteOrder` rule so the bare `Bytes` literal sorts below every
    concrete value (`design/TYPE-ORDERING.10.md`,
    `eng/go/compare_scalar_behaviors.go`).
  - `Equal` → bytewise (`bytes.Equal`). (HMAC verification uses a separate
    *constant-time* compare — see BIN-UTIL.10.md — not this `Equal`.)
  - `Sizer` → byte count, so the generic `length` / `size` word works on
    `Bytes` with no dedicated word (`eng/go/typebehavior.go` optional
    capabilities).
  - **No `DeepCloner`.** `Bytes` is immutable; clone shares the slice — see §4.

## 4. Storage & memory model

This is the load-bearing section: `Bytes` is designed so that the common
network/data paths copy **once, at ingest, and never again.**

### 4.1 Immutable, shared, zero-copy

The backing store is a single Go `[]byte` inside `bytesBody`. **No word mutates
a `Bytes` in place** — every operation (`slice`, `add`, `make Bytes`, …) returns a
fresh value. Because the storage is therefore never written after construction,
a `Bytes` is **shared by reference on clone** and pays nothing:

- `CloneValue` (`eng/go/clone.go`) shares an `ExtensionPayload` whose `Body`
  does **not** implement `DeepCloner`. `bytesBody` deliberately omits it, so a
  cloned `Bytes` shares the original's slice header and backing array — exactly
  how `StrPayload` (an immutable Go `string`) is shared.
- This is what makes `PROCESSES.0.md` §6 / `SERVICES.0.md` §7.1 **zero-copy
  messaging** real for binary: `send`ing a `Bytes` to another process, or
  `ForkConcurrent`-copying a registry that holds one, copies a slice header
  (24 bytes), not the payload. A 1 MB frame received on a socket reaches a
  handler in another goroutine with zero byte-copies.
- The Go memory model's happens-before edge on the mailbox send, plus the
  no-writer guarantee from immutability, means the receiver observes a fully
  published, race-free value (`SERVICES.0.md` §7.1).

> This **supersedes** the earlier draft's `DeepCloner` choice. Deep-cloning an
> immutable value on every fork/send is pure cost with no safety benefit; the
> only place a copy is genuinely needed is at the *ingest* boundary (§4.2).

### 4.2 Copy-on-ingest at trust boundaries

The one place a defensive copy is mandatory is where a `[]byte` enters AQL from
code that may still mutate it:

- **`eng.FromNative([]byte)`** copies the incoming slice before wrapping it. A
  Go caller (or a reused buffer) must not be able to mutate a live `Bytes`.
- **Socket `recv`** (`NETWORK-SERVERS.0.md` §4) reads into a reusable buffer and
  hands the handler a `Bytes` over a **copy** of just the bytes read — the read
  buffer is then free to be reused for the next read without corrupting the
  frame already in flight.

After ingest, the `Bytes` owns its storage and is immutable, so every
downstream op is copy-free. The cost model is thus "**copy once at the edge,
share forever after**".

### 4.3 Slice views share the backing array

`slice` and the bit-syntax `bytes(n)` segments (§7) return a **sub-slice view**
that shares the parent's backing array — zero-copy, O(1). This is ideal for
parsing (a length-prefixed body is a view into the read buffer, not a copy), but
carries Go's standard slice-retention caveat:

- **Retention caveat.** A small view keeps its *entire* backing array alive: a
  4-byte `slice` of a 1 MB frame pins all 1 MB until the view is unreachable.
- **`convert Bytes <Bytes>`** forces a minimal fresh copy (`len`-sized backing
  array) for the rare case where a small slice must outlive a large buffer
  (e.g. caching one field extracted from each of many big frames). Converting
  Bytes to Bytes is the "compact" operation; you only reach for it when
  retention actually bites.

### 4.4 Cost summary

| Operation | Copies | Notes |
| --- | --- | --- |
| construct from external `[]byte` / `recv` | 1 | the ingest copy (§4.2) |
| `+hb/…/` / `+bb/…/` literal kind | 1 (at the call) | the decoded constant, built once |
| clone / `ForkConcurrent` / `send` | 0 | slice header only (§4.1) |
| `slice`, `unpack` `bytes(n)` view | 0 | sub-slice view (§4.3) |
| `convert Bytes <bytes>` (compact) | 1 | force-copy to drop retention |
| `add` (concat) | 1 | one build of the joined slice |
| `make Bytes [spec]` (pack) | 1 | one build of the frame |
| `convert String`/`convert Bytes` (text⇄bytes) | 1 | crosses the bytes⇄text boundary |

## 5. Word surface — overloads, not new words

Bytes is exposed through **signature overloads of existing words** (no new core
words except the streaming `unpack-prefix`). A `Bytes` overload is a
more-specific signature that wins dispatch over the generic
`[Scalar …]`/`[Any …]` form, so `convert`/`slice`/`add`/`make`/`unpack` "just
work" on Bytes the way they already do on String/List/Number. Heavier binary
operations (crypto, encodings, hashing, random, UUID) live in `aql:bin-util`
(BIN-UTIL.10.md), each accepting `String | Bytes`.

| operation | overload | form |
| --- | --- | --- |
| encode text → Bytes (UTF-8) | `convert` | `convert Bytes "hi"` |
| decode Bytes → text (raises `bad-encoding`) | `convert` | `convert String b` |
| ints (0–255) → Bytes | `convert` | `convert Bytes [104 105]` |
| Bytes → ints | `convert` | `convert List b` |
| compact (force a fresh minimal copy, §4.3) | `convert` | `convert Bytes b` |
| sub-range (end-exclusive, zero-copy view) | `slice` | `slice <start> <end> b` |
| concatenate two byte strings | `add` | `a add b` |
| pack a frame from a bit-spec (§7) | `make` | `make Bytes [spec]` |
| pack a frame — terse sugar for `make Bytes` (§7) | `bytes` | `bytes [spec]` |
| decode a frame, binding fields (§7) | `unpack` | `unpack b [spec]` |
| streaming-decode a leading frame (§7) | `unpack-prefix` | `unpack-prefix b [spec]` |

`size`/`length`, `eq`, value ordering (`cmp`/`lt`/`sort`), and `is Bytes` need
**no dedicated word** — they resolve through the `Sizer`/`Equal`/`Comparer`
behaviors (§3). The `+hb/…/` / `+bb/…/` minilang kinds (§6) cover hex/binary
*input*; `BinUtil.hex-encode` covers hex *output*.

**Why overloads.** `Bytes` is a value type like `String`, so it slots into the
generic words by type-dispatch rather than adding a parallel `byte-*`
vocabulary. This keeps the core namespace small. The two dedicated words are
`bytes` — a one-token alias for the very common `make Bytes [spec]` pack — and
`unpack-prefix`, a genuinely new streaming operation with no generic equivalent.
(Implementation: each overload is registered from `native_bytes.go` and
**appended** to the existing word via `RegisterNativeFunc`; no edits to the
host words' files are needed.)

## 6. Hex / binary constants — the `+hb/…/` and `+bb/…/` minilang kinds

Byte constants for protocol magic numbers and fixtures are written with two
**`aql:minilang` kinds** rather than a bespoke core lexer literal: `hb` (hex
bytes) and `bb` (binary bytes), invoked through the standard `+name<delim>src<delim>`
literal sugar:

```aql
import "aql:minilang"

+hb/deadbeef/       # Bytes<de ad be ef>
+hb/de ad be ef/    # Bytes<de ad be ef>   (whitespace/`_` group, ignored)
+bb/01001100/       # Bytes<4c>            (8 bits, MSB-first)
+bb/01001100 11110000/   # Bytes<4c f0>   (grouping ignored)
```

- **`hb`** decodes an **even** number of hex digits (`[0-9a-fA-F]`); spaces,
  tabs, newlines and `_` are grouping separators and dropped. An odd count or a
  non-hex digit raises `mini_parse_error` when the kind runs.
- **`bb`** decodes a run of `0`/`1` whose length (after dropping the same
  grouping separators) is a **multiple of 8**, MSB-first per byte. A non-binary
  digit or a non-multiple-of-8 length raises `mini_parse_error`.
- Both are **generator kinds** (`[src:String opts:Map] → [Bytes]`), so the
  explicit `mini hb 'deadbeef'` / desugared `MiniLang.lang_hb 'deadbeef' {} end`
  forms are equivalent — see `lang/spec/module-minilang.tsv` §6.
- **Why a minilang kind, not a `0x"…"` core literal.** Byte constants are an
  *occasional* need, dominated by the network/binary code that already imports
  `aql:minilang` for its other DSLs; folding them into the existing
  `+name/src/` lexer sugar avoids a new core token and the `0x`-prefix
  disambiguation it would require (`0xff` stays the `Integer` 255 —
  `isBasePrefixedInteger`, `eng/go/parser/parse.go` — untouched). There is **no**
  `b"…"` ASCII literal either — use `convert Bytes "GET "` for text-as-bytes,
  which keeps the encoding explicit.
- **The `+hb/…/` result feeds the words directly**, but note the auto-`end` the
  literal sugar emits stops forward collection, so a kind result that is itself
  a forward arg must be parenthesised: `unpack (+hb/0100026869/) [ {name:'op' type:'u8'} … ]`.
  Implementation: `lang/go/modules/minilang.go` (`hb`/`bb` kinds + handlers),
  `lang/go/modules/docs_minilang.go` (docs).

## 7. The layout spec — JSON-representable Node data (`make Bytes` / `unpack`)

Erlang's bit syntax (`<<Len:16, Body:Len/binary>>`) is what makes binary code
short *and* safe. AQL's equivalent is a **layout spec**: a `List` of segment
`Map`s — plain, JSON-representable Node data that the handler only **reads**.
`make Bytes [spec]` packs (or the equivalent `bytes [spec]` sugar); `unpack b
[spec]` decodes and binds; `unpack-prefix b [spec]` is the streaming variant.

> **No secondary parsing (ADR-007).** The spec is ordinary Node data — there is
> **no** token-level sub-language and no in-handler parser. An earlier draft
> captured the spec raw (`name:u16/be`, a trailing `(len)` paren) and
> `strings.Split`/`strconv`-interpreted it; that was a second grammar living
> below the real one. It is removed. The seg-`type` is now a plain String the
> handler enum-dispatches on (data interpretation, not parsing), and the whole
> spec is constructable by a macro and serialisable as JSON. It is also **not**
> an `aql:minilang` (`MINILANG.5.md`) kind — those take an opaque String `src`;
> this is structured data.

### 7.1 Segment schema

A spec is a `List` of segment `Map`s. Each Map is a fully-explicit descriptor —
one key per attribute, every value a String / Integer / Boolean:

```
[ {name:'ver'  type:'u8'}
  {name:'len'  type:'u16' endian:'le'}
  {name:'body' type:'bytes' size:'len'} ]
```

| key | type | meaning |
| --- | --- | --- |
| `name` | String | **bind** (unpack) / **read from scope** (pack) this field. |
| `value` | Integer | a **pack constant** / an **unpack match-guard**. Mutually exclusive with `name`; exactly one is required. |
| `type` | String | `u8 u16 u32 u64`, `i8 i16 i32 i64`, `f32 f64` (IEEE-754, ref `IEEE-754-COMPLIANCE.8.md`), `bits` (sub-byte uint), `bytes` (raw `Bytes`), `utf8` (length-known text → `String`), `pad` (zero bits, no binding). |
| `endian` | String | `"be"` (default, network order) or `"le"`. |
| `signed` | Boolean | overrides the `u`/`i`-prefix default (ints only). |
| `size` | Integer \| String | a literal count, or a **String naming a previously-bound field** (`size:'len'`, Erlang's `Body:Len/binary` — the killer feature). Required for `bits`/`pad`; for `bytes`/`utf8` omitting it means "the rest". |

The `name`-vs-`value` split replaces the old "is the key numeric?" test; the
separate `endian`/`signed` keys replace the old `/be /le /signed` string
suffixes; the `size` key replaces the positional `(N)`/`(name)` paren — all
three were secondary-parsing devices.

### 7.2 `make Bytes [spec]` — build a frame

```
make Bytes <List<segment-Map>> -> Bytes
```

Reads the segments left-to-right into a single freshly-built `[]byte` (one
allocation, §4.4). A `name` key reads `name` from scope; a `value` key packs the
constant. Numerics honour width/`endian`/`signed`. (Overloads `make`'s
constructor.)

### 7.3 `unpack b [spec]` — decode & bind

```
unpack <Bytes> <List<segment-Map>>      # binds each field into scope; returns nothing
```

Reads the segments over the input bytes, **binding each `name` into scope**
(consistent with the existing `unpack [names] map`). A `value` key is a
**guard**; `bytes`/`utf8` segments are **zero-copy sub-slice views** (§4.3). A
guard mismatch or a too-short buffer `raise`s `no_match`; trailing bytes after
the spec are left unread (use `unpack-prefix` to capture them).

### 7.4 `unpack-prefix` — the streaming framer

```
unpack-prefix <Bytes> <List<segment-Map>> -> {ok: Map  rest: Bytes} | {need: Integer}
```

Reads **only what the spec requires** and returns the bound fields plus the
**leftover bytes** (`rest`), or — when the buffer is too short to complete the
spec — `{need: n}` (how many more bytes are required, when computable, else a
positive lower bound). This is the single primitive a length-prefixed framing
loop needs: read header → learn length → read body, declaratively, with no
hand-rolled buffer state machine (`NETWORK-SERVERS.0.md` §5.3, §6.1).

### 7.5 Alignment rule

`make`/`unpack` are **byte-aligned by default**: a contiguous run of `bits`
(and `pad`) segments must sum to a whole number of bytes, or the spec `raise`s
**`unaligned`** at build time. There is no implicit trailing padding — add an
explicit `pad` segment to round out a sub-byte run. (This resolves
`NETWORK-SERVERS.0.md` §12 Q2 in favour of safety: ragged frames are an error,
not a silent surprise.)

### 7.6 Reading & binding

`readBitSegments` reads each segment Map field by field — no token parsing
(§7's ADR-007 note). `unpack` binds decoded fields into scope via `InstallDef`
(the same binding the `def` word and the existing `unpack` use); `unpack-prefix`
collects them into the `{ok}` map instead. Size names (`size:'len'`) resolve
against the frame's already-decoded fields first, then scope — so the
non-binding `unpack-prefix` still resolves `len`.

> **Open: layouts as named types.** Because a spec *is* a binary record schema,
> the more AQL-native model is to make it a named **type** —
> `def Packet (refine Bytes [layout])`, then `make Packet {fields}` / `unpack
> Packet bytes` — which also removes the `make Bytes`-takes-data-**or**-a-spec
> overload. That evolution is under review; the Node-data spec above is the
> representation it would carry as a type body, so this section holds either way.

## 8. Usage examples

Forward form. `import "aql:bin-util"` only where a crypto/encoding word appears,
`import "aql:minilang"` where an `+hb/…/` / `+bb/…/` constant appears; the type
and bit-syntax need no import.

```aql
# hex/binary constants via the minilang kinds; 0xff (no quote) stays an Integer
import "aql:minilang"
+hb/deadbeef/                              # Bytes<de ad be ef>
+bb/01001100/                              # Bytes<4c>
0xff                                       # 255

# text round-trip (é is two UTF-8 bytes)
convert String (convert Bytes "café")     # 'café'
size (convert Bytes "café")               # 5

# build + match a [ver=1][u16 len][len-byte body] frame
def body ( convert Bytes "hello" )        # Bytes<68 65 6c 6c 6f>
def len  ( size body )
def msg  ( make Bytes [ {value:1 type:'u8'}  {name:'len' type:'u16'}  {name:'body' type:'bytes'} ] )
# msg == Bytes<01 00 05 68 65 6c 6c 6f>
unpack msg [ {name:'ver' type:'u8'}  {name:'n' type:'u16'}  {name:'body' type:'bytes' size:'n'} ]   # binds ver, n, body into scope
convert String body                        # 'hello'

# floats (little-endian) + signed
def x 1.5  make Bytes [ {name:'x' type:'f64' endian:'le'} ]   # 8-byte LE float
def k -2   make Bytes [ {name:'k' type:'i16'} ]               # Bytes<ff fe>

# sub-byte flags: a version nibble + flags (sums to one byte)
make Bytes [ {value:1 type:'bits' size:4}  {value:1 type:'bits' size:1}  {value:0 type:'bits' size:1}  {value:0 type:'bits' size:2} ]   # Bytes<90>

# streaming framer loop: pull complete [u32 len][len body] frames from a buffer
def drain fn [[buf:Bytes] [Never] [
  def step ( unpack-prefix buf [ {name:'len' type:'u32'}  {name:'body' type:'bytes' size:'len'} ] )
  case [
    [ step.need ?? false ] [ buf ]              # incomplete → return buffer to refill
    [ ]                    [ handle step.ok.body   drain step.rest ] ]  # got one → continue
]]

# core ↔ bin-util handoff: hash a Bytes, render hex
import "aql:bin-util"
convert Bytes "hello"  BinUtil.sha256  BinUtil.hex-encode
  # "2cf24dba5fb0a30e26e83b2ac5b9e29e1b161e5c1fa7425e73043362938b9824"

# zero-copy slice + retention: keep one small field out of a huge frame
def tag ( convert Bytes (slice 4 8 big-frame) )   # convert-to-Bytes drops the big backing array

# negative cases
unpack (+hb/01/) [ {name:'ver' type:'u8'}  {name:'len' type:'u16'} ]   # ERROR:no_match   (only 1 byte, needs 3)
make Bytes [ {value:1 type:'bits' size:3} ]   # ERROR:unaligned  (3 bits, not byte-aligned)
convert Bytes [256]                         # ERROR:expected-byte
convert String (+hb/ff/)                     # ERROR:bad-encoding (0xff is not valid UTF-8)
+hb/abc/                                      # ERROR:mini_parse_error (odd hex digit count)
```

## 9. Errors

| code | raised when |
| --- | --- |
| `mini_parse_error` | a `+hb/…/` source has an odd hex-digit count or a non-hex digit, or a `+bb/…/` source has a non-binary digit or a non-multiple-of-8 bit count (raised when the kind runs). |
| `expected-byte` | a `List` element passed to `convert Bytes` is not an Integer in 0–255. |
| `bad-encoding` | `convert String` (or a `utf8` segment) is given bytes that are not valid UTF-8. |
| `no_match` | an `unpack` guard fails, or the buffer is too short for the spec. |
| `unaligned` | a `bits`/`pad` run in a spec does not sum to a whole byte. |

Guard with `RequireConcreteList` / a `Bytes` unwrap helper and range-check
before converting; never panic (`eng/go/CLAUDE.md`). `slice` clamps an
out-of-range window rather than erroring. A short buffer during streaming is
**not** an error — it is `unpack-prefix`'s `{need: n}` result.

## 10. Policy / capabilities

None — the type, its literal, the core interop words, and the whole bit-syntax
are **pure** and run under any policy. (The *entropy* words that produce `Bytes`
— `random-bytes`, `uuid` — are gated; that gating lives with those words in
BIN-UTIL.10.md, not on the type.)

## 11. Overlap & relationships

- **Net-new type** — there is no binary type today. It *complements* `String`:
  `convert Bytes` / `convert String` are the explicit, lossless bridge, where
  previously binary silently became a (possibly lossy) String at the I/O
  boundary.
- **`aql:bin-util`** (BIN-UTIL.10.md) — owns crypto/HMAC/CRC/encodings/random/
  UUID; all consume/produce `Bytes`. §4.1 of that doc ("Bytes interop") now
  points here: those interop words are **core**, not module words.
- **`NETWORK-SERVERS.0.md` / `NETWORK-CLIENTS.0.md`** — `recv-frame`,
  `length-prefixed` codecs, and `pack`/`unpack` on the wire are this section's
  bit-syntax; the zero-copy memory model (§4) is what their §7 streaming/
  backpressure claims rest on.
- **`STREAM-WORDS.0.md`** — `stream.from-bytes`/`to-bytes` now carry the real
  `Bytes` type (no surface change).

## 12. Open questions / out of scope

- **Binary file I/O** — an `IO.read-bytes` (and `IO.write` accepting `Bytes`)
  yielding/consuming `Bytes` instead of String is the natural follow-on, but
  lives in `aql:io`; out of scope here. Cross-reference once Bytes lands.
- **Sub-byte endianness** — `bits(n)` for `n > 8` spanning bytes uses the
  segment's `/be`/`/le`; the exact bit-order for non-byte-multiple spans is
  pinned to big-endian-bit-order in the spec rows (Erlang's default). Confirm
  against real protocols during implementation.
- **`unpack-prefix` `need` precision** — when the missing length is itself in an
  unread field, `need` returns the bytes required to read *that* field (a lower
  bound), then the caller re-tries. Document the two-step convergence.
- **A pure functional `with-byte` mutator** — deliberately omitted; `Bytes` is
  immutable and edits go through `add`/`slice`/`make Bytes`. Revisit only if a
  builder pattern proves necessary.

## 13. Phased roadmap

Aligned with `NETWORK-SERVERS.0.md` §11 Phase A (the binary prerequisite):

- **Phase A1 — type + memory + interop overloads.** Register `Bytes` (§2–3),
  the zero-copy/copy-on-ingest model (§4), `gobridge` `[]byte`↔`Bytes`, and the
  `convert`/`slice`/`add` Bytes overloads (§5). No literal, no bit-syntax.
  Independently useful (hashing, file I/O). **(landed)**
- **Phase A2 — the `+hb/…/` / `+bb/…/` minilang kinds** (§6). Two generator
  kinds registered in `aql:minilang`, reusing the existing `+name/src/` literal
  sugar — no core lexer change. **(landed)**
- **Phase A3 — bit-syntax** (§7): `make Bytes`/`unpack` overloads + the new
  `unpack-prefix` word, over a custom segment-spec parser. This unblocks the
  declarative `length-prefixed` codec and `recv-frame`. **(landed)**

## 14. Implementation sketch

- **`eng/go/gobridge.go`** — add the `[]byte` ↔ `Bytes` cases, with the
  copy-on-ingest copy in `FromNative` (§4.2).
- **`lang/go/native/native_bytes.go`** — `registerBytesType()` via
  `RegisterExternalBuiltin("Scalar/Bytes", 1009, bytesBehavior{})`; behavior
  implements `Format`/`Comparer`/`Equal`/`Sizer` (§3) and **omits**
  `DeepCloner` (§4.1); the `convert`/`slice`/`add` overloads (§5) appended via
  `RegisterNativeFunc`; funnel init errors through `TypeInitError`.
- **`lang/go/native/native_bytes.go` (layout spec)** — the `make Bytes` /
  `unpack` overloads, the `bytes [spec]` pack sugar (shares `packBytesSpec`
  with `make Bytes`), and the new `unpack-prefix`; `readBitSegments`/
  `readOneSeg` **read** (never parse) the `List<Map>` spec — `name`/`value`,
  `type`, `endian`, `signed`, `size` keys (ADR-007: no secondary parsing); an
  MSB-first bit writer/reader; bind via `InstallDef`.
- **`lang/go/modules/minilang.go`** — the `hb`/`bb` generator kinds
  (`[src:String opts:Map] → [Bytes]`) + their handlers (drop grouping, decode
  hex / MSB-first bits, raise `mini_parse_error`), building the `Bytes` value via
  `eng.FromNative` (the bridge); `lang/go/modules/docs_minilang.go` — the
  `lang_hb`/`lang_bb` doc entries.
- **`lang/go/native/help/help_bytes.go`** — `register(&Entry{…})` for the new
  `bytes` and `unpack-prefix` words (the overloaded words keep their existing
  entries); `help_categories.go` lists `bytes` in the `type` category and
  `unpack-prefix` in `control` (beside `unpack`).
- **`lang/go/test/fixedid_stability_test.go`** — pin `Scalar/Bytes` = 1009.
- **`lang/go/test/bytes.tsv`** — positive rows (convert round-trips, slice,
  `add`, compact, `make Bytes`/`bytes`/`unpack`/`unpack-prefix`, float/signed)
  each paired with an `ERROR:<substring>` negative sibling (`no_match`,
  `unaligned`, `expected-byte`, `bad-encoding`); the `+hb/…/` / `+bb/…/` literal rows
  (positive + `mini_parse_error` negatives) live in
  `lang/spec/module-minilang.tsv` §6; plus a Go-bridge round-trip test.
