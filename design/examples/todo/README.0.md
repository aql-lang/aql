# Multi-user Todo — a DX probe for the Services/Processes design

This folder is a **developer-experience experiment**, not runnable code. It
writes a realistic multi-user todo application entirely against the proposed
`../SERVICES.0.md` and `../PROCESSES.0.md` surfaces — words that **do not exist
yet** — to answer two questions the RFCs cannot answer on their own:

1. **Does the architecture express a real app cleanly?**
2. **Which of the open review gaps (A–F, see the review in the branch history)
   actually bite when you build with it?**

Everything compiles only in the imagination. Collection-word spellings
(`get`/`set`/`del`/`vals`/`append`) and a few helpers (`now`, `none?`,
`Rand.uuid`) are illustrative; the real ones would be confirmed against
`aql describe`. The value is the *shape* of the code, not its execution.

> The `.0` suffix follows the repo convention: design-only, implementation
> completeness 0 (see `../IMPLEMENTATION-STATUS.10.md`).

---

## The app

A todo backend that several users hit concurrently, with live push when a
user's list changes on another device. It is deliberately small but touches
every layer of the design:

| File | Layer | What it demonstrates |
| --- | --- | --- |
| `todo.aql` | **service** | owned `Store` state, `add`/`call`/`send`, scalar-tag routing, `no_match`, event emission |
| `auth.aql` | **prior + wrap** | cross-cutting auth with zero handler edits; blanket gate (`wrap`) + per-pattern owner guard (`prior`) |
| `audit.aql` | **wrap** | ambient logging around the whole dispatch; observing app errors without swallowing them |
| `sessions.aql` | **service + process** | identity (service) + presence (one `spawn`ed process per user, `register`/`whereis`) |
| `presence.aql` | **process** | raw `spawn`/`receive` loop, two-layer clause matching, `after`, `self`, correlation tags |
| `notify.aql` | **service** | bounded lossy sink (`send`, `overflow: 'drop`), deferred reply (`defer` + `reply req.@from`) |
| `app.aql` | **server + transport** | `server`/`serve` with restart policy, `pool` hash-sharding, `listen` |
| `client.aql` | **connect** | the uniform failure contract: idempotent retry vs. reconcile, correlated push |

### Request lifecycle (a `create`)

```
client.aql                app.aql graph                         process layer
----------                -------------                         -------------
connect {http} ──call──▶  listen(/todo) ─▶ Audit.log
                                          └▶ Auth.gate ──whoami──▶ sessions svc
                                             └▶ own-guard (skipped: not clear/remove)
                                                └▶ pool ('hash 'user) ─▶ TodoStore #k
                                                   └▶ patrun {op:'create} ─▶ handler
                                                      ├─ mutate state.users[alice]
                                                      └─ send {tag:'created} ─▶ Notify svc
                                                                                 └─ whereis presence:alice
                                                                                    └─ send {op:'event} ─▶ presence proc
                                                                                                            └─ fan out to client sinks
```

One write traverses transport → two `wrap`/`prior` layers → a sharded pool →
a service handler → a fire-and-forget event → a registered process → live
subscribers. The same `call` would work in-process with none of the transport
or process machinery — that uniformity is the design's core bet (§8), and it
held up here.

---

## DX findings, mapped to the review gaps

The standalone review flagged gaps **A–F**. Building the app is the test of
whether the RFC fixes for them are ergonomic. Honest verdicts:

### Gap A — `receive` matcher/binding split (the headline)
**Verdict: the split reads cleanly, but the two-context rule must be taught.**
The revised RFC says: patrun routes on **scalar tags**, `ParseFnParams` types
and binds `name:Type` slots. In `presence.aql` this is pleasant —
`{cmd: 'ping reply: Pid}` obviously means "route on `cmd:'ping`, bind a typed
`reply`." But note the asymmetry the app exposes: **services** (`todo.aql`)
*cannot* use binding slots in `add` patterns — they route on scalar tags and
destructure off `req` by hand (`req.text`, `req.user`), whereas **processes**
(`presence.aql`) get the `name:Type` sugar in `receive`. A newcomer will try
`add {op:'create text:String}` and be surprised `text` is not bound. The RFC
should state plainly: *binding slots are a `receive` feature, not an `add`
feature.* (This is consistent and defensible — it just needs to be said.)
**Resolved:** `../PROCESSES.0.md` §3 and `../SERVICES.0.md` §1 now state this
contrast explicitly in both directions.

