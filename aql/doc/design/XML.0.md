# AQL XML Data Embedding Design

Status: design draft, no implementation.

This document specifies how AQL programs embed XML as a first-class
data value: literal syntax, runtime type, interpolation, querying,
and the `<aql-embed lang="...">` mechanism for embedding foreign
syntaxes inside an XML body.

The format is **not** an alternate syntax for AQL programs
themselves — AQL words stay in their concatenative source form.
XML is a data type, like list or map, with a recognisable literal
form. Everything below describes how XML *values* are written,
parsed, queried, and templated; word definitions, function bodies,
and program structure are unchanged.


## 1. Why XML as data?

AQL already has list and map literals. XML adds a third tree-shaped
data type that natively expresses:

- Documents — HTML, SVG, RSS, JATS articles, OpenDocument fragments.
- Configuration with mixed structure — Spring beans, .NET configs,
  Maven POMs, RelaxNG schemas.
- Mixed content where order matters and text is interspersed with
  elements — technical docs, runbooks, books, articles.
- Round-trippable input: parsed XML preserves attribute order,
  comments, and whitespace; converting to a map loses all three.

Without an XML literal, AQL users have to build trees through
deeply nested map literals or by parsing strings at runtime. An
in-source literal gives XML the same convenience that `[…]` and
`{…}` give lists and maps.


## 2. XML literal syntax

An XML literal is recognised by an opening `<` followed by a tag
name. The body matches XML 1.0 well-formedness rules with two
TSX-flavoured extensions: single-brace expression interpolation
(§3) and `<aql-embed>` for foreign payload (§6).

```
def page <root>
  <article id="1">
    <title>Hello</title>
    <body>This is <em>important</em>.</body>
  </article>
</root>

page                              # value of type Object/Xml
```

### 2.1 Recognition

The lexer recognises an XML literal when it sees `<` immediately
followed by a name-start character (letter or `_`), a `?` (PI), or
a `!` (comment / CDATA / DOCTYPE). To avoid clashing with the
comparison word `<`, the lexer requires:

- No whitespace between `<` and the tag name. `< foo` is the
  comparison word followed by an atom; `<foo>` is an XML literal.
- The position is one where a value is expected — start of a
  statement, after an open paren, after a forward-collection
  marker, etc. The same disambiguation today distinguishes `[` and
  `{` literals from operators.

### 2.2 Termination

An XML literal ends at the matching close tag for the outermost
element. Self-closing elements (`<br />`) terminate immediately.
Fragments (`<>...</>`) terminate at the matching `</>`.

### 2.3 Self-closing and fragment shorthands

Self-closing tags (`<br />`, `<img src="..." />`) match XML 1.0 and
JSX — equivalent to a tag with no children.

Fragments (`<>...</>`) wrap a sibling list without introducing a
parent element. The literal value is then a node-list rather than
a single rooted element — useful when interpolating a fragment
into another tree, or when the producer of the value does not have
a natural single root.


## 3. Interpolation

Inside an XML literal, single-brace `{expr}` switches into AQL
expression mode. The result is spliced into the tree at that
point.

```
def name "alice"
def greeting <p>Hello, {name}!</p>
```

### 3.1 Where interpolation appears

- **In tree text** — `<p>Hello, {name}!</p>` — the expression
  result is converted to a text node and concatenated with the
  surrounding literal text.
- **In an attribute value** — `<a href={url}>` — the result is
  converted to a string and used as the attribute value. Mixed
  literal text and expressions are also allowed inside a quoted
  attribute: `<a href="/users/{id}">`.
- **As an element child** — `<wrap>{items}</wrap>` — if the
  result is a node or a list of nodes, it is spliced as children.
  If it is a primitive (string, number, boolean), it becomes a
  text node.

### 3.2 Why `{}` instead of `${}`

JavaScript template-literal `${}` is the right call for *strings*,
where every literal `{` is a hazard — CSS, JSON, regex
quantifiers, set notation all use `{}`. Inside XML *tree* text the
trade-off is reversed:

- `<` and `>` are already reserved as tag delimiters; readers
  expect the tree language to "own" some characters.
- `{` is rare in markup-like text — most natural-language prose
  has no curly braces at all.
- Once `{` is the chosen sigil, the leading `$` is pure noise.

The escape mechanism — `{'{'}` to emit a literal brace, or the
character entity `&#x7B;` — covers the rare collisions. The same
escape pressure applies to literal `<` and `>` already, so no new
machinery is introduced for this.

