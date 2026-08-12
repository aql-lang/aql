# STATE-MACHINES

Design for general-purpose **state machines** in boru — a builtin
**`boru:state`** module (mixed Go+boru), plus the verdict on language
primitives: state machines arrive as **words and data literals, not syntax**.

## Context

boru has no state-machine facility today — `grep -rn "boru:state"` over the
tree finds nothing, and no prior design doc covers the topic. Yet the repo
already writes state machines by hand: `design/examples/todo/sessions.boru`
opens by calling itself "the SERVICE layer (request/reply state machine)",
every multi-state service scatters its transition logic across patrun rules
and `case` clauses, and the network roadmap (`NETWORK-SERVERS.0.md` codecs,
connection lifecycles) is heading into the most FSM-dense territory in
software. Most languages hand this problem to a library; this RFC specifies
what boru's library should be, and why the language itself needs no new
primitives to support it well.

What boru already owns that this design reuses (each claim verified against
the tree at design time — by `boru describe`, by running the program shown, or
by the cited file):

- **The actor substrate is implemented**: `spawn` / `self` / `send` /
  `receive` (with `after`) / `register` / `whereis`, bounded mailboxes,
  pattern-matched consume-front dispatch (`design/PROCESSES.0.md`; verified by
  `describe spawn` and by run). A hosted machine's event loop is these words.
  Note: `design/IMPLEMENTATION-STATUS.10.md` still records PROCESSES/SERVICES
  as "RFC; no code" — that is stale; `design/NETWORK-IMPLEMENTATION-PLAN.0.md`
  §1 is the ground truth for the shipped subset.
- **The service layer is implemented**: `service` / `add` / `call` / `send` /
  `state-of` — "a value that owns state and answers pattern-matched requests"
  (`design/SERVICES.0.md` §1; verified by run: a counter service with an
  `add {op: "inc"} (…) svc` handler answers `call {op: "inc"} svc`). A machine
  is precisely the *disciplined* version of such a service.
- **Gating exhaustiveness machinery** — the asset no mainstream FSM host
  language had: `case_not_exhaustive` is an Error-severity finding that names
  the uncovered member and gates `boru check`, `run` preflight, and
  compilation (`design/case-exhaustiveness.0.md`, landed July 2026); and
  `partial_dispatch` over a declared `tor` union is Error-severity and fires
  **even for uncalled fns** at construction (`check/go/carrier.go`).
- **Closed state sets as types**: `enum [a/q b/q c/q]` mints a closed atom
  union; `class` mints sealed nominal records; `const` mints singletons;
  `refine` mints newtypes with a nominal coverage boundary.
- **patrun** most-specific-wins routing (`lang/go/native/native_patrun.go`) —
  the dispatch engine under services and `receive`.
- **Code as data**: quoted lists, `do`, `canon` round-tripping data values,
  and the spec-map-driven-library precedent (`boru:cli`: one spec map drives
  parse, dispatch, usage, and main). A machine definition can be a plain data
  value with *named code* at the leaves — boru does not have to choose between
  a data DSL and a code API.
- **Replay for free**: `fold [step] events s0` folds a pure step over an event
  log (verified by run: `fold [bump] [{e: 1} {e: 2} {e: 3}] {n: 0}` → `{n:3}`
  with the element bound first, accumulator second); `scan` yields the audit
  trajectory.
- **TCO is a language guarantee** (`design/TCO.10.md`) — a process host's
  tail-recursive receive loop cannot blow the stack.
- **Timer machinery at the host layer**: `receive … after <ms>`, plus
  `boru:time-util`'s clock-capability-gated words.
