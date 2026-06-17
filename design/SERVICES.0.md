# SERVICES

Design for a unified **service / server** model in AQL — one model that covers
both *user code written in AQL* (a value that owns state and answers
pattern-matched requests) and *the CLI's own long-running servers* (`repl`,
`registry`, `lsp`, `exec`, `api`, `tui`, `vault-proxy`). BEAM/OTP is the
inspiration for the request/reply + supervision shape, but the vocabulary stays
**AQL-native**.

## Terminology (settled)

Two words, in a deliberate hierarchy:

- **service** — the unit. A value that owns **state** and answers requests
  matched against registered handlers (a gen_server, in OTP terms). This is also
  exactly the existing Go `service.Service` interface (`Name`/`Start`/`Stop`/
  `Status`). One service = one isolated unit of state + messages.
- **server** — a **collection of services**, supervised together. This is exactly
  the existing `serve` supervisor (`aql serve registry + lsp + …`). A server
  starts, stops, restarts, and exposes its services; it owns no request handlers
  of its own.

> **"Behaviour" is deliberately avoided.** In AQL a `Behavior` is already a
> core type-system concept — the per-type operation dispatch (Match/Format/
> Equal/Compare) in `eng/go/compare_scalar_behaviors.go` and
> `lang/spec/user-types.tsv`. Reusing the word for services would collide.

### Finding the middle ground (the naming journey)

This design went too far in both directions before settling:

| | dropped (too Seneca) | dropped (too Erlang) | **settled (AQL-native)** |
| --- | --- | --- | --- |
| the unit | `service` ✓ | `server` / behaviour | **`service`** |
| collection of units | (plugins in one instance) | supervision tree | **`server`** (a collection of services) |
| register a handler | `add` | `handle` (`handle_call`) | **`add`** (reuse the existing patrun word) |
| sync request → reply | `act` | `call` | **`call`** |
| async, no reply | *(none)* | `cast` | **`send`** (reuse the actor word) |
| handler state | closure `Store` | `[req state]->{reply newstate}` tuple | **`[req state] -> reply`**, `state` a private mutable `Store` |
| compose units | `prior` override chain | supervision tree | **server** + **proxy** (see §3, §6) |
| run it | `host` | `start_link` | **`serve`** (reuse the CLI verb) |
| no-match error code | `no_action` | `no_clause` | **`no_match`** |
| framework module | `aql:mesh` | `aql:otp` | **`aql:serve`** + **`aql:net`** |

The two anchors that pull this back to AQL: **`add`** (services register handlers
with the same word patrun already uses) and **`send`** (async delivery is the same
word the actor layer uses for a `Pid`). `call` and `serve` are kept because they
are plain and already meaningful (`aql serve`). Erlang's `cast`/`handle`/
`handle_call`/`start_link`/`one_for_one` jargon is dropped.

## Context

The end goal is efficient, safe network servers/clients. `PROCESSES.0.md`
specifies the low-level **actor substrate** (processes, mailboxes, `send`/
`receive`, selective receive via patrun). This document specifies the layer most
code is actually written against — services and servers — and, crucially, shows
how the CLI's **existing** servers fold into the same model (§5).

### Why this is mostly already in AQL

A service is **pattern-matching on requests + owned state**, and the matcher is
already core: patrun (`lang/go/native/native_patrun.go`). `add {pattern} value
patrun` already registers most-specific-wins rules; `find {subject} patrun`
already routes a request (`{op:'inc n:1}` matches a handler for `{op:'inc}`,
extra keys are payload) — the *same* matcher `receive` uses for selective receive.
"Match a message" means one thing everywhere.

This is a **design RFC only — no implementation code yet.**

### Scope decisions (carried forward)

1. **Both — hosted on demand.** A `Service` is a lightweight in-process value by
   default: `call` is direct dispatch in the caller's goroutine; state is a
   private `Store`. On demand it is placed in a **server** and `serve`d — then it
   runs as a `PROCESSES.0.md` process and `call`/`send` route via messages,
   transparently.
