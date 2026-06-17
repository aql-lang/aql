# BEHAVIOURS

Design for a **BEAM/OTP-inspired generic-server & supervision DX** in AQL — the
`gen_server` programming model (`call`/`cast`/`handle` with threaded state),
**callback modules** as the unit of packaging, and **supervision trees** as the
unit of composition.

> Erlang spelling "behaviour" is used throughout (Erlang's `-behaviour`
> attribute; `-behavior` is the accepted alias). A *behaviour* is the abstract
> pattern (gen_server, supervisor); a **server** is a concrete instance.

> Naming note: the AQL **`Server`** value here is a gen_server instance — *not*
> the CLI `Service` interface in `cmd/go/internal/service` (the OS-level
> supervisor for `repl`/`registry`/`lsp`). Unrelated.

## Context

The end goal is efficient, safe network servers/clients. `PROCESSES.0.md`
specifies the low-level **actor substrate** (processes, mailboxes, `send`/
`receive`, selective receive via patrun) — the raw Erlang-process layer. This
document specifies the **primary developer experience** layered on top, modelled
on **OTP**: most code is not written against raw `send`/`receive` but against
**`gen_server`** — a process that owns **state** and answers synchronous **calls**
and asynchronous **casts** via pattern-matched **handlers** — composed into
**supervision trees**. That is exactly how real Erlang systems are built, and it
is the right altitude for the network-server goal.

> **This supersedes the earlier Seneca framing.** An initial draft modelled this
> on Seneca (`act`/`add`/`plugin`/`prior`/`mesh`). On reflection the BEAM
> vocabulary fits the project (and this branch) far better, and is more honest
> about composition: OTP composes **servers under a supervisor**, not Seneca's
> single-instance plugin-override chain. The rename:
>
> | Concept | Seneca (dropped) | BEAM/OTP (this doc) |
> | --- | --- | --- |
> | the instance/value | `service` / `Service` | **`server`** / **`Server`** (a gen_server) |
> | register a handler | `add` | **`handle`** |
> | synchronous request → reply | `act` | **`call`** (`gen_server:call`) |
> | asynchronous, no reply | *(none)* | **`cast`** (`gen_server:cast`) |
> | the handler function | `action` | **handler** (`handle_call`/`handle_cast`) |
> | handler state | closure-captured `Store` | **threaded state** `[req state] -> {reply newstate}` |
> | the packaging unit | `plugin` | **callback module** (= an AQL module) |
> | plugin setup export | `Register` | **`Init`** (`init/1`) |
> | run as a process | `host` | **`start`** (`start_link`) |
> | compose units | `prior` override chain | **supervision tree** (`supervise`) |
> | no-match error | `[aql/no_action]` | **`[aql/no_clause]`** (`function_clause`) |
> | transport in / out | `listen` / `client` | **`listen`** / **`connect`** (`gen_tcp`) |
> | framework module | `aql:mesh` | **`aql:otp`** |

### Why this is mostly already in AQL

A gen_server is, at heart, **pattern-matching on messages + threaded state**, and
the matcher is already a **core word set**: patrun
(`lang/go/native/native_patrun.go`).

- `add {pattern} value patrun` already does most-specific-match registration;
  the `handle` word is a `Server`-receiver flavour of it.
- `find {subject} patrun` already does Seneca/OTP-style routing: a message
  `{op:'inc n:1}` matches a handler registered for `{op:'inc}`, extra keys are
  payload. This is the *same* matcher `receive` uses for selective receive in
  `PROCESSES.0.md` — "pattern-match a message" means one thing everywhere.

What is missing is small and specific:

1. a **`Server`** value bundling a patrun of handlers **with threaded state**, and
   the `call`/`cast` words that route a request, invoke the handler, thread the
   new state, and (for `call`) return a reply;
2. **callback modules** — answered by reusing the module system (§2);
3. **process hosting + supervision + transport** (§4) — built on `PROCESSES.0.md`.

This is a **design RFC only — no implementation code yet.**

### Scope decisions (carried over, re-expressed in OTP terms)

1. **Both — hosted on demand.** A `Server` is a lightweight in-process value by
   default (cheap, no scheduler): `call` is direct dispatch in the caller's
   goroutine; state is held in the (mutable, like `Patrun`) server handle. On
   demand, **`start`** promotes it to a real `PROCESSES.0.md` process
   (`start_link`); `call`/`cast` then route via `send`/`receive`, transparently.
