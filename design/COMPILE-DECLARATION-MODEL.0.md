# The compile declaration model — one general solution, or fifteen special cases?

**Status:** proposal, no code. **Recorded:** 2026-08-25.
**Provenance:** the fn-util compile refusal (PR #404), which turned out to be
the same defect the taxonomy had already produced twice before.

This note answers a question the fn-util investigation forced: *the compiler
seems to work each higher-order word out as a special case — is there a general
solution?*

The answer is that boru has **two** general solutions already. One is a
mechanism (interpreter islands) that was built, used at scale, and then
deliberately retired to zero. The other is a declaration taxonomy that grew to
fifteen flags to replace it. Neither is wrong; the trade between them has drifted
past its optimum, and the evidence for that is measurable. What follows is the
measurement, a diagnosis, and a proposal that keeps both mechanisms but changes
what a word has to say.

> Authority: this is a proposal. `design/COMPILABLE-SUBSET.md` is the statement
> of the current subset and `design/COMPILE-REFUSAL-SURVEY.0.md` is the prior
> measured survey; where this note and the code disagree, the code wins.

---

## 1. What is actually there today

### 1.1 The declaration surface

A word tells the recorder about itself through `Signature.CompileEffect`, a
`uint16` bitfield (`core/go/value.go:432`), plus five adjacent per-signature
fields. Measured declaration sites, excluding the definition, the two `aliases.go`
re-exports, the recorder's own consumption, and tests:

| Flag | Sites | What it declares |
|---|---:|---|
| `CompileStoresFn` | 24 | stores a fn, invokes it later off-tape |
| `CompileFallbackBody` | 22 | code-body word; body may island |
| `CompileIslandPure` | 17 | pure typed word; dispatch may island |
| `CompileReadsFn` | 13 | reads a fn's shape, never invokes |
| `CompileModuleFold` | 12 | pure module reader; const-foldable |
| `CompileFnHandlerStrict` | 10 | …and validates its operand as an `FnDefInfo` |
| `CompileScalarFold` | 10 | pure comparison; const-foldable |
| `CompileDynBody` | 7 | runs a computed body; needs `DynEnv` |
| `CompileQuoteInert` | 6 | its quoted operand is inert data |
| `CompileStoresBody` | 4 | stores a code body to run later |
| `CompileDiverges` | 4 | always raises |
| `CompileValueDiverges` | 3 | raises for a decidable operand shape |
| `CompileStoresBodyList` | 3 | stores a list of code bodies |
| `CompileRunsBodyIsolated` | 1 | runs bodies in a captured-registry frame |
| `CompileExecutesBody` | **0** | splices the body onto the tape ⇒ refuse |

Plus `FnInertArgs` (3 sites — the per-position variant of the fn flags),
`CallableSpec` (15 sites), `QuoteArgs`, `NoEvalArgs`, `BarrierPos`.

Two things stand out before any analysis.

**`CompileExecutesBody` has no declaration site.** It is defined, aliased twice,
and consumed once in `emit.go:4812` — and no word sets it. A member of the
taxonomy has become dead without anyone noticing, which is what happens to a
vocabulary that only ever grows.

**The flags are not one kind of thing.** They answer four unrelated questions:

- *What will your handler do with this operand?* — `ReadsFn`, `StoresFn`,
  `FnHandlerStrict`, `StoresBody`, `StoresBodyList`, `ExecutesBody`,
  `RunsBodyIsolated`, `DynBody`, `QuoteInert`, `FnInertArgs` — **10 of 15**
- *Is your result predictable?* — `ModuleFold`, `ScalarFold`
- *Does control return?* — `Diverges`, `ValueDiverges`
- *May I island you?* — `IslandPure`, `FallbackBody`

The first group is two thirds of the surface, and it is the group the
higher-order refusals live in.

### 1.2 The frontier: 153 rows, and what blocks them

`lang/spec/frontier/*.tsv` ledgers every spec row the interpreter answers
correctly and the compiler does not. **153 rows across 28 files.** The
`failsWith` histogram from `test/go/langspec/frontier_spec_test.go`:

| Reason | Rows |
|---|---:|
| `body result of unknown provenance` | 22 |
| `operand of unknown provenance` | 12 |
| `fn-value-call boundary` | 10 |
| `function value reaches is` | 8 |
| `def-bound computed fn apply` | 8 |
| `check diagnostics` | 8 |
| `islanded: program embeds an OpFallback span` | 5 |
| `function value reaches eq (Stage 3)` | 5 |
| `function value reaches deq (Stage 3)` | 5 |
| `computed closure at a word's argument slot` | 5 |
| `function value reaches canon (Stage 3)` | 2 |
| `fn call operand of unknown provenance` | 2 |
| `islanded` | 2 |
| *(tail: 19 reasons at 1–3 rows each)* | ~59 |

Grouped: **~43 rows are fn-value shapes**, **~36 are provenance**, and one
file — `frontier-hof-audit.tsv`, the higher-order capability audit's own §1
programs — is 65 rows, 43% of the whole ledger. The frontier *is* the
higher-order family.

### 1.3 The seven rows that work

Seven ledgered rows are not refusals at all. They compile, they run, they
produce the right answer — and they are ledgered as failures because the
program embeds an `OpFallback` span:

```
def dbl x:Integer => [mul 2 x]        each dbl/v [1 2 3]
def add2 fn [[a:Integer b:Integer][Integer][add a b]]  fold add2/v [1 2 3] 0
def add2 fn [[a:Integer b:Integer][Integer][add a b]]  fold add2/v [1 2 3]
def add2 fn [[a:Integer b:Integer][Integer][add a b]]  scan add2/v [1 2 3]
each ([x:Integer] => [x add 1]) [1]
[10 20] each [drop 1 2 3 1 pick]
import module […] end … filter A.big [1 2 3 4]
```

These are the most ordinary higher-order programs in the language: `each` with a
fn value, `fold` with a fn value, `each` with a lambda. The blocker is a policy
constant:

```go
// islandCeiling is the maximum number of compiled programs allowed to embed an
// interpreter island (OpFallback) … it must reach 0 before the OpFallback
// machinery can be deleted (P7).
const islandCeiling = 0  // 102 -> 36 -> 29 -> 26 -> 15 -> 9 -> 7 -> 0
```

This is worth stating plainly: **the ratchet is now ledgering working programs
as frontier failures.** That was a sound trade at 102 islands. It reads
differently at 7.

---

## 2. The two mechanisms and how they trade

### 2.1 Islands

`OpFallback` (`compiler/go/bytecode.go:71`) is the general escape:

> runs `Fallbacks[Arg]` as an interpreter island: a construct the checker could
> not type re-executes through a sub-engine over its recorded tokens, threading
> the operand stack. The compiled code on either side keeps running.

The VM pops `NIn` values, spins up a real `Engine`, runs the interpreter over
the saved source tokens with those values preloaded, and pushes the residual
back. This is, exactly, "the compiler has a tape".

It was used at scale — 102 compiled programs embedded one — and then driven to
zero. The stated reason is run-time independence, and there are three real costs behind it:

1. **It poisons downstream.** `TryRecordFallback`
   (`compiler/go/compiler_dispatch_record.go:801`) says so about islanding a
   case it should not: *"islanding poisons its result to dynamic, refusing every
   downstream typed dispatch (a net loss)."* The compiled lane is
   `COMPILABLE-SUBSET.md`'s "carrier type-checker run with a recording side
   effect" — lowering works because every value carries provenance and a static
   type, chained event to event. An island's output has neither, so the next
   typed dispatch refuses, and the one after that. **An island does not contain
   the loss; it propagates it.**
2. **It is interpreter speed** for the span, which is what compiling was for.
3. **It is re-entrant.** `COMPILABLE-SUBSET.md` §6: *"Soundness rests on island
   runs being non-nested/non-concurrent within a VM run"* — and the VM's own
   islands re-enter `Engine.Run` on the same registry, indistinguishable from a
   foreign interpreter run without goroutine identity.

Cost 1 is the important one and it is the reason "just give the compiler a tape"
is not a free answer *in this architecture*. It is also the cost that a change
to the escape's contract could remove; see §4.

### 2.2 Flags

Each flag buys back one link of the type chain an island would have cut. That is
a real service and it is why the ratchet could fall: every step in
`102 → 36 → 29 → 26 → 15 → 9 → 7 → 0` is a shape that used to island and now
lowers natively, with its type intact.

The trouble is what the flags are made of.

---

## 3. Diagnosis: the flags are a patch set over a wrong default

```go
// CompileDefault is an ordinary word: no compile-relevant capability. A
// fn-valued operand reaching it means the handler invokes the fn on the tape,
// which the VM cannot honour, so the recorder refuses (Stage 3).
const CompileDefault CompileEffect = 0
```

The zero value is not "no information". It is a **substantive claim**: *this
handler re-steps fn operands on the tape.* For most words that ever receive a fn
operand, that claim is false — and a word is not asked, it is assumed. Every
flag in the operand-facing group exists to withdraw the assumption for one word.

And the assumption runs in both directions. `CompileExecutesBody` exists because
a *different* default — an inert word-list body may bake as a `CALL_NATIVE` —
was wrong for `var`, whose handler returns tape-coupled tokens. Rather than fix
the heuristic, a flag was added to opt out of it. (That the flag now has zero
declaration sites suggests the heuristic was later fixed and the opt-out was
never removed.)

This produces a recognisable failure mode: **silent, correct-but-refusing
behaviour that nobody notices until someone measures it.** Three instances, all
the same shape:

| Word family | What was assumed | What was true | Cost |
|---|---|---|---|
| `var` | inert body ⇒ bakeable | splices tape-coupled tokens | a VM screen trip, then `CompileExecutesBody` |
| `service`/`add` | stores ⇒ any representation | validates as `FnDefInfo`; a `ClosurePayload` is rejected | the §9.2e miscompile, then `CompileFnHandlerStrict` |
| `boru:fn-util` | *(no declaration)* ⇒ re-steps on tape | stashes and invokes from Go | every row refused; found 2026-08-25 |

The fn-util case is the cleanest specimen because the correct declaration turned
out to be a *transcription* of what the handler visibly does. `invokeFnUtil`
calls `MatchFnSig` + `CallBoruFn` from inside a Go handler; that is
`CompileStoresFn` by the same criterion `parse.go:262` states for its own slot
("the fn is stored, not invoked on the tape"). Nothing had to be discovered —
only written down. And because nobody wrote it down, the recorder refused every
`FnUtil.compose` / `pipe` / `curry` / `partial` / `on` / `memoize` /`flip` call
in the language, keyed on the *signature's declared arg type* before it ever
looked at the operand:

```go
for i, t := range sig.ArgTypes() {
    if t != nil && t.ConformsTo(core.TFunction) {
        if inertFn || sig.FnInertArgs[i] { continue }
        es.MarkUncompilable("function-valued operand at " + word + " (Stage 3)")
```

Two consequences worth naming, because both surprised readers:

- **`/v` cannot lift it.** Measured, `FnUtil.compose addone/v addone/v` and
  `FnUtil.compose addone addone` refuse byte-identically. `/v` is a *dispatch*
  control — it answers "does this token dispatch here, now", which is the only
  kind of question the interpreter ever needs, because the interpreter is always
  at "now". The recorder's question is about all later moments, and the answer
  lives in the handler's Go body.
- **The refusal was hiding a second one.** With the declaration in place every
  fn-util behaviour row refuses one step later, at `def-bound computed fn apply
  (closure shape unknown — Stage 1)`. The `const` row is the control: `_f_const`
  takes `TAny`, so the fn-operand gate never applied to it, and it refused that
  way all along. A missing declaration had been masking the real frontier.

---

## 4. The proposal

Two parts. The first is cheap and removes the taxonomy. The second is the real
lever and removes the cliff.

### 4.1 Collapse the operand-facing flags to three orthogonal facts

Ten of the fifteen flags encode answers to one question — *will the VM tape ever
have to carry this operand?* — plus two follow-ups that the code already reasons
about explicitly but has no vocabulary for. Replace them with three fields
**per operand position**:

```
tapeBound  bool     // does this operand end up re-stepped on a tape?
needs      Repr     // {Any, FnDefInfo, RawTokens, CompiledUnit}
env        Env      // {None, Captured, Live}
```

- **`tapeBound`** is the binary the VM actually cares about. The VM can do
  everything except re-step tokens on a tape it does not have. Calling a fn from
  Go, stashing it, reading its shape, running a body in an isolated frame — all
  fine.
- **`needs`** is the representation the handler will accept. This is the axis
  `CompileFnHandlerStrict` discovered the hard way: a capturing fn lowers to an
  `OpPushClosure` carrying a `ClosurePayload`, and a handler that validates
  `FnDefInfo` rejects it with an error the interpreter never raises.
- **`env`** is where names inside a stored body resolve. The recorder already
  reasons about this precisely — `emit.go:4827` refuses a re-run body that
  *"names a binding, because the sub-engine resolves a name the COMPILED
  context holds as a VM frame local … against the registry instead —
  diverging"* — it simply has no field for it. This is
  `design/FUNCTION-VALUE-SCOPE.0.md`'s rule appearing on the compiler side.

Every operand-facing flag maps onto the triple without loss:

| Today | `tapeBound` | `needs` | `env` |
|---|:--:|---|---|
| `CompileReadsFn` | false | `Any` (captures irrelevant — shape only) | `None` |
| `CompileStoresFn` | false | `Any` | `Captured` |
| `… \| CompileFnHandlerStrict` | false | **`FnDefInfo`** | `Captured` |
| `CompileStoresBody` | false | `CompiledUnit` | `Captured` |
| `CompileStoresBodyList` | false | `CompiledUnit` (element-wise) | `Captured` |
| `CompileRunsBodyIsolated` | false | `RawTokens` | `Captured` |
| `CompileDynBody` | false | `RawTokens` | **`Live`** (arms `DynEnv`) |
| `CompileQuoteInert` | false | `Any` | `None` |
| `CompileExecutesBody` | **true** | — | — |
| `FnInertArgs[i]` | false | `Any` | `Captured` |
| *(default)* | **true** | — | — |

Three properties this buys:

1. **`CompileExecutesBody` stops being a special case.** It becomes the honest
   answer `tapeBound: true` — the same answer the default gives, said out loud.
   A flag that exists to force a refusal is a smell; a field with a truthful
   value is not.
2. **`CompileFnHandlerStrict` stops being a flag.** It becomes
   `needs: FnDefInfo`, which is a property a reader can check against
   `fnUtilArg`'s source in one glance instead of having to know that
   `StoresFn|FnHandlerStrict` is a meaningful conjunction.
3. **The axes are orthogonal**, where the flags are not. Today the valid
   combinations are folklore. Three independent fields cannot encode an invalid
   one.

The declaration itself is **irreducible** — you cannot infer what Go code does
by inspection, and any scheme that tries will produce the fn-util defect a fourth
time. The goal is not to remove the obligation. It is to make it one obligation
with three obvious answers instead of a growing vocabulary whose membership is
learned by grep.

**What this does not do:** it does not compile one additional row. It is a
refactor of how the same facts are stated. Its value is that the next word to
arrive has a form to fill in rather than a taxonomy to study — and that the
fn-util defect becomes a visibly empty field rather than an invisible absence.

### 4.2 Typed islands — let the escape carry a result contract

The deeper fix addresses §2.1's cost 1, which is the reason the escape had to be
retired. Two corrections to the obvious version of this idea, both from reading
the code rather than the refusal strings — and both make the proposal smaller
than it first looks.

**First: an island's result already has provenance.** `RecordFallback`
(`emit.go:4274`) ends with `es.setProduced(out, seq)`, and its own doc says the
output is *"registered so the island's result flows downstream (to the residual,
or another fallback)"*. What the result lacks is a **type**: it is a dynamic
carrier, and `check.AnyDynamicCarrier` then refuses any downstream typed
dispatch that consumes it (`emit.go:4881`). So the cascade is not a provenance
failure — addressability survives. It is purely a typing failure.

