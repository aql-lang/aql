# Networking + TUI verification apps (working examples)

Working AQL programs that verify the networking stack — the acceptance
apps of `design/NETWORK-IMPLEMENTATION-PLAN.0.md` §1.5 — and, since the
`aql:tui` landing, the terminal-UI stack (`design/TUI.0.md`,
graduated from the `../tui/` probe by implementation-plan P5).
Unlike the aspirational sketches in `design/examples/todo/` (written
against the RFCs before implementation), everything here **runs today**
and is exercised over real loopback sockets by
`lang/go/test/apps_test.go` — the TUI apps over a scripted virtual
terminal by `lang/go/test/app_todo_tui_test.go` and over a real wire by
`lang/go/modules/tui_serve_app_test.go` (plus `aql:repl`, which
ships as a native module implemented in AQL — `lang/go/modules/repl.go`).

| File | Tier | What it proves |
| --- | --- | --- |
| `todo-api.aql` | high (codec) | REST CRUD over `Net.http`: patrun routes on method+path, `:id` templates → `req.params`, JSON bodies via `req.body-json`, `wrap` middleware, tombstone deletes. |
| `todo-tui.aql` | TUI (§2 loop + §6 remote) | The same todo domain as a full-screen TUI: `update`/`view` over a real process mailbox, focus-as-state across two zones, `Tui.edit`, quit-as-a-value — and the SAME app map under `TodoTui.serve` for a remote `aql attach` viewer. |
| `todo-tui-client.aql` | TUI (§2.3 effects) | The TUI as a *client of the real `todo-api.aql`*: every mutation is a `spawn`ed HTTP round trip that `send`s results home as mailbox messages; the status line tracks sync state; update never blocks. |
| `mini-redis.aql` | high (custom codec) | The common Redis commands (strings, lazy expiry, KEYS, lists, hashes) over a **custom AQL codec** — the NETWORK-SERVERS.0.md §6.6 extension point; RESP-flavoured replies; the client side is plain `Net.lines`. |
| `mini-s3.aql` | low (raw sockets) | A basic S3-style object store with hand-rolled HTTP over `recv-until`/`recv-bytes`: bodies **stream** in bounded 64 KiB chunks both ways, uploads are **resumable** (`HEAD` → `x-size` resume point, `PUT x-offset` with 409 on a wrong offset), downloads resume via `Range` → 206. |
| `mini-s3-client.aql` | low (raw sockets) | The matching hand-framed client (`dial`/`req`). |

## AQL patterns these apps pinned down

Hard-won rules for writing socket apps in today's AQL (each one bit
during development):

- **Statement residue is real.** A flex `set` returns its receiver;
  follow every statement-position `set` with `drop`. An fn body's
  return values are whatever the body leaves on the stack — loop
  iterations must leave *nothing*.
- **`break` cannot cross an fn-body `for` today** — it kills the rest
  of the fn body. Open-ended loops inside fns recurse instead
  (`s3-conn`, header readers); bounded chunk loops compute their
  iteration count up front.
- **A trailing map literal referencing body locals evaluates too
  late** (after local teardown) — bind it first: `def out {…} out`.
- **`set` quotes a bare-word key**: `m set id v` writes the key
  `"id"`; a computed key needs parens — `m set (id) v`.
- **Dotted paths don't ride mid-argument-list**: bind first
  (`def k req.key`) or parenthesise (`(req.line)`).
- **Store expiries (and any cross-dispatch bookkeeping) as scalars**
  (epoch-ms Integers), and remember `a div b` / `a sub b` compute in
  swap form (`100000 div 1000` = 100).
- **`filter` predicates receive `{key value}` entry maps**, not bare
  elements; the result carries the original elements.
- Avoid shadowing built-ins in fn locals (`all`, `base`, `min`,
  `take`, `state`, …) — params/locals cannot shadow registered words.
