# VAULT-PROXY-REVIEW

> **Status: point-in-time review (dated).** A code review of the vault
> credential broker and a competitive comparison with **onecli**, pinned
> to commit `9e5ecd8` (2026-07-24). Per `design/README.md`'s
> classification, treat this as a *historical report*: the file/line
> anchors and the onecli feature summary are correct as of that date and
> will drift as both projects evolve. The forward-looking design home for
> the recommendations here is `design/SERVICES.0.md` §6 (the first-class
> `proxy` abstraction).

## Scope

There is no `proxy/` package. The subject of this review is the **vault
credential broker** — the local reverse-proxy that lets an AI agent call
an upstream API without ever holding the real credential. Its code lives
in `cmd/go/internal/vault/`:

| File | Role |
| --- | --- |
| `proxy.go` | The HTTP broker: routing, auth, policy, injection, streaming, audit. |
| `mcp.go` | An MCP (Model Context Protocol) server exposing the same aliases as agent tools, gated by the same capabilities. |
| `providers.go` | Built-in upstream presets and credential-injection styles. |
| `store.go` | Alias / capability / password-slot model; token hashing. |
| `service.go` | `ProxyService` — the `aql serve` lifecycle wrapper (pause/resume/status/metadata). |
| `ip_whitelist.go` | Per-alias source-IP allowlist enforced at the broker. |
| `audit.go` | Metadata-only append-only JSONL audit log. |
| `keyslot.go` / `keyring.go` / `session.go` | Envelope crypto, OS-keychain / file backends, authenticated sessions. |

The broker solves the same problem as **onecli** (`github.com/onecli/onecli`):
keep real API keys out of agent code. The two make nearly opposite
architectural bets, which makes the comparison instructive (§4).

## 1. Request model

```
agent → <method> http://127.0.0.1:8787/<alias>/<upstream-path>
        Authorization: Bearer <capability-token>

broker (proxy.go ServeHTTP):
  1. protocol-version check         (X-AQL-Vault-Protocol; fail-loud on newer client)
  2. split /<alias>/<path>          (splitAliasPath)
  3. authenticate bearer token      (FindCapabilityByToken → hashed, constant-time)
  4. token bound to this alias?     (tok.Alias == alias)
  5. resolve alias → provider       (LookupProvider; fixed upstream host)
  6. per-alias source-IP allowlist  (ipAllowed)
  7. capability policy              (capabilityDenial: revoked/expired/method/calls/budget/approval/host)
  8. look up real secret           (session.getValue, from keyring)
  9. build upstream request; strip Authorization; InjectAuth(secret)
 10. forward; stream response back; record use (calls + cost); audit
```

The design is a **reverse proxy with explicit alias routing**: the agent
targets a rewritten local endpoint and presents a scoped bearer token. The
upstream host is never taken from the request — it comes from a fixed
provider preset bound to the alias.

## 2. Strengths

- **Capability model** (`store.go:51`, enforced in `proxy.go:325`
  `capabilityDenial`). One token is bound to one alias and carries: TTL
  (`ExpiresAt`), method allowlist, host allowlist, per-call quota
  (`MaxCalls`), cost budget (`MaxCostCents`), a human-approval flag, and
  one-shot revocation. `rotate --revoke-caps` is a real
  incident-response primitive. This is materially richer than a plain
  "scoped token".

- **Token handling is correct.** Bearer tokens are stored only as
  hex SHA-256 (`hashToken`), compared in **constant time**
  (`FindCapabilityByToken`, `store.go:591`), with **no prefix match**.
  The plaintext token is shown once at grant and never persisted. The
  historical prefix-bypass is locked shut by a dedicated regression test
  (`proxy_security_test.go` `TestProxyRejectsTokenPrefix`,
  `TestCapabilityTokenAuth`).

- **The alias→provider binding is a hard SSRF boundary.** The upstream
  host is a fixed preset; `mustHost(provider.BaseURL)` is the host used
  for the capability host-check, and the forwarded URL is
  `provider.BaseURL + upstreamPath` (`proxy.go:272`). A crafted path such
  as `/<alias>/../../evil` still resolves to the provider's host. The
  request can never redirect the broker to an arbitrary host.

- **Cryptography well beyond a single symmetric key.** One scrypt
  (`N=2^15`, `keyring.go:780`) derives a master KEK over a per-vault salt;
  HKDF expands per-slot KEKs (`keyslot.go`), so `authenticate` is O(1) in
  slot count. Secrets are sealed under per-namespace data keys (NDKs),
  themselves X25519-sealed (`nacl/box`) into LUKS-style scoped password
  slots; values use AES-256-GCM with header-bound AAD. Backends are the OS
  keychain (macOS Keychain, Linux Secret Service, Windows Credential
  Manager) with an AES-256-GCM file fallback.

