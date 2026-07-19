# `aql:fmt` — a formatter as a core module, and the XSLT question

Status: Phases 1, 3-rules, 3-vocabulary, 4-embedded, and the Phase-2 seam
have **landed**; the trivia-preserving tabnas CST (Phase 2 front end) and the
final declarative rule-set conversion — retiring the Go layout code by
routing `Fmt.format` through AQL rules (Phase 3) — remain. See "Phased plan"
for the per-phase state. This is a discovery/design note, not an ADR (see
`lang/go/CLAUDE.md`, "ADRs — only on explicit instruction").

## Context

`aql fmt` today is a CLI subcommand (`cmd/go/internal/fmt`) that calls a
self-contained, hand-rolled pretty-printer, `formatter.Format(src) string`
(`lang/go/formatter/format.go`). It has three limitations we want to remove:

1. **No runtime API.** Formatting is reachable only from the shell, not from
   AQL code. We want `import "aql:fmt"` / `Fmt.format`.
2. **Rules are baked into Go control flow.** The layout policy (spacing,
   wrapping at 70 cols, type-name capitalisation, map-shorthand expansion,
   `fn` bracketing) is imperative Go. We want the rules expressed
   **declaratively, as AQL**, so the formatter's policy is data the language
   can read and extend — the same "the tool documents/defines itself" stance
   as `describe`.
3. **Its own tokenizer.** It re-lexes source with a bespoke scanner rather
   than the real parser. We want it to **parse with a tabnas parser**, passed
   in as a **parameter**, so there is one lexing story and the parser is
   swappable/testable.

Plus new behaviour: **72-column** lines (was 70) except **backtick/template
content is left verbatim**; **single fn params/returns drop their brackets**;
**all code examples everywhere** (docs, `describe`, …) run through fmt; and a
way for user code to format **embedded** AQL inside Markdown/HTML via special
comment markers.

## Findings that constrain the design

These come from reading the parser, the module system, and the formatter
(see the four investigation threads summarised below).

- **F1 — The semantic parser is lossy.** `eng/go/parser.Parse` returns a flat
  `[]eng.Value` stream (not a CST). It **drops comments** (jsonic consumes
  `#`/`//` at the lex layer — `parse_test.go:556-572`), **alphabetises map
  keys** (jsonic hands back a Go `map[string]any`; the converter sorts —
  `parse.go:876,1106,1251`), **collapses backtick strings** to plain strings
  or lossy `InterpString`s (`grammar.go:322-381`, no `InterpString` arm in
  `canon.go`), and **discards whitespace**. Formatting off this stream would
  reorder users' maps and delete their comments. Unusable for a faithful
  formatter as-is.
- **F2 — tabnas' *lexer* is not lossy.** `jsonic.NewLex(src)` is drivable
  directly and exposes raw token text (`Token.Src`), positions (row/col/byte
  offset), and quote spelling (`Text.Quote` distinguishes `` ` `` / `'` /
  `"`). Custom `LexMatcher`s (priority bands match=1e6 … space=3e6) can
  intercept spans **before** the built-in space/comment consumer — the exact
  mechanism AQL already uses for template literals and minilang. So a
  **lossless CST is buildable on the lexer layer**: comments (medium effort —
  capture is easy, re-attachment to the right node is the work), backtick
  verbatim (low — read `Token.Src`), source-ordered maps (parse maps at the
  token layer instead of via `MapRef`).
- **F3 — Import direction pins placement.** `modules → native → eng`. The
  `formatter` package is a leaf both can import (it has no engine deps today).
  Therefore: the shared driver must stay a **plain package importable by both
  `native` and `modules`**; describe/help (in `native`) cannot call the
  `aql:fmt` *module* directly (that would be `native → modules → native`).
  Doc-example formatting must run at the **CLI / `genhelp`** layer, which
  already builds a full registry and runs AQL.
