# VAULT-WIRE-PROTOCOL.0 — `boru vault serve`, the secret-provision wire protocol

Status: implemented (`cmd/go/internal/vault/serve.go`; tests in
`serve_test.go`). Version 0 of this note documents wire protocol **v1**.

## Problem

The vault had three access surfaces — the CLI (`get`/`exec`), the
forwarding credential broker (`proxy`), and the MCP server — but no way
for an ordinary application to *fetch a secret over a socket*. The proxy
deliberately never returns the secret (it injects it into an upstream
call); `get` requires spawning the CLI. A secret-provider client library
(voxgig/sekreto) needs a stable, minimal wire protocol: address + token
in, secret value out.

## Decision

`boru vault serve` runs a **read-only HTTP API shaped after HashiCorp
Vault's KV v2 surface** — the de-facto wire protocol for secret
provision. Compatibility buys two things: sekreto's clients stay
trivial, and stock Vault client libraries work against a boru vault
unchanged (reads only). The server reuses the existing machinery
wholesale: capability tokens from `vault grant` (hashed storage,
constant-time compare), `capabilityDenial` policy checks, the broker
Session (one scrypt at startup), per-alias IP whitelists, and the audit
log.

## Surface (protocol v1, fixed `secret` mount)

| Method + path | Auth | Purpose |
|---|---|---|
| `GET /v1/sys/health` | none | liveness: `200` ok, `503` sealed (locked), `501` uninitialized |
| `GET /v1/auth/token/lookup-self` | token | metadata for the presented token (never echoes it) |
| `GET /v1/secret/data/<name>` or `/v1/secret/data/<ns>/<name>` | token | read one secret, KV v2 response shape |
| `LIST /v1/secret/metadata[/<ns>]` (or `GET …?list=true`) | token | keys the token may read; `<ns>/` folder entries at root |

- A secret path is the wire spelling of the stored alias: `name` (root
  namespace) or `ns/name` for `ns:name`. Exactly one namespace level;
  segments follow the alias charset (`[A-Za-z0-9._-]`).
- Tokens ride in `X-Vault-Token` (HashiCorp convention) or
  `Authorization: Bearer …`.
- Success bodies use the HashiCorp envelope: `request_id`, `lease_*`,
  and `data`. A KV read nests the value as `data.data.value` with
  `data.metadata.created_time` (the alias's last write),
  `custom_metadata.provider` / `.expires_at`, and a constant
  `version: 1` (boru keeps no per-secret version history).
- Errors are `{"errors":["…"]}`. A missing secret (and an empty list)
  is a **bare 404 with an empty errors list**, matching HashiCorp so
  stock clients' not-found detection works. Auth failures are `403
  permission denied`; a locked vault is `503 sealed`; quota exhaustion
  maps like the proxy (`429` calls, `402` budget).
- Every response advertises `X-Boru-Vault-Protocol: 1`; a client
  declaring a newer protocol is refused `400`, mirroring the proxy's
  negotiation.
- Every response carries `Cache-Control: no-store` — behind the
  operator's reverse proxy, a shared cache must never replay one
  caller's secret to the next.
- LIST is bounded by every gate a GET enforces: the capability, the
  serving session's namespace scope, and the per-alias IP whitelist. A
  name a client could never read is not a name it gets to learn — and a
  temporary broker password past its expiry stops answering lists too.
- Store-loading failures answer with a fixed `vault store unreadable`;
  the path/parse detail stays in server-side logs, never in a response
  to an unauthenticated caller.
- `sys/health` writes an access-log line but no audit record: liveness
  probes arrive every few seconds and record no secret-relevant act, so
  auditing them would only grow the log without bound.

## Authorization model

The unit of authorization stays the **capability** (`vault grant`):

- A capability bound to one alias reads exactly that alias.
- New: **namespace wildcards**. `vault grant 'ns:*'` mints a capability
  whose Alias is the wildcard form; it covers every alias in exactly
  one namespace — never more — keeping the invariant that a capability
  binds to a single namespace. Wildcard references resolve exactly like
  names: a bare `'*'` means the active default namespace (root when
  none is set), `':*'` forces root, `'ns:*'` is explicit.
  `validAlias` rejects `*`, so a wildcard can never collide with a real
  alias; the proxy and MCP broker resolve capabilities by exact alias
  name, so a wildcard token **fails closed** everywhere except `serve`.
- Shared policy via `capabilityDenial`: revocation, TTL, `--methods`
  (checked against the wire verb, so `--methods=GET` fits read-only
  provisioning), `--max-calls` (each successful read debits the
  counter via `recordCapabilityUse`, persisted before the value is
  written), `--max-cost-cents` state, and `--require-approval`. The
  host allowlist is skipped — there is no upstream host here.
- Per-alias `--ip-whitelist` is enforced against the client IP, as at
  the proxy.
- Defence in depth: the serving process opens its own Session (broker
  password), and every read additionally passes `requireScopeNS(OpRead)`
  — a token for namespace `ns` provisioned through a broker password
  that does not cover `ns` is refused (and the session would lack the
  NDK to decrypt it anyway).

## Non-goals

- **Writes.** Provisioning hands secrets out; mutation stays on the
  authenticated CLI. A read-only network surface cannot be escalated
  into a vault editor.
- **Vault's auth methods / leases / policies.** `lease_duration` is 0,
  `renewable` false, policies constant `["default"]`. The capability is
  the policy.
- **TLS.** Like the proxy, `serve` binds loopback by default and
  refuses non-loopback without `--allow-public`; production exposure
  belongs behind the operator's own TLS terminator.

## Client contract (what sekreto implements)

1. Config: address (default `http://127.0.0.1:8200`) + token, typically
   from `SEKRETO_ADDR`/`SEKRETO_TOKEN` falling back to HashiCorp's
   `VAULT_ADDR`/`VAULT_TOKEN`.
2. `get(name)` → `GET /v1/secret/data/<path>` where `<path>` replaces
   the alias's `:` with `/`; on 200 return `data.data.value`; 404 →
   not-found; other → error carrying `errors[]`.
3. `list(ns?)` → `LIST /v1/secret/metadata[/<ns>]` → `data.keys`.
4. `health()` → `GET /v1/sys/health`; `lookup()` →
   `GET /v1/auth/token/lookup-self` for token validation.
