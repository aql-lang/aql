# BORU-SHARP-EDGES

Language- and compiler-level sharp edges found by **dogfooding** — writing a
large, real application entirely in boru. These are findings *about the
language*, deliberately kept out of the app-specific RFC that surfaced them so
a language/engine maintainer can find them without reading about vaults.

This is a **discovery doc**, not a design proposal: each entry is a minimal
reproduction, the observed-vs-expected behavior, a root-cause hypothesis with
`file:line` where known, the workaround the app used, and a **classification**.
It is the place to record "boru made me do an unobvious thing" so the next
author does not re-derive it — and so the genuine bugs get fixed.

## Provenance

All findings come from the **vault-TUI port** (`design/VAULT-TUI-PORT.0.md`) —
the first large application built on the `boru:tui` runtime
(`lang/go/modules/vault_tui.boru`, ~700 lines, ~80 functions). The port ran the
full test + coverage gate green, so every workaround below is load-bearing:
without it the app does not run (or does not compile).

Classification legend:

- **engine-bug** — behavior that looks incorrect, not merely surprising; a
  fix candidate.
- **sharp-edge** — correct-but-unobvious semantics; at minimum a docs gap.
- **latent-bug** — a defect in already-shipped repo code, not just the new app.
- **compiler-limit** — the interpreter is fine; the bytecode compiler refuses
  to lower a shape (falls back to the interpreter). Only matters for
  force-compiled code paths (see §3).

---

## 1. Runtime / interpreter findings

### G8 — a recovered `raise` inside a map-literal value tears down the enclosing fn's bindings  ·  *engine-bug (candidate)*

> **Status (re-verified 2026-07-30): NO LONGER REPRODUCES.** Fixed by
> unrelated work; the entry is kept for history. Recorded via the
> NUR029 umbrella (split 2026-07-31 — see NUR.md).

```
def g   fn [[] [String] [ do [ raise "boom" ] error [ dot message ] ]]
def bad fn [[t:String] [Map]  [ {title: t  text: (g)} ]]
bad "T"        # → undefined word: t   (at the OTHER map key)
```

A `do … error …` inside one map value **recovers** the raise (as designed), but
the unwind still runs the enclosing function's body-local-def cleanup, so `t`
(a *param* of `bad`) is gone by the time the next map key evaluates. The map is
half-built when the binding vanishes.

- **Expected:** a recovered raise is contained by `error`; the caller's params
  and locals remain live.
- **Workaround:** `def`-bind every raise-capable expression *before* the map
  literal (`vt-status-pager`, `vt-detail` build their text/rows first).
- **Why it looks like a bug:** the recovery boundary should not leak binding
  teardown into the recovering frame.

### G9 — a multi-arg call in a `case` DEFAULT slot mis-collects  ·  *sharp-edge*

> **Status: FIXED (NUR048, resolved 2026-07-31).** A match-position
> word bound to a function value now heads an OPEN-CALL default arm:
> the residue runs isolated with the case value pushed first, exactly
> like a matched arm (`caseDefaultStart` /`isCaseOpenCallHead`,
> lang/go/native/conditional.go; rows in lang/spec/case.tsv §7). The
> parenthesized workaround below is no longer required.

```
case ev.key [
  "q"  [ drop … ]
  vt-screen-key state ev        # DEFAULT arm — WRONG: collects case's stack values
]
# → "expected Map, got fn __casematch(…)"   or silently folds the wrong value
```

In the default (last) position of `case`, the case machinery's own values are
on the stack; an *open* call there forward-collects them as its own arguments.
In the vault app the earliest symptom was subtle: an event map became the whole
app state, because the default arm captured the event instead of dispatching.

- **Workaround:** parenthesize call-form defaults — `(vt-screen-key state ev)`.
- **Classification:** at minimum a documentation gap (the default slot's
  collection context is not obvious); arguably `case` should isolate the
  default arm's stack the way the matched arms are isolated.