2. **Hybrid packaging.** The in-process surface — `service`, `add`, `call`,
   `send`, `state` — is **core** (next to the patrun words). Running, supervising,
   proxying, and transport live in modules **`aql:serve`** and **`aql:net`**.
3. **Services live in modules.** A reusable service is just an AQL module that
   exports a constructor function returning a `Service`. No special plugin
   mechanism — `import` is the loader.

## 1. The core model — a service

### The `Service` value (core)

A new Ideal type **`Service`** wrapping: a **patrun** of handlers (reuse
`patrunMatcher`); the service's private **state**, a mutable `Store` (default
`{}`); and a **host** reference (`None` in-process, a `Pid` once `serve`d, §4).

`service {state} -> Service` constructs one.

### `add` (core) — register a handler

```
add {pattern} [handler] <service> -> Service
```

`handler` is **`[req:Map state] -> reply`**. The `state` argument is the
service's private mutable `Store`; the handler **mutates it in place** for state
changes and **returns the reply** value. This is the deliberate middle ground:
no functional `{reply, NewState}` tuple ceremony (the over-Erlang trap), and no
hidden closure capture (the Seneca trap) — state is an explicit, named, mutable
value, which is exactly what `Store` is for in AQL. It is safe because a service
processes **one request at a time** (in-process: the caller's goroutine; served:
the process's single goroutine — the gen_server guarantee, no locks). Reuses the
existing `patrunAddHandler` registration shape.

### `call` (core) — synchronous request → reply

```
call {request} <service> -> reply
```

Route `request` through the patrun (most-specific wins); on no match raise
the **`no_match`** error (`raise no_match …`) unless a catch-all `add {} […]`
exists; invoke the handler
with `(request, state)` and return its reply. Reuses
`execFnDefLiteral`/`CallAQL`.

### `send` (core) — asynchronous, no reply

```
send {request} <service>
```

Same dispatch, reply discarded. In-process it runs synchronously and returns
`None`; once the service is `serve`d (§4) it is true fire-and-forget delivery to
the process mailbox — the *same* `send` the actor layer uses for a `Pid`, so a
service handle and a `Pid` are interchangeable at the call site.

### `state` (core)

```
state <service> -> Store          # the service's state (rarely needed directly)
```

State is normally touched only inside handlers, via the `state` parameter.

### The `from`/reply question — settled

**Handlers are `[req state] -> reply`. There is no `from` parameter.** This is the
middle ground between OTP (which threads `From` into *every* `handle_call`) and
Seneca (which hides it entirely): the overwhelmingly common handler replies
synchronously by returning a value, so threading a reply-token everywhere is pure
ceremony. The two cases that genuinely need the caller's identity are served
without burdening the common case:

1. **Deferred reply** (the handler cannot answer yet — e.g. a proxy awaiting an
   upstream): the handler returns the sentinel **`defer`**; the reply target is
   available as the request metadata field **`req.@from`** (a `Pid`-like handle),
   and the service later does `reply req.@from value`. (`reply` lives in
   `aql:serve`, used only by deferred/proxy handlers.)
2. **Streaming reply** (the handler answers with a byte/value stream — the proxy
   case, §6): the handler returns a **stream** value; `call` yields a stream the
   caller consumes (ties to `aql:stream` / `STREAM-WORDS.0.md`).

So `[req state] -> reply` is the whole contract for normal services; `@from` and
`defer` exist for the proxy/transport minority. Settled.

## 2. Services live in modules

A reusable service is an AQL module that **exports a constructor** returning a
`Service`. Module-private `def`s are scaffolding; nothing leaks across modules
(encapsulation). No `attach`/`Init`/plugin machinery — `import` is the loader.

```aql
# counter.aql — a module that exports a service constructor
export "New" fn [[opts:Map] [Service] [
  def svc ( service {count: (opts get 'start ? 0)} )

  add {op:'inc} [ [req state] => [
      state set 'count (inc (state get 'count))   # mutate private state
      state get 'count                            # reply
  ] ] svc

  add {op:'get} [ [req state] => [ state get 'count ] ] svc
]]
```

```aql
"./counter.aql" import
def c ( Counter.New {start: 10} )
call {op:'inc} c            # → 11
call {op:'get} c            # → 11
call {op:'nope} c           # raises no_match
```

## 3. A server is a collection of services (module `aql:serve`)

```
"aql:serve" import
server [services] {restart: 'isolated} -> Server     # a supervised collection
serve <server-or-service>                            # run it
```

- **`server [svc …] {opts}`** builds a server: an ordered, named collection of
  services with a restart policy. Restart policies use plain names —
  `'isolated` (restart just the failed service; OTP `one_for_one`), `'all`
  (OTP `one_for_all`), `'cascade` (OTP `rest_for_one`) — Erlang's jargon noted
  but not surfaced.
- **`serve <server>`** runs it: each service becomes a `PROCESSES.0.md` process,
  supervised per policy; `serve` blocks until shutdown (matching the existing
  supervisor). `serve <service>` is sugar for a one-service server.

This **is** today's `serve` supervisor (`cmd/go/internal/serve`), surfaced as a
value. `aql serve registry + lsp + api` is `serve (server [Registry.New{…}
Lsp.New{…} Api.New{…}])`. Composition across services is **the server + routing/
proxying (§6)** — there is no per-service override chain (the dropped Seneca
`prior`); independent services with their own state, supervised together, is the
model.

## 4. Hosting & transport

- **In-process** (default): `call`/`send` dispatch directly; zero process
  machinery. Good for libraries and tests.
- **Served**: inside a `server`, a service runs as a supervised process; `call`
  becomes `send {call:req, @from:self}` + await reply; `send` is true async. Same
  surface, different locus — the actor `receive` loop and the service's handler
  patrun are the *same* matcher.
- **Transport** (`aql:net`): **`listen {transport} <service>`** exposes a served
  service over a wire protocol; **`connect {transport} -> Service`** returns a
  local proxy whose `call`/`send` forward to a remote service. A transport is an
  adapter translating wire frames ↔ requests: HTTP/JSON, stdio, raw TCP, and
  LSP-style framed JSON-RPC are all transports over the one service model. JSON
  envelope first (reuse `Format`/`jsonify`/`reify`); binary later (`Bytes` +
  bit-syntax, per `PROCESSES.0.md`). Transport needs the **TCP/socket server**
  AQL still lacks (only HTTP-client `fetch` exists) — later phase.

Capability gating: in-process `call`/`add` need no new capability; `serve`
(processes) is gated like `spawn`; `listen`/`connect` are gated by the
**`network`** scope (`PERMISSIONS.10.md`) — restrictive profiles get the service
DX but cannot open sockets.

## 5. Consolidating the existing CLI servers

The CLI already has the bones of this model — a `service.Service` interface
(`Name`/`Start`/`Stop`/`Status`, optional `Pausable`/`StdioUser`/`WithMetadata`)
and a supervisor in `serve`. The consolidation is to (a) make every CLI server a
**service** over a shared **transport** + **lifecycle** core, (b) make the
supervisor the **server**, (c) lift the lifecycle controls to standard request
patterns, and (d) make services *also* writable in AQL. Mapping:

| CLI server (today) | becomes | transport | notes |
| --- | --- | --- | --- |
| `repl` | service | stdio | eval handler; `pause`/`resume` = control requests |
| `registry` | service | HTTP | module-fetch handlers |
| `lsp` | service | stdio **or** TCP, JSON-RPC framing | request handlers; not pausable |
| `exec` | service | HTTP | immutable policy = service config / capability |
| `api` | service | HTTP | introspects its **server** (the supervisor) — a service that queries its own collection |
| `tui` | service | stdio | client of `api` via `connect` |
| `vault-proxy` | **proxy** (§6) | HTTP | the special one |
| `serve` supervisor | **server** | — | the collection + restart policy |

The lifecycle surface unifies onto standard requests/state:

- `Start`/`Stop` → service `serve`/shutdown (the server drives these).
- `Status()` → `call {op:'status}` (or service `state`).
- `Pausable.Pause/Resume` → control requests `call {op:'pause}` / `{op:'resume}`
  that flip a service-state flag and gate handlers — the same flag the proxy
  uses for emergency revocation (§6).
- `WithMetadata.Metadata()` → `call {op:'meta}` returning the service's map.
- `StdioUser`/transport choice → which `listen` adapter the service is exposed
  through, not an interface bolt-on.

Net effect: one lifecycle, one supervisor, one set of words — and a user can
write a registry-like or exec-like service *in AQL* and `serve` it next to the
built-in ones.

## 6. Proxies — careful thought

The vault proxy (`cmd/go/internal/vault/proxy.go`) is the case that justifies a
**first-class proxy abstraction**, because a proxy is *not* an endpoint service:
it **forwards** a request to a *target* (an upstream provider), wrapping that
forward with authorization, transformation, and accounting. Generalize it as:

```
"aql:serve" import
proxy <target> {before: [..] after: [..]} -> Service
```

A `proxy` is a `Service` whose handler, for a matched request:

1. runs **`before`** interceptors — authorize + transform the request;
2. **forwards** to `target` via `call`/`send` (local service, or a remote one via
   `aql:net connect`) — the location-transparent leg;
3. runs **`after`** interceptors — transform + account for the response;
4. returns the (possibly streamed) reply.

The vault proxy maps cleanly: `before` = capability-token auth + policy checks +
credential injection; `target` = the upstream provider URL (a `connect`ed
service); `after` = quota/cost accounting + audit. The careful points the model
must honour, each drawn from the real proxy:

- **Streaming replies, never buffered.** The proxy streams the upstream body
  straight back (`proxy.go` copies the response; secrets/bodies are never
  materialised in logs). The service model therefore must let `call` return a
  **stream** (§1 settlement, ties to `aql:stream`) — a reply is not always a
  single immutable value. This is the main reason streaming is a first-class
  reply shape.
- **Secret state is private and never travels in messages.** The proxy holds the
  unsealed session/credentials as its **private `Store` state**; replies crossing
  a process/transport boundary stay immutable and secret-free. The boundary
  immutability check (`PROCESSES.0.md` `not_sendable`) applies to forwarded
  requests/replies, *not* to the proxy's internal secret state.
- **Capability enforcement is the AQL permission model, unified.** The proxy's
  capability-token check (hashed token → alias binding → revoked/expired/method/
  budget/host policy, per `proxy_security_test.go`) is the *same* capability/
  scope machinery as `PERMISSIONS.10.md`, not a bespoke mechanism. A `before`
  interceptor consults capabilities; denial → an error reply. This is what makes
  the proxy a **trust boundary** rather than a dumb forwarder.
- **Stateful accounting.** Unlike endpoint services, a proxy tracks per-capability
  call counts and cost budgets across requests — held in service `Store` state,
  recorded in `after` *after* the response is authorized (so accounting never
  blocks the stream), exactly as today.
- **Emergency revocation = the unified pause control.** The proxy's "return 503,
  brake now" (`ProxyService.Pause`) is the §5 `call {op:'pause}` control request —
  open connections drain, new requests are rejected. Same mechanism as every
  pausable service.
- **Protocol/trust versioning.** The wire envelope carries a protocol version
  (`X-AQL-Vault-Protocol`); the transport adapter (`aql:net`) enforces fail-loud
  on mismatch. Versioning belongs to the transport layer, shared by all
  services, not re-implemented per proxy.

The existing `Proxy` (stateless per-request core) + `ProxyService` (lifecycle/
listener wrapper) split maps directly: the **interceptors + `target`** are the
proxy core, and the **service lifecycle + `listen` transport** are the wrapper —
so consolidation removes the bespoke wrapper, not the credential logic.

## 7. Lessons from BEAM's weaknesses

BEAM is the inspiration, but its model has well-known weaknesses. Below are the
generally-agreed ones, the ecosystem's responses, and the stance AQL takes — two
of them (zero-copy messaging and backpressure) are load-bearing enough to change
the design.

### 7.1 Zero-copy messaging — AQL structurally avoids BEAM's biggest tax

BEAM copies *every* message between processes. That cost is **not intrinsic to
the actor model**: it exists because each BEAM process has its **own heap and own
GC**, so a message must be copied to live in the receiver's heap (the same
per-process-heap design that gives BEAM its low pause times). AQL has no
per-process heaps — services and processes are **goroutines on one Go heap with
one GC** — and AQL values are **immutability-first** (`eng/go/clone.go`: scalars,
plain `List`, plain `Map`, type/function values are never mutated in place).

