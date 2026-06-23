# Stage-3 module-fn call-site inlining — verified implementation plan

_Status: PLAN. No Stage-3 code landed. This note records the
code-level findings established by tracing `module-test.tsv:38`,
`module-rand.tsv:38`, and `module-parselang.tsv:23` to ground truth,
so the dedicated build starts from certainty._

## Why these 3 rows refuse (and the 2 that must)

Live refusals (5), from the corpus census:

| row | reason | disposition |
| --- | --- | --- |
| `def-node-binding:54` | `fn mk: body result of unknown provenance` | **correct refusal** |
| `macro:45` | `residual value not statically materialisable` | **correct refusal** (error row) |
| `module-parselang:23` | operand provenance at `get` | Stage-3 |
| `module-rand:38` | residual value of unknown provenance | Stage-3 |
| `module-test:38` | `code-body word test-describe (Stage 2)` | Stage-3 |

The two correct refusals are PROVEN unsound to compile by running the
interpreter:

- `def c1 1 def mk fn [[c1:Integer] [List] [[c1]]] mk 9` → `[1]`. The
  returned `[[c1]]` resolves `c1` against the MODULE binding (1) at
  top-level auto-eval, not the param (9). Any compile that binds the
  param in body scope yields `[9]` → differential-gate divergence. The
  `body result of unknown provenance` guard is what keeps it sound.
- `def loopy (macro [[a] [quote [loopy unquote a]]]) macroexpand (loopy 1)`
  → raises `expansion too deep`, exactly the `ERROR:` the spec requires.
  Static divergence is undecidable; refuse-and-raise is the only sound
  behaviour.

So `refusalCeiling=0` is unreachable without unsoundness. The reachable
target is the 3 module rows.

## The single root cause of the 3 module rows

A module-preamble AQL fn (`run-spec`, `run-cases`, `run-case`;
`Rand.list-of`; `ParseLang.parse_calc`) is an `FnFrame` fn. In
check+emit mode its `buildFnBodyReturnsFn` (`eng/go/core_helpers.go:271`)
compiles the body as a **shared, type-memoized unit** against
GENERALISED carrier args:

```go
// core_helpers.go:371-380 — the deliberate invariant
genArgs := make([]Value, len(args))
for i, a := range args {
    genArgs[i] = NewCarrier(a.Parent) // concrete values would const-fold
}                                     // into the shared unit — wrong for
                                      // other call sites.
```

Because the param is a carrier, every field read inside the body
(`(s get "name")`, `(s get "cases")`) is `Any` (the `getNodeReturns`
fold only fires on CONCRETE containers — landed this session, see
below). `Any` then can't dispatch the body's typed code-body word
(`test-describe [String List]`), can't materialise the `for (subs size)`
count to prune the recursion, and can't type the parser/RNG residual.

A unit-body `MarkUncompilable` sets `es.Compilable=false` GLOBALLY
(no per-call isolation), so the body's refusal becomes the whole
program's refusal — that is why `module-test:38`'s program-level reason
is the deep `test-describe` one.

## What must be built (4 interdependent features)

None clears a row alone; only all four together flip `module-test:38`.

1. **Call-site inlining (the new compilation mode).** When a non-generic
   `FnFrame` fn is called with CONCRETE args, record the body's events
   INTO THE CALLER'S STREAM with the concrete args bound (not a shared
   CALL_USER unit), so field reads fold. This mode does not exist:
   `RecordUserCall` only emits `CALL_USER` to a type-memoized unit. The
   inline fragment must be keyed by call site / concrete value, never by
   arg TYPE (else a second call with same types, different values, reuses
   baked constants). Gate it PROBE-FIRST in an isolated `EmitState`
   (`IsolateEmit`) so a body that does not fully inline falls back to
   today's shared-unit path with zero regression.

2. **Zero-count `for` pruning.** A `for` whose count is concretely 0 has
   an unreachable body and yields `[]`. Required so the inlined
   `for (subs size) [ subspec run-spec ]` (subs=`[]`) drops the recursive
   branch instead of analysing it. Sound and bounded, but inert until (1)
   makes `subs` concrete inside the body (prototyped+reverted earlier for
   exactly this reason).

3. **Module closure code-bodies.** `test-describe`/`test-test` bodies
   already declare a `CallableSpec` (`lang/go/modules/test.go:232,279`);
   they need the closure path (`tryRecordClosure`) to admit them once
   their name arg is a concrete String (depends on (1)).

4. **Fn-value dispatch.** `run-case` does `(in subject test-invoke)`,
   where `subject` is the fn-VALUE `double/q`. `test-invoke` invokes it —
   currently `anonymous function dispatch (Stage 3)`. Compiling this is
   the LAST feature; it is the one that finally clears `module-test:38`.

`module-rand:38` needs (1)+(2)-style residual typing; `module-parselang:23`
needs (1) plus typing a user-registered fn's return — both narrower than
`module-test:38` but still rooted in (1).

## Landed this session (sound, gate-clean prerequisites)

- **`getNodeReturns` container fold** (`lang/go/native/native_storage.go`):
  a concrete list/map field read folds to the cloned container value
  (fresh ID), not a bare type carrier. 5 rows off the Any frontier
  (254→249). This is the precise prerequisite that makes the inlined
  body's field reads concrete in feature (1).
- Corrected the earlier (wrong) root-cause pin; this note supersedes it.

## Experimentally-verified prerequisite BENEATH feature (1)

Tracing `args` into `run-spec`'s `buildFnBodyReturnsFn` (temporary
instrumentation, reverted) shows the concrete map is ALREADY LOST before
the body is ever compiled:

```
EXP run-spec arg 0 parent Any concrete false
```

At top level `def s {…} end (s get "name")` compiles (the map is concrete
and the get-fold fires). But passing the same `s` through the module-fn
dispatch boundary `s Test.run-spec` delivers arg 0 as a bare `Any`
carrier — not even `Map`. So specialising the body unit to "concrete
args" is inert (verified: forcing `genArgs[i]=a` when `IsConcrete(a)`
left `module-test:38`'s refusal reason byte-identical) because the arg
that reaches the ReturnsFn is not concrete in the first place.

**Therefore feature (1) has a sub-prerequisite (1a): the module-fn /
dot-access dispatch boundary must PRESERVE the concrete arg** (today it
hands the callee an `Any` carrier in check+emit mode). Find where
`Test.run-spec`'s dispatch (execFnDefLiteral → module sub-registry →
shareCheckState) converts the concrete operand to `Any`, and thread the
concrete value through, before call-site inlining can fold anything.

## Discipline for the build

Probe-first per feature (isolate, commit only on full success, fall back
otherwise). `make verify-bytecode` (differential + fuzz + race + aqldebug,
0 divergences) gates every step; ceilings re-baseline only when a row
actually clears, with rationale. Expect features (1)-(3) to clear NO row
on their own — the ceiling moves only when (4) lands. Gate-clean-or-revert
throughout.
