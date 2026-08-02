package lang

import (
	"fmt"
	"strings"
	"testing"

	"github.com/boru-lang/boru/lang/go/policy"
)

// Per-export module gating (NUR045). A profile's per-module subscope
// rules — `modules.scopes."boru:time-util".words: deny ["sleep"]` — are
// enforced at dispatch, on BOTH engines, through the identity stamped
// onto a module's signatures at resolution time. Before the fix the
// schema was dead: a sandboxed program slept exactly as long as an
// unsandboxed one, and every shipped profile's per-export rule was a
// false guarantee in the security layer.
func denySleepPolicy(t *testing.T) Policy {
	t.Helper()
	pol, err := policy.LoadInline(`{
		name: "no-sleep",
		scopes: {
			modules: {
				words: { default: "allow" },
				scopes: {
					"boru:time-util": {
						words: { default: "allow", rules: [{ deny: ["sleep"] }] }
					}
				}
			}
		}
	}`)
	if err != nil {
		t.Fatal(err)
	}
	return pol
}

// allowAllPolicy carries NO per-export rules, so it also exercises the
// hasPerExportRules short-circuit: CheckModuleCall answers allow without
// walking any scope.
func allowAllPolicy(t *testing.T) Policy {
	t.Helper()
	pol, err := policy.LoadInline(`{ name: "open", scopes: { engine: { words: { default: "allow" } } } }`)
	if err != nil {
		t.Fatal(err)
	}
	return pol
}

const sleepSrc = `import "boru:time-util"  TimeUtil.sleep 1`

func TestModuleCallGateParityCompiledVsInterpreted(t *testing.T) {
	pol := denySleepPolicy(t)

	a, _ := New(Options{Policy: pol})
	gotI, errI := a.RunInterp(sleepSrc)
	if errI == nil || !strings.Contains(errI.Error(), "permission denied") {
		t.Fatalf("interpreter must deny the per-export rule: got %v / %v", gotI, errI)
	}
	if !strings.Contains(errI.Error(), "modules.call") {
		t.Errorf("denial should blame modules.call, got %v", errI)
	}

	b, _ := New(Options{Policy: pol})
	gotC, ran, reason, errC := b.RunCompiledReason(sleepSrc)
	if !ran {
		t.Fatalf("a module-gated program must still COMPILE: ran=%v reason=%q err=%v", ran, reason, errC)
	}
	if fmt.Sprint(errC) != fmt.Sprint(errI) || fmt.Sprint(gotC) != fmt.Sprint(gotI) {
		t.Errorf("deny parity: compiled=%v/%v interp=%v/%v", gotC, errC, gotI, errI)
	}
}

// A denied export must not be laundered by parking the fn as data and
// calling it through a plain binding — the stamp rides every copied
// signature, so the rebound word gates too.
func TestModuleCallGateResistsFnValueLaundering(t *testing.T) {
	pol := denySleepPolicy(t)
	const src = `import "boru:time-util"  def s TimeUtil.sleep/r  s 1`

	a, _ := New(Options{Policy: pol})
	_, errI := a.RunInterp(src)
	if errI == nil || !strings.Contains(errI.Error(), "permission denied") {
		t.Fatalf("parked fn value must still gate (interp): %v", errI)
	}
	b, _ := New(Options{Policy: pol})
	_, ran, _, errC := b.RunCompiledReason(src)
	if errC == nil || !strings.Contains(errC.Error(), "permission denied") {
		t.Fatalf("parked fn value must still gate (compiled, ran=%v): %v", ran, errC)
	}
	if fmt.Sprint(errC) != fmt.Sprint(errI) {
		t.Errorf("laundering-path parity: compiled=%v interp=%v", errC, errI)
	}
}

