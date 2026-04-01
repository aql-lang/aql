# Native Modules

Native modules are internal AQL modules with Go implementations that are
imported using names of the form `"aql:<name>"`. They provide extended
functionality beyond the built-in words.

## Importing Native Modules

Use the standard `import` word with an `aql:` prefixed string:

```
"aql:math" import
```

Native modules are loaded at most once per registry. Subsequent imports
of the same module are no-ops.

## Available Native Modules

### aql:math

Extended math operations beyond the built-in arithmetic (`add`, `sub`,
`mul`, `div`, `mod`, `pow`).

**Import:** `"aql:math" import`

#### Unary Operations

| Word     | Description                           | Example          |
|----------|---------------------------------------|------------------|
| `abs`    | Absolute value                        | `-5 abs` -> `5`  |
| `negate` | Flip the sign                         | `3 negate` -> `-3` |
| `sign`   | Sign: -1, 0, or 1                     | `-7 sign` -> `-1` |

#### Min/Max

| Word  | Description                | Example            |
|-------|----------------------------|--------------------| 
| `min` | Smaller of two numbers     | `3 min 7` -> `3`  |
| `max` | Larger of two numbers      | `3 max 7` -> `7`  |

#### Rounding

| Word    | Description                        | Example              |
|---------|------------------------------------|----------------------|
| `ceil`  | Round up to nearest integer        | `1.2 ceil` -> `2`   |
| `floor` | Round down to nearest integer      | `1.8 floor` -> `1`  |
| `round` | Round to nearest integer           | `1.5 round` -> `2`  |
| `trunc` | Truncate toward zero               | `1.9 trunc` -> `1`  |

#### Roots, Exponentials, Logarithms

| Word    | Description                     | Example              |
|---------|---------------------------------|----------------------|
| `sqrt`  | Square root                     | `4 sqrt` -> `2.0`   |
| `cbrt`  | Cube root                       | `8 cbrt` -> `2.0`   |
| `exp`   | e^x                             | `0 exp` -> `1.0`    |
| `log`   | Natural logarithm (base e)      | `1 log` -> `0.0`    |
| `log2`  | Base-2 logarithm                | `8 log2` -> `3.0`   |
| `log10` | Base-10 logarithm               | `100 log10` -> `2.0`|

#### Trigonometry

All trigonometric functions use radians.

| Word    | Description                        | Example               |
|---------|------------------------------------|-----------------------|
| `sin`   | Sine                               | `0 sin` -> `0.0`     |
| `cos`   | Cosine                             | `0 cos` -> `1.0`     |
| `tan`   | Tangent                            | `0 tan` -> `0.0`     |
| `asin`  | Arc sine                           | `0 asin` -> `0.0`    |
| `acos`  | Arc cosine                         | `1 acos` -> `0.0`    |
| `atan`  | Arc tangent                        | `0 atan` -> `0.0`    |
| `atan2` | Two-argument arc tangent (y x)     | `1 atan2 1`          |
| `hypot` | Hypotenuse: sqrt(x^2 + y^2)       | `3 hypot 4` -> `5.0` |

#### Constants

| Word      | Description              | Value         |
|-----------|--------------------------|---------------|
| `math-pi` | Pi (stack-only)          | 3.14159...    |
| `math-e`  | Euler's number (stack-only) | 2.71828... |

## Implementation

Native modules live in `internal/nativemod/`. Each module has:

- A Go file with registration functions (e.g. `math.go`)
- A `Register<Name>(r *engine.Registry)` function that registers all
  words for that module

The resolver (`nativemod.Resolve`) maps module names to their
registration functions and is wired into the registry via
`Registry.NativeModResolver` in `aql.go`.

## Adding a New Native Module

1. Create a new Go file in `internal/nativemod/` (e.g. `string.go`)
2. Implement a `RegisterString(r *engine.Registry)` function
3. Add the module to the `modules` map in `nativemod.go`
4. Add help entries in `internal/engine/help/`
5. Add tests in `internal/nativemod/`
6. Document in this file
