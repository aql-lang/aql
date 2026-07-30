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

**The decision logic must be reachable without running the program.** A
module written in AQL is gated by the `sift_coverage_test.go` pattern —
every executable row of `cli.aql` covered by `cli_test.aql`, with a small
allowlist whose entries are each *asserted to be genuinely uncovered* — and
`Cli.main` ends in `IO.exit`, which ends whatever is running it. More
importantly, a spec ROW cannot survive an exit at all, so a surface that
only exists behind `Cli.main` could not be pinned by `lang/spec` either.
The flag grammar is the part worth pinning, so it must be callable without
exiting.

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
  `err` to stderr, then either `IO.exit code` or call the handler. It is the
  only part that touches the world, and it is small enough that reviewing it
  by eye is honest verification.

  **Correction (2026-07-30):** the first cut of this note assumed the shell's
  rows would have to be allowlisted, because an exiting arm cannot be
  exercised by a runner that treats an exit as a file failure. That is wrong.
  `IO.exit` RAISES the reserved `aql/exit`, and `Assert.throws` observes a
  raise — so the arms are reachable from the module's own suite after all,
  and `cli.aql` ships at **100% AQL-line coverage with an empty allowlist**.
  (A plain `do […] error […]` genuinely does not catch an exit — deliberately,
  so a program cannot swallow its own termination — which is what made the
  wrong conclusion plausible.) The split still stands on its own merits: the
  decision logic is pure, spec-pinnable, and reusable by any driver that wants
  to act on it itself. It just is not forced by the coverage gate.

This is the same shape the RFC itself recommends elsewhere — a pure core
with an imperative rind — and it is what makes "written in AQL, gated like
Go" possible rather than aspirational.

### Consequence for callers

A program that wants the full behaviour writes `Cli.main`. A program that
wants to decide for itself — a test, a REPL, a subcommand dispatcher, a
program that logs before exiting — writes `Cli.dispatch` and acts on the
map. Nothing is hidden behind the shell.

## 3. `Cli.usage` takes its width and colour, it does not read them

**Decided here**, resolving a contradiction inside the RFC. §8 asks for two
things that cannot both hold: `Cli.usage (spec)` renders the help text
"wrapped to `IO.env "COLUMNS"` when present, colored only when
`IO.tty? (IO.stdout)` and no `NO_COLOR`" — and the module must run under the
spec runner with nothing installed, because `Cli.parse` / `Cli.dispatch` /
`Cli.usage` are the pure half.

A function that reads the environment is not pure, is not spec-runnable, and
its output depends on the terminal that happens to be attached — which is
also what makes it untestable. So the world comes IN:

```
Cli.usage (spec)                          → the default rendering
Cli.usage (spec) {width: 72 color: true}  → the caller's terminal, decided
```

`Cli.main` is where the reading happens: it consults `IO.env "COLUMNS"` and
`IO.is-tty (IO.stdout)` plus `IO.env "NO_COLOR"`, builds that options map,
and hands it over. The RFC's behaviour is preserved exactly; only the layer
that learns about the terminal moves — the same split, and the same reason,
as `Cli.dispatch` versus `Cli.main`.

## 4. What did not change

The RFC's parsing conventions, spec-map shape, `r.ok` / `r.err` result
shape, and the "subcommands are in the spec shape from day one, implemented
in stage two" staging all stand. The two cautions in §8 about the aless DX
round (no local alias fns and no recursion in the parse machinery; native
`is`-chains rather than multi-sig dispatch over argv values) are treated as
hypotheses to re-verify against the current engine rather than as standing
constraints — several sharp edges named there have since been fixed, and
`design/AQL-SHARP-EDGES.0.md` is the live list.
