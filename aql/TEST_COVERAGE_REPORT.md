# AQL Test Coverage Report

**Date:** 2026-03-09
**Project:** AQL (Concatenative Query Language)
**Language:** Go 1.24.7

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

---

## Go Coverage by Package (measured)

These packages compiled and ran successfully with `go test -cover`:

| Package | Coverage | Status |
|---------|----------|--------|
| internal/ast | **100.0%** | Excellent |
| internal/lexer | **100.0%** | Excellent |
| internal/object | **100.0%** | Excellent |
| internal/token | **100.0%** | Excellent |
| internal/fileops | **93.9%** | Very Good |

**Note:** The remaining packages (`engine`, `native`, `evaluator`, `parser`, `repl`,
`cmd/aql`, root, `test/`) could not compile in this environment due to missing
dependencies (`modernc.org/sqlite`, local replacement `github.com/voxgig/struct`).
The analysis below is based on static code inspection.

---

## Package-Level Analysis

### Test-to-Source Ratio by Package

| Package | Source Lines | Test Lines | Ratio | Test Funcs |
|---------|-------------|------------|-------|------------|
| root (aql.go) | 172 | 834 | 4.85x | 38 |
| cmd/aql | 101 | 142 | 1.41x | 12 |
| internal/ast | 70 | 102 | 1.46x | 5 |
| **internal/engine** | **9,575** | **14,046** | **1.47x** | **780** |
| internal/evaluator | 28 | 61 | 2.18x | 2 |
| internal/fileops | 93 | 315 | 3.39x | 17 |
| internal/lexer | 52 | 76 | 1.46x | 4 |
| **internal/native** | **1,336** | **581** | **0.43x** | **23** |
| internal/object | 58 | 50 | 0.86x | 2 |
| internal/parser | 523 | 1,081 | 2.07x | 103 |
| internal/repl | 91 | 83 | 0.91x | 7 |
| internal/token | 65 | 27 | 0.42x | 1 |
| test/ (integration) | — | 7,577 | — | 396 |

---

## Detailed Package Assessment

### Well-Tested Packages

- **internal/ast** — 100% coverage, all node types tested
- **internal/lexer** — 100% coverage, tokenization fully covered
- **internal/object** — 100% coverage, object model tested
- **internal/token** — 100% coverage, token lookup tested
- **internal/fileops** — 93.9% coverage, memory and OS file operations tested
- **internal/engine** — Largest package (9,575 src lines) with 780 test functions
  covering: compare, conditional, context, engine core, fileio, forloop, format,
  integration, mark/move, print, signature, trace, type scaling, value
- **internal/parser** — 103 test functions, parsing well-covered
- **root (aql.go)** — 38 test functions covering public API

### Integration Tests (test/)

396 test functions covering end-to-end scenarios:
- Aliases (59 tests), Queries (118 tests), Module chains (56 tests)
- File I/O (36 tests), Currying (21 tests), Resource types (20 tests)
- Struct functions (18 tests), Imports (18 tests), Factorial/recursion (27 tests)
- Definitions (10 tests), Lists (4 tests), Transforms (4 tests), Unify (1 test)

---

## Coverage Gaps

### 1. internal/native — LOW COVERAGE (ratio 0.43x, 23 tests for 22 modules)

Only 6 of 22 native modules have direct unit tests:

| Module | Funcs | Direct Unit Tests | Integration Coverage |
|--------|-------|-------------------|---------------------|
| clone | 2 | None | Yes (TestAliasClone) |
| create | 3 | Yes (entity_test.go) | Yes (TestAliasCreate) |
| **filter** | 2 | None | Partial |
| **flatten** | 3 | None | None identified |
| **getpath** | 2 | None | Yes (TestAliasGetpath) |
| **inject** | 2 | None | Yes (TestAliasInject) |
| **items** | 2 | None | None identified |
| **join** | 3 | None | Yes (TestAliasJoin) |
| **jsonify** | 3 | None | None identified |
| list | 7 | Yes (list_test.go) | Yes (TestAliasList) |
| load | 3 | Yes (entity_test.go) | Yes (TestAliasLoad) |
| **merge** | 2 | None | Yes (TestAliasMerge) |
| **pad** | 2 | None | Yes (TestAliasPad) |
| remove | 3 | Yes (entity_test.go) | Yes (TestAliasRemove) |
| **selector** | 2 | None | None identified |
| **setpath** | 2 | None | Yes (TestAliasSetpath) |
| **size** | 2 | None | Yes (TestAliasSize) |
| **slice** | 4 | None | Yes (TestAliasSlice) |
| transform | 5 | Yes (transform_test.go) | Yes (TestAliasTransform) |
| update | 3 | Yes (entity_test.go) | Yes (TestAliasUpdate) |
| **validate** | 2 | None | Yes (TestAliasValidate) |
| **walk** | 5 | None | None identified |

**16 of 22 native modules have no direct unit tests.** Many are exercised
indirectly through integration tests (alias tests, query tests), but lack
targeted edge-case testing.

### 2. internal/token — MINIMAL (1 test function)

Only `TestLookupIdent` exists. Token type definitions and string representations
are not tested.

### 3. internal/object — MINIMAL (2 test functions)

Only basic Inspect tests. 10 exported functions exist but most lack direct tests.

### 4. internal/repl — LOW (7 tests, ratio 0.91x)

REPL logic and history management have basic tests but limited coverage for
edge cases.

### 5. internal/evaluator — MINIMAL (2 tests)

Small module (28 lines) with basic delegation tests.

---

## Recommendations

### High Priority

1. **Add unit tests for native modules** — 16 modules with 0 direct tests.
   Priority targets:
   - `walk` (5 funcs, 156 lines, complex recursive logic)
   - `slice` (4 funcs, 70 lines, boundary conditions)
   - `flatten` (3 funcs, 52 lines, depth handling)
   - `jsonify` (3 funcs, 42 lines, serialization edge cases)
   - `filter` (2 funcs, 73 lines, predicate logic)

2. **Expand internal/object tests** — 10 exported functions, only 2 tests

### Medium Priority

3. **Add edge-case tests for token** — Token string representations, unknown tokens
4. **Expand repl tests** — Error handling, incomplete input, multi-line expressions
5. **Run full coverage profiling** in an environment with all dependencies available:
   ```bash
   go test -coverprofile=coverage.out ./...
   go tool cover -html=coverage.out -o coverage.html
   go tool cover -func=coverage.out
   ```

### Low Priority

6. **internal/evaluator** — Small module, basic delegation, acceptable as-is
7. **cmd/aql** — 12 tests for CLI, reasonable for its scope

---

## Overall Assessment

**Strong test foundation** — The project has 1,390 test functions with a 2.14x
test-to-source line ratio. The core engine is heavily tested (780 tests), the
parser is well-covered (103 tests), and there are 396 integration tests.

**Main gap:** The `internal/native` package has 22 functional modules but only
6 have direct unit tests. While many are exercised indirectly through integration
tests, the lack of targeted unit tests means edge cases (empty inputs, type
mismatches, boundary conditions) likely go untested.

**Measured coverage for compilable packages averages 98.8%** (ast, lexer, object,
token at 100%, fileops at 93.9%).
