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

// Phase C: canon renders a Reach back to its dotted surface (round-trip).
func TestReachCanonRoundTrip(t *testing.T) {
	mk := func(recv []Value, segs []ReachSeg) Value {
		return NewReach(ReachInfo{Receiver: recv, Segments: segs, Eval: true})
	}
	cases := []struct {
		name string
		v    Value
		want string
	}{
		{"bare", mk([]Value{NewWord("m")}, []ReachSeg{{KeyLit: NewWord("a")}, {KeyLit: NewWord("b")}}), "m.a.b"},
		{"getr", mk([]Value{NewWord("m")}, []ReachSeg{{Getr: true, KeyLit: NewWord("x")}}), "m!.x"},
		{"strkey", mk([]Value{NewWord("a")}, []ReachSeg{{KeyLit: NewString("x")}, {KeyLit: NewWord("c")}}), "a.'x'.c"},
		{"numkey", mk([]Value{NewWord("a")}, []ReachSeg{{KeyLit: NewInteger(0)}}), "a.0"},
		{"computed", mk([]Value{NewWord("m")}, []ReachSeg{{Computed: true, KeyExpr: []Value{NewWord("k")}}}), "m.(k)"},
		{"parenrecv", mk([]Value{NewParenExpr([]Value{NewWord("m"), NewWord("a")})}, []ReachSeg{{KeyLit: NewWord("b")}}), "(m a).b"},
		// Receiverless reach (a detached lens) renders with the reserved
		// `$` sentinel receiver, so `$.a.b` round-trips back to a lens.
		{"receiverless", mk(nil, []ReachSeg{{KeyLit: NewWord("a")}, {KeyLit: NewWord("b")}}), "$.a.b"},
		{"receiverless-getr", mk(nil, []ReachSeg{{Getr: true, KeyLit: NewWord("x")}}), "$!.x"},
	}
	for _, c := range cases {
		if got := CanonValue(c.v); got != c.want {
			t.Errorf("%s: canon = %q, want %q", c.name, got, c.want)
		}
	}
}

// A Quoted (codequote-captured) reach wraps in (codequote …) so it round-trips.
func TestReachCanonQuotedWraps(t *testing.T) {
	v := NewReach(ReachInfo{Receiver: []Value{NewWord("m")}, Segments: []ReachSeg{{KeyLit: NewWord("a")}}, Eval: true})
	v.Quoted = true
	if got := CanonValue(v); got != "(codequote m.a)" {
		t.Errorf("quoted reach canon = %q, want %q", got, "(codequote m.a)")
	}
}
