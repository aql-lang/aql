# AQL CLI Reference

The `aql` binary bundles the language runtime, the REPL, a static
type-checker, a code formatter, a module-packaging toolchain, a
registry client, a local key vault, a Language Server, and a
service supervisor. This document describes every subcommand it
supports.

## Contents

* [Quick start](#quick-start)
* [General usage](#general-usage)
* [Language execution](#language-execution)
  * [`aql` / `aql run`](#aql--aql-run)
  * [`aql do`](#aql-do)
  * [`aql check`](#aql-check)
  * [`aql help`](#aql-help)
  * [`aql describe`](#aql-describe)
  * [`aql fmt`](#aql-fmt)
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

### `aql do`

Evaluate the remaining args as an AQL expression. Slightly more
shell-friendly than `aql -e` because positional words don't need
extra quoting.

```bash
aql do add 1 2                  # prints 3
aql do '"aql:string-util" import end "hello" StringUtil.upper'  # prints HELLO
aql do 'iota 5 each [dup mul]'  # prints [0 1 4 9 16]
```

If the expression **begins** with a negative number, the leading
`-N` is parsed as an unknown command-line flag
(`flag provided but not defined: -7`). Separate the flags from the
expression with `--`:

```bash
aql do -- '-7 0 add'           # leading negative literal needs --   (prints -7)
```

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
```

Flags:

* `-e EXPR` — type-check an inline expression.
* `--json` — emit JSON diagnostics.
* `--soft` — return exit code 0 even when diagnostics are reported.
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

**Pre-flight a run.** `aql run --check` (or the short `aql --check`)
runs the checker first and aborts before executing if any error is
found, so a type bug can't slip into a run; stdout stays clean for the
program's own output:

```bash
aql --check script.aql       # check, then run; abort on any error
aql --check -e 'add 1 2'     # one-shot, checked first
```

**In your editor.** `aql lsp` publishes these same diagnostics as you
type — see [`aql lsp`](#aql-lsp).

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
aql vault init                          # initialise, pick backend
aql vault add --from-clipboard github_token   # read from clipboard, then wipe it
aql vault add --from-stdin github_token       # read one line from stdin
aql vault add github_token                     # prompt (input not echoed)
aql vault add --expiry=90d --from-stdin github_token  # optional expiry reminder
aql vault list                          # aliases and metadata (incl. EXPIRES)
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
aql vault verify                        # reconcile store + keyring (--prune repairs)
aql vault export --out=vault.aqlx       # portable, passphrase-encrypted bundle
aql vault import vault.aqlx             # restore a bundle (or a .env file)
aql vault grant --agent=ci --ttl=2h github_token   # issue scoped capability token
aql vault revoke <token-id>             # revoke a token
aql vault providers                     # list built-in provider presets
aql vault scan .                        # scan files for leaked secrets
aql vault audit                         # show the structured audit log
aql vault audit --action proxy.request --last 20
aql vault audit --json                  # raw JSONL
aql vault policy apply policy.aql       # declaratively apply policy
aql vault proxy                         # run local credential broker (loopback only)
aql vault mcp                           # stdio MCP server over aliases
aql vault exec gh,openai -- mycmd       # run mycmd with secrets in env

# Custom location: a folder and/or an inner file-name suffix, by flag or env.
aql vault --folder=/secure/vault --suffix=work init   # vault at /secure/vault/vault.work.*
AQL_VAULT_FOLDER=/secure/vault AQL_VAULT_SUFFIX=work aql vault list
```

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
aql vault exec --for=npm    npm_token   -- npm publish
aql vault exec --for=cargo  crates_tok  -- cargo publish
aql vault exec --for=gem    rubygems    -- gem push pkg.gem
aql vault exec --for=pypi   pypi_token  -- twine upload dist/*
aql vault exec --for=uv     pypi_token  -- uv publish

# Scoped / GitHub Packages npm registry:
aql vault exec --for=npm --registry=npm.pkg.github.com gh_npm -- npm publish
```

Recipes (`npm`, `cargo`, `gem`, `pypi`/`twine`, `uv`) are pure env
injection; file-only publishers like docker and maven are not yet
covered. The recipe sets the env-var names, so `--for` takes a single
alias (no `=ENV`/`--prefix`/`--upper`).

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
(`describe "ArrayUtil.indices"`, after `"aql:array-util" import`). The
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
