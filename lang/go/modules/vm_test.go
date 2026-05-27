package modules_test

import (
	"strings"
	"testing"

	lang "github.com/aql-lang/aql/lang/go"
	"github.com/aql-lang/aql/lang/go/policy"
)

func newAQL(t *testing.T, pol policy.Policy) *lang.AQL {
	t.Helper()
	a, err := lang.New(lang.Options{Policy: pol})
	if err != nil {
		t.Fatal(err)
	}
	return a
}

func TestVMRunReturnsLastValue(t *testing.T) {
	a := newAQL(t, nil)
	out, err := a.Run(`("aql:vm" import) "1 add 2" vm.run`)
	if err != nil {
		t.Fatalf("vm.run: %s", err)
	}
	if len(out) == 0 || out[len(out)-1] != int64(3) {
		t.Errorf("expected 3, got %v", out)
	}
}

func TestVMRunDefaultSandboxBlocksWrite(t *testing.T) {
	a := newAQL(t, nil)
	// Default vm.run uses sandbox. Sandbox denies fileops.write.
	out, err := a.Run(`("aql:vm" import) "write 'data' '/tmp/aql-test'" vm.run`)
	if err == nil {
		t.Errorf("expected sandbox denial, got %v", out)
	}
	if !strings.Contains(err.Error(), "permission denied") &&
		!strings.Contains(err.Error(), "disk.write") &&
		!strings.Contains(err.Error(), "denied") {
		t.Errorf("expected permission-denied error, got: %v", err)
	}
}

func TestVMRunSandboxAllowsCompute(t *testing.T) {
	a := newAQL(t, nil)
	out, err := a.Run(`("aql:vm" import) "5 mul 7" vm.run-sandbox`)
	if err != nil {
		t.Fatalf("vm.run-sandbox: %s", err)
	}
	if len(out) == 0 || out[len(out)-1] != int64(35) {
		t.Errorf("expected 35, got %v", out)
	}
}

func TestVMRunComputeWorksForArith(t *testing.T) {
	a := newAQL(t, nil)
	out, err := a.Run(`("aql:vm" import) "3 add 4" vm.run-compute`)
	if err != nil {
		t.Fatalf("vm.run-compute: %s", err)
	}
	if len(out) == 0 || out[len(out)-1] != int64(7) {
		t.Errorf("expected 7, got %v", out)
	}
}

func TestVMRunWithExplicitPolicy(t *testing.T) {
	a := newAQL(t, nil)
	// Inline jsonic policy via a map literal: deny `add`, allow
	// everything else. Sub-engine should refuse 1 add 2.
	// Stack order for binary dispatch (top=args[0], deeper=args[1]):
	// push policy-map first, then code string. Then vm.run-with
	// resolves to a FnDef and auto-invokes.
	out, err := a.Run(`
		("aql:vm" import)
		{ scopes: { engine: { words: { default: "allow", rules: [ { deny: ["add"] } ] } } } }
		"1 add 2"
		vm.run-with
	`)
	if err == nil {
		t.Fatalf("expected vm.run-with to refuse add, got %v", out)
	}
	if !strings.Contains(err.Error(), "denied") && !strings.Contains(err.Error(), "add") {
		t.Errorf("expected denial mentioning add: %v", err)
	}
}

func TestVMAttenuationRejectsBroaderChild(t *testing.T) {
	// Parent denies disk.write globally; child policy lifts the cap
	// (default-allow global). Attenuation must reject the child.
	parentPol, err := policy.LoadInline(`{
		name: "parent-deny-write"
		scopes: {
			global: {
				words: {
					default: "allow"
					rules: [{ deny: ["disk.write"] }]
				}
			}
			modules: {
				words: {
					default: "deny"
					rules: [{ allow: ["import"], where: { module: ["aql:vm"] } }]
				}
			}
		}
	}`)
	if err != nil {
		t.Fatal(err)
	}
	a := newAQL(t, parentPol)
	_, err = a.Run(`
		("aql:vm" import)
		{ scopes: { global: { words: { default: "allow" } } } }
		"1 add 2"
		vm.run-with
	`)
	if err == nil {
		t.Fatal("expected attenuation error")
	}
	if !strings.Contains(err.Error(), "attenuation") && !strings.Contains(err.Error(), "exceeds parent") {
		t.Errorf("expected attenuation/exceeds parent error: %v", err)
	}
}

func TestVMRunIsolatedFromParent(t *testing.T) {
	a := newAQL(t, nil)
	// def x in vm sub-engine should not leak into parent.
	_, err := a.Run(`("aql:vm" import) "def vm-only 42" vm.run-sandbox`)
	if err != nil {
		t.Fatalf("vm.run-sandbox def: %s", err)
	}
	// Trying to reference vm-only from the parent must fail.
	_, err = a.Run(`vm-only`)
	if err == nil {
		t.Error("vm-only should not leak into parent engine")
	}
}
