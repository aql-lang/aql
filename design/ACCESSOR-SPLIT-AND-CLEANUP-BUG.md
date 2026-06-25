# Accessor split (landed) + per-call cleanup over-pop bug (next session)

This note records the get/dot accessor split that landed this session and
hands off the one remaining bug it surfaced — a per-call frame-cleanup
over-pop — with a ready-to-use next-session prompt.

## ✅ RESOLVED (branch `claude/dx-driven-language-improvements`)

The over-pop is **fixed**, but the root cause is **NOT** the teardown over-pop
hypothesised below — it is an **install-time over-write**. When a fn-frame
installs a param/capture whose value is a function and whose name+signature
**overlap** a live caller binding (the classic comparator threaded in via a
`/r`-parked arg), `InstallDef`'s same-scope overlap-redefinition **filter**
(`core_helpers.go`, the `FnDefsOverlap` block) DROPPED the caller's binding at
install time, collapsing two shadow levels into one; the normal `undef` tail
then popped the survivor to depth 0. Decisive evidence: with two function
values whose signatures do **not** overlap the program runs fine; flipping only
to an overlapping value crashes — so the trigger is the overlap filter, not the
teardown. The `undef`-tail / snapshot-after-install / `teardownFrameState` are
all correct and symmetric once install pushes a real shadow level.

**Fix (smaller and safer than the "depth-precise teardown" direction below):**
a new `InstallFrameBinding` (`core_helpers.go`) installs frame params/captures
by **shadowing** (push a fresh DefStack level) instead of running the overlap
filter — the filter is correct for the `def` word (same-scope redefinition) but
wrong for a nested frame, which must stack and tear back down to the caller's
level. The six frame-install sites (`core_helpers.go` `buildFnBodyHandler`,
`engine.go` `execFnDefSig`, `registry.go` `CallAQL` — capture + param each) now
call it; `InstallDef` (the `def` path) keeps the filter. No change to the
teardown mechanisms or the capitalised-name type-retire path.

Note the repro's expected value is **14**, not 13: `g(5)+g(7) = 6+8`. Regression
tests: `lang/go/test/closures_test.go` (`TestFrameParamShadowsCollidingFunctionParam`
positive across both install paths + `TestDefWordOverlapStillRedefines` negative
guarding the `def` filter). The recursion-via-Function-param case (R4 in the
client reports) is a **separate** TCO eager-teardown defect and is NOT fixed by
this change — leave it for a dedicated session.

The original handoff is kept below as the record.

## Landed this session (branch `claude/aql-client-lib-issues-lev8cs`)

1. **`slice` element-type preservation** — `slice` no longer stringifies
   scalar list elements (`[3 1 2] slice 0 2` keeps `Integer`). Fix in
   `native/helpers.go` (`valueToSliceArg` boxes element Values verbatim).
2. **Mixed-arity gradual dispatch (compile)** — `set` over a dynamic
   receiver in paren-tail position now models one gradual value under a
   real compile, so `def k2 ((nd "a" get) set "x" 1)` compiles
   byte-identically instead of refusing (`carrier.go` `tailConsumed`).
3. **get/getr → evaluate-key; new dot/dotr → literal-key.** The `.` / `!.`
   sugar lowers to `dot` / `dotr` (`lowerReach`). `get`/`getr` dropped
   their QuoteArgs atom sigs (so `lst get i` evaluates `i` — fixes the
   §2.1 silent-None client report). `dot`/`dotr` carry the full sig set;
   both share `accessorGetSignatures`/`accessorGetrSignatures` with
   `get`/`getr` using the `dropQuoteAtomSigs` subset. Compiler/checker/VM
   fold sites route through `isGetWord`/`isGetrWord` (`eng/go/get_words.go`)
   so dot-access keeps full compiled coverage.
4. **getr/dotr over class instances** — added the `TClass` sigs so
   `(make P {})!.x` works (was a pre-existing gap).
5. **Coverage + docs** — new `lang/spec/accessor.tsv` (the cross-type
   contract for get/getr/dot/dotr/./!.); `pinnedUnflaggedErrorRows`
   178→183 (accessor strict-miss negatives are runtime-only, documented);
   `COMPILED_STATUS.md` regenerated; help/describe + `lang/go/CLAUDE.md`
   updated.

