# `os/exec` → `aql:exec` (`Exec`)

> **Status: design proposal — not implemented (2026-07-16).** This note
> specifies the curated AQL surface for Go's `os/exec` package. No Go
> code exists yet; the note exists so the proposed surface — and
> especially its **policy gating** — is auditable before any handler is
> written. Read [`README.10.md`](README.10.md) first for the shared
> conventions this note assumes. `OS.10.md` §10 and
> `../STDLIB-COVERAGE.10.md` reserved this module by name ("a future
> `aql:exec` with its own `process` gate"); this note is that design.
>
> **Companion:** [`../SIFT.0.md`](../SIFT.0.md) designs `aql:sift`, the
> pure module that parses the semi-structured text this module produces.
> They compose (§4.1) but neither depends on the other.

## 1. Package & status

Go [`os/exec`](https://pkg.go.dev/os/exec) runs child processes.
`aql:exec` curates the **synchronous, captured, argv-vector** slice of
that surface: run one command to completion, capture its output, return
a value. It is the most dangerous capability in the tree — beyond
`Os.exit` (which only terminates the host) — because it runs *arbitrary
other programs* with the host's identity. The design is therefore
policy-first (§7), and the module ships **dead by default** in every
restrictive builtin profile (verified — §7).

## 2. Why curated

The raw `go:` bridge would expose `exec.Cmd` — a mutable struct + method
soup with `([]byte, error)` returns, `LookPath` `(string, error)` pairs,
and shell-footgun ergonomics. The curated module:

- collapses the whole lifecycle to **one word** (`run`) with an options
  Map and a `Result` record;
- makes the argv vector the primary form — **no shell interpretation
  anywhere near the default path**, so quoting injection is impossible
  by construction, and the shell is a separate, separately-gated word;
- treats a **nonzero exit as data, not an error** (`grep`/`diff` exit
  codes are answers), with `{check:true}` to opt into raising;
- routes every effect through a host **capability seam** so sandboxes
  can deny, fake, or script it — specs never run real commands.

## 3. Import & namespace

```
import "aql:exec"          # binds the Exec namespace
```

Namespace is the plain capitalized package name, **`Exec`** — no `-util`
suffix (the convention marks pure helper libraries and builtin-type
clashes; `Exec` clashes with nothing — verified against the module
roster and core words). Words are reached args-before-dot:
`["df" "-P"] Exec.run`.

## 4. API

Signatures are **top-first, sig order**; every word's inner native uses
`BarrierPos: -1` so the swap form dispatches (no zero-arg words in this
module); see `README.10.md` "Argument order & dispatch".

| Go symbol | aql word | signature (top-first) | one-line doc | aql-ish refinement |
|---|---|---|---|---|
| `exec.Command` + `Run` | `run` | `argv:List opts:Map → Result` / `argv:List → Result` | Run an argv vector to completion; capture stdout, stderr, and the exit code. | Argv is a List of Strings with **execve semantics** — no shell, no globbing, no `$VAR`, no word-splitting. Go's `(output, err)` shapes collapse into the `Result` record (§5); a nonzero exit **returns the record** rather than raising, unless `{check:true}`. |
| (`/bin/sh -c`) | `sh` | `cmd:String opts:Map → Result` / `cmd:String → Result` | Run one command string under `/bin/sh -c`. | The explicit shell **opt-in**. A separate word — not a `{shell:true}` option — so the policy op differs (`shell` vs `exec`, §7) and auditing is a grep. POSIX hosts only; raises `exec-unsupported` where `/bin/sh` is absent. |
| `exec.LookPath` | `which` | `name:String → String\|None` | Resolve a command name on PATH, or None. | `(path, error)` collapses to **value-or-None** (the `getenv` refinement, `OS.10.md` §4) — enables graceful degradation before running anything. |
| — (composition) | `parse` | `argv:List opts:Map → Any` / `argv:List → Any` / `kind:Atom/q → Any` | Run a command and parse its output with the matching sift preset. | The sift bridge (§4.1). The kind form (`Exec.parse df` — a bare-word Atom/q capture) runs the preset's canonical pinned argv. |

**Options** (one trailing Map, atom keys — the Options pattern):

| key | default | meaning |
|---|---|---|
| `dir:String` | inherit | working directory of the child |
| `env:Map` | `{}` | environment **overlay** on the inherited environment |
| `inherit:Boolean` | `true` | when `false`, `env` is the child's *entire* environment — one merge rule (an `env` / `env-add` two-key split was rejected as two rules) |
| `stdin:String` | none | text fed to the child's stdin, then closed |
| `timeout:Integer` | 0 (none) | milliseconds; on expiry the child is killed and `exec-timeout` raises (§5) |
| `merge:Boolean` | `false` | merge stderr into `out` (2>&1); `err` is then `""` |
| `check:Boolean` | `false` | raise `exec-nonzero` when the exit code is not 0 |
| `limit:Integer` | 16 MiB | max captured bytes per stream; 0 = unlimited; exceeding raises `exec-output-limit` — **loud, not truncated** (silently truncated output feeding a parser is worse than a failure) |

**Env posture decision.** The default is *inherit* — what every tool user
expects, and the guest has already passed the `process` gate (§7).
Scrub-by-default was rejected: it breaks PATH-dependent tools
immediately and pushes every caller to `inherit:true` boilerplate,
destroying the safety it aimed for. Determinism/hardening hygiene is
delegated to (a) the seam's scripted fake for specs, (b) `Exec.parse`'s
`LC_ALL=C` overlay (§4.1), and (c) a scrubbing `ExecOps` installed by
hardened embedders (§7).

### 4.1 The sift bridge — `Exec.parse`

The bridge word lives **here**, not in `aql:sift` — sift is pure by
scope decision; running commands is exec's business. `Exec.parse`:

1. resolves and policy-checks the argv (op `exec`, §7);
2. overlays `{LC_ALL:"C" LANG:"C"}` on the child env — parse
   determinism on the producing side (`COLUMNS` needs no pinning: piped
   output is not tty-truncated);
3. runs with `check:true` semantics — a failed command's output must
   never silently parse;
4. resolves the preset through the shared host **detection table**
   (`HostFormatDetect` — the same hook `read {fmt:'auto'}` uses,
   `SIFT.0.md` §8.2), keyed by the argv0 basename. **No hard module
   dependency on sift in either direction**: with no sift (or preset
   pack) imported the lookup misses and raises `exec-no-preset` with a
   hint to `import "aql:sift"`;
5. dispatches the kind's parser and returns the parsed value.

The kind form `Exec.parse df` looks up the preset's canonical
`detect.cmd.argv` (`["df" "-P"]`) and runs **exactly that** — the
portability flags live declaratively in the spec, introspectable via
`Sift.spec df`. Magic per-tool flag injection inside exec was rejected:
per-tool knowledge belongs in the spec where it can be read, tested, and
overridden by preset packs.

## 5. Types — the `Result` record

`Exec.Result` is an exported named Record type (so `r is Exec.Result`
works; typed-export precedent: `EXTENSION-MODULES.10.md` §5.2):

```
{ok:Boolean  code:Integer  out:String  err:String  argv:List  ms:Integer}
```

`ok` is `code == 0`; `argv` is carried for provenance (logs, error
messages); `ms` is wall-clock duration.

**Outcome classification (load-bearing):**

| outcome | behaviour | rationale |
|---|---|---|
| started, exited 0 | return record, `ok:true` | — |
| started, exited nonzero | **return record**, `ok:false` | exit codes are data (`grep -c`, `diff`); `{check:true}` upgrades to `exec-nonzero` (detail carries the code and the first stderr line) |
| never started (not found / not executable) | **raise** `exec-start` | the command never ran; there is no meaningful record |
| timeout | **raise** `exec-timeout` (child killed) | a deadline breach is exceptional; folding it into exit codes invites silent misparses |

All raises are ordinary AQL errors, catchable with `do […] error […]` —
errors-as-values is preserved.

**Non-UTF8 output:** v1 decodes lossily (invalid bytes → U+FFFD),
documented. A `{raw:true}` variant returning `Bytes` is **reserved as
the named v1.1 follow-up**, not shipped in v1, to keep `Result`'s field
types stable (`out:String`; a Scalar-typed field was rejected — loose
typing on the 99% path). `Bytes` itself is **shipped** (`Scalar/Bytes`,
`lang/go/native/native_bytes.go`, FixedID 1009) — note
`../STDLIB-COVERAGE.10.md` still says "designed"; the doc lags. No
opaque handle type is introduced and no new FixedID is needed (the
`Result` is a plain record).

## 6. Errors

`r.AqlError(code, detail, word)` with kebab-case codes (the go-modules
family convention, `README.10.md`; the sift module uses snake codes —
each matches its own surface's neighborhood). Bad-type args are guarded
with `RequireConcreteList` / `AsConcreteString` **before** any host call
(ADR-005); every positive `.tsv` row pairs with an `ERROR:` sibling.

| code | raised by | when |
|---|---|---|
| `exec-denied` | any word | the policy denies the op — the `*policy.Denied` (with its blame chain) is surfaced verbatim |
| `exec-not-installed` | any word | the `process` capability scope is `install:false`; the seam stub answers, never a nil deref |
| `exec-bad-argv` | `run`, `parse` | empty list, non-String element, or empty argv0 |
| `exec-start` | `run`, `sh`, `parse` | the child never started (not found, not executable, bad `dir`) |
| `exec-timeout` | `run`, `sh`, `parse` | the `timeout` deadline expired; the child was killed |
| `exec-nonzero` | `run`/`sh` under `{check:true}`; always `parse` | nonzero exit upgraded to an error |
| `exec-output-limit` | `run`, `sh`, `parse` | a stream exceeded `limit` bytes |
| `exec-no-preset` | `parse` | no detection-table entry for the argv0 (hint: `import "aql:sift"` or a preset pack) |
| `exec-unsupported` | `sh` | no `/bin/sh` on this host |

## 7. Policy / capabilities (CRITICAL)

**No handler may call `os/exec` directly.** Every effect routes through
a host capability seam mirroring `FileOps`
(`lang/go/capabilities/capabilities.go` — whose package doc already
reserves this slot: "future host capabilities (network, **process
spawn**, …) go here too").

- **Seam:** `capabilities.ExecOps` —
  `Run(spec ExecSpec) (ExecResult, error)` + `LookPath(name string)
  (string, error)`, with `ExecSpec{Argv, Dir, Env, Stdin, TimeoutMs,
  MergeStderr, Limit}` and `ExecResult{Out, Err []byte, Code int,
  Ms int64}`. Two implementations: `OSExecOps` (real `os/exec` +
  `context.WithTimeout` + kill) and **`ScriptExecOps`** (the `MemFileOps`
  analogue: a scripted argv→result table plus a recorded-calls log) — so
  `lang/spec/module-exec.tsv` is fully deterministic and CI never runs a
  real command.
- **Wiring:** mirrors fileops exactly — a `CapExec` slot,
  `SetHostExec` / `HostExec` accessors, a `permissionedExec` gate
  auto-wrapped when `HostPolicy(r)` is non-nil, and a `notInstalledExec`
  stub when the scope is `install:false` (see
  `native/permissioned_fileops.go`, `notinstalled_fileops.go`).
- **Scope and ops.** Scope **`process`** is already in
  `policy.KnownScopes` (`lang/go/policy/policy.go:117`) and the
  **`process`** global cap is already in `policy.GlobalOps`
  (`policy.go:87`), bound for every op of the scope by `GlobalsFor`
  (`policy.go:153`) — **zero policy-package changes are needed**. The
  scope's op table, beside the actor words' op
  (`../PROCESSES.0.md` §7):

  | op | word(s) | notes |
  |---|---|---|
  | `process.spawn` | core `spawn` (BEAM actors) | in-VM concurrency — not this module |
  | `process.exec` | `run`, `parse` | argv-vector execution |
  | `process.shell` | `sh` | shell-string execution |
  | `process.which` | `which` | PATH resolution (leaks host layout; cheap but not free) |

- **Predicate args.** The gate passes `{argv0, path, dir}` for
  `exec`/`which` rules, where `path` is the **LookPath-resolved**
  binary — resolution happens *before* the check, so an allowlist cannot
  be dodged via `PATH` manipulation. Rule example (same `where` glob
  dialect as fileops paths / env names):

  ```jsonic
  process: { words: { default: "deny", rules: [
    { allow: ["exec"], where: { argv0: ["df", "ps", "uname"] } }
  ] } }
  ```

  **`shell` deliberately carries no useful predicate** — the payload is
  an opaque string. That asymmetry is exactly why `sh` is a separate op:
  an argv0 allowlist is meaningful for `exec` and meaningless for
  `shell`, so a profile can allow curated exec while denying shell
  wholesale. Ship-and-gate rationale (mirroring `OS.10.md`'s `exit`
  reasoning): omitting `sh` was rejected because users would immediately
  reconstruct it as `["sh" "-c" cmd] Exec.run` — which policy can still
  catch as `exec` + argv0 `sh`, but shipping the word keeps the intent
  legible and separately deniable.
- **Default posture (verified in the shipped profiles — no edits
  needed):** dead in `sandbox` / `compute` / `read-only` / `client`
  (the `process` **global** is denied and the `process` scope is
  `install:false` — `lang/go/policy/profiles/sandbox.jsonic:18` and
  `:59`; `compute`/`read-only`/`client` follow), live in `trusted` /
  `full`. `HostPolicy(r) == nil` means ungated (the documented opt-in
  posture, matching fileops today).
- **Blame chains are free.** Denials are `*policy.Denied` carrying
  `{Code, Scope, Op, Profile, Blame, Args}`
  (`lang/go/policy/error.go:19`), so `aql policy explain` and the
  `exec-denied` surface work with no exec-specific code.
- **Env is an injection surface in both directions** (`LD_PRELOAD`,
  `PATH`, locale). The controls, in order: argv0/path `where` rules
  (above), the `Exec.parse` `LC_ALL=C` overlay, and — for hardened
  embedders — installing a scrubbing `ExecOps` whose `Run` rewrites
  `Env` before delegating.

## 8. Overlap

- **BEAM actors (core `spawn`/`send`/`receive`/`register`,
  `lang/go/native/native_process.go` — LIVE core words, not a
  proposal).** In-VM concurrency: goroutines with mailboxes. Totally
  distinct from OS processes — which is why this module's word is
  **`Exec.run`, never `spawn`**, and this note's prose always says
  "OS command" vs "process (actor)". Both gate under the one `process`
  scope with distinct ops (§7).
- **`aql:os` ([`OS.10.md`](OS.10.md), designed).** Env/args/identity of
  *this* process; exec runs *other* programs. `OS.10.md` §10 explicitly
  deferred `os/exec` here.
- **`aql:stream` (`../STREAM-WORDS.0.md`, designed).** Streaming pipes
  between long-running processes are that design's future `stream.exec`
  sketch (its §"future extensions"). This module is deliberately
  synchronous whole-capture; the `ExecSpec` option and `Result` shapes
  are chosen so a future `stream.exec` can reuse them.
- **CLI `aql vault exec`** (`cmd/go/internal/vault/exec.go`). A host
  feature that injects vault secrets into a child's env from the CLI —
  not an AQL word, not this gate. Named here to preempt confusion.
- **Go `os/exec` semantics.** Like Go, no shell interpretation by
  default — the argv vector leaves no injection surface. When `sh` is
  unavoidable and interpolation is involved, quote with
  `StringUtil.escape … {tgt:'sh'}` (`native_string.go`).
- **`aql:sift` ([`../SIFT.0.md`](../SIFT.0.md)).** The consumer of this
  module's output; composed via `Exec.parse` (§4.1) and the shared
  detection table. Sift stays pure; exec stays parse-agnostic.

## 9. Examples (args-before form)

```
import "aql:exec"

# run an argv vector; the Result record is data
def r (["uname" "-srm"] Exec.run)
r.out                                  # returns 'Linux 6.18.5 x86_64\n'
r.ok                                   # returns true

# a nonzero exit is an answer, not an error
(["grep" "-c" "ssd" "/proc/mounts"] Exec.run) .code   # returns 0 or 1

# feed stdin
["sort"] Exec.run {stdin:"b\na\n"}     # returns Result with out 'a'\nb\n'

# start failure raises — catch it as a value
do [ ["no-such-tool"] Exec.run ] error [ ]   # returns error(exec-start)

# the shell is a separate, separately-gated word
"echo $HOME" Exec.sh                   # returns Result — or error(exec-denied)

# probe before running
"df" Exec.which                        # returns '/usr/bin/df' — or None

# with aql:sift imported, acquisition + parsing folds to one word
import "aql:sift"
Exec.parse df                          # runs ["df" "-P"]; returns the parsed Table
```

## 10. Open questions / out of scope

- **Out of scope (v1):** interactive processes / PTY; inter-process
  pipelines (compose via `r.out` → `{stdin:…}`, or wait for
  `aql:stream`); signals and a `kill` word (the `timeout` option is the
  only kill); background/daemon children (`run` is synchronous —
  long-lived concurrency is the actor system); streaming output;
  Windows shell semantics (`sh` is POSIX-only in v1); exposing
  `exec.Cmd` or any handle type.
- **Open:** should `which` gate under a milder op (it leaks host layout
  but executes nothing)? Leaning: keep it under `process.which` so a
  profile can allow probing while denying execution — the reverse is
  never wanted.
- **Open:** `sh`'s shell path — hardcode `/bin/sh` or consult `SHELL`?
  Leaning: hardcode `/bin/sh` (deterministic, POSIX-contract); `SHELL`
  is a user preference, not a script contract.
- **Open:** default `limit` (16 MiB chosen) — right order of magnitude
  for command output while bounding a runaway `yes`? Revisit with use.

## 11. Implementation sketch (wiring checklist — no code)

Follow `io.go` (the capability-backed reference) and `OS.10.md` §11's
worked checklist:

1. **Seam.** `ExecOps` + `OSExecOps` + `ScriptExecOps` in
   `lang/go/capabilities/capabilities.go` (the package doc already
   points here).
2. **Wiring.** `CapExec` key, `SetHostExec` / `HostExec` accessors,
   `permissionedExec` gate (auto-wrap under `HostPolicy`),
   `notInstalledExec` stub — mirroring the fileops trio.
3. **Policy.** No package changes (`process` scope + global already
   exist and bind, §7); add the op names to the gate call sites only.
4. **Module.** `lang/go/modules/exec.go` — `BuildExecModule`: handlers
   call `HostExec(r)`, never `os/exec`; declare the `Exec.Result`
   record type at build; inner sigs `BarrierPos: -1`.
5. **Registry.** `"exec": BuildExecModule` in
   `lang/go/modules/modules.go`; catalog row in
   `help/help_render.go`'s `moduleCatalog`.
6. **Docs.** `docs_exec.go` one-liners (the `TestModuleExportDocs`
   ratchet).
7. **Spec.** `lang/spec/module-exec.tsv` driven entirely by
   `ScriptExecOps` (deterministic rows; CI runs no real commands) +
   deny-policy rows for `exec-denied` + no-panic rows (ADR-005) +
   `ERROR:` siblings throughout (ADR-003); 100% coverage via the
   scripted seam (ADR-008).
8. **Doc index.** Roster row in [`README.10.md`](README.10.md); flip
   `os/exec` from bucket C to bucket A in `../STDLIB-COVERAGE.10.md`.
