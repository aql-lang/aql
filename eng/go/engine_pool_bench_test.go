package eng

import "testing"

// Quantifies what the engine pool buys on the interpreter callback hot
// path (design/SUB-ENGINE-MAIN-TAPE-REVIEW.0.md §3): a fresh sub-engine
// pays the tape initial-size floor (1024 entries × 160-byte Values ≈
// 164KB, allocated and zeroed) on EVERY invocation; a pooled engine
// reloads its tape in place.

func benchProgram() []Value { return []Value{NewInteger(1)} }

func BenchmarkSubEngineFresh(b *testing.B) {
	r, err := NewRegistry()
	if err != nil {
		b.Fatal(err)
	}
	toks := benchProgram()
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, err := New(r).Run(toks); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkSubEnginePooled(b *testing.B) {
	r, err := NewRegistry()
	if err != nil {
		b.Fatal(err)
	}
	toks := benchProgram()
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, err := RunPooled(r, toks); err != nil {
			b.Fatal(err)
		}
	}
}
