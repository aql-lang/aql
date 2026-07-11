package test

import (
	"strings"
	"testing"

	"github.com/aql-lang/aql/eng/go/parser"
	"github.com/aql-lang/aql/lang/go/native"
)

// runResidualSrc parses and runs src on a fresh default registry (no
// check pass — runtime semantics only), returning the run error.
func runResidualSrc(t *testing.T, src string) error {
	t.Helper()
	reg, err := native.DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	reg.ParseFunc = parser.Parse
	vals, pErr := parser.Parse(src)
	if pErr != nil {
		t.Fatal(pErr)
	}
	_, rErr := native.NewTop(reg).Run(vals)
	return rErr
}

// TestResidualEvalErrorPropagation pins the error path of the in-frame
// residual evaluation (the multi-token unification): a trailing computed
// container whose member raises propagates the error at the frame end,
// through each step-loop context that processes the frame's DefCleanup
// marker.
func TestResidualEvalErrorPropagation(t *testing.T) {
	const prDef = `def pr fn [[s:String] [Any] [def z 0 {a: (raise "boom")}]] `
	cases := []struct {
		name, src string
	}{
		// The main step loop (a direct statement call).
		{"main-loop", prDef + `pr "x"`},
		// The data-context paren evaluator (a map member's paren group).
		{"paren-group", prDef + `def m {k: (pr "x")} m`},
		// A word-context paren wrap.
		{"word-paren", prDef + `def r (pr "x") r`},
		// An if-arm body (fragment context).
		{"if-arm", prDef + `if true [pr "x"] [0]`},
		// A for body (loop fragment context).
		{"for-body", prDef + `for 1 [pr "x"]`},
	}
	for _, tc := range cases {
		err := runResidualSrc(t, tc.src)
		if err == nil || !strings.Contains(err.Error(), "boom") {
			t.Errorf("%s: err = %v, want boom", tc.name, err)
		}
	}
}

// TestResidualEvalHappyContexts pins the value side in the same contexts:
// the trailing container resolves the frame-locals in-frame everywhere.
func TestResidualEvalHappyContexts(t *testing.T) {
	const prDef = `def pr fn [[s:String] [Any] [def z 7 {a: z}]] `
	cases := []struct {
		name, src string
	}{
		{"main-loop", prDef + `pr "x"`},
		{"paren-group", prDef + `def m {k: (pr "x")} m.k`},
		{"word-paren", prDef + `def r (pr "x") r`},
	}
	for _, tc := range cases {
		if err := runResidualSrc(t, tc.src); err != nil {
			t.Errorf("%s: err = %v, want in-frame values", tc.name, err)
		}
	}
}
