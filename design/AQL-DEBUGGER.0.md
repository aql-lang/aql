# AQL-DEBUGGER — a first-class interactive `aql debug` debugger

Status: **Living reference — Phase 1 SHIPPED (2026-07); later phases
are design.** This note designs the interactive `aql debug <file.aql>`
debugger: launch a program, set breakpoints, single-step, inspect the
stack / scope / call chain, and evaluate expressions at a pause. It is
grounded in seams that already exist in the tree (verified against the
engine, the `aql:debug` module, and the CLI as of 2026-07) and it is
deliberately staged so a small, honest v1 ships on those seams before
any kernel work.

> **Shipped — Phase 1** (`cmd/go/internal/debugger` +
> `debugcmd.runLaunch`; see §10 for the phase definition): `aql debug
> [--script F] [--no-check] [--color M] <file.aql> [args...]` with
> source-line-coalesced `step`, `continue`, `quit` (detach-and-drain),
> inline `Debug.break` / `break-when` stops, `stack`, `bt` (live inline
> frames from the tape's `FrameOpenInfo` marks), `defs`, `print <expr>`
> (child-engine eval), `list`, and `help`; a drained command input
> (EOF / `--script`) detaches. The one interface widening shipped as
> designed but with stdlib field types: `StepFrame` gained `Row`, `Col
> int` and `File string` (not an `eng.SrcPos` — the capabilities
> package stays engine-agnostic), populated by `runStepped` (token
> position + `Registry.BaseFile`) and `pauseAtBreak` (file only; a
> one-shot pause carries no position). Deltas from the design as
> written: the prompt is `(adbg) `; the debugger session doubles as the
> installed `DebugOps`, so a `Debug.step` run *by the program* prompts
> the same session (re-entrant pauses during `print` evaluation are
> suppressed, never nested); policy gating of the launch surface is
> deferred with open Q4 (§13) — no perms flags in Phase 1; and the
> scripted/CI front end is the same prompt loop reading a file, with
> command echo. Tests: `debugcmd_launch_test.go` (scripted end-to-end
> transcripts — the §11 discipline, incl. quit-drains and coalescing
> negatives) + `debugger_test.go` (defensive arms).

It **reconciles and supersedes `DEBUG-MODULE.0.md §6**` (the "engine
seams" section), whose `DebugSession` struct, `StepOver` action, and
panic-based quit were *designed but never built* — see §12. The
`aql:debug` module itself (printing, introspection, perf, and the
in-process `Debug.step`/`break` words) is real and is the substrate this
debugger drives; this note is about the missing **front end**.

The inspiration is Tim Misiak's *Writing a Debugger From Scratch*
(timdbg.com). Its central lesson is the one this design is built on:

> **A debugger is an event loop.** Launch or attach a target, wait for
> events (a breakpoint, a single step, a fault), inspect and modify
> state, then continue. Everything else — symbols, stacks, disassembly —
> hangs off that loop.

AQL already *has* that loop. The engine fires a callback before every
step. What's missing is a host that stops on it and talks to a human.


## 1. The mental model — the article, mapped to AQL

The article builds a native x64 debugger on the Win32 debug API. AQL is
an interpreted concatenative stack language on a Go engine, so the
*substrate* differs entirely — but the *primitives* map one-to-one, and
several are cheaper here than on bare metal.

| Article (native x64) | AQL equivalent | Status in-tree |
|----------------------|----------------|----------------|
| Part 1 — attach to / launch a process | `aql debug <file>` (launch); `aql debug attach` (already exists via `debugserve`) | launch: **missing** |
| Part 2 — register state & stepping (`GetThreadContext`, trap flag) | the **data stack** + `pointer` are the "registers"; step = one engine step | seam **present** (`SetTrace`) |
| Part 3 — reading memory (`ReadProcessMemory`) | read AQL values on the stack, in scope, in stores | present (`CurrentStack`, `Defs`) |
| Part 4 — exports & private symbols | the registry's live word + def tables | present (`Debug.words`/`defs`) |
| Part 5 — breakpoints (INT3 `0xCC`) | `Debug.break` markers; source `file:line`; word/pattern | markers **present**; line/word **missing** |
| Part 6 — stacks (unwind) | the spliced fn-frame chain on the tape | raw material **present** (`fn_frame.go`) |
| Part 7 — disassembly | `Debug.disasm` — the StackForm "bytecode" view | present |
| Part 8 — source & symbols | `SrcPos{Row,Col}` on every value → `file:line` | present (per §7) |

Two things the article's native target *cannot* cheaply do, but AQL can,
because execution is a **deterministic step machine** with a whole-stack
snapshot available at every step:

- **Break-on-error / post-mortem** — stop *at the token that raised*,
  with the stack and scope intact (§6.1). Nearly free here.
- **Time-travel / step-backward** — re-render or replay past steps
  (§6.4). Cheap to prototype, exploiting determinism.


## 2. What already exists — the empty socket

The single most important finding: **the engine and capability machinery
a debugger needs is already in the tree, wired and tested — but it has no
production caller.** `Debug.step` in the shipping binary only *prints* a
trace, and `Debug.break` is a silent no-op, purely because nothing ever
installs a `StepController`.

### 2.1 The per-step pause seam (the "debug event loop")

`eng/go/engine.go`, the Run loop, fires once before each tape token is
dispatched:

```go
val := e.tape.At(e.pointer)
e.registry.noteCoverage(val.Pos())      // line-coverage hook — reads the token's SrcPos
if e.trace != nil {
    snapshot := e.tape.Snapshot()        // the WHOLE tape: resolved stack + pending tokens
    e.trace(step, e.pointer, snapshot, e.traceNote)
}
```

- `Engine.SetTrace(TraceCallback)` — `TraceCallback` is
  `func(step, pointer int, stack []Value, note string)`. Zero cost when
  unarmed; **the callback may block** for interactive input (it runs
  synchronously on the engine goroutine). This is the event loop.
- `snapshot[pointer]` is the token *about to* execute; `snapshot[0:pointer]`
  is the resolved data stack. Everything a pause needs is in that one call.
- The coverage hook two lines above (`val.Pos()`) is the precedent for
  line breakpoints: the same `SrcPos` a breakpoint index keys on is
  already read here every step.

### 2.2 The host-controller contract (engine-agnostic)

`lang/go/capabilities/capabilities.go`:

```go
type StepAction int
const ( StepInto StepAction = iota; StepContinue; StepQuit )

type StepFrame struct {          // stdlib-only, pre-rendered — package stays engine-agnostic
    Step    int
    Pointer int
    Stack   []string             // the tape, rendered one entry per slot
    AtBreak bool                 // paused on a Debug.break marker
}

type StepController interface { OnStep(frame StepFrame) StepAction }
type DebugOps interface        { Controller() StepController }
```

- Installed with `native.SetHostDebugOps(r, ops)`, read with
  `native.EffectiveDebugOps(r)` (capability key `CapDebugOps`).
- **No production caller today** — the socket is clean and empty. A test
  `scriptedController` proves it drives correctly; the TTY host does not
  exist yet.

### 2.3 The reference driver

`lang/go/modules/debug_step.go::runStepped(parent, tokens)` is a *complete*
working stepper the CLI debugger generalises:

```go
sub := native.New(parent)                 // child engine (aql:vm isolation model)
single := true                            // pause at the first step
detached := false                         // set by StepQuit
sub.SetTrace(func(step, pointer int, stack []Value, _ string) {
    if detached { return }
    atBreak := isBreakAt(stack, pointer)  // stack[pointer] is the debug-break marker word?
    if !single && !atBreak { return }     // fast path: "continue" pays one word-compare
    frame := capabilities.StepFrame{ Step: step, Pointer: pointer,
        Stack: renderTape(stack), AtBreak: atBreak }
    switch ops.Controller().OnStep(frame) {
    case StepInto:     single = true
    case StepContinue: single = false
    case StepQuit:     detached = true    // detach-and-drain; NOT preemption (see §5.4)
    }
})
res, err := sub.Run(tokens)
```

Plus `Registry.CurrentStack()` (a marker-filtered read-only snapshot of
the innermost engine's data stack) and `pauseAtBreak(r)` (the one-shot
`Debug.break` pause). The CLI debugger runs a *whole file's* top-level
tokens through this shape instead of a single quoted body.

### 2.4 Source positions, frames, attach

- **Positions.** Every parsed `Value` carries `SrcPos{Row, Col, Src}`
  (`eng/go/aql_error.go:127`; `Row`/`Col` 1-based, `0` = unknown).
  `Value.Pos()` / `Engine.currentPos()` map the pointer to a position.
  **There is no file field and no byte offset** — the file is resolved
  per-registry from `Registry.BaseFile`. The compiled VM carries the
  parallel `Program.Debug` / `CompiledFn.Debug` line tables.
- **Frames.** `fn_frame.go` marks each spliced fn body with
  `FrameOpenInfo{Meta *FnFrameMeta, ArgSpan int}` on its open paren;
  `IsFrameOpen` / `AsFrameOpen` / `FnFrameMeta{Name, InstallNames}` and
  `unwindLiveFrames` let a host enumerate the *inline* call frames on the
  tape. `Registry` also keeps a stack of all nested running engines
  (`pushEngine`/`popEngine`) — the backbone for a cross-sub-engine
  backtrace.
- **Attach.** `lang/go/debugserve` + `aql debug serve|attach` already
  serve authenticated HTTP introspection (`words`/`defs`/`heap`/`eval`)
  and an invocation-keyed event relay, with a `$TMPDIR/aql-debug.json`
  discovery file and Bearer auth (static token or vault capability id).
  It does **no stepping** yet.

### 2.5 The CLI dispatch point

`cmd/go/internal/debugcmd/debugcmd.go::cmdImpl.Run` is a flat dispatcher:

```go
switch args[0] {
case "serve":  return runServe(args[1:], …)
case "attach": return runAttach(args[1:], …)
default:       // "unknown subcommand" — rejects a bare file positional today
}
```

The interactive debugger slots in as a **third branch** — when `args[0]`
is neither `serve` nor `attach`, treat it as a file to launch (§4).


## 3. Design principles

1. **Ride existing seams; add engine code only when a feature truly
   needs it.** v1 is host wiring — no kernel change. Each later phase
   names the exact engine work it requires and why.
2. **One session, many front ends.** All step/breakpoint logic lives in
   a single host-side session object implementing `StepController`. The
   TTY prompt is one thin front end over it; a DAP adapter (§8) is a
   second. The two can never diverge on semantics because they share the
   session.
3. **Honest about the granularity floor.** You pause *between AQL
   tokens*, never inside a native handler. v1 is *blind inside
   sub-engines* (module fns, `each`/`fold`/`do`, forks); the UI labels
   those steps as stepped-over rather than pretending to descend.
4. **Production-safe by construction.** `Debug.break` left in committed
   code is inert when no session is attached (already true). The
   debugger is a capability gated by policy (§9); a program that never
   runs under it pays nothing and is granted nothing.
5. **Deterministic-first.** Prefer the reproducible signal (step index,
   step count) over wall-clock, mirroring `Debug.steps`. This is what
   makes scripted/CI debugging (§6.3) and time-travel (§6.4) tractable.
6. **No control-flow panics.** Per ADR-005, "quit" **detaches** the
   stepper and lets the run drain; it does not unwind the engine. The
   design never assumes a mid-Run interrupt that the kernel does not
   offer (§5.4).


## 4. The command surface

```
aql debug <file.aql> [flags]     # launch the file under the interactive debugger  (NEW — this note)
aql debug serve  [file.aql] …    # existing: expose introspection over HTTP
aql debug attach <verb> …        # existing: interrogate a running `serve`
```

Launch flow (host code, no engine change):

1. Add the file branch to `debugcmd.cmdImpl.Run`: if `args[0]` ∉
   {`serve`,`attach`}, parse it as a launch (`aql debug prog.aql`).
2. Read + preflight the file exactly as `aql run` does (expand path,
   `os.ReadFile`, `check.PreflightColor`) so the debugger honours the
   same check / compile / policy flags.
3. **Force interpreter mode** (as `Debug.step` does today — the compiled
   VM path is not stepped in v1, §5.5), install a host `DebugOps` whose
   `Controller()` is the interactive session (§5), then run the file's
   top-level tokens under `SetTrace`.

Proposed launch flags: `--break <spec>` (repeatable; pre-set a
breakpoint, §7), `--break-on-error` (§6.1), `--script <file>` (batch
mode, §6.3), `--stack-depth N` (render cap), plus the shared
check/policy flags.


## 5. The core — a host-side debug session

A single new host type (place it in a new `cmd/go/internal/debugger`
package, or alongside `debugcmd`) is the realisation of the
*designed-but-never-built* `DebugSession` — now as **host code**, not an
engine struct. It implements `capabilities.StepController.OnStep` and
owns everything:

```
type session struct {
    // control state
    mode        stepMode          // Into | Over | Out | Continue | Detached
    // breakpoints
    lines       map[loc]bool      // file:line breakpoints (§7)
    words       map[string]bool   // word/pattern breakpoints (§7)
    // frame tracking for over/out (§5.2)
    baseDepth   int
    // inspection context
    lastFrame   StepFrame
    reg         *native.Registry  // for eval / stack / defs at a pause
    // ui front end
    ui          frontEnd          // TTY prompt (v1) or DAP adapter (§8)
}
func (s *session) OnStep(f StepFrame) StepAction { … }
```

`OnStep` decides *whether this step is a stop*, and if so, renders the
pause and asks the front end for the next action. Non-stops return the
current action (`Into`/`Continue`) with no I/O — the §2.3 fast path.

### 5.1 What a "step" is, and why the raw one is too fine

One engine step is one **tape-token dispatch**. A single source line like
`print add 1 2` spans *many* token steps (a forward marker, per-arg
collection, a force-stack, the dispatch). Exposing the raw step as the
user's "step" would be unusably noisy. So the session coalesces:

- **`step` (into)** — run until `Pos().Row` changes (i.e. advance to the
  next *source line*), descending into inlined fn frames as it goes.
- **`next` / `over`** — like `step`, but auto-continue through any fn
  frame opened *deeper* than the current one on the way.
- **`out`** — auto-continue until the current fn frame's close paren.
- **`continue`** — run to the next breakpoint (or completion).
- **`quit`** — detach (§5.4).

### 5.2 Synthesising `over` / `out` from frame depth

The engine's `StepAction` is only `Into`/`Continue`/`Quit` — there is no
native "over" or "out". The session *synthesises* them by tracking
fn-frame depth on the tape and auto-continuing until depth returns to the
baseline:

- Depth is read from the spliced-frame markers: count live `IsFrameOpen`
  parens at/below the pointer (via `FrameOpenInfo` / `unwindLiveFrames`'s
  scan), or use the pointer/tape-length delta as a cheaper proxy.
- `over`: record `baseDepth` at the stop; while stepping, if depth >
  baseDepth, return `Into` without prompting (silent descent) until depth
  ≤ baseDepth and the row advanced, then stop.
- `out`: same, but stop only when depth < baseDepth.

**This works for inlined (same-registry) frames** — named fns, recursion,
closures. It does **not** cross into sub-engine calls (module fns,
higher-order bodies, forks), which the trace does not reach in v1. Those
are stepped *over* unconditionally and the UI says so
(`… stepping over <module fn>`). Closing that blindness is the headline
Phase 2 engine item (§10).

### 5.3 The backtrace

At a pause the session builds a call stack from two sources:

1. **Inline frames** — scan the snapshot for `IsFrameOpen` parens below
   the pointer; each yields `FnFrameMeta.Name`, its `ArgSpan` args, and
   `InstallNames` (the frame's params/captures) → one backtrace row with
   locals.
2. **Sub-engine frames** — the registry's nested-engine stack
   (`pushEngine`/`popEngine`) gives the outer frames the tape scan can't
   see. In v1 these self-identify weakly (no per-frame name yet); Phase 2
   stamps them (§10) so a full cross-engine `bt` names every frame.

### 5.4 `quit` is detach, not preemption

Per ADR-005 the engine has no mid-Run interrupt and control-flow panics
are forbidden. `StepQuit` therefore sets `detached = true`: the stepper
stops pausing and the program **runs to completion** uninterrupted. This
is the conventional debugger *detach*, and the UI must say so ("detached
— program continues") rather than implying the run was killed. A true
"terminate now" is a separate engine capability (a cooperative
cancellation seam), tracked as a Phase-2+ item and a candidate NUR record
if it lands (§10, §13).

### 5.5 Interpreter-forced in v1

Like `Debug.step`, the debugger forces the **interpreter** path: the
compiled VM does not fire the per-token trace. Line breakpoints on the
compiled path (via the `Program.Debug` line tables and a `noteVMCoverage`
-style hook) are a deferred item, gated on real demand for
stepping-while-compiled (§7 option D, §10).


## 6. Signature features

These four are what make the AQL debugger more than a port of the
article. **Break-on-error is prioritised for an early phase** (it is
nearly free and the highest-value); the others are designed here and
sequenced in §10.

### 6.1 Break-on-error / post-mortem  *(priority)*

Stop **at the token that raised**, before the error unwinds — AQL's
answer to `pdb.post_mortem` / `gdb`'s stop-on-signal. Two entry points:

- **`--break-on-error`** on launch: when a step is about to dispatch a
  word that raises `[aql/…]`, or when `Run` returns an error, pause with
  the stack and scope still intact and drop into the prompt so the user
  can inspect `stack`, `bt`, `defs`, and `eval`.
- **Post-mortem** `aql debug --post-mortem <file>`: run normally; only on
  an *uncaught* error, replay to the faulting step (determinism makes
  this exact) and open the prompt there.

Mechanism: the session already sees every step; it watches for the
error. The cleanest hook is to pause on the step *whose dispatch returns
an error* — which needs a small engine affordance to hand the error to
the trace before the loop returns it (a "fault note" on the existing
`traceNote` channel, or an error-aware trace variant). That is the one
non-trivial engine touch this feature needs; it is small and bounded.

### 6.2 Data watchpoints

`watch <name>` — pause when a def's binding changes value, the
data-breakpoint analogue of hardware watchpoints. Maps onto the existing
`Debug.watch` idea: the session snapshots `Registry.Defs.Top(name)` and,
each step, compares; on change it stops and shows old → new. Cost is one
lookup per watched name per step (only while watches are set).

### 6.3 Scripted / batch mode for CI

`aql debug --script cmds.dbg <file>` drives the session from a **command
script** instead of a TTY, reusing the already-tested `scriptedController`
shape. Because the engine is deterministic, a scripted session is fully
reproducible — so debugger behaviour itself is unit-testable and a team
can check in a repro script. This is also how the debugger satisfies the
repo's headless-test discipline (§11): every interactive path is
exercised by a scripted front end with no TTY.

### 6.4 Time-travel / step-backward  *(designed; later phase)*

Execution is a deterministic sequence of steps and the trace hands out a
whole-tape snapshot each step, so *stepping backward* is unusually cheap
to approximate:

- **Phase-3 form (bounded, read-only).** Keep a ring of the last *N*
  step snapshots. `back` re-renders the previous frame from the ring — a
  pure *re-render* of already-captured state, **no engine mutation**.
  `replay` re-runs from the start to a chosen step (deterministic, so it
  reproduces exactly). Memory is bounded by the ring; nothing in the
  engine changes.
- **Full time-travel (out of scope for now).** Restoring the engine's
  tape/pointer/registry to an arbitrary past step needs a global
  monotonic step clock spanning sub-engines, sub-engine trace
  propagation (§10), and a state-restore seam the kernel lacks — a
  substantial project. The determinism that makes it *possible* is
  recorded here so the opportunity isn't lost; it is not committed.


## 7. Breakpoints

v1 keeps the free, production-safe **inline markers** and adds a **live
`file:line` registry**, **conditional** breaks, and **word/pattern**
breaks — all set from the prompt.

| Kind | Prompt syntax | Mechanism | Phase |
|------|---------------|-----------|-------|
| Inline marker | `Debug.break` in source | `isBreakAt` matches the marker word at the pointer (exists) | 1 |
| Conditional | `Debug.break-when <cond>` / `break … if <expr>` | evaluate predicate at the marker / at the line (exists for the marker form) | 1 |
| **Source line** | `break app.aql:42` | index `{file,row}`; on each step compare `snapshot[pointer].Pos().Row` (+ registry file) against the index; **coalesce** so one source line = one stop | 2 |
| **Word / pattern** | `break add` | pause when the word about to dispatch is `add` (or matches a glob) | 2 |

Notes:

- **Line breakpoints need step coalescing** — sugar (dot-chains, forward
  collection) expands one line into many token steps, and some synthetic
  tokens are position-less (`Row == 0`); the session stops once per
  contiguous run of a given `Row`, skipping position-less tokens. The
  data is all present (`val.Pos()`), so **no engine change** — this is
  the same `SrcPos` the coverage hook at `engine.go:1283` already reads.
- **File identity comes from the executing registry**, not the value
  (`SrcPos` has no file). The session resolves the current file from
  `Registry.BaseFile`; multi-file / imports sessions need the file
  threaded onto the step frame (§8 / §12 additive `StepFrame` widening).
- **Word breakpoints** are cheap and *idiomatic for concatenative code*
  ("stop whenever `fold` runs") — a bonus the article's model doesn't
  have. They cost one name compare per step.


## 8. Inspection at a pause & the UI

### 8.1 The prompt (v1 — TTY, gdb/pdb-style)

Reuse the REPL's readline + meta-command shape
(`repl.startWithPauseGate` + `NewMetaRegistry`), but with a debugger
command set instead of the language prompt. At a stop the session prints
the current source line (from `SrcPos`), the top of the data stack, and a
prompt:

| Command | Does |
|---------|------|
| `step` / `s` | step into (next source line, §5.1) |
| `next` / `n` | step over deeper frames |
| `out` / `o` | run to end of current frame |
| `continue` / `c` | run to next breakpoint |
| `break <spec>` / `b` | set a breakpoint (§7); `break` alone lists them |
| `delete <id>` | clear a breakpoint |
| `stack` | the data stack at this point (`CurrentStack`) |
| `bt` | the call chain (§5.3) |
| `frame <n>` | select a frame; `defs`/`eval` then act in its scope |
| `defs` / `locals` | bindings in scope (`Defs`, frame `InstallNames`) |
| `print <expr>` / `p` | evaluate AQL against the paused state (reuse the `debugserve` sub-engine eval pattern) |
| `watch <name>` | data watchpoint (§6.2) |
| `list` | source around the current line |
| `back` / `replay` | time-travel (§6.4, later phase) |
| `quit` / `q` | detach (§5.4) |

`print`/`eval` runs the expression in a child engine over the paused
registry — the same isolation `Debug.trace`/`aql:vm` use, so an
evaluated expression can never grant itself more than the program had.

### 8.2 DAP-ready architecture (editors — later phase)

Editor users expect the **Debug Adapter Protocol** (`setBreakpoints`,
`stackTrace`, `scopes`, `variables`, `next`/`stepOut`, `pause`,
`terminate`). AQL already ships an **LSP** whose JSON-RPC transport
(`cmd/go/internal/lsp/jsonrpc.go`, `Content-Length` framing) is ~90%
reusable for a DAP server, and it would slot in under `aql serve` as an
LSP sibling.

The design commits only to *not foreclosing* it: the session (§5) is the
single source of step/breakpoint truth, and the TTY prompt is one front
end over it. A DAP adapter is a **second** front end mapping DAP requests
to the same session — so the two never diverge. Crucially, every DAP core
request that lacks an engine primitive today (`pause`, `terminate`,
compiled-path line breakpoints) is exactly what the TTY v1 also can't do,
so a DAP v1 would be no more constrained than the session already is.
DAP is Phase 3 (§10).

### 8.3 Local now, attach later

v1 is **local launch** — `aql debug <file>` runs the file in-process
under `SetTrace`. Debugging a *running* process (`debugserve` attach) is a
different substrate: the HTTP surface today is introspection/eval only,
with no step/break/continue verb and a poll-only event channel. Extending
it with a stepping protocol (a `/debug/step` control channel streaming
`StepFrame`s over the invocation-keyed event ring, preserving the
Bearer/loopback/`$TMPDIR` triad) reuses the *same session* behind the
handlers. It is a real protocol project, gated on demand, and sequenced
after local launch (§10, Phase 4).


## 9. Policy & safety

Reuse the `debug` policy scope proposed in `DEBUG-MODULE.0.md §8` — no
new policy machinery, just scope names:

- `debug.step` gates the interactive stepper / breakpoints / blocking for
  input (`aql debug <file>` needs it).
- `debug.introspect` gates reading the registry (`stack`/`defs`/`bt`).
- `debug.print` gates output.
- `debug.remote` / `debug.serve` gate the attach direction (§8.3), and
  compose with the vault capability scopes (`debug.observe/inspect/
  control`) already used by `debugserve`.

The `sandbox` profile denies `debug.step` (no interactive blocking).
Expression evaluation at a pause runs in a child engine composed **under
the program's policy** (the `aql:vm` model), so `print <expr>` can never
exceed what the program itself was granted. `--break-on-error` /
post-mortem read state that the program already had; they add no grant.


## 10. Engine changes, by phase

The whole point of the staging is to be explicit about where host wiring
ends and kernel work begins.

- **Phase 1 — host only, ~zero engine change.** `aql debug <file>`;
  single-step (source-line coalesced); `continue`; `quit` (detach);
  inline `Debug.break`/`break-when` stops; `stack`/`bt`(inline)/`defs`/
  `print`; scripted/CI front end (§6.3). Everything rides `SetTrace`,
  `SetHostDebugOps`, `CurrentStack`, `Pos()`, and `fn_frame.go`. The one
  *additive* interface widening: carry `SrcPos` + the executing
  registry's resolved **file** on `StepFrame` (§12) so stepping is
  source-aware and multi-file-correct — additive, per the codebase's
  contract-only-grows discipline.
- **Phase 1.5 — break-on-error (§6.1).** A small engine affordance to
  surface a raising step's error to the trace before the loop returns it
  (a fault note on the existing `traceNote` channel). High value, bounded.
- **Phase 2 — fidelity (the big one).** (a) `over`/`out` via frame-depth
  tracking (host, using `fn_frame.go`); (b) **thread the trace/session
  into sub-engines** so `into`/`over`/`out` cross module-fn / higher-
  order / fork boundaries — fire the callback in `New`/`NewTop` children
  and at `CallAQL`/`RunResolved`/the paren-group evaluators. This is the
  single biggest fidelity gap and the highest-value engine investment;
  (c) stamp sub-engine frames so `bt` names every frame (§5.3); (d)
  `file:line` + `word` breakpoints (§7).
- **Phase 3 — DAP + time-travel.** A DAP adapter as a *second* front end
  over the session (§8.2); bounded step-back / replay (§6.4).
- **Phase 4 — remote/attach stepping.** Extend `debugserve` with a
  stepping control channel over the existing transport (§8.3).
- **Beyond — cooperative cancellation.** A real "terminate now" seam so
  `quit` can stop a run mid-flight without a panic (§5.4). Collides with
  ADR-005's model and would want an NUR record; not committed.


## 11. Test discipline

Per `lang/go/CLAUDE.md` — pair every positive with a negative, and cover
100% of reachable statements (ADR-008 `cover-gate`):

- Drive **every interactive path headlessly** with a scripted front end
  (§6.3) — no TTY, no real stdin. Assert the exact action sequence
  (`step`, `step`, `continue`, `quit`) produces the exact stops.
- **Breakpoints:** a positive row (stops at `app.aql:42`) *and* a
  negative (does **not** stop at a position-less synthetic token, does
  **not** stop twice on one source line — the coalescing contract).
- **`quit` detaches, not kills:** assert the program's side effects
  *after* a mid-run `quit` are the full-run effects (§5.4).
- **`Debug.break` with no session** is a pure no-op (production safety) —
  already tested; keep it.
- **Policy refusal:** `aql debug` under a denying `debug.step` policy
  fails with the documented error and leaks no state.
- **Break-on-error:** a raising program stops at the faulting token
  (positive) and a *caught* error does **not** trigger the stop
  (negative).
- **Determinism:** the same launch + same script yields byte-identical
  transcripts across runs.
- Follow the package-level **test seams** (`newReadline`, file read,
  `os.Exit`) the CLI already uses so `cover-gate` stays satisfiable.

Spec-style rows for the underlying `Debug.*` words stay in
`lang/spec/module-debug.tsv`; host-level behaviour (scripted stepping,
policy refusal, break-on-error, detach semantics) lives in Go tests
beside `debugcmd`.


## 12. Relationship to `DEBUG-MODULE.0.md`

`DEBUG-MODULE.0.md` remains the design of the **`aql:debug` module**
(printing, introspection, perf, memory, and the in-process
`Debug.step`/`break` words) — all shipped and real. This note **owns the
interactive `aql debug` CLI debugger**, which that doc left as an open
question (its §11 Q3: "first-class `aql debug <file>` subcommand, or
REPL-only?" — answered here: **yes, CLI first, DAP-ready**).

It **supersedes `DEBUG-MODULE.0.md §6`** ("Architecture & engine seams")
where that section is now stale — those pieces were designed but never
built, and this note replaces them with what shipped:

| `DEBUG-MODULE.0.md §6` (as written) | Reality / this note |
|-------------------------------------|---------------------|
| a `DebugSession` **engine** struct owning the hook | no such struct; the hook is driven by `runStepped`; the session becomes **host** code (§5) |
| `StepAction` includes `StepOver` | shipped actions are `StepInto`/`StepContinue`/`StepQuit` only; `over`/`out` are **host-synthesised** (§5.2) |
| `StepQuit` preempts via `panic(stepQuit{})` | panics are forbidden (ADR-005); `StepQuit` **detaches** (§5.4) |
| a fat `DebugOps{MemStats, ForceGC, Controller}` | shipped `DebugOps` is `{Controller()}`; heap/gc took a different path |
| `Debug.stack` needs a new Run-loop field write | shipped as `Registry.CurrentStack()` (the "current engine" variant) |

`AQL-DX-REPORT-DEBUG.0.md` is a historical DX report and stays as-is.


## 13. Open questions for the maintainer

1. **Break-on-error engine hook (§6.1).** The one early-phase engine
   touch is surfacing a raising step's error to the trace. Prefer (a) a
   "fault note" reusing the existing `traceNote` string channel, or (b) a
   dedicated error-aware trace variant? (Leaning (a) — smaller.)
2. **`StepFrame` widening (§12).** Add `Pos SrcPos` + `File string` to
   `StepFrame` now (needed for any source-aware / multi-file stepping),
   or keep `StepFrame` stdlib-string-only and carry position out-of-band?
   (Leaning: add them — additive, and file-identity is genuinely needed.)
3. **CLI vs REPL entry.** Ship `aql debug <file>` only, or also a
   `/debug` REPL meta-command that steps the current line? (Leaning:
   CLI-first; REPL meta-command is a cheap follow-on.)
4. **Word/pattern breakpoints (§7).** Worth shipping in Phase 2, or is
   `file:line` + markers enough and word breakpoints a niche? (Leaning:
   ship — cheap and concatenative-idiomatic.)
5. **Cooperative cancellation (§5.4, §10).** Is `quit`-as-detach
   acceptable indefinitely, or is a real mid-run terminate a requirement?
   The latter is an engine project touching ADR-005 and would open an NUR
   record.
6. **Doc placement.** Keep this as a standalone `design/` note, or also
   add a one-line entry to `design/README.md`'s index and a user-facing
   section to `CLI.md` once Phase 1 ships? (No index/`CLI.md` edit made
   yet — Phase-1 landing is the natural trigger.)

No ADR entry is proposed — per repo policy (`lang/go/CLAUDE.md`, `ADR.md`
header) this stays a `design/` note until a maintainer says otherwise.
This change adds a design note only and does not alter the repository's
tracked structure/tooling or the knowledge-graph's module/doc set, so no
`kg/` update is required.
