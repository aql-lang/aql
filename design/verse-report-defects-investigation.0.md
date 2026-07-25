# Defect investigation — root causes for `verse-in-aql-report.0.md` §6

The Verse comparison report
([`verse-in-aql-report.0.md`](verse-in-aql-report.0.md)) verified its AQL
claims by running them, and seven defects fell out. This note is the
follow-up: for each, the cause **in the source**, the blast radius as
*tested*, and what a fix has to decide. Reproduced against `main` @
`ab0e1e0`.

Nothing here is a decision. Two items (A, B) are unambiguous engine bugs
whose fix shape is clear; three (C, D, F) need a design call named below;
one (E) is mostly documentation with one engine defect inside it; one (G)
is an already-recorded issue whose worst face is not recorded.

| | Defect | Kind | Severity |
|---|---|---|---|
| **A** | `await`/`spawn` share mutable payloads → data race | soundness | **highest** — undefined behaviour, docs assert the opposite |
| **B** | Compiled `do`/`each` leak `context set` to the parent | miscompile | high — silent wrong answer, default path |
| **C** | `do […] error […]` + trailing expr → leaked `internal_error` | miscompile | high — three variants, user-visible by default |
| **D** | `aql check` runs module bodies; a default run runs them twice | correctness | medium — effects during a "no-run" command |
| **E** | Error-code table documents codes the engine can't produce | docs + 1 engine bug | medium |
| **F** | `case` exhaustiveness works in only 1 of 4 union spellings | checker precision | medium — gating error on correct code |
| **G** | Un-separated forward calls evaluate right-to-left → stale reads | recorded, deferred | medium — invalidates tests silently |


## A. `await` branches share mutable container payloads — a data race

### Root cause

