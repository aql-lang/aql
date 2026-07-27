# AQL CLI Reference

The `aql` binary bundles the language runtime, the REPL, a static
type-checker, a code formatter, a module-packaging toolchain, a
registry client, a local key vault, a Language Server, and a
service supervisor. This document describes every subcommand it
supports.

## Contents

* [Quick start](#quick-start)
* [General usage](#general-usage)
  * [`--options`](#--options--engine-options-as-jsonic)
  * [Bytecode compilation](#bytecode-compilation)
* [Language execution](#language-execution)
  * [`aql` / `aql run`](#aql--aql-run)
  * [`aql do`](#aql-do)
  * [`aql check`](#aql-check)
  * [`aql test`](#aql-test)
  * [`aql help`](#aql-help)
  * [`aql describe`](#aql-describe)
  * [`aql fmt`](#aql-fmt)
  * [`aql model`](#aql-model)
  * [`aql build`](#aql-build)
* [Project lifecycle](#project-lifecycle)
  * [`aql prep`](#aql-prep)
  * [`aql pack`](#aql-pack)
  * [`aql clean`](#aql-clean)
* [Registry client](#registry-client)
  * [`aql install`](#aql-install)
  * [`aql register`](#aql-register)
  * [`aql login`](#aql-login)
  * [`aql publish`](#aql-publish)
* [Secrets](#secrets)
  * [`aql vault`](#aql-vault)
* [Permissions](#permissions)
  * [`aql policy`](#aql-policy)
  * [Per-command policy flags](#per-command-policy-flags)
* [Supervisor control](#supervisor-control)
  * [`aql ctl`](#aql-ctl)
* [Long-running services](#long-running-services)
  * [`aql repl`](#aql-repl)
  * [`aql registry`](#aql-registry)
  * [`aql lsp`](#aql-lsp)
  * [`aql exec`](#aql-exec)
  * [`aql serve`](#aql-serve)
  * [`aql tui`](#aql-tui)
* [REPL meta-commands](#repl-meta-commands)
* [Exit codes](#exit-codes)


## Quick start

```bash
# Until v0.1.0 is tagged, build from a clone:
git clone https://github.com/aql-lang/aql
cd aql/cmd/go && go install ./aql

aql -version
aql                                 # start the REPL
aql do 'add 1 2'                    # one-shot expression
aql script.aql                      # run a file
aql check script.aql                # type-check without running
aql fmt script.aql                  # format in place (always rewrites)
```


## General usage

```
aql [options] [script.aql]
aql <subcommand> [args...]
```

When the first argument is a registered subcommand, the binary
dispatches to it. Otherwise the legacy "execute or REPL" path runs.

Global flags accepted by `aql` (and equivalently by `aql run`):

| Flag | Meaning |
|------|---------|
| `-e EXPR` | Evaluate `EXPR` and exit. |
| `-r PATH` | Path to a local registry (used by import and install). |
| `-s INT` | Random seed for ID generation. Default: current time. |
| `-check` | Run static type-check before execution; abort on error. |
| `-compile` | **Experimental.** Execute via the bytecode compiler when the program is compilable; silently fall back to the interpreter otherwise (see [Bytecode compilation](#bytecode-compilation)). |
| `-force-compile` | **Experimental.** Require the bytecode compiler; abort with the refusal reason if the program is not compilable (see [Bytecode compilation](#bytecode-compilation)). |
| `-options OPTS` | Engine options as a jsonic blob (see below). |
| `-version` | Print the version and exit. |


### `--options` — engine options as jsonic

`--options` takes a single **jsonic** value — relaxed JSON where a bare
`key:value,key:value` is an implicit map and a **colon chain nests**, so
options read like dotted paths:

```bash
aql --options tape:initial:65536 script.aql          # {tape:{initial:65536}}
aql --options 'tape:initial:65536,tape:grows:9'      # two tape knobs
aql --options 'tape:{initial:65536,grows:9,factor:3}' # explicit nested map
```

The blob is a **nested map**; the same jsonic that parses AQL data parses
it (`a:1,b:c:2` → `{a:1, b:{c:2}}`). Option handling is **strict** — an
unknown key or a wrong-typed value is an error, not a silent no-op, so a
typo fails loudly:

```
$ aql --options tape:boguskey:10 -e 'add 1 2'
error: unknown tape option "boguskey" (known: initial, grows, factor)
```

Recognised options:

| Path | Type | Meaning | Default |
|------|------|---------|---------|
| `tape:initial` | int | Initial execution-tape capacity, in entries. | program size, floored at 1024 |
| `tape:grows`   | int | Max number of tape reallocations (N). | 7 |
| `tape:factor`  | number | Per-grow size multiplier (M). | 2.7 |

The tape is the engine's execution buffer. It grows at most `grows`
times by `factor` from `initial`, a hard ceiling of `initial · factorᴺ`
entries; crossing 90/95/99% of it warns on stderr and exceeding it fails
loudly with `[aql/tape_exhausted]` — the engine never consumes unbounded
space. Raise `tape:initial` (or `tape:grows`) for a legitimately large
program (deep recursion, huge generated programs); lower it to trip a
runaway sooner. See `design/TAPE-DATA-STRUCTURE.10.md`.


### Bytecode compilation

> **Experimental.** AQL ships an optional bytecode compiler that lowers the
> statically-typed subset of a program to a compact instruction stream and runs
> it on a small VM. **Execution defaults to the interpreter** — the compiler is
> opt-in and produces results identical to the interpreter, so the choice is
> purely about performance (and about exercising the compiler).

There are two modes, selected per run by a flag or an environment variable:

| Flag | Env | Behaviour |
|------|-----|-----------|
| `--compile` | `AQL_COMPILE` | **Best-effort.** Compile and run on the VM when the whole program is compilable; **silently fall back** to the interpreter when any part is not. Never changes the result. |
| `--force-compile` | `AQL_FORCE_COMPILE` | **Strict.** *Require* the bytecode path. If the program is not compilable, abort with the emitter's refusal reason instead of falling back. Use this to *guarantee* a run went through the compiler (verifying the compilable subset, benchmarking the VM, or catching a compiler regression the silent fallback would hide). |
| *(none)* | `AQL_NO_COMPILE` | **Interpreter** — the default. `AQL_NO_COMPILE` is a forward-compatible kill switch that **overrides** both of the above (and the future default flip), so a deployment can pin the interpreter. |

Precedence: `AQL_NO_COMPILE` wins over everything; otherwise `--force-compile` /
`AQL_FORCE_COMPILE` wins over `--compile` / `AQL_COMPILE`.

```bash
aql --compile script.aql              # try the compiler, fall back silently
aql --force-compile script.aql        # demand the compiler; fail loudly if it can't
AQL_COMPILE=1 aql script.aql          # same as --compile, via the environment
AQL_NO_COMPILE=1 aql --compile s.aql  # kill switch: runs the interpreter anyway
aql do --force-compile 1 add 2        # the flags work on `aql do` too
```

A `--force-compile` refusal names the construct the emitter could not lower:

```
$ aql --force-compile -e '(size (for 5 [i]))'
error: force-compile: consumes loop results (Stage 2 loops only feed the program residual)
```

Both flags are accepted by `aql` / `aql run` and by `aql do`. Genuine runtime
errors (e.g. division by zero, a type error) surface identically in every mode;
only the *uncompilable* outcome differs — silently absorbed by `--compile`,
reported by `--force-compile`.

## Language execution

### `aql` / `aql run`

Execute a script, an `-e` expression, or drop into the REPL when
nothing is supplied.

```bash
aql                         # REPL
aql -e 'add 1 2'            # prints "3"
aql script.aql              # runs the file
aql -check script.aql       # type-check first, then run
aql -e '...' -r ./registry  # with a custom registry
```

Output: the final stack contents, space-separated, on stdout.
Errors go to stderr; exit code 1 on failure.

**Program arguments (`IO.args`).** Anything after the script path — or,
in `-e` mode, after the expression — is the program's own argument
vector, readable with `IO.args` (`import "aql:io"`). `-e` **ends option
processing** (the `node -e` / `python -c` convention), so a dash-prefixed
first argument reaches the program instead of being read as an `aql`
flag:

```bash
aql script.aql --fast a       # IO.args → ['--fast' 'a']
aql -e '…' --fast a           # IO.args → ['--fast' 'a']  (no separator needed)
aql -e '…' -- --fast          # a leading `--` is accepted and stripped
```

`aql` flags for the run itself go **before** the script/`-e` expression
(`aql -s 5 -e '…' --fast` seeds the run and passes `--fast` to the
program).

### `aql do`

Evaluate the remaining args as an AQL expression. Slightly more
shell-friendly than `aql -e` because positional words don't need
extra quoting.

```bash
aql do add 1 2                  # prints 3
aql do 'import "aql:string-util" "hello" StringUtil.upper'  # prints HELLO
aql do 'iota 5 each [dup mul]'  # prints [0 1 4 9 16]
```

If the expression **begins** with a negative number, the leading
`-N` is parsed as an unknown command-line flag
(`flag provided but not defined: -7`). Separate the flags from the
expression with `--`:

```bash
aql do -- '-7 0 add'           # leading negative literal needs --   (prints -7)
```

`aql do` accepts the same `--compile` / `--force-compile` flags as `aql run`
(see [Bytecode compilation](#bytecode-compilation)); place them before the
expression: `aql do --force-compile 1 add 2`.

### `aql check`

Run the static type-checker without executing. It drives the same
engine in carrier mode — so checking stays in lockstep with runtime
dispatch — reports diagnostics to stderr, and exits non-zero when any
Error-severity diagnostic is found (exit 0 with `--soft`).

```bash
aql check script.aql
aql check -e '1 add "x"'
aql check --json script.aql        # machine-readable output
aql check --soft script.aql        # exit 0 even on errors
aql check --strict script.aql      # surface every dynamic dispatch
```

Flags:

* `-e EXPR` — type-check an inline expression.
* `--json` — emit JSON diagnostics.
* `--soft` — return exit code 0 even when diagnostics are reported.
* `--strict` — additionally report (as non-gating info) every dispatch
  over a dynamic operand: the points where the checker matched
  optimistically and the runtime re-verifies. The gradual-typing
  migration surface — tighten these and the diagnostics disappear.
* `-r PATH`, `-s SEED` — same as `aql run`.

**What it catches** (full list in the language reference's diagnostics
table):

* `no_signature` / `uncalled_function` — a call that matches no
  signature. `uncalled_function` covers the silent case where a named
  function value (e.g. an imported `Pkg.fn`) is called with the wrong
  arguments and would be left on the stack as data at runtime instead
  of erroring.
* `unreachable_signature` — an `fn` overload that an earlier, more
  general overload already subsumes, so first-match dispatch can never
  reach it.
* `undefined_word`, `unused_def`, `unreachable_branch`,
  `record_shape_mismatch`, … — typos, dead bindings, constant `if`
  branches, record-field mismatches.

**Every run pre-flights by default.** `aql run` (and `aql -e`) runs
the checker first and aborts before executing if any error is found,
so a type bug can't slip into a run. The default gate is quiet: a
clean program produces no checker output at all (stdout and stderr
stay clean for the program's own output); diagnostics print only when
the run is about to abort. `--check` upgrades the pre-flight to
verbose (all diagnostics, including the advisory tiers);
`--no-check` (or `AQL_NO_CHECK=1`) skips the pre-flight entirely:

```bash
aql script.aql               # checked by default; aborts on any error
aql --check script.aql       # verbose pre-flight: advisories too
aql --no-check script.aql    # skip the pre-flight (runtime errors only)
```

**Rich diagnostics.** Errors and check findings render as full
diagnostic reports: the source excerpt with a caret under the failing
word, secondary labeled locations (the declaration a violated contract
came from, where an offending value was produced), `= note:` lines
explaining *why* — for a failed call, per-candidate verdicts like
``candidate `upper (String)` — argument 1: expected String, got 99 (an
Integer)`` — and `= help:` lines with actionable fixes (``did you mean
`upper`?``, "did you swap the arguments?", `see aql describe <word>`).
Each `check:` one-liner keeps its stable grep-able format; the rich
block below it is additive.

**Color.** `run`, `do`, and `check` take `--color auto|always|never`
(default `auto`: color only when the output is a real terminal, and
never when `NO_COLOR` is set). The REPL auto-detects the same way.
Machine-read surfaces — `check --json`, the LSP, the wasm playground —
are always plain.

```bash
aql do --color always '99 upper'   # force the ANSI palette (e.g. under a pager)
aql check --color never script.aql # plain text even on a terminal
```

**In your editor.** `aql lsp` publishes these same diagnostics as you
type — see [`aql lsp`](#aql-lsp).

### `aql test`

Discover and run test suites written with the `aql:test` framework. A
suite is any `*_test.aql` file that imports `aql:test` and registers
cases with `Test.test` / `Test.describe` (parking cases with
`Test.skip`). `aql test` runs each file, prints its per-case report,
and exits non-zero if any case failed or any suite errored.

```bash
aql test                            # run every *_test.aql under the cwd
aql test math_test.aql              # run one suite file
aql test src/ lib/                  # walk several directories
aql test --coverage sift_test.aql   # add coverage + an HTML report
```

Suites run on the **bytecode compiler by default** — the normal, fast
execution mode — falling back to the interpreter only for a file the
compiler refuses. Discovery: no arguments walks the current directory
recursively for `*_test.aql`; a directory argument is walked the same
way; an explicit file argument is run verbatim (even without the
`_test.aql` suffix).

Flags:

* `--coverage` — measure line coverage of every user module a suite
  imports (`import "./mod.aql"`), aggregated across all suites. It prints
  a summary line per module —
  `cover <id>  <pct>% (<covered>/<total>)  uncovered: <lines>`, where the
  `uncovered:` list names the exact source lines that never ran — and
  writes a browsable **HTML report** to a `coverage/` folder: an
  `index.html` summary plus one page per module with each source line
  coloured covered (green) or uncovered (red). Because the bytecode VM
  folds some source positions, compiled coverage is a *subset* of the
  interpreter's (some folded lines show as falsely uncovered); pair with
  `--no-compile` for the exact, line-granular set when driving a module
  to full coverage.
* `--coverage-dir PATH` — where the HTML report is written (default
  `coverage`). Ignored without `--coverage`.
* `--coverage-min PCT` — fail the run (exit 1) when **aggregate** line
  coverage — total covered lines over total executable lines across every
  imported module — is below `PCT`, even if every test passed. Implies
  coverage measurement, so `aql test --coverage-min 80` gates without
  needing `--coverage` (add `--coverage` too if you also want the HTML
  report). Pair with `--no-compile` so folded compiled positions don't
  drag the number below the real figure.
* `--no-compile` / `--force-compile` / `--compile` — select the engine
  exactly as for [`aql run`](#bytecode-compilation).
* `-r PATH` — registry path, same as `aql run`. The permission flags
  (`--perms`, `--allow`, `--deny`, …) are accepted too.

### `aql help`

`help` documents the **aql tool**: its subcommands and their flags.
(For the **language** — words, categories, and modules — use
[`aql describe`](#aql-describe).)

```bash
aql help                    # introduction + the subcommand list (this overview)
aql help vault              # one subcommand: summary, and where its flags live
aql help check
```

With no argument it prints an orientation to the tool — the two kinds
of help (`help` for the CLI, `describe` for the language), the usage
forms, and every subcommand. With a subcommand name it prints that
command's one-line summary and points at `aql <subcommand> -h` for the
full flag set. An unknown name exits non-zero and suggests
`aql describe <name>` in case a language word was meant.

### `aql describe`

`describe` documents the **AQL language**: its built-in words, the
categories they fall into, and the loadable modules.

```bash
aql describe                       # a categorised guide to every word and module
aql describe add                   # full docs for one word: signatures, examples, notes
aql describe math                  # the words in one category
aql describe aql:type-util         # a module and the words it exports
aql describe aql:type-util:tpartial   # one exported word of a module
```

The forms:

* **no argument** — every built-in word grouped by category (math,
  compare, boolean, string, type, …), then the loadable modules, then
  the drill-in forms below.
* **`<word>`** — the word's signatures, precedence, description, and
  examples (the same data the REPL `describe` word and the LSP hover
  show). A bare name is matched as a category first, so `describe type`
  opens the *type* category; use `describe typeof` for the word.
* **`<category>`** — the words in that category, each with its one-line
  summary.
* **`aql:<module>`** (or a bare built-in module id like `math-util`) —
  the module's summary and exported words.
* **`aql:<module>:<word>`** — a single exported word, with its module
  provenance.

A name that matches none of these is given one more chance: `describe`
tries to **load it as a module** — a native `aql:` module, an installed
module, or a file path (`./lib.aql`) — and describes it if the load
succeeds. Only when that fails does it report that nothing is known by
that name.

Inside the REPL the `describe` *word* and `/describe` meta-command look
up a single word the same way.

### `aql fmt`

Format `.aql` source **in place**. `fmt` takes file paths only — it has
no flags and no stdout/diff mode; every named file that changes is
rewritten and its path is printed. With **no** arguments it walks the
current directory tree and reformats every `.aql` file it finds
(skipping anything under `.aql/`).

```bash
aql fmt script.aql              # rewrite this file in place
aql fmt lib/*.aql               # rewrite several files in place
aql fmt                         # rewrite every .aql file under the cwd
```

### `aql model`

Build a [voxgig](https://github.com/voxgig/model) model: unify a
`.jsonic` model (CUE-style unification, via
[aontu](https://github.com/rjrodger/aontu)) into a single JSON model and
write it to `<base>/<name>.json`. It is the command-line twin of the
`aql:model` language module and mirrors the `@voxgig/model` CLI. Generator
*actions* are Go callbacks that cannot be loaded from the command line, so
this command performs the core job — resolving the model and writing the
JSON — with an optional watch loop.

```bash
aql model model/model.jsonic            # build -> model/model.json
aql model --print model.jsonic          # print the unified JSON to stdout (no file written)
aql model --dryrun model.jsonic         # resolve only; write nothing
aql model --base build model.jsonic     # output + @"..." imports resolve from ./build
aql model --watch model.jsonic          # rebuild on change until Ctrl-C
```

Flags: `--base` (import / output directory; default the model file's
directory), `--watch`, `--dryrun`, `--print`, and `--config` (resolve a
`.model-config` build — off by default). A unification conflict, an
unresolvable reference, or a missing source file exits non-zero with the
error on stderr.


### `aql build`

Compile an AQL program into a **standalone native executable** — a single
file that runs the program without needing the source or a separate `aql`
install. The produced binary runs through the full interpreter, so it
executes *any* program, and prints the residual stack exactly as `aql run`
would.

```bash
aql build prog.aql                  # -> ./prog  (default name = basename)
aql build prog.aql -o tool          # -> ./tool
./prog                              # runs the baked-in program
```

There are two build mechanisms:

| Mechanism | How | Tradeoffs |
|-----------|-----|-----------|
| **Self-embedding launcher** (default) | Copies the running `aql` binary and appends the program as a payload; at startup the copy detects the payload and runs it. | No Go toolchain or network needed; works from a stock `aql`. Binary is ≈ the size of `aql` (it bundles the whole runtime) and targets the host OS/arch only. |
| **Native** (`--native`) | Generates a tiny Go `main` that embeds the program and runs `go build`. | Smaller, dead-code-eliminated binary; cross-compiles via `GOOS`/`GOARCH`. Requires the Go toolchain **and** the aql source/module graph — set `AQL_SRC` to a repo checkout, or run from within the `cmd/go` module. |

**File imports are bundled.** A program that imports other `.aql` files
(`import "./lib.aql"`) is fully self-contained: every transitively-imported
file is embedded and resolved from an in-memory file system at run time, so
the binary works even when the source files are gone. Built-in `aql:`
modules (`import "aql:math-util"`, …) are already part of the runtime and
need no bundling. An import that is neither an `aql:` module nor an explicit
`.aql`/`.lang` file path is rejected at build time.

Flags:

| Flag | Meaning |
|------|---------|
| `-o <path>` | Output binary path. Default: source basename without `.aql` (`prog.aql` → `prog`). |
| `--native` | Use the Go-toolchain path instead of the self-embedding launcher. |
| `--keep` | (native) Retain the generated temp build directory and print its path. |
| `--compile` / `--force-compile` | Bake the experimental bytecode compile-mode into the binary (see [Bytecode compilation](#bytecode-compilation)). `--force-compile` makes the produced binary abort on uncompilable input. |
| `-r <registry>` | Registry path baked into the binary. |
| `-s <seed>` | Random seed baked into the binary. |
| `--options <jsonic>` | Engine [`--options`](#--options--engine-options-as-jsonic) baked in (validated at build time). |

A missing source file, an unbundlable import, or (for `--native`) a failed
`go build` exits non-zero with the error on stderr.


## Project lifecycle

An AQL "project" is a directory with an `aql.jsonic` manifest plus
one or more `.aql` source files. The lifecycle commands operate on
that directory layout.

### `aql prep`

Parse `aql.jsonic` and write `.aql/aql.json` (the resolved manifest
used by downstream tools).

```bash
aql prep                    # current directory
aql prep ./mymodule         # specific directory
```

### `aql pack`

Build a publishable `.zip` of the current module from the resolved
manifest. Output goes under `.aql/`.

```bash
aql pack                    # uses ./aql.jsonic
aql pack ./mymodule
```

### `aql clean`

Delete everything under `.aql/` except dotfiles. A no-op if the
directory doesn't exist.

```bash
aql clean
aql clean ./mymodule
```


## Registry client

Registries are simple HTTP services that host module zips. The
default registry URL is baked into the binary; override with `-r`.

### `aql install`

Download and install a module by versioned name.

```bash
aql install acme/widgets-1.2.3
aql install acme/widgets-1.2.3 -r https://registry.example.com
```

Installed modules become importable as `acme/widgets`.

### `aql register`

Create an account on a registry. Interactive.

```bash
aql register
aql register -r https://registry.example.com
```

### `aql login`

Log in to a registry; stores a token in the local config. With
`--vault`, the token is stored in the (encrypted) vault under an alias
instead of plaintext `~/.aql/user.jsonic` (requires an initialized vault;
set `AQL_VAULT_PASSPHRASE` or be prompted).

```bash
aql login
aql login -r https://registry.example.com
aql login --vault                          # token → vault (alias: aql-registry-token)
aql login --vault --vault-alias=my-reg     # custom alias
```

### `aql publish`

Upload the current module (or a specified directory) to a registry.
Requires a prior `aql login`. When the token was stored with `aql login
--vault`, publish reads it back from the vault automatically; `--vault`
(optionally `--vault-alias=NAME`) forces a vault read.

```bash
aql publish                                # current dir
aql publish ./mymodule
aql publish -r https://registry.example.com
aql publish --vault                        # read the token from the vault
```


## Secrets

### `aql vault`

A local credentials vault, backed by the OS keyring where possible
(macOS Keychain, Linux Secret Service, Windows Credential Manager,
1Password) or a file fallback.

```bash
aql vault -i                            # interactive TUI (menu-driven; keys shown on screen)
aql vault init                          # initialise, pick backend
aql vault status                        # backend, secret count, lock state, generation
aql vault add --from-clipboard github_token   # read from clipboard, then wipe it
aql vault add --from-stdin github_token       # read one line from stdin
aql vault add github_token                     # prompt (input not echoed)
aql vault add --expiry=90d --from-stdin github_token  # optional expiry reminder
aql vault add --ip-whitelist=10.0.0.0/8,203.0.113.7 --from-stdin ci_key  # restrict proxy use to these client IPs/CIDRs
aql vault list                          # aliases and metadata (incl. EXPIRES, IP-WHITELIST)
aql vault get github_token              # redacted by default
aql vault get github_token --reveal     # show the value
aql vault expiry                        # list pending key expiries, soonest first
aql vault expiry --namespace=proj       # filter expiries by namespace
aql vault expiry --within=30d           # only keys due within 30 days (or overdue)
aql vault expiry set github_token 2026-12-31  # set/replace an expiry
aql vault expiry clear github_token     # remove an expiry
aql vault rm github_token               # remove (also: remove, delete)
aql vault mv github_token proj:gh       # rename / move between namespaces (also: rename)
aql vault mv proj: team:                # rename a whole namespace
aql vault rotate --from-stdin github_token         # replace the value (keeps the alias/metadata)
aql vault rotate --revoke-caps --from-stdin github_token  # …and revoke every capability on it (incident response)
aql vault lock                          # block get/grant until unlocked
aql vault unlock                        # re-enable access
aql vault verify                        # reconcile store + keyring (--prune repairs)
aql vault export --out=vault.aqlx       # portable, passphrase-encrypted bundle
aql vault import vault.aqlx             # restore a bundle (or a .env file)
aql vault grant --agent=ci --ttl=2h github_token   # issue scoped capability token
aql vault revoke <token-id>             # revoke a token
aql vault providers                     # list provider presets (built-in + custom)
aql vault provider add --url=https://api.corp.example --auth-style=header:X-Corp-Key corp   # define a custom upstream preset
aql vault provider rm corp              # remove a custom preset (refused while aliases still use it)
aql vault scan .                        # scan files for leaked secret-like strings
aql vault scan --home                   # scan credential dotfiles (~/.npmrc, ~/.netrc, ~/.aws/credentials, …) for plaintext secrets
aql vault scan --home --match-vault     # …and flag which on-disk creds are already vaulted
aql vault audit                         # show the structured audit log
aql vault audit --action proxy.request --last 20
aql vault audit --json                  # raw JSONL
aql vault policy apply policy.aql       # declaratively apply policy
aql vault config                        # show vault config
aql vault config --set namespace.default=proj   # set a config key (also --unset)
aql vault password add --scope=read --namespaces=ci ci-bot  # scoped password (keyslot)
aql vault password add --scope=read --ttl=30m --generate agent  # TEMPORARY password: random, printed once, expires in 30m (hand to an agent)
aql vault password rm --temp            # revoke ALL temporary passwords at once
aql vault password list                 # list keyslots (with scope/namespaces + EXPIRES)
aql vault history                       # content-revision history (newest first)
aql vault restore 7                     # restore metadata to generation 7 (admin)
aql vault proxy                         # run local credential broker (loopback only)
aql vault mcp                           # stdio MCP server over aliases
aql vault exec gh,openai -- mycmd       # run mycmd with secrets in env
aql vault exec --ask GITHUB_TOKEN -- make tag-push   # prompt (echo off) for a value not in the vault, inject as $GITHUB_TOKEN
aql vault exec --ask-passphrase -- make deploy-ts deploy-py  # prompt once, validate, inject AQL_VAULT_PASSPHRASE for nested aql calls
aql vault folder                        # list known vault folders (discovered + recorded by init)
aql vault folder add ~/.othervault      # register an already-existing vault into the index

# Custom location: a folder and/or an inner file-name suffix, by flag or env.
aql vault --folder=/secure/vault --suffix=work init   # vault at /secure/vault/vault.work.*
AQL_VAULT_FOLDER=/secure/vault AQL_VAULT_SUFFIX=work aql vault list
```

#### Mode notes

A few modes that need more than one line:

- **`rotate`** replaces a secret's *value* while keeping the alias and
  its metadata. Read the new value the same ways as `add`
  (`--from-stdin`/`--from-env`/`--from-clipboard`, or an interactive
  prompt). `--revoke-caps` additionally revokes every capability
  scoped to the alias — the incident-response move when a token leaks.
  `--expiry` updates the reminder; omit it to keep the current one.
  `--ip-whitelist` updates the IP allowlist (below); a *present but
  empty* `--ip-whitelist=` clears it, omitting it keeps the current one.

- **`--ip-whitelist`** (on `add` / `rotate`) is an optional per-key
  allowlist of client source IPs or CIDR blocks (comma-separated, e.g.
  `10.0.0.0/8,203.0.113.7`; IPv4 and IPv6). It is **enforced by the
  credential broker** (`vault proxy`): a proxy request for a
  whitelisted key from an IP outside the list is denied `403`. It does
  **not** affect host-side `vault get` / `vault exec` (which have no
  client IP), and only bites once the proxy is bound off-loopback
  (`proxy --allow-public`). An empty whitelist means no restriction.
  Shown in `vault list` (the `IP-WHITELIST` column) and the TUI detail.

- **`lock` / `unlock`** flip a flag in the store that blocks `get` and
  `grant` (and most mutations) without destroying anything — useful
  for an admin handoff or while investigating an incident. The values
  stay encrypted in the backend; `unlock` re-enables access.

- **`config`** reads and writes vault settings. `aql vault config`
  prints them; `--set key=value` / `--unset key` change one. Keys the
  vault reads: `namespace.default` (the namespace bare aliases use),
  `journal.keep` (how many content-history generations to retain), and
  `audit.enabled` (set to `false` to stop writing the audit log).

- **`password`** manages **keyslots** — the scoped passwords that can
  open the vault. `password add --scope=<read|write|move|admin>
  --namespaces=<list> <name>` mints one (a `*` namespace = all, `:` =
  root); `assign` binds it to a user, `set` changes its value, `rm`
  removes it (`--rekey` re-encrypts the namespaces it could reach),
  and `list` shows them. This is how a CI password reads `ci:` secrets
  without being able to touch `prod:`.

  **Temporary passwords** (`--ttl`) are the way to authorize an
  already-running agent (e.g. a Claude session) without handing over the
  master passphrase: `password add --scope=read --ttl=30m --generate
  <name>` mints a **time-boxed** slot with a **randomly generated**
  password printed once. It stops authenticating the moment `--ttl`
  elapses (enforced at every `openSession`), and is never admin-scoped.
  Revoke one early with `password rm <name>`, or pull **every** temporary
  password at once with `password rm --temp`. `list`'s `EXPIRES` column
  shows each slot's expiry (marked `(expired)` once past). The interactive
  TUI (`vault -i` → Passwords) can create a temporary password (set a TTL
  in the Add form), and revoke one (`D`) or all of them (`T`); the
  randomly-`--generate`d form is CLI-only. **Brokers**: `vault proxy` /
  `vault mcp` started under a temporary password refuse to cache the
  session and re-authenticate per request, so they stop serving the moment
  it expires (and warn at startup). See
  [Explanation → the vault](EXPLANATION.md#the-vault-why-a-local-credential-store)
  for the model.

- **`history` / `restore`** expose the append-only content journal.
  `history` lists past generations (`--limit N` for the newest few);
  `restore <generation>` rolls the *metadata* back to one of them
  (admin-scoped; `--list` enumerates restorable points, `--dry-run`
  previews). Secret values in the backend are not rewound.

- **`scan`** matches provider-token *shapes* in file contents
  (defaults to `.`). `--home` instead sweeps the well-known credential
  dotfiles (`~/.npmrc`, `~/.netrc`, `~/.aws/credentials`, `~/.pypirc`,
  `~/.gem/credentials`, …) for plaintext secrets in those tools' own
  formats; `--match-vault` flags which findings are already vaulted;
  `--quiet` prints nothing and relies on the exit code (`2` = found);
  `--max-bytes` skips large files. Env-var references like
  `${NPM_TOKEN}` and obvious placeholders are not flagged.

- **`audit`** prints the structured log. Each event records an
  `action` (e.g. `vault.add`, `vault.exec`, `proxy.request`), the
  `alias`, an `outcome` (`ok`/`error`/`denied`), and a `reason` —
  never a secret value. Filter with `--action`/`--last`, or `--json`
  for raw JSONL.

- **`mcp`** runs a stdio [Model Context Protocol](https://modelcontextprotocol.io)
  server that exposes the vault's aliases to an MCP-aware agent as
  request tools, brokering the real secret server-side exactly like
  `proxy`. `--agent=<name>` sets the identity attributed in the audit
  log and matched against capabilities (default `mcp`). It speaks
  JSON-RPC on stdin/stdout, so it is launched by the agent host, not
  run interactively.

#### Interactive TUI (`aql vault -i`)

`aql vault -i` (or `--interactive`) opens a menu-driven terminal UI for
managing the vault — no need to remember verbs or flags; the valid keys are
always shown in a footer, with `?` for full help and `:` for a command
palette. It covers vault *management* (secrets, capabilities, scoped
passwords, maintenance, config); the runtime commands `proxy`, `mcp`, and
`exec` stay on the command line.

Switching between vaults is built in: the active vault shows in the header,
and `ctrl+o` (or `:vaults`) opens a picker to switch, create, or set a
default — including vaults in custom `--folder` locations, which `init`
records in a small index (`~/.aql/vaults.jsonic`, locations only, never
secrets). `aql vault folder` prints the same list from the command line, and
`aql vault [--suffix=NAME] folder add <dir>` registers a pre-existing vault
(the suffix is auto-detected when the folder holds exactly one).

`aql vault -i --aql` (experimental) runs the TUI's **AQL implementation** —
the same vault driven by an AQL program (the `aql:vault-tui` module) over
the `aql:vault` bridge words, per `design/VAULT-TUI-PORT.0.md`. The
bubbletea TUI stays the default until the AQL port reaches full parity.

The secret value is never taken as a command-line argument — that
would leak it into your shell history and the process listing.
`vault add` (and `vault rotate`) read it from `--from-clipboard`,
`--from-stdin`, `--from-env=VAR`, or, with none of those, an
interactive no-echo prompt. `--from-clipboard` reads the value
straight from the OS clipboard and wipes the clipboard afterwards;
it works on macOS (pbpaste/pbcopy), Linux (wl-clipboard on Wayland,
or xclip / xsel on X11), and Windows (PowerShell).

**Namespaces.** Alias names may carry one namespace qualifier,
`ns:name`, so two projects can each have an `openai_key`
(`proj1:openai_key`, `proj2:openai_key`). The namespace is part of the
alias identity — it flows through capabilities, the proxy URL
(`/proj1:openai_key/v1/...`), export bundles, and the audit log. Set a
default namespace with `vault config --set namespace.default=proj` (or
per-invocation with `AQL_VAULT_NAMESPACE`); bare names then resolve
into it for every command — `vault add key` stores `proj:key`, `vault
get key` reads it back. `:name` (leading colon) forces the root
namespace, and `:` means root anywhere a namespace is named, including
filter values and the env var. There is no silent fallback between
namespaces: a bare name that misses errors (with a hint when a
root-level twin exists). Reporting filters by namespace: `list
--namespace=proj`, `audit --namespace=proj`, `export --namespace=proj`
(`:` = root only), and `status` breaks alias counts down per
namespace. `vault exec proj:key -- cmd` injects the secret as `$key` —
the env name derives from the base name. Policy files take names
literally (no default applied), so a committed policy means the same
thing on every machine.

**Expiries.** A key can carry an optional expiry — a reminder of when
the upstream credential lapses so you can rotate it in time. Set one at
`add` time with `--expiry`, on rotation with `rotate --expiry`, or any
time with `vault expiry set <alias> <when>`; clear it with `vault
expiry clear`. `<when>` is a calendar date (`2026-12-31`), an RFC3339
timestamp, or a duration from now with day support (`90d`, `720h`,
`30d12h`). Expiry is purely informational and **never enforced** — an
expired alias still resolves, since the real key may outlive the
estimate. `vault list` shows an `EXPIRES` column, and `vault expiry`
reports the keys that have one, soonest (and most overdue) first, with
a human status (`in 90d`, `expired 3d ago`). Narrow it with
`--namespace=NS` (`:` = root) or `--within=DURATION` (keys due inside a
window, plus anything already overdue).

**Custom location.** By default the vault lives in `~/.aql` with files
named `vault.<part>` (`vault.jsonic`, `vault.keyring`, `vault.lock`,
`vault.audit.jsonl`). Two knobs override this: `--folder`/`AQL_VAULT_FOLDER`
puts the vault in a folder you choose, and `--suffix`/`AQL_VAULT_SUFFIX`
names the files `vault.<suffix>.<part>` so several vaults can share one
folder without colliding (a flag wins over the matching env var). The
flags are global — place them before the mode (`aql vault --folder=PATH
add …`) — and apply to one invocation, so pass the same folder and
suffix (or export the env vars) to every command that touches that
vault.

The store (`vault.jsonic`) and the secret keyring are separate files,
written one after the other; each write is atomic and fsync-durable
(temp file → fsync → rename → directory fsync), and concurrent writers
— the broker persisting quota counters while you run a command, two
commands at once — are serialized by an advisory lock
(`~/.aql/vault.lock`) held only around each load-modify-save, so no
update is lost across processes. A crash between the two file writes —
or a partial import — can still desync them, so `vault verify`
reconciles them: it reports dangling metadata (an alias with no
secret), orphaned keyring entries (a secret with no metadata, file
backend only), capabilities bound to a vanished alias, and stale
`.tmp` files, exiting non-zero when anything is found; `--prune`
repairs them. Verify runs under the same vault lock as the writers,
so its snapshot is consistent — a half-committed `add` can never show
up as a false orphan — and a repair can never clobber a concurrent
command. The mutating commands are ordered to fail safe (a crash
leaves a harmless orphan, never a dangling reference), so in practice
`verify` should find nothing — it's the audit that proves it.

Rename and move with `vault mv <src> <dst>`: both sides are alias
references (`vault mv key proj:key`), or both denote whole namespaces
with a trailing colon (`vault mv proj: team:`; `:` alone is root, so
`vault mv proj: :` moves everything in `proj` to root). Destinations
are pre-flighted — a bulk rename never half-commits — and the keyring
copy is verified before the old entry is deleted, so a failure can
leave a duplicate but never lose a secret. Capabilities follow the
key by default (the same bearer token then works against the new
proxy path); pass `--revoke-caps` to revoke them instead, and
`--dry-run` to preview. Legacy aliases whose namespace is only a
metadata tag are selected by namespace moves too, gaining properly
qualified names (or just losing the tag when moved to root).

Passphrases follow the same rule: every command that needs one (the
vault passphrase for the file backend, the bundle passphrase for
`export`/`import`) prompts for it interactively with echo suppressed —
no flags or environment setup needed. `AQL_VAULT_PASSPHRASE` and
`AQL_VAULT_EXPORT_PASSPHRASE` are the non-interactive overrides for
services, CI, and stdin pipelines; prefer setting them per-invocation
over `export`-ing them into an interactive shell, where they would
land in shell history and every child process's environment.

`aql vault grant` issues a scoped capability for an alias and prints
a one-time bearer **token**; only the token's hash is stored, so save
it when shown. The credential broker (`aql vault proxy`) authenticates
that token and never accepts a prefix of it, and binds to loopback
only unless you pass `--allow-public`. The MCP server (`aql vault mcp
--agent=NAME`) is gated the same way: it exposes and forwards only
aliases the named agent has been granted a capability for, enforcing
the same TTL, host/method allowlists, and call/cost quotas. The file
backend requires a non-empty passphrase.

Two quota caveats to know when relying on `grant`'s quantitative
limits. **`--max-calls` is a soft cap under concurrency**: the check
runs at request start and the counter persists after the response, so
N simultaneous in-flight requests can overshoot the cap by up to N−1
— use it for budget hygiene, not as a hard rate limiter.
**`--max-cost-cents` is debited only from an `X-AQL-Vault-Cost-Cents`
response header**, which the built-in providers' real APIs do not
send; unless your upstream (or a middlebox you control) sets that
header, the cost meter stays at zero and the budget never trips.

**Upstream providers.** The broker forwards an alias's requests to the
base URL of its provider preset — the compiled-in ones (`openai`,
`anthropic`, `github`) or a **custom preset** minted with
`aql vault provider add --url=<base> [--auth-style=<style>] <name>`
(styles: `bearer`, `x-api-key`, `header:<name>`, `query:<name>`,
`none`). Custom presets live in the vault store, are listed by
`vault providers` with a `SOURCE` column, and are validated at mint
time — URL shape (http/https, a real hostname, a valid port, no embedded
`user:pass@`, no query or fragment), auth style (a `header:<name>` must
be a valid HTTP header name and not one net/http controls itself, such
as `Host`), and preset name; a plain-`http` base URL warns, since the
secret would travel unencrypted. Custom presets referenced by an alias
travel in `vault export` bundles, so a custom-backed alias still brokers
after `vault import`. The broker never follows an upstream redirect —
the injected secret only ever reaches the preset's own host. Built-in names can never be redefined or
removed — a store entry can never redirect a compiled-in provider — and
`provider rm` refuses while any alias still references the preset,
naming the blockers. (Flags precede the name, as with `password add`.)

**Moving a vault between machines or OSes.** A `file`-backend vault is
already portable: copy `~/.aql/vault.jsonic` and `~/.aql/vault.keyring`
and bring the passphrase. Secrets stored in an OS keychain or 1Password
do *not* live under `~/.aql`, so to move those — or to move to a
different OS — use `aql vault export`, which writes a self-describing,
passphrase-encrypted bundle (you are prompted for the bundle
passphrase; `AQL_VAULT_EXPORT_PASSPHRASE` overrides for scripts) of
the aliases and their values, independent of the source backend. `aql vault import` restores it into any backend on the target,
skipping aliases that already exist unless you pass `--overwrite`.
`import` reads a `.env` file or an export bundle, auto-detected. Both
formats are versioned: an older `aql` refuses a newer bundle rather than
mishandling it.

`aql vault exec` resolves the listed aliases against the keyring
and spawns the given command with each value injected as an
environment variable. The child inherits the caller's stdio and
exit code is propagated. The secret value only ever appears in the
child's environment block — never on the command line, never in
the audit log.

```bash
# alias `github_token` becomes $github_token in the child
aql vault exec github_token -- gh repo list

# Remap or uppercase the env names
aql vault exec github_token=GITHUB_TOKEN -- gh repo list
aql vault exec --upper github_token,openai -- ./my-script.sh

# Add a fixed prefix to every derived name
aql vault exec --prefix=APP_ --upper api_key -- ./run.sh   # → $APP_API_KEY

# Sanitize ambient env (keeps PATH/HOME/USER/SHELL/TERM/LANG/LC_ALL/TMPDIR)
aql vault exec --clear-env api_key -- ./hermetic-tool
```

### Publishing a package with a vault-held token

Most package publishers don't read an arbitrary env var — each reads its
credential from a specific variable (or, for npm, a `~/.npmrc` line). The
`--for=<tool>` recipe presents one vault secret in the exact form a
publisher expects, so the token is read from the (encrypted) vault and
exists only in the child's environment — no `~/.npmrc` edit, nothing on
the command line:

```bash
aql vault add --from-stdin npm_token            # store the token once
aql vault exec --for=npm     npm_token   -- npm publish
aql vault exec --for=pnpm    npm_token   -- pnpm publish
aql vault exec --for=cargo   crates_tok  -- cargo publish
aql vault exec --for=pypi    pypi_token  -- twine upload dist/*
aql vault exec --for=poetry  pypi_token  -- poetry publish
aql vault exec --for=github  gh_pat      -- gh release upload v1 dist/*
aql vault exec --for=hackage hackage_key -- stack upload .
aql vault exec --for=terraform tfc_token -- terraform apply

# Scoped / GitHub Packages npm registry (works for npm/pnpm/composer/terraform):
aql vault exec --for=npm --registry=npm.pkg.github.com gh_npm -- npm publish
```

`--for` is **repeatable**, and each entry may name its own secret as
`--for=<tool>=<alias>`, so one child process can carry several tools'
credentials at once — e.g. publish to npm *and* push a GitHub release tag
from a single `make publish`, each token from its own vault secret:

```bash
aql vault exec --for=npm=npm_token --for=github=gh_pat -- make publish
```

Each `--for=<tool>=<alias>` is independent: distinct tokens use distinct
aliases; one token serving two tools just repeats the alias
(`--for=github=gh_pat --for=npm=gh_pat`). The bare `--for=<tool> <alias>`
form (secret from the positional arg) still works for a single tool. If two
recipes would set the same variable to different secrets, exec stops with an
error rather than silently picking one.

Recipes are pure env injection — a secret into the exact variable(s) each
tool reads:

| `--for=` | Tool(s) | Env var(s) |
|---|---|---|
| `npm` · `yarn` · `pnpm` · `bun` | Node | per-registry `_authToken` / `YARN_NPM_AUTH_TOKEN` / `NPM_CONFIG_TOKEN` |
| `pypi`/`twine` · `uv` · `poetry` · `hatch` · `flit` | Python | `TWINE_*` / `UV_PUBLISH_TOKEN` / `POETRY_PYPI_TOKEN_PYPI` / `HATCH_INDEX_*` / `FLIT_*` |
| `cargo` · `gem` · `hex` | Rust/Ruby/Elixir | `CARGO_REGISTRY_TOKEN` / `GEM_HOST_API_KEY` / `HEX_API_KEY` |
| `hackage` | Haskell | `HACKAGE_KEY` (`stack upload` reads it directly; `cabal upload` needs `--token "$HACKAGE_KEY"`) |
| `swift` · `cocoapods`/`pod` | Swift/iOS | `SWIFTPM_REGISTRY_TOKEN` / `COCOAPODS_TRUNK_TOKEN` |
| `composer`/`packagist` | PHP | `COMPOSER_AUTH` (http-basic JSON) |
| `github`/`gh` · `gitlab`/`glab` · `terraform`/`tf` | CLI tokens | `GH_TOKEN`+`GITHUB_TOKEN` / `GITLAB_TOKEN` / `TF_TOKEN_<host>` |

The recipe sets the env-var names, so an alias under `--for` takes no
`=ENV`/`--prefix`/`--upper` remap. A tool that takes its credential
another way — a flag, stdin, or a config file — has no `--for` recipe,
but plain `vault exec` still keeps the secret in the vault; see
[Publishers without a recipe](#publishers-without-a-recipe) below.

To rehearse the plumbing without unlocking the vault, add `--dry-run`: no
passphrase is read and an obviously-fake filler value is injected in place
of the real secret. The aliases are still resolved (a typo is still an
error), so you can confirm the command and env wiring against a publisher's
own dry run:

```bash
aql vault exec --dry-run --for=npm npm_token -- npm publish --dry-run
```

#### Publishers without a recipe

Some tools don't read their credential from an environment variable, so
there's no `--for` recipe for them. The secret still belongs in the
vault — store it once and feed the injected `$tok` to whatever the tool
expects (the alias is `tok` below; `vault exec` puts its value in
`$tok`). Group by how the tool takes the credential:

**As a `--flag`** — wrap in `sh -c` so the shell expands `$tok` onto the
flag (the value can show in `ps`, usually fine in CI):

```bash
# NuGet — dotnet nuget push --api-key
aql vault exec tok -- sh -c \
  'dotnet nuget push pkg.nupkg --api-key "$tok" --source https://api.nuget.org/v3/index.json --skip-duplicate'

# JSR — deno / npx jsr publish --token
aql vault exec tok -- sh -c 'npx jsr publish --token "$tok"'
```

**On stdin** — pipe it in, so it never reaches the argument list:

```bash
# Docker — docker login --password-stdin, then push
aql vault exec tok -- sh -c \
  'printf %s "$tok" | docker login REGISTRY -u USER --password-stdin && docker push REGISTRY/IMAGE:TAG'

# Helm — helm registry login --password-stdin, then push (OCI)
aql vault exec tok -- sh -c \
  'printf %s "$tok" | helm registry login REGISTRY -u USER --password-stdin && helm push chart-0.1.0.tgz oci://REGISTRY/NS'
```

**In an environment variable the tool reads** (just not a standard name)
— remap the alias to that exact variable with `tok=ENV`:

```bash
# Gradle — ORG_GRADLE_PROJECT_<repo>Password (<repo> = the maven { name = } block, case-sensitive)
aql vault exec tok=ORG_GRADLE_PROJECT_mavenPassword -- gradle publish

# Conan 2 — CONAN_PASSWORD, with the (non-secret) username set alongside
CONAN_LOGIN_USERNAME=USER aql vault exec tok=CONAN_PASSWORD -- conan upload pkg/1.0 -r REMOTE -c
```

**In a config file that interpolates an environment variable** — point
the config at a variable, then inject it:

```bash
# Maven — settings.xml <server> with <password>${env.tok}</password>
#   (the <server> id must match the distributionManagement repository id)
aql vault exec tok -- mvn -s settings.xml deploy

# pub.dev — record that the token lives in $tok, then publish
aql vault exec tok -- sh -c 'dart pub token add https://pub.dev --env-var tok && dart pub publish --force'
```

In every case the token is read from the encrypted vault into one child
process and is gone when it exits — never written to `~/.npmrc`,
`~/.docker/config.json`, `settings.xml`, or your shell history. (Flag
and env-var spellings verified against each tool's official docs,
2026-06.)

For AQL's **own** registry, `aql login --vault` stores the registry token
in the vault instead of plaintext `~/.aql/user.jsonic`, and `aql publish`
reads it back automatically — see **[publish](#aql-publish)**.

Inside AQL programs the vault is accessed through the `vault`
capability — see **[Reference §Capabilities](REFERENCE.md#capabilities)**.


## Permissions

### `aql policy`

Inspect and test permission profiles. Most commands accept either a
built-in profile name (`full`, `trusted`, `client`, `read-only`,
`sandbox`, `compute`) or a path to a `.jsonic`/`.json` file.

```bash
aql policy list                              # built-in profile names
aql policy show sandbox                      # pretty-printed JSON
aql policy validate ./my-policy.jsonic       # schema + semantic check
aql policy test sandbox engine.add           # exit 0 = allowed
aql policy explain sandbox fileops.write path=/etc/passwd
# profile:  sandbox
# scope:    fileops
# op:       write
# decision: DENY
# blame:    global.disk.write (rule #1)
```

### Per-command policy flags

Every command that builds a `lang.AQL` accepts these flags:

| Flag | Effect |
|---|---|
| `--perms NAME\|PATH\|JSONIC` | Auto-detected: name, file, or inline. |
| `--perms-file PATH` | Explicit file path. |
| `--perms-inline JSONIC` | Inline jsonic (`@-` = stdin, `@PATH` = file). |
| `--allow scope.op` | Add an allow rule (repeatable). |
| `--deny scope.op` | Add a deny rule (repeatable). |
| `--allow-global OP` | Raise a global hard cap. |
| `--deny-global OP` | Lower a global hard cap. |
| `--no-install scope` | Remove a capability slot entirely. |
| `--install scope` | Force-install (overrides inherited install=false). |
| `--policy-dry-run` | Observe-only (logs but allows). |

Environment fallbacks (consulted when no `--perms*` flag is set):

```bash
AQL_POLICY=sandbox aql do 'add 1 2'
AQL_POLICY_FILE=./prod.jsonic aql script.aql
```

Examples:

```bash
aql do --perms=sandbox add 1 2
aql -e 'add 1 2' --perms=read-only
aql exec -p 8091 --perms=sandbox          # bound at startup; immutable per request
aql do --perms=sandbox --allow=engine.shell true
aql exec --perms=trusted --no-install=network --no-install=sqlite
```

See **[HOWTO §Sandbox untrusted code](HOWTO.md#sandbox-untrusted-code)**
for a walkthrough, and
**[design/PERMISSIONS.10.md](design/PERMISSIONS.10.md)**
for the schema.


## Supervisor control

### `aql ctl`

Drive a running `aql serve` process via its `api` service.

```bash
aql ctl status                          # list services
aql ctl info <service>                  # detail on one
aql ctl pause <service>                 # pause an instance
aql ctl resume <service>                # resume it
aql ctl stop <service>                  # stop and remove
```

Flags:

* `--api URL` — base URL of the api service. Defaults to the
  discovery file written by `aql serve`.
* `--token TOK` — bearer token. Defaults to the discovery file.


## Long-running services

These subcommands run until interrupted. They can all be composed
under one process via `aql serve`.

### `aql repl`

Start the read-eval-print loop. Same surface as plain `aql` with no
arguments — kept as an explicit subcommand for composition.

```bash
aql repl
aql repl -r ./registry
```

### `aql registry`

Serve a directory of module zips over HTTP — the simplest possible
registry.

```bash
aql registry -r ./modules -p 8080
```

* `-r PATH` — registry folder (required).
* `-p PORT` — listen port (default 8080).

### `aql lsp`

Run a Language Server Protocol server.

```bash
aql lsp                     # stdio mode (for IDE integration)
aql lsp -p 9001             # TCP mode
```

* `-p PORT` — TCP port (0 = stdio, the default).

### `aql exec`

Serve AQL code execution over HTTP. POST source to `/v1/exec` and
get back the residual stack; the last value on the stack is exposed
as the top-level `result`. Each request runs in a fresh AQL
instance, so requests are stateless and safe for concurrent use.

```bash
aql exec                                    # bind 127.0.0.1:8091
aql exec -p 8091                            # listen on :8091
aql exec -bind 0.0.0.0:8091 -r ./modules    # custom bind + registry
```

* `-bind HOST:PORT` — interface and port (default `127.0.0.1:8091`).
* `-p PORT` — short form; if non-zero, overrides `-bind`.
* `-r PATH` — registry folder passed to every AQL instance.

Routes:

* `POST /v1/exec` — body `{"code": "..."}`; returns
  `{"result": ..., "stack": [...], "output": "...", "error": "..."}`.
  AQL errors (parse / type / runtime) come back at HTTP 200 with
  `error` set, so clients can distinguish them from transport errors.
* `GET /healthz` — liveness probe.

Example:

```bash
curl -s -X POST http://127.0.0.1:8091/v1/exec \
  -H 'Content-Type: application/json' \
  -d '{"code": "add 1 2"}'
# {"result":3,"stack":[3]}
```

### `aql serve`

Run one or more services in a single process. Services are stacked
with `+` separators. Each service accepts its own flags.

```bash
aql serve repl
aql serve registry -r ./modules -p 8080
aql serve lsp + registry -r ./modules
aql serve api --bind 127.0.0.1:8090 + repl + lsp
```

The `api` service is the control plane; `aql ctl` talks to it.

### `aql tui`

Interactive terminal UI driven by an `api` service.

```bash
aql tui                            # connect via discovery file
aql tui --api http://localhost:8090 --token abc
```

Keys: ↑/↓ move, `p` pause, `r` resume, `x` stop, `q` quit.


## REPL meta-commands

Inside the REPL, lines that begin with `/` are *meta-commands*
(handled by the REPL, not the language):

| Meta-command | Effect |
|--------------|--------|
| `/help` | Print the language overview and the meta-command list |
| `/describe [name]` | Same as the `describe` word — the categorised guide, or one word / category / module |
| `/stack [n]` | Print the current stack (optionally just the top `n` entries) |

Help in the REPL mirrors the CLI, with one substitution: where
[`aql help`](#aql-help) lists the tool's subcommands, the REPL's `/help`
lists the REPL's meta-commands. Everything under
[`aql describe`](#aql-describe) — the categorised index, categories,
`aql:<module>` and `aql:<module>:<word>` — works the same at the prompt,
both as the `describe` *word* and as `/describe`:

```
>> describe                       # categorised guide to words and modules
>> describe add                   # full docs for one word
>> describe math                  # the words in one category
>> /describe aql:type-util:tpartial   # a module word (no quoting needed via /describe)
```

The `describe` and `help` *words* are ordinary AQL, so an argument that
contains punctuation must be quoted: a module reference carries `:`
(`describe "aql:type-util"`), and a dotted namespace export carries `.`
— which is otherwise the `get` operator — so it too is quoted
(`describe "ArrayUtil.indices"`, after `import "aql:array-util"`). The
`/describe` meta-command takes its argument raw, so no quoting is needed
there.

Plain AQL expressions work as usual; exit with Ctrl-D (EOF):

```
>> add 1 2
3
>> /stack
  [0] 3
```


## Exit codes

| Code | Meaning |
|------|---------|
| `0` | Success |
| `1` | A user-facing error (parse, type-check, runtime, I/O) |
| `2` | Usage error (bad flag or missing argument) |

Long-running services (`repl`, `serve`, etc.) exit `0` on a clean
shutdown (`SIGINT`/`SIGTERM`) and `1` on a fatal internal error.
