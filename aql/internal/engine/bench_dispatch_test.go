package engine

// Performance fixtures for the Symbol (Zef #4) migration.
//
// These benchmarks are permanent fixtures, not tests. They exercise
// the hot paths that string-keyed dispatch runs through:
//
//   - Word.Name lookup in Registry.DefStacks (tight arithmetic loop)
//   - Signature matching by name (varied native calls)
//   - Scope chain walking (nested def / undef)
//   - Literal list auto-evaluation (shared between all three)
//
// Run with:
//   go test -bench=BenchmarkDispatch -benchmem -run='^$' \
//       -count=5 ./internal/engine/...
//
// Each fixture is intentionally a small, easily-understood program
// so that regressions can be pinpointed. Fixtures do not depend on
// the parser — values are built directly as []Value streams — so
// changes in the parser do not show up as noise.

import (
	"testing"
)

// ----------------------------------------------------------------------
// Shared helpers
// ----------------------------------------------------------------------

func benchRegistry(b *testing.B) *Registry {
	b.Helper()
	reg, err := DefaultRegistry()
	if err != nil {
		b.Fatal(err)
	}
	return reg
}

// ----------------------------------------------------------------------
// Arithmetic dispatch — many calls to a single native word
// ----------------------------------------------------------------------

// BenchmarkDispatchAddChain100 runs 100 sequential `add` calls over
// integer constants. This is pure dispatch cost: every iteration
// looks up "add" in the Registry, matches its signature, invokes
// the native handler, and returns. No user-defined words.
func BenchmarkDispatchAddChain100(b *testing.B) {
	reg := benchRegistry(b)
	input := makeAddChain(100)
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		eng := New(reg)
		_, _ = eng.Run(input)
	}
}

// BenchmarkDispatchMixedOps100 varies the native word per call so
// the per-signature caches (if any) do not dominate. Each of
// add/sub/mul is hit ~33 times per run.
func BenchmarkDispatchMixedOps100(b *testing.B) {
	reg := benchRegistry(b)
	input := makeMixedOpChain(100)
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		eng := New(reg)
		_, _ = eng.Run(input)
	}
}

// ----------------------------------------------------------------------
// Scope-heavy workload — DefStacks lookup pressure
// ----------------------------------------------------------------------

// BenchmarkDispatchDefLookup100 defines 10 user words once, then
// inside the measured loop runs a 100-call sequence that dispatches
// through those defs. This is where the string-keyed DefStacks map
// hash dominates in the baseline. The def installation happens before
// ResetTimer so only the call cost is measured.
func BenchmarkDispatchDefLookup100(b *testing.B) {
	reg := benchRegistry(b)
	setup := makeDefInstallProgram(10)
	calls := makeDefCallSequence(10, 100)
	// Install defs once on the shared registry.
	if _, err := NewTop(reg).Run(setup); err != nil {
		b.Fatal(err)
	}
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		eng := New(reg)
		_, _ = eng.Run(calls)
	}
}

// ----------------------------------------------------------------------
// Parse + run — realistic workload
// ----------------------------------------------------------------------

// BenchmarkDispatchFactorialDirect runs a hand-built factorial-ish
// reduction without user-defined recursion. This keeps the parser
// out of the measurement while still using multiple natives.
func BenchmarkDispatchFactorialDirect(b *testing.B) {
	reg := benchRegistry(b)
	// 1 2 mul 3 mul 4 mul 5 mul 6 mul 7 mul 8 mul 9 mul 10 mul
	input := []Value{NewInteger(1)}
	for n := int64(2); n <= 10; n++ {
		input = append(input, NewInteger(n), NewWord("mul"))
	}
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		eng := New(reg)
		_, _ = eng.Run(input)
	}
}

// ----------------------------------------------------------------------
// Program builders
// ----------------------------------------------------------------------

// makeAddChain returns [0, 1, add, 1, add, 1, add, ...] with n adds.
func makeAddChain(n int) []Value {
	out := make([]Value, 0, 1+2*n)
	out = append(out, NewInteger(0))
	for i := 0; i < n; i++ {
		out = append(out, NewInteger(1), NewWord("add"))
	}
	return out
}

// makeMixedOpChain rotates through add / sub / mul.
func makeMixedOpChain(n int) []Value {
	ops := []string{"add", "sub", "mul"}
	out := make([]Value, 0, 1+2*n)
	out = append(out, NewInteger(1))
	for i := 0; i < n; i++ {
		out = append(out, NewInteger(2), NewWord(ops[i%len(ops)]))
	}
	return out
}

// makeDefInstallProgram builds:
//
//	def f0 [add] def f1 [add] ... def fK [add]
//
// Used as one-shot setup before the measured loop.
func makeDefInstallProgram(defs int) []Value {
	out := make([]Value, 0, 3*defs)
	for i := 0; i < defs; i++ {
		out = append(out,
			NewWord("def"),
			NewWord(fnName(i)),
			NewList([]Value{NewWord("add")}),
		)
	}
	return out
}

// makeDefCallSequence builds:
//
//	0 1 f0 1 f1 1 f2 ... (total calls = calls)
//
// Each call walks DefStacks by name for dispatch.
func makeDefCallSequence(defs, calls int) []Value {
	out := make([]Value, 0, 1+2*calls)
	out = append(out, NewInteger(0))
	for i := 0; i < calls; i++ {
		out = append(out, NewInteger(1), NewWord(fnName(i%defs)))
	}
	return out
}

func fnName(i int) string {
	// 10 fixed names — short enough to keep string hash cost low
	// but distinct enough that interning is meaningful.
	names := []string{
		"f0", "f1", "f2", "f3", "f4",
		"f5", "f6", "f7", "f8", "f9",
	}
	return names[i%len(names)]
}