**Second: the machinery for "a dynamic output is still safe to continue from"
already exists, and explicitly excludes islands.** `dynOutNativeOK`
(`compiler_dispatch_record.go:322`) lets a dispatch with concrete args and a
dynamic output bake a plain `CALL_NATIVE` anyway, on the argument that *"concrete
args mean the checker RESOLVED the sig by real matching (not widening), so a
dynamic output is just a declared-Any return … not a best-guess sig."* Its third
line is:

```go
if sig.CompileEffect.Has(core.CompileFallbackBody) {
    return false
}
```

— islanding words are screened out by name of their flag, not by an argument
about why they are different.

So the proposal is not a new mechanism. It is: **extend `dynOutNativeOK`'s
reasoning to the island path, gated on a declared result contract instead of on
"core builtin + concrete args".** A word that can state what it returns gets its
island's output typed; a word that cannot keeps today's dynamic carrier and
today's cascade.

And there is already somewhere to put the answer. `Signature.ReturnsFn` is *"the
check-mode return computer … orthogonal to the run implementation in `Impl`"*
(`core/go/value.go:353`) — a word can already compute its result type
independently of how it runs. `TryRecordFallback` even receives the resulting
`outs` and passes them through. The contract exists; the island path does not
trust it.

For a large class of islanded constructs that contract is trivially statable:
`FnUtil.compose f g` returns a Function from g's input to f's output; `each` over
a `[:Integer]` with an `Integer -> Integer` callback returns `[:Integer]`.

