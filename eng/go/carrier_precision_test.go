package eng

import "testing"

// LiteralCondValue reads a decided Boolean through a GuardFactInfo wrapper: a
// pre-evaluated paren cond arrives as a Boolean carrier whose Prev payload is
// the value the group reduced to, so the unreachable-branch analysis still
// sees the literal truth value.
func TestLiteralCondValueGuardFactPrev(t *testing.T) {
	for _, want := range []bool{true, false} {
		elem := NewCarrier(TBoolean)
		elem.Data = GuardFactInfo{Prev: BoolPayload{B: want}}
		got, ok := LiteralCondValue(NewList([]Value{elem}))
		if !ok || got != want {
			t.Errorf("LiteralCondValue(Prev=%v) = %v/%v, want %v/true", want, got, ok, want)
		}
	}
	// NEGATIVE: a GuardFactInfo whose Prev is NOT a BoolPayload does not decide
	// the cond (falls through the wrapper read).
	elem := NewCarrier(TBoolean)
	elem.Data = GuardFactInfo{Prev: IntPayload{N: 1}}
	if _, ok := LiteralCondValue(NewList([]Value{elem})); ok {
		t.Error("a non-Boolean Prev must not decide the cond")
	}
}

// AnalyseFnBody gradualizes an Undefined-atom argument (a forward-referenced fn
// name that leaked into a per-call-site analysis) to a dynamic carrier at the
// analysis boundary, instead of driving dispatch/memo keying with the phantom
// concrete Atom.
func TestAnalyseFnBodyGradualizesUndefinedArg(t *testing.T) {
	r, err := NewRegistry()
	if err != nil {
		t.Fatal(err)
	}
	r.Check.Mode = true

	und := NewAtom("phantom")
	und.Undefined = true
	body := []Value{NewWord("x")} // trivial body returning the param

	// The call must not panic and must produce a residual (the gradualized
	// carrier flows through the body).
	got := AnalyseFnBody(r, "f", []string{"x"}, body, []Value{und}, nil, nil, false)
	if len(got) == 0 {
		t.Fatal("AnalyseFnBody over an undefined arg produced no residual")
	}
	if got[0].Undefined {
		t.Error("the residual must be gradualized, not a raw Undefined atom")
	}

	// A NON-undefined arg leaves the sanitization loop a no-op (the continue
	// path) — a different fn name so the memo does not short-circuit.
	got2 := AnalyseFnBody(r, "g", []string{"x"}, body, []Value{NewInteger(3)}, nil, nil, false)
	if len(got2) == 0 {
		t.Fatal("AnalyseFnBody over a concrete arg produced no residual")
	}
}
