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
go install github.com/aql-lang/aql/cmd/go/aql@latest
aql -version
aql                                 # start the REPL
aql do '1 add 2'                    # one-shot expression
aql script.aql                      # run a file
aql check script.aql                # type-check without running
aql fmt -w script.aql               # format in place
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
| `-version` | Print the version and exit. |


## Language execution

### `aql` / `aql run`

Execute a script, an `-e` expression, or drop into the REPL when
nothing is supplied.

```bash
aql                         # REPL
aql -e '1 add 2'            # prints "3"
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
aql do 1 add 2                  # prints "3"
aql do '"hello" upper'          # prints 'HELLO'
aql do 'iota 5 each [dup mul]'  # prints '[0,1,4,9,16]'
```

### `aql check`

Run the static type-checker without executing. Reports diagnostics
to stderr; exit code 1 if any are found.

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

### `aql help`

List the available words, or describe one.

```bash
aql help                    # full word list
aql help add                # signature and example for `add`
aql help fn
aql help record
```

Inside the REPL the `help` *word* is also available — typing
`help` at the prompt does the same thing.

### `aql fmt`

Format `.aql` source. With `-w`, rewrite in place; otherwise print
to stdout.

```bash
aql fmt script.aql              # print formatted source
aql fmt -w script.aql           # rewrite in place
aql fmt -w lib/*.aql            # multiple files
aql fmt < input.aql             # stdin → stdout
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

Log in to a registry; stores a token in the local config.

```bash
aql login
aql login -r https://registry.example.com
```

### `aql publish`

Upload the current module (or a specified directory) to a registry.
Requires a prior `aql login`.

```bash
aql publish                                # current dir
aql publish ./mymodule
aql publish -r https://registry.example.com
```


## Secrets

### `aql vault`

A local credentials vault, backed by the OS keyring where possible
(macOS Keychain, Linux Secret Service, Windows Credential Manager,
1Password) or a file fallback.

```bash
aql vault init                          # initialise, pick backend
aql vault add github_token 'ghp_xxx'    # store a secret
aql vault list                          # aliases and metadata
aql vault get github_token              # redacted by default
aql vault get github_token --reveal     # show the value
aql vault rm github_token               # remove (also: remove, delete)
aql vault grant github_token <pid>      # issue scoped capability token
aql vault revoke <token-id>             # revoke a token
aql vault providers                     # list built-in provider presets
aql vault scan .                        # scan files for leaked secrets
aql vault audit                         # show the structured audit log
aql vault audit --action proxy.request --last 20
aql vault audit --json                  # raw JSONL
aql vault policy apply policy.aql       # declaratively apply policy
aql vault proxy                         # run local credential broker
aql vault mcp                           # stdio MCP server over aliases
aql vault exec gh,openai -- mycmd       # run mycmd with secrets in env
```

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

Inside AQL programs the vault is accessed through the `vault`
capability — see **[Reference §Capabilities](REFERENCE.md#capabilities)**.


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
  -d '{"code": "1 add 2"}'
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

Inside the REPL, lines that begin with `:` are *meta-commands*
(handled by the REPL, not the language):

| Meta-command | Effect |
|--------------|--------|
| `:help` | Print meta-command list |
| `:stack` | Print the current stack with indices |
| `:drop` | Drop the top of stack |
| `:clear` | Clear the stack |
| `:reset` | Reset the engine (clear stack and definitions) |
| `:trace on` | Enable per-expression tracing |
| `:trace off` | Disable tracing |
| `:check on` | Run the type-checker before each evaluation |
| `:check off` | Disable inline type-checking |
| `:load PATH` | Read and evaluate a file |
| `:save PATH` | Save the session's history to a file |
| `:quit` | Exit the REPL |

Plain AQL expressions work as usual:

```
aql> 1 add 2
3
aql> :stack
  [0] 3
aql> :drop
aql> :quit
```


## Exit codes

| Code | Meaning |
|------|---------|
| `0` | Success |
| `1` | A user-facing error (parse, type-check, runtime, I/O) |
| `2` | Usage error (bad flag or missing argument) |

Long-running services (`repl`, `serve`, etc.) exit `0` on a clean
shutdown (`SIGINT`/`SIGTERM`) and `1` on a fatal internal error.
