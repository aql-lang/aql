package test

import (
	"bytes"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/aql-lang/aql/cmd/go/internal/run"
	lang "github.com/aql-lang/aql/lang/go"
	"github.com/aql-lang/aql/lang/go/native"
)

// write creates path with content and returns path.
func write(t *testing.T, path, content string) string {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

// runCmd invokes the subcommand with args and returns (exitCode, stdout, stderr).
func runCmd(args ...string) (int, string, string) {
	var stdout, stderr bytes.Buffer
	code := New().Run(args, nil, &stdout, &stderr)
	return code, stdout.String(), stderr.String()
}

const passSuite = "import \"aql:test\"\nTest.test \"ok\" [1 1 Assert.equal]\n"
const failSuite = "import \"aql:test\"\nTest.test \"bad\" [1 2 Assert.equal]\n"

func TestNameSynopsis(t *testing.T) {
	c := New()
	if c.Name() != "test" {
		t.Errorf("Name = %q, want test", c.Name())
	}
	if c.Synopsis() == "" {
		t.Error("Synopsis is empty")
	}
}

func TestRunFlagParseError(t *testing.T) {
	code, _, stderr := runCmd("--nonexistent-flag")
	if code != 1 {
		t.Errorf("code = %d, want 1", code)
	}
	if stderr == "" {
		t.Error("expected flag error on stderr")
	}
}

func TestRunPermsResolveError(t *testing.T) {
	// --perms and --perms-inline are mutually exclusive, so Resolve fails.
	code, _, stderr := runCmd("--perms", "none", "--perms-inline", "{}", ".")
	if code != 1 {
		t.Errorf("code = %d, want 1", code)
	}
	if !strings.Contains(stderr, "error:") {
		t.Errorf("stderr = %q, want a resolve error", stderr)
	}
}

func TestRunDiscoverError(t *testing.T) {
	code, _, stderr := runCmd(filepath.Join(t.TempDir(), "does-not-exist"))
	if code != 1 {
		t.Errorf("code = %d, want 1", code)
	}
	if !strings.Contains(stderr, "error:") {
		t.Errorf("stderr = %q, want a stat error", stderr)
	}
}

func TestRunNoFiles(t *testing.T) {
	code, _, stderr := runCmd(t.TempDir())
	if code != 1 {
		t.Errorf("code = %d, want 1", code)
	}
	if !strings.Contains(stderr, "no *_test.aql files") {
		t.Errorf("stderr = %q, want no-files message", stderr)
	}
}

// Run with no positional targets defaults to the cwd.
func TestRunDefaultTargetCwd(t *testing.T) {
	t.Chdir(t.TempDir()) // empty cwd → discovers nothing → no-files exit
	code, _, stderr := runCmd()
	if code != 1 {
		t.Errorf("code = %d, want 1", code)
	}
	if !strings.Contains(stderr, "no *_test.aql files") {
		t.Errorf("stderr = %q, want no-files message", stderr)
	}
}

func TestRunAllPass(t *testing.T) {
	dir := t.TempDir()
	write(t, filepath.Join(dir, "a_test.aql"), passSuite)
	write(t, filepath.Join(dir, "b_test.aql"), passSuite)
	code, stdout, _ := runCmd(dir)
	if code != 0 {
		t.Errorf("code = %d, want 0", code)
	}
	if !strings.Contains(stdout, "2 files: 2 passed, 0 failed") {
		t.Errorf("stdout = %q, want grand total", stdout)
	}
}

func TestRunFailures(t *testing.T) {
	f := write(t, filepath.Join(t.TempDir(), "x_test.aql"), failSuite)
	code, stdout, stderr := runCmd(f)
	if code != 1 {
		t.Errorf("code = %d, want 1", code)
	}
	if !strings.Contains(stdout, "0 passed, 1 failed") {
		t.Errorf("stdout = %q, want the fail tally", stdout)
	}
	// The framework's loud per-case FAIL line routes to the runner's stderr.
	if !strings.Contains(stderr, "FAIL") {
		t.Errorf("stderr = %q, want the loud FAIL line", stderr)
	}
}

// A file that raises at top level errors the run (not a recorded failure).
func TestRunErroredFile(t *testing.T) {
	f := write(t, filepath.Join(t.TempDir(), "boom_test.aql"),
		"import \"aql:test\"\nraise boom \"top-level\"\n")
	code, _, stderr := runCmd(f)
	if code != 1 {
		t.Errorf("code = %d, want 1", code)
	}
	if !strings.Contains(stderr, "error:") || !strings.Contains(stderr, "boom") {
		t.Errorf("stderr = %q, want the run error", stderr)
	}
}

// A file that runs fine but never imports aql:test can't be read for results.
func TestRunReadResultsError(t *testing.T) {
	f := write(t, filepath.Join(t.TempDir(), "noframework_test.aql"), "1 add 2\n")
	code, _, stderr := runCmd(f)
	if code != 1 {
		t.Errorf("code = %d, want 1", code)
	}
	if !strings.Contains(stderr, "reading results") {
		t.Errorf("stderr = %q, want a results-read error", stderr)
	}
}

func TestRunCoverage(t *testing.T) {
	dir := t.TempDir()
	// triple (rows 4-6) is never called, so its body row stays uncovered.
	mod := write(t, filepath.Join(dir, "calc.aql"),
		"def add2 fn [[x:Integer] [Integer] [\n  x add 2\n]]\n"+
			"def triple fn [[x:Integer] [Integer] [\n  x mul 3\n]]\n"+
			"export \"Calc\" { add2: add2/r  triple: triple/r }\n")
	f := write(t, filepath.Join(dir, "calc_test.aql"),
		"import \"aql:test\"\nimport \""+mod+"\"\nTest.test \"add2\" [(Calc.add2 3) 5 Assert.equal]\n")
	// Run on the interpreter so the uncovered set is exact (no VM position
	// folding): add2 is exercised, triple is not, so triple's body (row 5) is
	// the uncovered line.
	code, stdout, _ := runCmd("--coverage", "--no-compile", f)
	if code != 0 {
		t.Errorf("code = %d, want 0", code)
	}
	if !strings.Contains(stdout, "cover "+mod) || !strings.Contains(stdout, "%") {
		t.Errorf("stdout = %q, want a coverage line for the module", stdout)
	}
	if !strings.Contains(stdout, "uncovered: 5") {
		t.Errorf("stdout = %q, want the uncovered line numbers (5)", stdout)
	}
}

// coverageLine appends uncovered row numbers only when some remain.
func TestCoverageLine(t *testing.T) {
	full := coverageLine("m.aql", 4, 4, nil)
	if strings.Contains(full, "uncovered") {
		t.Errorf("fully-covered line = %q, want no uncovered suffix", full)
	}
	partial := coverageLine("m.aql", 2, 4, []int{3, 6})
	if !strings.Contains(partial, "uncovered: 3, 6") {
		t.Errorf("partial line = %q, want uncovered: 3, 6", partial)
	}
}

func TestRunNoCompile(t *testing.T) {
	f := write(t, filepath.Join(t.TempDir(), "n_test.aql"), passSuite)
	if code, _, _ := runCmd("--no-compile", f); code != 0 {
		t.Errorf("--no-compile code = %d, want 0", code)
	}
}

func TestRunForceCompile(t *testing.T) {
	f := write(t, filepath.Join(t.TempDir(), "fc_test.aql"), passSuite)
	if code, _, _ := runCmd("--force-compile", f); code != 0 {
		t.Errorf("--force-compile code = %d, want 0", code)
	}
}

// discover: explicit files are added verbatim (even without the suffix) and
// de-duplicated; a directory is walked for *_test.aql.
func TestDiscover(t *testing.T) {
	dir := t.TempDir()
	a := write(t, filepath.Join(dir, "a_test.aql"), passSuite)
	write(t, filepath.Join(dir, "not-a-suite.aql"), passSuite) // ignored by walk
	plain := write(t, filepath.Join(dir, "plain.aql"), passSuite)

	// Directory walk finds only the _test.aql file.
	got, err := discover([]string{dir})
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0] != a {
		t.Errorf("walk = %v, want [%s]", got, a)
	}

	// Explicit file (any name) + dedup of a repeated target.
	got, err = discover([]string{plain, plain})
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0] != plain {
		t.Errorf("explicit+dedup = %v, want [%s]", got, plain)
	}
}

