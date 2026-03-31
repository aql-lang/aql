# Unit Test Failure Categorization

**Date:** 2026-03-31 (updated)
**Total failing tests:** 45 (was 52; 7 expandDottedWord tests now fixed)
**Packages affected:** `internal/engine`, `test`

---

## Category 1: Signature Matching Errors (21 tests)

Functions fail with `signature error: no matching signature for <word>`. The engine's
signature-matching logic rejects calls that should be valid.

**Root cause:** The signature resolver doesn't match certain argument patterns — likely
issues with how prefix/forward/stack argument modes interact with type matching for
user-defined and native functions.

| Test | Word | File |
|------|------|------|
| TestEdgeSetWithIntegerKey | `set` | engine_test.go:2365 |
| TestEngineFnConcatArgOrder4Mixed/OnePrefixThreeForward | `mix4` | integration_test.go:1070 |
| TestEngineFnConcatArgOrder7Mixed/OnePrefixSixForward | `mix7` | integration_test.go:1228 |
| TestEngineFnConcatArgOrderEndDisambiguate/EndStopsForward4Mixed | `cat4` | integration_test.go:1307 |
| TestEngineInspectUserDefined | `mul` | integration_test.go:2153 |
| TestAliasSetpath | `setpath` | alias_test.go:567 |
| TestArgOrder/L81 (Options arg) | `add` | arg_order_test.go:69 |
| TestColorHex2rgbRed | `hex2int` | module_work_test.go:26 |
| TestColorHex2rgbComponents | `hex2int` | module_work_test.go:38 |
| TestColorRgb2hex | `int2hex` | module_work_test.go:62 |
| TestColorInt2hex | `pad` | module_work_test.go:86 |
| TestColorMakeColor | `hex2int` | module_work_test.go:98 |
| TestColorRoundTrip | `hex2int` | module_work_test.go:110 |
| TestColorChainHex2rgbThenRgb2hex | `hex2int` | module_work_test.go:361 |
| TestColorChainMakeColorThenAccessRGB | `hex2int` | module_work_test.go:373 |
| TestColorHex2rgbBlack | `hex2int` | module_work_test.go:387 |
| TestColorHex2rgbWhite | `hex2int` | module_work_test.go:405 |
| TestProjectImportInstalledColor | `hex2int` | module_work_test.go:264 |
| TestColorSchemeSunsetName | `hex2int` | module_work_test.go:148 |
| TestColorSchemeOceanSecondaryRoundTrip | `hex2int` | module_work_test.go:483 |
| TestColorSchemeAllSchemesHaveExpectedFields | `hex2int` | module_work_test.go:467 |

**Sub-groups within this category:**

- **Color module (`hex2int`/`int2hex`/`pad`)** — 14 tests. All color-scheme tests fail
  because the `hex2int` native function signature isn't matched at call time. Fixing the
  signature resolver for these native functions would likely fix all 14 at once.
- **Multi-arg fn definitions (`mix4`/`mix7`/`cat4`/`mul`)** — 4 tests. Functions defined
  with mixed prefix+forward signatures of 4+ args can't be called.
- **Builtins (`set`/`setpath`/`add`)** — 3 tests. Specific argument patterns (integer
  keys, Options type args) don't match existing builtin signatures.

---

## Category 2: Argument Order / Concatenation Order (5 tests)

Functions execute successfully but produce arguments in **reversed order** (e.g. "CBA"
instead of "ABC"). The prefix argument collector appears to reverse the order of
collected arguments.

| Test | Details | File |
|------|---------|------|
| TestEngineFnConcatArgOrder/AllPrefix | "A" "B" "C" joiner → 'CBA', want "ABC" | integration_test.go:973 |
| TestEngineFnConcatArgOrder/MixedPrefixForward | "A" joiner "B" "C" → 'BCA', want "ABC" | integration_test.go:989 |
| TestEngineFnConcatArgOrder/TwoPrefixOneForward | "A" "B" joiner "C" → 'CBA', want "ABC" | integration_test.go:1005 |
| TestEngineFnConcatArgOrderEndDisambiguate/EndAfterPartialForward | cat3 = "RQP", want "PQR" | integration_test.go:1392 |
| TestUndefBugNamedStringParams | got 'CBA', want ABC | undef_bug_test.go:35 |

---

## ~~Category 3: expandDottedWord Flag Mismatch~~ (FIXED — 7 tests)

All 7 `expandDotted*` parser tests now pass. The `expandDottedWord()` function was
rewritten to emit `( foo dot a dot b )` instead of `( foo a dot b dot )`, and tests
were updated accordingly. `dot` is now a plain forward-precedence word — no
`ForceStack` or `WordModified` needed.

---

## Category 4: Wrong Computation Results (4 tests)

