# Function Model Consolidation (v0)

Status: the three structural unifications are complete and green. The
follow-up (FnDefInfo's two-slice collapse) is now also complete — see
"FnDefInfo single-slice collapse" at the end.

## Goal

A function is one thing with one runtime operation. The only thing that
makes a function "native" is that its body is written in Go. Remove the
historical dual representation and dual dispatch path so dispatch,
matching, arg-binding, return-checking, capture, and introspection are
uniform.

## 1. One dispatch path

Named boru fns already compiled to a Go handler: `InstallFnDef`
(core_helpers.go) lowers a `def f fn […]` into `RegisterNativeFunc` with a
body-splicing handler closure plus a check-mode `ReturnsFn`. The remaining
fork was Function-VALUE-on-stack dispatch — `afn` / `=>` lambdas and the
closures they return — which used a handler-less compiled table and fell
back to `execFnDefSigStackMatch`.

`compileFnDef` (engine.go) now attaches the shared boru body-runner
(`buildFnBodyHandler` + the check-mode `buildFnBodyReturnsFn`, both
extracted verbatim from `InstallFnDef`) to the compiled signatures of
**anonymous** fns. So an afn/closure Function value dispatches through the
uniform `execMatch` path exactly like a registered native; the
`sig.Handler==nil` fallback no longer fires for it.

Scope rule (learned via a regression): handler attachment is gated on
`fnDef.Anonymous`. A non-anonymous bare FnDef — notably a predicate-type
FnDef sitting on the stack (`def Bbd …; Bbd "c"`) — MUST keep
`Handler==nil` so it stays inert data rather than auto-dispatching.

## 2. One per-argument representation

`Params []FnParam` (name + type + pattern + optional) is the single source
of per-position arg shape. Every matcher / forward-planner reader goes
through `sigArgType` / `sigPattern` / `TotalArgs`, and external consumers
(the lang help/inspect surface) through the exported `Signature.ArgTypes()`.

The positional `Args []*Type` and `Patterns map[int]Value` fields are
retained as an **exported constructor convenience** for Go callers (tests,
plugins, the `NativeSig` shim) that build a signature positionally without
param names. The kernel does not read them: `normalizeSig` folds them into
`Params` at the registration/compile boundary (`upsertFnDef`,
`compileFnDef`) and refreshes them as mirrors, after which `Params` is
authoritative. The low-level accessors fall back to `Args`/`Patterns` only
for a not-yet-normalized positional signature (e.g. a test calling
`FlexibleMatch` directly).

## 3. One signature struct type

`Signature` is now `type Signature = FnSig` — a single struct. The former
Signature-only fields (`Handler`, `Args`, `Patterns`, `FullStack`,
`QuoteArgs`, `TypeArgs`, `Fallback`, `ReturnsFn`, `RunInCheckMode`,
`CheckFullStackFn`) were folded into `FnSig`, which already carried the
shared `Params`/`Returns`/`BarrierPos`/`NoEval*` and the boru `Body`.

Within the one type, **Body vs Handler is the sole Go-vs-boru distinction**:
a boru sig carries `Body` tokens; a native sig carries a Go `Handler`.

`NativeSig` remains the ergonomic Go authoring shim — it lowers into the
unified `FnSig`/`Signature` at `RegisterNativeFunc`; the ~348 `NativeSig`
literals are unchanged.

## FnDefInfo single-slice collapse (complete)

`FnDefInfo` now has ONE signature slice, `Signatures []Signature`. The old
`Sigs` field is gone. Each `Signature` is full-fidelity: it carries the
authored shape (`Params` with names, `Returns`, boru `Body`) AND, once
compiled, the dispatch fields (`Handler`, resolved `BarrierPos`). Body vs
Handler remains the sole Go-vs-boru distinction within the one type.

How the three reconciliation points were resolved:

- **Compiled sigs carry `Body`.** `compileFnDef` and `InstallFnDef` keep the
  authored `Body`/`Params`/`Returns` while layering on the handler, instead
  of lowering to a `Body`-less `NativeSig`.
- **Per-entry own sigs vs the dispatch table.** A stored DefStack entry (or a
  constructed Function value) now holds only THAT definition's own overloads.
  The accumulated, sorted, fallback-bearing dispatch table is built on demand
  at the registry boundary — `Registry.Lookup` → `aggregateDispatch` unions
  every stacked entry's own sigs, sorts with `CompareSignatures`, and appends
  the synthetic 0-arg `Fallback` when the name has any boru-bodied overload.
  This removed the carry-forward accumulation and the install-time fallback
  injection, and let `undef` / overlap-removal simplify to plain entry
  removal (the table just rebuilds from what remains).
- **Authored-order readers** (`canon`, `inspect`, predicate probes, trivial-
  delegation, targeted `undef`, overlap) read `FnDefInfo.OwnSigs()` — the
  signatures with any `Fallback` filtered out. `FirstOwnSig()` serves the
  single-overload readers (predicate input-type gate, refine probe).
- **Capture timing** is unchanged: `ComputeCaptures` still walks each authored
  sig at construction; it just reads the renamed field.
- **Construction → dispatch.** `execFnDefLiteral` compiles a self-contained
  value's own sigs (anonymous closure, or a fn defined in the current
  registry) via `compileFnDef`; a value carrying a FOREIGN sub-registry (a
  module wrapper or module-preamble fn) resolves the real definition in that
  registry. The sub-registry wrapper path is gated on a **body-bearing** own
  sig, so a `/r` reference to a Go native (Body-less sigs) dispatches straight
  through its Go handler.

Verified by the two equivalence goldens below plus the full eng + lang
suites; the one intended behavior change is eng-level `inspect` of a boru fn
(`eng/spec/inspect.tsv`), which now reports `kind:defined` and the synthetic
`{args:[]}` fallback, matching the lang surface.

## Verification

Two golden harnesses pin behavior byte-for-byte and held across every step:

- `lang/go/native/fnmodel_equivalence_test.go` (`TestFnModelEquivalence`,
  flag `-update`) — every word's sorted compiled signatures + a behavior
  corpus (native fwd/stack/swap, `if` code-bodies, named fns, recursion,
  multi-overload, afn, closures, each/fold, stack words) + check-mode
  return inference.
- `lang/go/modules/fnmodel_wrapper_equivalence_test.go`
  (`TestFnModelWrapperEquivalence`, flag `-update-wrapper`) — the
  module-wrapper captured-sub-registry dispatch path.
