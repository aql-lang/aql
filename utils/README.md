# utils/ — a coreutils subset, written in AQL

These are real programs, not examples. Each one is an AQL source file that
`aql build` turns into a standalone executable, parses its own arguments with
[`aql:cli`](../lang/go/modules/cli.aql), reads stdin or files, writes stdout,
and chooses an exit code. They exist to answer a question the language could
not answer before: *can you actually write a command-line tool in AQL?*

Every program here is therefore also a test of the runtime's CLI contract —
argv, environment, exit codes, stream detection, baked permissions — and each
was chosen because it exercises something specific:

| Program | What it proves |
|---|---|
| `cat`, `tee` | stream reading and writing; binary safety |
| `wc` | slurp versus incremental reads |
| `seq` | numeric formatting |
| `head` | early exit **without reading the rest** — the sharp test of incremental input (`tail` proves nothing: it is correctly implementable by slurping) |
| `grep` | regex, and the 0/1/2 exit-code convention |
| `cut`, `tr` | string words over unicode |
| `sort`, `uniq` | collections, and the stdin/file duality |
| `ls` | the filesystem walk |
| `printenv` | the enumerating half of the environment capability — nothing else witnesses it |
| `true`, `false`, `test` | `IO.exit`'s tri-state, in three lines each |

## Running them

```bash
make -C ../cmd/go build     # the aql binary
make check                  # type-check every program and test
make test                   # run the suites
make binaries               # build every program into bin/
printf 'b\na\n' | ./bin/sort
```

`aql build` performs no type check of its own, so `make check` is not
optional — it is the only gate between a typo and a shipped binary.

## Why a Makefile here and a Go test over there

This directory is not in the repository's Go module list, so nothing in it is
reached by `make test` at the root. A top-level AQL directory outside the Go
fan-out rots — `kg/` demonstrated that — so `cmd/go` carries an end-to-end Go
test that builds and runs two of these programs, pipes into them, and checks
their exit codes. That test is what keeps this directory honest in CI; the
Makefile is what makes it pleasant to work in.

## House rules for a program here

Learned by writing them, and each one prevents a silent failure:

- **Terminate every statement with `end`** when its head is a module export.
  Two consecutive unterminated statements can invert and drop one of the calls
  with no diagnostic (NUR038).
- **Swallow every residual.** The driver prints whatever the program leaves on
  the stack, so `IO.write` and `flex set` results must be dropped
  (`(IO.write …) drop`), and a program that has finished should end with
  `IO.exit 0;`.
- **Accumulate into `flex`**, not into an immutable Map or List: the immutable
  form is quadratic, and these programs read files.
- **Declare callbacks at top level**, never inside another fn: a fn-local
  callback resolves under the interpreter and is undefined under the compiler
  (NUR037).
- **Take argv as a parameter** in anything you want to test — `aql test`
  cannot inject an argument vector.
