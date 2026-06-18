package eng

import "testing"

// The fn-frame tape protocol (fn_frame.go) is what every body-splice
// path builds frames from; these tests pin the marked open paren's
// predicates and the canonical tail shape so a probe scanning frames
// (design/TCO-STAGED.10.md Stage 2) has an asserted contract to match.

func TestFrameOpenPredicates(t *testing.T) {
	meta := &FnFrameMeta{Name: "f"}
	fo := NewFrameOpen(meta)

	if !IsOpenParen(fo) {
		t.Error("NewFrameOpen must remain an ordinary OpenParen structurally")
	}
	if !IsFrameOpen(fo) {
		t.Error("NewFrameOpen not recognised by IsFrameOpen")
	}
	if fo.String() != "(" {
		t.Errorf("frame open renders %q, want \"(\" (payload must not leak into rendering)", fo.String())
	}

	info, err := AsFrameOpen(fo)
	if err != nil {
		t.Fatalf("AsFrameOpen: %v", err)
	}
	if info.Meta != meta {
		t.Error("AsFrameOpen did not round-trip the meta pointer")
	}

	// Negative cases: plain parens and non-parens are NOT frame opens.
	if IsFrameOpen(NewOpenParen()) {
		t.Error("a plain grouping paren must not read as a frame open")
	}
	if IsFrameOpen(NewCloseParen()) {
		t.Error("a close paren must not read as a frame open")
	}
	if IsFrameOpen(NewWord("(")) {
		t.Error("a word must not read as a frame open")
	}
	if _, err := AsFrameOpen(NewOpenParen()); err == nil {
		t.Error("AsFrameOpen on a plain paren must error")
	}
}

func TestAppendFrameTailShape(t *testing.T) {
	r, err := NewRegistry()
	if err != nil {
		t.Fatal(err)
	}
	snap := map[string]int{"x": 1}

	tail := AppendFrameTail(nil, FrameTailSpec{
		Registry:     r,
		Snapshot:     snap,
		Names:        []string{"a", "b"}, // install order; undefs must reverse
		Returns:      []*Type{TInteger},
		UnnamedCount: 1,
		FuncName:     "f",
	})

	// Expected: __DC  __pa  undef b  undef a  __RC
	if len(tail) != 7 {
		t.Fatalf("tail has %d tokens, want 7: %v", len(tail), tail)
	}
	if !IsDefCleanup(tail[0]) {
		t.Errorf("tail[0] = %v, want DefCleanup", tail[0])
	}
	dc, _ := AsDefCleanup(tail[0])
	if dc.Snapshot["x"] != 1 || dc.Registry != r {
		t.Error("DefCleanup does not carry the supplied snapshot/registry")
	}
	w, err := AsWord(tail[1])
	if err != nil || w.Name != "__pa" {
		t.Errorf("tail[1] = %v, want word __pa", tail[1])
	}
	// undef pairs in REVERSE install order, force-forward so undef
	// takes the name word rather than a same-typed stack value.
	wantNames := []string{"b", "a"}
	for i, name := range wantNames {
		u, err := AsWord(tail[2+2*i])
		if err != nil || u.Name != "undef" || !u.ForceForward {
			t.Errorf("tail[%d] = %v, want force-forward word undef", 2+2*i, tail[2+2*i])
		}
		n, err := AsWord(tail[3+2*i])
		if err != nil || n.Name != name {
			t.Errorf("tail[%d] = %v, want word %s", 3+2*i, tail[3+2*i], name)
		}
	}
	if !IsReturnCheck(tail[6]) {
		t.Fatalf("tail[6] = %v, want ReturnCheck", tail[6])
	}
	rc, _ := AsReturnCheck(tail[6])
	if rc.FuncName != "f" || rc.UnnamedCount != 1 || len(rc.Returns) != 1 || !rc.Returns[0].Equal(TInteger) {
		t.Errorf("ReturnCheck fields wrong: %+v", rc)
	}
	if rc.Pos.Row != 0 {
		t.Errorf("unstamped tail must leave Pos zero for execMatch's call-site stamping, got %+v", rc.Pos)
	}

	// No declared returns ⇒ no ReturnCheck, and no undef pairs when no
	// names were installed.
	bare := AppendFrameTail(nil, FrameTailSpec{Registry: r, Snapshot: snap})
	if len(bare) != 2 || !IsDefCleanup(bare[0]) {
		t.Fatalf("bare tail = %v, want exactly [__DC __pa]", bare)
	}
	if w, err := AsWord(bare[1]); err != nil || w.Name != "__pa" {
		t.Fatalf("bare tail[1] = %v, want word __pa", bare[1])
	}
}

func TestPopFrameArgsPairsArgsAndBaseline(t *testing.T) {
	r, err := NewRegistry()
	if err != nil {
		t.Fatal(err)
	}
	r.PushFnBaseline(r.Defs.Snapshot())
	if err := r.Args.Push(NewList(nil)); err != nil {
		t.Fatal(err)
	}
	if r.TopFnBaseline() == nil {
		t.Fatal("baseline not pushed")
	}
	if err := PopFrameArgs(r); err != nil {
		t.Fatalf("PopFrameArgs: %v", err)
	}
	if r.TopFnBaseline() != nil {
		t.Error("PopFrameArgs did not pop the FnBaseline alongside the Args entry")
	}
	if _, ok, _ := r.Args.Top(); ok {
		t.Error("PopFrameArgs did not pop the Args entry")
	}
}
