# Defect investigation — root causes for `verse-in-aql-report.0.md` §6

The Verse comparison report
([`verse-in-aql-report.0.md`](verse-in-aql-report.0.md)) verified its AQL
claims by running them, and seven defects fell out. This note is the
follow-up: for each, the cause **in the source**, the blast radius as
*tested*, and what a fix has to decide. Reproduced against `main` @
`ab0e1e0`.

Nothing here is a decision. Three items (A, B, F) are unambiguous engine
bugs whose fix shape is clear — F's fix is written and its blast radius
measured; two (C, D) need a design call named below; one (E) is mostly
documentation with one engine defect inside it; one (G) is an
already-recorded issue whose worst face is not recorded.

Two findings grew materially during investigation. **F** was reported as a
`case`-exhaustiveness precision gap and turned out to be a parameter-type
erasure that also changes arity silently. **E** was reported as four
phantom error codes and turned out to include an engine defect (a
code-less runtime error) alongside two claims that were simply wrong.

### How much to trust this

B, C, D and F were each put through two independent adversarial reviews
briefed to *refute* them — one attacking citations first, one attacking by
experiment first. Result: **no root cause was refuted**, every symptom
re-reproduced, one verdict CONFIRMED and seven PARTIAL. The PARTIALs were
citation drift (off-by-one line numbers, one term of art used loosely),
blast-radius omissions, and one overstated rule — all folded in here, with
each correction re-verified before it was written down. The causal stories
in A-G are what survived that; the fix *scopes* moved more than the
diagnoses did.

Corrections worth naming because they change what a fix must touch: the
leak rule is not "lowered to a closure" alone (B), `outer` and `walk` also
leak (B), and one of this note's own earlier fix recommendations for D was
retracted as blocked rather than cheap.

One of those corrections was itself wrong and has since been reversed: an
earlier revision of this note recorded that **`case` does not leak**. It
does. The argument for the exemption (no `CallableSpec`, `runCaseBody`
calls `RunResolved`) is accurate about the interpreter and irrelevant to
the compiled path, which desugars `case` inline. That mistake is worth
keeping visible, because it is the same mistake in miniature that the
whole of B turns on: reasoning about one engine's call graph and reporting
the conclusion as if it were the language's semantics. See the corrected
paragraph in B and "Blocker 2 is worse than the table says".

| | Defect | Kind | Severity |
|---|---|---|---|
| **A** | `await`/`spawn` share mutable payloads → data race | soundness | **highest** — undefined behaviour, docs assert the opposite |
| **B** | Compiled `do`/`each` leak `context set` to the parent | miscompile | high — silent wrong answer, default path |
| **C** | `do […] error […]` + trailing expr → leaked `internal_error` | miscompile | high — three variants, user-visible by default |
| **D** | `aql check` runs module bodies; a default run runs them twice | correctness | medium — effects during a "no-run" command |
| **E** | Error-code table documents codes the engine can't produce | docs + 1 engine bug | medium |
| **F** | Shorthand `fn` discards a `def`-bound union/enum param type | engine | **high** — silently makes a 1-arg fn callable with 0 args |
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
body runs at `eng/go/vm.go:410` (`return vc.run(cl.Unit, locals, nil)`;
`invokeClosureOn` spans :389-411) inside the **caller's** context layer.
`context set` then COWs and replaces that layer through
`ContextStack.UpdateChain`
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

