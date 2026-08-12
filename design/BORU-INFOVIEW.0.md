# boru infoview — the stack at the cursor

**Status: design proposal — not implemented.** Companion notes:
[BORU-SCRY.0.md](BORU-SCRY.0.md) (self-knowledge as data — the
infoview's data source), [BORU-VIZ.0.md](BORU-VIZ.0.md) (diagram
source generation — the infoview's picture vocabulary),
[BORU-DEBUGGER.0.md](BORU-DEBUGGER.0.md) (shipped: the time-travel
ring, replay, DAP — the infoview's dynamic backend), and
[DIAGNOSTICS.0.md](DIAGNOSTICS.0.md) (the structured diagnostics the
infoview surfaces). The name is borrowed from Lean 4's infoview with
attribution, not shame; a better boru-native name is an open question
(§11 Q1).

This note answers: what can boru learn from the Lean 4 infoview, and
what would a boru infoview offer? The research base is the infoview's
own architecture paper (Nawrocki, Ayers, Ebner, ITP 2023) plus the
wider family — Isabelle/PIDE, Coq's goal panels, Agda/Idris
interactive editing, Factor's listener and walker, Light Table,
Hazel — and a seam-by-seam inventory of this repository (§9 carries
the citations).

## 1. The thesis

The Lean infoview rests on one structural fact: **proof state is a
total function of cursor position.** Lean elaborates the whole buffer
continuously into snapshots; the panel renders `state(snapshot,
cursor)`, well-defined *between* every pair of tactics. Every feature
users love — goal display, semantic diffs, hover-into-subterms,
widgets — hangs off that.

boru has the same property with a simpler state: **the stack.** A
concatenative program has a well-defined stack at every word
boundary — statically the checker's carrier stack, dynamically the
actual values of a run. A Lean goal is a rich hypothesis context; a
boru stack is a short list of typed cells. The Lean move applies with
*less* machinery. Factor is the existence proof that it matters: what
made a stack language practical there was exactly a visible stack,
statically checked stack effects, clickable object presentations, and
a stepper with Back.

Two repo facts make this retention-and-transport work, not analysis
work:

- **The static oracle half-exists.** Member completion already answers
  "what type is on top of the stack at this cursor?" by
  truncate-and-check (`cmd/go/internal/lsp/membercomplete.go` — check
  the prefix, read the residual carrier stack). And because check mode
  drives the *same* engine Run loop as execution
  (`check/go/CLAUDE.md`), arming the existing per-step trace hook
  (`core/go/engine.go` — fires before every token dispatch) during a
  `Check` yields the carrier stack **at every source position** in one
  pass — retention work, not analysis work. (The cost and honesty
  conditions this implies — snapshot copying, per-context fn-body
  analysis, broken buffers — are owned by the oracle's design, §4.)
- **The dynamic side is better-instrumented than Lean's.** Lean shows
  elaboration-time state only. boru's shipped debugger has whole-tape
  snapshots per step, a positioned time-travel ring, deterministic
  `replay` to any earlier stop, a DAP adapter, and remote HTTP
  stepping. The missing piece is only the query direction: every
  surface serves "state at the current pause," none yet serves "state
  at row R."

## 2. Design principles

1. **The stack at the cursor is the product.** Everything else —
   diffs, holes, pictures — is derived from a single
   position-indexed state function, computed statically always and
   dynamically when a run exists. (Lean; Isabelle/PIDE's
   continuous-checking document model.)
2. **LSP-first delivery; the panel earns its keep.** Roughly 70% of
   the value ships over plain LSP 3.17 — inlay hints, hover, code
   actions, code lens — and therefore works in every editor in
   `editors/` with no webview. A dedicated panel is reserved for what
   genuinely needs it: things time-indexed (the scrubber), large
   (lazy trace trees), or pictorial (viz renderings).
3. **Presentations, not prints.** Every rendered fragment keeps a live
   reference to the thing it denotes — a stack cell is a handle, not a
   string. Detail is fetched lazily on expansion (Lean's tagged-text +
   session-RPC design; Factor's clickable presentations). Never bulk-
   serialize a big value into the editor.
4. **Rich UI round-trips to text.** Any action a panel offers —
   insert a shuffle word, apply a suggestion, case-split — lands as
   an edit to the program source. The artifact of record stays a
   plain-text boru program (Lean's answer to Paulson's critique).
5. **A plain endpoint beside every interactive one.** Lean pairs
   interactive goals with `$/lean/plainGoal`; boru does the same so
   CLIs, tests, and agents consume the identical state as text/JSON
   with no panel in the loop.
6. **Serve the panel from the binary.** Lean's known pain is the
   per-editor webview adapter tax. boru already owns the antidote
   patterns: `debugserve`'s tokened loopback HTTP and
   `Tui.serve`/`boru attach`'s trees-down-events-up wire. The panel
   is a page (and a TUI) served by the `boru` process; editors embed
   it if they can and merely link to it if they cannot.
7. **Reuse seams, add none gratuitously.** The trace hook, the ring,
   replay, DAP, debugserve, tuikit, the diagnostics payloads, and the
   scry/viz contracts already exist or are already designed. The
   infoview is a curation-and-transport layer over them (the
   `boru:debug` §2.4 discipline).

## 3. What the user sees

### Tier 1 — any LSP editor, no panel

- **Stack inlay hints.** After each top-level phrase (elision policy
  §11 Q3), a ghost annotation in Forth stack-comment dress:
  `( List Integer -- Integer )` — the checker's carrier at that
  boundary. Accepting a hint materialises it as a comment/annotation.
- **Hover = state + dispatch + docs.** Hovering a word shows the
  stack shape *at that position*, which signature the word will
  dispatch against it and why (mono/poly/dynamic site, forward vs
  swap reading), then the existing help text. Today's hover shows
  documentation only, no state (`cmd/go/internal/lsp/hover.go`).
- **Diagnostics, upgraded in place — a two-sided job.** The
  `DiagSuggestion.Replacement` field was designed as "the seed for
  editor code actions" (`DIAGNOSTICS.0.md`), but no production
  diagnostic populates it today (tests only), and secondary `Spans`
  live on runtime `BoruError` alone — `CheckDiagnostic`
  (`core/go/check_state.go`) does not carry them. So the upgrade is
  data first, transport second: plumb `Spans` onto the check wire
  and populate `Replacement` wherever the fix is mechanical, *then*
  surface them as `textDocument/codeAction` and
  `DiagnosticRelatedInformation`.
- **Code lens from scry.** Above each `def`: reference count from
  `Scry.word-graph`, "run examples," "show dependency diagram."

### Tier 2 — the panel

- **The stack view.** Static carriers and, when a run/replay session
  exists, the actual values, side by side. The static column never
  blocks on the dynamic one. An unfinished-but-parseable program
  still runs *to* the gap and shows the concrete stack that arrives
  there beside the expected carrier (Hazel's
  evaluation-around-holes); a buffer that does not parse degrades to
  the longest-prefix static column (§4) — never to silence.
- **Per-word stack diff.** What the word under the cursor consumed,
  produced, and rewrote — computed on carriers/values, not text, and
  directional relative to the cursor (Lean's goal diffing; for a
  stack language the diff is smaller and clearer than any goal
  diff).
- **Live cells.** Every stack item and subvalue is clickable:
  inspect/`describe`, expand lazily, go to the producing word.
  Structured traces expand on demand, never pretty-printed eagerly
  (Lean's lazy trace lesson — the difference between memory-bound
  and CPU-bound panels).
- **The time scrubber.** Scrub the run at word/line granularity over
  the debugger's ring; positions outside the ring reach via
  deterministic `replay`. Factor's walker proved Back for a stack
  VM; boru already ships it — the scrubber is a UI over an existing
  verb.
- **Pins and a query panel.** Pin the state at position A while
  editing at B (Isabelle's multiple State panels; Lean's pins), and
  one power-user panel that evaluates a scry expression against the
  state at the caret (Isabelle's Query).
- **Pictures where structure is graph-shaped.** The word at the
  cursor's dependency neighbourhood (`Scry.word-graph` →
  `Viz.graph`), a traced run as a sequence diagram (`Scry.trace` →
  `Viz.seq`), a record type under the cursor as a class box
  (`Scry.schema` → `Viz.classes`) — rendered from the Mermaid/DOT
  text viz emits. The panel is the first in-house renderer surface
  the viz design anticipated.

### Tier 3 — actions from holes

The deepest import is from Agda/Idris, not Lean: typed context should
*generate edits*, not just display. A hole in boru is a **carrier
gap** — the stack that exists here vs. the input some later word
requires. Offered as ordinary code actions (Idris's lesson: make them
compiler/LSP commands, editor-agnostic):

- **Insert shuffle** — the stack-rearranging word(s) bridging a
  permutation gap.
- **Case split** — branch on the content type of top-of-stack
  (`case` scaffold over the disjunct's alternatives).
- **Bridge search** — search scry's word index for words (or short
  compositions) whose net effect closes the gap, presented as
  completions ranked by signature fit. This is Agda's `auto` scaled
  down to honesty: a search over declared signatures, not a prover.

## 4. Architecture and seams

**What already exists (reuse, don't rebuild):**

- The LSP (`cmd/go/internal/lsp/`): stdio/TCP transport, JSON-RPC
  framing shared with the DAP adapter, diagnostics-per-keystroke via
  the full checker, hover, completion (including truncate-and-check
  member completion), formatting. Client configs for ~10 editors in
  `editors/`.
- The checker as oracle: carriers run through the engine's own Run
  loop; every parsed token carries `SrcPos`; the per-step trace hook
  fires in check mode exactly as at runtime;
  `Registry.SetDebugTraceFrom` + `RunningEngineChain` see into fn and
  module bodies (the same seam `Scry.trace` specifies).
- The dynamic backend: the debugger session core, the positioned
  ring, `replay`, PauseInfo, the DAP adapter, remote stepping over
  `debugserve`, and `debugserve`'s own endpoints (`/debug/words`,
  `/debug/eval`, events).
- Transport and rendering patterns: `Tui.serve`/`boru attach`
  (widget-data trees over json-lines, multi-viewer, reattach), the
  wasm playground (`wpg` — `boruEval`/`boruFmt` exports, web tui
  backend), tree-sitter/TextMate/CodeMirror grammars.
- Content contracts: scry's graphs/traces/schemas and viz's
  Mermaid/DOT emitters (both designed, unshipped — §5).

**What is new:**

- `lang/go/checktrace.go` — the `CheckTrace` position→stack oracle:
  arm the step trace during an ordinary `Check` run and render
  carriers with the same leaf/dynamic logic `CheckResult.Stack`
  already uses. Three honesty conditions the naive "bucket by
  `SrcPos`" sketch misses, all owned here:
  - **Context keying.** Fn bodies are re-analysed per call shape
    (`check/go/check_fnbody.go`'s per-instantiation memos), so
    inside a body one position legitimately carries one carrier
    stack *per analysis context*. The table key is
    (position, context) — never position alone, which would leave
    hover at the mercy of whichever analysis wrote last. Consumers
    show the join or a context picker (§11 Q7).
  - **Cost.** The trace hook snapshots the whole tape per step
    (`core/go/tape.go` — `Snapshot` is a full copy), so a traced
    check is roughly quadratic in buffer size. `CheckTrace` runs on
    demand per document version — debounced, cached, invalidated on
    edit — and never rides the per-keystroke diagnostics pass. A
    lightweight observer variant (position + top-k carriers, no
    tape copy) is the one *optional* core seam, taken only if
    profiling demands it.
  - **Prefix tolerance.** `Check` parses before it runs, so a
    mid-edit buffer (unclosed group, open string) yields no state
    at all today. The oracle checks the longest parseable prefix —
    the member-completion truncate-and-check move, generalised —
    and answers "no state" beyond the break rather than inventing
    one.
- `cmd/go/internal/lsp/` — inlay hints, hover, code actions, `boru/stackAt`:
  the LSP tier over `CheckTrace` plus the diagnostics data already
  in hand. `boru/stackAt` is the one custom method (position →
  rendered stack, static and — when a session exists — dynamic),
  kept beside the standard capabilities the way `plainGoal` sits
  beside Lean's interactive RPC.
- A **state-by-position verb on the debug surface** — serve a ring
  entry by row/ordinal over the existing remote-stepping HTTP
  surface, falling back to `replay` for evicted stops; inherits the
  ring's honesty contract (snapshots are authoritative for the
  stack; bindings stay live). Granularity is honest about what the
  ring stores: history is recorded on **line transitions** and
  entries carry a row but no column
  (`cmd/go/internal/debugger/debugger.go`), so Phase 2 serves
  line-level state host-layer-only — a cursor mid-line answers with
  its line's entry. Word-level dynamic state requires the
  ring/replay to retain word-level stop identities: a real (small)
  debugger extension, deliberately scoped out of Phase 2 rather
  than promised as free.
- The **display protocol** — the contract [BORU-VIZ.0.md](BORU-VIZ.0.md)
  §12 Q2 deferred: a value renders as a **representation map keyed
  by kind** — `{text:'…' mermaid:'…'}`, kinds `text`/`mermaid`/
  `dot`/`tui` — with the `text` key **required on every bundle**.
  This is Jupyter's mime-bundle rule in full: the producer emits
  every representation it has, the client picks the richest kind it
  knows and ignores the rest, and the mandatory `text` entry
  guarantees degradation is always to plain text, never to loss. (A
  single `{kind payload}` object cannot honour that guarantee —
  a client ignoring an unknown kind would drop the value.)
- The **panel clients**: an HTML page served by the boru process on
  the debugserve pattern (token + loopback), and a TUI view over
  `Tui.serve`/`boru attach` for terminal-native use. Editors embed
  the page where they have webviews and link to it where they do
  not — the adapter tax is paid once, by the binary.

## 5. Contract requests to unshipped designs

Both requests are cheap now and breaking later:

1. **`Scry.trace` rows gain `row`/`col`** ([BORU-SCRY.0.md](BORU-SCRY.0.md)
   §4, [BORU-VIZ.0.md](BORU-VIZ.0.md) §3.3): a position-indexed
   panel is exactly the consumer that needs trace rows addressable
   by source position, and the engine's trace callback already knows
   it.
2. **Viz/scry adopt the §4 display protocol** as the shape of "this
   value knows how to show itself," resolving VIZ §12 Q2 in favour
   of the representation map above (mandatory `text` key).

## 6. Delivery phases

Ordered value-per-effort; each phase is shippable and useful alone.

1. **Phase 1 — the LSP tier (no panel, no new process).**
   `CheckTrace` with its three honesty conditions (§4); stack inlay
   hints with the elision policy; hover upgraded to state +
   dispatch; the two-sided diagnostics upgrade (§3 — populate
   `Replacement`, plumb `Spans` onto the check wire, then serve
   code actions and `relatedInformation`); `boru/stackAt`. Works in
   every editor in `editors/` on day one.
2. **Phase 2 — the dynamic column.** The state-by-position verb over
   ring/replay; hover and `boru/stackAt` gain actual values when a
   debug session or replay exists; the static/dynamic pairing lands.
3. **Phase 3 — the panel.** Served page + attach TUI on the
   debugserve/tui-serve transports: stack view, per-word diff, lazy
   value expansion, the scrubber, pins. Plain endpoints mirror every
   interactive one.
4. **Phase 4 — pictures and holes.** Display-protocol rendering of
   viz output in the panel and the playground; the query panel; the
   Tier-3 hole actions (shuffle, case split, bridge search).

## 7. Policy and safety

The LSP evaluates buffer contents and is loopback-bound *by default*
— but `boru lsp -host` legally widens the bind ("only widen this on
a trusted network") and the transport is unauthenticated. Static
carriers are no more sensitive than the diagnostics already served;
**actual run values are** — so the rule is: `boru/stackAt` serves
dynamic state only on loopback binds, degrading to static-only on a
widened bind unless the LSP transport grows the same bearer-token
discipline the panel uses. The panel itself inherits `debugserve`'s
bearer-token + loopback posture (an introspection surface over the
user's own process, never exposed off the machine by default). The static tier (Phase 1) adds no execution
beyond what diagnostics-per-keystroke already performs — `CheckTrace`
is the same check run, observed. Dynamic state is served only from an
explicitly started debug session or replay, never by silently running
user code. Hole actions edit source only through the editor's normal
apply-edit flow; nothing mutates a program behind the user's back
(§2.4).

## 8. Test discipline

- `CheckTrace` goldens: position→stack tables for fixture programs,
  pinned exactly; permutation/determinism tests (same source → same
  table; unrelated edits do not perturb distant rows); a fn body
  analysed under two call shapes yields **both** contexts at its
  positions, pinned separately (the context-keying condition, §4).
- Truncation honesty: positions inside fn bodies resolve through the
  body-analysis path, and positions in unchecked/errored regions
  answer "no state" rather than a stale or invented stack — paired
  negative tests per the repo rule.
- LSP conformance: inlay-hint, code-action, and `boru/stackAt`
  round-trips through the jsonrpc framing tests; hover output pinned
  for a stack + dispatch fixture.
- Ring queries: state-by-position for in-ring, evicted-but-replayable,
  and never-visited rows (the last is a defined miss, not an error).
- ADR-008 coverage throughout; spec rows where single-line
  (`boru check --json` extensions), Go goldens where multi-line.

## 9. Prior art (research notes, 2026-08)

- **Lean 4 infoview** — architecture: per-file worker processes,
  immutable snapshots, LSP extensions (`$/lean/plainGoal`) beside a
  session-scoped RPC whose payloads are pretty-printed text tagged
  with opaque server handles (lazy detail, distributed GC);
  interaction: goal-at-cursor, semantic directional goal diffs,
  hover/click into every subterm, pins and pause, lazy trace trees;
  widgets: `@[widget_module]` JS + typed props, attached at source
  spans as peers of diagnostics — library authors ship views with
  their libraries. Pain: webview fragility and the per-editor
  adapter tax. ([ITP 2023 paper][lean-paper], [lean4-infoview][lean-iv],
  [ProofWidgets4][pw4])
- **Isabelle/PIDE** — continuous whole-document checking; dockable
  State/Output/Query panels; long-running tools as async print
  functions. ([PIDE][pide], [jEdit manual][jedit])
- **Agda / Idris** — holes as first-class interaction points whose
  typed context generates edits: case split, refine, auto; Idris
  ships them as line/column compiler commands. ([Agda][agda],
  [Idris][idris])
- **Factor** — the concatenative existence proof: listener with the
  stack always visible and printed objects as clickable
  presentations; the walker with Back; compile-time stack-effect
  checking surfaced in every doc and error. ([listener][f-listener],
  [walker][f-walker], [inference][f-infer])
- **Hazel** — evaluation proceeds around typed holes; each hole
  carries its closure so the environment at the gap is inspectable;
  fill-and-resume. ([paper][hazel])
- **Light Table / Learnable Programming** — inline results and
  watches; "show the data, control time." ([LT][lt], [Victor][victor])
- **LSP 3.17** — inlay hints (with accept-materialisation), code
  actions, code lens: the no-panel delivery channel; rust-analyzer
  as the breadth demonstration. ([spec][lsp], [rust-analyzer][ra])
- **Paulson's critique** — interaction must yield a maintainable
  plain-text script; answered by round-tripping every graphical
  action to source edits. ([blog][paulson])

[lean-paper]: https://drops.dagstuhl.de/storage/00lipics/lipics-vol268-itp2023/LIPIcs.ITP.2023.24/LIPIcs.ITP.2023.24.pdf
[lean-iv]: https://github.com/leanprover/vscode-lean4/tree/master/lean4-infoview
[pw4]: https://github.com/leanprover-community/ProofWidgets4
[pide]: https://arxiv.org/pdf/1905.01735
[jedit]: https://isabelle.in.tum.de/dist/doc/jedit.pdf
[agda]: https://agda.readthedocs.io/en/latest/tools/emacs-mode.html
[idris]: https://docs.idris-lang.org/en/latest/tutorial/interactive.html
[f-listener]: https://docs.factorcode.org/content/article-ui-listener.html
[f-walker]: https://docs.factorcode.org/content/article-ui-walker.html
[f-infer]: https://docs.factorcode.org/content/article-inference.html
[hazel]: https://arxiv.org/abs/1805.00155
[lt]: https://chris-granger.com/2013/08/22/light-table-050/
[victor]: https://worrydream.com/LearnableProgramming/
[lsp]: https://microsoft.github.io/language-server-protocol/specifications/lsp/3.17/specification/
[ra]: https://rust-analyzer.github.io/book/configuration.html
[paulson]: https://lawrencecpaulson.github.io/2022/12/14/User_interfaces.html

## 10. What this is not

- **Not a new checker.** Every stack shape shown is one the checker
  already computes; the infoview retains and transports, never
  re-derives.
- **Not an editor plugin per editor.** One LSP tier for every editor;
  one served panel for the rest. New `editors/` entries stay thin
  client configs.
- **Not a notebook or a REPL replacement.** The REPL's `/stack` and
  the playground remain; the infoview is the same state made ambient
  and position-indexed rather than asked for by hand.
- **Not speculative execution.** The dynamic column only ever shows
  state from a run the user started (session, replay, or explicit
  example invocation) — the Light Table instarepl/watch distinction
  kept explicit.

## 11. Open questions for the maintainer

1. **The name.** "infoview" is honest borrowing but Lean-flavoured;
   `boru stackview`? `boru panel`? The CLI verb matters more than
   the noun (`boru lsp` already exists; the panel likely rides
   `boru debug --serve` or a new `boru view`). (Leaning: keep
   "infoview" as the design name, decide the CLI verb at Phase 3.)
2. **Where `boru/stackAt` lives.** A custom LSP method (Lean's
   pattern) vs. overloading hover/inlay only. (Leaning: custom
   method — it is also the plain endpoint agents consume, per §2.5.)
3. **Inlay elision policy.** Full carriers crowd fast; options:
   top-k cells + depth summary, phrase-boundary-only hints,
   hint-on-request per line. (Leaning: top-3 cells with `…`, at
   phrase boundaries only, all configurable.)
4. **Dispatch explanation surface.** Hover needs per-site candidate
   signatures and the reason a signature wins; check knows this
   transiently. Does check export a per-site record (a small new
   surface, like scry's `info`), or does the LSP re-derive from
   `Scry.sig` + the carrier? (Leaning: a minimal check export —
   re-derivation duplicates dispatch logic, which is the one thing
   never to fork.)
5. **Holes in the syntax.** Tier-3 actions work on *implicit* gaps
   (diagnosed mismatches). Does boru want an explicit hole token
   (Agda's `?`) that parses, checks as `dynamic`, and anchors
   actions? (Leaning: not in v1; revisit once bridge search proves
   itself on diagnosed gaps.)
6. **Playground convergence.** wpg currently exposes eval/fmt only.
   Should the playground adopt `boru/stackAt` + the display protocol
   and become the zero-install infoview demo? (Leaning: yes at
   Phase 3 — it is the cheapest place to show the whole story.)
7. **Multi-context presentation.** When (position, context) keying
   yields several carrier stacks at one position (§4), does hover
   show the join, the most recent call shape, or a picker? (Leaning:
   join with a count badge — "2 contexts" — expanding to the list;
   a picker only in the panel.)

No ADR entry is proposed — per repo policy this stays a `design/`
note until a maintainer says otherwise.
