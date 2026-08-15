# FUNCTION VALUE SCOPE

Where a function value's **free words** resolve — the module that *defined*
it, or the module that *runs* it — and why boru currently answers both,
depending on which engine executes the program. Suffix `.0`: design and
defect report; no fix is implemented.

**Status:** defect + design. The behaviour below is a **compile ≠
interpret divergence that can return a silently wrong number**, verified
first-hand against `boru 0.1.0-dev (git 96bf5f9e)`. It is not recorded
anywhere else in `design/` — the closest prior mentions
(`FN-VALUE-DISPATCH.0.md` §, `VOXGIG-COMPILE-LEAVES.1.md`) are
compile-time emitter findings of the same *shape* but a different
problem. This is the first statement of the runtime rule.

## 1. Summary

A function value that crosses a module boundary should keep access to the
module that defined it. In boru it does — **on the bytecode compiler**.
On the interpreter it does not: the body's free words are resolved in the
*running* module, so a module-private helper is either not found, or —
worse — silently bound to a **same-named word in the calling module**.

```boru
# mod-a.aql — `pub` calls A's private `secret` (x + 1)
# mod-b.aql — B has its OWN `secret` (x * 100), and applies a Function param
print (B.apply1 A.pub 5) end
```

| Engine | Result | |
|---|---|---|
| `boru --no-compile t3.aql` | **`500`** | ran **B**'s `secret` — silently wrong |
| `boru --force-compile t3.aql` | **`6`** | ran **A**'s `secret` — correct |
| `boru t3.aql` (default) | `6` | whichever path the program happens to take |
| `boru check t3.aql` | **0 errors, 0 warnings** | statically invisible |

The default mode compiles when it can and *silently falls back* to the
interpreter when it cannot, so **the same source can produce either
answer depending on whether an unrelated part of the program happens to
be compilable.** No diagnostic fires in either direction.