- **Secrets never reach the observable surface.** The access log and the
  JSONL audit log are metadata-only (`audit.go:23` — no token, no secret,
  no body). The capability token is stripped before forwarding
  (`out.Header.Del("Authorization")`, `proxy.go:283`).

- **Operational hardening.** Loopback-only bind with an explicit
  `--allow-public` opt-out (`proxy.go:84`); a protocol-version header with
  fail-loud mismatch; temporary passwords deliberately **not** cached by
  the broker so expiry actually bites (`proxy.go:106`); store schema
  versioning that fails closed on a newer on-disk file (`store.go:246`).

- **One enforcement path, two front-ends.** The proxy and the MCP server
  share the exact same `capabilityDenial` (`mcp.go:336`), so HTTP-token
  auth and MCP identity-auth cannot drift in what they permit.

- **MCP server mode** (`mcp.go`) exposes each brokerable alias as an MCP
  tool, gated by the identical capability checks — a native AI-agent
  integration with no onecli equivalent.

- **Test rigor.** The repo-wide 100% coverage gate (ADR-008) applies; the
  security-relevant paths carry negative tests (prefix rejection, public
  bind refusal, protocol mismatch).

## 3. Findings

Ordered by impact. File/line anchors are at commit `9e5ecd8`.

### 3.1 Only three upstreams work (functional, high)

`providers` (`providers.go:31`) is a hardcoded map: `openai`, `anthropic`,
`github`, and `generic` (empty `BaseURL` → refused, `proxy.go:240`). The
**only** writer of that map is a test helper (`registerTestProvider`);
there is no CLI or config path to register a custom upstream (base URL +
auth style). So the broker fronts OpenAI, Anthropic, and GitHub and
nothing else out of the box. This is the single limitation that most
narrows the proxy's usefulness relative to onecli, which brokers any API
via host/path patterns.

### 3.2 Streaming is passed through but never flushed (correctness/UX, high for LLMs)

`io.Copy(w, resp.Body)` (`proxy.go:305`) does not call `Flush()`. Go's
default response buffering can hold small writes until the buffer fills or
the handler returns, so **server-sent-event streams — token-by-token LLM
output, the dominant traffic shape for this exact use case — can be
delayed or coalesced** rather than delivered in real time. `SERVICES.0.md`
§6 treats streaming replies as first-class, but the shipping handler does
no incremental flush.

### 3.3 Cost budgets are inert against real providers (design gap, medium)

`MaxCostCents` is debited only from an `X-AQL-Vault-Cost-Cents` **response
header** (`parseCostHeader`, `proxy.go:300`) that no real provider sends.
Genuine cost metering would require parsing provider-specific usage from
the response body — which the broker deliberately does not read (§3.2's
streaming pass-through depends on not materialising the body). So the
cost-budget capability is aspirational for the built-in providers today.
There is a real tension here: streaming pass-through vs body inspection
for cost.

### 3.4 Call quota is a soft cap (concurrency, medium)

The quota check (`UsedCalls >= MaxCalls`, `capabilityDenial`,
`proxy.go:333`) runs at request start under no lock; the increment
(`recordCapabilityUse`, `store.go:656`) happens afterward under
`mutateStore`. Concurrent requests can all pass the check before any
increment, overshooting `MaxCalls` by up to (concurrency − 1). Acceptable
for a single loopback agent, but `MaxCalls` is a **soft** limit, not a
hard one — it should be documented as such, or reserved-under-lock if it
must be hard.

### 3.5 Per-request full-file rewrite (scalability, medium)

Every request re-reads and re-parses `vault.jsonic` (`requireStore`,
`proxy.go:196`), and `recordUse` does a full read-modify-**rewrite** of
the whole metadata file. For a scoped-password vault, `SaveStore`
additionally computes a content hash, writes a signature sidecar, and
appends + prunes an event-sourced journal — **per request** (`store.go:319`).
That is a full-file rewrite (and fsync) on every proxied call. The reload
does buy live revocation without a restart (a fair trade), but the
per-request rewrite is the throughput ceiling once request volume is real.

### 3.6 `RequireApproval` over-promises (naming/semantics, low)

`RequireApproval` denies and audits but provides no approve-and-proceed
workflow — the code comment calls it "advisory" (`proxy.go:337`). It is
effectively a per-capability hard stop; the name suggests an approval
gate that does not exist.

### 3.7 Minor observations

- A use (call count + cost) is recorded for **any** upstream response,
  including upstream 4xx/5xx (`proxy.go:301`). Debiting a quota on an
  upstream rejection is defensible ("a call was made") but may surprise.
- `copyHeadersExceptHop` forwards all non-hop client headers upstream
  (minus `Authorization`). Injection styles use `Set` (overwrite), so a
  client cannot smuggle a conflicting auth header, but it can pass other
  arbitrary headers to the provider. Low risk given the fixed host.

## 4. Comparison with onecli

