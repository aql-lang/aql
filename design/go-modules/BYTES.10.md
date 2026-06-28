# The `Bytes` type — a first-class binary leaf + bit-syntax

> **Status: design proposal, not implemented.** This note specifies a new
> AQL value type, `Bytes`, the foundation for all binary-adjacent
> functionality, together with its **storage & memory model** (§4) and the
> **`pack`/`unpack` bit-syntax** (§7) that binary wire protocols are written
> against. The heavier binary words (cryptographic hashes, HMAC,
> CRC, base/hex encodings, secure random, UUIDs) live in the expanded
> `aql:bin-util` module — see [BIN-UTIL.10.md](BIN-UTIL.10.md). The type, its
> literal, its core interop words, and the bit-syntax are **core** (no import).
> Read [README.10.md](README.10.md) for the shared conventions.
>
> **Decisions taken at design-review time:** (1) this doc is the **authoritative
> full `Bytes` design** (the earlier type-only draft is folded in); (2) the
> word surface is **split** — the type, the `0x"…"` literal, the bit-syntax,
> and minimal interop are **core**, while crypto/encoding/hash/uuid stay in
> `aql:bin-util`; (3) a **hex literal only** (`0x"deadbeef"`), no `b"…"` form;
> (4) the memory model is **zero-copy share + copy-on-ingest** (this supersedes
> the earlier `DeepCloner` decision — see §4).
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
a `Bytes` in place** — every operation (`slice`, `concat`, `pack`, …) returns a
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
- **`compact <Bytes> -> Bytes`** forces a minimal fresh copy (`len`-sized
  backing array) for the rare case where a small slice must outlive a large
  buffer (e.g. caching one field extracted from each of many big frames).
  Idiomatically you only reach for `compact` when retention actually bites.

### 4.4 Cost summary

| Operation | Copies | Notes |
| --- | --- | --- |
| construct from external `[]byte` / `recv` | 1 | the ingest copy (§4.2) |
| `0x"…"` literal | 1 (at parse) | baked into the program once |
| clone / `ForkConcurrent` / `send` | 0 | slice header only (§4.1) |
| `slice`, `unpack` `bytes(n)` view | 0 | sub-slice view (§4.3) |
| `compact` | 1 | force-copy to drop retention |
| `concat` of k parts | 1 | one build of the joined slice |
| `pack` | 1 | one build of the frame |
| `to-text` / `utf8` | 1 | crosses the bytes⇄text boundary |

## 5. Core word surface (forward form)

The type ships a **minimal core set** in forward call form; everything else
(crypto, encodings, hashing, random, UUID) lives in `aql:bin-util`
(BIN-UTIL.10.md), where each "data" input accepts `String | Bytes`.

| word | signature (forward) | meaning |
| --- | --- | --- |
| `utf8` | `utf8 <String> -> Bytes` | encode text as UTF-8 octets. |
| `to-text` | `to-text <Bytes> -> String` | decode UTF-8; raises `bad-encoding`. |
| `bytes` | `bytes <List<Integer 0–255>> -> Bytes` | pack a list of byte values. |
| `byte-ints` | `byte-ints <Bytes> -> List<Integer>` | unpack to a list of 0–255 ints. |
| `slice` | `slice <Bytes> <start> <len> -> Bytes` | zero-copy sub-slice view (§4.3). |
| `concat` | `concat <List<Bytes>> -> Bytes` | join byte sequences (one build). |
| `compact` | `compact <Bytes> -> Bytes` | force a minimal copy (drop retention). |
| `pack` | `pack <List<segment>> -> Bytes` | build a frame from a bit-spec (§7). |
| `unpack` | `unpack <Bytes> <List<segment>> -> Map` | match/destructure a frame (§7). |
| `unpack-prefix` | `unpack-prefix <Bytes> <List<segment>> -> Map` | streaming framer (§7). |

`length` / `size`, `eq`, value ordering, and `is Bytes` need **no dedicated
word** — they resolve through the `Sizer`/`Equal`/`Comparer` behaviors (§3).

**Naming note (reconciliation).** Earlier sketches used module-qualified names
(`bin.len`, `bin.slice`, `bin.of-list`, `bin.hex`) and an alternate
`BinUtil.bytes`/`text`. The canonical core names are the forward-form words
above: `length`/`size` (behavior), `slice`, `bytes`, and `BinUtil.hex-encode`
for hex *output* (the `0x"…"` literal covers hex *input*). The NETWORK docs are
updated to match.

