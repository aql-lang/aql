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

## 3b. Outcome (P1–P5 landed — the plan is complete)

### P5 — polish: graduation, full TSV, the coalescing benchmark

**P5 landed in full.** The probe apps graduated to working, tested
examples in `design/examples/apps/`, the Tier-2 TSV grew its edit-fold
and constructor-negative rows, and the keystroke-storm benchmark pinned
the §2.2 render-on-change claim with a number. Specifics:

- **Graduation**: `todo-tui.aql` + `todo-tui-served.aql` merged into
  ONE module (`../apps/todo-tui.aql` exporting `TodoTui.app/run/serve`
  — the §5 reuse idiom is literally one export map), and
  `todo-tui-client.aql` became `../apps/todo-tui-client.aql` (spawned
  HTTP workers against the real `todo-api.aql`). Driven by
  `TestAppTodoTUI` (scripted virtual terminal, file-module import),
  `TestTuiServeGraduatedTodoApp` (real wire viewer), and
  `TestAppTodoTUIClient` (full loopback REST round trips, the test
  pacing itself on the virtual screen's sync status). The probe folder
  (`design/examples/tui/`) retired to a graduation record atop its
  findings; the probe-vs-landed spelling delta is recorded there
  (guard-list `case` → zone `if` + value-dispatch `case`, `append` →
  `push`, `MathUtil.min/max`, quotation-form `filter`/`each`, atom
  `parse json` / `emit json`, swap-form arithmetic, statement-residue
  drops and the `Net.close …;` terminator, the trailing-map-literal
  teardown gotcha, a `help` local colliding with the built-in).
- **One production change**: module sub-registries now inherit
  opted-in host-capability slots (`native.ModuleInheritedCaps`, the
  tui host key registered via init in `modules/tui.go`) — without it a
  TUI app loaded as a FILE MODULE could never see the host backend
  (`no_backend`), because `RunModuleBody` copies only a curated
  capability list. Registration must precede the importing program
  (embedders register at startup, so this is the natural order).
- **TSV**: +5 edit/`focusable` value rows (insert/left/home+delete/
  non-key passthrough/empty tree) and +10 per-constructor
  `uncalled_function` negatives. No check-accuracy pin churn —
  dispatch-miss negatives are check-flagged and value rows don't
  count.
- **The §2.2 number**: `BenchmarkTuiKeystrokeStorm` (modules) floods
  one session and reports renders per keystroke under coalescing —
  **0.031 renders/keystroke** at a 2000-key storm (~1/32, the drain
  limit), ~96µs per fold on CI-class hardware. Every keystroke folds;
  batches render.
- **Ecosystem finding, not tui's**: `Net.close h` with code following
  on the same line collects the WRONG forward argument (the module-net
  TSV's own `Net.close l;` semicolon spelling is the canon); the
  graduated client hit it and the probe README records it.

### P4 — the remote tier

**P4 landed in full**: `Tui.serve` (`modules/tui_serve.go` — json-lines
tree protocol, constant-time token auth with the `token: "none"`
opt-out, one viewer, busy denials, disconnect-quits) and `aql attach`
(`cmd/go/internal/attach` — dial, handshake, local layout via the
shared tuikit renderer, upstream event projection), with loopback tests
over real ephemeral listeners plus seam-driven session tests over
`net.Pipe`. Divergences and design points found necessary during
implementation:

- **The Renderer seam landed as driver closures, not an interface.**
  `tuiDriver` holds `events <-chan tuikit.Event` + `paint` + `finish`;
  the local half is `localPaint` (render → present → cursor), the
  remote half `tuiWirePaint` (marshal the *unrendered* tree onto the
  wire). The driver is transport-blind — exactly one loop for run and
  serve, and the attach client decodes into the SAME `any` shape the
  local path projects (P2's one-code-path claim held on the wire).
- **A write-side viewer loss is the §6.3 disconnect-quit.** The wire
  paint maps a failed frame write to the `errTuiViewerGone` sentinel
  and the driver folds it into "quit with the current state", twinning
  the read-side EOF (which arrives as a synthetic `__disconnect`
  event). Found by the loopback test: a vanishing viewer EPIPEs the
  in-flight frame, and that must not surface as an app error.
- **Teardown order is listener-first.** The driver's `finish` releases
  ONLY the listener (stopping the acceptor); the session conn outlives
  the driver so the `{tag: "quit"}` goodbye still writes, and a
  deferred close releases it on every path. The wire reader's channel
  sends are non-blocking so an input storm cannot strand the goroutine
  after the driver exits.
- **The serve options map is transport-only** (`{tcp token}`); `title`
  (and any future `mouse`/`ctrl-c` relevance) stays in the app config —
  the SAME app map runs locally via `Tui.run` and remotely via
  `Tui.serve`, with serve forwarding the title as a wire line.
- **Policy order is network-then-terminal**: `serve` gates on
  `network.listen` before `terminal.serve`, pinned by a stock-sandbox
  denial (direct handler call — sandbox refuses the import itself) and
  a `policy.LoadInline` split profile (network-allow/terminal-deny)
  that reaches the terminal gate through the engine path.
- **TSV**: +6 runtime-only `serve` option/config rows
  (`lang/spec/module-tui.tsv`), check-accuracy ratchet pin 141 → 147
  with the changelog entry.
- **`todo-tui-served.aql` does not graduate here** — the probe apps
  still use the aspirational guard-list `case` prose; all three
  graduate together in P5 on the landed `case` spelling (per the P3
  note).

### P3 — the Tier-2 app runtime + the real TTY backend

**P3 landed in full**: the fourteen Tier-2 words (nine constructors +
`run`/`quit`/`style`/`edit`/`focusable`) in `modules/tui_widgets.go` /
`tui_run.go`; the driver loop on a real `eng.Process` (input pump,
32-message drain-coalescing, quit marker, Ctrl-C/Ctrl-\ policy, resize,
init event carrying the UI's `Pid`); `cmd/go/internal/termback` (raw
mode + alt-screen + SGR damage painting + a hand-rolled input decoder,
every OS touchpoint behind TEST-SEAMS vars, 100% covered on a
terminal-less CI); registration in `buildrt` (run/do/build binaries)
and the REPL. Divergences found necessary during implementation:

- **`Backend` gained `SetCursor(x, y, visible)`.** The RFC's §3.4
  cursor-placement duty had no seam to land on — the driver calls it
  after every `Present` with the render's cursor verdict.
- **`RegisterName` REPLACES (BEAM re-register semantics)** — it never
  errors on a duplicate, so the single-owner rule is a `Whereis`
  pre-check; the residual register-fails-only-when-down guard carries a
  `covergate:allow` pragma.
- **No `x/cellbuf` promotion and no bubbletea input decoding.**
  tuikit's own `DiffFrames` + `SGR` cover damage painting, so termback
  needs only `x/term` + a small hand-rolled CSI/SS3/paste/SGR-mouse
  decoder (table-tested; no ESC timeout — a lone ESC delivers when no
  continuation byte is buffered, which VT-competent terminals satisfy).
- **The RFC prose's guard-list `case` is not the landed `case`.** The
  shipped word is VALUE dispatch (`case v [match block … default]`);
  the RFC's §5 examples and the probe apps use the aspirational
  guard-list form and read as pseudocode until P5 graduates them onto
  the landed spelling (where blocks run with the matched value pushed —
  a leading `drop` is the idiom).
- **Coalescing is observable**: a fully pre-scripted session drains in
  one batch and a quit skips the final paint — tests assert the init
  frame and the returned state, not per-event frames (the tuikit
  goldens own per-frame visuals).
- **Overflow policy is one knob per mailbox**: the UI process uses
  `block` (workers get backpressure; the pump is buffered upstream by
  the backend's event channel), not the RFC's split drop-input/block-
  workers scheme — the input-side shedding lives in the backends'
  bounded channels.

### P2 — layout/render core

**P2 landed in full** (`lang/go/tuikit`: `layout.go`, `render.go`,
`width.go`, `color.go`, `diff.go` — pure Go, 100% covered, no engine or
terminal involvement; goldens include CJK/emoji and combining-mark
rows). Divergences and design points found necessary during
implementation:

- **The renderer consumes the ValueToAny-shaped `any` tree** — not a
  bespoke Go widget-struct set. The layout engine's input is exactly
  `native.ValueToAny`'s projection (maps/lists/strings/numbers, with a
  numeric coercion helper spanning int/int64/float64/json.Number), which
  is also exactly what P4's `aql attach` decodes off the wire — local
  and remote rendering are literally one code path, and P3's module
  glue is a single projection call.
- **Hand-rolled wcwidth tables, no new dependency.** `RuneWidth`/
  `StringWidth` cover the zero-width (combining/ZWJ/variation) and
  East-Asian-wide + emoji ranges by table; full grapheme-cluster
  segmentation (ZWJ sequences, flags) remains future precision work if
  it bites. Wide-rune continuation cells are marked `Width: -1` so the
  `Screen()`/`ScreenAt` projections keep "漢字" a contiguous substring
  (P1's projection rendered spurious spaces there — corrected in the
  same commit).
- **Color degradation lives in tuikit** (`ParseColor` + `To256`/`To16`
  cube/grey-ramp tables) so the P3 termback and any future backend
  degrade identically; style DATA stays full-fidelity strings, exactly
  as the RFC specifies.
- **`Render` returns the cursor verdict** (`RenderResult.Cursor`, set
  when exactly one `input` has `focus: true`) — computed during paint
  since only layout knows the focused input's screen position; the P3
  driver just consumes it. A focused input inside a `viewport` does not
  place the cursor (its coordinates are virtual — recorded as the
  documented behaviour).
- **Viewport canvases are capped** (1024×4096 cells) so a runaway
  `fit` cannot allocate unboundedly; text wrapping is character-level
  in P2 (word-boundary wrap is polish).

### P1 — seam + Tier 1 (as of the P1 commit)

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
- ~~General terminal utilities usable outside the app framework~~ —
  **landed post-plan** (user-directed): `Tui.colorize` /
  `Tui.strip-ansi` / `Tui.text-width` on the module's pure no-backend
  tier, with the placement investigation recorded in
  [TUI-UTILITIES.0.md](TUI-UTILITIES.0.md) (a `term-util` extraction
  stays the fallback if the surface outgrows terminal output).
