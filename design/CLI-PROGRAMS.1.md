# CLI programs, part 2: baked policy and the `Cli.dispatch` split

Supplements `design/CLI-PROGRAMS.0.md`. That note is the RFC for the whole
C1–C6 arc; this one records two decisions the RFC does not contain, each
made because implementing the RFC as written would not have worked.

## 1. `aql build -perms` bakes a `*policy.Profile`, and it is a default

**Landed** with C1 (`aql build` argv + overlay + baked policy).

The RFC says the built binary should carry its build-time permissions. The
obvious spelling — put the `policy.Policy` on `buildrt.Config` — cannot
work: `Policy` is an interface (`lang/go/policy/policy.go`) and `Config` is
JSON-marshalled into the executable's trailer. What ships instead is the
**flattened `*policy.Profile`** (fully json-tagged), produced by the same
helper the CLI flags already use (`permsflags.ProfileFromPolicy`), and
compiled back with `policy.CompileProfile` in `buildrt.Main`.

**It is a default, not a boundary.** The trailer is plain JSON behind a
magic+length marker with no integrity check, so anyone holding the binary
can strip or rewrite the profile. What a baked profile does is constrain
the PROGRAM — which is what a tool author shipping an AQL script wants:
"this tool never writes files" is a property of the tool, enforced against
the tool's own code. It is not a sandbox against an adversary who has the
executable. `CLI.md` says so, and must keep saying so.

The acceptance test is a PAIR: the same program built twice, once with
permissions that deny the write and once with permissions that allow it.
One build failing proves nothing on its own — it could be failing for any
reason — and one build succeeding proves nothing either. Only the pair
shows the profile is what decided.

## 2. `Cli.main` splits into `Cli.dispatch` + a three-line shell

**Decided here, before the code.**

The RFC's §8 surface is:

```aql
Cli.main (spec) handler/r
```

which parses `IO.args`, prints usage or version and exits 0 for
`--help`/`--version`, prints the error plus a usage hint to stderr and
exits 2 on a usage error, and otherwise calls the handler.

**That cannot reach the coverage bar an AQL-authored module inherits.** A
module written in AQL is gated by the `sift_coverage_test.go` pattern:
every executable row of `cli.aql` must be covered by `cli_test.aql`, with a
small allowlist whose entries are each *asserted to be genuinely
uncovered*. But `Cli.main` ends in `IO.exit`, and `aql test`'s runner
treats a raised error — which is what `IO.exit` is — as a file-level
failure that ends the file (`cmd/go/internal/test`). So a test that
exercised `Cli.main`'s `--help` arm would kill its own suite at the first
arm and never reach the second. The arms are unreachable *from a test*, and
an allowlist entry per arm would be a lie: they are reachable in principle,
just not from a runner that dies when they fire.

So the module exposes two things:

```
Cli.dispatch (spec) (argv)  →  {action, code, out, err, flags, args, command}
Cli.main (spec) handler/r    →  the three-line shell
```

- **`Cli.dispatch` is pure.** Spec map plus argument vector in, a decision
  map out. `action` is one of `run` / `help` / `version` / `error`; `out`
  and `err` are the exact text to print; `code` is the exit status the
  driver should use. It never prints, never exits, never reads the
  environment. Every arm is therefore reachable from a spec row and from
  `cli_test.aql`, and the interesting logic — flag grammar, clustering,
  `--flag=value`, `--no-X`, `--`, arity, unknown-flag errors, usage
  rendering — lives here.
- **`Cli.main` is the shell**: call `Cli.dispatch`, print `out` to stdout or
  `err` to stderr, then either `IO.exit code` or call the handler. It is
  the only part that touches the world, and it is small enough that
  reviewing it by eye is the honest verification. Its rows are the
  allowlist entries, and the allowlist comment says why: an exiting arm
  cannot be exercised by a runner that treats an exit as a file failure.

This is the same shape the RFC itself recommends elsewhere — a pure core
with an imperative rind — and it is what makes "written in AQL, gated like
Go" possible rather than aspirational.

### Consequence for callers

A program that wants the full behaviour writes `Cli.main`. A program that
wants to decide for itself — a test, a REPL, a subcommand dispatcher, a
program that logs before exiting — writes `Cli.dispatch` and acts on the
map. Nothing is hidden behind the shell.

## 3. What did not change

The RFC's parsing conventions, spec-map shape, `r.ok` / `r.err` result
shape, and the "subcommands are in the spec shape from day one, implemented
in stage two" staging all stand. The two cautions in §8 about the aless DX
round (no local alias fns and no recursion in the parse machinery; native
`is`-chains rather than multi-sig dispatch over argv values) are treated as
hypotheses to re-verify against the current engine rather than as standing
constraints — several sharp edges named there have since been fixed, and
`design/AQL-SHARP-EDGES.0.md` is the live list.
