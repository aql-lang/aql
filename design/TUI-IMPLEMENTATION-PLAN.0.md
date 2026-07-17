# TUI-IMPLEMENTATION-PLAN

The executable slice of `TUI.0.md` — the `aql:tui` module rollout,
verified by the `design/examples/tui/` todo probes graduating to real,
test-driven apps (the `NETWORK-IMPLEMENTATION-PLAN.0.md` discipline:
phase by phase, each landing green, acceptance = driven apps rather than
per-phase checklists).

**Ratification.** The RFC's eight-fork decision block and all seven §11
open-question leanings are **ratified as-is** at plan time (owner
decision, 2026-07-17). Implementation proceeds with no open design
questions; anything that unravels in practice comes back as an explicit
divergence note here (§3b) or an RFC amendment, never a silent drift.

## 0. Design review — what is settled, what this slice takes

Everything the driver and module need already exists and is exercised by
shipped code; this plan invents no new architecture.

- **The actor substrate is DONE.** `eng/go/process.go` ships
  `NewProcess` / `(*Process).Send` / `PopFront` / `Close` / `Done` and
  `ProcessRuntime.Insert` / `RegisterName` / `Whereis` — the P3 driver
  loop is `serve-raw`'s connection-actor choreography
  (`lang/go/modules/net_socket.go`) retargeted at a terminal.
- **The callback seam is DONE.** `eng.InvokeCallback`
  (`eng/go/invoke.go`) is the single VM-first/CallAQL-fallback seam every
  native callback word dispatches through; `update`/`view` ride it
  unchanged.
- **The host-registration pattern is DONE.** `RegisterHostParser`
  (`lang/go/modules/parselang.go`) is the template `RegisterHostTui`
  instantiates — with one simplification: TUI words resolve the backend
  at **dispatch time**, so there is no pre-import replay step to copy.
- **Policy gating is DONE** (`native.HostPolicy`,
  `lang/go/native/capabilities.go`; template `checkNetPolicy`) — but the
  **`terminal` scope itself is new**: unlike `network`/`process` it did
  not pre-exist in `lang/go/policy/policy.go::KnownScopes` or the
  profile files. P1 adds it (§2).
- **The module scaffold is DONE**: trivial-delegation wrappers
  (`lang/go/modules/wrap.go`, inner sigs `BarrierPos: -1`), the
  `modules.go` map, catalog/docs/fixedid gates.
- **Terminal primitives are vendored**: `cmd/go/go.mod` carries
  `charmbracelet/x/ansi` (direct), `golang.org/x/term` (direct), and
  `charmbracelet/x/cellbuf` (indirect — P3 promotes it). Not bubbletea's
  `Program` (ratified fork 7).
- **The acceptance apps are pre-written**: `design/examples/tui/`
  (todo-tui, todo-tui-client, todo-tui-served + the speculative
  `TestAppTodoTUI`), authored against the RFC before any code — probe
  finding F2 (frame history) already shaped the P1 `VirtualBackend`.

## 1. Scope decisions for this slice

In scope (implemented, tested — P1 lands with this document):

1. **`lang/go/tuikit`** — the leaf seam package (zero terminal deps):
   `Backend`/`OpenOpts`/`Info`/`Event`, `Frame`/`Cell`/`Style`,
   `VirtualBackend` with scripted input (`Inject`), failure knobs
   (`OpenErr`/`CloseErr`/`SizeErr`/`PresentErr`/`TitleErr`/`BellErr`),
   and the **bounded frame history** (`FrameCount`/`ScreenAt`) that
   probe finding F2 requires.
2. **The nine Tier-1 words** (`lang/go/modules/tui.go`): `open close
   dims read-event print-at clear show title bell` behind
   trivial-delegation wrappers under namespace `Tui`, plus the exported
   `Terminal` type literal. Double-buffered drawing (print-at/clear →
   offscreen grid; show → `Present`, backend owns diffing).
