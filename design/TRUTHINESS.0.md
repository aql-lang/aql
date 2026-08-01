# TRUTHINESS — the One Truthiness Model

> **Status:** Design document (first cut). Commissioned by the NUR
> uniformity review (`design/NUR-RESOLUTION-PLAN.0.md`): truthiness
> spans several NUR records (NUR000, NUR001, NUR002, NUR003, NUR004,
> NUR016-as-reviewed), so the model is specified **once, here**, and
> every record and reference points at this document instead of
> restating fragments. An ADR stating "One Truthiness Model" as a
> language principle is a recorded candidate (candidate 1 in the
> resolution plan); this document is its substrate.

## 1. The single truthiness model

boru has exactly **one** truthiness rule, implemented in exactly one
function — `CoerceBoolean` (`eng/go/core_helpers.go`) — and every
construct that asks "is this value true?" routes through it. There is
no per-construct variation: `if`, `case` predicate arms, the boolean
word family, `convert Boolean`, `make Boolean`, and the mask/predicate
collection words all share the one rule, in both execution engines
(the interpreter and the bytecode VM call the same function).

**The rule is presence, not content.** A value is **false** exactly
when it is one of:

| Value | Why false |
|---|---|
| `false` | the Boolean false |
| `0`, `0.0` (any numeric zero) | zero magnitude |
| `none` | absence itself |
| `""` (empty String) | no content present |
| `[]` (empty List, plain or flex) | no elements present |
| `{}` (empty Map, plain or flex) | no entries present |

**Everything else is true.** In particular:

- `"false"`, `"0"`, `"no"`, `" "` are **true** — a String's characters
  are never inspected. (The historical magic-token rule that made the
  one string `"false"` falsy was removed — `design/WAT-AUDIT.5.md` §E.)
- `-1`, `0.5`, `nan` are **true** (non-zero magnitude; NaN is not a
  zero).
- A non-empty container is true regardless of what it contains:
  `[false]` and `{a: none}` are **true**.
- Non-concrete containers (a bare `List`/`Map` carrier) coerce false —
  there is nothing present to be true.

Two deliberate edge details, stated so they cannot rot silently:

- **The unresolved-literal carve-out.** A non-String value that
  *renders* as `false` (a bare `false` clause condition reaching
  truthiness as a Word/Atom, a quoted `false` atom) keeps its boolean
  reading and coerces false. This is the tail of `CoerceBoolean` and
  exists so `if [false …]`-shaped code cannot invert its meaning.
- **Exotic container backings** (table types, query builders — list
  or map types not backed by the ordinary element storage) are
  truthy: they are present, and emptiness is not cheaply observable
  for them.

## 2. Boolean coercion — `convert Boolean` / `make Boolean`

`convert Boolean` and `make Boolean` apply the same presence rule —
NOT content parsing:

```
convert Boolean "false"      # → true   (non-empty string is present)
convert Boolean ""           # → false
convert Boolean 0            # → false
make Boolean [1]             # → true
```

This is the divergence NUR001 records and allows: `convert
<ScalarType> <String>` parses content for every other scalar target,
but Boolean conversion shares **one** coercion rule with `if`
truthiness. Making the conversion path parse content would fork
truthiness into two rules — a worse non-uniformity than the one it
fixes.

**Content parsing is an explicit opt-in:** `convert Boolean {truthy:
true} <src>` first matches a String against the YAML boolean tokens
(`y`/`yes`/`true`/`on`, `n`/`no`/`false`/`off`, case-insensitive,
trimmed) and falls back to presence for anything unrecognised and for
every non-String source; it never raises. The option is inert for
non-Boolean targets. (`yamlTruthy` / `coerceBooleanTruthy`,
`lang/go/native/native_type.go`.)

## 3. Conditionals

Every conditional construct coerces its condition by the one rule:

- **`if`** — both arities (`if cond [then]`, `if cond [then] [else]`).
  The condition is coerced; the branches are quotations, evaluated
  only when taken.
