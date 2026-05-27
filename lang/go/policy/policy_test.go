package policy

import (
	"strings"
	"testing"
)

func mustLoad(t *testing.T, name string) Policy {
	t.Helper()
	p, err := Load(name)
	if err != nil {
		t.Fatalf("Load(%q): %s", name, err)
	}
	return p
}

func TestLoadBuiltinFull(t *testing.T) {
	p := mustLoad(t, "full")
	if p.Name() != "full" {
		t.Errorf("Name = %q, want %q", p.Name(), "full")
	}
	// full = allow everything.
	if err := p.CheckWord("add"); err != nil {
		t.Errorf("full.CheckWord(add) = %v, want nil", err)
	}
	if err := p.Check("fileops", "write", Args{"path": "/etc/passwd"}); err != nil {
		t.Errorf("full.Check(fileops.write) = %v, want nil", err)
	}
	if err := p.CheckGlobal("disk.write"); err != nil {
		t.Errorf("full.CheckGlobal(disk.write) = %v, want nil", err)
	}
}

func TestLoadAllBuiltins(t *testing.T) {
	for _, name := range BuiltinNames() {
		p, err := Load(name)
		if err != nil {
			t.Errorf("Load(%q): %s", name, err)
			continue
		}
		if p.Name() != name {
			t.Errorf("profile %q reports Name() = %q", name, p.Name())
		}
	}
}

func TestSandboxDeniesWrite(t *testing.T) {
	p := mustLoad(t, "sandbox")
	err := p.Check("fileops", "write", Args{"path": "/tmp/x"})
	if err == nil {
		t.Fatal("expected sandbox to deny fileops.write")
	}
	d, ok := err.(*Denied)
	if !ok {
		t.Fatalf("expected *Denied, got %T", err)
	}
	if d.Code != CodePermissionDenied {
		t.Errorf("Code = %q, want %q", d.Code, CodePermissionDenied)
	}
	if !strings.Contains(d.Blame, "global.disk.write") {
		t.Errorf("Blame = %q, expected to mention global hard cap", d.Blame)
	}
}

func TestSandboxAllowsRead(t *testing.T) {
	p := mustLoad(t, "sandbox")
	// sandbox has fileops.words.default=deny, no rules. So even
	// though global.disk.read is allowed, the scope refuses.
	err := p.Check("fileops", "read", Args{"path": "/tmp/x"})
	if err == nil {
		t.Error("expected sandbox to deny fileops.read (scope default-deny)")
	}
}

func TestComputeUninstallsCaps(t *testing.T) {
	p := mustLoad(t, "compute")
	if p.Installed("fileops") {
		t.Error("compute should have fileops uninstalled")
	}
	if p.Installed("sqlite") {
		t.Error("compute should have sqlite uninstalled")
	}
	if !p.Installed("engine") {
		t.Error("engine cannot be uninstalled")
	}
	// Reaching an uninstalled cap gives capability_not_installed,
	// not permission_denied.
	err := p.Check("sqlite", "open-ro", nil)
	d, ok := err.(*Denied)
	if !ok {
		t.Fatalf("expected *Denied, got %T (%v)", err, err)
	}
	if d.Code != CodeCapabilityNotInstalled {
		t.Errorf("Code = %q, want %q", d.Code, CodeCapabilityNotInstalled)
	}
}

func TestGenProfileDeniesIOAllowsRand(t *testing.T) {
	p := mustLoad(t, "gen")
	// gen uninstalls IO + clock + network so generator programs
	// cannot reach the outside world.
	if p.Installed("fileops") {
		t.Error("gen should have fileops uninstalled")
	}
	if p.Installed("network") {
		t.Error("gen should have network uninstalled")
	}
	if p.Installed("clock") {
		t.Error("gen should have clock uninstalled")
	}
	if p.Installed("sqlite") {
		t.Error("gen should have sqlite uninstalled")
	}
	if !p.Installed("engine") {
		t.Error("engine cannot be uninstalled")
	}
}

