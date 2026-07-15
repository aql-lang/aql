package modules_test

import (
	"strings"
	"testing"

	lang "github.com/aql-lang/aql/lang/go"
	"github.com/aql-lang/aql/lang/go/modules"
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
	out, err := a.Run(`(import "aql:vm") "1 add 2" Vm.run`)
	if err != nil {
		t.Fatalf("Vm.run: %s", err)
	}
	if len(out) == 0 || out[len(out)-1] != int64(3) {
		t.Errorf("expected 3, got %v", out)
	}
}

func TestVMRunDefaultSandboxBlocksWrite(t *testing.T) {
	a := newAQL(t, nil)
	// Default Vm.run uses sandbox. Sandbox allows importing aql:io but
	// still denies the disk.write capability, so IO.write is blocked.
	out, err := a.Run(`(import "aql:vm") "import \"aql:io\" IO.write 'data' '/tmp/aql-test'" Vm.run`)
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
	out, err := a.Run(`(import "aql:vm") "5 mul 7" Vm.run-sandbox`)
	if err != nil {
		t.Fatalf("Vm.run-sandbox: %s", err)
	}
	if len(out) == 0 || out[len(out)-1] != int64(35) {
		t.Errorf("expected 35, got %v", out)
	}
}

func TestVMRunComputeWorksForArith(t *testing.T) {
	a := newAQL(t, nil)
	out, err := a.Run(`(import "aql:vm") "3 add 4" Vm.run-compute`)
	if err != nil {
		t.Fatalf("Vm.run-compute: %s", err)
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
	// push policy-map first, then code string. Then Vm.run-with
	// resolves to a FnDef and auto-invokes.
	out, err := a.Run(`
		(import "aql:vm")
		{ scopes: { engine: { words: { default: "allow", rules: [ { deny: ["add"] } ] } } } }
		"1 add 2"
		Vm.run-with
	`)
	if err == nil {
		t.Fatalf("expected Vm.run-with to refuse add, got %v", out)
	}
	if !strings.Contains(err.Error(), "denied") && !strings.Contains(err.Error(), "add") {
		t.Errorf("expected denial mentioning add: %v", err)
	}
}

func TestVMAttenuationParentDenyWinsOnGlobal(t *testing.T) {
	// Parent denies disk.write globally; child policy lifts the cap
	// (default-allow global). Composition enforces: parent's deny
	// always wins, regardless of how the child is structured.
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
					rules: [{ allow: ["import"], where: { module: ["aql:vm", "aql:io"] } }]
				}
			}
		}
	}`)
	if err != nil {
		t.Fatal(err)
	}
	a := newAQL(t, parentPol)
	// Sub-engine: tries to write a file. Child policy is fully
	// permissive (default-allow everything) but the parent's
	// global.disk.write deny still applies via the composed wrapper.
	_, err = a.Run(`
		(import "aql:vm")
		{ scopes: { global: { words: { default: "allow" } }, fileops: { words: { default: "allow" } } } }
		"import \"aql:io\" 'data' IO.write '/tmp/aql-attenuation-test'"
		Vm.run-with
	`)
	if err == nil {
		t.Fatal("expected parent's global.disk.write deny to apply in sub-engine")
	}
	if !strings.Contains(err.Error(), "disk.write") && !strings.Contains(err.Error(), "denied") {
		t.Errorf("expected disk.write denial bubbled from parent: %v", err)
	}
}

func TestVMAttenuationParentDenyRuleSurvives(t *testing.T) {
	// Parent has default-allow but a SPECIFIC deny rule for
	// reading /secret/*. Child has default-allow with no rules.
	// The composed policy must still deny /secret/* reads — this
	// is the case the earlier RequireSubset failed to catch
	// (PR #99 review).
	parentPol, err := policy.LoadInline(`{
		name: "parent-secret-deny"
		scopes: {
			global: { words: { default: "allow" } }
			modules: {
				words: {
					default: "deny"
					rules: [{ allow: ["import"], where: { module: ["aql:vm", "aql:io"] } }]
				}
			}
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
	a := newAQL(t, parentPol)
	// Child opens fileops with no rules — under the old subset
	// check this slipped through. With Compose, parent's deny rule
	// is consulted on every check and the read is refused.
	_, err = a.Run(`
		(import "aql:vm")
		{ scopes: { fileops: { words: { default: "allow" } } } }
		"import \"aql:io\" IO.read '/secret/credentials.txt'"
		Vm.run-with
	`)
	if err == nil {
		t.Fatal("expected parent's path-specific deny to survive composition")
	}
	if !strings.Contains(err.Error(), "denied") && !strings.Contains(err.Error(), "/secret") {
		t.Errorf("expected /secret deny bubbled from parent: %v", err)
	}
}

func TestVMRunIsolatedFromParent(t *testing.T) {
	a := newAQL(t, nil)
	// def x in vm sub-engine should not leak into parent.
	_, err := a.Run(`(import "aql:vm") "def vm-only 42" Vm.run-sandbox`)
	if err != nil {
		t.Fatalf("Vm.run-sandbox def: %s", err)
	}
	// Trying to reference vm-only from the parent must fail.
	_, err = a.Run(`vm-only`)
	if err == nil {
		t.Error("vm-only should not leak into parent engine")
	}
}

// ---- Vm.check ---------------------------------------------------------

func TestVMCheckCleanSource(t *testing.T) {
	a := newAQL(t, nil)
	mustScalar(t, a, `(import "aql:vm") (Vm.check "1 add 2").ok`, "true")
	mustScalar(t, a, `(import "aql:vm") (Vm.check "1 add 2").errors`, int64(0))
	mustScalar(t, a, `(import "aql:vm") size (Vm.check "1 add 2").diagnostics`, int64(0))
}

func TestVMCheckReportsUndefinedWord(t *testing.T) {
	a := newAQL(t, nil)
	// Positive: the report flags the error and counts it.
	mustScalar(t, a, `(import "aql:vm") (Vm.check "totally-undefined-word").ok`, "false")
	mustScalar(t, a, `(import "aql:vm") (Vm.check "totally-undefined-word").errors`, int64(1))
	// The diagnostic carries a stable code and severity.
	mustScalar(t, a, `(import "aql:vm") ((Vm.check "totally-undefined-word").diagnostics.0).code`, "undefined_word")
	mustScalar(t, a, `(import "aql:vm") ((Vm.check "totally-undefined-word").diagnostics.0).severity`, "error")
}

func TestVMCheckSeparatesWarningsFromErrors(t *testing.T) {
	a := newAQL(t, nil)
	// A non-fatal finding is a warning, not an error: ok stays true.
	mustScalar(t, a, `(import "aql:vm") (Vm.check "1 add 2  def unused 99").ok`, "true")
	mustScalar(t, a, `(import "aql:vm") (Vm.check "1 add 2  def unused 99").warnings`, int64(1))
	mustScalar(t, a, `(import "aql:vm") (Vm.check "1 add 2  def unused 99").errors`, int64(0))
}

// Negative contract: a SYNTAX error is reported as data, never raised — the
// whole point of a check word is that the caller inspects findings uniformly.
func TestVMCheckSyntaxErrorIsDataNotRaised(t *testing.T) {
	a := newAQL(t, nil)
	out, err := a.Run(`(import "aql:vm") Vm.check "1 add ((("`)
	if err != nil {
		t.Fatalf("Vm.check must not raise on malformed source, got: %v", err)
	}
	if len(out) == 0 {
		t.Fatal("expected a result map")
	}
	mustScalar(t, a, `(import "aql:vm") (Vm.check "1 add (((").ok`, "false")
	mustScalar(t, a, `(import "aql:vm") ((Vm.check "1 add (((").diagnostics.0).code`, "parse_error")
}

// ---- Vm.compile -------------------------------------------------------

func TestVMCompileCompilable(t *testing.T) {
	a := newAQL(t, nil)
	mustScalar(t, a, `(import "aql:vm") (Vm.compile "1 add 2").ok`, "true")
	mustScalar(t, a, `(import "aql:vm") (Vm.compile "1 add 2").reason`, "")
	// The dispatch-site census is reported for a compiled program.
	mustScalar(t, a, `(import "aql:vm") ((Vm.compile "1 add 2").sites).mono`, int64(1))
}

// Negative contract: an UNCOMPILABLE program is refusal-as-data — ok:false
// with the first offender named, and no Go error raised.
func TestVMCompileRefusesAsData(t *testing.T) {
	a := newAQL(t, nil)
	out, err := a.Run(`(import "aql:vm") Vm.compile "(size (for 5 [i]))"`)
	if err != nil {
		t.Fatalf("Vm.compile must not raise on an uncompilable program, got: %v", err)
	}
	if len(out) == 0 {
		t.Fatal("expected a result map")
	}
	mustScalar(t, a, `(import "aql:vm") (Vm.compile "(size (for 5 [i]))").ok`, "false")
	// reason is non-empty: it names why the program could not be lowered.
	reasonLen := runScalar(t, a, `(import "aql:vm") size (Vm.compile "(size (for 5 [i]))").reason`)
	if n, _ := reasonLen.(int64); n == 0 {
		t.Error("expected a non-empty refusal reason")
	}
}

func TestVMCompileSyntaxErrorIsDataNotRaised(t *testing.T) {
	a := newAQL(t, nil)
	out, err := a.Run(`(import "aql:vm") Vm.compile "1 add ((("`)
	if err != nil {
		t.Fatalf("Vm.compile must not raise on malformed source, got: %v", err)
	}
	if len(out) == 0 {
		t.Fatal("expected a result map")
	}
	mustScalar(t, a, `(import "aql:vm") (Vm.compile "1 add (((").ok`, "false")
	mustScalar(t, a, `(import "aql:vm") (Vm.compile "1 add (((").reason`, "parse error")
}

// ---- helpers ----------------------------------------------------------

func runScalar(t *testing.T, a *lang.AQL, src string) any {
	t.Helper()
	out, err := a.Run(src)
	if err != nil {
		t.Fatalf("run %q: %v", src, err)
	}
	if len(out) != 1 {
		t.Fatalf("run %q: expected one residual value, got %v", src, out)
	}
	return out[0]
}

func mustScalar(t *testing.T, a *lang.AQL, src string, want any) {
	t.Helper()
	if got := runScalar(t, a, src); got != want {
		t.Errorf("run %q: got %v (%T), want %v (%T)", src, got, got, want, want)
	}
}

// The nil-CompiledSubRun arm: a host embedding modules WITHOUT the lang
// package (whose init installs the seam) keeps the tree-walker — the
// sub-engine result is identical.
func TestVMRunWithoutCompiledSeamKeepsTreeWalker(t *testing.T) {
	saved := modules.CompiledSubRun
	modules.CompiledSubRun = nil
	defer func() { modules.CompiledSubRun = saved }()

	a := newAQL(t, nil)
	out, err := a.Run(`(import "aql:vm") "1 add 2" Vm.run`)
	if err != nil {
		t.Fatalf("Vm.run without the seam: %s", err)
	}
	if len(out) == 0 || out[len(out)-1] != int64(3) {
		t.Errorf("tree-walker sub-run = %v, want 3", out)
	}
}