Tests produce incorrect numeric results, suggesting bugs in the engine's execution
model (step sequencing, prefix chain evaluation, or stack-based computation).

| Test | Got | Want | File |
|------|-----|------|------|
| TestEdgePrefixChain | 21 | 9 | engine_test.go:2160 |
| TestDefForthSumOfSquares | [25] | [147] | engine_test.go:3843 |
| TestEngineCoreStepEndSemicolonSequence | result[0]=6,result[1]=4 | result[0]=3,result[1]=7 | engine_core_coverage_test.go:52 |
| TestMarkMoveBasic | "2222" | "xx" | markmove_test.go:43 |

---

## Category 5: Inspect/Reflection Bugs (2 tests)

The `inspect` word returns empty name fields for user-defined types and record types.

| Test | Details | File |
|------|---------|------|
| TestEngineInspectTypeLiteral | name = "", want "Qty" | integration_test.go:2234 |
| TestEngineInspectRecordType | name = "", want "Pos" | integration_test.go:2266 |

---

## Category 6: Color Module Logic Errors (4 tests)

These color tests don't fail on signature errors but produce wrong results from `clamp`
and comparison logic — returning `"false"` or `"0"` instead of expected numeric values.

| Test | Got | Want | File |
|------|-----|------|------|
| TestColorClamp | "0" | "255" | module_work_test.go:124 |
| TestColorClampZero | "false" | "0" | module_work_test.go:427 |
| TestColorClampMax | "0" | "255" | module_work_test.go:439 |
| TestColorClampMiddle | "0" | "128" | module_work_test.go:451 |

---

## Category 7: Context / Value Types (1 test)

`TestContextDifferentValueTypes` — setting string and boolean values in context returns
wrong results (`None` instead of `"hello"`, `'bool'` instead of `true`).

| Test | File |
|------|------|
| TestContextDifferentValueTypes | context_test.go:208 |

---

## Category 8: typeof Word Bug (1 test)

`typeof` returns the type name once per iteration instead of a single atom describing the
type. `for 3 [i typeof]` → `"Number Number Number"`, want `"Atom"`.

| Test | File |
|------|------|
| TestForLoop/L153_for_3_[i_typeof] | forloop_test.go:78 |

---

## Category 9: Miscellaneous (7 tests)

| Test | Issue | File |
|------|-------|------|
| TestWriteStdout | `write` produces no stdout output | misc_coverage_test.go:372 |
| TestMergeMapList | merge returns 3 elements instead of 4 | struct_functions_test.go:91 |
| TestDefTransformWithLoad | missing `greeting` key in result | transform_parse_test.go:72 |
| TestImportFileRuntimeError | expected runtime error not raised | module_chain_test.go:467 |
| TestColorSchemeHasBothHexAndRGB | cascading hex2int failure | module_work_test.go:233 |
| TestColorSchemeOceanPrimaryHex | cascading hex2int failure | module_work_test.go:184 |
| TestColorSchemeOceanAccentGreen | cascading hex2int failure | module_work_test.go:196 |
| TestColorSchemeNeonPrimaryHex | cascading hex2int failure | module_work_test.go:208 |
| TestColorSchemeNeonDarkBlue | cascading hex2int failure | module_work_test.go:220 |
| TestColorSchemeSunsetPrimaryHex | cascading hex2int failure | module_work_test.go:160 |
| TestColorSchemeSunsetPrimaryRed | cascading hex2int failure | module_work_test.go:172 |

*Note: The 7 ColorScheme tests here also fail due to `hex2int` signature errors (Category 1)
but are listed separately because they test different functionality (scheme field access).*

---

## Summary by Impact

| Category | Count | Status | Likely Fix Scope |
|----------|-------|--------|-----------------|
| 1. Signature matching errors | 21 | OPEN | Engine signature resolver |
| 2. Argument order reversal | 5 | OPEN | Prefix argument collector |
| 3. ~~expandDottedWord flags~~ | ~~7~~ | **FIXED** | ~~Parser `expandDottedWord()`~~ |
| 4. Wrong computation results | 4 | OPEN | Engine step/execution logic |
| 5. Inspect/reflection bugs | 2 | OPEN | `inspect` word handler |
| 6. Color clamp logic | 4 | OPEN | AQL `clamp` implementation |
| 7. Context value types | 1 | OPEN | Context set/get for non-integer types |
| 8. typeof bug | 1 | OPEN | `typeof` word handler |
| 9. Miscellaneous | 7 | OPEN | Various (write, merge, transform, import) |
| **Total** | **45** (was 52) | **7 fixed** | |

**Highest-impact fixes:** Fixing the signature resolver (Category 1) would resolve ~21
tests. Fixing argument order (Category 2) would resolve ~5 more. These two fixes
alone would address ~26 of the 45 remaining failures.