Backtick template strings inside an `{...}` expression keep using
`${...}`: we are back in string mode there, with the same boundary
and the same convention as JS itself. AQL's existing backtick
interp (CLAUDE.md, "Template string interpolation") is unaffected.

### 3.3 Why single braces, not doubled `{{ ... }}`

Some templating systems (Mustache, Handlebars, Vue's text
interpolation) use `{{ ... }}` to dodge the rare collision with
literal `{}` in markup. JSX/TSX uses single `{}` for two reasons
we inherit:

1. The escape mechanism is good enough for the rare case.
2. Single braces compose cleanly with the rest of the language —
   a JSX expression is a JS expression and can carry object
   literals (`{ key: value }`) as a natural sub-form.

AQL similarly uses `{...}` for map literals at the language level.
Inside an XML literal, `{...}` is unambiguously an expression
boundary; map literals nest as `{( {key: value} )}` if the outer
braces would be mistaken for the expression boundary — the same
pattern TSX uses for returning an object from an arrow function:
`() => ({foo: 1})`.

### 3.4 Parsing strategy — single grammar, not nested parsers

JavaScript engines (V8, SpiderMonkey, JavaScriptCore) and parsers
(Babel, Acorn, esprima, swc, sucrase) handle JSX and
template-literal interpolation as a *single grammar* rather than
re-entering a fresh parser. The lexer carries a small mode stack —
`xml`, `expr`, `tplstr` — and pivots on the boundary tokens. Each
pivot increments or decrements the relevant nesting counter so
that matching delimiters can be paired back up regardless of
depth.

Single-grammar advantages over a sub-parser:

- One token stream, one error-reporting context, one set of source
  positions.
- Nested interpolations and nested elements at arbitrary depth
  work for free.
- No marshalling between parser instances, no doubled error
  infrastructure, no boundary semantics to define.

AQL already follows this pattern for backtick template strings —
the jsonic integration uses rule-state flags (`K["aql_tpl"]`) plus
dedicated tokens (`#BT`, `#IS`, `#TL`) inside the same parser.
The XML literal extends the approach with two additional rules:

- An `xml` rule opened on `<` that accepts elements, fragments,
  text, and `{...}` expression children.
- An `attr` rule that accepts string-literal-with-interpolation or
  `{...}` expression values for attribute right-hand sides.

Both pivot back into the regular `val` rule on `{` and return on
the matching `}`.


## 4. Runtime representation

XML literals produce values of type `Object/Xml`. The runtime
preserves:

- Element name, attributes (in source order), and namespace
  declarations.
- Children as an ordered list of nodes (elements, text, comments,
  PIs, CDATA, `aql-embed` blocks).
- Source position, when parsed from a literal.

Companion words for working with XML values:

| Word | Purpose |
|------|---------|
| `xml-parse` | Parse a string into an `Object/Xml` value. |
| `xml-print` | Serialise an `Object/Xml` value to a string. |
| `xml-to-map` | Lossy convert to AQL map (canonicalised). |
| `map-to-xml` | Build an `Object/Xml` value from a map skeleton. |
| `xml-attr` | Get/set an attribute value on a node. |
| `text` | Concatenate text content of a subtree. |
| `cs/...` | Apply a CSS selector — see §5. |


## 5. Querying — CSS selectors

XPath has been the historical default for XML query. CSS selectors
are a more attractive choice for AQL:

- The selector vocabulary (descendant, child, attribute,
  `:nth-child`, etc.) is what most working programmers already
  know from the browser.
- Selector strings are far shorter than equivalent XPath for the
  common cases (`.foo > .bar` vs `descendant::*[contains(@class,
  "foo")]/child::*[contains(@class, "bar")]`).
- The selector engine is easier to implement and keep small than a
  full XPath 3.1 evaluator.

### 5.1 Prior art

| Library              | Language    | What it offers                                                                  |
|----------------------|-------------|---------------------------------------------------------------------------------|
| **Cheerio**          | Node.js     | jQuery-style API over parse5 / htmlparser2; the de facto server-side scraper.   |
| **jQuery / Sizzle**  | Browser JS  | The selector engine that popularised the pattern.                               |
| **BeautifulSoup**    | Python      | `.select()` accepts CSS; `.find_all()` accepts predicates; both compose freely. |
| **lxml.cssselect**   | Python      | Compiles CSS to XPath internally — proves the equivalence and lets users pick.  |
| **Nokogiri**         | Ruby        | Both `.css()` and `.xpath()` on the same DOM, with implicit conversion.         |
| **Goquery**          | Go          | jQuery-style chainable API on top of `golang.org/x/net/html`.                   |
| **Scrapy / Parsel**  | Python      | XPath and CSS, plus chained `.get()`/`.getall()` and `re()` for regex on hits.  |

