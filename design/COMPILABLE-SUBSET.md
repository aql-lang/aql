# The Compilable Subset — a positive specification

The bytecode compiler (`eng/go/emit.go`, `lower.go`, `vm.go`, `bytecode.go`)
is **the carrier type-checker run with a recording side effect**: every
dispatch the checker resolves in a typed region is recorded as a classified
event, and `Finalize` linearises the trace into a `Program`. Anything the
recorder cannot prove it can lower **faithfully** is refused, and the caller
(`lang.(*AQL).RunCompiled`) silently falls back to the interpreter. The worst
failure mode is therefore *slow, not wrong*.

This document is the **positive** statement of what compiles and why. The code
expresses the subset as a pile of individually-justified refusal gates (chiefly
`EmitState.RecordCall` and `isInertConst`); read those for the exact edge, but
read **this** first for the rule each gate is defending. When you widen the
subset, update this file in lockstep — the gates should be a checklist against a
stated rule, not the rule itself.

> Authority: the code is authoritative for behaviour; this page is the index and
> the rationale. If they disagree, the code wins and this page is stale — fix it.

---

## 1. The contract

For every source program, exactly one of:

1. **Compiles** — `CompileCheck` returns a non-nil `*Program`; `RunProgram`
   executes it and returns a residual byte-identical to the interpreter's
   `Run` for the same source (the one documented exception is §7).
2. **Refuses** — `CompileCheck` returns `(nil, reason)`; `RunCompiled` rolls
   the registry back to its pre-check snapshot (`CompileSandbox`) and runs the
   interpreter. The `reason` names the first offending construct.

Refusal is always sound. Compilation is sound by construction + the differential
and property gates (§8); there is no independent proof.

---

## 2. The provenance model (why anything compiles at all)

Every dispatch argument must resolve to a known **operand** (`emitOperand`):