- **F4 — The XSLT substrate already exists.** AQL has, natively:
  type/shape **multiple dispatch** (most-specific-wins over the lattice) — a
  direct analogue of `<xsl:template match="…">`; **`patrun`** (property-bag
  router, specificity-ordered, stores fns as values) — a priority/mode table;
  **`walk`** (depth-first pre/post-order tree rewrite with node replacement) —
  `apply-templates`; and **`fold`/`each`/`scan`** — the serialisation
  catamorphism. No new engine machinery is required to express template rules.
- **F5 — The bracketless `fn` form already *parses*.** `fn x:Integer
  [Integer] [body]` and `fn x:Integer Integer [body]` are valid triple-form
  spellings (`lang/spec/fn-triple.tsv:28,30,39`); only a **sole `List`-typed
  param** must keep the bracketed spec-list form (`tnot List` guard,
  `fn-triple.tsv:49`). No renderer emits the bracketless form yet — this is a
  **render-side** change, not a grammar change.

## Target architecture (layered, cycle-safe)

```
        ┌─────────────────────────────────────────────┐
  cmd   │ aql fmt (files, .md/.html)   aql describe    │
        └───────────────┬───────────────────┬─────────┘
                        │ shared Go driver   │ genhelp: examples → fmt
        ┌───────────────▼───────────────┐    │
 modules│ aql:fmt  →  Fmt.format,        │    │
        │            Fmt.* rule vocab    │    │
        └───────────────┬───────────────┘    │
        ┌───────────────▼────────────────────▼─────────┐
 leaf   │ package fmtcore (importable by native+modules)│
 pkg    │  ┌─────────────┐  ┌───────────────┐  ┌──────┐ │
        │  │ lossless    │→ │ document      │→ │ emit │ │
        │  │ tabnas CST  │  │ algebra + rule│  │(72col│ │
        │  │ (param:     │  │ engine (AQL   │  │ back-│ │
        │  │  parser fn) │  │ decl. rules)  │  │ tick)│ │
        │  └─────────────┘  └───────────────┘  └──────┘ │
        └───────────────────────────────────────────────┘
```

**Shared driver.** One Go entry point, `formatter.Format` (evolving into
`fmtcore`), is called by *both* the CLI and the `Fmt.format` word. We
deliberately do **not** route file bytes through `a.Run("… Fmt.format …")`
(escaping hell, one registry per file). "The CLI calls into the module" is
satisfied in spirit by a single shared implementation, not by the CLI
literally dispatching an AQL word per file.

**Parser as a parameter.** `fmtcore.Format` takes a `Parse` function value
(the lossless tabnas CST builder in production; a stub in tests). This is the
"parse using a tabnas parser (which is a parameter to the formatting)"
requirement, made concrete.

## The XSLT investigation

The request: *algorithmically investigate whether an XSLT-like approach works,
and whether XSLT itself could be expressed by `aql:fmt`.*

### XSLT in one paragraph

XSLT is a declarative tree-transformation language: a stylesheet is a set of
**template rules**, each with a **match pattern** (an XPath) and a body; the
processor walks the source tree and, at each node, selects the
**most-specific / highest-priority** matching template, instantiates its body
(literal result nodes + `<xsl:value-of>`), and recurses via
`<xsl:apply-templates select="…">`. **Modes** partition rule sets;
**priorities** break ties. XSLT (with its template layer) is Turing-complete.

### Does an XSLT-shaped approach work for formatting?

Formatting *is* a tree transformation: `parse-tree → layout-document → text`.
The XSLT control model maps onto AQL primitives essentially 1:1 (F4):