- **`case`** — a **predicate clause** (`[gt 3]`-style match list) runs
  with the scrutinee on the stack and its result coerces by the one
  rule to decide the match. (Value and type clauses unify instead —
  matching, not truthiness.)
- **Loop continuation conditions** (the `while`-style checks in the
  control words) coerce the same way.
- **The VM mirrors the interpreter**: compiled conditionals call the
  same `CoerceBoolean` (`eng/go/vm.go`), so a program cannot change
  its branching by being compiled.

**Standing obligation:** any FUTURE conditional construct — new
control words, guard clauses, ternaries, pattern guards — MUST route
its condition through `CoerceBoolean`. A second truthiness predicate
anywhere in the tree is a non-uniformity and a PR blocker under the
NUR discipline.

## 4. The connectives: operand-returning `and` / `or`

`and` and `or` are **value-selecting connectives**, not strict-Boolean
operators (NUR003, Allowed):

- `a and b` → `b` if `a` is truthy, else `a`
- `a or b` → `a` if `a` is truthy, else `b`

The **left operand decides**; the returned value is one of the two
operands, unchanged and uncoerced — of whatever type:

```
1 and 2          # → 2
false 5 and      # → false     (stack form; the falsy left decides)
0 9 or           # → 9
'x' or 'dflt'    # → 'x'
```

This is the deliberate Lisp/Python semantics: the operand form
composes directly (`x or default`), and a strict Boolean is one
`not not` away. The rest of the boolean family — `not`, `xor`, `any`,
`all`, and the `boru:logic-util` gates (`nand`/`nor`/`xnor`/`iff`/
`implies`) — coerces by the one rule and returns **strict Boolean**.

**`otherwise` is the None-aware sibling, and it is NOT truthiness.**
`a otherwise b` selects on *none-ness* (`IsNoneShape`: the `none`
sentinel or the `None` literal), not on the falsy set — `0 otherwise
9` → `0` (zero is not none), where `0 or 9` → `9`. Keep the two
selection rules distinct when documenting either.

## 5. Short-circuit and evaluation order

Precision matters here, because "short-circuit" means two different
things in the literature:

- **Selection is short-circuit** (Lisp/Python style): the connective
  returns the deciding operand without touching the other, and no
  coercion is applied to the result.
- **Evaluation is strict.** boru is a stack/forward-collection
  language: both operand *expressions* are evaluated — in ordinary
  source order, by the engine's normal collection — before the
  connective runs; the handlers are pure selectors over the two
  already-computed values. `and`/`or` do NOT suppress evaluation of
  the non-selected operand's expression.
- **Deferred evaluation is spelled with quotations.** Where the
  suppression matters (an expensive or effectful right-hand side),
  the conditional forms carry it: `if cond [expensive]` evaluates
  `[expensive]` only when taken. That is the uniform boru idiom for
  laziness — quotation, not connective magic.

## 6. Interaction with typing

The non-uniform return type of `and`/`or` never degrades static
analysis to `Any` (`lang/go/native/native_boolean.go`):

- **Concrete fold** (`foldOrJoin`): when both operands are concrete,
  the selection is statically determined, so check mode runs the pure
  handler and types the EXACT selected operand — `and 0 false` types
  `Boolean`, not a union. A downstream op then rejects the real type
  (`add (and 0 false) 0` fails the checker) instead of optimistically
  accepting a union member that won't exist at runtime.
- **Operand join** (`operandJoinReturns`): with a non-concrete
  operand the selection is unknown, so the result types as the join
  of the two operand carriers — sound (the runtime result is always
  one of the operands) and still narrower than `Any`.
- The strict-Boolean family types `Boolean` throughout, and Boolean
  arithmetic is a *defined error* with a check-mode mirror
  (`booleanArithReturns` — NUR000): the diagnostic teaches the
  logical vocabulary (`and`/`or`/`xor`/`not`) instead of silently
  adopting an arbitrary arithmetic.

