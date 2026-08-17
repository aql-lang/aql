# FN-VALUE-OPEN-WORK — what is left of the function-value line

> **Status:** Open-work inventory. Four items remain from the `/r` survey and
> the StackForm recorder work (PRs #366, #375, #378). Two need a maintainer
> ruling and cannot be started without one; two are unblocked engineering.
> Every number here was re-measured against `8732662` — the figures the
> earlier notes carry were taken before several merges and **three of them
> were wrong**. §6 lists the corrections so the stale ones stop propagating.

## 1. The four items, and who is blocking each

| # | Item | State | Blocked on |
|---|---|---|---|
| A | **Clause 3** — "parens do not re-step" | Measured three ways. A discriminator was **found** that the earlier note said did not exist. | Maintainer: pick narrow or broad (§2) |
| B | **Clause 2** — passing a function requires `/r` | Mechanism located (4 sites, ~56 lines). Blast radius **3 rows**, not 9. | Maintainer: amend ADR-011 (§3) |
| C | **Break 2** — compiler refuses a 0-arg fn read from a plain container | Refusal is **sound**; it masks a confirmed miscompile. The mechanism to close it already exists and ships at arity 0. | Nobody — unblocked (§4) |
| D | **NUR077** — a StackForm cannot apply a function value | Design sketch tested. `apply` works as the target but has two holes. | Nobody — unblocked, but see §5 before building |

A and B are language decisions. C and D are engineering, and C is the smaller
and better-understood of the two.

## 2. Item A — clause 3, and the discriminator that was missed

**The ruling.** 2026-08-16: *"parens do not restep, they place a value or
values on the forward stack. Thus `(ref z)` is the same as `ref z`, `(z/r)` is
the same as `z/r`"*, plus a separate ruling that `inc/r 5` is **two values**.

### 2.1 The edit is one line

`core/go/engine.go:7764` reads:

```go
park := e.fnReturnPark(openIdx, closeIdx, wasFrameOpen)
```

Broad clause 3 = pass a condition that ignores `wasFrameOpen` and excludes only
a reach-lowered group. `fnReturnPark` (`core/go/engine.go:6331-6340`) needs no
change: its guard is `if !wasFrameOpen || …`, so handing `!wasReachGroup` into
that slot converts it exactly.

The `ReachGroup` exclusion is still load-bearing. Without it `MathUtil.sqrt 16`
fails, because dot access lowers to `( MathUtil dot sqrt )` and its re-step
*is* the dispatch.

**One hazard the earlier measurement could not see.** Since #378, `park` also
feeds the recorder-skip accounting (`survived := -park`,
`core/go/engine.go:7778`). Broadening the park therefore silently broadens that
adjustment to every user paren — and `TestSpecProd` installs no recorder, so
the corpus measurement below **does not exercise that half**. Whoever
implements this must measure `TestSpecCompiledDifferential` and the stackform
round-trip suite too.

### 2.2 The corrected count: 30 rows, not 31

Broad, measured on `8732662`: **30** failing rows across 9 files — apply 1,
corpus-core 1, corpus-structures 1, fn-triple 5, modifiers 9, module-fmt 1,
recursion 1, ref 7, usurp 4.

Of those, **7** are `ref.tsv` §2 + row 43, which the ruling voids by design.
The other **23** are not.

The earlier note's 31 is explained rather than merely corrected: applying the
same edit at `411362a` also yields 30, and appending the drafted `((h z/r))`
row that §12.6 says was *dropped* yields exactly 31. The old figure counted a
row that never landed.

### 2.3 The finding that changes the recommendation

`design/FUNCTION-VALUE-SCOPE.0.md` §12.6 concluded that narrow and broad are
"not separable by any positional mechanism", and therefore that narrow needs a
transient value-borne marker — which clause 1 (`/r` is not sticky) rejects.

**The positional half is confirmed.** An instrumented probe in `fnReturnPark`
shows an inline fn literal and a `/r` / `ref` / `usurp` reference arriving
bit-identically: same index, same `closeIdx == idx+2`, same
`frameOpen=false reachGroup=false quoted=false isTFunction=true`. Both
`stepWordRef` (`core/go/engine.go:2567`) and the `ParkResult` arm (`:3551`)
merely advance the pointer.

**But one payload field separates them, and it was missed: `FnDefInfo.Name`**
(`core/go/value.go:720`). A reference carries the name it resolved; an inline
literal does not.

```
(z/r)                            → Name "z"
(ref z)                          → Name "z"
(inc/r) 5                        → Name "inc"
def f ([n] => [n add 1]) (f/r) 5 → Name "f"      ← a bound lambda still has one
(fn Integer [Integer] [10 add]) 7 → Name ""
([n:Integer] => [n add 1]) 5      → Name ""
```

Measured, narrow-by-name: **24 failing rows — it recovers exactly the 6
inline-application rows** (`corpus-structures:40`, `fn-triple:41`,
`fn-triple:56`, `modifiers:111`, `module-fmt:78`, `recursion:48`) **with zero
new failures**, and the kernel `engspec` suite stays green. The remaining 24
are all `(reference) args`, the construct `ref.tsv` §2 pins — voided by design.

`Name` is not positional, but it is **not the mechanism clause 1 rejects**
either. It is intrinsic, set at definition, never written by `/r`, and cannot
leak because nothing mutates it. The recorded objection to narrow does not
apply to it.

Narrow's real cost is different and was measured: paren-application of a
container-fetched function would depend on whether the stored function happens
to remember a binding name. `(ops get "f") 5` where the map holds `inc/r`
currently gives `6` and would give `fn inc(Integer) 5`; the same shape holding
an anonymous lambda would still apply. That spelling is **not in the corpus** —
`ref.tsv` §3 uses dot access (`ops.f 5`), which is a ReachGroup and works under
all three builds.

### 2.4 The decision, stated as a choice

| | `(z/r)` | `(inc/r) 5` | `(fn …) 7`, `([n] => …) 5` | corpus rows to change |
|---|---|---|---|---|
| today | `42` | `6` | applies | — |
| **broad** | `fn z` ✅ | two values ✅ | **two values** — idiom removed | 30 (7 by design) |
| **narrow, by `Name`** | `fn z` ✅ | two values ✅ | **applies** — idiom kept | 24 (all by design) |

Narrow now looks strictly better than when §12.6 was written: it satisfies
every example in the ruling, keeps inline application, changes fewer rows, and
needs no rejected mechanism. Broad is what the ruling says *literally*, and
costs the inline-application idiom.

**Either way, the compiler needs a matching change.** It does *not* already
behave as clause 3 prescribes: `(z/r)` → `42`, `(ref z)` → `42`,
`(inc/r) 5` → `6`, `(fn Integer [Integer] [10 add]) 7` → `17` — identical under
`--no-compile` and `--force-compile`. Clause 3 is not "already implemented
compiled".

### 2.5 Clause 3 does not fully close NUR073

NUR073 says *"Whichever is chosen closes this record, since both readings give
`fn z`."* **That is wrong, and this note corrects it.** Measured: both readings
give **`fn f`**, not `fn z`; the compiler gives `fn z`.

The *value* divergence does close — under narrow,
`canon ((h z/r))` equals the compiled answer, `typeof` is `Function` on both,
and `deq ((h z/r)) z/r` is true. What survives is a **name-rendering**
difference: the interpreter's value carries `Name == "f"` because `f/r`
resolves the *param* name through `ResolveRef` → `Lookup("f")`
(`core/go/core_ref.go:26-50`), while the compiler's carries `"z"`.

So a row pinned on the bare residual would still fail the differential; one
pinned via `canon` or `typeof` would pass. That residue is NUR074's class of
problem (canon already strips the name; the residual renderer does not), not
clause 3's.

## 3. Item B — clause 2, and a much smaller amendment than recorded

### 3.1 The amendment is surgical

`design/FUNCTION-VALUE-SCOPE.0.md` says clause 2 "supersedes ADR-011's final
sentence". That is imprecise in a way that matters. The final sentence is a
four-clause semicolon list, and clause 2 **agrees with its first clause** —
*"A bare name bound to a function calls"* (`ADR.md:213-214`) — contradicting
only the **fourth**: *"a bare fn name before a `Function`-typed slot resolves
as a reference"* (`ADR.md:215-216`).

ADR-011 already carries both the rule and its exception. Clause 2 deletes the
exception so the rule applies universally. **The amendment strikes
`ADR.md:215-216` from the `;` to the period** — nothing else in ADR-011 moves.

*(ADRs are amended only on explicit maintainer instruction, so this is recorded
here rather than edited in.)*

### 3.2 Four sites, not two

| | What | Extent | Prod callers |
|---|---|---|---|
| A | `stepWord`'s TFunction intercept | `core/go/engine.go:2719-2742` | — (it *is* the site) |
| B | `hasPendingForwardExpectingFunction` | `core/go/engine.go:8071-8089` | 1 (`:2722`) |
| C | `sigWantsFunctionAt` | `core/go/engine.go:6435-6447` | 2 (`:6428`, `:8428`) |
| D | inline `ConformsTo(TFunction)` on the ReachGroup arrival path | `core/go/engine.go:4012` | — |

~56 lines plus one condition, one file, 4 production and 5 test call sites.

**Two things make it cost more than the line count.** First, B tests
`.Equal(TFunction)` while C and D test `.ConformsTo(TFunction)` — retiring A+B
and leaving C+D would leave the bare-name spelling working through dot access
and failing for a plain bare word, which is worse than either endpoint.
Second, **C is NUR038 machinery, not clause-2 machinery**: its comment says an
`Any` slot "is NOT a Function slot: Any also admits a fn value, but as a
swallowed call head, which is exactly the misfire the barrier exists for".
Retiring C re-opens the NUR038 call-head barrier question, which is a different
problem.

There is no separate checker or compiler implementation — `check/` runs the same
core engine. The compile-side analogue is the per-slot `FnInertArgs` marker
(`core/go/value.go:334-342`).

### 3.3 The blast radius is 3 rows, not 9

`:Function` slots today: **22 rows** (18 top-level + 4 frontier). The earlier
"20" reproduces exactly at the commit that wrote it — but only under git-grep
pathspec semantics, which recurse into `lang/spec/frontier/`; a shell glob gives
18. Today's +2 is `ref.tsv:81` and `:83`, both already using `/r`.

**"9 of them bare" does not reproduce under any definition.** Enumerated:

| Definition | Count |
|---|---|
| bare undotted fn **name** | **3** |
| including dotted reads (`A.pub`, `mm.run`) | 5 |
| "no `/r`" | 12 — but this sweeps in 7 anonymous-lambda rows with no name for `/r` to attach to |

The defensible answer is **3**, and only **one** (`path-modifier.tsv:67`) is in
the main corpus; the other two are already in the frontier divergence ledger.
Clause 2 is a far cheaper corpus change than recorded.

### 3.4 The coupled fix

The `unused_def` fix hooks the same intercept — it added a `noteAnalysisUse`
call so `boru check` stops reporting `unused_def` for a fn handed bare to a
callback API. **It is commit `36ba1a2`, merged by `7e98aeb` = PR #366**, not
#375 as stated elsewhere. Retiring site A means re-homing that use-recording;
`lang/go/fnslot_unused_def_test.go` pins both directions and would need to move
with it.

## 4. Item C — break 2 is unblocked, sound, and narrower than recorded

### 4.1 The refusal is sound

Two sites, identical text, both in `compiler/go/emit.go` — `:4647` (mono, a
`recordCallRefusal` arm) and `:5064` (poly, a guard in `RecordPolyCall`).

**Disabling both guards was measured**: with the guard off, `def z fn
[[][Integer][42]] def m {k: z/r} (m.k)` compiles and returns **`fn z` instead
of `42`** — a silent wrong value, no error. The read lowers to a bare
`CALL_NATIVE_POLY dot/2` and the program ends there, leaving the `FnDefInfo` on
the stack. The VM has no on-land dispatch of a surfaced fn value; the
interpreter has one (the 0-arg courtesy dispatch, `core/go/engine.go:682`).
Removing the refusal reinstates miscompile mechanism E.

So this is a **sound guard to be replaced, not removed**.

### 4.2 The scope is one shape, not a family

The trigger is `core.FnValueZeroArg` (`core/go/value_classify.go:76`) — a
**genuine 0-arg overload**, not "a fn value" generally. The survey's phrasing
covers three adjacent families that all compile today:

- plain-container member, **arity ≥ 1** → `CALL_DYN_METHOD ; inc/1`
- **shaped instance** member, arity 0 → `CALL_DYN_METHOD ; rand-bool/0`
- **module export**, arity 0 → compile-time resolution, `CALL_USER ; z/0`

The live hole is precisely: a **plain** (non-module, non-delegation) container
holding a **user fn** with a genuine 0-arg overload. `(m.f 5)` with a 1-arg fn
is **not** an instance of break 2 — it compiles natively in every mode.

### 4.3 It is not Phase 3, and the mechanism already ships

`OpDispatchRematch` is the wrong instrument: per its opcode doc
(`compiler/go/bytecode.go:150-167`) it re-runs a statically-**failed** dispatch
expecting a no-match, and it is **terminal**. Break 2's dispatch does not fail —
the read succeeds and execution must continue.

The right mechanism is `OpCallDynMethod` (`compiler/go/bytecode.go:378-395`),
the guarded mid-stream apply, past which "the program CONTINUES … with NOut
results committed downstream". It already ships at arity 0 for shaped
instances (`check/go/method_shape.go:217-233`).

Break 2 is therefore best characterised as an **arity-0 extension of
`tryMemberFnArrivalDispatch`** (`check/go/method_shape.go:452`). Two one-line
gates keep it out, both documented as deliberate scope limits:

- `check/go/method_shape.go:481` — `sig.TotalArgs() < 1`, annotated "arity >= 1
  (a 0-arg auto-fire is the read-guard's own class)"
- `core/go/check_state.go:1122` — `if !IsDelegationFnDef(fd) { return }`, so a
  plain user fn is never annotated

And that function's own comment already says the VM half works for a user fn:
"a plain user fn takes the island path, byte-identical to the interpreter's
auto-dispatch" (`:512-515`).

### 4.4 It is untracked, which is its own finding

`knownRefusals` is **empty** (`test/go/langspec/compiled_refusals_test.go:29`)
and the main corpus has **0 refusals**. Break 2's shape is absent from the main
corpus and from every frontier TSV. It is pinned only by a
*refusal-preserving* unit test
(`lang/go/bytecode_findings_test.go:3597 TestFnValueAutoApplyRefusals`), which
locks the refusal in rather than tracking it for closure.

**First step for whoever takes this: add a frontier row**, so the shape has a
graduation target instead of a lock.

## 5. Item D — the apply Op, and two holes in the sketch

The record's sketch is an apply-style Op that consumes the fn value, which
`Flatten` serialises using boru's existing `apply` word. Tested:

**`apply` works as the target on first replay.** Verified for every arity and
origin tried, including a `Quoted`-stamped harvested value and a fn read out of
a map, both as source postfix and as a hand-built form run through
`stackform.Eval`. The argument order also lines up — Flatten already pushes
deepest-first so `sig[0]` lands on top, which is `apply`'s stack-order
convention.

**`DoEval` cannot carry it.** `Flatten` maps `DoEval` to the word `do`, whose
signatures are `TList`/`TMap` only, and `do (inc/r)` is a signature error.
`DoEval` is also payloadless, so it cannot carry the arity `Pretty`/`Cost`/
`opEqual` need. Apply needs its own Op.

**Hole 1 — a 0-arg *anonymous* fn is silently not applied.**
`(([] => [42])/r) apply` → `[fn]`, not `42`; the named equivalent works. This
is the ADR-016 gate at `core/go/engine.go:5275`, which `lang/go/CLAUDE.md`
already declares a defect rather than a design. A Flatten built on `apply`
would reintroduce, for this one shape, exactly the quiet wrongness the refusal
was installed to prevent.

**Hole 2 — `apply` is not itself faithfully recordable.**
`777 inc/r apply` records **both** `Call{apply,1}` and the induced
`Call{inc,1}`, because `applyHandler` returns the fn as a value and the
engine's re-step of it is recorded as an ordinary named call. Replay applies
twice: direct `778`, `Eval` **779**, `Replayable` nil. So `Compile ∘ Flatten`
would not be idempotent, and — independently of the Apply design — **the
recorder cannot faithfully record any program a user writes with `apply`
today.** There is no `apply` row in the round-trip corpus, which is why this
was not caught.

### 5.1 The seam location is not where the marker is now

The empty-name marker sits in `execFnDefSig` (`core/go/engine.go:6135`). The
**named** container shape never reaches it — `execFnDefLiteral` looks a named fn
value up by name (`:5158-5163`) and dispatches it as an ordinary word. The two
shapes differ *only* by `fnDef.Name != ""`.

So an `OnApply` placed where the marker is today would leave the named shape
exactly as broken. The event has to fire in **`execFnDefLiteral`**
(`:5054`) — the only site that knows the callee arrived as a value and holds it.

### 5.2 What the Apply Op does *not* fix

The applied function's **argument literals are dropped**, at every arity, in
every application shape. That is a **skip-accounting over-count**, not a
vocabulary gap: a Function reached at the pointer never fires `OnPushLit`, yet
both the producing call's `returns` count and `stepCloseParen`'s
`RecorderSkipper` credit a skip for it. Each paren nesting level adds another
unspendable credit, and the surplus eats the next real literal. Control: with a
non-function paren result the balance is exact at every depth.

Same class as the frame-skeleton over-count fixed in `9bffdd3`, different
instance. **Fix this before or alongside the Op, not after** — the Op cannot
make these forms faithful on its own.

### 5.3 Three shapes are still silently wrong

The record's "Eval REFUSES rather than replays; loud beats quietly wrong" holds
for the three pinned shapes but **not for the family**. These are accepted by
`Replayable`, raise no error, and replay to a different stack:

```
(([] => [42])) 777                              direct [fn 777]   Eval [fn]
def z fn [[] [Integer] [42]] (z/r) 777          direct [42 777]   Eval [fn z 42]
def inc … 777 inc/r apply                       direct [778]      Eval [779]
```

The middle one is NUR077's own "strands the function and produces it twice"
happening literally and silently, for a **named 0-arg fn with no container and
no lambda**. `(inc/r) 777` and `(sub2/r) 700 8` are accepted-but-erroring in
the same class.

The one production consumer is partly shielded by accident: the shrinker maps
any `Eval` error to `shrink.Invalid` and falls back to value-level shrinking.
The three **silent** shapes get no such protection — they yield a plausible
value and the reducer trusts it.

## 6. Corrections this note makes to existing records

Recorded here so the wrong figures stop propagating.

| Where | Claim | Corrected |
|---|---|---|
| `NUR.md` NUR073 | "both readings give `fn z`" | Both give **`fn f`**; the compiler gives `fn z`. Value divergence closes; a name-rendering difference survives (§2.5) |
| `FUNCTION-VALUE-SCOPE.0.md` §12.6 | narrow and broad "not separable by any positional mechanism" | Positionally true, but **`FnDefInfo.Name` separates them** and is not the rejected mechanism (§2.3) |
| §12.6 | 31 spec rows | **30**; the 31 counted a drafted row that never landed (§2.2) |
| §12.4 | "20 rows, 9 of which pass a bare name" | 22 today; the bare count is **3**, only 1 in the main corpus (§3.3) |
| §12.6 / §12.4 | clause 2 "supersedes ADR-011's final sentence" | It contradicts only the **fourth clause** of that sentence and agrees with the first (§3.1) |
| §12.6 | the `unused_def` fix landed in `7e98aeb` (PR #375 in later prose) | `36ba1a2`, merged by `7e98aeb` = **PR #366** (§3.4) |
| break-2 prose | "fn value read from a container" | Only a **genuine 0-arg overload** on a **plain** container; three adjacent families compile today (§4.2) |
| break-2 prose | closing it is Phase 3 / `OpDispatchRematch` | `OpDispatchRematch` is terminal and wrong for it; the mechanism is `OpCallDynMethod`, already shipping at arity 0 (§4.3) |

### 6.1 Stale claims elsewhere in the tree

Found while measuring break 2, and **not fixed here** — each asserts something
the code no longer does:

- `test/go/langspec/compiled_metafallback_test.go:129` — says 4
  container-auto-dispatch rows "stay REFUSED … permanent unless a runtime
  auto-dispatch model is built". All four compile on main; the model was built.
- `design/STAGE3-INLINING-DESIGN-ROUND.0.md:45`, `:218-220` — the same stale
  accounting.
- `check/go/method_shape.go:48-52` — the file header says the get-family read
  guards "still refuse those reads outright before any model could run", while
  `:217-233` of the same file now models exactly those.

## 7. Suggested order

1. **C (break 2)** — unblocked, smallest, best understood, and it starts by
   adding a frontier row that gives the shape a graduation target.
2. **D's over-count half** (§5.2) — independent of the Op, same class as a fix
   already landed, and it is the part that is silently wrong today.
3. **A (clause 3)** once ruled — and note §2.1's warning that `park` now feeds
   the recorder skip, so the compiled differential must be measured too.
4. **B (clause 2)** once ADR-011 is amended — cheap in the corpus (3 rows), but
   it must retire sites A+B+D together and decide C's NUR038 question
   separately.
5. **D's Op** last: it needs a seam decision (§5.1), and two holes closed
   (§5) that are defects in their own right.

## 8. What is not verified

Stated so the next reader does not inherit these as settled:

- Whether **every** application path passes through `execFnDefLiteral`. Traced
  paths do; macro values, module-wrapper trivial delegation and foreign
  sub-registry exports were not enumerated.
- Whether `apply`'s double-recording can be fixed without new engine state.
  `applyHandler` returns a plain value and the re-step is indistinguishable
  from any other dispatch.
- Whether the three silent divergences are reachable from a real PBT generator
  body. `Debug.disasm` exposes the recorder to arbitrary user source, so the
  exposure is not zero either way.
- The ADR-008 cost of an optional-interface recorder seam. Reasoned, not
  measured.
- ADR-011 says "the TS twin moved in lockstep"; this tree contains no `.ts`
  files, so the port's state is unchecked.