- **Immutability-first values** (`eng/go/clone.go`) and immutable-only
  messages (`PROCESSES.0.md` §6) — snapshots and events are plain immutable
  maps, so they are always legal messages and always race-free. One
  cost-model correction against that RFC's zero-copy stance: the shipped
  `send` **deep-copies** even immutable payloads at the process boundary
  ("Deep-copy at the boundary so the receiver can never observe the sender
  mutating a shared container", `lang/go/native/native_process.go`), so a
  snapshot crossing a process is copied today, not shared by reference.

This is a **design RFC only — no implementation code yet**, matching how other
subsystems were designed first (`PROCESSES.0.md`, `SERVICES.0.md`,
`STREAM-WORDS.0.md`).

### Relationship to `PROCESSES.0.md` and `SERVICES.0.md`

Those RFCs (now partially shipped) supply the two **hosts** this design runs
on; neither changes. `PROCESSES.0.md` §3 already names this document's core
move: the idiomatic alternative to selective receive "is **explicit
state-based deferral** (`gen_statem`-style: stash the deferred message in
process state and handle it after the state transition)". `boru:state` is that
idea made a facility — the machine owns the deferral discipline, visibly and
boundedly, instead of every process reinventing it. A served machine is a
`Service` whose handler is the machine's step; a spawned machine is a process
whose receive loop is the machine's step. The machine itself is neither: it is
a **value**, host-independent and testable with no concurrency at all.

### Relationship to `case-exhaustiveness.0.md` and `VALUE-PATTERN-DISPATCH.0.md`

The exhaustiveness pass is the strongest static asset this design leans on,
and it is already load-bearing: hand-written machines that encode states as an
`enum` and dispatch with `case` get gating state×event coverage today (§6.4).
`VALUE-PATTERN-DISPATCH.0.md` records the precision gap that blocks the
*overload* encoding of the same idea (enum-state value-pattern overloads fail
through variable references); this RFC **endorses that fix as an independent
effort** (§8 item 3) — it is the one language-level investment adjacent to
state machines that pays for itself regardless.

### Scope decisions (agreed)

Maintainer-decided 2026-08-12:

1. **Library, not syntax.** State machines ship as the `boru:state` module —
   words and data literals only, per the corpus default ("new behaviour is a
   word or a literal, nothing else"). The primitives considered and declined
   are recorded in §8.
2. **Mixed Go+boru module from day one.** A pure-boru module measurably cannot
   deliver check-time validation (the probes in §6.2), and static checking is
   this design's headline advantage — so `State.define` is Go-native and mints
   `state_*` diagnostics from the first release.
3. **Default unhandled-event policy is `error`.** A machine that receives an
   event its current state does not handle raises, unless the state defers it
   or the machine declares `policy: ignore/q`. Protocol-safe by default; UIs
   opt out per machine.
4. **Flat v1; hierarchy is phase 2; orthogonal regions never.** v1 machines
   are flat; the spec shape reserves nested `states:` (rejected with a clear
   `state_bad_spec` "not yet" error). Phase 2 adopts the SCXML algorithm
   (inner-first handling, LCA entry/exit ordering) wholesale rather than
   minting a 21st statechart semantics. In-machine parallel regions are
   permanently declined — concurrent machines are processes/services, boru's
   native composition story.
5. **Machine ≠ host; three artifacts, never fused.** The **definition** is
   plain data (canonical, diffable, hashable); **bindings** attach code to
   names late; the **snapshot** is a small plain value the *host* owns. The
   pure step is the product; the two hosts are thin adapters over the shipped
   service/process layers.
6. **Semantics frozen here, pinned by a conformance corpus.** The §3.3 freeze
   list is the specification; `lang/spec/module-state.tsv` rows pin every item
   from the first implementation (ADR-003), because the statechart tradition's
   defining failure is semantic fragmentation (§2 row 12).
7. **Out of scope for v1**: supervision integration and cancel-on-exit child
   processes (blocked on `boru:serve` restart machinery), domain replies from
   served machines (blocked on the `@from` reply handle, `SERVICES.0.md` §1),
   persistence storage (the snapshot is *persistable*; storing it is the
   host program's business), distribution, and UML do-activities.

## 1. Motivation & end-goal

The target programming model: **define a machine as a data value, drive it
with a pure step, host it wherever it needs to live.**

```boru
def door (State.define door-spec {announce: announce/r})   # value
def r (State.step door snap {event: open/q})               # pure step
def svc (State.serve door)                                 # or: a Service
def pid (State.start door)                                 # or: a process
```

Because the definition is data, the same value answers every downstream need:
`boru check` lints it (unreachable states, unknown transition targets,
state×event holes), a diagram is generated from it, a model-based test is
derived from it, a snapshot is pinned to its hash. Because the step is pure,
a machine is tested with plain values and replayed with `fold` — no process,
no clock, no mock. Because the hosts are the existing service/process layers,
a machine gains request/reply, mailboxes, backpressure, and (in phase 2 of the
services roadmap) supervision without this module owning any concurrency.

For a *query language* this is the natural shape: the machine definition is
one more value the language can query, transform, and generate — the property
every code-first FSM library gives up, and every data-first workflow language
(Amazon States Language, BPMN) pays for with a crippled expression language.
boru's homoiconic spec maps refuse that dichotomy: the shape is data, the
leaves are named boru words.

## 2. Prior art — the hard lessons this design obeys

The design was preceded by a survey of the statechart tradition (Harel 1987,
UML, SCXML, Samek's QP), languages with first-class machine support (Erlang
`gen_fsm`→`gen_statem`, P, UnrealScript, Ragel, Esterel, Plaid/typestate),
the library ecosystem (XState, Stateless, Boost.*, Python `transitions`,
Rails AASM, Redux), durable/distributed machines (Step Functions, Temporal,
replicated state machines, event sourcing), and the practical-verification
literature (model checking adoption, eqc_statem-style model-based testing).
The rows below are the lessons that survived contact with production, mapped
onto boru; each shaped a concrete decision in §3.

| # | Hard lesson | Source | boru realization |
|---|---|---|---|
| 1 | RTC (run-to-completion) with queued events, never reentrant `fire()` — every library that shipped reentrant dispatch retrofitted a queue | Stateless, Python `transitions`, AASM | RTC is free per host: a service serializes handlers; a process loop takes one message per `receive` (§3.3.1) |
| 2 | Postpone/defer must exist — reinvented independently four times (selective receive, `gen_statem` `postpone`, P `defer`, Akka `stash`); its absence helped kill `gen_fsm` | Erlang/OTP, P, Akka | per-state `defer:` lists; deferred events live **in the snapshot**, bounded, replayed in arrival order on state change (§3.3.3) — the visible form `PROCESSES.0.md` §3 already blesses |
| 3 | Internal events drain before the next external event, or users bypass the machinery | SCXML microsteps, `gen_statem` `next_event`, P `raise` | the step drains `raise`d internal events before returning (§3.3.2) |
| 4 | State-scoped **named** timers, auto-cancelled by transition, kill the stale-timer bug class; one timer per state is below the floor (a TCP-ish machine needs retransmit + keepalive + 2MSL concurrently) | `gen_statem` timeouts; RFC 9293 | `after:` is a **map of named timers** per state; any state change cancels them; hosts implement, the pure step only ever sees timer *events* (§3.3.6) |
| 5 | Pure step in, effects out; I/O never inside the machine | sans-io (h11, quinn-proto), Elm's `update : Msg -> Model -> (Model, Cmd)`, Redux, Schneider's replicated-state-machine kernel | `State.step` is pure: `(machine, snap, event) → {snap effects status}`; hosts execute effects (§3.2) |
| 6 | Definition-as-data wins tooling; code-only definitions can't be diagrammed, diffed, or versioned — but pure-data DSLs grow ad-hoc expression languages | ASL's JSONata capitulation vs Temporal's code-first pain | spec map with **atoms naming words** at guard/action leaves; boru is the expression language (§3.1) |
| 7 | Bind guard/action *implementations* late, by name — embedding closures in the definition kills serialization and reuse | XState v5 `setup`/`provide` | `State.define {spec} {bindings}`: the spec carries names; the bindings map attaches fns (§3.1) |
| 8 | Pin persisted instances to an immutable definition version; nobody auto-migrates in-flight machines successfully | Step Functions versions, event-sourcing upcasters | snapshots carry `v:` (content hash of the canonical definition); `step` refuses a mismatch unless given `{migrate: …}` (§3.3.9) |
| 9 | Declared, typed identifiers — never stringly-typed states/events | a decade of ecosystem retrofits | states/events are atoms; an optional `events:` block declares the alphabet and payload schemas, making typos check-time errors (§3.3.11) |
| 10 | Unhandled-event policy is domain-dependent: protocols want loud, UIs want silent — make it declared, then check totality against it | P (mandatory handle/defer/ignore) vs XState (removed strict mode) | per-machine `policy:`, default `error/q` (scope decision 4); `state_unhandled` advisory computed against it (§6.3) |
| 11 | Hierarchy is the one statechart feature worth its cost (the principled home for "handle X from any state"); regions, deep history, and the pseudostate zoo are traps; entry/exit actions are the safety feature | Harel→UML fragmentation, QP, XState v5 | flat v1 + phase-2 XOR hierarchy via the SCXML algorithm; entry/exit in v1 (§3.3.4); regions never |
| 12 | Underspecification is not flexibility: 20+ mutually incompatible statechart semantics; the cure is one executable semantics + a conformance suite | von der Beeck's survey; SCXML's IRP tests | the §3.3 freeze list + `lang/spec/module-state.tsv` conformance rows from day one |
| 13 | Callbacks fused to persistence is a tarpit; the machine must never own storage | Rails AASM/state_machines | snapshots are plain values; hosts and programs decide storage; no callback ever fires from a persistence layer |
| 14 | The visualizer drives adoption; diagrams are a *generated view*, never a source | XState/Stately vs BPMN round-tripping | `State.diagram` emits Mermaid from the definition (phase 2) |
| 15 | Model-based testing is the verification sweet spot that actually ships; full model checking demands a second artifact and drifts | Erlang QuickCheck `eqc_statem`, AWS S3 ShardStore vs SPIN/TLA+ | the definition *is* the model: `State.explore` derives generate-run-shrink testing from it (phase 3) |
| 16 | The FSM-densest domain — RFC protocol machines (TCP, TLS 1.3 App. A, QUIC, HTTP/2) — never adopted FSM libraries; its correctness story is conformance suites and monitors | RFC 9293 §3.3.2, RFC 8446, RFC 9000 | `State.conform` trace monitors (phase 3); and honesty: `boru:net` codecs get a *candidate* tool, not a mandate |

## 3. The machine model

### 3.1 Three artifacts

**Definition** — a plain, canonical map. States and events are atoms; guards
and actions are **atoms naming words**. It contains no function values, so
`canon` round-trips it, `deq` diffs it, and a content hash of it is
well-defined. The spec shape (verified: this literal parses and dot-paths
resolve, e.g. `spec.states.closed.on.open.to` → `open`):

```boru
def door-spec {
  initial: closed/q
  # policy: error/q                 # the default; ignore/q opts out per machine
  ctx: {key: ""  locks: 0}          # extended-state defaults; init merges over these
  events: {                         # optional declared alphabet (§3.3.11)
    open: {}  close: {}  lock: {key: String}  unlock: {key: String}
    lock-timeout: {}                # timer events are ordinary alphabet members
  }
  states: {
    closed: { on: { open:  { to: open/q  act: announce/q } } }
    open:   { on: { close: { to: closed/q }
                    lock:  { to: locked/q  when: key-fits/q }
                    lock-timeout: { to: locked/q } }   # auto-relock on timer
              after: { relock: {ms: 30000  event: lock-timeout/q} } }
    locked: { entry: log-locked/q
              defer: [open/q]       # postponed while locked; replayed on exit
              on: { unlock: { to: closed/q  when: key-fits/q } } }
  }
}
```

**Bindings** — a map from those names to function values, supplied to
`State.define` (the `/r` reference form, or any fn value). Late binding is
deliberate (§2 row 7): the same definition runs with production
implementations or test stubs. Bindings are **complete at define time** —
every name the spec references must resolve, and each bound value must fit
its role's shape (§6.3 `state_unknown_name`, `state_bad_binding`) — so a
missing or malformed implementation is a define-time error, never a
step-time surprise; a test that wants inert behaviour binds stubs.

**Snapshot** — a plain map owned by the caller/host, never by the machine:
`{state: closed/q  ctx: {…}  deferred: []  timers: {…}  v: "…"}`. `ctx` is the
extended state (a plain map the actions/guards read and return). Deliberately
**not** a minted type: the snapshot is the persistence artifact, and a plain
map is exactly what `canon`/`jsonify` and the immutable-message rule
(`PROCESSES.0.md` §6) already handle.

`State.define` fuses definition + bindings into a **`Machine`** value (§5) and
is where all §6.3 validation fires.

### 3.2 The pure step

```
State.step <machine> <snap> {event: <atom> …} (opts) -> {snap: Map  effects: List  status: Atom}
```

The attached code splits into two kinds, so the step stays pure and replay
stays honest (the Elm/sans-io split, §2 rows 5–6):

- **Reducers.** The `act:`/`entry:`/`exit:` names bind to **pure context
  reducers**, `(event ctx) -> <new-ctx>`, which the step runs **inside
  itself**, in the §3.3.4 order, folding their results into `snap.ctx`
  before it returns. A reducer that returns a plain map returns the new
  `ctx`; the record form `{ctx: <map>  raise: [<event> …]  fx: [<desc> …]}`
  additionally queues internal events (drained in-step, §3.3.2) and
  requests host effects.
- **Effects.** The `fx:` descriptors accumulated from the step's reducers
  (plus timer bookkeeping the hosts consume) come back in `effects`, in
  request order, for the **host** to execute after it adopts the new
  snapshot (§3.3.7). Effects are the only place I/O lives; their results
  re-enter the machine as ordinary events.

So `State.step` is pure end to end — no clock, no I/O, no mailbox; time
reaches it only as timer events — and `fold`-replay is correct by
construction: context evolution happens in-step, while `fx` descriptors are
deliberately **not** re-executed on replay (a replay reconstructs state; it
must not re-fire the outside world).

`status` is one of `handled/q`, `deferred/q`, `ignored/q`, `done/q`
(§3.3.8). Guards (`when:`) are pure predicates over `(event ctx)`. Purity —
of guards and reducers alike — is a **documented contract, not an enforced
property**: boru has no effect analysis for user fns today; §10 lists it as
a later possibility. A guard or reducer that raises aborts the step. `opts`
carries `{migrate: <fn>}` for version skew (§3.3.9).

Replay falls out of purity (idiom verified by run — element first,
accumulator second, per `fold`'s binding order):

```boru
def apply-event fn [[ev:Map snap:Map] [Map] [
  def r (State.step door snap ev)
  r.snap
]]
fold [apply-event] event-log (State.init door) dot snap
```

### 3.3 The v1 semantic freeze

These twelve items are the specification. Every one gets conformance rows in
`lang/spec/module-state.tsv`; an implementation that diverges from this list
is wrong, and a change to this list is a new revision of this document.

1. **Run-to-completion.** One external event per step; a step never observes a
   half-applied predecessor. Hosts guarantee serialization (the service
   handler mutex; the process loop's one-message-per-receive).
2. **Internal before external.** A reducer may queue internal events via its
   record form's `raise:` list (§3.2); raised events are processed within the
   same step, depth-first in raise order, before the step returns. The
   returned snapshot reflects the full microstep chain. A raise depth cap
   (default 64) turns runaway loops into an error rather than a hang.
3. **Postpone.** A state's `defer:` list names events to postpone. A deferred
   event is appended to `snap.deferred` with `status: deferred/q`. On any
   state *change*, the deferred list is replayed in original arrival order
   before any new external event, and events still deferred by the new state
   go back on the list. The list is **bounded**: default cap 256, overridable
   per machine (`defer-cap: N` in the spec), and exceeding it raises
   `state_defer_overflow` — the *behaviour* is frozen; only the default's
   size is still tunable (open question #1). An unbounded defer list would
   silently recreate the mailbox-growth footgun that `PROCESSES.0.md` §1
   rejects.
4. **Entry/exit ordering and self-transitions.** A transition runs: exit
   reducer of the source state → transition `act:` reducer → entry reducer of
   the target, each folding `ctx` in that order (§3.2). A transition **with
   `to:`** — including `to:` the current state — is *external*: exit and
   entry run. A transition **without `to:`** is *internal*: the machine stays
   put, only `act:` runs, no exit/entry, timers untouched. This single
   explicit distinction removes the classic self-transition bug class; there
   is no silent default.
5. **Initial entry.** `State.init` runs the initial state's entry reducer —
   its `ctx` result is folded into the returned snapshot, its `fx` requests
   come back in `effects` (SCXML behaviour): initial entry happens
   observably, once, at init — not on the first step.
6. **Timers.** `after:` is a per-state **map of named timers**
   (`{relock: {ms: N  event: E}}`). Semantics: entering a state arms its
   timers (absolute deadlines from arrival time); *any state change* cancels
   all timers of the exited state; a timer is **one-shot** — expiry removes
   it from `snap.timers` before its `event:` is stepped, so an expiry the
   machine ignores or handles internally cannot re-fire, and re-arming
   happens only through a subsequent entry to the state. The pure step never
   reads a clock — it receives timer events and carries arm/cancel
   bookkeeping in the snapshot's `timers:` map; **hosts** own the clock.
   Hosts must check expired absolute deadlines **before** taking the next
   mailbox message: the shipped consume-front primitive pops a queued
   message before it checks its own deadline (`core/go/process.go`
   `PopFront`), so a naive `receive … after` loop under continuous traffic
   would starve timers forever — the process host therefore delivers any
   already-expired timer event first, and only then receives. v1: the
   process host implements timers; the service host **has no timers**
   (§3.5.2, honestly: nothing runs between calls).
7. **Effects.** The host executes `effects` in list order, *after* adopting
   the new snapshot. An effect that raises: the host reifies the error
   (`do … error …`); if the spec declares a top-level `catch: <event>` —
   an ordinary alphabet member (§3.3.11) — the error is fed back as that
   event with the error map as payload; otherwise the host re-raises.
   Effects must not `call` the machine's own hosting service (self-`call`
   deadlocks — documented for services generally); `send` to it is legal.
8. **Final states.** A state with `final: true` may have `entry:` but no
   `on:`/`after:`/`defer:`. Stepping a machine whose snapshot is final
   returns `status: done/q` and the snapshot unchanged (no error — done is a
   result, not a fault). The process host's loop exits after entering a final
   state; the service host answers subsequent calls with `done/q`. Phase 2's
   composition surfaces `done` to a parent machine as an event.
9. **Version pinning.** `State.init` stamps `snap.v` with the content hash of
   the canonical definition. `State.step` with a mismatched pair raises
   `state_version_skew` unless called with `{migrate: <fn>}` in its opts,
   whose result snapshot must carry the new hash. No auto-migration, ever
   (§2 row 8). The hash covers the **definition only** — including its
   optional author-bumped `version:` field — never the bindings: function
   identity does not hash, so changing a bound implementation without any
   spec change keeps `v` stable, and authors signal behavioural breaks by
   bumping `version:`. That is the honest limit of value-hashing code
   (open question #3).
10. **Invalid input is loud.** A snapshot naming an unknown state, a
    malformed event (no `event:` atom), or an event outside a declared
    alphabet raises typed errors (`state_bad_snapshot`, `state_bad_event`).
    Under `policy: error/q` an unhandled-but-well-formed event raises
    `state_unhandled_event` naming the state and event; under `ignore/q` the
    step returns `status: ignored/q` with the snapshot unchanged.
11. **Declared alphabet (optional, recommended).** With an `events:` block,
    every event name used anywhere (`on:`, `defer:`, `after:` targets,
    `raise`, the machine's `catch:`) must be declared — checked at define
    time (§6.3) — and event payloads are validated against the per-event
    schema at step time. Without the block, the alphabet is implicit and
    payloads unvalidated; the totality advisory (§6.3) then covers only
    mentioned events.
12. **Determinism and guarded variants.** A state×event key maps to either a
    single transition map or a **list of guarded variants** tried in
    declaration order, first satisfied wins; an all-guards-false outcome
    falls through to the unhandled policy. The list is the *only* spelling
    for multiple arcs on one event: a duplicate map key collapses silently
    at the parser, last one wins (verified by run: `{a: 1 a: 2}` → `{a:2}`),
    so `State.define` never sees the duplicate and cannot diagnose it.
    `state_conflict` is therefore defined over the list form: an unguarded
    variant anywhere but last — it shadows every variant after it — is the
    Error.

### 3.4 What v1 deliberately does not have

Hierarchy (phase 2, by the SCXML algorithm — the spec's nested-`states:`
shape is *reserved* and rejected with a clear error so flat specs stay
forward-compatible), machine-in-machine composition (phase 2: a parent
embedding a child machine value, stepping it purely, and receiving its
`done`), orthogonal regions (never), do-activities (never — long-running work
is a child process, cancel-on-exit, once supervision lands), history states
(open question #6), and domain replies computed by actions on the service
host (phase 3, blocked on `@from`).

### 3.5 Hosts

#### 3.5.1 Standalone (no host)

`State.init` + `State.step` + your own loop or `fold`. This is the testing
and replay surface, and the sans-io shape (§2 rows 5, 16): a codec or
protocol core can be a machine stepped by whatever owns the bytes.

#### 3.5.2 Service host — `State.serve <machine> -> Service`

A `Service` whose single handler runs the step: `call {event: …} svc`
answers with the step outcome `{state: <atom>  status: <atom>}` — the
*outcome record*, not a domain reply. Two honest limitations, stated rather
than papered over:

- **Deferral over `call` returns a receipt.** A deferred event's reply is
  `{status: deferred/q …}` immediately; when the event later replays, its
  effects run during whichever call triggered the state change. No caller
  waits across the deferral — that requires the decoupled reply handle
  (`@from`, `SERVICES.0.md` §1), which is design-only today; phase 3 adopts
  it when it ships.
- **No timers.** A service runs only when called; nothing fires between
  calls. Machines that need `after:` use the process host (or arrange an
  external ticker that `send`s timer events — which is exactly what the
  process host automates).

RTC comes from the service layer's handler serialization; mailbox bounds and
backpressure come from `SERVICES.0.md` §8.1 when the service is `serve`d.

#### 3.5.3 Process host — `State.start <machine> (opts) -> Pid`

A `spawn`ed tail-recursive receive loop (TCO-guaranteed) holding the snapshot
as its loop binding: each iteration first delivers any **already-expired**
timer deadline as its event — the expiry check precedes the mailbox take,
per §3.3.6, because the consume-front primitive would otherwise starve timers
under continuous traffic — otherwise `receive`s with the nearest deadline as
its `after`, steps the machine, executes effects, and recurses on the new
snapshot; it returns when a final state is entered. Events are tagged maps (`send {event: open/q} pid`),
the established message convention. Capability-gated like any `spawn`
(`process` scope), with `clock` scope for the timer arm (§7).

**Implementation note (probed):** a pure-boru module body runs in an isolated
sub-registry with its **own lazily-created `ProcessRuntime`** — a pid spawned
inside module code is invisible to the importer's `whereis`, and `self`/
shutdown-context diverge. The `boru:state` loader must share the parent
registry's `Procs` pointer (one line in the loader, plus a pinning test), and
this constraint holds for any future module that spawns on the user's behalf.

## 4. Language surface (the `boru:state` words)

All proposed names verified collision-free: `State` resolves to nothing today
(`describe State` → no description; `def State …` works), and every word below
is reached as `State.<word>` per the module convention, so no core word is
shadowed (ADR-001; no core-word overloads are proposed). Signatures use the
top-first convention with `describe`-ready summaries.

- **`State.define <spec:Map> <bindings:Map> -> Machine`** — validate the spec
  (all §6.3 rules), resolve guard/reducer names against `bindings` and check
  each bound value against its role's shape (§6.3 `state_bad_binding`), and
  mint the `Machine`. Raises `state_bad_spec` (and friends) on any violation;
  the same Error rules fire at **check time** when the literals are concrete
  (§6.3).
- **`State.init <machine:Machine> (opts:Map) -> Map`** — `{snap effects}`:
  the initial snapshot (version-stamped, §3.3.9; `ctx` = the spec's `ctx:`
  defaults merged under `opts.ctx`) and the initial entry's `fx` (§3.3.5).
- **`State.step <machine:Machine> <snap:Map> <event:Map> (opts:Map) -> Map`**
  — the pure step (§3.2): `{snap effects status}`. `opts` carries
  `{migrate: <fn>}`, the only path across a version skew (§3.3.9).
- **`State.can <machine:Machine> <state:Atom> -> List`** — the event atoms
  with *table-level* transitions from that state. Documented loudly as
  table-level: guards are **not** evaluated (advertising guard-aware
  availability is the lie XState removed `nextEvents` over).
- **`State.spec <machine:Machine> -> Map`** — the canonical definition back
  (bindings excluded): the diagram/diff/hash artifact.
- **`State.serve <machine:Machine> (opts:Map) -> Service`** — the service
  host (§3.5.2); `opts.ctx` seeds the instance's extended state as in `init`.
- **`State.start <machine:Machine> (opts:Map) -> Pid`** — the process host
  (§3.5.3); `opts.ctx` as in `init`, remaining opts pass through to `spawn`
  (mailbox bound etc.).

Phase 2 adds `State.diagram` (Mermaid text from the definition) and
`State.lint` (the §6.3 advisories as plain data, the observability channel
for dynamically built specs); phase 3 adds `State.conform` (trace monitor)
and `State.explore` (derived model-based testing) — named here so the v1
data model reserves nothing that blocks them.

Every export lands with `lang/spec/module-state.tsv` rows (ADR-003), and the
conformance corpus for §3.3 lives in the same file.

## 5. New types in the lattice

One minted type: **`Machine`**, an Ideal following the `Timeout`/`Interval`
precedent (`design/IDEAL.10.md`) — **opaque** (internals reached only via
`State.spec`/`State.can`), **immutable** (a legal message payload; send a
machine to a process), **comparable** by its definition content hash
(consistent with `TYPE-ORDERING.10.md`), **printable** as e.g.
`Machine<door 3 states>`. Minted per the module-type pattern so `describe`
and dispatch integrate.

The snapshot is deliberately **not** a type (§3.1): plainness *is* its
contract. If implementation experience shows snapshot-shaped Maps demand
their own carrier, that is a revision of this document, not a quiet change.

## 6. Static checking

### 6.1 What the checker already gives state machines

Verified against the shipped checker: `case` over an `enum` scrutinee
hard-gates on exhaustiveness (`case_not_exhaustive` names the uncovered
member); `partial_dispatch` fires as a gating Error when a declared `tor`
union reaches an overload set missing an alternative — **even for uncalled
fns** at construction; and calling a word with a state type it has no
signature for is a check-time `no_signature`. That is: exhaustive event
handling per state, exhaustive state handling per event, and invalid
transition calls rejected statically — for machines *hand-written* in the
right encodings (§6.4). No mainstream FSM host language ships this.

### 6.2 Why the module must be Go-native to check anything (measured)

Three probes, all against the current binary, decided scope decision 2:

- A pure-boru module fn that `raise`s on a concretely-bad spec literal
  passes `boru check` with **zero errors** (the branch-guarded raise is not
  decided even on a statically-decidable condition); the failure surfaces
  only at run time.
- An exported macro invoked across the module boundary returns its **raw
  unexpanded template** — so a pure-boru `State.compile` cannot lower a spec
  into checked `case`/class forms in the importer's scope.
- The workaround — returning quoted tokens for the user to `do` — installs
  definitions, but the checker does **not** gate generated code: a
  non-exhaustive `case` over a 3-member enum inside a `do`-run quote passes
  `boru check` clean, while the directly-written spelling is a gating Error.

So library-authored *lowering* cannot inherit the gating diagnostics, and
library-authored *validation* cannot reach check time from boru source. The
check-time story requires a Go-native `State.define` with a check-mode
mirror — the established `parse_bad_spec` pattern, severities registered in
the single table (`core/go/check_state.go`).

### 6.3 The `state_*` diagnostic family

Minted by the Go-native `define` when its spec (and bindings-key set) are
concrete literals at the call site — the common case for machine definitions.
The **Error** rows are enforced identically at runtime for dynamic specs
(same codes, same messages, per the checker's mirror discipline); the
**Info** advisories are check-time findings only — a runtime `define` can
neither gate nor warn, so dynamically built specs get them as plain data
from `State.lint` (phase 2):

| code | severity | meaning |
|---|---|---|
| `state_bad_spec` | Error | malformed shape: no `initial:`, unknown keys, nested `states:` (reserved for phase 2), `final` state with `on:` |
| `state_unknown_target` | Error | a `to:` names an undeclared state |
| `state_unknown_name` | Error | an unbound `act:`/`when:`/`entry:`/`exit:` name; or, with a declared alphabet, an event in `on:`/`defer:`/`after:`/`raise`/`catch:` outside `events:` |
| `state_bad_binding` | Error | a bound value is not a function or does not fit its role — guard: `(Map Map) -> Boolean`; reducer: `(Map Map) ->` a map or the `{ctx raise fx}` record (§3.2) |
| `state_conflict` | Error | an unguarded variant that is not last in its state×event variant list, shadowing every variant after it (§3.3.12) |
| `state_unreachable` | Info | a state with no path from `initial:` (advisory per the "gate on wrongness, advise on smell" precedent, `case_unreachable_clause`) |
| `state_unhandled` | Info | the state×alphabet totality matrix's holes, computed against the machine's declared policy; **Error** iff the spec opts in with `total: true` (open question #2) |
| `state_no_final_path` | Info | machine declares a final state some state cannot reach — the honest pseudo-liveness check; real liveness is out of scope |

This is the ranked shortlist the verification literature says actually ships
(checks over the artifact the author already wrote), and nothing more: no
session types, no model-checking second artifact.

### 6.4 The hand-written encodings stay blessed

The declarative machine is not the only citizen. Two encodings of
state-machine logic in plain boru get §6.1's checking for free, today:

- **enum + `case`**: states as `enum [a/q b/q]`, a step fn dispatching
  `case s [ … ]` per state then per event — every state×event hole is a
  gating `case_not_exhaustive`.
- **class-per-state + `tor` union**: data-carrying states as one class each,
  the state type as their union, transitions as per-state overloads —
  `partial_dispatch` reports the uncovered state, and an illegal
  transition call is a check-time `no_signature`. This is typestate-lite,
  with the caveat stated honestly: class instances sit in boru's
  shared-mutable column ("class instances are shared mutable state: writes
  are visible through every alias", REFERENCE.md §Classes), so an alias to a
  superseded state value can still invoke old-state operations — the
  encoding delivers *narrowing*, never an alias-safe typestate proof, and
  narrowing is the right ambition here (full typestate died of aliasing
  everywhere it was tried).

The module's documentation presents both as the drop-down path when the
declarative table doesn't fit (HOWTO material), and `VALUE-PATTERN-DISPATCH.0.md`'s
precision fixes (§8 item 3) make the second encoding robust through variables.

## 7. Safety & capability integration

Per `design/PERMISSIONS.10.md` scopes, and composing with `boru:vm`
attenuation:

- **`State.define` / `init` / `step` / `can` / `spec`**: pure — no capability,
  usable in `sandbox`/`compute` profiles, spec-rowable under the frozen spec
  clock (no `hermeticExempt` growth).
- **`State.start`**: gated exactly like `spawn` (`process` scope), plus
  `clock` scope when the machine declares `after:` timers (the host arms real
  deadlines). Restrictive profiles get the whole pure surface and `State.serve`,
  but cannot spawn machine processes — the same posture as the actor layer.
- **`State.serve`**: no new capability in-process (it is `service` + `add`);
  placing the service in a served `server` inherits that layer's gating when
  it lands.

Effects run with the host's ordinary permissions — the machine adds no
ambient authority; an action can do exactly what the program hosting the
machine could do directly.

## 8. Language primitives — considered and declined

The corpus default stands: "new behaviour is a word or a literal, nothing
else" (`effect-oriented-programming-in-boru-report.0.md`,
`fsharp-units-in-boru-report.0.md`), and `amop-in-boru-report.0.md` §2.1
already recommended library-first for exactly this shape ("Do not change the
core parser first"). Candidates, with verdicts:

1. **Dedicated machine syntax** (`machine Door … on open -> open …`) —
   **declined.** The strongest external evidence in the whole survey: language-
   level FSM syntax is a graveyard (UnrealScript `state` blocks dropped in
   UE4; Akka's FSM DSL deprecated for plain `become`; Plaid dead; SDL niche)
   while library embeddings survive decades. Internally, the parser is the
   costliest surface in the repo (100%-gated leaf module, Go+TS parity, eng
   lowering, checker, compiler, formatter), and the spec map already parses
   as idiomatic forward-form boru (verified, §3.1) — the sugar would buy
   punctuation. Revisit only on demonstrated post-v1 usage pain, per the amop
   report's condition. *(Maintainer-decided 2026-08-12: words only.)*
2. **Mailbox-level postpone / selective receive** — **declined.**
   `PROCESSES.0.md` deliberately demoted selective receive; snapshot-held
   deferral is the visible, bounded version of the same power, and Erlang's
   operational history (O(n) mailbox scans, silent accumulation) argues the
   RFC's side.
3. **Value-pattern-dispatch precision fixes** — **endorsed, as an independent
   effort.** The two partition bugs and the variable-reference gap recorded
   in `VALUE-PATTERN-DISPATCH.0.md` are not state-machine work, but fixing
   them completes §6.4's second encoding (overload-level exhaustiveness
   through variables). This document adds a consumer to that design's
   motivation; it does not depend on it.
4. **Checker flow-narrowing** ("this binding is `Idle` in this branch, so
   `stop` is invalid here") — **deferred.** The guard-narrowing chassis makes
   it plausible and value semantics makes it sound-ish without linearity, but
   it should be driven by evidence from `state_*` diagnostics in use, not
   designed speculatively.
5. **Generalizing `receive`-style binding slots** to `add`/machine clauses
   (healing the route-only vs route+bind asymmetry) — **out of scope here**;
   it belongs to the processes/services design line. The asymmetry itself is
   now recorded in the register as **NUR063** (Pending), so it cannot be
   silently baselined while that line decides.

## 9. Gap analysis

**This RFC adds:** the machine model (three artifacts, pure step, the §3.3
freeze), the `boru:state` surface (§4), the `Machine` type (§5), the
`state_*` check family (§6.3), two hosts over shipped layers (§3.5), and the
conformance-corpus obligation (scope decision 6).

**Preconditions (phase 0):**

- **The VM double-run bug.** Reproduced at design time: a two-`call` counter
  service whose handler body is `state set count (state.count add 1)` yields
  `{count:4}` compiled vs `{count:2}` under `--no-compile` — the poly-dispatch
  VM fallback ("result count 1 differs from the recorded claim 0; deferring to
  the interpreter") re-runs the mutating body. It is shape-dependent (the
  forward-form spelling `(add (state get count/q) 1)` does not trigger the
  fallback), so spec rows cannot rely on avoidance: `State.serve` rows driving
  mutating handlers would fail the repo's own compiled-vs-interpreted gates.
  Fix (or prove `State.serve`'s generated handler avoids the fallback path)
  before phase 1 rows land.
- **`ProcessRuntime` sharing in the module loader** (§3.5.3 note).

**Still missing after v1 (and where it lands):** hierarchy and definition-
level composition (phase 2); diagram (phase 2); conform/explore (phase 3);
deferred domain replies over `call` — blocked on `@from` (phase 3, with
`boru:serve`); cancel-on-exit child work — blocked on links/monitors/
supervision (phase 3+); persistence storage and distribution — not this
module's business (the snapshot is the portable artifact; `SERVICES.0.md`'s
transport story is the distribution story).

## 10. Phased roadmap

- **Phase 0 — preconditions.** The VM fallback double-run fix; loader `Procs`
  sharing; (independent, already recorded elsewhere) the
  `VALUE-PATTERN-DISPATCH.0.md` partition fixes.
- **Phase 1 — the core module.** `define`/`init`/`step`/`can`/`spec`/
  `serve`/`start`; the §3.3 semantics complete (RTC, internal drain, bounded
  postpone, entry/exit + explicit self-transition kinds, named state timers
  on the process host, effects contract with `catch:`, final states, version
  pinning, alphabet + payload validation, determinism rules); the `state_*`
  Errors and advisories of §6.3; the conformance TSV corpus pinning every
  freeze item, positive and negative rows paired.
- **Phase 2 — hierarchy, composition, diagrams.** Nested `states:` per the
  SCXML algorithm (inner-first, LCA entry/exit, declaration-order
  tie-breaks) with its own conformance rows; parent machines embedding child
  machine values (pure child-step, child `done` surfacing as a parent
  event); `State.diagram`; shallow history only if it falls out of the
  algorithm (open question #6).
- **Phase 3 — tooling and services integration.** `State.conform` trace
  monitors; `State.explore` derived model-based testing (quota-capped);
  `@from`-based deferred domain replies on the service host; supervised
  machines and cancel-on-exit invoke when `boru:serve` restart machinery
  lands.
- **Later, evidence-driven.** Checker flow-narrowing over machine states;
  effect/purity analysis for guards; persistence conventions
  (snapshot-store recipes, not machinery).

## 11. Worked example

The door machine end to end. (Illustrative surface; exact syntax settles
during implementation. The spec literal, its dot-paths, and the `fold` replay
idiom are verified against today's parser; the `State.*` words are this RFC.)

```boru
import "boru:state"

# ---- bindings: ordinary words; the spec refers to them by name ----------
def announce  fn [[ev:Map ctx:Map] [Map] [ ctx ]]            # act: no ctx change
def log-locked fn [[ev:Map ctx:Map] [Map] [ ctx set locks (add (ctx get locks/q) 1) ]]
def key-fits  fn [[ev:Map ctx:Map] [Boolean] [ eq ev.key ctx.key ]]   # pure guard

# ---- the machine: definition (data) + bindings (code), fused ------------
def door (State.define {
  initial: closed/q                       # policy: error/q is the default
  ctx: {key: "k1"  locks: 0}              # extended-state defaults; see init below
  events: { open: {}  close: {}  lock: {key: String}
            unlock: {key: String}  lock-timeout: {} }
  states: {
    closed: { on: { open: { to: open/q  act: announce/q } } }
    open:   { on: { close: { to: closed/q }
                    lock:  { to: locked/q  when: key-fits/q }
                    lock-timeout: { to: locked/q } }   # auto-relock on timer
              after: { relock: {ms: 30000  event: lock-timeout/q} } }
    locked: { entry: log-locked/q
              defer: [open/q]             # an open while locked waits, bounded
              on: { unlock: { to: closed/q  when: key-fits/q }
                    lock:   { } } }       # internal: no to:, no exit/entry
  }
} {announce: announce/r  log-locked: log-locked/r  key-fits: key-fits/r})

# ---- standalone: pure stepping and replay -------------------------------
def r0 (State.init door {ctx: {key: "front-door"}})   # opts.ctx merges over
                                          # the spec defaults; {snap effects}
def r1 (State.step door r0.snap {event: open/q})
r1.snap.state                             # => open
r1.status                                 # => handled

def apply-event fn [[ev:Map snap:Map] [Map] [
  def r (State.step door snap ev)
  r.snap
]]
fold [apply-event] event-log r0.snap      # replay an audit log to its end state

# ---- service host: request/reply outcomes -------------------------------
def svc (State.serve door)
call {event: open/q} svc                  # => {state: open  status: handled}
call {event: lock/q  key: "k1"} svc       # guard consulted; outcome record back

# ---- process host: timers live here -------------------------------------
def pid (State.start door)                # process- and clock-gated
send {event: open/q} pid                  # 30s later, relock delivers
                                          # lock-timeout to the machine
```

Note the conventions in play: the definition is a plain map whose guard and
reducer leaves are **atoms naming words**, bound late by `State.define`, so
the definition alone is canonical, diffable, and hashable; the reducers run
*inside* the pure step (§3.2), which is why the `fold` replay reconstructs
`ctx` faithfully without re-firing any effect; events are **tagged maps**
(`{event: open/q …}`), the same convention the actor layer uses; the
snapshot is caller-owned plain data — nothing above holds hidden
state except the hosts, which hold exactly the snapshot; the guard is a pure
predicate over `(event ctx)`; the `relock` timer armed in `open` is cancelled
by any exit from `open` and otherwise delivers `lock-timeout` as an ordinary
event (auto-relock); `lock` in `locked` shows an *internal* transition (no
`to:`, no exit/entry — locking a locked door consumes the event and moves
nothing); and the deferred `open` while locked replays, in order, the moment
`unlock` succeeds.

## Open questions

1. **Defer-list cap size** — the overflow *behaviour* is frozen (§3.3.3:
   bounded, `state_defer_overflow` error, per-machine `defer-cap: N`
   override); is 256 the right default number? (Leaning yes; mirrors the
   mailbox-bound posture.)
2. **`state_unhandled` gating opt-in** — is `total: true` in the spec the
   right switch to promote the totality advisory to a gating Error, or should
   the strictest `policy: error/q` machines get it automatically? (Leaning
   the explicit `total: true`: policy governs *runtime* behaviour, totality
   is a *model* claim, and conflating them makes the terse case noisy.)
3. **Content-hash algorithm for `snap.v`, and the bindings gap** — canonical
   `canon` text hashed how? (Leaning fnv64 over the canonical form, the kg
   pipeline's digest precedent; the field is opaque either way.) And is the
   author-bumped `version:` field (§3.3.9) enough to signal
   binding-behaviour changes, or should `define` accept an explicit
   implementation-version that folds into `v`? (Leaning `version:` alone:
   an impl-version parameter is one more thing to forget, and it cannot be
   verified against the code either.)
4. **Outcome record shape from `State.serve`** — is `{state status}` enough,
   or should it carry the effect names executed for observability? (Leaning
   minimal now; `call {op: "status"}`-style introspection can come with the
   services consolidation.)
5. **Timer events colliding with the alphabet** — must `after:` events be
   declared in `events:` like any other (current spec: yes, they are ordinary
   events), or auto-declared? (Leaning declared explicitly: one alphabet, no
   hidden members.)
6. **Shallow history in phase 2** — adopt iff it falls out of the SCXML
   algorithm without new semantics; deep history is declined outright.
   (Leaning yes-if-free.)