### 5.2 Notable design features worth borrowing

From the survey:

- **Chained, composable accessors.** Cheerio's
  `$('div.x').children('.y').first()` reads as a pipeline; AQL's
  concatenative shape is naturally pipeline-like, so a CSS
  selector word fits naturally.
- **Mixed selector types.** BeautifulSoup lets you pass a CSS
  selector, an attribute dict, or a predicate function to the same
  `find_all`. The same arg signature in AQL
  (`Selector | Map | FnDef`) gives users freedom without
  bifurcating the API.
- **Pseudo-classes for shape, not state.** `:has(...)`, `:not(...)`,
  `:contains(...)` are all data-shape predicates that translate
  cleanly to AQL filter words. Browser-only pseudos (`:hover`,
  `:focus`) are dropped because they have no analogue in static
  documents.
- **Result is itself a queryable collection.** Goquery returns a
  `*Selection` which is itself selectable. AQL should make
  selector results node-lists that accept further selector words
  as suffixes — chaining falls out of the existing
  forward-precedence rule.
- **Regex post-filter.** Parsel's `re()` and Scrapy's `.re_first()`
  show that regex composition on selector output is a frequent
  need; pairing CSS selection with `rm/.../` (see `MINILANG.0.md`)
  closes the loop without a new word.
- **Compile-once selectors.** Sizzle and Goquery both expose
  precompiled forms. AQL's `cs/` mini-language literal is already
  a compiled value, so this falls out for free.
- **`is`/`not` for predicate use.** jQuery's `$el.is('.foo')` is
  the same selector grammar reused as a predicate. AQL can do this
  with a single word that dispatches on a `node + cs/...`
  signature.

### 5.3 Surface for AQL

Reuse the `cs/` mini-language prefix introduced in `MINILANG.0.md`:

```
def page <root><a class="x">1</a></root>
page cs/a.x text                          # => "1"
```

For multi-step queries, the result is a list and the suffix
collection rule applies normally:

```
page cs/article cs/h1 first
```

Companion predicate / accessor words:

- `xml-attr` — take a node and an attribute name, return the
  attribute value (or `none`).
- `text` — extract concatenated text content of a subtree.
- `xml-is` — predicate version of `cs/`; tests whether a node
  matches the selector.


## 6. `<aql-embed>` — foreign syntaxes inside XML

Inside an XML literal, the special element `<aql-embed lang="...">`
declares that its content is in another syntax, not XML:

```
def page <root>
  <article>
    <aql-embed lang="markdown">
      # Heading
      Some *emphasised* text with a [link](http://example.com).
    </aql-embed>
  </article>
  <pre>
    <aql-embed lang="json">
      { "id": 1, "name": "alice", "tags": ["a", "b"] }
    </aql-embed>
  </pre>
  <script type="text/aql-pattern">
    <aql-embed lang="regex">
      ^[a-z][a-z0-9_]*$
    </aql-embed>
  </script>
</root>
```

The XML lexer skips structural tokenisation of the body — it does
not parse `<` or `>` or `{` inside an `<aql-embed>` body — and
hands the raw content to a registered handler keyed by `lang`.
Handlers parse the body and produce an AQL value: typically a
`MiniLang` subtype, a string, a record, or a deferred parse-result.
Unknown `lang` values are surfaced as opaque strings carrying the
body and language tag, so a document with unfamiliar embeddings
still loads.

This mirrors the inline mini-language facility (`MINILANG.0.md`):
the `xy/` prefix gives a compact inline form for short literals,
and `<aql-embed>` gives a multi-line block form for documents and
longer payloads. The same handlers can back both surfaces —
`cs/foo > .bar` and `<aql-embed lang="css">foo > .bar</aql-embed>`
produce the same value type.

### 6.1 Why this matters

- Documents (technical specs, runbooks, notebooks) commonly mix
  prose, code, queries, schemas, and config. A single XML value
  can carry all of them through `<aql-embed>` without spawning N
  satellite files.
- The `lang` attribute is the natural extension point: new DSLs
  are added by registering a handler, not by modifying the parser.
- Tooling — editors, linters, formatters — can dispatch on `lang`
  to give each block its native treatment (syntax highlighting,
  autocompletion, formatter) while still treating the surrounding
  document as a single AQL value.