**onecli** (`github.com/onecli/onecli`, as of 2026-07-24): a Rust HTTP
gateway (`:10255`) + Next.js dashboard (`:10254`) + PostgreSQL/Prisma,
deployed via Docker. Agents set an HTTP proxy, call the **real** API host
with a placeholder key (e.g. `FAKE_KEY`), and the gateway **MITM-intercepts
HTTPS** (installed CA cert), matches the request by host/path pattern,
swaps `FAKE_KEY` → real key (AES-256-GCM at rest under
`SECRET_ENCRYPTION_KEY`), and forwards. Team mode adds Google OAuth. Its
public docs mention no quotas, cost tracking, audit log, expiry, or
revocation.

| Dimension | **aql vault proxy** | **onecli** |
| --- | --- | --- |
| Proxy model | Reverse proxy: `localhost/<alias>/<path>` + bearer token | Forward proxy: `HTTP_PROXY`, real host, `FAKE_KEY` swap |
| Agent code changes | Yes — rewrite endpoint, present token | ~None — placeholder key + proxy env var; any SDK works unchanged |
| TLS trust surface | None (no MITM; upstream leg is HTTPS) | MITM of all agent HTTPS via an installed CA cert |
| Upstreams | 3 hardcoded presets (§3.1) | Arbitrary, via host/path patterns |
| Stack / ops | Single Go binary, no DB, no dashboard | Rust gateway + Next.js + PostgreSQL + Prisma + Docker |
| Secret at rest | scrypt → KEK, X25519 envelope, scoped keyslots, OS keychain | AES-256-GCM under one env-var key |
| Access control | TTL, method/host/IP allowlist, call quota, cost budget, approval, revoke | Per-agent scoped tokens (details unspecified) |
| Audit / history | Metadata-only JSONL + event-sourced journal + generations/restore | Not documented |
| AI-agent integration | Native **MCP** server mode | None documented |
| Team UX | CLI / TUI, single-user | Web dashboard + Google OAuth |

### The philosophical split

- **onecli optimizes for drop-in transparency.** Install a CA cert, point
  `HTTP_PROXY` at the gateway, leave agent code untouched — at the cost of
  MITM-ing all agent TLS (a large trust surface) and running a
  Postgres + dashboard + Docker stack.
- **AQL optimizes for an explicit, auditable trust boundary.** No MITM, no
  CA cert, no DB; a hard alias→host binding (§2 SSRF boundary) and a rich
  capability/audit model — at the cost of making the agent target a
  rewritten endpoint and speak a bearer token.

For AQL's stated positioning (a single self-documenting binary,
security-first, agent-oriented), these are the right bets. AQL is
**meaningfully more secure and more governable**; onecli is **more
convenient and more universal** out of the box. The two big things onecli
buys — arbitrary upstreams and zero agent-code change — are exactly the
gaps §5 recommends closing.

## 5. Recommendations

In priority order.

1. **User-definable providers (closes §3.1).** Add
   `vault provider add <name> --url=<base> --auth-style=<style>` persisting
   custom presets in the store, so the broker fronts any API rather than
   three. The alias→provider→SSRF-boundary machinery already supports it;
   only the registry is closed.
2. **Optional drop-in forward-proxy mode (neutralizes onecli's ergonomic
   edge).** Offer a non-default `HTTP_PROXY` + placeholder-swap mode
   alongside the explicit reverse proxy, so teams that cannot reconfigure
   agent endpoints still get zero-code-change onboarding — while the safer
   reverse proxy stays the default.
3. **Flush streaming responses (closes §3.2).** Wrap the copy in a
   periodic / `http.Flusher` flush so SSE token streams pass through in
   real time.
4. **Reconcile cost budgets with reality (closes §3.3).** Either ship
   opt-in provider usage parsers or document `MaxCostCents` as inert for
   the built-in providers today.
5. **Decide hard vs soft on `MaxCalls` (closes §3.4).** Reserve-under-lock
   for a hard cap, or document the TOCTOU as an intentional soft cap.
6. **Throttle the per-request store rewrite (closes §3.5).** Keep quota
   counters in memory and flush periodically (or to a compact append-only
   counter log) before this handles real volume.
7. **Rename or complete `RequireApproval` (closes §3.6).** Either build the
   approve-and-proceed workflow or rename it to reflect the hard-stop it
   is.

## 6. Relationship to the SERVICES roadmap

`design/SERVICES.0.md` §6 already envisions generalizing this broker into
a first-class `proxy <target> {before after}` abstraction — `before` =
capability auth + injection, `target` = a `connect`ed upstream, `after` =
quota/cost accounting + audit — with **streaming replies as a first-class
reply shape** and capability-checked interceptors. That RFC is the right
home for recommendations 3–6 in particular: streaming flush, cost
accounting placement, and the accounting/quota model are precisely the
"careful points the model must honour" it enumerates, not yet reflected in
the shipping handler. This review is the point-in-time evidence those
design points are drawn from.
