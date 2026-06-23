# `aql:hmac` — Go `crypto/hmac`

> **Status: design proposal, not implemented.** A curated, hand-written
> native module wrapping Go's `crypto/hmac`. Read
> [README.10.md](README.10.md) first for the shared conventions this note
> assumes. The hash family it selects over —
> [SHA256](CRYPTO-SHA256.10.md), [SHA512](CRYPTO-SHA512.10.md),
> [SHA1](CRYPTO-SHA1.10.md), [MD5](CRYPTO-MD5.10.md) — share the digest
> convention this module reuses.

## 1. Package & status

Go [`crypto/hmac`](https://pkg.go.dev/crypto/hmac) implements keyed-hash
message authentication (HMAC, RFC 2104): `hmac.New(h func() hash.Hash,
key []byte) hash.Hash` builds a MAC writer over a chosen hash, and
`hmac.Equal(mac1, mac2 []byte) bool` compares two MACs in **constant
time**. This note wraps both. Nothing is implemented yet.

## 2. Why curated

The raw `go:` bridge would surface `hmac.New(func() hash.Hash, []byte)`
— a function-valued first argument and a `hash.Hash` interface result,
neither expressible in AQL — plus Bytes keys/MACs AQL has no type for.
The curated surface collapses this to two scalar-in/scalar-out words:
`sign` produces a hex MAC, `verify` checks one. Following the **shared
digest convention** (see README), the key, message, and MAC are all
`String`s and the MAC output is a hex `String`; the hash is chosen by a
small `algo` Atom (`sha256` / `sha512` / `sha1` / `md5`). This is the
common authentication primitive (signed webhooks, API request signing,
cookie integrity) made first-class.

## 3. Import & namespace

```
import "aql:hmac"        # binds the Hmac namespace
```

`Hmac` does not clash with any builtin type or existing module
namespace, so no `-util` suffix (naming rule in `lang/go/CLAUDE.md`
"Package layout"). Words are dot-accessed: `Hmac.sign`, `Hmac.verify`.

## 4. API

Signatures are **top-first, sig order** (position 0 is the top of the
stack). All inner native sigs use `BarrierPos: -1` so the swap form
dispatches. The `algo` Atom selects the underlying hash constructor
(`hmac.New(sha256.New, key)` and friends); it is captured with `/q` so
the bare word `sha256` reaches the handler as an Atom rather than
dispatching (`lang/go/CLAUDE.md` "Undefined Words").

| Go symbol | aql word | signature (top-first) | one-line doc | aql-ish refinement |
|---|---|---|---|---|
| `hmac.New(h,key)` + write + `Sum` | `sign` | `[Atom algo, String message, String key] -> String` | Compute the HMAC of a message under a key, as hex. | Bytes key/message → UTF-8 Strings; `hash.Hash` writer → one-shot; raw MAC → hex String. `algo` (`/q` Atom) picks the hash. `unknown-algo` error on an unsupported atom. |
| `hmac.Equal(mac1,mac2)` | `verify` | `[Atom algo, String mac, String message, String key] -> Boolean` | Verify a hex MAC for a message+key in constant time. | Recomputes the expected MAC via `sign`, decodes both hex MACs to bytes, returns `hmac.Equal(expected, given)` — **constant-time**. `unknown-algo` / `bad-mac-hex` errors. |

Argument order note: with sig position 0 = top of stack, the forward
canonical form reads `Hmac.sign algo message key` — algo first (it is
the smallest, most constant arg), then the message, then the secret key
last. `verify` prepends the supplied `mac` to compare against.

**Why constant-time matters.** `verify` MUST use `hmac.Equal`, never a
plain string/`==` comparison. A normal comparison returns as soon as the
first differing byte is found, so its running time leaks *how many
leading bytes matched* — a timing side channel an attacker can exploit
to forge a valid MAC byte-by-byte. `hmac.Equal` always examines the full
length, removing the signal. This is the single most important contract
of the module and is pinned by a spec row (a correct vs a tampered MAC
both go through `verify`).

## 5. Types

Scalars only — String key/message/MAC, Atom algo, Boolean/String out. No
opaque handle type, no `RegisterExternalBuiltin` / FixedID (the
`hash.Hash` writer is internal to the handler, never surfaced). Boundary
conversion via `eng.FromNative` / `eng.ToNative` (`eng/go/gobridge.go`):
Strings via `AsConcreteString`, algo via `AsConcreteAtom`.

## 6. Errors

Signalled with `r.AqlError(code, detail, word)`, kebab-case codes:

| code | raised when |
|---|---|
| `unknown-algo` | `algo` is not one of `sha256` / `sha512` / `sha1` / `md5`. |
| `bad-mac-hex` | the supplied `mac` String is not valid hex (`verify` only; from `hex.DecodeString`). |

Guard the algo with `AsConcreteAtom` and the Strings with
`AsConcreteString` before use; never panic (`eng/go/CLAUDE.md` "Panic
Prevention"). A length-mismatched MAC is **not** a separate error —
`hmac.Equal` simply returns false (constant time over the recomputed
length), so `verify` returns `false`, not an error.

## 7. Policy / capabilities

None — pure computation, no side effects, runs under any policy. (The
secret key is supplied by the caller as a value; the module reads no
environment or files.)

## 8. Overlap

Reuses the same hash cores as the four hash modules but does not
duplicate them: `sign` is keyed authentication, whereas
`Sha256.hex` etc. are unkeyed digests — a different primitive (an
unkeyed hash of `key ++ message` is **not** an HMAC and is forgeable).
The hex encoding of the MAC overlaps conceptually with
[`aql:hex`](ENCODING-HEX.10.md) but is baked in for convenience, as in
the hash notes.

## 9. Examples (args-before form)

```
import "aql:hmac"

# sign: algo, message, key  (key last, as the secret)
Hmac.sign sha256/q "the message" "secret-key"
# e.g. "1b2c…"  (64-char hex HMAC-SHA256)

# verify: algo, mac, message, key  → constant-time Boolean
def mac (Hmac.sign sha256/q "the message" "secret-key")
Hmac.verify sha256/q mac "the message" "secret-key"      # true
Hmac.verify sha256/q mac "tampered"    "secret-key"      # false
Hmac.verify sha256/q mac "the message" "wrong-key"       # false

Hmac.sign md5/q "m" "k"                # HMAC-MD5 (legacy; see CRYPTO-MD5)
Hmac.sign bogus/q "m" "k"              # ERROR:unknown-algo
Hmac.verify sha256/q "zz" "m" "k"      # ERROR:bad-mac-hex
```

## 10. Open questions / out of scope

- **base64 MAC variant** — `sign`/`verify` are hex-only for now, matching
  the hash notes' primary form. A `sign-base64` / `verify-base64` pair
  is deferred until needed (Open question).
- **More algos** — only `sha256`/`sha512`/`sha1`/`md5` are wired (the
  four hash modules). SHA-384, SHA-512/256, etc. could be added as algo
  atoms once their hash modules expose constructors; deferred.
- **Bytes key/message** — once AQL has a Bytes type, the words should
  accept raw bytes for binary keys (a hex/base64-encoded key String is
  the interim workaround). Tracked with the hash notes.
- **Legacy algos.** `sha1`/`md5` are accepted for legacy interop
  (HMAC-SHA1 is still widely deployed and is **not** broken the way bare
  SHA-1 is, since HMAC tolerates a weak hash better), but new designs
  should pick `sha256`/`sha512`.

## 11. Implementation sketch

Wiring checklist — no Go code. Reference: `lang/go/modules/math.go`
(`BuildMathModule`, the pure-module canonical builder).

- `lang/go/modules/hmac.go` — `BuildHmacModule(parent *native.Registry)
  (native.ModuleDesc, error)`: isolated `native.DefaultRegistry()`
  sub-registry, register an `HmacNatives []native.NativeFunc` slice (each
  inner sig `BarrierPos: -1`; the algo Atom position carries `/q` via
  `Signature.QuoteArgs`), wrap each word as an `FnDef` export, return
  `ModuleDesc{ID: parent.Modules.NextID(), Exports: {"Hmac": …}}`. The
  algo→constructor map (`sha256.New`, `sha512.New`, `sha1.New`,
  `md5.New`) lives in the handler; `verify` calls the same `sign` path
  then `hmac.Equal`.
- Register `"hmac": BuildHmacModule` in the `modules` map in
  `lang/go/modules/modules.go`.
- `lang/go/modules/docs_hmac.go` — `registerDocs("aql:hmac",
  map[string]string{…})`, one line per export (`TestModuleExportDocs`).
- `lang/spec/module-hmac.tsv` — rows leading with `import "aql:hmac"`;
  every positive row paired with an `ERROR:<substring>` negative sibling
  — `unknown-algo`, `bad-mac-hex`, a tampered-message `verify → false`,
  and the unqualified-word `undefined_word` case without import.
- No FixedID entry, no policy wiring.
