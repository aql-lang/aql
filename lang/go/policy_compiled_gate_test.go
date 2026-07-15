package lang

import (
	"fmt"
	"strings"
	"testing"

	"github.com/aql-lang/aql/lang/go/policy"
)

// The compiled-path policy gate (SECURITY, found 2026-07-13): compiled
// dispatch does not consult the engine word policy — the interpreter's
// policyGateWord runs per stepWord dispatch — so before this gate a
// "deny add" policy interpreted `1 add 2` to permission-denied but RAN IT
// COMPILED to 3 (aql run's default CompileTry path had the bypass; exec
// inherited it when it moved to the compiled entry). A policy-gated
// registry now refuses compilation and the interpreter owns every
// dispatch. Lifting the refusal requires a VM-side policy gate (plan
// Phase 10 item).
func denyAddPolicy(t *testing.T) Policy {
	t.Helper()
	pol, err := policy.LoadInline(`{
		name: "no-add",
		scopes: { engine: { words: { default: "allow", rules: [{ deny: ["add"] }] } } }
	}`)
	if err != nil {
		t.Fatal(err)
	}
	return pol
}

func TestPolicyGatedRegistryRefusesCompilationWithParity(t *testing.T) {
	// Legacy refusal+fallback-parity contract: pins the one-release
	// AQL_COMPILE_FALLBACK=1 hatch behavior (Stage J flipped the default
	// to compile_refused; migrate this contract or retire it with the hatch).
	t.Setenv("AQL_COMPILE_FALLBACK", "1")
	pol := denyAddPolicy(t)
	a, _ := New(Options{Policy: pol})
	gotI, errI := a.RunInterp("1 add 2")
	if errI == nil || !strings.Contains(errI.Error(), "permission denied") {
		t.Fatalf("interpreter must deny: got %v / %v", gotI, errI)
	}

	b, _ := New(Options{Policy: pol})
	gotC, ran, reason, errC := b.RunCompiledReason("1 add 2")
	if ran {
		t.Fatal("a policy-gated registry must never run compiled")
	}
	if !strings.Contains(reason, "policy-gated registry") {
		t.Errorf("reason = %q, want the policy-gate refusal", reason)
	}
	if fmt.Sprint(errC) != fmt.Sprint(errI) || fmt.Sprint(gotC) != fmt.Sprint(gotI) {
		t.Errorf("fallback parity: compiled=%v/%v interp=%v/%v", gotC, errC, gotI, errI)
	}

	// Strict mode surfaces the refusal instead of bypassing the policy.
	c, _ := New(Options{Policy: pol})
	if _, err := c.RunCompiledStrict("1 add 2"); err == nil ||
		!strings.Contains(err.Error(), "policy-gated registry") {
		t.Errorf("RunCompiledStrict = %v, want the policy-gate refusal", err)
	}
}

// An ALLOWED word under the same policy still runs (via the interpreter
// fallback) — the gate refuses compilation, never execution.
func TestPolicyGatedRegistryAllowedWordStillRuns(t *testing.T) {
	// Legacy refusal+fallback-parity contract: pins the one-release
	// AQL_COMPILE_FALLBACK=1 hatch behavior (Stage J flipped the default
	// to compile_refused; migrate this contract or retire it with the hatch).
	t.Setenv("AQL_COMPILE_FALLBACK", "1")
	pol := denyAddPolicy(t)
	a, _ := New(Options{Policy: pol})
	got, ran, _, err := a.RunCompiledReason("10 sub 3")
	if err != nil || ran {
		t.Fatalf("got %v ran=%v err=%v; want the interpreter fallback result", got, ran, err)
	}
	if fmt.Sprint(got) != "[7]" {
		t.Errorf("got %v, want [7]", got)
	}
}

// A policy-FREE instance keeps the compiled path — the gate keys strictly
// on an installed word checker (the negative proving no blanket refusal).
func TestNoPolicyKeepsCompiledPath(t *testing.T) {
	a, _ := New()
	got, ran, _, err := a.RunCompiledReason("1 add 2")
	if err != nil || !ran || fmt.Sprint(got) != "[3]" {
		t.Errorf("got %v ran=%v err=%v; want a compiled [3]", got, ran, err)
	}
}

// NewFromRegistry construction contract: nil is rejected; a wired registry
// yields a working compiled-by-default instance (the REPL's construction).
func TestNewFromRegistry(t *testing.T) {
	if _, err := NewFromRegistry(nil); err == nil {
		t.Fatal("nil registry must be rejected")
	}
	base, _ := New()
	inst, err := NewFromRegistry(base.NativeRegistry())
	if err != nil {
		t.Fatal(err)
	}
	vals, ran, _, err := inst.RunAutoValues("1 add 2")
	if err != nil || !ran || len(vals) != 1 || vals[0].String() != "3" {
		t.Errorf("RunAutoValues over a wrapped registry: vals=%v ran=%v err=%v", vals, ran, err)
	}
}
