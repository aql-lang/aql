package lang

import (
	"fmt"
	"strings"
	"testing"
)

// checkDiags runs boru check over src and returns (hasErrorSeverity,
// all diagnostic codes joined).
func checkDiags(t *testing.T, src string) (bool, string) {
	t.Helper()
	a, err := New()
	if err != nil {
		t.Fatal(err)
	}
	res, err := a.Check(src)
	if err != nil {
		t.Fatalf("%q: %v", src, err)
	}
	hasErr := false
	codes := []string{}
	for _, d := range res.Diagnostics {
		codes = append(codes, string(d.Severity)+":"+d.Code)
		if string(d.Severity) == "error" {
			hasErr = true
		}
	}
	return hasErr, strings.Join(codes, ",")
}

// Completion plan 2.2 — predicate-refinement guard narrowing: an
// `x is T` guard narrows x to T's MINTED node in the then-branch, so
// validate-then-call checks clean (previously a gating no_signature
// false positive — the check-by-default blocker). Plan 2.3 — a guard
// whose binding already entails T emits the non-gating redundant_guard
// advisory.
func TestPredicateGuardNarrowing(t *testing.T) {
	clean := []string{
		// paren-form guard
		`def Big (Integer gt 10) def g fn [[n:Big] [Integer] [n]] def f fn [[x:Integer] [Integer] [if (x is Big) [g x] [0]]] f 50`,
		// list-form guard
		`def Big (Integer gt 10) def g fn [[n:Big] [Integer] [n]] def f fn [[x:Integer] [Integer] [if [x is Big] [g x] [0]]] f 50`,
	}
	for _, src := range clean {
		if hasErr, codes := checkDiags(t, src); hasErr {
			t.Errorf("validate-then-call flagged: %s\n%q", codes, src)
		}
	}

	// NEGATIVE: no guard → still a gating rejection.
	if hasErr, _ := checkDiags(t, `def Big (Integer gt 10) def g fn [[n:Big] [Integer] [n]] def f fn [[x:Integer] [Integer] [g x]] f 50`); !hasErr {
		t.Error("unguarded refinement call not flagged")
	}
	// NEGATIVE: the ELSE branch is not narrowed.
	if hasErr, _ := checkDiags(t, `def Big (Integer gt 10) def g fn [[n:Big] [Integer] [n]] def f fn [[x:Integer] [Integer] [if (x is Big) [0] [g x]]] f 50`); !hasErr {
		t.Error("else-branch refinement call not flagged")
	}

	// Runtime parity: the guard is real at run time.
	for _, c := range []struct {
		arg  string
		want string
	}{{"50", "[50]"}, {"5", "[0]"}} {
		a, _ := New()
		got, err := a.RunInterp(`def Big (Integer gt 10) def g fn [[n:Big] [Integer] [n]] def f fn [[x:Integer] [Integer] [if (x is Big) [g x] [0]]] f ` + c.arg)
		if err != nil {
			t.Fatalf("run f %s: %v", c.arg, err)
		}
		if out := fmt.Sprint(got); out != c.want {
			t.Errorf("f %s = %s, want %s", c.arg, out, c.want)
		}
	}

	// 2.3: a guard the binding already entails emits redundant_guard (info).
	if _, codes := checkDiags(t, `def Big (Integer gt 10) def h fn [[n:Big] [Integer] [if (n is Big) [1] [2]]] h 50`); !strings.Contains(codes, "info:redundant_guard") {
		t.Errorf("redundant guard not advised: %s", codes)
	}
	// NEGATIVE: the discharging guard (x:Integer, genuinely narrowing) stays quiet.
	if _, codes := checkDiags(t, `def Big (Integer gt 10) def g fn [[n:Big] [Integer] [n]] def f fn [[x:Integer] [Integer] [if (x is Big) [g x] [0]]] f 50`); strings.Contains(codes, "redundant_guard") {
		t.Errorf("legit guard advised as redundant: %s", codes)
	}
	// NEGATIVE: a dynamic binding's guard discharges the modality — quiet.
	if _, codes := checkDiags(t, `def f fn [[x:Any] [Integer] [if (x is Integer) [1] [2]]] f 5`); strings.Contains(codes, "redundant_guard") {
		t.Errorf("dynamic-binding guard advised as redundant: %s", codes)
	}
}
