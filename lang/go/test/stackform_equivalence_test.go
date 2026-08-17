// Equivalence tests for the kernel-level eng/go/stackform package
// against the language-layer registry. The contract under test:
//
//	stackform.Eval(reg, stackform.Compile(reg, src)) == native.Engine.Run(src)
//
// for representative boru programs. Lives here in lang/go/test rather
// than eng/go/stackform_test because the stackform package can't
// import the language layer (upward dependency) but the test needs
// real native words (math, comparison, etc.) to exercise.
package test

import (
	"errors"
	"testing"

	core "github.com/boru-lang/boru/core/go"
	"github.com/boru-lang/boru/lang/go/native"
	"github.com/boru-lang/boru/lang/go/stackform"
	parser "github.com/boru-lang/boru/parser/go"
)

func stackformReg(t *testing.T) *native.Registry {
	t.Helper()
	r, err := native.DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	registerIOWords(r)
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
	direct, err := native.NewTop(directReg).Run(append([]core.Value(nil), tokens...))
	if err != nil {
		t.Fatalf("direct run %q: %v", src, err)
	}

	// Compile + Eval round-trip.
	_, form, err := stackform.Compile(r, append([]core.Value(nil), tokens...))
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

func stacksEqual(a, b []core.Value) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if core.DeepEqual(a[i], b[i]) {
			continue
		}
		// NUR031: a Function value is not `deq` to ITSELF — eq/deq key on the
		// binding name rather than the function — so DeepEqual reports two
		// identical functions as different. Fall back to a hand-written
		// comparison for that one kind. Delete this arm when NUR031 resolves.
		if isFn(a[i]) && isFn(b[i]) && fnValuesEqual(a[i], b[i]) {
			continue
		}
		return false
	}
	return true
}

func isFn(v core.Value) bool {
	return v.Parent != nil && v.Parent.Equal(core.TFunction)
}

