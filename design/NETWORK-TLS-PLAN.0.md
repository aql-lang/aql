# TLS and client certificates for `aql:net` — implementation plan

> **Status: implementation plan.** Nothing here is built. It plans the
> work behind the TLS options that [NETWORK-CLIENTS.0.md](NETWORK-CLIENTS.0.md)
> §4.4 and [NETWORK-SERVERS.0.md](NETWORK-SERVERS.0.md) §4.4 already
> specify but which no code implements, and it **amends** their credential
> grammar: a client certificate is named, not pathed. Read
> [NETWORK-CLIENTS.0.md](NETWORK-CLIENTS.0.md) §4 and §9 first.

## 1. Context — why

Three facts, all verified against the tree:

1. **There is no TLS surface at all.** `lang/go/native/fetch.go:253`
   builds `&http.Client{Timeout: timeout}` — stock transport, system
   roots, no client certificates, and no injection point. `net_socket.go`
   has no `tls` handling, so `connect-raw` is plaintext TCP. Neither
   `crypto/tls` nor `crypto/x509` is imported anywhere in the tree.
2. **The design exists but is unimplemented.** NETWORK-CLIENTS §4.4
   specifies `tls: {verify: ca: sni: cert: key:}`, and its §11 roadmap
   puts "TLS-client, mutual TLS" in Phase B — never landed.
3. **The specified credential shape is wrong** and should be fixed before
   it is built, not after (§3).

The outcome: `fetch` and `connect-raw` can speak TLS, can verify a peer
against a supplied CA set, and can present a **client certificate the
guest program can name but never read**.

## 2. What Go gives us (the shape to mirror)

`tls.Certificate` holds a DER chain plus a `PrivateKey` typed
`crypto.PrivateKey` and required to satisfy `crypto.Signer` — **an
interface, not bytes**. The key need not exist in the process; it can be
an HSM/KMS/agent handle. Selection is either a static
`tls.Config.Certificates` slice or the per-handshake callback
`GetClientCertificate(*tls.CertificateRequestInfo)`, which wins when set
and receives the server's `AcceptableCAs` and `SignatureSchemes`.

**Mirror the callback, not the slice.** Late binding is what buys
rotation, multi-identity selection and HSM backing, and it costs nothing
now versus retrofitting later.

## 3. The one design change to the existing spec

NETWORK-CLIENTS §4.4 currently specifies `tls: {cert: "./gw.pem" key:
"./gw.key"}` — **guest-supplied file paths**. Do not build that. The path
is chosen by the guest but dereferenced by the network capability under
host authority, so a program holding `network` but not `fileops` obtains a
file read it is not entitled to — and, even blind to the bytes, an oracle
for path existence and key well-formedness. It also puts key locations in
source, logs and error text, and forecloses rotation and HSMs.

**Replace it with a named identity**, resolved host-side:

```
fetch {url: "https://api.internal/v1/orders"  tls: {identity: acme/q}}
connect-raw {tcp: "billing.internal:443"  tls: {identity: acme/q}}
```

Keep an explicit-bytes fallback (`tls: {cert: <Bytes> key: <Bytes>}`)
where the bytes arrive through `aql:io`/`aql:pki` under *their own* gates
— never a path the network layer dereferences. It requires the new
`client-cert` op and is off in the default profile.

Holding principle: **a certificate is data, a private key is a
capability.**

## 4. Architecture

Four pieces, each following an existing in-tree pattern.

| Piece | Pattern it copies | Location |
|---|---|---|
| `ClientIdentity` interface + `CertRequest` | `capabilities.FileOps` / `Clock` | `lang/go/capabilities/tlsident.go` (new) |
| `CapClientIdents` registry slot + `SetHostClientIdents` | `CapFormats` / `SetHostFormats`, `lang/go/native/capabilities.go:168-183` | `lang/go/native/capabilities.go` |
| `(*AQL).RegisterClientIdentity` | `(*AQL).RegisterFormat`, `lang/go/aql.go:484-491` (lazy map init + register) | `lang/go/aql.go` |
| HTTP transport seam | `SetHostFileOps` slot + `design/TEST-SEAMS.10.md` | `lang/go/native/fetch.go` + `capabilities.go` |

### 4.1 The identity seam

```go
// lang/go/capabilities/tlsident.go
type CertRequest struct {
    Host          string     // the peer being dialled
    AcceptableCAs [][]byte   // from tls.CertificateRequestInfo
    SigSchemes    []uint16
}

type ClientIdentity interface {
    // Certificate returns the chain + signer for this handshake, or a nil
    // certificate to decline (mapping to Go's empty-Certificate return).
    Certificate(req CertRequest) (*tls.Certificate, error)
}
```

Plus two host-side constructors so the common cases need no boilerplate —
and note both are **host** code, so the path read happens under host
authority with a host-chosen path, which is exactly the distinction §3
turns on:

- `capabilities.StaticIdentity(certPEM, keyPEM []byte) (ClientIdentity, error)` — wraps `tls.X509KeyPair`.
- `capabilities.FileIdentity(certPath, keyPath string) (ClientIdentity, error)` — wraps `tls.LoadX509KeyPair`.