func TestDiscoverWalkError(t *testing.T) {
	old := walkDir
	t.Cleanup(func() { walkDir = old })
	walkDir = func(root string, fn fs.WalkDirFunc) error {
		return fn(root, nil, errors.New("boom"))
	}
	if _, err := discover([]string{t.TempDir()}); err == nil {
		t.Fatal("expected a walk error")
	}
}

// runFile surfaces a file-read error as an errored suite.
func TestRunFileReadError(t *testing.T) {
	var stdout, stderr bytes.Buffer
	p, f, errored := runFile(&stdout, &stderr, filepath.Join(t.TempDir(), "gone_test.aql"), lang.Options{}, defaultMode(), false)
	if !errored || p != 0 || f != 0 {
		t.Errorf("runFile(missing) = (%d,%d,%v), want (0,0,true)", p, f, errored)
	}
	if !strings.Contains(stderr.String(), "error:") {
		t.Errorf("stderr = %q, want a read error", stderr.String())
	}
}

// runFile surfaces an instance-init error (newAQL seam).
func TestRunFileInitError(t *testing.T) {
	old := newAQL
	t.Cleanup(func() { newAQL = old })
	newAQL = func(...lang.Options) (*lang.AQL, error) { return nil, errors.New("init boom") }
	f := write(t, filepath.Join(t.TempDir(), "i_test.aql"), passSuite)
	var stdout, stderr bytes.Buffer
	if _, _, errored := runFile(&stdout, &stderr, f, lang.Options{}, defaultMode(), false); !errored {
		t.Error("runFile should report an init error as errored")
	}
	if !strings.Contains(stderr.String(), "init") {
		t.Errorf("stderr = %q, want an init error", stderr.String())
	}
}

func TestPercent(t *testing.T) {
	if got := percent(0, 0); got != 100 {
		t.Errorf("percent(0,0) = %v, want 100", got)
	}
	if got := percent(1, 2); got != 50 {
		t.Errorf("percent(1,2) = %v, want 50", got)
	}
}

// tallyFromSummary and stringFromStack skip the value type they don't consume.
func TestExtractHelpers(t *testing.T) {
	om := native.NewOrderedMap()
	om.Set("total", native.NewInteger(3))
	om.Set("passed", native.NewInteger(2))
	om.Set("failed", native.NewInteger(1))
	stack := []native.Value{native.NewMap(om), native.NewString("report text")}

	total, passed, failed := tallyFromSummary(stack)
	if total != 3 || passed != 2 || failed != 1 {
		t.Errorf("tally = (%d,%d,%d), want (3,2,1)", total, passed, failed)
	}
	if got := stringFromStack(stack); got != "report text" {
		t.Errorf("stringFromStack = %q, want report text", got)
	}
}

// defaultMode is CompileTry, the mode a no-flag invocation resolves to.
func defaultMode() run.CompileMode { return run.CompileTry }
