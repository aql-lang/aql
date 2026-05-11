package test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/metsitaba/voxgig-exp/eng/parser"
	"github.com/metsitaba/voxgig-exp/lang/internal/engine"
	"github.com/metsitaba/voxgig-exp/lang/internal/fileops"
	"github.com/metsitaba/voxgig-exp/lang/internal/native"
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
func runRealFileSteps(t *testing.T, dir string, steps []string) ([]engine.Value, error) {
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

	reg, err := engine.DefaultRegistry(native.Register)
	if err != nil {
		t.Fatal(err)
	}
	native.Register(reg)
	engine.SetHostFileOps(reg, fileops.NewDefault())
	reg.SetParseFunc(parser.Parse)
	reg.BaseDir = absDir

	eng := engine.New(reg)
	var result []engine.Value
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
