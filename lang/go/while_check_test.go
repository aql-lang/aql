package lang

import (
	"strings"
	"testing"
)

// TestWhileCheckModel drives whileReturnsFn's arms through the full
// checker braid: the typed-List approximation, the zero-net body, the
// disjunct-topped body, and the carrier-conditioned loop that must NOT
// loop the analysis (the model replaces stepping the spliced regions).
func TestWhileCheckModel(t *testing.T) {
	cases := []struct {
		name, src string
	}{
		{"typed", `while [false] [1] end 0`},
		{"zero-net", `while [false] [] end 0`},
		{"disjunct", `def f fn n:Integer List [ while [n gt 0] [ if (n gt 1) [1] ['s'] ] ] end 0`},
		{"carrier-cond", `def g fn n:Integer List [ while [n gt 0] [1] ] end 0`},
	}
	for _, c := range cases {
		a, err := New()
		if err != nil {
			t.Fatal(err)
		}
		res, cerr := a.Check(c.src)
		if cerr != nil {
			t.Fatalf("%s: check error: %v", c.name, cerr)
		}
		if res.Summary.Errors != 0 {
			t.Errorf("%s: check reported %d error(s)", c.name, res.Summary.Errors)
		}
	}
}

// TestWhileStepBudget pins the §5.9-closure guarantee: a non-terminating
// while trips evaluation_limit under the step budget instead of hanging
// (the regions are engine-stepped).
func TestWhileStepBudget(t *testing.T) {
	a, err := New()
	if err != nil {
		t.Fatal(err)
	}
	a.NativeRegistry().StepLimit = 50_000
	_, rerr := a.RunInterp(`while [true] []`)
	if rerr == nil || !strings.Contains(rerr.Error(), "evaluation_limit") {
		t.Fatalf("want evaluation_limit, got %v", rerr)
	}
}
