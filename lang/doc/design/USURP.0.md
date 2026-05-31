# `usurp` — Argument-Order Reversal as a Higher-Order Word (v0)

## Problem

AQL's unified dispatch rule maps a value sitting on the **stack**
(a prefix operand) into the function's **last** signature slot,
while forward tokens fill the leading slots. This is the right
default for most words, but it makes the natural cond-first reading
of `if` bind incorrectly:

```
cond if [then] [else]
```

Here `if`'s signature is `[cond, then, else]`. `[then]` and
`[else]` are forward tokens, so they fill `cond` and `then`; the
stack value `cond` falls into the trailing `else` slot. Concretely:

```
true if [99] [88]   →   88      # NOT 99
```

The condition is silently ignored — it lands in the `else`
position. This is the §6.2 issue from the DX report.

We classify `cond if …` as **incorrect usage** rather than
something to detect and error on (detection would require special-
casing one word in core dispatch). What we want instead is a small,
**general** primitive that lets a user opt into stack-first /
reversed binding for *any* word — turning the mis-binding into a
correct one by construction.

## Solution: `usurp`

`usurp` is a higher-order word:

```
usurp <fn>   →   <fn'>      # fn' is fn with its argument order reversed
```

It takes a function (by name, e.g. `usurp if`, or by value) and
returns a **new function** whose signatures have their parameters
reversed. Because dispatch maps a stack-prefix value to the last
slot, reversing the parameters makes that slot the *first* logical
argument — so the cond-first form binds correctly:

```
def ifu (usurp if)
cond ifu fail pass            # cond → if's condition slot ✓
#  ≡  if cond pass fail
```

Worked example (3-arg `if`, sig `[cond, then, else]`):

- `usurp if` builds a wrapper with reversed params
  `[p0, p1, p2] = [else, then, cond]` and a body that re-invokes
  `if` with the params in reverse: `if p2 p1 p0`.
- Dispatching `cond ifu fail pass` fills `p0=fail`, `p1=pass`
  (forward) and `p2=cond` (stack), then runs `if cond pass fail` —
  `cond=cond`, `then=pass`, `else=fail`. Correct.
- Plain `if cond pass fail` is completely unaffected.

### Why this design

The decisive property is that **nothing in the kernel changes** —
no parser, `WordInfo`, `matchSignature`, deferred-collection, or
check-mode edits. `usurp` synthesises an ordinary `FnDef` wrapper
and the existing function-dispatch machinery does the rest. This is
strictly smaller than the alternative considered (a `/u` suffix
*modifier* that would have threaded a new flag through every core
dispatch path).

The wrapper delegates through an **AQL body** (`if p2 p1 p0`)
rather than wrapping the Go handler directly. That choice means
both the runtime handler **and** the check-mode return-type
inference (`ReturnsFn`) are reused for free, because the body is
dispatched normally. It is the same pattern already proven by:

- module FnDef wrappers — `lang/go/modules/binary.go::makeBinUnaryFnDef`;
- returned-function closures — `def add5 (make-adder 5)` then
  `add5 3` (see lang/go/CLAUDE.md, "Closures and Capture").

## Mechanics

`usurp` reads the target's `FnDefInfo` (eng/go/value.go:261) and
emits a new one. `FnDefInfo` carries both representations:

- builtins (e.g. `if`) expose compiled `Signatures []Signature`;
- AQL `def` fns expose `Sigs []FnSig`.

For each source signature (`N` params) the wrapper builds one
reversed `FnSig` (eng/go/value.go:209):

| Source field | Wrapper field |
| --- | --- |
| `Params` / `Args` (types) | reversed; synthetic names `p0..p_{N-1}` so the body can reference them |
| `NoEvalArgs`, `NoEvalMapArgs`, `QuoteArgs`, `TypeArgs` (index-keyed) | remapped `k → N-1-k` |
| `Returns` | copied unchanged (return type is independent of arg order) |
| `BarrierPos` | `N` (stays all-forward) |
| `Body` | `[Word(inner), Word(p_{N-1}), …, Word(p_0)]` |

