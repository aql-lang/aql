# CLI programs, part 2: baked policy and the `Cli.dispatch` split

Supplements `design/CLI-PROGRAMS.0.md`. That note is the RFC for the whole
C1–C6 arc; this one records two decisions the RFC does not contain, each
made because implementing the RFC as written would not have worked.

## 1. `boru build -perms` bakes a `*policy.Profile`, and it is a default

**Landed** with C1 (`boru build` argv + overlay + baked policy).

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
the PROGRAM — which is what a tool author shipping a BORU script wants:
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

```boru
Cli.main (spec) handler/r
```

which parses `IO.args`, prints usage or version and exits 0 for
`--help`/`--version`, prints the error plus a usage hint to stderr and
exits 2 on a usage error, and otherwise calls the handler.

**The decision logic must be reachable without running the program.** A
module written in BORU is gated by the `sift_coverage_test.go` pattern —
every executable row of `cli.boru` covered by `cli_test.boru`, with a small
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
  `cli_test.boru`, and the interesting logic — flag grammar, clustering,
  `--flag=value`, `--no-X`, `--`, arity, unknown-flag errors, usage
  rendering — lives here.
- **`Cli.main` is the shell**: call `Cli.dispatch`, print `out` to stdout or
  `err` to stderr, then either `IO.exit code` or call the handler. It is the
  only part that touches the world, and it is small enough that reviewing it
  by eye is honest verification.

  **Correction (2026-07-30):** the first cut of this note assumed the shell's
  rows would have to be allowlisted, because an exiting arm cannot be
  exercised by a runner that treats an exit as a file failure. That is wrong.
  `IO.exit` RAISES the reserved `boru/exit`, and `Assert.throws` observes a
  raise — so the arms are reachable from the module's own suite after all,
  and `cli.boru` ships at **100% BORU-line coverage with an empty allowlist**.
  (A plain `do […] error […]` genuinely does not catch an exit — deliberately,
  so a program cannot swallow its own termination — which is what made the
  wrong conclusion plausible.) The split still stands on its own merits: the
  decision logic is pure, spec-pinnable, and reusable by any driver that wants
  to act on it itself. It just is not forced by the coverage gate.

This is the same shape the RFC itself recommends elsewhere — a pure core
with an imperative rind — and it is what makes "written in BORU, gated like
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
`design/BORU-SHARP-EDGES.0.md` is the live list.

## 5. Follow-ups from the PR #319 review

An automated review of the branch raised eleven points. Two were stale (fixed
by later commits), one was a false positive (the ADR amendment was explicitly
instructed by the maintainer), and seven were technical claims — **every one of
which was confirmed** by independent verification that ran the code rather than
read it, several coming back stronger than stated. Five were fixed in the same
PR. What is recorded here is what was deliberately NOT fixed there, and why.

The one engine-level item — the paren-group step budget failing to compose, so
`Options.Steps` does not bound a program — is recorded in
[MODULE-SECURITY.0](MODULE-SECURITY.0.md) beside the governor table it
contradicts, not here.

### 5.1 `Cli.parse` raises on a malformed spec instead of returning data

**Status: open, and the recommendation is to leave it.**

`cli.boru`'s header says a spec is read defensively and that parse errors come
back as data (`{ok:false err usage}`). That is true of the ARGUMENT VECTOR,
which is genuinely untrusted input, and it is not true of the SPEC: a
non-convertible numeric field (`args:{min:"many"}`) or a non-Map element in
`flags` raises out of `Cli.parse` / `Cli.usage` / `Cli.dispatch`.

The reviewer is factually right and the severity is low. A spec is not user
input — it is the tool author's own source, written as a literal `def spec {…}`
in every caller in this repository, so a malformed one is a bug in the program
being written, discovered the first time it runs. A raise at that point is a
better report than a `{ok:false}` the author has to remember to check.

Two things would change that assessment, and are the trigger for revisiting:
a program that BUILDS a spec at runtime from configuration, or a `boru:cli`
caller that accepts a spec across a trust boundary. Neither exists today.

If it is fixed, the shape is a `Cli.check-spec (spec)` returning
`{ok, err}` — validation as a separate, callable word — rather than making
every entry point defensive, so the cost falls on the author who wants the
check and not on every parse.

### 5.2 Two defects in the end-to-end test itself

Found by an adversarial audit of `cmd/go/internal/build/utils_e2e_test.go`,
both reproduced by execution. **Both are open.** They matter more than their
size suggests, because this file is the tripwire that keeps `utils/` from
rotting the way `kg/` did — a tripwire that fails for the wrong reason, or
passes for the wrong reason, is worse than none.

1. **A hidden ordering dependency between sibling subtests.** In
   `TestUtilsCatEndToEnd`, the subtest *"a flag in argv is a flag, not an
   operand"* derives `dir/in.txt` but never writes it: it consumes the file the
   PREVIOUS subtest wrote, and works only because Go runs non-parallel subtests
   in registration order. Run alone — which is how anyone iterates on a
   ten-second test — it fails with `stdout="", want -n to number the lines`,
   blaming `-n` for a missing operand. (`-shuffle` does NOT trigger it: it
   reorders top-level tests only.) Fix: give the subtest its own `t.TempDir()`
   and write its own input, and assert `got.code == 0` alongside the stdout
   check so a missing operand can never again masquerade as a flag defect.

2. **The build inherits an ambient `BORU_POLICY`.** `buildUtil` calls the real
   subcommand, and `permsflags.Resolve` falls back to `BORU_POLICY_FILE` /
   `BORU_POLICY` when no `-perms` flag is given. Five of the six tests build
   without `-perms`, so whatever policy the environment names is silently baked
   into those binaries: `BORU_POLICY=read-only go test -run TestUtilsCatEndToEnd`
   fails with empty stdout and exit 1. The failure direction is the less
   dangerous one — a PERMISSIVE ambient policy would instead mask a real
   regression. `permsflags_test.go` already pins both variables with
   `t.Setenv`, so this is a deviation from the repo's own convention rather
   than a novel hazard. Fix: pin them empty in the test's setup.

The baked-permissions pair is immune to both: it passes `-perms` explicitly,
and `buildrt` reads only `cfg.Profile` at run time, never the environment.
