# CLI PROGRAMS

Design for **full command-line program support in BORU** — the layer where a
developer writes a real Unix-style tool in BORU (`mytool --fast notes.json`,
`cat data | mytool -o out.json`, a packaged `boru build` binary on `$PATH`)
and gets the things every CLI runtime owes its programs: arguments, an
argument-parsing convention, environment access, exit codes, stream
discipline, signals, and (policy-gated) subprocesses.

The motivating client is the `aless` file viewer (voxgig-boru/aless
`viewer/`), the first substantial end-user program written in BORU. Its
2026-07-21 DX round found the runtime strong exactly where BORU has
invested — packaging (`boru build`/`pack`/`install`), per-invocation
permission policies, structured output (`emit`, `boru:report`, `boru:log`),
stdio handles, and the TUI stack — and empty on the invocation side: a
script could not see its own command line, read an environment variable,
choose an exit code, notice a signal, or run another program. `IO.args`
(the first slice of this design) has landed; this RFC completes the
picture.

The through-line of the design: **invocation context is host-injected
capability state, delivered through the same seams every other host
resource uses** (the `CapX` slots + `SetHostX` installers of
`lang/go/native/capabilities.go`), and **anything asynchronous arrives as
actor mailbox messages** — the substrate `PROCESSES.0.md` built and
`boru:tui` proved out. Nothing here invents a new mechanism; each section
is a new tenant on an existing floor.

This is a **design RFC** in the house style (`TUI.0.md`,
`PROCESSES.0.md`): sections are the spec, §11 is the staged plan. One
stage (C0/argv) is already implemented and cited as the pattern the rest
follow.

> **Decisions proposed at design time** (the forks this RFC closes; a
> reviewer ratifies or reopens them):
> (1) invocation context (args, env) is **host-injected capability
> state**, never read by the runtime from the OS directly — hermetic
> registries and embedded hosts stay in full control (§1);
> (2) argument *parsing* is a **loadable BORU module (`boru:cli`)**, not a
> native word — the raw vector is the runtime surface, conventions are
> library (§8);
> (3) `IO.exit` unwinds via a **reserved control error** the drivers
> recognize — no new engine mechanism, and `do…error` handlers can
> observe but are documented not to swallow it (§4);
> (4) signals and subprocess output are **delivered to a Pid mailbox**,
> never via callback bodies — the callback-fork path is exactly what the
> `IO.watch`-under-`Tui.run` starvation bug (aless dx §6) shows cannot be
> trusted while a driver owns the runtime (§6, §7);
> (5) subprocess execution takes an **argv vector only** — there is no
> shell-string form at any tier, `{shell:true}` is explicitly rejected as
> a non-goal (§7);
> (6) `proc.exec` is **deny-by-default under any configured policy** —
> the one scope that inverts the permissive default, because it is the
> one that escapes the sandbox (§7.3).


## 0. Current state (verified 2026-07-21, main @ c1d2a1a + branch)

| Concern | State |
|---|---|
| Invocation forms | `boru script.boru`, `boru -e`, REPL, `boru /dev/stdin`, `boru build` standalone binaries |
| Arguments | ✅ `IO.args` landed for `run`/`-e` (branch); ❌ `boru build` binaries still discard argv (`buildrt.Main(_, _ []string, …)`) |
| Argument parsing / `--help` | ❌ nothing — userland string-splitting |
| Environment | ❌ no read access of any kind |
| Exit codes | half: clean run → 0, uncaught error → 1; ❌ no chosen codes |
| stdin/stdout/stderr | ✅ `IO.stdin`/`stdout`/`stderr` handles; `IO.read (IO.stdin)` slurps; ❌ no line-at-a-time read, no isatty |
| Structured output | ✅ `emit` family, `boru:report`, `boru:log` |
| Packaging | ✅ `boru build`/`prep`/`pack`/`publish`/`install` + registry |
| Permissions | ✅ `-perms`/`-allow`/`-deny` policy per invocation |
| Signals | ❌ nothing (Ctrl-C only inside the tui driver) |
| Subprocess | ❌ nothing (by design so far; §7 changes this deliberately) |
| Long-running / TUI | ✅ `boru serve` + ctl, `boru:tui`, `Tui.serve`/`boru attach` |