// An export with no deny rule under the same profile keeps dispatching,
// on both engines — the gate is per export, not per module.
func TestModuleCallGateAllowsUnruledExport(t *testing.T) {
	pol := denySleepPolicy(t)
	const src = `import "boru:time-util"  TimeUtil.now`

	a, _ := New(Options{Policy: pol})
	if _, err := a.RunInterp(src); err != nil {
		t.Errorf("an unruled export must run (interp): %v", err)
	}
	b, _ := New(Options{Policy: pol})
	if _, ran, _, err := b.RunCompiledReason(src); err != nil || !ran {
		t.Errorf("an unruled export must run (compiled, ran=%v): %v", ran, err)
	}
}

// With no per-export rules anywhere, the precomputed short-circuit
// answers allow and a denied-elsewhere module word still runs.
func TestModuleCallGateShortCircuitsWithoutPerExportRules(t *testing.T) {
	pol := allowAllPolicy(t)
	a, _ := New(Options{Policy: pol})
	if _, err := a.RunInterp(sleepSrc); err != nil {
		t.Errorf("no per-export rules: sleep must run: %v", err)
	}
	b, _ := New(Options{Policy: pol})
	if _, ran, _, err := b.RunCompiledReason(sleepSrc); err != nil || !ran {
		t.Errorf("no per-export rules, compiled (ran=%v): %v", ran, err)
	}
}

// A module whose subscope denies nothing is unaffected, and the denial
// is addressed by the RESOLVED module id — a different module's
// same-named export does not gate.
func TestModuleCallGateIsAddressedByModuleID(t *testing.T) {
	pol := denySleepPolicy(t)
	const src = `import "boru:math-util"  MathUtil.sqrt 16.0`
	a, _ := New(Options{Policy: pol})
	got, err := a.RunInterp(src)
	if err != nil {
		t.Fatalf("another module must be unaffected: %v", err)
	}
	if fmt.Sprint(got) != "[4.0]" {
		t.Errorf("got %v, want [4.0]", got)
	}
}

// A module-preamble boru fn gates on its EXPORT key, not the
// module-private fn name its compiled unit carries — the compiled
// CALL_USER arm reads the stamped identity rather than reconstructing
// one. Before this, `Cli.usage` denied on the interpreter and RAN
// compiled: the exact bypass class NUR045 exists to close.
func denyCliUsagePolicy(t *testing.T) Policy {
	t.Helper()
	pol, err := policy.LoadInline(`{
		name: "no-usage",
		scopes: {
			modules: {
				words: { default: "allow" },
				scopes: { "boru:cli": { words: { default: "allow", rules: [{ deny: ["usage"] }] } } }
			}
		}
	}`)
	if err != nil {
		t.Fatal(err)
	}
	return pol
}

func TestModuleCallGateBoruPreambleFnBothEngines(t *testing.T) {
	pol := denyCliUsagePolicy(t)
	const src = `import "boru:cli"  Cli.usage {name:"x" flags:{}}`

	a, _ := New(Options{Policy: pol})
	_, errI := a.RunInterp(src)
	if errI == nil || !strings.Contains(errI.Error(), "permission denied") {
		t.Fatalf("interpreter must deny a boru-preamble export: %v", errI)
	}
	b, _ := New(Options{Policy: pol})
	_, ran, _, errC := b.RunCompiledReason(src)
	if errC == nil || !strings.Contains(errC.Error(), "permission denied") {
		t.Fatalf("compiled must deny it too (ran=%v): %v", ran, errC)
	}
	if fmt.Sprint(errC) != fmt.Sprint(errI) {
		t.Errorf("boru-fn deny parity: compiled=%v interp=%v", errC, errI)
	}
}

func TestModuleCallGateBoruPreambleUnruledExportRuns(t *testing.T) {
	pol := denyCliUsagePolicy(t)
	const src = `import "boru:cli"  Cli.parse {name:"x" flags:{}} ["x"]`
	a, _ := New(Options{Policy: pol})
	if _, err := a.RunInterp(src); err != nil {
		t.Errorf("an unruled boru-fn export must run (interp): %v", err)
	}
	b, _ := New(Options{Policy: pol})
	if _, ran, _, err := b.RunCompiledReason(src); err != nil || !ran {
		t.Errorf("an unruled boru-fn export must run (compiled, ran=%v): %v", ran, err)
	}
}
