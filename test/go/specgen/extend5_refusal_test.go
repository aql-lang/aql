package main

import (
	"os"
	"path/filepath"
	"testing"
)

// TestExtendLayerCompileBucket pins -extend5's compile-refusal bucket with a
// SYNTHETIC passing root whose one-atom extensions refuse: a module-export
// value left on the stack has no static materialisation, so `<root> drop`
// (and friends) classify as compile-refused. The pipeline test's generated
// alphabet corpus stopped producing this bucket when OpDispatchRematch
// compiled its last refusing candidates — a user-supplied corpus still can,
// so the arm is pinned here with one.
func TestExtendLayerCompileBucket(t *testing.T) {
	dir := t.TempDir()
	passing := filepath.Join(dir, "passing.tsv")
	// The root is interpreter-clean (its expected value is the module
	// render); the pass1 file is empty so every extension of the root is a
	// candidate.
	if err := os.WriteFile(passing,
		[]byte("def m (module [export \"X\" {a: 1}]) m\tModule(inline)\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	pass1 := filepath.Join(dir, "pass1.tsv")
	if err := os.WriteFile(pass1, []byte(""), 0o644); err != nil {
		t.Fatal(err)
	}
	pOut := filepath.Join(dir, "p.tsv")
	ckOut := filepath.Join(dir, "ck.tsv")
	coOut := filepath.Join(dir, "co.tsv")
	roOut := filepath.Join(dir, "ro.tsv")
	mmOut := filepath.Join(dir, "mm.tsv")
	runSpecgenMain(t, "-extend5",
		"-passing", passing, "-len123", pass1,
		"-pass-out", pOut, "-check-out", ckOut, "-compile-out", coOut,
		"-runtime-out", roOut, "-mismatch-out", mmOut)

	rows := decodeFile(t, coOut)
	if len(rows) == 0 {
		t.Fatal("the module-residual root must yield compile-refused extensions (the five-way compile bucket)")
	}
	found := false
	for _, r := range rows {
		if r.input == `def m (module [export "X" {a: 1}]) m drop` {
			found = true
			if r.expected == "" {
				t.Error("the compile bucket row must carry the refusal reason")
			}
		}
	}
	if !found {
		t.Errorf("`... m drop` missing from the compile bucket; rows: %v", rows)
	}
}