The ❌ rows are this RFC's scope.


## 1. The invocation contract

One host-facing contract, honored identically by every way a BORU program
can start:

```
args:    []string      what followed the program name
env:     lookup + all  the environment, read-only
stdin:   stream        already present (IO.stdin)
stdout:  stream        already present (IO.stdout)
stderr:  stream        already present (IO.stderr)
exit:    int           how the program ends (§4)
signals: messages      delivered on request (§6)
```

**Injection, not ambient reads.** The runtime never reaches for
`os.Args`/`os.Environ` itself. Each host installs what it wants the
program to see:

- `boru run` / `-e`: CLI positionals (`fs.Args()[1:]`, or all positionals
  in `-e` mode) and the real process environment.
- `boru build` binaries: `os.Args[1:]` and the real environment —
  `buildrt.Main` gains the plumbing it currently drops (this is the
  C1 headline: a compiled tool that cannot see its arguments is not a
  tool).
- REPL / spec runner / hermetic tests: nothing, unless the test says so.
- Embedded hosts: whatever they choose, via `lang.Options`.

**Surface.** `lang.Options` grows alongside the existing fields:

```go
ScriptArgs []string            // landed (C0)
Env        func(string) (string, bool)  // nil = no environment visible
```

`Env` is a lookup function, not a copied map: the common case reads two
or three variables, `IO.env` (no argument) materializes the full map
lazily via an enumerator the host also provides (concretely: an
interface `capabilities.EnvOps { Get(string) (string, bool); All() []string }`
with an OS-backed and a map-backed fake implementation, mirroring
`FileOps`). Capability slots `CapScriptArgs` (landed) and `CapEnv`
follow the `HostX`/`SetHostX` pattern.

**Precedent.** C0 (landed) is the template each row of this contract
copies: capability slot + typed accessors + `lang.Options` field + CLI
plumbing + spec rows for the deterministic (uninstalled) surface + Go
tests for the host-dependent surface. It cost ~60 lines plus tests and
passed the coverage gate without allowlist entries; C1–C2 are the same
shape.


## 2. Arguments — `IO.args` (landed) and build parity

`IO.args → List` returns the installed vector as Strings; **empty list
when none is installed** — never `none`, never an error, so programs
iterate unconditionally. The inner native is `script-args` (the core
fn-arguments word `args` shares the module sub-registry; BuildIOModule
renames on export — the log/debug rename pattern).

Remaining work (C1): thread argv through `buildrt.Config` so `Main`
installs `os.Args[1:]` before running the embedded program, with a
`cmd/go` test that `boru build`s a program and runs it with positionals.
The CLI should also stop *silently* dropping positionals in the one form
that still ignores them (none, after C1 — the warning becomes moot).


## 3. Environment — `IO.env`

```
IO.env <name>   → String | none      one variable ("" is a real value; none = unset)
IO.env          → Map                snapshot of everything visible
```

**Shipped as two words:** `IO.env <name>` and `IO.env-all`. The 0-arg
overload above was unreachable when C1 landed (a 0-arg signature won
unconditionally on a module export — NUR035, since fixed in the engine),
and the split was kept on its own merits once it was no longer forced:
each arity is a separately describable word, and enumerating the whole
environment is visible at the call site.

- Read-only. There is no `IO.set-env`: mutating one's own environment is
  almost always a smell (child processes get their env explicitly in §7,
  which is the actual use case).
- Policy scope `env.read`, following the existing scope grammar. A nil
  policy allows (the global convention); a configured profile can deny
  wholesale or the host can install a filtered `EnvOps` that exposes an
  allowlist — the seam supports both without new mechanism.
- Spec rows pin the uninstalled surface (`IO.env "HOME"` → none,
  `size (IO.env)` → 0); Go tests pin the installed one through a fake.
- Well-known conventions (`NO_COLOR`, `TERM`, `HOME`) become *possible*
  and stay app-level; §5's `tty?` plus `IO.env "NO_COLOR"` is the
  documented color-detection recipe (HOWTO chapter, §10).


## 4. Exit codes — `IO.exit`

```
IO.exit <code>       terminate with this exit code (0..125)
```