## 6. The `0x"…"` Bytes literal

A first-class hex literal for protocol magic numbers and constants:

```aql
0x"deadbeef"        # Bytes<de ad be ef>
0x"00"              # Bytes<00>   (a single zero byte)
0x""                # Bytes<>     (empty)
```

- The payload is an **even** number of hex digits (`[0-9a-fA-F]`); an odd count
  or a non-hex digit is a **parse error** (`bad-bytes-literal`), caught at
  compile time, not runtime.
- **Disambiguation from the integer `0x` prefix is the quote.** `0xff` is the
  existing `Integer` 255 (`isBasePrefixedInteger`, `eng/go/parser/parse.go`);
  `0x"ff"` is `Bytes<ff>`. The matcher fires only on `0x` *immediately followed
  by a quote*, so no existing literal changes meaning.
- **Lexer hook.** A custom jsonic lex matcher registered at high priority
  (before the string matcher) in `eng/go/parser/` (`grammar.go`/`parse.go`),
  converting the matched `0x"…"` token to a `Bytes` value in
  `convertDataValue`. There is **no `b"…"` ASCII literal** — use `utf8 "GET "`
  for text-as-bytes, which keeps the encoding choice explicit.

## 7. Bit-syntax — the segment-spec DSL (`pack` / `unpack`)

Erlang's bit syntax (`<<Len:16, Body:Len/binary>>`) is what makes binary code
short *and* safe. AQL gets an equivalent that **reuses existing machinery**: the
`name:Type` binding slot parsed by `eng.ParseFnParams` (`eng/go/fn_params.go` —
the same parser behind lambda params and `receive` clauses), extended with a
**size** and **modifier** suffix.

> **Not an `aql:minilang`.** This is a `List` of ordinary AQL **segment tokens**
> parsed by `eng.ParseFnParams`, **not** a kind of the `mini`/MiniLang facility
> (`MINILANG.5.md`), whose kinds take an *opaque String `src`* (`mini re '…'`,
> `+re/…/`). The list form is deliberate for two reasons a String-sourced `mini`
> kind could not serve: (1) a `pack` segment value can be an **inline AQL
> expression** (`(length body):u16`), and (2) the `name:Type` slot reuses the
> **same `ParseFnParams` binding** as `receive` clauses — one "destructure by
> `name:Type`" concept across the language, rather than a second bespoke spec
> parser inside a kind handler. "Bit-syntax" / "segment spec" — never
> "minilang".

### 7.1 Segment grammar

A frame spec is a `List` of **segments**; each segment is:

```
<value-or-name> : <seg-type> ( ( <size> ) )? ( / <modifier> )*
```

- **value-or-name** — in `pack`, a bound `name` reads its value from scope, or a
  **literal** (`1`, `0x"7f"`) packs a constant. In `unpack`, a `name` **binds**
  the field into scope; a **literal** is a **match guard** (the bytes must equal
  it, else `raise no_match`). This is exactly Erlang's dual use of one syntax.

| element | values |
| --- | --- |
| **seg-type** | `u8 u16 u32 u64`, `i8 i16 i32 i64` (signed), `f32 f64` (IEEE-754, ref `IEEE-754-COMPLIANCE.8.md`), `bits(n)` (sub-byte unsigned int of *n* bits), `bytes` (raw `Bytes`), `utf8` (a length-known text run → `String`), `pad(n)` (*n* zero bits, no binding) |
| **size** | an integer literal, or a **previously-bound name** — `body:bytes(len)` (Erlang `Body:Len/binary`), the killer feature. A trailing `bytes`/`utf8` with **no** size means "the rest". `size` applies to `bytes`/`utf8` (octet count) and is implicit for fixed-width numerics. |
| **modifier** | `/be` (default, network byte order) or `/le`; `/unsigned` (default) or `/signed`. Modifiers apply to multi-byte numerics. |

### 7.2 `pack` — build a frame

```
pack <List<segment>> -> Bytes
```

Evaluates each segment left-to-right into a single freshly-built `[]byte`
(one allocation, §4.4). `name:` segments read `name` from scope; literal
segments pack the constant. Numerics honour width/endianness/sign.

### 7.3 `unpack` — match & destructure

```
unpack <Bytes> <List<segment>> -> Map
```

