# AQL

**AQL** is a concatenative query language: programs are sequences
of *words* that transform a *stack*. Every value carries a
hierarchical type, every word declares typed signatures, and the
same word can be called in prefix, infix, or suffix position. The
reference implementation is in Go and ships as a single `aql`
binary that includes a REPL, a type checker, a formatter, an LSP
server, a registry client, a secrets vault, and a multi-service
supervisor.

```aql
# stack-based arithmetic — three equivalent forms
1 2 add                              # prefix
add 1 2                              # forward
1 add 2                              # infix

# typed functions, lists, maps, records, concurrency
def square fn [[x:Number] [Number] [x mul x]]
4 square                             => 16

[1, 2, 3] each [dup mul]             => [1,4,9]
{name: "Ada"} . name                 => 'Ada'

type Point record [x:Number y:Number]
make Point [3 4]                     => {x:3,y:4}

await [[sleep 50 1] [sleep 50 2]]    => [1,2]
```


## Install

```bash
go install github.com/aql-lang/aql/cmd/go/aql@latest
aql -version
```

Then:

```bash
aql                                  # start the REPL
aql do '1 add 2'                     # one-shot expression
aql script.aql                       # run a file
aql check script.aql                 # type-check, don't run
aql fmt -w script.aql                # format in place
aql help                             # list every built-in word
```

A wasm-powered browser playground is bundled in
[`docs/index.html`](docs/index.html); build it with
`make -C wpg wasm`.


## Documentation

The manual is organised into four documents, one for each kind of
learning need, plus a CLI reference:

| Document | When to read it |
|----------|-----------------|
| **[Tutorial](TUTORIAL.md)** | You are new to AQL and want to learn it step by step. |
| **[How-To Guides](HOWTO.md)** | You have a specific task and want a recipe. |
| **[Reference](REFERENCE.md)** | You need the precise behaviour of a syntax form, type, or word. |
| **[Explanation](EXPLANATION.md)** | You want to understand *why* AQL is the way it is. |
| **[CLI Reference](CLI.md)** | You want to drive the `aql` binary from the shell. |

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
| `lang/doc/design/` | Internal design notes and proposals. |


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


## Contributing

Bug reports, proposals, and pull requests are welcome on
[GitHub](https://github.com/aql-lang/aql). For non-trivial
language changes, open an issue first — the design notes under
`lang/doc/design/` are the historical record of how previous
proposals played out.


## License

AQL is released under the terms of the [MIT License](LICENSE.md).