| XSLT construct | AQL realisation |
|---|---|
| `<xsl:template match="Integer">` | a rule fn dispatched by node **type/shape** (multiple dispatch, most-specific-wins over the lattice); or a `patrun` pattern keyed on node attributes |
| template **priority** / import precedence | `patrun` specificity + alphabetical tie-break, or lattice `Rank` |
| **modes** (`mode="toc"`) | a `mode` key in the `patrun` subject, or a separate rule table |
| `<xsl:apply-templates select="children">` | `walk` (pre/post-order) or `fold`/`each` over child nodes, re-entering rule dispatch |
| `<xsl:value-of>` / literal result | AQL producing **document** fragments (the `Fmt.*` vocabulary below) |
| `<xsl:param>` / `<xsl:with-param>` | ordinary fn params / captured closure state |

So an XSLT-style declarative formatter is not only feasible, it is the
*natural* AQL expression of the problem. The rule set becomes a collection of
`Fmt.rule`-registered fns (or a `patrun` table) keyed by node kind; a driver
`apply`s them depth-first and folds the resulting document to text.

### Could XSLT *itself* be expressed by `aql:fmt`?

Partly, and instructively:

- **The template-dispatch + apply-templates core: yes.** If `aql:fmt`'s rule
  engine is generalised from "match AQL fmt-CST nodes" to "match **arbitrary**
  nodes by pattern, emit a sequence, recurse on selected children", it is a
  general match-driven tree-transform engine — the XSLT execution model. Both
  are declarative, both Turing-complete, so any XSLT transformation has an
  `aql:fmt` rule-set equivalent in principle.
- **The gap is XPath.** XSLT's selection power lives in XPath — axes
  (ancestor/descendant/following-sibling), positional predicates
  (`[position() mod 2]`), and value predicates. AQL's analogues are partial:
  `Reach`/dotted paths, `StructUtil.getpath`/`selector`, and `patrun`'s
  attribute matching cover child/attribute selection and equality predicates,
  but not the full axis algebra. Expressing arbitrary XSLT would require a
  small **path-query vocabulary** (an XPath-subset) layered on `walk` — a
  well-scoped addition, not a redesign.

**Conclusion.** Adopt the XSLT model for the formatter (template rules keyed by
node kind, `apply` recursion, a document-algebra output). Design the rule
engine to be **node-generic** so it is, modulo an XPath-subset query layer, a
superset capable of expressing XSLT — but do **not** build the XPath layer now;
it is out of scope for formatting and belongs behind its own `parse`/`query`
module if ever wanted.

### The output side: a document algebra

XSLT emits a result tree; a *formatter* needs width-aware layout, so the rule
bodies produce a **Wadler/Prettier-style document** rather than raw text. The
proposed `Fmt.*` vocabulary (the "formatting words"):

- `Fmt.text s` — literal text (never broken).
- `Fmt.verbatim s` — literal text exempt from width (backtick/template bodies).
- `Fmt.line` / `Fmt.softline` / `Fmt.hardline` — a space-or-break / nothing-or-break / forced break.
- `Fmt.concat [d …]`, `Fmt.group d` (flatten if it fits ≤ width, else break),
  `Fmt.indent n d`, `Fmt.nest d`.
- `Fmt.kind node` — the node's dispatch key (an XSLT match pattern): a
  `$kind`-tagged Map's tag, else `map` / `list` / `scalar`. **LANDED.**
