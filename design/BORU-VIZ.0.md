# boru:viz — diagram source generation from arbitrary data structures

**Status: design proposal — not implemented.** Companion note:
[BORU-SCRY.0.md](BORU-SCRY.0.md) specifies `boru:scry`, the module that
produces the structural self-knowledge this module draws. Read this note
first; the data contract the two modules share is defined here (§3).

Maintainer decisions already taken (2026-08-12, recorded so the scope is
not relitigated):

1. **Code generation only.** `boru:viz` emits diagram *source text*
   (Mermaid, DOT). It never lays out, never rasterises, never embeds a
   rendering engine. Rendering happens downstream — GitHub markdown,
   GitLab, Obsidian, VS Code, `dot`, `d2`, Kroki.
2. **Introspection is a separate module.** The mechanism whereby a boru
   system gains knowledge of itself is `boru:scry`
   ([BORU-SCRY.0.md](BORU-SCRY.0.md)). Viz never introspects; scry never
   draws. The seam between them is plain boru data (§3).
3. **v1 emits Mermaid and DOT; D2 is a later phase** (§9).
4. **The module is written in boru itself**, embedded via the
   `cli.boru` pattern (§7).
5. **All four built-in views are in scope**: graph, value shape, trace
   sequence, and type/record schema (§4), delivered in dependency order
   (§9).

Per `lang/go/CLAUDE.md`, this is a framework/DSL module, not a pure
utility library, so the id stays plain: **`boru:viz`, namespace `Viz`**
(the same call `boru:report` and `boru:emitlang` made).

## 1. Purpose and boundaries

boru programs are systems of words, modules, types, and data flowing
over a stack — structure that is real but invisible. The point of
`boru:viz` is to make that structure *visible* by generating diagram
source from ordinary boru values, so that understanding a boru system
(or any data structure a boru program holds) is one word away from a
picture someone can actually look at.

**This is not a charting library.** Bar charts, scatter plots, and
dashboards are out of scope permanently, not just deferred. The subject
matter is *structure*: what depends on what, what contains what, what
called what, what shape a value has.

Boundaries against the neighbouring modules:

- **`boru:scry`** is the *source* of self-knowledge (modules, words,
  signatures, dependency edges, traces — as data). Viz consumes that
  data but works identically on user data: a config tree, a query
  result, a kg bundle. Nothing in viz knows it is drawing "the system".
- **`boru:emitlang`** owns the `emit` word and its frozen kind
  namespace (json/yaml/csv/…, `emit_registry_frozen` — see
  `lang/go/modules/emitlang.go`). Viz does not add `emit` kinds; it is
  its own module surface following the same emitter signature
  convention `[value opts] ~> String` (`emitlang.go:16-23`). Whether
  viz words are *also* reachable through `emit <fn> <data>` as
  Function values is an open question (§12 Q1).
- **`boru:report`** renders values as aligned *text* for humans at a
  terminal; viz renders *diagram source* for renderers. Same purity
  discipline: `report.go:14` — "No word prints to stdout itself; the
  caller controls IO."
- **`boru:tui` / wpg** are interactive display surfaces. Any future
  live-diagram viewer is a consumer of viz output, not part of it.
- **The kg pipeline** (`kg/`) already produces a repo-level structure
  graph and already hand-rolls one projection (the Markdown emitter in
  `kg/storage.boru`). Viz generalises that move; `KgQuery.edge-list`
  output renders through viz unchanged (§6, example 5).

## 2. Design principles

1. **Code generation only; layout is downstream.** The module's entire
   output is strings. No layout engine, no SVG geometry, no embedded
   renderer — and therefore no new Go dependencies, no capability
   needs, and no change to the single-static-binary or wasm story.
