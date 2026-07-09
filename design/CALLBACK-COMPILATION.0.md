# CALLBACK-COMPILATION — runtime independence for callback bodies

Status: implemented (foundation + Stages 1–4 routing), gate-green. This note is
the "endgame accounting" record for the callback-compilation work: what was
retired from the interpreter, what deliberately stays, and what the remaining
frontier is. It is a design note, **not** an ADR (see `lang/go/CLAUDE.md` — ADR
entries only on explicit maintainer instruction).

## Problem

The bytecode compiler lowered only the **top-level program** to the VM tape.
Function bodies invoked as runtime callbacks — server handlers, service and
codec endpoints, predicates, spawned processes, higher-order lambdas — ran
through `Registry.CallAQL` on the tree-walking interpreter
(`sub := NewTop(r); sub.Run(tokens)`). A networking benchmark localized a ~19×
per-word penalty to exactly this: identical `convert`/`join`/`def` work cost
76 ms compiled at top level but ~1457 ms inside a `serve-raw` handler fork
(`bench/networking/`). The gap: a runtime fn value carried only raw tokens
(`AQLImpl.Body`), with **no edge** to its compiled form (`Program.Fns`).

## Mechanism (what landed)

A purely additive extension of the existing "compiler = carrier type-checker
with a recording side-effect" architecture (no rewrite):

- **`CompiledFnRef` (`eng/go/bytecode.go`)** — a durable `{Prog, Unit, Captures}`
  reference. `AQLImpl.Compiled` carries it *alongside* `Body`, so a fn value
  holds both the VM edge and the interpreter fallback. `Signature.CompiledRef()`
  reads it.
- **Store-fn bake (`eng/go/emit.go`, `compileStoredFnUnit` + `stampCompiledRef`)**
  — at a `CompileStoresFn` slot (serve-raw, service `add`, codec builders), a
  capture-free handler body is compiled to its own unit (via the existing
  `compileClosureBody`) and a `CompiledFnRef` is stamped on the interned const.
  `Finalize` back-stamps `.Prog` over a `Program.storedFnRefs` side-list once the
  `*Program` exists. A body that refuses to compile is left un-stamped and falls
  back to the interpreter, per-body and sound.
- **Store-body bake (`eng/go/emit.go`, `compileStoredBody`; `CompileStoresBody`)**
  — the code-list twin, for `spawn`: a `NoEvalArgs` process body is compiled to a
  0-param unit and the word receives a synthetic fn-value carrier (raw tokens +
  `CompiledFnRef`) in place of the raw list, so `spawnHandler` runs the unit via
  `RunUnit` on the process fork. A body that refuses rides as the plain list const
  and runs on the interpreter, unchanged.
- **`RunUnit` (`eng/go/vm.go`)** — starts a *fresh* VM run entered at a unit, on
  an idle (forked) registry. The durable-callback path: a serve-raw connection
  handler on its per-connection `connFork` runs compiled even though it fires
  after the enclosing `RunProgram` returned. Guarded by the same `vmRunning` CAS
  as `runProgram` (shared `runVMEntry` prologue).
- **`runUnitNested` + `Registry.nestedRunner` (`eng/go/vm.go`, installed by
  `runVMEntry`)** — the *live-run* twin: a callback invoked synchronously during
  a compiled run (a service handler) runs the unit **nested** in the current VM
  run, since `vmRunning` is already 1. Off the VM hot path (only `InvokeCallback`
  calls it), so the corpus differential is untouched.
- **`InvokeCallback` (`eng/go/invoke.go`)** — the single routing seam every
  native callback word dispatches through: idle registry → `RunUnit`; mid-run →
  `nestedRunner`; else → `CallAQL`. Fail-safe: values and error taxonomy are
  identical to `CallAQL` when the VM path isn't taken, and the differential gates
  prove unit execution ≡ interpreter when it is.

Wired callers: `serveRawHandler` (`net_socket.go`), service `call`
(`native_service.go`), codec `callCodecFn` (`net_codec.go`), `RunPredicate`
(`registry.go`), and `spawnHandler` (`native_process.go`, via `CompileStoresBody`).

## Retirement scope (precise)

- **Retired from the interpreter (run on the VM when the body compiles):**
  serve-raw connection handlers, service handlers, codec endpoints, and **spawn
  process bodies** — durable and live-run alike. Verified end-to-end (a service
  program's handler runs via `runUnitNested` byte-identically to the interpreter,
  `TestServiceCallNestedVMDifferential`; a compiled `spawn [print 42]` runs its
  body on the VM via `RunUnit` on the fork, `TestSpawnCompiledRunsBodyOnVM`).
- **Uniform seam, currently no-op:** `RunPredicate` routes through
  `InvokeCallback`, but predicate fns are **not stamped** (refine / is / typed-def
  are not `CompileStoresFn` slots), so predicates still interpret. The routing is
  in place so predicate compilation benefits automatically once stamping lands.
- **Deliberately NOT retired (stays interpreted, by design):** runtime-*constructed*
  code (`Vm.run` of source built at runtime, `makePriorFn` Go continuations,
  macro/parselang bodies) has no ahead-of-time form; the confined `OpFallback`
  island and `Engine.Run` remain. The whole-program fallback and the three sound
  non-definite-error corpus refusals are unchanged — this work did not touch the
  corpus refusal count, so the `refusalCeiling` / `islandCeiling` ratchets and
  `COMPILABLE-SUBSET.md §5` are unchanged.

## The networking benchmark is gated on a separate frontier

The mechanism is complete and correct, but the echo benchmark's own handler body
does **not** compile: it is `Net.recv-until` / `Net.send-bytes` over a dynamic
(`Any`) socket param — module fn-value dispatch over a dynamic receiver, the
carrier-inference "hard wall" (`aql-bytecode-stage3-inlining-plan.0.md`,
`MODULE-FN-PARAM-SLOT-COMPILATION.0.md`; two prior reverts). Every real networking
handler is socket-word-dominated, so **the echo req/s number cannot move without
module socket-word compilation** — out of scope here. A pure-core handler body
(`for`, `def`, `add`, `convert`) compiles and runs on the VM (verified).

## Remaining frontier (not landed)

- **Higher-order fn-value callbacks** (`filter`/`each` with a `(fn …)` argument)
  currently *island* rather than compiling to a closure. Retiring them needs
  non-islanding higher-order **recording** for fn-value callbacks plus an
  `invokeClosureOn` extension — it touches the VM hot path (corpus-differential
  risk), so it was left for a dedicated change.
- **Predicate compilation**: stamping refine / is / typed-def predicate fns so
  `RunPredicate` runs them on the VM. `is`-over-predicate is a current refusal
  class; high risk.
- **`model` action handlers** are not `CompileStoresFn`-stamped; routing them
  would be a no-op until they are.

## Verification

Every increment landed gate-green: `make fmt && make vet && make lint`, the full
eng + lang suites including the compiled-vs-interpreted differential, `make
cover-gate` at 100% (new defensive guards allowlisted with proof; line-shifted
allowlist entries remapped), and `-race` on the serve-raw / service / stored-fn
paths. Off-corpus regression: `TestStoredFnCompilesAndRunsOnVM`,
`TestInvokeCallbackVMAndFallbackAgree`, `TestServiceCallNestedVMDifferential`
(`lang/go/storedfn_vm_test.go`), and the `RunUnit` unit tests
(`eng/go/rununit_test.go`).
