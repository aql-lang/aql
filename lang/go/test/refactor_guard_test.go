package test

import (
	"strings"
	"testing"

	lang "github.com/aql-lang/aql/lang/go"
	"github.com/aql-lang/aql/lang/go/native"
)

// This file pins the behaviour touched by the duplication-removal
// refactorings (def installAndRecordDef, forceStackWord helper, if2/if3
// mark+move extraction, core_helpers triple-return hoist) so the changes
// are provably non-regressing. Each behaviour has a positive and a
// negative case.

func mustResult(t *testing.T, src string) string {
	t.Helper()
	vals, err := runNativeSteps(t, nil, []string{src})
	if err != nil {
		t.Fatalf("runNativeSteps(%q) error: %v", src, err)
	}
	parts := make([]string, len(vals))
	for i, v := range vals {
		parts[i] = v.String()
	}
	return strings.Join(parts, " ")
}

func mustErr(t *testing.T, src string) string {
	t.Helper()
	_, err := runNativeSteps(t, nil, []string{src})
	if err == nil {
		t.Fatalf("runNativeSteps(%q): expected error, got nil", src)
	}
	return err.Error()
}

// ---- core_helpers.go: synthesized 0-arg fallback handler ----

func TestGuardZeroArgFnDispatch(t *testing.T) {
	// Positive: a 0-arg fn body runs and returns its value.
	if got := mustResult(t, `def greet fn [[] String ["hi"]] greet`); got != `'hi'` {
		t.Errorf(`def greet ... greet = %s, want "hi"`, got)
	}
}

func TestGuardFnWrongArgFallbackErrors(t *testing.T) {
	// Negative: invoking a typed fn with a non-matching arg falls through
	// to the synthesized fallback handler, which returns a signature error.
	msg := mustErr(t, `def f fn [[n:Integer] Integer [n]] "x" f`)
	if !strings.Contains(msg, "no matching signature for f") {
		t.Errorf("expected 'no matching signature for f', got: %s", msg)
	}
}

// ---- engine.go: forward collection + force-stack Pos preservation ----

func TestGuardForwardCollectionEvaluates(t *testing.T) {
	// Positive: forward-collecting words (exercises the forward-completion
	// path that rewrites the word to force-stack) evaluate correctly.
	if got := mustResult(t, `add 1 2`); got != "3" {
		t.Errorf("add 1 2 = %s, want 3", got)
	}
	if got := mustResult(t, `sub 10 3`); got != "-7" {
		t.Errorf("sub 10 3 = %s, want -7", got)
	}
}

func TestGuardForwardCollectedWordErrorHasPosition(t *testing.T) {
	// Negative + position: a forward-collecting fn whose later arg mismatches
	// fails, and the force-stack rewrite must preserve the word's position so
	// the error points at line 2 (not "unknown").
	runErr := runWithSource(t, "def f fn [[a:Integer b:Integer] Integer [a]]\nf 1 \"x\"")
	if runErr == nil {
		t.Fatal("expected error for f 1 \"x\"")
	}
	ae, ok := runErr.(*native.AqlError)
	if !ok {
		t.Fatalf("expected *AqlError, got %T: %v", runErr, runErr)
	}
	if ae.Row != 2 {
		t.Errorf("expected error Row=2 (the call site), got %d", ae.Row)
	}
}

// ---- native_control.go: if2 / if3 (mark+move list cond + scalar cond) ----

func TestGuardIf3ScalarCondition(t *testing.T) {
	if got := mustResult(t, `if true ["yes"] ["no"]`); got != `'yes'` {
		t.Errorf(`if true = %s, want "yes"`, got)
	}
	if got := mustResult(t, `if false ["yes"] ["no"]`); got != `'no'` {
		t.Errorf(`if false = %s, want "no"`, got)
	}
}

func TestGuardIf3ListCondition(t *testing.T) {
	// List condition takes the mark+move path: the list is run and its
	// result chooses the branch.
	if got := mustResult(t, `if [1 gt 0] ["yes"] ["no"]`); got != `'yes'` {
		t.Errorf(`if [1 gt 0] = %s, want "yes"`, got)
	}
	if got := mustResult(t, `if [1 lt 0] ["yes"] ["no"]`); got != `'no'` {
		t.Errorf(`if [1 lt 0] = %s, want "no"`, got)
	}
}

func TestGuardIf2ScalarCondition(t *testing.T) {
	if got := mustResult(t, `if true ["yes"]`); got != `'yes'` {
		t.Errorf(`if2 true = %s, want "yes"`, got)
	}
	// False, no else: produces no value.
	if got := mustResult(t, `if false ["yes"]`); got != "" {
		t.Errorf(`if2 false = %q, want empty`, got)
	}
}

func TestGuardIf2ListCondition(t *testing.T) {
	if got := mustResult(t, `if [1 gt 0] ["yes"]`); got != `'yes'` {
		t.Errorf(`if2 [1 gt 0] = %s, want "yes"`, got)
	}
	if got := mustResult(t, `if [1 lt 0] ["yes"]`); got != "" {
		t.Errorf(`if2 [1 lt 0] = %q, want empty`, got)
	}
}

// ---- native_definition.go: def / def-typed (InstallDef + RecordDef) ----

func TestGuardDefPlain(t *testing.T) {
	if got := mustResult(t, `def x 5 x`); got != "5" {
		t.Errorf("def x 5; x = %s, want 5", got)
	}
}

func TestGuardDefTypedPositive(t *testing.T) {
	if got := mustResult(t, `def y:Integer 7 y`); got != "7" {
		t.Errorf("def y:Integer 7; y = %s, want 7", got)
	}
}

func TestGuardDefTypedNegative(t *testing.T) {
	// Wrong-type body must be rejected.
	msg := mustErr(t, `def z:Integer "str" z`)
	if !strings.Contains(msg, "z") || !strings.Contains(strings.ToLower(msg), "unify") {
		t.Errorf("expected a unify/type error for def z:Integer \"str\", got: %s", msg)
	}
}

func TestGuardDefUnusedDiagnosticHasPosition(t *testing.T) {
	// Check mode: an unused def is flagged, with the def-name position
	// recorded (exercises r.Check.RecordDef in the def handler).
	a, err := lang.New()
	if err != nil {
		t.Fatal(err)
	}
	res, err := a.Check("def unused-thing 1\n42")
	if err != nil {
		t.Fatalf("check: %v", err)
	}
	var found bool
	for _, d := range res.Diagnostics {
		if d.Code == "unused_def" && d.Word == "unused-thing" {
			found = true
			if d.Row != 1 {
				t.Errorf("unused_def Row = %d, want 1", d.Row)
			}
		}
	}
	if !found {
		t.Errorf("expected unused_def diagnostic for 'unused-thing', got %+v", res.Diagnostics)
	}
}
