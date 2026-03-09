# AQL Test Coverage Report

**Date:** 2026-03-09
**Project:** AQL (Concatenative Query Language)
**Language:** Go 1.24.7
**Overall Coverage: 77.4%**

---

## Summary

| Metric | Value |
|--------|-------|
| Source files | 49 |
| Test files | 50 |
| Source lines | 12,554 |
| Test lines | 26,917 |
| Test-to-source ratio | 2.14x |
| Total test functions | 1,390 |
| **Statement coverage** | **77.4%** |

---

## Go Coverage by Package (measured via `go test -cover`)

| Package | Coverage | Status |
|---------|----------|--------|
| internal/ast | **100.0%** | Excellent |
| internal/lexer | **100.0%** | Excellent |
| internal/object | **100.0%** | Excellent |
| internal/token | **100.0%** | Excellent |
| root (aql.go) | **96.0%** | Excellent |
| cmd/aql | **96.1%** | Excellent |
| internal/fileops | **93.9%** | Very Good |
| internal/evaluator | **87.5%** | Good |
| internal/repl | **84.2%** | Good |
| internal/parser | **80.9%** | Good |
| internal/engine | **80.6%** | Good |
| **internal/native** | **34.5%** | **Poor** |

---

## Package-Level Detail

### Test-to-Source Ratio by Package

| Package | Source Lines | Test Lines | Ratio | Test Funcs | Coverage |
|---------|-------------|------------|-------|------------|----------|
| root (aql.go) | 172 | 834 | 4.85x | 38 | 96.0% |
| cmd/aql | 101 | 142 | 1.41x | 12 | 96.1% |
| internal/ast | 70 | 102 | 1.46x | 5 | 100.0% |
| **internal/engine** | **9,575** | **14,046** | **1.47x** | **780** | **80.6%** |
| internal/evaluator | 28 | 61 | 2.18x | 2 | 87.5% |
| internal/fileops | 93 | 315 | 3.39x | 17 | 93.9% |
| internal/lexer | 52 | 76 | 1.46x | 4 | 100.0% |
| **internal/native** | **1,336** | **581** | **0.43x** | **23** | **34.5%** |
| internal/object | 58 | 50 | 0.86x | 2 | 100.0% |
| internal/parser | 523 | 1,081 | 2.07x | 103 | 80.9% |
| internal/repl | 91 | 83 | 0.91x | 7 | 84.2% |
| internal/token | 65 | 27 | 0.42x | 1 | 100.0% |
| test/ (integration) | — | 7,577 | — | 396 | — |

---

## Detailed Function-Level Coverage

### internal/engine — 80.6% (largest package)

**Functions at 0% coverage:**
- `baseValueForConstraint` (registry.go:2400)
- `loadFileModule` (registry.go:2743)

**Functions below 50%:**
- `registerMake` — 33.1%
- `resolveSelectSubExprs` — 37.8%
- `resolveWhereSubExprs` — 37.8%
- `toQueryBuilder` — 40.0%
- `valToAtomOrString` — 42.9%
- `resolveScalarValue` — 50.0%
- `doWrite` — 52.9%
- `convertTopLevel` — 53.8%
- `convertWordList` — 53.8%
- `registerModule` — 53.8%
- `mergedSchema` — 56.2%
- `installExports` — 57.1%
- `parseColumnSpec` — 57.8%
- `convertDataValue` — 60.0%

### internal/native — 34.5% (worst coverage)

**All functions at 0% (no unit test coverage):**

