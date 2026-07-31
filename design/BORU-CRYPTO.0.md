# `crypto` → `boru:crypto` (`Crypto`)

> **Status: design proposal — not implemented.** This note specifies the
> cryptographic-primitive module and its secure-randomness word. It is the
> authoritative spec for the deferred **"vault logic in boru"** phase of
> [VAULT-TUI-PORT.0](VAULT-TUI-PORT.0.md) §7.1–§7.2. No Go code exists yet;
> the note exists so the surface — and especially its **parameter fidelity
> to the existing Go vault** and its **policy gating** — is auditable
> before a handler is written. The governing constraint is **parity, not
> reinvention**: every constant below must match `cmd/go/internal/vault`
> byte-for-byte, or an in-boru vault cannot open a vault the Go path wrote.

## 1. Package & status

`boru:crypto` is the curated home for the cryptographic primitives the
vault needs to move its key-management and envelope logic out of Go and
into boru: authenticated encryption, a password KDF, a sub-key KDF,
hashes, a MAC, constant-time comparison, public-key sealing, and a
cryptographically secure random source. It is the first module in the
family whose **inputs and outputs are secret key material**, so it is
curated with more care than any `-util` library and is deliberately
*not* part of core.

It is a **follow-on**, not a rewrite: the port in
[VAULT-TUI-PORT.0](VAULT-TUI-PORT.0.md) kept the vault crypto in Go
behind the `boru:vault` bridge. This module is step one of shrinking that
bridge — once `Crypto.*` exists, `store.jsonic` read/write, keyslot
envelopes, and capability bookkeeping become expressible in boru (`boru:io`
already covers atomic writes, locks, and permissions), and the bridge
retires op by op.

## 2. Why curated — and the `boru:rand` firewall

A raw `go:` reflection bridge over `crypto/*` would expose `[]byte`,
`(n int, err error)`, and `hash.Hash` streaming state at `Any`
boundaries with Go names, and — most dangerously — would put a
cryptographically secure random source one reflection call away from the
same namespace as everything else. The curated module instead:

- exposes a **small, closed** word set (no streaming `hash.Hash`, no
  cipher objects — one-shot calls only);
- fixes each primitive to the algorithm and parameters the vault already
  uses (§5), so there is exactly one way to call it and it interoperates
  with on-disk vaults;
- returns a **single opaque error** on any open/verify failure so the
  surface is not a padding/tag oracle (§7);
- and keeps secure randomness **here**, never in `boru:rand`.

**The `boru:rand` firewall (CRITICAL).** `boru:rand` (`Rand`) is backed by
`math/rand` (`lang/go/modules/rand.go` imports `mathrand "math/rand"`) —
it is seedable and reproducible, which is exactly what makes it **unfit
for key material**: salts, nonces, keys, and tokens must come from a
CSPRNG. `Crypto.rand-bytes` (§9) is the only sanctioned source and sits
on `crypto/rand`. `boru:rand` must never be reached for anything a
`Crypto.*` word will consume. The two modules are kept separate precisely
so the type of randomness is visible at the import line.

## 3. Import & namespace

```boru
import "boru:crypto"        # binds the Crypto namespace
```

Namespace is the plain capitalized package name **`Crypto`** — no `-util`
suffix (that convention is only for pure helper libraries and for
avoiding builtin-type name clashes; `Crypto` clashes with nothing). Words
are reached args-before-dot: `key Crypto.hmac message`.

## 4. API — the word surface

Every byte-valued argument and result — keys, nonces, salts, IKM,
ciphertext, digests, MAC tags, sealed boxes — is the first-class
**`Bytes`** type (§6). Text crosses the boundary with `convert String
<bytes>` / `convert Bytes <str>`, and transport encodings live in
`boru:bin-util` (base64/hex; the `Bytes`↔hex/base64 handoff is documented
in [BYTES.10](go-modules/BYTES.10.md) §8). Every signature is one-shot
and pure.

### Authenticated encryption (AES-256-GCM)

```boru
Crypto.aead-seal <key> <nonce> <aad> <plain>  → Bytes   # ciphertext‖tag
Crypto.aead-open <key> <nonce> <aad> <sealed> → Bytes   # plaintext, or [boru/crypto] on failure
```