2. **Hybrid packaging.** The in-process gen_server interface — `Server`,
   `server`, `handle`, `call`, `cast`, `state` — is **core** (next to the patrun
   words). **Process hosting, packaging, supervision, and transport** —
   `start`, `attach`, `supervise`, `listen`, `connect` — live in module
   **`aql:otp`**.
3. **Callback modules = plugins.** A behaviour's implementation is an **AQL
   module**: its module-private `def`s are scaffolding, its exported `Init`
   (`init/1`) sets the initial state and registers handlers, and its handlers are
   the messages it serves.

## 1. The core model — gen_server

### The `Server` value (core)

A new Ideal type **`Server`** (precedent: `Ideal/Patrun`; FixedID in the
5000-band per `native_patrun.go`). It wraps:

- a **patrun** of handlers — reuse `patrunMatcher` from `native_patrun.go`, so
  matching/specificity/`patterns` introspection come for free;
- the server's current **state** value (default `{}`), held mutably in the handle
  — consistent with `Patrun` already being a mutable reference type (`add`
  mutates in place); single-threaded access makes this safe (§3);
- a **host** reference: `None` while in-process, a `Pid` once `start`ed (§4).

`server {state} -> Server` constructs one (state optional, default `{}`).

### `handle` (core) — register a handler clause

```
handle {pattern} [handler] <server> -> Server
```

`handler` is `[req:Map state] -> {reply: <value>  state: <newstate>}` — the
`handle_call/3` shape (`{reply, Reply, NewState}`), state threaded functionally.
A cast handler may return just `{state: <newstate>}` (no reply,
`handle_cast/2`). Reuses the existing `patrunAddHandler` registration shape.

### `call` (core) — synchronous request → reply

```
call {request} <server> -> reply        # gen_server:call
```

1. Route `request` through the server's patrun (most-specific wins).
2. No match → raise **`[aql/no_clause]`** (Erlang `function_clause`), unless a
   catch-all `handle {} […]` is registered.
3. Invoke the handler with `(request, state)`, **store the returned `state`**
   back on the server handle, and return `reply`. Reuse the existing invocation
   path (`execFnDefLiteral`/`CallAQL`).

### `cast` (core) — asynchronous, no reply

```
cast {request} <server>                 # gen_server:cast
```

In-process this runs the handler and updates state synchronously, returning
`None`. Once the server is `start`ed (§4), `cast` is a true fire-and-forget
`send` (no await). Same surface, different locus.

### `state` (core) — initial / current state

```
state {value} <server> -> Server        # set initial state (used inside Init)
state <server> -> value                 # read current state
```

### Dispatch is patrun, unchanged

Specificity, unknown-key tolerance, and `patterns <server>` introspection are
inherited from the patrun core — the same matcher `receive` uses in
`PROCESSES.0.md`. Messages that match no `call`/`cast` handler but arrive at a
`start`ed process (e.g. a raw `send`) are **info** messages (`handle_info/2`);
a server may register `handle {info: …}` for them.

## 2. Callback modules are plugins — grouping messages **and** state

OTP's unit of behaviour is a **callback module**, and AQL already has modules
with **private bindings** and **capitalised exports** (`lang/go/CLAUDE.md`). A
plugin *is* a module used this way:

- **The module is the callback module.** Its module-private `def`s are
  scaffolding; nothing leaks across modules (encapsulation).
- **It exports `Init`** — the `init/1` callback — `[srv:Server args:Map] ->
  Server`, which sets initial `state` and registers the `handle` clauses. Because
  AQL fns capture lexically (`lang/go/CLAUDE.md` "Closures and Capture"), handlers
  may also close over module-private helpers.
- **Its handlers are the messages it serves** — the patterns it `handle`s.

State is **threaded through handlers** (`[req state] -> {reply newstate}`), the
OTP model — not Seneca's closure-mutated `Store`. This is more in the spirit of
both AQL (immutable values; new state each call) and BEAM (functional state
threading), and removes the "single-threaded so in-place mutation is safe"
caveat from the handler contract (the only mutable thing is the `Server` handle's
current-state slot, updated by `call`/`cast` between invocations).

A callback module (illustrative):

```aql
# counter.aql — a callback module (gen_server)
export "Init" fn [[srv:Server args:Map] [Server] [
  state {count: (args get 'start ? 0)} srv          # init/1 initial state

  handle {op:'inc} [ [req st] => [
      def n (inc (st get 'count))
      {reply: n  state: (st set 'count n)}          # {reply, Reply, NewState}
  ] ] srv

  handle {op:'get} [ [req st] => [
      {reply: (st get 'count)  state: st}
  ] ] srv
]]
```

