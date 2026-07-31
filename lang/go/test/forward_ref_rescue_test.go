package test

import (
	"strings"
	"testing"

	lang "github.com/boru-lang/boru/lang/go"
)

// Forward-reference rescue (eng RescueForwardRefDiagnostics): a fn
// body runs at CALL time, so a body reference to a name defined
// LATER in the program is the documented forward-reference idiom
// (recursion via forward ref, mutual recursion — lang/spec/
// recursion.tsv §3), not a defect. The install-time body analysis
// used to flag it undefined_word, which both polluted check results
// and blocked the bytecode pass from compiling mutual recursion.

func undefinedWordErrors(res lang.CheckResult) []string {
	var out []string
	for _, d := range res.Diagnostics {
		if d.Code == "undefined_word" {
			out = append(out, d.Detail)
		}
	}
	return out
}

// Positive: the spec's mutual-recursion idiom checks clean.
func TestCheckMutualForwardRefClean(t *testing.T) {
	a, err := lang.New()
	if err != nil {
		t.Fatalf("new: %v", err)
	}
	res, err := a.Check(`def isod fn [[n:Integer] [Boolean] [if (n eq 0) [false] [isev (n sub 1)]]] def isev fn [[n:Integer] [Boolean] [if (n eq 0) [true] [isod (n sub 1)]]] isev 10`)
	if err != nil {
		t.Fatalf("check: %v", err)
	}
	if u := undefinedWordErrors(res); len(u) != 0 {
		t.Errorf("mutual recursion flagged undefined_word: %v", u)
	}
}

// Positive: a plain forward reference (callee defined after the
// caller) checks clean too.
func TestCheckSimpleForwardRefClean(t *testing.T) {
	a, err := lang.New()
	if err != nil {
		t.Fatalf("new: %v", err)
	}
	res, err := a.Check(`def f fn [[n:Integer] [Integer] [g n]] def g fn [[n:Integer] [Integer] [n add 1]] f 3`)
	if err != nil {
		t.Fatalf("check: %v", err)
	}
	if u := undefinedWordErrors(res); len(u) != 0 {
		t.Errorf("forward reference flagged undefined_word: %v", u)
	}
}

// Negative: a body reference to a name that is NEVER defined keeps
// its diagnostic — the rescue applies only to names bound by the end
// of the pass.
func TestCheckGenuineBodyTypoStillFlagged(t *testing.T) {
	a, err := lang.New()
	if err != nil {
		t.Fatalf("new: %v", err)
	}
	res, err := a.Check(`def f fn [[n:Integer] [Integer] [zzyzx n]] f 3`)
	if err != nil {
		t.Fatalf("check: %v", err)
	}
	u := undefinedWordErrors(res)
	if len(u) == 0 || !strings.Contains(strings.Join(u, " "), "zzyzx") {
		t.Errorf("genuine fn-body typo not flagged: %v", res.Diagnostics)
	}
}

// Negative: the rescue is fn-body-only. A TOP-LEVEL use before the
// def genuinely errors at run time and keeps its diagnostic even
// though the name is bound by end of pass.
func TestCheckTopLevelUseBeforeDefStillFlagged(t *testing.T) {
	a, err := lang.New()
	if err != nil {
		t.Fatalf("new: %v", err)
	}
	res, err := a.Check(`zzyzx 3 def zzyzx fn [[n:Integer] [Integer] [n]] zzyzx 4`)
	if err != nil {
		t.Fatalf("check: %v", err)
	}
	u := undefinedWordErrors(res)
	if len(u) == 0 || !strings.Contains(strings.Join(u, " "), "zzyzx") {
		t.Errorf("top-level use-before-def not flagged: %v", res.Diagnostics)
	}
}

// Negative: a typo'd body of a fn that is never called is still
// flagged — the install-time analysis is what catches it, and the
// rescue must not eat it. (The fn name must be unique across this
// package's tests: the dynamic-help example cache is global by name,
// and a cache hit skips the install-time analysis entirely — a
// pre-existing property, not part of the rescue.)
func TestCheckUncalledFnBodyTypoStillFlagged(t *testing.T) {
	a, err := lang.New()
	if err != nil {
		t.Fatalf("new: %v", err)
	}
	res, err := a.Check(`def uncfq fn [[n:Integer] [Integer] [zzyzx n]] 1 add 2`)
	if err != nil {
		t.Fatalf("check: %v", err)
	}
	u := undefinedWordErrors(res)
	if len(u) == 0 || !strings.Contains(strings.Join(u, " "), "zzyzx") {
		t.Errorf("uncalled fn's body typo not flagged: %v", res.Diagnostics)
	}
}