**The rule is not simply "lowered to a closure".** Adversarial review
produced a live counterexample: `with-decimal` carries a `CallableSpec`
(`native_math.go:406`), lowers to a closure, runs through
`invokeClosureOn` — and does **not** leak (measured 1/1 compiled and
interpreted, against `each`'s 2/1 on the same shape). Its handler pushes
its own frame:

```go
// native_math.go:435-436
r.Contexts.Push(r.Contexts.Top())
defer r.Contexts.Pop()
```

The accurate rule is: **a word leaks iff its body lowers to a closure and
nothing else on the path pushes a frame.**

That counterexample is also the single best argument for the fix below —
it *is* the fix, already in production for one word, and the spec comment
above it (`native_math.go:404-405`) states the reason outright: "the
handler pushes the context around `InvokeBody` so the VM-run body's
BigDecimal ops read the override."

Two further leaking words were found on review and are confirmed here —
`outer` (`CallableSpec` at `native_array.go:428`) and `walk`
(`natives.go:337`), both measured compiled 2 / interpreted 1. So the
leaking set is at least `do`, `each`, `fold`, `scan`, `filter`, `outer`,
`walk`, plus nested `do`, list- and map-literal elements and
interpolated-string holes.

**`case` leaks too — an earlier correction in this note said otherwise and
was wrong.** The reasoning behind that correction was sound but answered a
different question: `caseHandler` → `caseClauses` (`conditional.go:78`) →
`runCaseBody` (`:355-363`) does call `RunResolved` directly and `case` does
have no `CallableSpec`, so `case` never reaches `InvokeBody` (`invoke.go`'s
doc comment at line 10 listing `case` among its clients really is stale).
But "no closure" means the closure-seam patch *cannot reach it*, not that
it is correct. Compiled, `case` does not run `runCaseBody` at all:
`caseReturnsFn` desugars the whole form to a nested-`if` chain
(`conditional.go:141-148`) that is emitted **inline into the caller's
unit**, so the clause body has no sub-engine and no frame. Measured on the
pristine tree, default mode, no fallback warning:

```
case 1 [ 1 [ context set y 77 5 ] 2 [ 6 ] ]
context has y
→ compiled: 1 5 true      interpreted: 1 5 false
```

So `case` is a context boundary interpreted and is not one compiled —
the same divergence as `do`, arrived at by a different route. It belongs
in the leaking set, and it is the clearest single case of why a
seam-level patch cannot finish this job (see "the VM inlines bodies"
below).

`otherwise` with a list argument is the same shape:
`false otherwise [ context set y 77 5 ]` then `context has y` gives
compiled `false true` against the interpreter's `false false`.

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

Confidence here is unusually high because the patch already exists: it is
the two lines `with-decimal` runs today (`native_math.go:435-436`), on the
same seam, producing the interpreter's answer. Hoisting them from one
handler to `invokeClosureOn` fixes every leaking word at once and lets
`with-decimal`'s local copy be dropped. Use the handler's own registry
(`invoke.go:23` calls `r.Invoker(r, …)`), not the sub-registry.

### That fix was implemented, validated, and REVERTED — read this first

The three-line patch above was applied and put through the full gate and
an adversarial sweep (1762 test programs across four lenses — differential
hunting, the context model, cost and unwind lifecycle, and registry
choice; the fourth reported after the revert and is folded in below).
**It must not be landed as written.** It is recorded here so the next attempt starts
from the findings rather than from the same confident paragraph.

**The gate was not the reason.** Two checklist runs failed on the patched
tree — once on `TestTuiServeAllViewersGoneQuits`, once on the
`cover-gate` profile-generation step — and *both were contention
artifacts* on a 4-core box, reproduced clean in isolation afterwards
(`covergate` itself reported 100.0%, 64888/64888 statements). Neither was
caused by the patch. The patch was reverted for the three substantive
blockers below, all found by the adversarial sweep rather than by any
gate. That distinction matters for the next attempt: **`make test` and
`make cover-gate` both pass with this patch applied**, which is precisely
why the sweep was necessary.

**What worked.** Every divergence in the table above closed: `do`, `each`,
`fold`, `filter`, `outer`, `walk`, nested `do`, the per-element
accumulation (`[1 2]` → `[1 1]`), and the new-key repro (compiled `7` →
`unknown_key`, matching the interpreter). Shadowing inside a body,
`for-each`, `with-decimal` and prototype-chain reads were unaffected. No
wall-clock regression at 200k elements. A differential regression test was
written and verified to fail on 10 subtests without the patch.

**Blocker 1 — it turns a latent bug into an uncatchable crash.** This is
ship-blocking on its own.

```
context set 'mode' "test" end
do [ context.__sys.fs set mem true  1 ]
context has nope
```

Pre-patch: `1 false`, exit 0. Post-patch: `fatal error: stack overflow`, a
Go runtime fatal that the engine's `recover()` cannot catch and that no
interpreter fallback can rescue — **on the default path**, from a
documented idiom (the in-memory-FS toggle).

`ContextStack.UpdateChain` (`eng/go/contextstack.go:87-105`) relinks
`p.Prototype` *while walking* the chain. With two stack entries that both
reach `origRoot` only through their prototypes, the second walk re-finds
the already-relinked `newRoot` and sets `newRoot.Prototype = newRoot` — a
self-cycle. Pre-patch a compiled body had only one such entry, so the
cycle was unreachable; the patch adds the second. Any later missing-key
lookup then spins forever, and misses are routine: `native_helpers.go:73`
does a `ContextStoreLookup(r, "$decimal-precision")` on **every** BigDecimal
operation.

So `UpdateChain` must be made cycle-safe *before* any second context frame
exists — stop at `newRoot`, or skip entries already relinked.

**Blocker 2 — it is incomplete, so the invariant it advertises is false.**
The patch brackets only the `InvokeBody` closure seam. Still leaking,
each verified compiled-vs-interpreted:

| Path | Why the bracket misses it |
| --- | --- |
| `case` clause bodies | the compiler desugars `case` to an inline nested-`if` chain (`conditional.go:141-148`); no closure is ever created |
| `otherwise [list]` | the list argument is evaluated in the caller's unit |
| a body bound or passed as a **value** (`def b […]`, a `List` param) | the list is auto-evaluated inline as `OpMakeList` operands — and the write lands at *bind* time, before any `do`/`each` runs |
| module-exported `fn` bodies | compiled to a unit reached by the user-call opcode, not through `InvokeBody` |
| `service` handler callbacks | routed via `InvokeCallback` → `runUnitNested` (`vm.go:365`) / `RunUnit` (`vm.go:244`), neither bracketed |
| `def f [body]` | compiled to a user-fn unit reached through CALL_USER/RET |

`vm.go` itself documents `RunUnit` as "the DURABLE-callback twin of
`vmContext.invokeClosureOn`" — so the twin seams are named in the source
and the patch covered one of them. The service case is the worst: handler
dispatch is serialized but *shared*, so an escaping `context set` becomes
cross-request state (observed `2/3/3` compiled against the interpreter's
`2/2/1`).

**Blocker 3 — an unbudgeted per-invocation allocation.**
`ContextStack.Push` allocates a `StoreInstanceInfo` *and* an empty
`map[string]Value`: +2 allocs and +112 bytes per closure-body invocation,
on every `each`/`fold`/`filter` element, whether or not the body mentions
`context`. Measured with the repo's own `allocStatsPerOp` helper: +29-33%
allocs/op on each/fold, and the `do_body` alloc-guard row went 212 → 412
against its 500 ceiling — consuming two thirds of the headroom silently,
because it still fits. `bytecode_allocguard_test.go` states that
allocations are "the hard regression signal" and that a ceiling must never
be raised "without a documented reason"; the guard table has **no**
each/fold/filter row, so the largest regression class is uncovered.

The cost is avoidable: a frame is only observable if the body can reach a
context-writing word, so it can be gated on a static per-`CompiledFn` flag
set by the emitter, or the map made lazy.

### Blocker 2 is worse than the table says — the VM inlines bodies

The fourth adversarial lens (registry choice) landed after the revert and
sharpened the picture materially. Its own question came back **negative**,
which is good news for the patch's shape, and its incidental findings are
bad news for the patch's scope. Everything below re-reproduced by hand on
the pristine tree in **default** mode with no fallback warning.

**A third body-execution surface: list auto-evaluation.** A code body that
arrives as a *value* rather than a literal leaks — and it leaks at the
moment it is bound, not when it is run:

```
def b [ context set y 77 5 ] end
print (context has y) end          # compiled: true    interpreted: false
print "no-call" end                # b is never invoked
print (context has y) end          # compiled: true    interpreted: false
```

`def name [list]` auto-evaluates the list and binds the result (both
engines bind `[5]` — documented behaviour, `lang/go/CLAUDE.md` "`def name
<node>` binds a value"). The interpreter runs that evaluation in a
sub-engine (`autoEvalList` → `runPooledSub`, `engine.go:4218-4224`) and so
gets the `Engine.Run` frame; the emitter records the same list as an
`OpMakeList` over operands emitted **inline into the caller's unit**
(`RecordMakeList` / `recordMakeListInner`, same function), so there is no
frame. The identical divergence appears when the list is a fn argument,
with a body the callee never touches:

```
def run fn [[b:List] [Any] [ 0 ]] end
print (run [ context set y 77 5 ]) end   # 0 on both
print (context has y) end                # compiled: true   interpreted: false
```

**A correction to the lens's own report.** It filed the named-body `for`
and `if` divergences (`def b [context set y 77]` then `for 3 b`) as a
`for`/`if` seam gap. They are not: the write has already happened at
`def` time, and `b` is by then the inert list `[5]`. Deleting the `for`
line entirely leaves the divergence unchanged. `for` and `if` remain
non-boundaries in **both** engines, exactly as this note recorded. The
distinction matters because a fixer chasing the reported symptom would
instrument the wrong two words.

**The registry choice is right.** The lens instrumented `invokeClosureOn`
to compare the registry the patch pushes on against the body unit's actual
dispatch registry, and ran it over the repo's own corpora (`langspec`
including `TestSpecCompiledDifferential`, `docexamples`, `cliexamples`,
`engspec`) plus 94 hand-written cross-registry programs: **zero
mismatches**. Cross-registry closure travel is refused by the compiler and
falls back; `await`, `spawn` and `service` bodies run through `RunUnit` on
a fork whose `vc.r` *is* the fork. So `reg` is the correct stack, and the
patch's cross-registry surfaces (await branch bodies, module fn `do`/`each`,
timer and interval callbacks, service handlers' nested `do`) all moved to
agree with the interpreter. Nothing about the revert is a verdict on the
push target.

**The formulation that makes the scope obvious.** The interpreter pushes a
context frame at exactly **one** site — `engine.go:1172-1174` — and every
nested body reaches it, because spawning a sub-engine is the interpreter's
only way to run one. The VM runs bodies through at least **four** paths:
closure units (`invokeClosureOn`), user-fn units (CALL_USER/RET), durable
callbacks (`RunUnit` / `runUnitNested`), and inlining into the caller's
unit (`case`, `otherwise`, list auto-evaluation, and `if`/`for` — the last
two correctly). The reverted patch bracketed one of the four, and **the
inline path cannot be bracketed by any seam patch at all**: there is no
call to wrap. Either those forms stop being inlined when their tokens can
touch the context, or the frame becomes an emitted opcode pair rather than
a Go-side push around a call.

### What the sweep could not break

Recording the negatives, because they are what a second attempt gets to
keep rather than re-derive:

- **No behavioural regression, at scale.** The differential lens ran ~1425
  distinct programs — a 720-program combinatorial corpus (every ordered
  pair of twelve body-taking words nested two deep × five context
  payloads), 400 randomized 1-3-level programs, and targeted batteries —
  and found **zero** cases where the patch changed an answer the old
  compiled path had got right. Every behaviour change moved compiled
  *toward* the interpreter.
- **The unwind is balanced.** The deferred `Pop` was exercised through
  error, `raise`, `break`/`continue`, early return and tail calls without
  leaking or double-popping, including through `with-decimal`'s
  now-doubled push.
- **No new race at the closure seam.** A `-race` build under 400
  concurrent service calls and parallel `await` bodies each doing
  `do [context set …]` reported nothing new.
- The repo's compiled-differential gates (`TestSpecCompiledDifferential`,
  `TestSpecCompiledOrFallback`, `TestCompiledCombination`,
  `TestPropertyDifferential`) all pass on the patched tree — which is the
  point made above about the gate, restated from the other side: the
  corpus contains no row that would have caught any of this.

### One residual that belongs with defect A

`reg` can be a **shared module sub-registry**, and module sub-registries
are not forked. The patch's `Push`/`Pop` therefore mutates a
`ContextStack` that concurrent branches also touch. Under `-race`, a
module fn called from several `await` branches reports the patch's own
`Pop` racing the interpreter's `Engine.Run` — but the *unpatched* tree
reports **more** races on the same program (95 against 85, 65 of the old
ones already inside `ContextStack` via `CowSet`/`UpdateChain`), the
interpreter dies on it with `fatal error: concurrent map iteration and map
write`, and the compiled answers are nondeterministic. There is no oracle,
so it is not a defect of the patch — it is defect **A**'s hole seen from
another angle, and the patch adds one more writer to it. Whatever fixes A
has to cover module sub-registries, not just container payloads.

### What a correct fix requires, in order

1. Make `UpdateChain` cycle-safe. Nothing else can land first.
2. Bracket **all** body-entry seams, not one: `invokeClosureOn`,
   CALL_USER/RET, `RunUnit`, `runUnitNested` — or establish a narrower,
   honestly-stated invariant. Note that this step cannot be completed by
   bracketing alone: the `case` desugaring, `otherwise`'s list argument
   and list auto-evaluation are **inlined**, so they need either an
   emitted frame opcode pair or a rule that stops inlining a body whose
   tokens can reach a context-writing word.
3. Gate the frame on a static "this body may touch context" flag so the
   allocation is paid only where it is observable, and add an
   each/fold/filter row to the alloc guard.
4. Land the differential regression test (written, and confirmed to fail
   on 10 subtests without the fix).
5. Only then correct EXPLANATION.md's boundary list, which is
   independently wrong: it names `for` (not a boundary in either engine)
   and omits `for-each` (a boundary in both).

### Pre-existing defects found while validating the patch

Not caused by it, reproduced on the unpatched binary, all
`-force-compile`-only unless noted:

- `do [context set a 2 end do [context set a 3 end] (context dot a)]` →
  `STORE_LOCAL stack underflow`; interpreter is clean.
- `do [var b 5 (context dot a)]` → `CALL_DYNAMIC underflow`; interpreter
  gives a clean `signature_error`.
- `({k:1} each [drop (outer [drop 7] [1]) get 0]) dot k` → recovered panic
  "index out of range [2] with length 2"; interpreter clean.
- A module fn cannot read a caller-scope context key on the compiled path
  (`unknown_key` compiled, value interpreted) — `RunModuleBody` snapshots
  the parent's top layer at import time (`native_module_module.go:162-163`),
  and under the VM the top-level run has no such layer. This is the mirror
  image of defect B and belongs in the same audit.
- The compiled path renders the `unknown_key` caret span with 11 carets
  where the interpreter uses 3 — harmless today, but it will break any
  future gate that compares stderr byte-for-byte, and defect B's fix
  routes *more* programs onto exactly this error.
- `[1 2 3] each [ do [ 9 drop ] ]` → compiled raises `[aql/each_error]:
  element 0: body produced no result`; the interpreter returns `[1 2 3]`.
  **Default mode**, no fallback rescue. A nested body that leaves no
  residual is a value the two engines disagree about, unrelated to
  context; the fn-shaped twin (`def run fn [[b:List] [Any] [ do b ]]` over
  `[ 9 drop ]`) fails the same way as a return-arity error.
- A lambda in a **map slot**, called as a method, loses its context write
  compiled but keeps it interpreted (`def m {f: ([a:Integer] => [context
  set zz 9 end a])}`) — the *opposite* direction to defect B, and
  pre-existing. So the interpreter is itself inconsistent about which call
  forms are context boundaries. That inconsistency has to be settled
  before EXPLANATION.md's boundary list can be made correct, since there
  is currently no single true list to write down.

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

   The codebase already makes the symmetric assertion on the *input*
   side, which is the strongest argument for adding the output one.
   `runFallback` refuses an island threading more than one input
   (`eng/go/vm.go:1155-1158`) with the rationale: "Assert it so a future
   lowering bug degrades to a loud internal_error → whole-program
   fallback rather than silently mis-ordering the island's inputs." That
   is exactly the failure mode the output side has — and the output side
   has no assertion at all. `runFallback` ends `return append(stack,
   results...)`, and `screenResults` (`vm.go:217-222`) checks only
   `tapeCoupled`, never a count.

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

### The multiplication

The pre-flight check runs by default before execution, and file modules
are **never** cached, so the two factors multiply. Counted with a `print`
in the body:

| Program | `-no-check` | `check` | default |
|---|---|---|---|
| one import of `m.aql` | 1 | 1 | **2** |
| two imports of `m.aql` | 2 | 2 | **4** |
| diamond (`a.aql` and `b.aql` each import `m.aql`) | 2 | 2 | **4** |

So the effect count is *importers × passes*, not a fixed 2 — a module
imported from N places in a program run with the default pre-flight check
executes its body 2N times.

`design/MODULE-CACHE.0.md` confirms the caching asymmetry and its status:
native `aql:` modules load "at most once per registry" via an `IsLoaded`
short-circuit, file modules are "never cached — `loadFileModule` re-reads,
re-parses, and re-runs the file body on every `import`", and the note is
**"analysis only — not implemented."**

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

1. **Cache the check-pass module instance for the run.** ~~Kills the
   doubling; check still performs effects once.~~ **Retracted on
   follow-up** — this is not the cheap option it looked like.
   `design/MODULE-CACHE.0.md` exists precisely because "a cache is *not* a
   transparent optimization — it changes observable semantics", and it
   gates the work on an unresolved singleton question. Sharing an instance
   across the check→run boundary is additionally riskier than a
   same-pass cache, since a module instance built while the parent was
   carrier-stripping could carry check-pass state into the real run. This
   option is blocked on MODULE-CACHE.0, not adjacent to it.
2. **Report it.** Emit an info diagnostic naming each module whose body
   was executed during check. No semantic change, and it makes CLI.md's
   claim true-with-a-caveat rather than false.
3. **Deny-all policy around the check-pass body.** Cheap, but denial
   *raises*, aborting the import and losing the exports check needs — so
   it only works if denial on this path becomes a silent stub, which is
   new policy behaviour.

Recommend **(2) now** — it is the only option with no semantic risk, and
it converts a false doc claim into a true one immediately. The split-mode
change is the actual repair. (1) is blocked on MODULE-CACHE.0.

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


## F. The shorthand `fn` form silently discards a `def`-bound union/enum type

This started as "`case` exhaustiveness is lost in the shorthand form".
The case symptom is real but minor; the actual defect is that **the
shorthand `fn` form loses the declared type of any parameter whose type
name resolves to a payload-carrying value**, and `case` is only the
loudest of five consequences. Two of the others are silent wrong
behaviour.

### Root cause

`NoEvalArgs` does not gate *map* auto-evaluation — only `NoEvalMapArgs`
does. The engine says so itself, at the exact site
(`eng/go/engine.go:3380-3386`):

> `NoEvalMapArgs` (separate from the list-only `NoEvalArgs`) suppresses
> map auto-evaluation at this slot. Used by `def`'s typed-name sig so a
> **Word at the type position arrives raw** — important when the type is a
> fn that's also a registered callable.

The scenario that comment describes *is* this defect, and the gate that
prevents it is applied to exactly one of the three words that need it:

| Signature | `NoEvalArgs` | `NoEvalMapArgs` |
|---|---|---|
| `def` typed-name (`native_definition.go:29`) | — | **`{0: true}`** |
| `fn` 3-arg triple (`:155-161`) | `{0,1,2}` | **absent** |
| `afn` (`:187-194`) | `{0,1}` | **absent** |

Both forms parse identically — the annotation is the map `{x: word(IS)}`
either way — so the divergence is entirely post-parse:

- **Bracket form.** Slot 0 is a *List*, which `NoEvalArgs{0}` does
  suppress, so the nested map is never visited. `IS` reaches
  `ResolveSigType` as a raw **Word** and takes the authoritative name path
  (`eng/go/fn_params.go:487-495`, `r.LookupTypeName("IS")`), yielding the
  minted lattice node with its `*disjunctUnifier` behaviour.
- **Shorthand form.** Slot 0 *is* the map, so `autoEvalMap` dispatches the
  word `IS`, pushing the type's **body** — a `Disjunct` *value* carrying
  `DisjunctInfo`, not a lattice node. `ResolveSigType` has no branch that
  mints a `*Type` from a Disjunct value, so it falls to the inline-disjunct
  branch (`eng/go/fn_params.go:536-542`) and returns `(TAny, &pattern,
  nil)`.

The declared type is gone before any analysis runs. Downstream,
`paramBodyCarrier` (`eng/go/core_helpers.go:701-720`) reads only `p.Type`
and calls `ParamInputCarrier(TAny)` → `NewDynamicCarrier(TAny)`
(`carrier.go:307-310`), instead of the distributing declared-union carrier
`UnionCarrierForType` builds (`carrier.go:316-323`).
`checkCaseExhaustiveness` then sees `v.Dynamic` and takes its dynamic
branch (`case_exhaustive.go:727`) before ever computing alternatives.

**Nothing in the case checker is wrong.** It is fed a `dynamic(Any)`.

Why `Boolean` and `refine` newtypes are unaffected: their `def`'d bodies
evaluate to *bare type nodes* (`Data == nil`), which `IsBareTypeNode`
catches at `fn_params.go:443` and returns as the node itself. Only bodies
that evaluate to a payload-carrying value lose the name.

### Causation proved by patch

Adding `NoEvalMapArgs: {0: true}` to `fn`'s triple signature makes the
shorthand parameter resolve to the named union and the finding disappear.
(Patch applied, observed, reverted; tree clean.)

### Consequences, all verified on the pristine binary

| Consequence | shorthand | bracket |
|---|---|---|
| `case` over a union | `case_not_exhaustive` | proves exhaustive |
| `case` over an **enum** (`def E enum [a b c]`) | `case_not_exhaustive` | proves exhaustive |
| `=>` / `afn` with a union param | `case_not_exhaustive` | n/a (same gap) |
| `case_redundant_default` advisory | lost (0 emitted) | emitted |
| **arity**: `def IN (Integer tor None)`, `f` called with **no** argument | **runs, returns `0`** | `no_signature` |
| **return type**: `def f fn x:Integer IS [x]` then `1 f` | bogus `type_error: f: expected 1 return value(s), got 2` | `1` |

The arity row is the serious one: a declared one-parameter function
becomes callable with zero arguments, silently, because the evaluated
disjunct hits `ParseFnParams`' `None`-stripping branch
(`fn_params.go:158-176`) that is only meant to fire for inline disjuncts,
synthesising a 0-arg overload.

Dispatch remains *sound* in the shorthand form — the `Pattern` still
enforces the union at call time, and `f true` is rejected in both forms.
Only the analysis and the diagnostics degrade.

### The documentation is wrong, and the test that would catch it opts out

`case`'s own hand-authored examples ship two false claims
(`lang/go/native/help/help_control.go:113-114`):

```
def IS (Integer tor String) def f fn x:IS String [case x [Integer "i" String "s"]]
  ; # exhaustive over IS — no default needed          ← actually errors
def f fn x:IS Integer [case x [Integer 1]]
  ; # check ERROR case_not_exhaustive — uncovered: String
                                                     ← actually reports "the scrutinee is dynamic"
```

`TestHelpExamplesCorrect` skips hand-authored examples **by
construction** (`lang/go/test/help_examples_test.go:145-152`, `if e :=
help.Lookup(word); e != nil && len(e.Examples) > 0 { continue }`), with a
reasoned comment: they are curated prose, not the `;# <exact-stack>` shape
the matcher validates.

This is worth stating plainly because AGENTS.md rests real weight on the
opposite property — that `describe` output is "generated from the *live
engine* … so they cannot drift from the code the way prose can"
(AGENTS.md:30). For auto-generated signature and lattice output that
holds. For hand-authored `Entry.Examples` it does not: they are prose that
merely *lives* in Go, and nothing executes them. `describe case`
currently tells the user something false.

### Fixes

1. **(verified)** Add `NoEvalMapArgs: {0: true}` to `fn`'s triple
   signature (`native_definition.go:155-161`) and `{1: true}` to `afn`'s
   (`:187-194`). This is the same suppression `def`'s typed-name signature
   already carries at `native_definition.go:29`, and
   `registerDefKeywordForms` already propagates it into the synthesized
   `def … fn …` forms via `shiftPosFlags` (`:517`). Fixes the union, enum,
   `=>`/afn, arity and lost-advisory rows at once.
2. **(verified)** Add `|| IsDisjunct(v)` to `IsSigTypeValue`
   (`eng/go/fn_def.go:185-187`) so a Disjunct in the output slot is not
   read as a concrete return-by-value. Still needed after (1), because
   `NoEvalMapArgs` does not gate a bare Word in the output slot.
3. **(design decision)** The inline union `x:(Integer tor String)` is a
   *different route into the same defect*, not a second bug:
   `ParseFnParams` deliberately evaluates a paren annotation itself
   (`fn_params.go:135-152`) and hands the Disjunct to the same branch.
   Neither fix above touches it. Either mint anonymous disjunct lattice
   nodes (today `installDisjunctUnifier` is reachable only from
   `InstallType`, `core_type.go:496` — so this introduces unnamed disjunct
   types, with knock-on effects on `Type.Equal`, dispatch sorting and error
   rendering), or — smaller and strictly local — teach `paramBodyCarrier`
   to build the distributing declared carrier from a Disjunct `Pattern`
   when `p.Type` is `TAny`. The latter also fixes the shorthand row
   independently of (1).
4. **Diagnostic wording.** "the scrutinee is dynamic — no static type can
   prove the clauses cover it" is reachable for genuinely-annotated
   parameters independent of this bug — `paramBodyCarrier` marks
   `{:T}`/`[:T]` typed-container params dynamic (`core_helpers.go:703-716`).
   It should not deny the annotation: "no static type is available for
   this scrutinee (it is gradual here)" or similar. Rewording alone fixes
   nothing — do it *in addition to* 1-3.

Measured fix blast radius: with (1) + (2) applied, the full
`eng/go/... lang/go/...` suite passes with exactly one golden update —
4 lines of `lang/go/native/testdata/fnmodel_equivalence.golden` (the
sig-table rows for `fn`, `afn` and the two synthesized `def` keyword
forms). Zero behaviour-corpus drift.

### Test gap

The suite pins case exhaustiveness **exclusively through the bracket
form**. Every union/enum row in `lang/go/test/case_exhaustive_check_test.go`
is written `fn [[x:IS][…][…]]` — lines 65, 69, 73, 77, 81, 219, 268, 272,
275, 298, 306, 310, 339, 349, 428. There is not one shorthand row and not
one `=>`/afn row in that file. The `Boolean`, `refine` and interval rows
that *are* form-insensitive pass either way, so a form-agnostic reader
would not notice the omission.

The tests that would have caught it, in order of leverage:

1. A **form-equivalence** assertion: the shorthand and bracket spellings of
   the same signature must produce the *same* diagnostic set. This
   generalises past `case` and would also have caught the arity and
   return-type divergences.
2. An `FnParam`-level golden asserting `ParseFnParams` yields
   `Type=IS, Pattern=nil` for both forms.
3. Running hand-authored help `Examples` through `check` — a weak
   "must not error" gate is enough and does not need the exact-stack
   matcher.

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

**The two engines have different *numbers* of body-entry points, and that
is the structural root of the seam's productivity.** The interpreter has
one (`Engine.Run`), so any invariant installed there is automatically
universal. The VM has four, so an invariant has to be installed four
times and nobody can tell by reading one of them whether the other three
agree. Every fix in this seam that is written as "wrap the call" inherits
that asymmetry, which is why the reverted patch was simultaneously correct
in shape and 25% complete. The durable version of this observation is not
a bug list: it is that the VM needs *one* named body-entry function that
every path routes through, in the way `matchSignature` is the one place
argument positions are decided.

**Two are "the guarantee is real but narrower than the prose"** (A, and
B's doc half). Both times the Go-level comment is accurate and the
user-facing doc generalises it. `eng/go/fork.go`'s header says "execution
scope"; EXPLANATION.md says "side effects".

**Three would have been caught by tests that already exist** — B's
semantics are asserted in `context_test.go` but only against the
interpreter; A's shape is one spec row away from the existing `-race`
gate; and F's correct behaviour is asserted fifteen times in
`case_exhaustive_check_test.go`, every one of them in the bracket form.
None needs new machinery, only coverage of the second spelling or the
second execution path.

**The recurring shape is "one axis is exercised, its sibling is not."**
Interpreter but not compiler (B, C). One call spelling but not the other
(F). One container kind but not the mutable one (A). Where the suite has a
choice of surface, it has consistently pinned one and left the other to
inference — and every defect here lives in the unpinned half. A gate that
asserted *equivalence between sibling surfaces* — same program, both
engines; same signature, both forms — would have caught four of the seven
without anyone predicting the specific bug.

**Two documentation claims are load-bearing and false.** EXPLANATION.md
and HOWTO.md assert branch isolation for exactly the values that race (A),
and `describe case` ships an example asserting exhaustiveness for a
program that errors (F). The second matters beyond its own row: AGENTS.md
rests real weight on `describe` being generated from the live engine and
therefore unable to drift. That holds for signatures and lattice output;
it does not hold for hand-authored `Entry.Examples`, which are prose that
merely lives in Go and which the example test skips by construction.