## 3. Composition is a supervision tree (not an override chain)

This is where BEAM diverges sharply — and better — from Seneca. Seneca composes
plugins inside one instance via a `prior` override chain. **OTP composes
*separate servers* under a supervisor.** Each callback module is its own server
(its own state, its own handlers); you arrange them in a **supervision tree** and
route requests to the right one. That is the BEAM-native answer to "how messages
and state are grouped": **one module = one server = one isolated unit of
state+messages**, and the tree gives you fault isolation and restart strategies
for free. (Within a single server, handlers are just patrun clauses; a `next`
word for invoking a shadowed handler is possible but deferred — see Open
Questions — because multi-server + supervisor is the idiomatic composition.)

## 4. Running as processes, supervision & transport (module `aql:otp`)

```
"aql:otp" import
```

- **`OTP.attach <module> {args} <server> -> Server`** — load a callback module:
  run its `Init` against the server (initial state + handlers). The in-process
  way to assemble a server from a module. (Reuses `import`/`Resolve`/
  `loadFileModule`.)
- **`OTP.start <server-or-module> {opts} -> Pid`** — `start_link`: spawn a
  `PROCESSES.0.md` process whose `receive` loop **is** the server's patrun
  dispatch, holding state in the process. Returns a `Pid` that `call`/`cast`
  accept transparently — `call pid` becomes `send {call: req, from: self}` +
  `receive` reply. The actor's selective `receive` and the server's handler
  patrun are the *same* matcher, so hosting is conceptually free.
- **`OTP.supervise {strategy: 'one_for_one} [child-specs] -> Pid`** — a
  supervisor process: starts children, restarts them per strategy
  (`one_for_one`/`one_for_all`/`rest_for_one`), the OTP fault-tolerance core.
  This is the composition primitive (§3).
- **`OTP.register name <pid>` / a router server** — name/pattern registration so
  callers can `call` by name across servers (Erlang `register/2` / a process
  registry), the optional "mesh" convenience over a supervision tree.
- **`OTP.listen {opts}` / `OTP.connect {opts}`** — bridge `call`/`cast` across
  the network (`gen_tcp:listen`/`connect`): an acceptor spawns a server
  interaction per connection; `connect` forwards `call`s to a remote node. JSON
  envelope first (reuse `Format`/`jsonify`/`reify`); binary with the `Bytes`/
  bit-syntax work (`PROCESSES.0.md` gap analysis). → the network-server goal, and
  the basis for **distribution** (location-transparent `call` across nodes).

Transport needs the **TCP/socket server primitive** AQL still lacks (only the
HTTP *client* `fetch` exists) — hence later phase; in-process gen_server +
callback modules come first.

## 5. Packaging (hybrid)

| Word | Where | OTP analogue |
| --- | --- | --- |
| `server` / `Server` | **core** | a gen_server value |
| `handle {pat} [h] <server>` | **core** | `handle_call`/`handle_cast` clause |
| `call {req} <server>` | **core** | `gen_server:call` |
| `cast {req} <server>` | **core** | `gen_server:cast` |
| `state … <server>` | **core** | `init` state / `State` |
| `patterns <server>` | **core** | (introspection, from patrun) |
| `OTP.attach <module> {args} <server>` | **`aql:otp`** | load a callback module |
| `OTP.start <server\|module> {opts}` | **`aql:otp`** | `gen_server:start_link` |
| `OTP.supervise {strategy} [children]` | **`aql:otp`** | `supervisor` |
| `OTP.register` / router | **`aql:otp`** | `register/2` / process registry |
| `OTP.listen` / `OTP.connect` | **`aql:otp`** | `gen_tcp:listen`/`connect`, dist |

`aql:otp` is a framework module (plain name, per the `-util` naming rule in
`lang/go/CLAUDE.md`).

## 6. Safety & capability integration

- In-process `call`/`cast`/`handle`/`attach` need no new capability beyond
  module loading (`attach` obeys the existing import/file-access policy).
- `OTP.start`/`OTP.supervise` are gated behind the `process`/`concurrency`
  capability as `spawn` (`PROCESSES.0.md` §7).
