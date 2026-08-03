package lang

import (
	"strings"
	"testing"
)

// check_error_handler_test.go pins the NUR049 resolution: `error` handler
// bodies are checked on the PLAIN pass (errorReturnsFn's seeded body run,
// un-gated from the compile pass 2026-08-03), so the one body the checker
// never visited now has the same diagnostic surface as everything else —
// and the same surface as the compile pass (the review's §8.4.2
// one-diagnostic-surface direction). Positive and negative per the repo
// discipline.
func TestErrorHandlerBodyPlainCheck(t *testing.T) {
	diag := func(src string) []CheckDiagnostic {
		t.Helper()
		a, _ := New()
		res, err := a.Check(src)
		if err != nil {
			t.Fatalf("%q: check harness error: %v", src, err)
		}
		return res.Diagnostics
	}
	hasError := func(diags []CheckDiagnostic, code, detailSub string) bool {
		for _, d := range diags {
			if d.Code == code && d.Severity == SeverityError && strings.Contains(d.Detail, detailSub) {
				return true
			}
		}
		return false
	}

	// A typo in a handler body is now a STATIC error (was: runtime-only,
	// the NUR049 witness `do [raise bad_input "x"] error [zzz-undefined]`).
	if !hasError(diag(`do [raise bad_input "x"] error [zzz-undefined]`), "undefined_word", "zzz-undefined") {
		t.Error("a handler-body typo must flag undefined_word on the plain pass")
	}

	// The sealed-group receiverless accessor — NUR049's shipped-example
	// idiom `def why (dot message)` — is now PROVEN statically: the paren
	// seals the stack receiver, so no `dot` candidate can match.
	if !hasError(diag(`do [raise bad_input "x"] error [ def why (dot message) why ]`), "no_signature", "dot") {
		t.Error("the sealed-group receiverless dot must flag no_signature on the plain pass")
	}

	// NEGATIVES — the working idioms stay clean.
	for _, src := range []string{
		`do [raise bad_input "x"] error [dot message]`,                 // sequential receiver read
		`do [raise bad_input "x"] error [dot message def why end why]`, // the repaired example arm shape
		`do [raise bad_input "x"] error [drop "fallback"]`,             // discard-and-default
		`do [7] error [drop 9]`,                                        // no-raise pass-through
	} {
		for _, d := range diag(src) {
			if d.Severity == SeverityError {
				t.Errorf("%q: working handler idiom must stay clean, got [%s] %s", src, d.Code, d.Detail)
			}
		}
	}

	// A NON-CONCRETE handler operand (a computed fn result in the handler
	// slot) keeps the historical wide dynamic(Any) bound — the seeded run
	// only fires over a concrete token body, so nothing new is analysed and
	// no diagnostics appear.
	for _, d := range diag(`def mk fn [[] [Any] [7]] do [7] error (mk)`) {
		if d.Severity == SeverityError {
			t.Errorf("a non-concrete handler operand must stay un-analysed, got [%s] %s", d.Code, d.Detail)
		}
	}

	// The seeded run also narrows the PLAIN-pass result bound (the join was
	// compile-pass-only before): a scalar handler over a scalar body reports
	// a real family, not dynamic(Any).
	a, _ := New()
	res, err := a.Check(`do [7] error [drop 9]`)
	if err != nil {
		t.Fatalf("check harness error: %v", err)
	}
	if len(res.Stack) != 1 || !strings.Contains(res.Stack[0], "Integer") {
		t.Errorf("plain-pass error bound must narrow to the Integer join, got %v", res.Stack)
	}
}

// TestStrandedForwardSequentialHint pins the NUR049 help-text repair: when a
// pending forward is starved by a BARRIER-RECEIVER boundary word (`dot` — its
// accessor sigs read the receiver from the enclosing stack, which a paren
// group seals off), the stranded-forward error offers the sequential
// spelling alongside the paren fix, never only the dead grouped form.
func TestStrandedForwardSequentialHint(t *testing.T) {
	a, _ := New()
	_, err := a.RunInterp(`do [raise bad_input "x"] error [ def why dot message why ]`)
	if err == nil {
		t.Fatal("the stranded def must error")
	}
	if !strings.Contains(err.Error(), "group the call in parens") {
		t.Errorf("the paren fix must stay first, got %q", err.Error())
	}
	if !strings.Contains(err.Error(), "run it first") {
		t.Errorf("the sequential fallback must be offered for a barrier-receiver boundary, got %q", err.Error())
	}
	// NEGATIVE: an all-forward boundary word keeps the plain suggestion — no
	// sealed-stack caveat (mul reads nothing from beyond a paren).
	b, _ := New()
	_, err2 := b.RunInterp(`def zw mul 2 3`)
	if err2 != nil && strings.Contains(err2.Error(), "run it first") {
		t.Errorf("an all-forward boundary must not gain the sequential caveat: %q", err2.Error())
	}
}
