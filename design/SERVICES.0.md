# SERVICES

Design for a **Seneca-inspired message-service & plugin DX** in AQL — the
pattern-matched `add`/`act` programming model, with **plugins that group the
messages they handle together with the state those handlers own**.

> Naming note: "service" here is the **AQL message-service value** (a Seneca-
> style instance), *not* the CLI `Service` interface in `cmd/go/internal/service`
> (the OS-level supervisor for `repl`/`registry`/`lsp`). They are unrelated.

## Context

The end goal is efficient, safe network servers/clients. `PROCESSES.0.md`
specifies the low-level **actor substrate** (processes, mailboxes, `send`/
`receive`, selective receive via patrun). This document specifies the **primary
developer experience** layered on top, modelled on **Seneca**
(senecajs.org): you describe a system as **message patterns mapped to actions**,
grouped into **plugins**, and you invoke behaviour by **sending a message and
getting a reply** — never by naming a function or a host. Local and remote calls
look identical (Seneca's transport transparency), which is what makes
message-first systems easy to split across processes and machines later.

This pairing mirrors Erlang: `PROCESSES.0.md` is the raw process/mailbox layer
(like Erlang processes), and this document is the high-level behaviour
(like OTP `gen_server` + a pattern router) that most code is actually written
against.

### Why this is mostly already in AQL

Seneca's matching engine is **patrun**, and patrun is the *same library*
(`github.com/rjrodger/patrun`) already vendored and exposed as **core words** in
`lang/go/native/native_patrun.go`. The Seneca substrate is therefore already
present:

- `add {pattern} value patrun` — register a rule. This is literally
  `seneca.add(pattern, action)`. (The core `add` word already carries a Patrun
  overload alongside math `add`, disambiguated by the receiver type.)
- `find {subject} patrun` — most-specific match wins, unknown subject keys
  ignored. This is exactly Seneca's routing: `act {role:'math cmd:'sum left:1
  right:2}` matches a rule added for `{role:'math cmd:'sum}` and the action reads
  the extra keys off the message.

What is missing is small and specific:

1. a **service** value that bundles a patrun **with the state its actions own**
   and that *invokes* the matched action (`act` = `find` + invoke + reply);
2. an **override chain** so two plugins adding the same pattern compose
   (Seneca's `prior`); raw patrun currently overwrites a same-signature rule;
3. a **plugin** model that groups `add`s + state — answered here by **reusing the
   module system**;
4. **hosting + transport** so a service can run as an isolated actor and `act`
   can cross process/network boundaries.

This is a **design RFC only — no implementation code yet**.

### Scope decisions (agreed)

1. **Service model = both (hosted on demand).** A service is a lightweight
   in-process value by default — `act` is synchronous `find`+invoke in the
   caller's process. A service can be **hosted** inside a process
   (`PROCESSES.0.md` actor) on demand to gain isolation and to become reachable
   over transport; once hosted, `act` becomes `send`+await-reply, transparently.
2. **Packaging = hybrid.** The core message-DX — the `Service` type, `service`,
   `add` (service overload), `act`, and `prior` — lives in **core words**
   alongside the existing patrun words. **Plugin packaging (`use`) and transport
   (`host`/`listen`/`client`) live in a module, `aql:mesh`.**
3. **Plugins = modules.** A plugin is an **AQL module** whose exports are
   patterns+actions and whose **module-private `def`s are its state**. `use`
   loads such a module (via the existing `import` machinery) and applies its
   registration to a service.

## 1. The core model

### The `Service` value (core)

A new Ideal type **`Service`** (precedent: `Ideal/Patrun`, `Timeout`,
`Interval`; FixedID in the 5000-band per `native_patrun.go`). It wraps:

- a **patrun router** — reuse `patrunMatcher` from `native_patrun.go` (the trie +
  side table), so all the matching, specificity, and `patterns` introspection
  come for free;
- a small **override chain** per exact pattern signature (for `prior`, §2);
- a reference to its **host** when hosted (`None` while in-process; a `Pid` once
  hosted — §4).

State owned by actions is *not* a field on the Service — it lives in the action
**closures** (§3), exactly as in Seneca. `service -> Service` constructs an empty
one.

### `add` (core, extends the existing patrun overload)

```
add {pattern} [action] <service>
```

Registers `action` (an AQL function value) for `pattern`. Reuses the existing
`patrunAddHandler` shape; the only change is a `Service` receiver overload that
also pushes onto the override chain instead of plain-overwriting (§2). `pattern`
values must be scalars (already enforced by `coercePattern`).

### `act` (core)

```
act {message} <service> -> reply
```

1. Route `message` through the service's patrun (`find`, most-specific wins).
2. If no rule matches: raise `[aql/no_action]` (Seneca's "act not found"),
   or run a registered catch-all (`add {} […]`).
3. Invoke the matched action with the message bound, returning its reply.
   Reuse the existing invocation path (`execFnDefLiteral`/`CallAQL`) — the action
   is just a stored function value.

**Action signature.** An action is `[msg:Map srv:Service] -> reply` (the service
is passed so an action can `act` nested messages and call `prior`, like Seneca's
`this`). A one-param `[msg:Map]` action is also accepted (service omitted). The
reply is any value (typically a `Map`). Replies, like all cross-process messages,
must be **immutable** when the service is hosted (§4, inheriting the
`PROCESSES.0.md` send rule); in-process `act` has no such restriction.

### Pattern dispatch is patrun, unchanged

Specificity (more matched keys win), unknown-subject-key tolerance, and
`patterns <service>` introspection are inherited directly from the patrun core.
This is the whole point of AQL having bundled patrun.

## 2. Plugin composition: the override chain (`prior`)

Seneca's superpower is that plugins **layer**: two plugins can `add` the same
pattern, and the later action can call `prior` to invoke the earlier one
(middleware / decoration / overrides). Raw patrun overwrites a same-signature
rule (`native_patrun.go` `m.side[sig] = …`), so the service layer keeps a small
**stack of actions per exact pattern signature**. `add` pushes; the trie always
points at the top; **`prior {message} <service>`** (core) invokes the
next-older action in the chain (or returns `None` at the bottom). This makes
`use`-ing two plugins that touch the same pattern compose predictably.

## 3. Plugins are modules — grouping messages **and** state

This is the headline. AQL already has a module system with **private bindings**
and **capitalised exports** (`lang/go/CLAUDE.md` "Module / ModuleExport
instances"). A **plugin is just a module** used this way:

- **The module boundary is the plugin boundary.** Everything the plugin needs is
  one module.
- **Its exports are the messages it handles.** The module exports a single
  registration entry (convention: `export "Register" fn [[srv:Service
  opts:Map] [Service] [ … ]]`) that performs the plugin's `add`s.
- **Its module-private `def`s are the plugin's state.** Because AQL fns use
  **implicit lexical capture** (`lang/go/CLAUDE.md` "Closures and Capture"), each
  action closes over the module-private bindings it references. Module privacy
  guarantees one plugin cannot see another's state — true Seneca-style
  encapsulation. Mutable state uses a pointer-backed `Store` (capture shares the
  pointer, so updates persist across calls), which is safe because a service
  processes messages **single-threaded** (in-process: the caller's goroutine;
  hosted: the actor's one goroutine — §4) — the gen_server state model, no locks.

A plugin module (illustrative):

```aql
# math-plugin.aql  — a module = a plugin
def calls (store {n: 0})                 # plugin-private STATE

