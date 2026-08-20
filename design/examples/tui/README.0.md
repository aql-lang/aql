# Todo over TUI — a DX probe for the `boru:tui` design

> **GRADUATED (implementation plan P5).** The probe apps this README
> describes were rewritten onto the landed `boru:tui` surface and moved
> to `../apps/` as working, tested examples:
>
> - `todo-tui.boru` + `todo-tui-served.boru` → **`../apps/todo-tui.boru`**
>   (one module: `TodoTui.run` / `TodoTui.serve` over the same app map),
>   driven by `TestAppTodoTUI` (`lang/go/test/app_todo_tui_test.go`, a
>   scripted virtual terminal) and `TestTuiServeGraduatedTodoApp`
>   (`lang/go/modules/tui_serve_app_test.go`, a real wire viewer).
> - `todo-tui-client.boru` → **`../apps/todo-tui-client.boru`** (the
>   REST-backed client against the real `todo-api.boru`), driven by
>   `TestAppTodoTUIClient` end to end over loopback HTTP.
> - `todo-tui_test.go` (the speculative harness sketch) → superseded by
>   the real tests above; the frame-history contract it demanded (F2)
>   shipped in `tuikit.VirtualBackend`.
>
> What the graduation changed, spelling-wise (the probe's guesses vs the
> landed language): the guard-list `case` became zone-routing `if` +
> value-dispatch `case` (matched value pushed — blocks open with
> `drop`); `append` (flex-only) became `push`; bare `min`/`max` became
> `MathUtil.*` (`boru:math-util`); `filter`/`each` take quotation bodies
> (`filter [ dot done ] xs`); `parse "json"` / `emit "json"` became
> `parse json` / `emit json` (atom kinds, `boru:parselang` /
> `boru:emitlang`); `sub a b` forward-form arithmetic became infix
> (`a sub 1`); a `help` local collided with the built-in help system;
> the app map is bound with `def` before returning (body locals tear
> down before a trailing literal evaluates); statement-position `spawn`
> and `Net.close` calls need their residue dropped / a `;` terminator.
> The findings below stand as the historical record.

This folder was a **developer-experience experiment**, not runnable code. It
rewrites the repo's canonical validation app — the todo list of
`../apps/todo-api.boru` (the `NETWORK-SERVERS.0.md` §6.4 verification app,
validated end-to-end by `TestAppTodoAPI` in `lang/go/test/apps_test.go`) —
against the proposed `../../TUI.0.md` surface — words that **do not exist
yet** — to answer the two questions an RFC cannot answer about itself:

1. **Does the architecture express a real app cleanly?**
2. **Which of the decision-block forks (1–8) and §11 open questions
   actually bite when you build with it?**

Everything compiles only in the imagination. `Tui.*` words are the RFC's;
non-Tui spellings (`each`, `append`, list-index `get`, `parse "json"` /
`emit "json"`, `/r` references as map values) are illustrative and would be
confirmed against `boru describe`. The value is the *shape* of the code, not
its execution.

> The `.0` suffix follows the repo convention: design-only, implementation
> completeness 0 (see `../../IMPLEMENTATION-STATUS.10.md`).

---

## The app

The same todo domain as the REST server — `{id text done}` rows and a
`next` id counter that never reuses ids — driven by key events instead of
HTTP verbs, then composed *with* the real REST server, then served to a
remote viewer. Small, but it touches every layer of the design:

| File | Layer | What it demonstrates |
| --- | --- | --- |
| `todo-tui.boru` | **§2 loop + §3 widgets** | the §6.4 CRUD contract as a key-driven `update`/`view` pair; focus-as-state across two zones; `Tui.edit`; constraint layout; quit-as-a-value returning the final state |
| `todo-tui-client.boru` | **§2.3 effects + `boru:net`** | the same UI as a *client of the real `todo-api.boru`*: every mutation is a `spawn`ed HTTP round trip that `send`s results home; the uniform failure contract surfaces on the status line |
| `todo-tui-served.boru` | **§6 remote tier** | the same app **map** under `Tui.serve` + a `boru attach` transcript; v1 session rules (token, one viewer, disconnect-quits) |
| `todo-tui_test.go` | **§8 testing** | the speculative graduation of `TestAppTodoAPI`: same mem-FS harness + `"|"`-joined final-state assertions, with scripted key events replacing scripted HTTP calls and golden *frames* replacing golden bodies; negatives paired throughout |