### 6.2 Interaction with interpolation

Embedded blocks may opt in to TSX-style `{...}` interpolation:

```
<aql-embed lang="sql" interpolate>
  SELECT * FROM users WHERE id = {user_id}
</aql-embed>
```

The boolean attribute `interpolate` (JSX shorthand for
`interpolate={true}`) enables `{...}` substitution inside the
body. The lexer pivots into expression mode at `{` and the
embedded handler receives the post-substitution text — or, for
handlers that want it, the original template plus a list of
substitutions, so a SQL handler can produce a parameterised query
with bind variables rather than string concatenation.

Without the attribute, `{...}` is literal content of the embedded
language — the same convention TSX uses for `<pre>` and other
verbatim blocks. This matters for languages where `{` is
meaningful (C-family code, JSON, regex), which then embed without
escape churn:

```
<aql-embed lang="json">
  { "id": 1, "name": "alice" }
</aql-embed>
```

### 6.3 Whitespace and indentation

Many embedded languages (Python, YAML, Markdown code fences) are
whitespace-sensitive. `<aql-embed>` honours an
`xml:space="preserve"` hint and supports a `dedent` boolean
shorthand that strips the common leading indent — equivalent to
Python's `textwrap.dedent`. The default policy preserves content
verbatim.


## 7. Plugin architecture: `lang` → jsonic plugin

Each `<aql-embed lang="X">` block is parsed by a registered plugin
keyed by `X`. The architecture matches existing AQL parser patterns
(see `internal/parser/parse.go` — `j := jsonic.Make(opts)` followed
by token registration, lex matchers, grammar rules, and a converter
pass) so that adding a new embedded language is the same kind of
work as adding a new piece of AQL syntax.

### 7.1 Plugin shape

A plugin owns a `jsonic.Jsonic` instance configured for its
language and a converter that turns the jsonic output into an AQL
`Value`:

```go
package embed

import (
    jsonic "github.com/jsonicjs/jsonic/go"
    "github.com/metsitaba/voxgig-exp/aql/internal/engine"
)

// Plugin is the unit of registration. One plugin handles exactly
// one lang attribute value.
type Plugin struct {
    Lang      string

    // Setup runs once at plugin construction. It receives a fresh
    // jsonic.Jsonic and registers tokens, lex matchers, grammar
    // rules, and options for the language.
    Setup     func(*jsonic.Jsonic) error

    // MakeOpts returns the jsonic.Options used to construct the
    // jsonic instance. Defaults are fine for JSON-like syntaxes;
    // word-context languages override TextInfo, ListRef, etc.
    MakeOpts  func() jsonic.Options

    // Convert turns the jsonic parse result into an AQL Value.
    // For raw-body plugins (markdown, regex), Convert receives the
    // raw string body and runs an external parser instead.
    Convert   func(parsed any, body string, opts ParseOpts) (engine.Value, error)

    // Result is the declared engine.Type that Convert produces.
    // Used by the carrier-based static type checker before any
    // runtime parse occurs.
    Result    engine.Type

    // Mode declares whether the plugin wants a resolved string
    // (AQL substitutes {…} before calling Convert) or the
    // original template plus a Substitutions list (handler
    // chooses how to splice). See §7.6.
    Mode      InterpMode

    // built lazily on first use; reused across parses
    j         *jsonic.Jsonic
}

type ParseOpts struct {
    Interpolate   bool                   // interpolate attribute set
    Dedent        bool                   // dedent attribute set
    Preserve      bool                   // xml:space="preserve"
    Attrs         map[string]string      // every other attr on <aql-embed>
    Substitutions []Substitution         // populated when Mode == InterpTemplate
}

type Substitution struct {
    Offset int            // byte offset into body where {…} appeared
    Length int            // length of the {…} span in the original body
    Value  engine.Value   // the resolved AQL value
}

type InterpMode int

const (
    InterpResolved InterpMode = iota  // body has been substituted
    InterpTemplate                    // body still has {…} markers
)
```

### 7.2 Registry

A package-level registry maps lang names to plugins. Lookups are
case-sensitive; `markdown` and `Markdown` are different. Plugins
self-register via `init()`:

