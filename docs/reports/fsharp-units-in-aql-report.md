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
