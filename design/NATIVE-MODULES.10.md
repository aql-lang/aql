# Native Modules

Native modules are internal AQL modules with Go implementations that are
imported using names of the form `"aql:<name>"`. They produce a module
descriptor with exports, just like file-based modules, and their words
are accessed via dot notation.

## Importing Native Modules

Use the standard `import` word with an `aql:` prefixed string:

```
"aql:math" import
```

This creates a `math` def containing all the module's words. Access
them via dot notation:

```
"aql:math" import

-5 math.abs           # 5
0.5 math.sin          # 0.479...
4 math.sqrt           # 2.0
3 7 math.min          # 3
math.pi               # 3.14159...
```

Native modules are loaded at most once per registry. Subsequent imports
of the same module are no-ops.

## Calling Convention

Native module words accessed via dot notation are FnDef values that
auto-invoke when they find matching args on the stack. Arguments must
appear **before** the dot-access expression:

```
# Unary: value then function
-5 math.abs           # 5
1.5 math.ceil         # 2

# Binary: both args then function
3 7 math.min          # 3
3 4 math.hypot        # 5.0

# Constants: no args needed
math.pi               # 3.14159...
math.e                # 2.71828...
```

To use a module word in a forward-argument position (e.g. inside a list
body), place args before the dot expression:

```
"aql:math" import
for 5 [i 2 math.min]     # 0 1 2 2 2
for 3 [i math.negate]     # 0 -1 -2
```

## Available Native Modules

### aql:math

Extended math operations beyond the built-in arithmetic (`add`, `sub`,
`mul`, `div`, `mod`, `pow`).

**Import:** `"aql:math" import`

#### Unary Operations

| Word           | Description                           | Example               |
|----------------|---------------------------------------|-----------------------|
| `math.abs`     | Absolute value                        | `-5 math.abs` -> `5` |
| `math.negate`  | Flip the sign                         | `3 math.negate` -> `-3` |
| `math.sign`    | Sign: -1, 0, or 1                     | `-7 math.sign` -> `-1` |

#### Min/Max

| Word        | Description                | Example              |
|-------------|----------------------------|----------------------|
| `math.min`  | Smaller of two numbers     | `3 7 math.min` -> `3` |
| `math.max`  | Larger of two numbers      | `3 7 math.max` -> `7` |

#### Rounding

| Word          | Description                        | Example                |
|---------------|------------------------------------|------------------------|
| `math.ceil`   | Round up to nearest integer        | `1.2 math.ceil` -> `2` |
| `math.floor`  | Round down to nearest integer      | `1.8 math.floor` -> `1` |
| `math.round`  | Round to nearest integer           | `1.5 math.round` -> `2` |
| `math.trunc`  | Truncate toward zero               | `1.9 math.trunc` -> `1` |

#### Roots, Exponentials, Logarithms

| Word          | Description                     | Example                |
|---------------|---------------------------------|------------------------|
| `math.sqrt`   | Square root                     | `4 math.sqrt` -> `2.0` |
| `math.cbrt`   | Cube root                       | `8 math.cbrt` -> `2.0` |
| `math.exp`    | e^x                             | `0 math.exp` -> `1.0` |
| `math.log`    | Natural logarithm (base e)      | `1 math.log` -> `0.0` |
| `math.log2`   | Base-2 logarithm                | `8 math.log2` -> `3.0` |
| `math.log10`  | Base-10 logarithm               | `100 math.log10` -> `2.0` |

#### Trigonometry

All trigonometric functions use radians.

| Word          | Description                        | Example                 |
|---------------|------------------------------------|-------------------------|
| `math.sin`    | Sine                               | `0 math.sin` -> `0.0`  |
| `math.cos`    | Cosine                             | `0 math.cos` -> `1.0`  |
| `math.tan`    | Tangent                            | `0 math.tan` -> `0.0`  |
| `math.asin`   | Arc sine                           | `0 math.asin` -> `0.0` |
| `math.acos`   | Arc cosine                         | `1 math.acos` -> `0.0` |
| `math.atan`   | Arc tangent                        | `0 math.atan` -> `0.0` |
| `math.atan2`  | Two-argument arc tangent           | `1 1 math.atan2`       |
| `math.hypot`  | Hypotenuse: sqrt(x^2 + y^2)       | `3 4 math.hypot` -> `5.0` |

#### Constants

| Word      | Description              | Value         |
|-----------|--------------------------|---------------|
| `math.pi` | Pi                       | 3.14159...    |
| `math.e`  | Euler's number           | 2.71828...    |

## Implementation

Native modules live in `modules/`. Each module:

1. Registers Go-implemented words into an isolated sub-registry
2. Creates FnDef wrappers for each word (with the sub-registry for
   closure semantics)
3. Packages them into a `ModuleDesc` with named exports
4. The import handler installs exports as defs via `installExports`

The resolver (`modules.Resolve`) maps module names to their builder
functions and is wired into the registry via `Registry.NativeModResolver`
in `aql.go`.

## Adding a New Native Module

1. Create a new Go file in `modules/` (e.g. `strings.go`)
2. Implement a `BuildStringsModule(parent *engine.Registry) (engine.ModuleDesc, error)`
3. Add the module to the `modules` map in `modules.go`
4. Add help entries in `internal/engine/help/`
5. Add tests in `modules/`
6. Document in this file
