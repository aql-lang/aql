// Equivalence tests for the kernel-level eng/go/stackform package
// against the language-layer registry. The contract under test:
//
//	stackform.Eval(reg, stackform.Compile(reg, src)) == native.Engine.Run(src)
//
// for representative AQL programs. Lives here in lang/go/test rather
// than eng/go/stackform_test because the stackform package can't
// import the language layer (upward dependency) but the test needs
// real native words (math, comparison, etc.) to exercise.
package test

import (
	"testing"

	"github.com/aql-lang/aql/eng/go"
	"github.com/aql-lang/aql/eng/go/parser"
	"github.com/aql-lang/aql/eng/go/stackform"
	"github.com/aql-lang/aql/lang/go/native"
)

func stackformReg(t *testing.T) *native.Registry {
	t.Helper()
	r, err := native.DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	r.SetParseFunc(parser.Parse)
	return r
}

// equivalentRun runs `src` through both:
//
//  1. A normal NewTop engine — the "direct" baseline.
//  2. stackform.Compile to record a StackForm, then stackform.Eval
//     to replay it.
//
// Both should produce the same final stack. Differences are
// surfaced as test failures with both stacks in the message.
func equivalentRun(t *testing.T, src string) {
	t.Helper()
	r := stackformReg(t)
	tokens, err := parser.Parse(src)
	if err != nil {
		t.Fatalf("parse %q: %v", src, err)
	}

	// Direct run on a fresh engine for the baseline.
	directReg := stackformReg(t)
	direct, err := native.NewTop(directReg).Run(append([]eng.Value(nil), tokens...))
	if err != nil {
		t.Fatalf("direct run %q: %v", src, err)
	}

	// Compile + Eval round-trip.
	_, form, err := stackform.Compile(r, append([]eng.Value(nil), tokens...))
	if err != nil {
		t.Fatalf("compile %q: %v", src, err)
	}
	round, err := stackform.Eval(r, form)
	if err != nil {
		t.Fatalf("eval %q: %v\n  form: %s", src, err, stackform.Pretty(form))
	}

	if !stacksEqual(direct, round) {
		t.Errorf("%q: stacks differ\n  direct=%v\n  round =%v\n  form=%s",
			src, direct, round, stackform.Pretty(form))
	}
}

func stacksEqual(a, b []eng.Value) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if !eng.DeepEqual(a[i], b[i]) {
			return false
		}
	}
	return true
}

// TestStackFormEquivalence_Arithmetic covers integer + decimal
// arithmetic in both forward and stack form. These are the smallest
// programs the compiler-via-recorder should handle.
func TestStackFormEquivalence_Arithmetic(t *testing.T) {
	for _, src := range []string{
		`1 2 add`,
		`add 1 2`,
		`3 4 mul`,
		`mul 3 4`,
		`10 3 sub`,
		`sub 10 3`,
		`5 2 mul 7 add`,
		`(5 2 mul) 7 add`,
		`100 3 div`,
		`1.5 2.5 add`,
	} {
		t.Run(src, func(t *testing.T) {
			equivalentRun(t, src)
		})
	}
}

// TestStackFormEquivalence_Comparisons + boolean ops.
func TestStackFormEquivalence_Comparisons(t *testing.T) {
	for _, src := range []string{
		`5 3 gt`,
		`5 3 lt`,
		`5 5 eq`,
		`true false and`,
		`true false or`,
		`true not`,
	} {
		t.Run(src, func(t *testing.T) {
			equivalentRun(t, src)
		})
	}
}

// TestStackFormEquivalence_Strings covers string concat / case.
func TestStackFormEquivalence_Strings(t *testing.T) {
	for _, src := range []string{
		`"hello" upper`,
		`"WORLD" lower`,
		`"foo" "bar" add`,
		`add "foo" "bar"`,
	} {
		t.Run(src, func(t *testing.T) {
			equivalentRun(t, src)
		})
	}
}

// TestStackFormEquivalence_StackOps covers dup, swap, drop —
// pure stack manipulation that the recorder needs to track even
// though no math is involved.
func TestStackFormEquivalence_StackOps(t *testing.T) {
	for _, src := range []string{
		`1 dup`,
		`1 2 swap`,
		`1 2 3 drop`,
		`1 2 over`,
		`1 dup mul`, // 1²
		`3 dup mul`, // 9
	} {
		t.Run(src, func(t *testing.T) {
			equivalentRun(t, src)
		})
	}
}

// TestStackFormEquivalence_Lists covers list literals + simple list ops.
func TestStackFormEquivalence_Lists(t *testing.T) {
	for _, src := range []string{
		`[1 2 3]`,
		`[1 2 3] length`,
	} {
		t.Run(src, func(t *testing.T) {
			equivalentRun(t, src)
		})
	}
}

// TestStackFormPrettyRoundTrip checks Pretty produces output that
// the parser can re-ingest into an equivalent StackForm. This is a
// weaker property than Compile-Eval equivalence — it's about the
// human-readable rendering being faithful.
func TestStackFormPrettyRoundTrip(t *testing.T) {
	for _, src := range []string{
		`1 2 add`,
		`5 3 sub 2 mul`,
		`"foo" "bar" add`,
		`true false and`,
	} {
		t.Run(src, func(t *testing.T) {
			r := stackformReg(t)
			tokens, err := parser.Parse(src)
			if err != nil {
				t.Fatal(err)
			}
			_, form1, err := stackform.Compile(r, append([]eng.Value(nil), tokens...))
			if err != nil {
				t.Fatal(err)
			}
			pretty := stackform.Pretty(form1)
			t.Logf("Pretty(%q) = %q", src, pretty)
			tokens2, err := parser.Parse(pretty)
			if err != nil {
				t.Fatalf("re-parse pretty output %q: %v", pretty, err)
			}
			r2 := stackformReg(t)
			_, form2, err := stackform.Compile(r2, tokens2)
			if err != nil {
				t.Fatalf("re-compile: %v", err)
			}
			if !stackform.Equal(form1, form2) {
				t.Errorf("round-trip not stable\n  first  : %s\n  pretty : %s\n  second : %s",
					stackform.Pretty(form1), pretty, stackform.Pretty(form2))
			}
		})
	}
}
