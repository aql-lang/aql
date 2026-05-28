package policy

import (
	"strings"
	"testing"
)

func TestComposeNilParentReturnsChild(t *testing.T) {
	child := mustLoad(t, "sandbox")
	got := Compose(nil, child)
	if got != child {
		t.Errorf("Compose(nil, child) should return child unchanged")
	}
}

func TestComposeNilChildReturnsParent(t *testing.T) {
	parent := mustLoad(t, "sandbox")
	got := Compose(parent, nil)
	if got != parent {
		t.Errorf("Compose(parent, nil) should return parent unchanged")
	}
}

func TestComposeParentDenyOverridesChildAllow(t *testing.T) {
	// Parent: default-allow globals but deny disk.write.
	// Child:  default-allow globals (no rules).
	// Result: disk.write denied — parent's deny wins.
	parent, err := LoadInline(`{
		name: "parent"
		scopes: {
			global: { words: { default: "allow", rules: [{ deny: ["disk.write"] }] } }
		}
	}`)
	if err != nil {
		t.Fatal(err)
	}
	child, err := LoadInline(`{
		name: "child"
		scopes: {
			global: { words: { default: "allow" } }
		}
	}`)
	if err != nil {
		t.Fatal(err)
	}
	c := Compose(parent, child)
	if err := c.CheckGlobal("disk.write"); err == nil {
		t.Error("parent's disk.write deny must apply through composed policy")
	}
	if err := c.CheckGlobal("disk.read"); err != nil {
		t.Errorf("disk.read should still pass: %v", err)
	}
}

func TestComposeParentRuleDenyOverridesChild(t *testing.T) {
	// The case that the old RequireSubset missed: parent default-allow
	// with a path-specific deny rule, child default-allow with no rules.
	parent, err := LoadInline(`{
		name: "parent"
		scopes: {
			fileops: {
				words: {
					default: "allow"
					rules: [{ deny: ["read"], where: { path: ["/secret/**"] } }]
				}
			}
		}
	}`)
	if err != nil {
		t.Fatal(err)
	}
	child, err := LoadInline(`{
		name: "child"
		scopes: {
			fileops: { words: { default: "allow" } }
		}
	}`)
	if err != nil {
		t.Fatal(err)
	}
	c := Compose(parent, child)
	// /secret/foo: parent denies → composed must deny.
	if err := c.Check("fileops", "read", Args{"path": "/secret/foo"}); err == nil {
		t.Error("parent rule-level deny must survive composition")
	}
	// /tmp/foo: parent allows, child allows → composed allows.
	if err := c.Check("fileops", "read", Args{"path": "/tmp/foo"}); err != nil {
		t.Errorf("/tmp/foo should pass both layers: %v", err)
	}
}

func TestComposeChildDenyAlsoApplies(t *testing.T) {
	// Symmetry: child can further restrict beyond parent.
	parent := mustLoad(t, "trusted")
	child, err := LoadInline(`{
		name: "tight"
		scopes: {
			global: { words: { default: "allow", rules: [{ deny: ["network"] }] } }
		}
	}`)
	if err != nil {
		t.Fatal(err)
	}
	c := Compose(parent, child)
	if err := c.CheckGlobal("network"); err == nil {
		t.Error("child's network deny must apply through composed policy")
	}
}

func TestComposeInstalledRequiresBoth(t *testing.T) {
	// Parent has sqlite uninstalled; child reinstates → composed
	// must still report sqlite as uninstalled.
	parent, err := LoadInline(`{
		name: "parent"
		scopes: { sqlite: { install: false } }
	}`)
	if err != nil {
		t.Fatal(err)
	}
	child, err := LoadInline(`{
		name: "child"
		scopes: { sqlite: { install: true, words: { default: "allow" } } }
	}`)
	if err != nil {
		t.Fatal(err)
	}
	c := Compose(parent, child)
	if c.Installed("sqlite") {
		t.Error("parent's install=false must override child's install=true")
	}
}

func TestComposeNameMentionsBothLayers(t *testing.T) {
	parent := mustLoad(t, "sandbox")
	child := mustLoad(t, "compute")
	c := Compose(parent, child)
	name := c.Name()
	if !strings.Contains(name, "compute") || !strings.Contains(name, "sandbox") {
		t.Errorf("composed Name should mention both layers, got %q", name)
	}
}

func TestComposeLimitsTakesMostRestrictive(t *testing.T) {
	parent, err := LoadInline(`{ name: "p", limits: { timeoutMs: 5000, maxStepBudget: 100 } }`)
	if err != nil {
		t.Fatal(err)
	}
	child, err := LoadInline(`{ name: "c", limits: { timeoutMs: 1000, maxStepBudget: 200 } }`)
	if err != nil {
		t.Fatal(err)
	}
	c := Compose(parent, child)
	lim := c.Limits()
	if lim.TimeoutMs != 1000 {
		t.Errorf("TimeoutMs = %d, want 1000 (smaller)", lim.TimeoutMs)
	}
	if lim.MaxStepBudget != 100 {
		t.Errorf("MaxStepBudget = %d, want 100 (smaller)", lim.MaxStepBudget)
	}
}
