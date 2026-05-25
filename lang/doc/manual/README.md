# AQL Documentation

AQL is a concatenative query language: programs are sequences of
words that transform a stack. Every value carries a hierarchical
type, every word declares typed signatures, and the same word can
be called in prefix, infix, or suffix position. The language is
implemented in Go and ships as a single `aql` binary that includes
a REPL, type checker, formatter, LSP, registry client, and a
service supervisor.

These docs are organised after the [Diátaxis](https://diataxis.fr)
framework — four documents, one for each kind of learning need:

| Document | When to read it |
|----------|-----------------|
| **[Tutorial](tutorial.md)** | You are new to AQL and want to learn it step by step. |
| **[How-To Guides](how-to.md)** | You have a specific task and want a recipe. |
| **[Reference](reference.md)** | You need the precise behaviour of a syntax form, type, or word. |
| **[Explanation](explanation.md)** | You want to understand *why* AQL is the way it is. |
| **[CLI Reference](cli.md)** | You want to drive the `aql` binary from the shell. |


## 30-second tour

```aql
# Stack-based arithmetic — three equivalent forms
1 2 add                     # prefix: both args on the stack
add 1 2                     # forward: both args collected after
1 add 2                     # infix: one stack, one forward

# Named definitions and typed functions
def double [dup add]
5 double                    => 10

def square fn [[x:Number] [Number] [x mul x]]
4 square                    => 16

# Lists, maps, and dot-access
[1, 2, 3] each [dup mul]    => [1,4,9]
{name: "Ada"} . name        => 'Ada'

# Records, tables, and instantiation
type Point record [x:Number y:Number]
make Point [3 4]            => {x:3,y:4}

# Concurrency
await [[sleep 50 1] [sleep 50 2]]   => [1,2]

# Files
read "data.csv"
write "out.json" {x: 1}
```


## Installation

```bash
go install github.com/aql-lang/aql/cmd/go/aql@latest
aql -version
aql                                  # drop into the REPL
aql do '1 add 2'                     # one-shot expression
aql script.aql                       # run a file
```

A wasm-powered browser playground is bundled in `docs/index.html`;
see `wpg/Makefile` for the build target.


## What to read next

- Brand new to concatenative languages? Start with the
  **[Tutorial](tutorial.md)**; come back to **[Explanation](explanation.md)**
  once you've written a few programs.
- Coming from Forth / Factor / APL? Skim the
  **[Explanation](explanation.md)** first — it explains how forward
  collection and type-directed dispatch change the feel of stack code —
  then dive into the **[Reference](reference.md)**.
- Building a script or a tool? The **[How-To Guides](how-to.md)** and
  **[CLI Reference](cli.md)** are the fastest path.
