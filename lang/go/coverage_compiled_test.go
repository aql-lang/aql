package lang

import (
	"os"
	"path/filepath"
	"testing"
)

// TestCompiledCoverage verifies the compiled-mode coverage seam (the VM hook in
// eng/go/vm.go): when a coverage run is armed and a module-under-test runs
// COMPILED (its fns stamped to bytecode units on a coverID-tagged registry),
// the executed source rows are recorded — the same rows the interpreter records
// (the compiled==interpreted differential). A `Test.cover` body usually can't
// compile (it holds an `import`), so this is the path the `aql test --coverage`
// runner takes: arm the hook externally, run the program compiled.
func TestCompiledCoverage(t *testing.T) {
	dir := t.TempDir()
	mod := filepath.Join(dir, "calc.aql")
	// add2's body is rows 2-3; triple's body is rows 6-7; the module never
	// exercises triple, so triple's body must stay uncovered.
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
		if _, err := a.Run(prog); err != nil {
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

	// Mode independence: the interpreter records the same covered set.
	interpRows, _ := rowsFor(false)
	if len(compiledRows) != len(interpRows) {
		t.Errorf("compiled/interpreter covered sets differ: compiled=%v interp=%v", compiledRows, interpRows)
	}
	for row := range compiledRows {
		if !interpRows[row] {
			t.Errorf("row %d covered compiled but not interpreted (compiled=%v interp=%v)", row, compiledRows, interpRows)
		}
	}
}
