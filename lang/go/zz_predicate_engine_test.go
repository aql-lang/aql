package lang

import (
	"sync"
	"testing"
)

// TestPredicateBodyRunsOnInterpreter is a MEASUREMENT pinned as a test, and it
// is the reason the `is`-against-a-predicate-type refusal stays.
//
// The eight `function value reaches is (Stage 3)` ledger rows compile cleanly
// the moment RecordCallOperands stops refusing a predicate-type operand — all
// ten shapes probed answered correctly on both lanes, with no FALLBACK in the
// disassembly. That looked like a graduation. It is not one.
//
// A program-level FALLBACK check cannot see a CallBoru made INSIDE a native
// handler, and that is exactly where the predicate runs: RunPredicate reaches
// the body through InvokeCallbackFn, whose contract is "the VM when the body
// compiled to a unit, CallBoru otherwise". Measured through the InterpEntry
// hook, the body is never a unit — every predicate invocation takes the
// interpreter arm.
//
// Relaxing the gate would therefore trade an HONEST whole-program refusal for a
// compiled program with an interpreter island hidden inside a handler, which is
// the one outcome the compilation mission rules out. So the refusal stands
// until predicate bodies compile to units (design/FULL-COMPILATION.0.md §6.3,
// "Predicate and membership-type bodies join the same inventory").
//
// The control row is the load-bearing half: a predicate reached through ORDINARY
// DISPATCH takes the same interpreter arm today, with no `is` and no gate
// involved. So this entry is PRE-EXISTING debt in compiled programs, not
// something the gate protects against — the gate only keeps the census honest
// about it.
func TestPredicateBodyRunsOnInterpreter(t *testing.T) {
	for _, tc := range []struct{ name, src string }{
		{"dispatch through a predicate param (no `is`, no gate)",
			`def Pos fnpred n:Integer [n gt 0]  def f fn [[x:Pos] [Integer] [x]]  f 5`},
		{"a refine predicate at a typed def",
			`def Pos fnpred n:Integer [n gt 0]  def x:Pos 5  x`},
	} {
		a := mustNew(t)
		var mu sync.Mutex
		seams := map[string]int{}
		disarm := a.ArmInterpEntryHook(func(e InterpEntry) {
			mu.Lock()
			defer mu.Unlock()
			if !e.CheckMode {
				seams[e.Seam]++
			}
		})
		_, ran, err := a.RunCompiled(tc.src)
		disarm()
		if !ran || err != nil {
			t.Fatalf("%s: ran=%v err=%v", tc.name, ran, err)
		}
		mu.Lock()
		got := seams["InvokeCallback:callboru"]
		mu.Unlock()
		if got == 0 {
			t.Errorf("%s: expected the predicate body to take InvokeCallback's CallBoru arm, "+
				"saw none — if predicate bodies now compile to units, that is the graduation "+
				"the `function value reaches is (Stage 3)` ledger rows are waiting for: admit "+
				"a predicate-type operand in RecordCallOperands and move those rows into "+
				"lang/spec/fnpred.tsv", tc.name)
		}
	}
}