```go
package embed

var registry = map[string]*Plugin{}

func Register(p *Plugin) error {
    if _, dup := registry[p.Lang]; dup {
        return fmt.Errorf("embed: lang %q already registered", p.Lang)
    }
    registry[p.Lang] = p
    return nil
}

func Lookup(lang string) (*Plugin, bool) {
    p, ok := registry[lang]
    return p, ok
}

// Parse is what the XML literal parser calls when it encounters an
// <aql-embed> end tag. The caller has already extracted body and
// resolved attributes; opts.Substitutions is populated when the
// outer parser ran AQL on the {…} spans.
func Parse(lang, body string, opts ParseOpts) (engine.Value, error) {
    p, ok := Lookup(lang)
    if !ok {
        return rawValue(lang, body), nil // §7.5 fallback
    }
    if p.j == nil {
        p.j = jsonic.Make(p.MakeOpts())
        if err := p.Setup(p.j); err != nil {
            return engine.Value{}, fmt.Errorf("embed[%s]: setup: %w", lang, err)
        }
    }
    if opts.Dedent { body = dedent(body) }

    // Plugins that don't use jsonic at all (markdown, regex) leave
    // Setup as a no-op and read body directly in Convert. The
    // jsonic parse below produces a single string token in that
    // case via a catch-all matcher registered in Setup.
    parsed, perr := p.j.Parse(body)
    if perr != nil {
        return engine.Value{}, fmt.Errorf("embed[%s]: parse: %w", lang, perr)
    }
    return p.Convert(parsed, body, opts)
}
```

### 7.3 Anatomy of a plugin: JSON

A jsonic-shaped language reuses jsonic's strict-JSON dialect almost
unchanged:

```go
func init() {
    embed.Register(&embed.Plugin{
        Lang: "json",
        MakeOpts: func() jsonic.Options {
            return jsonic.Options{
                ListRef: boolPtr(true),
                MapRef:  boolPtr(true),
                Strict:  &jsonic.StrictOptions{Json: boolPtr(true)},
            }
        },
        Setup:   func(j *jsonic.Jsonic) error { return nil },
        Convert: convertJsonValue,
        Result:  engine.TJson,
        Mode:    embed.InterpResolved,
    })
}
```

`convertJsonValue` walks jsonic's typed list/map output and produces
an `engine.Value` of type `Object/Map`/`Object/List`. Because
jsonic's JSON support is built in, this plugin is ~30 lines.

### 7.4 Anatomy of a plugin: regex

A non-jsonic language uses a single catch-all matcher to consume
the whole body as one token, then runs an external parser in
`Convert`:

```go
func init() {
    embed.Register(&embed.Plugin{
        Lang: "regex",
        MakeOpts: func() jsonic.Options { return jsonic.Options{} },
        Setup: func(j *jsonic.Jsonic) error {
            // One matcher: emit the whole input as a #BODY token.
            // The "raw" rule replaces "val" so jsonic doesn't try
            // to tokenise the body further.
            return embed.InstallRawBodyRule(j)
        },
        Convert: func(parsed any, body string, _ embed.ParseOpts) (engine.Value, error) {
            re, err := regexp.Compile(body)
            if err != nil { return engine.Value{}, err }
            return engine.NewMiniLang(engine.TRegExp, re, body), nil
        },
        Result: engine.TRegExp,
        Mode:   embed.InterpResolved,
    })
}
```

`InstallRawBodyRule` is a helper that sets up the single-token
shape so non-jsonic plugins don't repeat the same boilerplate.
Plugins that genuinely use jsonic (json, jsonic itself, AQL
self-embedding) skip the helper.

### 7.5 Built-in plugins and the unknown-lang fallback

A core set is registered at `embed` package init:

| Lang        | Result type                       | Backing                                       |
|-------------|-----------------------------------|-----------------------------------------------|
| `aql`       | `Scalar/MiniLang/Aql`             | reuses `internal/parser/parse.go`             |
| `json`      | `Object/Map` / `Object/List`      | jsonic strict-JSON dialect                    |
| `jsonic`    | `Object/Map` / `Object/List`      | jsonic default dialect                        |
| `xml`       | `Object/Xml`                      | the XML literal parser (re-entry)             |
| `markdown`  | `Scalar/MiniLang/Markdown`        | external — `goldmark` (CommonMark)            |
| `regex`     | `Scalar/MiniLang/RegExp`          | external — `regexp.Compile`                   |
| `css`       | `Scalar/MiniLang/Css`             | external — `andybalholm/cascadia`             |
| `sql`       | `Scalar/MiniLang/Sql`             | external — `pg_query_go` or `sqlparser`       |
| `yaml`      | `Scalar/MiniLang/Yaml`            | external — `gopkg.in/yaml.v3`                 |
| `toml`      | `Scalar/MiniLang/Toml`            | external — `BurntSushi/toml`                  |
| `text`      | `Scalar/String`                   | identity — body is the value, after dedent    |