Slot registration mirrors `SetHostFormats` exactly, including the
policy-uninstall branch:

```go
const CapClientIdents = "engine.clientidents" // map[string]capabilities.ClientIdentity

func SetHostClientIdents(r *Registry, ids map[string]capabilities.ClientIdentity) {
    if pol := HostPolicy(r); pol != nil && !pol.Installed("network") {
        _, _ = r.Capabilities.Delete(CapClientIdents)
        return
    }
    _ = r.Capabilities.Set(CapClientIdents, ids)
}
```

### 4.2 The transport seam

`fetch.go:253` must stop constructing its own client. Introduce:

```go
// lang/go/capabilities/httpops.go
type HTTPOps interface {
    Client(p TLSProfile, timeout time.Duration) (*http.Client, error)
}
```

`TLSProfile` is the resolved, comparable result of parsing the guest's
`tls:` map (identity name, verify flag, CA-set fingerprint, SNI, min
version). The default implementation:

- clones `http.DefaultTransport` and keeps **`ForceAttemptHTTP2: true`** —
  setting `TLSClientConfig` on a fresh `http.Transport` silently disables
  HTTP/2, so without this, adding a client cert would quietly downgrade
  every request to HTTP/1.1;
- sets `GetClientCertificate` to a closure over the resolved
  `ClientIdentity`;
- **caches one `*http.Client` per distinct `TLSProfile`** on the registry,
  so connection reuse survives and a per-request transport is not built.

This seam is independently valuable: it is the point where tests stub the
network without touching the wire.

### 4.3 Option grammar (one parser, both call sites)

```
tls: {
  identity: <Atom>     # host-registered client identity
  verify:   <Boolean>  # default true
  ca:       <Bytes>    # additional roots, PEM
  sni:      <String>   # server name override
  min:      <String>   # "1.2" | "1.3"
  cert:     <Bytes>    # gated fallback (with key:)
  key:      <Bytes>
}
```

One `parseTLSOpts(r, v Value, word string) (TLSProfile, error)` shared by
`fetch` and `parseNetAddr` (`net_socket.go:172` neighbourhood), so the two
call sites cannot drift.

### 4.4 Policy

No new scope. Inside the existing `network` scope
(`lang/go/policy/policy.go`, `KnownScopes`):

| Op | Meaning |
|---|---|
| `client-cert` | present a client certificate at all |
| `tls-insecure` | permit `verify: false` |

The check is already argument-aware — `fetch` passes
`policy.Args{"host":…, "port":…}` and `checkNetPolicy`
(`net_socket.go:172`) does the same — so **add the identity name to
`policy.Args`**. A profile can then allow-list which identity is usable
for which host, which is the property the path form cannot have: the
deployment decides which credential a program may present, not the
program.

`GlobalsFor("network", …)` already returns `["network"]`; both new ops
inherit it unchanged.

## 5. Phases

Ordered so each lands green on its own. Phase 1 is the smallest useful PR.

### Phase 1 — transport seam, no user-visible change
`lang/go/native/fetch.go`, `lang/go/native/capabilities.go`,
`lang/go/capabilities/httpops.go` (new), `lang/go/aql.go`.

Replace the inline client with `HTTPOps`, default implementation
reproducing today's behaviour exactly (stock transport + timeout).
**Done when:** existing `fetch_test.go` / `fetch_policy_test.go` /
`fetch_seam6c1_test.go` pass unchanged and `make cover-gate` is green.

### Phase 2 — base TLS client options
`parseTLSOpts` + `verify` / `ca` / `sni` / `min` on `fetch`. `tls-insecure`
policy op. Client certificates are meaningless before this exists.
**Done when:** a `httptest` TLS server with a private CA is reachable via
`tls: {ca: …}` and rejected without it.

### Phase 3 — client identities (the actual mTLS)
`capabilities.ClientIdentity`, `CapClientIdents`,
`(*AQL).RegisterClientIdentity`, `tls: {identity: …}`, `client-cert`
policy op, identity name in `policy.Args`.
**Done when:** an mTLS `httptest` server with `ClientAuth:
RequireAndVerifyClientCert` accepts a request carrying a registered
identity, and the same request without it fails with a distinguishable
error.

### Phase 4 — sockets
Wire the same `TLSProfile` into `connect-raw` via `parseNetAddr`
(`net_socket.go`), so `Socket` may be a TLS stream. Per NETWORK-SERVERS
§4.4 TLS is a socket option, not a separate API — the `Socket` type and
every socket word stay unchanged.

### Phase 5 — vault-backed identity (opaque handle)
`Vault.identity "acme-mtls"` → an opaque, module-minted external type
usable **only** as a `tls: {identity: …}` argument. Registered via
`RegisterExternalBuiltin` with a FixedID from the documented `10000+`
range, `Format` behaviour printing `<identity acme-mtls>`, no reveal path.
Add it to `lang/go/test/fixedid_stability_test.go`.