### Event lifecycle (a `space` toggle, standalone)

```
key press           driver (native Go)                      app (boru, pure-ish)
---------           ------------------                      -------------------
space ──decode──▶ Backend.Events() ─▶ input pump ─▶ mailbox (real Process)
                                                    └▶ PopFront ─▶ update state {tag:"key" key:"space"}
                                                                   └▶ toggle-sel → state'
                                                    └▶ drain (empty — no storm)
                                                    └▶ view state' ─▶ widget tree (plain data)
                                                                      └▶ layout ─▶ diff ─▶ Present
```

The client variant inserts a process hop: `enter` → `update` spawns a
worker (state marked `syncing`, one paint) → worker `POST`s then re-fetches
then `send {tag:"loaded" …}` → the message re-enters the same mailbox → a
second `update`/paint. The served variant replaces the two ends:
`boru attach` encodes the key as a json-line up; the tree comes back as a
json-line down and is laid out client-side. **The app file is identical in
all three placements** — that is the §0 sendable-data invariant doing its
job, and it held.

---

## DX findings, mapped to the decision block and §11

Honest verdicts from writing the code. `**Resolved:**` marks findings that
led to `../../TUI.0.md` amendments in this change.

### F1 — Focus routing is the app's least pleasant code (fork 2, §3.4, §11.1)
**Verdict: focus-as-state works, and costs exactly what the RFC said it
would.** Two zones cost: a `focus` field, a `tab` clause, a zone guard on
*every* routed key (`[(state.focus eq "entry") and (ev.key eq "enter")]`),
and the view deriving the widget's `focus:` flag. Nothing was impossible
and `view` stayed pure — but the zone-guard boilerplate in `update` is the
first thing a widget-toolkit user will notice. It is also *exactly* the
shape patrun routing already handles (`{key:"enter" focus:"entry"}` as a
clause pattern), which turns §11.1 (service-shaped apps) from a nicety into
the natural fix. **Resolved:** §11.1's leaning now cites this probe as
evidence for the revisit.

### F2 — The validation test cannot exist without frame history (fork 7, §8.1)
**Verdict: the RFC's `VirtualBackend` as first drafted was un-testable in
the one way that matters.** After quit the terminal is *restored* — the
final screen is empty by design — so every meaningful golden
(`[x] walk dog`, the `todos 1/2` box title) is a **mid-run** frame.
`Screen()`/`CellAt` over the current grid cannot see them.
**Resolved:** §4.4/§8.1 now specify that the virtual backend records every
`Present` into a bounded frame history (`FrameCount()`/`ScreenAt(i)` —
`Screen(i)` as first amended could not coexist with `Screen()` in Go), and
the goldens in `todo-tui_test.go` are written against it.

### F3 — "Export the app map" is the reuse idiom, and needed saying (fork 5, §5)
**Verdict: one sentence of RFC prevents a whole class of un-servable
examples.** `Tui.run` and `Tui.serve` both take the `{init update view}`
map; an example exporting only a `run`-shaped word cannot be served.
`todo-tui.boru` exports `app:` (the map constructor) *and* `run:` (the local
sugar); `todo-tui-served.boru` is then a one-word change.
**Resolved:** §5.3 now states the idiom explicitly.

### F4 — No `wrap` analog for cross-cutting event observation (§11.1)
**Verdict: the REST app's best trick has no TUI counterpart yet.**
`todo-api.boru` counts every request in a three-line `wrap` with zero
handler edits; mirroring a `hits` counter in the TUI means editing
`update` (or hand-wrapping the update word). Livable in v1 — a manual
wrapper is three lines of boru — but it is the same itch §11.1 scratches:
a service-shaped app would inherit `wrap`/`prior` for free. Folded into
F1's §11.1 amendment; no separate change.