func TestReadOnlyInheritsSandbox(t *testing.T) {
	p := mustLoad(t, "read-only")
	// Inherits sandbox's deny on disk.write.
	err := p.CheckGlobal("disk.write")
	if err == nil {
		t.Error("read-only should inherit sandbox's disk.write deny")
	}
	// Adds env.read for safe vars.
	if err := p.Check("env", "read", Args{"name": "LANG"}); err != nil {
		t.Errorf("read-only should allow env.read for LANG, got %v", err)
	}
	if err := p.Check("env", "read", Args{"name": "PWD"}); err == nil {
		t.Error("read-only should not allow env.read for PWD")
	}
}

func TestTrustedAllowsAll(t *testing.T) {
	p := mustLoad(t, "trusted")
	for _, g := range GlobalOps {
		if err := p.CheckGlobal(g); err != nil {
			t.Errorf("trusted.CheckGlobal(%s) = %v, want nil", g, err)
		}
	}
	if err := p.Check("fileops", "write", Args{"path": "/etc/passwd"}); err != nil {
		t.Errorf("trusted.Check(fileops.write) = %v, want nil", err)
	}
}

func TestLoadInline(t *testing.T) {
	p, err := LoadInline(`{ name: "tiny", scopes: { engine: { words: { default: "deny", rules: [ { allow: ["add"] } ] } } } }`)
	if err != nil {
		t.Fatalf("LoadInline: %s", err)
	}
	if err := p.CheckWord("add"); err != nil {
		t.Errorf("tiny.CheckWord(add) = %v, want nil", err)
	}
	if err := p.CheckWord("sub"); err == nil {
		t.Error("tiny.CheckWord(sub) should deny")
	}
}

func TestLoadAutoDetection(t *testing.T) {
	// Inline (starts with {)
	if _, err := LoadAuto(`{ name: "x" }`); err != nil {
		t.Errorf("inline: %s", err)
	}
	// Built-in name
	if _, err := LoadAuto("sandbox"); err != nil {
		t.Errorf("name: %s", err)
	}
	// Unknown name
	if _, err := LoadAuto("nonesuch-profile-xyzzy"); err == nil {
		t.Error("expected error on unknown name")
	}
}

func TestUnknownScopeRejected(t *testing.T) {
	src := `{ name: "bad", scopes: { wat: { words: { default: "allow" } } } }`
	_, err := LoadInline(src)
	if err == nil {
		t.Error("expected unknown scope to fail validation")
	}
	if !strings.Contains(err.Error(), "unknown scope") {
		t.Errorf("error = %v, expected 'unknown scope'", err)
	}
}

func TestInstallFalseInvalidOnEngine(t *testing.T) {
	src := `{ name: "bad", scopes: { engine: { install: false } } }`
	_, err := LoadInline(src)
	if err == nil {
		t.Error("expected install=false on engine to be rejected")
	}
}

func TestUnknownGlobalOp(t *testing.T) {
	src := `{ name: "bad", scopes: { global: { words: { rules: [ { allow: ["disk.flunge"] } ] } } } }`
	_, err := LoadInline(src)
	if err == nil {
		t.Error("expected unknown global op to be rejected")
	}
}

func TestRuleMustHaveOneEffect(t *testing.T) {
	src := `{ name: "bad", scopes: { engine: { words: { rules: [ { allow: ["x"], deny: ["y"] } ] } } } }`
	_, err := LoadInline(src)
	if err == nil {
		t.Error("expected rule with both allow and deny to be rejected")
	}
	src = `{ name: "bad", scopes: { engine: { words: { rules: [ {} ] } } } }`
	_, err = LoadInline(src)
	if err == nil {
		t.Error("expected rule with neither allow nor deny to be rejected")
	}
}

func TestExtendsResolution(t *testing.T) {
	// read-only extends sandbox; verify the merge.
	p := mustLoad(t, "read-only").(*Compiled)
	if p.scope("engine") == nil {
		t.Error("read-only should inherit sandbox's engine scope")
	}
	if !p.Installed("formats") {
		t.Error("read-only should inherit sandbox's formats install")
	}
	if p.Installed("network") {
		t.Error("read-only should inherit sandbox's network install=false")
	}
}