**So, inside a server, an immutable message is passed by reference — zero copy,
zero race** — both for an in-process `call` and for delivery to a `serve`d
service in another goroutine. The mailbox send supplies the happens-before edge
(Go memory model) so the receiver observes a fully-published value, and
immutability guarantees no concurrent writer. This is exactly the property
`PROCESSES.0.md` already states: AQL gets BEAM's **isolation** (a handler can
never observe a caller mutating a message mid-flight) **without** BEAM's **copy
cost** — sidestepping the per-message-copy weakness entirely. (It also gets
Erlang's refc-binary optimization for free: a future immutable `Bytes` is shared
by reference like any other immutable value.)

The one case needing care is the **mutable subset** — `Object`, `Array`, `Store`.
Sharing one across goroutines would reintroduce precisely the data race the actor
model exists to prevent. The rule (already proposed as `PROCESSES.0.md`'s
`not_sendable` check) is **refuse, not copy**: a `call`/`send` crossing a
process/transport boundary must carry only immutable values; a mutable value at
the boundary is an error. So AQL *never copies messages* — it shares immutable
ones and forbids mutable ones. A service's own state `Store` is unaffected because
it never crosses a boundary: it is owned and mutated by that one service's single
goroutine. In-process `call` is cheaper still — a direct function call in the
caller's goroutine, nothing sent or copied — so the immutability/`not_sendable`
discipline only becomes load-bearing once a service is `serve`d as a process.