| Module | Function | Lines |
|--------|----------|-------|
| clone | cloneFunc, cloneHandler | 37 |
| filter | filterFunc, filterHandler | 73 |
| flatten | flattenFunc, flattenDefaultHandler, flattenDepthHandler | 52 |
| getpath | getpathFunc, getpathHandler | 38 |
| inject | injectFunc, injectHandler | 39 |
| items | itemsFunc, itemsHandler | 41 |
| join | joinFunc, joinDefaultHandler, joinSepHandler | 44 |
| jsonify | jsonifyFunc, jsonifyDefaultHandler, jsonifyFlagsHandler | 42 |
| merge | mergeFunc, mergeHandler | 38 |
| pad | padFunc, padDefaultHandler, padWidthHandler | 42 |
| selector | selectorFunc, selectorHandler | 39 |
| setpath | setpathFunc, setpathHandler | 39 |
| size | sizeFunc, sizeHandler | 29 |
| slice | sliceFunc, sliceAllHandler, sliceStartHandler, sliceStartEndHandler | 70 |
| validate | validateFunc, validateHandler | 42 |
| walk | walkFunc, walkHandler, makeWalkApply, walkBeforeHandler, walkBeforeAfterHandler | 156 |
| native | Register, makeFullStackHandler, All | 91 |
| create | createFunc, createRecordHandler | 64 |
| list | listFunc, listRecordAllHandler, listRecordFilterHandler | 118 |
| load | loadFunc, loadRecordHandler | 53 |
| remove | removeFunc, removeRecordHandler | 72 |
| update | updateFunc, updateRecordHandler | 81 |

**Functions with coverage (tested via unit tests):**
- `createHandler` — 93.8%
- `listAllHandler` — 100.0%
- `listFilterHandler` — 91.7%
- `recordMatches` — 100.0%
- `valuesEqual` — 87.5%
- `loadHandler` — 88.9%
- `removeHandler` — 87.0%
- `transformHandler` — 85.7%
- `valueToAny` — 83.3%
- `anyToValue` — 72.7%
- `sortedAnyMapKeys` — 100.0%
- `updateHandler` — 93.1%

### internal/parser — 80.9%

**Functions below 60%:**
- `convertTopLevel` — 53.8%
- `convertTopLevelValue` — 52.9%
- `convertWordList` — 53.8%
- `convertDataValue` — 60.0%

### internal/engine/sqlite.go

**Functions below 50%:**
- `init` — 6.7%
- `aqlValueToSQLParam` — 32.3%
- `toInt64` — 37.5%
- `toString` — 42.9%

---

## Integration Tests (test/)

396 test functions covering end-to-end scenarios:
- Queries (118 tests), Aliases (59 tests), Module chains (56 tests)
- File I/O (36 tests), Currying (21 tests), Resource types (20 tests)
- Struct functions (18 tests), Imports (18 tests), Factorial/recursion (27 tests)
- Definitions (10 tests), Lists (4 tests), Transforms (4 tests), Unify (1 test)

**Note:** Integration tests exercise many native functions indirectly but this
coverage is not reflected in the per-package numbers since Go only counts coverage
within the package being tested.

---

## Recommendations

### High Priority — internal/native at 34.5%

16 of 22 native modules have **zero** unit test coverage. These functions are
exercised indirectly through integration tests, but Go's coverage tool only
measures coverage within the tested package. Adding unit tests would both
increase measured coverage and test edge cases.

Priority targets (by complexity and risk):
1. **walk** — 5 funcs, 156 lines, complex recursive logic
2. **slice** — 4 funcs, 70 lines, boundary conditions
3. **flatten** — 3 funcs, 52 lines, depth handling
4. **filter** — 2 funcs, 73 lines, predicate logic
5. **native.go** — Register/All/makeFullStackHandler (91 lines, core wiring)

### Medium Priority — internal/engine low-coverage functions

Several engine functions are below 50%:
- `registerMake` (33.1%) — large function, 263 lines
- `resolveSelectSubExprs` / `resolveWhereSubExprs` (37.8%) — SQL subexpression handling
- `baseValueForConstraint` (0%) — unused or untested path
- `loadFileModule` (0%) — file-based module loading

### Low Priority

- **internal/parser** — Several convert* functions at ~53%, deeper syntax coverage needed
- **internal/engine/sqlite.go** — Type conversion functions (toInt64, toString) below 50%

---

## How to Regenerate This Report

```bash
cd aql
GONOSUMCHECK='*' GONOSUMDB='*' GOPROXY=off go test -coverprofile=coverage.out ./...
go tool cover -func=coverage.out          # function-level summary
go tool cover -html=coverage.out -o coverage.html  # visual HTML report
```
