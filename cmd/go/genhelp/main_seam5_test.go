// Seam-based coverage for the genhelp generator (design/TEST-SEAMS.10.md):
// main() through the runGenhelp/osExit seams, run()'s error arms through
// the newDefaultRegistry and formatSource seams, the duplicate-expression
// skip through the exampleExprs seam, and seedMemFS's capability-store
// error arm through a nil Capabilities registry.
//
// Every test here swaps package-level seams, so none of them may use
// t.Parallel().
package main

import (
	"bytes"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/aql-lang/aql/lang/go/native"
	"github.com/aql-lang/aql/lang/go/native/help"
)

func TestSeam5MainSuccess(t *testing.T) {
	origRun := runGenhelp
	origExit := osExit
	t.Cleanup(func() {
		runGenhelp = origRun
		osExit = origExit
	})

	var gotPath string
	var gotErrw io.Writer
	runGenhelp = func(outPath string, errw io.Writer) error {
		gotPath = outPath
		gotErrw = errw
		return nil
	}
	var exitCodes []int
	osExit = func(code int) { exitCodes = append(exitCodes, code) }

	main()

	if want := outputPath(); gotPath != want {
		t.Errorf("main() generated into %q, want outputPath() = %q", gotPath, want)
	}
	if gotErrw != io.Writer(os.Stderr) {
		t.Errorf("main() progress writer = %v, want os.Stderr", gotErrw)
	}
	if len(exitCodes) != 0 {
		t.Errorf("os.Exit called with %v on success, want no exit", exitCodes)
	}
}

func TestSeam5MainFailure(t *testing.T) {
	origRun := runGenhelp
	origExit := osExit
	origStderr := os.Stderr
	t.Cleanup(func() {
		runGenhelp = origRun
		osExit = origExit
		os.Stderr = origStderr
	})

	runGenhelp = func(string, io.Writer) error { return errors.New("boom") }
	var exitCodes []int
	osExit = func(code int) { exitCodes = append(exitCodes, code) }

	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	os.Stderr = w
	done := make(chan string, 1)
	go func() {
		b, _ := io.ReadAll(r)
		done <- string(b)
	}()

	main()

	w.Close()
	os.Stderr = origStderr
	stderr := <-done

	if len(exitCodes) != 1 || exitCodes[0] != 1 {
		t.Errorf("exit codes = %v, want [1] on generator failure", exitCodes)
	}
	if want := "genhelp: boom\n"; stderr != want {
		t.Errorf("stderr = %q, want %q", stderr, want)
	}
}

func TestSeam5RunRegistryError(t *testing.T) {
	orig := newDefaultRegistry
	t.Cleanup(func() { newDefaultRegistry = orig })
	regErr := errors.New("registry construction failed")
	newDefaultRegistry = func(...func(*native.Registry)) (*native.Registry, error) {
		return nil, regErr
	}

	err := run(filepath.Join(t.TempDir(), "x.go"), io.Discard)
	if !errors.Is(err, regErr) {
		t.Fatalf("run() = %v, want the registry construction error", err)
	}
}

func TestSeam5RunWordRegistryError(t *testing.T) {
	orig := newDefaultRegistry
	t.Cleanup(func() { newDefaultRegistry = orig })
	wordErr := errors.New("per-word registry failed")
	calls := 0
	newDefaultRegistry = func(providers ...func(*native.Registry)) (*native.Registry, error) {
		calls++
		if calls == 1 {
			return orig(providers...)
		}
		return nil, wordErr
	}

	err := run(filepath.Join(t.TempDir(), "x.go"), io.Discard)
	if !errors.Is(err, wordErr) {
		t.Fatalf("run() = %v, want the per-word registry error", err)
	}
}

func TestSeam5RunSeedError(t *testing.T) {
	orig := newDefaultRegistry
	t.Cleanup(func() { newDefaultRegistry = orig })
	calls := 0
	// Word registries come back with no capability store, so seedMemFS's
	// Capabilities.Set fails and run surfaces the wrapped seed error.
	newDefaultRegistry = func(providers ...func(*native.Registry)) (*native.Registry, error) {
		calls++
		reg, err := orig(providers...)
		if err != nil {
			return nil, err
		}
		if calls > 1 {
			reg.Capabilities = nil
		}
		return reg, nil
	}

	err := run(filepath.Join(t.TempDir(), "x.go"), io.Discard)
	if err == nil || !strings.Contains(err.Error(), "seed fs:") {
		t.Fatalf("run() = %v, want a wrapped 'seed fs:' error", err)
	}
}

func TestSeam5RunSkipsDuplicateExprs(t *testing.T) {
	orig := exampleExprs
	t.Cleanup(func() { exampleExprs = orig })
	// Every word reports the same expression twice: the second (and every
	// later) occurrence must hit the already-computed skip, leaving exactly
	// one result.
	exampleExprs = func(help.FuncInfo) []string {
		return []string{"1 2 add", "1 2 add"}
	}

	out := filepath.Join(t.TempDir(), "examples_gen.go")
	var errb bytes.Buffer
	if err := run(out, &errb); err != nil {
		t.Fatalf("run: %v", err)
	}
	src, err := os.ReadFile(out)
	if err != nil {
		t.Fatalf("read generated: %v", err)
	}
	if !strings.Contains(string(src), `"1 2 add": "3"`) {
		t.Errorf("generated source missing the deduplicated row:\n%s", src)
	}
	if !strings.Contains(errb.String(), "genhelp: wrote 1 examples") {
		t.Errorf("progress line = %q, want exactly one example written", errb.String())
	}
}

func TestSeam5RunFormatSourceError(t *testing.T) {
	origFmt := formatSource
	origEx := exampleExprs
	t.Cleanup(func() {
		formatSource = origFmt
		exampleExprs = origEx
	})
	// No examples to evaluate: the test only targets the gofmt arm.
	exampleExprs = func(help.FuncInfo) []string { return nil }
	formatSource = func([]byte) ([]byte, error) { return nil, errors.New("bad token") }

	err := run(filepath.Join(t.TempDir(), "x.go"), io.Discard)
	if err == nil || !strings.Contains(err.Error(), "gofmt generated source:") {
		t.Fatalf("run() = %v, want a wrapped gofmt error", err)
	}
}

func TestSeam5SeedMemFSNilCapabilities(t *testing.T) {
	reg := newRegistry(t)
	reg.Capabilities = nil
	if err := seedMemFS(reg); err == nil {
		t.Fatal("seedMemFS with a nil capability store must fail, got nil error")
	}
}
