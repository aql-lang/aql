# MODULE-VIEWS

Design for **module-provided views and widgets** in boru — how a module
exposes visual and interactive projections of its *own* semantics (a state
machine's diagram with the current state lit, an event timeline, a mailbox
gauge, a live inspector panel) without any rendering surface having to know
the module exists.

This is the "rich-display protocol" note that `BORU-VIZ.0.md` §12 Q2
explicitly deferred ("a `view` word or REPL affordance that auto-renders
drawable values … is the natural sequel. Separate design note when wanted"),
generalised to the question that prompted it: **what is the contract by which
any module — `boru:state` first — contributes views keyed to its own value
types?**

## Context

Everything below builds on machinery that is either shipped or already
designed; the new surface proposed here is deliberately thin. What the tree
already has (each claim verified in-tree at design time):

- **A widget system with a closed vocabulary.** A tuikit widget is a plain
  Map with a `w:` kind discriminator; the renderer's kind set is **closed**
  (`lang/go/tuikit/render.go:58` — nine kinds: text/rows/cols/box/list-view/
  table/input/viewport/spacer) and `parseWidget` rejects unknown kinds —
  `TUI.0.md` §11.8 treats that rejection as the *reservation mechanism* for
  future kinds. Widgets are stateless data; all interactivity funnels through
  the app's single fold.
- **A shipped host that takes a module value + a view function.**
  `Tui.run {service: <Service> view: <fn(state)->tree>}`
  (`lang/go/modules/tui_run.go`) — the service owns the state and folds
  events; the view is a pure function to a widget tree. Trees are sendable
  plain data, so the same app runs on a local TTY, over `Tui.serve`/`boru
  attach` json-lines, and in the browser (`wpg/wasm/tuiweb.go` renders frames
  as styled HTML; `wpg/e2e/tui-e2e.js` proves interactive apps in the page).
- **A shipped module-contribution contract, in miniature.**
  `Debug.dashboard` (`lang/go/modules/debug_dashboard.go`) renders widgets
  that are plain `{title: String  sample: String}` Maps, and its file header
  states the convention this note generalises: "any module can contribute
  widgets simply by exporting a function that returns a list of these maps".
  `DEBUG-MODULE.0.md` §7 designs the richer render-agnostic model this grew
  from — `DebugCell` (`{kind data}`, kind ∈ text/table/record/gauge/
  sparkline/log), `WidgetMeta`, `DebugWidget` (a Service answering
  `{op:"meta"}`/`{op:"sample"}`), and zero-config discovery by scanning
  imported modules for a conventional export name.
- **A neutral view-data contract.** `BORU-VIZ.0.md` §3 pins graph / tree /
  trace / schema shapes as plain Maps and Lists — the seam between *data
  producers* and *format emitters*, designed so "hand-written literals, scry
  output, and kg query results are all directly drawable", with `kind:` as
  "an open tag emitters may style; unknown kinds render plainly".
- **The type-keyed contribution mechanism.** Open-word extension
  (`core/go/word_extend.go`, `design/OPEN-WORDS.0/1.md`): a module exports
  extra overloads of a locked-signature word, each **anchored on a nominal
  type the module owns** (rule R1, `requireOwnedAnchor`); `import`
  transplants them onto the importer's word. In production use by
  `boru:time-util` and `boru:matrix-util` extending core `add`/`sub`/`mul`
  (`lang/go/native/native_math.go:674`, `lang/go/modules/matrix.go:888`).
- **The anti-registry verdict.** The `emit` kind namespace is frozen and
  `EmitLang.register` is a tombstone raising `emit_registry_frozen`
  (`lang/go/modules/emitlang.go:25-35`): custom emitters are Function
  *values*, lexically scoped, not entries in a flat name registry. Any
  design here that reached for `RegisterWidget("state-machine", …)` would
  contradict that maintainer precedent.
- **Per-type text rendering.** `TypeBehavior.Format` (consulted by
  `FormatForPrint`, `core/go/print.go:27`) is the shipped one-line-of-text
  version of "a type controls its own display" — the floor every richer view
  degrades to.

This is a **design RFC only — no implementation code yet**, matching how
other subsystems were designed first (`PROCESSES.0.md`, `SERVICES.0.md`,
`BORU-VIZ.0.md`).

### Relationship to `BORU-VIZ.0.md` and `BORU-SCRY.0.md`

Viz draws data; scry produces data about the system; **this note is about
who produces data about a *module's own values*, and how surfaces find it.**
The §3 contract shapes are adopted here verbatim as the lingua franca of
structural views; nothing below adds a diagram format or an introspection
seam. Viz stays "code generation only"; the interactive tier below is a
consumer of viz output, exactly as viz §1 anticipates ("any future
live-diagram viewer is a consumer of viz output, not part of it").

### Relationship to `STATE-MACHINES.0.md`

`boru:state` is the motivating first supplier: a `Machine` is a minted,
module-owned nominal Ideal (a valid open-word anchor by construction) whose
definition is already canonical data. Its § on tooling is amended in the
same change as this note to produce the viz contract (`State.graph`) instead
of Mermaid text, and its host words (`State.serve`) plug into `Tui.run`
today. Every pattern below is illustrated with the machine because it is
the hard case: views need the *definition*, the *snapshot*, and sometimes
the *live host* — three different sources with different lifetimes.

### Relationship to `DEBUG-MODULE.0.md` §7

The dashboard cell/widget model there is the direct ancestor of §5's
interactive tier. This note does not replace it; it aligns the two: the
cell's `{kind data}` shape and the discovery-by-export convention
generalise from "debug dashboards" to "any surface that hosts module
views". Where they overlap, the DEBUG-MODULE shapes win — one contract,
two consumers.

### Scope decisions (proposed)

1. **Views are plain data; module semantics travel as vocabulary, not
   hooks.** A view value is a Map in an already-pinned shape (viz §3
   contract, tuikit tree, or the §4 display bundle). A module expresses
   what is special about *its* semantics through the shapes' open fields —
   `kind:`/`group:` tags on graph nodes and edges, `style:`/extra keys on
   widget maps — never through code injected into a renderer.
2. **Producers are pure words; hosts own IO, interaction, and refresh.**
   The `[value opts] ~> Map` signature discipline of viz/report applies to
   every view producer. Liveness (polling, event loops) belongs to hosts
   (`Tui.run`, the dashboard, debugserve pollers), never to view words.
3. **No flat registries.** Contribution is by exported words, word
   extension on owned nominal anchors, and value passing — the emitlang
   verdict and the net-codec precedent ("a Codec is a plain Map of
   functions … custom protocols are ordinary boru values",
   `lang/go/modules/net_codec.go:28-37`).
4. **Text is the universal fallback.** Every surface renders unknown view
   kinds as text (`canon` of the data, or `TypeBehavior.Format`); a view
   can therefore never make a value *less* displayable than today.
5. **New primitive renderer kinds stay a tuikit/Go decision.** The closed
   `widgetKinds` set is the reservation mechanism, per `TUI.0.md` §11.8; a
   module cannot mint paint behaviour. (A pure *lowering* pre-pass —
   module-registered expanders from semantic kinds to primitive trees — is
   recorded as the phase-3 option if composition of the nine primitives
   proves insufficient.)

## 1. The view pipeline

One sentence of architecture: **domain value → producer word → view value →
surface.**

```
Machine/snapshot ──State.graph──▶ viz §3.1 graph ──Viz.graph──▶ Mermaid/DOT ──▶ PR, docs
Machine/snapshot ──State.panel──▶ tuikit tree   ──Tui.run───▶ TTY / attach / browser
any value        ──view───────▶ display bundle ──REPL/wpg──▶ inline rendering (§4)
```

The producer is module code (it knows the semantics); the view value is one
of a small set of neutral shapes (so surfaces need no module knowledge); the
surface is host code (it knows the medium). Every arrow is a plain value —
sendable, spec-rowable by equality, wire-portable.

## 2. Tier 0 — structural views over the viz contract (convention only)

A module ships pure producer words emitting `BORU-VIZ.0.md` §3 shapes, with
its semantics encoded in the contract's open vocabulary. For `boru:state`:

```boru
State.graph <machine> (opts) ~> Map     # §3.1 graph: nodes = states, edges = transitions
```

- node `kind:` ∈ `'initial'`/`'final'`/`'state'` — and `'current'` when the
  caller passes `{snap: s}`, which is how "highlight where the machine is"
  crosses a drawing layer that has never heard of snapshots;
- edge labels carry the event name; edge `kind:` ∈ `'guarded'` (a `when:`
  variant), `'deferred'` (a `defer:` entry, drawn to self), `'timer'` (an
  `after:` arc);
- `group:` is reserved for phase-2 hierarchy, which then collapses to a
  parent-state view via `Viz.collapse` with zero new machinery.

Everything downstream is free: `Viz.graph (State.graph door) {}` for a PR;
`{to:'dot'}` past Mermaid's ceiling; and — the property viz built the
contract for — **assertions and pictures from the same value**:

```boru
def g (State.graph door)
Viz.cycles g                        # machine loop structure, testable
eq 0 (size (unreachable-of g))      # reachability checked over the drawn graph
```

The same tier gives machine *histories* a shape for free: an event log
collected by a host is a viz §3.3 trace (`[{seq event from to} …]` maps
onto `{seq word …}` rows or a small adapter), so `Viz.seq` draws "what this
machine did" as a sequence diagram with no state-machine-aware code in viz.

The rule this tier sets for every module: **your semantics become tag
vocabulary, documented next to your producer word** — scry's graphs already
do exactly this (`kind:'native'|'defined'`, `group:<module>`, which is why
`Viz.collapse` lifts a word graph to a module graph "for free").

## 3. Tier 1 — TUI widgets by composition (works today)

A terminal/browser widget is a pure function to a tree of the nine
primitive kinds, plus (when interactive) a pure fold for its interaction
state — both module exports, both plain data in and out:

```boru
State.panel <machine> <snap> (opts) ~> Map    # box/rows/table tree: states as rows,
                                              # current one styled {reverse: true},
                                              # defer depth and armed timers as gauges
```

The composition idiom is shipped: the boru-written vault TUI builds every
screen from pure component fns over the nine kinds
(`lang/go/modules/vault_tui.boru`), interaction state lives in app state
and folds through pure words (`Tui.edit` is the precedent for a
module-exported widget-state fold).

For a *live* widget, `Tui.run {service: … view: …}` is the shipped host and
a machine already fits it: `State.serve` returns a Service, so

```boru
Tui.run {service: (State.serve door)  view: door-inspector/r}
```

is a running inspector — every keypress becomes an event dispatched through
the machine's own step (unhandled UI noise is swallowed by the host's
`no_match` tolerance), and the view renders the snapshot after each fold.
Because trees are plain data this identical program is also a remote widget
(`Tui.serve`) and a browser widget (the playground's DOM backend) — the
module wrote none of that.

A generic **machine inspector** falls out of the uniform surface: current
state and `status` from the outcome record, the table-level event menu from
`State.can` (documented as guard-blind), defer/timer panes from the
snapshot — one inspector for every machine anyone defines, which is the
payoff of machines-as-values.

## 4. Tier 2 — the display bundle and the `view` word (the new surface)

The piece that does not exist yet, kept minimal. A **display bundle** is:

```
{view: <kind-atom>  data: <plain sendable value>  meta: {title: … }}
```

with the v1 kind set frozen small and aligned with shapes surfaces already
understand: `text/q`, `graph/q`, `tree/q`, `trace/q`, `schema/q` (the four
viz contract shapes), and `tui/q` (a widget tree). `meta` is optional and
open (title, a preferred emitter, a refresh hint for polling hosts).

One word carries the protocol:

```
view <value> (opts:Map) ~> Map        # a display bundle
```

- **Base behaviour (locked signature, `[Any opts:Map]`)**: the text
  fallback — `{view: text/q  data: (canon value)}` — so `view` is total.
- **Modules contribute per-type overloads by open-word extension**, the
  established mechanism (time-util/matrix-util precedent): `boru:state`
  exports a `Machine`-anchored overload whose bundle is
  `{view: graph/q  data: (State.graph m)  meta: {title: …}}`. Rule R1 is
  satisfied by construction — `Machine` is module-owned and nominal. A
  snapshot-aware variant rides opts (`view m {snap: s}`), not new arity.
- **Surfaces render the kinds they know and fall back to text.** The REPL's
  `/view` meta-command prints text and (for graph/tree/trace/schema kinds)
  the Mermaid source via viz. The playground extends its result path — the
  `{result output error}` map `wpg/wasm/main.go` returns and `showResult`
  renders — with an optional `displays:` list, rendering `tui/q` through
  the existing DOM backend and diagram kinds as source text first, inline
  Mermaid later iff mermaid.js is vendored (the sql.js precedent).
  debugserve ships bundles as JSON unchanged. Unknown kind ⇒ text, always
  (scope decision 4).

Why a word and not a behavior slot: the `behave` table is closed and each
slot exists because a *kernel* dispatch point consults it
(`lang/go/native/native_behave.go:106`); views are a module-layer concern,
and opening the kernel table for a lang-layer consumer inverts the layering
(ADR-013 direction). Why a word and not a surface: a `Viewed` surface
(`def Viewed surface {view: (fnsig [[Self Map] [Map]])}`) is a good
*optional conformance check* for generic containers of viewables, but
surfaces are content-membership and cannot anchor extensions — the word is
the dispatch mechanism, the surface at most documents it.

## 5. Tier 3 — interactive widget catalogs and discovery

For hosts that assemble many modules' views (the debug dashboard today; any
future inspector): adopt `DEBUG-MODULE.0.md` §7 as written rather than
minting a rival —

- the **cell** (`{kind data}`, kinds text/table/record/gauge/sparkline/log)
  is the row-level unit a dashboard renders;
- the **widget** is a Service answering `{op:"meta"}` / `{op:"sample"}` —
  which a hosted machine already is, one `add` away;
- **discovery is an export-name convention** (a module exports a
  conventional word returning its widget list; `Debug.discover-widgets`
  scans imported modules) — zero-config, no registry, exactly the
  `debug_dashboard.go` file-header contract already shipped in miniature.

This note adds only the alignment rule: a cell's `data` may be a display
bundle (§4), so the same producer feeds the one-shot dashboard, the live
TUI, and the REPL.

## 6. What other modules get, immediately

The point of the mechanism is that `boru:state` is not special:

- **`boru:serve`** (when it lands): a supervision tree is a §3.1 graph
  (`kind:'service'`, edges = supervision), mailbox depth/high-water from
  `{op:"status"}` is a gauge cell — the DEBUG-MODULE §7 table already
  sketches exactly this widget.
- **`boru:scry`** is *already* Tier 0 — its words exist to emit the
  contract; `view` on nothing more than a word name could bundle
  `Scry.word-graph {roots: [it]}`.
- **`boru:origin`** (`VALUE-TRACING.0.md`, unshipped): provenance graphs
  adopt the same contract (scry §9 Q5 already records that expectation);
  `view` on a marked value shows where it came from.
- **The kg pipeline**: `KgQuery.edge-list` output is drawable today (viz §6
  example 5); a `view` overload is one line.
- **`boru:net`**: an `Endpoint`'s codec/transport chain as a tree view; a
  connection lifecycle is literally a state machine — RFC 9293 renders via
  `State.graph` the day someone writes the spec map.

## 7. Safety and policy

Nothing here adds capability surface. View producers are pure words (no
scope); `view` is pure; bundles are immutable plain data and cross process
boundaries under the ordinary `not_sendable` rule. Interactive hosts carry
their existing gates (`Tui.run` needs the registered terminal backend;
`State.serve`-backed widgets inherit the service words' posture; dashboards
that evaluate `sample` source run it with the caller's permissions, exactly
as `Debug.dashboard` does today). The one obligation is inherited from viz:
bundle `data` reaching a diagram emitter goes through viz's escaping
guarantees, never raw concatenation.

## 8. What does NOT transfer / declined

- **`behave view/q …`** — declined for v1 (closed kernel table, wrong
  layer); reconsider only if the open-word route proves insufficient for
  non-nominal values (plain Maps cannot anchor an extension — for *those*,
  passing the bundle explicitly or wrapping in a class is the answer, and
  is honest about identity).
- **A name-keyed widget registry** — declined permanently (emitlang
  tombstone; net-codec value-passing precedent).
- **Module-defined tuikit kinds** — declined for v1; the closed set is the
  reservation (TUI.0.md §11.8). Phase-3 option: a pure lowering pre-pass
  (semantic kind → primitive tree) run before `tuikit.Render`, server-side,
  so the wire protocol and all three clients stay unchanged.
- **In-engine diagram rendering** — already investigated and rejected by
  viz (§11 "in-binary rendering"); this note inherits that verdict.
- **Auto-refresh/reactivity in view words** — liveness is host business
  (scope decision 2); a view word that reads a clock would also fall out of
  the frozen-clock spec harness.

## 9. Phases

1. **Phase A (with boru:state phase 2, no new machinery):** `State.graph`
   and the kind vocabulary; machine history as trace rows; `State.panel` +
   the `Tui.run {service:}` inspector as a worked example in HOWTO. Every
   piece is convention over shipped/designed machinery.
2. **Phase B (the protocol):** the `view` word with its locked base
   signature and text fallback; `Machine`-anchored overload in boru:state;
   REPL `/view`; bundle kinds frozen; spec rows for base + extension +
   fallback (ADR-003; positive/negative pairs).
3. **Phase C (surfaces):** playground `displays:` extension (structured
   bundles across the Go→JS boundary; mermaid vendoring decision);
   dashboard/DebugWidget alignment when `boru:serve` lands; the lowering
   pre-pass only if composition pressure demands it.

## 10. Worked example — the machine, viewed four ways

(Illustrative surface; exact syntax settles during implementation. The viz
words are `BORU-VIZ.0.md`'s; `State.*` words are `STATE-MACHINES.0.md`'s;
`view` is §4's.)

```boru
import "boru:state"
import "boru:viz"
import "boru:tui"

def door (State.define door-spec {…})          # STATE-MACHINES.0.md §11
def r    (State.init door {ctx: {key: "front-door"}})

# 1. structure, for a PR description — semantics as kind: tags
Viz.graph (State.graph door) {title: "door"}
#   states are nodes (closed = kind:'initial'), transitions are labelled
#   edges; guarded arcs kind:'guarded'; the locked defer loops to self.

# 2. the same structure, live — current state lit from the snapshot
Viz.graph (State.graph door {snap: r.snap}) {}
#   the node for r.snap.state carries kind:'current'; viz styles it, and
#   never learns what a snapshot is.

# 3. auto-display — one word, any surface
view door                                       # ~> {view: graph/q data: {…} meta: {…}}
view 42                                         # ~> {view: text/q data: '42'} — the total fallback

# 4. a live inspector in the terminal (and unchanged in the browser)
def door-inspector fn [[s:Map] [Map] [
  Tui.rows [
    (Tui.text (join "" ["state: " (convert String s.state)]) {style: {bold: true}})
    (Tui.table {rows: (events-table (State.can door s.state))})   # guard-blind menu
    (Tui.text (join "" ["deferred: " (convert String (size s.deferred))]))
  ]
]]
Tui.run {service: (State.serve door)  view: door-inspector/r}
```

Note the conventions in play: the module contributed only *pure words and
tag vocabulary* — `State.graph` (semantics → contract), an anchored `view`
overload (default picture), `door-inspector` (snapshot → primitive tree) —
while every renderer involved (viz emitters, tuikit, the playground page)
is generic code that has never heard of state machines; the snapshot's
plainness is what lets one value feed all four views; and the guard-blind
`State.can` menu is honest by design (the `nextEvents` lesson).

## Open questions

1. **The `view` word's home.** A core-adjacent word in the lang native
   layer (so extension needs no import), or a small `boru:view` module that
   owns the bundle contract and the kind atoms? (Leaning lang native layer,
   like `service`/`patrun` — the protocol is only worth having if it is
   ambient; name verified unclaimed.)
2. **Bundle kind vocabulary freeze.** Are the six v1 kinds (`text graph
   tree trace schema tui`) right, and is a `cells/q` kind for dashboard
   rows worth adding in v1 or deferred to the DEBUG-MODULE alignment?
   (Leaning: six now; cells at phase C.)
3. **Non-nominal viewables.** Should `view` consult a `canon`-style behave
   slot as a *second* dispatch tier for values whose types cannot anchor
   extensions, or is explicit bundling enough? (Leaning explicit —
   revisit with evidence, per the behave-table posture.)
4. **Playground rendering depth.** Vendor mermaid.js into the wasm bundle
   (≈1 MB, sql.js precedent) for inline diagrams, or keep diagram kinds as
   copyable source text in v1? (Leaning source text first; vendoring is a
   maintainer call with bundle-size numbers in hand.)
5. **Discovery export name.** Adopt `debug-widgets` (DEBUG-MODULE §7.1) as
   the single convention, or generalise the name (`views`) now that
   consumers beyond debugging exist? (Leaning: keep `debug-widgets` for the
   dashboard, defer a general name until a second consumer ships.)

No ADR entry is proposed — per repo policy this stays a `design/` note
until a maintainer says otherwise.
