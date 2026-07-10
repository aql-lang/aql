package eng

import "testing"

// TestBodyNeedsFrameState pins the frame-state static analysis, including the
// splice-macro resolution that closes the capture hole where a `word`-macro
// hides a frameStateWord (fn/def/…) behind a bare word reference.
func TestBodyNeedsFrameState(t *testing.T) {
	r, err := NewRegistry()
	if err != nil {
		t.Fatal(err)
	}
	// A splice macro whose expansion constructs an inner fn — the hidden
	// frameStateword. `def mk word [fn [...]]`.
	fnBody := NewList([]Value{
		NewWord("fn"),
		NewList([]Value{NewWord("x"), NewWord("x"), NewWord("y")}),
	})
	r.Defs.Push("mk", NewSplice(fnBody))
	// A data-only splice macro — resolved and walked, but no frameStateword.
	r.Defs.Push("dat", NewSplice(NewList([]Value{NewInteger(1), NewInteger(2)})))
	// A plain (non-splice) value binding — resolved, not a splice.
	r.Defs.Push("v", NewInteger(5))
	// Mutually-recursive macros — exercise the cycle guard.
	r.Defs.Push("a", NewSplice(NewList([]Value{NewWord("b")})))
	r.Defs.Push("b", NewSplice(NewList([]Value{NewWord("a")})))

	cases := []struct {
		name string
		r    *Registry
		body []Value
		want bool
	}{
		// Literal frameStateword.
		{"literal-fn", r, []Value{NewWord("fn")}, true},
		// Two frameStatewords: the second callback hits the `needs` short-circuit.
		{"two-def", r, []Value{NewWord("def"), NewWord("x"), NewWord("def"), NewWord("z")}, true},
		// Splice macro hiding an fn — the closed hole.
		{"macro-fn", r, []Value{NewWord("mk")}, true},
		// Data-only splice macro — walked, nothing needs a frame.
		{"macro-data", r, []Value{NewWord("dat")}, false},
		// Plain recursion / native word: unresolved or non-splice, stays fast.
		{"plain-word", r, []Value{NewWord("add"), NewWord("someUnbound")}, false},
		// Non-splice value binding: resolved, Data is not SpliceInfo.
		{"value-binding", r, []Value{NewWord("v")}, false},
		// Mutually-recursive macros: cycle guard prevents infinite descent.
		{"macro-cycle", r, []Value{NewWord("a")}, false},
		// Nil registry: no resolution, only literal frameStatewords count.
		{"nil-reg-plain", nil, []Value{NewWord("mk")}, false},
		{"nil-reg-fn", nil, []Value{NewWord("fn")}, true},
	}
	for _, c := range cases {
		if got := bodyNeedsFrameState(c.r, c.body); got != c.want {
			t.Errorf("%s: bodyNeedsFrameState = %v, want %v", c.name, got, c.want)
		}
	}
}
