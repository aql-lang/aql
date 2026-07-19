package lang

import (
	"os"
	"path/filepath"
	"testing"
)

// TestCompiledCoverage verifies the compiled-mode coverage seam (the VM hook in
// eng/go/vm.go): when a coverage run is armed and a module-under-test runs
// COMPILED (its fns stamped to bytecode units on a coverID-tagged registry),
// the executed source rows are recorded via the VM hook — the path the
// `aql test --coverage` runner takes (arm the hook externally, run compiled).
//
// It also pins the HONEST relationship between the two engines' line coverage:
// the compiled covered set is a SUBSET of the interpreter's. The tree-walking
// interpreter steps every source token, so its row set is maximal; the bytecode
// VM folds some source positions (a trailing bare-word return like `r` compiles
// into the preceding expression's result, carrying no distinct instruction for
// its own row), so compiled coverage can MISS rows the interpreter records. It
// never covers a row the interpreter doesn't. (This is why the runner offers
// `--coverage --no-compile`: the interpreter's line-granular coverage is the
// one to drive to 100%.)
func TestCompiledCoverage(t *testing.T) {
	dir := t.TempDir()
	mod := filepath.Join(dir, "calc.aql")
	// add2's body is rows 2-3; triple's body is rows 6-7; the module never
	// exercises triple, so triple's body must stay uncovered in BOTH engines.
	src := "def add2 fn [[x:Integer] [Integer] [\n" +
		"  def r (x add 2)\n" +
		"  r\n" +
		"]]\n" +
		"def triple fn [[x:Integer] [Integer] [\n" +
		"  def r (x mul 3)\n" +
		"  r\n" +
		"]]\n" +
		"export \"Calc\" { add2: add2/r  triple: triple/r }\n"
	if err := os.WriteFile(mod, []byte(src), 0o644); err != nil {
		t.Fatal(err)
	}
	prog := "import \"" + mod + "\"\nCalc.add2 3"

	rowsFor := func(compiled bool) (map[int]bool, bool) {
		a, err := New(Options{})
		if err != nil {
			t.Fatal(err)
		}
		rows := map[int]bool{}
		disarm := a.NativeRegistry().ArmCoverageHook(func(id string, row int) {
			if id == mod {
				rows[row] = true
			}
		})
		defer disarm()
		if compiled {
			_, ran, reason, err := a.RunCompiledReason(prog)
			if err != nil {
				t.Fatalf("compiled run: %v", err)
			}
			return rows, ran && reason == ""
		}
		// The interpreter arm must be the TREE-WALKER (RunInterp), not the
		// compiled-by-default Run — otherwise this compares compiled to compiled
		// and the subset relationship below is vacuous.
		if _, err := a.RunInterp(prog); err != nil {
			t.Fatalf("interpreter run: %v", err)
		}
		return rows, false
	}

	compiledRows, ranCompiled := rowsFor(true)
	if !ranCompiled {
		t.Fatal("expected the program to run COMPILED (on the VM) — the VM coverage hook is what this test exercises")
	}
	if !compiledRows[2] {
		t.Errorf("compiled coverage missed add2's body (row 2); got %v", compiledRows)
	}
	if compiledRows[6] {
		t.Errorf("triple's body (row 6) recorded, but triple was never called; got %v", compiledRows)
	}

	interpRows, _ := rowsFor(false)
	// Compiled coverage is a SUBSET of interpreter coverage: the VM never
	// records a row the tree-walker doesn't.
	for row := range compiledRows {
		if !interpRows[row] {
			t.Errorf("row %d covered compiled but NOT interpreted — compiled must be a subset (compiled=%v interp=%v)", row, compiledRows, interpRows)
		}
	}
	// The interpreter is strictly more complete here: it records row 3 (add2's
	// trailing `r` return) that the VM folds into the preceding expression.
	if !interpRows[3] {
		t.Errorf("interpreter should cover add2's trailing return (row 3); got %v", interpRows)
	}
	if compiledRows[3] {
		t.Errorf("compiled unexpectedly covered row 3 — if the VM now records trailing returns, update this pin; got %v", compiledRows)
	}
	// triple stays uncovered in the interpreter too (it is never called).
	if interpRows[6] {
		t.Errorf("triple's body (row 6) recorded by the interpreter, but triple was never called; got %v", interpRows)
	}
}
