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

### aql:report

Pretty-printers for the kernel value types. Every word returns a
String — none print directly — so callers compose with `print`,
embed in error messages, or feed into further formatting. Useful
beyond testing: any console-bound output of Records and Tables.

**Import:** `"aql:report" import`

| Word            | Description                                                |
|-----------------|------------------------------------------------------------|
| `report.value`  | Generic — any Value to a String (delegates to FormatForPrint). |
| `report.record` | A Map rendered as vertical `key : value` lines, colons aligned. |
| `report.table`  | A TableData or List-of-Maps rendered with headers and aligned columns. |
| `report.list`   | A List rendered one numbered element per line.             |

```
"aql:report" import
{name:"alice" age:30} report.record print
# name : alice
# age  : 30

[{a:1 b:2} {a:3 b:4}] report.table print
# a | b
# --+--
# 1 | 2
# 3 | 4
```

### aql:test

Test framework with two complementary surfaces:

1. **Imperative API** — `test.describe`, `test.test` / `test.it`,
   `assert.equal`, `assert.deep-equal`, `assert.ok`, `assert.throws`,
   `assert.match`. Eager execution (node:test style). Each case
   captures errors so the rest of the suite continues.
2. **Declarative spec runner** — define a `TestSpec` Record carrying
   a `subject` (the word under test, named as an Atom or dotted
   String like `"decision.eval-cond"`) and a list of `TestCase`
   Records (each `{name, in, out}`). Sub-specs nest. `test.run-spec`
   walks the tree, dispatches the subject against each case's `in`
   list, deep-compares the top-of-stack result to `out`, and records
   the outcome.

Results accumulate into a `TestSet` Table that `test.results`
returns; pipe through `report.table` to print.

**Import:** `"aql:test" import`

**Types** (exported via `test.TestCase`, `test.TestSet`, …):

| Type         | Shape                                                                  |
|--------------|------------------------------------------------------------------------|
| `TestCase`   | `refine Record [name:String in:List out:Any]`                          |
| `TestSet`    | `refine Table TestCase`                                                |
| `TestSpec`   | `refine Record [name:String subject:Any cases:List subs:List]`         |
| `TestResult` | `refine Record [name:String path:List ok:Boolean expected:Any actual:Any error:Any duration-ms:Integer]` |

**Imperative words**:

| Word            | Description                                                |
|-----------------|------------------------------------------------------------|
| `test.describe` | Group: `[body] "name" test.describe` — body sees nested describes / tests, results inherit the path. |
| `test.test`     | Run a single case body, catch errors: `[body] "name" test.test`. |
| `test.it`       | Alias for `test.test`.                                     |
| `test.results`  | Return the accumulated TestSet Table.                      |
| `test.summary`  | Return `{total, passed, failed}` Map.                      |
| `test.fail-count` | Return failure count as Integer.                         |
| `test.reset`    | Clear the active TestRun.                                  |

**Assertions** (raise `[aql/assertion_failure]` on failure; caught by enclosing `test`):

| Word                | Description                                |
|---------------------|--------------------------------------------|
| `assert.equal`      | `a b assert.equal` — exact equality (identity for maps/lists). |
| `assert.not-equal`  | Inverse of equal.                          |
| `assert.ok`         | Value is truthy (not None, not false).     |
| `assert.throws`     | `[body] assert.throws` — body must raise.  |
| `assert.match`      | `substring fullString assert.match` — substring contains check. |

**Spec runner**:

| Word              | Description                                                |
|-------------------|------------------------------------------------------------|
| `test.spec`       | Constructor — `name subject cases` → TestSpec (no subs).   |
| `test.spec-with-subs` | Constructor with sub-spec list.                        |
| `test.case`       | Constructor — `name in out` → TestCase.                    |
| `test.run-spec`   | `spec test.run-spec` — execute every case, recurse into subs. |
| `test.invoke`     | `inputs subject test.invoke` — Go-side helper that dispatches a subject by name in the caller's registry. |

```
"aql:test" import
"aql:report" import
"aql:decision" import

def my-spec {
  name: "eval-cond"
  subject: "decision.eval-cond"
  cases: [
    {name: "adult"  in: [{age:25} {field:"age",op:"gte",value:18}] out: true}
    {name: "minor"  in: [{age:15} {field:"age",op:"gte",value:18}] out: false}
  ]
  subs: []
}
my-spec test.run-spec
test.results report.table print
```

The spec runner uses `deq` (deep equality), so structurally-equal
maps and lists match without requiring identity. Subject names with
dots (e.g. `"decision.eval-cond"`) are split into a `get` chain in
the parent registry — flat imports aren't required.

The decision module is exercised entirely via this mechanism in
`modules/decision_spec.aql`; see `modules/decision_spec_test.go`
for the loader.

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