This exists because `Vault.reveal` returns a `String`
(`lang/go/modules/vault.go:186`) — revealing a private key into a guest
string is precisely the failure mode this design avoids.

### Phase 6 — server side
`listen {tls: {identity: … require-client: <Bytes>}}` (`ClientCAs` +
`ClientAuth`), and the verified peer chain surfaced as a Record
(`Net.peer-cert`) so authorization can be written in AQL.

### Phase 7 — docs
- **NETWORK-CLIENTS.0.md §4.4** — replace `cert:`/`key:` paths with
  `identity:`; record the confused-deputy rationale.
- **NETWORK-SERVERS.0.md §4.4** — same for the `listen` example at §723.
- **STDLIB-ALLOCATION.0.md §5.4** — the `crypto/tls` row says certificates
  arrive as `Bytes` from `aql:pki`; correct for the cert, incomplete for
  the key, which needs this seam.
- **STDLIB-COVERAGE.10.md** — move `crypto/tls` from bucket C to A.

## 6. Repo rules this work must satisfy

Non-negotiable, from `lang/go/CLAUDE.md`, `design/go-modules/README.10.md`
and ADR-008:

- **`BarrierPos: -1`** on any inner native sig of a module FnDef wrapper,
  or swap form stops dispatching (pinned by `wrapper_dispatch_test.go`).
  `fetch`'s three sigs already do this (`net_module.go:31-33`) — new sigs
  must match.
- **Every export needs a `registerDocs` line** (`docs_net.go`) or
  `TestModuleExportDocs` fails.
- **Spec rows** in `lang/spec/module-net.tsv`
  (`input⇥expected⇥description`, rows lead with `import "aql:net"`), and
  every positive row needs a negative sibling (`ERROR:<substring>`).
- **100% statement coverage** (`make cover-gate`). TLS code is
  error-branch-dense — plan a test per branch. A genuinely unreachable
  guard needs a proof-carrying `//covergate:allow <reason>` on the guard's
  opening line (`design/COVERAGE-ALLOWLIST.10.md`).
- **No panics**; guard args with `AsConcreteString` / `RequireConcreteMap`
  and fail with `r.AqlError(code, detail, word)`.
- **Effect fence.** `r.NoteEffect()` at `fetch.go:~250` stays where it is:
  with a custom transport the handshake happens inside `client.Do`, so the
  existing placement remains correct — everything before it provably sent
  nothing.

## 7. Errors

NETWORK-CLIENTS §8.2 routes handshake failure to `transport`. Keep that,
but make credential failure distinguishable — it is a configuration
problem, not a connectivity one:

| Condition | Code |
|---|---|
| dial/handshake failure | `transport` |
| unknown identity name | `net_error` (detail names the identity) |
| identity denied by policy | `policy.Denied` (existing shape) |
| peer rejected our certificate | `transport`, detail `client-cert-rejected` |

Note for the implementer: a server may reject a client certificate either
as a handshake alert **or** as an application-level 4xx after a successful
handshake. Only the first is catchable as `transport`; document that the
second surfaces as an ordinary response.

## 8. Verification

```bash
make fmt && make vet && make lint && make test && make cover-gate
```

Plus, specific to this work:

1. **Unit** — `go test ./lang/go/native/ -run Fetch` and
   `./lang/go/modules/ -run Net`. Use `net/http/httptest` (already an
   in-tree dependency) to stand up a TLS server; mint test certs with
   `crypto/x509` + `crypto/ed25519` in a test helper. **No test may touch
   a real network.**
2. **mTLS round trip** — `httptest.NewUnstartedServer` with
   `TLSConfig{ClientCAs: pool, ClientAuth: RequireAndVerifyClientCert}`;
   assert success with a registered identity, `transport` without one.
3. **Policy** — assert `client-cert` denial and `tls-insecure` denial
   produce `*policy.Denied`, and that `SetHostClientIdents` removes the
   slot when the profile has `network.install=false`.
4. **Redaction** — assert the Phase 5 handle formats as
   `<identity …>` and that no test can obtain key bytes from it.
5. **HTTP/2 non-regression** — assert the default transport still reports
   HTTP/2 against an h2 `httptest` server once `TLSClientConfig` is set;
   this is the regression the `ForceAttemptHTTP2` note in §4.2 prevents.
6. **Spec** — `make spec-test` (excluded from `make test`) after adding
   `module-net.tsv` rows.

## 9. Open questions

1. **Identity naming**: Atom (`acme/q`) or String? Atom reads better and
   matches the codec/kind convention; String is easier to compute. Lean
   Atom, accept both.
2. **Does `ca:` belong in `tls:` or should roots be a host seam only?**
   A guest-supplied root set widens trust; the `RootsProvider` seam
   proposed in STDLIB-ALLOCATION §4.1 may be the better owner.
3. **Client cache eviction** — an unbounded `TLSProfile → *http.Client`
   map is a slow leak in a long-lived server. Bound it, or key it to the
   registry lifetime?
4. **`min:` default** — TLS 1.2 (Go's default) or 1.3 (stricter, breaks
   older peers)?