2. **One model, many views.** A small neutral data contract (§3) sits
   between data producers and format emitters. Every diagram is
   *selection* (plain boru: `filter`, `fold`, or the §4 transform
   words) followed by *serialisation* (`Viz.graph`, `Viz.tree`, …).
   This is the Structurizr/C4 lesson — diagrams are views over one
   model, never hand-drawn artifacts — arrived at independently by
   dependency-cruiser, goda, and LikeC4 (§11).
3. **Pure words: data in, string out.** Signature convention
   `[value:Any opts:Map] ~> String` throughout. No I/O, no clock, no
   randomness. The caller composes with `boru:io` to write files. The
   module therefore needs no capability or policy scope of its own and
   runs wherever the engine runs — spec runner, wasm playground, and
   sandboxed programs once the sandbox profile's module allowlist
   admits it (§8).
4. **Readable by default: budgets and honest elision.** Every tool
   that survives contact with a real codebase prunes before it draws
   (Doxygen caps local graphs at 50 nodes; pydeps defaults to a
   2-hop neighbourhood; goda and dependency-cruiser make reachability
   the primary operation — §11). Viz emitters enforce default
   `max-nodes` / `max-edges` budgets and, when they cut, say so *in
   the diagram* with an explicit elision node — never silently.
5. **Deterministic, diffable output.** Same value in, same bytes out:
   nodes and edges are emitted in sorted order, synthetic ids are
   assigned deterministically, and nothing reads a clock. Generated
   diagrams are meant to be committed next to code and reviewed as
   diffs, so byte stability is a feature, not an implementation
   detail.
6. **Written in boru.** String projection over Lists and Maps is
   exactly what the language is for; `kg/storage.boru` (the Markdown
   projection) and `design/VALUE-TRACING.0.md` §9.2 (which already
   planned boru-written DOT and Mermaid renderers) are the precedents.
   The module is its own best demo.

Notation below: `~>` denotes "returns". Types in `Capitalised` are boru
lattice types.

## 3. The data contract (the viz⇄scry seam)

These shapes are the normative interchange between data producers
(`boru:scry`, `KgQuery.*`, user code) and viz emitters. They are plain
Maps and Lists — no minted types in v1 (§12 Q5) — chosen so that
hand-written literals, scry output, and kg query results are all
directly drawable.

### 3.1 Graph

```
{
  directed: true              # optional, default true
  nodes: [                    # optional if edges name every endpoint
    {id:'core'  label:'core/go'  kind:'module' group:'kernel'}
    {id:'parser' label:'parser/go' kind:'module' group:'kernel'}
  ]
  edges: [
    {from:'parser' to:'core' label:'depends on' kind:'dep'}
  ]
  groups: [                   # optional; nodes opt in via group:
    {id:'kernel' label:'Kernel modules'}
  ]
}
```

- `id`, `from`, `to` are required (String or Atom); everything else is
  optional. Unknown edge endpoints are auto-materialised as nodes whose
  label is the id — the lenient default DOT and D2 both use.
- `kind` is an open tag emitters may style (e.g. distinct shape for
  `'native'` vs `'defined'` words); unknown kinds render plainly.
- `groups` nest via an optional `parent:` and become Mermaid
  `subgraph` / DOT `subgraph cluster_*` blocks.
- **Shorthand**: a plain adjacency Map `{a:[b c] b:[c]}` is accepted
  wherever a graph is expected (the rhizome contract: nodes + an
  adjacency relation is the minimal honest graph).

### 3.2 Tree

Either any boru value at all (the emitter derives structure by walking
it — this is what `Viz.shape` does), or the explicit form:

```
{label:'root' children:[ {label:'left'} {label:'right' children:[…]} ]}
```

### 3.3 Trace

A List of step rows, ordered:

```
[ {seq:0 word:'load-orders' depth:1 module:'user'}
  {seq:1 word:'IO.read'     depth:2 module:'boru:io'} … ]
```

`seq`, `word` required; `depth` (frame depth), `module`, and `note` are
optional but `Viz.seq` groups lifelines by `module` when present.
`Scry.trace` produces exactly this shape
([BORU-SCRY.0.md](BORU-SCRY.0.md) §4).