Mechanism: `IO.exit` raises a **reserved control error**
(`boru/exit`, payload `{code}`). The run/build drivers recognize it at
the top level: unwind, flush output, exit with the code, print nothing.
Everything else is unchanged: clean run → 0, any other uncaught error →
1. No `os.Exit` inside the runtime — embedded hosts see the error value
and decide for themselves (`lang` exposes `ExitCode(err) (int, bool)`).

`do … error …` handlers *can* observe `boru/exit` (it is an error value;
inventing an uncatchable error class is a bigger mechanism than the
problem deserves). The documented contract is: **a handler that catches
a foreign error it does not recognize must re-raise it**; the spec pins
that an `IO.exit` crossing a `do` without a matching handler still exits
with its code, and REFERENCE.md documents the re-raise rule where
`do`/`error` is specified.

Convention (enforced nowhere, documented everywhere, used by `boru:cli`):
`0` success, `1` runtime failure, `2` usage error.


## 5. Streams — line reads and terminal probes

Two small additions make filter-style programs first-class:

```
IO.read-line <stream|File>   → String | none     next line (LF or CRLF stripped); none at EOF
IO.tty? <stream>             → Boolean           is this stream a terminal?
```

- `IO.read-line` buffers internally on the handle (the `File`/stream
  payloads are already stateful resources — the cursor precedent is
  `IO.seek`). The canonical filter loop is a `for`+`break` or
  fold-with-sentinel over it; the HOWTO chapter shows both. Whole-input
  slurping stays `IO.read (IO.stdin)`.
- `IO.tty?` answers per-stream (stdout piped, stderr on a terminal is
  common), through a host-injected probe on the same handle types — the
  spec runner's streams answer false; termback answers truthfully. This
  is the missing half of the color story: `tty?` + `NO_COLOR` + the
  existing `Tui.colorize {profile}` compose into "color when
  appropriate" without the runtime hard-coding the decision.
- Terminal *size* outside a running TUI is deliberately excluded: a
  non-TUI program that wants width-aware layout reads `IO.env "COLUMNS"`
  or opens the tui tier; wiring SIGWINCH into plain scripts is TUI-scope
  (`TUI.0.md` owns it).


## 6. Signals — mailbox delivery

```
IO.signals <Pid>                → Subscription    deliver signals as messages
IO.signals <Pid> [int term hup] → Subscription    subset
IO.unsubscribe <Subscription>                     restore default handling
```

Delivery: each arriving signal becomes `{tag: "signal", name: "int"}` in
the Pid's mailbox — **the same contract `Tui.run` already honors for any
message**, so a TUI app subscribes its `"tui"` process and its `update`
sees `{tag:"signal"}` like any other event; a headless service spawns a
handler process. While a subscription covers INT/TERM, default
termination is suspended for those signals; dropping the last
subscription restores it. KILL is not interceptable; the Windows story
is INT-only (Ctrl-C) and documented.

Explicitly **not** a callback body: the `IO.watch` starvation finding
(aless dx §6 — callback forks never run while a driver owns the runtime)
makes body-style delivery untrustworthy in exactly the programs that
need signals most. Mailbox delivery is the path that demonstrably works
under `Tui.run` (`send {…} "tui"` from the metronome process). Fixing
`IO.watch`'s own delivery the same way is tracked in the companion
proposal (aless `proposals/tui-live-io-and-testability.md`) and shares
this section's plumbing.


## 7. Subprocess — `boru:proc`

The largest addition, and the one that changes BORU's security posture,
so it gets the tightest contract.

### 7.1 Surface

```
import "boru:proc"

Proc.run <argv:List> {opts}      → {code, out, err}         synchronous, captured
Proc.spawn <argv:List> {opts} <Pid> → Child                 streaming, async
Proc.write <Child> <String|Bytes>                           feed child stdin
Proc.close <Child>                                          close child stdin
Proc.kill <Child> {signal}                                  terminate
```

- `argv` is a List of Strings — program name first, arguments after.
  **There is no shell-string form and no `{shell:true}` option**; a
  program that wants a shell says so in argv (`["sh", "-c", …]`) and
  policy sees `sh` as the program. This keeps injection a visible choice
  rather than a default.