The consequence is that an island costs *one construct* instead of *the rest of
the program*. At that point the all-or-nothing cliff becomes a local step, and
`islandCeiling` can stop being a ratchet toward deletion and become what it
should be: a **performance** budget, not a correctness gate.

That reframes the frontier. Today a ledger entry means "the compiler cannot do
this". With typed islands most would mean "the compiler does not yet
*specialise* this" — an optimisation backlog rather than a capability gap, workable
in value order instead of one shape at a time.

`boru:fn-util` is the ideal first demonstrator: nine words whose result types are
mechanical to state (`memoize` caches, so it is not side-effect-free, but its
*return* is its operand's return — the contract is still exact), all currently
behind a wall that a declaration alone does not clear.

### 4.3 What this does **not** fix, and one thing it costs

**Provenance is a separate axis.** ~36 of the 153 ledgered rows fail on
provenance (`body result of unknown provenance`, `operand of unknown
provenance`), and `COMPILE-REFUSAL-SURVEY.0.md` §3a already established that
this class is not a dispatch problem: *"`depth` alone — no overloads in play,
nothing ambiguous — still refuses … what this needs is a lowering that models
the word's output as an event with provenance."* §4.2 establishes that an island's own
OUTPUT already has an address — `setProduced` registers it — so nothing there
helps these rows: their missing address is on the **operand** side, and a result
contract says nothing about operands. This note does not claim that family, and
a reader should not read the 7 + ~11 in §5 as a dent in the 36.

**And the cost: two lanes that must agree is a bug detector.** NUR101 was found
*by* the disagreement between them — and there the compiled lane was correct and
the interpreter was wrong. An escape makes divergence impossible for its span by
making one lane authoritative, which is only a win when that lane is right.
Widen island use and the differential gate stops finding things, because there
is less that differs. That argues for typed islands as a **coverage** tool with
the differential corpus deliberately kept on the natively-lowered paths — not
for collapsing to one semantics.

---

## 5. Staging

| Stage | Work | Rows moved | Risk |
|---|---|---:|---|
| **0** | Delete `CompileExecutesBody` (0 sites) | 0 | none |
| **1** | Introduce the triple; derive it from today's flags; assert equivalence over every registered signature | 0 | low — mechanical, gated by the existing differential corpus |
| **2** | Flip the recorder to read the triple; retire the ten flags | 0 | low |
| **3** | Add the result contract; make `OpFallback` results typed | ? | medium — changes what downstream sees |
| **4** | Re-tier `islandCeiling` from a correctness gate to a perf budget; graduate the 7 working-but-ledgered rows | **7** | medium — needs a perf story, not just a correctness one |
| **5** | Declare result contracts for `boru:fn-util`; graduate its rows behind §5.4 | **~11** | depends on NUR101 |

Stages 0–2 are a refactor with no behaviour change, and they are worth doing on
their own: they are what stops the taxonomy producing a fourth fn-util. Stage 3
is the real decision and should not be taken on this note alone.

---

## 6. What would falsify this

- **If most operand-facing flags do not map cleanly onto the triple.** The table
  in §4.1 is by inspection of the flag doc comments, not by implementing it.
  Stage 1's equivalence assertion over every registered signature is the real
  test, and it is cheap; run it before Stage 2.
- **If a typed island result does not actually stop the cascade.** §4.2 argues
  the cascade is purely a typing failure, on the strength of `setProduced` and
  `AnyDynamicCarrier`. The cheap check, before any implementation: instrument
  `recordCallRefusal` to log, per refusal, whether the offending operand lacked a
  *type* or an *address*. If a material share is address, §4.2 is worth much less
  than claimed and Stage 3 should not be taken.
- **If `dynOutNativeOK`'s exclusion of `CompileFallbackBody` turns out to be
  load-bearing** rather than conservative. §4.2 reads that line as screening by
  flag rather than by argument; if there is a soundness reason recorded elsewhere
  that a body-islanding word's dynamic output is *not* merely a declared-Any
  return, the extension in §4.2 is wrong at its root. Check the history on that
  line first.
- **If the 7 island rows are slow enough to matter.** They are ordinary `each` /
  `fold` / `scan` calls; if an islanded callback is dramatically worse than an
  interpreted whole program, then keeping them refused is the right call and
  §4.2's premise is wrong. Benchmark before Stage 4.

---

## 7. Related

- `design/COMPILABLE-SUBSET.md` — the positive statement of the current subset;
  §2 (provenance) and §6 (execution-environment seams) are the load-bearing
  sections for this note.
- `design/COMPILE-REFUSAL-SURVEY.0.md` — the prior measured survey. Its question
  was narrower ("would a signature-matching opcode remove the refusals?") and
  its answer was no, for the reason §4.3 restates: the remaining refusals are
  not dispatch-resolution problems.
- `design/HIGHER-ORDER-FUNCTIONS.0.md` — the capability audit whose §1 programs
  are 65 of the 153 ledgered rows.
- `design/FUNCTION-VALUE-SCOPE.0.md` — the interpreter-side statement of the
  `env` axis in §4.1.
- `NUR.md` NUR101 — the §5.4 computed-fn wall that sits behind the fn-util
  declaration, and Stage 5's dependency.
