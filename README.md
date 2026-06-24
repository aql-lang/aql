# AQL

**AQL** is a typed, word-based query language. A program is a sequence
of *words*, and a word takes its arguments where you write them —
`add 1 2` reads left to right, like a conventional call — while binary
operations also read naturally in infix position (`10 sub 3`). Every
value carries a hierarchical type, and every word declares typed
signatures that drive dispatch. Underneath, AQL is concatenative: words
can equally take their arguments from a value stack, which is what
makes point-free pipelines compose — but you can write a lot of AQL
before thinking about the stack, so start with the forward forms below
and meet the stack later. The reference implementation is in Go and
ships as a single `aql` binary that includes a REPL, a type checker, a
formatter, an LSP server, a registry client, a secrets vault, and a
multi-service supervisor.

> **Notation.** In code, a trailing `# returns …` comment shows what an
> expression evaluates to (`square 4  # returns 16`); in prose we say
> "`square 4` returns `16`". The comment is ordinary documentation — `#`
> just begins a line comment — not special syntax. (We deliberately
> avoid an `=>` arrow for results: `=>` is real syntax in AQL, the
> anonymous-function arrow, sugar for the word `afn`.)

```aql
# words take their arguments where you write them
add 1 2                              # returns 3
10 sub 3                             # returns 7

# typed functions, lists, maps, records, concurrency
def square fn [[x:Number] [Number] [mul x x]]
square 4                             # returns 16

[1, 2, 3] each [square]              # returns [1 4 9]
{name: "Ada"} . name                 # returns 'Ada'

def Point refine Record [x:Number y:Number]
make Point [3 4]                     # returns {x:3 y:4}

import "aql:time-util" TimeUtil.await [[add 1 2] [add 3 4]]    # returns [3 7]

# macros: add new syntax in AQL itself (hygienic; this one expands to an `if`)
def unless (macro [[c body] [quote [if unquote c [] unquote body]]])
unless false [42]                    # returns 42
```

The stack is still there when you want it: `10 3 sub` is the all-stack
spelling of `10 sub 3`, and pipeline-style code leaves intermediate
values on the stack between words (stack-shuffling words like `dup` and
`swap` work only that way). The **[Tutorial](TUTORIAL.md)** introduces
the stack model when you need it, and the
**[Explanation](EXPLANATION.md)** covers how forward collection changes
the feel of stack code.


## Forward arguments — the primary way to write AQL

AQL is concatenative, but you rarely have to think in stack terms,
because the defining feature of the surface syntax is **forward
arguments**: a word takes its operands from the tokens written *after*
it, in declared order, exactly like a conventional function call.

```aql
add 1 2                              # returns 3 — the word comes first
def square fn [[x:Number] [Number] [mul x x]]
square 4                             # returns 16 — your own words read the same way
import "aql:math-util"               # imports read forward, too
3 7 MathUtil.min                     # returns 3 — module words are then in scope
```

This is the canonical, recommended form for **all new code, examples,
and documentation** — including imports: write `import "aql:math-util"`,
not the stack spelling. Forward form reads top-to-bottom and
left-to-right, the written argument order matches the declared parameter
order, and you never have to track what is sitting on the stack:

- `f a b c` binds `a`, `b`, `c` to the first, second, third parameters.
- Calls compose by parenthesising: `(f a b) g c`.
- The same rule covers built-ins, your own `def fn`s, and `import`.
- One exception: a dotted module word (`MathUtil.min`) auto-invokes from
  what's already on the stack, so call it args-first (`3 7 MathUtil.min`).

The stack forms (`c b a f`, `10 3 sub`) remain fully equivalent and are
there for point-free pipelines, but forward form is what you reach for
first. See the **[Tutorial](TUTORIAL.md)** to start writing it and the
**[Explanation](EXPLANATION.md)** for how forward collection works
underneath.


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
aql do 'add 1 2'                     # one-shot expression
aql script.aql                       # run a file
aql check script.aql                 # type-check, don't run
aql fmt script.aql                   # format in place (always rewrites)
aql build script.aql -o tool         # compile to a standalone executable
aql help                             # introduction + the subcommand list
aql describe                         # a categorised guide to every built-in word
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
| **[Agent Guide](AGENTS.md)** | You are an AI agent (or new contributor) and want a map of the docs, the tooling, and how to discover the language with `aql describe`/`aql help`. |

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
- **`refine Object {…}` is removed — classes are defined with `class`:**
  `def Foo class {x:1}` (paren-free), subclass via `def Bar refine Foo
  {…}`. Class instances are flat (no prototype chain), **sealed**
  (undeclared field writes raise `sealed_field`), strictly typed at
  `make` and `set` (no silent conversion; predicate field types run),
  root under `Ideal/Class` (so `p is Object` is now false), and
  serialize as pure JSON with a `$class` key (`StructUtil.jsonify` /
  `StructUtil.reify`).
- **`set` mutates Store/Object/Array in place and returns nothing** —
  `def r (b set k v)` binds nothing; read the container back instead.
  On an immutable **Map**, `set` is copy-returning: `{a:1} set b 2`
  yields a new map and the receiver is unchanged.


## Contributing

Bug reports, proposals, and pull requests are welcome on
[GitHub](https://github.com/aql-lang/aql). For non-trivial
language changes, open an issue first — the design notes under
`design/` are the historical record of how previous
proposals played out.


## License

AQL is released under the terms of the [MIT License](LICENSE.md).