Walks the segments over the input bytes, **binding each `name` into scope** and
also returning all bound fields as a `Map` (for chaining). A literal segment is
a **guard**. `bytes(n)`/`utf8(n)` segments are **zero-copy sub-slice views**
(§4.3). Mismatches (`guard` fails, declared size overruns the input, trailing
bytes when the spec is exact) `raise no_match`.

### 7.4 `unpack-prefix` — the streaming framer

```
unpack-prefix <Bytes> <List<segment>> -> {ok: Map  rest: Bytes} | {need: Integer}
```

Matches **only what the spec requires** and returns the bound fields plus the
**leftover bytes** (`rest`), or — when the buffer is too short to complete the
spec — `{need: n}` (how many more bytes are required, when computable, else a
positive lower bound). This is the single primitive a length-prefixed framing
loop needs: read header → learn length → read body, declaratively, with no
hand-rolled buffer state machine (`NETWORK-SERVERS.0.md` §5.3, §6.1).

### 7.5 Alignment rule

`pack`/`unpack` are **byte-aligned by default**: a contiguous run of `bits(n)`
(and `pad(n)`) segments must sum to a whole number of bytes, or the spec
`raise`s **`unaligned`** at build time. There is no implicit trailing padding —
add an explicit `pad(n)` to round out a sub-byte run. (This resolves
`NETWORK-SERVERS.0.md` §12 Q2 in favour of safety: ragged frames are an error,
not a silent surprise.)

### 7.6 Parser reuse & binding

Splitting a segment into its `name:type` core (→ `ParseFnParams`) and its
`(size)`/`/mod` suffix (→ a thin additional pass) is a build-time step over the
segment list; the `name:type` parsing, type resolution, and scope binding reuse
`eng.ParseFnParams` and the per-call `DefTable` unchanged — the **same**
mechanism `receive` clauses use (`PROCESSES.0.md` §3). One "destructure by
`name:Type`" concept across the language, not two.

## 8. Usage examples

All forward form. `import "aql:bin-util"` only where a crypto/encoding word
appears; the type, literal, and bit-syntax need no import.

```aql
# hex literal + magic-number guard (the leading byte MUST be 0x7f)
def frame ( recv-frame conn [ 0x"7f":u8  ver:u8  len:u16  body:bytes(len) ] )

# text round-trip
utf8 "café"  to-text                     # "café"   (é is two UTF-8 bytes)
utf8 "café"  length                       # 5

# build + match a [ver=1][u16 len][len-byte body] frame
def body  ( utf8 "hello" )                # Bytes<68 65 6c 6c 6f>
def msg   ( pack [ 1:u8  (length body):u16  body:bytes ] )
# msg == Bytes<01 00 05 68 65 6c 6c 6f>
def parts ( unpack msg [ ver:u8  len:u16  body:bytes(len) ] )
# parts == {ver: 1  len: 5  body: Bytes<68 65 6c 6c 6f>}
to-text parts.body                        # "hello"

# floats + little-endian
pack [ 3.14:f64/le  count:u32/le ]        # 8-byte float then 4-byte LE int

# sub-byte flags: a version nibble + 4 one-bit flags (sums to one byte)
pack [ 1:bits(4)  urgent:bits(1)  ack:bits(1)  0:bits(2) ]   # one byte

# streaming framer loop: pull complete [u32 len][len body] frames from a buffer
def drain fn [[buf:Bytes] [Never] [
  def step ( unpack-prefix buf [ len:u32  body:bytes(len) ] )
  case [
    [ step.need ?? false ] [ buf ]              # incomplete → return buffer to refill
    [ ]                    [ handle step.ok.body   drain step.rest ] ]  # got one → continue
]]

# core ↔ bin-util handoff: hash a Bytes, render hex
import "aql:bin-util"
utf8 "hello"  BinUtil.sha256  BinUtil.hex-encode
  # "2cf24dba5fb0a30e26e83b2ac5b9e29e1b161e5c1fa7425e73043362938b9824"

# zero-copy slice + retention: keep one small field out of a huge frame
def tag ( compact (slice big-frame 4 8) )   # compact drops the 1 MB backing array

# negative cases
unpack 0x"01" [ ver:u8  len:u16 ]          # ERROR:no_match   (only 1 byte, needs 3)
pack [ flag:bits(3) ]                       # ERROR:unaligned  (3 bits, not byte-aligned)
bytes [256]                                  # ERROR:expected-byte
to-text 0x"ff"                              # ERROR:bad-encoding (0xff is not valid UTF-8)
```

