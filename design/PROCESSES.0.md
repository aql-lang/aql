# PROCESSES

Design for BEAM-style **processes & actors** in AQL — phase 1, core actors.

## Context

The end goal is to make AQL capable of writing **highly efficient and safe
network servers and clients** — primarily JSON APIs, eventually binary wire
protocols. The most battle-tested architecture for that problem is the Erlang
**BEAM**: huge numbers of cheap, isolated processes that communicate only by
asynchronous message passing, with selective receive and supervision on top. The
canonical server shape — **one lightweight process per connection**, each
blocking on a mailbox and pattern-matching the messages it cares about — is
what makes high-concurrency network code both efficient and easy to reason about.
(We adopt this shape but, unlike BEAM, with **bounded** mailboxes and
**pattern-matched dispatch** rather than skip-and-save selective receive — see §1.)

AQL today is a concatenative data/query language with only **fork-join**
concurrency (`await`, `timeout`, `interval` in
`lang/go/native/native_temporal_await.go`). It has no long-lived process, no
mailbox, and no message passing. But it already owns the *hardest* piece of a
BEAM-like runtime:

- **`Registry.ForkConcurrent()`** (`eng/go/fork.go`) hands each goroutine an
  isolated copy of all mutable evaluation state — defs, contexts, args,
  flow-control — while sharing the read-only infrastructure (type lattice,
  ideals, capabilities, modules, native registrations, parser). This is the
  direct analog of a BEAM **per-process heap**.
- **patrun** (`lang/go/native/internal/patrun/`, wired up in
  `lang/go/native/native_patrun.go`) is a trie pattern matcher that is a natural
  engine for **pattern-matched message dispatch** (and, opt-in, selective receive).
- AQL values are **immutability-first** (`eng/go/clone.go`): scalars, plain
  `List`, plain `Map`, and type/function values are never mutated in place. Only
  `Object`, `Array`, and `Store` are mutable. Immutable values can therefore be
  shared between goroutines with no copy and no race — exactly the property a
  message-passing runtime needs.

What is missing is a *long-lived* process primitive, mailboxes, `send`/`receive`,
PIDs, and a named process registry. This document specifies them.

This is a **design RFC only — no implementation code yet**, matching how other
subsystems were designed first (`STREAM-WORDS.0.md`, `PERMISSIONS-PLAN.10.md`).

### Relationship to `aql:stream`

`STREAM-WORDS.0.md` designs a *module* for back-pressured, bounded pipelines
(fan-out/fan-in over data). This document is complementary and lower-level:
unbounded, long-lived, individually-addressable **actors** as **core** language
words. Streams are about data flow; processes are about concurrent state and
message protocols. A future network server uses both — actors own connections,
streams move bytes.

### Relationship to `SERVICES.0.md`

`SERVICES.0.md` designs the **primary developer experience** — a unified
**service / server** model (a service owns state and answers pattern-matched
`call`/`send` requests; a server is a supervised collection of services) — *on
top of* this substrate, and folds the CLI's existing servers (`registry`/`lsp`/
`exec`/`api`/`tui`/`vault-proxy`) into it. The mapping is direct: a served
**service** is an actor whose `receive` loop *is* its handler patrun (consume +
dispatch), and `call` is `send` + await-reply on a dedicated channel. This
document is the low-level layer (raw
processes/mailboxes, like Erlang processes); `SERVICES.0.md` is the high-level
layer most code is written against. The two are designed together.

### Scope decisions (agreed)

