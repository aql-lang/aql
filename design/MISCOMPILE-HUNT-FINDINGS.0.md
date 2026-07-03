# Broad miscompile hunt — 23 confirmed compile==interpret violations

A 30-agent adversarial sweep (1 mapper + 6 feature-axis hunters + per-divergence
verifiers) probing `--compile` vs the interpreter across the whole compiler. Every
finding is a confirmed `--compile != interpret` divergence on the HEAD binary — a
real violation of the bytecode compiler's one hard guarantee (the silent fallback
should have masked it). All are OFF the langspec corpus, so `verify-bytecode` /
the differential is BLIND to them — the same blind spot that hid the four
violations already fixed this session (param-guard-skip ×2, tail-return-bypass,
Options over-raise).

They cluster into **5 root-cause mechanisms**. The sound fix for each is EITHER
compile-it-correctly OR `MarkUncompilable` (refuse → fall back to the interpreter);
refusal is always sound and keeps the guarantee. Coverage (voxgig) is a separate,
advisory axis — never trade soundness for it.

## STATUS (updated)

- **D, C, B — FIXED** (commits in this session): higher-order over gradual-Any
  collection (refuse), multi-overload user fn + gradual-Any (refuse), typed-def
  refinement validate/reparent (refuse dynamic + DepScalar, keep static newtype).
- **E — PARTIALLY FIXED**: the fn-body fn-value apply (`(fnv 100)` over a Function
  PARAM) is fixed — the fn finish refuses a user-fn count mismatch whose residual
  carries a Function/FnDef value (resolveDynamicApply runs only for the MAIN
  residual, so a fn-body apply was never lowered; refuse → fall back). REMAINING:
  the /r-deferred map field auto-invoke (`{f:make42/r}.f`) and the nested-factory
  apply in the MAIN residual (`(((mk 1) 2) 3)` → leaks a Function) — both separate
  resolveDynamicApply gaps (it handles the single leading-fn-carrier `(mk 5) 10`
  but not nested levels nor the deferred-field auto-invoke). Fix: extend
  resolveDynamicApply to nested/deferred shapes, or refuse a MAIN residual that
  ends with an unconsumed Function the program did not declare.
