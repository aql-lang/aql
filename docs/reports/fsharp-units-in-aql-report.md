# F# Units of Measure in AQL: Short Feasibility Report

## Scope
Evaluate whether F#-style units of measure (dimensional tagging of numeric values, checked at signature match time) can be applied to AQL.

## F# Units Recap
- Numeric types are annotated with units: `1.5<m>`, `float<m/s^2>`.
- Units form an algebra under `*`, `/`, integer powers; dimensionless is `1`.
- Checked statically, erased at runtime, supports generic unit variables.
- Catches dimensional mismatch bugs (`m + s` is a type error).
- Does not do unit conversion or affine offsets — those stay explicit.

## AQL Baseline
- `Scalar/Number` with `Integer` and `Decimal` subtypes.
- Hierarchical type paths with subtype matching (`Matches()`).
- Signature-based dispatch on argument types.
- Typed records and typed lists/maps already model structured values.

## Mapping Sketch
- Represent a unit as a structured tag: product of base units with integer exponents, e.g. `{m:1, s:-2}`.
- Attach tag to a `Number` either as a **subtype extension** (`Number/Quantity/<tag-id>`) or as a **wrapper record** `{value:Number, unit:UnitTag}`.
- Extend arithmetic words (`add`, `sub`, `mul`, `div`, `pow`) to check/propagate tags.
- Provide dimensionless fallback: unadorned numbers keep current behaviour.

## Feasibility Verdict

| Area | Verdict |
|---|---|
| Wrapper-record units (library only) | **Feasible now**, no engine changes |
| Subtype-based units via hierarchical type path | **Feasible**, needs dynamic type-path generation |
| Signature dispatch on unit tags | **Feasible** via `Matches()` extension for tagged number types |
| Generic unit variables in signatures | **Moderate** — signature matcher has no unit-variables today |
| Fully static checking (F#-equivalent) | **Hard** — AQL dispatch is runtime; best AQL can do is fail fast at call site |
| Unit algebra for `mul`/`div`/`pow` | **Feasible**, mechanical |
| Conversion between compatible units (m ↔ cm) | **Feasible** as explicit `convert` word |
| Affine units (°C/K, timestamps) | **Out of scope**, same as F# |

## Syntax Recommendations

### Design constraints
- AQL is concatenative: every token is a word or a literal. New syntax must either be a literal form or a word.
- `<` and `>` are used as comparison words, so F#'s `1.5<m>` form would clash.
- Atoms are bare unquoted words, so `9.81 m` parses today as *number then atom* — giving us a ready composition point.
- Existing map syntax `{m:1, s:-2}` already expresses an exponent-keyed unit tag cleanly.

### Tier 1 — library-only, no parser change (preferred first step)
Use a word that pairs a number with a unit map.

```aql
def g qty 9.81 {m:1, s:-2}      # 9.81 m/s^2
def d qty 100 {m:1}              # 100 m
def t qty 10 {s:1}               # 10 s
```

Convenience constructors per base unit avoid map noise:

```aql
def g 9.81 m/s^2                 # m/s^2 is a unit word that rebuilds the tag
def d 100 m
def t 10 s
mul d (div 1 t)                  # -> qty 10 {m:1, s:-1}
```

Here `m`, `s`, `m/s^2`, `kg`, etc. are **unit words** registered in a `aql:units` module. Each takes a `Number` from the stack and returns a tagged `Quantity`. Composite unit words (`m/s`, `N`, `J`) are just predefined compositions.

### Tier 2 — unit-literal sugar (opt-in, one parser hook)
If Tier 1 proves noisy, introduce a single postfix-marker token. Two candidates, both avoid the `<…>` clash:

**Option A — `#` suffix** (recommended):
```aql
9.81#m/s^2
100#m
10#s
```
Parse rule: after a number literal, `#` followed by a unit expression (slash, caret, digits, atoms) produces a `Quantity` literal.

**Option B — `` ` `` suffix**: conflicts with template strings — rejected.

**Option C — bracketed**: `9.81[m/s^2]` — conflicts with list-index intuition — rejected.

### Unit expressions inside the tag
Regardless of literal form, the unit side uses a tiny grammar reused for both literals and the `qty` word:

```
unit   := factor ( ('*' | '/') factor )*
factor := base ( '^' integer )?
base   := atom | '(' unit ')'
```

Examples:
- `m`
- `m/s`
- `m/s^2`
- `kg*m/s^2`
- `1/s` (dimensionless numerator allowed)

This grammar produces the same `{base:exponent}` map everywhere, so `qty 9.81 m/s^2` and `9.81#m/s^2` are interchangeable.

### Type-annotation syntax
For record fields and function signatures, annotate a numeric type with a unit tag using the typed-map convention already in AQL:

```aql
type Velocity record [speed:Number#m/s]
type Force    record [magnitude:Number#kg*m/s^2]

def accelerate fn [
  [[v:Number#m/s, a:Number#m/s^2, t:Number#s] [Number#m/s]]
  [a t mul v add]
]
```

The `Number#<unit>` form is a compound type literal parsed as `Number` refined by a unit tag. Signature matching treats unit mismatch the same as type mismatch.

### Generic unit variables
Use a leading `'` on an atom inside the unit expression to mark a unit variable, echoing F#'s `'u`:

```aql
def sqr fn [[x:Number#'u] [Number#'u^2]] [x x mul]
def per-second fn [[x:Number#'u] [Number#'u/s]] [x 1#s div]
```

Unit variables are matched structurally at call time: the matcher records a binding on first occurrence and checks consistency on the rest.

### Pretty-printing
- Default string form of a `Quantity` is `<value><space><unit>`, e.g. `9.81 m/s^2`.
- Inspection/REPL form shows the tag map when `verbose` is set: `qty(9.81, {m:1, s:-2})`.

### Syntax recommendation
Ship **Tier 1 only** first: `qty`, unit words per base, and `Number#<unit>` in type positions kept as documentation convention (no parser change yet). Promote to **Tier 2** (the `#` literal suffix and real compound type parsing) once usage in 2-3 realistic modules confirms the noise cost of Tier 1.

## Recommended First Cut
1. Library word `qty` builds a tagged number: `qty 9.81 {m:1, s:-2}`.
2. Arithmetic words gain `[Quantity, Quantity]` signatures that check tag compatibility and propagate via unit algebra.
3. `convert` word applies a scale factor and retags.
4. Errors use existing error-value convention: `unit-mismatch`, `non-integer-exponent`.
5. Defer generic unit variables and engine-native tagging until real usage justifies them.

## Risk
- Dispatch cost: tagged-number signatures widen the match set for hot arithmetic words.
- Interop: tagged numbers flowing into untagged words must degrade gracefully (strip tag or error — pick one and document).
- Serialization: JSON/record round-trips need a canonical encoding of unit tags.

## Verdict
**Feasible as a library-first subsystem.** AQL cannot replicate F#'s zero-cost static guarantee, but a runtime-checked equivalent at signature-match time captures the same class of dimensional bugs with minimal engine change.
