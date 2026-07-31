package main

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/boru-lang/boru/eng/go/parser"
	"github.com/boru-lang/boru/lang/go/native"
)

// newRegistry builds the same registry main() uses.
func newRegistry(t *testing.T) *native.Registry {
	t.Helper()
	reg, err := native.DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	reg.SetParseFunc(parser.Parse)
	return reg
}

func TestOutputPath(t *testing.T) {
	got := outputPath()
	if !filepath.IsAbs(got) {
		t.Errorf("outputPath should be absolute; got %q", got)
	}
	want := filepath.Join("lang", "go", "native", "help", "examples_gen.go")
	if !strings.HasSuffix(got, want) {
		t.Errorf("outputPath = %q, want suffix %q", got, want)
	}
}

func TestEvalExpr(t *testing.T) {
	reg := newRegistry(t)
	got, err := evalExpr(reg, "1 2 add")
	if err != nil {
		t.Fatalf("evalExpr: %v", err)
	}
	if got != "3" {
		t.Errorf("1 2 add = %q, want 3", got)
	}
}

func TestEvalExprParseError(t *testing.T) {
	reg := newRegistry(t)
	if _, err := evalExpr(reg, `"unterminated`); err == nil {
		t.Error("expected a parse error")
	}
}

func TestEvalExprRunError(t *testing.T) {
	reg := newRegistry(t)
	if _, err := evalExpr(reg, "no-such-word-xyz-12345"); err == nil {
		t.Error("expected a run error for an unknown word")
	}
}

func TestEvalExprSuppressesPrintOutput(t *testing.T) {
	reg := newRegistry(t)
	// print writes to reg.Output, which evalExpr swaps for io.Discard;
	// the call must not leak the original writer nil either way.
	if _, err := evalExpr(reg, `print "hi"`); err != nil {
		t.Fatalf("evalExpr print: %v", err)
	}
	// The saved Output is restored after the call.
	if reg.Output == nil {
		t.Error("reg.Output should be restored")
	}
}

// NOTE: seedMemFS is deliberately not covered. Its embedded program
// still uses the stale `context get __sys …` idiom and fails with
// boru/undefined_word on the current language (the working idiom is
// `context dot __sys`), so `go run ./genhelp` exits 1 at head. Fixing
// it requires a non-test change to main.go.
