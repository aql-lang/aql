package main

import (
	"flag"
	"os"
	"path/filepath"
	"testing"
)

// exitCode drives fn with the osExit seam swapped to a panicking stub
// (execution must not continue past an exit point) and returns the code
// it exited with, or -1 when fn returned normally.
func exitCode(t *testing.T, fn func()) (code int) {
	t.Helper()
	prev := osExit
	t.Cleanup(func() { osExit = prev })
	type exited struct{ code int }
	osExit = func(c int) { panic(exited{c}) }
	code = -1
	defer func() {
		if r := recover(); r != nil {
			e, ok := r.(exited)
			if !ok {
				panic(r)
			}
			code = e.code
		}
	}()
	fn()
	return code
}

// runMain drives main() with swapped os.Args (flag.CommandLine is
// rebuilt per call because main registers its flags on the global set).
func runMain(t *testing.T, args ...string) int {
	t.Helper()
	prevArgs, prevFlags := os.Args, flag.CommandLine
	t.Cleanup(func() { os.Args, flag.CommandLine = prevArgs, prevFlags })
	flag.CommandLine = flag.NewFlagSet("specgen", flag.ExitOnError)
	os.Args = append([]string{"specgen"}, args...)
	return exitCode(t, main)
}

func TestUsageExits(t *testing.T) {
	// Each mode with missing required flags exits 2.
	if got := runMain(t, "-frontier"); got != 2 {
		t.Errorf("-frontier without flags exited %d, want 2", got)
	}
	if got := runMain(t, "-extend5"); got != 2 {
		t.Errorf("-extend5 without flags exited %d, want 2", got)
	}
	if got := runMain(t, "-extract"); got != 2 {
		t.Errorf("-extract without flags exited %d, want 2", got)
	}
}

func TestGenerateCreateFailureExits(t *testing.T) {
	// -out into a missing directory: the create fails and exits 1.
	bad := filepath.Join(t.TempDir(), "no-such-dir", "m.tsv")
	if got := runMain(t, "-max", "1", "-out", bad); got != 1 {
		t.Errorf("generate with bad -out exited %d, want 1", got)
	}
}

func TestModeIOFailureExits(t *testing.T) {
	dir := t.TempDir()
	missing := filepath.Join(dir, "absent.tsv")
	badOut := filepath.Join(dir, "no-such-dir", "out.tsv")

	// A tiny but valid passing file for the write-side failures.
	passing := filepath.Join(dir, "passing.tsv")
	if err := os.WriteFile(passing, []byte("#kept: 1 of 1 scanned (max-len 1)\n1\t1\n"), 0644); err != nil {
		t.Fatal(err)
	}

	cases := []struct {
		name string
		fn   func()
		want int
	}{
		{"extractPassing missing -in", func() { extractPassing(missing, badOut, 4) }, 1},
		{"extractPassing bad -out", func() { extractPassing(passing, badOut, 4) }, 1},
		{"extractFrontier missing -passing", func() { extractFrontier(missing, badOut, badOut, badOut, 4) }, 1},
		{"extractFrontier bad outputs", func() { extractFrontier(passing, badOut, badOut, badOut, 4) }, 1},
		{"extendFive missing -passing", func() { extendFive(missing, missing, badOut, badOut, badOut, badOut, "") }, 1},
		{"extendFive missing -len123", func() { extendFive(passing, missing, badOut, badOut, badOut, badOut, "") }, 1},
		{"extendFive bad outputs", func() { extendFive(passing, passing, badOut, badOut, badOut, badOut, "") }, 1},
		{"writeFrontierFile bad path", func() { writeFrontierFile(badOut, "check", "t", nil, nil, nil, classCheck, 4) }, 1},
		{"writeLen5Pass bad path", func() { writeLen5Pass(badOut, nil, nil, nil) }, 1},
		{"writeLen5Fail bad path", func() { writeLen5Fail(badOut, "check", nil, nil, nil, fiveCheck) }, 1},
	}
	for _, c := range cases {
		if got := exitCode(t, c.fn); got != c.want {
			t.Errorf("%s: exited %d, want %d", c.name, got, c.want)
		}
	}
}