### Gap B — patrun routing limits (top-level scalars only)
**Verdict: never hit the limit in normal modelling — but only because we knew.**
Every routed field is a top-level atom (`op:'create`, `tag:'toggled`). We never
wanted to route on a nested field. The one place it *would* have tempted us —
authorization on `{user:{role:'admin}}` — we instead expressed as a `prior`
layer (`own-guard`) comparing `req.@user` to `req.user`, which is arguably
better design anyway. So the limit nudged us toward flatter messages and
cross-cutting guards. Worth keeping the "flatten your tags" note prominent.

### Gap C — reply correlation
**Verdict: the correlation-tag convention works and is the right phase-1 call.**
`client.aql` matches the `subscribe` reply with `{ok:Boolean user:String}` and
the push with `{push:Atom user:String}`, never a bare `{}`. `presence.aql`
answers a ping with `{tag:'pong …}`. It is mildly noisy that the caller and
callee must agree on a tag by convention with no compiler help — this is
exactly the itch the phase-2 `call`/`reply` sugar (a dedicated per-call
channel) is meant to scratch, and the app makes that future value concrete.

### Gap D — in-process service is the first slice
**Verdict: confirmed, emphatically.** `todo.aql` + `auth.aql` + `audit.aql`
are fully meaningful with **none** of the process/transport/pool machinery —
you could exercise them with direct `call`s in a test today. The `prior`/`wrap`
stack (the part the RFC notes must live in the Service layer, not patrun) is
the only non-trivial piece, and it is the highest-leverage one. Build this
first.

### Gap E — capability prerequisites
**Verdict: the gates land exactly where you expect.** `spawn` in
`sessions.aql` and `serve`/`listen` in `app.aql` are the only capability-gated
calls; the pure service files need nothing. A `sandbox`/`read-only` profile
gets the entire service DX and is hard-denied only at `serve`/`listen`. That
the boundary falls on so few lines is a good sign the scoping is natural.

### Gap F — `after` / timeout integration
**Verdict: ergonomic.** `presence.aql`'s `(after 30000)` heartbeat and
`client.aql`'s `(after 60000)` drain-or-give-up read naturally as just another
clause. Specifying it against the existing `TTimeout` machinery (rather than a
new timer) is invisible to the author, which is the goal.

### Beyond the gaps — one friction the app surfaced

**`overflow` granularity is server-level, but services want different
policies.** `notify` should be `'drop` (lossy telemetry must never stall the
hot path) while the todo store wants `'block` (backpressure). The RFC sets
`overflow` as a `server` option (§8.1), forcing either a per-service mailbox
option or a separate sub-server for `notify`. See the flagged comment in
`app.aql`. Recommend the RFC allow per-service mailbox/overflow overrides, with
the server value as the default. **Resolved:** `../SERVICES.0.md` §8.1 now
specifies a per-service override — `server [ worker (metrics {overflow: 'drop}) ]
{…}` — with precedence per-service > server default > built-in default.

---

## What felt genuinely good

- **Services are values; layers are functions over values.** The whole graph
  in `app.aql` is ordinary composition — `Audit.log (Auth.gate sessions (…))`.
  No registration, no annotations, no config format.
- **`prior`/`wrap` paid for itself immediately.** Auth and audit are entirely
  absent from `todo.aql`; they are added from the outside in `app.aql`. This is
  the design's clearest DX win.
- **The service↔process contrast is legible.** `todo.aql` (service) vs.
  `presence.aql` (raw process) makes the "request/reply with owned state" vs.
  "long-lived bespoke loop" decision obvious. A one-paragraph "when to use
  which" in the RFC, pointing at these two files, would close the §"stream vs
  process boundary" minor item from the review.

## Cross-references

- Service model, `prior`/`wrap`, deferred reply: `../SERVICES.0.md` §1
- Modules as service constructors: `../SERVICES.0.md` §2
- `server`/`serve`/restart: `../SERVICES.0.md` §3
- `listen`/`connect` transport & capability gating: `../SERVICES.0.md` §4
- Bounded mailboxes & the uniform failure contract: `../SERVICES.0.md` §8
- `pool` & hash-sharding: `../SERVICES.0.md` §9.2
- `spawn`/`receive`/`register`, two-layer matching, correlation: `../PROCESSES.0.md` §3, §4, §10
- Immutable-only message rule (why events are plain Maps): `../PROCESSES.0.md` §6