### 3.4 Schema

One record/class-like shape per Map:

```
{name:'Order' kind:'record'
 fields: [ {name:'id' type:'String'} {name:'items' type:'List'} ]
 parents: ['Record']
 relations: [ {to:'Customer' kind:'field'} ]}
```

`Scry.schema` produces this; `Viz.classes` consumes a List of them.

## 4. The word surface

All emitters share the opts convention and are typo-proofed with an
Options-schema `Pattern` on the signature, exactly as `boru:emitlang`
does (the G6 fix, `emitlang.go:133-143`) — `{max-node: 10}` is a hard
dispatch error, not a silent no-op.

Common opts (emitters): `to` (`'mermaid'` default, `'dot'`),
`direction` (`'td'` default, `'lr'`, `'bt'`, `'rl'`), `title`,
`max-nodes` (default 60), `max-edges` (default 300), `elide`
(default `true`; `false` errors instead of eliding when over budget).

### Emitters (view × format)

| Word | Signature | Behaviour |
|------|-----------|-----------|
| `Viz.graph` | `Map opts:Map ~> String` | A §3.1 graph (or adjacency shorthand) as a Mermaid `flowchart` or DOT `digraph`/`graph`. Groups become subgraphs/clusters; `kind` selects node shape; budgets + elision per §5.3. |
| `Viz.tree` | `Any opts:Map ~> String` | A §3.2 tree as a Mermaid flowchart (tree-shaped) or DOT tree. Shared structure is repeated (a tree view never invents a DAG); use `Viz.graph` when sharing matters. |
| `Viz.shape` | `Any opts:Map ~> String` | The structure of an arbitrary value: Maps/Lists as nested nodes annotated with `typeof`, long Lists elided to head + count, scalars as leaves. The "show me this thing" word; needs no scry. |
| `Viz.seq` | `List opts:Map ~> String` | §3.3 trace rows as a Mermaid `sequenceDiagram`: lifelines from `module` (or `word` when `participants:'word'`), activations from `depth`. Mermaid-only; `{to:'dot'}` raises `viz_unsupported_target`. |
| `Viz.classes` | `List opts:Map ~> String` | A List of §3.4 schemas as a Mermaid `classDiagram`: fields in the box, `parents` as inheritance edges, `relations` as associations. DOT record-label form in a later phase. |
| `Viz.kinds` | `~> List` | The supported target formats, as data (mirrors `EmitLang.kinds`). |

### Transforms (graph in, graph out — pure)

The minimal selection vocabulary that budgets alone cannot replace.
These operate on §3.1 graphs and compose with ordinary `filter`/`fold`;
if they outgrow viz they split into a future `boru:graph-util`
(§12 Q3).

| Word | Signature | Behaviour |
|------|-----------|-----------|
| `Viz.focus` | `Map opts:Map ~> Map` | The neighbourhood of `on:` within `depth:` hops (default 2), following `direction:'out'|'in'|'both'` (default both). The pydeps `--max-bacon` / go-callvis `-focus` operation. |
| `Viz.collapse` | `Map opts:Map ~> Map` | Merge nodes into their `group:` (or by id `prefix:`), lifting edges to the merged nodes and dropping self-loops. The dependency-cruiser `collapsePattern` operation; turns a word graph into a module graph. |
| `Viz.cycles` | `Map ~> List` | Directed cycles, as a List of id-Lists: strongly-connected components with more than one member, **plus singleton components carrying a self-edge** (`a -> a` is a cycle — a directly recursive word must not slip the assertion). Pairs visualisation with validation on the same data (madge `--circular`): the same value drives a highlighted drawing or a test assertion. |

Error codes (registered per `REFERENCE.md` error-code discipline):
`viz_unknown_target`, `viz_unsupported_target`, `viz_bad_graph`,
`viz_bad_trace`, `viz_bad_schema`, `viz_over_budget` (only when
`elide:false`).