- `{opts}`: `{dir, env, stdin, timeout}` — `env` is the child's full
  environment as a Map (no implicit inheritance; the recipe for
  "inherit + override" is `(IO.env) merge {…}`, which is why §3 has the
  no-argument form), `stdin` a String/Bytes to feed and close, `timeout`
  in ms after which the child is killed and `code` reports it.
- `Proc.run` blocks the calling process (not the scheduler — it parks
  like `TimeUtil.sleep`) and returns captured output with a size cap
  (`{limit}` opt, default a few MB, exceeded → error).
- `Proc.spawn` delivers to the given Pid — again mailbox, never
  callbacks: `{tag:"proc-out", chunk}`, `{tag:"proc-err", chunk}`,
  `{tag:"proc-exit", code}`. A TUI app can babysit a build and paint its
  output with no new mechanism.

### 7.2 Capability seam

`capabilities.ProcOps` interface (start/wait/kill/pipes) with the
OS-backed implementation and a scriptable fake — the fake is what the
Go tests and the coverage gate exercise (ADR-008 makes an untestable
os/exec path a non-starter; the seam is not optional plumbing, it is how
the feature is testable at all). Slot `CapProc`, `SetHostProcOps`,
uninstalled → `capability_not_installed` error, which is also the spec
runner's deterministic surface.

### 7.3 Policy

Scope `proc.exec`, with the **inverted default**: a nil policy (the
"no permissions configured" historical mode) allows, matching every
other scope — but any *configured* policy denies `proc.exec` unless it
grants it explicitly (`-allow proc.exec`, or a profile listing it).
Rationale: every other scope mediates access to resources the sandbox
still contains; exec escapes the sandbox entirely, so "I configured
permissions" must not silently include it. This is the one place the
RFC breaks scope-grammar symmetry, and it is deliberate (decision 6).
Optional refinement, staged later: `proc.exec:<basename>` per-program
grants.


## 8. Argument parsing — the `boru:cli` module

The runtime surface stays the raw vector; conventions live in a
loadable module, **written in BORU** (dogfooding; pure; testable by the
aless suite conventions; no Go coverage cost). Its shape follows the
`boru:test` precedent — a declarative spec map driving an imperative
surface:

```boru
import "boru:cli"

def spec {
  name: "av"  version: "0.2.0"
  summary: "view structured files in the terminal"
  flags: [
    {name: "watch"  short: "w"  kind: "bool"    default: true
     help: "reload files when they change"}
    {name: "kind"   short: "k"  kind: "string"  value: "KIND"
     help: "force a parser kind"}
    {name: "verbose"            kind: "count"
     help: "-v, -vv…"}
  ]
  args: {name: "files"  min: 0  help: "files to open"}
}

def r (Cli.parse (spec) (IO.args))
# r = {ok: true,  flags: {watch: true, kind: none, verbose: 0}, args: [...]}
#   | {ok: false, err: "unknown flag: --wach", usage: "…"}
```