`ForkConcurrent` (`eng/go/fork.go:38`) isolates **execution scopes** and
nothing else. It clones `Defs`, `Types`, `builtinWords`, `Contexts` (with
a private child layer over the parent's top store) and `Args`. Its header
states the guarantee precisely and correctly:

> it shares the parent's read-only infrastructure but isolates every
> mutable execution scope. The fork observes the parent's bindings and
> context as of the fork call; its later writes stay private.

"Its later writes" means writes to those *scopes* — `def`, `context set`.
Cloning a binding table copies `Value`s, and a `FlexMap` / `FlexList` /
class-instance `Value` holds a pointer to a shared `*OrderedMap`. Nothing
deep-copies the payload, so in-place mutation is visible to every fork and
to the parent.

### Evidence

```
import "aql:time-util"
def m (make FlexMap {})
TimeUtil.await [[m set a 1] [m set b 2]] ;
m
→ [{b:2 a:1} {b:2 a:1}] {b:2 a:1}
```

Both branches and the parent observe both writes; a sibling reads an
earlier branch's write. An immutable `Map` behaves correctly by contrast
(copy-returning `set`, parent unchanged) — so the hazard is exactly the
mutable containers the docs name as safe.

`OrderedMap.Set` (`eng/go/value.go:85-91`) is an unsynchronised map assign
plus a slice append, so this is undefined behaviour rather than merely
surprising. `go build -race`:

```
WARNING: DATA RACE
Read at 0x… by goroutine 17:
  eng/go.(*OrderedMap).Set()      eng/go/value.go:87
  native.setFlexMapHandler()      lang/go/native/native_storage.go:749
  native.awaitAll.func1()         lang/go/native/native_temporal_await.go:104
Previous write at 0x… by goroutine 16:
  eng/go.(*OrderedMap).Set()      eng/go/value.go:90
```

### Wrong claims

- EXPLANATION.md:683 — "Mutable side effects within a branch are local to
  that branch's sub-engine."
- HOWTO.md:353 — "Each branch runs in a sub-engine, so writes to mutable
  objects inside one branch do not bleed into the others."

`native_temporal_await.go:23-25` is careful and correct about what it
*does* guarantee (forking on the dispatching goroutine makes reads of
parent scope state race-free); the user-facing prose overclaims from it.

### Fix decision

The safe rule already exists in the tree — `send` refuses to pass mutable
containers between processes — and `await`/`spawn` do not apply it.

1. **Deep-clone mutable payloads into each branch.** The only option that
   leaves the documented semantics standing. Costs a copy per branch per
   container; `AdoptIntoNode`/`AdoptIntoFlex` already do the traversal.
2. **Refuse at the boundary, as `send` does.** Cheap and loud, breaks any
   program that shares a flex container across branches today.
3. **Document sharing as intended and synchronise `OrderedMap`.** Fast,
   but makes `await` a concurrency primitive whose users must reason about
   interleaving — a large change to AQL's story, and it would leave
   `deq`/rendering racing too.

Recommend (1), with (2) as the interim if a copy cost is unacceptable.

### Test gap

`make verify-bytecode` and the `-race` gate both run the corpus, but no
spec row shares a mutable container across `await` branches, so the race
is unreachable from the suite. One row of the shape above under `-race`
catches it.


## B. Compiled `do`/`each` leak `context set` into the parent scope

### Root cause

The context write boundary is implemented **only in the interpreter**, as
a property of `Engine.Run`: every sub-engine run pushes a fresh child
`StoreInstanceInfo` over the parent and pops it on exit
(`eng/go/engine.go:1172-1175`, `defer`-balanced).

The bytecode VM has no equivalent. `eng/go/vm.go` never calls
`Contexts.Push`/`Pop` — its only three `Contexts` references are read-only
`TopData()` handler arguments (vm.go:518, :1127, :1627) — and there is no
context-frame opcode anywhere in the VM or emitter. So when `InvokeBody`
(`eng/go/invoke.go:21`) routes a body through `r.Invoker`, the compiled
body runs at `eng/go/vm.go:411` (`return vc.run(cl.Unit, locals, nil)`)
inside the **caller's** context layer. `context set` then COWs and
replaces that layer through `ContextStack.UpdateChain`
(`eng/go/core_helpers.go:1999`, implemented at
`eng/go/contextstack.go:88-105`, which scans from the top and *replaces*
the matching entry). With no child entry to replace, it replaces the
parent's — so the write survives.

This is a structural omission, not a wrong branch: the frame was never
modelled in the VM.

### Why it surfaced now, and why the word set is what it is

`do`/`each`/`fold`/`scan`/`filter` grew a `CallableSpec`, so their bodies
now lower to real closure units instead of islanding —
`lang/go/native/native_control.go:14-21` says so explicitly ("no longer a
baked-const list re-run through an interpreter sub-engine at run time").
Words that still island keep the correct semantics **by accident**:
`for-each` has `CompileEffect: CompileFallbackBody` and no `Callable`
(`native_array.go:334-335`), so its body reaches `invokeClosureOn`, misses
the `ClosurePayload` cast (vm.go:390-395) and falls through to
`RunResolved` → `runPooledAt` → `Engine.Run`, which pushes.

The other two rows of the report's table also have explanations:

- **`await` agrees between paths** because `ForkConcurrent` builds a fresh
  `ContextStack` and pushes its own child layer (`fork.go:55-58`),
  independent of which engine runs the branch.
- **`for` and `if` agree at 2** because neither is a boundary in *either*
  engine: `runForLoop` (`forloop.go:44-53`) and `if3Handler`
  (`native_control.go:534-547`) return mark/move tokens spliced onto the
  same engine's tape, so there is no sub-engine to push. **EXPLANATION.md's
  list of sub-engine creators is therefore wrong about `for`, and omits
  `for-each`** — a doc fix independent of the miscompile.

### It accumulates

The divergence is not a single end-of-loop artefact; the interpreter's
boundary is per body invocation (per element), and the compiled path
accumulates across elements:

```
context set 'z' 0 end each [drop context set 'z' ((context get 'z') add 1) end (context get 'z')] [1 2]
→ -force-compile: [1 2]      --no-compile: [1 1]
```

### Fix

Give the VM the frame the interpreter has: push a child context layer
around `invokeClosureOn`'s closure-unit run (`vm.go:389-411`) and pop it
on every exit path, mirroring `engine.go:1172-1175`. A frame-scoped
push/pop is preferable to an opcode, since it belongs to body invocation
rather than to any instruction.

### Test gap — the sharpest one here

The intended semantics are **already asserted by name in Go tests**, but
only against the interpreter:
`lang/go/native/context_test.go:98` `TestContextSubEngineIsolation`
("parent context should be unchanged after sub-engine write") and `:120`
`TestContextSubEngineNewKeyDoesNotLeak`. Neither runs under the compiler.

The spec corpus is more interesting still. `lang/spec/edge-containers-2.tsv:101`
asserts the shadowing half and explains the omission of the other half in
its comment:

> a child write SHADOWS inside the child (it also VANISHES when the child
> ends — that shape is pinned interpreter-side only, **as it currently
> compiles to a fallback island**)

So the gap was not "we know this is broken". It was "we know this is
untested on the compiled path, and here is why that is safe" — and the
reason stopped being true when `do` gained a `CallableSpec` and began
lowering to a closure instead of islanding. The comment's premise was
invalidated by a later change, and nothing re-examined it.

That is the transferable lesson: a spec comment justifying an omission by
appeal to *current* lowering behaviour is a landmine, because the
justification is invisible to the gate that would otherwise catch the
change. Either the row covers both modes, or the claim belongs somewhere a
lowering change has to confront it.

Running the two Go tests in both modes, plus a spec row for the vanish
shape, closes the immediate hole.


## C. `do […] error [handler]` + a trailing expression → leaked `internal_error`

### Root cause

An **arity model error at the `error` ReturnsFn seam**, not a
stack-discipline bug in the lowerer.

`errorReturnsFn` (`lang/go/native/native_control.go:1272-1301`) runs the
handler during the compile pass over a seeded `Error` carrier
(`RunCarrierBody`, line 1282), so it holds the resulting stack and can see
`len(stk) == 0` for a handler like `[drop]`. At lines 1289-1290 it treats
"not exactly one" as *unknown* and returns the wide one-value
`dynamic(Any)` bound instead of refusing. The runtime handler
(`errorHandler`, lines 1319-1337) returns `InvokeBody`'s residual
verbatim, with no padding to the declared `Returns: []*Type{TAny}` — so
`error` genuinely produces zero values.

Everything downstream is faithful to that wrong model:

1. `error` declares `CompileEffect: CompileFallbackBody` (line 1188) and
   its handler nets 0, so the closure path (which needs
   `CallableSpec.BodyOut: 1`) declines and the dispatch is recorded as a
   single-output interpreter island — `RecordFallback(span, ins, outs[0],
   pos)` at `eng/go/carrier.go:1888`.
2. `FallbackSpan` (`eng/go/bytecode.go:811-815`) has **no out-count
   field**, so the island's real arity is unrepresentable.
3. `lowerFallback` unconditionally pushes exactly one simulated slot
   (`eng/go/lower.go:2433, 2453`).
4. `Finalize` reconciles the residual, `resolveDynamicApply` sees a
   leading `dynamic(Any)` over static args and classifies it as a runtime
   fn-value apply (`eng/go/emit.go:7143`), emitting `OpCallDynamic /1`
   (`emit.go:7635`).
5. At run time `runFallback` (`eng/go/vm.go:1147-1171`) appends the
   island's real 0 results with no count check, so the stack is one short
   and `callDynamic`'s guard fires (`eng/go/vm.go:673-674`).

The check pass shows the mis-model without compiling at all: `aql check`
on the repro reports the residual as `dynamic(Any) Integer` where the
interpreter's real residual is the single value `5`.

### Three variants, one cause

| Program | compiled | interpreted |
|---|---|---|
| `do [1 div 0] error [drop]` ⏎ `2 add 3` | `CALL_DYNAMIC underflow` | `5` |
| `1` ⏎ `do [1 div 0] error [drop]` | `CALL_DYNAMIC underflow` (pc=4) | `1` |
| `def x (do [1 div 0] error [drop])` ⏎ `x` | `BIND_GLOBAL underflow` | `[aql/undefined_word]: undefined word: x` |

A handler that leaves a value (`error [drop 9]`) is fine, and so is a
program that ends at the `error`.

### Why the adjacent no-error case refuses cleanly

`do [1] error [drop]` + a trailing expression refuses with
`force-compile: code-body word error (Stage 2)` for an unrelated reason: a
statically concrete do-result bakes into the island span, and a baked arg
beyond the signature's `BarrierPos` (1 for `error`) is rejected
(`eng/go/carrier.go:1878-1879`), so the island declines and
`recordCallRefusal` marks the program uncompilable. That is the correct
behaviour the error-fired path is missing.

### Fix

Two independent repairs, both worth doing:

1. **Make `errorReturnsFn` refuse instead of widening.** It already knows
   the true arity — it ran the handler. When `len(stk) != 1`, call
   `MarkUncompilable` rather than returning a one-value bound. Under
   ADR-005 and the refusal architecture (fallback is always sound), that
   is the sanctioned response and it fixes all three variants at once.
2. **Give `FallbackSpan` an out-count.** The island model being
   structurally single-output is the reason a correct arity could not be
   expressed even if known. Adding `NOut` and honouring it in
   `lowerFallback` + `runFallback` removes the whole class.

### A note for the coverage allowlist

`eng/go/vm.go:1811` carries
`//covergate:allow bindGlobal's only error path is its own allow-listed
defensive underflow guard, unreachable without a bytecode-level fault`.
The premise is satisfied as written — a compiler bug *is* a bytecode-level
fault — but a two-line AQL program reaches it. These guards are doing real
work in production rather than being dead defensive arms, which is worth
recording in `design/COVERAGE-ALLOWLIST.10.md` when (1) lands.


## D. `aql check` executes imported module bodies; a default run executes them twice

### Root cause — two deliberate decisions that compose badly

**1. `import` runs during check.** All eight `import` signatures are
registered `Go(handler, RunInCheck())`
(`lang/go/native/native_misc.go:131-175`). `RunInCheck()`
(`eng/go/sigimpl.go:86`) sets `GoImpl.RunInCheckMode`, consulted at
`sigimpl.go:214`, telling the check pass to run the real handler instead
of modelling its result. `import` needs this: the checker cannot type
`Mod.v` without the module's actual exports.

**2. Check mode is deliberately not propagated into the body.**
`lang/go/native/native_module_module.go:139-146`:

> CheckMode is deliberately NOT propagated to the module sub-registry.
> Module bodies need concrete string literals (used as export names / map
> keys) which carrier-stripping under CheckMode destroys.

`runModuleBodyCover` (`native_module_module.go:36`) builds a fresh
`newSubRegistry()` and copies an explicit list — `ModuleScope`, the type
sequence, writers, `Effects`, observe hooks, coverage, runtime stamping,
host fileops/formats/extensions, module config, parent context. `Check` is
not on it.

Together: `check` runs `import` for real, and the body it runs has check
mode off — so every effect in a module body executes with full ambient
authority during a type check.

### The doubling

The pre-flight check runs by default before execution and file modules are
not cached, so a default run imports twice. Counted with a `print` in the
body:

| Invocation | body executions |
|---|---|
| `aql main.aql` (default) | **2** |
| `aql -no-check main.aql` | 1 |
| `aql check main.aql` | 1 |

CLI.md:63 documents `check` as "type-check without running".

### Fix decision

The stated rationale is sound but conflates two *separable* things check
mode does:

- **carrier stripping** — replacing concrete values with type carriers;
  this is what breaks module bodies, since export names and map keys must
  stay concrete;
- **handler substitution** — running a signature's `ReturnsFn` instead of
  its handler; gated independently at `eng/go/sigimpl.go:214`.

Only the first breaks module bodies. So the real repair is a third mode —
*substitute effectful handlers, keep values concrete* — expressible with
switches that already exist, letting a body still produce real export
names while `IO.write` is modelled rather than performed.

Cheaper measures, in increasing order of honesty:

1. **Cache the check-pass module instance for the run.** Kills the
   doubling; check still performs effects once.
2. **Report it.** Emit an info diagnostic naming each module whose body
   was executed during check. No semantic change, and it makes CLI.md's
   claim true-with-a-caveat rather than false.
3. **Deny-all policy around the check-pass body.** Cheap, but denial
   *raises*, aborting the import and losing the exports check needs — so
   it only works if denial on this path becomes a silent stub, which is
   new policy behaviour.

Recommend (1) + (2) now, split-mode as the repair.

### Test gap

No spec row imports a module whose body has an observable effect, so
nothing distinguishes "imported and typed" from "imported, typed, and
executed twice". `Registry.Effects` (`eng/go/effects.go`) already counts
observable effects for the compiled-fallback fence and is the natural
oracle for such a row.


## E. REFERENCE.md documents error codes the engine cannot produce

### Method

Extracted all 22 rows of REFERENCE.md's "Common codes" table, searched Go
source, tests and `lang/spec/*.tsv` for each, then probed each documented
condition through `do […] error [dot code]` to see what the engine
actually attaches.

### Result

| Documented | Reality |
|---|---|
| `cap_denied` | phantom — 0 occurrences anywhere; policy denial is `aql/permission_denied` (`lang/go/policy/error.go:10`) |
| `type_mismatch` | phantom — a real mismatch raises `aql/signature_error` |
| `unify_fail` | phantom, and wrong in kind: a failed `unify` does not raise, it returns the value `~unify-fail false` |
| `extend_user_type` | phantom |
| `out_of_range` | wrong name **and** an engine defect (below) |
| `io_error` | never engine-minted; a user-raisable atom only (`lang/spec/edge-errors-1.tsv:16`), though EXPLANATION.md's worked example handles it as if the engine produced it |

Correct as documented: `not_found`, `arith_error`, `signature_error`,
`incomparable`, `user_error`, and the remaining rows.

### The engine defect inside it

`[1,2] dotr 9` produces an error whose `code` is `None` — a **code-less**
error, which cannot be dispatched on, defeating
`do […] error [dup .code eq …]` for that condition entirely. The
check-time diagnostic for the same condition is `index_out_of_range`, a
third spelling.

It is one instance of a systemic gap: `lang/go/native` has 417 non-test
`fmt.Errorf` sites and `lang/go/modules` 119, none attaching an `[aql/…]`
code.

### Fix

Generate the table from the registry, the doctrine that makes `aql
describe` unable to drift. That needs a registry-side enumeration of
codes, which does not exist today — and adding one also supplies the data
source for `aql explain <code>` (R4 in
`rust-zig-roc-faber-in-aql-report.0.md`). Separately, give the
out-of-bounds runtime error a code.


## F. `case` exhaustiveness works in only one of four union spellings

### The symptom is narrower and odder than first reported

The report described this as "the shorthand `fn` form loses the union".
Completing the matrix shows the working cell is the exception, not the
broken one:

| | `def`-bound union name | inline `(Integer tor String)` |
|---|---|---|
| shorthand `fn x:IS R [body]` | FAIL | FAIL |
| bracket `fn [[x:IS] [R] [body]]` | **PASS** | FAIL |

Controls pass in both forms: `Boolean` (true+false clauses) and a
`refine Integer` newtype. So three of four union cells emit
`case_not_exhaustive` on a correct program, and only bracket-plus-named
proves coverage.

### What is established

The decision point is `v.Dynamic` on the scrutinee carrier
(`lang/go/native/case_exhaustive.go:727`): a dynamic scrutinee has no
static type to prove coverage against, so the clause list must carry its
own catch-all, and the diagnostic at line 732 is emitted. Everything
therefore turns on why a declared union parameter arrives as a *dynamic*
carrier in three of the four spellings.

Type identity is **not** the difference. All of these hold:

```
def IS (Integer tor String)
typeof IS                          → Disjunct
IS teq (Integer tor String)        → true      # named and inline are the same node
(Integer tor String) teq (Integer tor String)  → true
5 is IS                            → true
```

Nor is the declared type lost for dispatch — the shorthand form still
rejects a wrong argument (`f true` → `[aql/signature_error]`). So the type
is recorded and enforced; it is specifically the carrier the
exhaustiveness analysis sees that is dynamic.

One suggestive datum: `IS istype` → **false**. A `Disjunct` is not a type
*literal*, so any path that decides "is this annotation a type?" by
`istype` would treat a union annotation as a value and fall back to
dynamic.

`check --strict` confirms the dynamism directly and shows it is
union-specific rather than a general property of the shorthand form:

| shorthand parameter type | exhaustive proved | dynamic operand |
|---|---|---|
| `x:Boolean` (true+false clauses) | yes | no |
| `x:Pos` (`refine Integer`) | yes | no |
| `x:Integer` (interval clauses `[gt 3]`/`[lte 3]`) | yes | no |
| `x:IS` (`Integer tor String`) | **NO** | **YES** |

The strict advisory reads "case dispatched over a dynamic operand —
matched optimistically, re-checked at runtime"; the bracket form with the
same union emits no such advisory.

That also identifies what the dynamism costs. `disjunctPartitionReturns`
(`eng/go/carrier.go:2178`) is the machinery for declared-union parameters,
and it gates on `IsDisjunct(a) && a.Carrier && !a.Dynamic` (line 2185)
plus `DisjunctInfo.Declared` (line 2187) — it needs a **strict** declared
Disjunct carrier. A dynamic one skips the whole per-alternative partition,
so the union parameter loses not just `case` exhaustiveness but the
`partial_dispatch` analysis (line 2231) that reports a body with no
overload for one alternative of its own declared domain.

### What is not yet established

Which of the ~12 sites that set `Carrier.Dynamic` (`eng/go/carrier.go:48,
57, 83, 231, 875, 890, 985, 1031, 2978, 3006, 3025, 3271`) fires for a
union parameter, and why the bracket-plus-named path avoids it. The
working hypothesis — that an inline annotation is *computed under
check-mode carriers*, and `tor` over carriers yields a dynamic result,
whereas a `def`-bound union was computed at `def` time outside the fn — is
consistent with the matrix but **not traced to code**, so it is
speculative and stated as such.

### Independently, the diagnostic is misleading

"the scrutinee is dynamic — no static type can prove the clauses cover it"
is wrong wording for these cases: the scrutinee *has* a declared type; the
analysis lost it. The message should distinguish "genuinely untyped
scrutinee" from "declared type not recoverable here", because the two ask
different things of the author.

### Fix

Blocked on finishing the trace above. The wording fix is independent and
can land now.


## G. Un-separated forward calls evaluate right-to-left → stale reads

### Not a new defect

`design/ERRORS.8.md:188-206` records this as VOXGIG **B2a**:

> `(1 add 1) print (2 add 2) print` prints `4` then `2` — un-separated
> chained forward calls evaluate right-to-left.

The fix is §6.1 of that note, explicitly **not landed** ("deferred to the
structure-first lazy-resolution rework"), with statement separation
(`;` / `end`) as the named mitigation.

### The unrecorded manifestation

The note frames B2a as an output-ordering nuisance. Because a newline is
*not* a separator, the same rule silently yields a wrong **value**:

```
def m (make FlexMap {n:0})
m set n 1
m.n                 → 0      (the pre-write value)
```

`m set n 1 ;` then `m.n` → `1`; `(m set n 1)` then `m.n` → `1`. Any
mutate-then-read pair written without an explicit barrier reads stale.

That is a data-correctness face of B2a and belongs in that note's §6 and
NUR029's neighbourhood. `mixed_form_call` already exists as the advisory
hook, and mutate-then-read is a recognisable shape for one.

### Methodological note, recorded deliberately

The first version of the Verse report used un-separated mutate-then-read
programs to *test* whether `await` branches were isolated, read the stale
values as evidence of isolation, and on that basis rejected a correct
claim that mutations escape a branch (defect **A**). The barrier is
load-bearing in test programs, not only in production ones — which is
itself an argument for landing §6.1, since a sequencing rule that can
invalidate a test silently is more expensive than an ordering surprise.


## Cross-cutting observations

**Three of the seven are compiled-vs-interpreted divergences** (B, C, and
the compiled half of G's neighbourhood), all in the same architectural
seam: body invocation. `design/MISCOMPILE-HUNT-FINDINGS.0.md` records 23
prior `--compile != interpret` divergences and the same seam keeps
producing them, which argues for a differential gate specifically over
*body-invoking words × observable side effects* rather than over the
corpus at large.

**Two are "the guarantee is real but narrower than the prose"** (A, and
B's doc half). Both times the Go-level comment is accurate and the
user-facing doc generalises it. `eng/go/fork.go`'s header says "execution
scope"; EXPLANATION.md says "side effects".

**Two would have been caught by tests that already exist** — B's
semantics are asserted in `context_test.go` but only against the
interpreter, and A's shape is one spec row away from the existing `-race`
gate. Neither needs new machinery, only coverage of the second execution
path.