## 5. Format targets

Two v1 targets, chosen for complementary reasons; research notes in
§11.

### 5.1 Mermaid — the paste-anywhere format

Mermaid is the only diagram DSL rendered natively by GitHub markdown
(issues, PRs, wikis), GitLab, Obsidian, Notion, Docusaurus, and
MkDocs-Material — i.e. everywhere a boru user's README or PR
description already lives. That reach is the whole argument.

Constraints a *generator* must respect (they shaped §4's defaults):

- Embedders enforce `maxEdges` = 500 and `maxTextSize` = 50k as
  non-overridable "secure" config — an oversized generated diagram
  simply errors on GitHub. Hence the default budgets and elision.
- Layout is dagre-only in the embedders (ELK is a separate package
  hosts don't ship); there is no rank/port/routing control. Mermaid
  output is for *summarised* views; graphs that outgrow it are what
  `{to:'dot'}` is for.
- Lexer traps: the bare word `end` breaks flowcharts; ids starting
  `o`/`x` can parse as edge decorations; `(){}[];#"` in labels need
  quoting/entity escapes. The emitter therefore always writes
  synthetic node ids (`n0`, `n1`, … assigned in sorted-id order) with
  quoted display labels, and entity-escapes `"` and `#` in label
  text. User data never becomes Mermaid syntax.

### 5.2 DOT — the serious-graph format

For graphs past Mermaid's ceiling, DOT is the machine-generation
lingua franca: ~35 years of Graphviz, cluster/rank/port control, and
universal downstream tooling (local `dot`, VS Code extensions, Sphinx,
Kroki, viz-js in any page). The emitter:

- always double-quotes ids and labels, escaping `"` and `\`, and
  encodes literal line breaks as DOT `\n`/`\l` escapes — a raw newline
  inside a quoted string breaks the parse, and arbitrary data contains
  newlines;
- maps `groups` to `subgraph cluster_<id>` blocks and `direction` to
  `rankdir`;
- emits nodes and edges in sorted order with one statement per line,
  so diffs are line-oriented.

### 5.3 Budgets and elision (both formats)

When a graph exceeds `max-nodes`/`max-edges`, the emitter keeps the
highest-degree nodes (ties broken by id, so the cut is deterministic),
drops edges to removed nodes, and adds one clearly-labelled synthetic
node — `⋯ 214 nodes, 890 edges elided` — with the counts. A diagram
that hides something must say that it does (the Doxygen
`DOT_GRAPH_MAX_NODES` discipline). `elide:false` turns the cut into
`viz_over_budget` for callers that would rather fail.

### 5.4 Deferred: D2, SVG, PlantUML

- **D2** (phase 3): the best-looking modern DSL and a natural third
  emitter over the same contract — but no markdown host renders it,
  so it earns its place only after the format-neutral core is proven
  by two targets.
- **SVG**: generating SVG *source* requires computing layout, which
  is exactly what "code generation only" excludes. Note that boru can
  already build SVG by hand today — XML literals + `emit xml`
  (`design/XML-LITERAL.0.md` names SVG in scope as data) — so
  hand-layout SVG is user-land, not a viz gap. Layout-free SVG forms
  (treemaps, timelines) are a possible phase-3+ exploration and are
  deliberately not promised.
- **PlantUML**: JVM-bound, GPL-default; its unique asset (C4) is
  reachable via Kroki if ever needed. Not a target.

## 6. Composition examples

```
import "boru:viz"
import "boru:scry"
import "boru:io"

# 1. the system's module graph, ready to paste into a PR description
Viz.graph (Scry.module-graph) {}
#   ~> "flowchart TD\n  n0[\"boru:io\"] --> …"

# 2. who does my-report reach, two hops out — as DOT, because it is big
Viz.graph
  (Viz.focus (Scry.word-graph {}) {on:'my-report' depth:2})
  {to:'dot' direction:'lr'}

# 3. what shape is this value?
Viz.shape config {}
#   ~> a Mermaid tree of the config Map: keys as nodes, types annotated

# 4. what actually ran? (trace rows from scry, drawn as a sequence)
Viz.seq (Scry.trace [load-orders summarize] {}) {}

# 5. the repository's own knowledge graph — kg output draws unchanged
#    (run from kg/, extending kg/README.md's own query example)
import "./queries.boru"
def g (IO.read (make Pathon "out/graph.json"))
Viz.graph {edges: (KgQuery.edge-list g)} {title:'boru repo'}

# 6. record schemas as a class diagram
Viz.classes [(Scry.schema Order) (Scry.schema Customer)] {}

# 7. validation and visualisation from the same value
def wg (Scry.word-graph {})
Viz.cycles wg              # ~> [] — assert this in a test
IO.write (make Pathon "docs/words.mmd") (Viz.graph wg {})
```

Example 7 is the point of the shared contract: the graph a test
asserts about is byte-for-byte the graph the documentation draws.

## 7. Architecture

**What already exists (reuse, don't rebuild):**

- The embedded-boru module pattern: `cli.boru` / `sift.boru` —
  `//go:embed`, parse-once, fresh sub-registry, collect
  `export "Viz" {…}` (`lang/go/modules/cli.go:44-70`).
- The emitter signature + Options-schema conventions
  (`lang/go/modules/emitlang.go`).
- String/List/Map machinery in core plus `boru:string-util` /
  `boru:array-util` for join/sort needs; `typeof` for `Viz.shape`
  annotations.
- The docs/catalog/spec enforcement lattice: `registerDocs` /
  `registerExamples` (`docs_<name>.go`, test-enforced),
  `moduleCatalog` row (`lang/go/native/help/help_render.go`, pinned by
  `TestModuleCatalogMatchesModules`), `lang/spec/module-*.tsv` rows
  (ADR-003).

**What is new:**

- `lang/go/modules/viz.boru` — the module body: contract validation,
  transforms, and the per-format serialisers (all string building).
- `lang/go/modules/viz.go` — `BuildVizModule` scaffold on the
  `cli.go` pattern; registration in the `modules` map
  (`modules.go:23`).
- `lang/go/modules/docs_viz.go`, catalog row, `lang/spec/module-viz.tsv`,
  `viz_test.boru` + Go golden tests (§10).

No engine seams, no capabilities, no policy scopes, no new Go
dependencies.

File layout (mirrors existing modules):

```
lang/go/modules/viz.go          # builder: embed, parse-once, export collection
lang/go/modules/viz.boru        # the module, written in boru
lang/go/modules/viz_test.boru   # boru:test suite (multi-line goldens)
lang/go/modules/viz_test.go     # Go harness + golden/determinism tests
lang/go/modules/docs_viz.go     # registerDocs / registerExamples
lang/spec/module-viz.tsv        # executable spec rows (ADR-003)
```

## 8. Policy and safety

Every viz word is pure: no file, network, clock, process, or terminal
access, so no `lang/go/policy` scope gates the words themselves.
Module *imports* are gated separately, and the `sandbox` profile
denies them by default behind an explicit allowlist
(`lang/go/policy/profiles/sandbox.jsonic` — currently `boru:math-util`,
`boru:time-util`, `boru:io`, `boru:cli`). `boru:cli` is on that list
precisely because it is pure; viz qualifies on the same argument, so
phase 1 adds `boru:viz` to the sandbox allowlist (and to any profile
extending it) with a purity comment mirroring cli's. Any boru modules
`viz.boru` itself imports must be admitted the same way. Writing a
diagram to disk is the caller's composition with `boru:io` and rides
the existing `fileops`/`disk.write` gates.

The one safety obligation viz itself carries is **escaping**: node
labels come from arbitrary user data, and both Mermaid and DOT are
languages. The emitters guarantee that data can never inject diagram
syntax (synthetic ids, always-quoted labels, entity/backslash escapes
— §5.1, §5.2), and the negative spec rows pin that guarantee (§10).

## 9. Delivery phases

Ordered value-per-effort; each phase is shippable alone.

1. **Phase 1 — the contract and the generic emitters.** §3 shapes;
   `Viz.graph`, `Viz.tree`, `Viz.shape`, `Viz.kinds`; Mermaid + DOT;
   budgets, elision, escaping, determinism; the sandbox-profile
   allowlist line (§8); full docs/spec/test lattice. No scry
   dependency: kg output, adjacency literals, and hand-built Maps are
   already drawable (§6 ex. 5).
2. **Phase 2 — the system views and transforms.** `Viz.seq` and
   `Viz.classes` over the scry phase-1/2 words
   ([BORU-SCRY.0.md](BORU-SCRY.0.md) §8); `Viz.focus`,
   `Viz.collapse`, `Viz.cycles`.
3. **Phase 3 — reach extensions (each maintainer-gated).** The D2
   emitter over the unchanged contract; share-link words
   (`mermaid.ink` needs URL-safe base64 from `boru:bin-util`; Kroki
   additionally needs a `BinUtil.deflate` word — a candidate addition,
   §12 Q4); layout-free SVG exploration (§5.4), only if wanted.

## 10. Test discipline

- **Spec TSV** (`lang/spec/module-viz.tsv`): transforms and error
  words are single-line-expressible and get full positive/negative row
  pairs there (`Viz.cycles`, `Viz.focus` via `canon`, every error
  code, budget behaviour with `elide:false`). Emitter outputs are
  multi-line and TSV cannot hold them — the `boru:report` precedent
  applies: multi-line goldens are pinned in tests, TSV keeps the
  single-line and error forms.
- **Goldens**: `viz_test.boru` (boru:test) and `viz_test.go` pin exact
  emitter output for small fixed inputs, including every escaping
  trap: labels containing `end`, `"`, `#`, `-->`, `];`, embedded
  newlines (multi-line labels), ids starting `o`/`x`, unicode.
- **Determinism**: permuting input node/edge order must not change one
  output byte; running twice must not either.
- **Every positive test pairs with a negative one** (repo rule):
  malformed graphs (`edges` row without `to`), bad opts keys (the
  Options-schema rejection), unsupported view×format combinations.
- Coverage: the Go scaffold meets ADR-008 via `make cover-gate`; the
  boru body is exercised via `boru test -coverage` with the source
  registered for attribution (the `cli.go` coverage-tagging pattern).

## 11. Prior art (research notes, 2026-08)

Condensed from a survey of diagram DSLs, architecture-as-code tools,
and language-integrated visualisation; kept here because each line
justified a §2 principle or a §4/§5 decision.

- **Rendering surfaces.** GitHub renders only Mermaid natively (since
  2022); GitLab bundles Mermaid + PlantUML and gates the rest behind
  Kroki; no markdown host renders DOT or D2. Mermaid embedders cap
  `maxEdges` at 500, non-overridable. ([docs.github.com][gh-mermaid],
  [mermaid #5042][mm-caps])
- **Model/view separation.** Structurizr: one typed model, every
  diagram a view with include/exclude expressions; LikeC4 generalises
  with user-defined element kinds; Ilograph's "perspectives" essay
  argues the master-diagram anti-pattern directly.
  ([structurizr][structurizr], [likec4][likec4], [ilograph][ilograph])
- **Readability = pruning.** goda's set-algebra (`reach`, `cut`,
  `shared`) over package graphs; dependency-cruiser's
  `--focus`/`--reaches`/`collapsePattern` plus rules rendered into
  the picture; pydeps' default 2-hop bacon distance; Doxygen's
  50-node cap with explicit elision. ([goda][goda],
  [dependency-cruiser][depcruise], [pydeps][pydeps],
  [doxygen][doxygen])
- **Language-integrated precedents.** Elixir's `Kino.Process`
  (supervision trees and message-sequence diagrams as *Mermaid text*,
  shipped in the box) is the closest analogue to viz+scry; Glamorous
  Toolkit's moldable views and Racket's pict argue for cheap per-type
  views and picture-values; Clojure rhizome's `(nodes, adjacency)`
  contract with a pure graph→DOT layer is §3.1's ancestor; Wolfram
  demonstrates "the language draws its own expressions" at full
  scale. ([kino][kino], [gtoolkit][gt], [pict][pict],
  [rhizome][rhizome])
- **In-binary rendering (investigated, rejected with the
  code-generation-only decision).** It *is* feasible — D2 is now pure
  Go end-to-end (dagre and ELK ports, MPL-2.0, ~tens of MB; ASCII
  output since v0.7.1) and goccy/go-graphviz embeds real Graphviz via
  wazero (MIT wrapper, ~1.4 MB wasm) — recorded so a future
  revisiting starts from facts, not archaeology. Mermaid has no
  browser-free renderer and never will soon ([mermaid #3650][mm-ssr]).
  ([d2][d2], [go-graphviz][goccy])

[gh-mermaid]: https://docs.github.com/en/get-started/writing-on-github/working-with-advanced-formatting/creating-diagrams
[mm-caps]: https://github.com/mermaid-js/mermaid/issues/5042
[mm-ssr]: https://github.com/mermaid-js/mermaid/issues/3650
[structurizr]: https://docs.structurizr.com/
[likec4]: https://likec4.dev/
[ilograph]: https://www.ilograph.com/blog/posts/breaking-up-the-master-diagram
[goda]: https://github.com/loov/goda
[depcruise]: https://github.com/sverweij/dependency-cruiser
[pydeps]: https://github.com/thebjorn/pydeps
[doxygen]: https://www.doxygen.nl/manual/diagrams.html
[kino]: https://kino.hexdocs.pm/Kino.Process.html
[gt]: https://gtoolkit.com
[pict]: https://docs.racket-lang.org/pict/
[rhizome]: https://github.com/ztellman/rhizome
[d2]: https://github.com/d2lang/d2
[goccy]: https://github.com/goccy/go-graphviz

## 12. Open questions for the maintainer

1. **`emit` interop.** Viz emitters deliberately share the
   `[value opts] ~> String` emitter signature, so they should be
   usable as Function-valued emitters (`emit <fn> <data>`) without
   unfreezing the kind registry. Confirm this works for module
   exports referenced with `/r`, or note the gap. (Leaning yes —
   zero-cost if the reference form cooperates.)
2. **A rich-display protocol.** Racket's convertible / Jupyter's
   mime-bundle pattern — a `view` word or REPL affordance that
   auto-renders drawable values (e.g. the playground rendering
   Mermaid inline) — is out of scope here but is the natural sequel.
   Separate design note when wanted.
3. **Transform words' home.** `focus`/`collapse`/`cycles` live in viz
   for v1. If graph algebra grows (reach, shared, cut à la goda), it
   splits into `boru:graph-util`. (Leaning: keep in viz until it
   hurts.)
4. **`BinUtil.deflate`.** Kroki share-links need raw-deflate +
   URL-safe base64. Is a `deflate` word in `boru:bin-util` acceptable
   (phase 3)? mermaid.ink links need only URL-safe base64.
5. **Minted shape types.** Should §3's Graph/Trace/Schema shapes
   become module Record refinements (`Viz.Graph`) for checkable
   signatures, or stay plain Maps? (Leaning plain Maps in v1 — the kg
   bundle precedent — with refinements addable later without breaking
   Map-shaped callers.)

No ADR entry is proposed — per repo policy this stays a `design/` note
until a maintainer says otherwise.