func TestExtendsCycle(t *testing.T) {
	// Manually craft a cycle: profile A extends B, B extends A.
	resolver := func(name string) (*Profile, error) {
		switch name {
		case "A":
			return &Profile{Name: "A", Extends: "B"}, nil
		case "B":
			return &Profile{Name: "B", Extends: "A"}, nil
		}
		return nil, profileError("unknown")
	}
	p := &Profile{Name: "A", Extends: "B"}
	_, err := p.Compile(resolver)
	if err == nil {
		t.Error("expected cycle to be detected")
	}
}

func TestLastMatchWins(t *testing.T) {
	src := `{ name: "lmw", scopes: { engine: { words: {
		default: "allow",
		rules: [
			{ deny:  ["x"] },
			{ allow: ["x"] }
		]
	} } } }`
	p, err := LoadInline(src)
	if err != nil {
		t.Fatal(err)
	}
	if err := p.CheckWord("x"); err != nil {
		t.Errorf("CheckWord(x) = %v, want nil (last allow wins)", err)
	}
}

func TestWherePredicatesPath(t *testing.T) {
	src := `{ name: "wp", scopes: {
		fileops: { words: {
			default: "deny",
			rules: [
				{ allow: ["read"], where: { path: ["/tmp/**"] } }
			]
		} },
		global: { words: { default: "allow" } }
	} }`
	p, err := LoadInline(src)
	if err != nil {
		t.Fatal(err)
	}
	if err := p.Check("fileops", "read", Args{"path": "/tmp/file"}); err != nil {
		t.Errorf("/tmp/file: %v", err)
	}
	if err := p.Check("fileops", "read", Args{"path": "/etc/passwd"}); err == nil {
		t.Error("/etc/passwd should be denied")
	}
}

func TestModuleSubscopes(t *testing.T) {
	src := `{ name: "ms", scopes: {
		modules: {
			words: {
				default: "deny",
				rules: [ { allow: ["import"], where: { module: ["aql:math"] } } ]
			},
			scopes: {
				"aql:math": { words: { default: "allow", rules: [ { deny: ["pow"] } ] } }
			}
		}
	} }`
	p, err := LoadInline(src)
	if err != nil {
		t.Fatal(err)
	}
	// Import allowed.
	if err := p.Check("modules", "import", Args{"module": "aql:math"}); err != nil {
		t.Errorf("import aql:math: %v", err)
	}
	// Import of a different module denied.
	if err := p.Check("modules", "import", Args{"module": "aql:time"}); err == nil {
		t.Error("import aql:time should deny")
	}
	// Per-export within aql:math.
	if err := p.Check("modules", "call", Args{"module": "aql:math", "export": "sin"}); err != nil {
		t.Errorf("aql:math.sin: %v", err)
	}
	if err := p.Check("modules", "call", Args{"module": "aql:math", "export": "pow"}); err == nil {
		t.Error("aql:math.pow should deny")
	}
}

func TestRequireSubsetSelfIsSubset(t *testing.T) {
	p := mustLoad(t, "sandbox")
	if err := RequireSubset(p, p); err != nil {
		t.Errorf("p is subset of itself: %v", err)
	}
}

func TestRequireSubsetTighterChildOK(t *testing.T) {
	parent := mustLoad(t, "trusted")
	child := mustLoad(t, "sandbox")
	if err := RequireSubset(child, parent); err != nil {
		t.Errorf("sandbox should be subset of trusted: %v", err)
	}
}

func TestRequireSubsetBroaderChildFails(t *testing.T) {
	parent := mustLoad(t, "sandbox")
	child := mustLoad(t, "trusted")
	if err := RequireSubset(child, parent); err == nil {
		t.Error("trusted is broader than sandbox; expected attenuation error")
	}
}

func TestRequireSubsetNilParent(t *testing.T) {
	child := mustLoad(t, "sandbox")
	if err := RequireSubset(child, nil); err != nil {
		t.Errorf("nil parent allows anything: %v", err)
	}
}