3. **The `Terminal` handle**: `Ideal/Terminal`, FixedID **5011** at
   landing time (IDs drift — the network plan's 5005–5008 shipped as
   5007–5010; the snapshot in `lang/go/test/fixedid_stability_test.go`
   is the authority, not this prose). Close latch (`atomic.Bool`),
   pointer-identity `Equal`, `Terminal<TM_… closed>` `Format`.
4. **`RegisterHostTui`** (`TuiSpec{Name, Open}`) on registry capability
   slot `engine.tui.host`; one backend per registry; dispatch-time
   resolution; no backend → the first-class
   `no_backend "no terminal backend registered"` error.
5. **The `terminal` policy scope**: `KnownScopes` entry; denied
   (`install: false`) in `sandbox`/`compute`/`gen` (and `read-only`/
   `client` by inheritance); allowed in `trusted`/`full`. `open` checks
   `terminal.open`; P4's `serve` adds `terminal.serve` + the existing
   `network.listen`.
6. **Gates**: `lang/spec/module-tui.tsv` (namespace binding, type
   export, no-backend arms, per-word `uncalled_function` rejections —
   live happy paths stay in Go, the `module-net.tsv` split), Go
   end-to-end tests over `VirtualBackend`, direct-handler cover tests,
   docs/catalog/fixedid rows, 100% cover-gate.

Out of scope, deferred to P2–P5: layout/widgets, the app runtime
(`run`/`serve`/`quit`/`edit`/`style`/`focusable` and widget
constructors), the real TTY backend, remote protocol + `aql attach`,
grapheme widths, mouse decoding beyond the event shape, Windows.

## 2. Concrete design points

- **Error vocabulary** (the `closed`/`timeout`/`transport` analog):
  `closed` (operations on a closed Terminal; end of input), `no_backend`
  (nothing registered), `terminal` (backend operation failures, via
  `mapTuiErr`), `tui_error` (usage: wrong handle, malformed options or
  style). `read-event`'s deadline is **not** an error — it returns
  `None`, per the RFC.
- **Options**: `open {mouse: Boolean, title: String}`;
  `read-event {within: ms}` reuses net's `recvDeadline` helper verbatim
  (absent options fine; present-but-malformed is a hard error).
- **Event maps**: exactly TUI.0.md §2.4 minus `init` (Tier-2): tagged
  maps, `key/resize/mouse/paste/focus`, top-level scalar discriminators.
- **The `VirtualBackend` contract**: every `Present` is recorded (clone)
  into a bounded ring (256); `Screen()`/`CellAt` read the current grid,
  `FrameCount()`/`ScreenAt(i)` read the history. Failure knobs make
  every backend-error arm reachable on a terminal-less CI.
- **Sub-registry invariants**: every inner sig `BarrierPos: -1` (the
  zero-arg `open` overload is `BarrierPos: 0`, the `math-pi` precedent);
  statement words carry `ReturnsFn: tuiNoReturns`.

## 3. Phases (each lands with tests, fmt/vet/lint/test + cover-gate green, committed)

- **P1 — seam + Tier 1 (this landing).** `lang/go/tuikit/` (backend.go,
  frame.go, virtual.go + tests), `lang/go/modules/tui.go` +
  `docs_tui.go` + `tui_test.go` + `tui_cover_test.go`,
  `lang/spec/module-tui.tsv`, rows in `modules.go` /
  `help_render.go::moduleCatalog` / `fixedid_stability_test.go`,
  `terminal` scope in `policy.go` + `profiles/*.jsonic`.
- **P2 — layout/render core.** In `tuikit`: the constraint solver
  (fixed/pct/fit/flex, largest-remainder), widget measure/paint for the
  nine widget shapes, style inheritance/degradation tables, the single
  grapheme-width source, `DiffFrames`. Pure Go, table-driven
  tree→Frame goldens (CJK/emoji rows); no engine involvement.