### G10 — `def why (dot message)` in an `error` handler never works  ·  *latent-bug (shipped code)*

> **Status: still reproduces (re-verified 2026-07-30). Registered as
> [NUR049](../NUR.md#nur049)** — verdict (2026-07-31): FIX by making
> the paren barrier symmetric (a group may not reach backward past its
> open paren); fix `todo-tui-client.boru` and test its error arms.

```
do [ … ] error [ def why (dot message)  … ]     # → dot: no receiver
```

Inside the `error` handler the raised error is **on the stack**. Wrapping
`(dot message)` in a paren opens a *fresh* collection context, so `dot` finds
no receiver. The working form is unparenthesized `dot message`, feeding the
result to a helper that takes it from the stack (`vt-err-text`,
`vt-vault-fail`).

- **Latent bug:** this exact broken idiom ships in
  `design/examples/apps/todo-tui-client.boru` (its `error [ def why (dot
  message) … ]` arms). Those arms are **never exercised by a test** — the
  client e2e only drives the success path — so the breakage is invisible.
- **Recommended:** fix the example and add a test that forces a sync failure.

### G11 — a list literal returned as a fn result evaluates lazily, after body teardown  ·  *sharp-edge*

> **Status (re-verified 2026-07-30): NO LONGER REPRODUCES.** Fixed by
> unrelated work; kept for history (NUR029 umbrella, split 2026-07-31).

```
def row fn [[a:Map] [List] [ [ (vt-str a.name) (vt-str a.provider) ] ]]
# → undefined word: a   (raised at the CALL SITE that consumes the list)
```

A bare list-literal *tail* returned from a function is auto-evaluated when it is
finally consumed — by which point the function's frame (and its param `a`) is
gone. `def` evaluates eagerly, so binding the list and returning the name
snapshots it in-frame.

- **Workaround:** `def row2 [ … ]  row2` (used by every list builder:
  `vt-alias-row`, `vt-rrow`, `vt-*-cols`).
- **Classification:** sharp-edge / docs gap; arguably a returned list should
  snapshot its lexical bindings like `def` does.

### G12 — an `/r`-parked fn does not match a `Function`-typed param  ·  *engine-bug (candidate)*

> **Status: FIXED (NUR050, resolved 2026-07-31 — ADR-011).** Two
> halves: (1) a `/r`-marked word now feeds forward collection as its
> reference (`wa {x:1} some-fn/r` dispatches; the function-word
> barrier fired before the marker was visible), and (2) `Word/__FN`
> was collapsed into `Type/Function` — there is exactly one function
> type, so the checker's `__FN` misdiagnosis is gone and a bare fn
> name before a Function-typed slot resolves as a reference (the
> planner's designed intercept, un-broken by the collapse). The
> map-continuation workaround below is no longer required.

```
def wa fn [[s:Map act:Function] [Map] [ act s ]]
wa {x:1} some-fn/r      # → dispatch fails: got (Map, __FN); nearest [Map Function]
```

A `/r`-parked function value carries the dispatch-marker type `__FN`, which does
**not** satisfy a `Function`-typed parameter. Under the interpreter the call
simply fails to dispatch and leaves the args stranded (the checker names it;
the runtime is silent). Yet the **same** parked value stored in a map and
invoked by dot-access dispatches perfectly:

```
def m {run: some-fn/r}   m.run {x:1}     # works
```

- **Workaround:** continuations travel inside maps — every form `submit` and
  every auth continuation is `{run: <fn/r>}`, invoked as `scr.submit.run state`.
- **Why it looks like a bug:** `__FN` and `Function` should be interchangeable
  at a dispatch boundary — either the parked marker satisfies `Function`, or
  parking converts. The map/dot-access path proving it *can* work makes the
  direct-param failure look like a missing case, not intended semantics.

---

## 2. Bytecode-compiler findings

The interpreter runs all of these correctly; only the bytecode compiler
(`--force-compile` / `RunCompiled*`) refuses to lower them. Both refusals are
**"body result of unknown provenance."**

### G13a — a *single-token* bare computed-map body refuses  ·  *compiler-limit*

> **Status (re-verified 2026-07-30): NO LONGER REPRODUCES** — a
> single-token bare computed-map body now compiles. Fixed by unrelated
> work; kept for history (NUR029 umbrella, split 2026-07-31).

```
def f fn [[a:Integer] [Map] [ {x: (a add 1)} ]]      # single-token body
# --force-compile → force-compile: fn f: body result of unknown provenance
```

A function whose *entire body is one bare computed `{…}` (or `[…]`) literal* is
a deferred residual the compiler cannot lower. **Note the narrowness:** the
compiler was widened on **2026-07-11** (`design/NET-COMPILE-FRONTIER.0.md`
ADDENDUM 5) so a **multi-token** body ending in a pending container now records
an in-frame `OpMakeMap` and compiles:

```
def f fn [[a:Integer] [Map] [ def y (a add 1)  {x: a  y: y} ]]   # COMPILES
```

- **Cure:** `def`-bind the map and return the name (which also makes the body
  multi-token). This is the fix applied to `vt-palette-closed`.
- **Confirmed** (this repo, today): single-token refuses; multi-token compiles.

### G13b — a *type-literal* map value refuses even when def-bound  ·  *compiler-limit (distinct root cause)*

> **Status: FIXED (NUR051, resolved 2026-07-31).** A bare type node in
> map/list DATA position now interns as a canonical-ID type operand
> (`OpPushType`) in RecordMakeMap / recordMakeListInner, per ADR-010
> "Types are values" — `{r: None}`, `[a None]`, `{k: Integer}` compile
> exactly as they interpret (rows in lang/spec/bytecode-migrated.tsv).
> The `None` → `none` workaround below is RETIRED — kept only as
> history.

```
def f fn [[a:Integer] [Map] [ def m {x: a  r: None}  m ]]   # STILL refuses
def f fn [[a:Integer] [Map] [ def m {x: a  r: none}  m ]]   # compiles
```

A map value that is a **capitalized type literal** (`None`, `Integer`, …) mixed
with any computed value refuses **even with the G13a def-bind cure** — the
provenance gap is the type-literal, not the deferred residual. The lowercase
`none` *value* is fine.

- **Where it bit the app:** `vt-detail` used to return `{… revealed: None}` —
  the one internal function that carried a compile-refusing shape. It was
  changed to the lowercase `none` value (`revealed: none`, with the reveal
  toggle's `eq None` → `eq none`); `none` and `None` compare equal so behavior
  is identical. The app now carries **zero** latent refusals (§3).
- **Cure:** use the lowercase `none` value (or a non-type sentinel) as the
  sentinel, not the `None` type literal.
- **Confirmed** (this repo, today): `{… r: None}` def-bound refuses;
  `{… r: none}` compiles.

---

## 3. The compile-status finding: the vault TUI runs **100% interpreted**

A question worth its own section, because the answer is unintuitive:
*does `vault_tui.boru` compile fully to bytecode without refusals?*

**No — and it does not matter, because none of the app ever runs as bytecode.**
The evidence:

- **The launcher runs the program on the interpreter, unconditionally.**
  `cmd/go/internal/vault/borubridge.go:360,389` execute the two-line app source
  via `a.RunInterp(...)` — never `.Run` (compile-try) or `RunCompiled*`. Per
  `lang/go/boru.go:754`, `RunInterp` "executes on the tree-walking interpreter,
  unconditionally." Runtime fn-stamping is therefore never armed.
- **The module preamble is loaded on the interpreter.**
  `lang/go/modules/vault_tui.go:99` does `native.New(modReg).Run(tokens)` — the
  tree-walking engine. Every `def vt-* fn` is *installed* as an `FnDefInfo`;
  no body is compiled at import (identical to `sift.boru`, `repl.boru`).
- **`Tui.run` dispatches every `update`/`view` callback through the
  interpreter.** `lang/go/modules/tui_run.go:371,414` call
  `eng.InvokeCallback`, which (`eng/go/invoke.go:66,106`) runs the VM only if
  the callback has a stamped `CompiledRef`; with none, it falls to `CallBoru` —
  the interpreter — on every keystroke and every frame.

So the app is interpreted exactly like every other boru app in the repo. The
**only** part known to bytecode-compile is the **init-construction graph**
(`vault-app` → `vt-pal`, `vt-palette-closed`, `vt-home`, and the nested init
map), because three rows in `lang/spec/module-vault-tui.tsv` force-compile it
through the langspec census. That graph compiles cleanly (census + no-refusals
gates green) after the `vt-palette-closed` G13a fix.

The **deeper fold graph** (`vt-update`, `vt-view`, and the ~70
handler/builder/view fns) is **never compile-checked by any gate and is not
even reachable by CLI force-compile**: `update`/`view` are parked `/r` refs
reached only through dynamic dot-access dispatch, and force-compiling an
`(app.update) …` call refuses at the *call site* ("fn-value application bounded
by a paren") before any body is analyzed. A whole-app compile check is not
expressible today.

A static + empirical audit of the internal functions found one function
carrying a compile-refusing shape — `vt-detail` (G13b, `revealed: None`) —
**since fixed** to the `none` value, so the app now carries **zero** latent
refusals. Seven functions carry the *structural* bare-map-tail shape but all
compile fine under the post-2026-07-11 multi-token rule; every list builder
already def-binds (zero G11-shape list tails). The runtime impact of the
compiler items was **zero** either way (nothing here is ever force-compiled).

**Defensible one-liner:** *"The vault TUI does not fully bytecode-compile, and
that is irrelevant — it runs entirely on the interpreter like every boru app in
the repo; only the spec-forced init graph is known to compile (and does), and a
single internal function (`vt-detail`) still carries a latent refusal that is
never exercised."*

---

## 4. Triage

Re-verified against the binary 2026-07-30; per-item NUR records and
verdicts issued 2026-07-31 (the NUR029 umbrella was split — see NUR.md
and `design/NUR-RESOLUTION-PLAN.0.md`).

| # | Finding | Class | Status → action |
|---|---|---|---|
| G8  | recovered-raise binding teardown | engine-bug? | **no longer reproduces** (fixed by unrelated work) |
| G12 | `/r` fn ≠ `Function` param | engine-bug? | **FIXED** (NUR050 resolved 2026-07-31 — /r collection admission + the ADR-011 one-Function-type collapse) |
| G10 | `def why (dot message)` in `error` | latent-bug | reproduces → **NUR049**: symmetric paren barrier; fix `todo-tui-client.boru` + failing-sync test |
| G9  | `case` default slot collection | sharp-edge | **FIXED** (NUR048 resolved 2026-07-31 — open-call default runs isolated like a matched arm) |
| G11 | returned list literal laziness | sharp-edge | **no longer reproduces** (fixed by unrelated work) |
| G13a| single-token bare-map body | compiler-limit | **no longer reproduces** (single-token now compiles) |
| G13b| type-literal map value | compiler-limit | **FIXED** (NUR051 resolved 2026-07-31 — nested bare type nodes intern as `OpPushType` operands; ADR-010) |

The one live item (G10 → NUR049) carries a verdict-bearing NUR
record; a per-item fix retires its record — as G9 (NUR048), G12
(NUR050) and G13b (NUR051) now demonstrate. The two dead engine items (G8, G11) and
the dead compiler item (G13a) were fixed by unrelated work — which the
register only noticed on re-verification, the cost of the original
umbrella entry.