| Operand kind | Source | Lowers to |
|---|---|---|
| `opConst` | an inert literal interned in `Program.Consts` (§4) | `PUSH_CONST` |
| `opType` | a bare type node, resolved by canonical ID at run time | `PUSH_TYPE` |
| `opLocal` | a frame-local slot (loop iterator, fn param, value-def local) | `PUSH_LOCAL` |
| `opEvent` | the result of a prior recorded event, live on the VM stack | (already on the stack) |
| `opClosure` | a compiled body unit (a higher-order word's code body) | `PUSH_CLOSURE` |

Provenance rides `Value.ID`: `RecordStrip` / `RememberOriginal` save a literal's
original before the checker strips it to a carrier, and `setProduced` maps each
event output's ID to its producing event. An argument whose ID resolves to none
of the above is **unknown provenance** → refuse. A computed value is on the
simulated stack *exactly once*; a value referenced more than once is either a
re-pushable operand (const/local/type) or gets promoted to a value-def local
(`planValueDefLocals` → `STORE_LOCAL`).

---

## 3. What compiles (by construct)

| Construct | Compiles when… | Lowering |
|---|---|---|
| Native call | the checker selected ONE signature (`SiteMono`), all operands resolve, no fn-valued / quoted / code-body / dynamic operand | `CALL_NATIVE` |
| Polymorphic native | the checker widened a dynamic operand to `Any` across overloads (`RecordPolyCall`) | `CALL_NATIVE_POLY` (run-time `MatchSignature`) |
| Literal push | the value is an inert const (§4) | `PUSH_CONST` |
| Type operand | a bare type node with a registered canonical ID | `PUSH_TYPE` (by-ID, never a stale by-value copy) |
| `if` / `case` | each arm's result resolves; arms may diverge (break/continue/tail/`raise`) or be variadic (0-or-1, only the program residual may absorb it) | `JMP_IF_FALSE` + arm fragments + merge |
| Counted `for` | start/step are consts, the body nets ≤1 value/iteration, range is not a runtime-assembled list | `FOR_SETUP` / `FOR_NEXT` + back-edge `JMP` |
| `break` / `continue` | inside a compiled loop | `JMP` to loop end / `FOR_NEXT` |
| User fn (`def f fn […]`) | checked, ≤ the staged return shape; recursion via forward-ref; generics one unit per memoised instantiation | `CALL_USER` / `TAIL_CALL_USER` + `RET` (return-type checked) |
| Higher-order code body | the body compiles to a capture-resolved closure unit and the driving word invokes it via the VM seam | `PUSH_CLOSURE` + `CALL_NATIVE` |
| └ `var`-body that captures/refs an enclosing binding | the `var` cleanup is emitted as the 1-arg-only `__varundef` (not the overloaded `undef`), so the body's dynamic-Any residual can no longer mis-match `undef name fnUndefSpec`'s `TFnUndef` slot in check mode — the cleanup dispatches identically (1-arg unbind) in check and at runtime, so the loop binding never leaks into the capture set | same closure unit; cleanup lowers to a `__varundef` `CALL_NATIVE` |
| Fn-value as DATA | introspection (`typeof`/`arityof`/…) or a residual/member, never an INVOKED fn value | baked const / `OpCallDynamic` at the residual boundary |
| Computed list literal | top-level only, every element a core-builtin (deterministic) result or const | `MAKE_LIST n` |
| Computed map literal | every value operand resolves AND the map is CONSUMED in-frame (a word/fn arg, incl. `make`'s body) — not a deferred residual (a bare map tail, evaluated after its frame pops). Sound in fn bodies / branches / loops: `OpMakeMap` re-assembles a fresh map per run, never frozen | `OpMakeMap` (keys ride in `MakeMaps`, values popped) |
| └ list-valued entry (`{n:[expr]}`, the `do {map}` idiom) | the value list's elements all resolve; the list WRAPPER is recorded inline (interleaved per value, in stack order) as a nested `OpMakeList`, bypassing its top-frame guard because it is a consumed operand of the enclosing in-frame `OpMakeMap` | nested `OpMakeList` then `OpMakeMap` |
| Multi-/0-result words | any (P5): the VM pushes every handler result | `CALL_NATIVE` (nout slots) |
| Typed value-def over a refinement (`def x:Pos n`, `def v:Positive x`, `def v:(Integer gt 10) x`) | the constraint is a predicate type / bare-refine newtype / DepScalar subset and the DYNAMIC body operand resolves; the bind runs the interpreter's own validate/reparent (`RunTypedBind`) over the runtime value — raise on failure is the byte-identical plain error, a passing value reparents where `defTypedHandler` reparents. A CONCRETE body keeps the proven const-pool reparent (no `BIND_TYPED` emitted); a statically-failing body stays a check-diagnostics row | `BIND_TYPED` (spec in `TypedBinds`; the STORE stays with the value-def local plan — `STORE_LOCAL` when promoted, `DROP` after validation when the binding is dead) |

---

## 4. The const pool — the load-bearing mutation-safety invariant

`isInertConst` is the whitelist of values that may live in `Program.Consts`. A
pooled const is pushed by the **same pointer-backed `Value`** on every
execution, including each loop iteration, so it MUST be immutable. The whitelist
admits only: scalars, temporal values, predicate/refine `DepScalarInfo`,
structural type bodies (`typeBodyConstOK`), fn-signature descriptors, surfaces,
generic schemas, inert reach lenses, and lists/maps of the same. It MUST NEVER
admit a mutable instance type (`Array` / `Object` / `Store`): one of those in a
pooled const would be corrupted in place by an `set` across iterations.

This is the single sharpest soundness edge. It is pinned by
`eng/go/bytecode_constbake_test.go::TestIsInertConstRejectsMutableInstances`
(mutable instances rejected; immutable scalars/compounds accepted). **Before
adding any type to `isInertConst`, prove it is immutable or that its mutators
can never reach a pooled const, and extend that test.**

Compounds (lists/maps) and type bodies are **never deduped** — `eq` on compounds
is identity, so two source literals must stay two consts with two IDs.

---

## 5. What refuses (the fallback taxonomy)

`RecordCall` and friends latch the program uncompilable on the first of these;
the interpreter then owns the whole program:

- **Compile-time word** — `RunInCheckMode` (def/import/type/macro/Test). Runs at
  compile time, emits nothing; a residual that depends on its *runtime* error
  (a check-mode-suppressed error) also refuses.
- **Context-dependent word** — `args` / `__pa` (no per-call args stack in a VM
  frame), `FullStack` words.
- **Fn-INVOKING word** — `apply` of a non-re-stepped value, `is` over a
  predicate fn: their handlers re-step the fn on the tape, which the VM cannot
  honour. (Fn-INTROSPECTION words are exempt — they only read the value.) A
  higher-order form over a LAMBDA value (filter/each/fold/scan, and walk's
  hook slots) compiles to a closure unit via `tryRecordLambdaClosure` when the
  lambda has a single own sig and the word has a callback convention; the BODY
  lambda may carry LEXICAL captures (resolved to compiled homes and threaded
  at OpPushClosure — the mini-redis KEYS shape) and the collection operand may
  be a typed non-dynamic carrier (a computed `keys` result). Still refusing:
  multi-overload lambdas, DYNAMIC (gradual) collections (the pair-vs-KeyVal
  convention is ambiguous), captures on the extras/hook path, and unreachable
  captures.
- **Quoted-operand word** — usurp / force-arity / ref-family (results re-stepped)
  — except `get`/`getr`/`set` and module-inner natives over inert atom keys.
- **Code-body word** — a `NoEvalArgs` body that is not inert (a computed paren,
  or a body carrying a flow-control sentinel that targets an enclosing frame).
- **Dynamic input / opaque output** — a dynamic carrier reached the site, or the
  checker could not type the result (unannotated / opaque wrapper) — except a
  concrete-args core builtin whose dynamic result is merely a declared-`Any`
  return (`dynOutNativeOK`).
- **Anonymous / fn-value dispatch**, **operand of unknown provenance**, **a fn
  value preceding residual args** (auto-dispatch boundary), and any operand
  shape beyond the lowerer's stack discipline (`layoutOperands` refusals).

---

## 6. The execution-environment seams

- **Fallback islands** (`OpFallback`) re-run a recorded token span through a
  reused sub-engine, threading the operand stack. Soundness rests on island runs
  being non-nested/non-concurrent within a VM run.
- **`OpCallDynamic`** applies a runtime fn value to trailing residual args (the
  `r.int 0 100` method-field boundary), leaving a non-callable value untouched —
  faithful to the interpreter either way.
- **Resource ceilings** mirror the interpreter: the VM value stack and frame
  depth share the tape's bounded-growth ceiling (`tape_exhausted`); the step
  budget raises `evaluation_limit` (but see §7).
- **Concurrency**: a registry must not be driven by two executions at once. The
  `vmRunning` CAS rejects an overlapping compiled run; an `interpRunActive()`
  check rejects starting a compiled run while an interpreter run is in flight.
  Neither can catch a foreign interpreter run STARTING on another goroutine
  once the compiled run is underway (the VM's own islands re-enter `Engine.Run`
  on the same registry, indistinguishable without goroutine identity) — that
  shape stays caller responsibility: one `*Registry` per goroutine.

---

## 7. The one semantic divergence — the step budget

`Run` meters its step budget per **tape token**; `RunProgram` meters the same
cap per **bytecode instruction**. The compiled stream is leaner than the
expanded token walk, so the VM reaches at least as far as the interpreter before
the cap. The divergence is **one-directional**: a long-but-terminating program
the interpreter would abort with `evaluation_limit` may COMPLETE under
compilation; the reverse never happens. A genuine runaway trips fast in both.

So at the ceiling, `Run` and `RunCompiled` are observably different programs;
everywhere below it they agree. This is the *only* place the opt-in flag is not
result-transparent. Pinned by
`lang/go/bytecode_findings_test.go::TestStepBudgetNoSpuriousLimit`; the property
fuzzer keeps its corpus well under the cap so the divergence never makes it
flaky.

---

## 8. The safety net

Compilation is sound by construction + these gates — keep them green and crank
them when widening the subset:

- **`test/go/langspec/compiled_differential_test.go`** — every spec row the
  emitter accepts must match the interpreter; a `minCompiledRows` floor catches
  a regression that silently refuses everything.
- **`test/go/langspec/compiled_property_test.go`** — generated well-typed
  programs (arithmetic, `if`/`for`/`case`, `each`/`fold`/`scan`/`filter`,
  maps, array/object mutation, closures/captures, apply/usurp, direct named-fn
  calls) checked for the same agreement, with shrinking. Crank via
  `AQL_FUZZ_SEEDS` / `AQL_FUZZ_ITERS`.
- **`-tags aqldebug`** (`make verify-bytecode`) — re-runs the differential and
  property gates with a fresh args slice per `CALL_NATIVE`, so a native that
  illegally retains its args slice corrupts nothing and is localised here.
- **`eng/go/bytecode_constbake_test.go`** — pins the §4 mutation-safety
  whitelist.
- **`make verify-bytecode`** — the full bracket: fmt/vet/lint, the gates above,
  `-race` concurrency gates, and the aqldebug lane.