`key` is 32 bytes, `nonce` 12 bytes, `aad` any length (may be empty),
tag is 16 bytes appended by GCM. `aead-open` returns a **single opaque
error** for every failure mode (§7).

### Key derivation

```boru
Crypto.scrypt {n:Integer r:Integer p:Integer len:Integer} <salt> <pass> → Bytes
Crypto.hkdf   {info:String len:Integer}                    <salt> <ikm>  → Bytes
```

`scrypt` defaults to the vault parameters `{n: 32768, r: 8, p: 1, len:
32}` (§5) when the option map omits them, so a migration call site can
write `salt Crypto.scrypt pass` and match the Go path exactly. `hkdf` is
HKDF-SHA256; `info` is the domain-separation label (e.g. `"BORU-slot-kek"`),
`len` the output length. `pass` may be a `String` or `Bytes` (passphrases
are text; `ikm` is key material).

### Hashes & MAC

```boru
Crypto.sha256 <bytes>       → Bytes       # 32-byte digest
Crypto.sha512 <bytes>       → Bytes       # 64-byte digest
Crypto.hmac   <key> <bytes> → Bytes       # HMAC-SHA256, 32-byte tag
```

### Constant-time comparison

```boru
Crypto.eq <a> <b> → Boolean                # constant-time; false on length mismatch
```

`eq` is the only correct way to compare a secret (a MAC, a verifier, a
token hash). The generic `eq`/`Equal` on `Bytes` uses `bytes.Equal` and
the byte-lexicographic Comparer uses `bytes.Compare` — **neither is
constant-time** (`native_bytes.go`), so both short-circuit and leak
length and prefix timing. `Crypto.eq` wraps `crypto/subtle` /
`hmac.Equal` instead.

### Public-key sealing (X25519 anonymous sealed box)

```boru
Crypto.box-keypair              → {public:Bytes secret:Bytes}
Crypto.box-seal <recipient-public> <plain>                  → Bytes
Crypto.box-open <recipient-public> <recipient-secret> <sealed> → Bytes
```

An **anonymous sealed box** (NaCl `box.SealAnonymous`): the sender is
ephemeral and unauthenticated; only the holder of `recipient-secret` can
open. This is exactly how the vault wraps per-namespace data keys to a
password slot's public key (§5), so an in-boru keyslot can unwrap existing
`WrappedKeys`.

### Secure randomness

```boru
Crypto.rand-bytes <n:Integer> → Bytes     # n cryptographically-random bytes
```

See §9.

### Resource bounds (CRITICAL)

`scrypt` exposes `n`/`r`/`p` and every KDF/`rand-bytes` word takes an
output length. Left unbounded these are a **memory/CPU denial-of-service**:
scrypt allocates `128 · N · r · p` bytes and does the matching work
*before* it can return, so a valid-but-huge `{n, r, p}` exhausts the host
— and because §8 permits `crypto` in a **compute** sandbox, an untrusted
guest reaches it. Every bound below is validated as `crypto_usage`
(§7) **before any allocation**:

| Word / param | Hard cap (proposed) | Rationale |
| --- | --- | --- |
| `scrypt` `n` | power of two, `≤ 2²⁰` | `n` must be a power of two (scrypt requirement); the cap bounds the cost base |
| `scrypt` `r`, `p` | `r ≤ 32`, `p ≤ 16` | small constants; the vault uses `r=8, p=1` |
| `scrypt` memory | `128·N·r·p ≤ 1 GiB` | the real ceiling — reject the *combination* before `scrypt.Key`, not each factor alone |
| `scrypt` / `hkdf` `len` | `≤ 1024` (hkdf `≤ 255·32`, its HMAC-SHA256 max) | derived-key sizes are small; HKDF has a hard protocol max |
| `rand-bytes` `n` | `≥ 1`, `≤ 1 MiB` | key material is small; a huge draw is almost always a bug or an attack |
| `aead` / `hmac` inputs | streamed, but a per-call size ceiling ties to `Limits` | bound total work per op |

The caps are a **starting proposal** — the load-bearing requirement is
that the combination is bounded pre-allocation. The cleaner long-term
answer is to route these through an enforced runtime budget
(`policy.Limits` / the `MaxStepBudget`-style governors) so the ceiling is
policy-configurable rather than a hard constant; until that exists, the
module ships with the constants above and rejects anything larger.