- **A — FIXED (2026-07-03)**: per-call identity for fn-body compound literals.
  `intern` now pools compound consts by value ID (same materialised value = one
  slot; distinct source literals keep distinct IDs, so gotcha #13 is untouched),
  `resolveOperand` marks fn-unit compound-literal consts whose ID is NOT an
  enclosing binding's (an `emitUnit.enclosingIDs` DefTable snapshot at unit
  open), and `freshenFnUnitConsts` (finalize) rewrites a single-push-site marked
  const IN PLACE to the new `OpPushConstFresh` (pushes `CloneValue(const)` —
  fresh identity per call, and per loop iteration). Multi-push-site marked
  consts are reads of ONE per-call binding: they stay shared when every
  declared return conforms to Scalar (nothing compound escapes — exact
  within-call parity), and REFUSE otherwise ("compound body literal read at
  multiple sites may escape") — the sound fallback until a per-call local-seat
  lowering exists. Landing test: `lang/go/bytecode_findings_test.go::
  TestFnBodyContainerLiteralIdentity` (8 parity shapes + the refusal).
  Enclosing-binding reads (`def c [9] … [c]`) keep shared identity — probe-
  verified `(get) eq (get)` stays true in both engines.

## A. Const-pool aliasing of a fn-returned container literal (4 cases) — SILENT wrong value

> **FIXED 2026-07-03** — see the STATUS block above (OpPushConstFresh +
> ID-pooled compound interning + the multi-site escape refusal). The analysis
> below is the historical record that scoped the fix.

UPDATE after scoping: the practical impact is NARROWER than first thought, and the
fix is the riskiest. A fn body `[1]` bakes as a WHOLE const (`PUSH_CONST k0; RET`),
so every call returns the SAME instance. But the List/Map const-bake is INTENTIONAL
and load-bearing (mutation-safe — `set`/`push` on a Map/List return COPIES, never
mutate in place; only Array/Object/Store mutate in place and those are excluded from
isInertConst). So the ONLY divergence is identity (`eq` / ExactEqual): `(mk) eq
(mk)` → compiled true (one const), interpreter false (fresh per call). Mutation does
NOT diverge (verified: a `set`/`push` on the result returns a copy, leaving the const
intact); inline / def-bound literals do NOT alias (each is its own const). `eq` is
rare (value-equality is the common path), so this is the least practically reachable
of the five.

The correct fix is to construct the RETURNED container fresh per call — matching the
interpreter, which re-evaluates `[1]` each call (so it is NOT a penalty vs interp,
just parity): either an OpClone (pop → CloneValue (clone.go:42) → push) emitted
before RET when the fn residual is a container-typed const, or re-emit the residual
literal as OpMakeList/OpMakeMap (recursive for nested containers) instead of
PUSH_CONST. Both are VM/lowering changes to the intentional const-bake path —
deferred to a focused effort rather than rushed at this session's tail, since a
mistake here (mutation safety, the isInertConst whitelist) risks a NEW, broader
miscompile. A blanket refusal of container-literal-returning fns would restore
soundness but drop coverage on a deliberate optimization.

```
def mk fn [[] [List] [[1]]]   ((mk) eq (mk))     # compiled true  ; interp false
def mk fn [[] [Map]  [{}]]    ((mk) eq (mk))     # compiled true  ; interp false
```
A `List`/`Map` literal in a fn body is hoisted into the constant pool, so every
call returns the SAME object pointer; `eq` (identity / ExactEqual, compare.go)
sees one object. The interpreter constructs a FRESH instance per call. Worse than
identity: if the program MUTATES the returned container, the mutation persists
across calls in compiled but not interpreted. Mechanism: typed-carrier-return
lowering const-bakes the literal body. Fix: a container literal that ESCAPES a fn
(returned/stored) must be constructed fresh (OpMakeList/OpMakeMap), not
const-loaded — or refuse to compile a fn whose residual is a bare container
literal. Relates to the `isInertConst` invariant (COMPILABLE-SUBSET.md §4).

## B. Typed-local refinement binding drops the tag (7 cases) — both directions

```
def x:(Integer gt 10) 5  x                          # compiled 5    ; interp ERROR (5 ⊄ gt 10)
def Pos (refine Integer) def g fn [[n:Integer][Type][def x:Pos n (typeof x)]] (g 5)
                                                    # compiled Integer ; interp Pos
def Pos (refine Integer) def g fn [[n:Integer][Pos][def x:Pos n x]] (g 5)
                                                    # compiled ERROR (expected Pos got Integer) ; interp 5
```
`def name:Type value` in a fn body (and at top level) lowers to a plain
OpStoreLocal that POPS the value WITHOUT (a) validating a DepScalar/predicate
refinement (`Integer gt 10`) — so invalid values pass — and (b) reparenting to a
refine NEWTYPE (`Pos`) — so `typeof`, sig-dispatch (`need fn [[p:Pos]…]`), and the
`[Pos]` return-check all see the base `Integer`. The interpreter's `def` runs
`ReparentValue` + the unify/predicate validation. Fix: at a typed-local store,
mirror the interpreter — validate the predicate and reparent to the declared type
(the value-level check, not just base-lattice membership). Or refuse typed-local
refinement bindings.

## C. Multi-overload user fn + gradual-Any arg (2 cases) — bakes ONE overload

```
def f fn [[s:String][Integer][1] [n:Integer][Integer][2]]  (f ({b:2} getr 'b'))
                                                    # compiled signature_error ; interp 2
def id fn [[x:Any][Any][x]] def g fn [[a:Integer][String]['i'] [a:String][String]['s']] (g (id 5))
                                                    # compiled signature_error ; interp 'i'
```
A gradual-Any value (concrete provenance erased through an `[Any]` boundary)
dispatched to a MULTI-overload user fn matches every overload optimistically;
check-mode commits to the first-sorted overload and records ONE mono CALL_USER. At
run time `checkParamContract` finds the value matches a SIBLING overload and raises
instead of dispatching to it (the interpreter runtime-re-matches). Natives are
immune (OpCallNativePoly re-matches); user-fn overloads have no poly path.
SHARPLY MASKED when the reachable overloads return DIFFERENT types (carrier.go
widens to a Disjunct, ≥2 distinct returns → safe path); UNIFORM return types →
single commit → bug. Fix: MarkUncompilable when a gradual arg reaches a user fn
whose other overloads could also match (the COMPILABLE-SUBSET claim "multi-overload
gradual calls never compile" is UNENFORCED for user fns) — or record a
runtime-dispatching user-poly call.

## D. each/fold/scan over a gradual-Any collection (3 cases) — bakes Map overload

```
def mk fn [[][Any][[1 2 3]]] (each [mul 2] (mk))    # compiled each_error "expected concrete map" ; interp [2 4 6]
def id fn [[x:Any][Any][x]] (fold [add] (id [1 2 3]) 0)  # compiled fold_error ; interp 6
```
each/fold/scan's overload is statically resolved from the collection arg's Any
return type to the `[TList,TMap]` MAP signature (eachMapHandler / foldMapInitHandler
/ scanMapHandler). At run time the value is a List → "expected concrete map". The
interpreter dispatches over the List. Fix: route a gradual-Any collection through
the native poly path (OpCallNativePoly re-match, as other dynamic natives do), or
refuse each/fold/scan when the collection arg is gradual-Any.

## E. Dynamic Function-value application / auto-invoke (5 cases) — Function leaks

```
def apply1 fn [[fnv:Function][Integer][(fnv 100)]] (apply1 ([y:Integer] => [5]))
                                                    # compiled type_error (expected 1 got 2) ; interp 5
def make42 fn [[][Integer][42]] {f:make42/r}.f      # compiled "fn make42" ; interp 42
def mk fn [[x:Integer][Function][([y]=>[([z]=>[z])])]] (((mk 1) 2) 3)
                                                    # compiled Function({2 [] 0}) ; interp 3
def adders ([5] each [[k:Integer]=>[([y:Integer]=>[k add y])]]) (adders each [[fnv:Function]=>[(fnv 100)]])
                                                    # compiled [100] ; interp [105]   (each-capture upvalue not promoted)
```
Applying a runtime `Function` VALUE — a fn-typed param `(fnv args)`, a nested
factory's returned closure, a `/r`-deferred map field on `.field`, a nullary
closure extracted from a container — is not lowered to a dynamic apply
(OpCallDynamic), and a captured each-block param is not promoted to a per-iteration
upvalue. The Function leaks as data / the wrong capture is used. Fix: lower
`(fnv args)` over a Function-typed value to a dynamic apply with a runtime param
guard + auto-invoke for nullary fn values; fix each-block capture upvalue
promotion — or refuse these dynamic-Function shapes. (Biggest cluster; OpCallDynamic
is a real feature, so refusal is the near-term sound fix.)

## Residual latent risks (mapper, unconfirmed — probe before trusting)

- **ParamPatterns matcher asymmetry**: `checkParamContract` (vm.go) picks
  OpenUnifyMap-vs-Unify with FOUR fewer guards than the interpreter's
  `MatchSignature` (signature.go:199-202: `!IsTypedMap(pattern)`,
  `!IsRecordType(value)`, `!IsTypedMap(value)`, `!IsOptionsType(value)`). A
  `{:Integer}`-typed-map pattern fed a violating laundered map could diverge
  (OpenUnifyMap vacuously accepts where Unify rejects). Not reproduced (typed-map
  constraints currently land in Type, not Pattern), but a code-level asymmetry.
- **Tail-call return-check** residual: `tailCompatibleReturns` uses STATIC
  `ConformsTo`; a predicate/refine caller return where static conformance doesn't
  imply the runtime `v.Is` predicate could still bypass. Probe refine-return
  mutual/self tail recursion.

## Priority

Silent wrong-value (A, B-`typeof`, E-closures) outranks loud compiled-errors (C, D,
B-spurious-error) — a wrong value corrupts silently; a spurious error fails safe.
All are real. Fix order by tractability of a SOUND fix: C and D (clean refusal /
poly route) first, then A (fresh-construct), then B (reparent+validate at the
typed-local store), then E (refuse now, OpCallDynamic later).
