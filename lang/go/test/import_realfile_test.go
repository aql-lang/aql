package test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/aql-lang/aql/eng/go/parser"
	"github.com/aql-lang/aql/lang/go/capabilities"
	"github.com/aql-lang/aql/lang/go/modules"
	"github.com/aql-lang/aql/lang/go/native"
)

// testdataDir returns the absolute path to the testdata directory.
func testdataDir(t *testing.T) string {
	t.Helper()
	abs, err := filepath.Abs("testdata")
	if err != nil {
		t.Fatal(err)
	}
	return abs
}

// runRealFileSteps creates a registry backed by real OS file operations,
// with the working directory set to dir, then executes AQL steps.
func runRealFileSteps(t *testing.T, dir string, steps []string) ([]native.Value, error) {
	t.Helper()

	origDir, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(dir); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.Chdir(origDir) })

	absDir, err := filepath.Abs(".")
	if err != nil {
		t.Fatal(err)
	}

	reg, err := native.DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	native.Register(reg)
	native.SetHostFileOps(reg, capabilities.NewDefault())
	reg.SetParseFunc(parser.Parse)
	modules.InstallResolver(reg) // production module wiring (lang.New)
	reg.BaseDir = absDir

	eng := native.New(reg)
	var result []native.Value
	for _, step := range steps {
		vals, err := parser.Parse(step)
		if err != nil {
			return nil, err
		}
		result, err = eng.Run(vals)
		if err != nil {
			return nil, err
		}
	}
	return result, nil
}

// §4.3 — `aql check` on a file that imports a sibling and uses its
// exports via dot-access must NOT emit spurious undefined_word /
// no_signature diagnostics for the imported namespace. In check mode the
// import path literal is preserved (StripToCarriers) and the target's
// exports are loaded so `Pkg.word` resolves.
func TestCheckSiblingImportResolvesNamespace(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "bloom.aql", `export "Bloom" {
  make: ([n:Integer] => [{n: n}])
  add:  ([b:Map s:String] => [b])
}`)
	writeFile(t, dir, "index.aql", `import "./bloom.aql" end
def bf (100 Bloom.make)
def bf2 (bf "hello" Bloom.add)
bf2`)

	diags := checkRealFile(t, dir, "index.aql")
	for _, d := range diags {
		if d.Code == "undefined_word" && strings.Contains(d.Detail, "Bloom") {
			t.Errorf("spurious undefined_word for imported namespace: %+v", d)
		}
		if d.Code == "no_signature" {
			t.Errorf("spurious no_signature from cross-module dot-access: %+v", d)
		}
	}
}

// §4.3 — checking a file that imports a MISSING sibling must degrade to
// an opaque module rather than hard-failing the whole check.
func TestCheckMissingSiblingImportDoesNotHardFail(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "index.aql", `import "./does-not-exist.aql" end
def x 5
x add 3`)
	// checkRealFile fails the test on a hard run error; reaching the
	// assertions means the missing import did not abort the check.
	diags := checkRealFile(t, dir, "index.aql")
	for _, d := range diags {
		if strings.Contains(d.Detail, "not found") {
			t.Errorf("missing sibling import should not surface a not-found error: %+v", d)
		}
	}
}

// writeFile writes name with the given contents under dir.
func writeFile(t *testing.T, dir, name, contents string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, name), []byte(contents), 0o644); err != nil {
		t.Fatal(err)
	}
}

// checkRealFile runs `name` (relative to dir) under check mode with real
// file ops + production module wiring, returning the diagnostics.
func checkRealFile(t *testing.T, dir, name string) []native.CheckDiagnostic {
	t.Helper()
	src, err := os.ReadFile(filepath.Join(dir, name))
	if err != nil {
		t.Fatal(err)
	}
	reg, err := native.DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	native.SetHostFileOps(reg, capabilities.NewDefault())
	reg.SetParseFunc(parser.Parse)
	modules.InstallResolver(reg)
	reg.BaseDir = dir
	reg.Check.Mode = true
	defer func() { reg.Check.Mode = false }()

	toks, err := parser.Parse(string(src))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if _, err := native.NewTop(reg).Run(toks); err != nil {
		t.Fatalf("check run hard-failed (should degrade gracefully): %v", err)
	}
	return reg.Check.Diagnostics
}