- `OTP.listen`/`OTP.connect` are gated behind the **`network`** capability scope
  (`PERMISSIONS.10.md`). Restrictive profiles (`sandbox`, `compute`,
  `read-only`) thus get the gen_server DX but cannot open sockets — the "safe
  server" guarantee.

## 7. Gap analysis — what AQL still lacks

- **`Server` type + `call`/`cast`/`state`** and the threaded-state handler
  contract (this RFC adds them; the patrun router itself is reused).
- **Handler invocation plumbing** — minor; reuses `execFnDefLiteral`/`CallAQL`.
- **`attach` callback-module loader** — reuses the module system; needs the
  `Init` export convention.
- **`start`/`supervise`** — depend on `PROCESSES.0.md` (processes, links, exit
  signals for restarts).
- **Transport (`listen`/`connect`)** — needs the **TCP/socket server** AQL does
  not have, plus a wire envelope. **JSON covered**; **binary needs `Bytes` +
  bit-syntax** (`PROCESSES.0.md`).
- **Message metadata** — a correlation/trace tag (Erlang's monitor ref / a `from`
  tag) for matching replies and tracing across transport. Minimal `from` needed
  for `call`-over-process; richer tracing later.

## 8. Phased roadmap

- **Phase 1: in-process gen_server.** `Server`, `server`, `handle`, `call`,
  `cast`, `state`, `patterns` — all core. Pure pattern→handler with reused
  patrun and threaded state. No processes required.
- **Phase 2: callback modules + processes.** `aql:otp` `attach` loader + the
  `Init` convention; `OTP.start` (`start_link`) to run a server as a
  `PROCESSES.0.md` process; `OTP.supervise` for supervision trees (depends on
  links/exit signals — `PROCESSES.0.md` phase 2).
- **Phase 3: transport.** `OTP.listen`/`connect` over TCP/HTTP; JSON envelope,
  then binary with `Bytes`/bit-syntax; `from`/correlation metadata. → the
  network-server goal.
- **Later: distribution.** Location-transparent `call`/`cast` across nodes —
  Erlang distribution, built on hosted servers + transport.

## 9. Worked example

```aql
"aql:otp" import

# In-process: assemble a server from the callback module, call it directly.
def c ( OTP.attach "./counter.aql" {start: 10} (server) )

call {op:'inc} c          # → 11   (state threaded; handle's current state now 11)
call {op:'inc} c          # → 12
call {op:'get} c          # → 12
call {op:'nope} c         # raises [aql/no_clause]

# Promote to a linked gen_server process; the call site is unchanged.
def pid ( OTP.start c {} )         # start_link
call {op:'inc} pid                 # → 13  (now via send / receive)
cast {op:'inc} pid                 # async, no reply (fire-and-forget)

# Composition = a supervision tree (NOT an override chain): two independent
# servers, each its own callback module + state, restarted one-for-one.
OTP.supervise {strategy: 'one_for_one} [
  {id: 'counter  start: ["./counter.aql" {start: 0}]}
  {id: 'math     start: ["./math.aql"    {}]}
]
```

Conventions: **messages are tagged maps** (`op`/`role` keys drive patrun, extra
keys are payload); **a callback module is one server** with its own threaded
state; **`call`/`cast`** are the synchronous/asynchronous request words; **`start`**
turns a server value into a process without changing the call surface; and
**`supervise`** composes servers into a fault-tolerant tree.

## 10. Open questions

1. **Handler `from`** — expose the reply-to tag to handlers (`[req from state]`,
   enabling deferred/`noreply` replies) or keep the simple `[req state]` shape?
   (Leaning simple now, add `from` with transport.)
2. **In-server handler layering (`next`)** — provide a word to invoke a shadowed
   handler within one server (a thin Seneca-`prior` analogue), or insist all
   composition go through multiple servers + `supervise`? (Leaning the latter as
   idiomatic; `next` only if a concrete need appears.)
3. **`Server` mutability** — keep the mutable-handle model (consistent with
   `Patrun`, ergonomic) or make `call` return `{reply server}` for full value
   purity? (Leaning mutable handle in-process; state is owned by the process once
   `start`ed anyway.)
4. **`Init` export name** — fixed `Init`, or configurable via `attach {init: …}`?
   (Leaning fixed `Init` with an override option.)
5. **State at the process boundary** — should `call`/`cast` to a `start`ed server
   enforce request/reply immutability via the `PROCESSES.0.md` `not_sendable`
   check? (Leaning yes, at the boundary only; in-process is unrestricted.)