Parsing conventions (all pinned by the module's own spec rows):
`--long`, `-s`, clustered shorts (`-vv`), `--flag=value` and
`--flag value`, `--no-X` for booleans, `--` ends flags, repeated
string flags collect into a List, unknown flag / missing value / arity
violation → `{ok:false}` with a one-line error and the usage text.
`--help` and `--version` are recognized by:

```boru
Cli.main (spec) handler/r
```

which parses `IO.args`, prints usage or version to the right stream and
exits `0` for `--help`/`--version`, prints the error + usage hint to
stderr and `IO.exit 2` on a usage error, and otherwise calls
`handler {flags, args}` — whose return value (Integer) becomes the exit
code (anything else → 0). `Cli.usage (spec)` renders the help text —
generated, wrapped to `IO.env "COLUMNS"` when present, colored only when
`IO.tty? (IO.stdout)` and no `NO_COLOR` (§3, §5 composing).

Subcommands (`spec.commands: [{name, summary, flags, args, …}]`) are
part of the spec shape from day one but ship in the module's second
stage; `Cli.parse` reports the chosen command in `r.command`.

Two implementation cautions from the aless DX round, binding on the
module author: no local alias fns and no recursion in the parse
machinery (dx §5 — fold-based like `AvTabs`), and native `is`-chains
rather than multi-sig dispatch on values from the args vector (dx §7).
If those engine bugs are fixed first (the soundness proposal), this
paragraph deletes; until then it is load-bearing.

`boru:cli` depends only on `boru:io` (§2–§5) and `boru:string-util` — it
must run under the spec runner with nothing installed (parse is pure:
vector in, map out).


## 9. Non-goals

- **Shell-string execution** in any form (§7; `["sh","-c",…]` is the
  visible spelling).
- **Environment mutation** (`IO.set-env`) — children get env explicitly.
- **A prompt/readline library** — interactive input is `boru:tui`'s
  inline tier (`TUI.0.md` §9), not a parallel stack.
- **A config-file framework** — `IO.read` already parses every format;
  the HOWTO shows the ten-line XDG recipe (`IO.env "XDG_CONFIG_HOME"` +
  fallback + `IO.read`); framework-ness earns nothing.
- **Chosen exit codes above 125**, signal-death codes, and other
  waitpid arcana: `code` reports what the OS said; BORU programs choose
  0–125.
- **Windows signal parity** beyond Ctrl-C→`int` (documented limitation,
  aligned with the platform).

## 10. Testing, docs, and repo discipline

Per-stage, the C0 pattern (already merged, cover-gate clean) is
normative:

- **Spec TSVs** pin every deterministic surface: the uninstalled
  defaults (`IO.args` → `[]`, `IO.env "X"` → none,
  `Proc.run` → `capability_not_installed`), the pure module
  (`boru:cli` parse tables get their own `module-cli.tsv`, positive and
  negative rows in pairs per the house rule).
- **Go tests** pin host-dependent behavior through fakes (`EnvOps`,
  `ProcOps`, stream fakes for `read-line`/`tty?`), plus one CLI-level
  test per invocation form (`run`, `-e`, built binary) per feature.
- **ADR-008**: every stage lands at 100% statement coverage; the
  capability fakes exist precisely so no path needs an allowlist entry.
- **Docs**: `describe` summaries at word-registration time (the
  `docs_io.go` map — the catalog-sync test enforces completeness);
  REFERENCE.md sections per word; a new HOWTO chapter **"Write a
  command-line tool"** walking one tool from `IO.args` through
  `Cli.main`, exit codes, NO_COLOR, and a `Proc.run` step; CLI.md notes
  the positional-forwarding behavior of `run` and built binaries.
- **kg**: the project graph gains the `boru:cli`/`boru:proc` module nodes
  and the HOWTO chapter when they land (`make -C kg graph`).

## 11. Staged plan

Each stage lands green and independently useful; sizes are relative to
C0 (≈60 lines + tests, one day including the gauntlet).

- **C1 — invocation parity** (S): argv through `buildrt.Config` into
  built binaries; `IO.env` + `EnvOps` + `env.read` scope; `IO.exit` +
  driver recognition + `lang.ExitCode`. The contract of §1 holds
  everywhere. *Unblocks: every packaged tool.*
- **C2 — stream ergonomics** (S): `IO.read-line`, `IO.tty?`, HOWTO
  color recipe. *Unblocks: filters and pipelines.*
- **C3 — `boru:cli`** (M, BORU-side): parse/usage/main, flags tier;
  `module-cli.tsv`; HOWTO chapter. *Unblocks: conventional UX;
  `aless` migrates its launcher as the reference client.*
- **C4 — signals** (M): subscription word + mailbox delivery + driver
  integration (suspend/restore default handling); `Tui.run` apps get
  clean SIGTERM. *Unblocks: services and long-running tools.*
- **C5 — `boru:proc`** (L): ProcOps seam + fake, run/spawn/write/kill,
  `proc.exec` policy inversion, streaming into mailboxes. *Unblocks:
  orchestration tools; also the `aless` clipboard-yank deviation.*
- **C6 — `boru:cli` subcommands** (S, BORU-side): nested specs,
  per-command usage. *Unblocks: multi-verb tools.*

Ordering rationale: C1–C2 are pure catch-up with zero design risk and
maximal unblocking; C3 needs only C1 and makes every subsequent tool
readable; C4–C5 ride the actor substrate and are independent of each
other; C6 is sugar on C3. A reviewer can cut the line after any stage
and the runtime is strictly better than before it.
