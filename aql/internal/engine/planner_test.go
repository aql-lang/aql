package engine

import "testing"

func TestPlannerPrefixCoverage(t *testing.T) {
	resolved := []Value{NewInteger(1), NewString("a")}
	count, used := plannerPrefixCoverage([]Type{TInteger, TString}, resolved)
	if count != 2 {
		t.Fatalf("expected 2 prefix values, got %d", count)
	}
	if !(used[0] && used[1]) {
		t.Fatalf("expected both arg slots used, got %#v", used)
	}
}

func TestUnifiedPlannerFlag_BasicInfix(t *testing.T) {
	r, err := DefaultRegistry()
	if err != nil {
		t.Fatalf("new registry: %v", err)
	}
	e := New(r)

	out, err := e.Run([]Value{NewInteger(2), NewWord("add"), NewInteger(3)})
	if err != nil {
		t.Fatalf("run failed: %v", err)
	}
	if len(out) != 1 || out[0].AsInteger() != 5 {
		t.Fatalf("unexpected output: %#v", out)
	}
}

func TestPeekPlannableSuffixValue(t *testing.T) {
	r, err := DefaultRegistry()
	if err != nil {
		t.Fatalf("new registry: %v", err)
	}
	e := New(r)
	e.stack = []Value{NewWord("add"), NewInteger(2)}
	e.pointer = 0

	v := e.peekPlannableSuffixValue()
	if v == nil || v.AsInteger() != 2 {
		t.Fatalf("expected suffix integer 2, got %#v", v)
	}
}

func TestPeekPlannableSuffixValue_StructuralWordsSkipped(t *testing.T) {
	r, err := DefaultRegistry()
	if err != nil {
		t.Fatalf("new registry: %v", err)
	}
	e := New(r)
	e.stack = []Value{NewWord("add"), NewWord("end")}
	e.pointer = 0
	if v := e.peekPlannableSuffixValue(); v != nil {
		t.Fatalf("expected nil for structural suffix token, got %#v", v)
	}
}

func TestPlannerBestSigForForward_NoCandidates(t *testing.T) {
	r, err := DefaultRegistry()
	if err != nil {
		t.Fatalf("new registry: %v", err)
	}
	e := New(r)
	fn := r.Lookup("add")
	if fn == nil {
		t.Fatal("add function not registered")
	}
	// Impossible arg-count filter should reject all signatures.
	sig, prefix := e.plannerBestSigForForward(fn, WordInfo{Name: "add", ArgCount: 999}, nil)
	if sig != nil || prefix != 0 {
		t.Fatalf("expected no candidate, got sig=%#v prefix=%d", sig, prefix)
	}
}

func TestPlannerPrefixCoverage_PartialPositional(t *testing.T) {
	// One integer value should positionally match first arg of [integer, integer].
	resolved := []Value{NewInteger(2)}
	count, used := plannerPrefixCoverage([]Type{TInteger, TInteger}, resolved)
	if count != 1 {
		t.Fatalf("expected 1 prefix value, got %d", count)
	}
	if !used[0] || used[1] {
		t.Fatalf("expected only first arg slot used, got %#v", used)
	}
}