The result is wrapped with `native.NewFnDef(...)` and returned.
Bound via `def ifu (usurp if)`, it dispatches through the standard
user-fn path (`InstallFnDef` / `execFnDefSig` / `CallAQL`).

### Preserving code-body (NoEval) semantics — CRITICAL

`if`'s branches are unevaluated code bodies (`if3` has
`NoEvalArgs{0,1,2}`, lang/go/native/native_control.go:31-64). The
wrapper **must** copy the reversed `NoEvalArgs` onto its `FnSig`
(the field exists on `FnSig`), so the branches bind to the wrapper
params **unevaluated**. The delegating body then re-forwards them
and `if`'s own `NoEvalArgs` keeps them unevaluated through to the
handler's mark/move. Without this, a naive wrapper would auto-
evaluate `[then]`/`[else]` at param-binding time and break `if`.
This is the primary correctness risk and the focus of testing — in
particular a **list-valued condition** (`[1 gt 0] ifu fail pass`)
must still flow through `if`'s mark/move evaluation.

## Scope and limitations (v0)

- **All-forward-eligible signatures** (`BarrierPos == len(Args)` —
  the common case, incl. `if`, math ops, typed user fns) are
  supported. Typed words work because the wrapper's reversed
  parameter **types** drive forward type-collection
  (`def subr (usurp sub)` ⇒ `subr a b ≡ sub b a`).
- **Explicit `|` barrier signatures** (`0 < BarrierPos < N`) cannot
  be faithfully reversed with a single `BarrierPos` (forward-
  eligible positions would have to become trailing). v0 **rejects**
  these with a clear error; revisit only if a concrete need
  appears.
- **Pattern / Optional params** reverse positionally with the param
  list; index-keyed metadata is remapped. Any shape that cannot be
  faithfully reversed errors rather than mis-handling silently.

## Usage notes

- **Bind the result**: `def ifu (usurp if)`, then `cond ifu fail
  pass`. Inline `cond (usurp if) fail pass` (a bare function value
  mid-stack) is not supported — bind it, like an alias.
- **Reversal is total**: every argument flips. With `ifu`, branches
  read **else-first** (`cond ifu <else> <then>`). That is the
  documented, intended consequence of a general reverse primitive,
  not a special case for `if`.

## Implementation sketch (for when this is built)

- New native `usurp` in `lang/go/native/native_usurp.go`, two
  overloads: `[Atom]` with `QuoteArgs{0:true}` (capture an upcoming
  word name → `r.Lookup`) and `[Function]` (a function value);
  both `RunInCheckMode: true`.
- Core helper `reverseFnDef(inner, innerName) (Value, error)`
  implementing the table above; error on word-not-found,
  not-a-function, barrier sigs, and irreversible shapes; add to the
  type-literal no-panic gate.
- Register in the natives aggregator; add a `help` entry with
  examples and a `REFERENCE.md` / `HOWTO.md` note documenting the
  `def ifu (usurp if)` cond-first idiom.

### Tests (positive + negative)

- `def ifu (usurp if)`: `true ifu 88 99 → 99`, `false ifu 88 99 →
  88`; `if true 99 88 → 99` unchanged.
- list condition: `[1 gt 0] ifu fail pass → pass` (NoEval +
  mark/move survive the wrapper).
- 2-arg no-else `cond ifu pass ≡ if cond pass`; clause-list `ifu
  [clauses] ≡ if [clauses]`.
- typed word: `def subr (usurp sub)`, `subr a b ≡ sub b a`.
- check mode produces sane carriers (body delegation reuses `if`'s
  `ReturnsFn`).
- negatives: usurp a non-word, usurp a barrier-sig word, type-
  literal input → no panic.
