package native

import (
	"testing"

	"github.com/boru-lang/boru/eng/go/parser"
)

// w8Reg builds a fresh DefaultRegistry with the I/O words seeded, the
// standard driver for the W8 seam-8 coverage tests.
func w8Reg(t *testing.T) *Registry {
	t.Helper()
	r, err := DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	registerIOWords(r)
	return r
}

// w8Run parses and runs BORU source on a fresh registry, returning the
// final stack and any run error. Parse errors fail the test.
func w8Run(t *testing.T, src string) ([]Value, error) {
	t.Helper()
	r := w8Reg(t)
	toks, err := parser.Parse(src)
	if err != nil {
		t.Fatalf("parse %q: %v", src, err)
	}
	out, runErr := NewTop(r).Run(toks)
	return out, runErr
}

// w8RunOn parses and runs BORU source on the supplied registry.
func w8RunOn(t *testing.T, r *Registry, src string) ([]Value, error) {
	t.Helper()
	toks, err := parser.Parse(src)
	if err != nil {
		t.Fatalf("parse %q: %v", src, err)
	}
	return NewTop(r).Run(toks)
}
