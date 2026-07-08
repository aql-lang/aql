package lang

import "testing"

// countErrors returns the number of error-severity diagnostics for src.
func countErrors(t *testing.T, src string) (int, []string) {
	t.Helper()
	a, err := New()
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	res, err := a.Check(src)
	if err != nil {
		t.Fatalf("Check(%q): %v", src, err)
	}
	n := 0
	var details []string
	for _, d := range res.Diagnostics {
		if d.Severity == "error" {
			n++
			details = append(details, d.Detail)
		}
	}
	return n, details
}

// A value-or-None branch inside a fn body must type-check cleanly. Before the
// joinCarriersInner None-arm guard, joining Integer (or Any) with the None arm
// produced a Parent-less carrier that HALTED the fn-body analyser
// ("[aql/halt]: undefined stack entry"), so the whole fn wrongly reported a
// fn_body_error and a spurious "produces no return value". This is the exact
// shape of mini-redis's `arg-at` (`if (size gt i) [xs get i] [None]`).
func TestNoneBranchInFnBodyChecksClean(t *testing.T) {
	for _, src := range []string{
		`def f fn [[b:Boolean] [Any] [ if b [99] [None] ]] f true`,
		`def f fn [[b:Boolean] [Any] [ if b [None] [99] ]] f true`,
		`def arg-at fn [[xs:List i:Integer] [Any] [ if ((size xs) gt i) [ xs get i ] [ None ] ]] arg-at [10 20] 0`,
	} {
		if n, ds := countErrors(t, src); n != 0 {
			t.Errorf("expected 0 errors for %q, got %d: %v", src, n, ds)
		}
	}
}

// A side-effect for-loop (body nets 0 per iteration) with a CARRIER count must
// contribute no residual, so a trailing statement is the sole return. Before
// the forCarrierAnalyse 0-net guard the loop wrongly returned a single List
// carrier, over-counting the fn's return arity as a false
// "expected 1 return value(s), got 2". Both the count form and the
// computed-range form (mini-s3's s3-send-resp `for [0 total 65536]`) must check
// clean — even though the computed-range form falls back to the interpreter at
// compile time rather than lowering to bytecode (a separate axis).
func TestForZeroNetBodyChecksClean(t *testing.T) {
	for _, src := range []string{
		`def f fn [[n:Integer] [Any] [ for n [ 5 drop ] 0 ]] f 3`,
		`def f fn [[n:Integer] [Any] [ for [0 n 1] [ 5 drop ] 0 ]] f 3`,
		`def f fn [[n:Integer] [Any] [ for [0 n 65536] [ 5 drop ] 0 ]] f 3`,
	} {
		if n, ds := countErrors(t, src); n != 0 {
			t.Errorf("expected 0 errors for %q, got %d: %v", src, n, ds)
		}
	}
}

// The negative side of the same contract: a VALUE-producing loop genuinely
// leaves one residual per iteration, so a fn declaring a single return whose
// body is `for n [5] 0` really does over-return (n copies plus the trailing 0).
// The 0-net guard must NOT suppress this true error.
func TestForValueProducingBodyStillErrors(t *testing.T) {
	src := `def f fn [[n:Integer] [Any] [ for n [ 5 ] 0 ]] f 3`
	n, ds := countErrors(t, src)
	if n == 0 {
		t.Fatalf("expected a return-count error for a value-producing loop, got none")
	}
	found := false
	for _, d := range ds {
		if containsSub(d, "return value") {
			found = true
		}
	}
	if !found {
		t.Errorf("expected a 'return value(s)' error, got %v", ds)
	}
}

func containsSub(s, sub string) bool {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}
