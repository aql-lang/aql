# VALUE-PATTERN-DISPATCH — signature exhaustiveness & the variable-reference precision gap

**Status: investigation report (point-in-time).** Captures why enum
*signature* exhaustiveness — Site 3 of `design/ENUM-EXHAUSTIVENESS.0.md` — is
not a self-contained addition, and records the ready-to-use partition fix plus
the real blocker so a future scoped effort can pick it up without re-deriving
it.

## Goal

Turn an uncovered enum overload into a static finding. Given value-pattern
overloads over a closed-set (enum) parameter:

```
def render fn [ [red/q] [String] ["R"]  [green/q] [String] ["G"] ]   # 'blue' omitted
```

an uncovered call (`blue`) raises `no_signature` at runtime. Site 3 wanted to
mirror that at check time — a genuine `RuntimeMirror`.

## What already exists

The checker has a call-site partition, `disjunctPartitionReturns`
(`eng/go/carrier.go`), reached as a dispatch-failure rescue in `engine.go`
(the "strict disjunct rescue"). When a disjunct-typed argument reaches a word,
it enumerates the alternatives (`disjunctCombos` → `alternativeCarriers`),
first-matches each against the overloads (`firstMatchingSig`), and emits
**`partial_dispatch`** for any alternative that reaches no overload. It works
today for **type** unions:

```
def IorS (Integer tor String)
def only-int fn [[n:Integer] [Integer] [n add 1]]
def use fn [[x:IorS] [Integer] [only-int x]]
use 5
# check: partial_dispatch: only-int has no overload for alternative (String)
#        of a declared union parameter …
```

## The partition fix (ready, correct, confined — but not sufficient alone)

The partition did NOT fire for enums for two reasons, both traced to exact
sites, both fixable with a confined change that leaves the type-union path
byte-identical:

1. **`alternativeCarriers` widened concrete members.** `flattenAlternatives`
   maps a concrete alternative (`red`) to its type node (`Atom`), so every enum
   member collapsed to an `Atom` carrier and the value identity a value-pattern
   overload needs was gone. Fix: expand a strict disjunct preserving CONCRETE
   alternatives as their value, widening only bare-type / nested alternatives
   (a new `expandClosedAlternatives` helper). Type-union alternatives are type
   literals, so that path is unchanged.

2. **`firstMatchingSig` was pattern-blind.** It type-matched only, so it could
   not tell `[red/q]` from `[blue/q]` — every atom member matched the first
   `Atom`-typed sig. Fix: for a CONCRETE arg, additionally require the sig's
   concrete `Pattern` to `Unify` with it, mirroring `patternsOk`'s
   concrete-scalar branch (`eng/go/match.go`). Non-concrete args (type
   carriers, the type-union path) keep the prior type-only behaviour.

With both, the partition correctly computes enum coverage — an incomplete set
yields `partial_dispatch: render has no overload for alternative (blue) …`
naming the exact uncovered member (a third tweak makes `comboTypeNames` render
a concrete member by value, `blue`, not the bare family `Atom`).

These changes are correct and low-risk (confined to the disjunct-partition
subsystem; type unions unaffected). They are **reverted** on the branch because
they do not, on their own, deliver the feature — see the blocker.

## The real blocker: value-pattern dispatch through a variable reference

The value never reaches the partition AS a disjunct in real programs, and even
a concrete member fails to dispatch through a variable. The decisive
comparison (clean tree, no partition fix):

| Program | Result |
|---|---|
| `render red/q` — direct literal | clean |
| `def c red/q ; render c` — **untyped** variable | false `no_signature` "got (Atom)" |
| `def c:Color red/q ; render c` — typed variable | identical false `no_signature` |

The binding `c` stores **concrete `red`** (value retained — verified). But at
the **call site**, resolving the reference `c` yields an abstract `Atom`
carrier, so the value-pattern sig `[red/q]` cannot match. This is a **general
value-pattern-dispatch precision gap through any variable reference** — it is
NOT enum-specific and reproduces with a plain `def`.

Two consequences compound it:

- **Abstract path never checks.** A fn that takes an enum param and dispatches
  on it (`def paint fn [[c:Color] [String] [render c]]`) is only analysed when
  `paint` is *called* (fn bodies run only if reached —
  `CheckState.FnBodyDepth`). Uncalled, no coverage check happens at all.
- **Concrete path hits the gap.** When `paint` IS called (`paint red/q`), the
  body is analysed with the concrete call shape (`c` = `red`), and `render c`
  then trips the variable-reference precision gap above — a false
  `no_signature` even when the overload set is COMPLETE.

So the disjunct partition, even fixed, is not reliably on the path: the value
arrives either abstractly (body unanalysed) or concretely-through-a-variable
(precision gap), not as the strict disjunct the partition consumes.

## Why this is not a safe increment

Closing the gap means changing one of two core things, both high-blast-radius
and error-severity (`no_signature` / `partial_dispatch` gate `aql check`):

1. **Variable-reference precision** — resolve a reference to a known-concrete
   binding as that value at the call site. Touches how every `def` reference is
   typed in analysis; shifts inferred types across the whole corpus.
2. **Per-call-shape vs. abstract fn-body analysis** — analyse an enum param as
   its disjunct so the partition sees it.

Both are core checker changes deserving their own design + review, not a
tail-end add-on. The motivating win is broader than enums: fixing (1) improves
ALL value-pattern dispatch through variables, of which enum exhaustiveness is
one beneficiary.

## Recommendation

- Ship enum **`case`** exhaustiveness (done — `design/ENUM-EXHAUSTIVENESS.0.md`,
  Site 1). `case` is the blessed enum-dispatch construct and does not depend on
  any of the above.
- Open **value-pattern-dispatch precision through variable references** as a
  separate, scoped effort with its own design pass. Pair the partition fix
  (recorded above) with the precision fix; enum signature exhaustiveness falls
  out.
- Do NOT land a partial signature-exhaustiveness change on the enum branch: on
  its own it leaves a fully-covered enum fn erroring (the pre-existing gap),
  which reads as a regression.

## Reproductions

`design/`-local, runnable against `cmd/go/bin/aql check`:

```
# false no_signature via untyped variable (the core gap)
def render fn [[red/q] [String] ["R"] [green/q] [String] ["G"] [blue/q] [String] ["B"]]
def c red/q
render c

# works — direct literal
render red/q
```
