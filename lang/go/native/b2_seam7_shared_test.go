package native

import (
	"testing"

	"github.com/boru-lang/boru/parser/go"
)

// Wave-7 (cluster B2) shared drivers for the *_seam7_test.go coverage
// suites (design/TEST-SEAMS.10.md). Uniquely named (b2*) so they never
// collide with any other wave's shared helpers when patches are merged.
// Most B2 error/edge arms are reached by calling the handler directly
// with crafted args; these entry points cover the arms that only fire
// through real dispatch or the static-analysis pass.

// b2Reg returns a fresh default registry with the parser installed and
// the boru:io words seeded (some behaviour uses print/trace).
func b2Reg(t *testing.T) *Registry {
	t.Helper()
	r, err := DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	r.SetParseFunc(parser.Parse)
	registerIOWords(r)
	return r
}

// b2Run parses and runs src on r, returning the residual stack.
func b2Run(r *Registry, src string) ([]Value, error) {
	toks, err := parser.Parse(src)
	if err != nil {
		return nil, err
	}
	r.Source = src
	e := NewTop(r)
	e.SetSource(src)
	return e.Run(toks)
}