### F5 — Handle capture into workers is assumed, not yet specified (client)
**Verdict: the client leans on it four times.** `state.ep` (a `Net`
`Endpoint`) and `ev.ui` (a `Pid`) are captured into `spawn` bodies. `Pid`
is documented sendable; `Socket` is sendable-by-handle
(`NETWORK-SERVERS.0.md` §4.1); an `Endpoint` (a Service value wrapping a
socket) is *presumably* in the same handle class — but no document says
so. Needs a one-line confirmation on the `NETWORK-CLIENTS.0.md` /
`SERVICES.0.md` side. Recorded, not resolved here (out of this RFC's
scope).

### F6 — Client-side JSON reply bodies decode by hand (client)
**Verdict: an asymmetry the TUI merely trips over.** Server-side, the
`http` codec decodes request bodies to `req.body-json`; client-side, a
`call` reply's `r.body` is JSON *text* — `TestAppTodoAPI` itself
string-greps it, and the TUI client must `parse "json"` by hand.
Symmetric reply-body decode is a codec-layer gap for the networking docs.
Recorded, not resolved here.

### F7 — The `init` event earned its keep (§2.4)
**Verdict: validated in both directions.** The client needs exactly what
`{tag:"init"}` delivers — the UI's own pid, before any worker can be
spawned (the caller is blocked inside `Tui.run`, so `self` could never
supply it). And in the standalone app, where init has no work to do, it
falls harmlessly through the `(ev has "key") not` guard — mandatory
lifecycle traffic imposed zero ceremony. Worker messages arriving
*verbatim* (`loaded`/`sync-error` as ordinary clauses) also read exactly
as the RFC promised.

### F8 — UI state wants an ordered list; the server keeps a map
**Verdict: expected divergence, no RFC change.** `todo-api.boru` keeps
todos as an id-keyed map with `None` tombstones (transport- and
patch-friendly); a cursor-driven UI wants a stable *ordered* list. The
standalone app keeps a list plus the same `next` counter; the client
simply adopts the server's list-shaped `GET /todos` response. Worth a
"model your UI state as ordered collections" note in the eventual module
docs, nothing more.

---

## What felt genuinely good

- **Quit is a value.** `Tui.quit state` threading out through nested
  `case` arms, and `Tui.run` *returning* the final state, made the app an
  expression — the test asserts on its result exactly like the REST test
  asserts on response bodies, and `(TodoTui.run {}).todos` composes
  mid-pipeline with no ceremony.
- **The worker idiom transplants unchanged.** The client's
  spawn → HTTP → `send {tag:"loaded"}` loop is the `PROCESSES.0.md` shape
  with zero TUI-specific adaptation; sync status on screen fell out of two
  ordinary state fields. Update never blocks; the screen never froze *by
  construction*.
- **One app, three placements.** Standalone, REST-backed, and served
  needed changes only at the edges (which words construct the state, which
  word runs the map). The widget-tree-as-sendable-data invariant is the
  whole reason, and the probe never fought it.
- **The test is the old test.** `TestAppTodoTUI` is `TestAppTodoAPI` with
  key events for HTTP calls and frames for bodies — the validation
  *pattern* survived the transport swap, which suggests the module will
  slot into the existing acceptance-app discipline without new harness
  machinery.

## Cross-references

- App model, event vocabulary, quit/teardown: `../../TUI.0.md` §2
- Widget trees, constraints, focus-as-state: `../../TUI.0.md` §3
- Backend seam + `VirtualBackend` (incl. the F2 frame history): `../../TUI.0.md` §4, §8.1
- Remote tier (`Tui.serve` / `boru attach`): `../../TUI.0.md` §6
- The mirrored REST app + its live test: `../apps/todo-api.boru`,
  `lang/go/test/apps_test.go` (`TestAppTodoAPI`)
- Actor substrate the workers ride: `../../PROCESSES.0.md` §3–4, §6
- The aspirational-probe convention this folder follows:
  `../todo/README.0.md`