- `Fmt.children node` — the child sequence (apply-templates' node list): a
  Map's `{$kind:'entry' key value}` entry nodes, a List's elements, else
  `[]`. **LANDED.**

A Go (or AQL) `Fmt.render width doc` prints the document, and `width` defaults
to **72**, with `Fmt.verbatim` spans passed through untouched (the
backtick-verbatim rule falls out of the algebra, not a special case in every
wrapper).

`Fmt.kind` and `Fmt.children` are deliberately **pure value→value
transforms** — classification and child-exposure only. Dispatch and recursion
stay in ordinary AQL: fetch the rule fn from a table keyed by kind, apply it,
recurse on children. So the rule engine itself is AQL (no fn-invocation or
registry-threading in Go), and a whole formatter reads like an XSLT
stylesheet — a rule table plus a one-line `apply` driver:

```
import "aql:fmt"
def apply fn [nd:Any Any [nd (rules get (Fmt.kind nd))]]
def rules {
  scalar: (nd:Any => (canon nd))
  entry:  (nd:Any => ({fmt:'concat' parts:[nd.key ':' (apply nd.value)]}))
  map:    (nd:Any => ({fmt:'group' body:{fmt:'concat' parts:((Fmt.children nd) each [apply])}}))
}
Fmt.render 72 (apply {a:1 b:{c:2}})    # → "a:1b:c:2"  (nested map recursed)
```

The `apply` driver applies a fn fetched from the table (a dynamic dispatch the
bytecode compiler leaves to the interpreter). See
`modules/fmtrule.go`, the `module-fmt.tsv` rows, and the end-to-end
`TestFmtDeclarativeFormatter`.

## Existing rules → declarative form

The current `format.go` rules restated as template rules (Phase 3 encodes
these as `Fmt.*` AQL):

| Current behaviour (`format.go`) | Template rule |
|---|---|
| 2-space indent; no tabs | `Fmt.indent 2` in container rules |
| collapse blank runs to one; trailing newline | root rule |
| comma → no space before, one after; `:`/`.`/`?` attach | token-adjacency rules |
| no space inside `[] {} ()` | container rules emit `open · body · close` |
| type-name capitalisation in type position (not after `def`/`refine`, not before `:`, not `record`/`object`/`function`) | a rule on type-annotation nodes |
| map shorthand `{foo}`→`{foo:foo}`, `{foo?}`, `{foo/r}` | map-entry rule |
| wrap at width; `fn`/trailing-container/statement strategies | `Fmt.group` + width |
| **NEW: width 72** | `Fmt.render` default |
| **NEW: backtick verbatim** | `Fmt.verbatim` |
| **NEW: single param/return unbracketed** (except sole `List` param) | fn rule; keep spec-list form when the sole param conforms to `List` (F5) |

## Phased plan

- **Phase 1 — module + runtime API (DONE).** `lang/go/modules/fmt.go`
  (`BuildFmtModule`, `FmtNatives` with `format` : String→String wrapping
  `formatter.Format`, `BarrierPos:-1`), `docs_fmt.go`, catalog registration in
  `modules.go` + `help_render.go`, tests in `fmt_test.go` (both handler arms +
  end-to-end dispatch). `import "aql:fmt"` / `Fmt.format` works; `aql describe
  "aql:fmt"` lists it; the CLI keeps calling the shared driver.
- **Phase 2 — parser as a parameter.** DONE (seam): `Format(src)` →
  `FormatWith(src, Parse)`, with `Parse` a `func(src) *Node` front end and
  `DefaultParse` the built-in lossless lexer. The layout rules and emitter
  run on the tree regardless of front end. REMAINING: the trivia-preserving
  tabnas CST front end that plugs into `Parse`. Two findings from the
  investigation pin the shape and the blocker:
  - **Trivia lexing is solved (mechanism found).** `Lex.Next` skips the
    SP/LN/CM IGNORE set, but `Lex.Config.IgnoreSet` is a **per-instance**
    map (`{TinSP,TinLN,TinCM}` by default, "plugins can customize"). A
    `jsonic.NewLex(src, cfg)` whose `cfg.IgnoreSet` omits those Tins makes
    `Next` return the space / line / **comment** tokens verbatim (each
    carries `Token.Src` + row/col/byte offsets). So a trivia-preserving
    token stream needs no custom matcher — just an ignore-set override.
    The cleanest wiring reuses the existing `buildTree([]Token)` by writing
    only a `tabnasTin → formatter.TokenKind` adapter over that stream.
  - **The real blocker is layering (F3), a DECISION not a task.** A *raw*
    `jsonic.NewLex` is not the AQL grammar: AQL's backtick-verbatim /
    template literals and `+minilang/…/` literals are recognised by the
    eng parser's *configured* jsonic instance (custom tokens + rules in
    `eng/go/parser/grammar.go`), not by a bare lexer. A raw-lex front end
    would mis-tokenise exactly the spans the new rules must keep verbatim.
    The two honest options: (a) the `formatter` package takes a dependency
    on `eng/go/parser` to reuse the configured instance — abandoning its
    engine-dep-free leaf status (F3), which would move doc-example
    formatting and the import graph; or (b) the trivia-lexer lives in a new
    leaf package that re-declares AQL's backtick/minilang lex customisations
    independently — duplicating grammar knowledge. Pick (a) or (b) before
    writing the front end; both are viable, neither is mechanical.
  Until then keep the current lossless hand-lexer as `DefaultParse` — it
  already preserves comments/newlines/backticks — so fmt stays correct.