Unknown `lang` values fall through to a generic raw plugin that
captures `(lang, body, attrs)` into a `Scalar/MiniLang/Raw` value.
Downstream code can route the raw value (e.g. by passing it to
`embed-handle "graphviz" raw` once a graphviz plugin is loaded),
so a document with unfamiliar embeds still loads, parses, and
queries.

### 7.6 Interpolation handoff

When `interpolate` is set on the `<aql-embed>` element, the outer
XML parser evaluates each `{…}` span as an AQL expression before
the plugin runs. The plugin's `Mode` decides what arrives:

- **`InterpResolved`** (default for markdown, regex, css). The
  body is substituted in place; `Convert` receives a single
  resolved string. Simple — no template state to manage.
- **`InterpTemplate`** (used by sql, log-template). The body
  retains `{…}` placeholder markers, and `opts.Substitutions`
  carries the resolved values plus their byte offsets. The
  plugin chooses how to splice — SQL emits `?` bind variables and
  binds the values via `database/sql`; logging emits structured
  fields; templated YAML emits anchors.

The mode is declared at registration time, not per-call, because
the security and correctness story differs sharply: a SQL plugin
that accepts `InterpResolved` is a SQL-injection vulnerability,
so the plugin author must opt in to template mode.

### 7.7 Lazy mode

`<aql-embed lang="X" lazy>` defers parsing. The XML literal parser
records `(lang, body, opts)` in an `Object/Lazy<X>` value and
returns immediately. The first operation that requires concrete
content (`text`, `cs/`, `xml-attr`, `xml-print`, etc.) calls
`embed.Parse` and memoises the result on the value. Errors from
lazy plugins surface at first use, with the original
`<aql-embed>` source position attached.

Lazy mode is what makes a 50-block runbook cheap: only the blocks
actually inspected pay the parsing cost.

### 7.8 Registering plugins from AQL

The Go-side `embed.Register` is matched by an AQL host word for
runtime-loaded plugins:

```
embed-register "graphviz"
  [ String -> Object/MiniLang/Graphviz ]
  [ body -> body graphviz-parse ]
```

The body is captured as an AQL fn (with its declared signature)
and wrapped in a `Plugin` whose `Convert` invokes the fn through
`CallAQL`. The form is the dynamic counterpart to compile-time Go
registration; both populate the same registry.

### 7.9 Concurrency and reuse

A `*Plugin`'s `j *jsonic.Jsonic` is built lazily once and reused
for every parse of that lang. jsonic isolates per-parse state in
its `Context` so a single instance can serve concurrent calls
provided the configuration phase has completed. The first call
into an unbuilt plugin takes a sync-once latch; subsequent calls
take no lock.

External parsers (goldmark, cascadia, regexp) are similarly
long-lived — `regexp.Compile`'d patterns and goldmark instances
are created once per plugin and reused.

### 7.10 Error handling

Plugin errors are wrapped in the AQL error code
`[aql/embed/parse]` with three pieces of context:

- `lang` — the value of the `lang` attribute
- `pos` — the source position of the offending `<aql-embed>`
- `cause` — the raw plugin error (jsonic parse failure,
  goldmark error, `regexp.Compile` error, etc.)

The hint string includes a 40-character snippet of the body
around the error position so the user does not have to scroll
back to the embed declaration. Lazy plugins attach the original
`<aql-embed>` source position to the error even though the parse
runs much later.


## 8. Status and open questions

This is a design sketch (completeness 0). No parser, runtime, or
query support has been built. Main open questions:

- Whether to mirror jsonic's grammar mechanics (extending the
  `val` rule with `xml` and `attr` peers) or build a separate XML
  pass that emits the same engine `Value` stream.
- DOM vs streaming representation for large XML values.
- Whether `<aql-embed>` blocks should be lazily parsed (handler
  invoked on first use) or eagerly parsed at load time.
- Namespace handling — full XML namespaces, or a simplified
  prefix-string-only model?
- Which subset of XPath, if any, to support alongside CSS for
  callers who already have legacy XPath strings.
- Schema validation (RNG, XSD, or a bespoke AQL-flavoured schema)
  for tooling that wants structural guarantees.

Subsequent revisions will pin these down and bump the completeness
suffix in the filename.
