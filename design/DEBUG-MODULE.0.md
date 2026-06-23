# aql:debug — a debugging & introspection module

Status: **proposal** (discovery note, not yet built). This captures the
design of a new native module `aql:debug` (namespace `Debug`) that
collects AQL's debugging affordances behind one import: simple printing,
interactive stepping, and structural / memory / performance analysis of
both a program and the running system.

Per `lang/go/CLAUDE.md` this is a *framework / capability* module, so the
id stays plain (`aql:debug`, not `-util`) and the namespace is `Debug`.
Nothing here is built yet — the file exists so the surface, the engine
seams it needs, the policy story, and the phasing are agreed before code
lands.


## 1. Why a module (and why not core)

The pieces of "debugging" already exist in scattered, partial forms:

- `print` (core) writes a value and **consumes** it — no passthrough, so
  you cannot drop it into the middle of a pipeline without restructuring
  the code.
- `IO.trace` (`aql:io`) runs a quoted body in a sub-engine and prints the
  full step-by-step stack evolution. This is the single richest debug
  affordance today, and it is buried in the I/O module.
- `Vm.parse` (`aql:vm`) returns the parsed token/value list without
  running it — a structural view, but framed as a VM concern.
- `inspect`, `typeof`, `is`, `describe`, `help` (core) answer
  per-value / per-word structural questions.
- `aql:report` pretty-prints records / tables / lists.

These were each added where they were first needed, not where a user
*looks* for them. A developer who wants to debug should reach for one
import — `import "aql:debug"` — and find printing, tracing, stepping, and
introspection together, composed and consistent. The module **reuses**
the underlying machinery (it does not fork it): `Debug.trace` shares
`eng.RunTrace`, `Debug.parse` shares the parser path `Vm.parse` uses,
the pretty-printers delegate to `aql:report`, and the structural words
delegate to the existing `inspect` / `typeof` / `describe` algorithms.

These words stay where they are for back-compat; `Debug.*` is the curated
front door, and a couple of genuinely new capabilities (interactive
stepping, profiling, memory stats) live only here.

Keeping it a module also means **zero cost when unused** — debugging
touches the host (stdout, the clock, the Go runtime, interactive input)
and pokes at the registry, so it is naturally a *capability* gated by
policy. A program that never imports `aql:debug` pays nothing and is
granted nothing.


## 2. Design principles

1. **Value-returning where possible; effects are explicit.** A debug
   word that can return data does (so it composes), and the few words
   that print do so as a deliberate, labelled side effect. The signature
   "print *and pass the value through*" (`Debug.tap`) is the workhorse —
   it is the gap `print` leaves.
2. **Pure analysis is pure.** Parsing source, sizing an in-memory value,
   inspecting a word's signature, or disassembling a body needs no host
   capability and is allowed even under restrictive policies. Only the
   *effectful* surfaces (writing output, reading the clock, sampling Go
   memstats, blocking for interactive input) require the `debug` scope.
3. **Forward-form canonical.** Every example is written `Debug.word a b`
   (see `eng/go/CLAUDE.md` "Surface form recommendation"). Inner natives
   registered into the sub-registry use `BarrierPos: -1` so the dotted
   swap form dispatches (the module-wrapper rule in `lang/go/CLAUDE.md`).
4. **Build on existing seams, add none gratuitously.** The engine already
   exposes a per-step `TraceCallback` (`Engine.trace`), a `Recorder`
   (StackForm), `EffectiveClock`, the `inspect`/`typeof` algorithms, and
   the `vm` sub-engine. The module is mostly a *curation and packaging*
   layer over these. The one new engine-side concept is a `DebugSession`
   (§6) that owns the step hook for stepping and profiling.
5. **Sub-engine isolation, parent attenuation.** Words that *run* user
   code (`Debug.trace`, `Debug.time`, `Debug.bench`, `Debug.step`) run it
   in a sub-engine composed under the parent policy, exactly as `aql:vm`
   does — a debug word can never grant the traced code more than the
   parent already had.


