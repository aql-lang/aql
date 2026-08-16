package core

import "testing"

// fnReturnPark decides whether a collapsing paren is a fn FRAME delivering a
// Function as its RETURN, in which case stepCloseParen's rewind steps PAST the
// value instead of re-stepping it into a call
// (design/FUNCTION-VALUE-SCOPE.0.md §12.6).
//
// The end-to-end behaviour is pinned above this module — `lang/spec/ref.tsv`
// §8 and `lang/go/fn_return_park_test.go`, on both engines. This test exists
// for the arm ITSELF: core/go is gated by its OWN suite at a 100% floor
// (`make cover-gate-core`, core/go/CLAUDE.md), so a decision function reached
// only from above reads as uncovered there however well the merged profile
// scores it. Same failure mode, and same remedy, as commit cdc7a11's
// canon_stage5_test.go.
//
// Each row also carries a reason, because every `return 0` here is a
// deliberate negative: the whole risk of this change is a park that fires one
// case too wide and swallows a value the main loop still owes work on.
func TestFnReturnPark(t *testing.T) {
	e := &Engine{}

	fnv := NewFunction(FnDefInfo{Name: "parked"})

	quoted := NewFunction(FnDefInfo{Name: "quoted"})
	quoted.Quoted = true

	// A Function value REPARENTED to a refined function type. stepLiteral's
	// dispatch guard requires Parent == TFunction, so this one would be
	// PUSHED, not called — parking it would drop its OnPushLit.
	refined := ReparentValue(NewFunction(FnDefInfo{Name: "refined"}), TAny)

	cases := []struct {
		name      string
		tape      []Value
		closeIdx  int
		frameOpen bool
		want      int
		why       string
	}{
		{
			name: "fn frame returning a Function parks", tape: []Value{fnv},
			closeIdx: 2, frameOpen: true, want: 1,
			why: "the frame is delivering a return, which is not a fresh use of the value",
		},
		{
			name: "user paren re-steps", tape: []Value{fnv},
			closeIdx: 2, frameOpen: false, want: 0,
			why: "only a fn FRAME parks; a user paren is a fresh use (lang/spec/ref.tsv §2)",
		},
		{
			name: "quoted Function is data already", tape: []Value{quoted},
			closeIdx: 2, frameOpen: true, want: 0,
			why: "a Quoted value would not have dispatched on re-step, so there is nothing to suppress",
		},
		{
			name: "reparented Function is pushed, not called", tape: []Value{refined},
			closeIdx: 2, frameOpen: true, want: 0,
			why: "stepLiteral requires Parent == TFunction to dispatch; parking this loses its OnPushLit",
		},
		{
			name: "non-Function survivor is untouched", tape: []Value{NewInteger(42)},
			closeIdx: 2, frameOpen: true, want: 0,
			why: "the park applies only to a Function residual — every ordinary call returns through here",
		},
		{
			name: "more than one survivor is untouched", tape: []Value{fnv, NewInteger(1)},
			closeIdx: 3, frameOpen: true, want: 0,
			why: "the park is the exactly-one-survivor case; stepping past a multi-value collapse strands the rest",
		},
		{
			name: "empty collapse is untouched", tape: []Value{},
			closeIdx: 2, frameOpen: true, want: 0,
			why: "nothing survived, so idx is past the tape end — the bounds guard, not a value decision",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			e.Tape = NewTape(tc.tape, 4)
			if got := e.fnReturnPark(0, tc.closeIdx, tc.frameOpen); got != tc.want {
				t.Errorf("fnReturnPark = %d, want %d — %s", got, tc.want, tc.why)
			}
		})
	}
}

// IsFrameOpen is the ONLY thing telling a fn frame's collapse from a user
// paren's, so fnReturnPark's whole correctness rests on it being unforgeable
// from source text. Pin that here: a parser-produced open paren must not read
// as a frame, and both machine-generated constructors must.
func TestFrameOpenIsMachineGeneratedOnly(t *testing.T) {
	if IsFrameOpen(NewOpenParen()) {
		t.Error("a plain source-written open paren reads as a fn frame — the park would fire on user parens")
	}
	meta := &FnFrameMeta{}
	if !IsFrameOpen(NewFrameOpen(meta)) {
		t.Error("NewFrameOpen does not read as a fn frame — the park would never fire")
	}
	if !IsFrameOpen(NewFrameOpenSpan(meta, 2)) {
		t.Error("NewFrameOpenSpan does not read as a fn frame — frames with a resolved-arg span would not park")
	}
}