## 5. Parameter fidelity to the Go vault (CRITICAL)

`boru:crypto` cannot import `cmd/go/internal/vault` (an internal package),
so it must **re-implement** these primitives over `crypto/*` +
`golang.org/x/crypto`. The re-implementation is only correct if it
reproduces the vault's constants exactly. This table is the contract; a
parity test (§12) must assert equality against the Go source.

| Primitive | Word | Algorithm & constants | Go source of truth |
| --- | --- | --- | --- |
| AEAD | `aead-seal`/`aead-open` | AES-256-GCM, 12-byte nonce, 16-byte tag, caller AAD | `keyslot.go` (`newGCM`), `keyring.go` (`aesGCMSeal`) |
| Password KDF | `scrypt` | scrypt **N=2¹⁵ (32768), r=8, p=1, keyLen=32**, 16-byte salt | `keyring.go` `scryptKey` (`scrypt.Key(pass, salt, 1<<15, 8, 1, 32)`) |
| Sub-key KDF | `hkdf` | HKDF-SHA256, 32-byte output, info label e.g. `"BORU-slot-kek"` | `keyslot.go` `slotKEK` (`hkdf.New(sha256.New, kek, salt, label)`) |
| Hash | `sha256`/`sha512` | SHA-256 / SHA-512 | `crypto/sha256`, `crypto/sha512` |
| MAC | `hmac` | HMAC-SHA256 | `keyslot.go` `deriveVerifier`/`derivePubMAC` |
| Constant-time eq | `eq` | `crypto/hmac` `hmac.Equal` semantics | `keyslot.go` `verifyMatches`, `store.go` (`subtle.ConstantTimeCompare`) |
| Public-key seal | `box-*` | X25519 NaCl **anonymous** sealed box (`box.SealAnonymous`/`OpenAnonymous`) | `keyslot.go` `sealNDK`/`openNDK` |
| CSPRNG | `rand-bytes` | `crypto/rand` | `keyslot.go` `newRandom`, `p7_seam7.go` (`rand.Reader`) |

**The envelope byte layouts stay in boru, not in these words** — and each
envelope's AAD is **distinct**, so the boru layer must build the exact
bytes below and pass them as the `aead-seal`/`aead-open` `aad` argument
(`boru:crypto` seals nothing implicitly). There is **no** single universal
AAD rule; getting any one wrong produces envelopes the Go vault cannot
open (and vice versa). The four the vault uses today
(`cmd/go/internal/vault`):

| Envelope | Wire layout | **AAD bytes** | Go source |
| --- | --- | --- | --- |
| Sealed value | `"BORUE" ‖ format(1) ‖ ndkID(8) ‖ nonce(12) ‖ ct‖tag` | `valueAAD` = `"BORUE" ‖ format ‖ ndkID ‖ namespace ‖ 0x00 ‖ alias` | `keyslot.go` `valueAAD`/`sealValue` |
| Slot private key | base64(`nonce(12) ‖ ct‖tag`) | `privAAD` = `macInput("BORUP", name, scope[, expiresAt])` | `keyslot.go` `privAAD`/`sealPrivKey` |
| File keyring blob (headered) | `"BORUK" ‖ format(1) ‖ salt(16) ‖ nonce(12) ‖ ct‖tag` | `keyringAAD` = `header(magic‖format) ‖ salt` | `keyring.go` `keyringAAD`/`encryptBlob` |
| Export bundle | `exportMagic ‖ format(1) ‖ salt(16) ‖ nonce(12) ‖ ct‖tag` | `header(magic‖format) ‖ salt` | `export.go` `sealExport` |

The `macInput` helper — `label ‖ 0x00 ‖ fields joined by 0x00`
(`keyslot.go`) — is the **MAC / verifier** domain-separation convention
(used by `privAAD`, `deriveVerifier`, `derivePubMAC`); it is *not* the
value/keyring/export AEAD AAD, which are the concatenations above. A
legacy headerless file-keyring blob authenticates `salt` alone. The boru
layer reproduces these byte-for-byte; the parity test (§12) must pin at
least the value and keyring AADs against a Go-sealed fixture.