## 3. The five surfaces

Below, each surface lists its proposed words with signatures in
forward form. `~>` denotes "returns". Types in `Capitalised` are AQL
lattice types; new module-owned types are defined in §5.

### (A) Printing & tracing — "what is flowing through here?"

| Word | Signature | Behaviour |
|------|-----------|-----------|
| `Debug.tap` | `Any ~> Any` | Print the value (formatted), then **return it unchanged**. The pipeline tap `print` can't be. `(compute) Debug.tap further-process`. |
| `Debug.label` | `Any String ~> Any` | Like `tap` but prefixes a label: `x "after-map" Debug.label`. Prints `after-map: <value>`, returns `x`. |
| `Debug.dump` | `Any ~> Any` | Like `tap` but prints the *full* `inspect` view (type, structure, provenance) rather than the short form. Returns the value. |
| `Debug.watch` | `Atom/q ~> None` | Print the current binding of a def each time control passes the word: `x/q Debug.watch`. Resolves the name in the live def stack. |
| `Debug.trace` | `List ~> Any` | Run a quoted body in a sub-engine, printing the step-by-step stack evolution. Delegates to `eng.RunTrace` (the engine behind `IO.trace`); re-exported here as the discoverable home. |
| `Debug.assert` | `Boolean String ~> None` | If the boolean is false, raise `[aql/assertion_failure]` with the message; else no-op. (Distinct from `aql:test`'s `assert.*`, which are suite-scoped.) |
| `Debug.todo` | `String ~> Never` | Always raises `[aql/not_implemented]` with the message. A typed hole that the checker treats as `Never` so it unifies anywhere. |

`Debug.tap` / `Debug.label` are the headline ergonomics win: printf-style
debugging that doesn't force you to break a concatenative pipeline apart.

### (B) Interactive stepping — "walk me through execution"

The engine's `Engine.trace` hook already fires once per step with
`(step, pointer, stack, note)`. `Debug.trace` consumes that
non-interactively. Stepping makes it **interactive**: install a hook that,
at each step (or each breakpoint), pauses and consults a controller for
the next action (step / continue / step-over / inspect / quit).

| Word | Signature | Behaviour |
|------|-----------|-----------|
| `Debug.step` | `List ~> Any` | Run the body in a sub-engine under interactive control. At each step, prints the current stack + pointer and prompts (via the host `DebugOps` controller, §6) for `n`(ext) / `c`(ontinue) / `s`(tack) / `q`(uit). Returns the body's residual top-of-stack. |
| `Debug.break` | `None ~> None` | A breakpoint marker. When `Debug.step` (or a stepping session) reaches it, control pauses regardless of step/continue state. A bare no-op when no debug session is active, so leaving `Debug.break` in code is harmless in production. |
| `Debug.break-when` | `Boolean ~> None` | Conditional breakpoint: pauses only if the boolean is true. |
| `Debug.run-stepped` | `String ~> Any` | Parse + step a source string (the REPL / CLI entry point), rather than a pre-quoted body. |

Non-interactive hosts (CI, wasm playground, tests) supply a *scripted*
controller — a pre-recorded list of actions — so stepping is testable
without a TTY. The REPL supplies a line-reading controller; a future
`aql debug <file>` CLI subcommand (out of scope here, noted in §8) would
supply a richer one.

### (C) Program structural analysis — "what is this code/word made of?"

| Word | Signature | Behaviour |
|------|-----------|-----------|
| `Debug.parse` | `String ~> List` | Parse source to the quoted token/value list without running it. Shares the path behind `Vm.parse`; re-exported as the natural home for "show me the AST". |
| `Debug.disasm` | `List ~> List` | Compile a quoted body to its StackForm op list (the `Recorder` output) and return it as inspectable data — the "bytecode view". |
| `Debug.sig` | `Atom/q ~> List` | The signatures of a word, as structured data (params, returns, barrier position) — the machine-readable form of what `describe` renders. |
| `Debug.body` | `Atom/q ~> Any` | For an AQL-defined fn, the quoted body tokens; for a native, a descriptor noting it is host-implemented. |
| `Debug.deps` | `List ~> List` | The set of word names a quoted body references (via the body-walker `WalkBodyWords` already used for closure capture). Useful for "what does this touch?". |
| `Debug.explain` | `Atom/q ~> String` | The full `describe` text for a word, returned as a String (so it can be embedded, not just printed). |

These are pure — no policy needed — because they read source, the
registry's static word table, or an in-memory value.

### (D) System structural analysis — "what is the running system?"

| Word | Signature | Behaviour |
|------|-----------|-----------|
| `Debug.words` | `None ~> List` | Every registered word name (the registry's native + def word table). |
| `Debug.defs` | `None ~> Map` | Current value/type bindings in scope (`r.Defs.Names()` → top binding each), as a Map. |
| `Debug.modules` | `None ~> List` | Imported modules with their id / kind / exports (from the Module descriptors). |
| `Debug.types` | `None ~> List` | The type lattice: every minted/builtin type with name, rank, and parent — a dump of `r.Types`. |
| `Debug.stack` | `None ~> List` | A **snapshot of the current data stack** at the call site, as a list, without consuming it. The REPL `/stack` meta-command as a word. |
| `Debug.scope` | `None ~> Map` | The active scope chain: params, captures, and local defs of the enclosing fn frames. |

`Debug.stack` is subtle — a word cannot normally see "the whole stack"
because dispatch only hands it its matched args. It is implemented as an
engine-assisted primitive (§6): the handler receives the live engine's
residual stack via a privileged path, copies it, and pushes the copy
back. This is the one place the module reads engine-internal state that
no existing word exposes.

### (E) Memory analysis — "what does this cost to hold?"

| Word | Signature | Behaviour |
|------|-----------|-----------|
| `Debug.sizeof` | `Any ~> Integer` | Estimated retained byte size of an AQL value (deep walk of lists/maps/strings/payloads). Pure — walks the value graph in-process. |
| `Debug.shape` | `Any ~> Map` | Structural census of a value: counts by kind (ints, strings, lists, maps), max depth, total node count. Pure. |
| `Debug.heap` | `None ~> Map` | Go-runtime heap stats (`runtime.MemStats`: alloc, total-alloc, heap-objects, num-GC) as a Map. **Host capability** — only meaningful with a real runtime; gated under `debug`. |
| `Debug.gc` | `None ~> Map` | Force a GC and return before/after heap deltas. Capability-gated; useful for "is this leaking?". |

`Debug.sizeof` / `Debug.shape` are pure value analysis (no host).
`Debug.heap` / `Debug.gc` read the Go runtime and so are effectful
capabilities behind a `DebugOps` seam (so wasm / sandboxed hosts can stub
or refuse them).

### (F) Performance analysis — "where does the time/work go?"

| Word | Signature | Behaviour |
|------|-----------|-----------|
| `Debug.time` | `List ~> Map` | Run the body once, return `{result, elapsed-ms, steps}`. Wall-clock via `EffectiveClock` (so a `FixedClock` makes it deterministic in tests); step count from the engine loop. |
| `Debug.bench` | `List Integer ~> Map` | Run the body N times, return `{n, total-ms, mean-ms, min-ms, max-ms, steps-per-run}`. |
| `Debug.steps` | `List ~> Integer` | Run the body, return only the number of engine steps executed (cost in "instructions", clock-independent — the deterministic perf metric). |
| `Debug.profile` | `List ~> Table` | Run the body under the step hook, accumulating per-word step counts (and, if the clock is real, per-word time), returned as a Table sorted by cost. The concatenative analogue of a sampling profiler. |

`Debug.steps` is the *deterministic* performance signal — it does not
depend on the wall clock, so it is reproducible in CI and is the right
thing to assert on in regression tests ("this refactor must not increase
step count"). `Debug.time` / `Debug.bench` add the wall-clock dimension
for real tuning.


## 4. Composition examples

```
import "aql:debug"

# (A) printf-debugging without breaking the pipeline
[1 2 3 4] (each [dup mul]) Debug.tap (fold add 0) Debug.label "sum"
#   prints:  [1 4 9 16]
#   prints:  sum: 30
#   result:  30

# (C/D) what is this word, and what is in scope?
add/q Debug.sig            # ~> structured signatures of `add`
Debug.words                # ~> every registered word
Debug.stack                # ~> snapshot of the current data stack

# (F) is my refactor cheaper?
[expensive-thing] Debug.steps      # ~> 1842   (deterministic)
[expensive-thing] 1000 Debug.bench # ~> {n:1000 mean-ms:0.7 ...}

# (E) how big is this in memory?
big-table Debug.sizeof     # ~> 48211
big-table Debug.shape      # ~> {maps:300 strings:1200 max-depth:4 ...}

# (B) walk it interactively (REPL / debug CLI)
[tricky-pipeline] Debug.step
```


## 5. Module-owned types

Registered into the sub-registry per import (module-scoped, *not* global
builtins — they have no FixedID and reach the user via exports, exactly
like `aql:io`'s `StreamKind`). Reach them as `Debug.TypeName`.

| Type | Shape | Used by |
|------|-------|---------|
| `Timing` | `refine Record [result:Any elapsed-ms:Float steps:Integer]` | `Debug.time` |
| `BenchResult` | `refine Record [n:Integer total-ms:Float mean-ms:Float min-ms:Float max-ms:Float steps-per-run:Integer]` | `Debug.bench` |
| `ProfileRow` | `refine Record [word:String calls:Integer steps:Integer ms:Float]` | `Debug.profile` (Table of these) |
| `MemStat` | `refine Record [alloc:Integer total-alloc:Integer heap-objects:Integer num-gc:Integer]` | `Debug.heap` / `Debug.gc` |
| `StepRecord` | `refine Record [step:Integer pointer:Integer word:String stack-depth:Integer]` | stepping / profiling hook payloads |

Returning structured records (rather than ad-hoc maps) lets callers pipe
straight into `report.table`, assert with `aql:test`, or `getpath` into
fields with type safety.


## 6. Architecture & engine seams

### What already exists (reuse, don't rebuild)

- **`Engine.trace TraceCallback`** — `func(step, pointer int, stack []Value, note string)`, set on a sub-engine and driven by the Run loop. `eng.RunTrace` already builds on it. This is the foundation for `Debug.trace` (non-interactive), `Debug.step` (interactive), `Debug.steps`/`Debug.profile` (counting).
- **`Engine.recorder Recorder`** (StackForm) — the compiled op stream behind `Debug.disasm`.
- **`EffectiveClock(r)`** — the time source behind `Debug.time`/`Debug.bench`; honours an installed `FixedClock` for deterministic tests.
- **`inspect` / `typeof` / `describe` / `WalkBodyWords`** — the algorithms behind surface (C).
- **The `aql:vm` sub-engine pattern** (`runInSubEngine`, policy composition) — the isolation model every code-running debug word follows.

### What is new

1. **`DebugSession`** (lives in `eng/go`, alongside `trace.go`) — a small
   struct that owns an interactive/scripted step controller and the
   per-word accumulators for profiling. `Debug.step` / `Debug.profile`
   install a `DebugSession`-backed `TraceCallback` on the sub-engine.
   The controller is an interface:

   ```go
   type StepController interface {
       // OnStep is called at each engine step (or each breakpoint hit);
       // it returns the next action (Step, Continue, StepOver, Quit).
       OnStep(rec StepRecord, stack []Value) StepAction
   }
   ```

   Implementations: `lineController` (REPL/TTY), `scriptedController`
   (a fixed action list — used by tests and non-interactive hosts),
   `noopController` (production: `Debug.break` becomes a no-op).

2. **`DebugOps` host capability** (lang `capabilities/`, alongside
   `FileOps` / `Clock`) — the seam for the genuinely host-bound surfaces:

   ```go
   type DebugOps interface {
       MemStats() MemStat        // Debug.heap / Debug.gc
       ForceGC()                 // Debug.gc
       Controller() StepController // interactive stepping source
   }
   ```

   Default OS-backed impl reads `runtime.MemStats` and wires a TTY
   controller; the wasm / sandbox impl stubs MemStats to zeroes and
   supplies a no-op controller, so `aql:debug` *loads* everywhere but its
   effectful words degrade gracefully (and are policy-refusable).

3. **`Debug.stack` privileged read** — a single engine-assisted native
   that receives the live engine's residual stack. The cleanest seam is a
   new handler variant (or a `CallableSpec`-style flag) that asks the Run
   loop to pass a *copy* of the current stack into the handler. This is
   the only engine-internal exposure the module adds; it is read-only
   (the copy is pushed back, the live stack is untouched).

### File layout (mirrors existing modules)

```
lang/go/modules/debug.go          # BuildDebugModule + sub-registry wiring
lang/go/modules/debug_test.go     # word-level tests (incl. negative rows)
lang/go/modules/docs_debug.go     # registerDocs("aql:debug", {...})
lang/go/capabilities/debug.go     # DebugOps interface + OS / stub impls
eng/go/debug_session.go           # DebugSession, StepController, profiling accumulators
lang/spec/debug.tsv               # executable spec rows (positive + negative)
design/DEBUG-MODULE.0.md          # this file
```

`BuildDebugModule` follows the `BuildIOModule` shape: a sub-registry,
inner natives with `BarrierPos: -1`, FnDef wrappers via the
`makeModuleFnDef` helper, module-scoped types via a `Mint…` call,
exported under the `"Debug"` namespace; registered in the `modules` map
in `modules.go` as `"debug": BuildDebugModule`; docs in `docs_debug.go`.

### FixedID range

Only the *global* externally-registered types need FixedIDs. The
`Debug.*` types above are **module-scoped** (minted per import, no
FixedID — like `IO.StreamKind`), so no FixedID allocation is required and
the `fixedid_stability_test.go` snapshot is untouched. Should any debug
type ever need to be wire-stable, draw from the reserved
`5000-9999` band per `eng/go/CLAUDE.md`.


## 7. Policy & safety

A new policy scope **`debug`** governs the effectful surfaces:

| Scope op | Gates |
|----------|-------|
| `debug.print` | `Debug.tap` / `label` / `dump` / `watch` / `trace` (host Output writer) |
| `debug.step` | `Debug.step` / `break*` / `run-stepped` (blocks for input) |
| `debug.introspect` | `Debug.words` / `defs` / `modules` / `types` / `stack` / `scope` (reads registry) |
| `debug.runtime` | `Debug.heap` / `gc` (reads Go runtime) |

The **pure** words (`Debug.parse`, `disasm`, `sig`, `body`, `deps`,
`explain`, `sizeof`, `shape`, `steps`, `time`, `bench`, `assert`, `todo`)
need no scope — they read source / in-memory values / the static word
table and a clock that is itself already a capability. Code-running words
(`trace`, `step`, `time`, `bench`, `profile`) compose the child engine
under the parent policy (the `aql:vm` model), so traced code can never
exceed the parent's grants.

Default profiles: `sandbox` denies `debug.runtime` and `debug.step`
(no host runtime, no interactive blocking) but may allow
`debug.introspect` and `debug.print`. The repo's existing policy plumbing
(`HostPolicy`, `pol.Check`, per-scope `Installed()`) is reused verbatim —
no new policy machinery, just a new scope name.


## 8. Phasing

The surfaces have very different build costs; ship in order of
value-per-effort:

- **Phase 1 — printing + pure structural + deterministic perf** (small,
  no new engine seams beyond `Debug.stack`):
  `tap`, `label`, `dump`, `watch`, `assert`, `todo`, `parse`, `sig`,
  `body`, `deps`, `explain`, `words`, `defs`, `modules`, `types`,
  `stack`, `sizeof`, `shape`, `steps`, `time`, `bench`. Almost all of
  this is curation over existing algorithms.
- **Phase 2 — tracing + profiling** (drives the existing `TraceCallback`
  harder): `trace` (re-export), `profile`, `disasm` (needs the
  `Recorder`/StackForm wired through a sub-engine).
- **Phase 3 — interactive stepping** (`DebugSession` + `StepController` +
  `DebugOps`): `step`, `break`, `break-when`, `run-stepped`, plus a
  scripted controller for tests. The TTY/REPL controller and a possible
  `aql debug <file>` CLI subcommand are the host-side follow-on.
- **Phase 4 — runtime memory** (`DebugOps.MemStats`): `heap`, `gc`.

Each phase is independently shippable and independently useful; nothing
in a later phase blocks an earlier one.


## 9. Test discipline

Per `lang/go/CLAUDE.md`, every behaviour gets a paired negative test:

- `Debug.tap`/`label`/`dump` must **return the input unchanged** (assert
  the residual value, not just that something printed) and must print to
  the *registry Output writer* (capture it), not real stdout.
- `Debug.steps` must be **deterministic** across runs; pair with a row
  asserting that a *more expensive* body yields a *strictly greater*
  count (the contract is "counts real work").
- `Debug.time`/`bench` must use a `FixedClock` in tests so `elapsed-ms`
  is reproducible; assert structure/types, and assert that a real clock
  path doesn't panic.
- Negative rows: `Debug.sig undefined-word/q` errors (not silent `Any`);
  `Debug.sizeof` / `shape` on a **type literal / carrier** must not panic
  (the `TestTypeLiteralNoPanic` discipline); effectful words refuse under
  a denying policy with the documented error; `Debug.assert false "msg"`
  raises `[aql/assertion_failure]`, `Debug.assert true "msg"` does not.
- `Debug.step` is exercised with the `scriptedController` so the
  interactive path is covered headlessly; assert that `Debug.break`
  with **no active session** is a pure no-op (production safety).

Spec rows live in `lang/spec/debug.tsv`; Go-level tests (host capture,
policy refusal, no-panic, scripted stepping) in `debug_test.go`.


## 10. Open questions for the maintainer

1. **Re-export vs relocate.** `Debug.trace` and `Debug.parse` duplicate
   `IO.trace` / `Vm.parse`. Re-export (keep both, `Debug.*` is an alias)
   is the back-compat-safe default proposed here. Do you instead want the
   canonical home moved to `aql:debug` with `IO.trace`/`Vm.parse`
   deprecated? (Affects whether this is purely additive.)
2. **`Debug.stack` engine seam.** Exposing the live stack to a handler is
   the one new engine-internal read. Acceptable as a read-only privileged
   native, or would you prefer stepping be the *only* way to see the full
   stack (keeping handlers strictly arg-scoped)?
3. **CLI surface.** Should interactive stepping get a first-class
   `aql debug <file>` subcommand (a Phase-3 host follow-on), or stay
   REPL-only for now?
4. **Scope granularity.** Is a single `debug` scope with four ops
   (§7) the right shape, or should `debug.runtime` be folded into the
   existing host/runtime gating instead of living under `debug`?

No ADR entry is proposed — per repo policy this stays a `design/` note
until a maintainer says otherwise.