- **Phase 3 — rules.** Rule behaviour DONE in the Go formatter: 72-col,
  backtick-verbatim (`scanBacktick`), and bracketless single param/return
  (`elideFnBrackets`). The declarative **substrate** is DONE: a
  Wadler/Prettier document algebra (`formatter.Doc`/`RenderDoc`) exposed as
  the runtime word **`Fmt.render`**, driven by an AQL doc tree
  (`{fmt:'group' body:…}` etc.) — "formatting words in the fmt module,
  expressed as AQL data". The XSLT-style **rule vocabulary** is now also DONE:
  the two pure words **`Fmt.kind`** (node → dispatch key) and
  **`Fmt.children`** (node → child sequence) make a declarative rule table +
  a one-line `apply` driver the natural AQL expression of a formatter
  (`modules/fmtrule.go`, demonstrated end-to-end by
  `TestFmtDeclarativeFormatter` — see "The output side" above). REMAINING: port
  the built-in AQL-source layout rules (the table above) onto this vocabulary
  and route `Fmt.format` through them, retiring the Go rule code (the largest
  piece — a full rule-set port under the coverage gate, and the one that must
  reconcile with the lexical formatter, R4).
- **Phase 4 — embedded formatting + fenced doc blocks DONE; describe/inline
  examples REMAINING.** `aql fmt` reformats ```` ```aql ```` fences and
  `<!-- aqlfmt --> … <!-- /aqlfmt -->` marker regions in Markdown/HTML (CLI
  dispatch by extension; runtime words `Fmt.format-markdown` /
  `Fmt.format-html`). The prose docs' ```` ```aql ```` blocks are now run
  through fmt (README) and pinned fmt-clean by a gate
  (`test/go/docexamples`). A formatter fix keeps a trailing `# comment`
  attached to its statement (never wrapped off), so annotated example lines
  survive fmt and the `# returns` extractor still pairs them. REMAINING: the
  inline `expr # returns X` prose examples (not in fenced blocks) and the
  `describe`/help worked examples. Both use deliberate column alignment that
  fmt collapses — a visible-output change to the docs and the `describe`
  command warranting maintainer sign-off, not a silent migration.

## Risks

- **R1 — Coverage gate (ADR-008).** Every new Go statement needs a test or a
  proof-carrying `//covergate:allow`. Phases 2–3 add real logic; budget test
  effort accordingly. Phase 1 is fully covered.
- **R2 — Blast radius of the render changes.** Bracketless params and 72-col
  change the output of *every* formatted fn/long line; expect wide updates to
  golden tests, `lang/spec/*.tsv`, and docs. Doing them in Phase 3 (behind the
  declarative engine) keeps the churn to one reviewable change.
- **R3 — Comment re-attachment** is the genuinely hard part of Phase 2
  (placing a captured comment against the right CST node). This is why the
  current formatter is lexical; the lossless CST must solve it, not inherit it.
- **R4 — Two formatting paths during transition.** Until Phase 3 subsumes it,
  `formatter.Format` (lexical) and any `fmtcore` path must be kept reconciled
  or one clearly authoritative.