## 7. Where truthiness is consumed (the complete registry)

The audit obligation from NUR001 ("document every place where
truthiness is used"). Every site routes through `CoerceBoolean`:

| Construct | Site |
|---|---|
| `if` condition (both arities) | `lang/go/native/native_control.go` / `conditional.go`; interpreter fold `eng/go/engine.go`; VM branch `eng/go/vm.go` |
| `case` predicate clauses | `conditional.go` (`caseClauses`) |
| Loop continuation conditions | `native_control.go` |
| `not` | `native_boolean.go` |
| `and` / `or` (selection test) | `native_boolean.go` |
| `any` / `all` (per element) | `native_boolean.go` |
| `xor` and the `boru:logic-util` gates (`nand`/`nor`/`xnor`/`iff`/`implies`) | `boolBinaryNative`, `impliesHandler` |
| `convert Boolean` (and the `{truthy: true}` fallback) | `native_type.go` |
| `make Boolean` | `eng/go/core_make.go` |
| `ArrayUtil.where` (truthy elements → indices) | `native_array.go` |
| Mask-consuming array words (`expand` and kin) | `native_array.go` |

Anything not in this table that wants a boolean reading of a value
must call `CoerceBoolean` and add itself here.

## 8. Rationale

1. **One rule is learnable; two are a trap.** The falsy set is six
   entries long and enumerable from memory. Every fork (a construct
   with its own truthiness, a content-parsing conversion) doubles the
   audit surface and creates the JavaScript-style corner museum.
2. **Presence over content.** Parsing content is a *conversion policy*
   question with locale/format baggage (`"no"`, `"off"`, `"0"`,
   `"FALSE"`); presence is structural and type-uniform. The YAML
   opt-in exists precisely so the policy question has an explicit,
   named home instead of leaking into the default.
3. **Operand-returning connectives compose.** `x or default` and
   `m.k and (f m.k)` are the idioms the choice buys; strictness is
   always one `not not` away, while un-losing a discarded operand is
   impossible.
4. **The engines must agree.** Truthiness is consumed on both sides of
   the compile boundary; a single shared function is what makes
   "compiled programs branch identically" a fact rather than a hope.

## 9. Examples

```
if 1 ['yes'] ['no']              # → 'yes'
if "" ['yes'] ['no']             # → 'no'
if "false" ['yes'] ['no']        # → 'yes'   (content never inspected)
if [] ['yes'] ['no']             # → 'no'
if [false] ['yes'] ['no']        # → 'yes'   (non-empty list)

convert Boolean "false"          # → true    (presence)
convert Boolean {truthy: true} "no"     # → false  (YAML opt-in)
convert Boolean {truthy: true} "maybe"  # → true   (unrecognised → presence)

1 and 2                          # → 2
0 and 2                          # → 0
'' or 'dflt'                     # → 'dflt'
0 otherwise 9                    # → 0       (otherwise is None-aware, not truthiness)
none otherwise 9                 # → 9

not ""                           # → true
any [0 "" 3]                     # → true
all [1 "x" {}]                   # → false
```

## 10. Relations

- **NUR000** — Boolean arithmetic is a defined error (the diagnostic
  recommends the logical vocabulary).
- **NUR001** — `convert Boolean` coerces by presence (this document
  is the required design doc; the record points here).
- **NUR002** — finite-domain `case` exhaustiveness (value coverage,
  adjacent to but distinct from truthiness).
- **NUR003** — `and`/`or` operand return (specified in §4–§6).
- **NUR004** — no `True`/`False` lattice leaves (value-layer
  machinery, §1).
- **ADR candidate** — "One Truthiness Model" as a language principle
  (`design/NUR-RESOLUTION-PLAN.0.md`, ADR candidate 1; to be added to
  ADR.md only on explicit maintainer instruction).