// fnValuesEqual compares two Function values for round-trip purposes.
//
// It cannot just be `Canon(a) == Canon(b)`. `canonFnDef` renders name, params,
// returns and body, and DELIBERATELY omits both the closure environment
// (core/go/canon.go: "excludes Registry and Captured") and the Quoted flag —
// omissions that are right for canon's own job (identity for ordering, and not
// spilling a module's exports) and wrong for this one. Two closures capturing
// `x=1` and `x=2` canon identically while invoking to different results, and a
// value the replay left permanently inert canons the same as the live one it
// was recorded from. Both are exactly the corruption these tests exist to
// catch, so compare them explicitly.
func fnValuesEqual(a, b core.Value) bool {
	if a.Quoted != b.Quoted {
		return false
	}
	if core.Canon([]core.Value{a}) != core.Canon([]core.Value{b}) {
		return false
	}
	fa, aok := a.Data.(core.FnDefInfo)
	fb, bok := b.Data.(core.FnDefInfo)
	if !aok || !bok {
		return aok == bok
	}
	if len(fa.Captured) != len(fb.Captured) {
		return false
	}
	for i := range fa.Captured {
		if fa.Captured[i].Name != fb.Captured[i].Name ||
			!core.DeepEqual(fa.Captured[i].Value, fb.Captured[i].Value) {
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
		`[1 2 3] size`,
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
			_, form1, err := stackform.Compile(r, append([]core.Value(nil), tokens...))
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

// TestStackFormEquivalence_UserFunctions is the case the suite above was
// missing entirely: every one of its programs calls a NATIVE word, so the
// contract went untested for a boru `fn` and the recorder broke it for
// years without a red test.
//
// It broke two ways at once. The frame-splice OVER-COUNT: OnCall was handed
// `len(results)`, which for a boru fn is the spliced fn-frame skeleton (eight
// tokens for a 1-arg/1-return fn), not the return count — so the recorder's
// skip counter swallowed unrelated later literals, and `z 99` recorded a form
// that dropped the 99. And the BODY LEAK: the callee's own dispatches were
// recorded at top level, so the form re-ran fragments of the body after the
// call had already run it.
//
// The trailing-literal rows are the over-count's direct witness: a value
// AFTER the call is exactly what an inflated skip count eats.
func TestStackFormEquivalence_UserFunctions(t *testing.T) {
	const z = `def z fn [[] [Integer] [42]] `
	const inc = `def inc fn [[n:Integer] [Integer] [n add 1]] `

	for _, src := range []string{
		z + `z`,
		z + `z 99`,          // over-count witness: 0-arg
		inc + `inc 5`,       // body leak witness: add/__pa/undef leaked
		inc + `(inc 5) 99`,  // over-count witness: 1-arg
		inc + `inc (inc 5)`, // nesting
		inc + `def twice fn [[n:Integer] [Integer] [inc (inc n)]] twice 5`,
		`def p fn [[] [Integer Integer] [1 2]] p`, // multi-return
		`def fact fn [[n:Integer] [Integer] [if (n lte 1) [1] [n mul (fact (n sub 1))]]] fact 4`,
		// A fn handed to a fn as a Function param — the callback shape every
		// boru library uses — with the returned reference held by /r.
		inc + `def ap fn [[f:Function n:Integer] [Integer] [f n]] ap inc/r 5`,
		z + `def h fn [[f:Function] [Any] [f/r]] h z/r 99`,
		// A `/r` reference left as the RESIDUAL, not consumed by a later call.
		// Flatten has to mark a Function PushLit Quoted to stop the replay
		// dispatching it, and Quoted is STICKY where `/r` is positional — so
		// without Eval undoing the mark this row returns a permanently-inert
		// copy of a value the direct run returns live. `canon` omits the flag,
		// so only stacksEqual's explicit check sees it (PR #378 review, P1).
		inc + `inc/r`,
		z + `z/r`,
		// A CLOSURE as the residual: same shape, but the value also carries a
		// captured environment. canon deliberately omits Captured too, so this
		// row is only meaningful because stacksEqual compares it (P2).
		`def mk fn [[x:Integer] [Function] [([y:Integer] => [x add y])]] (mk 5)`,
	} {
		t.Run(src, func(t *testing.T) {
			equivalentRun(t, src)
		})
	}
}

// TestStackFormRefusesFunctionValueApplication pins the NEGATIVE half: a
// program the recorder cannot capture faithfully must be REFUSED, never
// replayed to a different answer than it was recorded from.
//
// Applying a function VALUE — an inline lambda, or a fn read out of a
// container — is not expressible: `Call{Name, Arity}` re-invokes by name and
// does not consume a receiver, while an application consumes the fn value the
// stack already holds. Recording it as a Call would strand that value; the op
// vocabulary needs an apply-style Op it does not have (NUR074).
//
// Before this, these silently evaluated to the FUNCTION rather than its
// result — the same class of quiet wrongness the over-count caused, and the
// reason the PBT shrinker could report a counterexample its generator cannot
// produce.
func TestStackFormRefusesFunctionValueApplication(t *testing.T) {
	for _, src := range []string{
		`([n:Integer] => [n add 1]) 5`,
		`def fs [ fn [[n:Integer] [Integer] [n add 1]] ] ((fs get 0) 5)`,
		`def m {f: (fn [[n:Integer] [Integer] [n add 1]])} (m.f 5)`,
	} {
		t.Run(src, func(t *testing.T) {
			r := stackformReg(t)
			tokens, err := parser.Parse(src)
			if err != nil {
				t.Fatal(err)
			}
			_, form, err := stackform.Compile(r, tokens)
			if err != nil {
				t.Fatalf("compile %q: %v", src, err)
			}
			if err := stackform.Replayable(form); !errors.Is(err, stackform.ErrUnnamedApply) {
				t.Errorf("%q: Replayable = %v, want ErrUnnamedApply\n  form: %s",
					src, err, stackform.Pretty(form))
			}
			if _, err := stackform.Eval(stackformReg(t), form); !errors.Is(err, stackform.ErrUnnamedApply) {
				t.Errorf("%q: Eval = %v, want it to refuse rather than replay", src, err)
			}
		})
	}

	// POSITIVE control: an ordinary named call in the same suite must stay
	// replayable, so the refusal cannot silently widen to everything.
	r := stackformReg(t)
	tokens, err := parser.Parse(`def inc fn [[n:Integer] [Integer] [n add 1]] inc 5`)
	if err != nil {
		t.Fatal(err)
	}
	_, form, err := stackform.Compile(r, tokens)
	if err != nil {
		t.Fatal(err)
	}
	if err := stackform.Replayable(form); err != nil {
		t.Errorf("a named fn call must stay replayable, got %v", err)
	}
}