- **P3 — the Tier-2 app runtime.** `Tui.run`/`Tui.quit` (driver on a
  real `Process`: input pump → mailbox → drain/coalesce →
  `InvokeCallback(update/view)` → layout → present), the widget
  constructor words + `edit`/`style`/`focusable`, Ctrl-C/Ctrl-\ policy,
  restore guarantees (the three `//covergate:allow` recover arms arrive
  here, not before); `cmd/go/internal/termback` on x/term + x/ansi +
  x/cellbuf (promoted to direct) with TEST-SEAMS-style seams, registered
  in the run/REPL wiring. Deliverable: the §1/§5 RFC examples run
  locally; `design/examples/tui/todo-tui.aql` graduates to
  `design/examples/apps/` + `TestAppTodoTUI` in `lang/go/test/`.
- **P4 — remote.** The `Renderer` seam refactor, `Tui.serve` (tree
  protocol over json-lines, token auth, one viewer, disconnect-quits),
  `aql attach` (`cmd/go/internal/attach`), loopback tests;
  `todo-tui-served.aql` graduates.
- **P5 — polish.** Full TSV for the Tier-2 surface, docs, examples,
  keystroke-storm benchmark (the render-on-change check), retro notes
  into §3b, `todo-tui-client.aql` graduates once P3+P4 both hold.

## 3b. Outcome (P1 landed — status as of this commit; later phases append here)

**P1 landed in full**: `tuikit` (100% covered), the nine words, the
handle, the seam, the scope, all gates green. Divergences found
necessary during implementation, recorded per the network plan's habit:

- **Statement-form returns, not `-> None`.** RFC §4.5 wrote
  `close/print-at/clear/show/title/bell -> None`; shipped as
  **returns-nothing statement words** (`ReturnsFn: tuiNoReturns`),
  matching the landed net divergence for `send-bytes`/`close` ("send
  returns nothing — statement form"). Rationale: a draw loop calling
  `print-at` per row would otherwise stack a `None` per call.
  `read-event`/`dims`/`open` return values as specified; `read-event`'s
  deadline `None` is unchanged.
- **`ScreenAt(i)`, not `Screen(i)`.** The RFC amendment's
  `Screen() []string` (current grid) and `Screen(i)` (history) cannot
  coexist in Go; shipped as `FrameCount()`/`ScreenAt(i)` beside
  `Screen()`. RFC §4.4/§8.1 and the probe corrected in the same commit.
- **Rune-per-cell in P1.** `print-at` paints one rune per cell, width 1;
  grapheme clustering and wide-rune widths are the P2 width source's
  job. Recorded so nobody mistakes P1's `print-at` for the final
  measurement story.
- **No registration replay.** Unlike parselang, `RegisterHostTui` keeps
  no pre-import spec queue and no live-module handoff: words resolve the
  backend at dispatch time, so pre- and post-import registration are the
  same code path. One backend per registry, second registration errors.
- **`open` failure semantics**: `spec.Open()` and `backend.Open()`
  failures both surface as `terminal` errors; nothing to close on
  failure (the handle is only minted after a successful open).
- **`terminal` scope profile edits** turned out to span five files
  (`policy.go` + `sandbox`/`compute`/`gen`/`trusted` jsonic; `client`
  and `read-only` inherit sandbox's denial; `full` is allow-all) — the
  absent-scope-defaults-to-installed rule makes the *deny* edits the
  load-bearing ones.

## 4. Follow-ups this plan leaves open

- The §11.1 revisit (patrun-clause `update` / service-shaped apps
  inheriting `wrap`/`prior`) once `SERVICES.0.md` lands its surface —
  the probe's F1/F4 evidence is the input.
- Multi-viewer `Tui.serve`, reattach/persistence, tree-diff and
  cell-mode wire encodings, the xterm.js backend (wpg), image
  protocols — all flagged v2 in RFC §9.
- Whether `read-event` should gain an `{active: true}`-style
  mailbox-delivery mode for Tier-1 users once P3's pump exists
  (the net passive/active split, revisited for terminals).