export "Register" fn [[srv:Service opts:Map] [Service] [
  add {role:'math cmd:'sum} [ [msg srv] => [
      calls set 'n (inc (calls get 'n)) drop      # mutate private state
      msg.left add msg.right                       # reply
  ] ] srv

  add {role:'math cmd:'product} [ [msg srv] => [
      msg.left mul msg.right
  ] ] srv
]]
```

### `use` (module: `aql:mesh`)

```
"aql:mesh" import
Mesh.use <module-or-path> {options} <service> -> service
```

`use` imports the plugin module (existing `import`/`Resolve`/`loadFileModule`
path), looks up its `Register` export, and calls it with the service + options;
the returned service carries the plugin's patterns (whose actions hold the
plugin's captured state). Multiple `use`s compose via the override chain (§2).
Putting `use` in `aql:mesh` (not core) honours the hybrid packaging decision and
keeps plugin-loading conventions together with transport.

> Alternative considered: a plugin module exporting a **declarative** list of
> `{pattern action}` entries instead of an imperative `Register` fn. The
> imperative form is chosen because it lets a plugin run `opts`-dependent
> registration and initialise state in one place (Seneca's `init`), while still
> being "just a module". A declarative manifest could be added later as sugar.

## 4. Hosting on demand & transport (module: `aql:mesh`)

By default a service is in-process and `act` is a direct call. To gain isolation
or remote reach, **host** it as an actor:

- **`Mesh.host <service> -> Service`** — spawn a `PROCESSES.0.md` process whose
  `receive` loop *is* the service's patrun dispatch; return a service handle
  bound to that `Pid`. Now `act <msg> <hosted-service>` transparently becomes
  `send {act: msg, reply: self}` + `receive` of the reply — same surface, same
  result, different locus. This is the in-process↔actor unification: the actor's
  selective `receive` and the service's patrun are the *same* matcher, so hosting
  is almost free conceptually.
- **`Mesh.listen {transport opts}` / `Mesh.client {pin, transport opts}`** —
  bridge `act` across the network: a `client` pins a pattern subset and forwards
  matching `act`s over a transport to a remote `listen`ing host, which routes
  them into its service and returns the reply. JSON is the default envelope
  (reuse the `Format`/`jsonify`/`reify` machinery); binary envelopes come with
  the `Bytes`/bit-syntax work (`PROCESSES.0.md` gap analysis). This is the
  Seneca-transport analogue and the concrete realisation of the
  "efficient/safe JSON & binary network server" goal: a `listen`er is an
  acceptor that spawns a hosted service interaction per connection.

Transport requires the **TCP/socket server primitive** AQL still lacks (only the
HTTP *client* `fetch` exists) — see the gap analysis. Hence transport is later
phase; in-process `service`/`add`/`act` and module-plugins via `use` come first.

## 5. Packaging (hybrid)

| Word | Where | Notes |
| --- | --- | --- |
| `service` (constructor), `Service` type | **core** | wraps a reused `patrunMatcher` + override chain |
| `add {pat} [action] <service>` | **core** | service overload of the existing patrun `add` |
| `act {msg} <service>` | **core** | `find` + invoke + reply; `[aql/no_action]` on miss |
| `prior {msg} <service>` | **core** | next-older action in the override chain |
| `patterns <service>` | **core** | inherited from patrun introspection |
| `Mesh.use <module> {opts} <service>` | **`aql:mesh`** | plugin loading via the module system |
| `Mesh.host <service>` | **`aql:mesh`** | run a service as a `PROCESSES.0.md` actor |
| `Mesh.listen` / `Mesh.client` | **`aql:mesh`** | network transport (later phase) |

`aql:mesh` is a framework module (plain name, like `aql:net`/`aql:io`, per the
`-util` naming rule in `lang/go/CLAUDE.md`).

## 6. Safety & capability integration

- In-process `act`/`add`/`use` need no new capability beyond the ability to load
  modules (`use` is subject to the existing import/file-access policy).
- `Mesh.host` is gated behind the same `process`/`concurrency` capability as
  `spawn` (`PROCESSES.0.md` §7).
- `Mesh.listen`/`client` are gated behind the **`network`** capability scope
  (`PERMISSIONS.10.md`), which is where the planned `fetch`-family network words
  also slot. Restrictive profiles (`sandbox`, `compute`, `read-only`) thus get
  the message-DX but cannot open sockets — the "safe server" guarantee.

## 7. Gap analysis — what AQL still lacks

- **Service type + `act`/`prior`** and the **override chain** (this RFC adds
  them; the patrun router itself is reused).
- **Action invocation plumbing** — minor; reuses `execFnDefLiteral`/`CallAQL`.
- **`use` plugin-loader** — reuses the module system; needs the `Register`
  export convention.
- **Hosting** — depends on `PROCESSES.0.md` (processes, `send`/`receive`).
- **Transport (`listen`/`client`)** — needs the **TCP/socket server** AQL does
  not have (only HTTP *client* `fetch`), plus a wire envelope. **JSON is already
  covered** (`Format`/`jsonify`/`reify`); **binary needs the `Bytes` type +
  bit-syntax** (`PROCESSES.0.md` gap analysis).
- **Message metadata** — Seneca attaches `$meta` (correlation id, trace, caller
  pattern) for tracing and transport correlation. Out of scope for the first
  cut; noted for when transport lands.

## 8. Phased roadmap

- **Phase 1: in-process message-DX.** `Service`, `service`, `add` (service
  overload), `act`, `prior`, `patterns` — all core. Pure pattern→action with
  reused patrun. No processes required.
- **Phase 2: plugins as modules.** `aql:mesh` `use` loader + the `Register`
  export convention; plugin composition via the override chain; `Mesh.host` to
  run a service as a `PROCESSES.0.md` actor (depends on phase 1 of
  `PROCESSES.0.md`).
- **Phase 3: transport.** `Mesh.listen`/`client` over TCP/HTTP (depends on the
  TCP-server primitive); JSON envelope first, binary envelope with the `Bytes`/
  bit-syntax work. Message `$meta`/correlation. → the network-server goal.
- **Later: distribution.** Location-transparent `act` across nodes — Seneca's
  mesh, built on hosted services + transport.

## 9. Worked example

```aql
"aql:mesh" import