## 9. Errors

| code | raised when |
| --- | --- |
| `bad-bytes-literal` | `0x"…"` literal has an odd digit count or a non-hex digit (compile time). |
| `expected-byte` | a `List` element passed to `bytes` is not an Integer in 0–255. |
| `bad-encoding` | `to-text` (or a `utf8` segment) is given bytes that are not valid UTF-8. |
| `index-range` | `slice` start/len is out of range. |
| `no_match` | an `unpack` guard fails, or a declared size overruns / underruns the input. |
| `unaligned` | a `bits(n)`/`pad(n)` run in a spec does not sum to a whole byte. |

Guard with `RequireConcreteList` / a `Bytes` unwrap helper and range-check
before converting; never panic (`eng/go/CLAUDE.md`). Buffer-too-short during
streaming is **not** an error — it is `unpack-prefix`'s `{need: n}` result.

## 10. Policy / capabilities

None — the type, its literal, the core interop words, and the whole bit-syntax
are **pure** and run under any policy. (The *entropy* words that produce `Bytes`
— `random-bytes`, `uuid` — are gated; that gating lives with those words in
BIN-UTIL.10.md, not on the type.)

## 11. Overlap & relationships

- **Net-new type** — there is no binary type today. It *complements* `String`:
  `utf8`/`to-text` are the explicit, lossless bridge between the two, where
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
  immutable and edits go through `concat`/`slice`/`pack`. Revisit only if a
  builder pattern proves necessary.

## 13. Phased roadmap

Aligned with `NETWORK-SERVERS.0.md` §11 Phase A (the binary prerequisite):

- **Phase A1 — type + memory + core interop.** Register `Bytes` (§2–3), the
  zero-copy/copy-on-ingest model (§4), `gobridge` `[]byte`↔`Bytes`, and the
  core words `utf8`/`to-text`/`bytes`/`byte-ints`/`slice`/`concat`/`compact`
  (§5). No literal, no bit-syntax. Independently useful (hashing, file I/O).
- **Phase A2 — the `0x"…"` literal** (§6). The lexer matcher only.
- **Phase A3 — bit-syntax** (§7): `pack`/`unpack`/`unpack-prefix` + the
  segment-spec parser reusing `ParseFnParams`. This is what unblocks the
  declarative `length-prefixed` codec and `recv-frame`.

## 14. Implementation sketch

- **`eng/go/gobridge.go`** — add the `[]byte` ↔ `Bytes` cases, with the
  copy-on-ingest copy in `FromNative` (§4.2).
- **`lang/go/native/native_bytes.go`** — `registerBytesType()` via
  `RegisterExternalBuiltin("Scalar/Bytes", <FixedID 1000–1999>, bytesBehavior{})`;
  behavior implements `Format`/`Comparer`/`Equal`/`Sizer` (§3) and **omits**
  `DeepCloner` (§4.1); the core words (§5); funnel init errors through
  `TypeInitError`.
- **`lang/go/native/native_bytes.go` (bit-syntax)** — `pack`/`unpack`/
  `unpack-prefix` and the segment-spec parser: split each segment into the
  `name:type` core (reuse `eng.ParseFnParams`) and a `(size)`/`/mod` suffix
  pass; bind via the per-call `DefTable`.
- **`eng/go/parser/` (`grammar.go`/`parse.go`)** — the `0x"…"` custom lex
  matcher (§6), converting to a `Bytes` value in `convertDataValue`.
- **`lang/go/native/help/help_bytes.go`** + `describe.go` — `register(&Entry{…})`
  for each core word and the bit-syntax words, with the §8 examples; live sigs
  via `BuildFuncInfo`.
- **`lang/go/test/fixedid_stability_test.go`** — pin the new FixedID.
- **`lang/spec/`** — positive rows (round-trip `pack`→`unpack`, ordering /
  type-literal-first, `Equal`, slice/concat, literal parse) each paired with an
  `ERROR:<substring>` negative sibling (`no_match`, `unaligned`,
  `expected-byte`, `bad-encoding`, `bad-bytes-literal`), per Test discipline
  (`lang/go/CLAUDE.md`); include known-answer hex vectors.