### 7.2 Backpressure — `send` must not be unbounded

BEAM's worst production failure mode is the **unbounded mailbox**: async `send`
with no flow control lets a fast producer grow a slow consumer's mailbox until
OOM, and a large mailbox also makes selective `receive` O(n). The ecosystem bolts
backpressure on afterwards (synchronous `call`, GenStage/Flow/Broadway, sbroker,
jobs). AQL should take a stance up front:

- **`call` is the backpressured path** — synchronous request/reply naturally
  rate-limits the caller to the service's throughput. Prefer it.
- **`send` (async) gets a bounded mailbox** — when a `serve`d service's mailbox is
  full, `send` either blocks (applying backpressure) or fails fast with an
  overload error, configurable per service (`server [..] {mailbox: N}`). Never
  silently unbounded.
- A demand-driven streaming path (GenStage-style) is deferred to the `aql:stream`
  integration but should reuse the same bounded-mailbox mechanism.

### 7.3 The rest — mapped to AQL stances

| BEAM weakness | ecosystem response | AQL stance |
| --- | --- | --- |
| Numeric/CPU throughput | BeamAsm JIT, NIFs/Rustler, Nx | Go-hosted (compiled, real arrays/maps); hot paths are Go natives, not a VM/NIF boundary — the gap mostly doesn't arise |
| Dynamic typing / untyped protocols | Dialyzer, Gleam, Elixir set-theoretic types, Akka Typed | AQL has a static-leaning type system + `Behavior`; **a service may carry a schema on its accepted patterns**, validated at the `connect` boundary (Open Q #4) — ahead of Erlang |
| Distribution: full mesh, head-of-line blocking | Partisan, `erpc`, message fragmentation | distribution is "later"; when built, follow Partisan (overlays, parallel connections), not disterl's full mesh |
| Distribution: all-or-nothing trust | (largely unsolved in core) | **the proxy + capability model is the answer** — every cross-boundary `call` carries capability scopes; no ambient "connected = trusted" |
| Location transparency leaks | Orleans virtual actors; "make failure explicit" | keep a transparent `call` surface but make served/remote `call` **fail explicit** — timeouts and a `down`/error result, never a pretence that remote == local |
| Single `gen_server` bottleneck | pooling, sharding, scalable registries | a server is a collection of services; shard by key into many services; a router/proxy pin (Open Q #2) fronts them |
| NIF safety / scheduler blocking | dirty schedulers, Rustler | natives run on Go's preemptively-scheduled goroutines; a slow native can't stall a cooperative scheduler the way a BEAM NIF can |
| Mnesia limits | khepri, external DBs | no built-in DB ambition — persistence is an external service |
| Hot code loading complexity | rolling/blue-green deploys | not a goal; out of scope |
| Testing concurrency | QuickCheck, PropEr, Concuerror | property-based testing is already idiomatic here (`lang/spec`); systematic concurrency testing to follow |

## 8. Delivery semantics — backpressure & explicit failure

§7.1–7.2 set the *stances*; this section specifies them. Both apply only once a
service is **`serve`d** as a process (or reached remotely via **`connect`**): an
in-process `call`/`send` is a direct function call with neither a mailbox nor a
delivery step, so neither concern arises until you cross a process boundary.
(Syntax below is illustrative; error forms follow `lang/spec/error.tsv` —
`raise <code> "msg"` and `do […] error […]`.)

### 8.1 Bounded mailboxes & `send`

**Mailbox.** Every `serve`d service has a bounded mailbox. Capacity and overflow
policy are set where the service is placed in a server:

```
server [ svc ] {mailbox: 1024  overflow: 'block}
```

- `mailbox: N` — capacity in messages (default `1024`). `mailbox: 'unbounded` is
  allowed but must be written explicitly — you opt *in* to BEAM's footgun, you
  never get it by accident.
- `overflow:` — what a delivery does when the mailbox is full:
  - **`'block`** (default) — the sender blocks until space frees. Backpressure
    propagates to the producer, pacing it to the service's drain rate.
  - **`'fail`** — the delivery raises **`overload`** immediately; the caller sheds
    or reroutes.
  - **`'drop`** — the new message is discarded and the delivery returns; lossy,
    for best-effort/telemetry traffic (a `'drop_oldest` variant evicts the head).

**`send` (async)** resolves against the policy: enqueue and return `None` if there
is room; otherwise block / raise `overload` / drop. An optional bound on blocking
— `send {req} svc {within: (TimeUtil.seconds 1)}` — blocks at most that long under
`'block`, then raises `overload`. `send` to a dead service raises `down`; `send`
never raises `timeout` (no reply is awaited).

**`call` is self-limiting.** A `call` also enqueues (request + reply handle), so it
is bounded by the same mailbox — but because the caller blocks for the reply, at
most `N` calls are in flight before further callers block to enqueue. So `call`
gives backpressure *for free*; the explicit policy mainly governs `send`.

**Observability.** `call {op:'status} svc` reports `{mailbox: {depth capacity
high_water dropped}}`, so overload is measurable, not silent. (The bound governs
the inbound mailbox; the per-process save-queue used for selective `receive` in
`PROCESSES.0.md` is separate.)

**Why.** Erlang's unbounded mailbox is its classic overload/OOM failure mode and
makes selective `receive` O(n). A bound turns overload into one of three *chosen*
behaviours — pace, shed, or drop — instead of an unbounded-queue death spiral.

```aql
"aql:serve"     import
"aql:time-util" import

# A slow worker behind a bounded, backpressured mailbox.
def worker ( service {} )
add {op:'job} [ [req state] => [ heavy-work req.work  None ] ] worker

# Default 'block paces producers: the 1025th in-flight send parks the
# producer until the worker drains one — no unbounded growth.
serve ( server [ worker ] {mailbox: 1024  overflow: 'block} )

# A telemetry sink that must never block the hot path: drop on overflow.
def metrics ( service {} )
add {op:'metric} [ [req state] => [ record req  None ] ] metrics
serve ( server [ metrics ] {mailbox: 4096  overflow: 'drop} )

# A front door that sheds load rather than queueing without limit:
serve ( server [ worker ] {mailbox: 256  overflow: 'fail} )
do [ send {op:'job  work: payload} worker ]
error [ case [
    [get code eq overload/q] [ "503 busy, retry later" ]   # caller shed load
    [ raise ] ] ]                                          # re-raise anything else
```

### 8.2 Explicit failure for served / remote `call`

An **in-process** `call` has two outcomes: it returns the reply, or it propagates
whatever the handler `raise`d (an *application* error). A **`serve`d or
`connect`ed** `call` adds failure modes with no in-process analogue — the request
might never arrive, the service might be dead, the reply might never come. AQL
makes these explicit three ways.

**(1) A deadline is always in effect.** A served/remote `call` takes a timeout, and
one is *always* applied:

```
call {req} svc {timeout: (TimeUtil.seconds 5)}
```

If omitted, a default deadline applies (proposed `5s`). `timeout: 'infinity` is
permitted but must be written explicitly — you can never *accidentally* hang
forever, the way an Erlang `receive` with no `after` can.

**(2) A closed, named set of delivery errors,** distinct from application errors,
each catchable via `do … error …` and dispatchable on `code`:

| code | meaning | did the request run? |
| --- | --- | --- |
| `timeout` | no reply within the deadline | **unknown** — may have run |
| `down` | target crashed / supervisor gave up / never existed — a dead handle (ties to `PROCESSES.0.md` monitors) | **unknown** — possibly partial |
| `overload` | mailbox full under `'fail` / timed-out `'block` (§8.1) | **no** — never enqueued |
| `transport` | remote only: connection refused/reset/closed, DNS/TLS — carries the cause | pre-send **no** / post-send **unknown** |

An **application error** the handler raised (e.g. `raise bad_input "…"`) propagates
back **unchanged** — same `code`, `message`, and payload, serialized across
transport — so the caller distinguishes *"the call could not complete"* (the
delivery codes above) from *"the service ran and said no"* (the app error).

**(3) The type marks remote `call` as fallible.** The static type of a `connect`ed
(or `serve`d) service handle tags its `call` as *may-raise-delivery-error*; an
in-process `Service` handle's `call` is not so tagged. The checker can then flag a
remote `call` whose delivery failures are never handled — the "make failure
explicit" lesson enforced, lightweight checked-exception style.

**Retries & idempotency.** Because `timeout`/`down` leave the outcome *unknown*, the
runtime never silently retries them. A caller that knows a request is idempotent
opts in:

```
call {req} svc {timeout: (TimeUtil.seconds 2)  retries: 3  idempotent: true}
```

- `overload` (and pre-send `transport`) → never executed → retried automatically,
  even without `idempotent`.
- `timeout` / `down` / post-send `transport` → unknown → retried **only** when
  `idempotent: true`; otherwise the error surfaces for the caller to reconcile.

This honesty about "unknown outcome" is the whole point: the model refuses to
pretend a timed-out remote mutation definitely did — or definitely did not —
happen.

```aql
"aql:serve"     import
"aql:net"       import
"aql:time-util" import

# Reach a remote service; its `call` is statically fallible.
def billing ( connect {http: "https://billing.internal"} )

# Read path — idempotent, so auto-retry is safe; degrade on failure.
def total (
  do [ call {op:'get-total  user: uid} billing
         {timeout: (TimeUtil.seconds 2)  retries: 3  idempotent: true} ]
  error [ case [
      [get code eq timeout/q]   [ -1 ]                  # unknown → show stale
      [get code eq down/q]      [ -1 ]
      [get code eq transport/q] [ raise unavailable "billing offline" ]
      [ raise ] ] ] )                                   # app errors propagate

# Write path — NOT idempotent: never blind-retry; reconcile a timeout.
do [ call {op:'charge  user: uid  cents: 500} billing
       {timeout: (TimeUtil.seconds 5)} ]
error [ case [
    [get code eq timeout/q]  [ enqueue-reconciliation uid ]  # unknown → verify
    [get code eq down/q]     [ enqueue-reconciliation uid ]
    [get code eq overload/q] [ retry-later uid ]             # definitely not charged
    [ raise ] ] ]                                           # app errors propagate
```

## 9. Gap analysis — what AQL still lacks

- **`Service` type + `add`/`call`/`send`/`state`** and the `[req state] -> reply`
  handler contract (the patrun router is reused; invocation reuses
  `execFnDefLiteral`).
- **`server`/`serve`/restart** — depend on `PROCESSES.0.md` (processes, links,
  exit signals).
- **`proxy` + streaming replies** — needs `call` to return a stream (`aql:stream`)
  and the `before`/`after`/`target` plumbing.
- **Transport (`listen`/`connect`)** — needs the **TCP/socket server** AQL lacks,
  plus the wire envelope. JSON covered; binary needs `Bytes` + bit-syntax.
- **Capability integration for proxies** — wiring the proxy's auth to
  `PERMISSIONS.10.md` scopes.
- **Refactor of the Go servers** onto the shared service/transport/lifecycle core
  (§5) — mechanical but broad.
- **Bounded mailboxes + backpressure** for `send` (§8.1) — per-service capacity +
  `'block`/`'fail`/`'drop` overflow; depends on the `PROCESSES.0.md` mailbox.
- **Explicit-failure `call`** (§8.2) — mandatory deadline, the `timeout`/`down`/
  `overload`/`transport` delivery-error set, fallible-call typing, and
  idempotent-only retries; depends on monitors (`PROCESSES.0.md`) and transport.

## 10. Phased roadmap

- **Phase 1: in-process service.** `service`, `add`, `call`, `send`, `state`,
  `no_match` — all core. Pure request→handler over reused patrun, state in
  a `Store`. No processes.
- **Phase 2: server + supervision.** `aql:serve` `server`/`serve`/restart on the
  `PROCESSES.0.md` process layer; services-in-modules; `pause`/`status`/`meta`
  control requests; bounded mailboxes + backpressure for `send` (§8.1); the
  served `call` deadline + delivery-error set (§8.2).
- **Phase 3: transport + proxy.** `aql:net` `listen`/`connect` (HTTP/stdio/TCP/
  JSON-RPC); remote `call` failure modes + fallible-call typing + retries (§8.2);
  `proxy` with streaming replies and capability-checked interceptors; begin
  refactoring the CLI servers (§5) onto the model — vault-proxy as the proving
  ground. → the network-server goal.
- **Later: distribution.** Location-transparent `call`/`send` across nodes.

## 11. Worked example

```aql
"aql:serve" import
"aql:net"   import

# A server = a collection of services, supervised; restart each in isolation.
def app ( server [
    ( Registry.New {dir: "./mods"} )      # an AQL-written service
    ( Counter.New  {start: 0} )
  ] {restart: 'isolated} )

# A proxy in front of an upstream, with auth + accounting interceptors.
def gw ( proxy ( connect {http: "https://api.example.com"} ) {
    before: [ AuthCheck  InjectCredential ]
    after:  [ RecordUsage ]
  } )

serve ( server [ app gw ] )               # run everything (blocks)
```

## 12. Open questions

1. **`state` mutation vs purity** — the settled model mutates a `Store`; should a
   pure variant (`add` with `[req state] -> {reply state}`) also be offered for
   handlers that prefer functional state? (Leaning: mutable `Store` only, to keep
   one contract.)
2. **`proxy` matching** — does a proxy forward *all* requests to `target`, or only
   those matching a pin pattern (so one server can host several proxies)? (Leaning
   pin pattern, like the vault alias prefix.)
3. **Restart policy granularity** — server-wide policy only, or per-service
   overrides and restart-intensity limits (OTP `max_restarts`)? (Leaning
   server-wide first.)
4. **`connect` reply typing** — should a remote `connect`ed service carry a schema
   so `call` replies are typed/validated at the boundary? (Leaning optional,
   later.)
5. **Go-server refactor order** — refactor the simplest endpoint (registry) first
   to prove the shared core, then the proxy, then stdio (repl/lsp)? (Leaning yes.)
6. **Default deadline & mailbox values** (§8) — are `timeout: 5s`, `mailbox: 1024`,
   `overflow: 'block` the right defaults, and should they be per-profile
   (`PERMISSIONS.10.md`) rather than global constants? (Leaning per-profile
   defaults with per-service overrides.)
7. **Fallible-call strictness** (§8.2) — is an unhandled remote `call` a checker
   *warning* or an *error*? (Leaning warning first, to avoid friction before the
   type story is mature.)