# Build a service and load the math plugin (a module) into it.
def svc ( Mesh.use "./math-plugin.aql" {} (service) )

act {role:'math cmd:'sum     left:1 right:2} svc      # → 3
act {role:'math cmd:'product left:3 right:4} svc      # → 12
act {role:'math cmd:'nope} svc                        # raises [aql/no_action]

# A second plugin overrides 'sum to add audit logging, then calls prior.
# audit-plugin.aql's action:
#   add {role:'math cmd:'sum} [ [msg srv] => [
#       "summing" log drop
#       prior msg srv                 # delegate to the math plugin's sum
#   ] ] srv
def svc2 ( Mesh.use "./audit-plugin.aql" {} svc )
act {role:'math cmd:'sum left:5 right:6} svc2          # logs, then → 11

# Host it as an isolated actor; act is now message-passing under the hood,
# but the call site is unchanged.
def hosted ( Mesh.host svc2 )
act {role:'math cmd:'sum left:7 right:8} hosted        # → 15 (via send/receive)
```

Conventions in play: **messages are tagged maps** (the `role`/`cmd` keys drive
patrun, extra keys are payload); **plugins are modules** whose exports register
patterns and whose private `def`s are state; **`prior`** composes overlapping
plugins; and **hosting** changes the execution locus without changing the `act`
surface.

## 10. Open questions

1. **Action arity** — pass the service as a 2nd param (`[msg srv]`), or make the
   "current service" reachable implicitly (a `self`-like word) so actions stay
   `[msg]`? (Leaning explicit param for clarity.)
2. **`act` reply shape** — return the raw action result, or always wrap as a
   map/result envelope? (Leaning raw; envelope only on transport.)
3. **`Register` export name** — fixed convention (`Register`), or configurable in
   `use {entry: …}`? (Leaning fixed with an override option.)
4. **`host` placement** — it composes two core features (service + `spawn`); keep
   it in `aql:mesh` with transport (chosen), or promote to core? (Chosen:
   module, to keep all hosting/transport conventions together.)
5. **Pattern-add immutability** — should `act` deep-validate that a *hosted*
   service's replies/messages are immutable at the boundary, reusing the
   `PROCESSES.0.md` `not_sendable` check? (Leaning yes, at the host boundary
   only.)
