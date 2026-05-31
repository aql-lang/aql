# Function Model Consolidation (v0)

Status: the three structural unifications are complete and green. One
follow-up (FnDefInfo's two-slice collapse) is deliberately deferred and
documented at the end.

## Goal

A function is one thing with one runtime operation. The only thing that
makes a function "native" is that its body is written in Go. Remove the
historical dual representation and dual dispatch path so dispatch,
matching, arg-binding, return-checking, capture, and introspection are
uniform.

## 1. One dispatch path

Named AQL fns already compiled to a Go handler: `InstallFnDef`
(core_helpers.go) lowers a `def f fn […]` into `RegisterNativeFunc` with a
body-splicing handler closure plus a check-mode `ReturnsFn`. The remaining
fork was Function-VALUE-on-stack dispatch — `afn` / `=>` lambdas and the
closures they return — which used a handler-less compiled table and fell
back to `execFnDefSigStackMatch`.

`compileFnDef` (engine.go) now attaches the shared AQL body-runner
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
shared `Params`/`Returns`/`BarrierPos`/`NoEval*` and the AQL `Body`.

Within the one type, **Body vs Handler is the sole Go-vs-AQL distinction**:
an AQL sig carries `Body` tokens; a native sig carries a Go `Handler`.

`NativeSig` remains the ergonomic Go authoring shim — it lowers into the
unified `FnSig`/`Signature` at `RegisterNativeFunc`; the ~348 `NativeSig`
literals are unchanged.

## Open item: FnDefInfo two-slice collapse (deferred — redesign, not refactor)

`FnDefInfo` still has two `[]FnSig` fields — now the SAME element type, but
two distinct lifecycle artifacts:

- `Sigs` — the **authored** signatures (carry `Body` + param names), built
  at fn/afn *construction* time. Read by capture computation
  (`ComputeCaptures`, before any install), `inspect`, targeted `undef`,
  trivial-delegation detection, refine single-param probes, `canon`.
- `Signatures` — the **install-time compiled dispatch table**: sorted by
  `CompareSignatures`, handler-bearing, with an injected 0-arg `Fallback`
  sig. Read by `matchSignature` / `execMatch` / `HasForwardSigs`.
- Natives have `Signatures` only (`Sigs == nil`, no Body); an AQL fn at
  construction has `Sigs` only (`Signatures` built later, at install).

Collapsing to one slice is a behavioral **redesign**, not a mechanical
merge: compiled sigs would need to also carry `Body`, capture computation
would need re-timing (it runs before install), and the sorted order +
injected `Fallback` sig would have to be reconciled with the authored-order
readers. It touches ~30 call sites across eng + lang, carries real
regression risk, and has near-zero behavioral payoff (the split is
well-defined and internal). Recommended as its own focused effort with the
equivalence goldens as the gate.

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