1. **Phase 1 = core actors only.** Lightweight processes, PIDs, a named process
   registry, asynchronous `send`, and a **bounded** mailbox with **pattern-matched
   dispatch via patrun** (selective receive demoted to a bounded opt-in, §3).
   Explicitly **out of scope** for phase 1: links/monitors,
   supervisors and restart strategies, `gen_server`-style behaviours, TCP/HTTP
   *servers*, and binary wire-protocol support. These are later phases
   (see [Roadmap](#9-phased-roadmap)).
2. **Packaging = core language words.** `spawn`, `self`, `send`, `receive`,
   `register`, `whereis`, `unregister` sit in the core word set alongside
   `await` — not behind an `import`. Process concurrency is a first-class
   language feature.
3. **Message isolation = immutable-only.** Messages must be immutable values.
   `send` rejects mutable containers rather than deep-copying them. This keeps
   sends zero-copy while preserving BEAM's "no shared mutable state" guarantee.
4. **Standard Go throughout.** Processes are goroutines; mailboxes use
   `sync`/channels; shutdown uses `context.Context`. We do **not** build a
   reduction-counting scheduler — the Go runtime scheduler is the scheduler.

## 1. Motivation & end-goal

The target programming model is **actor-per-connection + pattern-matched
dispatch**:

- A listener process accepts connections and `spawn`s a handler process per
  connection.
- Each handler is a loop that `receive`s — matching each incoming message (a
  parsed JSON request, a timer tick, a shutdown signal) against patrun clauses
  and running the matched body.
- State lives *inside* the process (as ordinary AQL bindings in its forked
  registry), never shared, so there are no locks in user code.

> **Dispatch, not selective receive (decided).** The primary `receive` consumes
> the **front** message and dispatches it by patrun clause — it does *not* scan
> past and save non-matching messages. True Erlang **selective receive** (skip +
> save) is demoted to a bounded, opt-in escape hatch (§3), because (a) an
> unbounded save-set defeats the bounded-mailbox backpressure that `SERVICES.0.md`
> §8.1 relies on, (b) it is the source of the O(n²) mailbox-scan footgun, and
> (c) its canonical use — synchronous call/reply — is handled structurally by a
> dedicated per-call reply channel, not a mailbox scan. The pattern-matching
> *receiver* idea is fully retained; only the skip-and-save part is opt-in.

This RFC delivers the concurrency substrate for that model. The networking and
binary-codec layers that complete the end-goal are scoped as later phases and
enumerated honestly in the [gap analysis](#8-gap-analysis). AQL's stated stance
is that it "is a query/data language, not a systems language"
(`BATTERIES-INCLUDED-REPORT.5.md`); actors are introduced as *controlled,
capability-gated* concurrency, not as an invitation to write arbitrary systems
code.

## 2. BEAM → Go mapping

The architecture maps each BEAM fundamental onto a standard Go construct plus an
existing AQL primitive. Nothing here requires a custom scheduler or a new memory
model.

| BEAM concept | Go / AQL realization |
| --- | --- |
| Lightweight process | a goroutine running an AQL body on its own `ForkConcurrent()` registry |
| Per-process heap isolation | forked registry + **immutable-only messages** (no shared mutable state crosses a `send`) |
| PID | a new opaque **`Pid`** Ideal value (precedent: `Timeout`/`Interval` Ideals) wrapping a `*process` handle |
| Mailbox | per-process goroutine-safe queue: a slice guarded by a mutex/`sync.Cond`, **bounded** with a configurable overflow policy (`SERVICES.0.md` §8.1; BEAM's are unbounded — we deliberately diverge) |
| Async send (`!`) | the `send` word — enqueue into the target's mailbox; blocks / fails / drops on a full mailbox per the overflow policy |
| Pattern-matched receive | the `receive` word — consume the front message and dispatch it by **patrun** clause; selective (skip + save) receive is a bounded opt-in (§3) |
| Process registry (named procs) | a shared `*ProcessRuntime` pointer on `Registry` (auto-shared by `ForkConcurrent`'s shallow copy) holding a mutex-guarded `name→*process` map and a `pid→*process` table |
| Preemptive scheduling | the Go runtime scheduler (note: no per-process *fairness* guarantees — acceptable for phase 1) |
| "Let it crash" | phase 1 has no supervision, but every process goroutine **must `recover()`** so a crash terminates only that process (logged), never the host |
| Process exit / cleanup | goroutine returns → mailbox closed, table + name entries removed |
| Shutdown / cancellation | a **`context.Context`** on `ProcessRuntime` (new to AQL) so the host can cancel all processes on exit and avoid goroutine leaks |

The key enabling fact: `ForkConcurrent()` does a **shallow struct copy**
(`fork := *r`) and then replaces the *mutable* fields with isolated clones. Any
**pointer** field left untouched by the copy is therefore shared by every
process automatically. The process runtime is exactly such a pointer field — one
runtime, many forked registries pointing at it, no extra plumbing.

## 3. Runtime design (Go)

### `ProcessRuntime` (new, `eng/go`, alongside `fork.go`)

One per top-level engine, shared by every fork. Owns:

- `table` — `map[string]*process` keyed by pid id, guarded by a `sync.Mutex`
  (or `sync.RWMutex`).
- `names` — `map[string]*process` for the named registry, same lock.
- `ctx context.Context` + `cancel context.CancelFunc` — the root cancellation
  scope; cancelling it unblocks every `receive` and tears down all processes on
  host shutdown.
- id allocation via the existing `GenerateID("P_")` helper (used today by
  `Timeout`/`Interval`).

It is stored as a **pointer field on `Registry`** (`Procs *ProcessRuntime`,
`eng/go/registry.go`), lazily created on first `spawn` so programs that never use
processes pay nothing and start no goroutines. Because `ForkConcurrent` shallow-
copies, child forks share the same `*ProcessRuntime` with no change to
`fork.go`'s clone logic.

### `process` (new, `eng/go`)

```
type process struct {
    id      string            // "P_…"
    mailbox []Value           // bounded FIFO, oldest first (cap = bound)
    bound   int               // mailbox capacity; overflow policy on the process
    mu      sync.Mutex
    cond    *sync.Cond        // signals new arrivals to a blocked receive
    reg     *Registry         // the forked registry this process runs on
    done    chan struct{}     // closed when the process exits
    name    string            // registered name, "" if none (for cleanup)
    rt      *ProcessRuntime   // back-reference for deregistration
}
```

The mailbox is a slice + `sync.Cond` rather than a Go channel for two reasons:
the **configurable overflow policy** (`'block`/`'fail`/`'drop`, `SERVICES.0.md`
§8.1) needs more than a buffered channel's block-only behaviour, and the **opt-in
selective receive** (§3) needs to inspect-and-leave, which a channel cannot do.
The common path is cheap: `send` locks, enforces the bound, appends, and
`cond.Signal()`s; `receive` locks, takes the head, and `cond.Wait()`s when empty.
Synchronous call/reply does **not** go through the mailbox — the reply travels on
a dedicated per-call channel (so a caller never scans its own mailbox for a reply).

### spawn flow

1. On the **caller's** goroutine (honouring the `ForkConcurrent` contract — the
   fork must be created before any concurrent execution and never races parent
   mutation), call `r.Procs.spawn`.
2. `ForkConcurrent()` the caller's registry; wrap shared output in `SyncWriter`
   (as `await` already does) so concurrent process prints don't interleave.
3. Allocate the id, build the `process`, insert into the table.
4. Launch the goroutine with a `defer recover()` that logs the crash, then runs
   cleanup (close `done`, drop from table + names).
5. Return the `Pid` value to the caller.

This deliberately mirrors the existing goroutine/fork patterns in
`lang/go/native/native_temporal_await.go` (`makeBranchForks`,
`runParallelBranch`) and the timer-fork pattern in `native_misc.go`
(`doTimeout` forks on the dispatcher goroutine before scheduling).

### mailbox + dispatch (the core algorithm)

`receive` is given an ordered set of clauses, each a `{pattern}` + a body, plus
an optional `after <ms>` body. The primary semantics are **consume-front +
dispatch** (not selective):

1. Build a patrun matcher from the clause patterns (reusing
   `native_patrun.go`'s `coercePattern` and the `internal/patrun` trie). The
   matcher maps a message's scalar fields to the most-specific clause.
2. Lock the mailbox. Take the **front** (oldest) message and match it. If it
   matches a clause, remove it and run that clause body (on the process's own
   registry) and return its result. If it matches **no** clause, run the
   catch-all `{}` clause if present, else raise `no_match` — an unmatched message
   is *not* silently saved (that is what kept BEAM mailboxes growing).
3. If the mailbox is empty: if an `after` clause's deadline has passed, run it;
   otherwise `cond.Wait()` for a new arrival (or `ctx.Done()` for shutdown). An
   `after 0` clause makes `receive` non-blocking (poll once, else fire `after`).

Because patrun keys on `map[string]string`, the **message convention is the
tagged map**: messages are maps whose discriminating fields (e.g. `cmd`) drive
the match, just as Erlang uses tagged tuples. Non-map messages are handled via a
catch-all clause (`{}` matches anything in patrun).

### selective receive (opt-in, bounded)

For raw protocol code that genuinely must take a message **out of order** (defer
non-matching messages until a later phase), `receive` accepts an explicit
`{select: true}` option that restores Erlang semantics: scan oldest-first, take
the first match, and **save** the skipped messages in a bounded save-set. The
save-set has a cap; on overflow the process raises (it is misusing the feature),
so selective receive can never silently grow memory. This is documented as an
advanced escape hatch — the idiomatic alternative is **explicit state-based
deferral** (`gen_statem`-style: stash the deferred message in process state and
handle it after the state transition), which makes the buffering visible and
bounded by construction. Most code never needs `{select: true}`.

### Lifecycle & edge cases

- **Process exit:** when the body returns, errors, or panics (recovered), close
  the mailbox, close `done`, and remove the process from the table and the name
  registry. The goroutine ends — no leak.
- **`send` to a dead or unknown pid/name:** silently dropped (BEAM semantics).
  Sending is fire-and-forget and never blocks the sender.
- **`receive` with no senders:** blocks indefinitely by design (inherent to
  actors, same as BEAM). It is unblocked by an arriving message, by an `after`
  clause, or by `ProcessRuntime` cancellation on host shutdown. Document this so
  authors add `after` where they need a timeout.
- **Bounded mailbox (diverges from BEAM):** mailboxes are bounded with a
  configurable overflow policy — `'block` (backpressure, default), `'fail`
  (raise `overload`), or `'drop` — per `SERVICES.0.md` §8.1. This is a deliberate
  divergence from BEAM's unbounded mailbox, whose unbounded growth is its classic
  overload/OOM failure mode. The bound is what makes the demotion of selective
  receive coherent: with a bounded mailbox there is no unbounded save-set.

## 4. Language surface (new core words)

All names verified collision-free against existing native word registrations.
Signatures use AQL's top-first argument convention with `describe`-ready
summaries and examples.

- **`spawn [body] -> Pid`** — start a process running `body` (a quoted list,
  not auto-evaluated by the caller); typically `body` loops on `receive`.
  Returns the new `Pid`. Capability-gated (see §7).
- **`self -> Pid`** — the current process's pid. At the top level (not inside a
  spawned process) returns `None` (or raises; see open questions).
- **`send <msg> <pid> -> <msg>`** — asynchronously enqueue `<msg>` into the
  mailbox of `<pid>`. Non-blocking. Returns `<msg>` for chaining. **Validates
  that `<msg>` is immutable** (§6) and raises otherwise. `<pid>` may be a `Pid`
  *or* a registered name (atom/string).
- **`receive [ {pat} [body] … (after <ms> [body]) ] -> result`** — take the front
  mailbox message and dispatch it by patrun clause (§3); block when the mailbox is
  empty. Returns the chosen clause's result. An optional `{select: true}` restores
  bounded selective receive (§3) for the rare out-of-order case.
- **`register <name> <pid>`** — bind a name to a pid in the shared registry.
- **`whereis <name> -> Pid`** — look up a registered name; `None` if absent.
- **`unregister <name>`** — remove a name binding.

`link` and `monitor` are **reserved** for phase 2 and intentionally not defined
here.

## 5. New `Pid` type in the lattice

Register **`Pid`** as an Ideal type, following the existing `Timeout`/`Interval`
precedent (`design/IDEAL.10.md`, the `eng/go` typetable, and the Ideal payload
machinery). Properties:

- **Opaque** — no user-visible internals; the payload wraps `*process` (or the
  id + a runtime handle).
- **Immutable** — so a `Pid` is itself a legal message payload (a process can
  send its own pid to ask for a reply: the `{cmd:'get pid: self}` pattern).
- **Comparable by id** — equality and the total value order key on the id
  string, consistent with `TYPE-ORDERING.10.md`.
- **Printable** as e.g. `Pid<P_a1b2c3>`.

## 6. Message-isolation rule (immutable-only)

`send` enforces the BEAM "no shared mutable state" guarantee by **value class**
rather than by copying, leaning on AQL's existing mutability taxonomy
(`eng/go/clone.go`, `lang/go/native/mutability_test.go`):

- **Sendable (immutable):** all scalars (Integer, Float, BigInteger,
  BigDecimal, String, Boolean, Atom, Path, None), the Time family, plain `List`,
  plain `Map`, `Pid`, type/function values — anything never mutated in place.
- **Rejected (mutable):** `Object`, `Array`, `Store`, and any typed/mutable
  variants thereof. `send` raises a clear AQL `Error` (proposed code
  `not_sendable`, message naming the offending type) and does not enqueue.

Rationale: immutable values are safe to share across goroutines by reference, so
sends are **zero-copy** and there is no race. Rejecting mutable containers (vs.
silently deep-copying or freezing them) keeps the cost model obvious and the
semantics honest. The validation is a recursive check over containers (a `List`
or `Map` containing an `Object` is not sendable).

**Non-goal for phase 1:** an opt-in `send`-with-copy (deep-copy or freeze a
mutable value into an immutable snapshot at the boundary). It is a reasonable
future ergonomic but is excluded now to keep the rule simple.

## 7. Safety & capability integration

"Safe" is half the goal, so `spawn` is wired into the existing object-capability
model (`design/PERMISSIONS.10.md`). Process creation is gated behind the
`process` hard cap (or a dedicated `concurrency` scope), so the restrictive
built-in profiles (`sandbox`, `compute`, `read-only`) **cannot spawn
processes**, while `trusted`/`full` can. Denials carry the usual blame chain
(profile name, rule, resolved scope). `send`/`receive`/registry operations act
only within an already-granted process and need no additional capability beyond
`spawn`. This composes with the sandboxed sub-engine model (`aql:vm`): a child
policy can attenuate away the process capability entirely.

## 8. Gap analysis

What this RFC adds, and what still blocks the network-server end-goal:

- **Added here:** long-lived process primitive; `Pid` type; bounded mailbox;
  `send` / `receive` (pattern-matched dispatch; bounded opt-in selective receive);
  named process registry; the first use of `context.Context` in the engine (for
  shutdown/cancellation).
- **Still missing — networking (phase 3):** AQL has **no TCP/socket server** —
  only the HTTP *client* `fetch` (`lang/go/native/fetch.go`,
  `aql:net`). A listener/acceptor primitive (Go `net`/`net/http`) is required
  for actor-per-connection servers.
- **Still missing — binary (phase ≥3):** there is **no `Bytes`/binary value
  type and no bit-syntax** (only the `aql:bin-util` integer bitwise words and
  base conversion via `convert`). Binary wire protocols need a `Bytes` payload
  and an Erlang-style binary-construction/match facility. **JSON is already
  covered** by the `Format` interface plus `jsonify`/`reify`.
- **Still missing — OTP (phase 2):** links/monitors, supervisors with restart
  strategies ("let it crash"), and a `gen_server`-style request/reply behaviour.

## 9. Phased roadmap

- **Phase 1 (this RFC): core actors.** `spawn`/`self`/`send`/`receive`/registry,
  `Pid` type, pattern-matched dispatch via patrun (bounded mailbox; selective
  receive a bounded opt-in), immutable-only messages, capability
  gating, context-based shutdown.
- **Phase 2: OTP fundamentals.** Links & monitors; supervisors with restart
  strategies (one-for-one, one-for-all, rest-for-one); a `gen_server`-style
  behaviour (synchronous call/reply built on async `send` + a correlation tag).
- **Phase 3: networking + binary.** TCP/HTTP server primitive + actor-per-
  connection; a `Bytes` value type and bit-syntax for binary framing → the
  stated efficient/safe JSON **and** binary network-server goal.
- **Later (optional): distribution.** Location-transparent `send` across nodes,
  the BEAM feature that turns actors into a distributed system.

The OTP-style **service/server DX** that rides on these phases is specified
separately in `SERVICES.0.md` (its `serve`/`server`/`proxy`/`listen`/`connect`
build on phases 1–3 here).

## 10. Worked example

A counter actor demonstrating spawn → register → tagged-map messages →
pattern-matched dispatch. (Illustrative surface; exact syntax settles during
implementation.)

```aql
# The actor body: a loop that holds its count as a binding and
# dispatches three message shapes (front message matched against the clauses).
def counter-loop fn [[n:Integer] [Never] [
  receive [
    {cmd: 'inc}            [ inc n counter-loop ]          # tail-recurse with n+1
    {cmd: 'get reply: Pid} [ send n reply  counter-loop ]  # send count to caller, loop
    {cmd: 'stop}           [ ]                              # fall through → process exits
  ]
]]

# Start it, name it, drive it.
spawn [ 0 counter-loop ]   register 'counter

send {cmd: 'inc} 'counter
send {cmd: 'inc} 'counter
send {cmd: 'get reply: self} 'counter

receive [ {} [ ] ]   # receive the reply (catch-all) -> 2

send {cmd: 'stop} 'counter
```

Note the conventions in play: messages are **tagged maps** (the `cmd` field
drives patrun matching); a process passes **`self`** so the counter can reply;
and the loop carries state purely as a binding (`n`), never shared.

## Open questions

1. **`self` at top level** — return `None`, or raise? (Leaning `None` for
   composability.)
2. **Reply correlation** — phase 1 leaves request/reply to user convention
   (`reply: self` + a `receive`). Should a minimal `call`/`reply` sugar land
   now, or wait for the phase-2 `gen_server`? (Leaning: wait.)
3. **`send` return value** — return the message (chains naturally) or the pid
   (Erlang's `!` returns the message)? (Leaning: the message.)
4. **Non-map messages** — rely on the empty-pattern catch-all, or add an
   explicit value-equality clause form to `receive`? (Leaning: catch-all is
   enough for phase 1.)
5. **Mailbox bound** — *decided:* bounded with a `'block`/`'fail`/`'drop` overflow
   policy from phase 1 (`SERVICES.0.md` §8.1), diverging from BEAM. Open sub-point:
   the default capacity and whether it is per-permission-profile.
6. **Selective receive** — *decided:* demoted to a bounded `{select: true}` opt-in;
   the default `receive` is consume-front + dispatch. Open sub-point: do we ship
   `{select: true}` in phase 1 at all, or defer it until a concrete protocol needs
   it (leaning defer)?
