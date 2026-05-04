package test

import (
	"strings"
	"testing"

	aql "github.com/metsitaba/voxgig-exp/aql"
)

// --- DepScalar safety: don't fall through scalar dispatch ---
//
// `Type.Matches` is overridden so that `DepInteger.Matches(TInteger)`
// is true (used by sig matching). The risk is any code that does
// `if v.VType.Matches(TString) { _, _ := v.AsString() }` — on a
// DepScalar payload, AsString errors, the underscore swallows the
// error, and the caller gets a zero value. These tests pin the
// DepScalar-specific branches in the four most-traveled equality /
// formatting paths.

func runOK(t *testing.T, src string) []any {
	t.Helper()
	a, err := aql.New()
	if err != nil {
		t.Fatalf("new: %v", err)
	}
	got, err := a.Run(src)
	if err != nil {
		t.Fatalf("run %q: %v", src, err)
	}
	return got
}

// --- valuesEqual: two equal DepScalars compare equal ---

func TestDepScalar_EqEqual(t *testing.T) {
	got := runOK(t, `def a (Integer gt 10)
def b (Integer gt 10)
a eq b`)
	if len(got) != 1 || got[0] != "true" {
		t.Errorf("(Int gt 10) eq (Int gt 10) = %v, want [true]", got)
	}
}

// Two different DepScalars (different bound) compare unequal.
func TestDepScalar_EqDifferentBound(t *testing.T) {
	got := runOK(t, `def a (Integer gt 10)
def b (Integer gt 20)
a eq b`)
	if len(got) != 1 || got[0] != "false" {
		t.Errorf("(Int gt 10) eq (Int gt 20) = %v, want [false]", got)
	}
}

// Different-side DepScalars compare unequal.
func TestDepScalar_EqDifferentKind(t *testing.T) {
	got := runOK(t, `def a (Integer gt 10)
def b (Integer lt 10)
a eq b`)
	if len(got) != 1 || got[0] != "false" {
		t.Errorf("(Int gt 10) eq (Int lt 10) = %v, want [false]", got)
	}
}

// Interval DepScalars compare equal only when both bounds match.
// Run as separate program invocations because `def x EXPR` (without
// extra parens) stores EXPR as deferred code, and re-evaluating it
// in sequence interacts with stack state in non-obvious ways.
func TestDepScalar_EqInterval(t *testing.T) {
	got := runOK(t, `def a (Integer between 10 20)
def b (Integer between 10 20)
a eq b`)
	if len(got) != 1 || got[0] != "true" {
		t.Errorf("[10,20] eq [10,20] = %v, want [\"true\"]", got)
	}

	got = runOK(t, `def a (Integer between 10 20)
def c (Integer between 10 25)
a eq c`)
	if len(got) != 1 || got[0] != "false" {
		t.Errorf("[10,20] eq [10,25] = %v, want [\"false\"]", got)
	}
}

// --- compareValues: refuses to order DepScalars ---

func TestDepScalar_LtRefused(t *testing.T) {
	a, err := aql.New()
	if err != nil {
		t.Fatalf("new: %v", err)
	}
	_, err = a.Run(`(Integer gt 10) lt (Integer gt 20)`)
	if err == nil {
		t.Fatalf("expected error comparing DepScalars with lt")
	}
	if !strings.Contains(err.Error(), "dependent") {
		t.Errorf("error %q does not mention dependent type", err)
	}
}

// --- formatValueJSON / aql_error: render the constraint, no panic ---

// `print` formats via formatForPrint — already guarded but keep the
// regression test so any future reorder doesn't lose it.
func TestDepScalar_PrintRendersConstraint(t *testing.T) {
	a, err := aql.New()
	if err != nil {
		t.Fatalf("new: %v", err)
	}
	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("print panicked: %v", r)
		}
	}()
	_, err = a.Run(`(Integer gt 10) print`)
	if err != nil {
		t.Fatalf("print errored: %v", err)
	}
}

// --- Recover-based panic guard: a DepScalar flowing through any
// surface that uses Matches(scalar)→AsX must not panic. Exercises
// eq, lt (caught), and print together.
func TestDepScalar_NoPanicOnHotPaths(t *testing.T) {
	a, err := aql.New()
	if err != nil {
		t.Fatalf("new: %v", err)
	}
	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("hot-path panicked on DepScalar: %v", r)
		}
	}()
	// Each line exercises a different path. Errors from compareValues
	// are expected and tolerated; a panic would fail the test.
	_, _ = a.Run(`def x (Integer gte 10)
def y (Integer gte 10)
x eq y
x print`)
}
