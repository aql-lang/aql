package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestVaryUsageExits — -vary without -vary-out exits 2.
func TestVaryUsageExits(t *testing.T) {
	if got := runMain(t, "-vary"); got != 2 {
		t.Errorf("-vary without -vary-out exited %d, want 2", got)
	}
}

// TestVarySweepIOFailures — a missing seed dir and an uncreatable output
// directory both exit 1.
func TestVarySweepIOFailures(t *testing.T) {
	dir := t.TempDir()
	if got := exitCode(t, func() { runVarySweep(filepath.Join(dir, "absent"), dir, 1) }); got != 1 {
		t.Errorf("missing seed dir exited %d, want 1", got)
	}

	seedDir := filepath.Join(dir, "seeds")
	if err := os.Mkdir(seedDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(seedDir, "s.tsv"), []byte("1 add 2\t3\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	badOut := filepath.Join(dir, "no-such-dir")
	if got := exitCode(t, func() { runVarySweep(seedDir, badOut, 1) }); got != 1 {
		t.Errorf("bad -vary-out exited %d, want 1", got)
	}
}

// TestVarySweepEndToEnd — a tiny corpus (one passing seed, one refusing
// seed, one interp-rejected seed) classified through the real pipeline via
// the main() dispatch, producing the expected classification files.
func TestVarySweepEndToEnd(t *testing.T) {
	dir := t.TempDir()
	seedDir := filepath.Join(dir, "seeds")
	outDir := filepath.Join(dir, "out")
	for _, d := range []string{seedDir, outDir} {
		if err := os.Mkdir(d, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	corpus := "1 add 2\t3\n" + // passing base: fans out into every transform
		"for 3 [1 2]\t1 2 1 2 1 2\n" + // refused base: skipped, no variants
		// A passing base whose do-body variant DIVERGES (the known do-unit
		// registry-replay miscompile, lang/spec/frontier/
		// frontier-do-registry-replay.tsv) — drives the divergence report arm
		// through the real pipeline.
		"def Big Integer 15 is Big\ttrue\n" +
		"# comment\nbroken zz\tERROR:undefined_word\n" // error row: not a seed
	if err := os.WriteFile(filepath.Join(seedDir, "s.tsv"), []byte(corpus), 0o644); err != nil {
		t.Fatal(err)
	}

	if got := runMain(t, "-vary", "-seed-dir", seedDir, "-vary-out", outDir); got != -1 {
		t.Fatalf("vary sweep exited %d, want normal return", got)
	}

	pass, err := os.ReadFile(filepath.Join(outDir, "vary-pass.tsv"))
	if err != nil {
		t.Fatalf("vary-pass.tsv: %v", err)
	}
	if !strings.Contains(string(pass), "1 add 2") {
		t.Errorf("vary-pass.tsv has no variant of the passing seed:\n%s", pass)
	}
	div, err := os.ReadFile(filepath.Join(outDir, "vary-diverged.tsv"))
	if err != nil {
		t.Fatalf("vary-diverged.tsv (the do-unit registry-replay variant must diverge): %v", err)
	}
	if !strings.Contains(string(div), "do [def Big Integer 15 is Big]") {
		t.Errorf("vary-diverged.tsv missing the registry-replay divergence:\n%s", div)
	}
	// The refusing seed contributes NO variant rows (its base is skipped) —
	// no row of any output file may reference s.tsv:2.
	for _, fn := range []string{"vary-pass.tsv", "vary-refused.tsv", "vary-diverged.tsv", "vary-interp-reject.tsv"} {
		data, err := os.ReadFile(filepath.Join(outDir, fn))
		if os.IsNotExist(err) {
			continue // a bucket with no rows writes no file
		}
		if err != nil {
			t.Fatal(err)
		}
		if strings.Contains(string(data), "s.tsv:2") {
			t.Errorf("%s references the refused base's variants:\n%s", fn, data)
		}
	}
}
