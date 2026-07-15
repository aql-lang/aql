package lang

import (
	"bytes"
	"fmt"
	"strings"
	"testing"

	"github.com/aql-lang/aql/lang/go/policy"
)

// The compiled-path policy contract (the 2026-07-15 LIFT, user-authorized):
// policy-gated registries COMPILE, and every named VM dispatch consults the
// SAME WordChecker the interpreter's policyGateWord runs (vmContext.gateWord)
// — so a denied word raises the identical permission error on either engine,
// closing the 2026-07-13 bypass ("deny add" interpreted to permission-denied
// but ran compiled to 3) at the dispatch layer. The pre-lift refusal pins
// are replaced by this compiled-vs-interpreted parity sweep.
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

func TestPolicyParityCompiledVsInterpreted(t *testing.T) {
	pol := denyAddPolicy(t)

	// DENIED word: the compiled run executes on the VM and raises the SAME
	// permission error the interpreter raises.
	a, _ := New(Options{Policy: pol})
	gotI, errI := a.RunInterp("1 add 2")
	if errI == nil || !strings.Contains(errI.Error(), "permission denied") {
		t.Fatalf("interpreter must deny: got %v / %v", gotI, errI)
	}
	b, _ := New(Options{Policy: pol})
	gotC, ran, reason, errC := b.RunCompiledReason("1 add 2")
	if !ran {
		t.Fatalf("a policy-gated registry must now COMPILE (the lift): ran=%v reason=%q err=%v", ran, reason, errC)
	}
	if fmt.Sprint(errC) != fmt.Sprint(errI) || fmt.Sprint(gotC) != fmt.Sprint(gotI) {
		t.Errorf("deny parity: compiled=%v/%v interp=%v/%v", gotC, errC, gotI, errI)
	}

	// A DENIED user fn gates identically (the CALL_USER arm).
	const userSrc = `def zzf fn [[] [Integer] [5]] zzf`
	polF, err := policy.LoadInline(`{
		name: "no-zzf",
		scopes: { engine: { words: { default: "allow", rules: [{ deny: ["zzf"] }] } } }
	}`)
	if err != nil {
		t.Fatal(err)
	}
	c, _ := New(Options{Policy: polF})
	_, errUI := c.RunInterp(userSrc)
	d, _ := New(Options{Policy: polF})
	_, ranU, _, errUC := d.RunCompiledReason(userSrc)
	if !ranU || fmt.Sprint(errUC) != fmt.Sprint(errUI) || errUC == nil {
		t.Errorf("user-fn deny parity: ran=%v compiled=%v interp=%v", ranU, errUC, errUI)
	}

	// ALLOWED word under the same deny-add policy: compiled result parity.
	e, _ := New(Options{Policy: pol})
	got, ranA, _, errA := e.RunCompiledReason("10 sub 3")
	if errA != nil || !ranA || fmt.Sprint(got) != "[7]" {
		t.Errorf("allowed word must run compiled: got %v ran=%v err=%v", got, ranA, errA)
	}

	// An effect BEFORE the denial must not diverge: PolicyDenied is the
	// program's VERDICT, never a bail — no re-run (the output would
	// double) and no fence-blocked internal_error (the pre-wrapper
	// behaviour: a plain checker error looked foreign, bailed, and the
	// escaped print fence-blocked into a DIVERGENT error).
	g, _ := New(Options{Policy: pol})
	var outC bytes.Buffer
	g.SetOutput(&outC)
	_, ranE, _, errE := g.RunCompiledReason(`print "once" ; 1 add 2`)
	h, _ := New(Options{Policy: pol})
	var outI bytes.Buffer
	h.SetOutput(&outI)
	_, errEI := h.RunInterp(`print "once" ; 1 add 2`)
	if !ranE || fmt.Sprint(errE) != fmt.Sprint(errEI) {
		t.Errorf("effect-before-deny parity: ran=%v compiled=%v interp=%v", ranE, errE, errEI)
	}
	if outC.String() != outI.String() {
		t.Errorf("effect-before-deny output: compiled %q interp %q", outC.String(), outI.String())
	}

	// Strict mode: the program compiles and the deny raises at dispatch —
	// no refusal anywhere.
	f, _ := New(Options{Policy: pol})
	if _, err := f.RunCompiledStrict("1 add 2"); err == nil ||
		!strings.Contains(err.Error(), "permission denied") {
		t.Errorf("RunCompiledStrict = %v, want the runtime permission denial", err)
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