All gated: `make test`, `verify-bytecode`, `crossdiff` (1787 rows, 0
divergences), `test-ts` (3622), langspec census/status/coverage/accuracy.

## The remaining bug — per-call cleanup over-pops a colliding param

**Symptom** (client report §1.1, root-caused tighter than the report):

```aql
def g ([x:Integer] => [x add 1])
def h fn [[comp:Function v:Integer] [Integer] [v comp/r apply]]
def t fn [[comp:Function] [Integer] [ def a (5 comp/r h)  def b (7 comp/r h)  a add b ]]
print ((g/r t))     # => error: undefined word: comp  (should be 13)
```

**Root cause.** Each `comp/r h` call over-pops `comp` by one extra shadow
level. Confirmed by tracing the def-stack: `comp` installs at depth 1
(caller `t`'s param) then depth 2 (callee `h`'s param), and the teardown
pops it to **0** instead of 1 — destroying the caller's binding. The
per-call cleanup tail tears params down with `undef <name>`
(`AppendFrameTail`, `fn_frame.go`), which pops the **top** binding of the
name; by the time it runs, the callee's own `comp` has already been removed
by another path, so `undef comp` pops the **caller's** `comp`.

**Required trigger:** (a) callee param name collides with a live caller
binding, AND (b) the arg is a `/r`-parked function value. Plain tail
recursion (where the name also shadows) works, because the frame is
genuinely replaced. Genuine tail-position calls work (the caller frame is
dying anyway); only **non-tail + reuse** errors — which is why the report's
"box pattern" (bind to a *differently-named* local) sidesteps it.

**Files in play:** `eng/go/core_helpers.go` (`buildFnBodyHandler` — snapshot
is taken AFTER param install, line ~225), `eng/go/fn_frame.go`
(`AppendFrameTail` emits the `undef <name>` tail), `eng/go/engine.go`
(`stepDefCleanup` — snapshot-based truncation), `eng/go/fn_frame_elide.go`
(`teardownFrameState` — the eager TCO twin). The two teardown mechanisms
(snapshot-truncate for body-locals; `undef`-tail for params) can desync.

**Fix direction (sound, but high blast radius).** Make param teardown
**depth-precise** instead of "pop top by name": take the DefCleanup
snapshot BEFORE installing params so `stepDefCleanup` truncates params to
their pre-frame depth exactly (immune to mid-body pops), and drop the
fragile `undef`-tail — BUT preserve `undef`'s **type-retire** path for
capitalised param names (a `def X` param Retires its minted lattice node;
plain truncate would leak it — see the `tcoEligible` "capitalised name
takes undef's type-retire path" comment). The eager TCO twin
(`teardownFrameState`) must stay in lockstep.

**Why it was deferred:** this is the per-call cleanup path that runs on
*every* function call and is entangled with TCO elision, closure capture,
and type-retire. It must be landed with the FULL gate suite (recursion /
closure / loop suites, TCO regression tests, `verify-bytecode` property
fuzz, `crossdiff`, `test-ts`), which needs a dedicated budget.

---

## Next-session prompt (paste this to start)

> Fix the per-call frame-cleanup over-pop documented in
> `design/ACCESSOR-SPLIT-AND-CLEANUP-BUG.md`. Repro:
> `def g ([x:Integer] => [x add 1])  def h fn [[comp:Function v:Integer] [Integer] [v comp/r apply]]  def t fn [[comp:Function] [Integer] [ def a (5 comp/r h)  def b (7 comp/r h)  a add b ]]  print ((g/r t))`
> currently errors `undefined word: comp` (should be 13). The callee's
> param `comp` teardown pops the caller's same-named `comp` because the
> `undef <name>` cleanup tail pops the top binding rather than the frame's
> own level. Make param teardown depth-precise (pre-param-install snapshot
> + truncate via `stepDefCleanup`, keeping the capitalised-name type-retire
> path), and keep the eager TCO twin `teardownFrameState` in lockstep.
> Add a positive+negative regression test (callee param name colliding with
> a caller param, via a `/r`-parked fn arg, reused after the call), plus a
> spec row. Gate with `make test`, `verify-bytecode`, the recursion /
> closure / TCO suites, `crossdiff`, and `test-ts`; nothing may regress.
> Develop on `claude/aql-client-lib-issues-lev8cs`.
