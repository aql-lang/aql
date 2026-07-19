# Go stdlib coverage map & non-goals

> **Status: living reference.** Maps the Go standard library against AQL's
> module surface — what is covered (shipped or designed), what AQL
> **deliberately will not cover** (non-goals), and what remains a
> **coverable gap** (fits AQL's model, not yet designed). Read
> [NATIVE-MODULES.10.md](NATIVE-MODULES.10.md),
> [GO-MODULES.10.md](GO-MODULES.10.md),
> [go-modules/README.10.md](go-modules/README.10.md), and
> [EXTENSION-MODULES.10.md](EXTENSION-MODULES.10.md) first.

## Framing

AQL is a **sealed, deterministic, single-threaded data/query language**
(no runtime `reflect`/`plugin`, one engine per registry, host effects
behind capability seams). That model — not feature parity with Go — is
what decides whether a stdlib package belongs. This note records the
boundary so it stays auditable as modules land.

Three buckets:

- **A. Covered** — shipped or specified by a design note.
- **B. Non-goals** — against AQL's nature; intentionally never wrapped.
- **C. Coverable gaps** — fit the model, simply not designed yet.

## A. Covered (shipped or designed)

| Go package(s) | AQL home | Status |
|---|---|---|
| `math`, `math/big`, `math/bits` | `aql:math-util`, `aql:bin-util` | shipped |
| `math/cmplx`, `math/big` (rationals) | number systems (`Arith` seam) | designed — [EXTENSION-MODULES](EXTENSION-MODULES.10.md) §6 |
| `math/rand` | `aql:rand` | shipped |
| `strings`, `regexp`, `unicode` | `aql:string-util` (+ Unicode classification) | shipped + designed — [STRING-UTIL](go-modules/STRING-UTIL.10.md) |
| `strconv` | `aql:strconv` | designed — [STRCONV](go-modules/STRCONV.10.md) |
| `fmt` | `aql:fmt` (+ core `format`, template strings) | designed — [FMT](go-modules/FMT.10.md) |
| `time` | `aql:time-util` | shipped |
| `sort` | core `sort` + `aql:array-util` | shipped |
| `bytes` | the `Bytes` type | designed — [BYTES](go-modules/BYTES.10.md) |
| `crypto/{sha*,md5,hmac,rand}`, `hash/{fnv,crc32,crc64}` | `aql:bin-util` | designed — [BIN-UTIL](go-modules/BIN-UTIL.10.md) |
| `encoding/{hex,base32,base64,ascii85}` (binary) | `aql:bin-util` | designed |
| `encoding/{json,xml,csv}`, yaml/toml/ini/… | `aql:parselang` / `aql:emitlang` / `aql:csv` | shipped + designed |
| `html` (escape), `net/url` (escape), text escapes, `mime/quotedprintable` | `aql:coding` | designed — [EXTENSION-MODULES](EXTENSION-MODULES.10.md) §4 |
| `net/mail`, message/header micro-formats | `aql:micro-format` | designed — [EXTENSION-MODULES](EXTENSION-MODULES.10.md) §5 |
| `net/http`, `net/url` (structure) | `aql:net`, `aql:url` | shipped + designed — [NET-URL](go-modules/NET-URL.10.md) |
| `io`, `io/fs`, all `os` filesystem | `aql:io` (all filesystem) | shipped + designed — [IO](go-modules/IO.10.md) |
| `os` (env/args/identity/exit) | `aql:os` | designed — [OS](go-modules/OS.10.md) |
| `os/exec` (run a command, capture output) | `aql:exec` | designed — [EXEC](go-modules/EXEC.10.md) |
| `runtime` (GOOS/GOARCH/NumCPU/…) | `aql:runtime` | designed — [RUNTIME](go-modules/RUNTIME.10.md) |
| `path`, `path/filepath` | `aql:path-util`, `aql:filepath` | designed |
| `text/template` | `aql:template` | designed — [TEXT-TEMPLATE](go-modules/TEXT-TEMPLATE.10.md) |
| `slices`, `maps` | `aql:array-util`, `aql:struct-util` | shipped |
| `testing` | `aql:test` | shipped |
| `context` (cancellation/deadlines) | `aql:time-util` async (timeout/await/cancel) | shipped (capability, not the package) |

## B. Non-goals — AQL deliberately will **not** cover

These serve Go-the-systems-language; they have no meaning in a sealed
query language. The reflection bridge ([GO-MODULES.10.md](GO-MODULES.10.md))
remains the escape hatch if a host ever truly needs one ad hoc.

| Area | Packages | Why excluded |
|---|---|---|
| Go toolchain / introspection | `go/*`, `debug/*`, `reflect`, `unsafe`, `embed` | Compiler/AST/unsafe-memory/build features. `reflect` is used *inside* the bridge, never exposed. |
| Dynamic loading | `plugin` | Breaks the sealed, deterministic model (rejected in GO-MODULES). |
| Concurrency primitives | `sync`, `sync/atomic`, `syscall`, `golang.org/x/sys` | AQL concurrency is the `time-util` async model; raw goroutines/mutexes/syscalls are not a query-language concern (one engine per registry). |
| Go-specific serialization / errors | `encoding/gob`, `errors` | `gob` is a Go-only wire format; AQL has its own `Error` type and value model. |
| Native collections (already built in) | `container/{heap,list,ring}` | AQL has `List`/`Map`/`Table`/`Record` + `array-util`/`struct-util`. |
| Host / runtime internals | `runtime/{pprof,trace,debug}`, `expvar`, `os/signal`, `flag` | Profiling, signals, metrics export, CLI-flag parsing belong to the *host* (`cmd/go`), not the language. |
| Graphics | `image/*` | AQL is a data/query language, not a rendering toolkit. |
| Low-level interface plumbing | exposed `io.Reader`/`Writer`, `bufio` | Streaming stays behind the `io`/capability seam; not a user-facing surface. |

## C. Coverable gaps — fit the model, not yet designed

Plausible future modules, roughly in priority order. None conflicts with
the model; each would follow the standard module conventions (and, where
effectful, a capability seam + policy scope).

| Candidate | Go packages | Notes / priority |
|---|---|---|
| **`aql:crypto`** (real ciphers) | `crypto/{aes,cipher,rsa,ecdsa,ed25519,tls,x509}`, `encoding/{pem,asn1}` | **Biggest gap.** `bin-util` did only hashing/HMAC/random — symmetric/asymmetric **encryption, signing, TLS, certificates** are uncovered and security-sensitive; deserves its own gated capability. |
| **`aql:archive`** | `archive/{tar,zip}` | Container files; pairs with `Bytes`/`io`. |
| **`aql:compress`** | `compress/{gzip,flate,zlib,bzip2}` | Byte→byte; sits beside `bin-util`. |
| **raw networking** | `net` (TCP/UDP/IP), `net.Lookup*` (DNS), `net/smtp`, `net/textproto` | `aql:net` is HTTP-only today; sockets/DNS/SMTP are gaps (security-sensitive, capability-gated). |
| **`aql:mime`** | `mime`, `mime/multipart` | Content-type detection + multipart form uploads (HTTP-adjacent). |
| html auto-escaping | `html/template` | `text/template` is covered; the *contextual auto-escaping* variant would build on `aql:coding`. |
| **`aql:log`** | `log`, `log/slog` | Structured logging — borderline with bucket B (host concern); include only if guest-side logging is wanted. |
| DB drivers | `database/sql` + non-sqlite drivers | AQL has native sqlite; other backends uncovered. |
| misc text/encoding | `encoding/binary` (struct pack/unpack), `unicode/utf16`, `text/tabwriter`, `os/user`, `hash/adler32` | Small/niche; `encoding/binary` extends `Bytes`; `tabwriter` overlaps `aql:report`. |

## Bottom line

The **"won't cover"** boundary is bucket B — Go's systems/runtime/
toolchain machinery, intentionally invisible to a sealed query language.
Everything in bucket C fits AQL's grain; the only sizeable, broadly-useful
omission from current designs is **real cryptography** (ciphers / PKI /
TLS), since `bin-util` stopped at hashing. After that, **archive /
compress** and **raw networking / DNS** are the next most defensible
additions (`os/exec` graduated to bucket A —
[EXEC](go-modules/EXEC.10.md)).
