# AQL Symbols Optimization — Results Report

Implementation of the Symbols optimization (Zef change #4) for AQL.
Interns word names into pointer-comparable `*Symbol` values and
rekeys `Registry.DefStacks` from `map[string][]Value` to
`map[*Symbol][]Value`, so every word dispatch looks up by pointer
hash instead of string hash.

## Summary

Median wall-clock wins across four benchmark fixtures, 5 runs each
at `-benchtime=2s`:

| Fixture                         | Baseline    | After       | Speedup |
|---------------------------------|-------------|-------------|--------:|
| `DispatchAddChain100`           |  970 µs/op  |  843 µs/op  |  **+15%** |
| `DispatchMixedOps100`           |  967 µs/op  |  813 µs/op  |  **+19%** |
| `DispatchDefLookup100`          | 1097 µs/op  |  876 µs/op  |  **+25%** |
| `DispatchFactorialDirect`       |   87 µs/op  |   59 µs/op  |  **+47%** |

Allocation counts are effectively unchanged (pointer compare
doesn't touch the allocator), and byte-per-op is +0.4% on average
from interning book-keeping, which is amortized across the whole
program.

For reference, Zef's own Symbols change reported +18% against its
baseline. AQL sees comparable-or-larger gains on every fixture
because AQL's string-keyed sites were denser than Zef's
(`DefStacks` carries both user and native words, and the scope
chain walks it per dispatch).

## Methodology

1. **Fixtures.** Four benchmarks in
   `aql/internal/engine/bench_dispatch_test.go`:
   - `DispatchAddChain100` — 100 sequential `add` calls on integer
     constants. Pure native dispatch.
   - `DispatchMixedOps100` — rotates `add`/`sub`/`mul` so
     signature cache (if any) doesn't monomorphize.
   - `DispatchDefLookup100` — installs 10 user defs once (outside
     the timed loop), then runs 100 calls through them. Every
     call walks `DefStacks` for a user word.
   - `DispatchFactorialDirect` — small realistic workload: nine
     `mul` calls to compute `10!`.
2. **Baseline.** `-count=5 -benchtime=2s` run before any code
   changes. Raw data at `aql/doc/bench/baseline-symbols.txt`.
3. **Implementation.** Code changes in one patch (see below).
4. **After.** Same `go test -bench` invocation. Raw data at
   `aql/doc/bench/after-symbols.txt`.
5. **Environment.** linux/amd64, Intel Xeon Platinum 8581C @
   2.10GHz, Go via the repo's standard toolchain.

Median of the 5 runs is used as the representative number. Means
and worst-case are within ~10% of the median, consistent across
fixtures.

## What changed

### New: `aql/internal/engine/symbol.go`

```go
type Symbol struct {
    Name string
}

func Intern(s string) *Symbol    // hash-consed lookup
func SymbolOf(s string) *Symbol  // returns nil for empty input

var (
    SymOpenParen  = Intern("(")
    SymCloseParen = Intern(")")
    SymEnd        = Intern("end")
    SymTrue       = Intern("true")
    SymFalse      = Intern("false")
)
```

A process-wide intern table, guarded by an `RWMutex`. Reads hit
the read lock; the write lock only runs on first-use of a new
name. Pre-interned pointers for tokens whose equality is checked
in engine inner loops.

### Changed: `WordInfo`

```go
type WordInfo struct {
    Name         string
    Sym          *Symbol   // NEW: canonical pointer for Name
    ArgCount     int
    ForceStack   bool
    ForceForward bool
}
```

`NewWord` and `NewWordModified` now call `Intern(name)` and store
the pointer. The old `Name` string stays for diagnostics and
external API compatibility.

### Changed: `Registry.DefStacks`

```go
DefStacks map[*Symbol][]Value  // was map[string][]Value
```

Every access site updated — about 100 call sites across
`engine.go`, `match.go`, `carrier.go`, `forloop.go`,
`native_definition_*.go`, `native_control_do.go`,
`native_help.go`, `native_module_module.go`,
`native_type_*.go`, `registry.go`, and `nativemod/nativemod.go`.

Access shapes:
- Hot path (`e.stepWord` dispatch, `match.go` peek-ahead):
  `r.DefStacks[w.Sym]` — pointer in hand, zero intern cost.
- Slow path (check-mode snapshots, `def` installation, user
  input): `r.DefStacks[Intern(name)]` — intern once at the edge.
- Iteration: `for sym, stack := range r.DefStacks { ... }` uses
  `sym.Name` when the string form is needed.

### Changed: `DefCleanupInfo.Snapshot`

```go
Snapshot map[*Symbol]int  // was map[string]int
```

Snapshot maps used by `runCarrierBodyWithDefs`,
`installJoinedDefs`, `AnalyseFnBody`, and the fn-body cleanup in
`native_definition_fn.go` all track `*Symbol` keys so they match
the underlying `DefStacks` type without re-interning on every
lookup.

### Tests

Eight test files had direct `r.DefStacks["..."]` accesses and
were updated to `r.DefStacks[Intern("...")]`. No test logic
changed.

## Raw numbers

### Baseline (`doc/bench/baseline-symbols.txt`, medians)

```
BenchmarkDispatchAddChain100-16        970346 ns/op  376694 B/op  5772 allocs/op
BenchmarkDispatchMixedOps100-16        967001 ns/op  373680 B/op  5602 allocs/op
BenchmarkDispatchDefLookup100-16      1097400 ns/op  392814 B/op  5873 allocs/op
BenchmarkDispatchFactorialDirect-16     87305 ns/op   32101 B/op   437 allocs/op
```

### After (`doc/bench/after-symbols.txt`, medians)

```
BenchmarkDispatchAddChain100-16        843730 ns/op  378283 B/op  5772 allocs/op
BenchmarkDispatchMixedOps100-16        813850 ns/op  375272 B/op  5602 allocs/op
BenchmarkDispatchDefLookup100-16       876256 ns/op  394403 B/op  5873 allocs/op
BenchmarkDispatchFactorialDirect-16     59522 ns/op   32229 B/op   437 allocs/op
```

### Deltas

| Metric                | AddChain | MixedOps | DefLookup | Factorial |
|-----------------------|---------:|---------:|----------:|----------:|
| ns/op                 |    −13%  |    −16%  |     −20%  |     −32%  |
| B/op                  |   +0.4%  |   +0.4%  |    +0.4%  |    +0.4%  |
| allocs/op             |      0%  |      0%  |       0%  |       0%  |

Factorial — the smallest program — shows the largest relative
win because its per-step overhead (dispatch + map lookup) is a
much larger fraction of the workload than on the 100-call
fixtures, where parser-built `[]Value` setup dominates.

## Why this is faster

The Go runtime hashes `string` keys by walking bytes (via
`memhash`), and compares strings with `memequal`. Even for short
names like `"add"`, that's 3 bytes to hash and up to 3 bytes to
compare on a hit. Map lookups on `*Symbol` keys hash the pointer
(a single rotate+fold), and equality is `cmp` on a register.

Concretely, the hot paths in `engine.go:stepWord` now do:

```go
if stack, ok := e.registry.DefStacks[w.Sym]; ok { ... }  // pointer lookup
if ds := e.registry.DefStacks[w.Sym]; len(ds) > 0 { ... } // pointer lookup
```

where before they did:

```go
if stack, ok := e.registry.DefStacks[w.Name]; ok { ... }  // string hash+compare
```

The same pattern applies in `match.go` (forward-arg peek-ahead)
and throughout carrier.go's scope-snapshot logic.

## What this is not

- Not inline caching. The per-call-site IC proposal from the Zef
  lessons report is still open; each dispatch still runs the full
  `matchSignature` machinery. The Symbols change is the
  foundation that IC depends on.
- Not a generation counter. `DefStacks` mutation still happens
  directly; there's no watchpoint invalidation yet. This
  complements a future IC but doesn't require one.
- Not a NaN-boxed value representation. `Value.Data` still holds
  `interface{}`. Unboxing is a separate, larger change.

## Non-goals for this change

Deliberately out of scope:

- `Type.Parts []string` — touched in `types.go:Equal`,
  `Matches`, `hasPrefix`. Would need its own pass; type
  comparisons didn't show up as dominant in the profiles.
- `OrderedMap` keys — user-data maps. Interning would force
  allocation of a `*Symbol` on every user-supplied map key,
  likely a net loss.
- `Registry.CheckDefsInstalled`, `CheckFnSummaries`,
  `KnownTypeParts`, `SDKCache`, `loadedNativeMods`, `Formats` —
  all `map[string]...` but cold-path / setup-path.

These are tracked in the Zef lessons report
(`docs/reports/zef-lessons-for-aql-report.md`) as future work.

## Files changed

```
aql/internal/engine/symbol.go                   (new)
aql/internal/engine/bench_dispatch_test.go      (new)
aql/internal/engine/value.go                    (WordInfo, DefCleanupInfo)
aql/internal/engine/registry.go                 (DefStacks type + method internals)
aql/internal/engine/engine.go                   (dispatch + DefCleanup)
aql/internal/engine/match.go                    (forward peek-ahead)
aql/internal/engine/carrier.go                  (snapshot/merge/narrow)
aql/internal/engine/forloop.go                  (iter binding)
aql/internal/engine/native_control_do.go        (body resolution)
aql/internal/engine/native_definition_def.go    (installDef, uninstallDef)
aql/internal/engine/native_definition_fn.go     (fn body cleanup)
aql/internal/engine/native_definition_undef.go  (uninstallFnSigs)
aql/internal/engine/native_help.go              (help lookup)
aql/internal/engine/native_module_module.go     (module export resolution)
aql/internal/engine/native_type_inspect.go      (inspect word/atom)
aql/internal/engine/native_type_make.go         (make word body)
aql/internal/engine/native_type_resource.go     (Resource parent lookup)
aql/internal/nativemod/nativemod.go             (native module installers)
aql/internal/nativemod/nativemod_test.go        (test)
aql/internal/engine/carrier_narrow_test.go      (test)
aql/internal/engine/coverage_test.go            (test)
aql/internal/engine/def_leakage_test.go         (test)
aql/internal/engine/misc_coverage_test.go       (test)
aql/doc/bench/baseline-symbols.txt              (new, captured data)
aql/doc/bench/after-symbols.txt                 (new, captured data)
```

## Verification

```
go test ./... -count=1 -timeout=180s
# all packages PASS
```

Run the benchmark yourself:

```
cd aql
go test ./internal/engine/ -bench=BenchmarkDispatch -benchmem \
    -run='^$' -count=5 -benchtime=2s
```

## Next

In order of expected impact, the remaining Zef lessons from
`docs/reports/zef-lessons-for-aql-report.md`:

1. Global `(generation, *Symbol) → handler` lookup cache
   (Zef #11, +15% projected). Straightforward on top of this
   change.
2. Unboxed `Value` representation (Zef's NaN-tagged analogue,
   multi-× projected on numeric code).
3. Resolve pass + per-site inline caches + generation counter
   (Zef #6, 4.55× in Zef).