## 6. Types — `Bytes` is the carrier

boru already has a **first-class `Bytes` type** — `Scalar/Bytes`, FixedID
1009, its own design doc [BYTES.10](go-modules/BYTES.10.md) — and the
repo deliberately chose a dedicated binary leaf over `String` or
`List[Integer]` (BYTES.10 §1). `boru:crypto` uses it directly: every key,
nonce, salt, digest, tag, ciphertext, and sealed box is `Bytes`. The
RFC's `→ Bytes` notation was correct; there is no gap to work around.

What `Bytes` gives the crypto surface for free (`native_bytes.go`):

- **Immutable, zero-copy.** No word mutates a `Bytes` in place, and it
  shares its backing array on clone/fork/send — so key material can't be
  clobbered underneath a handler, and passing it around is cheap.
- **Distinct from text.** Because `Bytes` is not `String`, key material
  does not silently flow into text words (`upper`, `split`, template
  interpolation); crossing to text is an explicit `convert String
  <bytes>` (which requires valid UTF-8 and so fails loudly on raw key
  bytes).
- **Hex render, not raw dump.** `Format` shows `Bytes<68 65 6c 6c 6f>`
  (length-capped hex), so a stray `print` of a digest yields hex, not the
  raw bytes — though it is **not** redacted (§11 notes a possible
  "secret" refinement).
- **`convert` / `slice` / `add` / `size`** for framing envelopes, and a
  full **bit-syntax** (`make`/`unpack`/`convert Bytes` over a
  `BinarySpec`) that the boru layer can use to build the on-disk envelope
  headers (§5) declaratively.

**But `Bytes` comparison is not constant-time.** The type's `Equal` uses
`bytes.Equal` and its Comparer uses `bytes.Compare` — both short-circuit
(`native_bytes.go`). So the generic `eq`/`cmp`/`sort` on `Bytes` must
**never** compare a secret; `Crypto.eq` (§4) is the sanctioned
constant-time comparator, and the spec should say so prominently.

Option maps (`{n r p len}`, `{info len}`) follow the module convention:
a `TMap` param whose keys are validated (present, right scalar type)
before any compute.

## 7. Errors

Two families, checked in this order:

- **`crypto_usage`** — malformed *arguments*, detected **before** any
  cryptographic work: a key/nonce of the wrong length, a non-positive
  `rand-bytes` count, a scrypt/hkdf option out of range, a `box` key of
  the wrong size. These name the offending argument, exactly like the
  vault bridge validates op params before dispatch.
- **`crypto`** — a cryptographic *failure*: `aead-open`/`box-open` tag or
  MAC verification failed (wrong key, tampered ciphertext, or tampered
  AAD). This is a **single opaque error** — it must not distinguish
  wrong-key from tampered-AAD from bad-length-ciphertext, matching the
  vault's `errSlotAuth` / `aesGCMOpen` single-error discipline
  (`keyslot.go`, `keyring.go`). Surfacing *why* an open failed is a
  padding/authentication oracle.

`Crypto.eq` never errors — it returns `false` (including on length
mismatch), so comparison branches carry no exception channel.

## 8. Policy / capabilities (CRITICAL)

Every `Crypto.*` word is **pure compute**: it touches no disk, network,
process, environment, or clock. It therefore parallels the `terminal`
and `log` scopes, which are gated by their own `words` block but bind
**no global hard-cap**.

Wiring (the recipe from [PERMISSIONS.10](PERMISSIONS.10.md), verified
against `lang/go/policy`):

1. Add `"crypto"` to `policy.KnownScopes` (`lang/go/policy/policy.go`) —
   required before any profile or gate can name it (`Profile.Validate`
   rejects unknown scopes).
2. Do **not** add a `policy.GlobalsFor` case — omitting it means "gated
   only by the scope's own `words` block," which is correct for pure
   compute (pin it in the `GlobalsFor` table test).
3. Add `crypto` to every built-in profile in
   `lang/go/policy/profiles/*.jsonic`: `trusted` → `words.default:
   allow`; the restrictive profiles (`sandbox`, `compute`, `gen`) decide
   per profile. Note `crypto` legitimately belongs to a **compute**
   sandbox (it is pure math), so unlike `network`/`process` it need not be
   `install:false` there — an explicit call either way.
