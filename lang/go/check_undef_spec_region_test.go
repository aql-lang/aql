package lang

import (
	gofmt "fmt"
	"testing"
)

// check_undef_spec_region_test.go pins the wrapped-undef FP fix
// (completeness-review §9.12; surfaced by PR #327's Codex review): an
// `undef` of an ENCLOSING binding inside a SPECULATIVE check region — a
// skipped error handler, a higher-order body over a possibly-empty
// collection, an uncalled fn body — must not leak the deletion into the
// pass model (core.Registry.SpecUndefBlocked, consulted by the undef
// handler; baselines pushed by the rolled-back nested-body run, the
// fn-body analysis, and the higher-order body analysis).
func TestUndefInSpeculativeRegionKeepsBinding(t *testing.T) {
	for _, c := range []struct{ name, src, want string }{
		{"skipped error handler", `def x 1 do [7] error [undef x 9] x`, "[7 1]"},
		{"each body over an empty collection", `def x 1 each [drop undef x] [] x`, "[[] 1]"},
		{"uncalled fn body", `def x 1 def f fn [[] [] [undef x]] x`, "[1]"},
	} {
		a, _ := New()
		res, err := a.Check(c.src)
		if err != nil {
			t.Fatalf("%s: check harness error: %v", c.name, err)
		}
		for _, d := range res.Diagnostics {
			if d.Code == "undefined_word" {
				t.Errorf("%s: the wrapped-undef FP regressed: [%s] %s", c.name, d.Code, d.Detail)
			}
		}
		out, rerr := mustNew(t).RunInterp(c.src)
		if rerr != nil || gofmt.Sprint(out) != c.want {
			t.Errorf("%s: witness = %v / %v, want %s clean", c.name, out, rerr, c.want)
		}
	}
}

// TestUndefCommitsOutsideSpeculativeRegions pins what the guard must NOT
// silence: a top-level undef and a `do`-body undef (leak fidelity — do
// bodies run unconditionally and leak their defs AND undefs) really delete
// the binding, on the checker and the runtime alike; and an undef of a
// binding created INSIDE the region still pops (frame teardown, body-local
// def+undef).
func TestUndefCommitsOutsideSpeculativeRegions(t *testing.T) {
	for _, c := range []struct {
		name, src  string
		wantFlag   bool
		runErrCode string
	}{
		{"top-level undef", `def x 1 undef x x`, true, "undefined_word"},
		{"do-body undef (leak fidelity)", `def x 1 do [undef x] x`, true, "undefined_word"},
		{"in-region def+undef", `def f fn [[] [Integer] [def y 1 undef y 5]] f`, false, ""},
	} {
		a, _ := New()
		res, err := a.Check(c.src)
		if err != nil {
			t.Fatalf("%s: check harness error: %v", c.name, err)
		}
		flagged := false
		for _, d := range res.Diagnostics {
			if d.Code == "undefined_word" {
				flagged = true
			}
		}
		if flagged != c.wantFlag {
			t.Errorf("%s: undefined_word flagged=%v, want %v (diags: %v)", c.name, flagged, c.wantFlag, res.Diagnostics)
		}
		_, rerr := mustNew(t).RunInterp(c.src)
		if c.runErrCode == "" && rerr != nil {
			t.Errorf("%s: want clean run, got %v", c.name, rerr)
		}
		if c.runErrCode != "" && rerr == nil {
			t.Errorf("%s: want runtime %s, ran clean", c.name, c.runErrCode)
		}
	}
}
