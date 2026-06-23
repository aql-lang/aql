# `aql:crypto-rand` — Go `crypto/rand`

> **Status: design proposal, not implemented.** A curated, hand-written
> native module wrapping Go's `crypto/rand`. Read
> [README.10.md](README.10.md) first for the shared conventions this note
> assumes. This is the **one non-pure** module in the crypto family — it
> draws OS entropy. Contrast it with the existing
> [`aql:rand`](../../lang/go/modules/rand.go) (`Rand` namespace), which is
> a deterministically seedable PRNG.

## 1. Package & status

Go [`crypto/rand`](https://pkg.go.dev/crypto/rand) is the
cryptographically secure random source: `rand.Read(b []byte)` fills a
buffer from the OS CSPRNG (`rand.Reader`), `rand.Int(rand.Reader, max
*big.Int) (*big.Int, error)` draws a uniform integer in `[0, max)`, and
`rand.Prime(rand.Reader, bits)` generates a random prime. This note wraps
the first two (Prime is deferred). Nothing is implemented yet.

## 2. Why curated

The raw `go:` bridge would surface `rand.Read([]byte)` (a Bytes buffer
AQL cannot hold) and `rand.Int(io.Reader, *big.Int)` (an `io.Reader` and
a `*big.Int`, neither expressible). The curated surface hides
`rand.Reader` entirely and returns AQL-native scalars: random bytes as a
hex `String` or a `List[Integer]`, and a bounded random `Integer`.
Crucially it routes entropy through a **host capability seam** (see §7)
so a sandbox or test host can deny or substitute it — the raw bridge
would call the OS directly, uncontrollably.

## 3. Import & namespace

```
import "aql:crypto-rand"        # binds the CryptoRand namespace
```

The bare namespace `Rand` is **already taken** by the existing
`aql:rand` module (the deterministic PRNG; `lang/go/modules/rand.go`).
To avoid the collision this module uses the compound id `aql:crypto-rand`
and the namespace `CryptoRand` (per the disambiguation rationale in
`lang/go/CLAUDE.md` "Package layout" — the `-util` suffix marks utility
libraries, so a domain prefix is used here instead). Words are
dot-accessed: `CryptoRand.hex`, `CryptoRand.bytes`, `CryptoRand.int`.

## 4. API

Signatures are **top-first, sig order** (position 0 is the top of the
stack). All inner native sigs use `BarrierPos: -1` so the swap form
dispatches.

| Go symbol | aql word | signature (top-first) | one-line doc | aql-ish refinement |
|---|---|---|---|---|
| `rand.Read(b)` | `hex` | `[Integer n] -> String` | n cryptographically random bytes as a hex string. | Fills an n-byte buffer from the entropy seam, hex-encodes it → 2n-char String. `(value, err)` → value-or-error `entropy-failed`; `n < 0` errors `bad-length`. |
| `rand.Read(b)` | `bytes` | `[Integer n] -> List` | n cryptographically random bytes as integers 0–255. | Same fill, returned as a `List[Integer]` (each 0–255). Same errors. The interim stand-in for a real Bytes type. |
| `rand.Int(rand.Reader, max)` | `int` | `[Integer max] -> Integer` | Uniform random integer in [0, max). | `max` as `*big.Int`; `rand.Int` result back to Integer; `max <= 0` errors `bad-bound`; `(value, err)` → value-or-error `entropy-failed`. |

## 5. Types

Scalars + List only — Integer in; String / `List[Integer]` / Integer
out. No opaque handle type, no `RegisterExternalBuiltin` / FixedID (the
`*big.Int` and the byte buffer are internal to the handler). Boundary
conversion via `eng.FromNative` / `eng.ToNative` (`eng/go/gobridge.go`):
the Integer arg via `AsConcreteInteger`, the byte list built with
`eng.ToNative` over a `[]int`.

## 6. Errors

Signalled with `r.AqlError(code, detail, word)`, kebab-case codes:

| code | raised when |
|---|---|
| `bad-length` | `n < 0` for `hex` / `bytes`. |
| `bad-bound` | `max <= 0` for `int`. |
| `entropy-failed` | the entropy seam returns an error (host denied, or OS CSPRNG read failed). |
| `entropy-denied` | the policy denies the random capability (distinct from a runtime failure — see §7). |

Guard the Integer arg with `AsConcreteInteger`; never panic
(`eng/go/CLAUDE.md` "Panic Prevention"). A denied capability surfaces as
a `*policy.Denied` unwrapped into `entropy-denied`, mirroring how
`fileops` denials surface.

## 7. Policy / capabilities

**This is the crux of the note.** Drawing OS entropy is a **side effect**
and is **nondeterministic** — every call returns different bytes and the
result cannot be reproduced. That makes `aql:crypto-rand` fundamentally
unlike every other module in this crypto family (all pure) and unlike its
near-namesake `aql:rand`.

**Contrast with `aql:rand`.** `aql:rand` is a `math/rand` PRNG that is
**deterministically seedable**: `Rand.with-seed 42` mints an isolated
instance whose stream is fully reproducible (the docstring in
`lang/go/modules/rand.go` spells this out — good for property tests,
demo fixtures, replayable simulations). `aql:crypto-rand` is the
opposite: an **unseedable CSPRNG**. There is deliberately no
`with-seed` — seeding a CSPRNG would defeat its purpose. So the two
modules occupy distinct niches:

| | `aql:rand` (`Rand`) | `aql:crypto-rand` (`CryptoRand`) |
|---|---|---|
| Source | `math/rand` PRNG | OS CSPRNG (`crypto/rand`) |
| Seedable | yes (`with-seed N`) | **no** — unseedable by design |
| Reproducible | yes (per seed) | **no** — nondeterministic |
| Use for | tests, fixtures, sampling | tokens, keys, nonces, salts |
| Purity / policy | non-deterministic default, but no host effect gated | **host effect — gated** |

**The capability seam.** There is currently **no policy scope for
randomness** — `policy.KnownScopes` is `global / engine / modules /
fileops / network / sqlite / formats / env / process / clock`, and
`policy.GlobalOps` is `disk.read / disk.write / network / process / env /
clock / system-info / mutate` (`lang/go/policy/policy.go`). None of these
fits OS entropy. Rather than calling `crypto/rand` directly, this module
should route the entropy draw through a **host capability seam** modelled
on `FileOps` and `EffectiveClock` (see `lang/go/capabilities/` and the
clock seam used in `rand.go`'s `native.EffectiveClock(parent)`): an
injectable interface (e.g. `EntropySource.Read([]byte) error`) resolved
off the registry, with an OS-backed default and an in-memory / deny-able
test implementation. A sandbox or test host can then **deny** entropy
(→ `entropy-denied`) or **supply a deterministic source** (so a test
run is reproducible despite using the crypto API). This matches the
README's "host-backed effects (entropy for `crypto/rand`, any file read)
go through a capability seam like `FileOps`, not direct OS calls" rule.

## 8. Overlap

Heavy *conceptual* overlap with [`aql:rand`](../../lang/go/modules/rand.go)
(`Rand`) — both produce random values — but **no word overlap and a clear
dividing line**: `aql:rand` is the deterministic/seedable utility PRNG
for sampling and tests; `aql:crypto-rand` is the secure, unseedable
source for security material. The note does not move or change any
`Rand` word. The hex encoding overlaps with
[`aql:hex`](ENCODING-HEX.10.md) but is baked in, as in the hash notes.

## 9. Examples (args-before form)

```
import "aql:crypto-rand"

16 CryptoRand.hex          # e.g. "9f86d081884c7d659a2feaa0c55ad015"  (32 hex chars)
4  CryptoRand.bytes        # e.g. [155, 200, 17, 42]  (List[Integer], each 0–255)
100 CryptoRand.int         # e.g. 73   (uniform in [0, 100))
-1 CryptoRand.hex          # ERROR:bad-length
0  CryptoRand.int          # ERROR:bad-bound
# under a sandbox policy that denies the random capability:
16 CryptoRand.hex          # ERROR:entropy-denied
```

## 10. Open questions / out of scope

- **Should there be a dedicated `random` policy scope?** This is the open
  design question. Options: (a) add `random` to `policy.KnownScopes` +
  a `random` global op in `policy.GlobalOps` and gate the entropy seam on
  it, mirroring `clock`; (b) reuse an existing coarse cap (none fits
  cleanly — entropy is neither `system-info` nor `process`); (c) keep it
  purely at the capability-seam level (install / don't install the
  `EntropySource`) with no named scope. Recommendation leans toward (a)
  for auditability, but it expands the fixed policy enum and so needs
  maintainer sign-off.
- **`rand.Prime`** — random prime generation is out of scope for the
  first cut (niche; needs a big-int return story). Add as
  `CryptoRand.prime n-bits` later if a key-generation use case appears.
- **Bytes output** — `bytes` returns `List[Integer]` only because AQL has
  no Bytes type; once one exists, `bytes` should return it directly and
  `hex` becomes a thin convenience over it. Tracked with the hash notes.
- **Default purity expectation** — even with the seam, `CryptoRand.*` is
  nondeterministic; `describe`/docs must make this explicit so it is
  never mistaken for a pure word.

## 11. Implementation sketch

Wiring checklist — no Go code. Reference: `lang/go/modules/io.go`
(`BuildIOModule`, the **capability-backed** builder) — NOT the pure
`math.go`, because this module gates a host effect.

- `lang/go/modules/crypto_rand.go` — `BuildCryptoRandModule(parent
  *native.Registry) (native.ModuleDesc, error)`: isolated
  `native.DefaultRegistry()` sub-registry, register a `CryptoRandNatives
  []native.NativeFunc` slice (each inner sig `BarrierPos: -1`), each
  handler resolving the `EntropySource` seam off the registry (à la
  `native.EffectiveClock` / `HostPolicy(reg)` in `io.go`) and checking
  the policy before drawing. Wrap each word as an `FnDef` export, return
  `ModuleDesc{ID: parent.Modules.NextID(), Exports: {"CryptoRand": …}}`.
- Register `"crypto-rand": BuildCryptoRandModule` in the `modules` map in
  `lang/go/modules/modules.go`.
- Add the entropy seam under `lang/go/capabilities/` (OS-backed default +
  deny-able / deterministic test impl) and a `SetHostEntropy`-style hook,
  paralleling `FileOps`.
- `lang/go/modules/docs_crypto_rand.go` —
  `registerDocs("aql:crypto-rand", map[string]string{…})`, one line per
  export, each flagging nondeterminism (`TestModuleExportDocs`).
- `lang/spec/module-crypto-rand.tsv` — rows leading with `import
  "aql:crypto-rand"`. Because output is random, positive rows assert
  *shape* not value (e.g. `16 CryptoRand.hex` length is 32 via a wrapping
  expression, or use the deterministic test seam); every positive row
  paired with an `ERROR:<substring>` negative sibling — `bad-length`,
  `bad-bound`, `entropy-denied` (under a deny policy), and the
  unqualified-word `undefined_word` without import.
- Policy wiring per the §7 decision (new `random` scope vs seam-only). No
  FixedID entry (no external type).