4. A `checkCryptoPolicy(r, op)` gate copying `checkVaultPolicy`
   (`lang/go/modules/vault.go`): `nil` registry → allow; `nil` policy →
   allow; `!pol.Installed("crypto")` → `[boru/capability_not_installed]`;
   else `pol.Check("crypto", op, args)`.

**Low severity of the child-registry leak here.** The known gap that
dispatch-time `checkXPolicy` gates become allow-all inside an imported
module body (see [PERMISSIONS.10 → Known gap](PERMISSIONS.10.md#known-gap-child-module-registries-do-not-inherit-the-policy))
applies to `checkCryptoPolicy` too — but the consequence is mild:
crypto has **no ambient effect**, so a module doing unauthorized *pure
math* leaks nothing (the real ceiling on crypto is CPU, a `Limits`
concern — e.g. an expensive `scrypt` — not a scope concern). The main use
of the `crypto` scope is `install:false` to remove the module wholesale.
The keyring seam and any effectful word are where the leak actually
matters — see [OS-KEYRING.0](OS-KEYRING.0.md).

## 9. Secure randomness

`Crypto.rand-bytes <n> → Bytes` returns `n` bytes read from
`crypto/rand` (`n` must be a positive Integer; otherwise `crypto_usage`).
It is the **only** sanctioned randomness for key material — salts,
nonces, keys, tokens, and the ephemeral scalars inside `box-*`. This is
§7.2 of the RFC, deliberately homed in `Crypto` and **not** `boru:rand`
(§2 firewall).

For tests, the entropy source must sit behind a package seam (mirroring
the vault's `p7boxRand` / `randRead` seams) so a CI run can inject a
deterministic reader and assert exact bytes, while production reads
`crypto/rand.Reader`. See [TEST-SEAMS.10](TEST-SEAMS.10.md).

## 10. Overlap

- **`boru:rand` (`Rand`)** — `math/rand`, seedable, reproducible;
  non-cryptographic. Never for key material (§2). No word overlap:
  `Rand.*` produces Integer/Float/Boolean/String draws, `Crypto.rand-bytes`
  produces CSPRNG `Bytes`.
- **`boru:bin-util` (`BinUtil`)** — the boundary codecs (`base64-*`,
  `hex-*`) that turn a `Crypto.*` `Bytes` into transportable text and
  back (the `Bytes`↔hex/base64 handoff is documented in
  [BYTES.10](go-modules/BYTES.10.md) §8), plus the **non-cryptographic**
  `fnv32`/`fnv64` hashes (which must never be used where
  `Crypto.sha256`/`hmac` is required).
  **Supersession (both proposals are unimplemented).** The
  [BIN-UTIL.10](go-modules/BIN-UTIL.10.md) proposal §4.3/§4.5/§4.6 also
  homes cryptographic hashes (`sha256`/`sha512`/…), `hmac`/`hmac-verify`,
  and secure random (`random-bytes`/`random-int`/`random-hex`) in
  `BinUtil`. Those overlap this module and are **superseded here**:
  cryptographic primitives belong in the curated, policy-scoped
  `boru:crypto`, not in a bit-twiddling utility that lacks the AEAD/KDF/
  sealed-box surface the vault actually needs. `boru:bin-util` should
  retain only its **non-cryptographic** binary surface — bitwise ops,
  rotates, `popcount`, CRC checksums, the `base*`/`hex`/`ascii85`
  encodings, and UUIDs — and drop the `sha*`/`hmac`/`random-*` words in
  favour of `Crypto.*`. (A pointer note is added at the head of those
  BIN-UTIL.10 sections; the final call is the maintainer's — flagged on
  the PR.)
- **`Bytes` ([BYTES.10](go-modules/BYTES.10.md))** — the carrier type
  (§6): immutable binary leaf with `convert`/`slice`/`add`/`size` and the
  bit-syntax the boru layer uses to frame envelopes.
- **`boru:vault` (`Vault`)** — the consumer. Today the bridge does the
  crypto in Go; the migration reverses that so the in-boru vault calls
  `Crypto.*`.
- **`boru:io` (`IO`)** — atomic writes, file locks, and permissions the
  in-boru vault uses to persist the sealed envelopes `Crypto.*` produces.

## 11. Open questions / out of scope

- **`argon2id` and XChaCha20-Poly1305** — RFC §7.1 lists both, but the Go
  vault uses **only** AES-256-GCM and scrypt (no argon2, no chacha, no
  pbkdf2 anywhere in `cmd/go/internal/vault`). They are **out of scope**
  for the migration: shipping them would add primitives the vault never
  calls and cannot interoperate through. Reserve an `{alg}` option key in
  the `aead-*` grammar and note `golang.org/x/crypto/argon2` as the future
  home, but do not build them until a consumer exists.
- **Shared low-level package vs re-implementation.** §5 requires
  duplicating the parameter set (the vault's copy is in an `internal`
  package this module cannot import). The alternative is a precursor
  refactor that extracts a shared, importable low-level crypto package
  both sides call. Recommendation: re-implement now with a parity
  cross-check test (§12); flag the shared-package extraction as an
  optional later cleanup.
- **A "secret" `Bytes` refinement (optional hardening).** `Bytes` already
  separates key material from text, but its `Format` still hex-dumps up to
  32 bytes, so a stray `print` of a secret leaks it as hex. A refined
  `Secret` subtype (see [REFINE-NEWTYPE-VS-SUBSET.10](REFINE-NEWTYPE-VS-SUBSET.10.md))
  with a redacting render (`Secret<redacted>`) and the constant-time
  comparator baked in would harden the ergonomics — a nice-to-have, not a
  blocker, and orthogonal to the migration.
- **Zeroization.** A GC'd interpreter cannot guarantee prompt erasure of
  secret bytes, and `Bytes` immutability makes an in-place wipe impossible.
  Document this as a known limit (the Go vault has the same constraint at
  the language layer) rather than promising memory hygiene the runtime
  cannot deliver.

## 12. Implementation sketch (wiring checklist — no code)

- **Module.** `BuildCryptoModule(parent)` in `lang/go/modules/crypto.go`,
  a fresh sub-registry holding the inner natives behind trivial-delegation
  FnDef wrappers with inner-sig **`BarrierPos: -1`** (the module-wrapper
  dispatch rule), exported into the `Crypto` map; register it in the
  resolver (`InstallResolver`) and add `moduleDocs["boru:crypto"]`.
- **Op table.** One descriptor per word (name, arg types, the policy op),
  mirroring `vaultOps` — arguments validated (`crypto_usage`) before the
  primitive runs.
- **Primitives.** `crypto/aes`+`cipher` (GCM), `golang.org/x/crypto/scrypt`,
  `golang.org/x/crypto/hkdf`, `crypto/sha256`+`sha512`, `crypto/hmac`,
  `golang.org/x/crypto/nacl/box`, `crypto/rand` — constants exactly per §5.
- **Entropy seam.** A package-level `randReader = crypto/rand.Reader` seam
  for deterministic tests (§9).
- **Policy.** Steps 1–4 of §8 (`KnownScopes`, no `GlobalsFor` case,
  profiles, `checkCryptoPolicy`).
- **Governance gates.** Add the module to `help.ModuleCatalog`
  (`catalog_sync`), the ADR-003 export-coverage TSV rows, and the
  `check-accuracy` ratchet — the same gates the `boru:vault` module had to
  satisfy.
- **Spec + tests.** `lang/spec/crypto.tsv` rows, positive **and** negative
  (wrong key/nonce length, tampered ciphertext → the single opaque
  `crypto` error, non-positive `rand-bytes`), a `TestTypeLiteralNoPanic`
  entry, and a **parity cross-check** test asserting the constants and a
  known-answer vector equal `cmd/go/internal/vault` (an AEAD seal the Go
  keyslot path can open, and vice versa).

## See also

- [VAULT-TUI-PORT.0](VAULT-TUI-PORT.0.md) §7 — the migration this unblocks.
- [OS-KEYRING.0](OS-KEYRING.0.md) — the sibling seam for OS keychain access.
- [PERMISSIONS.10](PERMISSIONS.10.md) — the policy model the `crypto` scope
  plugs into, and the child-registry gap §8 references.
- `cmd/go/internal/vault/keyslot.go`, `keyring.go` — the source of truth
  for every constant in §5.
