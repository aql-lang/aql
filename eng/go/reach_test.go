package eng

import "testing"

// Phase A of design/REACH.0.md: the Ideal/Reach type, payload, and accessors.
// The parser does not emit Reach yet (Phase B), so these construct it directly.

func TestReachConstructAndAccess(t *testing.T) {
	info := ReachInfo{
		Receiver: []Value{NewWord("m")},
		Segments: []ReachSeg{
			{KeyLit: NewAtom("a")},
			{Getr: true, KeyLit: NewAtom("b")},
			{Computed: true, KeyExpr: []Value{NewWord("k")}},
		},
		Eval: true,
	}
	v := NewReach(info)

	if !IsReach(v) {
		t.Fatalf("IsReach = false for a Reach value (parent %s)", v.Parent)
	}
	if !v.Parent.Equal(TReach) {
		t.Fatalf("parent = %s, want Ideal/Reach", v.Parent)
	}

	got, err := AsReach(v)
	if err != nil {
		t.Fatalf("AsReach: %v", err)
	}
	if len(got.Receiver) != 1 || len(got.Segments) != 3 || !got.Eval {
		t.Fatalf("round-trip mismatch: %+v", got)
	}
	if got.Segments[1].Getr != true || got.Segments[2].Computed != true {
		t.Fatalf("segment flags not preserved: %+v", got.Segments)
	}
}

// AsReach must error (not panic) on a non-Reach value.
func TestAsReachRejectsNonReach(t *testing.T) {
	if _, err := AsReach(NewInteger(5)); err == nil {
		t.Fatal("AsReach(5) should error, got nil")
	}
}