// --- Bare module with JSON import using relative paths ---

func TestRealFileBareModuleImportsJsonRelative(t *testing.T) {
	// testdata/.aql/planets/index.aql does: import "./data.json" def data end
	// The "./data.json" must resolve relative to index.aql's directory,
	// NOT relative to the process CWD.
	dir := testdataDir(t)
	result, err := runRealFileSteps(t, dir, []string{
		`import "planets"`,
		`Planets get catalog get earth get diameter`,
	})
	if err != nil {
		t.Fatal(err)
	}
	assertResult(t, result, "12756")
}

func TestRealFileBareModuleJsonMarsField(t *testing.T) {
	dir := testdataDir(t)
	result, err := runRealFileSteps(t, dir, []string{
		`import "planets"`,
		`Planets get catalog get mars get diameter`,
	})
	if err != nil {
		t.Fatal(err)
	}
	assertResult(t, result, "6792")
}

// --- Bare module with relative AQL import ---

func TestRealFileBareModuleImportsAqlRelative(t *testing.T) {
	// testdata/.aql/utils/index.aql does: import "./helpers.aql"
	// helpers.aql must resolve relative to the utils/ directory.
	dir := testdataDir(t)
	result, err := runRealFileSteps(t, dir, []string{
		`import "utils"`,
		`Utils.magicnum`,
	})
	if err != nil {
		t.Fatal(err)
	}
	assertResult(t, result, "42")
}

// --- File path import with relative path ---

func TestRealFileRelativePathImport(t *testing.T) {
	// Direct file path import: "./lib.aql" from the testdata directory.
	dir := testdataDir(t)
	result, err := runRealFileSteps(t, dir, []string{
		`import "./lib.aql"`,
		`Lib.version`,
	})
	if err != nil {
		t.Fatal(err)
	}
	assertResult(t, result, "1")
}

// --- Parent directory walk: CWD is a subdirectory ---

func TestRealFileBareModuleFromSubdir(t *testing.T) {
	// CWD = testdata/sub/deep, but "planets" is at testdata/.aql/planets/.
	// The walk should go: sub/deep -> sub -> testdata (found).
	dir := filepath.Join(testdataDir(t), "sub", "deep")
	result, err := runRealFileSteps(t, dir, []string{
		`import "planets"`,
		`Planets get catalog get earth get diameter`,
	})
	if err != nil {
		t.Fatal(err)
	}
	assertResult(t, result, "12756")
}

func TestRealFileBareModuleChildShadows(t *testing.T) {
	// CWD = testdata/sub. "local" is at testdata/sub/.aql/local/.
	// It should be found at the child level, not walk further.
	dir := filepath.Join(testdataDir(t), "sub")
	result, err := runRealFileSteps(t, dir, []string{
		`import "local"`,
		`Local.scope`,
	})
	if err != nil {
		t.Fatal(err)
	}
	assertResult(t, result, "'child'")
}

func TestRealFileBareModuleFromDeepSubdirFindsChild(t *testing.T) {
	// CWD = testdata/sub/deep. "local" is at testdata/sub/.aql/local/.
	// Walk: sub/deep -> sub (found).
	dir := filepath.Join(testdataDir(t), "sub", "deep")
	result, err := runRealFileSteps(t, dir, []string{
		`import "local"`,
		`Local.scope`,
	})
	if err != nil {
		t.Fatal(err)
	}
	assertResult(t, result, "'child'")
}

// --- Mixed: bare module + relative file import from same CWD ---

func TestRealFileMixedBareAndRelative(t *testing.T) {
	dir := testdataDir(t)
	result, err := runRealFileSteps(t, dir, []string{
		`import "planets"`,
		`import "./lib.aql"`,
		`Lib.version`,
	})
	if err != nil {
		t.Fatal(err)
	}
	assertResult(t, result, "1")
}

// --- Error: bare module not found ---

func TestRealFileBareModuleNotFound(t *testing.T) {
	dir := testdataDir(t)
	_, err := runRealFileSteps(t, dir, []string{
		`import "nonexistent"`,
	})
	if err == nil {
		t.Fatal("expected error for missing bare module")
	}
}