This is the outcome the repository's own tests call the cardinal
forbidden one (`lang/go/bytecode_stored_handler_freeze_test.go`: "a
compile ≠ interpret MISCOMPILE (the cardinal forbidden outcome)"), and it
is the same class as the stored-handler defect fixed in
`RELOAD-INVALIDATION.0.md` §3 — with one aggravating difference: that one
produced a *wrong-but-explicable* value from hoisted state, while this
one can run **an entirely different function than the one written**.

**The mechanism for the correct behaviour already exists and is already
set.** `FnDefInfo.Registry` holds the defining module's registry, and its
doc comment states the intent verbatim (`core/go/value.go:697-700`):

> If Registry is non-nil, the function was defined in a module and should
> execute in that registry's context (closure semantics).

The defect is that only *some* dispatch paths read it. This is therefore
a **wiring gap, not a design flaw** — which is what makes it worth
fixing rather than documenting.

## 2. The rule, as measured

Three dispatch classes, three different behaviours:

| Class | How the value is invoked | Interpreter | Bytecode |
|---|---|---|---|
| **Value dispatch** | `A.pub 5`; an *unnamed* `Function` param applied directly | ✅ defining module | ✅ defining module |
| **Name dispatch** | bound to a name — `def g (A.pub/r)`, or a **named** `f:Function` param | ❌ **running module** | ✅ defining module |
| **Native callback — group 1** (6 sites) | `filter`, map-lambda `each`/`fold`, `StructUtil.walk`, core `walk`, `IO.mount`, `boru:parse` | ❌ **running module** | ❌ **running module** |
| **Native callback — group 2** (8 sites) | `RunPredicate`, service handlers ×2, codecs, `serve-raw`, `Model`, `Tui.run` update/view | ❌ **running module** | ✅ defining module |

> **Correction (§10 audit).** An earlier revision of this table claimed *all*
> native-callback sites were wrong on both engines. That holds only for
> group 1. Group 2 already diverges like name dispatch — and two of them
> (`net_codec.go:100`, `native_service.go:419,423`) are **verified
> miscompiles today, check-clean**. `RunPredicate` is the worst single
> site in the tree: `native_type.go:825-829` converts its failure into a
> silent `false` rather than an error, so a `refine` predicate that
> cannot resolve its own helper quietly rejects every value.

Two consequences worth stating separately, because they have different
severities:

- **Name dispatch diverges between engines** (§1). Silent, and mode-dependent.
- **The native-callback seam is wrong on *both* engines.** It is not a
  divergence — it is uniformly the *wrong* scope. This is the class that
  matters most for the library ecosystem, because every callback API in
  boru goes through it: comparators, predicates, service handlers,
  codecs, TUI update/view, `RunPredicate`.

Within a class the behaviour is uniform: invocation shape (`each`,
`fold`, `do`, nested fn) does not change the outcome, and the failure is
a catchable runtime error, not a static one.

**The blast radius on the interpreter is total.** A function value
invoked by name across a module boundary loses its module's private
helpers, its module-level values, **its module's other exports**, and any
namespace its module imported. Only built-ins survive.

**The leak is one-directional**, which is the one piece of good news: a
caller never gains the callee module's privates, and a caller's
same-named definitions never corrupt a library's own *by-name* internal
calls. Only values that cross the boundary are affected.

## 3. Reproduction

```bash
cd $(mktemp -d)
cat > mod-a.aql <<'EOF'
def secret fn [[x:Integer] [Integer] [ x add 1 ]]
def pub    fn [[x:Integer] [Integer] [ secret x ]]
export "A" { pub: pub/r }
EOF
cat > mod-b.aql <<'EOF'
def secret fn [[x:Integer] [Integer] [ x mul 100 ]]
def apply1 fn [[f:Function n:Integer] [Integer] [ (f n) ]]
export "B" { apply1: apply1/r }
EOF
cat > t3.aql <<'EOF'
import "./mod-a.aql" end
import "./mod-b.aql" end
print (B.apply1 A.pub 5) end
EOF

boru --no-compile   t3.aql   # => 500   ← B's secret. WRONG, no error
boru --force-compile t3.aql  # => 6     ← A's secret. correct
boru check          t3.aql   # => 0 error(s), 0 warning(s)
```

A second, louder shape — rebinding an export under a new name — fails at
**check** time on both engines, which at least is honest:

```bash
cat > t9.aql <<'EOF'
import "./mod-a.aql" end
def g (A.pub/r)
print (g 5) end
EOF
boru t9.aql   # => check: 2:37: [error] undefined_word: undefined word: secret
```

## 4. Mechanism

### 4.1 Where `Registry` is set

Only at module-export resolution — `resolveModuleExport`
(`lang/go/native/native_module_module.go:800-802`, and the same three
branches at `:830`, `:841`), with a local mirror in
`lang/go/modules/test.go:226-253`. The stated intent (`:792-795`):

> A function value — typically produced by `name/r` in the export map …
> must carry the module registry so it executes in module scope
> (resolving module-private words) when called after import.

Everything else merely propagates it by struct copy.

### 4.2 Which paths honour it

| Path | File:line | Honours `fnDef.Registry`? |
|---|---|---|
| `execFnDefLiteral` (value at the pointer) | `core/go/engine.go:5003` | **yes** — foreign-registry branch `:5277-5352` |
| `ExecFnDefSigStackMatch` | `core/go/engine.go:5474` | **yes** (pass-through) |
| `execFnDefSig` | `core/go/engine.go:5754` | **yes** — via its `capturedReg` parameter |
| trivial-delegation short-circuit | `core/go/engine.go:5250` | **yes** |
| VM `tryNativeFnApply` | `eng/go/vm.go:1113` | **yes** — and *declines* a foreign boru body so the island honours it |
| **`CallBoru` / `CallBoruNamed`** | `core/go/registry.go:1535` | **no — structurally cannot.** A method on a Registry; no `FnDefInfo` parameter exists |
| **`InvokeCallback`** | `core/go/invoke.go:65` | **no — structurally cannot.** Takes `(r, sig, args, captures)`; no `FnDefInfo` |
| **`InstallFnDef`'s handler** | `core/go/core_helpers.go:678` | **no** — closes over the *install-time* registry |
| VM `CALL_USER` | `eng/go/vm.go:1815` | different channel (`CompiledFn.Reg` = the analysis-pass registry) |

The `InstallFnDef` handler documents the drop as deliberate
(`core/go/core_helpers.go:235-238`):

> The handler closes over the install-time registry `r`: an FnDef
> registered into a name dispatches in the registry it was installed in.
> (The registry passed to the Handler at call time is intentionally
> ignored here — this mirrors the historical InstallFnDef closure.)

That comment is the whole bug in four lines. It was written about
*name registration*, and it is correct for a fn defined in the running
module — but it also swallows the case where the value arrived from
somewhere else carrying its own `Registry`.

### 4.3 Why binding to a name loses it

`A.pub 5` puts the export value at the pointer, where `execFnDefLiteral`
sees `fnDef.Registry` and routes to the foreign branch. But *binding* it
to a name — including binding it to a **named `Function` parameter** —
funnels through `installDef` → `compileFnSigs` → `buildFnBodyHandler(r,
…)`, which rebuilds the dispatch handler closed over the **installing**
registry. `entry.Registry` survives as a *field*, but the handler is what
`execMatch` actually calls, so the field is never consulted again.

The asymmetry is therefore: **applying a value keeps its scope; naming it
loses it.** A named `f:Function` parameter is the single most common way
a library receives a callback.

### 4.4 Why closure capture cannot fix it

`Captured` holds enclosing-fn locals only. `ComputeCaptures`
(`core/go/fn_capture.go:301-365`) skips any name whose
`Defs.Depth(name) <= baseline` — which is *precisely* the module-level
category:

| Reference resolves to… | Captured? |
|---|---|
| enclosing-fn param / body-local | **yes** — snapshotted |
| **module-level or global `def`** | **no** — dynamic lookup at call time |
| native word | no (never in `r.Defs`) |
| unbound name (forward ref, recursion) | no |

A module-private helper is exactly what `Captured` refuses to hold.
Only `fnDef.Registry` can carry it — which is why the fix has to be
wiring that field through, not extending capture (§7 option (b)).

### 4.5 Module registries are disjoint

`RunModuleBody` builds each module a **brand-new independent
`*Registry`** (`newSubRegistry`), inheriting host capabilities and I/O
plumbing by an explicit list — but **not `Defs`**. There is no parent
chain: `lookupUncached` reads `r.Defs` and nothing else. The two things
that look like a chain are not — `SetDebugParent` carries only the debug
hook, and `Contexts.PushExisting` shares only the `ctx-*` store.

So a lookup from the wrong registry cannot reach the right definitions
*at all*; it either misses, or hits a same-named word and returns the
wrong answer.

## 5. Not this bug — a separate, adjacent fault

While characterising this I checked a claim I had made elsewhere: that
the `sort` library's combinator failure shares this root cause. **It does
not**, and the distinction matters because the two need different fixes:

| | combinator fault | scope fault (this doc) |
|---|---|---|
| Symptom | `uncalled_function: call to 'by-number' matched no signature` | `undefined_word: <name>`, or a wrong value |
| Phase | **check / static**, exit 1, never runs | **runtime**, catchable |
| Mode-dependent | no — identical on both engines | **yes** (name dispatch) |
| Fix | add `/r` → works | `/r` is irrelevant |

`(Sort.by-number Sort.reverse)` fails because the bare namespace word is
**auto-invoked with zero args during forward collection** inside the
combinator call; `(Sort.by-number/r Sort.reverse)` works. That is an
arity/dispatch issue and belongs in its own record. It is noted here only
so the two are never again conflated.

## 6. The cost today

The workaround is "put everything in one module", and the ecosystem is
already paying for it:

- **`sort`** is a single 1,490-line file *by necessity*. Its header states
  the rule: "boru resolves a function value's free words in the module
  that runs it, so a comparator that calls a private helper must live in
  the same module as the algorithms that invoke it." Splitting the
  comparators into their own file made `Sort.natural` raise
  `undefined_word: natural-go`. A planned `then`-combinator was **dropped**
  because it would capture two functions, one of them cross-module.
- **`graph`** (the new pathfinding library) has had to specify that
  user-supplied **heuristics must be self-contained** — parameters and
  builtins only — which caps how much abstraction a caller can build
  around an A\* query.
- Generalised: **every callback API in boru inherits the constraint** —
  comparators, predicates, visitors, codecs, service handlers, TUI
  update/view. The library author cannot offer "pass me a function"
  without also saying "…but it may not call your own helpers."

That is a language-level tax on the exact abstraction — higher-order
functions across module boundaries — that first-class function values
exist to provide.

## 7. The fix

### 7.1 The option space

| Option | Mechanism | Verdict |
|---|---|---|
| **(a) evaluate in the defining module's registry** — wire `fnDef.Registry` through every path | Python `__globals__`; Lua `_ENV`; OCaml closures | **Ship this.** The field exists, is already set, and two of three classes already honour it |
| (b) capture module-level free words at construction | eager snapshot | **Reject.** Freezes the binding — kills redefinition, forward refs and hot reload; and a captured fn whose own body has free words just relocates the problem |
| (c) resolve word tokens to word objects at parse time | Factor / Forth XT / CL symbol cell | **Best endgame**, after (a). Name early-bound, definition late-bound; removes a per-call lookup |
| (d) caller-first, defining-module fallback | two-stage lookup | **Reject as semantics.** Preserves the capture bug and converts errors into silent wrong answers. Viable only as a temporary warn-and-continue instrument |
| (e) do nothing; export the helpers | documentation | **Reject.** Makes private helpers public — i.e. deletes the module boundary |

### 7.2 The semantic rule to adopt

> **A function value's free words resolve in the module that defined it,
> at the time it is called.**

Two halves, both load-bearing:

- **Lexical *module*** — *which* namespace is fixed at definition. This is
  what makes cross-module higher-order code work, and it is what the
  compiler already does.
- **Dynamic *within* that module** — *which definition* is found is a live
  lookup, not a snapshot. This preserves the documented module-level
  dynamic semantics (`lang/go/CLAUDE.md`: `def x 1; def f …; def x 2; f 0`
  → `2`), recursion via forward reference, and — critically —
  **hot reloading** (`HOT-CODE-LOADING.0.md`), which depends on a
  re-imported module's new definitions being seen by existing values.

Python is the exact precedent: a function object holds `__globals__`, a
reference to its **defining** module's dict, and `LOAD_GLOBAL` is a
lookup *in that dict at call time*. `importlib.reload` re-executes the
module into the *same* dict, so pre-existing function values see the new
helpers. That last detail is worth copying exactly — a reload that
*replaced* the registry would orphan every function value that pointed at
the old one, which is a constraint this design places on the `reload`
word proposed in `HOT-CODE-LOADING.0.md` §5.

The resulting asymmetry must be stated in the spec, not left implicit:
**module fns are lexically scoped; a fn defined in the running scope
continues to see that scope.** `t3.aql` is the discriminating case, and
the rule says the answer is `6`.

### 7.3 What changes

1. **`InvokeCallback` must take the `FnDefInfo`** (or the fn `Value`),
   not bare `(sig, captures)` — `core/go/invoke.go:65`. Its own doc
   already argues it is the right seam: *"the single seam every native
   callback word … dispatches through, so retiring the interpreter for
   reducible callback bodies is one routing decision rather than an edit
   per word."* Fixing it fixes the whole native-callback class at once:
   `filter.go:154`, `native_map_iter.go:110`, `walk.go:83`,
   `walk_core.go:165`, `io_mount.go:73`, `native_service.go:419,423`,
   `net_codec.go:100`, `net_socket.go:599`, `model.go:403`,
   `tui_run.go:375,418`, `registry.go:1895`.
2. **`buildFnBodyHandler` must prefer `fnDefCopy.Registry`** when it is
   non-nil and differs from the install registry
   (`core/go/core_helpers.go:678`). The narrower alternative is to widen
   `installDef`'s existing foreign-registry branch (`:80-115`) beyond
   trivial delegations, so a foreign boru-bodied fn keeps its *original*
   signatures — whose handlers are already closed over the right
   registry.
3. **`analysisFnConstructionPass` must be routed to the same registry**
   (`core/go/core_helpers.go:626`), or check keeps reporting
   `undefined_word` for code that now runs correctly — the `t9.aql`
   error above is exactly that pass.
4. **The compiler needs no separate edit** for name dispatch:
   `CompiledFn.Reg` is stamped from the registry the analysis pass ran
   the body in, so the compiled path follows automatically.

**Two further changes the audit found are required, which this plan
originally missed** (§10.4). Threading `FnDefInfo` is necessary but not
sufficient:

5. **`InvokeCompiled`'s freshness check must be re-anchored to the
   defining registry.** `core/go/invoke.go:79` validates
   `CompiledFnRef.depsFresh` against the *invoking* registry, which is
   why adding an unrelated `def` in another module can flip the answer.
   Threading `fnDef` fixes the interpreter fallback but leaves this
   check meaningless unless it moves too.
6. **The two competing stamp registries must agree.**
   `serviceAddHandler` (`native_service.go:249`) and `resolveCodec`
   (`net_codec.go:161-162`) stamp on the **running** registry, while
   `native_module_module.go:369` stamps on the **module** registry.
   After the fix these disagree, and the same source yields two scopes
   depending on which stamp won.

**Cost, measured:** 14 mechanical call-site edits. The audit checked the
warning below and **it does not materialise** — every one of the 14 sites
already type-asserts the `FnDefInfo` to reach `.Captured` and simply
discards the rest, so no site is blocked on having only a `*Signature`.

### 7.4 Hazards

- **This is a scoping-model change, not a local bug fix.** `installDef`'s
  comment states the current model plainly: *"A name bound as a local
  inside fn F is visible — via boru's dynamic scoping — to any fn F
  reaches on the call stack."* The change makes that false across module
  boundaries. It needs a spec row and a `lang/spec/*.tsv` battery, not
  just a patch.
- **Programs may depend on the current behaviour**, deliberately or
  accidentally. A caller that shadows a library's helper to influence it
  is relying on today's semantics; after the fix it silently stops
  working. §8 phases for this.
- **Check-mode and compiler invariants key on one `DefTable`** —
  `NoteFrozenRead`, `NotifyNameRebound`, and `CompiledFnRef` dep
  snapshots all snapshot a single registry's `Defs`. Option (d)'s
  two-registry cascade would make the freshness test unsound; option (a)
  does not, because each value still has exactly one resolution scope.
- **Performance.** Option (a) adds a pointer per value and a frame field
  per call; per-name lookup cost is unchanged. Option (b) would instead
  make every call O(module size) — another reason to reject it.

## 8. Phasing

Modelled on the Emacs lexical-binding migration, which is the closest
real precedent for flipping a scoping default in a live ecosystem:

- **Phase 0** — no semantic change. Make the divergence *visible*: a
  differential spec battery pinning `t3.aql`-shaped programs across
  interpreter / check / compiler, so the current disagreement is a
  recorded failure rather than an invisible one.
- **Phase 1** — fix the **native-callback seam** (§7.3 items 1, 5, 6)
  first: it is the class the library ecosystem actually hits, and the
  audit measured its migration exposure at **zero**. It is *not*,
  however, the "pure bug fix with nothing to migrate" this plan
  originally assumed — group 2 already diverges (§2), so the seam fix
  must land **together with** the `depsFresh` re-anchoring and the
  stamp-registry reconciliation, or it will repair the interpreter
  fallback while leaving the VM path arriving at the right answer for
  the wrong reason.
- **Phase 2** — fix **name dispatch** (items 2–3). This is the one that
  changes interpreter semantics; land it with the spec rows and a
  `boru check` diagnostic that reports every free word which *would*
  resolve differently under the new rule. That diagnostic is the entire
  migration tool.
- **Phase 3** — if a deliberate dynamic-evaluation idiom turns out to be
  needed, give it a *name* (`with-registry`-shaped) before the accidental
  one becomes an error. Do not leave dynamic evaluation reachable only by
  accident.
- **Phase 4 (optional)** — given (a), parse-time word resolution (c) is a
  representation change rather than a semantic one: take it for the
  speedup and for parse-time unresolved-name errors.

Two invariants throughout: **capture bindings, never values** (this is
what keeps redefinition and hot reload working), and **make the
deliberate-dynamic case visible before making the accidental-dynamic case
an error**.

## 9. Open questions

1. **Should a fn defined at top level and passed *into* a module get the
   top-level scope?** The rule in §7.2 says yes (its defining module is
   the main program), which fixes the `sort` comparator case — a user
   comparator would reach the user's own helpers. Confirm this is
   intended, because it is the change users will notice most.
2. **What is a "module" for a `Vm.run` sub-engine or a spawned process
   fork?** Both create registries; is the defining scope the fork or its
   parent? (Leaning: the fork inherits the parent's `Defs` clone, so the
   value's `Registry` pointer stays valid — but this needs a spec row.)
3. **Does the `reload` word (`HOT-CODE-LOADING.0.md` §5.1) mutate the
   module registry in place, or replace it?** §7.2 says it must mutate:
   replacing orphans every function value already handed out. This
   constrains that design and should be recorded there too.
4. ~~**Is the current behaviour load-bearing anywhere in-tree?**~~
   **ANSWERED — no.** Measured tree-wide in §10: **0 migration sites and
   0 silent-change sites** across the language repo and all seven library
   repos, against **224+ sites that the fix repairs**. The migration cost
   is nil; the reason it is nil is that the ecosystem already worked
   around the defect, in writing (§10.2).

## 10. Migration audit

Tree-wide, over `/home/user/boru` and the seven sibling library repos.
Each site classified: **A** the fix repairs it · **B** the fix breaks it
(migration) · **C** the fix silently changes its value · **D**
unaffected.

### 10.1 The numbers

| | A (repaired) | B (migration) | C (silent change) | unauditable |
|---|---|---|---|---|
| Library repos (7) | **224** | **0** | **0** | 16 files + 2 probe shapes |
| boru repo | 3 + 12 worked-around | **0** | **0** | — |

**The migration cost is zero.** No function value anywhere in the tree
resolves a free word that exists only in the module that runs it, and no
module pair that exchanges function values shares a module-level name.

Two caveats keep this honest. **B and C are not unreachable** — both were
reproduced against `sort`'s shipped, documented comparator API. The class-C
repro is the one to look at: with `by-string` defined in both the caller
and `sort.aql`, the same program returns

```
--no-compile    → ["Apple", "fig", "pear"]     (ascending)
--force-compile → ["pear", "fig", "Apple"]     (descending)
```

— opposite orderings, exit 0, no diagnostic on either engine. And the
**collision scan found one near-miss**: `trie.aql`/`radix.aql`/`tst.aql`/
`burst.aql` share 28–33 module-level names pairwise. They are class D only
because the variants exchange Lists, never function values. Add one
function-valued word to that API and the tree acquires its first class-C
site.

### 10.2 Why the cost is zero: the ecosystem already paid it

The zero is not evidence the defect is harmless. It is evidence that
every library author hit it and worked around it, and three of them wrote
down why:

- **`boru/utils/*.boru` — 12 CLI tools, and not one calls `Cli.main`.**
  Each unrolls it into `Cli.dispatch` plus three statements, naming this
  defect. `cat.boru:209-217` also prices it: routing through `Cli.main`
  forces interpreter mode, costing **12× on the hot path** (8.6 s vs
  0.72 s over 8,000 lines, measured).
- **`aless-app.aql:432-435`** — the `Aless.feed` wrapper exists solely
  because "an `/r`-parked update fn called from another module cannot
  resolve this module's private words; a module wrapper can."
- **`sort.aql` is one 1,490-line file because of this**, with 34
  `Function` params and 11 exports needing privates. `CLAUDE.md` states
  it as a structural rule. After the fix, all 34 seams become safe and
  **the file can finally be split** — the single largest win in the
  corpus.

Every comparator example in every `AGENTS.md` is written with a
builtin-only body. That is not style; it is the shape that dodges the
defect.

### 10.3 The guards never could have caught it

Seven of nine repos ship a divergence runner whose job is exactly
interpreter-vs-compiler comparison. **All of them are structurally
blind**, for one reason:

```sh
interp="$("$AQL" "$s" 2>&1)"      # sort/test/divergence/run.sh:76 — and 5 siblings
```

No flag means the **default** mode, which prefers the bytecode compiler.
The column labelled `INTERPRETER` is running the compiler, so the gate
compares bytecode against bytecode. Proof on the one file that actually
diverges:

```
boru              test/sort_unit_test.aql  → all green   exit 0
boru --no-compile test/sort_unit_test.aql  → FAIL reverse …  exit 1
```

The runner prints `INTERPRETER ok` for a file the interpreter fails.
**Fix: add `--no-compile` to the interpreter leg — one line per repo.**
This should land regardless of anything else in this document.

Two further blind spots bound the audit's reach: the **checker implements
the old rule**, so `--force-compile` refuses precisely the programs that
trip the defect (16 files unauditable that way); and `lang/spec/*.tsv`
has **zero rows** exercising a cross-module function value, a boru codec,
a cross-module service handler, `IO.mount` across modules, `Tui.run`'s
update/view, or `Model.*` at all. `make verify-bytecode` could not have
caught this because the corpus never poses the question.

### 10.4 Corrections to this document

The audit falsified three claims made in earlier sections, now fixed
in place: the native-callback classification (§2 — group 2 diverges
rather than being uniformly wrong), the "hard ones" warning in §7.3 (no
site is blocked on a bare `*Signature`; the fix is 14 mechanical edits),
and the characterisation of phase 1 as a pure bug fix (§8 — it must land
with two further changes).

### 10.5 Unrelated breakage found in passing

**`/home/user/aless` does not run at all.** All 14 modules import the
retired `aql:` namespace (`import "aql:io"` → `module "aql:io" not
found`); the modules now live under `boru:`. A `sed 's/"aql:/"boru:/g'`
port runs clean, so it is a pure rename. Not this defect, but it means
the repo has been dead since the namespace change and nothing reported it.

## 11. Target semantics

The rule this document recommends is one clause of a three-clause model.
Stating all three together matters, because users do not experience them
separately — they experience one expectation, which every mainstream
language meets:

> **A function value means the same thing wherever it goes.**

1. **Free names resolve where the function was written** — lexically by
   module, dynamically in time. This document. boru already has Python's
   model (local → captured → module globals → builtins) and differs in
   one respect: its "module globals" is whichever module is running.
   `FnDefInfo.Registry` is the missing `__globals__` pointer, already
   set. This is completing a design, not replacing one.
2. **Captures should be bindings, not values.** Today `Captured` holds a
   shallow snapshot, so the universal counter idiom silently does not
   work — the closure sees the value at construction forever, unless the
   payload happens to be pointer-backed, which makes the rule invisible
   and type-dependent. Python, JS, Lua, Scheme and Ruby all capture the
   *cell*. This is also the invariant hot reload needs
   (`HOT-CODE-LOADING.0.md`): capture bindings and redefinition stays
   visible.
3. **A function value is data until something applies it.** The `sort`
   combinator fault (§5) is this rule breaking: `Sort.by-number` is a
   value in one argument position and an application in another. In a
   concatenative language a bare word *is* application, so `/r` is right
   and worth keeping — the defect is the inconsistency. A parameter slot
   typed `Function` should never auto-apply its argument.

The spec sentence:

> A function value carries the scope it was written in. Its free names
> are looked up in its **defining** module, at **call** time; names it
> captured from an enclosing function are shared **bindings**, not
> copies; and a function value is **data** until something applies it.

Rules 1 and 3 are live defects with reproductions. Rule 2 is latent —
nobody has filed it because the `flex`-cell workaround is discoverable —
and it will surface the first time someone ports a closure-heavy design.

## Appendix — how other languages bind free names in a function value

| Language | Representation | Free-name lookup | Redefinition-friendly |
|---|---|---|---|
| **Python** | code + `__globals__` + `__closure__` | local → enclosing cell → **defining module dict** → builtins | yes — the dict is mutable and shared with `reload` |
| **Lua ≥ 5.2** | proto + upvalues incl. `_ENV` | `x` compiles to `_ENV.x`; `_ENV` is a lexically captured upvalue | yes |
| **Scheme / OCaml / Haskell** | closure = code + heap environment | lexical, by construction | n/a (immutable bindings) |
| **Common Lisp** | code + lexical env; named fns via the symbol's **function cell** | lexical for variables; global cell for functions | yes — cell is mutable |
| **Erlang** | fun + captured environment | lexical; module-local funs tie to the module version | via the two-generation module rule |
| **Factor / Forth** | quotation of **word objects** | resolved at parse time to a word owning a mutable definition slot | yes — name early-bound, definition late-bound |
| **boru today** | `FnDefInfo` + `Registry` + `Captured` | **defining module on some paths, running module on others** | — |

The unifying observation is Moses's (1970): a function value is
meaningless without its environment, so the environment must be *part of
the value*. boru already made that decision — `FnDefInfo.Registry` is the
environment pointer. What remains is to consult it everywhere.
