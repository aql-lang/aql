package modules

import (
	"github.com/boru-lang/boru/eng/go/stackform"
	"github.com/boru-lang/boru/lang/go/native"
)

// Test seams for wave-9 coverage (design/TEST-SEAMS.10.md). Each var defaults
// to the real implementation and is swapped only by *_seam9_test.go (with a
// t.Cleanup restore, never in parallel) to drive an otherwise-unreachable
// error arm — the PBT shrink machinery's rand-instance / registry builders and
// StackForm evaluator whose failures never occur on well-formed input, and the
// test-module preamble parse whose fixed source always parses/runs cleanly.
// W9-prefixed to avoid collision with the shared seams.go and the other
// coverage agents' identifiers.
var (
	// w9BuildSeededRandInstance seams BuildSeededRandInstance for the PBT
	// per-iteration and gen-program-shrink rand-map builders (test.go:745 /
	// 907). The builder only fails on an internal registry-construction error
	// the built-in rand natives never produce.
	w9BuildSeededRandInstance = BuildSeededRandInstance

	// w9BuildSeededRandRegistry seams BuildSeededRandRegistry for the
	// gen-program shrinker's per-candidate eval registry (test.go:928 / 972).
	w9BuildSeededRandRegistry = BuildSeededRandRegistry

	// w9StackformEval seams stackform.Eval for the shrinker's candidate /
	// materialise evaluations (test.go:932 / 976 / 1026 / 1062). A
	// PushLit-shaped form never fails to evaluate.
	w9StackformEval = stackform.Eval

	// w9ParsePreamble seams the boru:test preamble parse in BuildTestModule
	// (test.go:57-61 parse-error arm, and the run-error arm at 128 when the
	// returned tokens run-fail). Defaults to the sync.Once-guarded parse.
	w9ParsePreamble = w9ParsePreambleReal
)

// w9ParsePreambleReal is the production preamble parse: the sync.Once-guarded
// single parse of testBoruPreamble, returning the cached tokens/error.
func w9ParsePreambleReal(parent *native.Registry) ([]native.Value, error) {
	testParseOnce.Do(func() {
		testParsed, testParseErr = parent.ParseFunc(testBoruPreamble)
	})
	return testParsed, testParseErr
}
