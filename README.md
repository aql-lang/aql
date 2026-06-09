# AQL

**AQL** is a concatenative query language: programs are sequences
of *words* that transform a *stack*. Every value carries a
hierarchical type, every word declares typed signatures, and most
words can be called in prefix, infix, or suffix position (the
exceptions are stack-shuffling words like `dup` and `swap`, which
only take their arguments from the stack). The reference
implementation is in Go and ships as a single `aql` binary that
includes a REPL, a type checker, a formatter, an LSP server, a
registry client, a secrets vault, and a multi-service supervisor.

> **Notation.** In code, a trailing `# returns …` comment shows what an
> expression evaluates to (`4 square  # returns 16`); in prose we say
> "`4 square` returns `16`". The comment is ordinary documentation — `#`
> just begins a line comment — not special syntax. (We deliberately
> avoid an `=>` arrow for results: `=>` is real syntax in AQL, the
> anonymous-function arrow, sugar for the word `afn`.)

```aql
# stack-based arithmetic — three equivalent forms (all compute a-b)
10 3 sub                             # all-stack
10 sub 3                             # mixed
sub 3 10                             # all-forward

# typed functions, lists, maps, records, concurrency
def square fn [[x:Number] [Number] [x mul x]]
4 square                             # returns 16

[1, 2, 3] each [dup mul]             # returns [1 4 9]
{name: "Ada"} . name                 # returns 'Ada'

def Point refine Record [x:Number y:Number]
make Point [3 4]                     # returns {x:3 y:4}

"aql:time-util" import end TimeUtil.await [[1 add 2] [3 add 4]]    # returns [3 7]

# macros: add new syntax in AQL itself (hygienic; this one expands to an `if`)
def unless (macro [[c body] [quote [if unquote c [] unquote body]]])
unless false [42]                    # returns 42
```


## Install

Until v0.1.0 is tagged, build from a clone (the `cmd/go` module
carries local `replace` directives, so `go install …@latest` is
not yet supported):

```bash
git clone https://github.com/aql-lang/aql
cd aql/cmd/go
go install ./aql
aql -version
```

Then:

```bash
aql                                  # start the REPL
aql do '1 add 2'                     # one-shot expression
aql script.aql                       # run a file
aql check script.aql                 # type-check, don't run
aql fmt script.aql                   # format in place (always rewrites)
aql help                             # list every built-in word
```

A wasm-powered browser playground is bundled in
[`docs/index.html`](docs/index.html); build it with
`make -C wpg wasm`.


## Documentation

The manual is organised into four documents, one for each kind of
learning need, plus a CLI reference and the architecture record:

| Document | When to read it |
|----------|-----------------|
| **[Tutorial](TUTORIAL.md)** | You are new to AQL and want to learn it step by step. |
| **[How-To Guides](HOWTO.md)** | You have a specific task and want a recipe. |
| **[Reference](REFERENCE.md)** | You need the precise behaviour of a syntax form, type, or word. |
| **[Explanation](EXPLANATION.md)** | You want to understand *why* AQL is the way it is. |
| **[CLI Reference](CLI.md)** | You want to drive the `aql` binary from the shell. |
| **[Architecture Design Record](ADR.md)** | You want the key architectural decisions and the reasoning behind them. |

Suggested reading orders:

- **Brand new to AQL?** Start with the **[Tutorial](TUTORIAL.md)**.
  After you've written a few programs, read the
  **[Explanation](EXPLANATION.md)** to understand the model.
- **Coming from Forth / Factor / APL?** Skim the
  **[Explanation](EXPLANATION.md)** first — it explains how forward
  collection and type-directed dispatch change the feel of stack
  code — then dive into the **[Reference](REFERENCE.md)**.
- **Building a script or a tool?** Use the
  **[How-To Guides](HOWTO.md)** and **[CLI Reference](CLI.md)** as
  your starting points; consult the **[Reference](REFERENCE.md)**
  for word-by-word detail.


## Repository layout

| Path | What it is |
|------|------------|
| `cmd/go/` | The `aql` CLI / REPL (`github.com/aql-lang/aql/cmd/go`). |
| `lang/go/` | The language layer: public `lang` API and the consolidated `native` word library. |
| `eng/go/` | Engine kernel, parser, and kernel spec runner. |
| `calc/go/` | A small calculator built directly on `eng` (learning example). |
| `wpg/` | The wasm web playground (`wpg/wasm` + `wpg/serve`). |
| `test/` | Shared TSV spec-runner scaffolding and HTTP test fixtures. |
| `docs/` | The bundled wasm playground (`index.html`). |
| `lang/spec/` | Engine spec TSV files (the language's executable spec). |
| `design/` | Internal design notes and proposals. |


## Building from source

```bash
make test                            # all modules
make vet                             # all modules
make fmt                             # all modules
make lint                            # all modules (golangci-lint)

cd cmd/go && make build              # builds bin/aql
cd wpg     && make wasm              # builds docs/index.html
cd wpg     && make serve             # runs the playground on :8080
```

Before committing, run from the repo root:

```bash
make fmt && make vet && make lint && make test
```


## Upgrade notes (pre-1.0 breaking changes)

AQL is pre-1.0 and the surface still moves. Libraries written against
mid-2026 snapshots most commonly need these renames:

- **Namespaces are capital-initial.** `test.test` → `Test.test`,
  `assert.equal` → `Assert.equal`, `math.sqrt` → `MathUtil.sqrt`.
  Lowercase export names are rejected.
- **Utility modules took a `-util` suffix** (avoiding builtin type-name
  clashes): `aql:array` → `aql:array-util` (binds `ArrayUtil`), and
  likewise `aql:math-util`, `aql:time-util`, `aql:type-util`,
  `aql:matrix-util`, `aql:bin-util`, `aql:struct-util`,
  `aql:logic-util`, `aql:string-util`.
- **Words moved out of core into modules:** string words →
  `aql:string-util` (with `indexof` now **haystack-last**, and the list
  form split out as `ArrayUtil.indices`); voxgig-struct words
  (`merge`, `setpath`, `jsonify`, …) → `aql:struct-util`; bitwise →
  `aql:bin-util`; I/O except `print` → `aql:io`; HTTP → `aql:net`;
  clock/async → `aql:time-util`; derived boolean connectives →
  `aql:logic-util`. Moved words are no longer available unqualified.
- **Module fn exports use the referent form:**
  `export "Mod" {double: double/r}` (a bare fn name in an export map
  would be invoked, not referenced).
- **`set` mutates in place and returns nothing** — `def r (b set k v)`
  binds nothing; read the store back instead.


## Contributing

Bug reports, proposals, and pull requests are welcome on
[GitHub](https://github.com/aql-lang/aql). For non-trivial
language changes, open an issue first — the design notes under
`design/` are the historical record of how previous
proposals played out.


## License

AQL is released under the terms of the [MIT License](LICENSE.md).
